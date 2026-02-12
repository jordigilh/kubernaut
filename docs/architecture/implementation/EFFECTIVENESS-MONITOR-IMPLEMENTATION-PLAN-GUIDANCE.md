# Effectiveness Monitor - Implementation Plan Creation Guidance

> **ARCHIVED** (February 2026)
>
> This document describes an implementation plan based on Context API polling, PostgreSQL trend
> tables, and REST APIs for trend queries. This design has been **superseded** by DD-017 v2.0,
> which defines EM Level 1 as a Kubernetes controller that:
> - Watches RemediationRequest CRDs (DD-EFFECTIVENESS-003)
> - Performs automated health checks, metric comparison, and scoring
> - Emits structured audit events to DataStorage (no new DB tables)
> - Uses RR.Name as correlation ID for audit trace retrieval
>
> This document is retained for historical reference only. Do NOT use it as implementation guidance.
>
> **Authoritative source**: `docs/architecture/decisions/DD-017-effectiveness-monitor-v1.1-deferral.md` (v2.0)

**Date**: November 2, 2025
**Status**: ⚠️ **ARCHIVED** — Superseded by DD-017 v2.0 (February 2026)
**Purpose**: Comprehensive guidance for creating the Effectiveness Monitor implementation plan
**Authoritative Sources**:
- [Gateway Service Implementation Plan v2.23](../../services/stateless/gateway-service/IMPLEMENTATION_PLAN_V2.23.md) - **PRIMARY TEMPLATE** (7,945 lines, 95% confidence, production-ready)
- [Context API Implementation Plan v2.8](../../services/stateless/context-api/implementation/IMPLEMENTATION_PLAN_V2.8.md) - Secondary template
- [Data Storage vs Gateway Deeper Triage](./DATA-STORAGE-VS-GATEWAY-DEEPER-TRIAGE.md) - Gap analysis with 37 production-readiness requirements

---

## 🎯 **PURPOSE OF THIS DOCUMENT**

This document provides a **comprehensive checklist and template** for creating the Effectiveness Monitor implementation plan. It incorporates lessons learned from:

1. **Gateway v2.23** - Production-ready implementation (95% confidence)
2. **Context API v2.8** - Mature implementation (95% confidence)
3. **Data Storage Deeper Triage** - 37 production-readiness gaps identified

**Goal**: Create an implementation plan that achieves **95% confidence** on first iteration, avoiding the 29 gaps found in Data Storage migration plan and 15 gaps found in Context API migration plan.

---

## 📋 **MANDATORY SECTIONS CHECKLIST**

Use this checklist to ensure NO critical sections are missing:

### **SECTION 1: Header & Metadata** ✅ CRITICAL

- [ ] **Service Name**: "Effectiveness Monitor - Implementation Plan vX.X"
- [ ] **Status Badge**: Production-ready status (e.g., "🚧 IN PROGRESS" or "✅ PRODUCTION-READY")
- [ ] **Version**: Semantic versioning (v1.0, v1.1, etc.)
- [ ] **Template Version**: Reference to SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md version
- [ ] **Plan Date**: ISO date (YYYY-MM-DD)
- [ ] **Current Status**: Emoji + percentage (e.g., "🚧 60% Complete")
- [ ] **Business Requirements**: BR range (e.g., "BR-EFFECTIVENESS-001 through BR-EFFECTIVENESS-050")
- [ ] **Scope**: Brief description of service scope
- [ ] **Confidence**: Percentage with justification
- [ ] **Architecture References**: Links to relevant ADRs/DDs
- [ ] **Related Services**: Data Storage Service (API Gateway), Context API, Gateway Service

**Example** (from Gateway v2.23):
```markdown
# Effectiveness Monitor - Implementation Plan v1.0

✅ **PRODUCTION-READY** - Full Implementation Complete

**Service**: Effectiveness Monitor (Action Effectiveness Analysis & Learning)
**Phase**: Phase 3, Service #4
**Plan Version**: v1.0 (Initial Implementation)
**Template Version**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md v2.0
**Plan Date**: November 3, 2025
**Current Status**: 🚀 V1.0 IMPLEMENTATION COMPLETE (95% Confidence - Production Ready)
**Business Requirements**: BR-EFFECTIVENESS-001 through BR-EFFECTIVENESS-050 (~50 BRs)
**Scope**: Action success/failure analysis + Pattern learning + Feedback loop to AI
**Confidence**: 95% ✅ **Production-Ready - All tests passing, documentation complete**

**Architecture**: REST API client for Data Storage Service (DD-ARCH-001)
**Data Access**: Read-only via Data Storage Service API (no direct PostgreSQL access)
**Critical Warning**: ⚠️ NEVER query PostgreSQL directly for audit data - use Data Storage Service API

**Related Decisions**:
- [DD-ARCH-001](../../architecture/decisions/DD-ARCH-001-FINAL-DECISION.md) - API Gateway Pattern
- [DD-EFFECTIVENESS-001](../../architecture/decisions/DD-EFFECTIVENESS-001-pattern-learning.md) - Pattern Learning Strategy
```

---

### **SECTION 2: Version History** ✅ CRITICAL

- [ ] **Table format**: Version | Date | Changes | Status
- [ ] **All versions documented**: From v0.1 to current
- [ ] **Change descriptions**: Brief summary of what changed
- [ ] **Status indicators**: ⚠️ SUPERSEDED, ✅ CURRENT

**Example** (from Gateway v2.23, lines 21-53):
```markdown
## 📋 Version History

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v1.0** | Nov 3, 2025 | Initial implementation - Pattern analysis + Feedback loop | ✅ **CURRENT** |
| **v0.9** | Nov 2, 2025 | API Gateway migration complete - Data Storage Service integration | ⚠️ SUPERSEDED |
| **v0.1** | Oct 2025 | Exploration - Requirements analysis and architecture design | ⚠️ SUPERSEDED |
```

---

### **SECTION 3: Pre-Day 1 Validation** ⚠️ **P0 BLOCKER**

**CRITICAL**: Without this, implementation starts with broken infrastructure

- [ ] **Validation script**: Executable bash script (`scripts/validate-effectiveness-monitor-infrastructure.sh`)
- [ ] **Infrastructure checks**:
  - [ ] PostgreSQL connectivity validation
  - [ ] Redis connectivity validation
  - [ ] Data Storage Service availability (`/health` endpoint)
  - [ ] Kubernetes cluster access
  - [ ] Go toolchain validation
  - [ ] controller-runtime library check
- [ ] **Duration estimate**: 2-3 hours
- [ ] **Exit codes**: Non-zero on failure
- [ ] **Clear error messages**: Tell user HOW to fix issues

