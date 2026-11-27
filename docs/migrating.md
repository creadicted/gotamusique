# Migrating from botamusique

gotamusique is a clean rewrite, not a drop-in replacement.

## Changed command keys

| botamusique key  | gotamusique key   | Reason |
|------------------|-------------------|--------|
| `pause = pause`  | `mute = mute`     | Radio streams are live and cannot be seeked. `!mute` sets volume to 0 without killing ffmpeg. |
| `play = p, play` | `unmute = unmute` | Counterpart to `!mute`. `!play` no longer exists; use `!np`. |

## Unsupported config keys

!!! warning "Remove these keys before starting"
    gotamusique rejects unknown keys in the sections it knows (`[server]`, `[bot]`, `[youtube_dl]`,
    `[files]`, `[debug]`, `[webinterface]`) and exits at startup with `unknown key "..." in section [...]`.
    Delete every key below from your `configuration.ini` before migrating. Unknown *sections* are
    ignored.

| Key | Status | Notes |
|-----|--------|-------|
| `allow_private_message` | deferred | Private messages are always ignored |
| `allow_other_channel_message` | deferred | |
| `auto_check_update` | removed | Use the GitHub release workflow |
| `clear_when_stop_in_oneshot` | deferred | |
| `database_path` | deferred | `music_database_path` is implemented |
| `delete_allowed` | deferred | |
| `download_attempts` | deferred | |
| `ducking` / `ducking_threshold` / `ducking_volume` | deferred | |
| `language` | deferred | English only |
| `pip3_path` / `target_version` | removed | Not applicable to a Go binary |
| `save_playlist` / `save_music_library` | deferred | |
| `tmp_folder` / `tmp_folder_max_size` | deferred | |
| `when_nobody_in_channel` | deferred | |

The `[webinterface]` section is read, but off by default and only partly implemented (see
[Web interface](configuration/web.md)). All its original keys are accepted; `user`, `password`,
`max_attempts`, `flask_secret`, `is_web_proxified`, `max_upload_file_size`, `upload_enabled` and
`web_logfile` are ignored. `auth_method = password` is not supported and stops the bot at startup;
use `token` (the default here) or `none`. Note the default differs from botamusique, which used
`none`. The `!web` command is new.

## New keys

| Key | Section | Description |
|-----|---------|-------------|
| `tls_skip_verify` | `[server]` | Skip TLS verification (default `True`); many Murmur servers use self-signed certs. |
