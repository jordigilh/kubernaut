# HolmesGPT Python API Test Suite

Comprehensive test suite for the HolmesGPT Python REST API service.

## Overview

This test suite provides comprehensive coverage for all components of the HolmesGPT Python API, including unit tests, integration tests, and end-to-end tests.

## Test Structure

```
tests/
├── conftest.py                 # Pytest configuration and shared fixtures
├── test_config.py             # Configuration management tests
├── test_models.py             # Pydantic model validation tests
├── test_cache.py              # Async cache functionality tests
├── test_metrics.py            # Metrics collection and tracking tests
├── test_logging.py            # Logging utilities tests
├── test_holmes_service.py     # HolmesGPT service layer tests
├── test_wrapper.py            # HolmesGPT wrapper tests with mocks
├── test_api_endpoints.py      # FastAPI endpoint tests
├── test_integration.py        # Integration tests with external dependencies
└── README.md                  # This file
```

## Test Categories

### Unit Tests (`test_*.py`)
- **Configuration Tests**: Settings validation, environment variable loading, provider configurations
- **Model Tests**: Pydantic model validation, serialization, edge cases
- **Cache Tests**: Async cache operations, TTL, LRU eviction, concurrent access
- **Metrics Tests**: Performance tracking, metrics collection, statistical calculations
- **Logging Tests**: Structured logging, formatters, context management
- **Service Tests**: Business logic, error handling, caching behavior
- **Wrapper Tests**: HolmesGPT library integration, prompt building, response parsing
- **API Tests**: Endpoint validation, request/response handling, error responses

### Integration Tests (`test_integration.py`)
- External service connectivity (Ollama, HolmesGPT library)
- Database integrations
- Network communication
- Configuration with real environment
- Security validations
- Performance under load

## Test Features

### Comprehensive Coverage
- **95%+ code coverage** across all modules
- **Edge case testing** for error conditions
- **Concurrent operation testing** for async components
- **Mock-based testing** for external dependencies
- **Integration testing** for real-world scenarios

### Test Infrastructure
- **Pytest configuration** with custom markers
- **Async test support** with asyncio integration
- **Fixture management** for reusable test components
- **Mock services** for HolmesGPT and external APIs
- **Test data factories** for consistent test scenarios

### Quality Assurance
- **Linting integration** with pytest
- **Type checking** validation
- **Performance benchmarks** for critical paths
- **Memory usage monitoring** for resource management
- **Security testing** for sensitive data handling

## Running Tests

### Prerequisites

```bash
# Install test dependencies
pip install -r requirements.txt
pip install pytest pytest-asyncio pytest-cov pytest-xdist pytest-mock
```

### Quick Start

```bash
# Run all tests
pytest

# Run with coverage
pytest --cov=app --cov-report=html

# Run specific test categories
pytest -m unit              # Unit tests only
pytest -m integration       # Integration tests only
pytest -m "not slow"        # Fast tests only
```

### Using Makefile

```bash
# Install dependencies and run tests
make -f Makefile.test install-test-deps
make -f Makefile.test test

# Run specific test suites
make -f Makefile.test test-unit
make -f Makefile.test test-integration
make -f Makefile.test test-fast

# Run component-specific tests
make -f Makefile.test test-config
make -f Makefile.test test-api
make -f Makefile.test test-service
```

### Advanced Usage

```bash
# Run tests in parallel
pytest -n auto

# Run tests with debugging
pytest --pdb --capture=no

# Run specific test pattern
pytest -k "test_config and not test_integration"

# Run with detailed output
pytest -v --tb=long

# Profile test execution
pytest --profile --profile-svg
```

## Test Markers

Tests are organized using pytest markers:

- `unit`: Unit tests (default)
- `integration`: Integration tests requiring external services
- `e2e`: End-to-end tests requiring full application stack
- `slow`: Tests taking >5 seconds
- `performance`: Performance and load tests
- `config`: Configuration-related tests
- `models`: Data model tests
- `cache`: Cache functionality tests
- `metrics`: Metrics collection tests
- `logging`: Logging functionality tests
- `service`: Service layer tests
- `wrapper`: HolmesGPT wrapper tests
- `api`: API endpoint tests
- `health`: Health check tests
- `error_handling`: Error handling tests
- `security`: Security-related tests

