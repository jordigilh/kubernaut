# Test Plan: Issue #353 - NextAllowedExecution on NoActionRequired RRs

**Feature**: Wire NoActionRequiredDelayHours through production config to suppress Gateway duplicate RR creation
**Version**: 1.0
**Created**: 2026-03-11
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/v1.0.1-chart-platform-agnostic`

**Authority**:
- BR-ORCH-037: WorkflowNotNeeded completes with NoActionRequired
- Issue #314: Suppress Gateway duplicate RR creation for same signal fingerprint
- Issue #353: NextAllowedExecution not set on NoActionRequired RRs (#314 regression)

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- **Config layer**: `RoutingConfig` must include `NoActionRequiredDelayHours` with default 24, YAML loading, and validation
- **Production wiring**: `cmd/remediationorchestrator/main.go` must pass the field to the routing engine
- **Helm chart**: ConfigMap template must emit the field
- **Integration chain**: Full reconciler -> handler -> status update chain must set `NextAllowedExecution`
- **E2E validation**: Real cluster with production config must populate the field

### Out of Scope

- Handler logic changes (already correct in `aianalysis.go:171`)
- Gateway suppression logic (already correct in `phase_checker.go:143`)
- CRD type changes (field already defined)

### Design Decisions

- NoActionRequiredDelayHours uses `int` type (hours) matching routing.Config convention
- Default 24h matches the documented intent in routing/blocking.go
- Validation allows 0 (opt-out) but rejects negative values
- Helm template uses hardcoded default; operator override deferred to future Helm values refactor

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

- **Unit**: >=80% of config parsing/validation code for the new field
- **Integration**: >=80% of the reconciler -> handler -> status update chain for NoActionRequired path
- **E2E**: Validates production config produces correct NextAllowedExecution

### 2-Tier Minimum

All 3 tiers are covered: UT + IT + E2E.

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/config/remediationorchestrator/config.go` | `DefaultConfig`, `LoadFromFile`, `Validate` | ~5 new lines |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `cmd/remediationorchestrator/main.go` | Startup wiring (routingCfg) | ~1 new line |
| `internal/controller/remediationorchestrator/reconciler.go` | Config -> handler chain | existing |
| `pkg/remediationorchestrator/handler/aianalysis.go` | `handleWorkflowNotNeeded` | existing |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| Issue #353 | Default config gives operators a 24h suppression window without config changes | P0 | Unit | UT-RO-353-001 | GREEN |
| Issue #353 | Operator can customize suppression window via YAML without breaking other routing | P1 | Unit | UT-RO-353-002 | GREEN |
| Issue #353 | Invalid config rejected at startup; zero allowed as explicit opt-out | P1 | Unit | UT-RO-353-003 | GREEN |
| Issue #353 | Full reconciler chain produces a future suppression window on NoActionRequired RR | P0 | Integration | IT-RO-353-001 | GREEN |
| Issue #353 | Production-deployed RO populates suppression window on real CRD in Kind cluster | P0 | E2E | E2E-RO-353-001 | GREEN |

---

## 5. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: `internal/config/remediationorchestrator/config.go` — DefaultConfig, LoadFromFile, Validate

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-353-001` | Operator gets a working 24h suppression window by default: field present, correct value, produces valid duration, config validates | GREEN |
| `UT-RO-353-002` | Operator can override the delay via YAML without breaking other routing fields: override applied, other defaults preserved, config validates | GREEN |
| `UT-RO-353-003` | Negative delay rejected with operator-friendly error; zero explicitly allowed as opt-out | GREEN |

### Tier 2: Integration Tests

**Testable code scope**: Full reconciler -> AIAnalysisHandler -> UpdateRemediationRequestStatus chain

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-RO-353-001` | NoActionRequired RR gets a future suppression window proportional to configured delay: NextAllowedExecution non-nil, strictly in future, ~now+24h; CompletedAt populated proving normal completion | GREEN |

### Tier 3: E2E Tests

