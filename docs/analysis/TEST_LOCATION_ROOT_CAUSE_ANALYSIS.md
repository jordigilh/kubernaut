# Test Location Violation - Root Cause Analysis

**Date**: 2025-10-09
**Incident**: Tests created in `internal/controller/remediation/` instead of `test/integration/remediation/`
**Severity**: Medium (violates documented standards, but tests are functional)

---

## Executive Summary

**Root Cause**: **AI-generated action plan contradicted project-specific testing strategy** by following Kubebuilder's default scaffolding pattern instead of Kubernaut's documented test location standards.

**Contributing Factors**:
1. Kubebuilder framework defaults to co-located tests (`internal/controller/*/suite_test.go`)
2. Existing codebase contains 5 scaffolded `suite_test.go` files in `internal/controller/` directories
3. Action plan explicitly specified wrong test paths (`go test ./internal/controller/remediation/...`)
4. APDC Analysis phase failed to validate test location against documented standards

**Impact**:
- Tests violate documented project standards
- Testing pyramid tiers (unit/integration/e2e) cannot be distinguished
- CI/CD pipeline test tier targeting broken
- Documentation-to-implementation mismatch

---

## Timeline of Events

### 1. Kubebuilder Scaffolding Created (Historical)

**Evidence**: All 5 CRD controllers have `suite_test.go` in `internal/controller/*/`:
```bash
$ find internal/controller -name "suite_test.go"
internal/controller/aianalysis/suite_test.go
internal/controller/kubernetesexecution/suite_test.go
internal/controller/remediation/suite_test.go
internal/controller/remediationprocessing/suite_test.go
internal/controller/workflowexecution/suite_test.go
```

**Kubebuilder Default Pattern**:
```bash
kubebuilder create api --group remediation --version v1alpha1 --kind RemediationRequest
# Creates: internal/controller/remediation/remediationrequest_controller.go
# Creates: internal/controller/remediation/suite_test.go  ← Default location
```

**Analysis**: Kubebuilder's scaffolding pattern places tests co-located with controllers by default.

---

### 2. Service Documentation Specified Correct Standards (Historical)

**Evidence**: All 5 service `testing-strategy.md` documents specify `test/{unit,integration,e2e}/`:

**RemediationOrchestrator** (`docs/services/crd-controllers/05-remediationorchestrator/testing-strategy.md:19`):
```markdown
**Test Directory**: [test/unit/](../../../test/unit/)
**Service Tests**: Create `test/unit/remediation/controller_test.go`
```

**RemediationProcessor** (`docs/services/crd-controllers/01-signalprocessing/testing-strategy.md:19`):
```markdown
**Test Directory**: [test/unit/](../../../test/unit/)
**Service Tests**: Create `test/unit/remediationprocessing/controller_test.go`
```

**AIAnalysis** (`docs/services/crd-controllers/02-aianalysis/testing-strategy.md:20`):
```markdown
**Test Directory**: [test/unit/aianalysis/](../../../test/unit/aianalysis/)
```

**WorkflowExecution** (`docs/services/crd-controllers/03-workflowexecution/testing-strategy.md:20`):
```markdown
**Test Directory**: [test/unit/workflowexecution/](../../../test/unit/workflowexecution/)
```

**KubernetesExecutor** (`docs/services/crd-controllers/04-kubernetesexecutor/testing-strategy.md:19`):
```markdown
**Test Directory**: [test/unit/](../../../test/unit/)
**Service Tests**: Create `test/unit/kubernetesexecutor/controller_test.go`
```

**Analysis**: 100% of service documentation correctly specifies project-specific test location standards.

---

### 3. Core Testing Strategy Rule Established (Historical)

**Evidence**: `.cursor/rules/03-testing-strategy.mdc:19-24`:
```markdown
### Unit Tests (70%+ - AT LEAST 70% of ALL BRs) - **MAXIMUM COVERAGE FOUNDATION LAYER**
**Location**: [test/unit/](mdc:test/unit/)
**Purpose**: **EXTENSIVE business logic validation covering ALL unit-testable business requirements**
**Coverage Mandate**: **AT LEAST 70% of total business requirements, extended to 100% of unit-testable BRs**
**Confidence**: 85-90%
**Execution**: `make test`
```

