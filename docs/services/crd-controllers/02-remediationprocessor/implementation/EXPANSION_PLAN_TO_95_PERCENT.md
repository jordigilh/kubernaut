# Remediation Processor - Expansion Plan to 95% Confidence

**Current Status**: 1,513 lines, 70% confidence
**Target Status**: 5,200 lines, 95% confidence
**Gap**: +3,687 lines
**Effort**: 12-15 hours

---

## Current State Analysis

### Existing Content (1,513 lines)
- ✅ Day 1 outline (foundation)
- ✅ Days 2-10 brief outlines (~50-100 lines each)
- ✅ Basic BR Coverage Matrix (150 lines, needs expansion)
- ⚠️ Missing: APDC detail, integration tests, EOD docs, error philosophy

### What's Missing vs Notification Standard
1. **APDC Phase Expansions**: 0/3 days fully detailed
2. **Integration Tests**: 0/3 complete tests
3. **EOD Documentation**: 0/3 milestone docs
4. **Error Handling Philosophy**: 0/1 document
5. **Table-Driven Test Examples**: 0 examples
6. **Production Deployment**: Brief only

---

## Expansion Phase 1: APDC Day Expansions (+2,800 lines)

### Day 2: Context Enrichment with PostgreSQL (~900 lines)

**Location**: After current Day 2 outline, replace with full expansion

**Structure**:
```markdown
## 📅 Day 2: Context Enrichment with PostgreSQL (8h)

### ANALYSIS Phase (1h)
- Search existing PostgreSQL integration patterns
- Search pgvector usage in Context API
- Map BR-REMEDIATION-002 (Context Enrichment) requirements
- Identify dependencies (Context API, Data Storage Service)

### PLAN Phase (1h)
- TDD Strategy: Unit tests (70%), Integration tests (>50%)
- Integration points: Context API client, pgvector queries
- Success criteria: Semantic search working, enrichment accurate
- Timeline: RED (2h) → GREEN (3h) → REFACTOR (2h)

### DO-RED: Context Enrichment Tests (2h)
**File**: `test/unit/remediationprocessing/enricher_test.go`
**BR Coverage**: BR-REMEDIATION-002, BR-REMEDIATION-003

[~300 lines of complete Ginkgo test code]
- Describe("BR-REMEDIATION-002: Context Enrichment")
- Context("when enriching with historical data")
- It("should query pgvector for similar incidents")
- DescribeTable("semantic similarity scoring", 10+ entries)
- It("should merge context from multiple sources")

**Expected Result**: Tests fail - ContextEnricher doesn't exist

### DO-GREEN: Minimal Context Enricher (3h)
**File**: `pkg/remediationprocessing/enricher/context_enricher.go`
**BR Coverage**: BR-REMEDIATION-002

[~400 lines of complete implementation code]
- type ContextEnricher struct
- func (e *ContextEnricher) EnrichContext()
- PostgreSQL client integration
- pgvector similarity query
- Context merging logic

**Expected Result**: Tests pass - basic enrichment working

### DO-REFACTOR: Advanced Enrichment (2h)
**Goal**: Add caching, batch queries, confidence scoring

[~200 lines of refactored code]
- Redis caching for frequent queries
- Batch enrichment for multiple signals
- Confidence score calculation
- Context deduplication

**Validation**:
- [ ] Tests passing
- [ ] pgvector queries < 100ms
- [ ] Cache hit rate > 60%
```

**Total Day 2 Lines**: ~900 lines (currently ~80)

---

### Day 4: Classification Logic (~900 lines)

**Location**: After current Day 4 outline, replace with full expansion

