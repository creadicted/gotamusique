package command

import (
	"context"
	"html"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/konradk/gotamusique/internal/audio"
	"github.com/konradk/gotamusique/internal/config"
	"github.com/konradk/gotamusique/internal/library"
	"github.com/konradk/gotamusique/internal/web"
	"layeh.com/gumble/gumble"
)

// BotAPI is the subset of *bot.Bot that command handlers need. Defined here to
// avoid a circular import between the bot and command packages.
type BotAPI interface {
	Config() *config.Config
	Enqueue(item audio.MediaItem)
	Stop()
	Mute()
	Unmute()
	IsMuted() bool
	Skip()
	Clear()
	// Pause halts playback keeping the queue position; false if already paused by a user.
	Pause() bool
	// Resume continues after a pause; false if the bot was not paused.
	Resume() bool
	IsPaused() bool
	// RestoreLast re-queues the item remembered from the previous run (position for
	// local files); ok is false when nothing can be restored.
	RestoreLast() (title string, at time.Duration, ok bool)
	QueueItems() []audio.MediaItem
	QueueCurrentIndex() int
	CurrentItem() audio.MediaItem
	TargetVolume() float64
	SetVolume(pct int)
	JoinChannel(ch *gumble.Channel)
	Kill()
	PlaybackMode() string
	SetPlaybackMode(name string) error
	HasLibrary() bool
	Rescan(ctx context.Context, force bool) (string, error)
	// ListLibrary returns library tracks ordered by path, optionally only those
	// whose path or title match re (nil = all).
	ListLibrary(re *regexp.Regexp) ([]library.Track, error)
	// SearchLibrary returns tracks whose keywords contain every keyword.
	SearchLibrary(keywords []string) ([]library.Track, error)
	// LookupFile resolves a !file argument: exact path, else folder, else partial match.
	LookupFile(arg string) (library.FileLookup, error)
	// Last jumps to and plays the final queue item; false when the queue is empty.
	Last() bool
	// RandomizeQueue shuffles the upcoming queue items without interrupting playback.
	RandomizeQueue()
	// RepeatCurrent queues n more plays of the current item right after it.
	RepeatCurrent(n int) (audio.MediaItem, bool)
	// FindTagged returns tracks carrying every given tag (whole tag, case-insensitive).
	FindTagged(tags []string) ([]library.Track, error)
	// UpdateTags rewrites the tags of the library row id with fn. Returns
	// library.ErrNotInLibrary when there is no such row.
	UpdateTags(id string, fn func(tags []string) []string) error
	// WebLogin returns the address to open the web UI for id: a one-time login link
	// valid for ttl, or (ttl 0) the plain address when no login is required. It
	// returns web.ErrDisabled when the web interface is off.
	WebLogin(id web.Identity) (link string, ttl time.Duration, err error)
	// WebRevoke cancels the unused login link of id, for when it could not be delivered.
	WebRevoke(id web.Identity)
}

// HandlerFunc is the signature every command handler must implement.
type HandlerFunc func(bot BotAPI, c *Ctx, cmd, arg string)

type entry struct {
	fn          HandlerFunc
	adminOnly   bool
	description string
	aliases     []string // all registered aliases; first is the primary (used in !help sort)
}

// Dispatcher parses incoming Mumble text messages and routes them to handlers.
type Dispatcher struct {
	handlers map[string]*entry
	entries  []*entry // registration order, used for !help output
	cmdRE    *regexp.Regexp
}

// NewDispatcher builds a Dispatcher for the given command symbols (e.g. ["!", "！"]).
func NewDispatcher(symbols []string) *Dispatcher {
	escaped := make([]string, len(symbols))
	for i, s := range symbols {
		escaped[i] = regexp.QuoteMeta(s)
	}
	if len(escaped) == 0 {
		escaped = []string{regexp.QuoteMeta("!")}
	}
	pattern := `^(?:` + strings.Join(escaped, "|") + `)(?P<command>\S+)(?:\s+(?P<argument>.*))?$`
	return &Dispatcher{
		handlers: make(map[string]*entry),
		cmdRE:    regexp.MustCompile(pattern),
	}
}