**Integration Tests** (`.cursor/rules/03-testing-strategy.mdc:72-78`):
```markdown
### Integration Tests (>50% - 100+ BRs) - **CROSS-SERVICE INTERACTION LAYER**
**Location**: [test/integration/](mdc:test/integration/)
**Purpose**: **Cross-service behavior, data flow validation, and microservices coordination**
**Coverage Mandate**: **>50% of total business requirements due to microservices architecture**
**Confidence**: 80-85%
**Execution**: `make test-integration-kind`
```

**Analysis**: Core testing strategy rule mandates `test/{unit,integration,e2e}/` structure.

---

### 4. Action Plan Created with Wrong Test Paths (2025-10-09)

**Evidence**: `docs/analysis/RR_CONTROLLER_IMPLEMENTATION_ACTION_PLAN.md:223-225`:
```markdown
6. **Check** (15 min)
   - [ ] Run unit test: `go test ./internal/controller/remediation/... -run TestReconcile_CreateAIAnalysisAfterProcessingCompletes`
   - [ ] Verify test passes
   - [ ] Run full test suite: `go test ./internal/controller/remediation/...`  ← WRONG PATH
```

**Also in lines 265, 311, 358, 402** (repeated pattern across all tasks).

**Analysis**: Action plan explicitly specified `internal/controller/remediation/` paths, contradicting:
- Service-level documentation (`test/integration/remediation/`)
- Core testing strategy rule (`test/{unit,integration,e2e}/`)
- All 5 existing service documentation standards

**Why This Happened**:
1. **Pattern Matching**: AI likely matched existing `internal/controller/*/suite_test.go` pattern from scaffolded files
2. **Kubebuilder Convention Bias**: Followed framework defaults instead of project-specific standards
3. **APDC Analysis Phase Gap**: Failed to validate test location during Analysis phase
4. **Documentation Hierarchy Confusion**: Prioritized code examples over documented standards

---

### 5. Implementation Followed Action Plan (2025-10-09)

**Evidence**: Git commit `a08ef2c`:
```bash
feat(controller): Implement RemediationRequest orchestrator Phase 1 core logic
...
Files Modified:
- internal/controller/remediation/remediationrequest_controller.go (+510 lines)
- internal/controller/remediation/remediationrequest_controller_test.go (NEW)  ← WRONG LOCATION
- internal/controller/remediation/suite_test.go (MODIFIED)
```

**Analysis**: Implementation correctly followed the action plan, but the action plan itself was wrong.

---

### 6. Tests Run and Fail Due to Validation Errors (2025-10-09)

**Evidence**: Test execution `go test ./internal/controller/remediation/... -v`:
```
FAIL: RemediationRequest.remediation.kubernaut.io "test-remediation-001" is invalid:
- spec.signalFingerprint: Invalid value (pattern mismatch)
- spec.deduplication.firstSeen: Required value
- spec.firingTime: Required value
- spec.receivedTime: Required value
```

**Analysis**: Tests ran from wrong location but failed due to CRD validation errors (separate issue).

---

### 7. User Identified Test Location Issue (2025-10-09)

**User Query**:
```
tests should be under the test/{unit,integration,e2e}.
Triage if the documentation in docs/services/crd-controllers/** includes this information
```

**Analysis**: User correctly identified the violation and requested documentation triage.

---

## Root Cause Breakdown

### Primary Root Cause: AI Action Plan Error

**Failure Point**: Action plan generation (Task 1.1, lines 223-225)

**What Should Have Happened**:
```markdown
6. **Check** (15 min)
   - [ ] Run integration test: `go test ./test/integration/remediation/... -v`  ← CORRECT
   - [ ] Verify test passes
   - [ ] Run full integration test suite: `go test ./test/integration/...`
```

**What Actually Happened**:
```markdown
6. **Check** (15 min)
   - [ ] Run unit test: `go test ./internal/controller/remediation/...`  ← WRONG
   - [ ] Verify test passes
   - [ ] Run full test suite: `go test ./internal/controller/remediation/...`
```

**Why AI Made This Error**:
1. **Code Pattern Bias**: Existing `internal/controller/*/suite_test.go` files created a visible pattern
2. **Kubebuilder Convention Influence**: Framework defaults weighted more heavily than project docs
3. **Documentation Hierarchy Failure**: Did not prioritize documented standards over code examples
4. **APDC Analysis Phase Gap**: Failed to execute checkpoint:
   ```
   ✅ TEST LOCATION VALIDATION CHECKPOINT:
   - [ ] Test directory follows `test/{tier}/{service}/` pattern ✅/❌
   - [ ] Test tier correctly identified (unit/integration/e2e) ✅/❌
   - [ ] Aligns with testing-strategy.md documentation ✅/❌
   ```

