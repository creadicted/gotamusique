# Commands

All commands use `!` by default (`command_symbol`). Private messages are ignored. Partial names work
when unambiguous: `!hel` runs `!help`. Aliases can be changed under `[commands]`.

## Playback

| Command | Description |
|---------|-------------|
| `!radio` | List radio presets |
| `!radio <alias\|url>` | Play a preset or any direct stream URL |
| `!hls <url>` | Play an HLS/M3U8 stream |
| `!file <path>` | Play a local file, relative to `music_folder` |
| `!url <url>` | Play one track via yt-dlp |
| `!playlist <url>` | Queue a playlist (up to `max_track_playlist`) |
| `!mode [mode]` | Show or set `one-shot`, `repeat`, `random`, `autoplay`. Not saved across restarts |
| `!stop` | Stop and reset the queue position |
| `!mute` / `!unmute` | Set volume to 0 / restore it |
| `!skip` | Skip to the next queued item |
| `!clear` | Stop and empty the queue |

## Queue

| Command | Description |
|---------|-------------|
| `!queue` | List queued items |
| `!np` / `!now` | Show what is playing |

Adding an item while one is playing appends it to the queue.

```
You:  !radio soma
Bot:  Now playing: [Radio] SomaFM Groove Salad
You:  !radio jazz
Bot:  Queued: [Radio] Jazz Yeah! (position 2)
```

## Volume

```
!volume       → Volume: 80%
!volume 60    → Volume set to 60%
```

The argument is an absolute percentage (`60` = 0.6). `max_volume` is a separate ceiling: the
result is clamped to it, not scaled by it. With `max_volume = 0.5`, `!volume 40` sets 0.4 and
`!volume 80` is capped at 0.5.

## Radio Browser

`!rbquery <name> [-n N]` searches radio-browser.info (default 10 results, max 50).
`!rbplay <uuid|index>` plays a station by UUID or by index from the last search.

```
You:  !rbquery soma
Bot:  Radio-Browser results:
      | rbplay ID                            | Station Name          | Genre       | Codec/Bitrate | Country |
      | ------------------------------------ | --------------------- | ----------- | ------------- | ------- |
      | 9cf19f37-35f5-11e8-a303-52543be04c81 | SomaFM: Groove Salad  | ambient     | MP3/128       | US      |
      | 6bc2f454-f66c-11e8-b42b-52543be04c81 | SomaFM: Lush          | dream pop   | MP3/128       | US      |
      | ...

You:  !rbplay 9cf19f37-35f5-11e8-a303-52543be04c81
Bot:  Now playing: [Radio] SomaFM: Groove Salad
```

## Library

See [Music library](../configuration/library.md).

| Command | Description |
|---------|-------------|
| `!rescan` | Scan `music_folder` (incremental, one scan at a time) |
| `!listfile [regex] [-p page]` (`!list`) | List library files, filtered by regex on path and title |
| `!search <keywords> [-p page]` | Tracks matching every keyword |
| `!findtagged <tag,tag> [-p page]` (`!ft`) | Tracks carrying all the tags |
| `!shortlist <n> [n ...]\|*` (`!sl`) | Queue entries from the last list |
| `!tag <tag,tag>` | Queue every track carrying all the tags |
| `!addtag [n\|*] <tag,tag>` | Tag queued library files |
| `!untag [n\|*] <tag,tag\|*>` | Remove tags from queued library files |

## Other

| Command | Description |
|---------|-------------|
| `!joinme` | Move the bot to your channel |
| `!help` | List commands |
| `!kill` | **Admin only.** Disconnect and exit |
