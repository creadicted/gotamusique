package library

import "time"

// Track is one scanned library entry as exposed to the chat commands and the web
// API. It lives here, not in the command package, so every front end shares it.
type Track struct {
	Path     string // relative to music_folder; usable with !file
	Title    string
	Artist   string
	Tags     []string
	Duration time.Duration
}

// LookupKind says how a file argument matched the library.
type LookupKind int

const (
	LookupExact   LookupKind = iota // the argument is a library path
	LookupFolder                    // the argument is a folder: Tracks are all below it
	LookupPartial                   // case-insensitive substring match on paths (may be empty)
)

// FileLookup is the result of a file lookup.
type FileLookup struct {
	Kind   LookupKind
	Tracks []Track
}
