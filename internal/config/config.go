package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/ini.v1"
)

//go:embed configuration.default.ini
var defaultINI []byte

type Config struct {
	Server    ServerConfig
	Bot       BotConfig
	Files     FilesConfig
	YoutubeDL YoutubeDLConfig
	Commands  CommandsConfig
	Radio     map[string]RadioPreset
	Debug     DebugConfig
}

type ServerConfig struct {
	Host          string
	Port          int
	Password      string
	Channel       string
	Certificate   string
	Tokens        []string
	TLSSkipVerify bool
}

type BotConfig struct {
	Username             string
	Volume               float64
	MaxVolume            float64
	Bandwidth            int
	Admin                []string
	Comment              string
	Avatar               string
	Stereo               bool
	Logfile              string
	AnnounceCurrentMusic bool
	FormattedReplies     bool
	MaxTrackDuration     int    // minutes; 0 = no limit
	MaxTrackPlaylist     int    // max entries added by the playlist command
	PlaybackMode         string // startup playback mode: one-shot | repeat | random | autoplay
	MusicDatabasePath    string // music library database file
	RefreshCacheOnStart  bool   // run a library scan at startup
	AutoplayLength       int    // tracks added per autoplay refill
	AutoPauseWhenAlone   bool   // pause when the last other user leaves the bot's channel
	AutoResumeOnJoin     bool   // resume an auto-pause when someone is back (never a manual !pause)
}

// YoutubeDLConfig holds the [youtube_dl] section (yt-dlp settings).
type YoutubeDLConfig struct {
	Binary        string // yt-dlp executable; empty = "yt-dlp" from PATH
	CookieFile    string
	SourceAddress string
	UserAgent     string
}

type CommandsConfig struct {
	Symbol  []string            // colon-split, e.g. ["!", "！"]
	Aliases map[string][]string // canonical name → alias list
}

type RadioPreset struct {
	URL     string
	Comment string
}

type FilesConfig struct {
	MusicFolder    string
	IgnoredFolders []string // exact folder names skipped by the scan, at any depth
	IgnoredFiles   []string // exact file names skipped by the scan
}

type DebugConfig struct {
	Ffmpeg           bool
	MumbleConnection bool
}

var sectionAllowlists = map[string]map[string]bool{
	"server": {
		"host": true, "port": true, "password": true,
		"channel": true, "certificate": true, "tokens": true,
		"tls_skip_verify": true,
	},
	"bot": {
		"username": true, "volume": true, "max_volume": true,
		"bandwidth": true, "admin": true, "comment": true,
		"avatar": true, "stereo": true, "logfile": true,
		"announce_current_music":   true,
		"formatted_replies":        true,
		"max_track_duration":       true,
		"max_track_playlist":       true,
		"playback_mode":            true,
		"music_database_path":      true,
		"refresh_cache_on_startup": true,
		"autoplay_length":          true,
		"auto_pause_when_alone":    true,
		"auto_resume_on_join":      true,
	},
	"youtube_dl": {
		"binary": true, "cookiefile": true,
		"source_address": true, "user_agent": true,
	},
	"files": {
		"music_folder": true, "ignored_folders": true, "ignored_files": true,
	},
	"debug": {
		"ffmpeg": true, "mumble_connection": true,
	},
}

// DefaultUserConfigPath returns the path to configuration.ini in the same
// directory as the running executable.
func DefaultUserConfigPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "configuration.ini"
	}
	return filepath.Join(filepath.Dir(exe), "configuration.ini")
}

// Load reads the embedded defaults, optionally overlays userPath, and returns
// a populated Config. A missing userPath is not an error.
func Load(userPath string) (*Config, error) {
	_, statErr := os.Stat(userPath)
	userExists := statErr == nil

	if userExists {
		userIni, err := ini.Load(userPath)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", userPath, err)
		}
		if err := validateUserFile(userIni); err != nil {
			return nil, err
		}
	}

	sources := []interface{}{defaultINI}
	if userExists {
		sources = append(sources, userPath)
	}

	merged, err := ini.Load(sources[0], sources[1:]...)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	return build(merged)
}

func validateUserFile(f *ini.File) error {
	for _, section := range f.Sections() {
		name := section.Name()
		if name == ini.DefaultSection {
			continue
		}
		allowlist, known := sectionAllowlists[name]
		if !known {
			continue // unknown section: silently skip
		}
		for _, key := range section.Keys() {
			if !allowlist[key.Name()] {
				return fmt.Errorf("unknown key %q in section [%s]", key.Name(), name)
			}
		}
	}
	return nil
}

