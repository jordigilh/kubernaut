# Testing Framework Documentation

## Overview

The Kubernaut project uses a **hybrid testing strategy** with a BDD-style testing framework built on **Ginkgo v2** and **Gomega**. This framework provides comprehensive test coverage from unit tests to production E2E validation.

## 🎯 **Hybrid Testing Strategy**

**Strategy**: Kind for CI/CD and local testing, Kubernetes cluster for E2E tests

- **🏗️ Kind Cluster**: Fast iteration for development and CI/CD
- **🏢 Kubernetes**: Production-like E2E validation
- **🤖 Configurable LLM**: Real model locally, mocked in CI/CD
- **🗃️ Real Databases**: PostgreSQL + Vector DB for all scenarios

📋 **Full Strategy Guide**: [HYBRID_TESTING_STRATEGY.md](development/HYBRID_TESTING_STRATEGY.md)

## Framework Migration

### Completed Migration from Testify
- **Complete elimination** of all `github.com/stretchr/testify` dependencies
- **Full migration** to Ginkgo/Gomega testing framework
- **Modular test organization** with focused test files
- **BDD-style specifications** using Describe/Context/It patterns

### Benefits of Ginkgo/Gomega
- **Organization**: Hierarchical test structure with Describe/Context blocks
- **Assertions**: Gomega's Expect syntax for assertions
- **Focused Testing**: Individual test files for specific functionality areas
- **Reporting**: Test output with success/failure reporting
- **Parallel Execution**: Support for parallel test execution

## Test Organization

### Core Package Tests
- **`pkg/processor/processor_test.go`**: Alert processing and filtering logic tests
- **`pkg/executor/executor_test.go`**: Kubernetes action execution tests

### Integration Tests (Modular Structure)

All integration tests are organized by functional area in the `test/integration/` directory:

#### Action-Specific Test Files
1. **`storage_actions_ginkgo_test.go`**
   - Disk space critical scenarios
   - Database corruption handling
   - Storage remediation actions

2. **`security_actions_ginkgo_test.go`**
   - Security vulnerability detection
   - Compliance violation handling
   - Security-focused remediation actions

3. **`application_lifecycle_actions_ginkgo_test.go`**
   - Deployment failure scenarios
   - Application lifecycle management

4. **`network_actions_ginkgo_test.go`**
   - Network connectivity issues
   - Network-related remediation actions

5. **`database_actions_ginkgo_test.go`**
   - Database performance issues
   - Database-specific remediation actions

6. **`monitoring_actions_ginkgo_test.go`**
   - Monitoring system failures
   - Observability-related actions

7. **`resource_management_actions_ginkgo_test.go`**
   - Resource quota scenarios
   - Resource optimization actions

8. **`action_validation_ginkgo_test.go`**
   - Cross-functional action validation
   - Generic action testing scenarios

#### Shared Test Infrastructure
- **`new_actions_suite_test.go`**: Shared test setup with BeforeSuite/AfterSuite lifecycle management
- **`ollama_integration_ginkgo_test.go`**: SLM model connectivity and analysis tests
- **`slm_mcp_context_ginkgo_test.go`**: Context Provider integration and context-aware testing

## Test Structure Patterns

### Basic Test Structure
```go
var _ = Describe("Feature Name", func() {
    var (
        // Test variables
        client     slm.Client
        testConfig IntegrationConfig
    )

    BeforeEach(func() {
        // Setup for each test
        report.TotalTests++
    })

    Context("when specific condition", func() {
        It("should perform expected behavior", func() {
            // Test implementation
            Expect(result).To(Equal(expected))
        })
    })
})
```

### Shared Suite Setup
```go
var _ = BeforeSuite(func() {
    // Global test setup
    testConfig = LoadConfig()
    client, err = slm.NewClient(slmConfig, logger)
    Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
    // Global test cleanup
    if testEnv != nil {
        err := testEnv.Cleanup()
        Expect(err).ToNot(HaveOccurred())
    }
})
```

## Running Tests

### 🚀 **Quick Reference (New Hybrid Strategy)**

