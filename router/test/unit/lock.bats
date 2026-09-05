#!/usr/bin/env bats

load '../test_helper'

# acquire_lock opens /var/lock/<name>.lock on FD 200. The tests hold the same
# file on FD 201 from the test shell: `run` forks a subshell that inherits the
# open file description, and flock locks belong to descriptions, so the fresh
# open on FD 200 inside acquire_lock really contends with it.

LOCK_NAME="bats_lock_$$"
LOCK_FILE="/var/lock/${LOCK_NAME}.lock"

teardown() {
    rm -f "$LOCK_FILE"
    rm -rf /tmp/bats_test_*
    rm -rf /tmp/tunnel_director/*
}

hold_lock() {
    exec 201>"$LOCK_FILE"
    flock -n 201
}

@test "acquire_lock: takes a free lock and records the PID" {
    load_common
    run acquire_lock "$LOCK_NAME"
    assert_success
    [ -f "$LOCK_FILE" ]
    run cat "$LOCK_FILE"
    assert_output --regexp '^[0-9]+$'
}

@test "acquire_lock: busy lock without VPD_LOCK_WAIT exits 0 without waiting" {
    load_common
    hold_lock
    unset VPD_LOCK_WAIT
    local start=$SECONDS
    run acquire_lock "$LOCK_NAME"
    assert_success
    assert_output --partial "Another instance is already running"
    [ $((SECONDS - start)) -lt 2 ]
}

@test "acquire_lock: busy lock with VPD_LOCK_WAIT times out with ERROR and exit 1" {
    load_common
    hold_lock
    local start=$SECONDS
    VPD_LOCK_WAIT=1 run acquire_lock "$LOCK_NAME"
    assert_failure 1
    assert_output --partial "ERROR"
    assert_output --partial "Timed out waiting for lock"
    [ $((SECONDS - start)) -ge 1 ]
}

@test "acquire_lock: with VPD_LOCK_WAIT waits for the holder and then takes the lock" {
    load_common
    hold_lock
    # Release the shared description after ~1 s from a background subshell;
    # flock -u acts on the description, so the waiter's next retry succeeds.
    ( sleep 1; flock -u 201 ) &
    VPD_LOCK_WAIT=10 run acquire_lock "$LOCK_NAME"
    assert_success
    assert_output --partial "waiting up to 10s"
    wait
}

@test "acquire_lock: VPD_LOCK_WAIT with a leading zero is a decimal bound, not octal" {
    load_common
    hold_lock
    local start=$SECONDS
    # 08 is the smallest two-digit value that is not a valid octal number: read as
    # base 8 the timeout check never fires and the wait becomes unbounded.
    VPD_LOCK_WAIT=08 run acquire_lock "$LOCK_NAME"
    assert_failure 1
    assert_output --partial "Timed out waiting for lock"
    local elapsed=$((SECONDS - start))
    [ "$elapsed" -ge 8 ]
    [ "$elapsed" -lt 12 ]
}
