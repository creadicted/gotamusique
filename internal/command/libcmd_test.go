package command

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/konradk/gotamusique/internal/audio"
	"github.com/konradk/gotamusique/internal/library"
	fileitem "github.com/konradk/gotamusique/internal/media/file"
	"github.com/konradk/gotamusique/internal/radio"
)

func TestParsePageFlag(t *testing.T) {
	cases := []struct {
		in   string
		rest string
		page int
		ok   bool
	}{
		{"", "", 1, true},
		{"beethoven", "beethoven", 1, true},
		{"beethoven -p 2", "beethoven", 2, true},
		{"-p 3 beethoven", "beethoven", 3, true},
		{"symphony no 9", "symphony no 9", 1, true}, // trailing number is part of the query
		{"symphony no 9 -p 2", "symphony no 9", 2, true},
		{"a-p b", "a-p b", 1, true}, // -p inside a word is not the flag
		{"x -p", "x", 0, false},
		{"x -p 0", "x", 0, false},
		{"x -p abc", "x", 0, false},
	}
	for _, c := range cases {
		rest, page, ok := parsePageFlag(c.in)
		if ok != c.ok || (ok && (rest != c.rest || page != c.page)) {
			t.Errorf("parsePageFlag(%q) = (%q, %d, %v), want (%q, %d, %v)", c.in, rest, page, ok, c.rest, c.page, c.ok)
		}
	}
}

func TestPaginate_RespectsBudgetAndKeepsAllLines(t *testing.T) {
	var lines []string
	for i := 0; i < 400; i++ {
		lines = append(lines, fmt.Sprintf("dir/<track & %03d>.mp3 - %s", i, strings.Repeat("x", 60)))
	}
	pages := paginate(lines)
	if len(pages) < 2 {
		t.Fatalf("pages = %d, want several", len(pages))
	}
	total := 0
	for _, p := range pages {
		size := 0
		for _, l := range p {
			size += len(esc(l)) + 1
		}
		if size > pageBudget {
			t.Errorf("page size %d exceeds budget %d", size, pageBudget)
		}
		total += len(p)
	}
	if total != len(lines) {
		t.Errorf("lines across pages = %d, want %d", total, len(lines))
	}
}

func TestPaginate_Empty(t *testing.T) {
	if got := paginate(nil); len(got) != 0 {
		t.Errorf("paginate(nil) = %v", got)
	}
}

func TestListFile_NoLibrary(t *testing.T) {
	bot := defaultBot()
	callHandler(makeListFileHandler(newShortlist()), bot, "")
	if bot.listRE != nil || bot.libTracks != nil {
		t.Error("must not query without a library")
	}
}

func TestListFile_PassesCaseInsensitiveRegex(t *testing.T) {
	bot := defaultBot()
	bot.hasLibrary = true
	callHandler(makeListFileHandler(newShortlist()), bot, "Beet.*")
	if bot.listRE == nil || !bot.listRE.MatchString("beethoven") {
		t.Errorf("regex = %v, want case-insensitive Beet.*", bot.listRE)
	}
}

func TestListFile_NoArgListsEverything(t *testing.T) {
	bot := defaultBot()
	bot.hasLibrary = true
	callHandler(makeListFileHandler(newShortlist()), bot, "")
	if bot.listRE != nil {
		t.Errorf("regex = %v, want nil", bot.listRE)
	}
}

func TestListFile_InvalidRegexDoesNotQuery(t *testing.T) {
	bot := defaultBot()
	bot.hasLibrary = true
	bot.libTracks = []library.Track{{Path: "a.mp3"}}
	callHandler(makeListFileHandler(newShortlist()), bot, "([")
	if bot.listRE != nil {
		t.Error("invalid regex must not reach the library")
	}
}

func TestSearch_SplitsKeywordsAndPage(t *testing.T) {
	bot := defaultBot()
	bot.hasLibrary = true
	callHandler(makeSearchHandler(newShortlist()), bot, "beethoven symphony -p 2")
	if strings.Join(bot.searchKW, ",") != "beethoven,symphony" {
		t.Errorf("keywords = %v", bot.searchKW)
	}
}

