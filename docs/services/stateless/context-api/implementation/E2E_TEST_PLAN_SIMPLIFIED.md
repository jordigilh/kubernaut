# Day 12 E2E Tests - Simplified Infrastructure Plan

**Date**: 2025-11-05
**Status**: PLANNING
**Infrastructure**: Context API + Data Storage + Redis + PostgreSQL (4 components only)

---

## 🎯 **Objective**

Validate the **complete Context API aggregation flow** using only the **4 essential infrastructure components**:
1. **PostgreSQL** (database)
2. **Redis** (cache)
3. **Data Storage Service** (REST API for data access)
4. **Context API** (REST API for aggregation)

**No AI/LLM Service required** - E2E tests will simulate AI client behavior using HTTP requests.

---

## 📋 **Simplified E2E Test Scenarios**

### **Test 1: End-to-End Aggregation Flow** (P0 - Critical)
**Scenario**: Simulate AI client querying incident-type success rate

**Flow**:
```
1. Seed Data Storage with test data (via REST API)
   POST /api/v1/notification-audit (3 pod-oom incidents)
   
2. AI Client queries Context API
   GET /api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom
   
3. Context API → Data Storage Service → PostgreSQL
   
4. Response returned to AI Client
   {
     "incident_type": "pod-oom",
     "success_rate": 66.67,
     "total_executions": 3,
     "successful_executions": 2,
     "confidence": "medium"
   }
```

**Validation**:
- ✅ HTTP 200 OK
- ✅ Correct success rate calculation (2/3 = 66.67%)
- ✅ Confidence level matches sample size
- ✅ All 4 services working together

---

### **Test 2: Cache Effectiveness** (P0 - Critical)
**Scenario**: Verify Redis caching reduces Data Storage calls

**Flow**:
```
1. First request (cache miss)
   GET /api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom
   → Context API calls Data Storage Service
   → Data Storage queries PostgreSQL
   → Result cached in Redis
   
2. Second request (cache hit)
   GET /api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom
   → Context API returns from Redis cache
   → NO Data Storage call
   
3. Wait for TTL expiration (5 minutes default)
   
4. Third request (cache expired, miss again)
   → Context API calls Data Storage Service again
```

**Validation**:
- ✅ First request: Data Storage called (cache miss)
- ✅ Second request: Data Storage NOT called (cache hit)
- ✅ Cache hit rate > 50% (1 hit out of 2 requests)
- ✅ Response time: First request > Second request (cache faster)

---

### **Test 3: Data Storage Service Failure** (P1 - High)
**Scenario**: Context API handles Data Storage unavailability

**Flow**:
```
1. Warm cache with initial request
   GET /api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom
   → Result cached in Redis
   
2. Stop Data Storage Service (simulate failure)
   
3. Request with cached data
   GET /api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom
   → Context API returns from Redis cache (graceful degradation)
   → HTTP 200 OK
   
4. Request with no cached data
   GET /api/v1/aggregation/success-rate/incident-type?incident_type=disk-full
   → Context API cannot reach Data Storage
   → HTTP 503 Service Unavailable (RFC 7807 error)
```

**Validation**:
- ✅ Cached data: HTTP 200 OK (graceful degradation)
- ✅ Uncached data: HTTP 503 (proper error handling)
- ✅ RFC 7807 error response with retry-after header
- ✅ No Context API crashes or panics

---

### **Test 4: Multi-Dimensional Query** (P1 - High)
**Scenario**: Verify complex aggregation queries work end-to-end

**Flow**:
```
1. Seed Data Storage with multi-dimensional data
   - 5 pod-oom incidents with playbook-restart-v1
   - 3 pod-oom incidents with playbook-restart-v2
   
2. Query by incident_type only
   GET /api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom
   → Returns aggregated data (8 total executions)
   
3. Query by incident_type + playbook_id
   GET /api/v1/aggregation/success-rate/multi-dimensional?incident_type=pod-oom&playbook_id=playbook-restart-v1
   → Returns filtered data (5 executions for v1)
   
4. Query by playbook_id only
   GET /api/v1/aggregation/success-rate/playbook?playbook_id=playbook-restart-v1
   → Returns playbook-specific data (5 executions)
```

**Validation**:
- ✅ Incident-type aggregation: 8 total executions
- ✅ Multi-dimensional: 5 executions (filtered correctly)
- ✅ Playbook aggregation: 5 executions
- ✅ All queries return consistent data

---

### **Test 5: Concurrent Requests** (P2 - Medium)
**Scenario**: Verify Context API handles concurrent load

**Flow**:
```
1. Seed Data Storage with test data
   
2. Send 50 concurrent requests
   - 25 requests: incident-type=pod-oom
   - 25 requests: incident-type=disk-full
   
3. All requests complete successfully
   
4. Verify cache effectiveness
   - First request per incident_type: cache miss
   - Remaining 24 requests: cache hits
```

