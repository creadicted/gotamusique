#!/usr/bin/env bash
# A complete local gotamusique to try things out: a voice server, the bot, and the web
# interface and API, plus a small Mumble client in your terminal.
#
#   playground.sh [up]       build, start, show logs; Ctrl+C stops everything (default)
#   playground.sh up -d      start and leave it running
#   playground.sh down       stop everything (your data/ stays)
#   playground.sh reset      stop, and delete data/ (config, sample music, library)
#   playground.sh logs       follow the bot's log
#   playground.sh login      ask the bot for a web login link, as a Mumble user would (prints it)
#   playground.sh chat       a Mumble client in this terminal: type to talk to the bot (!help, !web)
#   playground.sh listen [s] listen for the bot's audio for s seconds (default 5) and say how loud it is
#   playground.sh smoke      start, check that everything works, stop; exit 1 if not
#
# The voice server always runs in Docker. SERVER chooses which:
#   mumble   the standard Mumble server: any Mumble client can join (default)
#   kallisti Kallisti, a Mumble server with a web client: join, chat and HEAR the bot in a
#            browser tab, with nothing to install. https://kallisti.chat
#
# The bot runs in one of two ways (MODE):
#   host    as a process on this machine, built with `go build` and using your ffmpeg.
#           Quickest to rerun after a change. Needs Go, ffmpeg and ffprobe.
#   docker  in a container built from the project's Dockerfile. Needs only Docker, but
#           the build downloads packages, which fails behind a TLS-inspecting proxy.
#
# Settings (environment variables):
#   SERVER=mumble|kallisti MODE=auto|host|docker (auto: host when Go and ffmpeg are installed)
#   AUTH=token|none        token (default): log in with !web.  none: no login
#   WEB_PORT=8181          the bot's web interface and API, on 127.0.0.1
#   MUMBLE_PORT=64738      the voice server's Mumble port, on 127.0.0.1
#   KALLISTI_PORT=8090     Kallisti's web client, on 127.0.0.1 (SERVER=kallisti)
#
# login, chat and listen need Go (they build the small client in cmd/mumbleclient).
# See README.md in this folder.
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(dirname "$here")
data="$here/data"
export WEB_PORT="${WEB_PORT:-8181}"
export MUMBLE_PORT="${MUMBLE_PORT:-64738}"
export KALLISTI_PORT="${KALLISTI_PORT:-8090}"
export PLAYGROUND_UID="${PLAYGROUND_UID:-$(id -u)}"
export PLAYGROUND_GID="${PLAYGROUND_GID:-$(id -g)}"
# Build with Docker's default builder (it uses the daemon, and so its network and CA
# settings). A custom builder, such as a docker-container one, has its own trust store and
# fails behind a TLS-inspecting proxy with "x509: certificate signed by unknown authority".
# Nothing here needs more than a local build; set BUILDX_BUILDER to use another one.
export BUILDX_BUILDER="${BUILDX_BUILDER:-default}"
AUTH="${AUTH:-token}"
MODE="${MODE:-auto}"
SERVER="${SERVER:-mumble}"
BOT_NAME="playground-bot"
base="http://127.0.0.1:${WEB_PORT}"
pidfile="$data/bot.pid"
botlog="$data/bot.log"
statefile="$data/state.env"
client_bin="$data/mumbleclient"

if [ -t 1 ]; then bold=$'\e[1m'; dim=$'\e[2m'; red=$'\e[31m'; green=$'\e[32m'; reset=$'\e[0m'; else bold=; dim=; red=; green=; reset=; fi
say()  { printf '%s\n' "$*"; }
step() { printf '%s==>%s %s\n' "$bold" "$reset" "$*"; }
die()  { printf '%s%s%s\n' "$red" "$*" "$reset" >&2; exit 1; }

# --profile '*' so that down finds the services of every profile, whichever server ran.
dc() { docker compose -f "$here/compose.yml" --project-directory "$here" --profile '*' "$@"; }

# The Mumble channel the bot is in: the root channel, except on Kallisti, whose voice
# channel is "General" (the root is not offered as a place to talk).
channel_name() { [ "$SERVER" = kallisti ] && echo General || echo ""; }

