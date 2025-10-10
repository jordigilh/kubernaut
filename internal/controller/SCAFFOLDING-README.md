# ⚠️ Kubebuilder Scaffolding Directory

**IMPORTANT**: Files in this directory are Kubebuilder scaffolding only.

---

## Test Files (suite_test.go)

**DO NOT** add production tests to `internal/controller/*/suite_test.go`.

### ❌ Wrong Location (Kubebuilder Default)
```
internal/controller/{service}/
├── controller.go
├── controller_test.go        ❌ WRONG - Do not add tests here
└── suite_test.go              ❌ SCAFFOLDING ONLY
```

### ✅ Correct Test Locations (Kubernaut Standard)

```
test/unit/{service}/
├── business_logic_test.go     ✅ Unit tests (70%+ coverage)
└── suite_test.go              ✅ Test setup with mocks

test/integration/{service}/
├── crd_lifecycle_test.go      ✅ Integration tests (>50% coverage)
├── controller_orchestration_test.go
└── suite_test.go              ✅ envtest setup with controller

test/e2e/{service}/
├── complete_workflow_test.go  ✅ E2E tests (<10% coverage)
└── suite_test.go              ✅ Real cluster setup
```

---

## Why Are suite_test.go Files Here?

Kubebuilder scaffolding creates `suite_test.go` files in `internal/controller/` by default:

```bash
$ kubebuilder create api --group remediation --version v1alpha1 --kind RemediationRequest
# Creates: internal/controller/remediation/remediationrequest_controller.go
# Creates: internal/controller/remediation/suite_test.go  ← Default location (WRONG for Kubernaut)
```

**These files should**:
1. Remain empty (placeholder for future migration)
2. Be deleted once tests are in correct locations
3. **NOT** be used for actual production tests

---

## Kubernaut Testing Strategy

**Test Tier Classification**:

| Test Type | Infrastructure | Location | Coverage Target |
|-----------|---------------|----------|-----------------|
| **Unit Tests** | Mocks only | `test/unit/{service}/` | 70%+ of BRs |
| **Integration Tests** | envtest/Kind | `test/integration/{service}/` | >50% of BRs |
| **E2E Tests** | Real cluster | `test/e2e/{service}/` | <10% of BRs |

**Decision Matrix**:
- Uses envtest? → **Integration test** → `test/integration/{service}/`
- Creates real CRDs? → **Integration test** → `test/integration/{service}/`
- Tests reconciliation loops? → **Integration test** → `test/integration/{service}/`
- Uses only mocks/fakes? → **Unit test** → `test/unit/{service}/`
- Tests business logic only? → **Unit test** → `test/unit/{service}/`

---

## Framework Defaults vs Project Standards

**Kubebuilder Default** (DO NOT FOLLOW):
- Tests co-located with controllers in `internal/controller/`
- Same package as controller (`package remediation`)
- Mixed test tiers (unit/integration/e2e)

**Kubernaut Standard** (FOLLOW THIS):
- Tests separated by tier in `test/{tier}/{service}/`
- Black-box testing (`package remediation_test`)
- Clear test tier separation for CI/CD targeting

---

## Makefile Targets

**Run tests by tier**:
```bash
# Unit tests only
make test

# Integration tests only
make test-integration

# Specific service integration tests
make test-integration-remediation

# E2E tests
make test-e2e
```

---

## References

- **Core Testing Strategy**: [.cursor/rules/03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc)
- **Service-Specific Testing**: `docs/services/crd-controllers/{service}/testing-strategy.md`
- **Test Location Triage**: [docs/analysis/TEST_LOCATION_STANDARDS_TRIAGE.md](../../docs/analysis/TEST_LOCATION_STANDARDS_TRIAGE.md)
- **Root Cause Analysis**: [docs/analysis/TEST_LOCATION_ROOT_CAUSE_ANALYSIS.md](../../docs/analysis/TEST_LOCATION_ROOT_CAUSE_ANALYSIS.md)

---

## Migration Status

| Controller | suite_test.go Status | Tests Migrated | Target Location |
|------------|---------------------|----------------|-----------------|
| **RemediationRequest** | ✅ Migrated | Yes | `test/integration/remediation/` |
| **RemediationProcessing** | 🔄 Scaffold only | No | `test/integration/remediationprocessing/` |
| **AIAnalysis** | 🔄 Scaffold only | No | `test/integration/aianalysis/` |
| **WorkflowExecution** | 🔄 Scaffold only | No | `test/integration/workflowexecution/` |
| **KubernetesExecution** | 🔄 Scaffold only | No | `test/integration/kubernetesexecution/` |

---

## Quick Reference

**When creating new tests**:
1. ✅ Classify test tier (unit/integration/e2e)
2. ✅ Create tests in `test/{tier}/{service}/`
3. ✅ Use black-box testing (`package {service}_test`)
4. ✅ Follow service-specific `testing-strategy.md`
5. ❌ DO NOT add tests to `internal/controller/{service}/`

**Why this matters**:
- **Testing Pyramid**: Enables proper unit/integration/e2e separation
- **CI/CD Targeting**: Allows independent test tier execution
- **Documentation Alignment**: Code matches documented standards
- **Package Visibility**: Proper black-box testing patterns

