# Music library

With `music_folder` set, `!rescan` scans it into a SQLite database (`music_database_path`).
The first scan runs `ffprobe` on every audio file and can take a while; later scans only probe new
or changed files and remove rows for files that are gone. Scanned extensions: `mp3, flac, ogg, oga,
opus, m4a, aac, wav, wma, ape, aiff`. Directory symlinks are not followed. A file that `ffprobe`
cannot read, or that has no audio stream (a video or an image with an audio extension), is skipped
and not listed; a track that is already in the library keeps its entry.

The scan never wipes the library because the folder is unavailable: if `music_folder` is not a
readable directory, or no audio files are found while the library has entries (e.g. an unmounted
share), nothing is removed. To empty the library on purpose (for example after deleting every
file), run `!rescan force`.

Titles and artists come from the file's tags; an untagged file is listed by its file name. Browse
and search the result with `!listfile`, `!search` and `!findtagged`; see [Commands](../commands/index.md#library).

!!! tip "Docker"
    Keep the database on a volume, or every container recreation loses it. The compose file mounts
    `./data`; set `music_database_path = data/music.db`.

## Autoplay

`!mode autoplay` plays random library tracks. It starts immediately on an idle bot and refills the
queue whenever it runs dry. If `autoplay_length` tracks in a row fail to play, autoplay stops and
says so; run `!mode autoplay` again to restart it.
