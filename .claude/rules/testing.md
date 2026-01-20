---
paths: "test/**/*"
---

# Testing

## Framework

Uses [Bats](https://bats-core.readthedocs.io/) (Bash Automated Testing System) with bats-support and bats-assert libraries.

## Running Tests

```bash
npx bats test/              # Run all tests (recursive)
npx bats test/unit/         # Run unit tests only
npx bats test/integration/  # Run integration tests only
npx bats test/common.bats   # Run specific test file
```

## Test Structure

```
test/
├── test_helper.bash         # Shared setup, helpers, paths
├── fixtures/                # Test data files
│   ├── hosts                # Mock /etc/hosts
│   ├── vpn-director.json
│   └── vless_servers.b64
├── mocks/                   # Mock executables
│   ├── nvram                # Mock nvram command
│   ├── iptables             # Mock iptables
│   ├── ip6tables            # Mock ip6tables
│   ├── ipset                # Mock ipset
│   ├── nslookup             # Mock DNS resolution
│   ├── logger               # Mock syslog
│   └── ip                   # Mock ip command
├── unit/                    # Unit tests for lib/ modules
│   ├── ipset.bats           # Tests for lib/ipset.sh
│   ├── tunnel.bats          # Tests for lib/tunnel.sh
│   └── tproxy.bats          # Tests for lib/tproxy.sh
├── integration/             # Integration tests
│   └── vpn_director.bats    # Tests for vpn-director.sh CLI
├── common.bats              # Tests for lib/common.sh
├── firewall.bats            # Tests for lib/firewall.sh
├── config.bats              # Tests for lib/config.sh
└── import_server_list.bats  # Tests for import_server_list.sh
```

## Writing Tests

### Basic Test

```bash
@test "function_name: description of behavior" {
    load_common  # Load utilities with mocks
    run function_name arg1 arg2
    assert_success
    assert_output "expected output"
}
```

### Test Helpers

| Helper | Purpose |
|--------|---------|
| `load_common` | Source lib/common.sh with mocks in PATH |
| `load_firewall` | Source lib/firewall.sh (includes common.sh) |
| `load_config` | Source lib/config.sh with test fixture |
| `load_ipset_module` | Source lib/ipset.sh module |
| `load_tunnel_module` | Source lib/tunnel.sh module |
| `load_tproxy_module` | Source lib/tproxy.sh module |

### Assertions (bats-assert)

| Assertion | Purpose |
|-----------|---------|
| `assert_success` | Exit code 0 |
| `assert_failure` | Exit code != 0 |
| `assert_output "text"` | Exact match |
| `assert_output --partial "text"` | Contains substring |
| `assert_output --regexp "pattern"` | Regex match |
| `assert_line "text"` | Line exists in output |
| `refute_output` | Output is empty |

### Environment Variables

| Variable | Set By | Purpose |
|----------|--------|---------|
| `TEST_MODE=1` | test_helper.bash | Disables syslog, uses fixtures |
| `LOG_FILE` | test_helper.bash | Test-specific log file |
| `HOSTS_FILE` | test_helper.bash | Mock hosts file path |
| `VPD_CONFIG_FILE` | load_config | Test config fixture path |
| `BATS_TEST_DIRNAME` | Bats | Directory containing test file |
| `PROJECT_ROOT` | test_helper.bash | Project root directory |

## Mocks

Mocks are shell scripts in `test/mocks/` that simulate router commands:

- **nvram**: Returns predefined values for router settings
- **iptables/ip6tables**: Tracks rule operations
- **ipset**: Simulates ipset management
- **nslookup**: Returns mock DNS responses
- **logger**: Silent (no syslog in tests)
- **ip**: Returns mock interface info

Mocks are added to PATH before real commands via `setup()`.

## Adding New Tests

1. Create test file in appropriate directory:
   - `test/unit/` for module unit tests
   - `test/integration/` for CLI integration tests
   - `test/` for utility tests (common, firewall, config)
2. Add `load 'test_helper'` at top (use relative path: `load '../test_helper'` from subdirs)
3. Use appropriate `load_*` helper to source scripts
4. Add mocks to `test/mocks/` if needed
5. Run: `npx bats test/unit/new_module.bats`
