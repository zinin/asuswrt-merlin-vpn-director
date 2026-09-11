# Plan 1 carry-forward: what plans 2 and 3 inherit

Date: 2026-09-11
Branch: `feature/keenetic-platform`
Source: the SDD ledger of plan 1 (`docs/superpowers/plans/2026-09-10-platform-layer-foundation.md`),
which lived in a git-ignored workspace and is condensed here so it survives in branch history.

Plan 1 is complete: 11 tasks, 16 commits, 390 Bats tests and the Go suite green, a whole-branch
review with one Critical and three Important findings, all fixed and re-reviewed.

## 1. Release plan — do not cut a single release from this branch

The deployed updater on an already-installed Merlin router downloads its own hard-coded list of 19
paths, which cannot contain `lib/platform.sh` or `lib/platform/merlin.sh`, and its update script
copies `$FILES_DIR/opt/vpn-director/lib/*.sh` — a non-recursive glob that never enters
`lib/platform/`. The new `common.sh` sources `platform.sh` unconditionally as its last line, so a
single release cut from this branch installs a `common.sh` whose dependency the old updater cannot
deliver. Under `set -euo pipefail` every CLI invocation then dies: `apply`, `stop`, `status`,
`update`, the `cron install` that `S99vpn-director` now calls, both jffs hooks, and every shell
call the daemons make. Reproduced empirically during the final review.

Ship two releases instead:

| Release | Range | Why it is safe |
|---------|-------|----------------|
| N | `159ffd4..52b0e76` | Rename, manifest, manifest-driven installer and updater. No `platform.sh` in the tree, none in the manifest, and `common.sh` does not yet depend on it. An old binary installs its own 19 files, and the manifest-driven binary lands with them. |
| N+1 | `b820cd3..c6199c9` | The platform layer. The now-deployed updater reads the release's manifest, sees the two platform files, downloads them, and the generated script creates `lib/platform/` via `mkdir -p "$(dirname "$dst")"`. |

One pull request; the split is at tagging.

**Residual risk:** a router that never updates during release N and jumps straight from a pre-branch
version to N+1 still lands in the broken state, because the update flow always fetches the *latest*
release and the deployed binary's file list is fixed in the binary. Mitigations: the author's own
routers pass through N; `install.sh` repairs a router in one command; since this branch a missing
`platform.sh` logs one ERROR naming the file and the recovery instead of a bare "No such file or
directory"; and the Go update path never sources `common.sh`, so the update button repairs the router
once release N+2 exists. Not before: the old updater has already installed the N+1 daemons, so
`updateflow.Start` answers `ErrUpToDate` (`server/internal/updateflow/start.go:58-59`). A self-heal in
the N+1 daemons (at startup, fetch the manifest files missing for their own tag) would close the risk;
it is new Go code and has not been proposed.

### Release sequencing (found 2026-09-11, not yet decided)

- **Rename before N.** N already contains `159ffd4`: its binary (`repoName`), its `install.sh` and the
  README's install URL name `zinin/vpn-director`, which answers 404 until `gh repo rename` runs.
  Routers on v0.11.x reach the old name through GitHub's redirects; check one API and one raw URL of
  the old name right after the rename, before tagging N.
- **N before the merge.** `install.sh` is served from `master` (the README installs
  `…/master/install.sh`), not from the release tag, and it downloads the tag's `router/files.manifest`,
  which v0.11.5 lacks. From the merge until N is out every fresh install would fail, so tag `52b0e76`
  before the merge (the build workflow fires on any `v*` tag) or right after it. `install.sh` is
  byte-identical at `52b0e76` and at HEAD, so `master`'s installer serves N unchanged.
- **Plain releases.** `install.sh` rejects any tag but `vX.Y.Z`, and `/releases/latest` never returns a
  pre-release, so N has to be an ordinary release to be the one routers pass through.
- **Merge commit**, as for every earlier pull request, so `52b0e76` stays in `master`'s history. N's
  tree still carries `docs/superpowers/` (the spec and the plan predate `159ffd4`), so N's source
  archive includes them.
