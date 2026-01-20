# test/test_helper.bash

# Load bats helpers
load '/usr/lib/bats/bats-support/load.bash'
load '/usr/lib/bats/bats-assert/load.bash'

# Project paths - find test/ directory regardless of nesting depth
_find_test_root() {
    local dir="$BATS_TEST_DIRNAME"
    while [[ "$dir" != "/" ]]; do
        if [[ -f "$dir/test_helper.bash" ]]; then
            printf '%s' "$dir"
            return
        fi
        dir="$(dirname "$dir")"
    done
    # Fallback to BATS_TEST_DIRNAME if not found
    printf '%s' "$BATS_TEST_DIRNAME"
}
export TEST_ROOT="$(_find_test_root)"
export PROJECT_ROOT="$TEST_ROOT/.."
export SCRIPTS_DIR="$PROJECT_ROOT/jffs/scripts/vpn-director"
export LIB_DIR="$SCRIPTS_DIR/lib"

# Test mode - disables syslog, uses fixtures
export TEST_MODE=1
export LOG_FILE="/tmp/bats_test_vpn_director.log"

# Override system paths for mocks
setup() {
    export PATH="$TEST_ROOT/mocks:$PATH"
    export HOSTS_FILE="$TEST_ROOT/fixtures/hosts"
    export RT_TABLES_FILE="$TEST_ROOT/fixtures/rt_tables"

    # Clean log file
    : > "$LOG_FILE"

    # Create a mock /etc/iproute2/rt_tables symlink for tests
    mkdir -p /tmp/bats_etc_iproute2
    ln -sf "$TEST_ROOT/fixtures/rt_tables" /tmp/bats_etc_iproute2/rt_tables
}

teardown() {
    # Cleanup temp files if any
    rm -rf /tmp/bats_test_*
}

# Helper to source common.sh with mocks
load_common() {
    # Set $0 to a fake script path for get_script_* functions
    export BASH_SOURCE_OVERRIDE="$SCRIPTS_DIR/test_script.sh"
    source "$LIB_DIR/common.sh"
}

# Helper to source firewall.sh (requires common.sh first)
load_firewall() {
    load_common
    source "$LIB_DIR/firewall.sh"
}

# Helper to source config.sh
load_config() {
    export VPD_CONFIG_FILE="$TEST_ROOT/fixtures/vpn-director.json"
    source "$LIB_DIR/config.sh"
}

# Helper to source import_server_list.sh without running main
load_import_server_list() {
    load_common
    export IMPORT_TEST_MODE=1
    source "$SCRIPTS_DIR/import_server_list.sh"
}
