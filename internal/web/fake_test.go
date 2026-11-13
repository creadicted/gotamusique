package web

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/konradk/gotamusique/internal/library"
	"github.com/konradk/gotamusique/internal/queue"
	"github.com/konradk/gotamusique/internal/radio"
	"github.com/konradk/gotamusique/internal/status"
)

// fakeBot records every call as a short string and returns the errors a test sets.
// The zero value is a working bot with an empty state.
type fakeBot struct {
	mu    sync.Mutex
	snap  status.Snapshot
	calls []string

	// errors returned by the matching method
	seekErr, modeErr, playErr, removeErr, moveErr, addErr, searchErr, rescanErr error

	added       Added
	addSrc      Source
	addPos      queue.Position
	addDeadline bool
	presets     []RadioPreset
	stations    []radio.Station
	searchQ     string
	searchTag   string
	searchLimit int

	libPage  LibraryPage
	libErr   error
	libQuery LibraryQuery

	uploadErr   error
	uploadTrack library.Track
	uploadName  string
	uploadBody  string
	uploadWho   Identity
	uploadRead  bool // read the body (false: reject without reading, like a bad name)
}

func (f *fakeBot) rec(format string, a ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf(format, a...))
}

func (f *fakeBot) called() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeBot) Snapshot() status.Snapshot { return f.snap }
func (f *fakeBot) Pause() bool               { f.rec("pause"); return true }
func (f *fakeBot) Resume() bool              { f.rec("resume"); return true }
func (f *fakeBot) Stop()                     { f.rec("stop") }
func (f *fakeBot) Skip()                     { f.rec("next") }
func (f *fakeBot) Clear()                    { f.rec("clear") }
func (f *fakeBot) RandomizeQueue()           { f.rec("shuffle") }
func (f *fakeBot) Mute()                     { f.rec("mute") }
func (f *fakeBot) Unmute()                   { f.rec("unmute") }
func (f *fakeBot) SetVolume(pct int)         { f.rec("volume %d", pct) }

func (f *fakeBot) Seek(pos time.Duration) error {
	f.rec("seek %dms", pos.Milliseconds())
	return f.seekErr
}
func (f *fakeBot) SetPlaybackMode(name string) error { f.rec("mode %s", name); return f.modeErr }
func (f *fakeBot) PlayEntry(id uint64) error         { f.rec("play %d", id); return f.playErr }
func (f *fakeBot) RemoveEntry(id uint64) error       { f.rec("remove %d", id); return f.removeErr }
func (f *fakeBot) MoveEntry(id, after uint64) error {
	f.rec("move %d after %d", id, after)
	return f.moveErr
}

func (f *fakeBot) Add(ctx context.Context, src Source, pos queue.Position) (Added, error) {
	f.mu.Lock()
	f.addSrc, f.addPos = src, pos
	_, f.addDeadline = ctx.Deadline()
	f.mu.Unlock()
	f.rec("add %s", src.Type)
	return f.added, f.addErr
}

func (f *fakeBot) SearchRadio(ctx context.Context, q, tag string, limit int) ([]radio.Station, error) {
	f.mu.Lock()
	f.searchQ, f.searchTag, f.searchLimit = q, tag, limit
	f.mu.Unlock()
	f.rec("search")
	return f.stations, f.searchErr
}

func (f *fakeBot) StartRescan() error { f.rec("rescan"); return f.rescanErr }

func (f *fakeBot) Library(q LibraryQuery) (LibraryPage, error) {
	f.mu.Lock()
	f.libQuery = q
	f.mu.Unlock()
	f.rec("library")
	return f.libPage, f.libErr
}

func (f *fakeBot) Upload(ctx context.Context, id Identity, filename string, r io.Reader) (library.Track, error) {
	f.rec("upload")
	f.mu.Lock()
	f.uploadName, f.uploadWho = filename, id
	f.mu.Unlock()
	if f.uploadRead {
		b, err := io.ReadAll(r)
		f.mu.Lock()
		f.uploadBody = string(b)
		f.mu.Unlock()
		if err != nil {
			return library.Track{}, fmt.Errorf("receiving the file: %w", err)
		}
	}
	return f.uploadTrack, f.uploadErr
}

func (f *fakeBot) RadioPresets() []RadioPreset { f.rec("presets"); return f.presets }
