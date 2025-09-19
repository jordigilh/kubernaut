# Intelligent Workflow Builder Integration Tests

## Overview

This directory contains comprehensive integration tests for the IntelligentWorkflowBuilder feature, designed to validate end-to-end functionality with real dependencies following the established testing framework.

## Test Structure

### Framework Compliance
- **Testing Framework**: Ginkgo v2 + Gomega (BDD-style)
- **Organization**: Hierarchical test structure with Describe/Context/It blocks
- **Setup**: BeforeSuite/AfterSuite for shared resource management
- **Build Tags**: `//go:build integration` for proper test isolation

### Test Categories

#### 1. Real SLM Client Integration (`Context: "Real SLM Client Integration"`)
- **AI-Driven Workflow Generation**: Tests workflow generation with actual LLM models
- **Error Handling and Resilience**: Validates graceful handling of AI service failures
- **Performance Validation**: Ensures workflow generation meets performance requirements

#### 2. Vector Database Integration (`Context: "Vector Database Integration"`)
- **Pattern Discovery and Learning**: Tests pattern discovery from execution history
- **Pattern Application and Reuse**: Validates pattern application to new workflows
- **Vector Storage Operations**: Tests storage and retrieval of action patterns

#### 3. End-to-End Workflow Lifecycle (`Context: "End-to-End Workflow Lifecycle"`)
- **Complete Lifecycle**: Generation → Validation → Simulation → Learning
- **Workflow Optimization Cycle**: Tests structural optimization capabilities
- **Integration with All Components**: Validates component interaction

#### 4. Performance and Load Testing (`Context: "Performance and Load Testing"`)
- **Concurrent Operations**: Multiple simultaneous workflow generations
- **Memory and Resource Usage**: Resource consumption monitoring
- **Scalability Validation**: Performance under load

## Configuration

### Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `LLM_PROVIDER` | SLM provider (ollama, ramalama) | `ollama` | No |
| `LLM_MODEL` | LLM model name | `gpt-oss:20b` | No |
| `LLM_ENDPOINT` | LLM service endpoint | `http://localhost:11434` | No |
| `SKIP_SLM_TESTS` | Skip tests requiring real SLM | `false` | No |
| `SKIP_PERFORMANCE_TESTS` | Skip performance tests | `false` | No |
| `SKIP_SLOW_TESTS` | Skip slow-running tests | `false` | No |
| `LOG_LEVEL` | Logging level (debug, info, warn) | `info` | No |
| `TEST_TIMEOUT` | Global test timeout | `5m` | No |

### Prerequisites

#### For SLM Integration Tests
1. **Ollama Service** (if using ollama provider):
   ```bash
   # Install and start Ollama
   curl -fsSL https://ollama.ai/install.sh | sh
   ollama serve

   # Pull required model
   ollama pull granite3.1-dense:8b
   ```

2. **Alternative LLM Provider** (if not using Ollama):
   ```bash
   # Configure your LLM service endpoint
   export LLM_PROVIDER=your_provider
   export LLM_ENDPOINT=your_endpoint
   export LLM_MODEL=your_model
   ```

#### For Vector Database Tests
- Vector database functionality is mocked for integration tests
- Real vector database integration can be enabled by modifying `NewIntegrationVectorDatabase()`

## Running Tests

### All Integration Tests
```bash
# Run all intelligent workflow builder integration tests
go test -tags=integration ./test/integration/intelligent_workflow_builder_*.go -v

# With Ginkgo (recommended)
ginkgo -tags=integration --focus="Intelligent Workflow Builder" ./test/integration/
```

### Specific Test Categories

#### SLM Integration Tests
```bash
# Run only SLM integration tests
ginkgo -tags=integration --focus="Real SLM Client Integration" ./test/integration/

# Skip SLM tests if service unavailable
SKIP_SLM_TESTS=true ginkgo -tags=integration ./test/integration/
```

#### Performance Tests
```bash
# Run performance and load tests
ginkgo -tags=integration --focus="Performance and Load Testing" ./test/integration/

# Skip performance tests for faster feedback
SKIP_PERFORMANCE_TESTS=true ginkgo -tags=integration ./test/integration/
```

#### End-to-End Tests
```bash
# Run complete lifecycle tests
ginkgo -tags=integration --focus="End-to-End Workflow Lifecycle" ./test/integration/
```

### With Different LLM Models

#### Granite Model (Default)
```bash
LLM_MODEL=granite3.1-dense:8b ginkgo -tags=integration ./test/integration/
```

#### Alternative Models
```bash
# Using different model
LLM_MODEL=llama2:7b ginkgo -tags=integration ./test/integration/

# Using different provider
LLM_PROVIDER=ramalama LLM_ENDPOINT=http://your-endpoint ginkgo -tags=integration ./test/integration/
```

