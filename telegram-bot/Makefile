.PHONY: build build-arm64 build-arm clean test

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +%Y-%m-%d)

LDFLAGS = -ldflags "-s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildDate=$(BUILD_DATE)"

build:
	go build -trimpath $(LDFLAGS) -o bin/telegram-bot ./cmd/bot

build-arm64:
	GOOS=linux GOARCH=arm64 go build -trimpath $(LDFLAGS) -o bin/telegram-bot-arm64 ./cmd/bot

build-arm:
	GOOS=linux GOARCH=arm GOARM=7 go build -trimpath $(LDFLAGS) -o bin/telegram-bot-arm ./cmd/bot

clean:
	rm -rf bin/

test:
	go test -v ./...