---

### Contributing Factors

#### Factor 1: Kubebuilder Scaffolding Defaults

**Impact**: Medium
**Explanation**: Kubebuilder creates `suite_test.go` in `internal/controller/` by default

**Evidence**:
```bash
$ kubebuilder create api --help
# Default test location: internal/controller/<kind>/<kind>_controller_test.go
```

**How This Contributed**:
- Created precedent of co-located tests
- 5 existing `suite_test.go` files reinforced the pattern
- No migration notes in scaffold files to indicate these should be moved

**Mitigation**: Add `.kubebuilder-scaffold-note.md` to `internal/controller/` directories:
```markdown
# ⚠️ IMPORTANT: Kubebuilder Scaffolding Note

These `suite_test.go` files are Kubebuilder scaffolding defaults.
**DO NOT** use this location for actual tests.

**Correct Test Locations**:
- Unit tests: `test/unit/{service}/`
- Integration tests: `test/integration/{service}/`
- E2E tests: `test/e2e/{service}/`

See: [.cursor/rules/03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc)
```

---

#### Factor 2: Test Tier Misclassification

**Impact**: Medium
**Explanation**: Action plan labeled integration tests as "unit tests"

**Evidence**: Action plan Task 1.1, step 3:
```markdown
3. **Do-RED** (1 hour)
   - [ ] Write unit test: `TestReconcile_CreateAIAnalysisAfterProcessingCompletes`  ← WRONG TIER
```

**Actual Test Behavior**:
```go
// Uses envtest (Kubernetes API server)
testEnv = &envtest.Environment{
    CRDDirectoryPaths: []string{...},
}

// Creates actual CRDs
Expect(k8sClient.Create(ctx, remediationRequest)).To(Succeed())

// Waits for controller reconciliation
Eventually(func() error {
    return k8sClient.Get(ctx, types.NamespacedName{...}, aiAnalysis)
}, timeout, interval).Should(Succeed())
```

**Correct Classification**: **Integration Test** (CRD lifecycle, controller reconciliation, envtest)

**How This Contributed**:
- "Unit test" label implied co-location with controller code
- Masked the fact these are actually integration tests requiring separate infrastructure
- Conflated "testing a controller" with "unit testing" (not the same thing)

---

#### Factor 3: APDC Analysis Phase Not Executed for Test Strategy

**Impact**: High
**Explanation**: Action plan skipped APDC Analysis checkpoint for test location validation

**What Was Missing**:

**APDC Analysis Phase - Test Strategy Validation**:
```markdown
#### Analysis Phase: Test Location & Tier Classification (15 min)

**MANDATORY VALIDATIONS**:

1. **Test Tier Classification**:
   - [ ] Uses envtest/Kind cluster? → Integration Test
   - [ ] Tests CRD lifecycle? → Integration Test
   - [ ] Tests controller reconciliation? → Integration Test
   - [ ] Uses only mocks? → Unit Test

2. **Test Location Validation**:
   - [ ] Review testing-strategy.md for service
   - [ ] Confirm `test/{tier}/{service}/` structure
   - [ ] Verify alignment with `.cursor/rules/03-testing-strategy.mdc`

3. **Documentation Cross-Check**:
   - [ ] Service testing-strategy.md specifies location
   - [ ] Core testing strategy rule specifies tier
   - [ ] No conflicts with existing test patterns
```

**Why This Was Skipped**:
- Action plan focused on business logic implementation
- Test infrastructure assumed rather than validated
- APDC methodology not fully applied to test strategy decisions

---

#### Factor 4: Insufficient Documentation Precedence

**Impact**: Medium
**Explanation**: Code examples (scaffolded files) weighted more heavily than documented standards

**Documentation Hierarchy That Should Apply**:
1. **Tier 1 - Core Rules**: `.cursor/rules/03-testing-strategy.mdc` (HIGHEST AUTHORITY)
2. **Tier 2 - Service Documentation**: `docs/services/crd-controllers/*/testing-strategy.md`
3. **Tier 3 - Code Examples**: Existing test files (LOWEST AUTHORITY, may be outdated/scaffolds)

