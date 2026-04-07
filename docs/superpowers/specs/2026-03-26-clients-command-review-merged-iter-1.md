# Merged Design Review — Iteration 1

## codex-executor (gpt-5.4)

- [ARCH-1] IP format mismatch (IP vs IP/32) — Major
- [CONC-1] Lost update in concurrent load-modify-save — Major
- [CONC-2] Split-brain: config saved before apply succeeds — Major
- [SHELL-1] jq array subtraction requires normalized values — Major
- [SHELL-2] to_entries/from_entries fragile to schema evolution — Minor
- [BOT-1] Callback data should not contain raw client string — Minor
- [ERR-1] Stale keyboards need idempotent version-aware actions — Minor
- [ERR-2] Remove must also clean paused_clients — Major
- [TEST-1] Missing canonicalization tests (IP vs IP/32) — Major
- [TEST-2] Missing rollback and lost-update scenario tests — Major
- [COMPAT-1] Missing paused_clients must default to empty array — Major

---

## gemini-executor

- [TYPE-1] Duplicate IP validation vulnerability (IP vs IP/32 normalization) — Major
- [TYPE-2] Stale paused_clients accumulation (phantom entries on stale keyboard) — Minor
- [TYPE-3] Missing AnswerCallback with text (no toast notifications) — Major
- [TYPE-4] Keyboard updates despite apply failure (missing return) — Major
- [TYPE-5] Insufficient duplicate IP test coverage — Suggestion

---

## ccs-executor (GLM direct)

- [ARCH-1] IP format mismatch in paused_clients — Major
- [ARCH-2] Remove needs IP+route binding — High
- [ARCH-3] Pause filters all routes — Low
- [CONC-1] load-modify-save race condition — Major
- [CONC-2] Race in add-flow state check — Major
- [SHELL-1] Test fixture validation — Major
- [SHELL-2] to_entries ordering — Low
- [BOT-1] Callback 64-byte limit — Minor
- [BOT-2] Missing stale callback handling — Minor
- [BOT-3] Stale keyboard handling not detailed — Medium
- [ERR-1] Apply failure leaves inconsistent state — High
- [ERR-2] Missing xray section check — Minor
- [TEST-1] No ClearState reverse tests — Suggestion
- [TEST-2] No all-paused scenario test — Minor
- [TEST-3] No nonexistent route test — Suggestion
- [PLAN-1] File Map completeness — Low
- [PLAN-2] NewRouter signature breakage — Medium

---

## ccs-executor (albb-glm)

- [ARCH-1] IP format mismatch — Medium
- [ARCH-2] Remove needs IP+route binding — High
- [ARCH-3] Pause filters all routes — Low
- [CONC-1] Race condition load-modify-save — High
- [SHELL-1] jq subtraction on nested objects — Medium
- [SHELL-2] to_entries ordering — Low
- [BOT-1] Callback 64-byte limit — Medium
- [BOT-2] ClearState cross-clearing — Low
- [BOT-3] Stale keyboard handling — Medium
- [ERR-1] Apply failure inconsistent state — High
- [TEST-1] No concurrency tests — Medium
- [TEST-2] Missing edge case tests — Low
- [COMPAT-1] Backward compat OK — No issue
- [PLAN-1] File Map completeness — Low
- [PLAN-2] NewRouter signature breakage — Medium

---

## ccs-executor (albb-qwen)

- [ARCHITECTURE-1] IP format inconsistency — Medium
- [CONCURRENCY-1] Race condition load-modify-save — High
- [SHELL-CORRECTNESS-1] Field loss in jq — Medium
- [BOT-INTEGRATION-1] Callback 64-byte limit — High
- [ERROR-HANDLING-1] Apply failure recovery — High
- [EDGE-CASE-1] Stale keyboard state — Medium
- [BACKWARD-COMPATIBILITY-1] Default value handling — Low

---

## ccs-executor (albb-kimi)

- [DATA-1] IP format mismatch — High
- [DATA-2] Non-atomic load-modify-save — Medium
- [SHELL-1] to_entries ordering — Medium
- [BOT-1] Callback 64-byte limit — High
- [BOT-2] In-memory addState lost on restart — Low
- [BOT-3] No rollback on apply failure — Medium
- [ARCH-1] Paused client on re-add — Low
- [ARCH-2] Remove doesn't clean paused_clients from all routes — Low
- [TEST-1] Integration test gaps — Medium
- [SHELL-2] Cross-format jq subtraction — Low
- [UX-1] CIDR display clarity — Low

---

## ccs-executor (albb-minimax)

- [CRITICAL-1] to_entries/from_entries loses tunnel fields — Critical (FALSE POSITIVE)
- [CRITICAL-2] IP format mismatch — Critical
- [HIGH-1] DataDir lacks omitempty — High (FALSE POSITIVE)
- [HIGH-2] Race condition load-modify-save — High
- [MEDIUM-1] Callback 64-byte limit — Medium
- [MEDIUM-2] Route validation should check active state — Medium
- [LOW-1] Apply failure leaves stale config — Low
