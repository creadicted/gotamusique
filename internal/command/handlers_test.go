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
	msg := makeMsg("!joinme")
	msg.Sender.Channel = nil
	m := &joinTrackBot{mockBot: defaultBot()}
	handleJoinMe(m, "testuser", msg, "joinme", "")
	if m.joinedChannel != nil {
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
	callHandler(makeRBQueryHandler(newRBCache(), nil), bot, "")
	if len(bot.enqueueCalls) != 0 {
		t.Error("handleRBQuery with no arg should not enqueue")
	}
}

func TestHandleRBPlay_NoArg_NoEnqueue(t *testing.T) {
	bot := defaultBot()
	callHandler(makeRBPlayHandler(newRBCache(), nil), bot, "")
	if len(bot.enqueueCalls) != 0 {
		t.Error("handleRBPlay with no arg should not enqueue")
	}
}

// --- parseRBQueryArg ---

func TestParseRBQueryArg_noFlag(t *testing.T) {
	name, limit, err := parseRBQueryArg("soma")
	if err != nil || name != "soma" || limit != defaultSearchLimit {
		t.Errorf("got (%q, %d, %v), want (soma, %d, nil)", name, limit, err, defaultSearchLimit)
	}
}

func TestParseRBQueryArg_shortFlag(t *testing.T) {
	name, limit, err := parseRBQueryArg("soma -n 20")
	if err != nil || name != "soma" || limit != 20 {
		t.Errorf("got (%q, %d, %v), want (soma, 20, nil)", name, limit, err)
	}
}

func TestParseRBQueryArg_longFlag(t *testing.T) {
	name, limit, err := parseRBQueryArg("soma --limit 20")
	if err != nil || name != "soma" || limit != 20 {
		t.Errorf("got (%q, %d, %v), want (soma, 20, nil)", name, limit, err)
	}
}

func TestParseRBQueryArg_capLimit(t *testing.T) {
	name, limit, err := parseRBQueryArg("soma --limit 999")
	if err != nil || name != "soma" || limit != maxSearchLimit {
		t.Errorf("got (%q, %d, %v), want (soma, %d, nil)", name, limit, err, maxSearchLimit)
	}
}

func TestParseRBQueryArg_invalidLimit(t *testing.T) {
	_, _, err := parseRBQueryArg("soma -n abc")
	if err == nil {
		t.Error("expected error for non-integer limit")
	}
}

func TestParseRBQueryArg_zeroLimit(t *testing.T) {
	_, _, err := parseRBQueryArg("soma -n 0")
	if err == nil {
		t.Error("expected error for zero limit")
	}
}

func TestParseRBQueryArg_flagWithoutValue(t *testing.T) {
	_, _, err := parseRBQueryArg("soma -n")
	if err == nil {
		t.Error("expected error for flag without value")
	}
}

func TestParseRBQueryArg_emptyNameWithFlag(t *testing.T) {
	_, _, err := parseRBQueryArg("-n 20")
	if err == nil {
		t.Error("expected error when station name is empty")
	}
}

func TestParseRBQueryArg_multiWordName(t *testing.T) {
	name, limit, err := parseRBQueryArg("bbc radio 4 -n 20")
	if err != nil || name != "bbc radio 4" || limit != 20 {
		t.Errorf("got (%q, %d, %v), want (bbc radio 4, 20, nil)", name, limit, err)
	}
}

// --- handleRBQuery handler tests ---

func TestHandleRBQuery_defaultLimit(t *testing.T) {
	var capturedLimit int
	searchFn := func(name string, limit int) ([]radio.Station, error) {
		capturedLimit = limit
		return []radio.Station{{UUID: "abc", Name: "Test FM"}}, nil
	}
	callHandler(makeRBQueryHandler(newRBCache(), searchFn), defaultBot(), "soma")
	if capturedLimit != defaultSearchLimit {
		t.Errorf("limit = %d, want %d", capturedLimit, defaultSearchLimit)
	}
}

func TestHandleRBQuery_customLimit(t *testing.T) {
	var capturedLimit int
	searchFn := func(name string, limit int) ([]radio.Station, error) {
		capturedLimit = limit
		return []radio.Station{{UUID: "abc", Name: "Test FM"}}, nil
	}
	callHandler(makeRBQueryHandler(newRBCache(), searchFn), defaultBot(), "soma -n 20")
	if capturedLimit != 20 {
		t.Errorf("limit = %d, want 20", capturedLimit)
	}
}

func TestHandleRBQuery_limitCap(t *testing.T) {
	var capturedLimit int
	searchFn := func(name string, limit int) ([]radio.Station, error) {
		capturedLimit = limit
		return []radio.Station{{UUID: "abc", Name: "Test FM"}}, nil
	}
	callHandler(makeRBQueryHandler(newRBCache(), searchFn), defaultBot(), "soma --limit 999")
	if capturedLimit != maxSearchLimit {
		t.Errorf("limit = %d, want %d (capped)", capturedLimit, maxSearchLimit)
	}
}