# ---------------------------------------------------------------- checks

preflight() {
  command -v docker >/dev/null 2>&1 || die "Docker is not installed. The voice server runs in Docker (https://docs.docker.com/engine/install/)."
  docker info >/dev/null 2>&1 || die "Cannot reach the Docker daemon. Is Docker running, and may you use it (try: docker ps)?"
  docker compose version >/dev/null 2>&1 || die "Docker Compose v2 is missing (the 'docker compose' command)."
  case "$AUTH" in token|none) ;; *) die "AUTH must be 'token' or 'none', not '$AUTH'." ;; esac
  case "$MODE" in auto|host|docker) ;; *) die "MODE must be 'auto', 'host' or 'docker', not '$MODE'." ;; esac
  case "$SERVER" in mumble|kallisti) ;; *) die "SERVER must be 'mumble' or 'kallisti', not '$SERVER'." ;; esac
}

have_host_tools() { command -v go >/dev/null 2>&1 && command -v ffmpeg >/dev/null 2>&1 && command -v ffprobe >/dev/null 2>&1; }

resolve_mode() {
  if [ "$MODE" = auto ]; then
    if have_host_tools; then MODE=host; else MODE=docker; fi
  fi
  if [ "$MODE" = host ] && ! have_host_tools; then
    local missing="" t
    for t in go ffmpeg ffprobe; do command -v "$t" >/dev/null 2>&1 || missing="$missing $t"; done
    die "Host mode needs Go, ffmpeg and ffprobe on your PATH; missing:$missing. Install them, or use MODE=docker."
  fi
}

port_busy() { (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; }

check_ports() {
  local p ports=("$WEB_PORT" "$MUMBLE_PORT")
  [ "$SERVER" = kallisti ] && ports+=("$KALLISTI_PORT")
  for p in "${ports[@]}"; do
    if port_busy "$p"; then
      local who; who=$(ss -ltnp 2>/dev/null | grep -E ":$p\b" | head -1 || true)
      die "Port $p on 127.0.0.1 is in use by something else${who:+: $who}
Stop it, or pick other ports: WEB_PORT=8282 MUMBLE_PORT=64739 KALLISTI_PORT=8081 $0"
    fi
  done
}

# How the playground was started, remembered so that down, logs, login, chat and listen
# work later without repeating the settings. The ports still come from the environment.
save_state() { printf 'MODE=%s\nSERVER=%s\nAUTH=%s\n' "$MODE" "$SERVER" "$AUTH" >"$statefile"; }
load_state() {
  [ -f "$statefile" ] || return 0
  local k v
  while IFS='=' read -r k v; do
    case "$k" in MODE|SERVER|AUTH) printf -v "$k" '%s' "$v" ;; esac
  done <"$statefile"
}

# ---------------------------------------------------------------- data

write_config() {
  mkdir -p "$data"
  local uploads="False"
  # Without a login, uploads are refused unless explicitly allowed. This is a throwaway
  # local setup, so allow it there.
  [ "$AUTH" = none ] && uploads="True"
  # In a container the bot runs in /app and keeps its files under data/; on the host it
  # runs inside data/ itself. The paths below are relative to where the bot runs.
  local p="" mumble_host="127.0.0.1" mumble_port="$MUMBLE_PORT" listen="127.0.0.1" web_port="$WEB_PORT" ch
  if [ "$MODE" = docker ]; then p="data/"; mumble_host="$SERVER"; mumble_port=64738; listen="0.0.0.0"; web_port=8181; fi
  ch=$(channel_name)
  cat >"$data/configuration.ini" <<EOF
# Generated by playground.sh on every start: edit playground.sh, not this file.
[server]
host = ${mumble_host}
port = ${mumble_port}
channel = ${ch}

[bot]
username = ${BOT_NAME}
volume = 0.4
refresh_cache_on_startup = True
music_database_path = ${p}music.db
last_played_path = ${p}last_played.json

[files]
music_folder = ${p}music

[radio]
jazz = http://jazz-wr04.ice.infomaniak.ch/jazz-wr04-128.mp3 "Jazz Yeah !"
groove = https://ice1.somafm.com/groovesalad-128-mp3 "SomaFM Groove Salad"

[webinterface]
enabled = True
listening_addr = ${listen}
listening_port = ${web_port}
access_address = ${base}
auth_method = ${AUTH}
upload_enabled = True
allow_upload_without_auth = ${uploads}
EOF
}

