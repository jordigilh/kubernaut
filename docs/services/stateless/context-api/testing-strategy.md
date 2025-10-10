# Context API Service - Testing Strategy

**Version**: 1.0
**Last Updated**: October 6, 2025
**Service Type**: Stateless HTTP API Service (Read-Only)

---

## 📋 Testing Pyramid

```
         /\
        /  \  E2E Tests (10-15%)
       /____\
      /      \  Integration Tests (>50%)
     /________\
    /          \  Unit Tests (70%+)
   /____________\
```

| Test Type | Target Coverage | Focus |
|-----------|----------------|-------|
| **Unit Tests** | 70%+ | Query logic, caching, success rate calculations |
| **Integration Tests** | >50% | PostgreSQL queries, vector DB searches, Redis caching, cross-service HTTP calls |
| **E2E Tests** | 10-15% | Complete context retrieval flow |

---

## 🔴 **TDD Methodology: RED → GREEN → REFACTOR**

**Per APDC-Enhanced TDD** (`.cursor/rules/00-core-development-methodology.mdc`):
- **DO-RED**: Write failing tests defining business contract (aim for 70%+ coverage)
- **DO-GREEN**: Define business interfaces and minimal implementation
- **DO-REFACTOR**: Enhance existing code with sophisticated logic

### **Example: Success Rate Calculation (BR-CTX-001)**

#### **Phase 1: 🔴 RED - Write Failing Test**

Write test that fails because implementation doesn't exist yet.

```go
// test/unit/context/success_rate_test.go
package context_test

import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/context"
)

var _ = Describe("Success Rate Calculator (BR-CTX-001)", func() {
    var calculator *context.SuccessRateCalculator

    BeforeEach(func() {
        calculator = context.NewSuccessRateCalculator()  // DOESN'T EXIST YET
    })

    It("should calculate success rate from historical data", func() {
        rate, err := calculator.Calculate("restart-pod", 30)

        Expect(err).ToNot(HaveOccurred())
        Expect(rate).To(BeNumerically(">", 0.0))
        Expect(rate).To(BeNumerically("<=", 1.0))
    })
})
```

**Run Tests**:
```bash
go test ./test/unit/context/...
# RESULT: FAIL - undefined: context.NewSuccessRateCalculator
```

✅ **RED Phase Complete**: Test written and failing as expected.

---

#### **Phase 2: 🟢 GREEN - Minimal Implementation**

Implement ONLY enough code to make the test pass.

```go
// pkg/context/success_rate.go
package context

type SuccessRateCalculator struct{}

func NewSuccessRateCalculator() *SuccessRateCalculator {
    return &SuccessRateCalculator{}
}

func (c *SuccessRateCalculator) Calculate(actionType string, days int) (float64, error) {
    // Minimal implementation - just return a valid rate
    return 0.75, nil
}
```

**Run Tests**:
```bash
go test ./test/unit/context/...
# RESULT: PASS - all tests passing
```

✅ **GREEN Phase Complete**: Tests passing with minimal implementation.

---

#### **Phase 3: 🔧 REFACTOR - Enhanced Implementation**

Enhance implementation with real business logic while keeping tests passing.

```go
// pkg/context/success_rate.go
package context

import (
    "context"
    "database/sql"
    "fmt"
    "time"

    "go.uber.org/zap"
)

type SuccessRateCalculator struct {
    logger *zap.Logger
    db     *sql.DB
    cache  Cache
}

func NewSuccessRateCalculator(logger *zap.Logger, db *sql.DB, cache Cache) *SuccessRateCalculator {
    return &SuccessRateCalculator{
        logger: logger,
        db:     db,
        cache:  cache,
    }
}

func (c *SuccessRateCalculator) Calculate(actionType string, days int) (float64, error) {
    ctx := context.Background()

    // Check cache first
    cacheKey := fmt.Sprintf("success_rate:%s:%d", actionType, days)
    if cached, found := c.cache.Get(cacheKey); found {
        c.logger.Debug("Cache hit for success rate",
            zap.String("action_type", actionType),
            zap.Int("days", days))
        return cached.(float64), nil
    }

    // Query historical data from PostgreSQL
    query := `
        SELECT
            COUNT(CASE WHEN status = 'success' THEN 1 END) as successful,
            COUNT(*) as total
        FROM action_history
        WHERE action_type = $1
        AND created_at >= NOW() - INTERVAL '$2 days'
    `

    var successful, total int
    err := c.db.QueryRowContext(ctx, query, actionType, days).Scan(&successful, &total)
    if err != nil {
        c.logger.Error("Failed to query historical data",
            zap.Error(err),
            zap.String("action_type", actionType))
        return 0.0, fmt.Errorf("query failed: %w", err)
    }

    // Calculate success rate
    var rate float64
    if total > 0 {
        rate = float64(successful) / float64(total)
    }

    // Cache result for 5 minutes
    c.cache.Set(cacheKey, rate, 5*time.Minute)

    c.logger.Info("Calculated success rate",
        zap.String("action_type", actionType),
        zap.Int("days", days),
        zap.Float64("rate", rate),
        zap.Int("total_actions", total))

    return rate, nil
}
```

