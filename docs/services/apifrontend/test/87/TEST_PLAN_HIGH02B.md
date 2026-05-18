# Test Plan: HIGH-02b Controller-Runtime Wiring for CRDSessionService

**Test Plan Identifier:** TP-AF-HIGH02B-01
**Issue:** #87 (E2E CI Pipeline), kubernaut-operator #97
**Version:** 1.0
**Date:** 2026-05-15
**Predecessor:** TP-AF-GA-P1, TP-AF-056-PW
**Status:** Draft

---

## 1. Introduction

This test plan validates the controller-runtime wiring for `CRDSessionService` and `SessionCleanupReconciler` in kubernaut-apifrontend v1.5.0-rc1. The current implementation uses a fake in-memory Kubernetes client, meaning InvestigationSession CRDs are never persisted to the cluster and the TTL reconciler never runs. The kubernaut-operator team (issue #97) expects AF to manage its own embedded `ctrl.Manager` so that session lifecycle is Kubernetes-native and observable.

### 1.1 Scope

- Wire a real `ctrl.Manager` in `buildSessionInfra()` replacing the fake client
- Add `SetupWithManager` to `SessionCleanupReconciler` per kubernaut pattern
- Verify InvestigationSession CRDs are persisted to the K8s API server on session creation
- Verify TTL reconciler enforces disconnect and retention TTL policies
- Verify `af_sessions_active` gauge reflects real cluster state
- Verify audit events are emitted for all session lifecycle actions
- Fill UT coverage gaps in audit/counter paths of `ttl.go`

### 1.2 Out of Scope

- Leader election for HA AF deployments (v1.6)
- ValidatingAdmissionPolicy for InvestigationSession (operator issue #42)
- Session hydration from CRD on pod restart (documented as PR7)
- A2A protocol integration with session decorator (covered by TP-AF-056-PW)

### 1.3 References

- ARCHITECTURE.md Section 4: CRD Design (InvestigationSession)
- ARCHITECTURE.md Section 7: Observability (`af_sessions_active`)
- ADR-005: Session persistence via InvestigationSession CRD
- kubernaut-operator issue #97: AF Session CRD + TTL controller
- kubernaut-operator issue #42: Install InvestigationSession CRD + ValidatingAdmissionPolicy
- kubernaut `remediationorchestrator` `SetupWithManager` pattern
- kubernaut `TESTING_GUIDELINES` v2.7.0
- 100 Go Mistakes and How to Avoid Them
- IEEE 829-2008 Standard for Software and System Test Documentation
- FedRAMP controls: AU-2/AU-12 (audit events), AU-11 (retention)

### 1.4 Business Requirements and Feature Traceability

| BR ID | Description | Source | v1.5 Feature |
|-------|-------------|--------|--------------|
| BR-SESS-001 | MCP session creation persists InvestigationSession CRD to K8s API server | ARCHITECTURE.md §4 | Session Persistence |
| BR-SESS-002 | CRD spec is immutable; contains remediationRequestRef, a2aTaskID, userIdentity, joinMode | ARCHITECTURE.md §4.1 | Session Metadata |
| BR-SESS-003 | CRD status tracks phase transitions per state machine (Active, Disconnected, Completed, Cancelled, Failed) | ARCHITECTURE.md §4.3 | Session State Machine |
| BR-SESS-004 | TTL controller transitions Disconnected sessions to Cancelled after configurable TTL | ARCHITECTURE.md §4.4 | Disconnect TTL |
| BR-SESS-005 | TTL controller deletes terminal CRDs after retention period (NIST AU-11 >= 30 days) | ARCHITECTURE.md §4.4 | Retention TTL |
| BR-SESS-006 | `af_sessions_active` gauge reflects real session counts by phase | ARCHITECTURE.md §7 | Observability |
| BR-SESS-007 | Session deletion removes the CRD and decrements gauge | ARCHITECTURE.md §4 | Session Cleanup |
| BR-SESS-008 | Audit events emitted for session create, delete, auto-cancel, retention-delete | FedRAMP AU-2/AU-12 | Audit Trail |
| BR-SESS-009 | AF is stateless between requests -- CRD is the only persistent state | ARCHITECTURE.md §1.3 | Stateless Design |
| BR-SESS-010 | Controller-runtime manager runs embedded in AF process; operator installs CRD and RBAC | operator #97 Option B | Operator Integration |

---

## 2. Test Items

| Item | Package(s) | Source Files | BR |
|------|-----------|-------------|-----|
| `SetupWithManager` | `internal/controller` | `ttl.go` | BR-SESS-010 |
| `buildSessionInfra` manager wiring | `cmd/apifrontend` | `main.go` | BR-SESS-010 |
| CRD creation on session create | `internal/session` | `service.go` | BR-SESS-001, BR-SESS-002 |
| CRD deletion on session delete | `internal/session` | `service.go` | BR-SESS-007 |
| TTL disconnect enforcement | `internal/controller` | `ttl.go` | BR-SESS-004 |
| TTL retention enforcement | `internal/controller` | `ttl.go` | BR-SESS-005 |
| Phase label sync | `internal/session` | `statemachine.go` | BR-SESS-003 |
| `af_sessions_active` gauge | `internal/session` | `service.go` | BR-SESS-006 |
| Audit event emission | `internal/controller`, `internal/session` | `ttl.go`, `service.go` | BR-SESS-008 |
| InvestigationSession CRD YAML | `config/crd/bases` | `apifrontend.kubernaut.ai_investigationsessions.yaml` | BR-SESS-010 |

---

## 3. Business Acceptance Criteria

| ID | Criterion | Source | Priority |
|----|-----------|--------|----------|
| BAC-02B-01 | When an MCP session is created, an InvestigationSession CRD exists in the K8s API server with `status.phase=Active` | BR-SESS-001, BR-SESS-003 | P0 |
| BAC-02B-02 | The CRD spec fields match the CreateConfig: `remediationRequestRef`, `a2aTaskID`, `userIdentity.username`, `userIdentity.groups`, `joinMode` | BR-SESS-002 | P0 |
| BAC-02B-03 | The CRD label `apifrontend.kubernaut.ai/phase` matches `status.phase` for queryability | BR-SESS-003 | P0 |
| BAC-02B-04 | When a session is deleted, the CRD is removed from the API server; subsequent Get returns NotFound | BR-SESS-007 | P0 |
| BAC-02B-05 | A Disconnected session is automatically transitioned to Cancelled after the configured `disconnectTTL` | BR-SESS-004 | P0 |
| BAC-02B-06 | A terminal session CRD is deleted after the configured `retentionTTL` (clamped to >= 30 days per AU-11) | BR-SESS-005 | P0 |
| BAC-02B-07 | `af_sessions_active{phase="Active"}` increments by 1 when a session is created | BR-SESS-006 | P0 |
| BAC-02B-08 | `af_sessions_active{phase="Active"}` decrements by 1 when a session is deleted | BR-SESS-006 | P0 |
| BAC-02B-09 | After TTL auto-cancel, `af_sessions_active` gauge adjusts: Active -1, Cancelled +1 | BR-SESS-006 | P0 |
| BAC-02B-10 | Audit emitter receives `SessionCreated` event with `session_id` and `user` on create | BR-SESS-008 | P0 |
| BAC-02B-11 | Audit emitter receives `SessionAutoCancelled` event when disconnect TTL expires | BR-SESS-008 | P0 |
| BAC-02B-12 | Audit emitter receives `SessionRetentionDeleted` event when retention TTL expires | BR-SESS-008 | P1 |
| BAC-02B-13 | 5 concurrent Create calls produce 5 distinct CRDs with unique names | BR-SESS-001 | P0 |
| BAC-02B-14 | Non-RFC-1123 session ID is rejected with descriptive error before CRD creation | BR-SESS-002 | P1 |
| BAC-02B-15 | `SessionCleanupReconciler` registers with a `ctrl.Manager` via `SetupWithManager` | BR-SESS-010 | P0 |
| BAC-02B-16 | TTL disconnect handler increments `af_session_ttl_actions_total{action="cancel"}` counter | BR-SESS-008 | P1 |
| BAC-02B-17 | TTL retention handler increments `af_session_ttl_actions_total{action="delete"}` counter | BR-SESS-008 | P1 |

---

## 4. Approach

### 4.1 TDD Methodology

RED -> GREEN -> REFACTOR for each test category. Tests are written first to define expected business outcomes. Implementation follows to make tests pass. Refactor validates code quality against 100-go-mistakes.

**TDD validates business outcomes, not implementation logic:**
- Tests assert observable state changes (CRD exists, gauge value, audit event received)
- Tests do NOT assert internal method calls, struct field assignments, or code paths
- Tests use real K8s API interactions (envtest/Kind), not mocked clients in IT/E2E
- Tests verify the "what" (business behavior), not the "how" (implementation mechanics)

### 4.2 Anti-Patterns Avoided

| Anti-Pattern | Avoidance Strategy |
|-------------|-------------------|
| `time.Sleep` for timing assertions | Use short TTLs (1s) with `Eventually` + generous timeout |
| Shared mutable state between specs | Each `It` creates its own client, reconciler, and service |
| Testing implementation details | Assert CRD state in API server, not internal struct fields |
| Ignoring errors | All `err` returns are asserted with `Expect(err)` |
| Test helpers that hide assertions | Helpers build fixtures; specs contain all assertions |
| Flaky concurrent tests | Use sync barriers and `Eventually` with deterministic triggers |
| Overspecified error messages | Assert error category (`ContainSubstring`), not exact text |

### 4.3 Coverage Target

- **UT tier**: `internal/controller` + `internal/session` >= 80% statement coverage
- **IT tier**: >= 80% of IT-testable code in `internal/controller` + `internal/session`
- Verified via `make coverage-report` per-tier methodology

---

## 5. Test Cases

### 5.1 UT: SetupWithManager and TTL Coverage Gaps

**Intent:** Fill existing UT coverage gaps in `ttl.go` where audit emission and Prometheus counter paths are never exercised (currently `nil` auditor/counter in all specs). Add `SetupWithManager` unit test.

| TC ID | BAC | Description | Input | Expected Outcome | Type |
|-------|-----|-------------|-------|-----------------|------|
| UT-AF-220-013 | BAC-02B-15 | SetupWithManager registers controller for InvestigationSession kind | envtest manager + reconciler | `SetupWithManager` returns nil error; manager recognizes InvestigationSession watches | UT |
| UT-AF-220-014 | BAC-02B-11, BAC-02B-16 | TTL disconnect handler emits audit event and increments cancel counter | Disconnected session past TTL, non-nil auditor + counter | Auditor receives `SessionAutoCancelled` event with session name; `af_session_ttl_actions_total{action="cancel"}` == 1 | UT |
| UT-AF-220-015 | BAC-02B-12, BAC-02B-17 | TTL retention handler emits audit event and increments delete counter | Terminal session past retention, non-nil auditor + counter | Auditor receives `SessionRetentionDeleted` event; `af_session_ttl_actions_total{action="delete"}` == 1 | UT |

### 5.2 UT: buildSessionInfra Manager Wiring

**Intent:** Verify that `buildSessionInfra` correctly creates the session infrastructure components and that the manager fallback works when no kubeconfig is available (unit test environment).

| TC ID | BAC | Description | Input | Expected Outcome | Type |
|-------|-----|-------------|-------|-----------------|------|
| UT-AF-220-016 | BAC-02B-15 | buildSessionInfra returns non-nil SessionService when no kubeconfig | Config with valid session params, no KUBECONFIG | `sessInfra.SessionService` is non-nil; warning logged | UT |
| UT-AF-220-017 | BAC-02B-15 | buildSessionInfra returns non-nil Reconciler | Config with valid session params | `sessInfra.Reconciler` is non-nil | UT |
| UT-AF-220-018 | BAC-02B-15 | StopFunc is callable and does not panic | Call StopFunc after build | No panic; function returns | UT |
| UT-AF-220-019 | BAC-02B-15 | Scheme recognizes InvestigationSession GVK | Inspect returned scheme | `scheme.Recognizes(apifrontend.kubernaut.ai/v1alpha1 InvestigationSession)` is true | UT |

---

### 5.3 IT: Session CRD Lifecycle (envtest)

**Intent:** Verify that when AF's `CRDSessionService` creates, queries, and deletes sessions through a real Kubernetes API server (envtest), the InvestigationSession CRDs are persisted correctly and the business-level session contract is honored.

**Prerequisites:** Register InvestigationSession CRD YAML in envtest `SynchronizedBeforeSuite`. Start a `ctrl.Manager` with `SessionCleanupReconciler` registered for the IT suite.

| TC ID | BAC | Description | Input | Expected Outcome | Type |
|-------|-----|-------------|-------|-----------------|------|
| IT-SESS-001 | BAC-02B-01 | Session create persists CRD to API server | `CRDSessionService.Create` with valid CreateConfig | `client.Get` for InvestigationSession returns the resource; `status.phase == Active` | IT |
| IT-SESS-002 | BAC-02B-02 | CRD spec fields match CreateConfig input | CreateConfig with specific userIdentity, a2aTaskID, remediationRef, joinMode | Retrieved CRD `spec.userIdentity.username`, `spec.a2aTaskID`, `spec.remediationRequestRef.name`, `spec.joinMode` all match input | IT |
| IT-SESS-003 | BAC-02B-03 | Phase label matches status.phase on creation | Freshly created session | CRD label `apifrontend.kubernaut.ai/phase` == `"Active"` | IT |
| IT-SESS-004 | BAC-02B-04 | Session delete removes CRD from API server | Create then Delete session | `client.Get` returns `IsNotFound` error | IT |

### 5.4 IT: TTL Reconciler Behavioral Contracts (envtest)

**Intent:** Verify that the TTL reconciler enforces the business-level TTL policies -- disconnected sessions are auto-cancelled and terminal sessions are garbage-collected -- through the real K8s API server, not through mocked method calls.

| TC ID | BAC | Description | Input | Expected Outcome | Type |
|-------|-----|-------------|-------|-----------------|------|
| IT-SESS-005 | BAC-02B-05 | Disconnected session auto-cancels after TTL | Session with `disconnectTTL=1s`, phase set to Disconnected with `disconnectedAt` in past | `Eventually` CRD `status.phase == Cancelled` within 5s; `status.message` contains "auto-cancelled" | IT |
| IT-SESS-006 | BAC-02B-06 | Terminal session deleted after retention TTL | Completed session with `completedAt` far in past, `retentionTTL=MinRetentionTTL` | `Eventually` `client.Get` returns `IsNotFound` within 5s | IT |

### 5.5 IT: Gauge Observability (envtest)

**Intent:** Verify that the `af_sessions_active` Prometheus gauge accurately reflects the number of active sessions as observed through the Kubernetes API -- the gauge is the business contract for operators to monitor session health.

| TC ID | BAC | Description | Input | Expected Outcome | Type |
|-------|-----|-------------|-------|-----------------|------|
| IT-SESS-007 | BAC-02B-07, BAC-02B-08 | Gauge increments on create and decrements on delete | Create session, read gauge, delete session, read gauge | Gauge `{phase="Active"}` goes from 0 -> 1 -> 0 | IT |
| IT-SESS-008 | BAC-02B-09 | Gauge adjusts on TTL auto-cancel | Session created (Active gauge=1), transitioned to Disconnected, TTL expires | `Eventually` Active gauge == 0, Cancelled gauge >= 1 | IT |

### 5.6 IT: Audit Trail Fidelity (envtest)

**Intent:** Verify that security-relevant session lifecycle events produce audit records that satisfy FedRAMP AU-2/AU-12. The audit trail is the business contract for compliance -- every session action must be traceable.

| TC ID | BAC | Description | Input | Expected Outcome | Type |
|-------|-----|-------------|-------|-----------------|------|
| IT-SESS-009 | BAC-02B-10 | SessionCreated audit event on create | Create session with known userID | Audit emitter receives event with `Type==SessionCreated`, `UserID` matches, `Detail["session_id"]` present | IT |
| IT-SESS-010 | BAC-02B-11 | SessionAutoCancelled audit event on TTL expiry | Disconnected session past TTL | Audit emitter receives event with `Type==SessionAutoCancelled`, `Detail["session"]` matches CRD name | IT |

### 5.7 IT: Robustness and Concurrency (envtest)

**Intent:** Verify that the session infrastructure handles concurrent access and invalid input gracefully -- concurrent session creation must not produce conflicts or lost CRDs, and invalid input must be rejected early with clear errors.

| TC ID | BAC | Description | Input | Expected Outcome | Type |
|-------|-----|-------------|-------|-----------------|------|
| IT-SESS-011 | BAC-02B-13 | Concurrent session creation produces distinct CRDs | 5 goroutines each call `Create` with unique sessionIDs | All 5 succeed; `client.List` returns 5 InvestigationSession resources with distinct names | IT |
| IT-SESS-012 | BAC-02B-14 | Non-RFC-1123 session ID rejected | SessionID = `"UPPER_CASE_ID"` | Error returned containing "invalid session ID" or "RFC 1123"; no CRD created in API server | IT |

### 5.8 E2E: Session CRD Visibility in Kind Cluster

**Intent:** Verify end-to-end that when a user interacts with AF through MCP, the resulting InvestigationSession CRD is visible to cluster operators via standard `kubectl` queries -- this is the contract the operator team depends on.

**Prerequisites:** Apply InvestigationSession CRD YAML to Kind cluster during E2E setup.

| TC ID | BAC | Description | Input | Expected Outcome | Type |
|-------|-----|-------------|-------|-----------------|------|
| TC-E2E-SESS-001 | BAC-02B-01, BAC-02B-02, BAC-02B-03 | InvestigationSession CRD visible after MCP session | MCP `initialize` + tool call flow | `kubectl get investigationsessions -n kubernaut-system` returns >= 1 CRD with `phase=Active` and populated user identity | E2E |
| TC-E2E-SESS-002 | BAC-02B-07 | Metrics endpoint reports active sessions during session | Active MCP session | `/metrics` contains `af_sessions_active{phase="Active"} >= 1` | E2E |

---

## 6. Test Environment

### UT
- Go 1.26+, Ginkgo v2/Gomega
- `sigs.k8s.io/controller-runtime/pkg/client/fake` for bare unit tests
- `go test -race -count=1`
- No external dependencies

### IT
- envtest (etcd + kube-apiserver from `setup-envtest`)
- CRDs: 9 kubernaut + 1 apifrontend InvestigationSession
- Real `ctrl.Manager` started in `SynchronizedBeforeSuite`
- Podman containers: PostgreSQL, Redis, DataStorage, Mock LLM, Kubernaut Agent
- `KUBEBUILDER_ASSETS` set from `setup-envtest use -p path`

### E2E
- Kind cluster with InvestigationSession CRD applied
- AF binary deployed as pod
- `kubectl` for CRD verification

---

## 7. Schedule (TDD Phases)

| Phase | Description | Specs | TDD Stage |
|-------|-------------|-------|-----------|
| Phase 1 | SetupWithManager + UT coverage gaps | UT-AF-220-013..015 | RED -> GREEN |
| Phase 2 | buildSessionInfra manager wiring | UT-AF-220-016..019 | RED -> GREEN |
| Phase 3 | IT CRD lifecycle | IT-SESS-001..004 | RED -> GREEN |
| Phase 4 | IT TTL reconciler contracts | IT-SESS-005..006 | RED -> GREEN |
| Phase 5 | IT gauge + audit fidelity | IT-SESS-007..010 | RED -> GREEN |
| Phase 6 | IT robustness + concurrency | IT-SESS-011..012 | RED -> GREEN |
| Phase 7 | E2E session visibility | TC-E2E-SESS-001..002 | RED -> GREEN |
| Phase 8 | Kustomize CRD inclusion | — | GREEN |
| REFACTOR | 100-go-mistakes audit + lint + coverage verification | All | REFACTOR |

---

## 8. Risks

| Risk | Impact | Mitigation |
|------|--------|-----------|
| envtest startup latency for `ctrl.Manager` | IT suite takes longer | Manager started once in `SynchronizedBeforeSuite`; shared across specs |
| TTL timing tests may be flaky in CI | IT-SESS-005/006/008 fail intermittently | Use 1s disconnect TTL + `Eventually` with 10s timeout; no `time.Sleep` |
| InvestigationSession CRD not installed in Kind | E2E specs fail on CRD not found | E2E setup explicitly applies CRD YAML before test run |
| Manager port conflicts | `ctrl.Manager` binds metrics/health ports | Set `MetricsBindAddress: "0"` and `HealthProbeBindAddress: ""` to disable |
| Fake client in UT does not match real API server behavior | UT passes but IT fails | UT tests construction/wiring only; behavioral assurance comes from IT/E2E with real API server |

---

## 9. Traceability Matrix

### 9.1 BR -> TC Mapping

| BR ID | TCs Covering | Count |
|-------|-------------|-------|
| BR-SESS-001 | IT-SESS-001, IT-SESS-011, TC-E2E-SESS-001 | 3 |
| BR-SESS-002 | IT-SESS-002, IT-SESS-012, TC-E2E-SESS-001 | 3 |
| BR-SESS-003 | IT-SESS-003, TC-E2E-SESS-001 | 2 |
| BR-SESS-004 | UT-AF-220-014, IT-SESS-005 | 2 |
| BR-SESS-005 | UT-AF-220-015, IT-SESS-006 | 2 |
| BR-SESS-006 | IT-SESS-007, IT-SESS-008, TC-E2E-SESS-002 | 3 |
| BR-SESS-007 | IT-SESS-004, IT-SESS-007 | 2 |
| BR-SESS-008 | UT-AF-220-014, UT-AF-220-015, IT-SESS-009, IT-SESS-010 | 4 |
| BR-SESS-009 | IT-SESS-001, IT-SESS-004 (CRD is the state) | 2 |
| BR-SESS-010 | UT-AF-220-013, UT-AF-220-016..019 | 5 |

### 9.2 TC -> Implementation File Mapping

| TC ID | Test File | Spec Description |
|-------|-----------|------------------|
| UT-AF-220-013 | `internal/controller/ttl_test.go` | SetupWithManager registers controller |
| UT-AF-220-014 | `internal/controller/ttl_test.go` | TTL disconnect emits audit + counter |
| UT-AF-220-015 | `internal/controller/ttl_test.go` | TTL retention emits audit + counter |
| UT-AF-220-016 | `cmd/apifrontend/main_wiring_test.go` | buildSessionInfra returns non-nil service (no kubeconfig) |
| UT-AF-220-017 | `cmd/apifrontend/main_wiring_test.go` | buildSessionInfra returns non-nil reconciler |
| UT-AF-220-018 | `cmd/apifrontend/main_wiring_test.go` | StopFunc callable without panic |
| UT-AF-220-019 | `cmd/apifrontend/main_wiring_test.go` | Scheme recognizes InvestigationSession |
| IT-SESS-001 | `test/integration/session_test.go` | Session create persists CRD |
| IT-SESS-002 | `test/integration/session_test.go` | CRD spec fields match input |
| IT-SESS-003 | `test/integration/session_test.go` | Phase label matches status |
| IT-SESS-004 | `test/integration/session_test.go` | Session delete removes CRD |
| IT-SESS-005 | `test/integration/session_test.go` | Disconnected auto-cancel after TTL |
| IT-SESS-006 | `test/integration/session_test.go` | Terminal CRD deleted after retention |
| IT-SESS-007 | `test/integration/session_test.go` | Gauge increments/decrements on create/delete |
| IT-SESS-008 | `test/integration/session_test.go` | Gauge adjusts on TTL auto-cancel |
| IT-SESS-009 | `test/integration/session_test.go` | SessionCreated audit event |
| IT-SESS-010 | `test/integration/session_test.go` | SessionAutoCancelled audit event |
| IT-SESS-011 | `test/integration/session_test.go` | Concurrent create produces distinct CRDs |
| IT-SESS-012 | `test/integration/session_test.go` | Non-RFC-1123 ID rejected |
| TC-E2E-SESS-001 | `test/e2e/session_test.go` | CRD visible in Kind after MCP session |
| TC-E2E-SESS-002 | `test/e2e/session_test.go` | Metrics reports active sessions |

---

## 10. TDD Refactor: 100 Go Mistakes Audit

*To be populated during REFACTOR phase. Will audit:*
- `internal/controller/ttl.go` (new `SetupWithManager`)
- `cmd/apifrontend/main.go` (new manager wiring in `buildSessionInfra`)

---

## 11. Execution Results

*To be populated after implementation.*

| Phase | Specs | Status | Duration | Coverage |
|-------|-------|--------|----------|----------|
| 1 (UT SetupWithManager) | 3 | | | |
| 2 (UT buildSessionInfra) | 4 | | | |
| 3 (IT CRD lifecycle) | 4 | | | |
| 4 (IT TTL contracts) | 2 | | | |
| 5 (IT gauge + audit) | 4 | | | |
| 6 (IT robustness) | 2 | | | |
| 7 (E2E session) | 2 | | | |
| **Total** | **21** | | | **>= 80%** |

---

## 12. Checkpoint Audit

*GA readiness audit to be performed after all phases complete.*

| # | Dimension | Score | Notes |
|---|-----------|-------|-------|
| 1 | Correctness | | |
| 2 | Coverage | | |
| 3 | Lint Compliance | | |
| 4 | Security (AppSec) | | |
| 5 | API Contract | | |
| 6 | Observability | | |
| 7 | Resilience | | |
| 8 | FedRAMP Alignment | | |
| 9 | Test Quality (QE) | | |
| 10 | Test Documentation | | |
| 11 | Regression Safety | | |
| 12 | Product Acceptance | | |
| 13 | UX / DX | | |
| 14 | Product Security | | |
| **Average** | | | |
| **Confidence** | | | |
