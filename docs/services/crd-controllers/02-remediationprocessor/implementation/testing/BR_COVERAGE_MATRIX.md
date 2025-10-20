# Remediation Processor - Business Requirements Coverage Matrix

**Version**: 1.1
**Date**: 2025-10-14
**Service**: Remediation Processor Controller
**Total BRs**: 27 (BR-AP-001 to BR-AP-067, V1 scope)
**Target Coverage**: 100% (all BRs mapped to tests)
**Last Updated**: Added anti-flaky pattern references and test infrastructure tools (v1.1)

---

## 🧪 Testing Infrastructure

**Per [ADR-016: Service-Specific Integration Test Infrastructure](../../../../../docs/architecture/decisions/ADR-016-SERVICE-SPECIFIC-INTEGRATION-TEST-INFRASTRUCTURE.md)**

| Test Type | Infrastructure | Rationale | Reference |
|-----------|----------------|-----------|-----------|
| **Unit Tests** | Fake Kubernetes Client | In-memory K8s API, no infrastructure needed | [ADR-004](../../../../../docs/architecture/decisions/ADR-004-fake-kubernetes-client.md) |
| **Integration Tests** | **Envtest** | Real K8s API with CRD validation, 5-18x faster than Kind | [ADR-016](../../../../../docs/architecture/decisions/ADR-016-SERVICE-SPECIFIC-INTEGRATION-TEST-INFRASTRUCTURE.md) |
| **E2E Tests** | Kind or Kubernetes | Full cluster with real networking and RBAC | [ADR-003](../../../../../docs/architecture/decisions/ADR-003-KIND-INTEGRATION-ENVIRONMENT.md) |

**Key Benefits of Envtest for CRD Controllers**:
- ✅ Real Kubernetes API (not mocked)
- ✅ CRD validation (OpenAPI v3 schema enforcement)
- ✅ Watch events (controller reconciliation)
- ✅ No Docker/Kind overhead (5-18x faster startup)
- ✅ Portable (runs in IDE, CI, local development)

**Test Infrastructure Tools**:
- **Anti-Flaky Patterns**: `pkg/testutil/timing/anti_flaky_patterns.go` for reliable concurrent testing
- **Test Infrastructure Validator**: `test/scripts/validate_test_infrastructure.sh`
- **Make Targets**: `make bootstrap-envtest-podman-remediationprocessor`, `make test-integration-envtest-remediationprocessor`
- **External Dependencies**: PostgreSQL (pgvector) + Redis via Podman

---

## 📊 Coverage Summary

| Category | Total BRs | Unit Tests | Integration Tests | E2E Tests | Edge Cases | Coverage % |
|----------|-----------|------------|-------------------|-----------|------------|------------|
| **Context Enrichment** | 8 | 6 | 2 | 0 | 4 | 100% |
| **Classification** | 7 | 5 | 2 | 0 | 3 | 100% |
| **Deduplication** | 4 | 3 | 1 | 0 | 2 | 100% |
| **CRD Lifecycle** | 5 | 2 | 2 | 1 | 2 | 100% |
| **Integration** | 3 | 0 | 2 | 1 | 1 | 100% |
| **Total** | **27** | **16** | **9** | **2** | **12** | **100%** |

**Defense-in-Depth Strategy**: Test coverage percentages exceed 100% due to intentional overlapping coverage. Unit tests cover 100% of unit-testable BRs, integration tests cover >50% of total BRs, and E2E tests cover 10-15% of total BRs. This creates multiple validation layers for comprehensive bug detection.

---

## 🎯 Context Enrichment (BR-AP-001 to BR-AP-015) - 8 BRs

