package config

import (
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
