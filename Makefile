VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: build test vet check docker-build

build:
	go build -trimpath -ldflags "$(LDFLAGS)" ./cmd/mihomo-exporter

test:
	go test ./...

vet:
	go vet ./...

check: test vet build

docker-build:
	docker build -t mihomo-exporter:local .
