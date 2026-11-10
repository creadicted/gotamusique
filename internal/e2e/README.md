# End-to-end tests against a real Mumble server

These tests start the official `mumblevoip/mumble-server` image with Docker, run the
real bot against it and drive it with a second Mumble client that plays the part
of a listener. They catch what the unit tests cannot: real join/leave events,
channel membership, reconnects and chat.

```sh
make e2e          # needs Docker; audio tests also need ffmpeg and skip without it
```

- Build tag `e2e`: plain `go test ./...` never starts Docker.
- The server container binds a random localhost port and is removed afterwards.
- Tests skip (not fail) when Docker is unavailable.
- To add a scenario: `startMumble(t)`, `startBot(t, addr, "key = value")`,
  `joinHuman(t, addr, "alice")`, then `eventually` / `never` for assertions.
  `silenceServer(t)` gives a stream that plays until the bot stops it.

Not covered: audio actually reaching a listener (the harness client does not
decode Opus yet) and the web UI.
