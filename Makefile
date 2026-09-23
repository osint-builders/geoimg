VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build test lint dist clean

build: ## Build ./geoimg for this machine
	CGO_ENABLED=0 go build -trimpath -tags nodynamic -ldflags "-s -w -X main.version=$(VERSION)" -o geoimg ./cmd/geoimg

test: ## Run all tests with the race detector
	go test -race ./...

lint: ## gofmt + go vet
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	go vet ./...

dist: ## Cross-compile release archives into dist/
	scripts/dist.sh $(VERSION)

clean:
	rm -rf geoimg geoimg.exe dist