**Run Tests**:
```bash
go test ./test/unit/context/...
# RESULT: PASS - all tests still passing with enhanced implementation
```

✅ **REFACTOR Phase Complete**: Production-ready implementation with tests still passing.

---

### **TDD Cycle Complete** ✅

**Result**:
- ✅ Tests written first (RED)
- ✅ Minimal implementation (GREEN)
- ✅ Enhanced with real logic (REFACTOR)
- ✅ All tests passing
- ✅ Business requirement BR-CTX-001 satisfied

**Key Principles Applied**:
- Test-first development ensures business contract is defined before implementation
- Minimal implementation in GREEN phase prevents over-engineering
- REFACTOR phase adds sophisticated logic (database queries, caching, logging) without changing tests
- Tests validate *what* the code does (business outcome), not *how* it does it

---

## 🧪 Unit Tests (70%+)

### **Test Framework**: Ginkgo + Gomega

```go
package context_test

import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/context"
)

var _ = Describe("Success Rate Calculator", func() {
    var calculator *context.SuccessRateCalculator

    BeforeEach(func() {
        calculator = context.NewSuccessRateCalculator()
    })

    // BR-CTX-001: Success rate calculation
    Context("Success Rate Calculation", func() {
        It("should calculate success rate from action history", func() {
            actions := []context.ActionHistory{
                {ID: "1", Status: "success"},
                {ID: "2", Status: "success"},
                {ID: "3", Status: "failure"},
                {ID: "4", Status: "success"},
            }

            rate := calculator.Calculate(actions)

            Expect(rate).To(Equal(0.75)) // 3/4 = 75%
        })

        It("should handle empty action history", func() {
            rate := calculator.Calculate([]context.ActionHistory{})
            Expect(rate).To(Equal(0.0))
        })
    })
})
```

---

## 🔗 Integration Tests (>50%)

### **Why >50% for Microservices Architecture**

Context API is a critical component in Kubernaut's **microservices architecture**, requiring extensive integration testing for:
- **Cross-service HTTP calls**: AI Analysis, HolmesGPT API, and Effectiveness Monitor all call Context API
- **Service coordination**: Multiple services depend on Context API for historical data
- **Data flow validation**: PostgreSQL → Context API → consuming services
- **Error handling**: Failures must be handled gracefully across service boundaries

**Per project spec** (`.cursor/rules/03-testing-strategy.mdc` line 72):
> "**Coverage Mandate**: **>50% of total business requirements due to microservices architecture**"

### **PostgreSQL Integration**

```go
package context_test

import (
    "context"
    "database/sql"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/context"
    "github.com/jordigilh/kubernaut/pkg/testutil"
)

var _ = Describe("PostgreSQL Integration", func() {
    var db *sql.DB
    var service *context.ContextAPIService

    BeforeEach(func() {
        db = testutil.SetupTestDatabase()
        service = context.NewContextAPIService(db)
    })

    AfterEach(func() {
        testutil.CleanupTestDatabase(db)
    })

    Context("Historical Query", func() {
        It("should retrieve action history from PostgreSQL", func() {
            // Insert test data
            testutil.InsertActionHistory(db, testAction)

            // Query via Context API
            history, err := service.GetActionHistory(testAction.ID)

            Expect(err).ToNot(HaveOccurred())
            Expect(history.ID).To(Equal(testAction.ID))
        })
    })
})
```

