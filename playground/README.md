# Playground

A complete local gotamusique to try things out: a **Mumble server**, the **bot**, the **web
interface and API**, and a **Mumble client**, started with one command.

    make playground

When it is up it prints the addresses and a ready login link. Open the web interface, queue some
test music, and hear it in the channel. Ctrl+C stops everything.

Which Mumble client? You have three choices:

- **A browser tab: `make playground-kallisti`** (same as `SERVER=kallisti make playground`). Runs [Kallisti](https://kallisti.chat) as the
  server, a Mumble server with a web client. Open http://localhost:8090, join as a guest, click the
  voice channel **General** (the bot is there) and you hear the music in the tab. Type `!web` in that
  channel's chat for a login link. Nothing to install. (Kallisti shows chat as text, so the link is
  printed in full rather than as a clickable word.)
- **Any desktop Mumble client**, with the default server: connect to `localhost:64738`.
- **The one in this terminal**, no matter which server: `make playground-chat` (type `!help`, `!web`),
  `make playground-login` (prints a fresh login link) and `make playground-listen` (listens for a few
  seconds and reports the audio level, which proves the music arrives; it cannot play sound). It is
  `cmd/mumbleclient`, built on the spot, so it needs Go.

## What you get

| | |
|---|---|
| Web interface | http://127.0.0.1:8181/ |
| API documentation | http://127.0.0.1:8181/api/docs |
| Mumble server | `127.0.0.1:64738`, empty and without a password. Join the root channel, where the bot is (with `SERVER=kallisti`: the voice channel *General*) |
| Kallisti | http://localhost:8090, only with `SERVER=kallisti` |
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
    make playground-kallisti   the same with Kallisti, a browser client, as the server
    make playground-down       stop everything (keeps playground/data)
    make playground-reset      stop and delete playground/data (a clean start)
    make playground-logs       follow the bot's log
    make playground-login      print a web login link, asked for the way a Mumble user would
    make playground-chat       a Mumble client in the terminal: type to talk to the bot
    make playground-listen     listen for the bot's audio and report its level
    make playground-smoke      start, check that everything works, stop; exit 1 if not

The script is `playground/playground.sh`; `./playground/playground.sh help` lists everything, including
`up -d` to leave it running in the background. Settings are environment variables:

    SERVER=mumble|kallisti   MODE=auto|host|docker   AUTH=token|none
    WEB_PORT=8181   MUMBLE_PORT=64738   KALLISTI_PORT=8090

Rebuilding after a change to the bot: stop it and run `make playground` again. In host mode that is
a `go build` plus a restart, a few seconds.

## What the smoke test checks

`make playground-smoke` is a real end-to-end run, the way a person would use it: a Mumble client
asks the bot for a login link with `!web`, the link logs in once (and only once), the API refuses
callers who are not logged in, the page, API docs and OpenAPI document are served, the library was
scanned, adding a track with "play now" starts it, the position **advances** (so ffmpeg is feeding the
audio pipeline), a **second Mumble client hears the audio and it is not silence**, pause holds the
position, resume, volume, seek, an upload lands in the music folder, and stop ends playback. It needs
Go, curl and python3. The workflow `.github/workflows/playground.yml` runs it with the bot in a
container and on the host, against both servers on the host, when the playground, the Dockerfile or
the bot's entry points change.

## Troubleshooting

**Port already in use.** Something else uses 8181 or 64738 (8090 with Kallisti), or the playground is
already running (`make playground-down`). Pick other ports:
`WEB_PORT=8282 MUMBLE_PORT=64739 KALLISTI_PORT=8091 make playground`.

**Kallisti's page does not load although the container is up.** On WSL, Windows can hold a port such as
8080 that Linux does not see, so the connection is refused. That is why the default is 8090; pick any
free `KALLISTI_PORT`.

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

**I hear nothing.** You need a Mumble client in the bot's channel (or Kallisti's tab). The web interface
alone cannot play sound. `make playground-listen` tells you whether audio arrives at all. Check the bot's volume in the interface and that you are not muted.
