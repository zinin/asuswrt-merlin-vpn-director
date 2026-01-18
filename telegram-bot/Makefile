.PHONY: build build-arm64 build-arm clean test

build:
	go build -o bin/telegram-bot ./cmd/bot

build-arm64:
	GOOS=linux GOARCH=arm64 go build -o bin/telegram-bot-arm64 ./cmd/bot

build-arm:
	GOOS=linux GOARCH=arm GOARM=7 go build -o bin/telegram-bot-arm ./cmd/bot

clean:
	rm -rf bin/

test:
	go test -v ./...
