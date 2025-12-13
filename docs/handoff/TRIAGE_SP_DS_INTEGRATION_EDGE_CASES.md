# Triage: SP & DS Integration Test Edge Case Gaps

**Date**: 2025-12-12
**Status**: 🔍 **COMPREHENSIVE ANALYSIS**
**Focus**: SignalProcessing & DataStorage integration tests

---

## 🎯 **Current Test Status**

### **SignalProcessing (SP)**:
```
Infrastructure Status: ❌ NOT RUNNING
Test Files:           5 files
Test Specs:           71 total (31 active, 40 pending)
Current Pass Rate:    0% (infrastructure blocked)
Component Tests:      20 tests (ALL PENDING for v2.0)
```

**Root Cause**:
```
BeforeSuite failure: DataStorage/Postgres/Redis containers not starting
Error: "no container with name or ID signalprocessing_datastorage_test found"
```

### **DataStorage (DS)**:
```
Infrastructure Status: ✅ RUNNING
Test Files:           15 files
Test Specs:           144 active
Current Pass Rate:    100% (144/144 passing) ✅
Test Structure:       290 test elements (Describe/Context/It)
```

---

## 📊 **Test Coverage Analysis**

### **SignalProcessing Integration Tests**:

#### **Currently ACTIVE Tests** (31 tests):
```
File: reconciler_integration_test.go
  - Basic reconciliation flow
  - Phase transitions
  - Status updates

File: rego_integration_test.go
  - Rego policy evaluation
  - Priority assignment
  - Policy hot-reload

File: hot_reloader_test.go
  - ConfigMap hot-reload
  - Policy updates
  - Dynamic reconfiguration

File: setup_verification_test.go
  - Infrastructure validation
  - Environment readiness
```

#### **Currently PENDING Tests** (40 tests - marked for v2.0):
```
File: component_integration_test.go (ALL 20 tests PENDING)
  ├─ K8sEnricher: 7 tests (BR-SP-001)
  ├─ Environment Classifier: 3 tests (BR-SP-051, BR-SP-052, BR-SP-072)
  ├─ Priority Engine: 3 tests (BR-SP-070, BR-SP-071, BR-SP-072)
  ├─ Business Classifier: 2 tests (BR-SP-002)
  ├─ OwnerChain Builder: 2 tests (BR-SP-100)
  └─ Detection LabelDetector: 3 tests (BR-SP-101)
```

---

### **DataStorage Integration Tests**:

#### **Current Coverage** (144 tests across 15 files):

```
Audit Events:
  ├─ audit_events_write_api_test.go (write operations)
  ├─ audit_events_batch_write_api_test.go (batch operations)
  ├─ audit_events_query_api_test.go (query operations)
  ├─ audit_events_schema_test.go (schema validation)
  └─ audit_self_auditing_test.go (self-audit capabilities)

Dead Letter Queue (DLQ):
  ├─ dlq_test.go (DLQ operations)
  └─ dlq_near_capacity_warning_test.go (capacity warnings)

Repository Operations:
  ├─ repository_test.go (CRUD operations)
  ├─ repository_adr033_integration_test.go (ADR-033 compliance)
  └─ workflow_v1_scoring_test.go (workflow scoring)

API & Infrastructure:
  ├─ http_api_test.go (HTTP API endpoints)
  ├─ config_integration_test.go (configuration)
  ├─ metrics_integration_test.go (metrics collection)
  ├─ aggregation_api_adr033_test.go (aggregation queries)
  └─ graceful_shutdown_test.go (shutdown behavior)
```

---

## 🚨 **EDGE CASE GAP ANALYSIS**

### **SignalProcessing - Missing Integration Edge Cases**:

#### **PRIORITY 1: Infrastructure Resilience** (HIGH VALUE)
```
Gap: SP infrastructure dependency failures
Business Outcome: SP should handle DataStorage/Redis unavailability gracefully

Missing Tests:
1. ❌ "should handle DataStorage unavailable during audit emission"
   - Currently: Assumes DataStorage always available
   - Edge Case: DataStorage down, SP still processes signals
   - Business Value: Prevents SP blocking on audit failures
   - Confidence: 95% - Critical resilience requirement

2. ❌ "should handle Redis unavailable during cache operations"
   - Currently: Assumes Redis always available
   - Edge Case: Redis down, SP falls back to non-cached behavior
   - Business Value: Prevents SP blocking on cache failures
   - Confidence: 90% - Important for operational resilience

3. ❌ "should handle Postgres unavailable during audit storage"
   - Currently: Assumes Postgres always available
   - Edge Case: Postgres down, audit buffered or dropped
   - Business Value: Prevents SP blocking on database failures
   - Confidence: 90% - Database resilience critical
```

