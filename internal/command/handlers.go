package command

import (
	"fmt"
	"html"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/konradk/gotamusique/internal/config"
	"github.com/konradk/gotamusique/internal/radio"
	"layeh.com/gumble/gumble"
)

// format returns the html string when formatted is true, otherwise plain.
func format(formatted bool, htmlText, plainText string) string {
	if formatted {
		return htmlText
	}
	return plainText
}

func esc(s string) string { return html.EscapeString(s) }

// handleRadio: no arg → list presets; name → play preset; URL → play direct stream.
func handleRadio(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	cfg := bot.Config()
	if arg == "" {
		listPresets(bot, cfg, msg)
		return
	}

	if u, err := url.Parse(arg); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		playURL(bot, cfg, msg, arg)
	} else {
		playPreset(bot, cfg, msg, cmd, arg)
	}
}

func listPresets(bot BotAPI, cfg *config.Config, msg *gumble.TextMessage) {
	presets := cfg.Radio
	if len(presets) == 0 {
		sendToChannel(msg, "No presets configured.")
		return
	}
	keys := make([]string, 0, len(presets))
	for k := range presets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, k := range keys {
		name := presets[k].Comment
		if name == "" {
			name = hostnameOf(presets[k].URL)
		}
		if cfg.Bot.FormattedReplies {
			fmt.Fprintf(&sb, "<b>%s</b> — %s<br>", esc(k), esc(name))
		} else {
			fmt.Fprintf(&sb, "%s — %s\n", k, name)
		}
	}
	if cfg.Bot.FormattedReplies {
		sendToChannel(msg, "<b>Radio presets:</b><br>"+sb.String())
	} else {
		sendToChannel(msg, "Radio presets:\n"+sb.String())
	}
}

func playURL(bot BotAPI, cfg *config.Config, msg *gumble.TextMessage, rawURL string) {
	item := radio.NewRadioItemFromURL(rawURL)
	if err := item.Validate(); err != nil {
		sendToChannel(msg, format(cfg.Bot.FormattedReplies,
			"Could not reach stream: "+esc(err.Error()),
			"Could not reach stream: "+err.Error(),
		))
		return
	}
	bot.Enqueue(item)
	sendToChannel(msg, format(cfg.Bot.FormattedReplies,
		"Queued: <b>"+esc(item.Name)+"</b>",
		"Queued: "+item.Name,
	))
}

func playPreset(bot BotAPI, cfg *config.Config, msg *gumble.TextMessage, cmd, name string) {
	preset, ok := cfg.Radio[name]
	if !ok {
		sym := symbol(cfg)
		sendToChannel(msg, format(cfg.Bot.FormattedReplies,
			"Unknown preset <b>"+esc(name)+"</b>. Use <b>"+esc(sym+cmd)+"</b> to list presets.",
			"Unknown preset \""+name+"\". Use "+sym+cmd+" to list presets.",
		))
		return
	}
	item := radio.NewRadioItemFromPreset(name, preset)
	bot.Enqueue(item)
	sendToChannel(msg, format(cfg.Bot.FormattedReplies,
		"Queued: <b>"+esc(item.Name)+"</b>",
		"Queued: "+item.Name,
	))
}

// --- rbCache ---

const (
	defaultSearchLimit = 10
	maxSearchLimit     = 50
)

type rbCache struct {
	mu       sync.Mutex
	stations map[uint32][]radio.Station
}

func newRBCache() *rbCache {
	return &rbCache{stations: make(map[uint32][]radio.Station)}
}

func (c *rbCache) set(channelID uint32, stations []radio.Station) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stations[channelID] = stations
}

func (c *rbCache) get(channelID uint32) []radio.Station {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stations[channelID]
}

// parseRBQueryArg extracts an optional trailing [-n N | --limit N] flag from arg.
// Returns the station name, the result limit, and any parse error.
func parseRBQueryArg(arg string) (name string, limit int, err error) {
	fields := strings.Fields(arg)
	limit = defaultSearchLimit

	if len(fields) == 0 {
		return "", defaultSearchLimit, nil
	}

	// Trailing flag without a value: e.g. "soma -n"
	last := fields[len(fields)-1]
	if last == "-n" || last == "--limit" {
		return "", 0, fmt.Errorf("flag %q requires a value", last)
	}

	// Trailing flag pair: e.g. "soma -n 20" or "soma --limit 20"
	if len(fields) >= 2 {
		flag := fields[len(fields)-2]
		val := fields[len(fields)-1]
		if flag == "-n" || flag == "--limit" {
			n, parseErr := strconv.Atoi(val)
			if parseErr != nil || n <= 0 {
				return "", 0, fmt.Errorf("invalid limit %q: must be a positive integer", val)
			}
			if n > maxSearchLimit {
				n = maxSearchLimit
			}
			name = strings.TrimSpace(strings.Join(fields[:len(fields)-2], " "))
			if name == "" {
				return "", 0, fmt.Errorf("station name required")
			}
			return name, n, nil
		}
	}

	return arg, defaultSearchLimit, nil
}

