# Test Plan: Shared BusinessClassification Typed Enums (#458)

**Feature**: Add kubebuilder enum validation for `criticality` and `slaRequirement` fields on the shared `BusinessClassification` struct
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `feature/v1.0-remaining-bugs-demos`

**Authority**:
- BR-SP-002: Business Classification
- BR-SP-080: Business Unit Detection
- BR-SP-081: SLA Requirement Mapping (defines criticality: critical/high/medium/low; SLA tier: platinum/gold/silver/bronze)

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- `pkg/shared/types/enrichment.go`: Add `Criticality` and `SLARequirement` typed string aliases with kubebuilder enum annotations
- `internal/controller/signalprocessing/signalprocessing_controller.go`: Update `classifyBusiness` to use typed constants
- `pkg/signalprocessing/audit/helpers.go`: Update `toSignalProcessingAuditPayloadCriticality` to accept typed values
- CRD regeneration for both SignalProcessing and AIAnalysis CRDs

### Out of Scope

- #456 (SP environment/priority fields — separate issue, depends on this one)
- #457 (AA analysisTypes/reason/decision fields — separate issue, depends on this one)
- Adding new criticality or SLA values beyond the BR-SP-081 documented set

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Enum values: `critical, high, medium, low` for criticality | Per BR-SP-081 acceptance criteria |
| Enum values: `platinum, gold, silver, bronze` for SLA | Per BR-SP-081 acceptance criteria ("Dimension 4: SLA Tier") |
| `platinum` included in enum despite not currently produced | BR-SP-081 documents it as valid; allows future expansion without schema change |
| Types defined in `pkg/shared/types/enrichment.go` | Authoritative shared types location, already used by SP and AA CRDs |
| Issue body correction: SLA values are NOT `immediate/urgent/standard/relaxed` | Code + BR-SP-081 + CRD YAML all say `platinum/gold/silver/bronze` |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (pure logic: `classifyBusiness`, type constants, audit helpers)
- **Integration**: >=80% of integration-testable code (CRD round-trip, SP reconciliation with typed fields)

### 2-Tier Minimum

Both unit and integration tiers provide defense-in-depth:
- Unit tests catch type constant correctness and business logic mapping
- Integration tests catch CRD schema validation and round-trip fidelity

### Business Outcome Quality Bar

Tests validate that business classification values are schema-validated, correctly assigned per environment tier, and faithfully propagated through audit.

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/shared/types/enrichment.go` | `Criticality` type, `SLARequirement` type, constants | ~20 (new) |
| `internal/controller/signalprocessing/signalprocessing_controller.go` | `classifyBusiness` | ~38 |
| `pkg/signalprocessing/audit/helpers.go` | `toSignalProcessingAuditPayloadCriticality` | ~15 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/signalprocessing/signalprocessing_controller.go` | `reconcileCategorizing` (stores BusinessClassification on CRD) | ~30 |
| CRD schema | kubebuilder enum validation on `criticality` and `slaRequirement` | N/A |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-SP-081 | Criticality enum values: critical, high, medium, low | P1 | Unit | UT-SP-458-001 | Pending |
| BR-SP-081 | SLA enum values: platinum, gold, silver, bronze | P1 | Unit | UT-SP-458-002 | Pending |
| BR-SP-081 | classifyBusiness assigns typed constants per environment | P1 | Unit | UT-SP-458-003 | Pending |
| BR-SP-081 | Audit mapper accepts typed Criticality values | P1 | Unit | UT-SP-458-004 | Pending |
| BR-SP-081 | CRD rejects invalid criticality values | P1 | Integration | IT-SP-458-001 | Pending |
| BR-SP-081 | CRD rejects invalid slaRequirement values | P1 | Integration | IT-SP-458-002 | Pending |
| BR-SP-081 | BusinessClassification round-trips through K8s API with typed values | P1 | Integration | IT-SP-458-003 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `SP` (SignalProcessing — primary affected service)
- **ISSUE_NUMBER**: 458
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `classifyBusiness` (~38 lines), type constants (~20 lines), audit helper (~15 lines) — target >=80%

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-SP-458-001` | Criticality type has exactly 4 valid constants (critical, high, medium, low) | Pending |
| `UT-SP-458-002` | SLARequirement type has exactly 4 valid constants (platinum, gold, silver, bronze) | Pending |
| `UT-SP-458-003` | classifyBusiness returns typed constants matching environment tier (production->high/gold, staging->medium/silver, dev->low/bronze, default->medium/bronze) | Pending |
| `UT-SP-458-004` | Audit criticality mapper correctly translates all 4 typed Criticality values to audit enum | Pending |

### Tier 2: Integration Tests

**Testable code scope**: CRD schema validation, SP reconciliation with typed fields — target >=80%

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-SP-458-001` | CRD API server rejects a SignalProcessing CR with `criticality: "invalid"` | Pending |
| `IT-SP-458-002` | CRD API server rejects a SignalProcessing CR with `slaRequirement: "invalid"` | Pending |
| `IT-SP-458-003` | BusinessClassification with typed enum values round-trips through K8s API (create, read, verify) | Pending |

### Tier Skip Rationale