func TestSearch_NoKeywordsShowsUsage(t *testing.T) {
	bot := defaultBot()
	bot.hasLibrary = true
	callHandler(makeSearchHandler(newShortlist()), bot, "  ")
	if bot.searchKW != nil {
		t.Error("empty search must not query")
	}
}

func TestLibraryCommands_ErrorsDoNotPanic(t *testing.T) {
	for _, err := range []error{library.ErrUnavailable, errors.New("boom")} {
		bot := defaultBot()
		bot.hasLibrary = true
		bot.libErr = err
		callHandler(makeListFileHandler(newShortlist()), bot, "")
		callHandler(makeSearchHandler(newShortlist()), bot, "x")
	}
}

func TestTruncateRunes_KeepsMultibyteIntact(t *testing.T) {
	got := truncateRunes(strings.Repeat("é", 100), 10)
	if want := strings.Repeat("é", 7) + "..."; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if truncateRunes("short", 10) != "short" {
		t.Error("short strings must be unchanged")
	}
}

func TestPaging_PageCount(t *testing.T) {
	// Out-of-range and malformed pages must not panic.
	tracks := make([]library.Track, 120)
	for i := range tracks {
		tracks[i] = library.Track{Path: fmt.Sprintf("t%03d.mp3", i)}
	}
	bot := defaultBot()
	bot.hasLibrary = true
	bot.libTracks = tracks
	for _, arg := range []string{"", "-p 2", "-p 3", "-p 99", "-p"} {
		callHandler(makeListFileHandler(newShortlist()), bot, arg) // must not panic
	}
}

// ---- shortlist and tags ----

func tracks3() []library.Track {
	return []library.Track{{Path: "a.mp3", Title: "A"}, {Path: "b.mp3", Title: "B"}, {Path: "c.mp3", Title: "C"}}
}

func shortlistBot(t *testing.T) (*mockBot, *shortlist) {
	t.Helper()
	bot := defaultBot()
	bot.hasLibrary = true
	bot.cfg.Files.MusicFolder = t.TempDir()
	for _, n := range []string{"a.mp3", "b.mp3", "c.mp3"} {
		os.WriteFile(filepath.Join(bot.cfg.Files.MusicFolder, n), []byte("x"), 0o644)
	}
	return bot, newShortlist()
}

func enqueuedPaths(b *mockBot) string {
	var p []string
	for _, it := range b.enqueueCalls {
		p = append(p, it.(*fileitem.FileItem).Path)
	}
	return strings.Join(p, ",")
}

func TestShortlist_NoListYet(t *testing.T) {
	bot, sl := shortlistBot(t)
	callHandler(makeShortlistHandler(sl), bot, "1")
	if len(bot.enqueueCalls) != 0 {
		t.Error("nothing to pick from must queue nothing")
	}
}

func TestShortlist_PicksFromLastListing(t *testing.T) {
	bot, sl := shortlistBot(t)
	bot.libTracks = tracks3()
	callHandler(makeListFileHandler(sl), bot, "")
	callHandler(makeShortlistHandler(sl), bot, "3 1")
	if got := enqueuedPaths(bot); got != "c.mp3,a.mp3" {
		t.Errorf("queued %q, want c.mp3,a.mp3", got)
	}
}

func TestShortlist_Star_QueuesAll(t *testing.T) {
	bot, sl := shortlistBot(t)
	bot.libTracks = tracks3()
	callHandler(makeSearchHandler(sl), bot, "x")
	callHandler(makeShortlistHandler(sl), bot, "*")
	if got := enqueuedPaths(bot); got != "a.mp3,b.mp3,c.mp3" {
		t.Errorf("queued %q", got)
	}
}

func TestShortlist_BadNumberQueuesNothing(t *testing.T) {
	bot, sl := shortlistBot(t)
	bot.libTracks = tracks3()
	callHandler(makeListFileHandler(sl), bot, "")
	for _, arg := range []string{"1 9", "0", "x", "", "* 1", "-1"} {
		callHandler(makeShortlistHandler(sl), bot, arg)
	}
	if len(bot.enqueueCalls) != 0 {
		t.Errorf("queued %q for bad input", enqueuedPaths(bot))
	}
}