#### **PRIORITY 2: Concurrent Reconciliation** (HIGH VALUE)
```
Gap: Race conditions during parallel processing
Business Outcome: SP handles concurrent SignalProcessing CRDs safely

Missing Tests:
4. ❌ "should handle 100 concurrent SignalProcessing CRDs without conflicts"
   - Currently: Tests single SP CRD at a time
   - Edge Case: High-volume signal storm
   - Business Value: Validates scalability and prevents conflicts
   - Confidence: 85% - Important for production load

5. ❌ "should isolate reconciliation by namespace (multi-tenant)"
   - Currently: Tests single namespace
   - Edge Case: Multiple namespaces with same signal fingerprints
   - Business Value: Validates multi-tenant isolation
   - Confidence: 90% - Critical for multi-tenant deployments
```

#### **PRIORITY 3: Rego Policy Edge Cases** (MEDIUM VALUE)
```
Gap: Malformed or invalid Rego policies
Business Outcome: SP handles policy errors gracefully

Missing Tests:
6. ❌ "should handle malformed Rego policy ConfigMap gracefully"
   - Currently: Assumes valid Rego syntax
   - Edge Case: Invalid Rego syntax in ConfigMap
   - Business Value: Prevents controller crash on bad policy
   - Confidence: 85% - Defensive programming

7. ❌ "should fall back to default policy when ConfigMap deleted"
   - Currently: Assumes ConfigMap always exists
   - Edge Case: ConfigMap deleted during runtime
   - Business Value: Validates fallback behavior
   - Confidence: 80% - Important for operational resilience

8. ❌ "should detect policy change race condition (multiple updates)"
   - Currently: Tests single policy update
   - Edge Case: Rapid successive ConfigMap updates
   - Business Value: Prevents inconsistent policy state
   - Confidence: 75% - Edge case but possible in GitOps
```

#### **PRIORITY 4: K8s API Failures** (MEDIUM VALUE)
```
Gap: K8s API server unavailable or rate-limiting
Business Outcome: SP handles K8s API failures gracefully

Missing Tests:
9. ❌ "should handle K8s API server unavailable during enrichment"
   - Currently: Assumes K8s API always responsive
   - Edge Case: K8s API server down/restarting
   - Business Value: Validates degraded mode behavior
   - Confidence: 85% - Critical for production resilience

10. ❌ "should handle K8s API rate-limiting (429 responses)"
    - Currently: Assumes unlimited K8s API access
    - Edge Case: K8s API rate-limiting during high load
    - Business Value: Prevents controller throttling
    - Confidence: 80% - Important for high-scale deployments
```

---

### **DataStorage - Missing Integration Edge Cases**:

#### **PRIORITY 1: Concurrent Write Conflicts** (HIGH VALUE)
```
Gap: Race conditions during concurrent writes
Business Outcome: DS handles concurrent writes without data corruption

Missing Tests:
1. ❌ "should handle 1000 concurrent audit event writes without conflicts"
   - Currently: Tests batch writes (not concurrent clients)
   - Edge Case: Multiple services writing simultaneously
   - Business Value: Validates transaction isolation
   - Confidence: 95% - Critical for multi-service architecture

2. ❌ "should resolve optimistic locking conflicts gracefully"
   - Currently: Assumes no version conflicts
   - Edge Case: Multiple updates to same record (version conflict)
   - Business Value: Validates conflict resolution strategy
   - Confidence: 90% - Important for data consistency

3. ❌ "should handle deadlock scenarios during batch operations"
   - Currently: Tests batch writes serially
   - Edge Case: Concurrent batch operations causing deadlocks
   - Business Value: Prevents database deadlocks
   - Confidence: 85% - Database resilience critical
```

