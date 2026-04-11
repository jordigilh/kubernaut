# Test Plan: Per-Tier Coverage to >=80% Across All Services

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-668-v4.1
**Feature**: Raise per-tier test coverage to >=80% across all services
**Version**: 4.1
**Created**: 2026-04-11
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.3_part4`

---

## 1. Introduction

### 1.1 Purpose

This test plan defines the strategy to close the gap between current per-tier coverage and the >=80% mandate in TESTING_GUIDELINES.md v2.7.0. CI run #24268106203 showed that no service currently meets this threshold. This plan provides a systematic, service-by-service approach with prioritized test scenarios that deliver behavioral assurance against business acceptance criteria.

### 1.2 Objectives

1. **Achieve >=80% UT coverage** on unit-testable code for all 11 services
2. **Achieve >=80% IT coverage** on integration-testable code for all 10 services (shared-packages excluded)
3. **Zero anti-pattern violations** in all new tests (time.Sleep, Skip, ToNot(BeNil) sole assertion, context.Background for long-running ops, direct audit infra testing)
4. **Two-tier minimum** for every business requirement
5. **All tests use Ginkgo/Gomega BDD** with proper test ID naming convention

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| UT coverage per service | >=80% | `scripts/coverage/coverage_report.py --service X --tier unit` |
| IT coverage per service | >=80% | `scripts/coverage/coverage_report.py --service X --tier integration` |
| Anti-pattern violations | 0 | Pre-commit hook + `make lint-test-patterns` |
| Test pass rate | 100% | `make test-unit-{service}` / `make test-integration-{service}` |
| Backward compatibility | 0 regressions | Existing tests pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- Issue #668: Raise per-tier test coverage to >=80% across all services
- [TESTING_GUIDELINES.md v2.7.0](../../development/business-requirements/TESTING_GUIDELINES.md): Per-tier coverage mandate
- [03-testing-strategy.mdc](../../../.cursor/rules/03-testing-strategy.mdc): Defense-in-depth testing approach
- `scripts/coverage/coverage_report.py`: Authoritative coverage measurement tool

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- CI Run #24268106203 — baseline coverage data

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Generated code (ogen-client) inflates denominator | Coverage numbers appear lower than actual testable code | Medium | All DS tests | ogen-client/ is excluded in `GO_SERVICE_CONFIG.unit_exclude`; verify exclusion is correct |
| R2 | IT-testable code (handlers, repos) cannot be meaningfully unit-tested | Writing UT for I/O code produces brittle mocks | High | DS, GW, NT server packages | Respect code partitioning: UT for pure logic, IT for I/O. Do not mock databases in UT |
| R3 | Large function count in `pkg/datastorage/server` (154 funcs) | Effort estimate exceeds sprint capacity | Medium | DS IT suite | Prioritize by handler traffic volume; batch-test similar CRUD handlers |
| R4 | Reconciler code (`internal/controller/`) needs envtest | Slow test execution, flaky timing | Medium | NT, WE, RO, EM, SP IT suites | Use Eventually() with reasonable timeouts; avoid time.Sleep |
| R5 | Cross-service shared types at 4.7% | Shared type bugs propagate to all services | High | All services | Prioritize `pkg/shared/types` UT early in execution |
| R6 | Anti-pattern regression in new tests | CI pre-commit hook catches violations | Low | All new tests | Run `make lint-test-patterns` before each commit |
| R7 | Generated `zz_generated.deepcopy.go` inflates shared-packages UT denominator | 40 DeepCopy methods (~19% of 212 funcs) inflate denominator; 80% unreachable without 40 trivial tests | High | Shared-packages UT | **Phase 0**: Exclude `zz_generated*.go` from `coverpkg` or add to `GENERATED_CODE_PATTERNS` |
| R8 | `pkg/datastorage/partition/` not in `DATASTORAGE_COVERPKG` | Partition tests produce no coverage data | High | DS UT-DS-668-008 | **Phase 0**: Add `partition/` to `DATASTORAGE_COVERPKG` in Makefile |
| R9 | STATIC DATA anti-pattern check is line-local | `Equal("literal")` without `BR-` on the same line triggers violation even if BR is in Describe/It | Medium | All new tests | Use variables for expected values in table entries, or add `// BR-*` on `Equal("...")` lines |
| R10 | `audit/ds_store.go` not in KA `int_include` | IT tests for buffered audit store will not move KA IT coverage % | Medium | KA IT | Reclassified to UT scope (Phase 2e); functions ARE in UT tier |
| R11 | AW `RemediationRequestStatusHandler` not in webhook manifests | `IT-AW-668-002` requires extending `config/webhook/manifests.yaml` and suite registration | Medium | AW IT | Infra prerequisite documented in Phase 4i |
| R12 | Existing anti-pattern violations (~156 NULL-TESTING, ~2714 STATIC DATA candidates) | `make lint-test-patterns` exits non-zero on ANY violation across `test/`; will fail even if new tests are compliant | Low | All new tests | **Confirmed safe**: `check-test-anti-patterns.sh` is NOT in CI workflows or pre-commit hooks. New compliant tests coexist with existing violations. CI uses `golangci-lint` with `only-new-issues: true` + `continue-on-error: true`. |
| R13 | `config/webhook/manifests.yaml` drift beyond RemediationRequest | Helm charts define 2 additional validating webhooks (RemediationWorkflow, ActionType) not in envtest manifests | Low | AW IT | Phase 0 fixes the immediate RR blocker. RW/AT validators are Helm-only and not needed for IT-AW-668 scenarios. |
| R14 | SP Node enrichment in envtest | Component tests note "Node enrichment moved to E2E tier -- ENVTEST does not provide real nodes" | Low | IT-SP-668-001 | Synthetic Node API objects work in envtest for label/annotation enrichment. Only node-level system metrics are unavailable. DaemonSet/ReplicaSet have no such limitation. |

### 3.1 Risk-to-Test Traceability

- **R1**: Verified by checking `GO_SERVICE_CONFIG["datastorage"]["unit_exclude"]` includes `ogen-client/`
- **R2**: Mitigated by strict adherence to code partitioning in Section 5.1
- **R5**: Mitigated by Phase 1 execution order (shared-packages first)
- **R7**: Mitigated by Phase 0 config change (exclude generated code from denominator)
- **R8**: Mitigated by Phase 0 Makefile update
- **R9**: Mitigated by anti-pattern compliance conventions in Section 5.6
- **R10**: Confirmed via `coverage_report.py` regex analysis — `audit/ds_store.go` does not match `int_include`
- **R11**: Documented as infra prerequisite for IT-AW-668-002
- **R12**: Confirmed `check-test-anti-patterns.sh` is not wired into GitHub Actions CI; new tests can be compliant without fixing legacy debt
- **R13**: Helm-only validators (RW/AT) are outside IT-AW-668 scope; Phase 0 RR fix is sufficient
- **R14**: Synthetic Node objects in envtest support enricher label/annotation tests; node-level system behavior deferred to E2E

---

## 4. Scope

### 4.1 Features to be Tested

Services ordered by recommended execution priority (smallest gap first):

| # | Service | Current UT | Current IT | UT Gap | IT Gap | Scope |
|---|---------|-----------|-----------|--------|--------|-------|
| 1 | signalprocessing | 75.5% | 69.3% | 4.5% | 10.7% | UT + IT |
| 2 | aianalysis | 78.5% | 62.2% | 1.5% | 17.8% | UT + IT |
| 3 | notification | 58.2% | 63.1% | 21.8% | 16.9% | UT + IT |
| 4 | remediationorchestrator | 72.0% | 60.7% | 8.0% | 19.3% | UT + IT |
| 5 | effectivenessmonitor | 68.5% | 74.8% | 11.5% | 5.2% | UT + IT |
| 6 | workflowexecution | 65.3% | 46.3% | 14.7% | 33.7% | UT + IT |
| 7 | kubernautagent | 65.9% | 51.5% | 14.1% | 28.5% | UT + IT |
| 8 | gateway | 50.8% | 56.0% | 29.2% | 24.0% | UT + IT |
| 9 | authwebhook | 70.7% | 23.7% | 9.3% | 56.3% | UT + IT |
| 10 | datastorage | 35.5% | 33.5% | 44.5% | 46.5% | UT + IT |
| 11 | shared-packages | 61.6% | N/A | 18.4% | N/A | UT only |

### 4.2 Features Not to be Tested

- **E2E tier**: E2E coverage (3-12%) is measured via binary profiling in Kind and is not actionable at the function level. E2E validates user journeys, not per-function coverage. Deferred to a separate E2E scenario expansion issue.
- **Generated code**: `ogen-client/`, `mocks/` directories are excluded from coverage measurement per `GO_SERVICE_CONFIG`.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Order by smallest gap first | Quick wins build momentum and validate process before tackling large services |
| Respect `GO_SERVICE_CONFIG` partitioning | Coverage measurement uses this config; tests must target the correct tier |
| No new UT for I/O code | Pure logic gets UT, I/O code gets IT. Mocking databases in UT produces brittle tests |
| Batch similar handlers | DS has 154 server funcs; group by handler family (audit, workflow, effectiveness, DLQ) |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: TESTING_GUIDELINES.md v2.7.0 — Per-Tier Testable Code Coverage.

Code partitioning per `scripts/coverage/coverage_report.py` `GO_SERVICE_CONFIG`:

- **Unit-testable** (`unit_exclude` removes I/O): Config, validation, scoring, parsing, builders, types, engines
- **Integration-testable** (`int_include` selects I/O): Handlers, repositories, adapters, DLQ, controllers, reconcilers, clients

Coverage targets:

- **Unit**: >=80% of unit-testable code
- **Integration**: >=80% of integration-testable code
- **E2E**: Out of scope (see 4.2)

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers. Each service has both UT and IT test suites (except shared-packages which is UT-only).

### 5.3 Business Outcome Quality Bar

Tests validate business outcomes, not just code paths. Each test answers: "what does the user/operator/system get?" Examples:
- "When a malformed event is submitted, the API returns 400 with a structured error body" (not "the validation function returns false")
- "When two concurrent batches target the same correlation ID, both complete without deadlock" (not "the advisory lock SQL executes")

### 5.4 Pass/Fail Criteria

