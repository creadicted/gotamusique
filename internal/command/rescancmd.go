package command

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/konradk/gotamusique/internal/library"
	"layeh.com/gumble/gumble"
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
func handleRescan(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	cfg := bot.Config()
	if !bot.HasLibrary() {
		reply(cfg, msg, "library unavailable (set <code>[files] music_folder</code>)", "library unavailable (set [files] music_folder)")
		return
	}
	sendToChannel(msg, "Scanning…")
	scanWG.Add(1)
	go func() {
		defer scanWG.Done()
		ctx, cancel := context.WithTimeout(context.Background(), rescanTimeout)
		defer cancel()
		summary, err := bot.Rescan(ctx)
		if err != nil {
			text := "Scan failed: " + err.Error()
			switch {
			case errors.Is(err, library.ErrScanRunning):
				text = "scan already running"
			case errors.Is(err, library.ErrUnavailable):
				text = "library unavailable"
			case errors.Is(err, library.ErrFolderUnavailable):
				text = "Scan failed: music folder is not available; nothing removed"
			case errors.Is(err, library.ErrNoAudioFound):
				text = err.Error()
			}
			reply(cfg, msg, esc(text), text)
			return
		}
		reply(cfg, msg, "Done: "+esc(summary), "Done: "+summary)
	}()
}