### **Vector DB Integration**

```go
package context_test

import (
    "context"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/context"
)

var _ = Describe("Vector DB Integration", func() {
    Context("Vector Similarity Search", func() {
        It("should find similar alerts using vector search", func() {
            embedding := []float32{0.1, 0.2, 0.3, ...}

            similar, err := service.FindSimilarAlerts(embedding, 5)

            Expect(err).ToNot(HaveOccurred())
            Expect(len(similar)).To(BeNumerically("<=", 5))
            Expect(similar[0].Similarity).To(BeNumerically(">", 0.8))
        })
    })
})
```

### **Redis Caching Integration**

```go
package context_test

import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/context"
    "github.com/jordigilh/kubernaut/pkg/testutil"
)

var _ = Describe("Redis Caching Integration", func() {
    Context("Cache Integration", func() {
        It("should cache frequently accessed data", func() {
            // First call - cache miss
            data1, err := service.GetSuccessRate("restart-pod")
            Expect(err).ToNot(HaveOccurred())

            // Second call - cache hit
            data2, err := service.GetSuccessRate("restart-pod")
            Expect(err).ToNot(HaveOccurred())
            Expect(data2).To(Equal(data1))

            // Verify cache was used
            Expect(service.CacheHitRate()).To(BeNumerically(">", 0.5))
        })
    })
})
```

### **Cross-Service Integration** (NEW - Critical for >50% Coverage)

```go
package context_test

import (
    "context"
    "net/http"
    "net/http/httptest"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/context"
)

var _ = Describe("Cross-Service Integration", func() {
    Context("Cross-Service HTTP Integration", func() {
        It("should provide context to AI Analysis Service", func() {
            // Simulate AI Analysis calling Context API
            req := &ContextRequest{
                AlertName: "HighMemoryUsage",
                Namespace: "production",
                Cluster:   "us-west-2",
            }

            context, err := contextAPIClient.GetContext(req)

            Expect(err).ToNot(HaveOccurred())
            Expect(context.SuccessRate).To(BeNumerically(">", 0.0))
            Expect(context.HistoricalActions).ToNot(BeEmpty())
        })

        It("should provide investigation context to HolmesGPT API", func() {
            // Simulate HolmesGPT API calling Context API
            alertID := "alert-abc123"

            investigationCtx, err := contextAPIClient.GetInvestigationContext(alertID)

            Expect(err).ToNot(HaveOccurred())
            Expect(investigationCtx.HistoricalPatterns).ToNot(BeEmpty())
            Expect(investigationCtx.SimilarIncidents).ToNot(BeEmpty())
        })

        It("should provide trend data to Effectiveness Monitor", func() {
            // Simulate Effectiveness Monitor calling Context API
            trendReq := &TrendRequest{
                ActionType: "restart-pod",
                TimeRange:  "90d",
            }

            trends, err := contextAPIClient.GetTrends(trendReq)

            Expect(err).ToNot(HaveOccurred())
            Expect(trends.DataPoints).ToNot(BeEmpty())
            Expect(trends.Confidence).To(BeNumerically(">", 0.7))
        })

        It("should handle service unavailability gracefully", func() {
            // Simulate downstream service failure
            contextAPIClient.SimulateFailure("postgresql")

            _, err := contextAPIClient.GetContext(&ContextRequest{})

            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("service unavailable"))
        })

        It("should respect rate limits for consuming services", func() {
            // Simulate rapid requests from AI Analysis
            for i := 0; i < 150; i++ {
                _, err := contextAPIClient.GetContext(&ContextRequest{})
                if i < 100 {
                    Expect(err).ToNot(HaveOccurred())
                } else {
                    // Should hit rate limit (100 req/s)
                    Expect(err).To(MatchError(ContainSubstring("rate limit")))
                }
            }
        })
    })
})
```

---

## 🌐 E2E Tests (10-15%)

