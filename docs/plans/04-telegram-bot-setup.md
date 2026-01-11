# Telegram Bot: Setup скрипт и интеграция

## Контекст проекта

VPN Director — система управления VPN на роутерах Asus. Telegram бот — опциональный интерфейс.

Этот модуль создаёт скрипт настройки бота и интегрирует бота в существующий install.sh.

## Цель этого модуля

1. Создать `setup_telegram_bot.sh` — скрипт создания конфига бота
2. Обновить `install.sh` — скачивание бинарника и запуск

## Что нужно реализовать

### setup_telegram_bot.sh

**Сценарий работы:**

```bash
$ ./setup_telegram_bot.sh

VPN Director Telegram Bot Setup
================================

Для создания бота:
1. Откройте @BotFather в Telegram
2. Отправьте /newbot
3. Следуйте инструкциям
4. Скопируйте токен

Введите токен бота: 123456789:ABCdefGHI...

Введите Telegram username (без @): myusername
Добавить ещё пользователя? [y/N]: y
Введите username: anotherusername
Добавить ещё пользователя? [y/N]: n

✓ Конфиг создан: /jffs/scripts/vpn-director/telegram-bot.json
✓ Бот перезапущен

Готово! Напишите боту /start
```

**Логика:**

1. Проверить что VPN Director установлен (`/jffs/scripts/vpn-director/` существует)
2. Спросить bot_token
3. Спросить username(ы) в цикле
4. Создать `telegram-bot.json`
5. Перезапустить бота (kill + start)

**Формат telegram-bot.json:**

```json
{
  "bot_token": "123456789:ABCdefGHI...",
  "allowed_users": ["myusername", "anotherusername"]
}
```

### Изменения в install.sh

**Добавить скачивание бинарника:**

```bash
# Определить архитектуру
ARCH=$(uname -m)
case "$ARCH" in
    aarch64) BOT_BINARY="telegram-bot-arm64" ;;
    armv7l)  BOT_BINARY="telegram-bot-arm" ;;
    *)
        echo "WARN: Архитектура $ARCH не поддерживается для Telegram бота"
        BOT_BINARY=""
        ;;
esac

# Скачать бинарник если архитектура поддерживается
if [ -n "$BOT_BINARY" ]; then
    RELEASE_URL="https://github.com/zinin/asuswrt-merlin-vpn-director/releases/latest/download"
    echo "Загрузка Telegram бота..."
    curl -fsSL "$RELEASE_URL/$BOT_BINARY" -o "$JFFS_DIR/vpn-director/telegram-bot"
    chmod +x "$JFFS_DIR/vpn-director/telegram-bot"
fi
```

**Добавить в services-start:**

```bash
# Telegram бот (запустится только если есть конфиг)
/jffs/scripts/vpn-director/telegram-bot &
```

**Добавить скачивание setup скрипта:**

```bash
curl -fsSL "$REPO_URL/setup_telegram_bot.sh" -o "$JFFS_DIR/vpn-director/setup_telegram_bot.sh"
chmod +x "$JFFS_DIR/vpn-director/setup_telegram_bot.sh"
```

**Обновить финальное сообщение:**

```
Установка завершена!

Следующие шаги:
1. Импортируйте серверы:
   /jffs/scripts/vpn-director/import_server_list.sh

2. Настройте VPN Director:
   /jffs/scripts/vpn-director/configure.sh

3. (Опционально) Настройте Telegram бота:
   /jffs/scripts/vpn-director/setup_telegram_bot.sh
```

### Поведение бота при старте

Бот проверяет наличие конфига:

```go
configPath := "/jffs/scripts/vpn-director/telegram-bot.json"
config, err := LoadConfig(configPath)
if os.IsNotExist(err) {
    // Конфига нет — тихо выходим
    os.Exit(0)
}
if err != nil {
    log.Fatalf("Ошибка загрузки конфига: %v", err)
}
// Конфиг есть — работаем
```

Это позволяет:
- Всегда иметь строку запуска в services-start
- Боту "спать" если конфиг не создан
- Активировать бота просто создав конфиг

### Расположение файлов

```
/jffs/scripts/vpn-director/
├── telegram-bot              # Go бинарник (из install.sh)
├── telegram-bot.json         # Конфиг (из setup_telegram_bot.sh)
├── setup_telegram_bot.sh     # Скрипт настройки
├── xray_tproxy.sh
├── configure.sh
└── ...
```

### Shell conventions

Следовать существующим конвенциям проекта:
- Shebang: `#!/usr/bin/env bash`
- `set -euo pipefail`
- Использовать `[[ ]]` вместо `[ ]`

## Выходные артефакты

- `setup_telegram_bot.sh` — новый файл
- Обновлённый `install.sh`
- Обновлённый `jffs/scripts/services-start`

## Зависимости от других модулей

- **01-telegram-bot-core** — бинарник должен существовать и поддерживать проверку конфига
- **05-telegram-bot-ci** — бинарники должны быть в GitHub Releases
