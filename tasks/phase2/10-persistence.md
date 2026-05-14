# 2-10 — Playlist Persistence

**Status:** todo  
**Depends on:** 2-04, 2-01  
**Unlocks:** nothing

## Objective

Save the playlist to the settings DB on shutdown and restore it on startup.

## Behaviour

Controlled by `config.Bot.SavePlaylist` (requires `save_music_library = true`).

- `playlist.Save(db)`: on SIGINT and clean exit, serialise each item as a dict and store in settings DB under `[playlist]`
- `playlist.Load(db, cache)`: on startup after connecting, deserialise and reconstruct

Items that no longer exist (file deleted, URL expired) are silently skipped.

## Storage format (settings DB)

```
section="playlist", option="items",         value="[{...}, ...]"  (JSON)
section="playlist", option="current_index", value="3"
section="playlist", option="playback_mode", value="one-shot"
```

## Deliverables

- `internal/playlist/persist.go` — `Save` and `Load`
- Tests: save → load round-trip produces identical playlist

## Acceptance criteria

- File items survive a restart and are playable
- Radio items are restored as pending (re-validated on first play)
- URL items whose tmp file is gone are re-queued as pending (re-downloaded)
- Missing items are silently dropped, no crash
