BIN := bin/gotamusique

.PHONY: build test run dev clean fmt lint docs

build:
	go build -o $(BIN) ./cmd/gotamusique

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
