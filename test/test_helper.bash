# test/test_helper.bash

# Load bats helpers
load '/usr/lib/bats/bats-support/load.bash'
load '/usr/lib/bats/bats-assert/load.bash'

# Project paths
export PROJECT_ROOT="$BATS_TEST_DIRNAME/.."
export SCRIPTS_DIR="$PROJECT_ROOT/jffs/scripts/vpn-director"
export UTILS_DIR="$SCRIPTS_DIR/utils"

# Test mode - disables syslog, uses fixtures
export TEST_MODE=1
export LOG_FILE="/tmp/bats_test_vpn_director.log"

# Override system paths for mocks
setup() {
    export PATH="$BATS_TEST_DIRNAME/mocks:$PATH"
    export HOSTS_FILE="$BATS_TEST_DIRNAME/fixtures/hosts"

    # Clean log file
    : > "$LOG_FILE"
}

teardown() {
    # Cleanup temp files if any
    rm -f /tmp/bats_test_*
}

# Helper to source common.sh with mocks
load_common() {
    # Set $0 to a fake script path for get_script_* functions
    export BASH_SOURCE_OVERRIDE="$SCRIPTS_DIR/test_script.sh"
    source "$UTILS_DIR/common.sh"
}

# Helper to source firewall.sh (requires common.sh first)
load_firewall() {
    load_common
    source "$UTILS_DIR/firewall.sh"
}

# Helper to source config.sh
load_config() {
    export VPD_CONFIG_FILE="$BATS_TEST_DIRNAME/fixtures/vpn-director.json"
    source "$UTILS_DIR/config.sh"
}