```bash
# Unit Tests
make test

# Integration Tests (Kind + Real DB + Local LLM)
make test-integration-kind

# CI/CD Tests (Kind + Real DB + Mock LLM)
make test-ci

# E2E Tests (Kubernetes + Real Everything)
make test-e2e-ocp

# All Tests
make test-all
```

### Unit Tests
```bash
# Run all unit tests (auto-discovery)
make test

# Run specific package tests
go test ./pkg/processor/
go test ./pkg/executor/
```

### Integration Tests (Hybrid Strategy)

#### **Primary Commands (Recommended)**
```bash
# Local development with Kind cluster
make test-integration-kind

# CI/CD pipeline testing
make test-integration-kind-ci

# Quick integration tests
make test-integration-quick
```

#### **Legacy Commands (Deprecated)**
```bash
# ⚠️ DEPRECATED: Use test-integration-kind instead
make test-integration-fake-k8s
make test-integration-ollama
```

#### **Manual Integration Testing**
```bash
# Start services manually
make integration-services-start

# Run specific test suites
go test -tags=integration ./test/integration/ai/... -v
go test -tags=integration ./test/integration/infrastructure/... -v

# Stop services
make integration-services-stop
```

### E2E Tests (Kubernetes Strategy)

#### **Production E2E Testing**
```bash
# Full Kubernetes E2E validation
make test-e2e-ocp

# Specific E2E scenarios
make test-e2e-use-cases
make test-e2e-chaos
make test-e2e-stress
```

#### **Alternative E2E (Kind-based)**
```bash
# Limited E2E with Kind (for development)
make test-e2e-kind
make test-e2e-monitoring
```

### Test Configuration

Tests are automatically configured based on environment:

#### **Automatic Configuration**
- **Local Development**: Real LLM at `localhost:8080`, Real K8s via Kind
- **CI/CD Pipeline**: Mock LLM, Real K8s via Kind, Real databases
- **E2E Testing**: Real LLM, Real Kubernetes cluster

#### **Environment Variables**
```bash
# LLM Configuration
LLM_ENDPOINT=http://192.168.1.169:8080 # LLM server endpoint (new production endpoint)
LLM_MODEL=ggml-org/gpt-oss-20b-GGUF   # Model to use for testing
LLM_PROVIDER=ramalama                  # Provider type (ramalama, ollama, localai, mock)
USE_MOCK_LLM=false                     # Force mock LLM usage

# Kubernetes Configuration
USE_FAKE_K8S_CLIENT=false              # Use fake K8s client (default: real Kind cluster)
KUBECONFIG=~/.kube/config              # Kubernetes configuration

# Database Configuration
USE_CONTAINER_DB=true                  # Use containerized databases (default: true)
DB_HOST=localhost                      # Database host
DB_PORT=5433                           # Database port
SKIP_DB_TESTS=false                    # Skip database tests

# Test Control
SKIP_SLOW_TESTS=false                  # Skip performance and load tests
SKIP_INTEGRATION=false                 # Skip all integration tests
CI=false                               # CI/CD mode (auto-detected)
LOG_LEVEL=debug                        # Logging level for tests
TEST_TIMEOUT=120s                      # Test timeout duration
```

#### **Quick Configuration Examples**
```bash
# Force CI mode with mocked LLM
export CI=true USE_MOCK_LLM=true

# Use Ollama instead of local AI
export LLM_ENDPOINT=http://192.168.1.169:8080 LLM_PROVIDER=ramalama

# Skip slow tests for rapid iteration
export SKIP_SLOW_TESTS=true
```

## Test Assertions

### Ginkgo/Gomega Assertion Examples

```go
// Success assertions
Expect(err).ToNot(HaveOccurred())
Expect(recommendation).ToNot(BeNil())
Expect(recommendation.Action).ToNot(BeEmpty())

// Value assertions
Expect(recommendation.Confidence).To(BeNumerically(">=", 0.8))
Expect(types.IsValidAction(recommendation.Action)).To(BeTrue())

// Collection assertions
Expect([]string{"action1", "action2"}).To(ContainElement(recommendation.Action))

// Time-based assertions
Eventually(func() bool {
    return client.IsHealthy()
}).Should(BeTrue())
```

