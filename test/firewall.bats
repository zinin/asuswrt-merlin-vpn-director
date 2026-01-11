#!/usr/bin/env bats
# test/firewall.bats - Tests for firewall.sh utility functions

load 'test_helper'

# ============================================================================
# validate_port
# ============================================================================

@test "validate_port: accepts valid port 80" {
    load_firewall
    run validate_port 80
    assert_success
}

@test "validate_port: accepts valid port 443" {
    load_firewall
    run validate_port 443
    assert_success
}

@test "validate_port: accepts valid port 65535" {
    load_firewall
    run validate_port 65535
    assert_success
}

@test "validate_port: rejects port 0" {
    load_firewall
    run validate_port 0
    assert_failure
}

@test "validate_port: rejects port 70000" {
    load_firewall
    run validate_port 70000
    assert_failure
}

@test "validate_port: rejects non-numeric" {
    load_firewall
    run validate_port "abc"
    assert_failure
}

@test "validate_port: rejects empty" {
    load_firewall
    run validate_port ""
    assert_failure
}

# ============================================================================
# validate_ports
# ============================================================================

@test "validate_ports: accepts 'any'" {
    load_firewall
    run validate_ports "any"
    assert_success
}

@test "validate_ports: accepts single port" {
    load_firewall
    run validate_ports "443"
    assert_success
}

@test "validate_ports: accepts port range" {
    load_firewall
    run validate_ports "1000-2000"
    assert_success
}

@test "validate_ports: accepts comma list" {
    load_firewall
    run validate_ports "80,443,8080"
    assert_success
}

@test "validate_ports: accepts mixed list with range" {
    load_firewall
    run validate_ports "80,443,1000-2000"
    assert_success
}

@test "validate_ports: rejects invalid range (start > end)" {
    load_firewall
    run validate_ports "2000-1000"
    assert_failure
}

# ============================================================================
# normalize_protos
# ============================================================================

@test "normalize_protos: returns tcp for tcp" {
    load_firewall
    run normalize_protos "tcp"
    assert_success
    assert_output "tcp"
}

@test "normalize_protos: returns udp for udp" {
    load_firewall
    run normalize_protos "udp"
    assert_success
    assert_output "udp"
}

@test "normalize_protos: returns tcp,udp for any" {
    load_firewall
    run normalize_protos "any"
    assert_success
    assert_output "tcp,udp"
}

@test "normalize_protos: normalizes udp,tcp to tcp,udp" {
    load_firewall
    run normalize_protos "udp,tcp"
    assert_success
    assert_output "tcp,udp"
}