// makeRBQueryHandler returns a HandlerFunc that searches radio-browser.info and
// caches results. searchFn is rb.Search in production; inject a mock in tests.
func makeRBQueryHandler(cache *rbCache, searchFn func(string, int) ([]radio.Station, error)) HandlerFunc {
	return func(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		cfg := bot.Config()
		if arg == "" {
			sendToChannel(msg, "Usage: "+symbol(cfg)+cmd+" <name> [-n N]")
			return
		}

		name, limit, err := parseRBQueryArg(arg)
		if err != nil {
			sendToChannel(msg, "Usage: "+symbol(cfg)+cmd+" <name> [-n N] (N: 1-50): "+err.Error())
			return
		}

		stations, searchErr := searchFn(name, limit)
		if searchErr != nil {
			sendToChannel(msg, "radio-browser search failed: "+searchErr.Error())
			return
		}
		if len(stations) == 0 {
			sendToChannel(msg, format(cfg.Bot.FormattedReplies,
				"No stations found for <b>"+esc(name)+"</b>.",
				"No stations found for \""+name+"\".",
			))
			return
		}

		cache.set(msg.Channels[0].ID, stations)

		text := buildRBTable(name, stations)
		if cfg.Bot.FormattedReplies {
			sendToChannel(msg, "<pre>"+esc(text)+"</pre>")
		} else {
			sendToChannel(msg, text)
		}
	}
}

// makeRBPlayHandler returns a HandlerFunc that plays a station by UUID or cache index.
// byUUIDFn is rb.ByUUID in production; inject a mock in tests.
func makeRBPlayHandler(cache *rbCache, byUUIDFn func(string) (*radio.Station, error)) HandlerFunc {
	return func(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		cfg := bot.Config()
		if arg == "" {
			sendToChannel(msg, "Usage: "+symbol(cfg)+cmd+" <uuid|N>")
			return
		}

		// Integer index: resolve against the channel's cached query results.
		if n, err := strconv.Atoi(arg); err == nil && n > 0 {
			channelID := msg.Channels[0].ID
			stations := cache.get(channelID)
			if len(stations) == 0 {
				sendToChannel(msg, "No recent !rbquery results for this channel.")
				return
			}
			if n > len(stations) {
				sendToChannel(msg, "Index out of range — use !rbquery to see available stations.")
				return
			}
			item := radio.NewRadioItemFromStation(stations[n-1])
			bot.Enqueue(item)
			sendToChannel(msg, format(cfg.Bot.FormattedReplies,
				"Queued: <b>"+esc(item.Name)+"</b>",
				"Queued: "+item.Name,
			))
			return
		}

		// UUID flow.
		station, err := byUUIDFn(arg)
		if err != nil {
			sendToChannel(msg, format(cfg.Bot.FormattedReplies,
				"Station not found: "+esc(err.Error()),
				"Station not found: "+err.Error(),
			))
			return
		}

		item := radio.NewRadioItemFromStation(*station)
		bot.Enqueue(item)
		sendToChannel(msg, format(cfg.Bot.FormattedReplies,
			"Queued: <b>"+esc(item.Name)+"</b>",
			"Queued: "+item.Name,
		))
	}
}

func handleStop(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	bot.Stop()
	sendToChannel(msg, "Stopped.")
}

func handleMute(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	bot.Mute()
	sendToChannel(msg, "Muted.")
}

func handleUnmute(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	bot.Unmute()
	sendToChannel(msg, "Unmuted.")
}

func handleSkip(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	bot.Skip()
	sendToChannel(msg, "Skipped.")
}

func handleClear(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	bot.Clear()
	sendToChannel(msg, "Queue cleared.")
}

func handleQueue(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	cfg := bot.Config()
	items := bot.QueueItems()
	if len(items) == 0 {
		sendToChannel(msg, "Queue is empty.")
		return
	}

	idx := bot.QueueCurrentIndex()
	var sb strings.Builder
	fmt.Fprintf(&sb, "Queue (%d items):\n", len(items))
	for i, item := range items {
		marker := "  "
		if i == idx {
			marker = "> "
		}
		fmt.Fprintf(&sb, "%s%d. %s\n", marker, i+1, item.FormatTitle())
	}
	text := strings.TrimRight(sb.String(), "\n")

	if cfg.Bot.FormattedReplies {
		sendToChannel(msg, "<pre>"+esc(text)+"</pre>")
	} else {
		sendToChannel(msg, text)
	}
}