func TestShortlist_MissingFileIsSkipped(t *testing.T) {
	bot, sl := shortlistBot(t)
	bot.libTracks = append(tracks3(), library.Track{Path: "gone.mp3"})
	callHandler(makeListFileHandler(sl), bot, "")
	callHandler(makeShortlistHandler(sl), bot, "*")
	if got := enqueuedPaths(bot); got != "a.mp3,b.mp3,c.mp3" {
		t.Errorf("queued %q, want the three existing files", got)
	}
}

func TestShortlist_EmptyResultDoesNotReplaceOldList(t *testing.T) {
	bot, sl := shortlistBot(t)
	bot.libTracks = tracks3()
	callHandler(makeListFileHandler(sl), bot, "")
	bot.libTracks = nil
	callHandler(makeSearchHandler(sl), bot, "nothing")
	callHandler(makeShortlistHandler(sl), bot, "1")
	if enqueuedPaths(bot) != "a.mp3" {
		t.Errorf("a search with no hits must keep the previous list, queued %q", enqueuedPaths(bot))
	}
}

func TestShortlist_PerChannel(t *testing.T) {
	bot, sl := shortlistBot(t)
	bot.libTracks = tracks3()
	msgA := makeMsg("!list")
	msgA.Channels[0].ID = 1
	makeListFileHandler(sl)(bot, chatCtx(msgA), "list", "")
	msgB := makeMsg("!sl 1")
	msgB.Channels[0].ID = 2
	makeShortlistHandler(sl)(bot, chatCtx(msgB), "sl", "1")
	if len(bot.enqueueCalls) != 0 {
		t.Error("channel 2 must not see channel 1's list")
	}
}

func TestShortlist_EvictsOldestChannel(t *testing.T) {
	sl := newShortlist()
	for i := 0; i <= maxCacheChannels; i++ {
		sl.set(uint32(i), tracks3())
	}
	if sl.get(0) != nil || sl.get(uint32(maxCacheChannels)) == nil {
		t.Error("oldest channel must be evicted, newest kept")
	}
}

func TestFindTagged_ParsesTags(t *testing.T) {
	bot, sl := shortlistBot(t)
	callHandler(makeFindTaggedHandler(sl), bot, " rock , live ,, ")
	if strings.Join(bot.findTags, "|") != "rock|live" {
		t.Errorf("tags = %v", bot.findTags)
	}
	bot.findTags = nil
	callHandler(makeFindTaggedHandler(sl), bot, " , ")
	if bot.findTags != nil {
		t.Error("blank tag list must not query")
	}
}

func TestPlayTag_QueuesTaggedTracks(t *testing.T) {
	bot, _ := shortlistBot(t)
	bot.libTracks = tracks3()[:2]
	callHandler(handlePlayTag, bot, "fav")
	if enqueuedPaths(bot) != "a.mp3,b.mp3" || strings.Join(bot.findTags, ",") != "fav" {
		t.Errorf("queued %q for tags %v", enqueuedPaths(bot), bot.findTags)
	}
}

func TestTagTarget(t *testing.T) {
	cases := []struct {
		arg     string
		n, cur  int
		idx     string
		rest    string
		wantErr bool
	}{
		{"fav", 3, 1, "1", "fav", false},   // current item
		{"2 fav", 3, 0, "1", "fav", false}, // queue position, 1-based
		{"* fav,live", 3, 0, "0,1,2", "fav,live", false},
		{"9 fav", 3, 0, "", "", true},      // out of range
		{"0 fav", 3, 0, "", "", true},      // 0 is not a queue position
		{"fav", 0, 0, "", "", true},        // empty queue
		{"2024", 3, 2, "2", "2024", false}, // lone number is a tag, not a position
	}
	for _, c := range cases {
		idx, rest, err := tagTarget(c.arg, c.n, c.cur)
		var s []string
		for _, i := range idx {
			s = append(s, strconv.Itoa(i))
		}
		if (err != nil) != c.wantErr || (err == nil && (strings.Join(s, ",") != c.idx || rest != c.rest)) {
			t.Errorf("tagTarget(%q, %d, %d) = %v %q %v; want %q %q err=%v", c.arg, c.n, c.cur, s, rest, err, c.idx, c.rest, c.wantErr)
		}
	}
}

