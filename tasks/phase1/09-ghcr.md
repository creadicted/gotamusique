# 1-09 — GHCR Publish

**Status:** todo  
**Depends on:** 1-08  
**Unlocks:** one-line `docker pull` deployment for end users

## Objective

Publish the Docker image to GitHub Container Registry (GHCR) automatically on every release tag, so users can deploy without building from source.

## GitHub Actions workflow

`.github/workflows/release.yml`:

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  docker:
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
          tags: |
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=raw,value=latest

      - name: Build and push
        uses: docker/build-push-action@v5
        with:
          context: .
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
```

## Tagging strategy

| Push | Tags produced |
|---|---|
| `git tag v1.2.3` | `ghcr.io/konradk/gotamusique:1.2.3`, `:1.2`, `:latest` |

No `latest` is pushed for pre-release tags (`v1.0.0-rc.1`).

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
- `docker pull ghcr.io/konradk/gotamusique:latest` works without authentication
- Image version matches the git tag
- Pre-release tags (e.g. `v0.1.0-rc.1`) do not overwrite `:latest`
