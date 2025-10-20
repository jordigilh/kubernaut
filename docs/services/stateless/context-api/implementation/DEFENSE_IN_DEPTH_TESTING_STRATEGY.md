# Context API - Defense-in-Depth Testing Strategy

**Version**: 1.0
**Date**: 2025-10-15
**Status**: ✅ **COMPREHENSIVE EDGE CASE COVERAGE MANDATE**
**References**: [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc)

---

## 🎯 **Purpose**

This document provides **comprehensive edge case and boundary value testing strategy** for the Context API, following the project's mandatory **defense-in-depth testing approach** documented in `03-testing-strategy.mdc`.

**CRITICAL**: This addresses **Gap #7** from the quality triage - the original implementation plan lacked systematic edge case coverage.

---

## 📊 **Defense-in-Depth Pyramid for Context API**

```
           E2E (10-15% - Critical Workflows)
          /                                \
         /  Complete alert→analysis→result  \
        /____________________________________\
       
      Integration (>50% - Service Interactions)
     /                                        \
    /  CRD coordination, DB+Redis+Vector DB    \
   /__________________________________________\
  
 Unit (70%+ - MAXIMUM Business Logic Coverage)
/_______________________________________________\
  All business logic with external mocks only
  + Boundary testing + State matrices + Edge cases
```

---

## 🛡️ **Comprehensive Test Coverage Requirements**

### 1. Boundary Value Testing (MANDATORY)

#### Query Pagination Boundaries

```go
// test/unit/contextapi/query_builder_test.go (ADD THIS SECTION)

var _ = Describe("BR-CONTEXT-001: Query Boundary Testing", func() {
	DescribeTable("pagination boundary values",
		func(limit, offset int, expectErr bool, expectedErrType error) {
			params := &models.QueryParams{
				Limit:  limit,
				Offset: offset,
			}
			
			query, args, err := builder.BuildQuery(params)
			
			if expectErr {
				Expect(err).To(HaveOccurred())
				if expectedErrType != nil {
					Expect(err).To(MatchError(expectedErrType))
				}
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(query).ToNot(BeEmpty())
				Expect(args).ToNot(BeNil())
			}
		},
		// Boundary cases
		Entry("minimum limit (1)", 1, 0, false, nil),
		Entry("typical limit (10)", 10, 0, false, nil),
		Entry("typical limit (100)", 100, 0, false, nil),
		Entry("maximum limit (1000)", 1000, 0, false, nil),
		
		// Invalid cases
		Entry("zero limit (invalid)", 0, 0, true, models.ErrInvalidLimit),
		Entry("negative limit (invalid)", -1, 0, true, models.ErrInvalidLimit),
		Entry("exceed max limit (invalid)", 1001, 0, true, models.ErrLimitExceedsMaximum),
		
		// Offset boundaries
		Entry("zero offset (valid)", 10, 0, false, nil),
		Entry("typical offset (valid)", 10, 100, false, nil),
		Entry("large offset (valid)", 10, 999999, false, nil),
		Entry("negative offset (invalid)", 10, -1, true, models.ErrInvalidOffset),
	)
})
```

#### Time Range Boundaries

```go
var _ = Describe("BR-CONTEXT-001: Time Range Boundary Testing", func() {
	DescribeTable("time range edge cases",
		func(setupFunc func() (*time.Time, *time.Time), expectErr bool) {
			startTime, endTime := setupFunc()
			params := &models.QueryParams{
				StartTime: startTime,
				EndTime:   endTime,
			}
			
			_, _, err := builder.BuildQuery(params)
			
			if expectErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		},
		Entry("nil start and end (valid - no filter)", func() (*time.Time, *time.Time) {
			return nil, nil
		}, false),
		
		Entry("start only (valid)", func() (*time.Time, *time.Time) {
			start := time.Now().Add(-24 * time.Hour)
			return &start, nil
		}, false),
		
		Entry("end only (valid)", func() (*time.Time, *time.Time) {
			end := time.Now()
			return nil, &end
		}, false),
		
		Entry("valid range (1 day)", func() (*time.Time, *time.Time) {
			start := time.Now().Add(-24 * time.Hour)
			end := time.Now()
			return &start, &end
		}, false),
		
		Entry("valid range (30 days)", func() (*time.Time, *time.Time) {
			start := time.Now().Add(-30 * 24 * time.Hour)
			end := time.Now()
			return &start, &end
		}, false),
		
		Entry("valid range (1 year)", func() (*time.Time, *time.Time) {
			start := time.Now().Add(-365 * 24 * time.Hour)
			end := time.Now()
			return &start, &end
		}, false),
		
		Entry("invalid: end before start", func() (*time.Time, *time.Time) {
			start := time.Now()
			end := time.Now().Add(-24 * time.Hour)
			return &start, &end
		}, true),
		
		Entry("same start and end (valid)", func() (*time.Time, *time.Time) {
			t := time.Now()
			return &t, &t
		}, false),
	)
})
```