**Testable code scope**: Full Kind cluster with Helm-deployed RO service

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-RO-353-001` | Production-deployed RO populates NextAllowedExecution on real CRD: non-nil, strictly in future, ~now+24h; proves full Helm->config->reconciler->handler->K8s chain | GREEN |

---

## 6. Test Cases (Detail)

### UT-RO-353-001: DefaultConfig provides working 24h suppression window

**BR**: Issue #353
**Type**: Unit
**File**: `test/unit/remediationorchestrator/config_test.go`

**Given**: No config file (DefaultConfig used)
**When**: DefaultConfig() is called
**Then**: Operator gets a working 24h suppression window without any config changes

**Acceptance Criteria** (behavior, correctness, accuracy):
- Field value is exactly 24 (correctness)
- Converted duration matches 24h: `time.Duration(24) * time.Hour == 24*time.Hour` (accuracy -- proves the reconciler conversion produces the intended duration)
- DefaultConfig as a whole validates successfully (behavior -- no regression on other fields)

### UT-RO-353-002: YAML override applies without breaking other routing

**BR**: Issue #353
**Type**: Unit
**File**: `test/unit/remediationorchestrator/config_test.go`

**Given**: A YAML config file with `routing.noActionRequiredDelayHours: 48`
**When**: LoadFromFile is called
**Then**: Operator gets the custom value and all other routing defaults are preserved

**Acceptance Criteria** (behavior, correctness, accuracy):
- YAML tag `noActionRequiredDelayHours` correctly maps to field value 48 (correctness)
- Loaded config validates successfully (behavior -- override doesn't break validation)
- Other routing fields retain their YAML-specified values (accuracy -- override is surgical, not destructive)

### UT-RO-353-003: Negative value rejected; zero allowed as opt-out

**BR**: Issue #353
**Type**: Unit
**File**: `test/unit/remediationorchestrator/config_test.go`

**Given**: Configs with NoActionRequiredDelayHours = -1 and = 0
**When**: Validate() is called on each
**Then**: Negative value rejected with clear error; zero passes validation (explicit opt-out)

**Acceptance Criteria** (behavior, correctness, accuracy):
- Negative (-1): Validate returns error containing "noActionRequiredDelayHours" (behavior -- operator sees which field is wrong)
- Zero (0): Validate succeeds (correctness -- allows explicit opt-out per existing handler `if > 0` guard)

### IT-RO-353-001: Full reconciler chain produces future suppression window

**BR**: Issue #353
**Type**: Integration
**File**: `test/integration/remediationorchestrator/lifecycle_test.go`

**Given**: RO reconciler running with routing config containing NoActionRequiredDelayHours=24
**When**: AIAnalysis completes with WorkflowNotNeeded (problem self-resolved)
**Then**: RR has a future suppression window proportional to configured delay

**Acceptance Criteria** (behavior, correctness, accuracy):
- NextAllowedExecution is not nil (behavior -- field is populated through the chain)
- NextAllowedExecution is strictly in the future: `time.Now().Before(NextAllowedExecution.Time)` (correctness -- Gateway will actually suppress)
- NextAllowedExecution is ~now+24h within 2 minute tolerance (accuracy -- value proportional to configured delay, not a magic number)
- CompletedAt is also populated and in the past (correctness -- proves normal completion, not partial status)
- OverallPhase is Completed, Outcome is NoActionRequired (behavior -- the completion flow itself is unaffected)

### E2E-RO-353-001: Production config populates suppression window on real CRD

**BR**: Issue #353
**Type**: E2E
**File**: `test/e2e/remediationorchestrator/noaction_suppression_e2e_test.go`

**Given**: Kind cluster with Helm-deployed RO using default config (noActionRequiredDelayHours: 24)
**When**: RR completes with NoActionRequired via real AIAnalysis CRD
**Then**: Real RR CRD has a future suppression window

**Acceptance Criteria** (behavior, correctness, accuracy):
- NextAllowedExecution is non-nil on the real CRD (behavior -- Helm config -> Go config -> routing -> handler -> K8s API chain works)
- NextAllowedExecution is strictly in the future (correctness -- Gateway will suppress duplicate RRs)
- NextAllowedExecution is approximately now + 24h within 5 minute tolerance (accuracy -- matches Helm default)

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None (config parsing is pure logic)
- **Location**: `test/unit/remediationorchestrator/`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (uses envtest with real K8s API)
- **Infrastructure**: envtest (controller-runtime test environment)
- **Location**: `test/integration/remediationorchestrator/`

### E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks
- **Infrastructure**: Kind cluster with Helm-deployed services
- **Location**: `test/e2e/remediationorchestrator/`

---

## 8. Execution

```bash
# Unit tests
go test ./test/unit/remediationorchestrator/... --ginkgo.focus="UT-RO-353"

# Integration tests
make test-integration-remediationorchestrator

# E2E tests
make test-e2e-remediationorchestrator
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-11 | Initial test plan |
| 1.1 | 2026-03-11 | Strengthen all test cases with behavior/correctness/accuracy acceptance criteria |
| 1.2 | 2026-03-13 | All tiers GREEN: config field added, wired in main.go, Helm template updated, tests passing |
