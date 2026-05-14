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
	Server   ServerConfig
	Bot      BotConfig
	Commands CommandsConfig
	Radio    map[string]RadioPreset
	Debug    DebugConfig
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
}

type CommandsConfig struct {
	Symbol  []string            // colon-split, e.g. ["!", "！"]
	Aliases map[string][]string // canonical name → alias list
}

type RadioPreset struct {
	URL     string
	Comment string
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
		"announce_current_music": true,
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
