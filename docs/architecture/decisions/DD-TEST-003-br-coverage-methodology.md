# DD-TEST-003: Business Requirement Test Coverage Methodology

**Status**: ✅ **APPROVED**
**Date**: 2025-11-28
**Related**: ADR-005 (Integration Test Coverage), ADR-037 (BR Template Standard)
**Applies To**: ALL Kubernaut services (universal standard)
**Confidence**: 90%

---

## Context & Problem

Kubernaut uses Business Requirements (BR-[CATEGORY]-[NUMBER]) to drive all implementation. Each BR must have corresponding tests, but there's no standardized way to:

1. **Calculate coverage** - What formula to use?
2. **Determine minimum tests** - How many tests per BR?
3. **Report coverage** - What format for tracking?
4. **Validate completeness** - How to ensure no gaps?

**Key Questions**:
1. What is the minimum number of tests per BR?
2. How do we track BR coverage across test tiers?
3. What is the reporting format for implementation plans?

---

## Decision

**APPROVED: Standardized BR Coverage Calculation and Reporting**

### BR Coverage Formula

```
BR Coverage = (BRs with at least 1 test / Total BRs) × 100%

Target: 100% of BRs MUST have at least 1 test
```

### Minimum Test Counts by Complexity

| BR Complexity | Unit Tests | Integration Tests | E2E Tests | Minimum Total |
|---------------|------------|-------------------|-----------|---------------|
| **Low** | 3+ | 1+ | 0-1 | 5+ |
| **Medium** | 8+ | 3+ | 1 | 12+ |
| **High** | 15+ | 6+ | 2+ | 23+ |

#### Complexity Classification

| Complexity | Criteria | Examples |
|------------|----------|----------|
| **Low** | Single component, no external deps, < 3 code paths | Config validation, simple formatting |
| **Medium** | Multiple components, 3-5 code paths, 1 external dep | CRUD operations, basic workflows |
| **High** | Cross-service, 6+ code paths, multiple external deps | Reconciliation loops, classification engines |

---

## Reporting Format

### BR Coverage Matrix (Required in Implementation Plans)

```markdown
### Detailed BR Test Coverage

| BR ID | Description | Complexity | Unit | Int | E2E | Total | Status |
|-------|-------------|------------|------|-----|-----|-------|--------|
| BR-SP-001 | K8s Context Enrichment | High | 15 | 8 | 2 | 25 | ✅ |
| BR-SP-002 | Business Classification | High | 12 | 4 | 1 | 17 | ✅ |
| BR-SP-003 | Recovery Context | Medium | 8 | 6 | 2 | 16 | ✅ |
| BR-SP-051 | Env Classification (Primary) | Medium | 10 | 4 | 1 | 15 | ✅ |
| BR-SP-052 | Env Classification (Fallback) | Low | 8 | 2 | 1 | 11 | ✅ |
| BR-SP-053 | Env Classification (Default) | Low | 6 | 2 | 1 | 9 | ✅ |
| **TOTAL** | | | **59** | **26** | **8** | **93** | ✅ 100% |

**Coverage Calculation**: 6/6 BRs with tests = **100% BR Coverage**
```

### Status Indicators

| Status | Meaning | Action Required |
|--------|---------|-----------------|
| ✅ | Meets minimum test count | None |
| ⚠️ | Below minimum but has tests | Add more tests |
| ❌ | No tests | BLOCKING - must add tests |
| 🔄 | In progress | Complete before merge |

---

## Test Tagging Convention

### In Test Files

```go
// test/unit/signalprocessing/enricher_test.go

var _ = Describe("BR-SP-001: K8s Context Enrichment", func() {
    // All tests in this block count toward BR-SP-001

    Context("pod enrichment", func() {
        It("should fetch pod metadata", func() {
            // Test 1 of 15 for BR-SP-001
        })

        It("should handle pod not found", func() {
            // Test 2 of 15 for BR-SP-001
        })
    })
})

var _ = Describe("BR-SP-002: Business Classification", func() {
    // All tests in this block count toward BR-SP-002
})
```

### Verification Script

```bash
#!/bin/bash
# Verify BR coverage for a service

SERVICE=$1
BR_PREFIX=$2  # e.g., "BR-SP" for Signal Processing

echo "🔍 Scanning BR coverage for $SERVICE..."

# Find all BRs in implementation plan
PLAN_BRS=$(grep -oE "${BR_PREFIX}-[0-9]{3}" docs/services/*/IMPLEMENTATION_PLAN_*.md | sort -u)

# Find all BRs in test files
TEST_BRS=$(grep -oE "${BR_PREFIX}-[0-9]{3}" test/unit/$SERVICE/*.go test/integration/$SERVICE/*.go test/e2e/$SERVICE/*.go 2>/dev/null | sort -u)

echo "BRs in plan: $(echo "$PLAN_BRS" | wc -l)"
echo "BRs with tests: $(echo "$TEST_BRS" | wc -l)"

# Find uncovered BRs
UNCOVERED=$(comm -23 <(echo "$PLAN_BRS") <(echo "$TEST_BRS"))
if [ -n "$UNCOVERED" ]; then
    echo "❌ UNCOVERED BRs:"
    echo "$UNCOVERED"
    exit 1
else
    echo "✅ All BRs have test coverage"
fi
```

