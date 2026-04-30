# Kubernaut v1.4 — QE Readiness Audit

**Date**: 2026-04-29
**Scope**: All changes between `release/v1.3` and `main` (origin/main)
**Stats**: 235 commits, 450 files changed, +41,755 / -15,743 lines

---

## 1. Executive Summary

Kubernaut v1.4 ("Operator Overrides and Platform Hardening") introduces 15+ feature areas spanning security hardening, operational tooling, resilience improvements, and API cleanup. The release has strong unit and integration test coverage (+12,889 UT lines, +3,444 IT lines) and targeted E2E additions (+566 lines). Key gaps identified:

| Severity | Count | Summary |
|----------|-------|---------|
| **BLOCKER** | 0 | — |
| **HIGH** | 0 | All 3 resolved (test plans + dry-run E2E created) |
| **MEDIUM** | 1 | HolmesGPT naming residue (80+ files, dedicated PR recommended) |
| **LOW** | 4 | OCP-only features untestable in Kind, doc-only gaps |

---

## 2. Feature Inventory

### 2.1 Shadow Agent — Prompt Injection Guardrails (#601)

**Commits**: 9 | **Priority**: P1 — Flagship security feature

| Artifact | Status | Notes |
|----------|--------|-------|
| Production code | `internal/kubernautagent/alignment/` (6 files), `cmd/kubernautagent/main.go` wiring | Complete |
| ADR | `ADR-KA-001-shadow-agent-alignment-check.md` | **NEW** (created this session) |
| Config guide | `docs/services/kubernaut-agent/shadow-agent-configuration.md` | **NEW** (created this session) |
| Test plan | `docs/tests/601/TEST_PLAN_v2.md` (882 lines) | Complete |
| Unit tests | `test/unit/kubernautagent/alignment/alignment_test.go`, `payload_test.go`, `suite_test.go` | Complete |
| Integration tests | Covered via alignment wiring in KA investigator IT | Indirect |
| E2E tests | `test/e2e/kubernautagent/alignment_e2e_test.go` (193 lines) | **NEW** — Complete |
| Helm values | `kubernautAgent.alignmentCheck` block + schema validation | Complete |
| **QE Verdict** | **PASS** | Full pyramid coverage, E2E with mock-llm shadow scenario |

### 2.2 Dry-Run Mode (#712, #736)

**Commits**: 4 | **Priority**: P1 — Production onboarding critical

| Artifact | Status | Notes |
|----------|--------|-------|
| Production code | `analyzing_handler.go`, `reconciler.go`, `config.go`, CRD types | Complete |
| ADR | `ADR-RO-001-dry-run-mode-ea-policy-decoupling.md` | Complete |
| Test plan | `docs/tests/712/TEST_PLAN.md` | Complete |
| Unit tests | RO handler tests, config validation tests | Complete |
| Integration tests | RO reconciler integration tests | Complete |
| E2E tests | **No dedicated dry-run E2E** | **GAP — HIGH** |
| Helm values | `remediationOrchestrator.dryRun`, `dryRunHoldPeriod` | Complete |
| **QE Verdict** | **CONDITIONAL PASS** | Need E2E or explicit exclusion rationale (config toggle, no CRD interaction) |

**Recommendation**: Create minimal E2E that enables `dryRun: true`, triggers a signal, and asserts RR completes with outcome `DryRun` without WFE creation.

### 2.3 Config Hot-Reload (#835)

**Commits**: 3 (+ log level standardization commits) | **Priority**: P2

| Artifact | Status | Notes |
|----------|--------|-------|
| Production code | `AtomicConfig`, `FileWatcher`, `ReloadCallback` in RO | Complete |
| ADR | None | **GAP — MEDIUM** (DD-INFRA-001 exists for general pattern) |
| Test plan | **Missing** `docs/tests/835/` | **GAP — HIGH** |
| Unit tests | `test(ro): add unit + integration tests for config hot-reload (#835)` | Complete |
| Integration tests | Included in above commit | Complete |
| E2E tests | No dedicated E2E | Acceptable (file-system watcher hard to test in Kind) |
| Helm values | N/A (internal behavior) | N/A |
| **QE Verdict** | **CONDITIONAL PASS** | Test plan should be created for traceability |

### 2.4 TLS Security Profiles (#748)

**Commits**: 6 | **Priority**: P2

