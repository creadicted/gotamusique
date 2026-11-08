# 3-08 — Kallisti music-bot panel

**Status:** todo  
**Priority:** next (after 3-07)  
**Depends on:** 3-07  
**Unlocks:** nothing

## Objective

Replace the planned embedded botamusique frontend with a **panel inside Kallisti** that shows the bot's
state and lets users trigger its commands with buttons and forms, so nobody needs to learn the `!commands`.

**Decision:** gotamusique does not serve its own web UI. The old plan (embed the webpack `static/`
directory, serve language templates, `build-frontend` Makefile target) is dropped. No frontend
code or build step is added to this repo.

## Where the work happens

Most of this milestone is in the **Kallisti repo**, not here. This file is the cross-repo description so the
gotamusique side stays aligned.

### Kallisti server (Go)

- Config entry for the bot: base URL and API token (the `[api]` section from 3-07).
- Authenticated proxy routes, e.g. `/api/bot/*`, forwarding to the bot's `/api/v1/*`, including
  the SSE stream. Adds `Authorization: Bearer <token>` and the caller's username header; never exposes the token to the browser.
- Access control: who may open the panel (everyone connected, or only certain roles) is decided by Kallisti.
- No CSP change needed, because the browser only talks to Kallisti's own origin.

### Kallisti web (Svelte)

- A "Music bot" panel (sidebar or pane next to the chat).
- Now-playing header, play/stop/skip/mute, volume slider, mode selector.
- Queue list with the current track marked.
- Radio presets as one-click buttons.
- Library search box with paged results and "play" actions.
- A generic **command form** rendered from `GET /commands`, so commands added to the bot later appear without a Kallisti change.
- Live updates from `/events`; fall back to polling `/state` if SSE drops.

### gotamusique (this repo)

- Only what 3-07 delivers, plus a short "Using gotamusique from Kallisti" README section
  covering the `[api]` config and the proxy setup.

## Later (not in scope)

If a second bot or tool wants a panel in Kallisti, generalize this into a plugin system: a manifest
(name, icon, URL), a sandboxed iframe served same-origin, and a `postMessage` bridge. Design the panel so that move is possible
(keep it self-contained and talking only to the proxy), but do not build the generic system now.

## Acceptance criteria

- A Kallisti user with access opens the panel and sees what is playing, with the queue, volume and mode.
- Skip, volume, mode and play-radio-preset work from the panel with no chat typing, and the chat-visible effect matches the `!command`.
- A command added to the bot's registry shows up in the panel after a reload, with no Kallisti code change.
- Chat changes (someone types `!skip`) appear in the panel without a reload.
- The API token is never present in any browser-visible response or bundle.
- Users without panel access get 403 from the proxy.
