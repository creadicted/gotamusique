package version

import (
	"strings"
	"testing"
)

func TestGet_InjectedVersionWins(t *testing.T) {
	old := Version
	defer func() { Version = old }()
	for in, want := range map[string]string{"0.4.0": "0.4.0", "v0.4.0": "0.4.0", " v1.2.3-rc.1 ": "1.2.3-rc.1"} {
		Version = in
		if got := Get(); got != want {
			t.Errorf("Version=%q: Get() = %q, want %q", in, got, want)
		}
	}
}

func TestGet_UnsetFallsBackToDev(t *testing.T) {
	old := Version
	defer func() { Version = old }()
	for _, in := range []string{"", "  ", "v"} {
		Version = in
		if got := Get(); !strings.HasPrefix(got, "dev") {
			t.Errorf("Version=%q: Get() = %q, want a dev version", in, got)
		}
	}
}