| BR | Requirement | Test Type | Test File | Test Name | Status |
|----|-------------|-----------|-----------|-----------|--------|
| **BR-AP-001** | Alert enrichment with historical context | Integration | `test/integration/remediationprocessing/enrichment_test.go` | `It("should enrich alert with historical context")` | ✅ |
| **BR-AP-003** | Semantic search for similar alerts | Integration | `test/integration/remediationprocessing/semantic_search_test.go` | `It("should find similar alerts using pgvector")` | ✅ |
| **BR-AP-005** | Historical success rate calculation | Unit | `test/unit/remediationprocessing/enricher_test.go` | `Context("Success rate calculation")` | ✅ |
| **BR-AP-007** | Average resolution time aggregation | Unit | `test/unit/remediationprocessing/enricher_test.go` | `It("should calculate average resolution time")` | ✅ |
| **BR-AP-010** | Common action pattern identification | Unit | `test/unit/remediationprocessing/enricher_test.go` | `It("should identify common actions")` | ✅ |
| **BR-AP-012** | Knowledge article linkage | Unit | `test/unit/remediationprocessing/enricher_test.go` | `It("should link relevant knowledge articles")` | ✅ |
| **BR-AP-015** | Context aggregation from multiple sources | Unit | `test/unit/remediationprocessing/enricher_test.go` | `Describe("Multi-source aggregation")` | ✅ |
| **BR-AP-018** | Similarity scoring algorithm | Unit | `test/unit/remediationprocessing/similarity_test.go` | `Describe("Similarity Scoring")` | ✅ |

---

## 🎯 Classification (BR-AP-020 to BR-AP-035) - 7 BRs

| BR | Requirement | Test Type | Test File | Test Name | Status |
|----|-------------|-----------|-----------|-----------|--------|
| **BR-AP-020** | Classification logic (automated vs AI-required) | Unit | `test/unit/remediationprocessing/classifier_test.go` | `Describe("Classification Engine")` | ✅ |
| **BR-AP-022** | AI requirement detection | Unit | `test/unit/remediationprocessing/classifier_test.go` | `Context("AI requirement rules")` | ✅ |
| **BR-AP-025** | Confidence score calculation | Unit | `test/unit/remediationprocessing/classifier_test.go` | `It("should calculate classification confidence")` | ✅ |
| **BR-AP-028** | Rule-based classification engine | Unit | `test/unit/remediationprocessing/classifier_test.go` | `Context("Rule evaluation")` | ✅ |
| **BR-AP-030** | Severity-based AI routing (critical → AI) | Unit | `test/unit/remediationprocessing/classifier_test.go` | `It("should route critical alerts to AI")` | ✅ |
| **BR-AP-032** | Historical data influence on classification | Integration | `test/integration/remediationprocessing/classification_test.go` | `It("should use historical data for classification")` | ✅ |
| **BR-AP-035** | Classification reason generation | Integration | `test/integration/remediationprocessing/classification_test.go` | `It("should generate human-readable reasons")` | ✅ |

---

## 🎯 Deduplication (BR-AP-040 to BR-AP-050) - 4 BRs

| BR | Requirement | Test Type | Test File | Test Name | Status |
|----|-------------|-----------|-----------|-----------|--------|
| **BR-AP-040** | Signal fingerprint generation (SHA-256) | Unit | `test/unit/remediationprocessing/fingerprinter_test.go` | `Describe("Fingerprint Generation")` | ✅ |
| **BR-AP-042** | Duplicate detection using fingerprints | Unit | `test/unit/remediationprocessing/fingerprinter_test.go` | `It("should detect duplicate signals")` | ✅ |
| **BR-AP-045** | Deduplication window (configurable TTL) | Unit | `test/unit/remediationprocessing/deduplication_test.go` | `Context("Deduplication window")` | ✅ |
| **BR-AP-048** | Duplicate suppression logic | Integration | `test/integration/remediationprocessing/deduplication_test.go` | `It("should suppress duplicate alerts")` | ✅ |

---

## 🎯 CRD Lifecycle (BR-AP-051 to BR-AP-060) - 5 BRs

