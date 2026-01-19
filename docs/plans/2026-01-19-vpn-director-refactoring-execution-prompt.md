## TASK

Execute the implementation plan for VPN Director Refactoring.

Use `/superpowers:subagent-driven-development` skill for execution.

## DOCUMENTS

- Design: `docs/plans/2026-01-19-vpn-director-refactoring-design.md`
- Plan: `docs/plans/2026-01-19-vpn-director-refactoring-impl.md`

Read both documents before starting.

## SESSION CONTEXT

Key decisions from brainstorming session:

### CLI Design Decisions

1. **Command structure**: `vpn-director <command> [component]` (operation-first, not component-first)
2. **Commands**: `status`, `apply`, `stop`, `restart`, `update`
3. **Components**: `tunnel`, `xray`, `ipset` (ipset only for `status`)
4. **No `rebuild` command** - `apply` automatically fetches missing ipsets from cache or downloads them
5. **`update` is global only** - no `update tunnel` or `update ipset`, just `vpn-director update`

### IPSet Module Decisions

1. **No `stop ipset` or `apply ipset`** - ipsets are internal dependencies, not user-managed components
2. **`status ipset`** - shows loaded ipsets, sizes, cache age
3. **Automatic cache/download logic** - `ipset_ensure()` checks: loaded in memory? → in cache? → download from IPdeny
4. **Force update via `vpn-director update`** - sets `IPSET_FORCE_UPDATE=1` before calling `ipset_ensure()`

### Module Independence

1. **`lib/ipset.sh` knows nothing about tunnel/xray** - it just builds ipsets by name
2. **Tunnel and tproxy modules declare required ipsets** - `tunnel_get_required_ipsets()`, `tproxy_get_required_ipsets()`
3. **CLI orchestrates** - collects required ipsets from all components, deduplicates, passes to `ipset_ensure()`

### Hook Simplification

1. **All hooks call `vpn-director apply`** - firewall-start, wan-event connected, S99vpn-director start
2. **`apply` is idempotent** - safe to call multiple times
3. **Cron job**: `0 3 * * * vpn-director update` (daily refresh of ipsets)

### Testing Approach

1. **`--source-only` pattern** - all modules support sourcing without execution for testing
2. **Unit tests in `test/unit/`** - test individual module functions
3. **Integration tests in `test/integration/`** - test CLI commands with mocks

### Rejected Alternatives

1. **Rejected: Component-first CLI** (`vpn-director tunnel status`) - less intuitive for common operations
2. **Rejected: Separate `rebuild` command** - confusing, `apply` should be smart enough
3. **Rejected: `update ipset` as separate command** - update always needs to reapply rules afterward
4. **Rejected: Keeping old scripts as wrappers** - clean break is simpler to maintain

### Edge Cases to Handle

1. **IPdeny unavailable but cache exists** - use cache with warning
2. **IPdeny unavailable and no cache** - error, abort apply for dependent component
3. **xt_TPROXY module unavailable** - skip tproxy_apply with warning, continue with tunnel
4. **VPN client not active** - tunnel_apply applies rules anyway (fwmark won't route, but rules are ready)
5. **Empty TUN_DIR_RULES** - log "no rules defined", skip tunnel_apply

### File Naming

1. **`lib/` not `utils/`** - more standard naming for module directories
2. **`tproxy.sh` not `xray.sh`** - describes what it does (TPROXY), not what tool it uses
3. **Keep `send-email.sh` → consider renaming to `notify.sh`** - optional, not critical

## SPECIAL INSTRUCTIONS

When launching the `superpowers:code-reviewer` agent:
- Also launch `codex-code-reviewer` agent in parallel
- Wait for both to complete
- Compare findings and address issues from both reviews