- **N+1's release notes** are what the old bot shows next to its update button: the place for "coming
  from v0.11.x, update to N first or run `install.sh`".
- **How long N stays the latest release** is the author's call.
- **The author's RT-AX86U is the upgrade test.** It is still byte-identical to v0.11.5 (the regression
  of Task 11 copied nothing), so it can take v0.11.5 → N with the old updater and N → N+1 with the new
  one. Keep it that way until then: files copied by hand would leave `lib/platform*` behind and hide
  whether the updater delivered them.

## 2. Contract invariants the Keenetic implementation must satisfy

Discovered while moving the core modules onto the contract. `lib/platform.sh`'s header documents the
signatures; these are the properties the callers actually rely on.

- `platform_tunnels ⊆ dom(platform_tunnel_table)`. Every id `platform_tunnels` lists must map, or
  `tunnel_apply` aborts under `set -e` at `table="$(platform_tunnel_table …)"`.
- `platform_lan_ifaces` must never return empty. Both `tproxy.sh` and `tunnel.sh` now capture the
  list before touching the firewall and fail loudly on an empty answer — before this branch's final
  fix wave they built a fully populated chain with no jump at all and reported success, which for
  TPROXY is a silent leak of exactly the traffic the proxy exists to carry.
- `platform_tunnel_route_ensure` must be idempotent: `tunnel_apply` calls it on every up-to-date
  apply, which is what keeps a Keenetic route alive across interface flaps.
- `platform_tunnel_table_release` may be called for an index that was never ensured, because
  `tunnel_stop` walks `/tmp/tunnel_director/tun_dir_tables` unconditionally.
- `platform_tproxy_extra_rules` must accept exactly `apply|stop` and reject anything else — the
  Merlin implementation now does, so a typo fails instead of silently succeeding.
- No platform function may log. The caller owns every message (spec 5.2), and `platform.sh` can be
  sourced before `common.sh` defines `log`.
- `platform_tproxy_extra_rules stop` failing must not abort teardown: `tproxy_stop` is called bare
  under `errexit`, so the call site uses `|| true`. The `apply` side logs a WARN, because
  `_tproxy_setup_iptables` runs under `if !`, which would swallow the failure entirely.

## 3. Known gaps in the abstraction

- `_tproxy_table_label` (`lib/tproxy.sh`) reads `/etc/iproute2/rt_tables` directly — the one
  firmware fact still read outside the contract. It degrades correctly (iproute2 renders a table by
  name when rt_tables has one and by number otherwise, and the function falls back to the number,
  which is the Keenetic case), but a reader of the platform docs will expect otherwise.
- `install.sh` reads `nvram` directly at three places and detects the platform with an inline copy
  of the rules, because it runs before the libraries exist. Spec 11 puts the Keenetic installer in
  plan 2; `create_directories` also still creates `/jffs/scripts` unconditionally and ignores both
  `PLATFORM` and `INSTALL_ROOT`.
- `S99vpn-director` still calls `cru d update_ipsets` directly — the legacy cleanup this plan keeps
  deliberately, and the one exception to the "platform facts belong behind the contract" convention.
- `configure.sh` is not on the contract at all: it does not source `common.sh`, and its tunnel
  prompt only changes meaningfully once Keenetic tunnels exist (plan 2).
- `.claude/rules/entware-init.md` and `.claude/rules/testing.md` stay Merlin-framed; the latter never
  mentions `VPD_PLATFORM`/`VPD_PROBE_ROOT` although `CLAUDE.md` now points readers at
  `test_helper.bash`.

## 4. Deferred findings, verbatim from the ledger

Every one was raised by a task review, triaged by the whole-branch review, and judged safe to defer.
The four the whole-branch review called must-fix are already fixed in `c6199c9` and are not listed.