# Test tones with names and folders. They are not music: the point is to have something
# to browse, search, queue and seek in. Made with ffmpeg (yours in host mode, the image's
# in docker mode), so nothing else has to be installed. Skipped when the folder exists.
make_music() {
  [ -d "$data/music" ] && return 0
  step "Generating sample music (test tones)"
  cat >"$data/make-music.sh" <<'EOF'
set -e
cd "$(dirname "$0")"
mkdir -p music/Jazz music/Rock/Heavy music/Ambient music/Mixed
tone() { # file title artist frequency seconds
  ffmpeg -loglevel error -y -f lavfi -i "sine=frequency=$4:duration=$5" -af volume=0.25 \
    -ac 1 -b:a 48k -metadata title="$2" -metadata artist="$3" "$1"
}
tone "music/Jazz/So What.mp3"                   "So What"            "Miles Davis"        440 90
tone "music/Jazz/Take Five.mp3"                 "Take Five"          "Dave Brubeck"       523 60
tone "music/Rock/Heavy/Thunderstruck.mp3"       "Thunderstruck"      "AC/DC"              330 45
tone "music/Rock/Seven Nation Army.mp3"         "Seven Nation Army"  "The White Stripes"  196 75
tone "music/Ambient/Weightless.mp3"             "Weightless"         "Marconi Union"      262 300
tone "music/Mixed/Ümläut – 音楽.mp3"            "Ümläut – 音楽"       "Die Ärzte"          392 30
tone "music/Loose track.mp3"                    "Loose track"        ""                   349 20
ffmpeg -loglevel error -y -f lavfi -i "sine=frequency=220:duration=120" -af volume=0.25 -ac 1 \
  -metadata title="Discreet Music" -metadata artist="Brian Eno" "music/Ambient/Discreet Music.flac"
EOF
  if [ "$MODE" = host ]; then
    sh "$data/make-music.sh" || die "Could not generate the sample music. Look at the output above."
  else
    dc run --rm --no-deps --entrypoint sh bot /app/data/make-music.sh >/dev/null \
      || die "Could not generate the sample music. Look at the output above."
  fi
  rm -f "$data/make-music.sh"
}

# ---------------------------------------------------------------- the bot

build_bot() {
  if [ "$MODE" = host ]; then
    step "Building the bot with Go"
    (cd "$root" && CGO_CFLAGS="-w -O2" go build -ldflags "-X github.com/konradk/gotamusique/internal/version.Version=playground" \
      -o "$data/gotamusique" ./cmd/gotamusique) || die "go build failed. The messages above say why."
    return 0
  fi
  step "Building the bot image (the first time takes a few minutes)"
  local out; out=$(mktemp)
  if ! dc build bot 2>&1 | tee "$out"; then
    if grep -q "x509\|certificate verify failed" "$out"; then
      say ""
      say "A TLS certificate was not trusted inside the build. This happens behind a TLS-inspecting"
      say "proxy: the build downloads packages and has no copy of the proxy's CA. If you have Go and"
      say "ffmpeg installed, use host mode instead, which builds with Go and needs no download:"
      say "    MODE=host $0"
    fi
    rm -f "$out"
    die "The build failed. The messages above say why."
  fi
  rm -f "$out"
}

bot_alive() { [ -f "$pidfile" ] && kill -0 "$(cat "$pidfile")" 2>/dev/null; }