func queueOf(t *testing.T, bot *mockBot, paths ...string) {
	t.Helper()
	for _, p := range paths {
		bot.queueItems = append(bot.queueItems, fileitem.New(p, bot.cfg.Files.MusicFolder))
	}
}

func TestAddTag_CurrentItem_AddsWithoutDuplicates(t *testing.T) {
	bot := defaultBot()
	bot.hasLibrary = true
	queueOf(t, bot, "a.mp3", "b.mp3")
	bot.queueIdx = 1
	callHandler(handleAddTag, bot, "Fav, live")
	callHandler(handleAddTag, bot, "fav")
	id := fileitem.IDFromPath("b.mp3")
	if got := strings.Join(bot.tagStore[id], ","); got != "Fav,live" {
		t.Errorf("tags = %q, want Fav,live (no case-insensitive duplicate)", got)
	}
	if len(bot.tagStore) != 1 {
		t.Errorf("only the current item may change: %v", bot.tagStore)
	}
}

func TestAddTag_ByPositionAndStar(t *testing.T) {
	bot := defaultBot()
	bot.hasLibrary = true
	queueOf(t, bot, "a.mp3", "b.mp3", "c.mp3")
	callHandler(handleAddTag, bot, "3 x")
	if len(bot.tagStore) != 1 || bot.tagStore[fileitem.IDFromPath("c.mp3")][0] != "x" {
		t.Errorf("by position: %v", bot.tagStore)
	}
	callHandler(handleAddTag, bot, "* y")
	if len(bot.tagStore) != 3 {
		t.Errorf("star must tag all three: %v", bot.tagStore)
	}
}

func TestAddTag_NonLibraryItemsAreSkipped(t *testing.T) {
	bot := defaultBot()
	bot.hasLibrary = true
	bot.queueItems = []audio.MediaItem{&radio.RadioItem{Name: "r", URL: "http://x"}}
	callHandler(handleAddTag, bot, "fav")
	if len(bot.tagStore) != 0 {
		t.Errorf("radio item must not be tagged: %v", bot.tagStore)
	}
}

func TestAddTag_BadInputChangesNothing(t *testing.T) {
	bot := defaultBot()
	bot.hasLibrary = true
	queueOf(t, bot, "a.mp3")
	for _, arg := range []string{"", "9 x", "*", "x,*", " , "} {
		callHandler(handleAddTag, bot, arg)
	}
	if len(bot.tagStore) != 0 {
		t.Errorf("tagged on bad input: %v", bot.tagStore)
	}
}

func TestUntag_RemovesNamedAndClearsAll(t *testing.T) {
	bot := defaultBot()
	bot.hasLibrary = true
	queueOf(t, bot, "a.mp3")
	id := fileitem.IDFromPath("a.mp3")
	bot.tagStore = map[string][]string{id: {"fav", "live", "rock"}}
	callHandler(handleUntag, bot, "FAV, rock")
	if got := strings.Join(bot.tagStore[id], ","); got != "live" {
		t.Errorf("tags = %q, want live", got)
	}
	callHandler(handleUntag, bot, "*")
	if len(bot.tagStore[id]) != 0 {
		t.Errorf("tags = %v, want cleared", bot.tagStore[id])
	}
}

func TestTagCommands_StoreErrorsDoNotPanic(t *testing.T) {
	bot := defaultBot()
	bot.hasLibrary = true
	bot.tagErr = errors.New("boom")
	queueOf(t, bot, "a.mp3")
	callHandler(handleAddTag, bot, "x")
	callHandler(handleUntag, bot, "x")
}

func TestTagCommands_NoLibrary(t *testing.T) {
	bot := defaultBot()
	queueOf(t, bot, "a.mp3")
	callHandler(handleAddTag, bot, "x")
	callHandler(handlePlayTag, bot, "x")
	if len(bot.tagStore) != 0 || len(bot.enqueueCalls) != 0 {
		t.Error("nothing may happen without a library")
	}
}