**PASS** — all of the following:
1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. Per-tier code coverage meets >=80% threshold per `coverage_report.py`
4. No regressions in existing test suites
5. Zero anti-pattern violations detected by pre-commit hook

**FAIL** — any of the following:
1. Any P0 test fails
2. Per-tier coverage falls below 80% on any tier for any service
3. Existing tests that were passing now fail (regression)
4. Anti-pattern violation detected in new test code

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:
- Build is broken (code does not compile)
- PostgreSQL/Redis infrastructure cannot be provisioned for IT
- More than 3 tests fail for the same root cause (stop and investigate)
- Blocking dependency from another issue not yet merged

**Resume testing when**:
- Build fixed and green on CI
- Infrastructure restored
- Root cause identified and fix deployed

### 5.6 Anti-Pattern Compliance Conventions

All new tests must pass `make lint-test-patterns` (`scripts/validation/check-test-anti-patterns.sh`). The checker enforces 4 rules:

**NULL-TESTING**: Never use `ToNot(BeNil())` or `ToNot(BeEmpty())` as assertions. For UUIDs, use `MatchRegexp("^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$")` or inject a known UUID and assert `Equal(knownUUID)`. For timestamps, use `BeTemporally("~", time.Now(), 2*time.Second)`.

**STATIC DATA**: The checker is **line-local** — `Equal("literal")` on any line without `BR-` on that **same line** triggers a violation. BR references in `Describe`/`It` on other lines do NOT suppress this. Mitigation patterns:
- Assign expected values to variables: `want := "workflow.started"; Expect(got).To(Equal(want))`
- Or add Gomega message: `Expect(got).To(Equal("workflow.started"), "BR-STORAGE-001: event type") // BR-STORAGE-001`

**LIBRARY TESTING**: `logrus.New()` is flagged on **any line** in test files (no `Expect` required on the same line). `context.WithValue(...) ... Expect(...)` and `os.Setenv(...) ... Expect(...)` are flagged when both substrings appear on the **same line**. `prometheus.NewPedanticRegistry()` is safe.

**MISSING BR**: Every `_test.go` file must contain the substring `BR-` at least once. Place `BR-*` in `Describe` or `It` text.

### 5.7 TDD Compliance Enforcement

`scripts/validation/check-tdd-compliance.sh` enforces additional rules (run via `make lint-tdd-compliance`):

**NON-BDD (hard fail)**: Any `*_test.go` file with `func Test...(*testing.T)` and no `Describe|Context|It` triggers exit 1. All new tests MUST use Ginkgo/Gomega BDD framework.

**BR FORMAT (warning)**: Files missing `BR-[A-Z]*-[0-9]*` pattern produce a warning. Unlike `lint-test-patterns` which only requires substring `BR-`, TDD compliance expects fully-formed BR references (e.g., `BR-GATEWAY-001`).

**DIRECT MOCKS (warning)**: Lines matching `mock.*:=.*\.New` (unless containing `Factory` or `type Mock`) produce a warning.

---

## 6. Test Items

### 6.1 Unit-Testable Code (per `GO_SERVICE_CONFIG.unit_exclude`)

| Service | Packages (unit-testable) | Current UT | Target |
|---------|--------------------------|------------|--------|
| signalprocessing | `pkg/signalprocessing/` excluding `audit|cache|enricher|handler|status` | 75.5% | >=80% |
| aianalysis | `pkg/aianalysis/` excluding `handler.go|audit/` | 78.5% | >=80% |
| notification | `pkg/notification/` excluding `client.go|delivery/|phase/|status/` | 58.2% | >=80% |
| remediationorchestrator | `pkg/remediationorchestrator/` excluding `creator|handler/(aianalysis|signalprocessing|workflowexecution)|aggregator|status` | 72.0% | >=80% |
| effectivenessmonitor | `pkg/effectivenessmonitor/` excluding `client|status|reconciler` | 68.5% | >=80% |
| workflowexecution | `pkg/workflowexecution/` excluding `audit|status` | 65.3% | >=80% |
| kubernautagent | `pkg/kubernautagent/` + `internal/kubernautagent/` excluding multiple I/O files | 65.9% | >=80% |
| gateway | `pkg/gateway/` excluding `server.go|k8s/|processing/(crd_creator|distributed_lock|status_updater)` | 50.8% | >=80% |
| authwebhook | `pkg/authwebhook/` excluding handler files | 70.7% | >=80% |
| datastorage | `pkg/datastorage/` excluding `server/|repository/|dlq/|ogen-client/|mocks/|adapter/|query/service.go|reconstruction/query.go` | 35.5% | >=80% |
| shared-packages | `pkg/shared/`, `pkg/audit/`, `pkg/cache/`, `pkg/http/`, `pkg/k8sutil/` | 61.6% | >=80% |

### 6.2 Integration-Testable Code (per `GO_SERVICE_CONFIG.int_include`)

