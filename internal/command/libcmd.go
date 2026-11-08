package command

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/konradk/gotamusique/internal/config"
	fileitem "github.com/konradk/gotamusique/internal/media/file"
	"layeh.com/gumble/gumble"
)

// pageBudget is the most characters (after HTML escaping) one page may hold. Mumble
// servers reject text messages over 5000 characters by default; this leaves room for
// the header and the <pre> wrapper.
const pageBudget = 4000

// maxLineLen truncates one listed track so a single line can never exceed the budget.
const maxLineLen = 90

// ---- shortlist ----

// shortlist remembers the last numbered result (!listfile, !search, !findtagged)
// per channel, so !shortlist can queue entries from it. Channels are evicted FIFO.
type shortlist struct {
	mu     sync.Mutex
	tracks map[uint32][]LibraryTrack
	order  []uint32
}

func newShortlist() *shortlist { return &shortlist{tracks: map[uint32][]LibraryTrack{}} }

// channelKey identifies the channel a message came from (0 for a private message).
func channelKey(msg *gumble.TextMessage) uint32 {
	if len(msg.Channels) == 0 || msg.Channels[0] == nil {
		return 0
	}
	return msg.Channels[0].ID
}

func (s *shortlist) set(ch uint32, tracks []LibraryTrack) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tracks[ch]; !ok {
		if len(s.order) >= maxCacheChannels {
			delete(s.tracks, s.order[0])
			s.order = s.order[1:]
		}
		s.order = append(s.order, ch)
	}
	s.tracks[ch] = tracks
}

func (s *shortlist) get(ch uint32) []LibraryTrack {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tracks[ch]
}

// ---- argument parsing ----

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