**Structure**:
```markdown
## 📅 Day 4: Classification and Priority Assignment (8h)

### ANALYSIS Phase (1h)
- Search existing classification patterns
- Review priority matrix requirements
- Map BR-REMEDIATION-004 (Classification) requirements

### PLAN Phase (1h)
- TDD Strategy: Table-driven tests for classification rules
- Integration points: ML service (future), rule engine
- Success criteria: 95%+ classification accuracy

### DO-RED: Classification Tests (2h)
**File**: `test/unit/remediationprocessing/classifier_test.go`
**BR Coverage**: BR-REMEDIATION-004, BR-REMEDIATION-005

[~350 lines of complete test code]
- DescribeTable("priority assignment rules", 20+ entries)
  - Entry("CPU spike + production → Critical")
  - Entry("Memory leak + staging → High")
  - Entry("Disk warning + dev → Medium")
- DescribeTable("category classification", 15+ entries)
- It("should handle classification conflicts")
- It("should provide confidence scores")

### DO-GREEN: Minimal Classifier (3h)
**File**: `pkg/remediationprocessing/classifier/classifier.go`

[~400 lines of implementation]
- type Classifier struct with rule engine
- Priority matrix implementation
- Category classification logic
- Confidence scoring

### DO-REFACTOR: ML Integration Prep (2h)
[~150 lines of ML service integration hooks]

**Validation**:
- [ ] All classification tests passing
- [ ] Priority assignment accurate
- [ ] Confidence scores reasonable
```

**Total Day 4 Lines**: ~900 lines (currently ~100)

---

### Day 7: Complete Integration (~1,000 lines)

**Location**: After current Day 7 outline, replace with full expansion

**Structure**:
```markdown
## 📅 Day 7: Controller Integration + Metrics (8h)

### Morning: Manager Setup (3h)
**File**: `cmd/remediationprocessor/main.go`

[~300 lines of complete main.go]
- Manager creation with scheme registration
- Controller setup with all dependencies
- Context API client initialization
- PostgreSQL connection pool
- Leader election configuration

### Afternoon: Prometheus Metrics (2h)
**File**: `pkg/remediationprocessing/metrics/metrics.go`

[~250 lines of metrics definitions]
- 10+ metrics:
  - remediation_enrichment_duration_seconds
  - remediation_classification_accuracy
  - remediation_deduplication_hits_total
  - remediation_context_cache_hit_ratio
  - remediation_crd_processing_total

### Evening: Health Checks (1h)
[~150 lines of health check implementation]

### EOD Documentation (2h)
**File**: `phase0/03-day7-complete.md`

[~300 lines - see separate section below]
```

**Total Day 7 Lines**: ~1,000 lines (currently ~90)

---

## Expansion Phase 2: Integration Test Suite (+600 lines)

### Integration Test 1: Context Enrichment (~200 lines)

**File**: `test/integration/remediationprocessing/context_enrichment_test.go`

**Structure**:
```go
var _ = Describe("Integration Test 1: Context Enrichment with pgvector", func() {
    It("should enrich RemediationProcessing with historical context", func() {
        By("Creating RemediationProcessing CRD")
        // [Full CRD creation code]

        By("Waiting for controller to enrich context")
        // [Polling for status update]

        By("Verifying enriched context contains historical data")
        // [Detailed assertions on enriched fields]

        By("Verifying pgvector similarity scores")
        // [Validate semantic search results]

        By("Verifying cache usage")
        // [Check Redis cache hit]
    })
})
```

---

### Integration Test 2: Classification and Priority (~200 lines)

**File**: `test/integration/remediationprocessing/classification_test.go`

**Structure**:
```go
var _ = Describe("Integration Test 2: Classification and Priority Assignment", func() {
    DescribeTable("should classify signals correctly",
        func(signalType, severity, environment, expectedPriority, expectedCategory string) {
            // [Table-driven integration test]
        },
        Entry("production CPU spike", "metric", "critical", "production", "Critical", "Performance"),
        Entry("staging memory leak", "metric", "warning", "staging", "High", "Performance"),
        // ... 8+ entries
    )
})
```

---

### Integration Test 3: Deduplication (~200 lines)

**File**: `test/integration/remediationprocessing/deduplication_test.go`

**Structure**:
```go
var _ = Describe("Integration Test 3: Deduplication with Window Detection", func() {
    It("should detect duplicate signals within time window", func() {
        By("Creating first RemediationProcessing")
        // [CRD 1 creation]

        By("Creating duplicate signal within 5-minute window")
        // [CRD 2 creation with same fingerprint]

        By("Verifying second signal marked as duplicate")
        // [Status assertions]

        By("Verifying original signal reference recorded")
        // [Cross-reference validation]
    })
})
```

**Total Integration Tests**: ~600 lines (currently 0)