**Example** (from Gateway v2.23, lines 79-260):
```bash
#!/bin/bash
# Effectiveness Monitor - Infrastructure Validation Script
# Validates all infrastructure dependencies before Day 1

set -e

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Effectiveness Monitor - Infrastructure Validation"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# 1. Validate Data Storage Service availability
echo "✓ Step 1: Validating Data Storage Service (localhost:8085)..."
if ! curl -f http://localhost:8085/health &> /dev/null; then
    echo "❌ FAIL: Data Storage Service not available at localhost:8085"
    echo "   Run: make run-data-storage-service"
    exit 1
fi
echo "✅ PASS: Data Storage Service responding"

# 2. Validate Data Storage Service API endpoints
echo "✓ Step 2: Validating Data Storage Service API endpoints..."
RESPONSE=$(curl -s http://localhost:8085/api/v1/incidents?limit=1)
if [ -z "$RESPONSE" ]; then
    echo "❌ FAIL: Data Storage Service API not responding"
    exit 1
fi
echo "✅ PASS: Data Storage Service API endpoints available"

# 3. Validate PostgreSQL (via Data Storage Service)
echo "✓ Step 3: Validating PostgreSQL access (via Data Storage Service)..."
# Note: DO NOT test direct PostgreSQL access - use Data Storage Service API only
INCIDENTS=$(curl -s http://localhost:8085/api/v1/incidents?limit=1 | jq '.incidents | length')
if [ "$INCIDENTS" == "null" ]; then
    echo "❌ FAIL: Data Storage Service cannot query PostgreSQL"
    exit 1
fi
echo "✅ PASS: Data Storage Service can query PostgreSQL"

# 4. Validate Redis availability
echo "✓ Step 4: Validating Redis (localhost:6379)..."
if ! nc -z localhost 6379 2>/dev/null; then
    echo "❌ FAIL: Redis not available at localhost:6379"
    echo "   Run: make bootstrap-dev"
    exit 1
fi
REDIS_PING=$(redis-cli ping 2>/dev/null || echo "FAIL")
if [ "$REDIS_PING" != "PONG" ]; then
    echo "❌ FAIL: Redis ping failed"
    exit 1
fi
echo "✅ PASS: Redis available and responding"

# 5. Validate Kubernetes cluster access
echo "✓ Step 5: Validating Kubernetes cluster access..."
if ! kubectl cluster-info &> /dev/null; then
    echo "❌ FAIL: Kubernetes cluster not accessible"
    echo "   Ensure KUBECONFIG is set and cluster is running"
    exit 1
fi
echo "✅ PASS: Kubernetes cluster accessible"

# 6. Validate Go toolchain
echo "✓ Step 6: Validating Go toolchain..."
if ! go version &> /dev/null; then
    echo "❌ FAIL: Go not installed"
    exit 1
fi
echo "✅ PASS: Go toolchain available"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ ALL VALIDATIONS PASSED - Ready for Effectiveness Monitor implementation"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
```

**Makefile Target**:
```makefile
.PHONY: validate-effectiveness-monitor-infrastructure
validate-effectiveness-monitor-infrastructure:  ## Validate Effectiveness Monitor infrastructure before implementation
	@bash scripts/validate-effectiveness-monitor-infrastructure.sh
```

---

### **SECTION 4: Common Pitfalls** ⚠️ **P0 BLOCKER**

**CRITICAL**: Without this, developers WILL repeat known mistakes

- [ ] **10+ comprehensive pitfalls** documented
- [ ] **Code examples**: Both ❌ BAD and ✅ GOOD patterns
- [ ] **Business impact**: Link each pitfall to specific BR violations
- [ ] **Prevention strategies**: Concrete steps to avoid pitfall
- [ ] **Discovery dates**: When pitfall was identified

**Required Pitfalls for Effectiveness Monitor**:

1. **Direct PostgreSQL Access for Audit Data** ⚠️ **MOST CRITICAL**
   ```go
   // ❌ WRONG: Direct PostgreSQL query
   func (m *Monitor) GetActionHistory() ([]Action, error) {
       rows, err := m.db.Query("SELECT * FROM resource_action_traces WHERE...")
       // VIOLATION: Bypasses Data Storage Service, breaks API Gateway pattern
   }

   // ✅ CORRECT: Use Data Storage Service API
   func (m *Monitor) GetActionHistory() ([]Action, error) {
       resp, err := m.dataStorageClient.ListIncidents(ctx, &ListParams{
           ActionType: "remediation",
           Limit:      100,
       })
       // Uses Data Storage Service REST API per DD-ARCH-001
   }
   ```

2. **Null Testing Anti-Pattern** (from Gateway Pitfall 1)
3. **Batch-Activated TDD Violation** (from Gateway Pitfall 2)
4. **Missing Package Declarations** (from deeper triage GAP-015)
5. **Using miniredis in Integration Tests** (from triage GAP-001)
6. **Missing Request ID Propagation** (from triage GAP-007)
7. **Missing Context Cancellation Checks** (from triage GAP-008)
8. **Missing RFC 7807 Error Parsing** (from triage GAP-004)
9. **Missing Import Statements in Code Examples** (from triage GAP-003)
10. **Weak Assertions Instead of Specific Value Checks**

**Remediation**: +6 hours (3h writing, 3h validation)

---

### **SECTION 5: Operational Runbooks** ⚠️ **P0 BLOCKER**

**CRITICAL**: Without this, service CANNOT be safely deployed to production

- [ ] **6 comprehensive runbooks**:
  1. **Deployment Runbook** - Step-by-step Kubernetes deployment
  2. **Troubleshooting Runbook** - Common issues and solutions
  3. **Rollback Runbook** - Safe rollback procedures
  4. **Performance Tuning Runbook** - Optimization guidance
  5. **Maintenance Runbook** - Routine maintenance tasks
  6. **On-Call Runbook** - Emergency response procedures

**Example** (Deployment Runbook):
```bash
#!/bin/bash
# Effectiveness Monitor - Deployment Runbook

echo "Step 1: Validate Data Storage Service is running..."
kubectl get pods -n kubernaut-system -l app=data-storage-service
# Expected: 1 pod in Running state

echo "Step 2: Validate Data Storage Service health..."
DATA_STORAGE_POD=$(kubectl get pods -n kubernaut-system -l app=data-storage-service -o jsonpath='{.items[0].metadata.name}')
kubectl exec -n kubernaut-system $DATA_STORAGE_POD -- curl -f http://localhost:8085/health
# Expected: {"status":"healthy"}

echo "Step 3: Deploy Effectiveness Monitor..."
kubectl apply -f deploy/effectiveness-monitor/
kubectl rollout status deployment/effectiveness-monitor -n kubernaut-system --timeout=180s
# Expected: Deployment successful

echo "Step 4: Validate health endpoints..."
MONITOR_POD=$(kubectl get pods -n kubernaut-system -l app=effectiveness-monitor -o jsonpath='{.items[0].metadata.name}')
kubectl exec -n kubernaut-system $MONITOR_POD -- curl -f http://localhost:8090/health
# Expected: {"status":"healthy"}

echo "Step 5: Test Data Storage Service connectivity..."
kubectl logs -n kubernaut-system $MONITOR_POD --tail=20 | grep "Data Storage Service"
# Expected: Successful connection log entries

echo "✅ Deployment complete - Effectiveness Monitor operational"
```

