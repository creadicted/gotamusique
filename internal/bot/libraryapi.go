package bot

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"

	"github.com/konradk/gotamusique/internal/library"
	"github.com/konradk/gotamusique/internal/web"
)

// Upload stores r as a new audio file for id in the library and returns its entry.
// The size limit and all validation live in library.Scanner.Store.
func (b *Bot) Upload(ctx context.Context, id web.Identity, filename string, r io.Reader) (library.Track, error) {
	lib := b.libraryRef()
	if lib == nil {
		return library.Track{}, library.ErrUnavailable
	}
	rec, err := lib.scanner.Store(ctx, id.Name, filename, r, b.cfg.Web.MaxUploadBytes)
	if err != nil {
		return library.Track{}, err
	}
	b.log.Info("file uploaded", slog.String("user", id.Name), slog.String("path", rec.Path))
	return toLibraryTrack(*rec), nil
}

// Library lists the library for the web API; see web.LibraryQuery for the rules.
func (b *Bot) Library(q web.LibraryQuery) (web.LibraryPage, error) {
	lib := b.libraryRef()
	if lib == nil {
		return web.LibraryPage{}, library.ErrUnavailable
	}
	tags, err := lib.db.AllTags()
	if err != nil {
		return web.LibraryPage{}, fmt.Errorf("reading library: %w", err)
	}
	sort.Strings(tags)
	page := web.LibraryPage{Tags: tags}

	filtering := q.Search != "" || len(q.Tags) > 0
	// One pass over every track, in Go. The database's keyword column only holds
	// title and artist (path words are added just for untitled files), so asking it
	// would miss a query that only matches the path, such as a folder name.
	tracks, err := b.ListLibrary(nil)
	if err != nil {
		return page, err
	}
	if filtering {
		words := strings.Fields(strings.ToLower(q.Search))
		kept := tracks[:0:0]
		for _, t := range tracks {
			if matchesAll(strings.ToLower(t.Path+" "+t.Title+" "+t.Artist), words) && hasAllTags(t.Tags, q.Tags) {
				kept = append(kept, t)
			}
		}
		tracks = kept
	}
	sort.SliceStable(tracks, func(i, j int) bool { return tracks[i].Path < tracks[j].Path })

	prefix := ""
	if q.Dir != "" {
		prefix = q.Dir + "/"
	}
	var here []library.Track
	dirCount := map[string]int{}
	for _, t := range tracks {
		if !strings.HasPrefix(t.Path, prefix) {
			continue
		}
		rest := t.Path[len(prefix):]
		if filtering {
			here = append(here, t) // a search looks at every level below dir
			continue
		}
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			dirCount[rest[:i]]++
		} else {
			here = append(here, t)
		}
	}
	for name, n := range dirCount {
		page.Dirs = append(page.Dirs, web.LibraryDir{Name: name, Path: prefix + name, Tracks: n})
	}
	sort.Slice(page.Dirs, func(i, j int) bool {
		a, b := strings.ToLower(page.Dirs[i].Name), strings.ToLower(page.Dirs[j].Name)
		if a != b {
			return a < b
		}
		return page.Dirs[i].Name < page.Dirs[j].Name
	})

	page.Total = len(here)
	lo := min(max(q.Offset, 0), len(here))
	hi := len(here)
	if q.Limit > 0 {
		hi = min(lo+q.Limit, len(here))
	}
	page.Tracks = here[lo:hi]
	return page, nil
}

// matchesAll reports whether every word occurs in text (both already lower-case).
func matchesAll(text string, words []string) bool {
	for _, w := range words {
		if !strings.Contains(text, w) {
			return false
		}
	}
	return true
}

// hasAllTags reports whether have contains every wanted tag as a whole tag, ignoring
// case ("rock" does not match "hardrock").
func hasAllTags(have, want []string) bool {
	for _, w := range want {
		found := false
		for _, h := range have {
			if strings.EqualFold(h, w) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
