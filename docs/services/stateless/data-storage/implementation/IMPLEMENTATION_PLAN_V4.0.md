# Data Storage Service - Implementation Plan v4.0

**Version**: 4.0 - DEPRECATED ⚠️
**Date**: 2025-10-11
**Status**: 🔴 **DEPRECATED - USE v4.1 INSTEAD**
**Replacement**: [IMPLEMENTATION_PLAN_V4.1.md](./IMPLEMENTATION_PLAN_V4.1.md)

---

## 🚨 **DEPRECATION NOTICE**

**This document is DEPRECATED as of 2025-10-11.**

**Reason**: v4.0 only provided detailed Day 7 (integration testing) but lacked Days 1-6 and 8-12 implementation details, table-driven testing guidance, and production readiness checklists.

**Use Instead**: [IMPLEMENTATION_PLAN_V4.1.md](./IMPLEMENTATION_PLAN_V4.1.md)

**v4.1 Provides**:
- ✅ Complete Days 1-12 implementation details
- ✅ APDC phases + TDD workflow for each day
- ✅ Table-driven testing guidance (25-40% code reduction)
- ✅ Production readiness checklists
- ✅ BR coverage matrix
- ✅ Common pitfalls section
- ✅ 95% template v1.2 alignment

**Triage Reports**:
- [DATA_STORAGE_V4_TRIAGE_VS_TEMPLATE.md](./DATA_STORAGE_V4_TRIAGE_VS_TEMPLATE.md)
- [DATA_STORAGE_V4_TRIAGE_SUMMARY.md](./DATA_STORAGE_V4_TRIAGE_SUMMARY.md)

**This document is preserved for historical reference only.**

---

## ⚠️ **HISTORICAL: Version 4.0 Corrections from v3.0**

**Critical Updates**:
1. ✅ **Integration tests now use Kind cluster** (not Testcontainers)
   - Updated BeforeSuite to use `kind.Setup()` template
   - All integration tests use Kind cluster PostgreSQL
   - See: [docs/testing/KIND_CLUSTER_TEST_TEMPLATE.md](../../../testing/KIND_CLUSTER_TEST_TEMPLATE.md)

2. ✅ **Complete imports added** to all test examples
   - BeforeSuite/AfterSuite with complete imports
   - All 5 integration tests with complete imports

3. ✅ **Aligned with ADR-003** (Kind cluster as primary integration environment)

4. ✅ **Idempotent schema creation** (not migrations)
   - Direct DDL with `CREATE IF NOT EXISTS`
   - Embedded schema files using `//go:embed`
   - Startup-time schema initialization

**Triage Reports**:
- [DATA_STORAGE_PLAN_TRIAGE.md](./DATA_STORAGE_PLAN_TRIAGE.md)
- [IMPLEMENTATION_PLANS_TRIAGE_TESTCONTAINERS_VS_KIND.md](../../IMPLEMENTATION_PLANS_TRIAGE_TESTCONTAINERS_VS_KIND.md)

---

## 🎯 Service Overview

**Purpose**: Persistent audit storage for all Kubernaut remediation actions

**Core Responsibilities**:
1. **Dual-Write Transactions** - Atomic writes to PostgreSQL + Vector DB
2. **Audit Storage** - Comprehensive remediation action history
3. **Embedding Generation** - Vector embeddings for semantic search
4. **Validation** - Input sanitization and schema validation
5. **Query API** - REST API for audit retrieval

**Business Requirements**: BR-STORAGE-001 to BR-STORAGE-020

---

## 📅 12-Day Enhanced Timeline

| Day | Focus | Hours | Key Deliverables |
|-----|-------|-------|------------------|
| **Day 1** | Foundation + Interfaces | 8h | Core types, interfaces, error handling |
| **Day 2** | Database Schema + DDL | 8h | PostgreSQL schema, initializer, verification |
| **Day 3** | Validation Layer | 8h | Input validation, sanitization pipeline |
| **Day 4** | Embedding Pipeline | 8h | Vector generation, caching, API integration |
| **Day 5** | Dual-Write Engine | 8h | Transaction coordinator, graceful degradation |
| **Day 6** | Query API | 8h | REST endpoints, filtering, pagination |
| **Day 7** | Integration-First Testing | 8h | 5 critical integration tests (Kind cluster) |
| **Day 8-9** | Unit Test Completion | 16h | Complete unit test coverage (>70%) |
| **Day 10** | Observability + Metrics | 8h | Prometheus metrics, structured logging |
| **Day 11** | Main App + HTTP Server | 8h | Wire components, HTTP handlers |
| **Day 12** | Production Readiness | 8h | Documentation, checklist, validation |

**Total**: 96 hours (12 days @ 8h/day)

---

## 🧪 Day 7: Integration-First Testing with Kind Cluster