func build(f *ini.File) (*Config, error) {
	s := f.Section("server")
	b := f.Section("bot")
	fi := f.Section("files")
	yt := f.Section("youtube_dl")
	d := f.Section("debug")

	cfg := &Config{
		Server: ServerConfig{
			Host:          s.Key("host").String(),
			Port:          s.Key("port").MustInt(64738),
			Password:      s.Key("password").String(),
			Channel:       s.Key("channel").String(),
			Certificate:   s.Key("certificate").String(),
			Tokens:        splitComma(s.Key("tokens").String()),
			TLSSkipVerify: s.Key("tls_skip_verify").MustBool(true),
		},
		Bot: BotConfig{
			Username:             b.Key("username").String(),
			Volume:               b.Key("volume").MustFloat64(0.8),
			MaxVolume:            b.Key("max_volume").MustFloat64(1.0),
			Bandwidth:            b.Key("bandwidth").MustInt(96000),
			Admin:                splitComma(b.Key("admin").String()),
			Comment:              b.Key("comment").String(),
			Avatar:               b.Key("avatar").String(),
			Stereo:               b.Key("stereo").MustBool(true),
			Logfile:              b.Key("logfile").String(),
			AnnounceCurrentMusic: b.Key("announce_current_music").MustBool(true),
			FormattedReplies:     b.Key("formatted_replies").MustBool(true),
			MaxTrackDuration:     b.Key("max_track_duration").MustInt(60),
			MaxTrackPlaylist:     b.Key("max_track_playlist").MustInt(20),
			PlaybackMode:         b.Key("playback_mode").MustString("one-shot"),
			MusicDatabasePath:    b.Key("music_database_path").MustString("music.db"),
			RefreshCacheOnStart:  b.Key("refresh_cache_on_startup").MustBool(false),
			AutoplayLength:       b.Key("autoplay_length").MustInt(5),
			AutoPauseWhenAlone:   b.Key("auto_pause_when_alone").MustBool(false),
			AutoResumeOnJoin:     b.Key("auto_resume_on_join").MustBool(true),
		},
		YoutubeDL: YoutubeDLConfig{
			Binary:        yt.Key("binary").String(),
			CookieFile:    yt.Key("cookiefile").String(),
			SourceAddress: yt.Key("source_address").String(),
			UserAgent:     yt.Key("user_agent").String(),
		},
		Files: FilesConfig{
			MusicFolder:    fi.Key("music_folder").String(),
			IgnoredFolders: splitComma(fi.Key("ignored_folders").String()),
			IgnoredFiles:   splitComma(fi.Key("ignored_files").String()),
		},
		Debug: DebugConfig{
			Ffmpeg:           d.Key("ffmpeg").MustBool(false),
			MumbleConnection: d.Key("mumble_connection").MustBool(false),
		},
	}

	cfg.Commands = parseCommands(f.Section("commands"))

	var err error
	cfg.Radio, err = parseRadio(f.Section("radio"))
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func parseCommands(section *ini.Section) CommandsConfig {
	cc := CommandsConfig{
		Symbol:  []string{"!"},
		Aliases: make(map[string][]string),
	}
	for _, key := range section.Keys() {
		if key.Name() == "command_symbol" {
			cc.Symbol = strings.Split(key.Value(), ":")
		} else {
			cc.Aliases[key.Name()] = splitComma(key.Value())
		}
	}
	return cc
}

func parseRadio(section *ini.Section) (map[string]RadioPreset, error) {
	presets := make(map[string]RadioPreset)
	for _, key := range section.Keys() {
		preset, err := parseRadioValue(key.Value())
		if err != nil {
			return nil, fmt.Errorf("radio preset %q: %w", key.Name(), err)
		}
		presets[key.Name()] = preset
	}
	return presets, nil
}

func parseRadioValue(value string) (RadioPreset, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return RadioPreset{}, fmt.Errorf("empty value")
	}
	parts := strings.SplitN(value, " ", 2)
	url := parts[0]
	if url == "" {
		return RadioPreset{}, fmt.Errorf("empty URL")
	}
	comment := ""
	if len(parts) > 1 {
		rest := strings.TrimSpace(parts[1])
		if len(rest) >= 2 && rest[0] == '"' && rest[len(rest)-1] == '"' {
			comment = rest[1 : len(rest)-1]
		} else {
			comment = rest
		}
	}
	return RadioPreset{URL: url, Comment: comment}, nil
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			result = append(result, t)
		}
	}
	return result
}