## Testing Best Practices

### Test Organization
- **One concern per test file**: Each test file focuses on a specific functional area
- **Test descriptions**: Use descriptive names in Describe/Context/It blocks
- **Shared setup**: Use BeforeSuite/AfterSuite for expensive setup operations
- **Isolated tests**: Each test should be independent and not rely on others

### Assertion Guidelines
- **Specific assertions**: Use precise Gomega matchers for failure messages
- **Error handling**: Always check for errors with `Expect(err).ToNot(HaveOccurred())`
- **Meaningful messages**: Add custom failure messages for complex assertions
- **Resource cleanup**: Ensure proper cleanup in AfterEach/AfterSuite

### Integration Test Guidelines
- **Real dependencies**: Use actual LLM models (Ollama/ramalama) for realistic testing
- **Timeout handling**: Set appropriate timeouts for model inference calls
- **Resource monitoring**: Track memory usage and response times
- **Environment validation**: Check required services are available before testing

## Mock Strategy Assessment

### GoMock vs. Manual Fake Implementations - Confidence Assessment

**Overall Migration Confidence: 6.5/10 (Moderate-High with Caveats)**

#### Current State Analysis

Our codebase currently employs sophisticated **manual fake implementations** rather than formal mocking frameworks like GoMock. This analysis evaluates the feasibility and benefits of migrating to GoMock.

**Current Mock Complexity:**
- **843 lines** in `fake_slm_client.go` alone
- **66 FakeSLMClient methods** (for 3 interface methods!)
- **Sophisticated features:** Error injection, circuit breakers, network simulation, call recording
- **23 test files** using fake implementations

**Target Interface Simplicity:**
```go
type Client interface {
    AnalyzeAlert(ctx context.Context, alert types.Alert) (*types.ActionRecommendation, error)
    ChatCompletion(ctx context.Context, prompt string) (string, error)
    IsHealthy() bool
}
```

#### Migration Confidence by Test Type

| Test Type | Current Approach | GoMock Confidence | Recommendation |
|-----------|------------------|-------------------|----------------|
| **Unit Tests** | Manual fakes | **9/10** | ✅ Migrate to GoMock |
| **Integration Tests** | Sophisticated fakes | **4/10** | ❌ Keep current approach |
| **Error Scenarios** | Rich error injection | **3/10** | ❌ Keep current approach |
| **Simple Mocking** | Over-engineered | **9/10** | ✅ Migrate to GoMock |

#### High Confidence Areas (8-9/10)

**Simple Interface Mocking:**
```go
// GoMock would handle this beautifully
mockSLM.EXPECT().AnalyzeAlert(gomock.Any(), gomock.Any()).
    Return(&types.ActionRecommendation{Action: "restart"}, nil).
    Times(1)
```

**Call Verification Standardization:**
- Replace 66 manual methods with clean `EXPECT()` chains
- Automatic argument validation
- Built-in call ordering and counting
- **~800 lines → ~50 lines** per interface

#### Medium Confidence Areas (5-7/10)

**Advanced Simulation Features Loss:**
```go
// Current sophisticated behavior would be lost
func (f *FakeSLMClient) simulateNetworkConditions() error
func (f *FakeSLMClient) calculateComplexityDelay() time.Duration
func (f *FakeSLMClient) updateCircuitBreakerState()
```
**Risk:** GoMock doesn't provide these simulation capabilities out-of-the-box.

**Integration Test Compatibility:**
- Current fakes work excellently for **integration testing**
- GoMock is optimized for **unit testing**
- **Hybrid approach recommended**

#### Low Confidence Areas (3-4/10)

**Realistic Behavior Preservation:**
Current sophisticated scenarios that GoMock cannot easily replicate:
- Error injection with distribution patterns
- Circuit breaker state management
- Network latency simulation
- Memory/learning capabilities

