# Testing Framework Documentation

## Overview

The Prometheus Alerts SLM project uses a modern, BDD-style testing framework built on **Ginkgo v2** and **Gomega**. This framework provides clear, maintainable test specifications with excellent organization and readability.

## Framework Migration

### Completed Migration from Testify
- ✅ **Complete elimination** of all `github.com/stretchr/testify` dependencies
- ✅ **Full migration** to Ginkgo/Gomega testing framework
- ✅ **Modular test organization** with focused, manageable test files
- ✅ **BDD-style specifications** using Describe/Context/It patterns

### Benefits of Ginkgo/Gomega
- **Better Organization**: Clear hierarchical test structure with Describe/Context blocks
- **Readable Assertions**: Fluent, English-like assertions with Gomega's Expect syntax
- **Focused Testing**: Individual test files for specific functionality areas
- **Rich Reporting**: Detailed test output with clear success/failure reporting
- **Parallel Execution**: Built-in support for parallel test execution

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
- **`slm_mcp_context_ginkgo_test.go`**: MCP integration and context-aware testing

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

### Unit Tests
```bash
# Run processor tests
go test ./pkg/processor/

# Run executor tests  
go test ./pkg/executor/
```

### Integration Tests

#### All Integration Tests
```bash
go test -tags=integration ./test/integration/...
```

#### Specific Action Category Tests
```bash
# Storage actions
go test -tags=integration -ginkgo.focus="Storage and Persistence Actions" ./test/integration/

# Security actions
go test -tags=integration -ginkgo.focus="Security and Compliance Actions" ./test/integration/

# Network actions
go test -tags=integration -ginkgo.focus="Network and Connectivity Actions" ./test/integration/
```

#### Model-Specific Testing
```bash
# Test with specific Ollama model
OLLAMA_MODEL=granite3.1-dense:8b go test -tags=integration ./test/integration/

# Skip slow tests
SKIP_SLOW_TESTS=true go test -tags=integration ./test/integration/
```

### Test Configuration

Tests are configured via environment variables:
- `OLLAMA_ENDPOINT`: Ollama server endpoint (default: http://localhost:11434)
- `OLLAMA_MODEL`: Model to use for testing (default: granite3.1-dense:8b)
- `SKIP_SLOW_TESTS`: Skip performance and load tests
- `SKIP_INTEGRATION`: Skip all integration tests
- `LOG_LEVEL`: Logging level for tests

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
- **Clear test descriptions**: Use descriptive names in Describe/Context/It blocks
- **Shared setup**: Use BeforeSuite/AfterSuite for expensive setup operations
- **Isolated tests**: Each test should be independent and not rely on others

### Assertion Guidelines
- **Specific assertions**: Use precise Gomega matchers for clear failure messages
- **Error handling**: Always check for errors with `Expect(err).ToNot(HaveOccurred())`
- **Meaningful messages**: Add custom failure messages for complex assertions
- **Resource cleanup**: Ensure proper cleanup in AfterEach/AfterSuite

### Integration Test Guidelines
- **Real dependencies**: Use actual Ollama/Granite models for realistic testing
- **Timeout handling**: Set appropriate timeouts for model inference calls
- **Resource monitoring**: Track memory usage and response times
- **Environment validation**: Check required services are available before testing

## Troubleshooting

### Common Issues

1. **"No tests to run" error**
   - Ensure integration build tag: `go test -tags=integration`
   - Check that test files have proper build constraints

2. **Ollama connection failures**
   - Verify Ollama is running: `ollama ps`
   - Check model availability: `ollama list`
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
- **Improved Maintainability**: Smaller, focused test files vs monolithic suites
- **Better Readability**: BDD-style specifications with clear intent
- **Enhanced Organization**: Hierarchical test structure with shared setup
- **Rich Assertions**: Fluent, expressive assertion syntax
- **Better Error Messages**: Clear failure reporting with context
- **Modern Testing**: Industry-standard Go testing framework

This testing framework ensures comprehensive coverage of the Prometheus Alerts SLM system while maintaining clarity, maintainability, and ease of development.