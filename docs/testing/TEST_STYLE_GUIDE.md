# Test Style Guide for Kubernaut

**Version**: 1.0
**Last Updated**: 2025-10-14
**Status**: Approved - Mandatory for all tests

---

## Purpose

This guide provides mandatory test naming conventions, assertion patterns, and structural guidelines to ensure consistency, maintainability, and clarity across all Kubernaut tests.

---

## Table of Contents

1. [Test Naming Conventions](#test-naming-conventions)
2. [Business Requirement Mapping](#business-requirement-mapping)
3. [Assertion Style Guidelines](#assertion-style-guidelines)
4. [Test Structure Patterns](#test-structure-patterns)
5. [Anti-Patterns to Avoid](#anti-patterns-to-avoid)
6. [File Organization](#file-organization)
7. [Documentation Standards](#documentation-standards)

---

## 1. Test Naming Conventions

### Test File Naming

**Pattern**: `<component>_test.go`

```
✅ CORRECT:
- enricher_test.go
- classifier_test.go
- workflow_orchestrator_test.go

❌ INCORRECT:
- test_enricher.go (prefix instead of suffix)
- enricher-test.go (hyphen instead of underscore)
- enricher_tests.go (plural)
```

###Test Package Naming

**Pattern**: Same package as source code (white-box testing)

```go
✅ CORRECT:
package remediationprocessing

func TestEnricher(t *testing.T) { ... }

❌ INCORRECT:
package remediationprocessing_test  // DO NOT use _test postfix
```

### Test Suite Naming (Ginkgo)

**Pattern**: `Describe("BR-[CATEGORY]-[NUMBER]: [Business Requirement Description]", ...)`

```go
✅ CORRECT:
Describe("BR-SP-001: Historical Alert Enrichment", func() {
    // All tests in this block validate BR-SP-001
})

❌ INCORRECT:
Describe("Enricher", func() {  // No BR mapping
Describe("Test enrichment", func() {  // Unclear
Describe("BR-001", func() {  // Missing category
```

### Individual Test Naming (Ginkgo It blocks)

**Pattern**: `It("should [business outcome] when [condition]", ...)`

**Components**:
- Always start with "should"
- Describe business behavior, not implementation
- Include the "when" condition for clarity
- Use present tense
- Be specific and descriptive

```go
✅ CORRECT:
It("should classify as AIRequired when success rate below 80%", func() {})
It("should enrich alert with historical data when matches exist", func() {})
It("should create child CRDs when workflow has multiple steps", func() {})
It("should execute Job successfully when RBAC permissions valid", func() {})

❌ INCORRECT:
It("classifier works", func() {})  // Too vague
It("when success rate is low", func() {})  // Missing "should"
It("tests the enricher", func() {})  // Focus on testing, not behavior
It("calls the database", func() {})  // Implementation detail
```

### Context Block Naming

**Pattern**: Describe logical groupings of related scenarios

```go
✅ CORRECT:
Context("when historical data exists", func() {
    It("should enrich with high similarity matches", func() {})
    It("should enrich with low similarity matches", func() {})
})

Context("when no historical data exists", func() {
    It("should classify as AIRequired", func() {})
})

Context("environment-based classification", func() {
    It("should use stricter threshold in production", func() {})
    It("should use relaxed threshold in staging", func() {})
})

❌ INCORRECT:
Context("test cases", func() {})  // Too generic
Context("database", func() {})  // Implementation detail
Context("Case 1", func() {})  // Unclear
```

---

## 2. Business Requirement Mapping

### Mandatory BR Reference

**EVERY test must map to a specific business requirement**

**Format**: `BR-[CATEGORY]-[NUMBER]`

**Categories**: WORKFLOW, AI, INTEGRATION, SECURITY, PLATFORM, API, STORAGE, MONITORING, SAFETY, PERFORMANCE

```go
✅ CORRECT:
Describe("BR-SP-001: Historical Alert Enrichment", func() {
    // Test implements business requirement BR-SP-001
})

Describe("BR-WF-023: Parallel Step Execution", func() {
    // Test implements business requirement BR-WF-023
})

❌ INCORRECT:
Describe("Enrichment Tests", func() {})  // No BR reference
Describe("BR-1: Tests", func() {})  // Missing category
```

### BR Comment Headers

Add BR reference in comments for complex tests:

```go
// BR-SP-005: Classification Logic
// Tests automated vs AI-required classification based on
// historical success rates and environmental factors
var _ = Describe("BR-SP-005: Classification Logic", func() {
    ...
})
```

---

## 3. Assertion Style Guidelines

### Use Business-Meaningful Assertions

**Principle**: Assert on business outcomes, not implementation details

```go
✅ CORRECT:
Expect(classification.Type).To(Equal("AIRequired"))
Expect(workflow.Status.Phase).To(Equal("Completed"))
Expect(enrichment.SimilarityScore).To(BeNumerically(">", 0.8))

❌ INCORRECT (NULL-TESTING):
Expect(classification).ToNot(BeNil())  // Weak assertion
Expect(len(results)).To(BeNumerically(">", 0))  // Generic
Expect(error).To(BeNil())  // Use NotTo(HaveOccurred()) instead
```

### Specific Error Assertions

```go
✅ CORRECT:
Expect(err).ToNot(HaveOccurred())
Expect(err).To(MatchError(ContainSubstring("policy violation")))
Expect(err).To(MatchError(remediationprocessing.ErrNoHistoricalData))

❌ INCORRECT:
Expect(err).To(BeNil())  // Use NotTo(HaveOccurred())
Expect(err != nil).To(BeTrue())  // Don't use boolean expressions
```

### Gomega Matcher Patterns

```go
// Equality
Expect(actual).To(Equal(expected))

// Numeric comparisons
Expect(score).To(BeNumerically(">", 0.8))
Expect(score).To(BeNumerically(">=", 0.0))
Expect(score).To(BeNumerically("<=", 1.0))

// String matching
Expect(message).To(ContainSubstring("success"))
Expect(phase).To(MatchRegexp("^(Pending|Running|Completed)$"))

// Collection matching
Expect(items).To(HaveLen(3))
Expect(items).To(ContainElement("item1"))
Expect(items).ToNot(BeEmpty())

// Boolean conditions
Expect(condition).To(BeTrue())
Expect(condition).To(BeFalse())

// Time-based assertions
Expect(timestamp).To(BeTemporally("~", time.Now(), time.Second))
Expect(timestamp).To(BeTemporally(">", startTime))

// Error assertions
Expect(err).ToNot(HaveOccurred())
Expect(err).To(HaveOccurred())
Expect(err).To(MatchError("specific error message"))

// Asynchronous assertions
Eventually(func() string {
    return resource.Status.Phase
}).Should(Equal("Completed"))

Eventually(func() error {
    return checkCondition()
}, 5*time.Second, 100*time.Millisecond).Should(Succeed())
```

---

## 4. Test Structure Patterns

### Unit Test Structure

```go
package remediationprocessing

import (
    "context"
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/remediationprocessing"
    "github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

func TestRemediationProcessing(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Remediation Processing Suite")
}

var _ = Describe("BR-SP-001: Historical Alert Enrichment", func() {
    var (
        // Mock external dependencies
        mockDB      *mocks.MockDatabase
        mockLLM     *mocks.MockLLMClient

        // Real business components
        enricher    *remediationprocessing.Enricher
        ctx         context.Context
    )

    BeforeEach(func() {
        // Setup
        mockDB = mocks.NewMockDatabase()
        mockLLM = mocks.NewMockLLMClient()
        enricher = remediationprocessing.NewEnricher(mockDB, mockLLM)
        ctx = context.Background()
    })

    Context("when historical data exists", func() {
        BeforeEach(func() {
            // Context-specific setup
            mockDB.SetHistoricalData(testData)
        })

        It("should enrich alert with high similarity matches", func() {
            // Arrange
            alert := createTestAlert("deployment-failure")

            // Act
            result, err := enricher.Enrich(ctx, alert)

            // Assert
            Expect(err).ToNot(HaveOccurred())
            Expect(result.SimilarityScore).To(BeNumerically(">", 0.9))
            Expect(result.HistoricalMatches).To(HaveLen(3))
        })
    })

    AfterEach(func() {
        // Cleanup if needed
    })
})
```

### Integration Test Structure (Envtest)

```go
//go:build integration

package remediationprocessing

import (
    "context"
    "path/filepath"
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
    testEnv   *envtest.Environment
    k8sClient client.Client
    ctx       context.Context
)

func TestRemediationProcessingIntegration(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Remediation Processing Integration Suite")
}

var _ = BeforeSuite(func() {
    ctx = context.Background()

    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "..", "api", "remediationprocessing", "v1alpha1"),
        },
    }

    cfg, err := testEnv.Start()
    Expect(err).NotTo(HaveOccurred())

    k8sClient, err = client.New(cfg, client.Options{})
    Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
    Expect(testEnv.Stop()).To(Succeed())
})

var _ = Describe("BR-SP-012: CRD Lifecycle Management", func() {
    It("should create and update SignalProcessing CRD", func() {
        // Integration test with real Kubernetes API
    })
})
```

### Table-Driven Tests (DescribeTable)

Use for testing multiple similar scenarios:

```go
DescribeTable("environment-based classification thresholds",
    func(namespace string, labels map[string]string, expectedEnv string, expectedThreshold float64) {
        alert := createAlert(namespace, labels)
        classification := classifier.Classify(alert)

        Expect(classification.Environment).To(Equal(expectedEnv))
        Expect(classification.Threshold).To(BeNumerically("==", expectedThreshold))
    },
    Entry("production namespace → production env, 0.90 threshold",
        "prod-webapp", map[string]string{}, "production", 0.90),
    Entry("staging namespace → staging env, 0.70 threshold",
        "staging-api", map[string]string{}, "staging", 0.70),
    Entry("dev namespace → dev env, 0.60 threshold",
        "dev-test", map[string]string{}, "dev", 0.60),
    Entry("explicit environment label overrides namespace",
        "unknown", map[string]string{"environment": "production"}, "production", 0.90),
)
```

---

## 5. Anti-Patterns to Avoid

### ❌ Null-Testing

```go
❌ AVOID:
Expect(result).ToNot(BeNil())  // Weak assertion
Expect(len(items)).To(BeNumerically(">", 0))  // Generic

✅ PREFER:
Expect(result.Classification).To(Equal("AIRequired"))  // Specific
Expect(items).To(ContainElement(expectedItem))  // Meaningful
```

### ❌ Implementation Testing

```go
❌ AVOID (tests how, not what):
It("should call database.Query", func() {})
It("should invoke LLM API", func() {})
It("should use topological sort", func() {})

✅ PREFER (tests business outcome):
It("should enrich with historical data", func() {})
It("should classify based on success rate", func() {})
It("should execute steps in dependency order", func() {})
```

### ❌ Magic Numbers

```go
❌ AVOID:
Expect(score).To(BeNumerically(">", 0.8))  // Why 0.8?

✅ PREFER:
const HighConfidenceThreshold = 0.8
Expect(score).To(BeNumerically(">", HighConfidenceThreshold))
```

### ❌ Long Test Names

```go
❌ AVOID:
It("should classify the remediation request as requiring AI when the historical success rate is below 80% and the environment is production and the severity is critical", func() {})

✅ PREFER:
Context("production environment", func() {
    Context("critical severity", func() {
        It("should classify as AIRequired when success rate below 80%", func() {})
    })
})
```

---

## 6. File Organization

### Directory Structure

```
test/
├── unit/
│   ├── remediationprocessing/
│   │   ├── enricher_test.go
│   │   ├── classifier_test.go
│   │   └── suite_test.go
│   ├── workflowexecution/
│   │   └── orchestrator_test.go
│   └── kubernetesexecution/  # DEPRECATED - ADR-025
│       └── executor_test.go
├── integration/
│   ├── remediationprocessing/
│   │   ├── lifecycle_test.go
│   │   ├── enrichment_test.go
│   │   └── suite_test.go
│   ├── workflowexecution/
│   │   └── coordination_test.go
│   └── kubernetesexecution/  # DEPRECATED - ADR-025
│       └── job_execution_test.go
└── e2e/
    ├── complete_remediation_flow_test.go
    └── multi_step_workflow_test.go
```

### Suite File Pattern

Each test directory should have a `suite_test.go`:

```go
package remediationprocessing

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestRemediationProcessing(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Remediation Processing Suite")
}
```

---

## 7. Documentation Standards

### Test Comments

Add comments for:
- Complex setup logic
- Non-obvious business rules
- BR-specific behavior

```go
// BR-SP-005: Classification uses environment-specific thresholds
// Production requires 90% success rate, staging 70%
var _ = Describe("BR-SP-005: Classification Logic", func() {
    It("should use 90% threshold in production", func() {
        // Production environments require higher confidence
        // to minimize risk of incorrect automated remediation
        alert := createProductionAlert()
        classification := classifier.Classify(alert)
        Expect(classification.RequiredSuccessRate).To(Equal(0.90))
    })
})
```

### Test Documentation Headers

For complex integration tests:

```go
// Integration Test: BR-WF-023 Parallel Step Execution
//
// This test validates that WorkflowExecution correctly:
// 1. Resolves step dependencies using topological sort
// 2. Creates KubernetesExecution (DEPRECATED - ADR-025) CRDs for parallel steps
// 3. Respects concurrency limits (max 5 concurrent steps)
// 4. Coordinates execution through watch-based status updates
//
// Infrastructure: Envtest (CRD API only)
// External Dependencies: None
// Expected Duration: <5 seconds
var _ = Describe("BR-WF-023: Parallel Step Execution", func() {
    ...
})
```

---

## Quick Reference Card

### Test Naming Checklist

- [ ] File named `<component>_test.go`
- [ ] Package name matches source (no `_test` postfix)
- [ ] Describe block includes BR reference: `BR-[CAT]-[NUM]`
- [ ] It block starts with "should"
- [ ] It block describes business outcome
- [ ] It block includes "when" condition

### Assertion Checklist

- [ ] Assert on business outcomes, not implementation
- [ ] Use specific matchers, avoid null-testing
- [ ] Use `NotTo(HaveOccurred())` for errors
- [ ] Include meaningful failure messages
- [ ] Use `Eventually` for asynchronous conditions

### Structure Checklist

- [ ] Mock only external dependencies
- [ ] Use real business logic components
- [ ] Setup in `BeforeEach`, cleanup in `AfterEach`
- [ ] Group related tests in Context blocks
- [ ] Use DescribeTable for similar scenarios

---

**Version**: 1.0
**Last Updated**: 2025-10-14
**Compliance**: Mandatory for all new tests
**Review Cycle**: Quarterly