func TestHandleRBQuery_invalidFlag(t *testing.T) {
	called := false
	searchFn := func(name string, limit int) ([]radio.Station, error) {
		called = true
		return nil, nil
	}
	callHandler(makeRBQueryHandler(newRBCache(), searchFn), defaultBot(), "soma -n abc")
	if called {
		t.Error("searchFn should not be called when flag parsing fails")
	}
}

func TestHandleRBQuery_setsCache(t *testing.T) {
	stations := []radio.Station{
		{UUID: "abc", Name: "Test FM"},
		{UUID: "def", Name: "Jazz FM"},
	}
	searchFn := func(name string, limit int) ([]radio.Station, error) {
		return stations, nil
	}
	cache := newRBCache()
	callHandler(makeRBQueryHandler(cache, searchFn), defaultBot(), "soma")

	got := cache.get(0) // makeMsg creates a channel with ID=0
	if len(got) != 2 {
		t.Fatalf("cache has %d stations, want 2", len(got))
	}
	if got[0].UUID != "abc" || got[1].UUID != "def" {
		t.Errorf("cache content mismatch: %+v", got)
	}
}

// --- handleRBPlay handler tests ---

func TestHandleRBPlay_indexResolvesFromCache(t *testing.T) {
	stations := []radio.Station{
		{UUID: "aaa", Name: "First FM", URL: "http://example.com/1"},
		{UUID: "bbb", Name: "Second FM", URL: "http://example.com/2"},
	}
	cache := newRBCache()
	cache.set(0, stations)

	bot := defaultBot()
	callHandler(makeRBPlayHandler(cache, nil), bot, "2")

	if len(bot.enqueueCalls) != 1 {
		t.Fatalf("expected 1 enqueue, got %d", len(bot.enqueueCalls))
	}
	if got := bot.enqueueCalls[0].FormatTitle(); got != "[Radio] Second FM" {
		t.Errorf("enqueued %q, want %q", got, "[Radio] Second FM")
	}
}

func TestHandleRBPlay_indexCacheMiss(t *testing.T) {
	bot := defaultBot()
	callHandler(makeRBPlayHandler(newRBCache(), nil), bot, "1")
	if len(bot.enqueueCalls) != 0 {
		t.Error("cache miss should not enqueue anything")
	}
}

func TestHandleRBPlay_indexOutOfRange(t *testing.T) {
	stations := []radio.Station{
		{UUID: "aaa", Name: "A"},
		{UUID: "bbb", Name: "B"},
		{UUID: "ccc", Name: "C"},
	}
	cache := newRBCache()
	cache.set(0, stations)

	bot := defaultBot()
	callHandler(makeRBPlayHandler(cache, nil), bot, "99")
	if len(bot.enqueueCalls) != 0 {
		t.Error("out-of-range index should not enqueue anything")
	}
}

func TestHandleRBPlay_uuidFlowUnchanged(t *testing.T) {
	const uuid = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	station := &radio.Station{UUID: uuid, Name: "Test FM", URL: "http://example.com/stream"}

	var capturedUUID string
	byUUIDFn := func(u string) (*radio.Station, error) {
		capturedUUID = u
		return station, nil
	}

	bot := defaultBot()
	callHandler(makeRBPlayHandler(newRBCache(), byUUIDFn), bot, uuid)

	if capturedUUID != uuid {
		t.Errorf("byUUIDFn called with %q, want %q", capturedUUID, uuid)
	}
	if len(bot.enqueueCalls) != 1 {
		t.Errorf("expected 1 enqueue, got %d", len(bot.enqueueCalls))
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
	out := rbTableFull("test", stations)
	if !strings.Contains(out, "Radio-Browser results for") {
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
	if !strings.Contains(out, "#") {
		t.Error("table missing index column header")
	}
}

func TestRBTableFull_IndexColumn(t *testing.T) {
	stations := []radio.Station{
		{UUID: "aaa", Name: "First"},
		{UUID: "bbb", Name: "Second"},
	}
	out := rbTableFull("test", stations)
	if !strings.Contains(out, "1 ") {
		t.Error("table missing index 1")
	}
	if !strings.Contains(out, "2 ") {
		t.Error("table missing index 2")
	}
}

func TestRBTableFull_TruncatesLongName(t *testing.T) {
	name := strings.Repeat("X", 40)
	stations := []radio.Station{{UUID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", Name: name}}
	out := rbTableFull("test", stations)
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
	out := buildRBTable("test", stations)
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