start_host_bot() {
  rm -f "$pidfile"
  : >"$botlog"
  # The bot gives up when its first connection fails, and a freshly started server needs a
  # moment before it accepts logins, so start it again until it stays up.
  local attempt i
  for attempt in 1 2 3 4 5 6 7 8; do
    # exec: the subshell becomes the bot, so $! is the bot's own PID. (Without it, `cd && cmd &`
    # backgrounds the whole list in a helper, $! is the helper, and stopping it leaves the bot running.)
    (cd "$data" && exec nohup ./gotamusique --config configuration.ini) >>"$botlog" 2>&1 &
    echo $! >"$pidfile"
    for ((i = 0; i < 16; i++)); do
      sleep 0.5
      bot_alive || break
      bot_connected && return 0
    done
    bot_alive && return 0 # still starting; the wait afterwards decides
    sleep 2
  done
  return 1
}

# Our bots: the one in the pid file, and any other gotamusique process that runs inside our
# data folder (left behind by an interrupted run). Matched by name and working directory, so
# nothing unrelated is ever touched, even if a process id was reused.
playground_bot_pids() {
  local p
  if [ -f "$pidfile" ]; then
    p=$(cat "$pidfile")
    [ "$(cat "/proc/$p/comm" 2>/dev/null)" = gotamusique ] && echo "$p"
  fi
  for p in $(pgrep -x gotamusique 2>/dev/null || true); do
    [ "$(readlink "/proc/$p/cwd" 2>/dev/null)" = "$data" ] && echo "$p"
  done
  return 0 # "no bot" is not a failure (set -e and pipefail would end the script)
}

stop_host_bot() {
  local pids pid i
  pids=$(playground_bot_pids | sort -un)
  for pid in $pids; do kill "$pid" 2>/dev/null || true; done
  for pid in $pids; do
    for ((i = 0; i < 20; i++)); do kill -0 "$pid" 2>/dev/null || break; sleep 0.25; done
    kill -0 "$pid" 2>/dev/null && kill -9 "$pid" 2>/dev/null || true
  done
  rm -f "$pidfile"
}

# bot_logs LINES [follow]: the bot's log, from the file (host mode) or the container.
bot_logs() {
  local follow=()
  [ "${2:-}" = follow ] && follow=(-f)
  if [ "$MODE" = host ]; then
    tail -n "$1" "${follow[@]}" "$botlog"
  else
    dc logs --tail "$1" "${follow[@]}" bot
  fi
}

stop_all() {
  stop_host_bot
  dc down --remove-orphans >/dev/null 2>&1 || true
}

# ---------------------------------------------------------------- the Mumble client

# The small client in cmd/mumbleclient: login links, chat, and a listener that checks the
# bot's audio really arrives. It needs Go to build; everything else works without it.
build_client() {
  command -v go >/dev/null 2>&1 || die "This needs Go, to build the small Mumble client (cmd/mumbleclient). Use any Mumble client instead."
  (cd "$root" && CGO_CFLAGS="-w -O2" go build -o "$client_bin" ./cmd/mumbleclient) \
    || die "Could not build the Mumble client. The messages above say why."
}

client() { "$client_bin" -addr "127.0.0.1:$MUMBLE_PORT" -channel "$(channel_name)" "$@"; }

login_link() { client -name "playground-login-$$" -login -wait 15s; }

require_running() {
  preflight
  load_state
  mumble_up || die "The playground is not running. Start it with: make playground"
}

# ---------------------------------------------------------------- waiting

wait_for() { # description, timeout seconds, command...
  local timeout=$2 i; shift 2
  for ((i = 0; i < timeout * 2; i++)); do
    if "$@" >/dev/null 2>&1; then return 0; fi
    sleep 0.5
  done
  return 1
}

