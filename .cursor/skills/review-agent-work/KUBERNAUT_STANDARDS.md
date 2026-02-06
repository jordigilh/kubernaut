# Kubernaut Development Standards Reference

## Business Requirements System

### Format
`BR-[CATEGORY]-[NUMBER]`

### Categories
- WORKFLOW - Workflow management features
- AI - AI/ML analysis features
- INTEGRATION - External system integrations
- SECURITY - Security and access control
- PLATFORM - Platform infrastructure
- API - API design and contracts
- STORAGE - Data persistence
- MONITORING - Observability features
- SAFETY - Operational safety measures
- PERFORMANCE - Performance optimization

### Validation
Every code change must:
- Reference at least one business requirement
- Have tests that map to the BR
- Serve documented business needs only

---

## TDD Methodology Details

### RED Phase
**Purpose**: Define the business contract through tests

**Requirements**:
- Write failing tests FIRST
- Validate **business outcomes with correctness and behavior** (NOT implementation details)
- Use Ginkgo/Gomega BDD framework
- Include BR or test scenario ID in test description
- Cover edge cases and error conditions
- Use table-driven tests where appropriate

**Anti-patterns**:
- Writing implementation before tests
- Testing implementation logic (e.g., "helper X called 3 times")
- Using standard Go testing instead of Ginkgo/Gomega
- Tests without business context

### GREEN Phase
**Purpose**: Minimal implementation to pass tests

**Requirements**:
- Simplest possible implementation
- **MANDATORY**: Integrate with main application (cmd/)
- Handle errors appropriately
- Pass all tests

**Anti-patterns**:
- Sophisticated/complex logic (belongs in REFACTOR)
- New types not integrated with main apps
- Ignoring errors

### REFACTOR Phase
**Purpose**: Enhance code quality while maintaining passing tests

