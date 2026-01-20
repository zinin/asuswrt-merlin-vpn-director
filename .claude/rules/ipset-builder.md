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

| Type | Source | Format |
|------|--------|--------|
| Country | IPdeny | `{cc}-aggregated.zone` |

**URL pattern**: `https://www.ipdeny.com/ipblocks/data/aggregated/{cc}-aggregated.zone`

## IPSet Types

### Country Sets
- Named by 2-letter ISO code: `us`, `ru`, `ca`
- Type: `hash:net`
- Size: auto-calculated (load factor ~0.75)

### Combo Sets
- Union of multiple sets: `us,ca,uk` → `us_ca_uk`
- Type: `list:set`
- Created when rules reference multiple countries

## Dump/Restore

Cached dumps stored in `$IPS_BDR_DIR/ipsets/` (configurable via `data_dir` in vpn-director.json, default: `/jffs/scripts/vpn-director/data`):
```
ru-ipdeny.dump
us-ipdeny.dump
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

| Function | Purpose |
|----------|---------|
| `download_file()` | Download file with retry and timeout (prefers wget-ssl, fallback curl) |
| `ipset_exists()` | Check if ipset is loaded |
| `get_ipset_count()` | Get number of entries in ipset |
| `build_ipset()` | Download, create, swap, dump |
| `restore_dump()` | Atomic restore from cache |
| `save_hashes()` | Save rules hash for tunnel_director sync |
| `parse_country_codes()` | Extract CCs from rules (field 4 & 5) |
| `parse_combo_from_rules()` | Extract comma-separated combos |
| `derive_set_name()` | Handle long names (from `lib/ipset.sh`) |
| `_calc_ipset_size()` | Calculate hashsize from entry count |

## Build Steps

1. **Country ipsets**: Parse rules → download from IPdeny → create
2. **Combo ipsets**: Find multi-country refs → create `list:set`
3. **Save hash**: Write `TUN_DIR_IPSETS_HASH` for tunnel module sync
