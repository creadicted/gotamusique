BIN := bin/gotamusique

.PHONY: build test run dev clean fmt lint docs

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

docker-build:
	docker build -t gotamusique .

docker-run:
	docker compose up