**Validation**:
- ✅ All 50 requests: HTTP 200 OK
- ✅ No race conditions or deadlocks
- ✅ Cache hit rate: 48/50 = 96% (2 misses, 48 hits)
- ✅ Response times consistent (no degradation)

---

## 🏗️ **Infrastructure Setup**

### **Test Infrastructure Components**

```go
// test/e2e/contextapi/suite_test.go

var (
    postgresContainer   *exec.Cmd  // PostgreSQL 16+ with pgvector
    redisContainer      *exec.Cmd  // Redis 7+
    dataStorageContainer *exec.Cmd // Data Storage Service
    contextAPIContainer  *exec.Cmd // Context API Service
    
    postgresPort     int = 5433    // Avoid conflicts with system PostgreSQL
    redisPort        int = 6380    // Avoid conflicts with system Redis
    dataStoragePort  int = 8085    // Data Storage HTTP port
    contextAPIPort   int = 8086    // Context API HTTP port
)
```

### **BeforeSuite Setup**

```go
var _ = BeforeSuite(func() {
    GinkgoWriter.Println("🚀 Starting E2E test infrastructure...")
    
    // 1. Start PostgreSQL
    postgresContainer = startPostgres(postgresPort)
    Eventually(isPostgresReady, 30*time.Second).Should(BeTrue())
    
    // 2. Apply migrations
    applyMigrations(postgresPort)
    
    // 3. Start Redis
    redisContainer = startRedis(redisPort)
    Eventually(isRedisReady, 10*time.Second).Should(BeTrue())
    
    // 4. Start Data Storage Service
    dataStorageContainer = startDataStorage(postgresPort, redisPort, dataStoragePort)
    Eventually(isDataStorageReady, 20*time.Second).Should(BeTrue())
    
    // 5. Start Context API Service
    contextAPIContainer = startContextAPI(redisPort, dataStoragePort, contextAPIPort)
    Eventually(isContextAPIReady, 20*time.Second).Should(BeTrue())
    
    GinkgoWriter.Println("✅ E2E infrastructure ready")
})
```

### **AfterSuite Cleanup**

```go
var _ = AfterSuite(func() {
    GinkgoWriter.Println("🧹 Cleaning up E2E infrastructure...")
    
    stopContainer(contextAPIContainer, "Context API")
    stopContainer(dataStorageContainer, "Data Storage")
    stopContainer(redisContainer, "Redis")
    stopContainer(postgresContainer, "PostgreSQL")
    
    GinkgoWriter.Println("✅ E2E cleanup complete")
})
```

---

## 📊 **Test Data Seeding**

### **Helper Function: Seed Test Data**

```go
// test/e2e/contextapi/helpers.go

func seedTestData(dataStorageBaseURL string) error {
    // Seed incident-type: pod-oom (3 executions: 2 success, 1 failure)
    incidents := []map[string]interface{}{
        {
            "signal_name":       "pod-oom",
            "signal_fingerprint": "pod-oom-123",
            "namespace":         "default",
            "action_type":       "restart-pod",
            "action_status":     "success",
            "incident_type":     "pod-oom",
            "playbook_id":       "playbook-restart-v1",
            "playbook_version":  "1.0.0",
            "ai_execution_mode": "catalog",
        },
        {
            "signal_name":       "pod-oom",
            "signal_fingerprint": "pod-oom-456",
            "namespace":         "default",
            "action_type":       "restart-pod",
            "action_status":     "success",
            "incident_type":     "pod-oom",
            "playbook_id":       "playbook-restart-v1",
            "playbook_version":  "1.0.0",
            "ai_execution_mode": "catalog",
        },
        {
            "signal_name":       "pod-oom",
            "signal_fingerprint": "pod-oom-789",
            "namespace":         "default",
            "action_type":       "restart-pod",
            "action_status":     "failure",
            "incident_type":     "pod-oom",
            "playbook_id":       "playbook-restart-v1",
            "playbook_version":  "1.0.0",
            "ai_execution_mode": "catalog",
        },
    }
    
    for _, incident := range incidents {
        body, _ := json.Marshal(incident)
        resp, err := http.Post(
            dataStorageBaseURL+"/api/v1/notification-audit",
            "application/json",
            bytes.NewReader(body),
        )
        if err != nil || resp.StatusCode != 201 {
            return fmt.Errorf("failed to seed data: %v", err)
        }
        resp.Body.Close()
    }
    
    return nil
}
```

---

## 🧪 **Test Implementation Pattern**

### **Example: Test 1 - End-to-End Aggregation Flow**

