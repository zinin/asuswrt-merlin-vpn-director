---
paths: "jffs/scripts/vpn-director/**"
---

# IPSet Builder

Builds country and combo ipsets for Tunnel Director and Xray TPROXY.

Module location: `lib/ipset.sh`

## Usage via CLI

```bash
vpn-director.sh update              # Update ipsets + reapply all
vpn-director.sh apply               # Restore ipsets from cache + apply
```

## Data Sources

Sources are tried in priority order with automatic fallback:

| Priority | Source | URL Pattern | Notes |
|----------|--------|-------------|-------|
| 1 | GeoLite2 (GitHub) | `raw.githubusercontent.com/firehol/blocklist-ipsets/master/geolite2_country/country_{cc}.netset` | Most accurate |
| 2 | IPDeny (GitHub) | `raw.githubusercontent.com/firehol/blocklist-ipsets/master/ipdeny_country/id_country_{cc}.netset` | Mirror, not blocked |
| 3 | IPDeny (direct) | `www.ipdeny.com/ipblocks/data/aggregated/{cc}-aggregated.zone` | May be blocked in some regions |
| 4 | Manual | `/tmp/{cc}.zone` | Interactive mode only |

Files from GitHub sources contain comment lines (`#`) which are filtered automatically.

**Download timeout**: 30 seconds per source (uses `download_file()` from common.sh).

## IPSet Types

### Country Sets
- Named by 2-letter ISO code: `us`, `ru`, `ca`
- Type: `hash:net`
- Size: auto-calculated (load factor ~0.75)

## Dump/Restore

Cached dumps stored in `$IPS_BDR_DIR/ipsets/`:
```
ru.dump
us.dump
...
```

**Restore flow**:
1. Try restore from dump
2. If missing or forced (via `vpn-director.sh update`): download and rebuild
3. Atomic swap: `create tmp → swap → destroy tmp`

## Long Set Names

Names > 31 chars get SHA-256 prefix alias:
```bash
derive_set_name "very_long_set_name..."
# Returns: 24-char hash prefix
```

## State Files

| Path | Purpose |
|------|---------|
| `/tmp/ipset_builder/` | Runtime state directory (`IPS_BUILDER_DIR` in lib/ipset.sh) |
| `/tmp/ipset_builder/tun_dir_ipsets.sha256` | Rules hash after build (for tunnel module sync) |
| `$IPS_BDR_DIR/ipsets/` | Persistent dump storage (from `data_dir`) |

## Boot Delay

Config options in `vpn-director.json` (`advanced.boot`):
- `wait_delay`: seconds to wait if uptime < threshold
- `min_time`: minimum uptime before running

## Key Functions

**Public API**:

| Function | Purpose |
|----------|---------|
| `ipset_status()` | Show loaded ipsets, sizes, cache info |
| `ipset_ensure(spec)` | Ensure ipsets exist (load from cache or download) |
| `ipset_update(spec)` | Force fresh download of ipsets |

**Internal functions** (for testing):

| Function | Purpose |
|----------|---------|
| `_ipset_exists(name)` | Check if ipset is loaded |
| `_ipset_count(name)` | Get number of entries in ipset |
| `_build_country_ipset(cc, dump_dir)` | Download, create, swap, dump |
| `_restore_from_cache(name, dump, force)` | Atomic restore from dump |
| `_download_zone_multi_source(cc, dest)` | Try all sources with fallback |
| `_try_download_zone(url, tmp, dest)` | Download and validate single source |
| `_try_manual_fallback(cc, dest)` | Interactive manual fallback |
| `parse_exclude_sets_from_json()` | Extract exclude codes from tunnels JSON |
| `_derive_set_name(name)` | Handle long names (>31 chars → SHA-256 prefix) |
| `_calc_ipset_size(count)` | Calculate hashsize from entry count |
| `_normalize_spec(spec)` | Validate and normalize ipset spec |
| `_is_valid_country_code(cc)` | Check 2-letter ISO code validity |

## Build Steps

1. **Country ipsets**: Parse exclude arrays from JSON → download → create
2. **Save hash**: Write `TUN_DIR_IPSETS_HASH` for tunnel module sync