#### Vector Embedding Dimensions

```go
// test/unit/contextapi/vector_search_test.go (ADD THIS SECTION)

var _ = Describe("BR-CONTEXT-003: Vector Dimension Boundary Testing", func() {
	DescribeTable("embedding dimension validation",
		func(dimensions int, expectErr bool) {
			embedding := make([]float32, dimensions)
			for i := range embedding {
				embedding[i] = float32(i) * 0.1
			}
			
			params := &models.SemanticSearchParams{
				Embedding: embedding,
				Threshold: 0.8,
				Limit:     10,
			}
			
			err := params.Validate()
			
			if expectErr {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(models.ErrInvalidEmbeddingDimension))
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		},
		// Invalid dimensions
		Entry("zero dimensions (invalid)", 0, true),
		Entry("383 dimensions (invalid - too few)", 383, true),
		Entry("385 dimensions (invalid - too many)", 385, true),
		Entry("1536 dimensions (wrong model)", 1536, true),
		
		// Valid dimension
		Entry("384 dimensions (valid)", 384, false),
	)
})
```

---

### 2. State Matrix Testing (MANDATORY)

#### Cache State Combinations

```go
// test/integration/contextapi/cache_fallback_test.go (EXPAND THIS SECTION)

var _ = Describe("BR-CONTEXT-005: Cache State Matrix Testing", func() {
	DescribeTable("multi-tier cache fallback scenarios",
		func(redisState, l2State, dbState string, expectedSource string, expectErr bool) {
			// Setup states
			setupRedisState(redisState)    // UP, DOWN, SLOW, ERROR
			setupL2State(l2State)          // EMPTY, HIT, MISS
			setupDBState(dbState)          // UP, DOWN, SLOW, ERROR
			
			// Execute query
			result, err := cachedExecutor.QueryIncidents(ctx, testParams)
			
			// Validate outcome
			if expectErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.Source).To(Equal(expectedSource))
			}
		},
		// Optimal path
		Entry("Redis UP + L2 EMPTY + DB UP → Redis (populate L2)", 
			"UP", "EMPTY", "UP", "redis", false),
		
		// L1 failure, L2 success
		Entry("Redis DOWN + L2 HIT + DB UP → L2", 
			"DOWN", "HIT", "UP", "l2_cache", false),
		
		// L1+L2 failure, DB success
		Entry("Redis DOWN + L2 MISS + DB UP → DB", 
			"DOWN", "MISS", "UP", "database", false),
		
		// Complete failure
		Entry("Redis DOWN + L2 MISS + DB DOWN → Error", 
			"DOWN", "MISS", "DOWN", "", true),
		
		// Slow states (timeout scenarios)
		Entry("Redis SLOW + L2 HIT + DB UP → L2 (timeout fallback)", 
			"SLOW", "HIT", "UP", "l2_cache", false),
		
		Entry("Redis DOWN + L2 MISS + DB SLOW → DB (slow but succeeds)", 
			"DOWN", "MISS", "SLOW", "database", false),
		
		// Error states
		Entry("Redis ERROR + L2 EMPTY + DB UP → DB (skip Redis)", 
			"ERROR", "EMPTY", "UP", "database", false),
		
		Entry("Redis UP + L2 EMPTY + DB ERROR → Redis only", 
			"UP", "EMPTY", "ERROR", "redis", false),
	)
})
```

#### Database Connection States

```go
var _ = Describe("BR-CONTEXT-005: Database State Testing", func() {
	DescribeTable("database connection state scenarios",
		func(dbState string, expectedBehavior string) {
			setupDatabaseState(dbState)
			
			_, err := dbClient.QueryIncidents(ctx, testParams)
			
			switch expectedBehavior {
			case "success":
				Expect(err).ToNot(HaveOccurred())
			case "retry":
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("retry")))
			case "fail":
				Expect(err).To(HaveOccurred())
				Expect(err).ToNot(MatchError(ContainSubstring("retry")))
			}
		},
		Entry("connected → success", "connected", "success"),
		Entry("disconnected → fail", "disconnected", "fail"),
		Entry("slow → success with delay", "slow", "success"),
		Entry("connection timeout → fail", "timeout", "fail"),
		Entry("connection pool exhausted → retry", "pool_exhausted", "retry"),
		Entry("transient error → retry", "transient_error", "retry"),
		Entry("fatal error → fail", "fatal_error", "fail"),
	)
})
```