// Register associates fn with each alias in aliases. description is shown by !help.
func (d *Dispatcher) Register(aliases []string, fn HandlerFunc, adminOnly bool, description string) {
	if len(aliases) == 0 {
		return
	}
	e := &entry{fn: fn, adminOnly: adminOnly, description: description, aliases: aliases}
	d.entries = append(d.entries, e)
	for _, a := range aliases {
		d.handlers[a] = e
	}
}

// Dispatch parses msg and calls the matching handler, or replies with an error.
// Server messages (Sender == nil) and private messages (no Channels) are silently ignored.
func (d *Dispatcher) Dispatch(bot BotAPI, msg *gumble.TextMessage) {
	if msg.Sender == nil {
		return
	}
	if len(msg.Channels) == 0 {
		return
	}
	d.Run(bot, chatCtx(msg), stripHTML(msg.Message))
}

// Run parses raw (a full command line such as "!volume 40") and calls the matching
// handler with c, or replies with an error through c.
func (d *Dispatcher) Run(bot BotAPI, c *Ctx, raw string) {
	m := d.cmdRE.FindStringSubmatch(raw)
	if m == nil {
		return
	}

	cmdIdx := d.cmdRE.SubexpIndex("command")
	argIdx := d.cmdRE.SubexpIndex("argument")
	cmd := m[cmdIdx]
	arg := ""
	if argIdx >= 0 && argIdx < len(m) {
		arg = strings.TrimSpace(m[argIdx])
	}

	cfg := bot.Config()
	sym := symbol(cfg)
	user := c.User

	// 1. Exact match.
	if e, ok := d.handlers[cmd]; ok {
		if e.adminOnly && !isAdmin(cfg, user) {
			c.Send("Permission denied: admin only.")
			return
		}
		e.fn(bot, c, cmd, arg)
		return
	}

	// 2. Prefix match.
	var candidates []string
	for alias := range d.handlers {
		if strings.HasPrefix(alias, cmd) {
			candidates = append(candidates, alias)
		}
	}

	switch len(candidates) {
	case 0:
		c.Send("Unknown command \"" + cmd + "\". Use " + sym + "help for a list of commands.")
	case 1:
		e := d.handlers[candidates[0]]
		if e.adminOnly && !isAdmin(cfg, user) {
			c.Send("Permission denied: admin only.")
			return
		}
		e.fn(bot, c, candidates[0], arg)
	default:
		sort.Strings(candidates)
		c.Send("Did you mean: " + strings.Join(candidates, ", ") + "?")
	}
}

// HelpHandler returns a HandlerFunc that lists all registered commands. It closes
// over d so that commands registered after the call are also shown.
func (d *Dispatcher) HelpHandler() HandlerFunc {
	return func(bot BotAPI, c *Ctx, cmd, arg string) {
		cfg := bot.Config()
		c.Send(d.buildHelp(symbol(cfg), cfg.Bot.FormattedReplies))
	}
}

func (d *Dispatcher) buildHelp(sym string, formatted bool) string {
	sorted := make([]*entry, len(d.entries))
	copy(sorted, d.entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].aliases[0] < sorted[j].aliases[0]
	})

	var sb strings.Builder
	for _, e := range sorted {
		prefixed := make([]string, len(e.aliases))
		for i, a := range e.aliases {
			prefixed[i] = sym + a
		}
		sb.WriteString(strings.Join(prefixed, ", "))
		sb.WriteString(" — ")
		sb.WriteString(e.description)
		sb.WriteByte('\n')
	}
	text := strings.TrimRight(sb.String(), "\n")
	if formatted {
		return "<pre>" + html.EscapeString(text) + "</pre>"
	}
	return text
}

func isAdmin(cfg *config.Config, user string) bool {
	for _, a := range cfg.Bot.Admin {
		if a == user {
			return true
		}
	}
	return false
}

func symbol(cfg *config.Config) string {
	if len(cfg.Commands.Symbol) > 0 {
		return cfg.Commands.Symbol[0]
	}
	return "!"
}

var htmlTagRE = regexp.MustCompile(`<[^>]*>`)

func stripHTML(s string) string {
	s = htmlTagRE.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(s)
}
