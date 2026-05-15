# 1-09 — GHCR Publish

**Status:** todo  
**Depends on:** 1-08  
**Unlocks:** one-line `docker pull` deployment for end users

## Objective

Publish the Docker image to GitHub Container Registry (GHCR) automatically on every `v*` release tag, so users can deploy without building from source. Added as a job to the existing `release.yml` — same trigger, no duplicate test runs.

## Changes to `.github/workflows/release.yml`

Add a `docker` job after the existing `release` job:

```yaml
  docker:
    needs: release
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write

    steps:
      - uses: actions/checkout@v4

      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Extract metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/${{ github.repository }}
          flavor: |
            latest=auto
          tags: |
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}

      - name: Build and push
        uses: docker/build-push-action@v5
        with:
          context: .
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

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