---

### 3. Input Validation Matrix (MANDATORY)

#### String Field Validation

```go
// test/unit/contextapi/models_test.go (ADD THIS SECTION)

var _ = Describe("BR-CONTEXT-002: Input Validation Matrix", func() {
	DescribeTable("namespace field validation",
		func(namespace *string, expectValid bool, expectedErr error) {
			params := &models.QueryParams{
				Namespace: namespace,
				Limit:     10,
			}
			
			err := params.Validate()
			
			if expectValid {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(HaveOccurred())
				if expectedErr != nil {
					Expect(err).To(MatchError(expectedErr))
				}
			}
		},
		// Valid cases
		Entry("nil namespace (valid - no filter)", nil, true, nil),
		Entry("empty string (valid - no filter)", stringPtr(""), true, nil),
		Entry("typical namespace", stringPtr("production"), true, nil),
		Entry("namespace with dash", stringPtr("prod-webapp"), true, nil),
		Entry("max length (255 chars)", stringPtr(strings.Repeat("a", 255)), true, nil),
		
		// Invalid cases
		Entry("exceed max length (256 chars)", 
			stringPtr(strings.Repeat("a", 256)), false, models.ErrNamespaceTooLong),
		
		Entry("special characters", 
			stringPtr("prod_webapp!@#"), false, models.ErrInvalidNamespace),
		
		Entry("SQL injection attempt", 
			stringPtr("'; DROP TABLE remediation_audit; --"), false, models.ErrInvalidInput),
		
		Entry("XSS attempt", 
			stringPtr("<script>alert('xss')</script>"), false, models.ErrInvalidInput),
	)
	
	DescribeTable("severity field validation",
		func(severity *string, expectValid bool) {
			params := &models.QueryParams{
				Severity: severity,
				Limit:    10,
			}
			
			err := params.Validate()
			
			if expectValid {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(HaveOccurred())
			}
		},
		// Valid cases
		Entry("nil severity (valid)", nil, true),
		Entry("critical", stringPtr("critical"), true),
		Entry("high", stringPtr("high"), true),
		Entry("medium", stringPtr("medium"), true),
		Entry("low", stringPtr("low"), true),
		
		// Case sensitivity
		Entry("Critical (uppercase)", stringPtr("Critical"), true),
		Entry("CRITICAL (all caps)", stringPtr("CRITICAL"), true),
		
		// Invalid cases
		Entry("invalid severity", stringPtr("invalid"), false),
		Entry("empty string", stringPtr(""), false),
		Entry("numeric value", stringPtr("1"), false),
	)
})
```

---

### 4. Comprehensive Error Path Coverage (MANDATORY)

#### Database Error Scenarios

