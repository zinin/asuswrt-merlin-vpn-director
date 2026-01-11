#!/usr/bin/env bats

load 'test_helper'

# ============================================================================
# uuid4
# ============================================================================

@test "uuid4: returns valid UUID format" {
    load_common
    run uuid4
    assert_success
    # UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
    assert_output --regexp '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
}

@test "uuid4: generates unique values" {
    load_common
    uuid1=$(uuid4)
    uuid2=$(uuid4)
    [ "$uuid1" != "$uuid2" ]
}

# ============================================================================
# compute_hash
# ============================================================================

@test "compute_hash: hashes string from stdin" {
    load_common
    result=$(echo -n "test" | compute_hash)
    # SHA-256 of "test" is known
    [ "$result" = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08" ]
}

@test "compute_hash: hashes file" {
    load_common
    echo -n "test" > /tmp/bats_hash_test
    run compute_hash /tmp/bats_hash_test
    assert_success
    assert_output "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
    rm /tmp/bats_hash_test
}