| Service | Packages (IT-testable) | Current IT | Target |
|---------|------------------------|------------|--------|
| signalprocessing | `audit|cache|enricher|handler|status` + `internal/controller/signalprocessing/` | 69.3% | >=80% |
| aianalysis | `handler.go|audit/` + `internal/controller/aianalysis/` | 62.2% | >=80% |
| notification | `client.go|delivery/|phase/|status/` + `internal/controller/notification/` | 63.1% | >=80% |
| remediationorchestrator | `creator|handler/*|aggregator|status` + `internal/controller/remediationorchestrator/` | 60.7% | >=80% |
| effectivenessmonitor | `client|status|reconciler` + `internal/controller/effectivenessmonitor/` | 74.8% | >=80% |
| workflowexecution | `audit|status` + `internal/controller/workflowexecution/` | 46.3% | >=80% |
| kubernautagent | `enricher.go|investigator.go|handler.go|manager.go|tools/*.go|client.go` | 51.5% | >=80% |
| gateway | `server.go|k8s/|processing/(crd_creator|distributed_lock|status_updater)` | 56.0% | >=80% |
| authwebhook | Handler files (5 handlers) | 23.7% | >=80% |
| datastorage | `server/|repository/|dlq/|adapter/|query/service.go|reconstruction/query.go` | 33.5% | >=80% |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.3_part4` HEAD | Post-#667 deadlock fixes |
| Coverage baseline | CI run #24268106203 | Reference measurement |
| Coverage tool | `scripts/coverage/coverage_report.py` | Authoritative measurement |

---

## 7. BR Coverage Matrix

This issue is cross-cutting — it strengthens coverage for existing BRs across all services. The matrix maps the **specific service-level BRs** whose code is currently below 80% coverage to the test groups that will close the gap.

### 7.1 Service-to-BR Mapping

| Service | Primary BR Families (from existing tests & architecture docs) | Test Group |
|---------|--------------------------------------------------------------|------------|
| signalprocessing | BR-SP-001–106, BR-SP-110–111, BR-AUDIT-001/002 | UT/IT-SP-668-* |
| aianalysis | BR-AI-001–030, BR-AI-050–090, BR-HAPI-191/197/200, BR-AUDIT-005 | UT/IT-AA-668-* |
| notification | BR-NOT-051–069, BR-NOT-080–083, BR-NOT-104, BR-HAPI-200 | UT/IT-NT-668-* |
| remediationorchestrator | BR-ORCH-025–050, BR-RO-103, BR-EM-001/004, BR-SCOPE-010 | UT/IT-RO-668-* |
| effectivenessmonitor | BR-EM-001–012, BR-PLATFORM-452, BR-AUDIT-006 | UT/IT-EM-668-* |
| workflowexecution | BR-WE-001–017, BR-AUDIT-005, BR-WORKFLOW-004 | UT/IT-WE-668-* |
| kubernautagent | BR-HAPI-016/211/433, BR-TESTING-001 | UT/IT-KA-668-* |
| gateway | BR-GATEWAY-001–114, BR-GATEWAY-181–190, BR-SCOPE-002 | UT/IT-GW-668-* |
| authwebhook | BR-AUTH-001, BR-WE-013, BR-WORKFLOW-006/007 | UT/IT-AW-668-* |
| datastorage | BR-STORAGE-001–043, BR-WORKFLOW-001–007, BR-AUDIT-001–029, BR-SOC2-001–003 | UT/IT-DS-668-* |
| shared-packages | BR-WE-006/009/012, BR-NOT-051/052/055, BR-SP-072, BR-SCOPE-001 | UT-SH-668-* |

### 7.2 Cross-Cutting Quality BRs

| BR ID | Description | Priority | Tier | Test Group | Status |
|-------|-------------|----------|------|------------|--------|
| BR-QUALITY-001 | >=80% UT coverage on unit-testable code per service | P0 | Unit | UT-{SVC}-668-* | Pending |
| BR-QUALITY-002 | >=80% IT coverage on integration-testable code per service | P0 | Integration | IT-{SVC}-668-* | Pending |
| BR-QUALITY-003 | Zero anti-pattern violations in test code | P0 | All | All new tests | Pending |
| BR-QUALITY-004 | Two-tier minimum for every business requirement | P1 | All | Cross-tier mapping | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-668-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `SP` (SignalProcessing), `AA` (AIAnalysis), `NT` (Notification), `RO` (RemediationOrchestrator), `EM` (EffectivenessMonitor), `WE` (WorkflowExecution), `KA` (KubernautAgent), `GW` (Gateway), `AW` (AuthWebhook), `DS` (DataStorage), `SH` (Shared)

---

### Phase 1: Quick Wins (UT gap <10%)

**Estimated LOE**: 0.5–1 day (2 services, ~8 functions)

#### 1a. SignalProcessing (UT 75.5% -> >=80%)

**Unit-testable scope**: `pkg/signalprocessing/` excluding `audit|cache|enricher|handler|status`

**Zero-coverage function targets** (from `go tool cover -func`):
- `pkg/signalprocessing/evaluator/evaluator.go:145` — `StartHotReload` (UT-testable: `evaluator/` does NOT match `int_include` pattern `/(audit|cache|enricher|handler|status)/`; function counts toward UT tier only despite filesystem I/O)
- `pkg/signalprocessing/evaluator/evaluator.go:165` — `Stop` (UT-testable: nil-safe check)
- `pkg/signalprocessing/metrics/metrics.go:75` — `NewMetrics` (UT-testable: targets global registry path; `NewMetricsWithRegistry` already covered by existing `metrics_test.go`)
- `pkg/signalprocessing/status/manager.go:111` — `UpdatePhase` (IT-testable, not UT)
- `pkg/signalprocessing/status/manager.go:147` — `isTerminalPhase` (IT-testable, not UT)

| ID | BR | Business Outcome Under Test | Phase |
|----|----|-----------------------------|-------|
| `UT-SP-668-001` | BR-SP-001 | Signal deduplication logic correctly identifies duplicate fingerprints within the configured time window | Pending |
| `UT-SP-668-002` | BR-SP-072 | Signal grouping algorithm produces correct groupings when signals share the same resource/namespace | Pending |
| `UT-SP-668-003` | BR-SP-070 | Signal priority calculation returns correct urgency levels for critical vs warning severity | Pending |
| `UT-SP-668-004` | BR-SP-051 | Config parser rejects invalid deduplication windows and returns structured errors | Pending |
| `UT-SP-668-005` | BR-SP-100 | Signal metadata extraction correctly parses all Prometheus alert label formats | Pending |

#### 1b. AIAnalysis (UT 78.5% -> >=80%)

**Unit-testable scope**: `pkg/aianalysis/` excluding `handler.go|audit/`

**Zero-coverage function targets** (from `go tool cover -func`):
- `pkg/aianalysis/handlers/analyzing.go:68` — `WithConfidenceThreshold`
- `pkg/aianalysis/handlers/generated_helpers.go:48,125,151,161` — `GetOptNilStringValue`, `GetMapFromMapSafe`, `GetMapFromJxRaw`, `convertMapToStringMap`
- `pkg/aianalysis/handlers/investigating.go:88` — `WithSessionPollInterval`

| ID | BR | Business Outcome Under Test | Phase |
|----|----|-----------------------------|-------|
| `UT-AA-668-001` | BR-AI-001 | Analysis request builder correctly constructs prompts from signal metadata and context | Pending |
| `UT-AA-668-002` | BR-AI-008 | Analysis response parser handles malformed LLM responses gracefully with structured errors | Pending |
| `UT-AA-668-003` | BR-AI-011 | Confidence scoring algorithm produces correct scores for varying evidence quality levels | Pending |

---

### Phase 2: Moderate Gaps (UT gap 10-20%)

**Estimated LOE**: 3–5 days (5 services, ~33 functions)

#### 2a. Notification (UT 58.2% -> >=80%)

**Unit-testable scope**: `pkg/notification/` excluding `client.go|delivery/|phase/|status/`

Packages needing most work: `pkg/notification/credentials` (45.3%), `pkg/notification/config` (45.7%), `pkg/notification/enrichment` (57.1%), `pkg/notification/` root (70.7%)

**Zero-coverage function targets** (from `go tool cover -func`):
- `pkg/notification/client.go:73–179` — `NewClient`, `Create`, `Get`, `List`, `Update`, `Delete`, `UpdateStatus` (IT-testable)
- `pkg/notification/conditions.go:65` — `SetReady`
- `pkg/notification/config/config.go:112,132` — `LoadFromFile`, `LoadFromEnv`
- `internal/controller/notification/notificationrequest_controller.go:355,713,740` — `handleNotFound`, `cleanupAuditEventTracking`, `SetupWithManager` (IT-testable)
- `internal/controller/notification/retry_circuit_breaker_handler.go:118,159,181` — `getMaxAttemptCount`, `calculateBackoffWithPolicy`, `isSlackCircuitBreakerOpen` (IT-testable: controller paths are excluded from UT tier by `coverage_report.py`)
- `internal/controller/notification/routing_handler.go:123–281` — `formatAttributesForCondition`, `formatChannelsForCondition`, `receiverToChannels`, `handleConfigMapChange`, `loadRoutingConfigFromCluster`, `rebuildSlackDeliveryServices` (IT-testable)

| ID | BR | Business Outcome Under Test | Phase |
|----|----|-----------------------------|-------|
| `UT-NT-668-001` | BR-NOT-104 | Credential resolver correctly loads and validates webhook credentials from Secret references | Pending |
| `UT-NT-668-002` | BR-NOT-104 | Credential resolver returns structured error when referenced Secret is missing or malformed | Pending |
| `UT-NT-668-003` | BR-NOT-051 | Config parser validates all notification channel types (webhook, email, slack) with correct defaults | Pending |
| `UT-NT-668-004` | BR-NOT-052 | Config parser rejects invalid retry policies and returns structured errors | Pending |
| `UT-NT-668-005` | BR-NOT-060 | Enrichment engine correctly merges signal metadata into notification templates | Pending |
| `UT-NT-668-006` | BR-NOT-060 | Enrichment engine handles missing template variables gracefully without panicking | Pending |
| `UT-NT-668-007` | BR-NOT-055 | Route matching correctly evaluates label selectors including matchRe patterns | Pending |
| `UT-NT-668-008` | BR-NOT-055 | Route matching handles continue=true fanout by returning all matching receivers | Pending |
| `UT-NT-668-009` | BR-NOT-063 | Notification formatter produces correct JSON payload for each channel type | Pending |
| `UT-NT-668-010` | BR-NOT-080 | Metrics collector correctly increments counters for sent/failed/retried notifications | Pending |

#### 2b. RemediationOrchestrator (UT 72.0% -> >=80%)

**Unit-testable scope**: `pkg/remediationorchestrator/` excluding `creator|handler/*|aggregator|status`

**Zero-coverage function targets** (from `go tool cover -func`):
- `pkg/remediationorchestrator/audit/manager.go:748` — `BuildEACreatedEvent`
- `pkg/remediationorchestrator/creator/effectivenessassessment.go:72` — `StabilizationWindow` (IT-testable: `creator/` matches `unit_exclude`)
- `pkg/remediationorchestrator/creator/owner_resolver.go:117` — `extractLabelsViaUnstructured` (IT-testable: `creator/` matches `unit_exclude`)
- `pkg/remediationorchestrator/handler/aianalysis.go:543` — `HandleRemediationTargetMissing` (IT-testable)
- `pkg/remediationorchestrator/metrics/metrics.go:87,319` — `NewMetrics`, `RecordApprovalDecision`
- `internal/controller/remediationorchestrator/blocking.go:152–380` — `handleBlockedPhase`, `transitionToFailedTerminal`, `handleUnmanagedResourceExpiry`, `recheckResourceBusyBlock`, `recheckDuplicateBlock`, `clearEventBasedBlock` (IT-testable)
- `internal/controller/remediationorchestrator/reconciler.go:280,406,2693,2962` — `SetDSClient`, `getSafeDefaultTimeouts`, `emitCompletionAudit`, `SetupWithManager` (IT-testable)

| ID | BR | Business Outcome Under Test | Phase |
|----|----|-----------------------------|-------|
| `UT-RO-668-001` | BR-ORCH-025 | Workflow selection algorithm picks the highest-priority matching workflow for a given signal | Pending |
| `UT-RO-668-002` | BR-ORCH-026 | Workflow selection returns no-match when no workflows match the signal's labels | Pending |
| `UT-RO-668-003` | BR-ORCH-031 | Remediation config parser validates all required fields and returns structured errors | Pending |
| `UT-RO-668-004` | BR-ORCH-034 | Remediation phase engine correctly transitions through pending -> running -> completed states | Pending |
| `UT-RO-668-005` | BR-ORCH-042 | Approval policy evaluator correctly handles auto-approve, manual-approve, and deny outcomes | Pending |

#### 2c. EffectivenessMonitor (UT 68.5% -> >=80%)

**Unit-testable scope**: `pkg/effectivenessmonitor/` excluding `client|status|reconciler`

**Zero-coverage function targets** (from `go tool cover -func`):
- `pkg/effectivenessmonitor/audit/manager.go:136,487,556` — `RecordComponentAssessed`, `RecordHashComputed`, `RecordAssessmentScheduled` (UT-testable: `audit/` does NOT match `int_include` pattern `/(client|status|reconciler)/`)
- `pkg/effectivenessmonitor/client/alertmanager_health.go` — health check functions (IT-testable)
- `internal/controller/effectivenessmonitor/events.go:144` — `emitMetricsEvent` (IT-testable)
- `internal/controller/effectivenessmonitor/reconciler.go:205,237` — `SetupWithManager`, `SetRESTMapper` (IT-testable)

| ID | BR | Business Outcome Under Test | Phase |
|----|----|-----------------------------|-------|
| `UT-EM-668-001` | BR-EM-001 | Health scoring algorithm produces correct scores for all alert state combinations | Pending |
| `UT-EM-668-002` | BR-EM-004 | Validity window calculator correctly determines whether a remediation is within its monitoring period | Pending |
| `UT-EM-668-003` | BR-EM-009 | Metric comparison logic correctly identifies improved vs degraded vs unchanged states | Pending |
| `UT-EM-668-004` | BR-EM-010 | Config parser validates monitoring intervals and returns structured errors for invalid values | Pending |
| `UT-EM-668-005` | BR-EM-012 | Alert resolution detector correctly identifies resolved alerts from Prometheus query results | Pending |

#### 2d. WorkflowExecution (UT 65.3% -> >=80%)

**Unit-testable scope**: `pkg/workflowexecution/` excluding `audit|status`

Packages needing most work: `pkg/workflowexecution/phase` (55.2%), `pkg/workflowexecution/metrics` (60.0%), `pkg/workflowexecution/config` (66.7%), `pkg/workflowexecution/client` (71.9%)

**Zero-coverage function targets** (from `go tool cover -func`):
- `pkg/workflowexecution/executor/awx_client.go:121,144,260,410` — `CancelJob`, `FindJobTemplateByName`, `FindCredentialTypeByKind`, `GetJobTemplateCredentials` (UT-testable: `executor/` does NOT match `unit_exclude` pattern `/(audit|status)/`; counts toward UT tier only)
- `pkg/workflowexecution/metrics/metrics.go:97,170` — `NewMetrics`, `Register`
- `pkg/workflowexecution/phase/types.go:77,131` — `IsTerminal`, `Validate`
- `pkg/workflowexecution/status/manager.go:86,117` — `UpdatePhase`, `isTerminalPhase` (IT-testable)

| ID | BR | Business Outcome Under Test | Phase |
|----|----|-----------------------------|-------|
| `UT-WE-668-001` | BR-WE-001 | Phase transition engine correctly validates legal state transitions and rejects illegal ones | Pending |
| `UT-WE-668-002` | BR-WE-003 | Phase timeout calculator correctly computes deadlines based on workflow step configuration | Pending |
| `UT-WE-668-003` | BR-WE-005 | Metrics collector correctly records execution duration and outcome for each workflow step | Pending |
| `UT-WE-668-004` | BR-WE-009 | Config parser validates all executor types (Tekton, AWX, script) with correct defaults | Pending |
| `UT-WE-668-005` | BR-WE-014 | Client builder correctly constructs HTTP clients with configured timeouts and TLS settings | Pending |
| `UT-WE-668-006` | BR-WE-011 | DAG executor correctly topologically sorts steps and identifies parallelizable groups | Pending |
| `UT-WE-668-007` | BR-WE-012 | DAG executor correctly handles step failure with configurable continue-on-error semantics | Pending |

#### 2e. KubernautAgent (UT 65.9% -> >=80%)

**Unit-testable scope**: `pkg/kubernautagent/` + `internal/kubernautagent/` excluding I/O files

**Zero-coverage function targets** (from `go tool cover -func`):
- `internal/kubernautagent/audit/ds_store.go:437–531` — `validHumanReviewReason`, `WithFlushInterval`, `WithBufferSize`, `WithBatchSize`, `NewBufferedDSAuditStore`, `StoreAudit`, `Flush`, `Close` (all UT-testable: `audit/ds_store.go` is NOT in `unit_exclude` and NOT in `int_include` — these functions count toward UT tier only, not IT tier)
- `internal/kubernautagent/audit/nop.go:25` — `StoreAudit`
- `internal/kubernautagent/enrichment/enricher.go:150` — `WithLabelDetector` (IT-testable: `enrichment/enricher.go:` matches `unit_exclude`)
- `internal/kubernautagent/enrichment/k8s_adapter.go:106` — `GetSpecHash`
- `internal/kubernautagent/investigator/anomaly.go:110` — `TotalExceeded`
- `internal/kubernautagent/investigator/investigator.go:113–499` — `Investigate`, `backfillSeverity`, `resolveEnrichment`, `runRCA`, `runWorkflowSelection`, `enrichFromCatalog`, `runLLMLoop`, `totalPromptLength`, `lastUserMessage`, `truncatePreview`, `toolNames`, `toolDefinitionsForPhase`, `executeTool` (mostly IT-testable)

| ID | BR | Business Outcome Under Test | Phase |
|----|----|-----------------------------|-------|
| `UT-KA-668-001` | BR-HAPI-433 | Prompt builder correctly constructs investigation prompts from signal context and tool results | Pending |
| `UT-KA-668-002` | BR-HAPI-211 | Parser correctly extracts structured RCA from LLM response including root cause, evidence, and confidence | Pending |
| `UT-KA-668-003` | BR-HAPI-433 | Config parser validates all agent settings (model, temperature, max_tokens) with correct defaults | Pending |
| `UT-KA-668-004` | BR-HAPI-433 | Tool registry correctly resolves available tools based on cluster capabilities | Pending |
| `UT-KA-668-005` | BR-HAPI-016 | Audit event builder correctly constructs investigation audit trail with all required fields | Pending |
| `UT-KA-668-006` | BR-TESTING-001 | Structured output parser handles partial/malformed LLM JSON responses gracefully | Pending |

---

### Phase 3: Large Gaps (UT gap >20%)

**Estimated LOE**: 12–20 days (4 services; DS is disproportionately large)

**Per-service LOE breakdown**:
- Gateway UT: ~2–3 days (audit helpers, middleware, config — mostly pure functions)
- AuthWebhook UT: ~1–2 days (enum converters, payload builders, reconciler — mostly trivial)
- **DataStorage UT: ~10–15 days** (199 UT-testable functions: models 49, audit 62, validation 30, schema 24, reconstruction 13, partition 8, config 7, metrics 6; mix of trivial/non-trivial/complex; batch table-driven tests for trivial funcs)
- Shared Packages UT: ~2–3 days (effective gap only ~4.5% after DeepCopy exclusion; ~8 functions to cover)

#### 3a. Gateway (UT 50.8% -> >=80%)

**Unit-testable scope**: `pkg/gateway/` excluding `server.go|k8s/|processing/(crd_creator|distributed_lock|status_updater)`

Packages needing most work: `pkg/gateway` root (62.0%, 162 funcs), `pkg/gateway/config` (67.6%), `pkg/gateway/middleware` (68.3%)

**Zero-coverage function targets** (from `go tool cover -func`):
- `pkg/gateway/adapters/kubernetes_event_adapter.go:107,130` — `SetLogger`, `ReplayValidator` (UT-testable: `adapters/` does NOT match `int_include` pattern)
- `pkg/gateway/adapters/prometheus_adapter.go:100` — `ReplayValidator` (UT-testable: same reason)
- `pkg/gateway/audit_helpers.go:74–125` — `toGatewayAuditPayloadSignalType`, `toGatewayAuditPayloadSeverity`, `toGatewayAuditPayloadDeduplicationStatus`, `convertMapToJxRaw`, `toAPIErrorDetails`
- `pkg/gateway/config/config.go:285` — `LoadFromEnv`
- `pkg/gateway/k8s/client.go:104,128` — `UpdateRemediationRequest`, `ListRemediationRequestsByFingerprint` (IT-testable)
- `pkg/gateway/k8s/client_with_circuit_breaker.go:141,159,178` — `UpdateRemediationRequest`, `GetRemediationRequest`, `State` (IT-testable)
- `pkg/gateway/metrics/metrics.go:90` — `NewMetrics`
- `pkg/gateway/middleware/alertmanager_freshness.go:64` — `AlertManagerFreshnessValidator`
- `pkg/gateway/middleware/event_freshness.go:58,114` — `EventFreshnessValidator`, `respondFreshnessError`
- `pkg/gateway/middleware/http_metrics.go:91` — `InFlightRequests`
- `pkg/gateway/processing/clock.go:90` — `Set`
- `pkg/gateway/processing/errors.go:142,159,175` — `NewDeduplicationError`, `Error` (2x)
- `pkg/gateway/server.go:202,225` — `NewServer`, `NewServerWithK8sClient` (IT-testable)

| ID | BR | Business Outcome Under Test | Phase |
|----|----|-----------------------------|-------|
| `UT-GW-668-001` | BR-GATEWAY-001 | Signal validation correctly rejects payloads missing required fields (fingerprint, signal_type) | Pending |
| `UT-GW-668-002` | BR-GATEWAY-003 | Signal validation correctly accepts all supported signal types (alert, event, metric) | Pending |
| `UT-GW-668-003` | BR-GATEWAY-010 | Config parser validates all gateway settings (port, TLS, dedup window) with correct defaults | Pending |
| `UT-GW-668-004` | BR-GATEWAY-015 | Middleware chain correctly applies rate limiting, auth, and request logging in order | Pending |
| `UT-GW-668-005` | BR-GATEWAY-004 | Signal adapter correctly transforms Prometheus AlertManager webhook payload to internal format | Pending |
| `UT-GW-668-006` | BR-GATEWAY-004 | Signal adapter correctly transforms generic JSON webhook payload to internal format | Pending |
| `UT-GW-668-007` | BR-GATEWAY-008 | Signal deduplication correctly identifies duplicates by fingerprint within configured window | Pending |
| `UT-GW-668-008` | BR-GATEWAY-009 | Signal batching correctly groups signals by resource for downstream processing | Pending |
| `UT-GW-668-009` | BR-GATEWAY-190 | TLS config builder correctly constructs TLS settings from cert/key paths and CA bundle | Pending |
| `UT-GW-668-010` | BR-GATEWAY-100 | Type conversion functions correctly handle all edge cases (nil, empty, overflow) | Pending |

#### 3b. AuthWebhook (UT 70.7% -> >=80%)

**Unit-testable scope**: `pkg/authwebhook/` excluding handler files

**Zero-coverage function targets** (from `go tool cover -func`):
- `pkg/authwebhook/audit_enum_converters.go:31–43` — `toNotificationAuditPayloadType`, `toNotificationAuditPayloadPriority`, `toNotificationAuditPayloadNotificationType`, `toNotificationAuditPayloadFinalStatus`
- `pkg/authwebhook/audit_payload_builder.go:60` — `BuildRARApprovalAuditEvent`
- `pkg/authwebhook/ds_client.go:53,278` — `NewDSClientAdapter`, `ForceDisableActionType`
- `pkg/authwebhook/notificationrequest_handler.go:51,60,125` — `NewNotificationRequestDeleteHandler`, `Handle`, `InjectDecoder` (IT-testable)
- `pkg/authwebhook/notificationrequest_validator.go:54,63,70,82` — `NewNotificationRequestValidator`, `ValidateCreate`, `ValidateUpdate`, `ValidateDelete` (IT-testable)
- `pkg/authwebhook/rw_reconciler.go:187` — `SetupWithManager` (UT-only: `rw_reconciler.go` NOT in the five-file `int_include` list)
- `pkg/authwebhook/startup_reconciler.go:63` — `NeedLeaderElection` (UT-only: `startup_reconciler.go` NOT in `int_include`)

| ID | BR | Business Outcome Under Test | Phase |
|----|----|-----------------------------|-------|
| `UT-AW-668-001` | BR-AUTH-001 | Request validation correctly rejects CRDs with missing required fields | Pending |
| `UT-AW-668-002` | BR-WORKFLOW-006 | Request validation correctly applies defaulting rules for optional fields | Pending |
| `UT-AW-668-003` | BR-WORKFLOW-007 | Config parser validates all webhook settings with correct defaults | Pending |
| `UT-AW-668-004` | BR-WE-013 | Approval policy evaluator correctly handles all CRD types (notification, remediation, workflow) | Pending |

#### 3c. DataStorage (UT 35.5% -> >=80%)

**Unit-testable scope**: `pkg/datastorage/` excluding `server/|repository/|dlq/|ogen-client/|mocks/|adapter/|query/service.go|reconstruction/query.go`

Packages needing most work: `pkg/datastorage/models` (42.1%, 47 funcs), `pkg/datastorage/schema` (63.3%, 24 funcs), `pkg/datastorage/reconstruction` (69.7%, 15 funcs)

**Zero-coverage function targets** (from `go tool cover -func`):
- `pkg/datastorage/adapter/aggregations.go:36–231` — `AggregateSuccessRate`, `AggregateByNamespace`, `AggregateBySeverity`, `AggregateIncidentTrend` (IT-testable)
- `pkg/datastorage/adapter/db_adapter.go:40,52,225,291` — `NewDBAdapter`, `Query`, `CountTotal`, `Get` (IT-testable)
- `pkg/datastorage/adapter/utils.go:22,33` — `convertPlaceholdersToPostgreSQL`, `replaceFirstOccurrence` (IT-testable: `adapter/` matches `unit_exclude`)
- `pkg/datastorage/audit/workflow_catalog_event.go:60,132` — `NewWorkflowCreatedAuditEvent`, `NewWorkflowUpdatedAuditEvent`
- `pkg/datastorage/config/config.go:338,350,364` — `GetReadTimeout`, `GetWriteTimeout` (UT-testable), `GetConnectionString` (already covered by existing `config_test.go` UT-DS-040-M1-003)
- `pkg/datastorage/dlq/client.go:226–487` — `HealthCheck`, `ReadMessages`, `AckMessage`, `MoveToDeadLetter`, `IncrementRetryCount`, `GetPendingMessages`, `isConsumerGroupExistsError`, `isNoSuchKeyError`, `isNoGroupError` (IT-testable)
- `pkg/datastorage/metrics/metrics.go:155` — `NewMetrics`

| ID | BR | Business Outcome Under Test | Phase |
|----|----|-----------------------------|-------|
| `UT-DS-668-001` | BR-STORAGE-001 | Model validation correctly rejects events with missing required fields and returns structured errors | Pending |
| `UT-DS-668-002` | BR-STORAGE-002 | Model serialization/deserialization round-trips preserve all field values including timestamps | Pending |
| `UT-DS-668-003` | BR-STORAGE-006 | DateOnly type correctly serializes to YYYY-MM-DD format and deserializes back | Pending |
| `UT-DS-668-004` | BR-STORAGE-017 | Schema migration generator produces correct DDL for all table definitions | Pending |
| `UT-DS-668-005` | BR-AUDIT-001 | Reconstruction engine correctly rebuilds audit trail from exported data | Pending |
| `UT-DS-668-006` | BR-AUDIT-004 | Hash chain calculator produces deterministic hashes for identical event data | Pending |
| `UT-DS-668-007` | BR-STORAGE-028 | Config parser validates all DS settings (database, server, retention) with correct defaults | Pending |
| `UT-DS-668-008` | BR-STORAGE-040 | Partition spec generator produces correct partition names and date ranges for all months | Pending |
| `UT-DS-668-009` | BR-STORAGE-033 | Metrics collector correctly records event counts, batch sizes, and latency histograms | Pending |
| `UT-DS-668-010` | BR-STORAGE-007 | Workflow label builders correctly construct MandatoryLabels and CustomLabels from input | Pending |

> **Infra prerequisite for UT-DS-668-008**: `pkg/datastorage/partition/` must be added to `DATASTORAGE_COVERPKG` in Makefile.
>
> **Dropped**: Original UT-DS-668-009 (SortedCorrelationIDs) — function lives in `repository/` which is `unit_exclude`. Replaced with UT-DS-668-010 targeting `models/workflow_labels.go` (17 functions, in UT scope).
>
> **Moved to IT**: Original UT-DS-668-010 renumbered to UT-DS-668-009. `convertPlaceholdersToPostgreSQL`/`replaceFirstOccurrence` from `adapter/utils.go` added to IT-DS-668 scope (adapter/ is `unit_exclude`).

#### 3d. Shared Packages (UT 61.6% -> >=80%)

**Unit-testable scope**: `pkg/shared/`, `pkg/audit/`, `pkg/cache/`, `pkg/http/`, `pkg/k8sutil/`

**Effective denominator analysis** (Phase 0 prerequisite):
- Total functions in scope: **217** (audit 57, cache 10, http 7, k8sutil 2, shared 141)
- `zz_generated.deepcopy.go` functions to exclude: **40** (20 DeepCopyInto + 20 DeepCopy)
- Effective denominator after Phase 0 exclusion: **177**
- Projected coverage after exclusion (same covered count): **~75.5%** (up from 61.6%)
- Remaining gap to 80%: **~4.5%** (~8 additional functions need coverage)

Packages needing most work (post-exclusion): `pkg/shared/auth` (56.2%, 14 funcs), `pkg/http/cors` (67.0%, 7 funcs), `pkg/audit` (71.8%, 57 funcs), `pkg/shared/types` (only 3 non-generated funcs: `StructuredDescription.String`, `OgenDescriptionToShared`, `SharedDescriptionToOgen`)

**Zero-coverage function targets** (from `go tool cover -func`):
- `pkg/audit/event.go:180,193` — `NewAuditEvent`, `Validate`
- `pkg/audit/event_data.go:57,73,81,95,106` — `NewEventData`, `WithSourcePayload`, `ToJSON`, `FromJSON`, `Validate`
- `pkg/audit/helpers.go:84,89,94,131,142,176` — `SetClusterName`, `SetDuration`, `SetSeverity`, `SetEventDataFromEnvelope`, `EnvelopeToMap`, `StructToMap`
- `pkg/audit/store.go:271` — `Flush`
- `pkg/cache/redis/config.go:87,131` — `ToGoRedisOptions`, `DefaultOptions` (IT-testable)
- `pkg/http/cors/cors.go:165,314` — `ProductionOptions`, `parseMaxAge`
- `pkg/k8sutil/client.go:88` — `NewClientset` (IT-testable)
- `pkg/shared/auth/middleware.go:93,113,194,202` — `NewMiddleware`, `Handler`, `GetUserFromContext`, `writeError`
- `pkg/shared/auth/transport.go:106` — `NewServiceAccountTransport`
- `pkg/shared/conditions/conditions.go:71` — `SetWithGeneration`

| ID | BR | Business Outcome Under Test | Phase |
|----|----|-----------------------------|-------|
| `UT-SH-668-001` | BR-WE-006 | Shared type conversion functions correctly handle all edge cases (nil, empty, zero values) | Pending |
| `UT-SH-668-002` | BR-NOT-051 | Shared type validation functions correctly reject invalid values with structured errors | Pending |
| `UT-SH-668-003` | BR-AUTH-001 | Auth token parser correctly extracts and validates JWT claims from bearer tokens | Pending |
| `UT-SH-668-004` | BR-AUTH-001 | Auth RBAC evaluator correctly enforces role-based access for all resource types | Pending |
| `UT-SH-668-005` | BR-SCOPE-001 | CORS middleware correctly handles preflight requests with configured allowed origins | Pending |
| `UT-SH-668-006` | BR-SCOPE-001 | CORS middleware correctly rejects requests from non-allowed origins | Pending |
| `UT-SH-668-007` | BR-AUDIT-001 | Audit event builder correctly constructs events for all event types with required fields | Pending |
| `UT-SH-668-008` | BR-AUDIT-001 | Audit store buffering correctly batches events and flushes at configured intervals | Pending |
| `UT-SH-668-009` | BR-GATEWAY-008 | Redis cache correctly handles TTL expiration and returns cache-miss for expired keys | Pending |
| `UT-SH-668-010` | BR-SP-072 | K8s utility functions correctly handle namespace resolution and label selector parsing | Pending |

---

### Phase 4: Integration Tests (IT gap to >=80%)

**Estimated LOE**: 15–25 days (10 services; DS is disproportionately large)

**Per-service LOE breakdown** (approximate):
- SP IT: ~1 day (3 enricher arms + degraded validators)
- EM IT: ~1 day (3 scenarios, small gap)
- NT IT: ~2 days (5 scenarios, controller + delivery paths)
- AA IT: ~1–2 days (4 scenarios)
- RO IT: ~2 days (4 scenarios + creator functions)
- GW IT: ~2 days (4 scenarios, k8s client + server)
- KA IT: ~2–3 days (4 scenarios, investigator orchestration)
- WE IT: ~2 days (4 scenarios, controller lifecycle)
- **DS IT: ~8–12 days** (10 scenarios; 37 HTTP handlers, 106 repository funcs, 10 adapter funcs, 21 DLQ funcs; existing harness with 220 ITs helps but gap from 33.5% is large)
- AW IT: ~2 days (5 scenarios, envtest + webhook infra)

IT tests are organized by service, targeting the `int_include` code partition. Each IT uses real infrastructure (PostgreSQL, httptest, envtest) with zero mocks per the No-Mocks Policy.

#### 4a0. SignalProcessing IT (69.3% -> >=80%)

**IT-testable zero-coverage targets** (per `int_include` regex `/(audit|cache|enricher|handler|status)/` + `internal/controller/signalprocessing/`):
- `pkg/signalprocessing/enricher/k8s_enricher.go` — `enrichDaemonSetSignal`, `enrichReplicaSetSignal`, `enrichNodeSignal`, `enrichNamespaceOnly` (enricher arms not exercised by existing IT which covers Pod/Deployment/StatefulSet/Service only)
- `pkg/signalprocessing/enricher/degraded.go` — `ValidateContextSize`, `validateLabels` (no direct IT coverage)
- `pkg/signalprocessing/audit/manager.go` — narrow branches on `RecordEnrichmentComplete` (idempotency), `RecordError` (fatal vs non-fatal)
- `pkg/signalprocessing/status/manager.go` — `UpdatePhase`, `isTerminalPhase`

> **Note**: `handler/enriching.go` has NO function declarations (stub file). `SetupWithManager` is already exercised by `suite_test.go`. Existing IT (~11 test files) covers reconciler happy path, Pod/Deployment/StatefulSet/Service enrichment, Rego evaluation, metrics, severity, and signal mode. The gap is in **enricher switch arms** for uncommon resource kinds and **degraded context validators**.

| ID | BR | Business Outcome Under Test | Phase |
|----|----|-----------------------------|-------|
| `IT-SP-668-001` | BR-SP-001 | K8s enricher correctly enriches DaemonSet, ReplicaSet, and Node signal kinds via envtest | Pending |
| `IT-SP-668-002` | BR-SP-072 | Degraded enrichment correctly validates context size limits and label constraints | Pending |
| `IT-SP-668-003` | BR-AUDIT-002 | Audit manager correctly handles idempotent enrichment-complete and fatal error event paths | Pending |

#### 4a. EffectivenessMonitor IT (74.8% -> >=80%)

**IT-testable zero-coverage targets**: `SetupWithManager`, `SetRESTMapper`, `emitMetricsEvent`

> **Note**: `RecordComponentAssessed`, `RecordHashComputed`, `RecordAssessmentScheduled` from `audit/manager.go` are **UT-only** (`audit/` does NOT match `int_include` `/(client|status|reconciler)/`). Covered in Phase 2c UT scope.

| ID | BR | Business Outcome Under Test | Phase |
|----|----|-----------------------------|-------|
| `IT-EM-668-001` | BR-EM-001 | Reconciler correctly transitions EM CRD through monitoring lifecycle with real Prometheus data | Pending |
| `IT-EM-668-002` | BR-EM-004 | Status updater correctly reflects alert resolution state on the CRD status subresource | Pending |
| `IT-EM-668-003` | BR-EM-009 | Client correctly handles Prometheus query timeout and returns structured error | Pending |

#### 4b. Notification IT (63.1% -> >=80%)

**IT-testable zero-coverage targets**: `NewClient`, `Create`, `Get`, `List`, `Update`, `Delete`, `UpdateStatus`, `SetupWithManager`, `handleNotFound`, `cleanupAuditEventTracking`, `handleConfigMapChange`, `loadRoutingConfigFromCluster`, `rebuildSlackDeliveryServices`, `transitionToRetrying`, `transitionToPartiallySent`, `transitionToFailed`, `getMaxAttemptCount`, `calculateBackoffWithPolicy`, `isSlackCircuitBreakerOpen` (reclassified from UT per F1 audit: controller paths are IT-only in `coverage_report.py`)

| ID | BR | Business Outcome Under Test | Phase |
|----|----|-----------------------------|-------|
| `IT-NT-668-001` | BR-NOT-052 | Delivery engine correctly sends webhook notifications with retry on transient failures | Pending |
| `IT-NT-668-002` | BR-NOT-055 | Phase manager correctly transitions notification through pending -> sending -> delivered states | Pending |
| `IT-NT-668-003` | BR-NOT-064 | Status updater correctly reflects delivery outcome on the Notification CRD | Pending |
| `IT-NT-668-004` | BR-NOT-051 | Controller reconciler correctly picks up new Notification CRDs and triggers delivery | Pending |
| `IT-NT-668-005` | BR-NOT-063 | Client correctly reports delivery failures to DataStorage audit trail | Pending |

#### 4c. AIAnalysis IT (62.2% -> >=80%)

**IT-testable zero-coverage targets**: `SetupWithManager`, `aiAnalysisUpdatePredicate`, `handleDeletion`, `NewManager`, `RecordPhaseTransitionWithTimestamp`, `RecordErrorWithContext`, `RecordOperationTiming`, `RecordAIAgentCallWithTiming`, `RecordApprovalDecisionWithMetadata`, `RecordCompletionWithFinalStatus`, `WithPhaseContext`, `RecordError`, `AuditMiddleware`, `GetUnderlyingClient`, `determineNeedsHumanReview`, `RecordAIAgentSubmit`, `RecordAIAgentResult`, `RecordAIAgentSessionLost`

| ID | BR | Business Outcome Under Test | Phase |
|----|----|-----------------------------|-------|
| `IT-AA-668-001` | BR-AI-001 | Handler correctly processes analysis requests and returns structured responses via HTTP | Pending |
| `IT-AA-668-002` | BR-AI-008 | Controller reconciler correctly transitions AIAnalysis CRD through analysis lifecycle | Pending |
| `IT-AA-668-003` | BR-AUDIT-005 | Audit emitter correctly records analysis start/complete/fail events to DataStorage | Pending |
| `IT-AA-668-004` | BR-AI-001 | Handler correctly returns 400 for malformed requests with structured error body | Pending |

#### 4d. RemediationOrchestrator IT (60.7% -> >=80%)

**IT-testable zero-coverage targets**: `handleBlockedPhase`, `transitionToFailedTerminal`, `handleUnmanagedResourceExpiry`, `recheckResourceBusyBlock`, `recheckDuplicateBlock`, `clearEventBasedBlock`, `handleNotificationDeletion`, `SetDSClient`, `getSafeDefaultTimeouts`, `emitCompletionAudit`, `SetupWithManager`, `HandleRemediationTargetMissing`, `NewRARReconciler`, RARReconciler.`Reconcile`, RAR.`SetupWithManager`, `StabilizationWindow`, `extractLabelsViaUnstructured` (reclassified from UT per F4-v2 audit: `creator/` matches `unit_exclude`)

| ID | BR | Business Outcome Under Test | Phase |
|----|----|-----------------------------|-------|
| `IT-RO-668-001` | BR-ORCH-025 | Creator correctly provisions WorkflowExecution CRD from remediation plan | Pending |
| `IT-RO-668-002` | BR-ORCH-034 | Aggregator correctly merges results from parallel workflow executions | Pending |
| `IT-RO-668-003` | BR-ORCH-036 | Status updater correctly reflects aggregated outcome on RemediationRequest CRD | Pending |
| `IT-RO-668-004` | BR-ORCH-042 | Handler correctly processes signal processing callbacks with real envtest | Pending |

#### 4e. Gateway IT (56.0% -> >=80%)

**IT-testable zero-coverage targets**: `UpdateRemediationRequest`, `ListRemediationRequestsByFingerprint`, CB.`UpdateRemediationRequest`, CB.`GetRemediationRequest`, CB.`State`, `NewServer`, `NewServerWithK8sClient`

> **Note**: `SetLogger`, `ReplayValidator` (2x) from `adapters/` are **UT-only** (`adapters/` does NOT match `int_include`). Covered in Phase 3a UT scope.

| ID | BR | Business Outcome Under Test | Phase |
|----|----|-----------------------------|-------|
| `IT-GW-668-001` | BR-GATEWAY-003 | K8s CRD creator correctly creates Signal CRDs from validated gateway payloads | Pending |
| `IT-GW-668-002` | BR-GATEWAY-008 | Distributed lock correctly prevents duplicate signal processing across replicas | Pending |
| `IT-GW-668-003` | BR-GATEWAY-009 | Status updater correctly reflects processing outcome on Signal CRD | Pending |
| `IT-GW-668-004` | BR-GATEWAY-093 | Server correctly handles graceful shutdown draining in-flight requests | Pending |

#### 4f. KubernautAgent IT (51.5% -> >=80%)

**IT-testable zero-coverage targets** (per `int_include` regex): `Investigate`, `backfillSeverity`, `resolveEnrichment`, `runRCA`, `runWorkflowSelection`, `enrichFromCatalog`, `runLLMLoop`, `totalPromptLength`, `lastUserMessage`, `truncatePreview`, `toolNames`, `toolDefinitionsForPhase`, `executeTool`, `GetSpecHash`, `WithLabelDetector` (reclassified from UT per F5-v2 audit: `enrichment/enricher.go:` matches `unit_exclude`)

> **Note**: `NewBufferedDSAuditStore`, `StoreAudit`, `Flush`, `Close` from `audit/ds_store.go` are **NOT** in `int_include` — they count toward UT tier only. Moved to Phase 2e UT scope.

| ID | BR | Business Outcome Under Test | Phase |
|----|----|-----------------------------|-------|
| `IT-KA-668-001` | BR-HAPI-433 | Investigator correctly orchestrates tool calls and produces structured RCA via real HTTP | Pending |
| `IT-KA-668-002` | BR-HAPI-433 | Session manager correctly manages concurrent investigation sessions with isolation | Pending |
| `IT-KA-668-003` | BR-HAPI-433 | K8s tools correctly query pod logs and events via real envtest API | Pending |
| `IT-KA-668-004` | BR-HAPI-211 | Handler correctly processes investigation requests and returns structured responses | Pending |

#### 4g. WorkflowExecution IT (46.3% -> >=80%)

**IT-testable zero-coverage targets**: `SetupWithManager` (controller), `UpdatePhase`, `isTerminalPhase`

> **Note**: `CancelJob`, `FindJobTemplateByName`, `FindCredentialTypeByKind`, `GetJobTemplateCredentials` from `executor/awx_client.go` are **UT-only** (`executor/` does NOT match `unit_exclude`). Covered in Phase 2d UT scope.

| ID | BR | Business Outcome Under Test | Phase |
|----|----|-----------------------------|-------|
| `IT-WE-668-001` | BR-WE-001 | Controller reconciler correctly transitions WE CRD through execution lifecycle | Pending |
| `IT-WE-668-002` | BR-WE-003 | Status updater correctly reflects step-level outcomes on WE CRD status | Pending |
| `IT-WE-668-003` | BR-AUDIT-005 | Audit emitter correctly records execution start/step-complete/fail events | Pending |
| `IT-WE-668-004` | BR-WE-009 | Controller correctly handles step timeout by marking step as failed | Pending |

#### 4h. DataStorage IT (33.5% -> >=80%)

**IT-testable zero-coverage targets**: `AggregateSuccessRate`, `AggregateByNamespace`, `AggregateBySeverity`, `AggregateIncidentTrend`, `NewDBAdapter`, `Query`, `CountTotal`, `Get`, `HealthCheck`, `ReadMessages`, `AckMessage`, `MoveToDeadLetter`, `IncrementRetryCount`, `GetPendingMessages`, `isConsumerGroupExistsError`, `isNoSuchKeyError`, `isNoGroupError`, `convertPlaceholdersToPostgreSQL`, `replaceFirstOccurrence` (from `adapter/utils.go`, reclassified from UT per F2 audit)

| ID | BR | Business Outcome Under Test | Phase | Incrementality |
|----|----|-----------------------------|-------|----------------|
| `IT-DS-668-001` | BR-STORAGE-001 | Audit events handler correctly processes POST /api/v1/audit-events with hash chain | Pending | **INCREMENTAL** -- existing 46 audit ITs use `AuditEventsRepository` directly; no HTTP handler POST `It` exists |
| `IT-DS-668-002` | BR-STORAGE-028 | Audit events batch handler correctly enforces max batch size limit | Pending | **REDUNDANT** -- `batch_size_limit_test.go` (IT-DS-043-001-003) already covers HTTP POST batch at/over/under limits |
| `IT-DS-668-003` | BR-WORKFLOW-001 | Workflow CRUD handlers correctly create/read/update/delete workflows via HTTP | Pending | **PARTIAL** -- POST covered (content integrity, dependency validation, deterministic UUID); HTTP GET/UPDATE/DELETE untested (repo-only via `workflowRepo`) |
| `IT-DS-668-004` | BR-STORAGE-021 | Query handler correctly applies filters, pagination, and sorting to audit events | Pending | **PARTIAL** -- repo-level filter/pagination tested; HTTP query surface limited to GET in graceful shutdown tests; **sorting not asserted** |
| `IT-DS-668-005` | BR-STORAGE-033 | DLQ handler correctly drains and retries failed events with exponential backoff | Pending | **INCREMENTAL** -- drain+client covered; **no IT for background retry worker or exponential backoff** |
| `IT-DS-668-006` | BR-STORAGE-010 | Adapter layer correctly transforms between HTTP models and repository models | Pending | **INCREMENTAL** -- adapter exercised narrowly on remediation/effectiveness paths only; no broad DTO coverage |
| `IT-DS-668-007` | BR-AUDIT-004 | Repository correctly handles concurrent inserts with advisory lock ordering | Pending | **REDUNDANT** -- `create_batch_lock_ordering_test.go` (IT-DS-040-001-002) covers concurrent `CreateBatch` + hash chain |
| `IT-DS-668-008` | BR-EM-001 | Effectiveness handler correctly processes effectiveness data with real PostgreSQL | Pending | **PARTIAL** -- `context_propagation_test.go` has 3 `It`s for GET effectiveness + DB; adapter queries covered; broader response shape untested |
| `IT-DS-668-009` | BR-STORAGE-042 | Graceful shutdown correctly drains HTTP connections and flushes audit buffer | Pending | **REDUNDANT** -- `graceful_shutdown_integration_test.go` (18 `It`s) covers readiness, liveness, in-flight, DLQ drain, DB close, load |
| `IT-DS-668-010` | BR-STORAGE-043 | ActionType CRUD correctly handles create/read/update/delete operations | Pending | **PARTIAL** -- `actiontype_lifecycle_test.go` (15 `It`s) is deep repository CRUD; **HTTP API-level CRUD `It`s do not exist** |

> **DS IT Effort Recalibration**: Of 10 planned scenarios, **3 are redundant** (002, 007, 009 -- already at 220 `It`s), **4 are partially covered** (003, 004, 008, 010 -- HTTP handler layer gaps), and **3 are truly incremental** (001, 005, 006 -- new handler/worker/adapter coverage). Effective incremental work: **~7 scenarios** (3 full + 4 partial extensions). This reduces the DS IT LOE from 8-12 to **6-10 engineer-days**.

#### 4i. AuthWebhook IT (23.7% -> >=80%)

**IT-testable zero-coverage targets**: `NewNotificationRequestDeleteHandler`, `Handle`, `InjectDecoder`, `NewNotificationRequestValidator`, `ValidateCreate`, `ValidateUpdate`, `ValidateDelete`

> **Note**: `SetupWithManager` (`rw_reconciler.go`) and `NeedLeaderElection` (`startup_reconciler.go`) are **UT-only** (not in `int_include` five-file list). Covered in Phase 3b UT scope.

> **Infra prerequisite for IT-AW-668-002**: `RemediationRequestStatusHandler` is registered in production (`cmd/authwebhook/main.go`) but is NOT in `config/webhook/manifests.yaml` and NOT registered in `test/integration/authwebhook/suite_test.go`. Before writing this test, extend the webhook manifests and suite registration.
>
> **Note on IT-AW-668-005**: `NotificationRequestValidator.ValidateCreate`/`ValidateUpdate` are no-ops; there is no defaulting behavior. Scenario rewritten to test `ValidateDelete` attribution and audit behavior.
>
> **Note on NR handler divergence**: The IT suite registers `NotificationRequestValidator` (via `WithCustomValidator`), while production registers `NotificationRequestDeleteHandler`. Both should be tested to ensure coverage of `Handle` method on the delete handler.

| ID | BR | Business Outcome Under Test | Phase |
|----|----|-----------------------------|-------|
| `IT-AW-668-001` | BR-AUTH-001 | NotificationRequest delete handler correctly denies deletion and records audit via envtest | Pending |
| `IT-AW-668-002` | BR-AUTH-001 | RemediationRequest status handler correctly validates status transitions via envtest (requires manifests + suite extension) | Pending |
| `IT-AW-668-003` | BR-WE-013 | WorkflowExecution handler correctly validates and admits/rejects CRDs via envtest | Pending |
| `IT-AW-668-004` | BR-WORKFLOW-006 | RemediationApprovalRequest handler correctly processes approval/deny decisions | Pending |
| `IT-AW-668-005` | BR-WORKFLOW-007 | NotificationRequest validator ValidateDelete correctly attributes deletion actor and emits audit event | Pending |

### Tier Skip Rationale

- **E2E**: Out of scope per Section 4.2. E2E binary coverage profiling is not actionable at the function level.

---

## 9. Test Cases

> For this plan (>80 tests across 11 services), detailed cases are provided for P0 representative tests. P1/P2 tests follow the same Given/When/Then pattern.

### UT-SP-668-001: Signal deduplication identifies duplicates

**BR**: BR-SP-001
**Priority**: P0
**Type**: Unit
**File**: `test/unit/signalprocessing/dedup_test.go`

**Test Steps**:
1. **Given**: A deduplication engine configured with a 5-minute window
2. **When**: Two signals with identical fingerprints arrive 2 minutes apart
3. **Then**: The second signal is identified as a duplicate

**Acceptance Criteria**:
- **Behavior**: Dedup returns `isDuplicate=true` for the second signal
- **Correctness**: The first signal is not marked as duplicate
- **Accuracy**: Window boundary (exactly 5 minutes) is handled correctly

### IT-DS-668-001: Audit events POST with hash chain

**BR**: BR-STORAGE-001
**Priority**: P0
**Type**: Integration
**File**: `test/integration/datastorage/audit_events_handler_test.go`

**Test Steps**:
1. **Given**: A running DataStorage server with PostgreSQL and partitions created
2. **When**: A valid audit event is POSTed to `/api/v1/audit-events`
3. **Then**: The event is persisted with a valid hash chain link to the previous event

**Acceptance Criteria**:
- **Behavior**: HTTP 201 returned with event_id in response body
- **Correctness**: event_hash is SHA256 of (previous_hash + event_json)
- **Accuracy**: Querying the event back returns identical field values

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: External dependencies only (LLM, external HTTP, K8s API where envtest is not available)
- **Location**: `test/unit/{service}/`
- **Resources**: Standard CI runner (2 CPU, 4GB RAM)

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (No-Mocks Policy)
- **Infrastructure**:
  - PostgreSQL 16 (for DS) via Podman
  - Redis (for GW, DS) via Podman
  - envtest (for NT, RO, WE, EM, SP, AA, AW, KA) via setup-envtest
  - httptest (for all services with HTTP handlers)
- **Location**: `test/integration/{service}/`
- **Resources**: CI runner with Podman support (4 CPU, 8GB RAM)

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.23 | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| Podman | 4.x | Container tests (PostgreSQL, Redis) |
| setup-envtest | latest | K8s API for controller tests |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Issue #667 | Code | Merged | DS IT deadlock fixes required | Already on branch |
| Issue #597 | Code | Merged | Continue route fanout for NT tests | Already on branch |
| Issue #416 | Code | Merged | Label-based routing for NT tests | Already on branch |

### 11.2 Make Target Validation

All 21 required Make targets have been validated as available in the root `Makefile`:

| Target Pattern | Mechanism | Services Covered |
|----------------|-----------|------------------|
| `make test-unit-%` | Pattern rule | signalprocessing, aianalysis, notification, remediationorchestrator, effectivenessmonitor, workflowexecution |
| `make test-unit-{svc}` | Explicit rule | kubernautagent, gateway, authwebhook, datastorage |
| `make test-integration-%` | Pattern rule | signalprocessing, aianalysis, notification, remediationorchestrator, effectivenessmonitor, workflowexecution, gateway, authwebhook |
| `make test-integration-{svc}` | Explicit rule | kubernautagent, datastorage |
| `make lint-test-patterns` | Explicit rule | All (runs `scripts/validation/check-test-anti-patterns.sh`) |

All targets produce coverage profiles at `coverage_{unit,integration}_{service}.out` and are compatible with `go tool cover -func` for function-level analysis.

### 11.3 Execution Order

Each service follows a TDD cycle with checkpoints:

0. **Phase 0 — Infrastructure Prerequisites** (~0.5 day):
   - Exclude `zz_generated*.go` from shared-packages `coverpkg` in Makefile (or add `zz_generated` to `GENERATED_CODE_PATTERNS` in `coverage_report.py`)
   - Add `pkg/datastorage/partition/...` to `DATASTORAGE_COVERPKG` in Makefile
   - Extend `config/webhook/manifests.yaml` with `RemediationRequest` status webhook rule
   - Register `RemediationRequestStatusHandler` in `test/integration/authwebhook/suite_test.go`
   - **Checkpoint 0**: Verify `make test-unit-shared-packages` and `make test-unit-datastorage` still pass; verify `make test-integration-authwebhook` still passes with new webhook registration
1. **Phase 1 — Quick Wins** (~0.5–1 day): signalprocessing, aianalysis (UT only, <5% gap, ~8 functions)
   - **Checkpoint 1**: Verify UT coverage >=80% for SP and AA
2. **Phase 2 — Moderate Gaps** (~3–5 days): notification, remediationorchestrator, effectivenessmonitor, workflowexecution, kubernautagent (UT, ~10-20% gap, ~33 functions)
   - **Checkpoint 2**: Verify UT coverage >=80% for all Phase 2 services
3. **Phase 3 — Large Gaps** (~12–20 days): gateway, authwebhook, datastorage, shared-packages (UT, >20% gap; **DS alone is 10-15 days** with 199 UT-testable functions)
   - **Checkpoint 3**: Verify UT coverage >=80% for all Phase 3 services
4. **Phase 4 — Integration Tests** (~13–21 days): All 10 services (IT, targeting >=80%; **DS is 6-10 days** after incrementality analysis reduced 10 scenarios to 7 effective; 37 handlers + 106 repo funcs + 21 DLQ funcs but 3 scenarios redundant with existing 220 ITs)
   - **Checkpoint 4**: Verify IT coverage >=80% for all services

**Total estimated LOE**: 29–47 engineer-days sequential (revised; DS accounts for ~16–25 of this)

> **Wall-clock with parallelization** (see Section 11.4): UT phases can run fully in parallel per service. With CI matrix (1 service/job), Phase 4 wall-clock is bounded by the single longest service (DS: 6-10d). **Effective wall-clock: 18–33 days** assuming 2-3 service parallelism for UT phases and CI matrix for IT.

> **DS time-boxing**: DataStorage should be treated as a dedicated workstream, not bundled equally with other services. Recommend allocating a single owner for DS with 2-3 week calendar blocks for each tier.

Each checkpoint performs:
- Coverage measurement via `scripts/coverage/coverage_report.py`
- Anti-pattern scan via `make lint-test-patterns`
- Full test suite regression check via `make test-unit-{service}` / `make test-integration-{service}`

### TDD Phase Breakdown

For each service within each phase:

1. **TDD RED**: Write failing tests for all scenarios in the service's test group
   - **Adversarial audit**: Verify tests fail for the right reason, not due to compilation errors or wrong assertions
2. **TDD GREEN**: Implement minimal code changes (if any — most tests exercise existing code that simply lacks coverage)
   - **Adversarial audit**: Verify no over-engineering, no new abstractions beyond what's needed
3. **TDD REFACTOR**: Extract shared helpers, eliminate duplication across test files, improve assertion clarity
   - **Adversarial audit**: Verify no behavioral changes, coverage maintained, anti-patterns eliminated

### 11.4 Execution Model & Parallelization Strategy

#### Execution Split: UT Local / IT+E2E in CI/CD

| Tier | Where | TDD RED Phase | Rationale |
|------|-------|---------------|-----------|
| **UT** | Local | Local | Tight feedback loop, zero infrastructure dependencies, full parallelism via `make -j` |
| **IT** | CI/CD (matrix) | CI/CD | CI matrix eliminates all infrastructure constraints (generate race, envtest install, Podman prune); most #668 IT scenarios cover existing code retroactively — "RED" is simply the test not existing yet |
| **E2E** | CI/CD only | CI/CD | Requires Kind cluster + full service stack; not practical locally |

**Rationale for IT in CI/CD**: The majority of #668 integration test scenarios exercise **existing code that simply lacks coverage** rather than driving new implementation. The TDD RED phase is trivially satisfied (the test did not exist before). Running IT exclusively in CI/CD via matrix jobs provides:
- Full isolation per service (separate VMs eliminate generate/envtest/Podman races)
- Parallel execution across all 10 services simultaneously
- Consistent reproducible environments matching production CI

#### UT Parallelization (Phases 1-3) — Local

All `make test-unit-{service}` targets are **fully independent** and can run in parallel. Each target:
- Compiles a distinct test package under `test/unit/{service}/`
- Writes to a unique coverage profile `coverage_unit_{service}.out`
- Has no external infrastructure dependencies (no Podman, no envtest, no network)
- Shares only `GOCACHE` and CPU/RAM

**Local UT workflow**:
```bash
# Parallel execution across all services
make -j 11 test-unit-signalprocessing test-unit-aianalysis test-unit-notification \
  test-unit-remediationorchestrator test-unit-effectivenessmonitor test-unit-workflowexecution \
  test-unit-kubernautagent test-unit-gateway test-unit-authwebhook test-unit-datastorage \
  test-unit-shared-packages

# Anti-pattern check before pushing
make lint-test-patterns
```

Maximum local parallelism: **11 concurrent UT jobs** (10 services + shared-packages).

#### IT Parallelization (Phase 4) — CI/CD Matrix

IT suites use dedicated, non-overlapping port blocks per service (DD-TEST-001):

| Service | PostgreSQL Port | Redis Port | DS Port | Metrics Port |
|---------|----------------|------------|---------|-------------|
| DataStorage | 15433 | 16379 | — (in-process) | — |
| SignalProcessing | 15436 | 16382 | 18094 | 19094 |
| Gateway | 15437 | 16380 | 18091 | 19091 |
| Notification | 15440 | 16385 | 18096 | 19096 |
| AuthWebhook | 15442 | 16386 | 18099 | — |
| KubernautAgent | 13322-13329 | — | — | — |

**CI matrix configuration**: 1 service = 1 CI job. This naturally eliminates:

1. **`make generate` race**: Each job runs its own `generate` in an isolated workspace
2. **`setup-envtest` install race**: Each job installs to its own `bin/`
3. **Podman global prune**: No cross-talk — separate VMs/containers
4. **Host resource contention**: Each job has dedicated CPU/RAM for envtest + etcd + Podman stack

**CI/CD workflow per IT job**:
```bash
# Each matrix job runs a single service
make test-integration-${SERVICE}
# Coverage artifact uploaded for aggregation
python3 scripts/coverage/coverage_report.py --service ${SERVICE}
```

**LOE impact**: With CI matrix (1 service/job), Phase 4 wall-clock drops from **13-21 days sequential** to **6-10 days** (bounded by DS, the single largest service).

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/668/TEST_PLAN.md` | Strategy and test design |
| Unit test suites | `test/unit/{service}/` | Ginkgo BDD test files per service |
| Integration test suites | `test/integration/{service}/` | Ginkgo BDD test files per service |
| Coverage report | CI artifact | Per-tier coverage percentages via `coverage_report.py` |

---

## 13. Execution

### 13.1 Local — Unit Tests (Phases 1-3)

```bash
# TDD RED: Write failing test, run locally
make test-unit-{service}

# TDD GREEN: Implement minimal code, run locally
make test-unit-{service}

# TDD REFACTOR: Improve, run locally
make test-unit-{service}

# Parallel execution across all services (post-implementation validation)
make -j 11 test-unit-signalprocessing test-unit-aianalysis test-unit-notification \
  test-unit-remediationorchestrator test-unit-effectivenessmonitor test-unit-workflowexecution \
  test-unit-kubernautagent test-unit-gateway test-unit-authwebhook test-unit-datastorage \
  test-unit-shared-packages

# Anti-pattern compliance before push
make lint-test-patterns
```

### 13.2 CI/CD — Integration Tests (Phase 4)

CI matrix runs 1 service per job. Developer pushes IT test code; CI validates.

```bash
# CI matrix job (per service)
make test-integration-${SERVICE}
```

### 13.3 CI/CD — E2E Tests

E2E tests run in CI/CD only (requires Kind cluster + full service stack).

```bash
make test-e2e
```

### 13.4 Coverage Measurement

```bash
# Local: per-service UT coverage
python3 scripts/coverage/coverage_report.py --service {service}

# CI: aggregated across all tiers
python3 scripts/coverage/coverage_report.py  # all services

# Anti-pattern check
make lint-test-patterns

# Specific test by ID
go test ./test/unit/signalprocessing/... --ginkgo.focus="UT-SP-668"
```

---

## 14. Existing Tests Requiring Updates

No existing tests require modification. This effort adds new test coverage for uncovered code paths. All existing tests must continue to pass (regression check at each checkpoint).

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 5.1 | 2026-04-11 | Execution model: UT local / IT+E2E CI/CD split (Section 11.4 rewritten); Section 13 restructured into 13.1-13.4 for local UT, CI/CD IT, CI/CD E2E, and coverage measurement |
| 5.0 | 2026-04-11 | Gap closure: added R12-R14 (anti-pattern coexistence confirmed safe, webhook drift beyond RR scoped, SP Node enricher in envtest validated); DS IT incrementality analysis (3 redundant, 4 partial, 3 incremental — LOE revised to 6-10d for IT); added Section 11.4 parallelization strategy (UT fully parallel, IT via CI matrix — wall-clock from 31-51d to 18-33d with parallelization) |
| 4.1 | 2026-04-11 | Preflight corrections: rewrote IT-SP-668 targets to match actual codebase functions (enricher arms, degraded validators, audit branches — original targets didn't exist); added shared-packages effective denominator analysis (177 funcs after DeepCopy exclusion, ~75.5% projected, ~4.5% gap); revised DS LOE to dedicated time-boxing (UT 10-15d, IT 8-12d); revised total LOE to 31-51 days |
| 4.0 | 2026-04-11 | Audit v2 amendments: reclassified SP StartHotReload to UT (F1-v2); WE executor/ to UT (F2-v2); EM audit/Record* to UT-only (F3-v2); RO creator/ to IT (F4-v2); KA WithLabelDetector to IT (F5-v2); added IT-SP-668 scenarios and updated Phase 4 to 10 services (F6-v2); GW adapters/ to UT (F7-v2); AW rw_reconciler/startup_reconciler to UT (F8-v2); fixed section 5.5/5.6 ordering (F9-v2); corrected LIBRARY TESTING logrus.New() description (F10-v2); added Section 5.7 TDD compliance enforcement (F11-v2) |
| 3.0 | 2026-04-11 | Audit amendments: reclassified NT controller helpers, DS adapter/utils, KA ds_store from UT to correct tiers (F1-F3); added Phase 0 for infra prerequisites (F4,F8); rewrote AW IT-002/005 (F5,F6); dropped duplicate SP/DS tests (F7); added anti-pattern compliance conventions (Section 5.6); added risks R7-R11; added infra notes for AW webhook manifests |
| 2.0 | 2026-04-11 | Added: per-service BR mappings (Section 7.1), zero-coverage function targets per phase, LOE estimates per phase (total 15.5–24 days), Make target validation (Section 11.2), BR column in all test scenario tables |
| 1.0 | 2026-04-11 | Initial test plan with 80+ test scenarios across 11 services |
