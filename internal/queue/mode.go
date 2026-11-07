package queue

import (
	"errors"
	"fmt"
	"strings"
)

// Mode controls what Next does when the end of the queue is reached.
type Mode string

const (
	// ModeOneShot stops at the end of the queue (the default).
	ModeOneShot Mode = "one-shot"
	// ModeRepeat wraps around to the first item.
	ModeRepeat Mode = "repeat"
	// ModeRandom plays the queue in shuffled order and reshuffles at the end.
	ModeRandom Mode = "random"
	// ModeAutoplay behaves like one-shot inside the queue (Next goes idle at the
	// end); the bot refills the idle queue with random tracks from the music
	// library. The queue itself never touches the database.
	ModeAutoplay Mode = "autoplay"
)

// Modes lists the modes available to users, in display order.
var Modes = []Mode{ModeOneShot, ModeRepeat, ModeRandom, ModeAutoplay}

// ErrUnknownMode is returned by ParseMode for an unrecognised name.
var ErrUnknownMode = errors.New("unknown playback mode")

// ParseMode converts a user- or config-supplied name to a Mode. It is
// case-insensitive and ignores surrounding whitespace.
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "one-shot":
		return ModeOneShot, nil
	case "repeat":
		return ModeRepeat, nil
	case "random":
		return ModeRandom, nil
	case "autoplay":
		return ModeAutoplay, nil
	}
	return "", fmt.Errorf("%w %q", ErrUnknownMode, s)
}

// ModeList returns the available mode names joined for display.
func ModeList() string {
	names := make([]string, len(Modes))
	for i, m := range Modes {
		names[i] = string(m)
	}
	return strings.Join(names, ", ")
}
