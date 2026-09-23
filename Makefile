VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
MODULE  := github.com/osint-builders/geoimg

# Pinned linters, run via `go run` so nothing needs installing and every
# machine (and CI) uses the same versions.
GOIMPORTS := go run golang.org/x/tools/cmd/goimports@v0.50.0
GOLINES   := go run github.com/segmentio/golines@v0.13.0
GOCRITIC  := go run github.com/go-critic/go-critic/cmd/gocritic@v0.15.0
MAXLEN    := 120

.PHONY: build test fmt lint dist clean

build: ## Build the single static ./geoimg binary for this machine
	CGO_ENABLED=0 go build -trimpath -tags nodynamic -ldflags "-s -w -X main.version=$(VERSION)" -o geoimg ./cmd/geoimg

test: ## Run all tests with the race detector
	go test -race ./...

fmt: ## Rewrite code: goimports (grouped local imports) + golines (wrap at $(MAXLEN))
	$(GOLINES) -m $(MAXLEN) --base-formatter="$(GOIMPORTS) -local $(MODULE)" -w .

lint: ## Fail on any unformatted file, vet finding or go-critic finding
	@out="$$($(GOIMPORTS) -local $(MODULE) -l .)"; test -z "$$out" || { echo "goimports needed:\n$$out"; exit 1; }
	@out="$$($(GOLINES) -m $(MAXLEN) -l .)"; test -z "$$out" || { echo "golines needed:\n$$out"; exit 1; }
	go vet ./...
	$(GOCRITIC) check -enableAll ./...

dist: ## Cross-compile release archives into dist/
	scripts/dist.sh $(VERSION)

clean:
	rm -rf geoimg geoimg.exe dist
