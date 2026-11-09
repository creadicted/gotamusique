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
| `!pause` | Halt playback and keep the queue position. See [Pause and resume](#pause-and-resume) |
| `!resume` | Continue from the current item after a pause |
| `!stop` | Stop and reset the queue position to the first item |
| `!mute` / `!unmute` | Set volume to 0 / restore it |
| `!skip` | Skip to the next queued item |
| `!clear` | Stop and empty the queue |

## Pause and resume

`!pause` stops playback but leaves the queue where it is; `!stop` rewinds to the first item.
`!resume` continues the current item.

- **Local files** continue **from where you paused**. The bot announces it, for example
  `Now playing: ... (resumed at 1:24)`. The position is the audio sent to Mumble, so it can be a
  fraction of a second early, never late. It also applies to an auto-pause.
- **Live radio and HLS streams** **rejoin the live stream**: it kept broadcasting while you were
  paused, so you hear what is on air now. There is nothing to seek back to.
- **yt-dlp tracks (`!url`, `!playlist`)** restart from the beginning. Seeking them is not supported yet.

A paused bot runs no ffmpeg and downloads nothing.

### After a restart

The bot remembers the last item it played. When it is **not paused** and the queue is empty,
`!resume` plays that item again: a local file from where it stopped, a radio or HLS stream live,
and a yt-dlp track from the start.

```
You:  !resume
Bot:  Resuming: Band - Song (at 1:24)
```

It works once per start and only on an empty queue, so it never replaces what you queued. If the
file was deleted or `music_folder` is not set, the reply is `Not paused.` The position is saved when
you pause, when the bot shuts down cleanly, and when a track starts. After a crash you get the item
back, but from the position at which it started. Set `last_played_path =` (empty) to turn this off.

`!skip`, `!clear`, `!stop` and playing something new (for example `!radio`) end a pause and forget
the saved position.

### Auto-pause

With `auto_pause_when_alone = True` the bot pauses when the last other user leaves its channel and
resumes when someone is back (`auto_resume_on_join`). It only undoes its own pause: if you paused
with `!pause`, someone joining does not resume it. It stays silent when nothing is queued.

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

### Song announcements

On a radio stream the bot announces each new song as the station switches to it:

```
Bot:  Now playing: [Radio] SomaFM: Groove Salad
Bot:  Now playing: Boards of Canada - Dayvan Cowboy
      ... a few minutes later ...
Bot:  Now playing: Tycho - Awake
```

- The song comes from the stream's ICY metadata. The bot opens a second, light connection to the
  station to read it, so the audio is not affected.
- A station that sends no metadata stays silent after the first message. HLS (`.m3u8`) streams are
  not covered.
- The same title is never announced twice in a row, and the announcements stop when the radio
  stops, is skipped or paused.
- Turn it off with `announce_radio_tracks = False` (see [Configuration](../configuration/index.md)).
  It is separate from `announce_current_music`, which controls the message when a track starts.

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
