package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/konradk/gotamusique/internal/audio"
	"github.com/konradk/gotamusique/internal/library"
	"github.com/konradk/gotamusique/internal/radio"
)

func libFileBot(t *testing.T, names ...string) *mockBot {
	t.Helper()
	bot := defaultBot()
	bot.hasLibrary = true
	bot.cfg.Files.MusicFolder = t.TempDir()
	for _, n := range names {
		p := filepath.Join(bot.cfg.Files.MusicFolder, n)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	return bot
}

func tr(paths ...string) []LibraryTrack {
	var out []LibraryTrack
	for _, p := range paths {
		out = append(out, LibraryTrack{Path: p, Title: p})
	}
	return out
}

func TestFile_Exact_QueuesOneFromLibrary(t *testing.T) {
	bot := libFileBot(t, "a/x.mp3")
	bot.lookup = FileLookup{Kind: LookupExact, Tracks: tr("a/x.mp3")}
	callHandler(makeFileHandler(newShortlist()), bot, "a/x.mp3")
	if enqueuedPaths(bot) != "a/x.mp3" || bot.lookupArg != "a/x.mp3" {
		t.Errorf("queued %q for lookup %q", enqueuedPaths(bot), bot.lookupArg)
	}
}

func TestFile_Folder_QueuesAllBelow(t *testing.T) {
	bot := libFileBot(t, "a/x.mp3", "a/y.mp3")
	bot.lookup = FileLookup{Kind: LookupFolder, Tracks: tr("a/x.mp3", "a/y.mp3")}
	callHandler(makeFileHandler(newShortlist()), bot, "a")
	if enqueuedPaths(bot) != "a/x.mp3,a/y.mp3" {
		t.Errorf("queued %q", enqueuedPaths(bot))
	}
}

func TestFile_PartialUnique_QueuesIt(t *testing.T) {
	bot := libFileBot(t, "a/x.mp3")
	bot.lookup = FileLookup{Kind: LookupPartial, Tracks: tr("a/x.mp3")}
	callHandler(makeFileHandler(newShortlist()), bot, "x.m")
	if enqueuedPaths(bot) != "a/x.mp3" {
		t.Errorf("queued %q", enqueuedPaths(bot))
	}
}

func TestFile_PartialMany_FillsShortlistInsteadOfQueueing(t *testing.T) {
	bot := libFileBot(t, "a/x.mp3", "b/x.mp3")
	bot.lookup = FileLookup{Kind: LookupPartial, Tracks: tr("a/x.mp3", "b/x.mp3")}
	sl := newShortlist()
	callHandler(makeFileHandler(sl), bot, "x")
	if len(bot.enqueueCalls) != 0 {
		t.Fatalf("queued %q, want a choice list", enqueuedPaths(bot))
	}
	callHandler(makeShortlistHandler(sl), bot, "2")
	if enqueuedPaths(bot) != "b/x.mp3" {
		t.Errorf("after !sl 2 queued %q", enqueuedPaths(bot))
	}
}

func TestFile_NoLibraryMatch_FallsBackToDisk(t *testing.T) {
	bot := libFileBot(t) // file not in the library and not on disk
	bot.lookup = FileLookup{Kind: LookupPartial}
	callHandler(makeFileHandler(newShortlist()), bot, "nothing.mp3")
	if len(bot.enqueueCalls) != 0 {
		t.Error("a missing file must not be queued")
	}
}

func TestFile_NoLibrary_UsesDiskHandler(t *testing.T) {
	bot := defaultBot() // hasLibrary false, no music_folder
	callHandler(makeFileHandler(newShortlist()), bot, "song.mp3")
	if bot.lookupArg != "" {
		t.Error("must not consult the library when there is none")
	}
}

func TestFile_LookupErrorDoesNotPanic(t *testing.T) {
	bot := libFileBot(t)
	bot.libErr = library.ErrUnavailable
	callHandler(makeFileHandler(newShortlist()), bot, "x")
	callHandler(makeFileHandler(newShortlist()), bot, "")
}

func TestFileMatch(t *testing.T) {
	bot := libFileBot(t, "a.mp3", "b.mp3")
	bot.libTracks = tr("a.mp3", "b.mp3")
	callHandler(handleFileMatch, bot, "(?i)A")
	if enqueuedPaths(bot) != "a.mp3,b.mp3" || bot.listRE == nil {
		t.Errorf("queued %q, re %v", enqueuedPaths(bot), bot.listRE)
	}
}

func TestFileMatch_BadInputQueuesNothing(t *testing.T) {
	bot := libFileBot(t, "a.mp3")
	bot.libTracks = tr("a.mp3")
	for _, arg := range []string{"", "([", "  "} {
		callHandler(handleFileMatch, bot, arg)
	}
	bot.libTracks = nil
	callHandler(handleFileMatch, bot, "zzz")
	if len(bot.enqueueCalls) != 0 {
		t.Errorf("queued %q", enqueuedPaths(bot))
	}
	nb := defaultBot()
	callHandler(handleFileMatch, nb, "x") // no library
}

func TestBulkAdd_CappedAndReported(t *testing.T) {
	names := make([]string, maxBulkAdd+25)
	for i := range names {
		names[i] = fmt.Sprintf("t%04d.mp3", i)
	}
	bot := libFileBot(t, names...)
	bot.libTracks = tr(names...)
	callHandler(handleFileMatch, bot, "t")
	if len(bot.enqueueCalls) != maxBulkAdd {
		t.Errorf("queued %d, want the cap %d", len(bot.enqueueCalls), maxBulkAdd)
	}
	if got := enqueuedPaths(bot); !strings.HasPrefix(got, "t0000.mp3,t0001.mp3") {
		t.Error("the first matches in order must be the ones queued")
	}
}

func TestLast(t *testing.T) {
	bot := defaultBot()
	callHandler(handleLast, bot, "")
	if bot.lastCalls != 1 {
		t.Errorf("lastCalls = %d", bot.lastCalls)
	}
}

func TestRandom_EmptyQueueDoesNothing(t *testing.T) {
	bot := defaultBot()
	callHandler(handleRandom, bot, "")
	if bot.randomizeCalls != 0 {
		t.Error("must not shuffle an empty queue")
	}
	bot.queueItems = []audio.MediaItem{&radio.RadioItem{Name: "r"}}
	callHandler(handleRandom, bot, "")
	if bot.randomizeCalls != 1 {
		t.Errorf("randomizeCalls = %d", bot.randomizeCalls)
	}
}

func TestRepeat(t *testing.T) {
	bot := defaultBot()
	bot.currentItem = &radio.RadioItem{Name: "r"}
	for arg, want := range map[string]int{"": 1, "3": 3, " 7 ": 7} {
		callHandler(handleRepeat, bot, arg)
		if bot.repeatN != want {
			t.Errorf("repeat %q -> %d, want %d", arg, bot.repeatN, want)
		}
	}
	bot.repeatN = 0
	for _, arg := range []string{"0", "-1", "abc", "201", "1 2"} {
		callHandler(handleRepeat, bot, arg)
	}
	if bot.repeatN != 0 {
		t.Errorf("bad input reached the bot: %d", bot.repeatN)
	}
	bot.currentItem = nil
	callHandler(handleRepeat, bot, "2") // nothing playing: must not panic
}