func handleNP(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	cfg := bot.Config()
	item := bot.CurrentItem()
	if item == nil {
		sendToChannel(msg, "Nothing is currently playing.")
		return
	}
	title := item.FormatTitle()
	sendToChannel(msg, format(cfg.Bot.FormattedReplies,
		"Now playing: <b>"+esc(title)+"</b>",
		"Now playing: "+title,
	))
}

func handleVolume(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	cfg := bot.Config()
	if arg == "" {
		vol := int(math.Round(bot.TargetVolume() * 100))
		sendToChannel(msg, fmt.Sprintf("Volume: %d", vol))
		return
	}

	pct, err := strconv.Atoi(strings.TrimSpace(arg))
	if err != nil || pct < 0 || pct > 100 {
		sendToChannel(msg, "Usage: "+symbol(cfg)+cmd+" [0-100]")
		return
	}

	bot.SetVolume(pct)
	actual := int(math.Round(bot.TargetVolume() * 100))
	sendToChannel(msg, format(cfg.Bot.FormattedReplies,
		fmt.Sprintf("Volume set to <b>%d</b>", actual),
		fmt.Sprintf("Volume set to %d", actual),
	))
}

func handleJoinMe(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	if msg.Sender == nil || msg.Sender.Channel == nil {
		sendToChannel(msg, "You are not in a channel.")
		return
	}
	bot.JoinChannel(msg.Sender.Channel)
	sendToChannel(msg, "Joining your channel.")
}

func handleKill(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	bot.Kill()
}

// --- table helpers ---

const maxTableLen = 5000

func buildRBTable(name string, stations []radio.Station) string {
	text := rbTableFull(name, stations)
	if len(text) <= maxTableLen {
		return text
	}
	text = rbTableShort(name, stations)
	if len(text) <= maxTableLen {
		return text
	}
	return text[:maxTableLen]
}

func rbTableFull(name string, stations []radio.Station) string {
	const (
		wIndex   = 2
		wUUID    = 36
		wName    = 25
		wGenre   = 15
		wCodec   = 13
		wCountry = 7
	)
	var sb strings.Builder
	fmt.Fprintf(&sb, "Radio-Browser results for %q:\n", name)
	fmt.Fprintf(&sb, "| %-*s | %-*s | %-*s | %-*s | %-*s | %-*s |\n",
		wIndex, "#", wUUID, "rbplay ID", wName, "Station Name", wGenre, "Genre", wCodec, "Codec/Bitrate", wCountry, "Country")
	fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s | %s |\n",
		strings.Repeat("-", wIndex),
		strings.Repeat("-", wUUID),
		strings.Repeat("-", wName),
		strings.Repeat("-", wGenre),
		strings.Repeat("-", wCodec),
		strings.Repeat("-", wCountry),
	)
	for i, s := range stations {
		fmt.Fprintf(&sb, "| %-*d | %-*s | %-*s | %-*s | %-*s | %-*s |\n",
			wIndex, i+1,
			wUUID, truncateField(s.UUID, wUUID),
			wName, truncateField(s.Name, wName),
			wGenre, truncateField(firstTag(s.Tags), wGenre),
			wCodec, truncateField(bitrateStr(s.Codec, s.Bitrate), wCodec),
			wCountry, truncateField(s.Country, wCountry),
		)
	}
	return sb.String()
}

func rbTableShort(name string, stations []radio.Station) string {
	const (
		wIndex = 2
		wUUID  = 36
		wName  = 35
		wGenre = 20
	)
	var sb strings.Builder
	fmt.Fprintf(&sb, "Radio-Browser results for %q:\n", name)
	fmt.Fprintf(&sb, "| %-*s | %-*s | %-*s | %-*s |\n", wIndex, "#", wUUID, "rbplay ID", wName, "Station Name", wGenre, "Genre")
	fmt.Fprintf(&sb, "| %s | %s | %s | %s |\n",
		strings.Repeat("-", wIndex),
		strings.Repeat("-", wUUID),
		strings.Repeat("-", wName),
		strings.Repeat("-", wGenre),
	)
	for i, s := range stations {
		fmt.Fprintf(&sb, "| %-*d | %-*s | %-*s | %-*s |\n",
			wIndex, i+1,
			wUUID, truncateField(s.UUID, wUUID),
			wName, truncateField(s.Name, wName),
			wGenre, truncateField(firstTag(s.Tags), wGenre),
		)
	}
	return sb.String()
}

func bitrateStr(codec string, bitrate int) string {
	if bitrate > 0 {
		return fmt.Sprintf("%s/%d", codec, bitrate)
	}
	return codec
}

func truncateField(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func firstTag(tags string) string {
	if i := strings.IndexByte(tags, ','); i >= 0 {
		return strings.TrimSpace(tags[:i])
	}
	return tags
}

func hostnameOf(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
		return u.Host
	}
	return rawURL
}
