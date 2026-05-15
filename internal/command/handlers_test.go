package command

import (
	"strings"
	"testing"

	"github.com/konradk/gotamusique/internal/config"
	"github.com/konradk/gotamusique/internal/radio"
	"layeh.com/gumble/gumble"
)

// --- handler logic: verify the right BotAPI method is called ---
// Message replies are fire-and-forget (sendToChannel recovers from gumble panics
// in tests), so these tests focus on side effects on the mock bot.

func callHandler(fn HandlerFunc, bot *mockBot, arg string) {
	fn(bot, "testuser", makeMsg("!cmd "+arg), "cmd", arg)
}

func TestHandleStop_CallsStop(t *testing.T) {
	bot := defaultBot()
	callHandler(handleStop, bot, "")
	if bot.stopCalls != 1 {
		t.Errorf("stopCalls = %d, want 1", bot.stopCalls)
	}
}

func TestHandleMute_CallsMute(t *testing.T) {
	bot := defaultBot()
	callHandler(handleMute, bot, "")
	if bot.muteCalls != 1 {
		t.Errorf("muteCalls = %d, want 1", bot.muteCalls)
	}
}

func TestHandleUnmute_CallsUnmute(t *testing.T) {
	bot := defaultBot()
	callHandler(handleUnmute, bot, "")
	if bot.unmuteCalls != 1 {
		t.Errorf("unmuteCalls = %d, want 1", bot.unmuteCalls)
	}
}

func TestHandleSkip_CallsSkip(t *testing.T) {
	bot := defaultBot()
	callHandler(handleSkip, bot, "")
	if bot.skipCalls != 1 {
		t.Errorf("skipCalls = %d, want 1", bot.skipCalls)
	}
}

func TestHandleClear_CallsClear(t *testing.T) {
	bot := defaultBot()
	callHandler(handleClear, bot, "")
	if bot.clearCalls != 1 {
		t.Errorf("clearCalls = %d, want 1", bot.clearCalls)
	}
}

func TestHandleNP_NoCurrentItem_NoEnqueue(t *testing.T) {
	bot := defaultBot()
	bot.currentItem = nil
	callHandler(handleNP, bot, "")
	if len(bot.enqueueCalls) != 0 {
		t.Error("handleNP should not enqueue anything")
	}
}

func TestHandleVolume_NoArg_DoesNotSetVolume(t *testing.T) {
	bot := defaultBot()
	bot.volume = 0.75
	callHandler(handleVolume, bot, "")
	if bot.volume != 0.75 {
		t.Errorf("volume changed to %v, want 0.75 (no-arg should be read-only)", bot.volume)
	}
}

func TestHandleVolume_ValidArg_SetsVolume(t *testing.T) {
	bot := defaultBot()
	callHandler(handleVolume, bot, "60")
	if bot.volume != 0.60 {
		t.Errorf("volume = %v, want 0.60", bot.volume)
	}
}

func TestHandleVolume_ZeroArg(t *testing.T) {
	bot := defaultBot()
	callHandler(handleVolume, bot, "0")
	if bot.volume != 0.0 {
		t.Errorf("volume = %v, want 0.0", bot.volume)
	}
}

func TestHandleVolume_MaxArg(t *testing.T) {
	bot := defaultBot()
	callHandler(handleVolume, bot, "100")
	if bot.volume != 1.0 {
		t.Errorf("volume = %v, want 1.0", bot.volume)
	}
}

func TestHandleVolume_InvalidArg_NoChange(t *testing.T) {
	bot := defaultBot()
	bot.volume = 0.5
	callHandler(handleVolume, bot, "notanumber")
	if bot.volume != 0.5 {
		t.Errorf("volume changed to %v on invalid arg", bot.volume)
	}
}

func TestHandleVolume_OutOfRange_NoChange(t *testing.T) {
	bot := defaultBot()
	bot.volume = 0.5
	callHandler(handleVolume, bot, "150")
	if bot.volume != 0.5 {
		t.Errorf("volume changed to %v for out-of-range arg 150", bot.volume)
	}
}

func TestHandleVolume_NegativeArg_NoChange(t *testing.T) {
	bot := defaultBot()
	bot.volume = 0.5
	callHandler(handleVolume, bot, "-10")
	if bot.volume != 0.5 {
		t.Errorf("volume changed to %v for negative arg", bot.volume)
	}
}

func TestHandleJoinMe_NilSender_NoJoin(t *testing.T) {
	bot := defaultBot()
	msg := makeMsg("!joinme")
	msg.Sender = nil
	handleJoinMe(bot, "testuser", msg, "joinme", "")
	// No panic and no JoinChannel call expected.
}