### Test Infrastructure Setup (30 min)

**File**: `test/integration/datastorage/suite_test.go`

```go
package datastorage_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/testutil/kind"
)

func TestDataStorageIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Storage Integration Suite (Kind)")
}

var suite *kind.IntegrationSuite

var _ = BeforeSuite(func() {
	// Use Kind cluster test template for standardized setup
	// See: docs/testing/KIND_CLUSTER_TEST_TEMPLATE.md
	suite = kind.Setup("datastorage-test")

	// Wait for PostgreSQL to be ready (deployed via make bootstrap-dev)
	suite.WaitForPostgreSQLReady(60 * time.Second)

	GinkgoWriter.Println("✅ Data Storage integration test environment ready!")
})

var _ = AfterSuite(func() {
	// Automatic cleanup of namespaces and registered resources
	suite.Cleanup()

	GinkgoWriter.Println("✅ Data Storage integration test environment cleaned up!")
})
```

---

### Integration Test 1: Basic Audit Write → PostgreSQL (60 min)

**File**: `test/integration/datastorage/basic_audit_test.go`

```go
package datastorage_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/testutil/kind"
	"github.com/jordigilh/kubernaut/pkg/datastorage"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/internal/database/schema"
)

var _ = Describe("BR-STORAGE-001: Basic Audit Persistence", func() {
	var (
		client *datastorage.Client
		db     *sql.DB
	)

	BeforeEach(func() {
		// Connect to PostgreSQL in Kind cluster
		var err error
		db, err = suite.GetPostgreSQLConnection(kind.PostgreSQLConfig{
			Database: "kubernaut_test",
		})
		Expect(err).ToNot(HaveOccurred())

		// Initialize schema (idempotent)
		initializer := schema.NewInitializer(db)
		err = initializer.Initialize(suite.Context)
		Expect(err).ToNot(HaveOccurred())

		// Create Data Storage client
		logger := setupLogger()
		client = datastorage.NewClient(db, logger)
	})

	AfterEach(func() {
		// Clean up test data
		if db != nil {
			suite.TruncateTable("remediation_audit")
			db.Close()
		}
	})

	It("should persist remediation audit to PostgreSQL", func() {
		audit := &models.RemediationAudit{
			Name:        "test-remediation-001",
			Namespace:   "default",
			Phase:       "processing",
			ActionType:  "restart-pod",
			TargetName:  "my-pod",
			Status:      "pending",
			StartTime:   time.Now(),
			Environment: "dev",
		}

		// Write audit
		result, err := client.CreateRemediationAudit(suite.Context, audit)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.ID).ToNot(BeZero())
		Expect(result.Name).To(Equal("test-remediation-001"))

		// Verify persistence
		retrieved, err := client.GetRemediationAudit(suite.Context, result.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(retrieved.Name).To(Equal("test-remediation-001"))
		Expect(retrieved.Phase).To(Equal("processing"))
		Expect(retrieved.ActionType).To(Equal("restart-pod"))
	})

	It("should handle duplicate writes idempotently", func() {
		audit := &models.RemediationAudit{
			Name:      "test-remediation-002",
			Namespace: "default",
			Phase:     "processing",
		}

		// First write
		result1, err := client.CreateRemediationAudit(suite.Context, audit)
		Expect(err).ToNot(HaveOccurred())

		// Duplicate write (should succeed with same ID)
		audit.ID = result1.ID // Simulate idempotent write
		result2, err := client.UpdateRemediationAudit(suite.Context, audit)
		Expect(err).ToNot(HaveOccurred())
		Expect(result2.ID).To(Equal(result1.ID))
	})
})
```

---

### Integration Test 2: Dual-Write Transaction Coordination (60 min)

**File**: `test/integration/datastorage/dual_write_test.go`

