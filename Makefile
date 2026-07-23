BIN := bin/gotamusique

.PHONY: build test run dev clean fmt lint docs tag

build:
	CGO_CFLAGS="-w -O2" go build -o $(BIN) ./cmd/gotamusique

test:
	go test ./...

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

docs:
	go doc ./...

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
