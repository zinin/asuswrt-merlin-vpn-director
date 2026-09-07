---
paths: "**/*.sh, jffs/**/*"
---

# Shell Conventions

## Script Structure

- Shebang: `#!/usr/bin/env bash` with `set -euo pipefail`
- Debug mode: `DEBUG=1 ./script.sh` enables `set -x` with informative PS4
- shellcheck annotations for intentional expansions/externals (SC2086, SC2155, SC2034)

## Logging & Locking

- Logging: `log -l ERROR|WARN|INFO|DEBUG|TRACE "message"` (default: INFO)
- Locking: `acquire_lock [name]` prevents concurrent script execution; with `VPD_LOCK_WAIT=<sec>` (set by `vpn-director.sh --wait[=SEC]`) it waits for the lock instead of exiting 0
- Temp files: `tmp_file` / `tmp_dir` with auto-cleanup on exit

## Key Utilities (common.sh)

| Function | Description |
|----------|-------------|
| `uuid4` | Generate random UUIDv4 from kernel |
| `compute_hash [file\|-]` | SHA-256 digest of file or stdin |
| `get_script_path` | Absolute path to current script (resolves symlinks) |
| `get_script_dir` | Directory containing current script |
| `get_script_name [-n]` | Script filename; `-n` strips extension |
| `resolve_ip [-6] [-q] [-g] [-a] <host>` | DNS/hosts resolution |
| `resolve_lan_ip [-6] [-q] [-a] <host>` | Resolve only private/LAN addresses |
| `is_lan_ip [-6] <ip>` | Check if IP is in RFC1918/ULA range |
| `is_pos_int <value>` | Check if value is positive integer (>=1) |
| `strip_comments [text]` | Remove blank lines and # comments |
| `get_active_wan_if` | Get active WAN interface name |
| `get_ipv6_enabled` | Returns 1 if IPv6 enabled, 0 otherwise |
| `download_file <url> <dest> [timeout]` | Download with retry (wget/curl fallback) |
| `log_error_trace <msg>` | Log error with bash stack trace |

**Logging**: `LOG_FILE=/tmp/vpn-director.log` with 200KB rotation

## Firewall Utilities (firewall.sh)

| Function | Description |
|----------|-------------|
| `fw_chain_exists [-6] <table> <chain>` | Check if chain exists |
| `create_fw_chain [-6] [-q] [-f] <table> <chain>` | Create chain; `-f` flushes if exists |
| `delete_fw_chain [-6] [-q] <table> <chain>` | Flush and delete chain |
| `find_fw_rules [-6] "<table> <chain>" "<pattern>"` | Find rules matching regex |
| `purge_fw_rules [-6] [-q] [--count] "<table> <chain>" "<pattern>"` | Remove matching rules |
| `ensure_fw_rule [-6] [-q] [--count] <table> <chain> [-I [pos]\|-D] <rule>` | Idempotent rule add/delete |
| `sync_fw_rule [-6] [-q] [--count] <table> <chain> "<pattern>" "<desired>" [pos]` | Replace matching rules with one |
| `block_wan_for_host <host> [wan_id]` | Block host from WAN (IPv4/IPv6) |
| `allow_wan_for_host <host> [wan_id]` | Unblock host from WAN |
| `chg <cmd>` | Returns true if command output is non-zero integer |
| `validate_port <N>` | Validate port 1-65535 |
| `validate_ports <spec>` | Validate port spec (any, N, N-M, N,N2) |
| `normalize_protos <spec>` | Normalize to tcp, udp, or tcp,udp |

## State Tracking

Hash files in `/tmp/` detect config changes; scripts only reapply if changed.

## Bash-specific Patterns

- Use `[[ ]]` instead of `[ ]` for conditionals
- Use `read -ra array <<< "$string"` for splitting strings into arrays
- Use `${array[@]}` for iterating arrays
- Use `[[ $var =~ regex ]]` for regex matching instead of grep
- Debug mode: `DEBUG=1` enables `set -x` with PS4 showing file:line:function

## Known Pitfalls

### `tr` with POSIX character classes breaks on router

**Problem**: `tr '[:upper:]' '[:lower:]'` corrupts certain characters on Asuswrt-Merlin routers with Entware.

Example: letter `u` (0x75) becomes `l` (0x6c):
```bash
echo "SETUP" | tr '[:upper:]' '[:lower:]'
# Expected: setup
# Actual:   setlp
```