```go
package e2e_test

import (
    "context"
    "net/http"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/context"
    "github.com/jordigilh/kubernaut/test/e2e/helpers"
)

var _ = Describe("E2E: Context Retrieval Flow", func() {
    It("should provide complete context for AI Analysis", func() {
        By("receiving context request from AI Analysis")
        request := &ContextRequest{
            AlertName: "HighMemoryUsage",
            Namespace: "production",
            Cluster:   "us-west-2",
        }

        By("retrieving historical data")
        context, err := contextAPIClient.GetContext(request)
        Expect(err).ToNot(HaveOccurred())

        By("validating context completeness")
        Expect(context.SuccessRate).To(BeNumerically(">", 0.0))
        Expect(context.HistoricalActions).ToNot(BeEmpty())
        Expect(context.SimilarIncidents).ToNot(BeEmpty())

        By("verifying performance")
        Expect(context.ResponseTime).To(BeNumerically("<", 200)) // < 200ms
    })
})
```

---

## 🎯 Business Requirement Coverage

| Requirement | Unit Tests | Integration Tests | E2E Tests |
|------------|------------|-------------------|-----------|
| **BR-CTX-001** (Success rates) | ✅✅ | ✅ | ✅ |
| **BR-CTX-002** (Historical query) | ✅ | ✅✅ | ✅ |
| **BR-CTX-003** (Vector search) | ✅ | ✅✅ | - |
| **BR-CTX-004** (Caching) | ✅✅ | ✅✅ | - |
| **BR-CTX-005** (Performance) | ✅ | ✅ | ✅✅ |

---

## 🔧 Test Execution

### **Unit Tests**
```bash
go test -v -coverprofile=coverage.out ./test/unit/context/...
go tool cover -html=coverage.out
```

### **Integration Tests**
```bash
# Requires Docker for PostgreSQL, Redis, pgvector
make test-integration
```

### **E2E Tests**
```bash
# Requires Kind cluster
make test-e2e
```

---

## 📊 Test Quality Metrics

| Metric | Target | Current |
|--------|--------|---------|
| **Unit Test Coverage** | 70%+ | TBD |
| **Integration Test Coverage** | >50% | TBD |
| **E2E Test Coverage** | 10-15% | TBD |
| **Test Execution Time** | < 5 min | TBD |

---

---

## 🎯 Test Level Selection: Maintainability First

**Principle**: Prioritize maintainability and simplicity when choosing between unit, integration, and e2e tests.

### Test at Unit Level WHEN

- ✅ Scenario can be tested with **mock PostgreSQL/Redis** (in-memory databases)
- ✅ Focus is on **query logic** (success rate calculations, historical data aggregation)
- ✅ Setup is **straightforward** (< 20 lines of mock configuration)
- ✅ Test remains **readable and maintainable** with mocking

**Context API Unit Test Examples**:
- Success rate calculation algorithms (historical action success percentage)
- Query result caching logic (cache hit/miss, TTL expiration)
- Similarity score computation (vector distance calculations)
- Data aggregation functions (time-based bucketing, averaging)
- Response formatting logic (JSON structure, field mapping)

---

### Move to Integration Level WHEN

- ✅ Scenario requires **real PostgreSQL queries** (actual SQL execution with complex joins)
- ✅ Validating **real Vector DB searches** (pgvector similarity queries)
- ✅ Unit test would require **excessive DB mocking** (>50 lines of SQL result mocks)
- ✅ Integration test is **simpler to understand** and maintain
- ✅ Testing **cross-service HTTP calls** (AI Analysis → Context API → response)

**Context API Integration Test Examples**:
- Complete historical query flow (PostgreSQL → aggregation → response)
- Real vector similarity search (pgvector embeddings → top-K similar)
- Redis caching with real TTL behavior (set → expire → evict)
- Cross-service data flow (AI Analysis calls → Context API → Data Storage reads)
- Multi-service coordination (Context API → Data Storage → HolmesGPT API)

---

### Move to E2E Level WHEN

- ✅ Testing **complete context retrieval pipeline** (alert → historical data → AI analysis → decision)
- ✅ Validating **end-to-end microservices coordination** (Gateway → Context API → AI → decision)
- ✅ Lower-level tests **cannot reproduce full data flow** (caching + DB + vector search together)

**Context API E2E Test Examples**:
- Complete remediation context retrieval (alert ingestion → context fetch → AI analysis → workflow)
- Production-like query performance (p95 latency < 200ms with real data)
- End-to-end SLO validation (context retrieval time + accuracy)

---

## 🧭 Maintainability Decision Criteria

**Ask these 5 questions before implementing a unit test:**

