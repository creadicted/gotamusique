BIN := bin/gotamusique

.PHONY: e2e build test run dev clean fmt lint docs docs-build tag

# VERSION comes from the nearest git tag (v0.4.0 -> 0.4.0, or 0.4.0-3-gabc123 after it);
# empty outside a git checkout, in which case the binary reports "dev (<revision>)".
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')

build:
	CGO_CFLAGS="-w -O2" go build -ldflags "-X github.com/konradk/gotamusique/internal/version.Version=$(VERSION)" -o $(BIN) ./cmd/gotamusique

test:
	go test ./...

# Real Mumble server in Docker; see internal/e2e. Needs Docker (and ffmpeg for audio tests).
e2e:
	go test -tags e2e -count=1 -v ./internal/e2e/...

run: build
	./$(BIN)

dev: build
	-./$(BIN) --config bin/configuration.ini

clean:
	rm -rf bin/

fmt:
	gofmt -w .

lint:
	go vet ./...

# Live preview of the documentation site at http://127.0.0.1:8000 (needs uv)
docs:
	uvx --with-requirements docs/requirements.txt mkdocs serve

# Same strict build CI runs; output goes to site/
docs-build:
	uvx --with-requirements docs/requirements.txt mkdocs build --strict

# Usage: make tag VERSION=v0.2.0-beta.1   (pre-release)
#        make tag VERSION=v0.2.0           (full release)
tag:
	@test -n "$(VERSION)" || (echo "Usage: make tag VERSION=v<semver>"; exit 1)
	git tag -a $(VERSION) -m "Release $(VERSION)"
	git push origin $(VERSION)

docker-build:
	docker build -t gotamusique .

docker-run:
	docker compose up