**What Actually Happened**:
1. **Code Examples** (Tier 3) influenced decision
2. **Service Documentation** (Tier 2) was not consulted during action plan creation
3. **Core Rules** (Tier 1) were not validated

**How to Fix**:
Add to AI methodology:
```markdown
## Documentation Precedence Rule (MANDATORY)

When generating implementation plans:

1. **FIRST**: Validate against `.cursor/rules/` (HIGHEST AUTHORITY)
2. **SECOND**: Validate against service-level `docs/services/` documentation
3. **THIRD**: Use code examples (LOWEST AUTHORITY - may be scaffolds/legacy)

**BLOCKING CHECKPOINT**:
- [ ] Core rule validation completed ✅/❌
- [ ] Service documentation validation completed ✅/❌
- [ ] Code examples validated as non-scaffold/non-legacy ✅/❌

❌ STOP: Cannot proceed until ALL checkboxes are ✅
```

---

## Lessons Learned

### Lesson 1: Framework Defaults ≠ Project Standards

**Principle**: **ALWAYS validate framework scaffolding against project-specific standards**

**Application**:
- Kubebuilder scaffolds `internal/controller/*/suite_test.go` by default
- Kubernaut project requires `test/{tier}/{service}/` structure
- **Rule**: Project standards override framework defaults

**Prevention**:
```markdown
## APDC Analysis Phase - Framework vs Project Standards Validation

**TRIGGER**: Using scaffolded code or framework defaults

**MANDATORY VALIDATION**:
1. Identify framework scaffolding pattern
2. Search for project-specific documentation overriding defaults
3. Prioritize project standards over framework conventions
4. Document any deviations with justification
```

---

### Lesson 2: Test Tier Classification Is Critical

**Principle**: **Test tier (unit/integration/e2e) determines location, not what is being tested**

**Incorrect Thinking**:
- "Testing a controller" → "unit test" → co-locate with controller
- **WRONG**: This is environmental, not behavioral classification

**Correct Thinking**:
- Uses envtest? → Integration test → `test/integration/{service}/`
- Uses Kind cluster? → E2E test → `test/e2e/{service}/`
- Uses only mocks? → Unit test → `test/unit/{service}/`

**Decision Matrix**:
| Test Characteristic | Classification | Location |
|---|---|---|
| Uses envtest | Integration | `test/integration/{service}/` |
| Uses Kind/OCP cluster | E2E | `test/e2e/{service}/` |
| Creates real CRDs | Integration | `test/integration/{service}/` |
| Tests reconciliation loops | Integration | `test/integration/{service}/` |
| Uses only mocks/fakes | Unit | `test/unit/{service}/` |
| Tests business logic only | Unit | `test/unit/{service}/` |

---

### Lesson 3: APDC Analysis Must Include Test Strategy

**Principle**: **Test strategy decisions require same rigor as business logic decisions**

**What Was Missing**:
- Test location validation in Analysis phase
- Test tier classification in Plan phase
- Documentation cross-check before Do phase

**What Should Happen**:

**APDC Analysis Phase**:
```markdown
### Analysis: Test Strategy Validation (15 min)

**MANDATORY CHECKPOINTS**:
1. Review `.cursor/rules/03-testing-strategy.mdc` for tier definitions
2. Review `docs/services/crd-controllers/{service}/testing-strategy.md` for service-specific location
3. Classify test tier (unit/integration/e2e) based on infrastructure requirements
4. Validate location follows `test/{tier}/{service}/` pattern
5. Confirm no conflicts with existing test organization
```

---

### Lesson 4: Documentation Hierarchy Must Be Explicit

**Principle**: **Core rules > Service docs > Code examples**

**Implementation**:
```markdown
## AI Validation Sequence (MANDATORY)

When generating implementation plans, validate in this order:

### Step 1: Core Rule Validation (BLOCKING)
- [ ] Search `.cursor/rules/` for relevant standards
- [ ] Confirm plan aligns with core rules
- [ ] Document any rule conflicts for user approval

### Step 2: Service Documentation Validation (BLOCKING)
- [ ] Search `docs/services/crd-controllers/{service}/` for service-specific standards
- [ ] Confirm plan aligns with service documentation
- [ ] Document any service-specific requirements

### Step 3: Code Example Reference (INFORMATIONAL ONLY)
- [ ] Check existing code for examples
- [ ] **VALIDATE**: Are code examples scaffolds or production code?
- [ ] **RULE**: Only use non-scaffold examples; ignore framework defaults

❌ STOP: If Steps 1 or 2 fail, halt and request clarification
```

