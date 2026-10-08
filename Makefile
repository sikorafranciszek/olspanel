VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  = -s -w -X main.Version=$(VERSION)
BIN      = bin

.PHONY: all web go build linux test lint clean checksums dev

all: build

web:
	cd web && npm ci && npm run build
	rm -rf internal/web/dist && mkdir -p internal/web/dist && cp -r web/dist/. internal/web/dist/

go:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN)/olspanel ./cmd/olspanel

build: web go

linux: web
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN)/olspanel-linux-amd64 ./cmd/olspanel
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN)/olspanel-linux-arm64 ./cmd/olspanel

checksums:
	cd $(BIN) && sha256sum olspanel-linux-* > SHA256SUMS

test:
	go test ./...
	cd web && npm run typecheck

lint:
	go vet ./...

dev:
	OLSPANEL_DEV_SPA=http://localhost:5173 OLSPANEL_DATA_DIR=./.dev OLSPANEL_DEBUG=1 go run ./cmd/olspanel serve --no-system

clean:
	rm -rf $(BIN) web/dist internal/web/dist/*