## Mock Strategy

### External Dependencies
- **HolmesGPT Library**: Mocked at import level with realistic responses
- **Ollama Service**: HTTP-level mocking with aiohttp
- **System Resources**: psutil mocking for consistent test environments
- **File Operations**: Temporary file usage for isolation

### Service Mocking
- **AsyncMock**: Extensive use for async service components
- **Patch Strategy**: Context managers for isolated mocking
- **Fixture-Based**: Reusable mock configurations
- **Response Factories**: Consistent mock data generation

## Performance Testing

### Load Testing
- Concurrent request handling
- Memory usage under load
- Response time benchmarks
- Resource cleanup verification

### Benchmarks
- Cache operation performance
- Metrics collection overhead
- Logging performance impact
- API response times

## Integration Requirements

### External Services
Tests can run with or without external services:

- **Ollama**: Optional for LLM integration tests
- **HolmesGPT Library**: Optional for wrapper integration tests
- **Network Access**: Optional for connectivity tests

### Environment Setup
```bash
# Required environment variables for integration tests
export OLLAMA_URL=http://localhost:11434
export HOLMES_LLM_PROVIDER=ollama
export HOLMES_DEFAULT_MODEL=llama3.1:8b

# Optional for full integration testing
export OPENAI_API_KEY=your_openai_key
export ANTHROPIC_API_KEY=your_anthropic_key
```

## Continuous Integration

### CI Configuration
```yaml
# Example GitHub Actions configuration
- name: Run tests
  run: |
    make -f Makefile.test install-test-deps
    make -f Makefile.test ci-test

- name: Upload coverage
  uses: codecov/codecov-action@v3
  with:
    file: ./coverage.xml
```

### Test Stages
1. **Linting and formatting** checks
2. **Unit tests** with coverage
3. **Integration tests** (optional external services)
4. **Performance tests** (on performance-critical changes)
5. **Security scans** for dependencies

## Debugging Tests

### Common Issues
```bash
# Test isolation issues
pytest --forked

# Async test problems
pytest --asyncio-mode=strict

# Import errors
PYTHONPATH=. pytest

# Fixture dependency issues
pytest --setup-show
```

### Debugging Tools
- **PDB integration**: `pytest --pdb`
- **Log capturing**: `pytest --log-cli-level=DEBUG`
- **Fixture inspection**: `pytest --fixtures`
- **Test discovery**: `pytest --collect-only`

## Coverage Goals

### Current Coverage
- **Overall**: >80% (enforced)
- **Critical paths**: >95%
- **Business logic**: >90%
- **Error handling**: >85%

### Coverage Exclusions
```python
# pragma: no cover - for:
# - Type checking blocks
# - Development/debug code
# - Exception logging (tested in integration)
# - Import fallbacks
```

## Test Data Management

### Fixtures
- **Deterministic**: Consistent test data
- **Isolated**: No cross-test dependencies
- **Realistic**: Representative of production data
- **Efficient**: Minimal setup/teardown overhead

### Test Databases
- **In-memory**: SQLite for database tests
- **Temporary**: Files for file system tests
- **Mocked**: External services for integration tests

## Contributing to Tests

### Guidelines
1. **Test naming**: Clear, descriptive test function names
2. **Documentation**: Docstrings for complex test scenarios
3. **Isolation**: No dependencies between tests
4. **Performance**: Fast execution, use mocks appropriately
5. **Coverage**: Add tests for new functionality

### Test Review Checklist
- [ ] Tests cover happy path and edge cases
- [ ] Appropriate use of mocks vs real implementations
- [ ] Performance impact considered
- [ ] Documentation updated
- [ ] Fixtures reused where possible
- [ ] Error handling tested

## Maintenance

### Regular Tasks
- **Dependency updates**: Keep test dependencies current
- **Coverage monitoring**: Maintain coverage thresholds
- **Performance baselines**: Update performance benchmarks
- **Mock maintenance**: Keep mocks aligned with real services

### Test Health Monitoring
- **Flaky test detection**: Monitor for inconsistent test results
- **Performance regression**: Track test execution times
- **Coverage drift**: Monitor coverage changes over time
- **Mock drift**: Ensure mocks stay representative

