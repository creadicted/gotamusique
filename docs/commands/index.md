# Commands

All commands use `!` by default (`command_symbol`). Private messages are ignored. Partial names work
when unambiguous: `!hel` runs `!help`. Aliases can be changed under `[commands]`.

## Playback

| Command | Description |
|---------|-------------|
| `!radio` | List radio presets |
| `!radio <alias\|url>` | Play a preset or any direct stream URL |
| `!hls <url>` | Play an HLS/M3U8 stream |
| `!file <path\|folder\|name part>` (`!f`) | Queue a library file or folder; see [Library](#library) |
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
| `!last` | Jump to the last queued item and play it (any mode) |
| `!random` | Shuffle the upcoming items; the current track keeps playing |
| `!repeat [n]` | Play the current item `n` more times (1-200, default 1) |

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

See [Music library](../configuration/library.md) for setup and the scan.

| Command | Description |
|---------|-------------|
| `!rescan` | Scan `music_folder` (incremental, one scan at a time) |
| `!rescan force` | Scan and, if no audio files are found, clear the library. A plain `!rescan` never does |
| `!listfile [regex] [-p page]` (`!list`) | List library files, filtered by a case-insensitive regex on path and title |
| `!search <keywords> [-p page]` | Tracks matching every keyword (`%` and `_` match literally) |
| `!findtagged <tag,tag> [-p page]` (`!ft`) | Tracks carrying all the tags |
| `!shortlist <n> [n ...]\|*` (`!sl`) | Queue entries from the last list |
| `!shortlist -p <page>` | Show another page of the last list without queueing anything |
| `!file <path\|folder\|name part>` (`!f`) | Queue a library file, a whole folder, or a partial match |
| `!filematch <regex>` (`!fm`) | Queue every track whose path or title matches the regex |
| `!tag <tag,tag>` | Queue every track carrying all the tags |
| `!addtag [n\|*] <tag,tag>` | Tag queued library files |
| `!untag [n\|*] <tag,tag\|*>` | Remove tags from queued library files |

### Lists and the shortlist

`!listfile`, `!search` and `!findtagged` number their results (the numbers continue across pages)
and remember the last list **per channel**, so another channel's search does not change what
`!shortlist 3` means. Pages are sized to stay under Mumble's message limit; use `-p <page>` to
see the others. A trailing number is part of the query, so `!search symphony no 9` searches for
"9".

```
You:  !search beethoven
Bot:  3 tracks, page 1/1 (queue with !shortlist <n ...|*>):
      1. classical/beethoven/symphony-5.mp3 - Ludwig van Beethoven Symphony No. 5
      2. classical/beethoven/symphony-9.mp3 - Ludwig van Beethoven Symphony No. 9
      3. classical/beethoven/moonlight.mp3 - Ludwig van Beethoven Moonlight Sonata
You:  !sl 3 1
Bot:  Queued 2 tracks
```

`!shortlist` checks every number first: one bad number queues nothing.

### `!file`

`!file` resolves its argument in this order: an exact library path, then a folder (every track
below it; `rock` does not match `rockabilly/`), then a case-insensitive partial match. One partial
match is queued; several are listed so you can pick with `!shortlist`. If the library has no
match, or there is no library, the path is played straight from `music_folder`, so files that
have not been scanned yet still work.

### Tags

Tags are stored on library rows, so `!addtag` and `!untag` only work on queued **library files**;
radio, URL and stream items are skipped. They apply to the current item by default, to queue
position `n`, or to every item with `*`. Tags match whole words, case-insensitively, and survive
a rescan even when the file changed. `!untag *` clears all tags of the item.

### Bulk limit

`!file <folder>`, `!filematch`, `!tag` and `!shortlist *` queue at most **200** tracks per
command and say so when more matched; narrow the selection to get the rest.

## Other

| Command | Description |
|---------|-------------|
| `!version` | Show the bot version |
| `!joinme` | Move the bot to your channel |
| `!help` | List commands |
| `!kill` | **Admin only.** Disconnect and exit |
