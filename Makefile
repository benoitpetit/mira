.PHONY: build test clean run deps lint install bench bench-full

BINARY=mira
GO=go
GOFLAGS=-ldflags="-s -w"

build:
	mkdir -p bin
	$(GO) build $(GOFLAGS) -o bin/$(BINARY) ./cmd/mira

test:
	$(GO) test -v -race ./...

test-short:
	$(GO) test -v ./... -short

bench:
	$(GO) test -bench=. -benchmem -benchtime=100ms -count=1 ./...

bench-full:
	$(GO) test -bench=. -benchmem ./...

clean:
	rm -rf bin/ ./.mira/

run: build
	./bin/$(BINARY) -config config.yaml

deps:
	$(GO) mod download
	$(GO) mod tidy

lint:
	golangci-lint run ./...
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

install: build
	cp bin/$(BINARY) $(GOPATH)/bin/$(BINARY) 2>/dev/null || cp bin/$(BINARY) ~/go/bin/$(BINARY) 2>/dev/null || echo "Please add bin/ to your PATH"

prepublish:
	@./scripts/prepublish.sh $(VERSION)

.DEFAULT_GOAL := build