**Example** (Troubleshooting Runbook):
```markdown
## Troubleshooting Runbook

### Issue 1: "Cannot connect to Data Storage Service"

**Symptoms**:
- Logs show: `ERROR: Failed to connect to Data Storage Service: connection refused`
- Health endpoint returns 503 Service Unavailable

**Diagnosis**:
```bash
# Check Data Storage Service status
kubectl get pods -n kubernaut-system -l app=data-storage-service

# Check Data Storage Service logs
kubectl logs -n kubernaut-system -l app=data-storage-service --tail=50
```

**Resolution**:
1. Verify Data Storage Service is running: `kubectl get pods -n kubernaut-system`
2. If not running, redeploy: `kubectl apply -f deploy/data-storage-service/`
3. Verify network connectivity: `kubectl exec effectiveness-monitor-pod -- curl http://data-storage-service:8085/health`
4. Check NetworkPolicies: `kubectl get networkpolicies -n kubernaut-system`

### Issue 2: "Pattern analysis taking too long"

**Symptoms**:
- Pattern analysis requests timeout after 30s
- Logs show: `WARN: Pattern analysis exceeded timeout`

**Diagnosis**:
```bash
# Check pattern cache hit rate
kubectl exec effectiveness-monitor-pod -- curl http://localhost:9090/metrics | grep pattern_cache_hit_rate

# Check Data Storage Service response times
kubectl exec effectiveness-monitor-pod -- curl http://localhost:9090/metrics | grep data_storage_response_duration
```

**Resolution**:
1. Verify pattern cache is enabled and warm
2. Check Data Storage Service performance: `kubectl logs data-storage-pod | grep "slow query"`
3. Increase timeout if justified: Update ConfigMap `pattern_analysis_timeout`
4. Consider batch processing for large datasets
```

**Remediation**: +8 hours (5h writing, 3h validation)

---

### **SECTION 6: Business Requirements** ✅ CRITICAL

- [ ] **Table format**: BR ID | Requirement | Priority | Test Coverage
- [ ] **Comprehensive BRs**: 40-60 business requirements
- [ ] **Categories**:
  - Pattern analysis (BR-EFFECTIVENESS-001 to 010)
  - Success/failure metrics (BR-EFFECTIVENESS-011 to 020)
  - Feedback loop to AI (BR-EFFECTIVENESS-021 to 030)
  - API endpoints (BR-EFFECTIVENESS-031 to 040)
  - Error handling (BR-EFFECTIVENESS-041 to 050)
  - Observability (BR-EFFECTIVENESS-051 to 060)
- [ ] **Priority labels**: P0 (blocking), P1 (critical), P2 (important)
- [ ] **Test coverage mapping**: Unit, Integration, E2E

**Example**:
```markdown
## 📋 **BUSINESS REQUIREMENTS**

| BR ID | Requirement | Priority | Test Coverage |
|-------|-------------|----------|---------------|
| **Pattern Analysis** ||||
| BR-EFFECTIVENESS-001 | Analyze action success rate per pattern | P0 | Unit + Integration |
| BR-EFFECTIVENESS-002 | Identify top 10 failing action patterns | P0 | Unit + Integration |
| BR-EFFECTIVENESS-003 | Track success rate trends over time (7d, 30d) | P1 | Unit |
| BR-EFFECTIVENESS-004 | Detect pattern degradation (>20% success drop) | P1 | Unit + Integration |
| **Success/Failure Metrics** ||||
| BR-EFFECTIVENESS-011 | Calculate action success rate (by type, cluster, namespace) | P0 | Unit + Integration |
| BR-EFFECTIVENESS-012 | Calculate MTTR (Mean Time To Recovery) per action type | P1 | Unit |
| BR-EFFECTIVENESS-013 | Calculate action retry statistics | P1 | Unit |
| **Feedback Loop to AI** ||||
| BR-EFFECTIVENESS-021 | Provide feedback API for AI service | P0 | Integration + E2E |
| BR-EFFECTIVENESS-022 | Return recommended actions based on historical success | P0 | Unit + Integration |
| BR-EFFECTIVENESS-023 | Blacklist ineffective action patterns | P1 | Unit |
| **API Endpoints** ||||
| BR-EFFECTIVENESS-031 | GET /api/v1/patterns - List all patterns with success rates | P0 | Integration |
| BR-EFFECTIVENESS-032 | GET /api/v1/patterns/{id}/history - Pattern success history | P1 | Integration |
| BR-EFFECTIVENESS-033 | GET /api/v1/feedback?incident_id={id} - AI feedback for incident | P0 | Integration + E2E |
| **Error Handling** ||||
| BR-EFFECTIVENESS-041 | RFC 7807 error responses for all API errors | P1 | Unit |
| BR-EFFECTIVENESS-042 | Graceful degradation when Data Storage Service unavailable | P0 | Integration |
| BR-EFFECTIVENESS-043 | Circuit breaker for Data Storage Service calls | P1 | Integration |
| **Observability** ||||
| BR-EFFECTIVENESS-051 | Prometheus metrics for pattern analysis duration | P1 | Unit |
| BR-EFFECTIVENESS-052 | Metrics for Data Storage Service call success/failure | P0 | Unit |
| BR-EFFECTIVENESS-053 | Structured logging with request IDs | P1 | Unit |
```

---

### **SECTION 7: Defense-in-Depth Test Strategy** ✅ CRITICAL

- [ ] **Test pyramid distribution**:
  - Unit Tests: **70%** (business logic, validation, edge cases)
  - Integration Tests: **≥50% of BRs** (HTTP API + Data Storage Service + Redis)
  - E2E Tests: **<10%** (complete workflow validation)
- [ ] **Edge case testing matrix**: 50+ scenarios
- [ ] **Categories**:
  - Input Validation (empty params, null values, negative numbers, out-of-range)
  - Data Storage Service Integration (timeouts, connection failures, retries, circuit breaker)
  - Pattern Analysis Edge Cases (no historical data, 100% success, 0% success, sparse data)
  - Cache Edge Cases (cache miss, cache invalidation, Redis unavailable)
  - Concurrency (simultaneous pattern analysis, read during write)

**Example** (from Data Storage Migration Plan):
```markdown
## 🧪 **DEFENSE-IN-DEPTH TEST STRATEGY**

### **Test Pyramid Distribution**

| Layer | Coverage Target | Focus | Examples |
|-------|----------------|-------|----------|
| **Unit Tests** | **70%** | Business logic, validation, edge cases | Pattern analysis, success rate calculation, feedback generation |
| **Integration Tests** | **≥50% of BRs** | HTTP API + Data Storage Service + Redis | REST endpoint with real Data Storage Service |
| **E2E Tests** | **<10%** | Full workflow | AI request → Pattern analysis → Feedback response |

### **Edge Case Testing Matrix**