- **Task 1**: the report's "Files changed" section credits import changes to `github.go`/`github_test.go`, which only changed a literal and gained the test — report text only, no code impact.
- **Task 1**: `server/internal/wizard/handler.go` (missing trailing newline) and `server/internal/ssrf/ssrf_test.go` fail `gofmt -l`; both defects pre-date this branch (verified at db9309f) and were correctly left alone. Worth one cleanup commit, out of this plan's scope — final review to triage.
- **Task 2**: temp manifest leaks when `mkdir -p`/`chmod +x` abort under `set -e` (`install.sh:185,192`) — a `trap 'rm -f "$manifest"' RETURN` after `mktemp` covers every exit and folds three `rm -f` calls into one.
- **Task 2**: `${file#router/}` (`install.sh:184`) does not check the prefix, so an entry outside `router/` would install to a wrong absolute path; the Go parser validates it (Task 3 regex). Same threat model as every other file fetched from the release, so authoring-typo protection only.
- **Task 2**: the fake `curl` in `install.bats` exits without touching `-o` on failure, while the real one leaves a zero-byte target on 404 — the "empty file at the destination" path is uncovered.
- **Task 2**: the per-file download failure path (`install.sh:186-190`) has no test; only the manifest download failure is covered.
- **Task 2**: `create_directories` still creates `/jffs/scripts` unconditionally and ignores `PLATFORM`/`INSTALL_ROOT` — belongs to the Keenetic installer work in plan 2, not a defect of this commit.
- **Task 3**: `downloader.go:63` hardcodes the basename `"files.manifest"` instead of deriving it from `manifestPath`, so two spellings must agree (caught by tests today, hence Minor).
- **Task 3**: `downloader.go:61` reports every transient fetch error as `release %s has no files.manifest`, telling an operator the release is malformed when the network was at fault.
- **Task 3**: `manifest_test.go:86-94,108-116` duplicate the open/parse/fatal preamble verbatim — extract a `loadRepoManifest(t)` helper.
- **Task 3**: no test pins an empty manifest, a comment-only manifest or CRLF input, and `TestParseManifest_RejectsMalformedLines` asserts only that an error occurred, never that it names the line or path.
- **Task 3**: `ManifestEntry` is exported while every producer and consumer is unexported; brief-mandated shape, worth lowering after Task 4 confirms it stays package-local.
- **Task 3**: `manifest.go:43` also rejects a legitimate `router/a..b`; `TestGetPlatform_DefaultsToMerlin` sits in `downloader_test.go` while testing `updater.go`.
- **Task 4**: `script_test.go:99-107` lists only 2 of the 6 retired `cp` globs and 1 of the 5 retired `chmod` lines as forbidden strings; the golden is the real insurance and shows all eleven gone.
- **Task 4**: the "fail loudly on an incomplete payload" property is gone — six globs under `set -e` used to abort on a missing group, while a short-but-non-empty table now copies what it names and reports success. `downloader.go:46-48` only rejects a wholly empty selection. Reassess if a later task makes `generateScript` reachable with a payload not produced by `DownloadRelease`.
- **Task 4**: the destination perimeter widened — the globs implicitly pinned every destination to four directories, while the table writes as root to any path `manifestPathRe` admits. Intended by the spec (Keenetic's `opt/etc/ndm/**`) and no added risk, but the report's "nothing was relaxed" should be read narrowly: the parser was not relaxed, the destination constraint did disappear.
- **Task 4**: the behavioural test covers only the "destination absent" path (`script_test.go:281`, `t.TempDir()` is empty). In production every destination exists and `cp -f` preserves the *destination's* mode, so a `.template` whose destination is already executable stays executable — the loop has no `chmod -x`. Same as the old script's behaviour; spec 4.3 speaks of installation.
- **Task 5**: `platform_cron_add`'s `local name="$1" schedule="$2" cmd="$3"` (`merlin.sh:120-122`) is unguarded, so under `set -u` a short call aborts the caller instead of returning 1; nothing covers `cru` itself failing either.
- **Task 5**: `platform.bats:52-58` passes for the wrong reason on a dev box — with `VPD_PROBE_ROOT` ignored it would fail anyway, `/jffs` being absent. A tree with `/jffs` but no `/bin/nvram` would also cover the untested "directory present, binary missing" half of both probes.
- **Task 5**: `lsmod | grep -q "$name"` (`merlin.sh:110-116`) is a substring match — `xt_TPROXY` also matches `nf_tproxy_ipv4`'s dependency column — faithful to `lib/tproxy.sh:90` and worth a comment so the Keenetic author does not copy it as intentional; `modprobe "$name" 2>/dev/null` suppresses only stderr, so modprobe stdout could leak onto the answer channel.
- **Task 5**: `merlin.sh:46-51`'s `{ …; } || true` wrappers are a small behaviour delta from `_tunnel_init` (`lib/tunnel.sh:91-96`), where a missing `rt_tables` killed the substitution subshell under `set -e` before `printf main`. The new form is better and the brief's test mandates it — noted only because "behaviour on Merlin does not change" is not literally true here.
- **Task 5**: `platform_hostname` and `platform_model` (`merlin.sh:157-171`) have no failure-path test.
- **Task 6**: a dual-WAN semantic shift nobody declared — the old code always read `wan0_ifname` (the `wan_id=0` default) while `platform_wan_if` returns the *active* WAN, so on a failover router `block_wan_for_host` and `allow_wan_for_host` can name different interfaces at different moments and an unblock after a WAN switch would leave a stale `-i <old_if> … -j DROP` in FORWARD. Rules are byte-identical while primary is wan0; the plan's constraint is worded "identical … for the primary WAN" and there are no internal callers. Worth a line in the doc blocks (`firewall.sh:853,931`).
- **Task 6**: `allow_wan_for_host` has no contract-failure test, while its twin does (`firewall.bats:174-180`); neither the `platform_wan_if` failure nor the "cannot unblock WAN for host" message is pinned.
- **Task 6**: the `get_ipv6_enabled` → `platform_ipv6_enabled` substitution inside `firewall.sh:885,958` is pinned by nothing — in both new tests the override was a no-op, because with `ipv6_service=native` the v6 branch ran but `resolve_ip -6` found nothing, yielding the same rule set either way.
- **Task 6**: `firewall.sh:885,958` handle a contract failure only implicitly — a `platform_ipv6_enabled` that prints nothing and returns 1 makes `[[ "" -eq 1 ]]` false, so v6 rules are silently skipped and the function still logs "Blocked WAN for host=…". Same effect as the old `|| printf '0\n'` fallback and unreachable on Merlin, but on a future platform "could not answer" becomes "IPv6 not blocked" with no WARN.
- **Task 7**: `tunnel.bats:404-413` counts two *calls* in the mock's cumulative log rather than two surviving jumps — the `-S` mock returns no inserted rules, so both insertions happen and the test would pass with the old broad pattern too; the narrowed pattern at `tunnel.sh:442` is therefore uncovered.
- **Task 7**: `tunnel.sh:296` still logs "Configuration changed; removing existing rules…" although rebuild now has three triggers — a rebuild caused by a missing state file reports a config change that did not happen.
- **Task 7**: the WARN text is duplicated verbatim at `tunnel.sh:134` and `:423` (brief-mandated); `tunnel.sh:106`'s doc block for `_tunnel_table_allowed` still says "valid (wgcN, ovpncN, or main)", contradicting the now-neutral header summary at `:28` — **third Task 10 doc item**.
- **Task 7**: on the up-to-date path a failing `platform_tunnel_route_ensure` logs a WARN and is then immediately followed by "Rules are applied and up-to-date", with no `warnings` flag on that path — precisely the interface-flap scenario the state file exists for, reported more optimistically than the facts.
- **Task 8**: `resolve_ip` now runs with stdin bound to the process-substitution FD (`tproxy.sh:377-385`); safe today because `_resolve_ip_impl` reads no stdin (`common.sh:184-230` uses `awk` on the hosts file and `nslookup | awk`), but a backend that ever reads stdin would silently swallow the rest of the endpoint list — `resolve_ip … </dev/null` immunises the loop.
- **Task 8**: `tproxy.sh:410`'s rule comment still says "OpenVPN endpoints" while the function header at `:328` was updated to "Firmware VPN client endpoints" in the same commit; the unresolvable-endpoint branch is untested although its WARN text changed; and the neighbouring endpoint loop has `|| true` while the interface loop does not, which reads as a difference where there is none.
- **Task 8**: tests 40 and 44 (`tproxy.bats:425-431,474-480`) assert only that the stub was called — a line the stub itself writes — so they would pass on an implementation that calls the platform and ignores its exit status, i.e. on the code with Important 1. In test 43 the `-I PREROUTING 1 -i br0` assertion would also pass on the old hard-coded jump; the `br1` assertion beside it is the load-bearing half.
- **Task 9**: the `cru` mock logs `echo "cru $*"`, which flattens argument boundaries, so a quoting regression that split the schedule from the command would log identically — the byte-identity guarantee is therefore not actually asserted; a mock logging one argument per line would make it real.
- **Task 9**: `vpn-director.sh:348-349` lack the degradation `wan` gets one line above — a `platform_lan_ifaces` or `platform_password_file` that exits 1 aborts `cmd_platform` with no document at all. Constant `printf`s on Merlin, dynamic on Keenetic in plan 2.
- **Task 9**: `desc="$(… | sed -n 3p)"` truncates a multi-line description to its first line — inherited from the contract's line-oriented shape, a content defect rather than a parse one.
- **Task 9**: the `help lists platform and cron` test omits `assert_success`, unlike its neighbour.
- **Task 9**: the overriding `setup()` in `vpn_director.bats:14-24` still drops the helper's `HOSTS_FILE` export and its `/tmp/bats_etc_iproute2/rt_tables` symlink, so the same trap awaits the next test added to that file; calling the helper's `setup` from it would end the class of bug rather than copying exports one at a time.
- **Task 10**: `packet-flow.md:60-63`'s "Packet never continues to the TUN_DIR chain" holds after a full apply (`vpn-director.sh:234-235` runs `tunnel_apply` then `tproxy_apply`, which inserts from position 1 and shifts TD down) but not after `apply tunnel` alone; the conclusion still holds because pref 200 beats 16384, and the imprecision predates this plan.
- **Task 10**: `packet-flow.md:19,36`'s ASCII diagram still hard-codes "pos 2: TUN_DIR" and "(br0)" while the prose below now says "(Merlin, one LAN interface)"; and `shell-conventions.md:19` heads its table "Key Utilities (common.sh)" although it now lists functions defined in `lib/platform/<name>.sh`.

## 5. Rulings that shape later work

- **R7** — `TestRepoName_IsRenamed` was kept although it compares a constant with a literal and
  duplicates `TestGetLatestRelease`; the next rename touches two literals.
- **R8/R10** — the shell manifest parser fails on an unknown tag while the Go parser ignores one.
  Deliberate: the installer reads the manifest of the release it installs, so a bad tag there can
  only be an authoring typo, while an installed binary may legitimately meet a newer release's tag.
- **R13** — `platform_tunnel_table` validates its id. Spec 5.2's "pure mapping, always succeeds for
  ids from `platform_tunnels`" still holds; only an id that is not a tunnel at all now fails.
- **R16** — each LAN interface gets its own PREROUTING position (`1, 2, …` for TPROXY,
  `base_pos + n` for Tunnel Director). Unreachable today because `platform_lan_ifaces` returns one
  interface on both platforms and spec 16 puts additional bridges out of scope, but the shape is now
  the same in both modules.

## 6. Facts from the author's router (read-only, 2026-09-11)

- `platform_tunnels` lists every slot `rt_tables` names, configured or not: on the RT-AX86U that is
  `wgc1`–`wgc5` and `ovpnc1`–`ovpnc5`, seven of them down and four with empty descriptions. A route
  list built from it (plan 3) would show ten entries, as the old hard-coded list did; filter it to
  configured tunnels there.
- The router has a second WAN (`wan1_ifname` set, not primary), so the Task 6 note about
  `platform_wan_if` naming the *active* WAN is live there once `wan_if` has a consumer.
- A read-only check on a router: stream the script with `ssh … '/opt/bin/bash -s' < script`, wrapped
  in one `{ … }` so bash has read all of it before running anything, with `PATH=/opt/bin:…` exported,
  and with `iptables` (except `-S`), `ip` (except `show`), `nvram` (except `get`), `ipset`, `cru`,
  `modprobe` and the mutating contract functions shadowed by stubs that refuse.
