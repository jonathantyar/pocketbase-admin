BIN     := pocketbase-admin
CONFIG  ?= admin.json
GOFLAGS ?=

.PHONY: all build ui test vet fmt check run doctor clean help

all: check build

help:
	@echo "make build   - compile the Go binary (UI is embedded)"
	@echo "make ui      - rebuild the frontend into ui/dist"
	@echo "make test    - run Go tests with the race detector"
	@echo "make vet     - run go vet"
	@echo "make fmt     - format Go sources"
	@echo "make check   - fmt + vet + test"
	@echo "make run     - build and serve (CONFIG=admin.json)"
	@echo "make doctor  - check config against PocketBase (CONFIG=admin.json)"
	@echo "make clean   - remove the binary"

build:
	go build $(GOFLAGS) -o $(BIN) .

ui:
	cd ui && npm install && npm run build

test:
	go test $(GOFLAGS) -race -count=1 ./...

vet:
	go vet $(GOFLAGS) ./...

fmt:
	gofmt -w .

check: fmt vet test

run: build
	./$(BIN) serve --config $(CONFIG)

doctor: build
	./$(BIN) doctor --config $(CONFIG)

clean:
	rm -f $(BIN)