```go
// test/integration/contextapi/error_scenarios_test.go (NEW FILE)

package contextapi

import (
	"context"
	"database/sql"
	"time"
	
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BR-CONTEXT-007: Comprehensive Error Path Testing", func() {
	DescribeTable("database error recovery scenarios",
		func(errorType string, expectedRecovery string, shouldRetry bool) {
			// Inject specific error type
			mockDB.SetErrorType(errorType)
			
			_, err := dbClient.QueryIncidents(ctx, testParams)
			
			Expect(err).To(HaveOccurred())
			
			// Verify error classification
			switch expectedRecovery {
			case "retry":
				Expect(shouldRetry).To(BeTrue())
				Expect(err).To(MatchError(ContainSubstring("retry")))
			case "fail":
				Expect(shouldRetry).To(BeFalse())
			case "fallback":
				// Should fall back to cache or return partial results
				Expect(err).To(BeNil()) // Error handled gracefully
			}
		},
		// Retryable errors
		Entry("connection timeout", "timeout", "retry", true),
		Entry("deadlock detected", "deadlock", "retry", true),
		Entry("connection pool exhausted", "pool_exhausted", "retry", true),
		Entry("transient network error", "transient", "retry", true),
		
		// Non-retryable errors
		Entry("constraint violation", "constraint", "fail", false),
		Entry("syntax error", "syntax", "fail", false),
		Entry("permission denied", "permission", "fail", false),
		Entry("table not found", "not_found", "fail", false),
		
		// Fallback scenarios
		Entry("read replica unavailable", "replica_down", "fallback", false),
		Entry("slow query timeout", "slow_query", "fallback", false),
	)
	
	DescribeTable("connection lifecycle error scenarios",
		func(setupFunc func(), expectedOutcome string) {
			setupFunc()
			
			err := dbClient.HealthCheck(ctx)
			
			switch expectedOutcome {
			case "healthy":
				Expect(err).ToNot(HaveOccurred())
			case "degraded":
				Expect(err).ToNot(HaveOccurred())
				// But should log warning
			case "unhealthy":
				Expect(err).To(HaveOccurred())
			}
		},
		Entry("all connections available", func() {
			// Normal state
		}, "healthy"),
		
		Entry("80% connections available", func() {
			exhaustConnections(0.8)
		}, "healthy"),
		
		Entry("50% connections available", func() {
			exhaustConnections(0.5)
		}, "degraded"),
		
		Entry("10% connections available", func() {
			exhaustConnections(0.1)
		}, "unhealthy"),
		
		Entry("zero connections available", func() {
			exhaustConnections(0.0)
		}, "unhealthy"),
	)
})
```

#### Redis Error Scenarios

```go
var _ = Describe("BR-CONTEXT-005: Redis Error Handling", func() {
	DescribeTable("Redis failure scenarios",
		func(errorType string, expectedBehavior string) {
			mockRedis.SetErrorType(errorType)
			
			err := cacheManager.Get(ctx, "test-key", &result)
			
			switch expectedBehavior {
			case "graceful_degradation":
				// Should NOT fail, but log warning and continue without cache
				Expect(err).To(MatchError(models.ErrCacheMiss))
			case "fail":
				Expect(err).To(HaveOccurred())
			}
		},
		// Graceful degradation scenarios
		Entry("connection refused", "connection_refused", "graceful_degradation"),
		Entry("connection timeout", "timeout", "graceful_degradation"),
		Entry("authentication failed", "auth_failed", "graceful_degradation"),
		Entry("out of memory", "oom", "graceful_degradation"),
		Entry("maxclients reached", "maxclients", "graceful_degradation"),
		
		// These should also degrade gracefully (not fail the request)
		Entry("network partition", "network_partition", "graceful_degradation"),
		Entry("Redis failover in progress", "failover", "graceful_degradation"),
	)
})
```

---

### 5. Null/Empty Cases (MANDATORY)

```go
// test/unit/contextapi/models_test.go (ADD THIS SECTION)

var _ = Describe("BR-CONTEXT-002: Null/Empty Case Testing", func() {
	Context("Query Parameters", func() {
		DescribeTable("nil and empty handling",
			func(setupFunc func() *models.QueryParams, expectValid bool) {
				params := setupFunc()
				err := params.Validate()
				
				if expectValid {
					Expect(err).ToNot(HaveOccurred())
				} else {
					Expect(err).To(HaveOccurred())
				}
			},
			// Nil cases
			Entry("all fields nil except limit", func() *models.QueryParams {
				return &models.QueryParams{Limit: 10}
			}, true),
			
			Entry("nil namespace (valid)", func() *models.QueryParams {
				return &models.QueryParams{
					Namespace: nil,
					Limit:     10,
				}
			}, true),
			
			// Empty cases
			Entry("empty namespace (valid)", func() *models.QueryParams {
				return &models.QueryParams{
					Namespace: stringPtr(""),
					Limit:     10,
				}
			}, true),
			
			Entry("empty severity (invalid)", func() *models.QueryParams {
				return &models.QueryParams{
					Severity: stringPtr(""),
					Limit:    10,
				}
			}, false),
			
			// Empty slices
			Entry("empty alert names slice", func() *models.QueryParams {
				return &models.QueryParams{
					AlertNames: []string{},
					Limit:      10,
				}
			}, true),
			
			// Nil struct
			Entry("nil query params (invalid)", func() *models.QueryParams {
				return nil
			}, false),
		)
	})
	
	Context("Semantic Search Parameters", func() {
		DescribeTable("nil embedding handling",
			func(embedding []float32, expectValid bool) {
				params := &models.SemanticSearchParams{
					Embedding: embedding,
					Threshold: 0.8,
					Limit:     10,
				}
				
				err := params.Validate()
				
				if expectValid {
					Expect(err).ToNot(HaveOccurred())
				} else {
					Expect(err).To(HaveOccurred())
				}
			},
			Entry("nil embedding (invalid)", nil, false),
			Entry("empty embedding slice (invalid)", []float32{}, false),
			Entry("valid embedding", make([]float32, 384), true),
		)
	})
})
```