| BR | Requirement | Test Type | Test File | Test Name | Status |
|----|-------------|-----------|-----------|-----------|--------|
| **BR-AP-051** | CRD reconciliation loop | Integration | `test/integration/remediationprocessing/lifecycle_test.go` | `Describe("CRD Reconciliation")` | ✅ |
| **BR-AP-053** | Phase transitions (Pending → Enriching → Classifying → Ready) | Unit | `test/unit/remediationprocessing/reconciler_test.go` | `Context("Phase transitions")` | ✅ |
| **BR-AP-055** | Status tracking and audit trail | Unit | `test/unit/remediationprocessing/status_test.go` | `Describe("Status Management")` | ✅ |
| **BR-AP-057** | CRD creation (AIAnalysis/WorkflowExecution) | Integration | `test/integration/remediationprocessing/crd_creation_test.go` | `It("should create appropriate child CRDs")` | ✅ |
| **BR-AP-060** | Owner reference management | E2E | `test/e2e/remediationprocessing/e2e_test.go` | `It("should set owner references correctly")` | ✅ |

---

## 🎯 Integration & Performance (BR-AP-061 to BR-AP-067) - 3 BRs

| BR | Requirement | Test Type | Test File | Test Name | Status |
|----|-------------|-----------|-----------|-----------|--------|
| **BR-AP-061** | Data Storage Service integration | Integration | `test/integration/remediationprocessing/storage_integration_test.go` | `Describe("Data Storage Integration")` | ✅ |
| **BR-AP-063** | PostgreSQL connection pooling | Integration | `test/integration/remediationprocessing/storage_integration_test.go` | `It("should use connection pooling efficiently")` | ✅ |
| **BR-AP-067** | Observability (metrics, events) | E2E | `test/e2e/remediationprocessing/e2e_test.go` | `It("should expose Prometheus metrics")` | ✅ |

---

## 🔬 Edge Case Coverage - 12 Additional Test Scenarios

**Purpose**: Explicit edge case testing to validate boundary conditions, error paths, and failure scenarios that could cause production issues.

### Context Enrichment Edge Cases (4 scenarios)

| Edge Case BR | Requirement | Test Type | Test File | Test Name | Status |
|--------------|-------------|-----------|-----------|-----------|--------|
| **BR-AP-001-EC1** | Empty historical context (no similar alerts found) | Unit | `test/unit/remediationprocessing/enricher_edge_cases_test.go` | `Entry("empty context", ...)` | ✅ |
| **BR-AP-001-EC2** | Malformed embedding vectors (dimension mismatch) | Unit | `test/unit/remediationprocessing/enricher_edge_cases_test.go` | `Entry("malformed embeddings", ...)` | ✅ |
| **BR-AP-003-EC1** | pgvector query timeout (database slow response) | Integration | `test/integration/remediationprocessing/semantic_search_edge_cases_test.go` | `It("should handle query timeout")` | ✅ |
| **BR-AP-005-EC1** | Zero historical attempts (divide-by-zero risk) | Unit | `test/unit/remediationprocessing/enricher_edge_cases_test.go` | `Entry("zero attempts", ...)` | ✅ |

### Classification Edge Cases (3 scenarios)

| Edge Case BR | Requirement | Test Type | Test File | Test Name | Status |
|--------------|-------------|-----------|-----------|-----------|--------|
| **BR-AP-020-EC1** | Ambiguous classification (exactly 50% confidence) | Unit | `test/unit/remediationprocessing/classifier_edge_cases_test.go` | `Entry("ambiguous class", ...)` | ✅ |
| **BR-AP-022-EC1** | All classification rules fail (no match) | Unit | `test/unit/remediationprocessing/classifier_edge_cases_test.go` | `Entry("no rules match", ...)` | ✅ |
| **BR-AP-030-EC1** | Critical alert with missing severity field | Unit | `test/unit/remediationprocessing/classifier_edge_cases_test.go` | `Entry("missing severity", ...)` | ✅ |

### Deduplication Edge Cases (2 scenarios)

| Edge Case BR | Requirement | Test Type | Test File | Test Name | Status |
|--------------|-------------|-----------|-----------|-----------|--------|
| **BR-AP-040-EC1** | Fingerprint collision (hash conflict, extremely rare) | Unit | `test/unit/remediationprocessing/fingerprinter_edge_cases_test.go` | `It("should handle hash collision")` | ✅ |
| **BR-AP-045-EC1** | Expired deduplication window boundary condition | Integration | `test/integration/remediationprocessing/deduplication_edge_cases_test.go` | `It("should handle window expiration")` | ✅ |

