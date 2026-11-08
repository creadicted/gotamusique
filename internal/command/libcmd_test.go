package command

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/konradk/gotamusique/internal/library"
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
	callHandler(handleListFile, bot, "")
	if bot.listRE != nil || bot.libTracks != nil {
		t.Error("must not query without a library")
	}
}

func TestListFile_PassesCaseInsensitiveRegex(t *testing.T) {
	bot := defaultBot()
	bot.hasLibrary = true
	callHandler(handleListFile, bot, "Beet.*")
	if bot.listRE == nil || !bot.listRE.MatchString("beethoven") {
		t.Errorf("regex = %v, want case-insensitive Beet.*", bot.listRE)
	}
}

func TestListFile_NoArgListsEverything(t *testing.T) {
	bot := defaultBot()
	bot.hasLibrary = true
	callHandler(handleListFile, bot, "")
	if bot.listRE != nil {
		t.Errorf("regex = %v, want nil", bot.listRE)
	}
}

func TestListFile_InvalidRegexDoesNotQuery(t *testing.T) {
	bot := defaultBot()
	bot.hasLibrary = true
	bot.libTracks = []LibraryTrack{{Path: "a.mp3"}}
	callHandler(handleListFile, bot, "([")
	if bot.listRE != nil {
		t.Error("invalid regex must not reach the library")
	}
}

func TestSearch_SplitsKeywordsAndPage(t *testing.T) {
	bot := defaultBot()
	bot.hasLibrary = true
	callHandler(handleSearch, bot, "beethoven symphony -p 2")
	if strings.Join(bot.searchKW, ",") != "beethoven,symphony" {
		t.Errorf("keywords = %v", bot.searchKW)
	}
}

func TestSearch_NoKeywordsShowsUsage(t *testing.T) {
	bot := defaultBot()
	bot.hasLibrary = true
	callHandler(handleSearch, bot, "  ")
	if bot.searchKW != nil {
		t.Error("empty search must not query")
	}
}

func TestLibraryCommands_ErrorsDoNotPanic(t *testing.T) {
	for _, err := range []error{library.ErrUnavailable, errors.New("boom")} {
		bot := defaultBot()
		bot.hasLibrary = true
		bot.libErr = err
		callHandler(handleListFile, bot, "")
		callHandler(handleSearch, bot, "x")
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
	tracks := make([]LibraryTrack, 120)
	for i := range tracks {
		tracks[i] = LibraryTrack{Path: fmt.Sprintf("t%03d.mp3", i)}
	}
	bot := defaultBot()
	bot.hasLibrary = true
	bot.libTracks = tracks
	for _, arg := range []string{"", "-p 2", "-p 3", "-p 99", "-p"} {
		callHandler(handleListFile, bot, arg) // must not panic
	}
}