```go
// test/e2e/contextapi/01_aggregation_flow_test.go

var _ = Describe("E2E: Aggregation Flow", func() {
    var (
        contextAPIBaseURL   string
        dataStorageBaseURL  string
    )
    
    BeforeEach(func() {
        contextAPIBaseURL = fmt.Sprintf("http://localhost:%d", contextAPIPort)
        dataStorageBaseURL = fmt.Sprintf("http://localhost:%d", dataStoragePort)
        
        // Seed test data
        err := seedTestData(dataStorageBaseURL)
        Expect(err).ToNot(HaveOccurred(), "Test data seeding should succeed")
    })
    
    It("should complete end-to-end aggregation flow", func() {
        // BEHAVIOR: AI client queries Context API for incident-type success rate
        // CORRECTNESS: Returns accurate aggregation from Data Storage → PostgreSQL
        
        url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom", contextAPIBaseURL)
        resp, err := http.Get(url)
        Expect(err).ToNot(HaveOccurred())
        defer resp.Body.Close()
        
        // BEHAVIOR: Returns HTTP 200 OK
        Expect(resp.StatusCode).To(Equal(http.StatusOK), "E2E flow should succeed")
        
        // CORRECTNESS: Response contains accurate aggregation
        var result map[string]interface{}
        err = json.NewDecoder(resp.Body).Decode(&result)
        Expect(err).ToNot(HaveOccurred())
        
        // Validate specific business values (not null testing)
        Expect(result["incident_type"]).To(Equal("pod-oom"), "Incident type should match query")
        Expect(result["total_executions"]).To(BeNumerically("==", 3), "Should aggregate 3 seeded incidents")
        Expect(result["successful_executions"]).To(BeNumerically("==", 2), "Should count 2 successful executions")
        Expect(result["success_rate"]).To(BeNumerically("~", 66.67, 0.1), "Success rate should be 66.67% (2/3)")
        Expect(result["confidence"]).To(Equal("medium"), "Confidence should be medium for 3 samples")
    })
})
```

---

## ⏱️ **Estimated Timeline**

| Phase | Duration | Tasks |
|-------|----------|-------|
| **Infrastructure Setup** | 1.5h | Reuse existing Podman helpers from integration tests |
| **Test 1: E2E Flow** | 1h | TDD RED → GREEN → REFACTOR |
| **Test 2: Cache** | 1h | TDD RED → GREEN → REFACTOR |
| **Test 3: Failure** | 1h | TDD RED → GREEN → REFACTOR |
| **Test 4: Multi-Dim** | 1h | TDD RED → GREEN → REFACTOR |
| **Test 5: Concurrent** | 0.5h | TDD RED → GREEN → REFACTOR |
| **Total** | **6 hours** | 5 E2E tests + infrastructure |

---

## ✅ **Success Criteria**

**Infrastructure**:
- ✅ All 4 services start successfully in Podman
- ✅ Health checks pass for all services
- ✅ Test data seeding works

**Tests**:
- ✅ All 5 E2E tests pass (100%)
- ✅ No flaky tests (>99% pass rate over 10 runs)
- ✅ Tests follow Behavior + Correctness pattern
- ✅ Tests use specific value assertions (no null testing)

**Performance**:
- ✅ E2E test suite completes in <5 minutes
- ✅ Infrastructure startup in <2 minutes
- ✅ Cache hit rate > 80% in concurrent test

---

## 🎯 **Confidence Assessment**

**Confidence**: **95%** (up from 80% in original plan)

**Rationale**:
- ✅ **No AI/LLM Service dependency** (simplified to 4 components)
- ✅ **Reuse existing infrastructure helpers** (from integration tests)
- ✅ **All components already tested** (unit + integration tests passing)
- ✅ **Clear test scenarios** (no ambiguous AI behavior to simulate)

**Remaining 5% Risk**:
- Podman container orchestration complexity (4 services)
- Timing issues with service startup order
- Port conflicts if services already running

---

## 📚 **Comparison: Original vs Simplified Plan**

| Aspect | Original Plan | Simplified Plan |
|--------|--------------|-----------------|
| **Infrastructure** | 5 services (+ AI/LLM) | 4 services (no AI/LLM) |
| **Test Complexity** | Simulate AI decision-making | HTTP client requests only |
| **Duration** | 4 hours | 6 hours (more thorough) |
| **Confidence** | 80% | 95% |
| **Dependencies** | AI/LLM Service required | Self-contained |
| **Reusability** | Limited | High (standard HTTP tests) |

---

## 🚀 **Next Steps**

1. **Approve this simplified plan** (user confirmation)
2. **Implement infrastructure setup** (reuse integration test helpers)
3. **Implement Test 1** (TDD RED → GREEN → REFACTOR)
4. **Implement Tests 2-5** (TDD RED → GREEN → REFACTOR)
5. **Run full E2E suite** (verify 100% pass rate)
6. **Proceed to Day 12 Documentation** (4 hours)

---

**Ready to proceed with this simplified E2E plan?**

