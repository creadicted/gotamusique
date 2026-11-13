# Playground

A complete local gotamusique to try things out: a **Mumble server**, the **bot**, and the **web
interface and API**, started with one command.

    make playground

When it is up it prints the addresses. Open the web interface, connect a Mumble client to the
server, and you have the whole thing: log in with `!web`, queue some test music, hear it in the
channel. Ctrl+C stops everything.

## What you get

| | |
|---|---|
| Web interface | http://127.0.0.1:8181/ |
| API documentation | http://127.0.0.1:8181/api/docs |
| Mumble server | `127.0.0.1:64738`, empty and without a password. Join the root channel, where the bot is |
| Music | `playground/data/music`: generated test tones in folders (jazz, rock, ambient, one with a non-ASCII name, a five minute track for seeking, a FLAC). Drop your own files in and press *Rescan* |

Everything listens on `127.0.0.1` only. The Mumble server is fresh on every start; the bot's
configuration, library and music live in `playground/data/` (ignored by git).

## Logging in

By default the web interface asks for a login, like a real deployment: connect with a Mumble client,
type `!web` in the channel, and open the private link the bot sends you.

To skip that (you then do not need a Mumble client for the interface, but you will not hear
anything either):

    AUTH=none make playground

Then anyone who can reach port 8181 on your machine controls the bot. The playground allows
uploads in that mode too.

## Two ways to run the bot

The Mumble server always runs in Docker. The bot runs in one of two ways, chosen by `MODE`:

| | `MODE=host` | `MODE=docker` |
|---|---|---|
| The bot | a process on your machine, built with `go build` | a container built from the project's `Dockerfile` |
| You need | Docker, **Go, ffmpeg and ffprobe** | Docker only |
| Good for | quick reruns after a change | a clean machine; the same image as production |
| Breaks when | a tool is missing (the message says which) | the build cannot download packages, as behind a TLS-inspecting proxy |

`MODE=auto` (the default) picks **host** when Go and ffmpeg are installed, and docker otherwise. The
first line of the output says which one it chose.

## Commands

    make playground            start; shows the bot's log; Ctrl+C stops everything
    make playground-down       stop everything (keeps playground/data)
    make playground-reset      stop and delete playground/data (a clean start)
    make playground-logs       follow the bot's log
    make playground-smoke      start, check that everything works, stop; exit 1 if not

The script is `playground/playground.sh`; `./playground/playground.sh help` lists everything, including
`up -d` to leave it running in the background. Settings are environment variables:

    MODE=auto|host|docker   AUTH=token|none   WEB_PORT=8181   MUMBLE_PORT=64738

Rebuilding after a change to the bot: stop it and run `make playground` again. In host mode that is
a `go build` plus a restart, a few seconds.

## What the smoke test checks

`make playground-smoke` is a real end-to-end run with no login: the bot joined the Mumble server, the
page, API docs and OpenAPI document are served, the library was scanned and shows the sample folders,
adding a track with "play now" starts it, the position **advances** (so ffmpeg is feeding the audio
pipeline), pause holds the position, resume, volume, seek, an upload lands in the music folder, and
stop ends playback. The workflow `.github/workflows/playground.yml` runs it in both modes when
the playground, the Dockerfile or the bot's entry point changes.

## Troubleshooting

**Port already in use.** Something else uses 8181 or 64738, or the playground is already running
(`make playground-down`). Pick other ports: `WEB_PORT=8282 MUMBLE_PORT=64739 make playground`.

**`x509: certificate signed by unknown authority`, or `certificate verify failed` during the docker
build.** A TLS-inspecting proxy (Zscaler and similar) re-signs the traffic, and the build container
has no copy of its CA. Use host mode (`MODE=host`), which builds with Go and needs no download. The
script also builds with Docker's `default` builder, not a custom one, because a custom
`docker-container` builder has its own trust store; set `BUILDX_BUILDER` to override.

**Host mode says a tool is missing.** Install Go, ffmpeg and ffprobe (`sudo apt install ffmpeg`), or use
`MODE=docker`.

**The Mumble client cannot connect.** Use `localhost` and port 64738, with certificate warnings accepted
(the server makes its own certificate). On WSL, connect from Windows to `localhost`.

**The bot does not answer `!web`.** Type it in the channel the bot is in (the root channel), not in a
private message, and check `make playground-logs`.

**I hear nothing.** You need a Mumble client in the bot's channel. The web interface alone cannot play
sound. Check the bot's volume in the interface and that you are not muted.