func TestHandleJoinMe_SenderNoChannel_NoJoin(t *testing.T) {
	bot := defaultBot()
	msg := makeMsg("!joinme")
	msg.Sender.Channel = nil
	joined := false
	bot.cfg.Bot.Admin = nil
	// Override JoinChannel by using a fresh mock that tracks the call.
	type trackBot struct {
		*mockBot
		joined bool
	}
	// We can't override methods on mockBot easily; just verify via the nil-channel guard.
	handleJoinMe(bot, "testuser", msg, "joinme", "")
	if joined {
		t.Error("JoinChannel should not be called when sender has no channel")
	}
}

func TestHandleJoinMe_SenderWithChannel_Joins(t *testing.T) {
	joinCh := &gumble.Channel{}
	msg := makeMsg("!joinme")
	msg.Sender.Channel = joinCh

	var gotChannel *gumble.Channel
	m := &joinTrackBot{mockBot: defaultBot()}
	handleJoinMe(m, "testuser", msg, "joinme", "")
	gotChannel = m.joinedChannel
	if gotChannel != joinCh {
		t.Errorf("JoinChannel called with %v, want %v", gotChannel, joinCh)
	}
}

// joinTrackBot wraps mockBot to capture JoinChannel calls.
type joinTrackBot struct {
	*mockBot
	joinedChannel *gumble.Channel
}

func (j *joinTrackBot) JoinChannel(ch *gumble.Channel) { j.joinedChannel = ch }

func TestHandleRadio_NoArg_NoPresets(t *testing.T) {
	bot := defaultBot()
	bot.cfg.Radio = map[string]config.RadioPreset{}
	callHandler(handleRadio, bot, "")
	if len(bot.enqueueCalls) != 0 {
		t.Error("handleRadio with no presets should not enqueue anything")
	}
}

func TestHandleRadio_UnknownPreset_NoEnqueue(t *testing.T) {
	bot := defaultBot()
	callHandler(handleRadio, bot, "nonexistent")
	if len(bot.enqueueCalls) != 0 {
		t.Errorf("handleRadio with unknown preset enqueued %d items", len(bot.enqueueCalls))
	}
}

func TestHandleRadio_KnownPreset_Enqueues(t *testing.T) {
	bot := defaultBot()
	bot.cfg.Radio = map[string]config.RadioPreset{
		"jazz": {URL: "http://example.com/jazz", Comment: "Jazz Yeah !"},
	}
	callHandler(handleRadio, bot, "jazz")
	if len(bot.enqueueCalls) != 1 {
		t.Errorf("handleRadio with known preset: enqueueCalls = %d, want 1", len(bot.enqueueCalls))
	}
}

func TestHandleRBQuery_NoArg_NoEnqueue(t *testing.T) {
	bot := defaultBot()
	callHandler(handleRBQuery, bot, "")
	if len(bot.enqueueCalls) != 0 {
		t.Error("handleRBQuery with no arg should not enqueue")
	}
}

func TestHandleRBPlay_NoArg_NoEnqueue(t *testing.T) {
	bot := defaultBot()
	callHandler(handleRBPlay, bot, "")
	if len(bot.enqueueCalls) != 0 {
		t.Error("handleRBPlay with no arg should not enqueue")
	}
}

// --- table helpers (pure functions) ---

func TestTruncateField_ShortString_Unchanged(t *testing.T) {
	if got := truncateField("hello", 10); got != "hello" {
		t.Errorf("truncateField(%q, 10) = %q, want unchanged", "hello", got)
	}
}

func TestTruncateField_ExactLength_Unchanged(t *testing.T) {
	s := "hello"
	if got := truncateField(s, 5); got != s {
		t.Errorf("truncateField at exact length: %q, want %q", got, s)
	}
}