| Artifact | Status | Notes |
|----------|--------|-------|
| Production code | `pkg/shared/tls/`, `tlsProfile` on all 10 service configs | Complete |
| ADR | `ADR-TLS-001-openshift-tls-security-profiles.md` | Complete |
| Test plan | **Missing** `docs/tests/748/` | **GAP — MEDIUM** (OCP-specific) |
| Unit tests | TLS profile mapping tests, validation tests | Complete |
| Integration tests | Inter-service TLS tests updated | Complete |
| E2E tests | No dedicated TLS profile E2E | Acceptable (noop on vanilla K8s) |
| Helm values | `tlsProfile` field on all services | Complete |
| **QE Verdict** | **PASS for Kind / CONDITIONAL for OCP** | OCP testing requires Operator integration |

### 2.5 CRD-Aware Engine Registration (#868)

**Commits**: 2 | **Priority**: P2

| Artifact | Status | Notes |
|----------|--------|-------|
| Production code | `pkg/workflowexecution/` CRD discovery + degraded mode | Complete |
| ADR | None (docs commit references authoritative docs updated) | Partial |
| Test plan | **Missing** `docs/tests/868/` | **GAP — MEDIUM** |
| Unit tests | `test/unit/workflowexecution/engine_discovery_test.go` (119 lines) | Complete |
| Integration tests | Existing WE reconciler tests cover engine wiring | Indirect |
| E2E tests | Existing WE E2E exercises Tekton + Job paths | Indirect |
| Helm values | N/A (runtime detection) | N/A |
| **QE Verdict** | **PASS** | Behavior well-covered by existing tests; TP gap is traceability only |

### 2.6 Conversation API Removal (#592)

**Commits**: 23 | **Priority**: P1 — Breaking change (API removal)

| Artifact | Status | Notes |
|----------|--------|-------|
| Production code | Conversation handler, types, routes, DS OpenAPI spec — all removed | Complete |
| Test infrastructure | Conversation test suites and infra removed | Complete |
| Helm | Conversation ConfigMap, NetworkPolicy rule, RBAC removed | Complete |
| Documentation | `docs: remove #592 references from authoritative documentation` | Complete |
| Test plan | `docs/tests/592/TEST_PLAN.md` still marked **Active** | **GAP — MEDIUM** (stale artifact) |
| **QE Verdict** | **CONDITIONAL PASS** | Archive or deprecate `docs/tests/592/TEST_PLAN.md` |

**Recommendation**: Update TP-592 status to `Archived — Feature removed in v1.4 per PROPOSAL-EXT-001`.

### 2.7 OCP Helm Deprecation (#848)

**Commits**: 2 | **Priority**: P3

| Artifact | Status | Notes |
|----------|--------|-------|
| Production code | Deprecation comments in `values-ocp.yaml`, template blocks | Complete |
| ADR | None | **GAP — LOW** (operational, not architectural) |
| Test plan | None | **GAP — LOW** |
| **QE Verdict** | **PASS** | Deprecation-only, no behavioral change |

### 2.8 HolmesGPT Rename to KA (#843)

**Commits**: 6 | **Priority**: P2 — Internal cleanup

| Artifact | Status | Notes |
|----------|--------|-------|
| Production code | Go symbols renamed via gopls, comments updated | Complete |
| Test code | Comments updated | Complete |
| API types / CRD docs | Updated | Complete |
| Residual strings | Some `docs/architecture/` files still reference "HolmesGPT API" | **GAP — LOW** |
| **QE Verdict** | **PASS** | Semantic rename, no behavioral change |

### 2.9 Gateway Security Hardening

**Commits**: ~8 | **Priority**: P1

Includes: trusted proxy RealIP middleware, per-handler K8s API timeout, body limits, generic error responses, header stripping, RBAC, CORS restrictive default.

| Artifact | Status | Notes |
|----------|--------|-------|
| Production code | `pkg/gateway/` security middleware stack | Complete |
| Documentation | `docs(gateway): test plan v1.4 and gateway-security.md hardening guide` | Complete |
| Test plan | Gateway v1.4 test plan created | Complete |
| Unit tests | Gateway middleware tests | Complete |
| Integration tests | Gateway integration tests updated | Complete |
| E2E tests | Error handling test updated | Partial |
| Helm values | CORS, image pinning, schema update | Complete |
| **QE Verdict** | **PASS** |

### 2.10 Resilience: Cache Sync Readiness + HTTP Retry (#852, #853)

**Commits**: 2 | **Priority**: P2

| Artifact | Status | Notes |
|----------|--------|-------|
| Production code | Readiness gate on cache sync, `RetryTransport` | Complete |
| ADR | None (linked to resilience narrative) | Acceptable |
| Test plan | `docs/tests/852/TEST_PLAN.md`, `docs/tests/853/TEST_PLAN.md` | Complete |
| Unit tests | `retry_test.go`, readiness tests | Complete |
| Integration tests | Gateway integration updated | Complete |
| E2E tests | Health/readiness E2E probes `/readyz` | Indirect |
| **QE Verdict** | **PASS** | 503-before-sync semantics hard to test in Kind timing |