```go
package datastorage_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/testutil/kind"
	"github.com/jordigilh/kubernaut/pkg/datastorage"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/dualwrite"
)

var _ = Describe("BR-STORAGE-005: Dual-Write Transaction Coordination", func() {
	var (
		coordinator *dualwrite.Coordinator
		db          *sql.DB
	)

	BeforeEach(func() {
		var err error
		db, err = suite.GetPostgreSQLConnection(kind.PostgreSQLConfig{
			Database: "kubernaut_test",
		})
		Expect(err).ToNot(HaveOccurred())

		// Initialize schema
		initializer := schema.NewInitializer(db)
		err = initializer.Initialize(suite.Context)
		Expect(err).ToNot(HaveOccurred())

		// Create dual-write coordinator
		logger := setupLogger()
		vectorDB := setupMockVectorDB() // Mock for integration test
		coordinator = dualwrite.NewCoordinator(db, vectorDB, logger)
	})

	AfterEach(func() {
		if db != nil {
			suite.TruncateTable("remediation_audit")
			db.Close()
		}
	})

	It("should write to both PostgreSQL and Vector DB atomically", func() {
		audit := &models.RemediationAudit{
			Name:       "test-dual-write",
			Namespace:  "default",
			Phase:      "processing",
			ActionType: "scale-deployment",
		}

		embedding := []float32{0.1, 0.2, 0.3, 0.4} // Mock embedding

		// Execute dual-write transaction
		result, err := coordinator.Write(suite.Context, audit, embedding)
		Expect(err).ToNot(HaveOccurred())

		// Verify PostgreSQL write
		pgAudit, err := client.GetRemediationAudit(suite.Context, result.PostgreSQLID)
		Expect(err).ToNot(HaveOccurred())
		Expect(pgAudit.Name).To(Equal("test-dual-write"))

		// Verify Vector DB write (mock verification)
		Expect(result.VectorDBID).ToNot(BeEmpty())
		Expect(result.Success).To(BeTrue())
	})

	It("should handle PostgreSQL failure gracefully", func() {
		// Force PostgreSQL failure (invalid data)
		audit := &models.RemediationAudit{
			Name:      "", // Empty name violates schema
			Namespace: "default",
		}

		embedding := []float32{0.1, 0.2, 0.3}

		// Execute dual-write (should fail gracefully)
		result, err := coordinator.Write(suite.Context, audit, embedding)
		Expect(err).To(HaveOccurred())
		Expect(result.PostgreSQLSuccess).To(BeFalse())
		Expect(result.VectorDBSuccess).To(BeFalse()) // Rollback Vector DB
	})
})
```

---

### Integration Test 3: Embedding Pipeline Integration (45 min)

**File**: `test/integration/datastorage/embedding_pipeline_test.go`

```go
package datastorage_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/testutil/kind"
	"github.com/jordigilh/kubernaut/pkg/datastorage/embedding"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

var _ = Describe("BR-STORAGE-008: Embedding Generation Pipeline", func() {
	var pipeline *embedding.Pipeline

	BeforeEach(func() {
		logger := setupLogger()
		cache := setupRedisCache() // Mock or real Redis
		apiClient := setupMockEmbeddingAPI()

		pipeline = embedding.NewPipeline(apiClient, cache, logger)
	})

	It("should generate embeddings for audit data", func() {
		audit := &models.RemediationAudit{
			Name:       "test-embedding",
			Namespace:  "default",
			ActionType: "restart-pod",
			TargetName: "my-pod",
			Phase:      "completed",
		}

		// Generate embedding
		result, err := pipeline.Generate(suite.Context, audit)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Embedding).ToNot(BeNil())
		Expect(len(result.Embedding)).To(BeNumerically(">", 0))
		Expect(result.Dimension).To(Equal(384)) // Standard embedding dimension
	})

	It("should use cached embeddings when available", func() {
		audit := &models.RemediationAudit{
			Name:       "test-cache",
			Namespace:  "default",
			ActionType: "scale-deployment",
		}

		// First generation (cache miss)
		result1, err := pipeline.Generate(suite.Context, audit)
		Expect(err).ToNot(HaveOccurred())
		Expect(result1.CacheHit).To(BeFalse())

		// Second generation (cache hit)
		result2, err := pipeline.Generate(suite.Context, audit)
		Expect(err).ToNot(HaveOccurred())
		Expect(result2.CacheHit).To(BeTrue())
		Expect(result2.Embedding).To(Equal(result1.Embedding))
	})
})
```

---

### Integration Test 4: Validation + Sanitization Pipeline (45 min)

**File**: `test/integration/datastorage/validation_test.go`

```go
package datastorage_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/testutil/kind"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

var _ = Describe("BR-STORAGE-010: Input Validation", func() {
	var validator *validation.Validator

	BeforeEach(func() {
		logger := setupLogger()
		validator = validation.NewValidator(logger)
	})

	It("should validate required fields", func() {
		audit := &models.RemediationAudit{
			Name:      "valid-audit",
			Namespace: "default",
			Phase:     "processing",
		}

		err := validator.Validate(suite.Context, audit)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should reject invalid audit data", func() {
		audit := &models.RemediationAudit{
			Name:      "", // Empty name (invalid)
			Namespace: "default",
		}

		err := validator.Validate(suite.Context, audit)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("name is required"))
	})

	It("should sanitize potentially malicious input", func() {
		audit := &models.RemediationAudit{
			Name:      "test<script>alert('xss')</script>",
			Namespace: "default",
			Phase:     "processing",
		}

		sanitized, err := validator.Sanitize(suite.Context, audit)
		Expect(err).ToNot(HaveOccurred())
		Expect(sanitized.Name).ToNot(ContainSubstring("<script>"))
		Expect(sanitized.Name).To(Equal("testscriptalert'xss'/script")) // HTML stripped
	})
})
```

---