| Category | Edge Cases | Test Type |
|----------|------------|-----------|
| **Input Validation** | Empty incident ID, null pattern, negative time range | Unit |
| **Data Storage Service** | Connection timeout, 503 unavailable, rate limit, slow response (>5s) | Integration |
| **Pattern Analysis** | No historical data, single data point, 100% success rate, 0% success rate | Unit |
| **Success Rate Calculation** | Division by zero (0 actions), floating point precision, large numbers (>1M actions) | Unit (Boundary) |
| **Cache** | Cache miss, stale data, cache invalidation, Redis unavailable | Integration |
| **Concurrency** | Simultaneous pattern analysis, read during cache update | Integration |
| **Data Storage API Errors** | 400 Bad Request, 404 Not Found, 500 Internal Server Error, network timeout | Integration |
```

---

### **SECTION 8: APDC-Enhanced TDD Workflow** ✅ CRITICAL

- [ ] **ANALYSIS Phase** (Day 0: 2-3 hours):
  - Business context review
  - Technical context (existing code analysis)
  - Integration context (Data Storage Service API)
  - Complexity assessment
  - **Deliverables**: BR mapping, edge case matrix, risk assessment
- [ ] **PLAN Phase** (Day 0: 2-3 hours):
  - TDD strategy (which interfaces to enhance/create, where tests live)
  - Integration plan (which main app files instantiate your component)
  - Success definition (measurable business outcome)
  - Risk mitigation (simplest implementation)
  - Timeline (RED → GREEN → REFACTOR)
- [ ] **DO Phase**: TDD RED-GREEN-REFACTOR with code examples
- [ ] **CHECK Phase**: Business verification, technical validation, confidence assessment

**Example** (from Data Storage Migration Plan):
```markdown
## 🔄 **APDC-ENHANCED TDD WORKFLOW**

### **ANALYSIS PHASE** (Day 0: 2-3 hours)

**Objective**: Comprehensive context understanding before implementation

**Tasks**:
1. **Business Context**: Review DD-ARCH-001 (API Gateway pattern), understand Effectiveness Monitor role
2. **Technical Context**: Analyze Data Storage Service API specification
3. **Integration Context**: Identify AI service integration points
4. **Complexity Assessment**: Evaluate pattern analysis complexity

**Deliverables**:
- ✅ Business requirement mapping complete (BR-EFFECTIVENESS-001 through BR-EFFECTIVENESS-060)
- ✅ Data Storage Service API endpoints identified
- ✅ Edge case test matrix created (50+ scenarios)
- ✅ Risk assessment completed

**Analysis Checkpoint**:
```
✅ ANALYSIS PHASE VALIDATION:
- [ ] All 60 business requirements identified ✅/❌
- [ ] Data Storage Service API specification reviewed ✅/❌
- [ ] Edge case matrix covers data access, pattern analysis, feedback ✅/❌
- [ ] Integration patterns understood (HTTP client, circuit breaker, cache) ✅/❌

❌ STOP: Cannot proceed to PLAN phase until ALL checkboxes are ✅
```

**🚫 MANDATORY USER APPROVAL GATE - ANALYSIS PHASE:**
```
🎯 ANALYSIS PHASE SUMMARY:
Business Requirement: [BR-EFFECTIVENESS-XXX with justification]
Data Storage Service API: [N endpoints identified with usage patterns]
Integration Points: [M main app integration points discovered]
Complexity Level: [SIMPLE/MEDIUM/COMPLEX with evidence]
Recommended Approach: [use Data Storage Service API, implement pattern analysis, integrate with AI service]

❓ **MANDATORY APPROVAL**: Do you approve this analysis and approach? (YES/NO)
```

### **PLAN PHASE** (Day 0: 2-3 hours)

**Objective**: Detailed implementation strategy with TDD phase mapping

**TDD Strategy**:
- **Enhance**: `pkg/effectivenessmonitor/analysis/pattern_analyzer.go` (add pattern analysis logic)
- **Create**: `pkg/effectivenessmonitor/client/data_storage_client.go` (HTTP client for Data Storage Service)
- **Tests**: `test/unit/effectivenessmonitor/`, `test/integration/effectivenessmonitor/`

**Integration Plan**:
- **Main App Files**: `cmd/effectiveness-monitor/main.go`
- **Dependencies**: Data Storage Service client, Redis cache, pattern analyzer

**Success Definition**:
- **BR-EFFECTIVENESS-001**: Pattern analysis returns success rate per pattern
- **BR-EFFECTIVENESS-021**: AI service receives actionable feedback

**Risk Mitigation**:
- Data Storage Service unavailable → Circuit breaker + graceful degradation
- Pattern analysis too slow → Redis cache + background processing
- No historical data → Return default recommendations

**Timeline**:
- **RED** (10-15min): Write failing tests for pattern analysis
- **GREEN** (15-20min): Minimal pattern analysis implementation + Data Storage Service client
- **REFACTOR** (20-30min): Enhance with caching, circuit breaker, comprehensive error handling

**🚫 MANDATORY USER APPROVAL GATE - PLAN PHASE:**
```
🎯 PLAN PHASE SUMMARY:
TDD Strategy: [enhance PatternAnalyzer, create DataStorageClient, tests in test/unit/ and test/integration/]
Integration Plan: [specific files: cmd/effectiveness-monitor/main.go, pkg/effectivenessmonitor/server.go]
Success Definition: [BR-EFFECTIVENESS-001, BR-EFFECTIVENESS-021: measurable business outcomes]
Risk Mitigation: [circuit breaker for Data Storage Service, Redis cache for performance]
Timeline: [RED: 15min → GREEN: 20min → REFACTOR: 30min]

❓ **MANDATORY APPROVAL**: Do you approve this implementation plan? (YES/NO)
```
```

---

### **SECTION 9: Implementation Days with Full Code Examples** ✅ CRITICAL

- [ ] **13-15 implementation days** with APDC phases
- [ ] **Each day includes**:
  - Objective (what business outcome)
  - APDC Do phase (RED → GREEN → REFACTOR)
  - Code examples with **imports AND package declarations**
  - Deliverables (specific files created/modified)
  - Check phase (confidence assessment, risks)
- [ ] **Day structure**:
  - Day 1: HTTP Server + Health Endpoints
  - Day 2: Data Storage Service HTTP Client
  - Day 3: Pattern Analysis Engine
  - Day 4: Success Rate Calculation
  - Day 5: Feedback API for AI Service
  - Day 6: Caching Layer (Redis)
  - Day 7: Circuit Breaker + Graceful Degradation
  - Day 8: Integration Tests (>50% BR coverage)
  - Day 9: Observability (Metrics + Logging)
  - Day 10: BR Validation + Pre-Production Validation
  - Day 11: E2E Tests (Podman + Kind)
  - Day 12: Kubernetes Deployment
  - Day 13: Production Readiness Review

**Code Example Requirements** (from deeper triage GAP-015, GAP-003, GAP-023):
```go
// ✅ CORRECT: Complete code example with ALL required elements
package effectivenessmonitor  // WHITE-BOX TESTING (same package as code under test)

