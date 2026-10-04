.PHONY: build test clean run deps lint install bench bench-full bench-locomo bench-public bench-public-validate bench-release

BINARY=mira
GO=go
GOFLAGS=-ldflags="-s -w"
GOTAGS=fts5
BENCH_CONFIG?=benchmarks/config.example.json
MIRA_BENCH_REPORT?=benchmarks/results/local.json
BENCH_WARMUPS?=2
BENCH_REPETITIONS?=5
MIRA_SITE_DIR?=../mira-landing

build:
	mkdir -p bin
	$(GO) build -tags $(GOTAGS) $(GOFLAGS) -o bin/$(BINARY) ./cmd/mira

test:
	$(GO) test -tags $(GOTAGS) -v -race ./...

test-short:
	$(GO) test -tags $(GOTAGS) -v ./... -short

bench:
	$(GO) test -tags $(GOTAGS) -bench=. -benchmem -benchtime=100ms -count=1 ./...

bench-full:
	$(GO) test -tags $(GOTAGS) -bench=. -benchmem ./...

bench-locomo:
	./scripts/locomo_benchmark.sh

bench-public:
	$(GO) run -tags $(GOTAGS) ./cmd/mira-benchmark run --track all --config "$(BENCH_CONFIG)" --output "$(MIRA_BENCH_REPORT)" --warmups "$(BENCH_WARMUPS)" --repetitions "$(BENCH_REPETITIONS)"

bench-public-validate:
	$(GO) run -tags $(GOTAGS) ./cmd/mira-benchmark validate --report "$(MIRA_BENCH_REPORT)"

bench-release:
	./scripts/release-benchmark.sh "$(VERSION)" "$(MIRA_SITE_DIR)"

clean:
	rm -rf bin/ ./.mira/

run: build
	./bin/$(BINARY) -config config.yaml

deps:
	$(GO) mod download
	$(GO) mod tidy

lint:
	golangci-lint run --build-tags $(GOTAGS) ./...
	$(GO) vet -tags $(GOTAGS) ./...

fmt:
	$(GO) fmt ./...

install: build
	cp bin/$(BINARY) $(GOPATH)/bin/$(BINARY) 2>/dev/null || cp bin/$(BINARY) ~/go/bin/$(BINARY) 2>/dev/null || echo "Please add bin/ to your PATH"

prepublish:
	@./scripts/prepublish.sh $(VERSION)

.DEFAULT_GOAL := build