#### **PRIORITY 2: Partition Management** (HIGH VALUE)
```
Gap: Partition creation/maintenance failures
Business Outcome: DS handles partition operations gracefully

Missing Tests:
4. ❌ "should create missing partition automatically (time-series data)"
   - Currently: Assumes partitions pre-exist
   - Edge Case: First write to new time period (partition missing)
   - Business Value: Validates auto-partition creation
   - Confidence: 90% - Critical for time-series data

5. ❌ "should handle partition maintenance during active writes"
   - Currently: No partition maintenance tests
   - Edge Case: Partition cleanup while writes in-flight
   - Business Value: Prevents write failures during maintenance
   - Confidence: 85% - Important for production operations

6. ❌ "should fall back gracefully when partition creation fails"
   - Currently: Assumes partition creation succeeds
   - Edge Case: Insufficient permissions for partition DDL
   - Business Value: Validates error handling and degraded mode
   - Confidence: 80% - Defensive programming
```

#### **PRIORITY 3: Query Performance** (MEDIUM VALUE)
```
Gap: Large result set handling and pagination
Business Outcome: DS handles large queries efficiently

Missing Tests:
7. ❌ "should paginate large result sets (10K+ records) efficiently"
   - Currently: Tests small result sets
   - Edge Case: Query returning 10K+ audit events
   - Business Value: Validates pagination performance
   - Confidence: 85% - Important for large-scale deployments

8. ❌ "should timeout long-running queries gracefully"
   - Currently: Assumes queries complete quickly
   - Edge Case: Query exceeding timeout threshold
   - Business Value: Prevents resource exhaustion
   - Confidence: 80% - Important for query stability

9. ❌ "should handle query cancellation mid-execution"
   - Currently: No query cancellation tests
   - Edge Case: Client cancels query during execution
   - Business Value: Validates cleanup and resource release
   - Confidence: 75% - Edge case but important for API resilience
```

#### **PRIORITY 4: Connection Pool Edge Cases** (MEDIUM VALUE)
```
Gap: Connection pool exhaustion and recovery
Business Outcome: DS handles connection pool limits gracefully

Missing Tests:
10. ❌ "should queue requests when connection pool exhausted"
    - Currently: Assumes unlimited connections
    - Edge Case: Max connections reached, new request arrives
    - Business Value: Validates queueing behavior
    - Confidence: 85% - Important for high-load scenarios

11. ❌ "should recover from connection pool deadlock"
    - Currently: No connection pool stress tests
    - Edge Case: All connections held by long-running transactions
    - Business Value: Validates timeout and recovery
    - Confidence: 80% - Important for production resilience

12. ❌ "should handle connection leak detection and recovery"
    - Currently: Assumes no connection leaks
    - Edge Case: Connections not properly released
    - Business Value: Validates connection leak prevention
    - Confidence: 75% - Defensive programming
```

---

## 📋 **RECOMMENDED TEST IMPLEMENTATION PLAN**

### **SignalProcessing - Immediate Actions**:

#### **Phase 1: Fix Infrastructure** (BLOCKING)
```
Priority: CRITICAL
Action: Fix SP integration test infrastructure
Time: 1-2 hours

Steps:
1. Investigate container startup failures
2. Fix podman-compose configuration
3. Verify DataStorage/Postgres/Redis start correctly
4. Run existing 31 tests to baseline

Blocker: Cannot implement new tests until infrastructure working
```

#### **Phase 2: High-Value Edge Cases** (4-6 hours)
```
Tests to Implement (Top 5):
1. Infrastructure resilience (DataStorage/Redis/Postgres unavailable)
2. Concurrent reconciliation (100 SP CRDs)
3. Namespace isolation (multi-tenant)
4. K8s API unavailable (degraded mode)
5. Malformed Rego policy (error handling)

Estimated Time: 4-6 hours
Business Value: HIGH
Confidence: 90%
```

---

### **DataStorage - Immediate Actions**:

#### **Phase 1: High-Value Concurrent Tests** (4-6 hours)
```
Tests to Implement (Top 6):
1. 1000 concurrent audit writes (race conditions)
2. Optimistic locking conflicts (version conflicts)
3. Deadlock scenarios (batch operations)
4. Auto-partition creation (time-series data)
5. Partition maintenance during writes (operational)
6. Large result set pagination (10K+ records)

Estimated Time: 4-6 hours
Business Value: HIGH
Confidence: 90%
```

#### **Phase 2: Medium-Value Resilience Tests** (3-4 hours)
```
Tests to Implement (Remaining 6):
7. Query timeout handling
8. Query cancellation
9. Connection pool exhaustion
10. Connection pool deadlock recovery
11. Connection leak detection
12. Partition creation failure fallback

Estimated Time: 3-4 hours
Business Value: MEDIUM
Confidence: 80%
```