### 2.11 Pagination Exemption (#860)

**Commits**: 2 | **Priority**: P2

| Artifact | Status | Notes |
|----------|--------|-------|
| Production code | `anomaly.go` pagination exemption logic | Complete |
| Test plan | `docs/tests/860/TEST_PLAN.md` | Complete |
| Unit tests | `anomaly_test.go` | Complete |
| Integration tests | Investigator IT | Complete |
| E2E tests | Covered by KA E2E investigation scenarios | Indirect |
| **QE Verdict** | **PASS** |

### 2.12 Adversarial Due Diligence & Hierarchy-Aware Target Resolution (#847)

**Commits**: 4 | **Priority**: P2

| Artifact | Status | Notes |
|----------|--------|-------|
| Production code | RCA prompt enhancement, `causal_chain`/`due_diligence` propagation, `sameKindValidationGate` | Complete |
| ADR | None | **GAP — MEDIUM** |
| Test plan | **Missing** `docs/tests/847/` | **GAP — HIGH** |
| Unit tests | Phase 1 propagation tests, audit parity | Complete |
| Integration tests | Investigator IT updated | Complete |
| E2E tests | Adversarial parity E2E (TP-433-ADV lineage) overlaps | Indirect |
| **QE Verdict** | **CONDITIONAL PASS** | Test plan needed for traceability |

### 2.13 KA Log Level Configurable (#875)

**Commits**: 1 | **Priority**: P3

| Artifact | Status | Notes |
|----------|--------|-------|
| Production code | `cmd/kubernautagent/main.go` log level from config file | Complete |
| Test plan | **Missing** `docs/tests/875/` | **GAP — MEDIUM** |
| Unit tests | Config parsing tests | Complete |
| **QE Verdict** | **PASS** | Simple config wiring, low risk |

### 2.14 KA Investigator RBAC Fix (#872)

**Commits**: 1 | **Priority**: P3

| Artifact | Status | Notes |
|----------|--------|-------|
| Production code | ClusterRole RBAC rules added | Complete |
| **QE Verdict** | **PASS** | Helm/RBAC fix, covered by existing E2E |

### 2.15 RO Phase Handler Registry Refactor (#666)

**Commits**: ~8 | **Priority**: P2 — Major refactor

| Artifact | Status | Notes |
|----------|--------|-------|
| Production code | Phase handler interface, registry, extracted handlers | Complete |
| ADR | `ADR-062-phase-handler-registry-pattern.md` | Complete |
| Test plan | `docs/tests/666/TEST_PLAN.md` (650 lines) | Complete |
| Unit tests | Extensive handler tests | Complete |
| Integration tests | RO characterization tests | Complete |
| E2E tests | RO E2E suite | Complete |
| **QE Verdict** | **PASS** |

### 2.16 Notification: PagerDuty & MS Teams (#60, #593)

**Commits**: ~8 | **Priority**: P1

| Artifact | Status | Notes |
|----------|--------|-------|
| Production code | PagerDuty + Teams delivery channels | Complete |
| Test plan | `docs/tests/593/TEST_PLAN.md` | Complete |
| E2E tests | `10_pagerduty_delivery_test.go` (179 lines), `11_teams_delivery_test.go` (181 lines) | **NEW** — Complete |
| **QE Verdict** | **PASS** |

### 2.17 Log Level Standardization (lowercase canonical)

**Commits**: 3 (this session) | **Priority**: P3 — Breaking config change

| Artifact | Status | Notes |
|----------|--------|-------|
| Production code | `internal/config/logging.go`, `pkg/datastorage/config/config.go` — lowercase canonical | Complete |
| Helm charts | All 10 templates updated `INFO` → `info` | Complete |
| Unit tests | Both logging test suites updated | Complete |
| **QE Verdict** | **PASS** | Uppercase still accepted via normalization |

---

## 3. CRD Schema Changes

| CRD | Change | Impact |
|-----|--------|--------|
| `AIAnalysis` | New fields in status (expanded +36 lines) | Non-breaking additive |
| `RemediationApprovalRequest` | New fields (+24 lines) — operator override support | Non-breaking additive |
| `RemediationRequest` | New `DryRun` outcome, new status fields (+14 lines) | Non-breaking additive |
| `WorkflowExecution` | New status fields (+10 lines) | Non-breaking additive |
| `NotificationRequest` | Field removal (-1 line) | **Potentially breaking** — verify no consumers depend on removed field |
| `RemediationWorkflow` | `schemaVersion` field changes (-7 lines) | Schema standard alignment |

