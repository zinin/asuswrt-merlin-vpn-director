---
paths: "jffs/scripts/vpn-director/**"
---

# IPSet Builder

Builds country and combo ipsets for Tunnel Director and Xray TPROXY.

## Usage

```bash
ipset_builder.sh           # Restore from cache
ipset_builder.sh -u        # Force full rebuild
ipset_builder.sh -t        # Run tunnel_director.sh after
ipset_builder.sh -x        # Run xray_tproxy.sh after
ipset_builder.sh -c ru,ua  # Add extra countries
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

Cached dumps in `/tmp/ipset_builder/dumps/countries/`:
```
ru-ipdeny.dump
us-ipdeny.dump
...
```

**Restore flow**:
1. Try restore from dump
2. If missing or `-u` flag: download and rebuild
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
| `/tmp/ipset_builder/` | Base directory |
| `/tmp/ipset_builder/tun_dir_ipsets.sha256` | Rules hash after build |
| `/tmp/ipset_builder/dumps/` | Cached dumps |

## Boot Delay

Config options in `config.sh`:
- `BOOT_WAIT_DELAY`: seconds to wait if uptime < threshold
- `MIN_BOOT_TIME`: minimum uptime before running

## Key Functions

| Function | Purpose |
|----------|---------|
| `download_file()` | Curl wrapper with logging |
| `ipset_exists()` | Check if ipset is loaded |
| `build_ipset()` | Download, create, swap, dump |
| `restore_dump()` | Atomic restore from cache |
| `parse_country_codes()` | Extract CCs from rules |
| `derive_set_name()` | Handle long names (from `shared.sh`) |

## Build Steps

1. **Country ipsets**: Parse rules → download from IPdeny → create
2. **Combo ipsets**: Find multi-country refs → create `list:set`
3. **Save hash**: Write `TUN_DIR_IPSETS_HASH` for tunnel_director.sh
4. **Trigger scripts**: Run `-t` / `-x` if requested