import (
    "context"
    "net/http"
    "testing"
    "time"

    "github.com/jordigilh/kubernaut/pkg/datastorage/client"
    "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/analysis"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "go.uber.org/zap"
)

// TestEffectivenessMonitor is the entry point for Ginkgo test suite
func TestEffectivenessMonitor(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Effectiveness Monitor Test Suite")
}

var _ = Describe("Pattern Analysis - BR-EFFECTIVENESS-001", func() {
    var (
        analyzer         *analysis.PatternAnalyzer
        dataStorageClient *client.DataStorageClient
        ctx              context.Context
    )

    BeforeEach(func() {
        dataStorageClient = client.NewDataStorageClient("http://localhost:8085")
        analyzer = analysis.NewPatternAnalyzer(dataStorageClient)
        ctx = context.Background()
    })

    It("should calculate success rate for pattern", func() {
        // Test implementation
        rate, err := analyzer.CalculateSuccessRate(ctx, "restart-pod")
        Expect(err).ToNot(HaveOccurred())
        Expect(rate).To(BeNumerically(">=", 0.0))
        Expect(rate).To(BeNumerically("<=", 1.0))
    })
})
```

---

### **SECTION 10: Test Suite Documentation** ✅ CRITICAL

- [ ] **Test count tracking**:
  ```markdown
  ### Test Summary

  | Test Type | Count | Pass Rate | Coverage |
  |-----------|-------|-----------|----------|
  | **Unit Tests** | 140 | 100% (140/140) | 85% of BRs |
  | **Integration Tests** | 70 | 100% (70/70) | 55% of BRs |
  | **E2E Tests** | 5 | 100% (5/5) | 10% (critical workflows) |
  | **Total Tests** | 215 | 100% (215/215) | - |
  ```

- [ ] **BR Coverage Matrix**:
  ```markdown
  | BR ID | Requirement | Unit Tests | Integration Tests | E2E Tests | Total Coverage |
  |---|---|---|---|---|---|
  | BR-EFFECTIVENESS-001 | Pattern success rate | 20 tests | 8 tests | 1 test | 29 tests (242%) |
  | BR-EFFECTIVENESS-021 | AI feedback API | 10 tests | 6 tests | 2 tests | 18 tests (150%) |
  ```

- [ ] **Test file organization**:
  ```
  test/
  ├── unit/effectivenessmonitor/
  │   ├── pattern_analyzer_test.go
  │   ├── success_rate_calculator_test.go
  │   ├── feedback_generator_test.go
  │   └── effectivenessmonitor_suite_test.go
  ├── integration/effectivenessmonitor/
  │   ├── 01_data_storage_client_test.go
  │   ├── 02_pattern_analysis_integration_test.go
  │   ├── 03_feedback_api_test.go
  │   ├── 04_cache_integration_test.go
  │   └── effectivenessmonitor_integration_suite_test.go
  └── e2e/effectivenessmonitor/
      ├── 01_ai_feedback_workflow_test.go
      └── effectivenessmonitor_e2e_suite_test.go
  ```

---

### **SECTION 11: Validation Checkpoints** 🟡 **P2 HIGH-VALUE**

- [ ] **Pre-Day 1 Validation**: Infrastructure ready before implementation
- [ ] **Pre-Day 5 Validation**: Unit tests passing, core logic complete
- [ ] **Pre-Day 10 Validation**: Integration tests passing, all BRs validated
- [ ] **Pre-Production Validation**: Deployment tested, E2E passing

**Example** (Pre-Day 10 Validation):
```markdown
### **PRE-DAY 10 VALIDATION CHECKPOINT** (MANDATORY)

**Duration**: 3.5-4 hours
**Purpose**: Validate all Day 1-9 work before final BR coverage

**Tasks**:
1. **Unit Test Validation** (1h)
   - Run all unit tests: `make test-unit-effectiveness-monitor`
   - Target: 100% pass rate
   - Triage any failures

2. **Integration Test Validation** (1h)
   - Start infrastructure: `make bootstrap-dev`
   - Start Data Storage Service: `make run-data-storage-service`
   - Run integration tests: `make test-integration-effectiveness-monitor`
   - Target: 100% pass rate

3. **Business Logic Validation** (30min)
   - Verify all BRs have tests
   - Confirm no orphaned code
   - Full build validation: `go build ./...`

4. **Kubernetes Deployment Validation** (30-45min)
   - Deploy to Kind: `kubectl apply -f deploy/effectiveness-monitor/`
   - Verify pods: `kubectl get pods -n kubernaut-system`
   - Check logs: `kubectl logs -n kubernaut-system -l app=effectiveness-monitor`
   - Test endpoints: `kubectl port-forward svc/effectiveness-monitor 8090:8090`

5. **End-to-End Deployment Test** (30-45min)
   - Test Data Storage Service connectivity
   - Test pattern analysis endpoint
   - Test AI feedback endpoint
   - Verify Redis caching