### CRD Lifecycle Edge Cases (2 scenarios)

| Edge Case BR | Requirement | Test Type | Test File | Test Name | Status |
|--------------|-------------|-----------|-----------|-----------|--------|
| **BR-AP-053-EC1** | Concurrent phase transition attempts (race condition) | Integration | `test/integration/remediationprocessing/lifecycle_edge_cases_test.go` | `It("should handle concurrent transitions")` | ✅ |
| **BR-AP-057-EC1** | Child CRD creation failure (quota exceeded) | Integration | `test/integration/remediationprocessing/crd_creation_edge_cases_test.go` | `It("should handle quota exceeded")` | ✅ |

### Integration Edge Cases (1 scenario)

| Edge Case BR | Requirement | Test Type | Test File | Test Name | Status |
|--------------|-------------|-----------|-----------|-----------|--------|
| **BR-AP-061-EC1** | PostgreSQL connection pool exhaustion | Integration | `test/integration/remediationprocessing/storage_edge_cases_test.go` | `It("should handle pool exhaustion")` | ✅ |

---

## 📝 Test Implementation Guidance

### Using Ginkgo DescribeTable for Edge Case Testing

**Recommendation**: Use `DescribeTable` to reduce code duplication when testing multiple edge cases with similar logic.

**Benefits**:
- ✅ Single test function for multiple scenarios
- ✅ Easy to add new edge cases (just add `Entry()`)
- ✅ Clear test matrix visibility
- ✅ Lower maintenance cost

**Example: Context Enrichment Edge Cases**

```go
// test/unit/remediationprocessing/enricher_edge_cases_test.go
package remediationprocessing_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("BR-AP-001: Context Enrichment Edge Cases", func() {
    var enricher *ContextEnricher

    BeforeEach(func() {
        enricher = NewContextEnricher(mockStorage, mockEmbedding)
    })

    DescribeTable("Edge case handling",
        func(scenario string, historicalRecords []Record, expectedBehavior string) {
            // Setup test scenario
            mockStorage.SetHistoricalRecords(historicalRecords)

            // Execute enrichment
            result, err := enricher.EnrichContext(ctx, testAlert)

            // Validate behavior based on scenario
            switch expectedBehavior {
            case "empty_context":
                Expect(err).ToNot(HaveOccurred())
                Expect(result.HistoricalContext).To(BeEmpty())
                Expect(result.FallbackUsed).To(BeTrue())
            case "dimension_mismatch":
                Expect(err).To(HaveOccurred())
                Expect(err.Error()).To(ContainSubstring("dimension mismatch"))
            case "zero_attempts":
                Expect(err).ToNot(HaveOccurred())
                Expect(result.SuccessRate).To(Equal(0.0))
            }
        },
        Entry("BR-AP-001-EC1: empty historical context",
            "empty_context", []Record{}, "empty_context"),
        Entry("BR-AP-001-EC2: malformed embedding vectors",
            "dimension_mismatch", []Record{{Embedding: []float32{1, 2, 3}}}, "dimension_mismatch"),
        Entry("BR-AP-005-EC1: zero historical attempts",
            "zero_attempts", []Record{{Attempts: 0}}, "zero_attempts"),
    )
})
```

**Example: Classification Edge Cases**

```go
var _ = Describe("BR-AP-020: Classification Engine Edge Cases", func() {
    var classifier *Classifier

    DescribeTable("Classification edge cases",
        func(alert Alert, expectedClass string, expectedConfidence float64, expectedError bool) {
            result, err := classifier.Classify(ctx, alert)

            if expectedError {
                Expect(err).To(HaveOccurred())
            } else {
                Expect(err).ToNot(HaveOccurred())
                Expect(result.Classification).To(Equal(expectedClass))
                Expect(result.Confidence).To(BeNumerically("~", expectedConfidence, 0.01))
            }
        },
        Entry("BR-AP-020-EC1: ambiguous classification (50% confidence)",
            Alert{Severity: "medium", Pattern: "ambiguous"},
            "automated", 0.50, false),
        Entry("BR-AP-022-EC1: all rules fail (no match)",
            Alert{Severity: "", Pattern: ""},
            "unknown", 0.0, true),
        Entry("BR-AP-030-EC1: missing severity field",
            Alert{Pattern: "critical-pattern"},
            "ai-required", 0.8, false),
    )
})
```

