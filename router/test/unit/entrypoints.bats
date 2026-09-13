#!/usr/bin/env bats

load '../test_helper'

# The scripts a router executes directly - by hand, from S99vpn-director, from
# an NDM hook, or from the Go daemons (exec.CommandContext on the CLI path).
# KeeneticOS has no /usr/bin/env and its root filesystem is a read-only
# squashfs, so "#!/usr/bin/env bash" cannot be repaired on the device: every
# one of these has to start as a POSIX shell and hand over to bash itself.
# Library files are excluded on purpose - they are only ever sourced, except
# lib/send-email.sh, which S99vpn-director executes.
entry_scripts() {
    printf '%s\n' \
        "$SCRIPTS_DIR/vpn-director.sh" \
        "$SCRIPTS_DIR/configure.sh" \
        "$SCRIPTS_DIR/import_server_list.sh" \
        "$SCRIPTS_DIR/setup_telegram_bot.sh" \
        "$SCRIPTS_DIR/lib/send-email.sh" \
        "$PROJECT_ROOT/../install.sh"
}

@test "executed scripts do not depend on /usr/bin/env" {
    local script hit
    while IFS= read -r script; do
        [[ -f "$script" ]] || fail "missing entry script: $script"
        assert_equal "$(head -1 "$script")" '#!/bin/sh'
        # Naming it in a comment is fine; a shebang or a command is not.
        while IFS= read -r hit; do
            [[ -z "$hit" || "${hit#*:}" == \#* ]] ||
                fail "$script still reaches for /usr/bin/env, which KeeneticOS lacks: $hit"
        done < <(grep -n '/usr/bin/env' "$script" || true)
    done < <(entry_scripts)
}

@test "executed scripts carry the same hand-over block" {
    # By absolute path, Entware's bash first. PATH is not usable: Merlin's
    # /bin/sh has no "command" builtin and its /bin/bash is busybox.
    local script
    while IFS= read -r script; do
        run grep -c 'for _vpd_bash in /opt/bin/bash /usr/bin/bash /bin/bash' "$script"
        assert_success
        assert_output "1"
        run grep -c 'exec "\$_vpd_bash" "\$0" "\$@"' "$script"
        assert_success
        assert_output "1"
    done < <(entry_scripts)
}

@test "the hand-over block runs the body under real bash" {
    # The block itself, lifted out of the CLI and given a body that reports
    # which interpreter it ended up in.
    local probe="$BATS_TEST_TMPDIR/probe.sh"
    sed -n '1,/^fi$/p' "$SCRIPTS_DIR/vpn-director.sh" > "$probe"
    printf 'printf "BASH=%%s\\n" "${BASH_VERSION:-none}"\n' >> "$probe"
    chmod +x "$probe"

    # Started by a POSIX shell, the way KeeneticOS starts our hooks' children.
    run sh "$probe"
    assert_success
    refute_output --partial "BASH=none"

    # And started through its own shebang, the way a router runs the CLI.
    run "$probe"
    assert_success
    refute_output --partial "BASH=none"
}

@test "vpn-director.sh started by /bin/sh runs its bash body" {
    run sh "$SCRIPTS_DIR/vpn-director.sh" --help
    assert_success
    assert_output --partial "Usage:"
}