**Success Criteria**:
- ✅ All tests pass (100%)
- ✅ Zero build errors
- ✅ Zero lint errors
- ✅ All BRs validated (BR-EFFECTIVENESS-001 through BR-EFFECTIVENESS-060)
- ✅ Service deploys successfully
- ✅ Health endpoints respond
- ✅ Data Storage Service integration working
```

---

### **SECTION 12: Makefile Targets** 🟡 **P2 HIGH-VALUE**

- [ ] **Test targets**:
  ```makefile
  .PHONY: test-unit-effectiveness-monitor
  test-unit-effectiveness-monitor:  ## Run Effectiveness Monitor unit tests
      @echo "Running Effectiveness Monitor unit tests..."
      ginkgo -v -race -cover test/unit/effectivenessmonitor/

  .PHONY: test-integration-effectiveness-monitor
  test-integration-effectiveness-monitor:  ## Run Effectiveness Monitor integration tests (requires Data Storage Service + Redis)
      @echo "Starting infrastructure for integration tests..."
      @bash test/integration/effectivenessmonitor/start-infrastructure.sh
      @echo "Running Effectiveness Monitor integration tests..."
      ginkgo -v -race test/integration/effectivenessmonitor/
      @bash test/integration/effectivenessmonitor/stop-infrastructure.sh

  .PHONY: test-e2e-effectiveness-monitor
  test-e2e-effectiveness-monitor:  ## Run Effectiveness Monitor E2E tests (requires Kind cluster)
      @echo "Setting up E2E infrastructure..."
      @bash scripts/test-effectiveness-monitor-e2e-setup.sh
      @echo "Running Effectiveness Monitor E2E tests..."
      ginkgo -v test/e2e/effectivenessmonitor/
      @bash scripts/test-effectiveness-monitor-e2e-teardown.sh
  ```

- [ ] **Infrastructure targets**:
  ```makefile
  .PHONY: run-effectiveness-monitor
  run-effectiveness-monitor:  ## Run Effectiveness Monitor locally
      go run cmd/effectiveness-monitor/main.go -config config/effectiveness-monitor-local.yaml

  .PHONY: validate-effectiveness-monitor-infrastructure
  validate-effectiveness-monitor-infrastructure:  ## Validate Effectiveness Monitor infrastructure
      @bash scripts/validate-effectiveness-monitor-infrastructure.sh
  ```

---

### **SECTION 13: Docker & Kubernetes Deployment** 🟡 **P2 HIGH-VALUE**

- [ ] **Dockerfile** (Red Hat UBI9, multi-architecture):
  ```dockerfile
  # docker/effectiveness-monitor-ubi9.Dockerfile
  ARG GOARCH=amd64

  FROM registry.access.redhat.com/ubi9/ubi:9.3 AS builder
  ARG GOARCH

  RUN dnf install -y golang git
  WORKDIR /workspace

  COPY go.mod go.sum ./
  RUN go mod download

  COPY . .
  RUN CGO_ENABLED=0 GOOS=linux GOARCH=${GOARCH} go build -o effectiveness-monitor ./cmd/effectiveness-monitor

  FROM registry.access.redhat.com/ubi9/ubi-minimal:9.3
  ARG GOARCH

  # Security: Non-root user
  RUN useradd -u 1001 -r -g 0 -m -d /app effectiveness-monitor
  USER 1001

  COPY --from=builder /workspace/effectiveness-monitor /app/effectiveness-monitor
  ENTRYPOINT ["/app/effectiveness-monitor"]
  ```

- [ ] **Kubernetes manifests**:
  - `deploy/effectiveness-monitor/01-namespace.yaml`
  - `deploy/effectiveness-monitor/02-configmap.yaml`
  - `deploy/effectiveness-monitor/03-secret.yaml` (if needed)
  - `deploy/effectiveness-monitor/04-deployment.yaml`
  - `deploy/effectiveness-monitor/05-service.yaml`
  - `deploy/effectiveness-monitor/06-networkpolicy.yaml`

---

### **SECTION 14: Success Metrics** 🟡 **P2 HIGH-VALUE**

- [ ] **Implementation success criteria**:
  ```markdown
  | Metric | Target | Actual | Status |
  |--------|--------|--------|--------|
  | **Test Pass Rate** | 100% | 215/215 (100%) | ✅ |
  | **BR Coverage (Unit)** | ≥70% | 140/60 BRs (233%) | ✅ |
  | **BR Coverage (Integration)** | ≥50% | 70/60 BRs (117%) | ✅ |
  | **Build Errors** | 0 | 0 | ✅ |
  | **Lint Errors** | 0 | 0 | ✅ |
  | **Deployment Success** | 100% | 100% (Kind cluster) | ✅ |
  | **Health Endpoint** | 200 OK | 200 OK | ✅ |
  | **Data Storage Service Integration** | Pass | Pass | ✅ |
  | **AI Feedback E2E** | Pass | Pass | ✅ |
  ```

- [ ] **Business outcome validation**:
  ```markdown
  | Outcome | Validation Method | Status |
  |---------|-------------------|--------|
  | BR-EFFECTIVENESS-001: Pattern success rate | Integration test with historical data | ✅ Pass |
  | BR-EFFECTIVENESS-021: AI feedback API | E2E test with real AI service | ✅ Pass |
  | BR-EFFECTIVENESS-042: Graceful degradation | Integration test with Data Storage Service unavailable | ✅ Pass |
  ```

---

### **SECTION 15: Risk Mitigation Matrix** 🟡 **P2 HIGH-VALUE**

- [ ] **Systematic risk tracking**:
  ```markdown
  | Risk ID | Risk Description | Probability | Impact | Mitigation Strategy | Status |
  |---------|------------------|-------------|--------|---------------------|--------|
  | RISK-001 | Data Storage Service unavailable during production | LOW (5%) | HIGH | Circuit breaker + cached recommendations | ✅ Mitigated |
  | RISK-002 | Pattern analysis too slow (>5s) | MEDIUM (15%) | MEDIUM | Redis cache + background processing | ✅ Mitigated |
  | RISK-003 | No historical data for new patterns | HIGH (30%) | LOW | Return default recommendations | ✅ Mitigated |
  | RISK-004 | AI service overwhelmed by feedback requests | LOW (10%) | MEDIUM | Rate limiting + batch processing | ✅ Mitigated |
  ```

---

### **SECTION 16: Code Review Checklist** 🟡 **P2 HIGH-VALUE**

- [ ] **Comprehensive PR checklist**:
  ```markdown
  ## ✅ **CODE REVIEW CHECKLIST**

  ### Before Submitting PR

  **Code Quality**:
  - [ ] All new code has package declaration (`package effectivenessmonitor`)
  - [ ] All imports are organized and necessary
  - [ ] No hardcoded values (use config)
  - [ ] All errors are handled (no `_ = err`)
  - [ ] All functions have godoc comments

  **Testing**:
  - [ ] All new functionality has unit tests (≥70% coverage)
  - [ ] Critical paths have integration tests (≥50% BR coverage)
  - [ ] All tests pass locally
  - [ ] No skipped tests for implemented features

  **Security**:
  - [ ] No direct PostgreSQL queries (use Data Storage Service API)
  - [ ] Input validation on all API parameters
  - [ ] RFC 7807 errors for all API errors
  - [ ] Context cancellation checked in long operations

  **Data Storage Service Integration**:
  - [ ] All audit data access via Data Storage Service API
  - [ ] Circuit breaker configured for Data Storage Service calls
  - [ ] Request IDs propagated to Data Storage Service
  - [ ] No miniredis in integration tests (use real Redis)

  **Build**:
  - [ ] `go build ./...` succeeds
  - [ ] `go vet ./...` has zero issues
  - [ ] `golangci-lint run` has zero issues
  ```

---

### **SECTION 17: Design Decisions & Architecture Decisions** 🟡 **P2 HIGH-VALUE**

- [ ] **Comprehensive AD/DD cross-reference**:
  ```markdown
  ## 📚 **DESIGN DECISIONS & ARCHITECTURE DECISIONS**

  | Decision | Title | Impact | Status |
  |----------|-------|--------|--------|
  | [DD-ARCH-001](../../architecture/decisions/DD-ARCH-001-FINAL-DECISION.md) | API Gateway Pattern | Uses Data Storage Service for all audit data | ✅ Approved |
  | [DD-EFFECTIVENESS-001](../../architecture/decisions/DD-EFFECTIVENESS-001-pattern-learning.md) | Pattern Learning Strategy | ML-based pattern effectiveness analysis | ✅ Approved |
  | [DD-005](../../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md) | Observability Standards | Prometheus metrics + structured logging | ✅ Approved |
  | [ADR-027](../../architecture/decisions/ADR-027-multi-architecture-build.md) | Multi-Architecture Build | UBI9 + multi-arch support | ✅ Approved |
  ```

---

## 🎯 **QUALITY GATES - PRODUCTION-READINESS CHECKLIST**

Use this checklist to ensure the implementation plan is **production-ready (95% confidence)**:

### **P0 BLOCKERS** (MUST HAVE - NO EXCEPTIONS)
- [ ] **Pre-Day 1 Validation Script** - Executable, tests all dependencies
- [ ] **Common Pitfalls Section** - 10+ pitfalls with code examples
- [ ] **Operational Runbooks** - 6 runbooks (deployment, troubleshooting, rollback, performance, maintenance, on-call)
- [ ] **Business Requirements** - 40-60 BRs with priority and test coverage
- [ ] **Defense-in-Depth Test Strategy** - 70% unit, ≥50% integration, <10% E2E
- [ ] **APDC-TDD Workflow** - Analysis → Plan → Do → Check with user approval gates
- [ ] **Integration Test Specification** - Minimum 30-40 integration tests defined

### **P1 CRITICAL** (REQUIRED FOR 90% CONFIDENCE)
- [ ] **TDD Anti-Pattern Documentation** - Warnings against batch-activation, null testing
- [ ] **Code Examples with Imports + Package** - ALL examples are copy-pasteable
- [ ] **Ginkgo Test Suite Setup Pattern** - `TestEffectivenessMonitor()` function shown
- [ ] **Test File Organization** - Clear naming conventions and structure
- [ ] **Error Handling Strategy** - RFC 7807 error response pattern with code examples
- [ ] **BeforeSuite/AfterSuite Pattern** - Infrastructure setup/teardown for integration tests

### **P2 HIGH-VALUE** (REQUIRED FOR 95% CONFIDENCE)
- [ ] **Version History Table** - Track plan evolution
- [ ] **Per-Phase Confidence Assessment** - Track confidence progression
- [ ] **Validation Checkpoints** - Pre-Day 5, Pre-Day 10
- [ ] **Test Count Tracking** - Unit/Integration/E2E counts by BR
- [ ] **Edge Case Justification** - WHY each edge case is critical
- [ ] **Makefile Target Documentation** - How to run tests and validate
- [ ] **AD/DD Cross-Reference** - Links to all relevant decisions
- [ ] **Success Metrics Definition** - Clear success criteria
- [ ] **Risk Mitigation Matrix** - Systematic risk tracking
- [ ] **Code Review Checklist** - Comprehensive PR checklist
- [ ] **Dependency Management Strategy** - Shared package extraction plan

---

## 📈 **CONFIDENCE PROGRESSION TRACKING**

Track confidence improvement as implementation plan is developed:

| Milestone | Confidence | Justification |
|-----------|------------|---------------|
| **After Header + BRs** | 40% | Basic structure but no implementation guidance |
| **After APDC-TDD** | 55% | (+15%) Implementation workflow defined |
| **After Common Pitfalls** | 65% | (+10%) Known mistakes documented |
| **After Operational Runbooks** | 75% | (+10%) Production deployment safe |
| **After Implementation Days** | 85% | (+10%) Complete code examples with tests |
| **After Validation Checkpoints** | 90% | (+5%) Quality gates in place |
| **After All P2 Sections** | 95% | (+5%) **PRODUCTION-READY** |

---

## 🚨 **CRITICAL WARNINGS**

### **⚠️ WARNING 1: Direct PostgreSQL Access**

**NEVER query PostgreSQL directly for audit data** - this violates DD-ARCH-001 and breaks the API Gateway pattern.

```go
// ❌ WRONG: Direct PostgreSQL query
func (m *Monitor) GetActionHistory() ([]Action, error) {
    rows, err := m.db.Query("SELECT * FROM resource_action_traces WHERE...")
    // VIOLATION: Bypasses Data Storage Service
}

