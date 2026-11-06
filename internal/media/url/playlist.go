package url

import (
	"context"
	"fmt"

	"github.com/konradk/gotamusique/internal/config"
)

// Playlist expands rawURL into unresolved items, capped at [bot]
// max_track_playlist. Entries are not resolved here; each is resolved (and its
// duration checked) by Resolve just before it plays. A single video yields one item.
func Playlist(ctx context.Context, rawURL string, cfg *config.Config) ([]*URLItem, error) {
	entries, err := FlatPlaylist(ctx, rawURL, cfg, cfg.Bot.MaxTrackPlaylist)
	if err != nil {
		return nil, err
	}
	items := make([]*URLItem, 0, len(entries))
	for _, e := range entries {
		if e.entryURL() == "" {
			continue
		}
		if err := CheckURL(e.entryURL()); err != nil {
			continue
		}
		items = append(items, newFromEntry(e, cfg))
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no playable entries found")
	}
	return items, nil
}
