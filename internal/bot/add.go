package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/konradk/gotamusique/internal/audio"
	"github.com/konradk/gotamusique/internal/library"
	"github.com/konradk/gotamusique/internal/media/file"
	urlitem "github.com/konradk/gotamusique/internal/media/url"
	"github.com/konradk/gotamusique/internal/queue"
	"github.com/konradk/gotamusique/internal/radio"
	"github.com/konradk/gotamusique/internal/web"
)

// Add validates src, queues it at pos and returns the new entry. It does the
// network checks (is the stream reachable, yt-dlp metadata) before queueing and
// gives up when ctx ends. Problems with the source itself come back wrapped in
// web.ErrBadSource with a message that is safe to show to the user.
func (b *Bot) Add(ctx context.Context, src web.Source, pos queue.Position) (web.Added, error) {
	item, err := b.itemFor(ctx, src)
	if err != nil {
		return web.Added{}, err
	}
	id := b.queue.Add(item, pos)
	return web.Added{ID: id, Title: item.FormatTitle()}, nil
}

func badSource(format string, args ...any) error {
	return fmt.Errorf("%w: %s", web.ErrBadSource, fmt.Sprintf(format, args...))
}

func (b *Bot) itemFor(ctx context.Context, src web.Source) (audio.MediaItem, error) {
	switch src.Type {
	case web.SourceLibrary:
		return b.libraryItem(src.Path)
	case web.SourceRadio:
		return b.radioItem(ctx, src)
	case web.SourceURL:
		return b.urlItem(ctx, src.URL)
	default:
		return nil, badSource("unknown source type %q (use library, radio or url)", src.Type)
	}
}

// libraryItem queues one scanned file, addressed by its library path. The path is
// only used as a database key and the file is then re-checked to lie inside
// music_folder, so it cannot name anything outside the library.
func (b *Bot) libraryItem(path string) (audio.MediaItem, error) {
	lib := b.libraryRef()
	if lib == nil {
		return nil, library.ErrUnavailable
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, badSource("path is required")
	}
	rec, err := lib.db.QueryByID(file.IDFromPath(path))
	if err != nil {
		return nil, fmt.Errorf("reading library: %w", err)
	}
	if rec == nil || rec.Type != library.RecordType || rec.Path != path {
		return nil, library.ErrNotInLibrary
	}
	item, err := library.NewItem(toLibraryTrack(*rec), b.cfg.Files.MusicFolder)
	if err != nil {
		return nil, badSource("%v", err)
	}
	return item, nil
}

func (b *Bot) radioItem(ctx context.Context, src web.Source) (audio.MediaItem, error) {
	given := 0
	for _, v := range []string{src.Preset, src.URL, src.UUID} {
		if v != "" {
			given++
		}
	}
	if given != 1 {
		return nil, badSource("give exactly one of preset, url or uuid")
	}

	switch {
	case src.Preset != "":
		preset, ok := b.cfg.Radio[src.Preset]
		if !ok {
			return nil, badSource("unknown radio preset %q", src.Preset)
		}
		return radio.NewRadioItemFromPreset(src.Preset, preset), nil

	case src.URL != "":
		if u, err := url.Parse(src.URL); err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, badSource("not an http(s) URL")
		}
		item := radio.NewRadioItemFromURL(src.URL)
		// Validate has no context of its own; bound how long we wait for it.
		if _, err := withContext(ctx, func() (struct{}, error) { return struct{}{}, item.Validate() }); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, badSource("could not reach the stream: %v", err)
		}
		return item, nil

	default:
		st, err := withContext(ctx, func() (*radio.Station, error) { return b.radioBrowser().ByUUID(src.UUID) })
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, badSource("station not found: %v", err)
		}
		return radio.NewRadioItemFromStation(*st), nil
	}
}

func (b *Bot) urlItem(ctx context.Context, raw string) (audio.MediaItem, error) {
	if err := urlitem.CheckURL(raw); err != nil {
		return nil, badSource("%v", err)
	}
	item := urlitem.New(raw, b.cfg)
	if err := item.ValidateCtx(ctx); err != nil {
		switch {
		case errors.Is(err, urlitem.ErrBusy):
			// Checked before the context: running out of time while waiting for a
			// yt-dlp slot is "busy", not a slow site.
			return nil, web.ErrBusy
		case ctx.Err() != nil:
			return nil, ctx.Err()
		case errors.Is(err, urlitem.ErrIsPlaylist):
			return nil, badSource("that is a playlist; queue single tracks")
		case errors.Is(err, urlitem.ErrBinaryMissing):
			return nil, err // a server problem, not the user's
		}
		return nil, badSource("%v", err)
	}
	return item, nil
}

// withContext runs fn and returns its result, or ctx's error as soon as ctx ends.
// fn keeps running after that (the calls it wraps cannot be cancelled), but they
// are bounded by their own timeouts.
func withContext[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	type result struct {
		v   T
		err error
	}
	ch := make(chan result, 1)
	go func() {
		v, err := fn()
		ch <- result{v, err}
	}()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

func (b *Bot) radioBrowser() *radio.RadioBrowser {
	b.rbOnce.Do(func() { b.rb = radio.NewRadioBrowser() })
	return b.rb
}

// SearchRadio queries radio-browser.info for stations matching query.
func (b *Bot) SearchRadio(ctx context.Context, query string, limit int) ([]radio.Station, error) {
	return withContext(ctx, func() ([]radio.Station, error) { return b.radioBrowser().Search(query, limit) })
}

// rescanTimeout bounds one background scan; a first scan of a big library is slow.
const rescanTimeout = 2 * time.Hour

// StartRescan starts an incremental library scan in the background and returns at
// once. It returns library.ErrScanRunning or library.ErrUnavailable, starting
// nothing. How the scan ended shows up in the snapshot (Scan.LastResult/LastError).
func (b *Bot) StartRescan() error {
	lib := b.libraryRef()
	if lib == nil {
		return library.ErrUnavailable
	}
	if !lib.bgScan.CompareAndSwap(false, true) {
		return library.ErrScanRunning
	}
	if lib.scanner.Running() { // a !rescan from chat
		lib.bgScan.Store(false)
		return library.ErrScanRunning
	}
	go func() {
		defer lib.bgScan.Store(false)
		ctx, cancel := context.WithTimeout(b.life, rescanTimeout)
		defer cancel()
		if _, err := b.Rescan(ctx, false); err != nil && !errors.Is(err, library.ErrScanRunning) {
			b.log.Warn("background library scan failed", slog.String("err", err.Error()))
		}
	}()
	return nil
}