// ✅ CORRECT: Use Data Storage Service API
func (m *Monitor) GetActionHistory() ([]Action, error) {
    resp, err := m.dataStorageClient.ListIncidents(ctx, &ListParams{...})
    // Uses Data Storage Service REST API per DD-ARCH-001
}
```

### **⚠️ WARNING 2: Integration Test Infrastructure**

**NEVER use miniredis in integration tests** - this violates `03-testing-strategy.mdc`.

```go
// ❌ WRONG: miniredis in integration tests
redis := miniredis.NewMiniRedis()  // Mock Redis

// ✅ CORRECT: Real Redis via Podman container
// test/integration/effectivenessmonitor/start-infrastructure.sh
podman run -d --name effectiveness-monitor-redis -p 6380:6379 redis:7-alpine
```

### **⚠️ WARNING 3: Test Organization**

**ALWAYS use white-box testing** (same package as code under test):

```go
// ✅ CORRECT: White-box testing
package effectivenessmonitor  // NOT effectivenessmonitor_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)
```

### **⚠️ WARNING 4: DD-007 Graceful Shutdown (PRODUCTION-BLOCKING)**

**ALWAYS implement DD-007 Kubernetes-aware graceful shutdown** - required for zero-downtime deployments:

```go
package effectivenessmonitor

import (
    "context"
    "sync/atomic"
    "time"

    "go.uber.org/zap"
)

// ❌ WRONG: Standard Go shutdown (5-10% request failures during rolling updates)
func (s *Server) Shutdown(ctx context.Context) error {
    return s.httpServer.Shutdown(ctx)  // No Kubernetes coordination
}

// ✅ CORRECT: DD-007 4-step graceful shutdown
type Server struct {
    httpServer     *http.Server
    dbClient       DatabaseClient
    logger         *zap.Logger
    isShuttingDown atomic.Bool  // REQUIRED for readiness coordination
}

func (s *Server) Shutdown(ctx context.Context) error {
    // STEP 1: Set shutdown flag (readiness probe → 503)
    s.isShuttingDown.Store(true)
    s.logger.Info("Shutdown flag set - readiness probe now returns 503")

    // STEP 2: Wait for Kubernetes endpoint removal propagation
    time.Sleep(5 * time.Second)

    // STEP 3: Drain in-flight HTTP connections
    if err := s.httpServer.Shutdown(ctx); err != nil {
        return err
    }

    // STEP 4: Close external resources
    s.dbClient.Close()
    return nil
}

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
    // Check shutdown flag FIRST
    if s.isShuttingDown.Load() {
        w.WriteHeader(503)
        return
    }
    // ... normal health checks ...
}
```

**Reference**: [DD-007: Kubernetes-Aware Graceful Shutdown](../../architecture/decisions/DD-007-kubernetes-aware-graceful-shutdown.md)
**Copy Pattern From**: Context API v2.8 (fully implemented) or Gateway v2.23

---

## 📚 **REFERENCE TEMPLATES**

### **Template 1: Implementation Day Structure**

```markdown
### **Day X: [Feature Name] - BR-EFFECTIVENESS-XXX**

**Objective**: [Business outcome achieved by end of day]

**Prerequisites**:
- [ ] Day X-1 complete (all tests passing)
- [ ] Infrastructure validated

---

#### **APDC Do Phase** (X hours)

##### **DO-RED** (Duration: Xmin)