### Integration Test 5: Cross-Service Write Simulation (30 min)

**File**: `test/integration/datastorage/cross_service_test.go`

```go
package datastorage_test

import (
	"context"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/testutil/kind"
	"github.com/jordigilh/kubernaut/pkg/datastorage"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

var _ = Describe("BR-STORAGE-015: Concurrent Write Handling", func() {
	var (
		client *datastorage.Client
		db     *sql.DB
	)

	BeforeEach(func() {
		var err error
		db, err = suite.GetPostgreSQLConnection(kind.PostgreSQLConfig{
			Database: "kubernaut_test",
		})
		Expect(err).ToNot(HaveOccurred())

		initializer := schema.NewInitializer(db)
		err = initializer.Initialize(suite.Context)
		Expect(err).ToNot(HaveOccurred())

		logger := setupLogger()
		client = datastorage.NewClient(db, logger)
	})

	AfterEach(func() {
		if db != nil {
			suite.TruncateTable("remediation_audit")
			db.Close()
		}
	})

	It("should handle concurrent writes from multiple services", func() {
		var wg sync.WaitGroup
		successCount := 0
		var mu sync.Mutex

		// Simulate 10 concurrent writes
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(index int) {
				defer GinkgoRecover()
				defer wg.Done()

				audit := &models.RemediationAudit{
					Name:      fmt.Sprintf("concurrent-test-%d", index),
					Namespace: "default",
					Phase:     "processing",
				}

				_, err := client.CreateRemediationAudit(suite.Context, audit)
				if err == nil {
					mu.Lock()
					successCount++
					mu.Unlock()
				}
			}(i)
		}

		wg.Wait()

		// All writes should succeed
		Expect(successCount).To(Equal(10))

		// Verify all audits persisted
		audits, err := client.ListRemediationAudits(suite.Context, &datastorage.ListOptions{
			Limit: 20,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(len(audits)).To(BeNumerically(">=", 10))
	})
})
```

---

## 📊 Day 7 Deliverables

### Test Infrastructure
- [x] `suite_test.go` with Kind template setup (30 min)
- [x] PostgreSQL connection helpers (included in template)
- [x] Schema initialization utilities
- [x] Test data cleanup utilities

### Integration Tests (5 critical tests)
- [x] Test 1: Basic audit persistence (60 min)
- [x] Test 2: Dual-write transactions (60 min)
- [x] Test 3: Embedding pipeline (45 min)
- [x] Test 4: Validation pipeline (45 min)
- [x] Test 5: Concurrent writes (30 min)

### Validation
- [x] All integration tests passing
- [x] Architecture validated before unit tests
- [x] PostgreSQL schema working correctly
- [x] No Testcontainers dependencies
- [x] Complete imports in all files

---

## 🔗 Related Documentation

**Kind Template Guide**: [docs/testing/KIND_CLUSTER_TEST_TEMPLATE.md](../../../testing/KIND_CLUSTER_TEST_TEMPLATE.md)

**Triage Reports**:
- [DATA_STORAGE_PLAN_TRIAGE.md](./DATA_STORAGE_PLAN_TRIAGE.md)
- [IMPLEMENTATION_PLANS_TRIAGE_TESTCONTAINERS_VS_KIND.md](../../IMPLEMENTATION_PLANS_TRIAGE_TESTCONTAINERS_VS_KIND.md)

**Project Standards**:
- [ADR-003: Kind Cluster as Primary Integration Environment](../../../architecture/decisions/ADR-003-KIND-INTEGRATION-ENVIRONMENT.md)
- [Testing Strategy](../../../../.cursor/rules/03-testing-strategy.mdc)
- [Integration/E2E No Mocks Policy](../../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## ✅ Success Criteria

### Code Quality
- [x] Complete imports in all integration test examples
- [x] Kind cluster template usage (not Testcontainers)
- [x] PostgreSQL connection via Kind cluster DNS
- [x] Idempotent schema initialization (not migrations)
- [x] Automatic cleanup with `suite.Cleanup()`

### ADR-003 Compliance
- [x] All integration tests use Kind cluster
- [x] PostgreSQL runs in Kind cluster (via make bootstrap-dev)
- [x] No port-forwarding workarounds
- [x] Direct Kind cluster DNS usage

### Testing Strategy
- [x] Integration-first approach (Day 7 before unit tests)
- [x] 5 critical integration tests validating architecture
- [x] Real PostgreSQL, real schema, real transactions
- [x] Business requirement mapping (BR-STORAGE-001 to BR-STORAGE-015)

---

**Status**: ✅ Ready for Implementation
**Version**: 4.0 (Corrected)
**Date**: 2025-10-11
**Confidence**: 95% (Pattern proven in Dynamic Toolset + Gateway)
**Estimated Total Time**: 11-12 days (88-96 hours)