---

## Immediate Corrective Actions

### Action 1: Move Tests to Correct Location (PRIORITY: P0)

**Task**: Implement remediation plan from `TEST_LOCATION_STANDARDS_TRIAGE.md`

**Estimated Effort**: 1.2 hours

**Steps**:
1. Create `test/integration/remediation/` directory
2. Move `internal/controller/remediation/*_test.go` → `test/integration/remediation/`
3. Update package to `remediation_test`
4. Update `suite_test.go` CRD paths and controller setup
5. Update documentation references
6. Add Makefile targets

---

### Action 2: Update Action Plan Documentation (PRIORITY: P1)

**Task**: Fix action plan to reference correct test paths

**File**: `docs/analysis/RR_CONTROLLER_IMPLEMENTATION_ACTION_PLAN.md`

**Changes**:
```diff
- Run unit test: `go test ./internal/controller/remediation/... -v`
+ Run integration test: `go test ./test/integration/remediation/... -v`
```

**Lines to update**: 223, 225, 265, 267, 311, 313, 358, 360, 402, 404

---

### Action 3: Add Scaffold Warning Files (PRIORITY: P2)

**Task**: Prevent future confusion about scaffolded `suite_test.go` files

**Create**: `internal/controller/.SCAFFOLDING-README.md`

