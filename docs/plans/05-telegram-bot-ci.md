# Telegram Bot: CI/CD

## Контекст проекта

VPN Director — система управления VPN на роутерах Asus. Telegram бот компилируется на Go и доставляется через GitHub Releases.

## Цель этого модуля

Настроить GitHub Actions для автоматической сборки бинарников при создании релиза.

## Что нужно реализовать

### GitHub Actions workflow

Файл: `.github/workflows/telegram-bot.yml`

```yaml
name: Build Telegram Bot

on:
  push:
    tags:
      - 'v*'

jobs:
  build:
    runs-on: ubuntu-latest

    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Build arm64
        working-directory: telegram-bot
        run: |
          GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o ../bin/telegram-bot-arm64 ./cmd/bot

      - name: Build arm (armv7)
        working-directory: telegram-bot
        run: |
          GOOS=linux GOARCH=arm GOARM=7 go build -ldflags="-s -w" -o ../bin/telegram-bot-arm ./cmd/bot

      - name: Upload to Release
        uses: softprops/action-gh-release@v1
        with:
          files: |
            bin/telegram-bot-arm64
            bin/telegram-bot-arm
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### Флаги сборки

`-ldflags="-s -w"` — уменьшает размер бинарника:
- `-s` — убирает symbol table
- `-w` — убирает DWARF debug info

### Триггер

Workflow запускается при push тега `v*`:
```bash
git tag v1.0.0
git push origin v1.0.0
```

### Артефакты релиза

После выполнения workflow в релизе появятся:
- `telegram-bot-arm64` — для aarch64 роутеров
- `telegram-bot-arm` — для armv7 роутеров

### URL для скачивания

```
https://github.com/zinin/asuswrt-merlin-vpn-director/releases/latest/download/telegram-bot-arm64
https://github.com/zinin/asuswrt-merlin-vpn-director/releases/latest/download/telegram-bot-arm
```

### Локальная сборка (Makefile)

Файл: `telegram-bot/Makefile`

```makefile
.PHONY: build build-arm64 build-arm clean

build: build-arm64 build-arm

build-arm64:
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/telegram-bot-arm64 ./cmd/bot

build-arm:
	GOOS=linux GOARCH=arm GOARM=7 go build -ldflags="-s -w" -o bin/telegram-bot-arm ./cmd/bot

clean:
	rm -rf bin/

# Для локальной разработки (текущая платформа)
dev:
	go build -o bin/telegram-bot ./cmd/bot

test:
	go test ./...
```

### Версионирование

Рекомендуемая схема: semver (v1.0.0, v1.1.0, v2.0.0)

Можно встроить версию в бинарник:

```yaml
- name: Build arm64
  run: |
    VERSION=${GITHUB_REF#refs/tags/}
    GOOS=linux GOARCH=arm64 go build \
      -ldflags="-s -w -X main.Version=$VERSION" \
      -o ../bin/telegram-bot-arm64 ./cmd/bot
```

```go
// main.go
var Version = "dev"

func main() {
    if len(os.Args) > 1 && os.Args[1] == "--version" {
        fmt.Println(Version)
        os.Exit(0)
    }
    // ...
}
```

## Выходные артефакты

- `.github/workflows/telegram-bot.yml`
- `telegram-bot/Makefile`

## Зависимости от других модулей

- **01-telegram-bot-core** — должен существовать компилируемый Go код

## Порядок выполнения

Этот модуль можно делать параллельно с разработкой, но первый релиз возможен только после завершения хотя бы 01-telegram-bot-core.
