# gotamusique

A Go rewrite of [botamusique](https://github.com/azlux/botamusique), a Mumble music bot.
One self-contained binary, no Python runtime required. The Docker image is fully static.

## Features

- Radio presets, any direct stream URL, HLS streams and [radio-browser.info](https://www.radio-browser.info) search
- Local music library with scan, search, tags, shortlists and bulk queueing (`!file` folders, `!filematch`)
- `!url` / `!playlist` via yt-dlp
- Playback modes: one-shot, repeat, random, autoplay
- Configuration compatible with botamusique's INI schema

## Where to go next

- [Installation](installation.md): binary, Docker, building from source
- [Configuration](configuration/index.md): every INI section and key
- [Commands](commands/index.md): chat command reference
- [Migrating from botamusique](migrating.md)