```markdown
# ⚠️ Kubebuilder Scaffolding Directory

Files in this directory are **Kubebuilder scaffolding only**.

## Test Files (suite_test.go)

**DO NOT** add production tests to `internal/controller/*/suite_test.go`.

**Correct Test Locations**:
- **Unit Tests**: `test/unit/{service}/`
- **Integration Tests**: `test/integration/{service}/`
- **E2E Tests**: `test/e2e/{service}/`

## Why Are These Files Here?

Kubebuilder scaffolding creates `suite_test.go` files by default.
These files should either:
1. Remain empty (placeholder for future migration)
2. Be deleted once tests are in correct locations

See: [Testing Strategy](.cursor/rules/03-testing-strategy.mdc)
```

---

### Action 4: Enhance AI Validation Rules (PRIORITY: P1)

**Task**: Add test location validation checkpoint to AI methodology

**File**: `.cursor/rules/00-ai-assistant-methodology-enforcement.mdc`

**Add New Checkpoint**:

```markdown
#### **CHECKPOINT E: Test Location & Tier Validation**
**TRIGGER**: About to create any test file (*_test.go)

🚫 **BLOCKING REQUIREMENT - AI MUST EXECUTE BEFORE PROCEEDING**:

<function_calls>
<invoke name="read_file">
<parameter name="file_path">.cursor/rules/03-testing-strategy.mdc</parameter>
</invoke>
</function_calls>

<function_calls>
<invoke name="grep">
<parameter name="pattern">Test Directory|**Location**</parameter>
<parameter name="path">docs/services/crd-controllers/{service}/testing-strategy.md</parameter>
<parameter name="output_mode">content</parameter>
</invoke>
</function_calls>

```
✅ TEST LOCATION VALIDATION CHECKPOINT:
- [ ] Test tier classified (unit/integration/e2e) ✅/❌
- [ ] Infrastructure requirements identified (envtest/Kind/mocks) ✅/❌
- [ ] Test location follows `test/{tier}/{service}/` pattern ✅/❌
- [ ] Aligns with service testing-strategy.md ✅/❌
- [ ] Aligns with core testing-strategy rule ✅/❌
- [ ] NOT using framework scaffolding location ✅/❌

❌ STOP: Cannot create test files until ALL checkboxes are ✅
```

**RULE VIOLATION DETECTION**:
If ANY checkbox is ❌ → "🚨 TEST LOCATION VIOLATION: Test file creation attempted without location validation - DEVELOPMENT STOPPED"
If test path contains `internal/controller` → "🚨 SCAFFOLD LOCATION VIOLATION: Tests must use `test/{tier}/{service}/` structure - DEVELOPMENT STOPPED"
```

---

## Prevention Strategy

### Strategy 1: Automated Test Location Validation

**Implementation**: Add pre-commit hook

**File**: `.git/hooks/pre-commit`

```bash
#!/bin/bash
# Prevent test files in internal/controller directories

INVALID_TESTS=$(git diff --cached --name-only | grep "internal/controller/.*_test\.go$" | grep -v suite_test.go)

if [ -n "$INVALID_TESTS" ]; then
    echo "❌ ERROR: Test files in invalid location:"
    echo "$INVALID_TESTS"
    echo ""
    echo "Tests must be in: test/{unit,integration,e2e}/{service}/"
    echo "See: .cursor/rules/03-testing-strategy.mdc"
    exit 1
fi

exit 0
```

---

### Strategy 2: Documentation-First Test Planning

**Process**:
1. **BEFORE** writing action plan: Read `docs/services/crd-controllers/{service}/testing-strategy.md`
2. **MANDATORY**: Include "Test Strategy Analysis" section in action plan
3. **VALIDATE**: Test location follows documented standards

**Template**:
```markdown
### Test Strategy Analysis (MANDATORY)

**Service**: {service_name}
**Documentation Reference**: `docs/services/crd-controllers/{service}/testing-strategy.md`

**Test Tier Classification**:
- Infrastructure: [envtest/Kind/mocks]
- Tier: [unit/integration/e2e]
- Location: `test/{tier}/{service}/`

**Validation**:
- [x] Aligns with service testing-strategy.md
- [x] Aligns with core testing-strategy rule
- [x] Not using framework scaffolding location
```

---

### Strategy 3: Explicit Framework Override Documentation

**Where**: Service README files

**Add Section**:
```markdown
## ⚠️ Kubebuilder Scaffolding vs Project Standards

**Kubebuilder Default** (DO NOT USE):
```
internal/controller/{service}/
├── controller.go
├── controller_test.go        ❌ WRONG LOCATION
└── suite_test.go              ❌ WRONG LOCATION
```

**Kubernaut Standard** (USE THIS):
```
internal/controller/{service}/
└── controller.go              ✅ Implementation only

test/unit/{service}/
├── business_logic_test.go     ✅ Unit tests
└── suite_test.go              ✅ Test setup

test/integration/{service}/
├── crd_lifecycle_test.go      ✅ Integration tests
└── suite_test.go              ✅ envtest setup
```

**Why**: Kubernaut uses testing pyramid strategy requiring test tier separation.
See: [Testing Strategy](.cursor/rules/03-testing-strategy.mdc)
```

---

## Success Metrics

### Immediate (1 week)
- [ ] All RemediationRequest controller tests moved to `test/integration/remediation/`
- [ ] Action plan updated with correct test paths
- [ ] Scaffold warning files added to `internal/controller/`

### Short-term (1 month)
- [ ] AI validation rule enhanced with test location checkpoint
- [ ] Pre-commit hook prevents invalid test locations
- [ ] Service README files document scaffolding overrides

### Long-term (3 months)
- [ ] All 5 CRD controller test suites migrated to `test/{tier}/{service}/`
- [ ] Scaffolded `suite_test.go` files deleted or marked deprecated
- [ ] CI/CD pipeline targets correct test directories independently

---

## Confidence Assessment

**Root Cause Identification**: 95% confidence
- Clear evidence of action plan error
- Multiple contributing factors documented
- Timeline reconstructed from git history and documentation

**Remediation Plan Viability**: 90% confidence
- Straightforward file relocation
- No business logic changes required
- Clear validation checkpoints

**Prevention Strategy Effectiveness**: 85% confidence
- Pre-commit hook prevents technical violations
- Enhanced AI rules address process gaps
- Documentation improvements clarify standards

**Risk Level**: Low
- No business logic impact
- Purely organizational refactoring
- Clear rollback path if needed

---

## References

- [Testing Strategy Rule](../../.cursor/rules/03-testing-strategy.mdc) - Core testing standards
- [Test Location Triage](./TEST_LOCATION_STANDARDS_TRIAGE.md) - Detailed remediation plan
- [RemediationOrchestrator Testing Strategy](../services/crd-controllers/05-remediationorchestrator/testing-strategy.md) - Service-specific standards
- [Action Plan](./RR_CONTROLLER_IMPLEMENTATION_ACTION_PLAN.md) - Plan that specified wrong paths
- [Kubebuilder Documentation](https://book.kubebuilder.io/cronjob-tutorial/writing-tests.html) - Framework testing conventions