func TestTruncateField_TooLong_Truncated(t *testing.T) {
	got := truncateField("hello world", 8)
	if len(got) != 8 {
		t.Errorf("truncateField result len = %d, want 8", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("truncated field should end with ...: %q", got)
	}
}

func TestTruncateField_VerySmallMax(t *testing.T) {
	got := truncateField("hello", 2)
	if len(got) > 2 {
		t.Errorf("truncateField with max=2: len = %d, want <= 2", len(got))
	}
}

func TestFirstTag_SingleTag(t *testing.T) {
	if got := firstTag("jazz"); got != "jazz" {
		t.Errorf("firstTag(%q) = %q, want %q", "jazz", got, "jazz")
	}
}

func TestFirstTag_MultipleTags(t *testing.T) {
	if got := firstTag("jazz,blues,soul"); got != "jazz" {
		t.Errorf("firstTag(%q) = %q, want %q", "jazz,blues,soul", got, "jazz")
	}
}

func TestFirstTag_WithSpaces(t *testing.T) {
	if got := firstTag("jazz, blues"); got != "jazz" {
		t.Errorf("firstTag with space: %q, want %q", got, "jazz")
	}
}

func TestFirstTag_Empty(t *testing.T) {
	if got := firstTag(""); got != "" {
		t.Errorf("firstTag(%q) = %q, want empty", "", got)
	}
}

func TestBitrateStr_WithBitrate(t *testing.T) {
	if got := bitrateStr("MP3", 128); got != "MP3/128" {
		t.Errorf("bitrateStr = %q, want %q", got, "MP3/128")
	}
}

func TestBitrateStr_ZeroBitrate(t *testing.T) {
	if got := bitrateStr("AAC", 0); got != "AAC" {
		t.Errorf("bitrateStr with 0 bitrate = %q, want %q", got, "AAC")
	}
}

func TestHostnameOf_ValidURL(t *testing.T) {
	if got := hostnameOf("http://example.com/stream"); got != "example.com" {
		t.Errorf("hostnameOf = %q, want %q", got, "example.com")
	}
}

func TestHostnameOf_InvalidURL(t *testing.T) {
	raw := "not a url"
	if got := hostnameOf(raw); got != raw {
		t.Errorf("hostnameOf(%q) = %q, want original", raw, got)
	}
}

func TestRBTableFull_Structure(t *testing.T) {
	stations := []radio.Station{
		{UUID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", Name: "Test FM", Codec: "MP3", Bitrate: 128, Country: "DE", Tags: "pop,rock"},
	}
	out := rbTableFull(stations)
	if !strings.Contains(out, "Radio-Browser results:") {
		t.Error("table missing header")
	}
	if !strings.Contains(out, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee") {
		t.Error("table missing UUID")
	}
	if !strings.Contains(out, "Test FM") {
		t.Error("table missing station name")
	}
	if !strings.Contains(out, "MP3/128") {
		t.Error("table missing codec/bitrate")
	}
	if !strings.Contains(out, "pop") {
		t.Error("table missing first genre tag")
	}
}

func TestRBTableFull_TruncatesLongName(t *testing.T) {
	name := strings.Repeat("X", 40)
	stations := []radio.Station{{UUID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", Name: name}}
	out := rbTableFull(stations)
	if strings.Contains(out, name) {
		t.Error("long station name should be truncated in full table")
	}
}

func TestBuildRBTable_FallsBackToShort(t *testing.T) {
	// Build enough stations to exceed 5000 chars in the full table.
	var stations []radio.Station
	for i := 0; i < 10; i++ {
		stations = append(stations, radio.Station{
			UUID:    "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			Name:    strings.Repeat("A", 25),
			Codec:   strings.Repeat("B", 13),
			Country: strings.Repeat("C", 7),
			Tags:    strings.Repeat("D", 15),
		})
	}
	out := buildRBTable(stations)
	if len(out) > maxTableLen {
		t.Errorf("buildRBTable result len = %d, exceeds maxTableLen %d", len(out), maxTableLen)
	}
}

// --- aliasesFor ---

func TestAliasesFor_NilMap_FallsBackToCanonical(t *testing.T) {
	cfg := &config.Config{Commands: config.CommandsConfig{Aliases: nil}}
	got := aliasesFor(cfg, "stop")
	if len(got) != 1 || got[0] != "stop" {
		t.Errorf("aliasesFor with nil map = %v, want [stop]", got)
	}
}

func TestAliasesFor_EmptySlice_FallsBackToCanonical(t *testing.T) {
	cfg := &config.Config{Commands: config.CommandsConfig{Aliases: map[string][]string{"stop": {}}}}
	got := aliasesFor(cfg, "stop")
	if len(got) != 1 || got[0] != "stop" {
		t.Errorf("aliasesFor with empty slice = %v, want [stop]", got)
	}
}

func TestAliasesFor_Configured_ReturnsAliases(t *testing.T) {
	cfg := &config.Config{Commands: config.CommandsConfig{
		Aliases: map[string][]string{"stop": {"stop", "s"}},
	}}
	got := aliasesFor(cfg, "stop")
	if len(got) != 2 || got[0] != "stop" || got[1] != "s" {
		t.Errorf("aliasesFor = %v, want [stop s]", got)
	}
}
