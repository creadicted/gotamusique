package command

import (
	"fmt"
	"html"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"

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
		playPreset(bot, cfg, msg, arg)
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

func playPreset(bot BotAPI, cfg *config.Config, msg *gumble.TextMessage, name string) {
	preset, ok := cfg.Radio[name]
	if !ok {
		sendToChannel(msg, format(cfg.Bot.FormattedReplies,
			"Unknown preset <b>"+esc(name)+"</b>. Use <b>!radio</b> to list presets.",
			"Unknown preset \""+name+"\". Use !radio to list presets.",
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

// handleRBQuery searches radio-browser.info and displays results as a table.
func handleRBQuery(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	cfg := bot.Config()
	if arg == "" {
		sendToChannel(msg, "Usage: "+symbol(cfg)+"rbquery <name>")
		return
	}

	rb := radio.NewRadioBrowser()
	stations, err := rb.Search(arg)
	if err != nil {
		sendToChannel(msg, "radio-browser search failed: "+err.Error())
		return
	}
	if len(stations) == 0 {
		sendToChannel(msg, "No stations found for \""+arg+"\".")
		return
	}

	text := buildRBTable(stations)
	if cfg.Bot.FormattedReplies {
		sendToChannel(msg, "<pre>"+esc(text)+"</pre>")
	} else {
		sendToChannel(msg, text)
	}
}

// handleRBPlay fetches a station by UUID, validates its stream, and enqueues it.
func handleRBPlay(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	cfg := bot.Config()
	if arg == "" {
		sendToChannel(msg, "Usage: "+symbol(cfg)+"rbplay <uuid>")
		return
	}

	rb := radio.NewRadioBrowser()
	station, err := rb.ByUUID(arg)
	if err != nil {
		sendToChannel(msg, "Station not found: "+err.Error())
		return
	}

	item := radio.NewRadioItemFromStation(*station)
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
		sendToChannel(msg, "Usage: "+symbol(cfg)+"volume [0-100]")
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

func buildRBTable(stations []radio.Station) string {
	text := rbTableFull(stations)
	if len(text) <= maxTableLen {
		return text
	}
	text = rbTableShort(stations)
	if len(text) <= maxTableLen {
		return text
	}
	return text[:maxTableLen]
}

func rbTableFull(stations []radio.Station) string {
	const (
		wUUID    = 36
		wName    = 25
		wGenre   = 15
		wCodec   = 13
		wCountry = 7
	)
	var sb strings.Builder
	sb.WriteString("Radio-Browser results:\n")
	fmt.Fprintf(&sb, "| %-*s | %-*s | %-*s | %-*s | %-*s |\n",
		wUUID, "rbplay ID", wName, "Station Name", wGenre, "Genre", wCodec, "Codec/Bitrate", wCountry, "Country")
	fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s |\n",
		strings.Repeat("-", wUUID),
		strings.Repeat("-", wName),
		strings.Repeat("-", wGenre),
		strings.Repeat("-", wCodec),
		strings.Repeat("-", wCountry),
	)
	for _, s := range stations {
		fmt.Fprintf(&sb, "| %-*s | %-*s | %-*s | %-*s | %-*s |\n",
			wUUID, truncateField(s.UUID, wUUID),
			wName, truncateField(s.Name, wName),
			wGenre, truncateField(firstTag(s.Tags), wGenre),
			wCodec, truncateField(bitrateStr(s.Codec, s.Bitrate), wCodec),
			wCountry, truncateField(s.Country, wCountry),
		)
	}
	return sb.String()
}

func rbTableShort(stations []radio.Station) string {
	const (
		wUUID  = 36
		wName  = 35
		wGenre = 20
	)
	var sb strings.Builder
	sb.WriteString("Radio-Browser results:\n")
	fmt.Fprintf(&sb, "| %-*s | %-*s | %-*s |\n", wUUID, "rbplay ID", wName, "Station Name", wGenre, "Genre")
	fmt.Fprintf(&sb, "| %s | %s | %s |\n",
		strings.Repeat("-", wUUID),
		strings.Repeat("-", wName),
		strings.Repeat("-", wGenre),
	)
	for _, s := range stations {
		fmt.Fprintf(&sb, "| %-*s | %-*s | %-*s |\n",
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