ports_free() { ! port_busy "$WEB_PORT" && ! port_busy "$MUMBLE_PORT"; }
web_up()      { curl -s -o /dev/null --max-time 2 "$base/api/v1/state"; }
mumble_up()   { port_busy "$MUMBLE_PORT"; }
kallisti_up() { [ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 2 "http://127.0.0.1:${KALLISTI_PORT}/")" = 200 ]; }
# The bot logs "connected to Mumble" when it is in; this works for either server.
bot_connected() {
  if [ "$MODE" = host ]; then grep -q "connected to Mumble" "$botlog" 2>/dev/null
  else dc logs bot 2>&1 | grep -q "connected to Mumble"; fi
}

# ---------------------------------------------------------------- commands

start_stack() {
  preflight
  resolve_mode
  step "Voice server: $SERVER, bot mode: $MODE"
  stop_all # a previous run of the playground may still be up; anything left on the ports is not ours
  check_ports
  write_config
  build_bot
  make_music
  save_state

  step "Starting $SERVER"
  dc up -d "$SERVER" || die "Could not start the $SERVER container."
  wait_for "the voice server" 60 mumble_up || { dc logs --tail 30 "$SERVER"; die "The $SERVER server did not start."; }
  if [ "$SERVER" = kallisti ]; then
    wait_for "Kallisti's web client" 60 kallisti_up || { dc logs --tail 30 kallisti; die "Kallisti's web client did not come up."; }
  fi

  step "Starting the bot"
  if [ "$MODE" = host ]; then
    start_host_bot || { bot_logs 30; die "The bot did not stay up. Its log is above."; }
  else
    dc up -d bot || die "Could not start the bot container."
  fi

  step "Waiting for everything to come up"
  wait_for "the web interface" 90 web_up || { bot_logs 30; die "The web interface did not come up. The bot's log is above."; }
  wait_for "the bot in Mumble" 90 bot_connected || { bot_logs 30; die "The bot did not join the voice server. Its log is above."; }
}

banner() {
  local login="" voice
  if [ "$AUTH" = token ]; then
    if command -v go >/dev/null 2>&1 && build_client 2>/dev/null; then
      local link; link=$(login_link 2>/dev/null || true)
      if [ -n "$link" ]; then
        login="Log in now (copy it into your browser; valid for 5 minutes, ${bold}make playground-login${reset} gives a new one):
    ${link}"
      fi
    fi
    [ -n "$login" ] || login="Log in: type ${bold}!web${reset} in the channel where the bot is; the bot sends you a private link."
  else
    login="No login (AUTH=none): the interface is open to anyone who can reach this port."
  fi
  if [ "$SERVER" = kallisti ]; then
    voice="${bold}Kallisti${reset}       http://localhost:${KALLISTI_PORT}   Join as a guest, click the voice channel ${bold}General${reset} (the bot is there), and you hear the music in the tab.
                  Type ${bold}!web${reset} in the channel's chat for a login link. Mumble clients can join at 127.0.0.1:${MUMBLE_PORT} too."
  else
    voice="${bold}Mumble server${reset}  127.0.0.1:${MUMBLE_PORT}   (any Mumble client; join the root channel, where the bot is)"
  fi
  cat <<EOF

${green}${bold}The playground is running.${reset}  (voice server: ${SERVER}, bot in ${MODE} mode)

  ${bold}Web interface${reset}  ${base}/
  ${bold}API docs${reset}       ${base}/api/docs
  ${voice}
  ${bold}Music${reset}          playground/data/music   (test tones; add your own files, then press Rescan)

  ${login}

  No Mumble client? ${bold}make playground-chat${reset} is one in this terminal, ${bold}make playground-listen${reset} checks that
  the bot's audio arrives (it shows a level, it cannot play sound).
  Logs: make playground-logs        Stop: Ctrl+C here, or make playground-down
EOF
  if grep -qi microsoft /proc/version 2>/dev/null; then
    say "  ${dim}WSL: use localhost from Windows (browser and Mumble client) as shown above.${reset}"
  fi
  say ""
}

cmd_up() {
  local detach=0
  if [ "${1:-}" = "-d" ] || [ "${1:-}" = "--detach" ]; then detach=1; fi
  start_stack
  banner
  if [ "$detach" = 1 ]; then
    say "Left running in the background."
    return 0
  fi
  trap 'echo; step "Stopping"; stop_all; exit 0' INT TERM
  step "The bot's log (Ctrl+C stops everything)"
  bot_logs 20 follow
}

# These do not know how the playground was started; they read what start_stack saved.
cmd_down() { preflight; load_state; stop_all; say "Stopped."; }
cmd_logs() { preflight; load_state; [ -f "$pidfile" ] && MODE=host; bot_logs 50 follow; }
cmd_reset() {
  preflight
  stop_all
  if [ -d "$data" ]; then
    # The files are yours (the containers run as your user), so this works.
    rm -rf -- "$data"
    say "Deleted $data"
  fi
}

cmd_login() {
  require_running
  build_client
  local link; link=$(login_link) || exit 1
  say "$link"
}
cmd_chat()   { require_running; build_client; exec "$client_bin" -addr "127.0.0.1:$MUMBLE_PORT" -channel "$(channel_name)" -name "playground-user"; }
cmd_listen() { require_running; build_client; client -name "playground-ear-$$" -listen "${1:-5}s"; }

# ---------------------------------------------------------------- smoke

pass=0; fail=0
jar="$data/smoke-cookies.txt"
check() { # description, command...
  local what=$1; shift
  if "$@" >/dev/null 2>&1; then pass=$((pass + 1)); printf '  %sPASS%s %s\n' "$green" "$reset" "$what"
  else fail=$((fail + 1)); printf '  %sFAIL%s %s\n' "$red" "$reset" "$what"; fi
}

# Once logged in, every call carries the session cookie.
api()  { if [ -f "$jar" ]; then curl -s --max-time 20 -b "$jar" "$@"; else curl -s --max-time 20 "$@"; fi; }
json() { python3 -c 'import sys,json
d=json.load(sys.stdin)
for k in sys.argv[1].split("."):
    d=d[int(k)] if isinstance(d,list) else d[k]
print(d if not isinstance(d,bool) else str(d).lower())' "$1"; }
state_field() { api "$base/api/v1/state" | json "$1"; }

s_refused()  { [ "$(curl -s -o /dev/null -w '%{http_code}' "$base/api/v1/state")" = 401 ]; }
s_link()     { client -name smoke-login -login -wait 20s >"$data/smoke-link" && grep -q '/login?token=' "$data/smoke-link"; }
s_login()    { [ "$(curl -s -c "$jar" -L -o /dev/null -w '%{http_code}' "$(cat "$data/smoke-link")")" = 200 ] && [ "$(state_field me.name)" = smoke-login ]; }
s_once()     { [ "$(curl -s -o /dev/null -w '%{http_code}' "$(cat "$data/smoke-link")")" = 401 ]; }
s_scanned()  { [ "$(state_field scan.available)" = true ] && [ -n "$(state_field scan.last_result)" ]; }
s_library()  { [ "$(api "$base/api/v1/library" | python3 -c 'import sys,json; print(len(json.load(sys.stdin)["dirs"]))')" -ge 4 ]; }
s_add_now()  { [ "$(api -o /dev/null -w '%{http_code}' -X POST "$base/api/v1/queue" -d '{"type":"library","path":"Jazz/So What.mp3","position":"now"}')" = 201 ]; }
s_playing()  { [ "$(state_field playing)" = true ] && [ -n "$(state_field queue.0.title)" ]; }
s_advances() {
  local a b
  a=$(state_field position_ms); sleep 2; b=$(state_field position_ms)
  [ "$b" -gt "$((a + 1000))" ]
}
# A second Mumble client in the channel receives the audio, and it is not silence.
s_heard()    { client -name smoke-ear -listen 4s; }
s_pause() {
  api -X POST "$base/api/v1/playback/pause" >/dev/null; sleep 1
  local a b; a=$(state_field position_ms); sleep 1.5; b=$(state_field position_ms)
  [ "$(state_field pause)" = manual ] && [ "$a" = "$b" ]
}
s_resume()   { api -X POST "$base/api/v1/playback/resume" >/dev/null; sleep 1; [ "$(state_field playing)" = true ]; }
s_volume()   { api -X PUT "$base/api/v1/volume" -d '{"volume":0.3}' >/dev/null; [ "$(state_field volume)" = 0.3 ]; }
s_seek() {
  api -X POST "$base/api/v1/playback/seek" -d '{"position_ms":60000}' >/dev/null; sleep 1
  [ "$(state_field position_ms)" -ge 60000 ]
}
s_upload() {
  cp "$data/music/Loose track.mp3" "$data/upload-probe.mp3"
  local code; code=$(api -o /dev/null -w '%{http_code}' -F "file=@$data/upload-probe.mp3" "$base/api/v1/library/upload")
  rm -f "$data/upload-probe.mp3"
  [ "$code" = 201 ] && [ -f "$data/music/uploads/smoke-login/upload-probe.mp3" ]
}
s_ui()   { curl -s "$base/" | grep -q '<title>gotamusique</title>'; }
s_docs() { [ "$(api -o /dev/null -w '%{http_code}' "$base/api/docs")" = 200 ] && [ "$(api -o /dev/null -w '%{http_code}' "$base/api/v1/openapi.yaml")" = 200 ]; }
s_stop() { api -X POST "$base/api/v1/playback/stop" >/dev/null; sleep 1; [ "$(state_field playing)" = false ]; }
s_kallisti_page() { curl -s "http://127.0.0.1:${KALLISTI_PORT}/" | grep -q '<title>gotamusique playground</title>'; }

# The whole pipeline, as a person would use it: a Mumble client asks the bot for a login link,
# the link logs in once, and the API then drives playback that a second client can hear.
cmd_smoke() {
  command -v curl >/dev/null 2>&1 || die "smoke needs curl."
  command -v python3 >/dev/null 2>&1 || die "smoke needs python3 (to read JSON)."
  command -v go >/dev/null 2>&1 || die "smoke needs Go (it builds the small Mumble client in cmd/mumbleclient)."
  [ "$AUTH" = token ] || die "smoke tests the login: leave AUTH at its default (token)."
  trap 'stop_all' EXIT
  rm -f "$jar"
  start_stack
  build_client
  step "Checking the stack"
  check "the web interface answers"                        web_up
  check "the bot joined the voice server"                  bot_connected
  [ "$SERVER" = kallisti ] && check "Kallisti's web client is served"     s_kallisti_page
  check "the page is served"                               s_ui
  check "the API refuses a caller who is not logged in"    s_refused
  check "!web: a Mumble client gets a login link"          s_link
  check "the link logs in (session cookie)"                s_login
  check "the link works only once"                         s_once
  check "the API docs and the OpenAPI document are served" s_docs
  check "the library was scanned"                          wait_for scan 60 s_scanned
  check "the sample music shows up in folders"             s_library
  check "adding a track with 'play now' works"             s_add_now
  check "the bot is playing it"                            wait_for playing 20 s_playing
  check "the position advances (ffmpeg feeds the pipeline)" s_advances
  check "a Mumble client hears the audio, and it is not silence" s_heard
  check "pause holds the position"                         s_pause
  check "resume plays again"                               s_resume
  check "the volume changes"                               s_volume
  check "seeking jumps ahead"                              s_seek
  check "an uploaded file reaches the music folder"        s_upload
  check "stop ends playback"                               s_stop
  stop_all
  check "stopping the playground frees the ports"          wait_for ports 10 ports_free
  say ""
  if [ "$fail" -eq 0 ]; then
    printf '%s%s%d checks passed.%s\n' "$green" "$bold" "$pass" "$reset"
  else
    printf '%s%s%d passed, %d failed.%s The bot'"'"'s log:\n' "$red" "$bold" "$pass" "$fail" "$reset"
    bot_logs 40 || true
    return 1
  fi
}

# ---------------------------------------------------------------- main

case "${1:-up}" in
  up)     shift || true; cmd_up "${1:-}" ;;
  down)   cmd_down ;;
  reset)  cmd_reset ;;
  logs)   cmd_logs ;;
  login)  cmd_login ;;
  chat)   cmd_chat ;;
  listen) shift || true; cmd_listen "${1:-}" ;;
  smoke)  cmd_smoke ;;
  -h|--help|help) sed -n '2,/^set -euo/p' "$0" | sed '$d; s/^# \{0,1\}//' ;;
  *)      die "Unknown command '$1'. Try: $0 help" ;;
esac
