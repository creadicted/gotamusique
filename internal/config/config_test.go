package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_HappyPath(t *testing.T) {
	cfg, err := Load("testdata/valid.ini")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Host != "myhost.example.com" {
		t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "myhost.example.com")
	}
	if cfg.Bot.Username != "testbot" {
		t.Errorf("Bot.Username = %q, want %q", cfg.Bot.Username, "testbot")
	}
	if cfg.Radio["jazz"].URL != "http://jazz-wr04.ice.infomaniak.ch/jazz-wr04-128.mp3" {
		t.Errorf("Radio[jazz].URL = %q", cfg.Radio["jazz"].URL)
	}
	if cfg.Radio["jazz"].Comment != "Jazz Yeah !" {
		t.Errorf("Radio[jazz].Comment = %q, want %q", cfg.Radio["jazz"].Comment, "Jazz Yeah !")
	}
	if len(cfg.Commands.Symbol) == 0 || cfg.Commands.Symbol[0] != "!" {
		t.Errorf("Commands.Symbol = %v, want [!]", cfg.Commands.Symbol)
	}
	if cfg.Commands.Aliases["play_radio"][0] != "radio" {
		t.Errorf("Commands.Aliases[play_radio] = %v", cfg.Commands.Aliases["play_radio"])
	}
}

func TestLoad_UnknownKey(t *testing.T) {
	_, err := Load("testdata/unknown_key.ini")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown key") {
		t.Errorf("error %q does not mention 'unknown key'", err.Error())
	}
	if !strings.Contains(err.Error(), "invalid_key") {
		t.Errorf("error %q does not name the offending key", err.Error())
	}
}

func TestLoad_MissingUserFile(t *testing.T) {
	cfg, err := Load("testdata/does_not_exist.ini")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Server.Host = %q, want default 127.0.0.1", cfg.Server.Host)
	}
	if cfg.Server.Port != 64738 {
		t.Errorf("Server.Port = %d, want default 64738", cfg.Server.Port)
	}
	if cfg.Bot.Username != "gotamusique" {
		t.Errorf("Bot.Username = %q, want default gotamusique", cfg.Bot.Username)
	}
	if !cfg.Server.TLSSkipVerify {
		t.Errorf("Server.TLSSkipVerify = false, want default true")
	}
}

func TestLoad_YoutubeDLDefaults(t *testing.T) {
	cfg, err := Load("testdata/does_not_exist.ini")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bot.MaxTrackDuration != 60 || cfg.Bot.MaxTrackPlaylist != 20 {
		t.Errorf("limits = %d/%d, want 60/20", cfg.Bot.MaxTrackDuration, cfg.Bot.MaxTrackPlaylist)
	}
	if cfg.YoutubeDL.Binary != "" || cfg.YoutubeDL.CookieFile != "" {
		t.Errorf("unexpected youtube_dl defaults: %+v", cfg.YoutubeDL)
	}
	if got := cfg.Commands.Aliases["play_url"]; len(got) != 1 || got[0] != "url" {
		t.Errorf("play_url aliases = %v", got)
	}
	if got := cfg.Commands.Aliases["play_playlist"]; len(got) != 1 || got[0] != "playlist" {
		t.Errorf("play_playlist aliases = %v", got)
	}
}

func TestLoad_PlaybackModeDefault(t *testing.T) {
	cfg, err := Load("testdata/does_not_exist.ini")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bot.PlaybackMode != "one-shot" {
		t.Errorf("PlaybackMode = %q, want one-shot", cfg.Bot.PlaybackMode)
	}
	if got := cfg.Commands.Aliases["mode"]; len(got) != 1 || got[0] != "mode" {
		t.Errorf("mode aliases = %v", got)
	}
}

func TestLoad_LibraryDefaults(t *testing.T) {
	cfg, err := Load("testdata/does_not_exist.ini")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bot.MusicDatabasePath != "music.db" {
		t.Errorf("MusicDatabasePath = %q", cfg.Bot.MusicDatabasePath)
	}
	if cfg.Bot.RefreshCacheOnStart {
		t.Error("refresh_cache_on_startup must default to false")
	}
	if cfg.Bot.AutoplayLength != 5 {
		t.Errorf("AutoplayLength = %d, want 5", cfg.Bot.AutoplayLength)
	}
	if len(cfg.Files.IgnoredFolders) != 0 || len(cfg.Files.IgnoredFiles) != 0 {
		t.Errorf("ignore lists must default empty: %+v", cfg.Files)
	}
	if got := cfg.Commands.Aliases["rescan"]; len(got) != 1 || got[0] != "rescan" {
		t.Errorf("rescan aliases = %v", got)
	}
}

func TestLoad_LibraryOverrides(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.ini")
	os.WriteFile(p, []byte(`[bot]
music_database_path = data/lib.db
refresh_cache_on_startup = True
autoplay_length = 12
[files]
music_folder = /srv/music
ignored_folders = tmp, @eaDir ,.Trash
ignored_files = Thumbs.db,.DS_Store
`), 0o644)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bot.MusicDatabasePath != "data/lib.db" || !cfg.Bot.RefreshCacheOnStart || cfg.Bot.AutoplayLength != 12 {
		t.Errorf("bot = %+v", cfg.Bot)
	}
	if got := strings.Join(cfg.Files.IgnoredFolders, "|"); got != "tmp|@eaDir|.Trash" {
		t.Errorf("IgnoredFolders = %q (entries must be trimmed)", got)
	}
	if got := strings.Join(cfg.Files.IgnoredFiles, "|"); got != "Thumbs.db|.DS_Store" {
		t.Errorf("IgnoredFiles = %q", got)
	}
}

func TestLoad_BadAutoplayLengthFallsBackToDefault(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.ini")
	os.WriteFile(p, []byte("[bot]\nautoplay_length = lots\n"), 0o644)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bot.AutoplayLength != 5 {
		t.Errorf("AutoplayLength = %d, want the default 5", cfg.Bot.AutoplayLength)
	}
}

func TestLoad_AutoPauseDefaultsAndOverride(t *testing.T) {
	cfg, err := Load("testdata/valid.ini")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bot.AutoPauseWhenAlone {
		t.Error("auto_pause_when_alone default = true, want false")
	}
	if !cfg.Bot.AutoResumeOnJoin {
		t.Error("auto_resume_on_join default = false, want true")
	}

	p := filepath.Join(t.TempDir(), "c.ini")
	os.WriteFile(p, []byte("[bot]\nauto_pause_when_alone = True\nauto_resume_on_join = False\n"), 0o644)
	cfg, err = Load(p)
	if err != nil {
		t.Fatalf("new keys rejected: %v", err)
	}
	if !cfg.Bot.AutoPauseWhenAlone || cfg.Bot.AutoResumeOnJoin {
		t.Errorf("override not applied: %+v", cfg.Bot)
	}
	if got := cfg.Commands.Aliases["pause"]; len(got) == 0 || got[0] != "pause" {
		t.Errorf("pause alias = %v", got)
	}
}