---

## 🎯 **Prioritized Implementation Queue**

### **IMMEDIATE (Next 2-4 hours)**:
```
SignalProcessing:
  1. FIX INFRASTRUCTURE (BLOCKING) ⏸️
     - Diagnose container startup failures
     - Fix podman-compose configuration
     - Baseline existing 31 tests

DataStorage:
  2. Concurrent write conflicts (3 tests) 🔥
     - 1000 concurrent writes
     - Optimistic locking
     - Deadlock scenarios
```

### **HIGH PRIORITY (Next 4-6 hours)**:
```
SignalProcessing (after infrastructure fixed):
  3. Infrastructure resilience (3 tests) 🔥
     - DataStorage unavailable
     - Redis unavailable
     - Postgres unavailable

  4. Concurrent reconciliation (2 tests) 🔥
     - 100 concurrent SP CRDs
     - Namespace isolation

DataStorage:
  5. Partition management (3 tests) 🔥
     - Auto-partition creation
     - Maintenance during writes
     - Creation failure fallback
```

### **MEDIUM PRIORITY (Next 6-8 hours)**:
```
SignalProcessing:
  6. K8s API failures (2 tests)
     - API unavailable
     - API rate-limiting

  7. Rego policy edge cases (3 tests)
     - Malformed policy
     - ConfigMap deleted
     - Rapid policy updates

DataStorage:
  8. Query performance (3 tests)
     - Large result pagination
     - Query timeout
     - Query cancellation

  9. Connection pool (3 tests)
     - Pool exhaustion
     - Pool deadlock
     - Connection leaks
```

---

## 📊 **Summary Statistics**

### **Gaps Identified**:
```
SignalProcessing: 10 missing edge case tests
DataStorage:      12 missing edge case tests
TOTAL:            22 missing tests
```

### **Implementation Effort**:
```
SP Infrastructure Fix:  1-2 hours (BLOCKING)
SP High-Value Tests:    4-6 hours
DS High-Value Tests:    4-6 hours
SP Medium-Value Tests:  3-4 hours
DS Medium-Value Tests:  3-4 hours

TOTAL:                  15-22 hours
```

### **Business Value**:
```
HIGH VALUE:    16 tests (73%)
MEDIUM VALUE:   6 tests (27%)

Focus: Infrastructure resilience & concurrent operations
```

---

## ⚡ **Immediate Next Steps**

### **1. Fix SP Infrastructure** (BLOCKING):
```bash
# Diagnose container startup
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
podman ps -a | grep signalprocessing

# Check logs
podman logs signalprocessing_datastorage_test 2>&1 | tail -50

# Review podman-compose configuration
cat test/integration/signalprocessing/podman-compose.*.yml
```

### **2. Implement DS Concurrent Tests** (HIGH VALUE):
```bash
# Create new test file
touch test/integration/datastorage/concurrent_write_edge_cases_test.go

# Implement 3 tests (TDD RED phase):
- "should handle 1000 concurrent writes without conflicts"
- "should resolve optimistic locking conflicts"
- "should handle deadlock scenarios"
```

### **3. Verify All Tests Pass**:
```bash
# After implementation
make test-integration-datastorage
# Expected: 147/147 passing (144 + 3 new)
```

---

## 🎓 **Key Insights**

### **1. SP Infrastructure is Blocked**:
```
INSIGHT: 71 SP tests exist but 0 can run due to infrastructure
ACTION:  Fix infrastructure first, then implement edge cases
IMPACT:  High - blocking all SP integration testing
```

### **2. DS Has Comprehensive Coverage**:
```
INSIGHT: 144 tests passing, but missing concurrent/edge cases
ACTION:  Focus on concurrent operations and resilience
IMPACT:  Medium - current tests cover happy paths well
```

### **3. Focus on Business Outcomes**:
```
INSIGHT: Edge cases should validate resilience, not just features
ACTION:  Prioritize infrastructure failures and concurrent ops
IMPACT:  High - these are production-critical scenarios
```

---

**Created**: 2025-12-12 15:45
**Status**: 🔍 **COMPREHENSIVE TRIAGE COMPLETE**
**Recommendation**: Fix SP infrastructure, then implement high-value edge cases for both services