// parseTags splits a comma-separated tag list, trimming blanks and dropping empties.
func parseTags(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func noLibrary(cfg *config.Config, msg *gumble.TextMessage) {
	reply(cfg, msg, "library unavailable (set <code>[files] music_folder</code>)", "library unavailable (set [files] music_folder)")
}

// ---- listing ----

// makeListFileHandler: !listfile [regex] [-p N]. The regex is case-insensitive and
// matched against path and title.
func makeListFileHandler(sl *shortlist) HandlerFunc {
	return func(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		cfg := bot.Config()
		if !bot.HasLibrary() {
			noLibrary(cfg, msg)
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
		sendTrackPage(cfg, msg, sl, tracks, page, "No files found. Run "+symbol(cfg)+"rescan first?", cmd)
	}
}

// makeSearchHandler: !search <keywords> [-p N]; every keyword must match.
func makeSearchHandler(sl *shortlist) HandlerFunc {
	return func(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		cfg := bot.Config()
		if !bot.HasLibrary() {
			noLibrary(cfg, msg)
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
		sendTrackPage(cfg, msg, sl, tracks, page, "No matching tracks.", cmd)
	}
}

// makeFindTaggedHandler: !findtagged <tag,tag> [-p N]; a track needs every tag.
func makeFindTaggedHandler(sl *shortlist) HandlerFunc {
	return func(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		cfg := bot.Config()
		if !bot.HasLibrary() {
			noLibrary(cfg, msg)
			return
		}
		query, page, ok := parsePageFlag(arg)
		tags := parseTags(query)
		if !ok || len(tags) == 0 {
			sendToChannel(msg, "Usage: "+symbol(cfg)+cmd+" <tag>[,<tag>...] [-p page]")
			return
		}
		tracks, err := bot.FindTagged(tags)
		if err != nil {
			text := libraryErrorText(err)
			reply(cfg, msg, esc(text), text)
			return
		}
		sendTrackPage(cfg, msg, sl, tracks, page, "No tracks with those tags.", cmd)
	}
}

// trackLine renders one track as "path - artist title (tag, tag)".
func trackLine(t LibraryTrack) string {
	line := t.Path
	if t.Title != "" || t.Artist != "" {
		line += " - " + strings.TrimSpace(t.Artist+" "+t.Title)
	}
	if len(t.Tags) > 0 {
		line += " (" + strings.Join(t.Tags, ", ") + ")"
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

// sendTrackPage numbers the tracks, remembers them as the channel's shortlist and
// sends one page. Numbers run across pages, so "!shortlist 70" works from page 2.
func sendTrackPage(cfg *config.Config, msg *gumble.TextMessage, sl *shortlist, tracks []LibraryTrack, page int, empty, cmd string) {
	if len(tracks) == 0 {
		reply(cfg, msg, esc(empty), empty)
		return
	}
	lines := make([]string, len(tracks))
	for i, t := range tracks {
		lines[i] = fmt.Sprintf("%d. %s", i+1, trackLine(t))
	}
	pages := paginate(lines)
	if page > len(pages) {
		text := fmt.Sprintf("No page %d: %d tracks, %d pages.", page, len(tracks), len(pages))
		reply(cfg, msg, esc(text), text)
		return
	}
	sl.set(channelKey(msg), tracks)
	text := fmt.Sprintf("%d tracks, page %d/%d (queue with %sshortlist <n ...|*>):\n%s",
		len(tracks), page, len(pages), symbol(cfg), strings.Join(pages[page-1], "\n"))
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

// ---- queueing library tracks ----

// trackItem builds a playable file item from a library track without running
// ffprobe: the scan already stored title, artist and duration.
func trackItem(t LibraryTrack, musicFolder string) (*fileitem.FileItem, error) {
	it := fileitem.New(t.Path, musicFolder)
	if err := it.Validate(); err != nil {
		return nil, err
	}
	it.Title, it.Artist, it.Duration, it.Tags = t.Title, t.Artist, t.Duration, t.Tags
	return it, nil
}

// maxBulkAdd is the most tracks one command queues at once (!filematch, !file <folder>,
// !tag, !shortlist *), so a careless pattern cannot flood the queue. Also the largest
// !repeat count.
const maxBulkAdd = 200

// enqueueTracks queues the tracks that still exist on disk and replies with a summary.
// At most maxBulkAdd tracks are queued; the reply says when the rest was left out.
func enqueueTracks(bot BotAPI, cfg *config.Config, msg *gumble.TextMessage, tracks []LibraryTrack, what string) {
	if cfg.Files.MusicFolder == "" {
		noLibrary(cfg, msg)
		return
	}
	total := len(tracks)
	if total > maxBulkAdd {
		tracks = tracks[:maxBulkAdd]
	}
	queued, missing := 0, 0
	var last string
	for _, t := range tracks {
		it, err := trackItem(t, cfg.Files.MusicFolder)
		if err != nil {
			missing++
			continue
		}
		bot.Enqueue(it)
		queued++
		last = it.FormatTitle()
	}
	var text string
	switch {
	case queued == 1 && missing == 0:
		text = "Queued: " + last
	case queued == 0:
		text = "Nothing queued: " + strconv.Itoa(missing) + " file(s) missing, run " + symbol(cfg) + "rescan"
	default:
		text = fmt.Sprintf("Queued %d tracks%s", queued, what)
		if missing > 0 {
			text += fmt.Sprintf(" (%d missing, run %srescan)", missing, symbol(cfg))
		}
	}
	if total > maxBulkAdd {
		text += fmt.Sprintf(". Only the first %d of %d were queued, narrow your selection", maxBulkAdd, total)
	}
	reply(cfg, msg, esc(text), text)
}

// makeShortlistHandler: !shortlist <n> [n ...] | *  — queue entries from the last
// !listfile / !search / !findtagged result of this channel. Every number is
// checked first, so a bad number queues nothing.
func makeShortlistHandler(sl *shortlist) HandlerFunc {
	return func(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		cfg := bot.Config()
		usage := "Usage: " + symbol(cfg) + cmd + " <n> [n ...] | *"
		fields := strings.Fields(arg)
		if len(fields) == 0 {
			sendToChannel(msg, usage)
			return
		}
		list := sl.get(channelKey(msg))
		if len(list) == 0 {
			text := "No list to pick from. Use " + symbol(cfg) + "listfile, " + symbol(cfg) + "search or " + symbol(cfg) + "findtagged first."
			reply(cfg, msg, esc(text), text)
			return
		}
		var picked []LibraryTrack
		if len(fields) == 1 && fields[0] == "*" {
			picked = list
		} else {
			for _, f := range fields {
				n, err := strconv.Atoi(f)
				if err != nil || n < 1 || n > len(list) {
					text := fmt.Sprintf("Bad number %q (the list has %d entries). %s", f, len(list), usage)
					reply(cfg, msg, esc(text), text)
					return
				}
				picked = append(picked, list[n-1])
			}
		}
		enqueueTracks(bot, cfg, msg, picked, "")
	}
}

// handlePlayTag: !tag <tag,tag> — queue every library track carrying all the tags.
func handlePlayTag(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	cfg := bot.Config()
	if !bot.HasLibrary() {
		noLibrary(cfg, msg)
		return
	}
	tags := parseTags(arg)
	if len(tags) == 0 {
		sendToChannel(msg, "Usage: "+symbol(cfg)+cmd+" <tag>[,<tag>...]")
		return
	}
	tracks, err := bot.FindTagged(tags)
	if err != nil {
		text := libraryErrorText(err)
		reply(cfg, msg, esc(text), text)
		return
	}
	if len(tracks) == 0 {
		reply(cfg, msg, "No tracks with those tags.", "No tracks with those tags.")
		return
	}
	enqueueTracks(bot, cfg, msg, tracks, " tagged "+strings.Join(tags, ", "))
}

// ---- tagging queue items ----

// tagTarget picks the queue items a tag command applies to from its leading
// argument: "<n>" (1-based queue position), "*" (all), or none (the current item).
// It returns the indexes and the remaining argument.
func tagTarget(arg string, queueLen, current int) (idx []int, rest string, err error) {
	arg = strings.TrimSpace(arg)
	first, tail, _ := strings.Cut(arg, " ")
	tail = strings.TrimSpace(tail)
	switch {
	case first == "*" && tail != "":
		idx = make([]int, queueLen)
		for i := range idx {
			idx[i] = i
		}
		return idx, tail, nil
	case tail != "" && isDigits(first):
		n, _ := strconv.Atoi(first)
		if n < 1 || n > queueLen {
			return nil, "", fmt.Errorf("no queue item %s (the queue has %d)", first, queueLen)
		}
		return []int{n - 1}, tail, nil
	}
	if queueLen == 0 || current < 0 || current >= queueLen {
		return nil, "", fmt.Errorf("nothing is playing")
	}
	return []int{current}, arg, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// addTags adds tags (case-insensitive duplicates ignored); untagFn removes them.
func addTags(add []string) func([]string) []string {
	return func(have []string) []string {
		out := append([]string(nil), have...)
		for _, t := range add {
			if !containsFold(out, t) {
				out = append(out, t)
			}
		}
		return out
	}
}

func removeTags(remove []string, all bool) func([]string) []string {
	return func(have []string) []string {
		if all {
			return nil
		}
		var out []string
		for _, h := range have {
			if !containsFold(remove, h) {
				out = append(out, h)
			}
		}
		return out
	}
}

func containsFold(list []string, s string) bool {
	for _, l := range list {
		if strings.EqualFold(l, s) {
			return true
		}
	}
	return false
}

// handleAddTag: !addtag [n|*] <tag,tag>. Applies to queue items that are library files.
func handleAddTag(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	tagQueueItems(bot, msg, cmd, arg, false)
}

// handleUntag: !untag [n|*] <tag,tag|*>. A lone "*" as the tag list clears all tags.
func handleUntag(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	tagQueueItems(bot, msg, cmd, arg, true)
}

func tagQueueItems(bot BotAPI, msg *gumble.TextMessage, cmd, arg string, remove bool) {
	cfg := bot.Config()
	if !bot.HasLibrary() {
		noLibrary(cfg, msg)
		return
	}
	items := bot.QueueItems()
	idx, rest, err := tagTarget(arg, len(items), bot.QueueCurrentIndex())
	tagArg := strings.TrimSpace(rest)
	if err != nil || tagArg == "" {
		text := "Usage: " + symbol(cfg) + cmd + " [n|*] <tag>[,<tag>...]"
		if remove {
			text = "Usage: " + symbol(cfg) + cmd + " [n|*] <tag>[,<tag>...|*]"
		}
		if err != nil {
			text = err.Error() + ". " + text
		}
		reply(cfg, msg, esc(text), text)
		return
	}
	clearAll := remove && tagArg == "*"
	tags := parseTags(tagArg)
	if !clearAll && (len(tags) == 0 || containsFold(tags, "*")) {
		text := "Tags cannot be empty or \"*\"."
		reply(cfg, msg, esc(text), text)
		return
	}
	fn := addTags(tags)
	if remove {
		fn = removeTags(tags, clearAll)
	}

	done, skipped := 0, 0
	var lastTitle string
	for _, i := range idx {
		fi, ok := items[i].(*fileitem.FileItem)
		if !ok {
			skipped++
			continue
		}
		if err := bot.UpdateTags(fi.ID, fn); err != nil {
			skipped++
			continue
		}
		done++
		lastTitle = fi.FormatTitle()
	}

	verb, what := "Tagged", strings.Join(tags, ", ")
	if remove {
		verb = "Removed tags from"
		if clearAll {
			verb, what = "Cleared all tags from", ""
		}
	}
	var text string
	switch {
	case done == 0:
		text = "Nothing changed: only library files can be tagged (radio, URL and stream items cannot)."
	case done == 1 && skipped == 0:
		text = verb + " " + lastTitle
		if what != "" {
			text += ": " + what
		}
	default:
		text = fmt.Sprintf("%s %d items", verb, done)
		if what != "" {
			text += ": " + what
		}
		if skipped > 0 {
			text += fmt.Sprintf(" (%d skipped, not library files)", skipped)
		}
	}
	reply(cfg, msg, esc(text), text)
}