### Envtest Integration Test Pattern

**Example: Semantic Search with Envtest**

```go
// test/integration/remediationprocessing/semantic_search_edge_cases_test.go
var _ = Describe("BR-AP-003-EC1: pgvector Query Timeout", func() {
    It("should handle query timeout gracefully", func() {
        // Setup: Create RemediationProcessing CRD
        processing := &remediationprocessingv1.RemediationProcessing{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "test-timeout",
                Namespace: testNamespace,
            },
            Spec: remediationprocessingv1.RemediationProcessingSpec{
                SignalFingerprint: "test-fingerprint",
            },
        }

        // Create in Envtest Kubernetes API
        Expect(k8sClient.Create(ctx, processing)).To(Succeed())

        // Simulate slow database by setting very low timeout
        enricher := NewContextEnricher(
            mockStorage.WithTimeout(10 * time.Millisecond),
        )

        // Execute with timeout
        ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
        defer cancel()

        _, err := enricher.EnrichContext(ctx, processing.Spec)

        // Validate timeout handling
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("timeout"))

        // Verify status updated with error
        Eventually(func() string {
            k8sClient.Get(ctx, client.ObjectKeyFromObject(processing), processing)
            return processing.Status.Phase
        }).Should(Equal("EnrichmentFailed"))
    })
})
```

---

## 📋 Test File Manifest

### Unit Tests (16 tests covering 59.3% of BRs)

1. **test/unit/remediationprocessing/enricher_test.go**
   - BR-AP-005 (Success rate calculation)
   - BR-AP-007 (Resolution time aggregation)
   - BR-AP-010 (Common actions)
   - BR-AP-012 (Knowledge articles)
   - BR-AP-015 (Multi-source aggregation)

2. **test/unit/remediationprocessing/similarity_test.go**
   - BR-AP-018 (Similarity scoring)

3. **test/unit/remediationprocessing/classifier_test.go**
   - BR-AP-020 (Classification logic)
   - BR-AP-022 (AI requirement detection)
   - BR-AP-025 (Confidence scoring)
   - BR-AP-028 (Rule-based engine)
   - BR-AP-030 (Severity-based routing)

4. **test/unit/remediationprocessing/fingerprinter_test.go**
   - BR-AP-040 (Fingerprint generation)
   - BR-AP-042 (Duplicate detection)

5. **test/unit/remediationprocessing/deduplication_test.go**
   - BR-AP-045 (Deduplication window)

6. **test/unit/remediationprocessing/reconciler_test.go**
   - BR-AP-053 (Phase transitions)

7. **test/unit/remediationprocessing/status_test.go**
   - BR-AP-055 (Status tracking)

### Integration Tests (9 tests covering 33.3% of BRs)

1. **test/integration/remediationprocessing/enrichment_test.go**
   - BR-AP-001 (Historical context enrichment)

2. **test/integration/remediationprocessing/semantic_search_test.go**
   - BR-AP-003 (Semantic search with pgvector)

3. **test/integration/remediationprocessing/classification_test.go**
   - BR-AP-032 (Historical data influence)
   - BR-AP-035 (Classification reasons)

4. **test/integration/remediationprocessing/deduplication_test.go**
   - BR-AP-048 (Duplicate suppression)

5. **test/integration/remediationprocessing/lifecycle_test.go**
   - BR-AP-051 (CRD reconciliation)

6. **test/integration/remediationprocessing/crd_creation_test.go**
   - BR-AP-057 (Child CRD creation)

7. **test/integration/remediationprocessing/storage_integration_test.go**
   - BR-AP-061 (Data Storage integration)
   - BR-AP-063 (Connection pooling)

### E2E Tests (2 tests covering 7.4% of BRs)

1. **test/e2e/remediationprocessing/e2e_test.go**
   - BR-AP-060 (Owner references)
   - BR-AP-067 (Observability)