---

## Expansion Phase 3: EOD Documentation (+800 lines)

### EOD 1: Day 4 Midpoint (~400 lines)

**File**: `phase0/02-day4-midpoint.md`

**Structure**:
```markdown
# Day 4 Midpoint: Context Enrichment & Classification Complete

**Date**: [YYYY-MM-DD]
**Status**: Days 1-4 Complete (40% of implementation)
**Confidence**: 85%

## Accomplishments (Days 1-4)
### Day 1: Foundation ✅
- Controller skeleton created
- ...

### Day 2: Context Enrichment ✅
- PostgreSQL integration complete
- pgvector semantic search working
- Redis caching implemented
- ...

### Day 3: Deduplication ✅
- Fingerprint generation working
- Time window detection accurate
- ...

### Day 4: Classification ✅
- Priority matrix implemented
- Category classification 95%+ accurate
- ML service integration hooks ready
- ...

## Integration Status
### Working Components ✅
- Context enrichment: <100ms p95
- Classification accuracy: 97%
- Deduplication hit rate: 89%
- ...

### Pending Integration
- ML service (Phase 2 feature)
- Advanced analytics (Phase 2)
- ...

## BR Progress Tracking
[Table showing 27 BRs with status]

## Blockers
**None at this time** ✅

## Next Steps (Days 5-7)
- Day 5: CRD creation and lifecycle
- Day 6: Error handling and retry
- Day 7: Complete integration + metrics

## Confidence Assessment
**Current Confidence**: 85%
[Detailed justification]

## Team Handoff Notes
[Key files, running locally, debugging tips]
```

**Total**: ~400 lines

---

### EOD 2: Day 7 Complete (~400 lines)

**File**: `phase0/03-day7-complete.md`

**Structure**: Similar to Day 4 but covering Days 1-7, Integration Status, all BRs, final confidence 92%

**Total**: ~400 lines

---

## Expansion Phase 4: Error Handling Philosophy (+300 lines)

**File**: `design/ERROR_HANDLING_PHILOSOPHY.md`

**Structure**:
```markdown
# Error Handling Philosophy - Remediation Processor

## Executive Summary
When to retry vs fail permanently, ensuring:
- BR-REMEDIATION-006: Automatic retry for transient failures
- BR-REMEDIATION-015: Graceful degradation
- Operational excellence: Prevent infinite retry loops

## Error Classification Taxonomy

### 1. Transient Errors (RETRY)
| Error Type | Example | Retry | Max Attempts | Backoff |
|-----------|---------|-------|--------------|---------|
| **PostgreSQL Timeout** | Connection timeout | ✅ Yes | 5 | Exponential |
| **Redis Connection** | Cache unavailable | ✅ Yes | 3 | Exponential |
| **Context API 503** | Service unavailable | ✅ Yes | 5 | Exponential |

### 2. Permanent Errors (FAIL IMMEDIATELY)
| Error Type | Example | Retry | Action |
|-----------|---------|-------|--------|
| **Invalid Fingerprint** | Malformed data | ❌ No | Mark failed, alert ops |
| **Schema Violation** | Missing required field | ❌ No | Validation error |

### 3. Ambiguous Errors (RETRY WITH CAUTION)
[Table of edge cases]

## Retry Policy Defaults
[Configuration example with backoff progression]

## Operational Guidelines
[Monitoring metrics, alert thresholds]

## Testing Strategy
[Unit, integration, chaos engineering]

## Summary
[Key principles]
```

**Total**: ~300 lines

---

## Expansion Phase 5: Enhanced BR Coverage Matrix (+200 lines)

**Enhancement to existing**: `testing/BR_COVERAGE_MATRIX.md`

**Additions**:

### 1. Testing Infrastructure Section
```markdown
## 🧪 Testing Infrastructure

**Per ADR-016: Service-Specific Integration Test Infrastructure**

| Test Type | Infrastructure | Rationale | Reference |
|-----------|----------------|-----------|-----------|
| **Unit Tests** | Fake Kubernetes Client | Fast, no infrastructure | ADR-004 |
| **Integration Tests** | **Envtest** | Real CRD validation, 5-18x faster than Kind | ADR-016 |
| **E2E Tests** | Kind cluster | Full system validation | ADR-003 |
```