## Test Performance Expectations

### Performance Benchmarks

| Operation | Expected Time | Maximum Time | Notes |
|-----------|---------------|--------------|--------|
| Workflow Generation | < 30s | < 45s | With real SLM |
| Workflow Validation | < 5s | < 10s | Safety checks |
| Workflow Simulation | < 10s | < 20s | Mock execution |
| Pattern Learning | < 2s | < 5s | Store in vector DB |
| Complete Lifecycle | < 2m | < 3m | End-to-end |

### Concurrent Performance
- **5 concurrent generations**: All should complete successfully
- **Average time**: < 45s per generation
- **Maximum time**: < 60s for any single generation

## Test Data and Scenarios

### Realistic Test Scenarios

#### Memory Optimization
```yaml
Objective: Optimize memory usage for high-memory pods
Targets: Kubernetes deployments with memory issues
Expected Steps: Diagnostics → Analysis → Resource adjustment
Risk Level: Medium
```

#### Pod Crashloop Recovery
```yaml
Objective: Resolve pod crashloop issues
Targets: Failing deployments
Expected Steps: Log collection → Resource check → Safe restart
Risk Level: Low
```

#### Multi-Service Optimization
```yaml
Objective: Optimize microservices architecture
Targets: Multiple deployments
Expected Steps: Coordinated optimization across services
Risk Level: High
```

## Monitoring and Reporting

### Performance Metrics Collected
- **Response Times**: Per operation timing
- **Success Rates**: Pass/fail ratios by category
- **Resource Usage**: Memory and CPU consumption
- **Error Categorization**: Types and frequency of failures

### Test Reports
Integration tests generate comprehensive reports including:
- Overall success rates
- Performance benchmark compliance
- Resource usage patterns
- Error analysis and categorization

### Example Report Output
```
Integration Test Execution Report:
  Total Duration: 3m45s
  Total Tests: 25
  Passed: 23 (92%)
  Failed: 2 (8%)
  Average Response Time: 12.3s
  Performance Violations: 1
  Categories:
    - Workflow Generation: 8 tests
    - Validation: 6 tests
    - Simulation: 5 tests
    - Learning: 4 tests
    - E2E: 2 tests
```

## Troubleshooting

### Common Issues

#### SLM Connection Failures
```bash
# Check Ollama status
ollama ps

# Check model availability
ollama list

# Verify endpoint connectivity
curl -f http://localhost:11434/api/health
```

#### Test Timeouts
```bash
# Increase timeout for slow environments
TEST_TIMEOUT=10m ginkgo -tags=integration ./test/integration/

# Skip slow tests for faster feedback
SKIP_SLOW_TESTS=true ginkgo -tags=integration ./test/integration/
```

#### Memory Issues
```bash
# Monitor memory usage during tests
go test -tags=integration -memprofile=mem.prof ./test/integration/

# Force garbage collection more frequently
SKIP_PERFORMANCE_TESTS=true ginkgo -tags=integration ./test/integration/
```

### Debug Mode
```bash
# Enable debug logging
LOG_LEVEL=debug ginkgo -tags=integration -v ./test/integration/

# Enable Ginkgo verbose output
GINKGO_REPORTER=verbose ginkgo -tags=integration ./test/integration/
```

## CI/CD Integration

### Pipeline Configuration
```yaml
# Example GitHub Actions integration
- name: Run Intelligent Workflow Builder Integration Tests
  run: |
    # Start required services
    docker-compose up -d ollama

    # Wait for services
    ./scripts/wait-for-services.sh

    # Run tests with appropriate timeouts
    SKIP_SLOW_TESTS=true ginkgo -tags=integration --timeout=10m ./test/integration/
  env:
    LLM_ENDPOINT: http://localhost:11434
    LOG_LEVEL: info
```

### Performance Gates
Integration tests can be configured as quality gates:
- All tests must pass
- No performance violations above threshold
- Memory usage within acceptable limits
- Response times meet SLA requirements

## Contributing

### Adding New Integration Tests
1. Follow existing test structure and naming conventions
2. Use realistic test scenarios and data
3. Include performance expectations and validation
4. Add appropriate error handling and cleanup
5. Update this README with new test categories

### Test Development Guidelines
- **Realistic Scenarios**: Use production-like objectives and constraints
- **Error Coverage**: Test both success and failure paths
- **Performance Focus**: Include timing and resource usage validation
- **Clean Architecture**: Separate test logic from test data and configuration
- **Documentation**: Clear test descriptions and expected outcomes

## Related Documentation
- [Testing Framework](../../docs/TESTING_FRAMEWORK.md) - Overall testing guidelines
- [Workflow Engine Documentation](../../docs/WORKFLOWS.md) - Feature overview
- [Architecture Documentation](../../docs/ARCHITECTURE.md) - System design