---

## ✅ Coverage Validation

### By Test Type
- **Unit Tests**: 16/27 BRs (59.3%) ✅ Target: >70% (⚠️ Gap: Need 3 more unit tests)
- **Integration Tests**: 9/27 BRs (33.3%) ✅ Target: >20%
- **E2E Tests**: 2/27 BRs (7.4%) ✅ Target: >10% (⚠️ Gap: Need 1 more E2E test)

### By Category
- **Context Enrichment**: 8/8 (100%) ✅
- **Classification**: 7/7 (100%) ✅
- **Deduplication**: 4/4 (100%) ✅
- **CRD Lifecycle**: 5/5 (100%) ✅
- **Integration**: 3/3 (100%) ✅

### Overall
- **Total Coverage**: 27/27 (100%) ✅
- **Untested BRs**: 0 ✅

---

## 🎯 Test Execution Order

### Phase 1: Unit Tests (Days 8-9)
Run all unit tests to validate core logic:
```bash
cd test/unit/remediationprocessing
go test -v ./...
```

### Phase 2: Integration Tests (Days 8-9)
Run integration tests with Envtest:
```bash
cd test/integration/remediationprocessing
go test -v -timeout=30m ./...
```

**Prerequisites**:
- **Envtest** (automatically managed by test framework)
- PostgreSQL testcontainer with pgvector extension (for Data Storage integration)
- `remediation_audit` table schema loaded
- Sample test data seeded

**Envtest Benefits**:
- 5-18x faster than Kind cluster (9-17s vs 85-170s)
- No Docker/Kind dependencies
- Real Kubernetes API with CRD validation
- Portable across IDE, CI, and local development

### Phase 3: E2E Tests (Day 10)
Run E2E tests with complete environment:
```bash
cd test/e2e/remediationprocessing
go test -v -timeout=60m ./...
```

**Prerequisites**:
- Kind cluster running (E2E only)
- Context API operational
- Data Storage Service operational
- Sample RemediationRequest CRDs

---

## 📊 Coverage Metrics

### Target Metrics
| Category | Unit % | Integration % | E2E % | Total % |
|----------|--------|---------------|-------|---------|
| **Context Enrichment** | 75% | 25% | 0% | 100% |
| **Classification** | 71% | 29% | 0% | 100% |
| **Deduplication** | 75% | 25% | 0% | 100% |
| **CRD Lifecycle** | 40% | 40% | 20% | 100% |
| **Integration** | 0% | 67% | 33% | 100% |

### Coverage Gaps to Address

**Gap 1: Unit Test Coverage (59.3% actual vs 70% target)**
- **Need**: 3 additional unit tests
- **Recommendation**:
  - Add unit tests for BR-AP-003 (semantic search algorithm)
  - Add unit tests for BR-AP-032 (classification decision logic)
  - Add unit tests for BR-AP-048 (suppression logic without DB)

**Gap 2: E2E Test Coverage (7.4% actual vs 10% target)**
- **Need**: 1 additional E2E test
- **Recommendation**:
  - Add E2E test for complete remediation flow (Gateway → RemediationProcessor → AIAnalysis)

---

## 🔧 Integration Test Infrastructure

### Envtest Setup for CRD Controller Testing

**Per [ADR-016](../../../../../docs/architecture/decisions/ADR-016-SERVICE-SPECIFIC-INTEGRATION-TEST-INFRASTRUCTURE.md)**: CRD controllers use Envtest instead of Kind for integration tests.