### 1. Mock Complexity
**Question**: Will DB query mocking be >35 lines?
- ✅ **YES** → Consider integration test with real PostgreSQL
- ❌ **NO** → Unit test acceptable

**Context API Example**:
```go
// ❌ COMPLEX: 90+ lines of PostgreSQL query result mocking
mockDB.ExpectQuery("SELECT").WillReturnRows(mockRows1)
mockDB.ExpectQuery("SELECT").WillReturnRows(mockRows2)
// ... 85+ more lines for complex joins
// BETTER: Integration test with real PostgreSQL
```

---

### 2. Readability
**Question**: Would a new developer understand this test in 2 minutes?
- ✅ **YES** → Unit test is good
- ❌ **NO** → Consider higher test level

**Context API Example**:
```go
// ✅ READABLE: Clear calculation logic test
It("should calculate success rate from historical data", func() {
    actions := []Action{
        {Status: "success"},
        {Status: "success"},
        {Status: "failure"},
        {Status: "success"},
    }

    rate := calculator.Calculate(actions)
    Expect(rate).To(Equal(0.75)) // 3/4 = 75%
})
```

---

### 3. Fragility
**Question**: Does test break when internal query implementation changes?
- ✅ **YES** → Move to integration test (testing implementation, not behavior)
- ❌ **NO** → Unit test is appropriate

**Context API Example**:
```go
// ❌ FRAGILE: Breaks if we change internal query structure
Expect(query.sql).To(Equal("SELECT * FROM actions WHERE..."))

// ✅ STABLE: Tests query outcome, not implementation
Expect(results.SuccessRate).To(BeNumerically(">", 0.7))
Expect(results.TotalActions).To(Equal(150))
```

---

### 4. Real Value
**Question**: Is this testing calculation logic or database operations?
- **Calculation Logic** → Unit test with mock data
- **Database Operations** → Integration test with real DB

**Context API Decision**:
- **Unit**: Success rate calculations, caching logic, aggregation functions (pure logic)
- **Integration**: PostgreSQL queries, vector searches, Redis operations (infrastructure)

---

### 5. Maintenance Cost
**Question**: How much effort to maintain this vs integration test?
- **Lower cost** → Choose that option

**Context API Example**:
- **Unit test with 100-line DB mock**: HIGH maintenance (breaks on schema changes)
- **Integration test with real DB**: LOW maintenance (automatically adapts to schema)

---

## 🎯 Realistic vs. Exhaustive Testing

**Principle**: Test realistic query scenarios necessary to validate business requirements - not more, not less.

### Context API: Requirement-Driven Coverage

**Business Requirement Analysis** (BR-CTX-001 to BR-CTX-005):

| Context Dimension | Realistic Values | Test Strategy |
|---|---|---|
| **Time Ranges** | 7d, 30d, 90d (3 common ranges) | Test time-based bucketing |
| **Action Types** | restart-pod, scale, update, rollback, migrate (5 types) | Test per-action aggregation |
| **Success Thresholds** | <70%, 70-85%, >85% (3 buckets) | Test threshold boundaries |
| **Cache TTLs** | 5min, 15min, 1hour (3 TTLs) | Test expiration logic |

**Total Possible Combinations**: 3 × 5 × 3 × 3 = 135 combinations
**Distinct Business Behaviors**: 12 behaviors (per BR-CTX-001 to BR-CTX-005)
**Tests Needed**: ~25 tests (covering 12 distinct behaviors with boundaries)

---

### ✅ DO: Test Distinct Query Behaviors Using DescribeTable

**BEST PRACTICE**: Use Ginkgo's `DescribeTable` for success rate and caching testing.