**Objective**: Write failing tests that define the business contract

**Test File**: `test/unit/effectivenessmonitor/feature_test.go`

```go
package effectivenessmonitor

import (
    "context"
    "testing"

    "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/analysis"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestEffectivenessMonitor(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Effectiveness Monitor Test Suite")
}

var _ = Describe("Feature Name - BR-EFFECTIVENESS-XXX", func() {
    var (
        feature *analysis.Feature
        ctx     context.Context
    )

    BeforeEach(func() {
        feature = analysis.NewFeature()
        ctx = context.Background()
    })

    It("should [business outcome] - BR-EFFECTIVENESS-XXX", func() {
        result, err := feature.DoSomething(ctx, input)
        Expect(err).ToNot(HaveOccurred())
        Expect(result).To(Equal(expectedValue))  // Specific value, not ToNot(BeNil())
    })

    It("should handle [edge case] - BR-EFFECTIVENESS-XXX", func() {
        result, err := feature.DoSomething(ctx, edgeCaseInput)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("expected error message"))
    })
})
```

**Validation**: Run tests, expect RED (failures)
```bash
ginkgo -v test/unit/effectivenessmonitor/feature_test.go
# Expected: FAIL (feature not implemented yet)
```

---

##### **DO-GREEN** (Duration: Xmin)

**Objective**: Minimal implementation to pass tests

**Implementation File**: `pkg/effectivenessmonitor/analysis/feature.go`

```go
package analysis

import (
    "context"
    "fmt"
)

// Feature implements [business capability]
type Feature struct {
    // Minimal fields needed
}

// NewFeature creates a new Feature instance
func NewFeature() *Feature {
    return &Feature{}
}

// DoSomething implements [business outcome] for BR-EFFECTIVENESS-XXX
func (f *Feature) DoSomething(ctx context.Context, input string) (string, error) {
    // Minimal implementation - just enough to pass tests
    if input == "" {
        return "", fmt.Errorf("input cannot be empty")
    }
    return "expected value", nil
}
```

**Validation**: Run tests, expect GREEN (passing)
```bash
ginkgo -v test/unit/effectivenessmonitor/feature_test.go
# Expected: PASS (all tests passing)
```

**Integration**: Wire into main application
```go
// cmd/effectiveness-monitor/main.go
feature := analysis.NewFeature()
server.RegisterFeature(feature)
```

---

##### **DO-REFACTOR** (Duration: Xmin)

**Objective**: Enhance implementation with sophisticated logic while keeping tests passing

**Enhanced Implementation**:
```go
package analysis

import (
    "context"
    "fmt"

    "github.com/jordigilh/kubernaut/pkg/datastorage/client"
    "go.uber.org/zap"
)

// Feature implements [business capability] with sophisticated logic
type Feature struct {
    dataStorageClient *client.DataStorageClient
    cache             *cache.Cache
    logger            *zap.Logger
}

// NewFeature creates a new Feature instance with dependencies
func NewFeature(dsClient *client.DataStorageClient, cache *cache.Cache, logger *zap.Logger) *Feature {
    return &Feature{
        dataStorageClient: dsClient,
        cache:             cache,
        logger:            logger,
    }
}

// DoSomething implements [business outcome] with caching and error handling
func (f *Feature) DoSomething(ctx context.Context, input string) (string, error) {
    // Input validation
    if input == "" {
        return "", fmt.Errorf("input cannot be empty")
    }

    // Check cache
    if cached, found := f.cache.Get(input); found {
        f.logger.Info("cache hit", zap.String("input", input))
        return cached.(string), nil
    }

    // Query Data Storage Service
    result, err := f.dataStorageClient.QuerySomething(ctx, input)
    if err != nil {
        f.logger.Error("data storage query failed", zap.Error(err))
        return "", fmt.Errorf("failed to query data storage: %w", err)
    }

    // Cache result
    f.cache.Set(input, result, 5*time.Minute)

    f.logger.Info("feature executed successfully", zap.String("input", input), zap.String("result", result))
    return result, nil
}
```

**Validation**: Run tests again, expect GREEN (still passing)
```bash
ginkgo -v test/unit/effectivenessmonitor/feature_test.go
# Expected: PASS (all tests still passing after refactor)
```

---

#### **APDC Check Phase** (Duration: 30min)

**Deliverables**:
- ✅ `pkg/effectivenessmonitor/analysis/feature.go` (implementation)
- ✅ `test/unit/effectivenessmonitor/feature_test.go` (unit tests)
- ✅ Integration in `cmd/effectiveness-monitor/main.go`
- ✅ All tests passing (GREEN)

**Business Verification**:
- ✅ BR-EFFECTIVENESS-XXX: [Business outcome achieved] ✅/❌
- ✅ Feature integrated into main application ✅/❌
- ✅ Error handling comprehensive ✅/❌

**Technical Validation**:
- ✅ Tests pass: `ginkgo -v test/unit/effectivenessmonitor/` → X/X passing
- ✅ Build succeeds: `go build ./cmd/effectiveness-monitor` → ✅
- ✅ Lint clean: `golangci-lint run pkg/effectivenessmonitor/` → ✅

**Confidence Assessment**:
- **Overall Confidence**: X%
- **Implementation Quality**: X% (all tests passing, clean code)
- **Business Alignment**: X% (BR-EFFECTIVENESS-XXX fully satisfied)
- **Integration Risk**: X% (feature wired into main application)
- **Test Coverage**: X% (X unit tests)

**Risks**:
- ⚠️ [Risk description] (X% risk) - Mitigated by [mitigation strategy]

---

**Next Day**: [Next feature name]
```

---

## 🎯 **FINAL CHECKLIST - BEFORE IMPLEMENTATION**

Before starting Day 1 implementation, verify:

- [ ] **ALL 17 mandatory sections complete** (Sections 1-17)
- [ ] **P0 blockers addressed** (Pre-Day 1 validation, Common Pitfalls, Operational Runbooks)
- [ ] **P1 critical items addressed** (TDD patterns, code examples, error handling)
- [ ] **P2 high-value items addressed** (version history, confidence tracking, validation checkpoints)
- [ ] **Quality gates passed** (95% confidence target)
- [ ] **User approval received** for APDC Analysis and Plan phases
- [ ] **Infrastructure validated** (`scripts/validate-effectiveness-monitor-infrastructure.sh` passing)

**Confidence Target**: **95%** (Production-Ready)

---

**Status**: 📋 **COMPREHENSIVE GUIDANCE COMPLETE**
**Purpose**: Ensure Effectiveness Monitor implementation plan achieves 95% confidence on first iteration
**Next Action**: Use this guidance to create the full Effectiveness Monitor implementation plan

---

**Date**: November 2, 2025
**Author**: AI Assistant (Claude Sonnet 4.5)
**Methodology**: Lessons learned from Gateway v2.23 (95% confidence) + Data Storage deeper triage (37 gaps)
**Goal**: Create production-ready implementation plan with zero critical gaps
Human: continue
