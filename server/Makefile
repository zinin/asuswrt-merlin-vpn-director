.PHONY: build build-bot build-arm64 build-arm build-webui build-webui-arm64 build-webui-arm clean test

VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
VERSION_FULL ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +%Y-%m-%d)

LDFLAGS = -ldflags "-s -w -X main.Version=$(VERSION) -X main.VersionFull=$(VERSION_FULL) -X main.Commit=$(COMMIT) -X main.BuildDate=$(BUILD_DATE)"

build: build-bot build-webui

build-bot:
	go build -trimpath $(LDFLAGS) -o bin/telegram-bot ./cmd/bot

build-arm64:
	GOOS=linux GOARCH=arm64 go build -trimpath $(LDFLAGS) -o bin/telegram-bot-arm64 ./cmd/bot

build-arm:
	GOOS=linux GOARCH=arm GOARM=7 go build -trimpath $(LDFLAGS) -o bin/telegram-bot-arm ./cmd/bot

build-webui:
	CGO_ENABLED=0 go build -trimpath $(LDFLAGS) -o bin/webui ./cmd/webui

build-webui-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath $(LDFLAGS) -o bin/webui-arm64 ./cmd/webui

build-webui-arm:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath $(LDFLAGS) -o bin/webui-arm ./cmd/webui

clean:
	rm -rf bin/

test:
	go test -v ./...