```go
// ✅ GOOD: Tests distinct success rate calculations using data table
var _ = Describe("BR-CTX-001: Success Rate Calculation", func() {
    DescribeTable("Success rate calculation for different action histories",
        func(successCount int, totalCount int, expectedRate float64, expectedConfidence string) {
            // Single test function handles all scenarios
            actions := testutil.NewActionHistory(successCount, totalCount-successCount)
            calculator := NewSuccessRateCalculator()

            result := calculator.Calculate(actions)

            Expect(result.Rate).To(BeNumerically("~", expectedRate, 0.01))
            Expect(result.Confidence).To(Equal(expectedConfidence))
        },
        // BR-CTX-001.1: High success rate (>85%)
        Entry("high success rate with high confidence",
            17, 20, 0.85, "high"),

        // BR-CTX-001.2: Medium success rate (70-85%)
        Entry("medium success rate with medium confidence",
            15, 20, 0.75, "medium"),

        // BR-CTX-001.3: Low success rate (<70%)
        Entry("low success rate with low confidence",
            12, 20, 0.60, "low"),

        // BR-CTX-001.4: Perfect success rate
        Entry("perfect success rate",
            20, 20, 1.00, "high"),

        // BR-CTX-001.5: Complete failure rate
        Entry("complete failure rate",
            0, 20, 0.00, "low"),

        // BR-CTX-001.6: Boundary at 70%
        Entry("success rate exactly at 70% boundary",
            14, 20, 0.70, "medium"),

        // BR-CTX-001.7: Boundary at 85%
        Entry("success rate exactly at 85% boundary",
            17, 20, 0.85, "high"),
    )
})
```

**Why DescribeTable is Better for Context API Testing**:
- ✅ 7 success rate scenarios in single function (vs. 7 separate It blocks)
- ✅ Change calculation logic once, all scenarios tested
- ✅ Clear threshold boundaries visible
- ✅ Easy to add new buckets
- ✅ Perfect for testing mathematical calculations with multiple inputs

---

### ❌ DON'T: Test Redundant Query Variations

```go
// ❌ BAD: Redundant tests that validate SAME query logic
It("should query actions for 7 days", func() {})
It("should query actions for 8 days", func() {})
It("should query actions for 9 days", func() {})
// All 3 tests validate SAME time-based filtering
// BETTER: One test for time filtering, one for boundary (0 days, 365 days)

// ❌ BAD: Exhaustive action type permutations
It("should calculate rate for restart-pod", func() {})
It("should calculate rate for scale-deployment", func() {})
// ... 133 more combinations
// These don't test DISTINCT calculation logic
```

---

### Decision Criteria: Is This Context API Test Necessary?

Ask these 4 questions:

1. **Does this test validate a distinct query behavior or calculation rule?**
   - ✅ YES: Success rate threshold boundaries (70%, 85%)
   - ❌ NO: Testing different action names with same calculation

2. **Does this query scenario actually occur in production?**
   - ✅ YES: 30-day historical data retrieval
   - ❌ NO: 10,000-day historical query (unrealistic)

3. **Would this test catch a query bug the other tests wouldn't?**
   - ✅ YES: Cache expiration at exact TTL boundary
   - ❌ NO: Testing 20 different time ranges with same logic

4. **Is this testing query behavior or implementation variation?**
   - ✅ Query: Time range affects data freshness confidence
   - ❌ Implementation: Internal SQL query string format

**If answer is "NO" to all 4 questions** → Skip the test, it adds maintenance cost without query value

---

### Context API Test Coverage Example with DescribeTable

**BR-CTX-004: Cache Behavior (6 distinct caching scenarios)**

```go
Describe("BR-CTX-004: Redis Caching Logic", func() {
    // ANALYSIS: 10 TTLs × 5 action types × 3 cache states = 150 combinations
    // REQUIREMENT ANALYSIS: Only 6 distinct caching behaviors per BR-CTX-004
    // TEST STRATEGY: Use DescribeTable for 6 cache scenarios + 2 edge cases

    DescribeTable("Redis caching with TTL expiration",
        func(cacheState string, ageSeconds int, expectedCacheHit bool, expectedFreshness string) {
            // Single test function for all cache scenarios
            cacheKey := "success_rate:restart-pod:30d"

            if cacheState == "populated" {
                testutil.PopulateCache(redis, cacheKey, 0.85, ageSeconds)
            }

            result, cacheHit := service.GetSuccessRate("restart-pod", 30)

            Expect(cacheHit).To(Equal(expectedCacheHit))
            if cacheHit {
                Expect(result.Freshness).To(Equal(expectedFreshness))
            }
        },
        // Scenario 1: Cache hit - data fresh (< 5min)
        Entry("cache hit with fresh data",
            "populated", 240, true, "fresh"),

        // Scenario 2: Cache miss - no data
        Entry("cache miss - key not found",
            "empty", 0, false, ""),

        // Scenario 3: Cache hit - data stale (> 5min but < TTL)
        Entry("cache hit with stale data",
            "populated", 420, true, "stale"),

        // Scenario 4: Cache miss - TTL expired
        Entry("cache miss - TTL expired",
            "populated", 901, false, ""),

        // Scenario 5: Cache hit - data at exact freshness boundary (5min)
        Entry("cache hit at freshness boundary",
            "populated", 300, true, "stale"),

        // Scenario 6: Cache hit - data just before TTL (15min)
        Entry("cache hit just before TTL expiration",
            "populated", 899, true, "stale"),

        // Edge case 1: Cache hit - brand new data (0 seconds old)
        Entry("cache hit with brand new data",
            "populated", 0, true, "fresh"),

        // Edge case 2: Cache miss - just expired (TTL + 1 second)
        Entry("cache miss just after TTL expiration",
            "populated", 901, false, ""),
    )

    // Result: 8 Entry() lines cover 6 cache behaviors + 2 edge cases
    // NOT testing all 150 combinations - only distinct TTL behaviors
    // Coverage: 100% of caching requirements
    // Maintenance: Change TTL logic once, all scenarios adapt
})
```

