# Test Utilities Package

This package provides standardized test utilities to reduce redundancy and improve maintainability across all unit tests in the `pkg/` directory.

## 🎯 Goals

- **Reduce test code by 60-70%** through standardization
- **Eliminate duplicate mock implementations**
- **Provide consistent test data patterns**
- **Standardize common assertion patterns**
- **Maintain 100% test coverage and effectiveness**

## 📦 Components

### 1. TestSuiteBuilder (`test_suite_builder.go`)
Provides fluent interface for setting up test suites with common components.

```go
// Standard unit test (logger only)
components := testutil.StandardUnitTestSuite("My Tests")

// Test with K8s client
components := testutil.K8sUnitTestSuite("K8s Tests")

// Custom test setup
components := testutil.NewTestSuiteBuilder("Custom Tests").
    WithK8sClient().
    WithLogLevel(logrus.InfoLevel).
    WithCustomSetup(func() error {
        // Custom setup logic
        return nil
    }).
    Build()
```

### 2. MockFactory (`mock_factory.go`)
Centralized factory for all mock objects to eliminate duplicates.

```go
factory := testutil.NewMockFactory(logger)

// Standard mocks
mockSLMClient := factory.NewStandardMockSLMClient()
mockProcessor := factory.NewStandardMockAIResponseProcessor()
mockRepo := factory.NewStandardMockRepository()
mockVectorDB := factory.NewStandardMockVectorDatabase()
mockKB := factory.NewStandardMockKnowledgeBase()
```

### 3. TestDataFactory (`test_data_factory.go`)
Consistent test data creation patterns.

```go
dataFactory := testutil.NewTestDataFactory()

// Alerts
alert := dataFactory.CreateHighMemoryAlert()
customAlert := dataFactory.CreateCustomAlert("MyAlert", "critical", "prod", "my-app")

// Recommendations
recommendation := dataFactory.CreateStandardActionRecommendation()
restartRec := dataFactory.CreateRestartPodRecommendation()

// Configurations
llmConfig := dataFactory.CreateStandardLLMConfig()
actionsConfig := dataFactory.CreateStandardActionsConfig()

// Vector patterns
pattern := dataFactory.CreateStandardActionPattern()
patterns := dataFactory.CreateTestActionPatterns()
```

### 4. CommonAssertions (`common_assertions.go`)
Reusable assertion patterns for consistent validation.

```go
assertions := testutil.NewCommonAssertions()

// Action recommendation assertions
assertions.AssertValidActionRecommendation(recommendation)
assertions.AssertActionRecommendationWithConfidence(recommendation, 0.8)
assertions.AssertActionRecommendationHasReasoning(recommendation)

// Enhanced recommendation assertions
assertions.AssertValidEnhancedRecommendation(enhanced)
assertions.AssertEnhancedRecommendationHasValidation(enhanced)
assertions.AssertEnhancedRecommendationHasRiskAssessment(enhanced)

// Alert assertions
assertions.AssertValidAlert(alert)
assertions.AssertAlertSeverity(alert, "critical")
assertions.AssertAlertHasLabels(alert, "app", "pod")

// Component health assertions
assertions.AssertComponentHealthy(client)
assertions.AssertComponentUnhealthy(failingClient)
```

## 🔄 Migration Patterns

### Before (Duplicated Code)
```go
var _ = Describe("My Component", func() {
    var (
        ctx       context.Context
        logger    *logrus.Logger
        testEnv   *testenv.TestEnvironment
        k8sClient k8s.Client
        mockSLM   *MockSLMClient
    )

    BeforeEach(func() {
        var err error
        ctx = context.Background()

        logger = logrus.New()
        logger.SetLevel(logrus.FatalLevel)

        testEnv, err = testenv.SetupEnvironment()
        Expect(err).NotTo(HaveOccurred())

        err = testEnv.CreateDefaultNamespace()
        Expect(err).NotTo(HaveOccurred())

        k8sClient = testEnv.CreateK8sClient(logger)
        mockSLM = NewMockSLMClient() // Duplicate implementation
    })

    AfterEach(func() {
        if testEnv != nil {
            err := testEnv.Cleanup()
            Expect(err).NotTo(HaveOccurred())
        }
    })

    It("should work", func() {
        // Manual alert creation
        alert := types.Alert{
            Name: "TestAlert",
            Severity: "warning",
            // ... 20 more lines
        }

        // Manual assertions
        Expect(result).NotTo(BeNil())
        Expect(result.Action).NotTo(BeEmpty())
        Expect(result.Confidence).To(BeNumerically(">=", 0))
        // ... 10 more lines
    })
})
```

### After (Standardized Code)
```go
var _ = Describe("My Component", func() {
    var (
        components  *testutil.TestSuiteComponents
        factory     *testutil.MockFactory
        dataFactory *testutil.TestDataFactory
        assertions  *testutil.CommonAssertions
        mockSLM     *testutil.StandardMockSLMClient
    )

    BeforeEach(func() {
        components = testutil.K8sUnitTestSuite("My Component Tests")
        factory = testutil.NewMockFactory(components.Logger)
        dataFactory = testutil.NewTestDataFactory()
        assertions = testutil.NewCommonAssertions()

        mockSLM = factory.NewStandardMockSLMClient()
    })

    It("should work", func() {
        alert := dataFactory.CreateHighMemoryAlert()

        // Test logic here

        assertions.AssertValidActionRecommendation(result)
    })
})
```

## 📊 Expected Impact

| Metric | Before | After | Improvement |
|--------|---------|--------|-------------|
| **BeforeEach/AfterEach Setup** | 40+ duplicate patterns | 1 standardized builder | ~97% reduction |
| **Mock Implementations** | 15+ variants | 5 standardized mocks | ~65% reduction |
| **Test Data Creation** | 20+ scattered patterns | 1 centralized factory | ~70% reduction |
| **Common Assertions** | 100+ repeated patterns | Reusable helpers | ~80% reduction |
| **Overall Unit Test Code** | ~3000+ lines repetitive | ~600 lines utilities | **~80% reduction** |

## 🚀 Implementation Plan

### Phase 1: Create Utilities (✅ Complete)
- [x] TestSuiteBuilder
- [x] MockFactory
- [x] TestDataFactory
- [x] CommonAssertions

### Phase 2: Migrate High-Impact Files
Priority order based on redundancy:
1. AI component tests (`pkg/ai/*_test.go`)
2. Platform tests (`pkg/platform/*_test.go`)
3. Storage tests (`pkg/storage/*_test.go`)
4. Workflow tests (`pkg/workflow/*_test.go`)
5. Remaining tests

### Phase 3: Validation
- Ensure all tests pass
- Verify coverage maintained
- Performance check
- Code review

## 🔧 Usage Guidelines

1. **Always use TestSuiteBuilder** for test setup instead of manual BeforeEach/AfterEach
2. **Use MockFactory** instead of creating custom mocks
3. **Use TestDataFactory** for all test data creation
4. **Use CommonAssertions** for standard validations
5. **Only create custom utilities** if the standard ones don't fit your specific needs

## 🧪 Testing the Utilities

The utilities themselves should be tested to ensure reliability:

```bash
# Test the utilities
go test ./pkg/testutil/...

# Use utilities in existing tests
go test ./pkg/ai/llm/...
go test ./pkg/platform/executor/...
```

This standardization will significantly reduce maintenance burden while maintaining full test coverage and effectiveness.
