// Package version reports the running build's version.
//
// Release builds set Version with -ldflags "-X .../internal/version.Version=0.4.0"
// (the release workflow, the Dockerfile and `make build` all do). A plain
// `go build` or `go run` leaves it empty and Get falls back to the VCS revision
// Go embeds in the binary, so a development build is still identifiable.
package version

import (
	"runtime/debug"
	"strings"
)

// Version is set at build time. Empty means "not a release build".
var Version string

// Get returns the version string: the injected release version, else
// "dev (<short revision>[, modified])" from the embedded build info, else "dev".
func Get() string {
	if v := strings.TrimPrefix(strings.TrimSpace(Version), "v"); v != "" {
		return v
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	var rev string
	modified := false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if rev == "" {
		return "dev"
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if modified {
		return "dev (" + rev + ", modified)"
	}
	return "dev (" + rev + ")"
}