**Migration Effort vs. Benefit:**
- **High migration cost:** 23 test files to update
- **Learning curve:** Team needs GoMock expertise
- **Feature parity:** May lose advanced testing scenarios

#### Strategic Migration Recommendation

**Phase 1: Hybrid Approach (Confidence: 8/10)**
```go
// Keep sophisticated fakes for integration tests
func TestComplexIntegrationScenario() {
    testSLM := shared.NewTestSLMClient() // Updated to use TestSLMClient
    // ... complex scenario testing
}

// Use GoMock for simple unit tests
func TestProcessorLogic() {
    mockSLM := NewMockSLMClient(ctrl) // New GoMock
    mockSLM.EXPECT().AnalyzeAlert(gomock.Any(), gomock.Any()).Return(result, nil)
    // ... simple interaction testing
}
```

**Phase 2: Selective Migration (Confidence: 7/10)**
- Migrate **simple unit tests** to GoMock
- Keep **integration/E2E tests** with current fakes
- Create **GoMock interfaces** for new components

**Phase 3: Enhanced GoMock (Confidence: 6/10)**
```go
// Custom GoMock matchers for complex scenarios
func NetworkErrorMatcher() gomock.Matcher {
    return &networkErrorMatcher{}
}

// GoMock with custom actions
mockSLM.EXPECT().AnalyzeAlert(gomock.Any(), gomock.Any()).
    DoAndReturn(func(ctx context.Context, alert types.Alert) (*types.ActionRecommendation, error) {
        // Custom simulation logic here
        time.Sleep(simulatedLatency)
        return result, nil
    })
```

#### Risk Assessment and Mitigation

**High-Risk Items:**
1. **Feature Loss:** Circuit breakers, error injection, network simulation
2. **Test Brittleness:** Over-specified mocks can break easily
3. **Migration Time:** 23 files × ~2 days each = 46 days effort

**Mitigation Strategies:**
1. **Keep sophisticated fakes** for complex integration scenarios
2. **Use GoMock** only for straightforward unit tests
3. **Gradual migration** starting with new components

#### Final Recommendation

**Confidence: 6.5/10** for partial migration with hybrid approach:

**✅ Do Migrate:**
- New unit tests
- Simple interaction testing
- Components with basic mocking needs

**❌ Don't Migrate:**
- Complex integration tests
- Error injection scenarios
- Network simulation tests
- Circuit breaker testing

**Conclusion:** The current fake implementations are exceptionally sophisticated and provide testing capabilities that GoMock cannot easily replicate. The migration would be **high-effort, medium-reward** unless focused on specific use cases where GoMock's strengths (call verification, argument matching) provide clear benefits.

**Bottom Line:** Consider GoMock for **new, simpler** test scenarios rather than wholesale migration. The current approach is already quite effective for the complex integration testing requirements.

## Troubleshooting

### Common Issues

1. **"No tests to run" error**
   - Ensure integration build tag: `go test -tags=integration`
   - Check that test files have proper build constraints

2. **LLM connection failures**
   - For Ollama: Verify Ollama is running: `ollama ps`
   - For Ollama: Check model availability: `ollama list`
   - Validate endpoint configuration

3. **Ginkgo focus not working**
   - Use exact Describe/Context text for focus patterns
   - Ensure proper escaping for special characters

### Performance Considerations
- Integration tests with real models take 13-15 seconds per test
- Use `SKIP_SLOW_TESTS=true` for faster feedback during development
- Run focused test suites during development, full suite in CI

## Framework Benefits Summary

The migration to Ginkgo/Gomega provides:
- **Maintainability**: Smaller, focused test files vs monolithic suites
- **Readability**: BDD-style specifications with structured intent
- **Organization**: Hierarchical test structure with shared setup
- **Assertions**: Expressive assertion syntax
- **Error Messages**: Failure reporting with context
- **Testing Framework**: Standard Go testing framework

This testing framework provides coverage of the Prometheus Alerts SLM system components and functionality.

---

## Anti-patterns
