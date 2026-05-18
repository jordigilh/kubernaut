# Test Plan: Load/Stress/Performance Test Plan (Investigation)

**Test Plan Identifier:** TP-AF-043
**Issue:** [#43](https://github.com/jordigilh/kubernaut-apifrontend/issues/43)
**Version:** 1.0
**Date:** 2026-05-06
**Status:** Draft

---

## 1. Introduction

This test plan validates the performance test investigation deliverables for the kubernaut API Frontend. Per issue #43, this is **investigation and documentation only** — actual test execution is deferred until proper hardware infrastructure is available.

### 1.1 Scope

- Performance test plan document (`docs/testing/PERFORMANCE_TEST_PLAN.md`)
- Tool recommendation with rationale (k6 recommended per ADR-010)
- Hardware requirements specification
- k6 script skeleton in `test/perf/`
- Load profile definitions (5 tiers)
- Metrics collection plan linked to SLO definitions (#41)

### 1.2 References

- Issue #43: Load/stress/performance test plan
- `docs/adr/ADR-010-load-testing-tool.md` — k6 recommendation
- Issue #41: SLO definitions (performance targets to validate against)
- MCP Streamable HTTP spec (SSE stream testing)
- A2A JSON-RPC 2.0 (task lifecycle testing)

### 1.3 Definitions

| Term | Definition |
|------|-----------|
| k6 | Grafana load testing tool (JavaScript scripting, Go runtime) |
| Load profile | Named combination of concurrent users, RPS, duration |
| Soak test | Extended duration test to find memory leaks |
| VU | Virtual User (k6 concept for concurrent connections) |

---

## 2. Test Items

| Item | Location | Source |
|------|----------|--------|
| PERFORMANCE_TEST_PLAN.md | `docs/testing/` | New |
| k6 script skeleton | `test/perf/` | New |
| Hardware requirements | Part of PERFORMANCE_TEST_PLAN.md | New |

---

## 3. Features to Be Tested (Deliverable Acceptance)

### 3.1 Business Acceptance Criteria

| ID | Criterion | Testable |
|----|-----------|----------|
| BAC-1 | Load profiles defined for all 5 tiers | Yes (document review) |
| BAC-2 | Tool evaluated and recommended | Yes (document exists with rationale) |
| BAC-3 | Hardware requirements documented | Yes (document section) |
| BAC-4 | Metrics collection plan covers all SLOs | Yes (cross-ref with #41) |
| BAC-5 | Test plan document written and reviewed | Yes (file exists) |
| BAC-6 | Scripts ready to execute when hardware is available | Yes (k6 files exist) |

### 3.2 Deliverable Validation

#### Tier 1: Performance Test Plan Document

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-43.1 | Document exists at docs/testing/PERFORMANCE_TEST_PLAN.md | File present |
| F-43.2 | Load profiles table with 5 rows (Baseline, Normal, Peak, Stress, Soak) | Complete table |
| F-43.3 | Each profile has: concurrent users, tool calls/sec, SSE streams, duration | All columns filled |
| F-43.4 | Tool recommendation section with rationale | k6 selected, reasons documented |
| F-43.5 | Hardware requirements section | CPU, RAM, disk, network specified |
| F-43.6 | Metrics to capture section | All SLO metrics listed |
| F-43.7 | SLO validation section linking to #41 | Cross-references SLO targets |

#### Tier 2: k6 Script Skeleton

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-43.8 | k6 script for MCP tools/call | `test/perf/mcp_tools.js` exists |
| F-43.9 | k6 script for health endpoint baseline | `test/perf/health_baseline.js` exists |
| F-43.10 | Scripts use k6 options (stages, thresholds) | Proper k6 API usage |
| F-43.11 | Scripts are runnable with `k6 run` (syntax valid) | No JS errors |

---

## 4. Features Not Tested

| Feature | Reason |
|---------|--------|
| Actual performance test execution | Deferred until proper HW available |
| SSE stream load testing | Complex; skeleton only in this phase |
| A2A task lifecycle under load | Deferred to execution phase |
| Grafana dashboard creation | Separate deliverable |

---

## 5. Approach

### 5.1 Validation Methodology

Since this issue is investigation/documentation, testing approach is:
- **Document completeness**: Verify all sections present per acceptance criteria
- **Script validity**: Run `k6 run --dry-run` to verify JS syntax
- **Cross-reference**: Metrics in perf plan match SLO_DEFINITIONS.md

### 5.2 k6 Script Structure

```javascript
import http from 'k6/http';
import { check } from 'k6';

export const options = {
  stages: [
    { duration: '1m', target: 10 },  // ramp up
    { duration: '5m', target: 10 },  // hold
    { duration: '1m', target: 0 },   // ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1000'],
  },
};

export default function () {
  // MCP tools/call or health check
}
```

---

## 6. Test Cases (Deliverable Validation)

### 6.1 Document Completeness (5 tests)

| ID | Description | Priority | BAC |
|----|-------------|----------|-----|
| UT-AF-043-001 | PERFORMANCE_TEST_PLAN.md exists with all 7 sections | P0 | BAC-5 |
| UT-AF-043-002 | Load profiles table has 5 tiers with all columns | P0 | BAC-1 |
| UT-AF-043-003 | Tool recommendation section selects k6 with rationale | P0 | BAC-2 |
| UT-AF-043-004 | Hardware requirements specify CPU, RAM, disk, network | P0 | BAC-3 |
| UT-AF-043-005 | Metrics section lists all af_* metrics from SLO_DEFINITIONS.md | P0 | BAC-4 |

### 6.2 k6 Scripts (4 tests)

| ID | Description | Priority | BAC |
|----|-------------|----------|-----|
| UT-AF-043-006 | test/perf/mcp_tools.js exists with valid k6 structure | P0 | BAC-6 |
| UT-AF-043-007 | test/perf/health_baseline.js exists with valid k6 structure | P0 | BAC-6 |
| UT-AF-043-008 | Scripts include k6 options with stages and thresholds | P1 | BAC-6 |
| UT-AF-043-009 | Scripts reference correct endpoint paths (/mcp, /healthz) | P0 | BAC-6 |

---

## 7. Pass/Fail Criteria

### 7.1 Pass

- Performance test plan document complete with all sections
- k6 scripts have valid JavaScript syntax
- All SLO metrics from #41 referenced in performance plan
- Load profiles match issue #43 specification

### 7.2 Fail

- Missing document sections
- k6 scripts with syntax errors
- SLO metrics not cross-referenced
- Load profiles incomplete

---

## 8. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| Performance test plan | `docs/testing/PERFORMANCE_TEST_PLAN.md` |
| k6 MCP tools script | `test/perf/mcp_tools.js` |
| k6 health baseline script | `test/perf/health_baseline.js` |
| This test plan | `docs/tests/43/test_plan.md` |

---

## 9. Traceability Matrix

| BAC | Test Cases | Count |
|-----|-----------|-------|
| BAC-1 | UT-AF-043-002 | 1 |
| BAC-2 | UT-AF-043-003 | 1 |
| BAC-3 | UT-AF-043-004 | 1 |
| BAC-4 | UT-AF-043-005 | 1 |
| BAC-5 | UT-AF-043-001 | 1 |
| BAC-6 | UT-AF-043-006 to 009 | 4 |