This is a bug in glibc/busybox `tr` with certain locale settings (`LC_ALL=en_US.UTF-8`). Setting `LC_ALL=C` does not fix it.

**Solution**: Always use explicit ASCII ranges instead of POSIX character classes:
```bash
# BAD - breaks on router
tr '[:upper:]' '[:lower:]'

# GOOD - works everywhere
tr 'A-Z' 'a-z'
```

**Rule**: Never use `[:upper:]` / `[:lower:]` in shell scripts for this project. Always use `A-Z` / `a-z`.

**Alternative fix**: Install `opkg install coreutils-tr` which provides a working `/opt/bin/tr`. After installation and `hash -r`, the correct `tr` will be used. However, code should still use `A-Z` / `a-z` for compatibility with systems without coreutils.

### Uninitialized arrays with `set -u`

**Problem**: With `set -u` (nounset), accessing uninitialized array length fails:
```bash
local -a my_array
echo ${#my_array[@]}  # Error: my_array: unbound variable
```

**Solution**: Always initialize arrays:
```bash
local -a my_array=()
echo ${#my_array[@]}  # Works: outputs 0
```

### A `flock` outlives the script through inherited descriptors

**Problem**: `exec 9>lock; flock -n 9` holds the lock on the open file
description, and descriptors a shell opens are not close-on-exec. Any process
started while the descriptor is open inherits it — a daemon launched with
`nohup … &` then holds the lock for its whole lifetime, long after the script
that took it has exited. Every later `acquire_lock` sees the file locked: with
no `--wait` it exits 0 without doing anything, with `--wait` it times out.

**Solution**: release and close before starting anything, not at script exit:
```bash
flock -u 9
exec 9>&-
```
Both the success path and the recovery path need it; `update_script.sh.tmpl`
does this in `release_apply_lock`.

When the lock has to stay held, close the descriptor for the child instead:
`tproxy_restart_process` runs the Xray init script as `"$xray_init" restart
200>&-`, so the daemon rc.func backgrounds cannot inherit the lock
`vpn-director.sh` is holding on FD 200. For the same reason `acquire_lock`
returns early when this process already holds the lock: reopening FD 200 drops
it and takes it again, and another waiter can step into that window.

**Related**: POSIX allows a single digit in a redirection. `exec 201>` is a
bash/ksh extension that dash rejects, and generated scripts run under
`/bin/sh`.

### `ip rule show` prints the table by name, not by number

**Problem**: `ip rule add ... table 100` reads back as `lookup wan0`, because
`/etc/iproute2/rt_tables` on Asuswrt-Merlin maps `100 wan0` (and `111 ovpnc1`,
`116 wgc1`, …). An idempotency check that greps the output for the table
*number* never recognises its own rule:

```bash
# never matches on a router: the kernel prints "lookup wan0"
ip rule show | grep -c "fwmark 0x100.*lookup 100"
```

`_tproxy_setup_routing` did exactly this and re-added its rule on every apply —
seven copies on a router with 23 days of uptime, and auto-apply from the Web UI
made each click add another.

**Solution**: the preference belongs to the module, so reconcile everything
sitting on it instead of looking for one tuple. Keep a rule that carries the
configured mark and table — comparing what the kernel *prints*, via
`_tproxy_table_label` — and delete the rest, including rules an earlier
`route_table` or `fwmark_mask` left behind. Matching only the configured tuple
has the mirror-image failure: a stale rule counts as ours, and the new setting
never gets installed.

```bash
want_table=$(_tproxy_table_label "$TABLE")     # 100 -> wan0
mark=$(printf '%s' "$line" | sed -n 's/.*fwmark \([^ ]*\).*/\1/p')
table=$(printf '%s' "$line" | sed -n 's/.*lookup \([^ ]*\).*/\1/p')
```

`ip rule del` removes one rule per call and fails when none is left, so a
teardown deletes by preference in a loop rather than once.

### A dual-family DNS lookup on the router often never answers

**Problem**: `/etc/resolv.conf` points the router's own processes straight at
8.8.8.8/8.8.4.4, and anything resolving `AF_UNSPEC` - curl, wget, every
`getaddrinfo` caller - asks for A and AAAA together. The AAAA half goes
unanswered often enough to matter, and glibc waits its full `timeout:` of five
seconds before retrying. Interleaved on an RT-AX86U, same window, same host:

```
curl -s  --connect-timeout 5 --max-time 10 ifconfig.me    16 failures / 30
curl -4 -s --connect-timeout 5 --max-time 10 ifconfig.me   0 failures / 30
```

The failure is `curl: (6) Could not resolve host` arriving at exactly 5.011s:
`--connect-timeout` covers name resolution, so a timeout at or below the
resolver's retry boundary turns a slow lookup into a hard failure. It is not
the binary - Entware's curl fails 10/10 the same way - and not reachability:
`--connect-timeout 20` succeeds in about six seconds. `GetExternalIP` was the
only caller to show this because its 5 s was the only connect timeout in the
repository below the boundary; everything else uses 10 or 30.

**Solution**: ask only for the family the router can route. Everything here is
IPv4 - the ipsets, the TPROXY rules, the fwmark tables - so `-4` requests what
the code actually needs rather than papering over the resolver:

```sh
curl -4 -s --connect-timeout 5 --max-time 10 ifconfig.me
```

Where a wait is acceptable instead, keep the connect timeout above five seconds
so the resolver's second attempt can land.

### BusyBox `sh` has no `command` builtin

**Problem**: on Asuswrt-Merlin, `/bin/sh` is BusyBox and `command` is not there:

```
$ /bin/sh -c 'command -v pgrep'
/bin/sh: command: not found       # exit 127, even though /opt/bin/pgrep exists
```

Every `command -v X` in a `#!/bin/sh` script therefore reports "missing" for
tools that are installed. In `update_script.sh.tmpl` this turned the guard
`if ! command -v pgrep` into a refusal of every self-update on the router, and
made the two `command -v monit` gates skip silently, so monit was never
unmonitored during an update and was free to restart a daemon mid-copy.

**Solution**: probe with `type`, which BusyBox does have, and keep `which` as a
fallback for a shell that has neither:

```sh
have_cmd() {
    type "$1" >/dev/null 2>&1 || which "$1" >/dev/null 2>&1
}
```

Scripts with a `#!/usr/bin/env bash` shebang run under Entware's bash
(`/opt/bin/bash`), where `command -v` works — this applies to `#!/bin/sh`
scripts: the generated update script and the init scripts.

**Related**: `/bin/bash` on the router is a symlink to busybox. A login shell
puts `/opt/bin` first in `PATH`, so `curl … | bash` resolves to the real bash
5.x, but a non-interactive `ssh router 'bash script.sh'` does not — call
`/opt/bin/bash` explicitly there.

### A deleted working directory breaks monit and every shell below it

**Problem**: a process whose cwd has been removed keeps running, but the dead
directory is inherited by everything it starts, and on the router that is not
cosmetic. monit refuses to run at all:

```sh
cd /tmp/gone && rm -rf /tmp/gone && monit status telegram-bot
# AssertException: Monit: Cannot read current directory -- No such file or directory
#  raised in init_env at src/env.c:111        (exit 1)
```

and every shell the affected daemons spawn prefixes its output with

```
shell-init: error retrieving current directory: getcwd: cannot access parent directories: No such file or directory
chdir: error retrieving current directory: getcwd: ...
```

which is what the Web UI's Status page then shows above the real output.

Under `set -e` the same deletion is fatal in another way: a `>>` redirect into
the removed directory fails, and the shell exits at that line.

**Seen as**: the self-update script ran from `/tmp/vpn-director-update`
(`cmd.Dir`), the bot deleted that directory the moment it reported success, and
the script's `monit monitor …  2>/dev/null || true` swallowed the exception —
`telegram-bot` stayed unmonitored, and all three daemons carried a dead cwd.

**Rule**: start long-lived processes from a directory nothing deletes (`/`), and
let a log helper survive the loss of its own file.

A daemon checking at startup whether its own directory still exists is **not** a
defence, and v0.11.4 shipped one that could never fire: the update script starts
the daemons while the directory is still there, and the bot removes it moments
later, once it has reported the update. The move has to be unconditional — with
any relative flag resolved before it, or `--config vpn-director.json` starts
naming `/vpn-director.json`. Dev mode is the exception: `DevPaths` are relative
to the source tree, so it keeps the directory it was started in.

```sh
log() {
    { echo "$1" >> "$LOG_FILE"; } 2>/dev/null || true
}
```