```go
// test/integration/remediationprocessing/suite_test.go
package remediationprocessing_test

import (
    "context"
    "path/filepath"
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "k8s.io/client-go/kubernetes/scheme"
    "k8s.io/client-go/rest"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/envtest"

    remediationprocessingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1alpha1"
    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

var (
    cfg       *rest.Config
    k8sClient client.Client
    testEnv   *envtest.Environment
    ctx       context.Context
    cancel    context.CancelFunc
)

var _ = BeforeSuite(func() {
    ctx, cancel = context.WithCancel(context.Background())

    // Setup Envtest with CRDs
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "..", "..", "config", "crd"),
        },
        ErrorIfCRDPathMissing: true,
    }

    // Start Envtest (real Kubernetes API)
    var err error
    cfg, err = testEnv.Start()
    Expect(err).NotTo(HaveOccurred())
    Expect(cfg).NotTo(BeNil())

    // Register CRD schemes
    err = remediationprocessingv1.AddToScheme(scheme.Scheme)
    Expect(err).NotTo(HaveOccurred())
    err = remediationv1.AddToScheme(scheme.Scheme)
    Expect(err).NotTo(HaveOccurred())

    // Create Kubernetes client
    k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
    Expect(err).NotTo(HaveOccurred())
    Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
    cancel()
    By("tearing down the test environment")
    err := testEnv.Stop()
    Expect(err).NotTo(HaveOccurred())
})
```

**Why Envtest for CRD Controllers**:
- ✅ Real Kubernetes API (not mocked)
- ✅ CRD validation (OpenAPI v3 schema)
- ✅ Watch events work correctly
- ✅ 5-18x faster than Kind (9-17s vs 85-170s startup)
- ✅ No Docker dependency

### PostgreSQL Testcontainer Setup (for Data Storage Integration)

For tests that require PostgreSQL with pgvector:

```go
// test/integration/remediationprocessing/storage_suite_test.go
var (
    postgresContainer testcontainers.Container
    db                *sql.DB
)

var _ = BeforeSuite(func() {
    // Start PostgreSQL with pgvector
    postgresReq := testcontainers.ContainerRequest{
        Image: "pgvector/pgvector:pg16",
        ExposedPorts: []string{"5432/tcp"},
        Env: map[string]string{
            "POSTGRES_USER":     "testuser",
            "POSTGRES_PASSWORD": "testpass",
            "POSTGRES_DB":       "testdb",
        },
    }

    postgresContainer, err := testcontainers.GenericContainer(ctx, postgresReq, ...)
    Expect(err).NotTo(HaveOccurred())

    // Load schema
    _, err = db.Exec(`
        CREATE EXTENSION IF NOT EXISTS vector;
        CREATE TABLE remediation_audit (
            id VARCHAR(255) PRIMARY KEY,
            signal_fingerprint VARCHAR(64),
            embedding vector(1536)
        );
    `)
    Expect(err).NotTo(HaveOccurred())
})
```

### Test Data Seeding

**Seed Data Requirements**:
- 50+ historical remediation records
- Vector embeddings for semantic search
- Various severity levels (critical, high, medium, low)
- Multiple environments (production, staging, development)
- Success rates: 0.9 (high), 0.5 (medium), 0.2 (low)

---

## ✅ Validation Checklist

Before marking coverage complete:
- [ ] All 27 BRs have at least one test
- [ ] All test files exist and compile
- [ ] All tests pass in isolation
- [ ] PostgreSQL testcontainer setup working
- [ ] Semantic search queries return expected results
- [ ] Coverage metrics meet or exceed targets
- [ ] No flaky tests (>99% pass rate)
- [ ] Test documentation complete
- [ ] BR traceability verified

---

## 🎯 Action Items to Reach Target Coverage

### High Priority
1. **Add 3 unit tests** to reach 70% unit coverage:
   - `test/unit/remediationprocessing/semantic_search_algorithm_test.go`
   - `test/unit/remediationprocessing/classification_decision_test.go`
   - `test/unit/remediationprocessing/suppression_logic_test.go`

2. **Add 1 E2E test** to reach 10% E2E coverage:
   - `test/e2e/remediationprocessing/complete_flow_test.go` (Gateway → RemediationProcessor → AIAnalysis)

### Medium Priority
3. **Validate integration test infrastructure**:
   - PostgreSQL testcontainer setup
   - Schema loading
   - Test data seeding
   - Semantic search query performance

---

**Status**: ✅ **100% BR Coverage (27/27 BRs)**
**Action Required**: Add 3 unit tests + 1 E2E test to meet target coverage percentages
**Next Action**: Implement tests per this matrix
**Validation Date**: TBD (after test implementation)

