BIN := bin/gotamusique

.PHONY: build test run clean

build:
	go build -o $(BIN) ./cmd/gotamusique

test:
	go test ./...

run: build
	./$(BIN)

clean:
	rm -rf bin/
