package command

import (
	"errors"
	"testing"

	"github.com/konradk/gotamusique/internal/library"
)

func TestHandleRescan_NoLibrary_DoesNotScan(t *testing.T) {
	bot := defaultBot() // hasLibrary = false
	callHandler(handleRescan, bot, "")
	scanWG.Wait()
	if bot.rescanCalls != 0 {
		t.Errorf("rescanCalls = %d, want 0 without a library", bot.rescanCalls)
	}
}

func TestHandleRescan_WithLibrary_RunsOneScan(t *testing.T) {
	bot := defaultBot()
	bot.hasLibrary = true
	bot.rescanResult = "3 added, 0 updated, 1 removed"
	callHandler(handleRescan, bot, "")
	scanWG.Wait()
	if bot.rescanCalls != 1 {
		t.Errorf("rescanCalls = %d, want 1", bot.rescanCalls)
	}
}

func TestHandleRescan_IgnoresArguments(t *testing.T) {
	bot := defaultBot()
	bot.hasLibrary = true
	callHandler(handleRescan, bot, "force --all /etc")
	scanWG.Wait()
	if bot.rescanCalls != 1 {
		t.Errorf("rescanCalls = %d, want 1 (there is no force option)", bot.rescanCalls)
	}
}

func TestHandleRescan_ScanErrorsDoNotPanic(t *testing.T) {
	for name, err := range map[string]error{
		"already running": library.ErrScanRunning,
		"folder missing":  library.ErrFolderUnavailable,
		"no audio":        library.ErrNoAudioFound,
		"no library":      library.ErrUnavailable,
		"other":           errors.New("disk on fire"),
	} {
		bot := defaultBot()
		bot.hasLibrary = true
		bot.rescanErr = err
		callHandler(handleRescan, bot, "")
		scanWG.Wait()
		if bot.rescanCalls != 1 {
			t.Errorf("%s: rescanCalls = %d", name, bot.rescanCalls)
		}
	}
}

func TestHandleRescan_OpenToNonAdminsAndRegistered(t *testing.T) {
	d := NewDispatcher([]string{"!"})
	RegisterAll(defaultBot(), d)
	e, ok := d.handlers["rescan"]
	if !ok {
		t.Fatal("rescan command not registered")
	}
	if e.adminOnly {
		t.Error("!rescan must be open to all users")
	}
}

func TestHandleRescan_ReturnsImmediately_NotBlockedByScan(t *testing.T) {
	bot := defaultBot()
	bot.hasLibrary = true
	block := make(chan struct{})
	bot.rescanBlock = block
	done := make(chan struct{})
	go func() {
		callHandler(handleRescan, bot, "")
		close(done)
	}()
	select {
	case <-done: // handler returned while the scan is still blocked: it runs in a goroutine
	case <-timeAfterSecs(3):
		t.Fatal("the handler blocked on the scan; it must run it in a goroutine so the event loop stays free")
	}
	close(block)
	scanWG.Wait()
}