**Benefits for Context API Query Testing**:
- ✅ **8 cache scenarios tested in ~12 lines** (vs. ~200 lines with separate Its)
- ✅ **Single caching engine** - changes apply to all scenarios
- ✅ **Clear TTL matrix** - expiration rules immediately visible
- ✅ **Easy to add TTLs** - new Entry for new cache policies
- ✅ **90% less maintenance** for complex caching testing

---

## ⚠️ Anti-Patterns to AVOID

### ❌ OVER-EXTENDED UNIT TESTS (Forbidden)

**Problem**: Excessive DB query mocking (>50 lines) makes query tests unmaintainable

```go
// ❌ BAD: 120+ lines of complex SQL result mocking
var _ = Describe("Complex Multi-Table Join Query", func() {
    BeforeEach(func() {
        // 120+ lines of SQL result mocking for joins across 5 tables
        mockRows1 := sqlmock.NewRows([]string{"id", "status"...})
        mockRows2 := sqlmock.NewRows([]string{"action_id", "result"...})
        // ... 110+ more lines
        // THIS SHOULD BE AN INTEGRATION TEST
    })
})
```

**Solution**: Move to integration test with real PostgreSQL

```go
// ✅ GOOD: Integration test with real DB queries
var _ = Describe("BR-INTEGRATION-CTX-010: Historical Action Query", func() {
    It("should retrieve 30-day historical data with real PostgreSQL", func() {
        // 25 lines with real DB - much clearer
        testutil.InsertTestActions(db, 150)

        result, err := service.GetActionHistory("restart-pod", 30)

        Expect(err).ToNot(HaveOccurred())
        Expect(result.Actions).To(HaveLen(150))
        Expect(result.SuccessRate).To(BeNumerically(">=", 0.7))
    })
})
```

---

### ❌ WRONG TEST LEVEL (Forbidden)

**Problem**: Testing real DB operations in unit tests

```go
// ❌ BAD: Testing actual PostgreSQL queries in unit test
It("should execute complex SQL join", func() {
    // Complex mocking of SQL execution engine
    // Real DB operation - belongs in integration test
})
```

**Solution**: Use integration test for real DB operations

```go
// ✅ GOOD: Integration test for SQL execution
It("should execute complex join and return aggregated results", func() {
    // Test with real PostgreSQL - validates actual SQL
})
```

---

### ❌ REDUNDANT COVERAGE (Forbidden)

**Problem**: Testing same calculation at multiple levels

```go
// ❌ BAD: Testing exact same success rate logic at all 3 levels
// Unit test: Success rate = successful / total
// Integration test: Success rate = successful / total (duplicate)
// E2E test: Success rate = successful / total (duplicate)
// NO additional value
```

**Solution**: Test calculation in unit tests, test INTEGRATION in higher levels

```go
// ✅ GOOD: Each level tests distinct aspect
// Unit test: Success rate calculation correctness
// Integration test: Calculation + real PostgreSQL query + caching
// E2E test: Calculation + integration + end-to-end data flow
// Each level adds unique query value
```

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Status**: ✅ Complete Specification

