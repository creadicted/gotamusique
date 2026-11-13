BIN := bin/gotamusique

.PHONY: build test run dev ui-demo playground playground-down playground-reset playground-logs playground-login playground-chat playground-listen playground-smoke clean fmt lint docs docs-build tag

# VERSION comes from the nearest git tag (v0.4.0 -> 0.4.0, or 0.4.0-3-gabc123 after it);
# empty outside a git checkout, in which case the binary reports "dev (<revision>)".
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')

build:
	CGO_CFLAGS="-w -O2" go build -ldflags "-X github.com/konradk/gotamusique/internal/version.Version=$(VERSION)" -o $(BIN) ./cmd/gotamusique

test:
	go test ./...

run: build
	./$(BIN)

dev: build
	-./$(BIN) --config bin/configuration.ini

# The web interface with a fake bot (made-up music, nothing played), no Mumble server needed.
# Open http://127.0.0.1:8181. Add LOGIN=1 to try the !web login flow instead of skipping it.
ui-demo:
	go run ./cmd/uidemo $(if $(LOGIN),-login)

# A complete local setup in Docker: a Mumble server, the bot, the web interface and API.
# `make playground` starts it (Ctrl+C stops it); see playground/README.md.
playground:
	./playground/playground.sh up

playground-down:
	./playground/playground.sh down

playground-reset:
	./playground/playground.sh reset

playground-logs:
	./playground/playground.sh logs

# Starts the stack, checks that it works end to end, stops it. Exits 1 if anything fails.
playground-login:
	./playground/playground.sh login

playground-chat:
	./playground/playground.sh chat

playground-listen:
	./playground/playground.sh listen

playground-smoke:
	./playground/playground.sh smoke

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
