package command

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/konradk/gotamusique/internal/library"
)

// scanWG tracks in-flight scans; tests wait on it.
var scanWG sync.WaitGroup

// rescanTimeout bounds one scan. A first scan of a large library is slow (it runs
// ffprobe on every file), so this is generous; later scans only stat files.
var rescanTimeout = 2 * time.Hour

// handleRescan scans the music folder into the library. It is open to everyone:
// the first scan is the only expensive one (later scans are incremental), scans
// are serialized, and restricting commands as a group is 3-11's job.
//
// Like !url it replies immediately and runs in a goroutine so a long scan never
// blocks gumble's event goroutine.
func handleRescan(bot BotAPI, c *Ctx, cmd, arg string) {
	cfg := bot.Config()
	if !bot.HasLibrary() {
		reply(cfg, c, "library unavailable (set <code>[files] music_folder</code>)", "library unavailable (set [files] music_folder)")
		return
	}
	force := strings.EqualFold(strings.TrimSpace(arg), "force")
	c.Send("Scanning…")
	scanWG.Add(1)
	go func() {
		defer scanWG.Done()
		ctx, cancel := context.WithTimeout(context.Background(), rescanTimeout)
		defer cancel()
		summary, err := bot.Rescan(ctx, force)
		if err != nil {
			text := libraryErrorText(err)
			reply(cfg, c, esc(text), text)
			return
		}
		reply(cfg, c, "Done: "+esc(summary), "Done: "+summary)
	}()
}

// libraryErrorText maps library errors to the text shown in chat; shared by
// !rescan and !mode so both word them the same way.
func libraryErrorText(err error) string {
	switch {
	case errors.Is(err, library.ErrScanRunning):
		return "scan already running"
	case errors.Is(err, library.ErrUnavailable):
		return "library unavailable"
	case errors.Is(err, library.ErrFolderUnavailable):
		return "Scan failed: music folder is not available; nothing removed"
	case errors.Is(err, library.ErrNoAudioFound), errors.Is(err, library.ErrEmpty):
		return err.Error()
	}
	return "Scan failed: " + err.Error()
}