**Requirements**:
- Improve existing code structure
- Reduce duplication
- Enhance patterns (don't create new)
- Maintain all passing tests
- Validate build after changes

**Anti-patterns**:
- Creating new types in REFACTOR
- Skipping this phase
- Breaking tests during refactoring
- Refactoring without build validation

---

## AI Validation Checkpoints

### CHECKPOINT A: Type Reference Validation
**Trigger**: Any struct field access (object.FieldName)

**Validation**: Read type definition file BEFORE referencing

```bash
# Verify field exists
cat pkg/path/to/type_definition.go
# Check struct definition includes the field
```

### CHECKPOINT B: Test Creation Validation
**Trigger**: Creating test files

**Validation**: Search existing implementations FIRST

```bash
# Find existing patterns
rg "ComponentType" pkg/ --include="*.go"
# Enhance existing patterns, don't reinvent
```

### CHECKPOINT C: Business Integration Validation
**Trigger**: New business types or interfaces

**Validation**: Verify main application integration

```bash
# Must find integration
grep -r "NewComponentType" cmd/ --include="*.go"
# If zero results = violation
```

### CHECKPOINT D: Build Error Investigation
**Trigger**: Compilation or undefined symbol errors

**Validation**: Complete analysis before fixing

**Required**:
1. Analyze ALL symbol references
2. Map complete dependency chain
3. Present options A/B/C to user
4. Get approval before implementing

---

## Testing Framework Requirements

### Ginkgo/Gomega BDD
**Mandatory**: All tests use BDD framework

**Structure**:
```go
Describe("Component Name", func() {
    Context("specific scenario", func() {
        It("should do expected behavior [BR-CAT-NUM]", func() {
            // Test implementation
            Expect(result).To(Equal(expected))
        })
    })
})
```

### Test File Structure
**Unit Tests**: `test/unit/{service}/` (NOT colocated with code)
- Package name: `{service}` (NO `_test` suffix)
- Example: `test/unit/datastorage/config_test.go` → `package datastorage`

**Integration Tests**: `test/integration/{service}/`
- Package name: `{service}` (NO `_test` suffix)
- Example: `test/integration/datastorage/suite_test.go` → `package datastorage`

**E2E Tests**: `test/e2e/{service}/`
- Package name: `{service}` (NO `_test` suffix)
- Example: `test/e2e/datastorage/01_basic_crud_test.go` → `package datastorage`

### Test Identification
Tests must include ONE of:
1. **Preferred**: Test Scenario ID (e.g., UT-WF-197-001)
2. **Fallback**: Business Requirement (e.g., BR-WORKFLOW-197)

### Defense-in-Depth Coverage Strategy

Kubernaut uses **overlapping BR coverage** and **cumulative code coverage**:

#### BR Coverage (Overlapping)
- **Unit Tests**: 70%+ of all BRs (all unit-testable business requirements)
- **Integration Tests**: >50% of all BRs (cross-service coordination)
- **E2E Tests**: <10% BR coverage (critical user journeys only)

#### Code Coverage (Cumulative - ~100% combined)
- **Unit Tests**: 70%+ (algorithm correctness, edge cases, error handling)
- **Integration Tests**: 50% (cross-component flows, CRD operations, real K8s API)
- **E2E Tests**: 50% (full stack: main.go, reconciliation, business logic, metrics, audit)

**Key Insight**: With 70%/50%/50% code coverage targets, **50%+ of codebase is tested in ALL 3 tiers** - ensuring bugs must slip through multiple defense layers to reach production.

### Test Infrastructure Strategy

| Test Tier | K8s Environment | Services | Infrastructure |
|-----------|-----------------|----------|----------------|
| **Unit** | None | **Mocked** | None required |
| **Integration** | envtest | **REAL** (PostgreSQL, Redis, HolmesGPT-API) | Programmatic Go via `test/infrastructure/` |
| **E2E** | KIND cluster | **REAL** (full deployment) | KIND + deployed services |

**LLM Exception**: Mock LLM in ALL tiers due to cost constraints

**Integration Test Rules**:
- ✅ Use REAL services (PostgreSQL, Redis, K8s envtest, Data Storage, HolmesGPT-API)
- ✅ Orchestrated via programmatic Go (`test/infrastructure/container_management.go`)
- ✅ Direct business logic calls (NO HTTP)
- ✅ Mock LLM only (cost constraint)

**Unit Test Rules**:
- ✅ Mock all external dependencies
- ✅ Use real business logic from `pkg/`
- ✅ Fast, isolated, no I/O

---

## Code Quality Standards

### Error Handling
**Requirements**:
- Handle ALL errors (never ignore)
- Log every error with context
- Use structured error types from `internal/errors/`
- Wrap with context: `fmt.Errorf("context: %w", err)`

### Type System
**Requirements**:
- Avoid `any` or `interface{}` unless necessary
- Use structured field values with specific types
- Avoid local type definitions (use `pkg/shared/types/`)
- No empty struct definitions

### Business Integration
**Requirements**:
- All business code in `pkg/` must be used in `cmd/`
- Remove unused/orphaned code
- Seamless integration with existing architecture

---

## Integration Validation Patterns

### Pattern 1: Verify Main App Usage
```bash
# Check component appears in main applications
grep -r "ComponentName" cmd/ --include="*.go"
# Should find: instantiation, configuration, or usage
```

### Pattern 2: Dependency Chain Check
```bash
# After refactoring, verify no broken references
rg "OldFieldName|OldTypeName" . --include="*.go"
# Should find: zero results if fully refactored
```

### Pattern 3: Build Validation
```bash
# Build all services
make build-all-services

# Quick compile check (unit tests only, no execution)
make test-tier-unit
```

---

## Refactoring Safety

### Post-Refactor Validation
**MANDATORY** after any refactoring:

1. **Full build**: `make build-all-services`
2. **Unit tests**: `make test-tier-unit` (validates compilation)
3. **Reference search**: `rg "OldName" . --include="*.go"`

### Common Refactoring Pitfalls
- Field renames with missed references
- Type changes breaking dependent code
- Signature updates without caller updates
- Package moves without import updates

**Rule**: Treat refactoring as HIGH RISK for build failures

---

## Confidence Assessment Guidelines

### Confidence Levels

**90-100%**: High Confidence
- Implementation follows established patterns
- All tests pass with good coverage
- Integrates cleanly with existing code
- No assumptions or uncertainties

**75-89%**: Medium-High Confidence
- Generally solid implementation
- Minor risks or edge cases remain
- Some assumptions made but validated
- Minor performance or maintainability concerns

**60-74%**: Medium Confidence
- Implementation works but has concerns
- Some validation gaps
- Assumptions not fully validated
- Potential for issues in edge cases

**Below 60%**: Low Confidence (requires escalation)
- Significant uncertainties remain
- Major assumptions not validated
- High risk of issues
- Incomplete testing

### Confidence Justification
Must include:
1. **What's validated**: Specific checks performed
2. **Risks identified**: Potential issues or concerns
3. **Assumptions made**: What's assumed without validation
4. **Validation approach**: How correctness was verified

---

## Common Violations

### Critical (Blocking)
- ❌ No tests written (RED phase skipped)
- ❌ Tests validate implementation logic instead of business behavior/outcomes
- ❌ Skip() used to avoid test failures
- ❌ Business code not integrated in cmd/
- ❌ Build errors present
- ❌ Missing business requirement mapping

### Warnings (Should Fix)
- ⚠️ Sophisticated logic in GREEN phase
- ⚠️ New patterns instead of enhancing existing
- ⚠️ Low test coverage (<70% unit tests)
- ⚠️ Unhandled errors or missing logs
- ⚠️ Using `any` or `interface{}` unnecessarily

---

## When to Ask for User Input

**MANDATORY: Ask for user input immediately when:**

### Uncertainty About Standards
- Unclear if a pattern meets Kubernaut guidelines
- Test coverage threshold ambiguous for specific component
- Integration pattern differs from existing code (intentional or violation?)
- Unsure if issue is blocking vs. warning severity

### Ambiguous Requirements
- Business requirement interpretation is unclear
- Acceptance criteria not explicitly documented
- Multiple valid approaches and unclear which was intended
- Scope boundary is fuzzy

### Technical Ambiguity
- Cannot determine if integration exists without assumptions
- Expected behavior not documented
- Performance/quality thresholds not specified
- Architecture decisions lack clear rationale

### General Principle
**If thinking "I assume..." or "Probably..." → STOP and ASK**

**DO NOT**:
- Make assumptions and continue review
- Apply arbitrary thresholds without project context
- Guess at user/developer intent
- Complete review with unresolved uncertainties

**ALWAYS prefer asking over guessing.**

---

## Escalation Triggers

Report to user immediately if:

1. **Tests don't validate behavior** - Tests missing, all skipped, or testing implementation logic instead of business outcomes/behavior
2. **Orphaned components** - Business code with no main app integration
3. **Critical safety issues** - Security, data loss, or operational risks
4. **Scope significantly exceeds plan** - Major architectural changes
5. **Build failures** - Code doesn't compile
6. **Confidence below 80%** - Too many uncertainties
7. **ANY uncertainty during review** - When clarification needed to proceed

**Key Testing Principle**: Tests must validate **business outcomes with correctness and behavior**, NEVER implementation details (e.g., internal helper call counts, private method invocations).