---

## 📋 **Test Coverage Expansion Checklist**

Before considering Context API testing complete, verify:

### Unit Tests (70%+ coverage)
- [ ] **Boundary Values**: Min, max, edge values for all numeric inputs (limits, offsets, dimensions)
- [ ] **Invalid Inputs**: All validation rules tested with invalid data
- [ ] **Null/Empty Cases**: Nil, empty string, empty slice/map handling
- [ ] **State Combinations**: Realistic system state combinations tested
- [ ] **Error Conditions**: All error types from validation logic

### Integration Tests (>50% coverage)
- [ ] **Cache State Matrix**: All Redis + L2 + DB state combinations (8 scenarios minimum)
- [ ] **Database Errors**: Connection timeouts, deadlocks, pool exhaustion, constraint violations
- [ ] **Redis Errors**: Connection refused, timeout, auth failed, OOM, maxclients
- [ ] **Connection Lifecycle**: Healthy, degraded, unhealthy states
- [ ] **Error Recovery**: Retry logic, graceful degradation, fallback mechanisms

### E2E Tests (10-15% coverage)
- [ ] **Complete Workflows**: Alert → Context → Response paths
- [ ] **Failure Scenarios**: Service degradation, partial failures
- [ ] **Performance**: Response time under load

---

## 🎯 **Implementation Priority**

### Phase 1: Unit Test Expansion (Day 2-3 of implementation)
1. Add boundary value DescribeTable tests to query_builder_test.go
2. Add input validation matrix to models_test.go
3. Add vector dimension validation to vector_search_test.go

**Estimated Time**: 3 hours
**BR Coverage Impact**: +15 edge case scenarios

### Phase 2: Integration Test Expansion (Day 8 of implementation)
1. Expand cache_fallback_test.go with state matrix (8 scenarios)
2. Create error_scenarios_test.go with database error coverage (15 scenarios)
3. Add Redis error handling tests

**Estimated Time**: 4 hours
**BR Coverage Impact**: +23 error path scenarios

### Phase 3: Continuous Expansion
1. Add new edge cases as discovered during implementation
2. Bug-driven test addition (regression tests)
3. Production monitoring insights

---

## 📊 **Expected Coverage Metrics**

### Before Defense-in-Depth Expansion
- Unit tests: ~40 scenarios
- Integration tests: ~25 scenarios
- Total edge cases: ~5 scenarios

### After Defense-in-Depth Expansion
- Unit tests: ~70 scenarios (+30)
- Integration tests: ~50 scenarios (+25)
- Total edge cases: ~40 scenarios (+35)

### Confidence Impact
- Before: 85% confidence (insufficient edge case coverage)
- After: 92% confidence (comprehensive edge case + error path coverage)

---

## 🔗 **Integration with Implementation Plan**

This defense-in-depth strategy should be implemented alongside the main implementation plan:

1. **Day 2**: Add boundary value tests during query builder implementation
2. **Day 3**: Add state matrix tests during cache layer implementation  
3. **Day 5**: Add vector dimension tests during pattern matching implementation
4. **Day 8**: Add comprehensive error scenarios during integration testing

**Total Additional Time**: ~7 hours (spread across implementation days)
**Quality Impact**: Production-ready edge case coverage per project standards

---

## ✅ **Compliance with Project Standards**

This strategy ensures compliance with [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc):

- ✅ **Defense-in-Depth**: Multiple testing layers with overlapping coverage
- ✅ **Pyramid Approach**: 70%+ unit, >50% integration, 10-15% E2E
- ✅ **Boundary Value Analysis**: Systematic testing of input boundaries
- ✅ **State Matrix Coverage**: All realistic state combinations tested
- ✅ **Error Path Coverage**: All failure modes validated
- ✅ **DescribeTable Usage**: Reduces code duplication (25-40% reduction)
- ✅ **Business Requirement Mapping**: All tests map to BR-CONTEXT-XXX requirements

---

**Document Version**: 1.0  
**Last Updated**: 2025-10-15  
**Status**: ✅ Ready for implementation alongside IMPLEMENTATION_PLAN_V1.2




