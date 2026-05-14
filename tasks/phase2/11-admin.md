# 2-11 — Admin Features

**Status:** todo  
**Depends on:** 2-01 (for DB-backed bans)  
**Unlocks:** nothing

## Objective

Full admin command set and the ban/whitelist system.

## Admin commands

| Command | Description |
|---|---|
| `!maxvolume <n>` | Set max allowed volume; persisted in settings DB |
| `!userban <user>` | Ban user from all bot commands |
| `!userunban <user>` | Unban user |
| `!urlban [url]` | Ban a URL (or current playing URL); remove from queue |
| `!urlunban <url>` | Remove URL ban |
| `!urlbanlist` | List banned URLs |
| `!urlwhitelist <url>` | Whitelist a URL (overrides ban) |
| `!urlunwhitelist <url>` | Remove from whitelist |
| `!urlwhitelistlist` | List whitelisted URLs |
| `!webuseradd <user>` | Grant web access (password auth mode) |
| `!webuserdel <user>` | Revoke web access |
| `!webuserlist` | List web users |
| `!dropdatabase` | Drop and recreate both DBs |

## Ban system

All bans stored in `SettingsDB`:
- `user_ban` section: `option = username.lower()`
- `url_ban` section: `option = url`
- `url_whitelist` section: `option = url`

Checked in the command dispatcher before every dispatch:
- Sender in `user_ban` → reject
- Command argument is a URL in `url_ban` and not in `url_whitelist` → reject

## Deliverables

- `internal/command/handlers/admin.go`
- `internal/bot/admin.go` — `IsAdmin`, `IsURLBanned`, `IsURLWhitelisted`
- Update dispatcher to call ban checks
- Tests: ban/unban round-trip, admin guard rejection

## Acceptance criteria

- Non-admin `!kill` gets "not admin" response
- `!userban alice` blocks alice's commands
- `!urlban` while playing a YouTube URL removes it from queue and bans it
- Banned URL rejected on `!url` command
