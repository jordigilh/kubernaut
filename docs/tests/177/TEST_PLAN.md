# Test Plan: DD-SP-001 Vestigial Cleanup

**Feature**: Remove vestigial `configmap` source, confidence references, and dead pattern-match test from SignalProcessing integration tests
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `development/v1.2`

**Authority**:
- [DD-SP-001]: Remove Classification Confidence Scores from SignalProcessing
- [BR-SP-080 V2.0]: Classification Source Tracking (replaces confidence scoring)
- [BR-SP-002 V2.0]: Business Classification (confidence requirement removed)

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Issue #177](https://github.com/jordigilh/kubernaut/issues/177)

---

## 1. Scope

### In Scope

- **Integration test Rego fixtures**: Rename legacy `"configmap"` source to `"rego-inference"` in embedded Rego policies
- **Integration test assertions**: Update source value assertions to match renamed Rego output
- **Dead test removal**: Remove `business-pattern` integration test that tests a code path no longer in the controller
- **Fingerprint key consistency**: Rename map keys to reflect Rego-based classification
- **Test description accuracy**: Update descriptions where "ConfigMap" implies classification mechanism
- **Documentation**: Remove stale references to non-existent files and deprecated confidence values

### Out of Scope

- Production code changes (controller, CRD types, deployed Rego policies are already clean)
- `rego_integration_test.go`, `hot_reloader_test.go`, `severity_integration_test.go` descriptions that correctly reference K8s ConfigMap resources
- Unit tests (no unit-testable code changes)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Rename `configmap` -> `rego-inference` only in test Rego fixtures | Production Rego policies already use correct source names; test fixtures are stale |
| Keep "ConfigMap" in test names about K8s resource operations | Tests in rego_integration_test.go and hot_reloader_test.go correctly describe ConfigMap loading/hot-reload |
| Remove `business-pattern` test entirely | Tests pattern-match tier removed by ADR-060; passes trivially (only asserts Phase==Completed) |

---

## 2. Coverage Policy

### Tier Skip Rationale

- **Unit**: SKIPPED -- No unit-testable production code changes. All changes are in integration test fixtures, test descriptions, and documentation.
- **E2E**: SKIPPED -- No behavioral change in production code. E2E tests will continue to pass unchanged.

### Integration Tier

Existing integration tests are being MODIFIED (not new tests). Validation is:
1. Build succeeds after fingerprint key renames (`go build ./...`)
2. Renamed Rego source strings produce correct classification results (existing test logic validates this)
3. No dangling references to removed `business-pattern` fingerprint key

---

## 3. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-SP-080 | Source tracking: `rego-inference` replaces `configmap` | P1 | Integration | IT-SP-080-001 | Pending |
| BR-SP-080 | Source tracking: assertion validates `rego-inference` | P1 | Integration | IT-SP-080-002 | Pending |
| BR-SP-002 | Dead pattern-match test removed | P2 | Integration | IT-SP-002-001 | Pending |
| DD-SP-001 | Documentation reflects implemented state | P2 | N/A | DOC-SP-001-001 | Pending |

---

## 4. Test Scenarios

### IT-SP-080-001: Rego fixture source rename produces correct classification

**BR**: BR-SP-080
**Type**: Integration (existing test modification)
**Files**: `test/integration/signalprocessing/suite_test.go`, `hot_reloader_test.go`

**Given**: Rego policy fixtures emit `"source": "rego-inference"` for namespace-pattern-based classification
**When**: Integration tests run with renamed source values
**Then**: Environment classification produces correct `environment` value AND `source` field equals `"rego-inference"`

**Validation**: Existing test `BR-SP-052` in `reconciler_integration_test.go` asserts `Source == "rego-inference"` (updated from `"configmap"`)

### IT-SP-080-002: Fingerprint key rename has no compilation errors

**BR**: BR-SP-080
**Type**: Build validation
**Files**: `test/integration/signalprocessing/test_helpers.go`, `component_integration_test.go`

**Given**: Fingerprint keys renamed from `env-configmap` -> `env-rego`, `priority-cm` -> `priority-rego`
**When**: `go build ./...` is executed
**Then**: No compilation errors; all map key references resolve correctly

### IT-SP-002-001: Dead business-pattern test removed without side effects

**BR**: BR-SP-002
**Type**: Integration (test removal)
**File**: `test/integration/signalprocessing/component_integration_test.go`

**Given**: `business-pattern` test and fingerprint key are removed
**When**: `go build ./...` is executed
**Then**: No dangling references to removed key; remaining tests compile and maintain correct structure

---

## 5. Execution

```bash
# Build validation (primary)
go build ./...

# Integration tests (if CI environment available)
make test-integration-signalprocessing
```

---

## 6. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan for DD-SP-001 vestigial cleanup |