- **E2E**: Not applicable — shared type changes are fully validated by unit (logic) + integration (schema/round-trip). No end-to-end service behavior changes.

---

## 6. Test Cases (Detail)

### UT-SP-458-001: Criticality enum completeness

**BR**: BR-SP-081
**Type**: Unit
**File**: `test/unit/shared/types/business_classification_test.go`

**Given**: The `Criticality` type and its constants are defined
**When**: All documented criticality values from BR-SP-081 are checked
**Then**: Exactly 4 constants exist with values `critical`, `high`, `medium`, `low`

**Acceptance Criteria**:
- `CriticalityCritical` == `"critical"`
- `CriticalityHigh` == `"high"`
- `CriticalityMedium` == `"medium"`
- `CriticalityLow` == `"low"`

### UT-SP-458-002: SLARequirement enum completeness

**BR**: BR-SP-081
**Type**: Unit
**File**: `test/unit/shared/types/business_classification_test.go`

**Given**: The `SLARequirement` type and its constants are defined
**When**: All documented SLA tier values from BR-SP-081 are checked
**Then**: Exactly 4 constants exist with values `platinum`, `gold`, `silver`, `bronze`

**Acceptance Criteria**:
- `SLARequirementPlatinum` == `"platinum"`
- `SLARequirementGold` == `"gold"`
- `SLARequirementSilver` == `"silver"`
- `SLARequirementBronze` == `"bronze"`

### UT-SP-458-003: classifyBusiness returns typed constants per environment

**BR**: BR-SP-081
**Type**: Unit
**File**: `test/unit/signalprocessing/business_classification_test.go`

**Given**: An `EnvironmentClassification` with a known environment value
**When**: `classifyBusiness` is called
**Then**: Returned `BusinessClassification` uses typed constants matching the environment tier

**Acceptance Criteria**:
- `production` / `prod` -> `Criticality: CriticalityHigh, SLARequirement: SLARequirementGold`
- `staging` / `stage` -> `Criticality: CriticalityMedium, SLARequirement: SLARequirementSilver`
- `development` / `dev` -> `Criticality: CriticalityLow, SLARequirement: SLARequirementBronze`
- Default (nil envClass) -> `Criticality: CriticalityMedium, SLARequirement: SLARequirementBronze`

### UT-SP-458-004: Audit criticality mapper handles typed values

**BR**: BR-SP-081
**Type**: Unit
**File**: `test/unit/signalprocessing/audit_client_test.go`

**Given**: A `Criticality` typed value
**When**: `toSignalProcessingAuditPayloadCriticality` is called with `string(typed_value)`
**Then**: The correct audit enum value is returned for all 4 criticality levels

**Acceptance Criteria**:
- `string(CriticalityCritical)` -> `...CriticalityCritical` audit enum
- `string(CriticalityHigh)` -> `...CriticalityHigh` audit enum
- `string(CriticalityMedium)` -> `...CriticalityMedium` audit enum
- `string(CriticalityLow)` -> `...CriticalityLow` audit enum

### IT-SP-458-001: CRD rejects invalid criticality

**BR**: BR-SP-081
**Type**: Integration
**File**: `test/integration/signalprocessing/business_classification_integration_test.go`

**Given**: A SignalProcessing CR with `status.businessClassification.criticality: "invalid"`
**When**: The CR status is updated via the K8s API
**Then**: The API server rejects the update with a validation error

**Acceptance Criteria**:
- Status update returns an error
- Error message references the invalid enum value

### IT-SP-458-002: CRD rejects invalid slaRequirement

**BR**: BR-SP-081
**Type**: Integration
**File**: `test/integration/signalprocessing/business_classification_integration_test.go`

**Given**: A SignalProcessing CR with `status.businessClassification.slaRequirement: "invalid"`
**When**: The CR status is updated via the K8s API
**Then**: The API server rejects the update with a validation error

**Acceptance Criteria**:
- Status update returns an error
- Error message references the invalid enum value

### IT-SP-458-003: BusinessClassification round-trip with typed values

**BR**: BR-SP-081
**Type**: Integration
**File**: `test/integration/signalprocessing/business_classification_integration_test.go`

**Given**: A SignalProcessing CR with valid typed BusinessClassification values
**When**: The CR is created and read back from the K8s API
**Then**: All BusinessClassification fields preserve their typed enum values

**Acceptance Criteria**:
- `criticality` field value matches the typed constant used during creation
- `slaRequirement` field value matches the typed constant used during creation
- `businessUnit` and `serviceOwner` string fields are preserved

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None (pure logic tests)
- **Location**: `test/unit/shared/types/`, `test/unit/signalprocessing/`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (envtest with real K8s API)
- **Infrastructure**: envtest (CRD registration, API server)
- **Location**: `test/integration/signalprocessing/`

---

## 8. Execution

```bash
# Unit tests
go test ./test/unit/shared/types/... -ginkgo.focus="UT-SP-458"
go test ./test/unit/signalprocessing/... -ginkgo.focus="UT-SP-458"

# Integration tests
go test ./test/integration/signalprocessing/... -ginkgo.focus="IT-SP-458"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