---

## 4. Removed / Deprecated Features

| Feature | Status | Migration |
|---------|--------|-----------|
| Conversation API (#592) | **Removed** from `main` | Deferred to v1.5 via MCP/A2A (PROPOSAL-EXT-001) |
| OCP Helm paths (#848) | **Deprecated** | Migrate to Kubernaut Operator; removal in v1.5 |
| HolmesGPT API naming | **Renamed** to KA/AgentClient | No migration needed (internal) |
| `cmd/kubernautagent/llm_builder.go` | **Removed** (234 lines) | Logic moved to `main.go` |
| `pkg/kubernautagent/llm/swappable_client.go` | **Removed** (117 lines) | Replaced by alignment proxy pattern |
| RO deprecated skip handler package (#613) | **Removed** | Replaced by phase handler registry (#666) |
| SDK hotreload test infrastructure (#783) | **Removed** (multiple test files, ~1000+ lines) | Superseded by #835 FileWatcher pattern |

---

## 5. Test Coverage Summary

| Tier | Files Changed | Lines Added | Lines Removed | Net |
|------|--------------|-------------|---------------|-----|
| Unit tests | 79 files | +12,889 | -367 | +12,522 |
| Integration tests | 35 files | +3,444 | -757 | +2,687 |
| E2E tests | 9 files | +566 | -15 | +551 |
| **Total test** | **123 files** | **+16,899** | **-1,139** | **+15,760** |

---

## 6. Gap Summary and Recommendations

### HIGH Priority — RESOLVED

| # | Gap | Feature | Resolution |
|---|-----|---------|------------|
| H1 | ~~Missing test plan~~ | #847 (adversarial due diligence) | **RESOLVED**: Created `docs/tests/847/TEST_PLAN.md` |
| H2 | ~~Missing test plan~~ | #835 (config hot-reload) | **RESOLVED**: Created `docs/tests/835/TEST_PLAN.md` |
| H3 | ~~No dedicated dry-run E2E~~ | #712/#736 | **RESOLVED**: Created `test/e2e/remediationorchestrator/dryrun_e2e_test.go` |

### MEDIUM Priority — RESOLVED

| # | Gap | Feature | Resolution |
|---|-----|---------|------------|
| M1 | ~~Stale test plan~~ | #592 (conversation API removed) | **RESOLVED**: TP-592 status updated to `Archived` |
| M2 | ~~Missing test plan~~ | #868 (CRD-aware engine) | **RESOLVED**: Created `docs/tests/868/TEST_PLAN.md` |
| M3 | ~~Missing test plan~~ | #748 (TLS profiles) | **RESOLVED**: Created `docs/tests/748/TEST_PLAN.md` |
| M4 | ~~Missing test plan~~ | #875 (KA log level) | **RESOLVED**: Created `docs/tests/875/TEST_PLAN.md` |
| M5 | HolmesGPT naming residue | #843 | **PARTIAL**: User-facing DEVELOPER_GUIDE.md updated; 80+ historical docs retain original naming (dedicated rename PR recommended) |

### LOW Priority (deferred)

| # | Gap | Feature | Recommendation |
|---|-----|---------|----------------|
| L1 | No ADR | #835 (config hot-reload) | DD-INFRA-001 exists; consider explicit ADR if pattern is adopted service-wide |
| L2 | No ADR | #847 (adversarial prompts) | Document prompt engineering rationale |
| L3 | OCP E2E gap | #748 (TLS profiles) | Requires OCP + Operator environment |
| L4 | 503-before-sync E2E | #852 (cache readiness) | Timing-dependent, may not be feasible in Kind |

---

## 7. Checklist for QE Sign-Off

- [ ] **H1-H3**: HIGH gaps resolved or documented as accepted risk
- [ ] **M1-M5**: MEDIUM gaps resolved or deferred with rationale
- [ ] CI pipeline green on `main` (all tiers)
- [ ] Helm smoke tests pass in Kind
- [ ] CRD schema changes backward-compatible (no field removals that break existing resources)
- [ ] NotificationRequest field removal validated (no consumer impact)
- [ ] Release notes drafted covering all 17 feature areas
- [ ] Upgrade guide: v1.3 → v1.4 (log level case, #592 removal, OCP Helm deprecation)
- [ ] External repo issues filed:
  - [x] [kubernaut-operator#27](https://github.com/jordigilh/kubernaut-operator/issues/27) — Operator CR integration
  - [x] [kubernaut-docs#130](https://github.com/jordigilh/kubernaut-docs/issues/130) — Documentation updates