---

## Realistic Test Counts (Based on Implemented Services)

### Gateway Service (Reference)

| Category | Count | Notes |
|----------|-------|-------|
| **Unit Tests** | 100+ | Table-driven tests heavily used |
| **Integration Tests** | 100+ | K8s API, Redis, adapter tests |
| **E2E Tests** | 20+ | Critical workflows only |
| **Total** | 220+ | Comprehensive coverage |

### Data Storage Service (Reference)

| Category | Count | Notes |
|----------|-------|-------|
| **Unit Tests** | 150+ | Database operations, queries |
| **Integration Tests** | 80+ | PostgreSQL integration |
| **E2E Tests** | 15+ | API workflow tests |
| **Total** | 245+ | Comprehensive coverage |

### Expected Counts for New Services

Based on actual implementations, new services should target:

| Service Type | Unit | Integration | E2E | Total |
|--------------|------|-------------|-----|-------|
| **CRD Controller** | 80-120 | 40-60 | 15-25 | 135-205 |
| **Stateless HTTP** | 100-150 | 60-100 | 15-25 | 175-275 |

---

## Edge Case Categories (Must Be Covered)

Each BR should have tests for these categories where applicable:

### Input Validation
- Empty/nil input
- Malformed input (wrong type, format)
- Boundary values (max length, min/max numbers)
- Special characters, unicode

### Error Handling
- Network failures (timeout, connection refused)
- Resource not found (404)
- Permission denied (403)
- Rate limiting (429)
- Internal server error (500)

### State Transitions
- Initial state → intermediate state → final state
- Concurrent modifications
- State machine edge cases

### Resource Limits
- Memory pressure
- CPU throttling
- Connection pool exhaustion

### Concurrency
- Race conditions
- Deadlock scenarios
- Concurrent access to shared resources

---

## BR Coverage Validation Checklist

Before merging implementation:

- [ ] **All BRs have at least 1 test** (100% BR coverage)
- [ ] **Low complexity BRs have 5+ tests**
- [ ] **Medium complexity BRs have 12+ tests**
- [ ] **High complexity BRs have 23+ tests**
- [ ] **Edge cases covered** (input validation, error handling)
- [ ] **Test file naming follows convention** (`test/unit/[service]/[component]_test.go`)
- [ ] **BR tags in test descriptions** (`Describe("BR-XX-YYY: ...")`)

---

## Integration with Implementation Plans

Each implementation plan MUST include:

1. **BR Summary Table** (Day 1)
   - List all BRs with complexity classification

2. **BR Coverage Matrix** (Day 10)
   - Detailed test counts per BR per tier

3. **Coverage Verification** (Day 12)
   - Run verification script
   - Document any gaps and remediation

---

## Anti-Patterns

### ❌ Counting Tests Without BR Tags

```go
// ❌ WRONG: No BR reference - can't track coverage
var _ = Describe("Enricher", func() {
    It("should work", func() { /* test */ })
})
```

### ❌ Single Test for Complex BR

```go
// ❌ WRONG: High complexity BR with only 1 test
var _ = Describe("BR-SP-001: K8s Context Enrichment", func() {
    It("should enrich context", func() {
        // Only 1 test for a high-complexity BR - INSUFFICIENT
    })
})
```

### ❌ Weak Assertions Counting as Tests

```go
// ❌ WRONG: Null-testing anti-pattern
It("should return result", func() {
    result := enricher.Enrich(ctx, pod)
    Expect(result).ToNot(BeNil())  // Weak - doesn't validate business outcome
})

// ✅ RIGHT: Business outcome validation
It("should enrich pod with owner references", func() {
    result := enricher.Enrich(ctx, pod)
    Expect(result.OwnerReferences).To(HaveLen(1))
    Expect(result.OwnerReferences[0].Kind).To(Equal("Deployment"))
    Expect(result.OwnerReferences[0].Name).To(Equal("my-app"))
})
```

---

## Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| **BR Coverage** | 100% | All BRs have at least 1 test |
| **Minimum Test Counts** | Met | Per complexity classification |
| **Edge Case Coverage** | ≥80% | Edge case categories covered |
| **Test Quality** | No null-tests | All assertions validate outcomes |

---

## Cross-References

1. **ADR-005**: Integration Test Coverage (>50% target)
2. **ADR-037**: Business Requirement Template Standard
3. **SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md**: BR Coverage Matrix template
4. **DD-TEST-002**: Parallel Test Execution (test isolation)

---

**Document Owner**: Platform Architecture Team
**Last Updated**: 2025-11-28
**Next Review**: After V1.0 implementation complete

