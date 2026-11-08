package command

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/konradk/gotamusique/internal/config"
	"layeh.com/gumble/gumble"
)

// pageBudget is the most characters (after HTML escaping) one page may hold. Mumble
// servers reject text messages over 5000 characters by default; this leaves room for
// the header and the <pre> wrapper.
const pageBudget = 4000

// maxLineLen truncates one listed track so a single line can never exceed the budget.
const maxLineLen = 90

var pageFlagRE = regexp.MustCompile(`(?:^|\s)-p(?:\s+(\S+))?(?:\s|$)`)

// parsePageFlag removes a "-p N" flag from arg and returns the rest and the page
// (default 1). ok is false when -p has no positive integer.
func parsePageFlag(arg string) (rest string, page int, ok bool) {
	arg = strings.TrimSpace(arg)
	m := pageFlagRE.FindStringSubmatchIndex(arg)
	if m == nil {
		return arg, 1, true
	}
	rest = strings.TrimSpace(arg[:m[0]] + " " + arg[m[1]:])
	if m[2] < 0 {
		return rest, 0, false
	}
	n, err := strconv.Atoi(arg[m[2]:m[3]])
	if err != nil || n < 1 {
		return rest, 0, false
	}
	return rest, n, true
}

// handleListFile: !listfile [regex] [-p N]. The regex is case-insensitive and
// matched against path and title.
func handleListFile(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	cfg := bot.Config()
	if !bot.HasLibrary() {
		reply(cfg, msg, "library unavailable (set <code>[files] music_folder</code>)", "library unavailable (set [files] music_folder)")
		return
	}
	pattern, page, ok := parsePageFlag(arg)
	if !ok {
		sendToChannel(msg, "Usage: "+symbol(cfg)+cmd+" [regex] [-p page]")
		return
	}

	var re *regexp.Regexp
	if pattern != "" {
		var err error
		if re, err = regexp.Compile("(?i)" + pattern); err != nil {
			text := "Invalid regex: " + err.Error()
			reply(cfg, msg, esc(text), text)
			return
		}
	}
	tracks, err := bot.ListLibrary(re)
	if err != nil {
		text := libraryErrorText(err)
		reply(cfg, msg, esc(text), text)
		return
	}
	sendTrackPage(cfg, msg, tracks, page, "No files found. Run "+symbol(cfg)+"rescan first?")
}

// handleSearch: !search <keywords> [-p N]; every keyword must match.
func handleSearch(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	cfg := bot.Config()
	if !bot.HasLibrary() {
		reply(cfg, msg, "library unavailable (set <code>[files] music_folder</code>)", "library unavailable (set [files] music_folder)")
		return
	}
	query, page, ok := parsePageFlag(arg)
	keywords := strings.Fields(query)
	if !ok || len(keywords) == 0 {
		sendToChannel(msg, "Usage: "+symbol(cfg)+cmd+" <keywords> [-p page]")
		return
	}
	tracks, err := bot.SearchLibrary(keywords)
	if err != nil {
		text := libraryErrorText(err)
		reply(cfg, msg, esc(text), text)
		return
	}
	sendTrackPage(cfg, msg, tracks, page, "No matching tracks.")
}

// trackLine renders one track as "path - artist title".
func trackLine(t LibraryTrack) string {
	line := t.Path
	if t.Title != "" || t.Artist != "" {
		line += " - " + strings.TrimSpace(t.Artist+" "+t.Title)
	}
	return truncateRunes(line, maxLineLen)
}

// paginate splits lines into pages whose escaped text stays within pageBudget.
func paginate(lines []string) [][]string {
	var pages [][]string
	var cur []string
	size := 0
	for _, l := range lines {
		n := len(esc(l)) + 1
		if len(cur) > 0 && size+n > pageBudget {
			pages = append(pages, cur)
			cur, size = nil, 0
		}
		cur = append(cur, l)
		size += n
	}
	if len(cur) > 0 {
		pages = append(pages, cur)
	}
	return pages
}

func sendTrackPage(cfg *config.Config, msg *gumble.TextMessage, tracks []LibraryTrack, page int, empty string) {
	if len(tracks) == 0 {
		reply(cfg, msg, esc(empty), empty)
		return
	}
	lines := make([]string, len(tracks))
	for i, t := range tracks {
		lines[i] = trackLine(t)
	}
	pages := paginate(lines)
	if page > len(pages) {
		text := fmt.Sprintf("No page %d: %d tracks, %d pages.", page, len(tracks), len(pages))
		reply(cfg, msg, esc(text), text)
		return
	}
	text := fmt.Sprintf("%d tracks, page %d/%d:\n%s", len(tracks), page, len(pages), strings.Join(pages[page-1], "\n"))
	reply(cfg, msg, "<pre>"+esc(text)+"</pre>", text)
}

// truncateRunes shortens s to at most n runes, ending in "..." when cut.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-3]) + "..."
}