### 2. Edge Case Coverage Section
```markdown
## 🔬 Edge Case Coverage - 12 Additional Test Scenarios

**Purpose**: Explicit edge case testing for boundary conditions

| Edge Case BR | Requirement | Test Type | Test File | Status |
|--------------|-------------|-----------|-----------|--------|
| **BR-REMEDIATION-002-EC1** | Context enrichment with empty historical data | Unit | `enricher_edge_cases_test.go` | ✅ |
| **BR-REMEDIATION-002-EC2** | pgvector query timeout | Unit | `enricher_edge_cases_test.go` | ✅ |
| **BR-REMEDIATION-004-EC1** | Classification with conflicting rules | Unit | `classifier_edge_cases_test.go` | ✅ |
...
```

### 3. Test Implementation Guidance
```markdown
## 📝 Test Implementation Guidance

### Using Ginkgo DescribeTable for Edge Case Testing

**Example: Classification Edge Cases**

```go
DescribeTable("BR-REMEDIATION-004: Classification Edge Cases",
    func(scenario string, input SignalData, expectedPriority, expectedCategory string) {
        // [Full test implementation example]
    },
    Entry("conflicting severity indicators", ...),
    Entry("missing environment field", ...),
    Entry("unknown signal type", ...),
    // ... 10+ entries
)
```
```

### 4. Defense-in-Depth Coverage Update
```markdown
## 📊 Coverage Summary (Defense-in-Depth Strategy)

**Per 03-testing-strategy.mdc: Overlapping coverage with multiple test levels**

| Category | Total BRs | Unit Tests | Integration Tests | E2E Tests | Total Coverage |
|----------|-----------|------------|-------------------|-----------|----------------|
| **Context Enrichment** | 8 | 8 (100%) | 5 (63%) | 1 (13%) | 176% ✅ |
| **Classification** | 7 | 7 (100%) | 4 (57%) | 1 (14%) | 171% ✅ |
| **Deduplication** | 4 | 4 (100%) | 3 (75%) | 1 (25%) | 200% ✅ |
| **CRD Lifecycle** | 5 | 3 (60%) | 4 (80%) | 2 (40%) | 180% ✅ |
| **Integration** | 3 | 0 (0%) | 3 (100%) | 1 (33%) | 133% ✅ |
| **Total** | **27** | **22 (81%)** | **19 (70%)** | **6 (22%)** | **173% ✅** |

**Target Achievement**:
- ✅ Unit: 81% (target: 70%+)
- ✅ Integration: 70% (target: >50%)
- ✅ E2E: 22% (target: 10-15%)
- ✅ Overlapping: 173% (target: 130-165%)
```

**Total Enhancement**: ~200 lines added to existing matrix

---

## Summary: Total Additions by Phase

| Phase | Component | Lines | Effort |
|-------|-----------|-------|--------|
| **Phase 1** | Day 2 APDC Expansion | 900 | 3h |
| **Phase 1** | Day 4 APDC Expansion | 900 | 3h |
| **Phase 1** | Day 7 APDC Expansion | 1,000 | 3h |
| **Phase 2** | Integration Test 1 | 200 | 1.5h |
| **Phase 2** | Integration Test 2 | 200 | 1.5h |
| **Phase 2** | Integration Test 3 | 200 | 1.5h |
| **Phase 3** | EOD: Day 4 Midpoint | 400 | 1h |
| **Phase 3** | EOD: Day 7 Complete | 400 | 1h |
| **Phase 4** | Error Handling Philosophy | 300 | 1h |
| **Phase 5** | BR Coverage Matrix Enhancement | 200 | 0.5h |
| **Total** | **All Additions** | **4,700** | **17h** |

**Confidence Increase**: 70% → 95%

---

## Approval Required

**This plan shows WHAT will be added but does NOT implement it yet.**

Please review:
1. ✅ Is the structure appropriate?
2. ✅ Are the line counts realistic?
3. ✅ Is the effort estimate reasonable?
4. ✅ Should any sections be added/removed?
5. ✅ Any concerns with the approach?

**After approval, I will implement each phase systematically.**

