# Telegram Bot Daemon Design

**Goal:** Run telegram-bot as a proper Entware daemon with monit auto-restart.

**Problem:** Bot currently starts via simple `&` backgrounding in S99vpn-director, without proper init integration, status checking, or auto-restart on crash.

**Solution:** Separate S98telegram-bot init script with rc.func + monit monitoring.

---

## Architecture

### Init Script: S98telegram-bot

Standard Entware rc.func integration:

```sh
#!/bin/sh

ENABLED=yes
PROCS=telegram-bot
ARGS=""
PREARGS=""
DESC="Telegram Bot for VPN Director"
PATH=/opt/sbin:/opt/bin:/usr/sbin:/usr/bin:/sbin:/bin

. /opt/etc/init.d/rc.func
```

**Key points:**
- `PROCS=telegram-bot` — binary name for `pidof`
- `ENABLED=yes` always — bot self-exits gracefully if no config/token
- Standard commands: `start`, `stop`, `restart`, `check`

### Monit Config

`/opt/etc/monit.d/telegram-bot`:

```
check process telegram-bot matching "telegram-bot"
    start program = "/opt/etc/init.d/S98telegram-bot start"
    stop program = "/opt/etc/init.d/S98telegram-bot stop"
    if does not exist then restart
```

### Changes to Existing Files

**S99vpn-director:**
- Remove telegram-bot start in `start()`
- Remove telegram-bot killall in `stop()`

**setup_telegram_bot.sh:**
- Replace manual restart with `/opt/etc/init.d/S98telegram-bot restart`

**install.sh:**
- Add S98telegram-bot creation (heredoc, same pattern as S99vpn-director)

**README.md:**
- Add telegram-bot to Process Monitoring section

---

## File Summary

| File | Action |
|------|--------|
| `opt/etc/init.d/S98telegram-bot` | Create |
| `opt/etc/init.d/S99vpn-director` | Modify |
| `jffs/scripts/vpn-director/setup_telegram_bot.sh` | Modify |
| `install.sh` | Modify |
| `README.md` | Modify |

## Result

- `rc.unslung start/stop` manages bot correctly
- `S98telegram-bot check` shows alive/dead status
- monit auto-restarts on crash
- Single control point via init script
