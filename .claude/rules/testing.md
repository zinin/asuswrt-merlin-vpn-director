---
paths: "test/**/*"
---

# Testing

## Framework

Uses [Bats](https://bats-core.readthedocs.io/) (Bash Automated Testing System) with bats-support and bats-assert libraries.

## Running Tests

```bash
npx bats test/              # Run all tests
npx bats test/common.bats   # Run specific test file
npx bats test/*.bats        # Run all .bats files
```

## Test Structure

```
test/
├── test_helper.bash    # Shared setup, helpers, paths
├── fixtures/           # Test data files
│   ├── hosts           # Mock /etc/hosts
│   └── vpn-director.json
├── mocks/              # Mock executables
│   ├── nvram           # Mock nvram command
│   ├── iptables        # Mock iptables
│   ├── ip6tables       # Mock ip6tables
│   ├── ipset           # Mock ipset
│   ├── nslookup        # Mock DNS resolution
│   ├── logger          # Mock syslog
│   └── ip              # Mock ip command
├── common.bats         # Tests for common.sh utilities
├── firewall.bats       # Tests for firewall.sh
├── config.bats         # Tests for config.sh
├── ipset_builder.bats  # Tests for ipset_builder.sh
└── tunnel_director.bats # Tests for tunnel_director.sh
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
| `load_common` | Source common.sh with mocks in PATH |
| `load_firewall` | Source firewall.sh (includes common.sh) |
| `load_config` | Source config.sh with test fixture |

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

1. Create `test/new_feature.bats`
2. Add `load 'test_helper'` at top
3. Use appropriate `load_*` helper to source scripts
4. Add mocks to `test/mocks/` if needed
5. Run: `npx bats test/new_feature.bats`
