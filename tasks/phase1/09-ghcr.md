# 1-09 — GHCR Publish

**Status:** todo  
**Depends on:** 1-08  
**Unlocks:** one-line `docker pull` deployment for end users

## Objective

Publish the Docker image to GitHub Container Registry (GHCR) automatically on every `v*` release tag, so users can deploy without building from source. Added as a job to the existing `release.yml` — same trigger, no duplicate test runs.

## Changes to `.github/workflows/release.yml`

Replace the single `release` job with three jobs so the GitHub Release is only published after both the binary build and image push succeed:

```
build → docker → release
          ↗
build ───
```

- `build`: test + compile, uploads binary as a workflow artifact
- `docker`: `needs: build` — pushes image to GHCR
- `release`: `needs: [build, docker]` — downloads artifact, publishes GitHub Release

This prevents a versioned GitHub Release from existing without a corresponding container image.

## Tagging strategy

| Tag push | Images produced |
|---|---|
| `v1.2.3` | `ghcr.io/konradk/gotamusique:1.2.3`, `:1.2`, `:latest` |
| `v1.2.3-rc.1` | `ghcr.io/konradk/gotamusique:1.2.3-rc.1` only — no `:latest` |

`latest=auto` in `docker/metadata-action` handles this automatically.

## Platform

`linux/amd64` only — matches the existing binary release. ARM64 deferred.

## Post-deploy manual step (one-time)

After the first successful push, make the package public:

1. Go to `https://github.com/konradk/gotamusique/pkgs/container/gotamusique`
2. Package Settings → Change visibility → Public

Until this is done, `docker pull ghcr.io/konradk/gotamusique:latest` will return 403 for unauthenticated users.

## docker-compose snippet for end users

```yaml
services:
  gotamusique:
    image: ghcr.io/konradk/gotamusique:latest
    volumes:
      - ./configuration.ini:/app/configuration.ini:ro
    restart: unless-stopped
```

## Acceptance criteria

- Pushing a `v*` tag triggers the workflow and produces a public image on GHCR
- `docker pull ghcr.io/konradk/gotamusique:latest` works without authentication (after visibility is set to public)
- Image version matches the git tag
- Pre-release tags do not overwrite `:latest`
- Subsequent tag builds are faster due to GHA layer cache
