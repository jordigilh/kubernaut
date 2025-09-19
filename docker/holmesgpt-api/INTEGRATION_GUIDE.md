# HolmesGPT API - Integration with Main Project

This document describes how the HolmesGPT API testing integrates with the main Kubernaut project infrastructure.

## 🔗 Integration Points

### **Main Project Makefile Integration**

Add these targets to the main `Makefile` to integrate HolmesGPT API testing:

```makefile
##@ HolmesGPT API Testing
.PHONY: test-holmesgpt-api-unit
test-holmesgpt-api-unit: ## Run HolmesGPT API unit tests
	@echo "🧪 Running HolmesGPT API unit tests..."
	cd docker/holmesgpt-api && make -f Makefile.test test-unit

.PHONY: test-holmesgpt-api-integration
test-holmesgpt-api-integration: ## Run HolmesGPT API integration tests with Kind (adaptive LLM)
	@echo "🏗️ Running HolmesGPT API integration tests..."
	cd docker/holmesgpt-api && make -f Makefile.test test-integration

.PHONY: test-holmesgpt-api-with-real-llm
test-holmesgpt-api-with-real-llm: ## Run HolmesGPT API tests with real LLM
	@echo "🧠 Running HolmesGPT API tests with real LLM..."
	cd docker/holmesgpt-api && make -f Makefile.test test-with-real-llm

.PHONY: test-holmesgpt-api-with-mock-llm
test-holmesgpt-api-with-mock-llm: ## Run HolmesGPT API tests with mock LLM
	@echo "🤖 Running HolmesGPT API tests with mock LLM..."
	cd docker/holmesgpt-api && make -f Makefile.test test-with-mock-llm

.PHONY: test-holmesgpt-api-e2e
test-holmesgpt-api-e2e: integration-services-start ## Run HolmesGPT API E2E tests
	@echo "🚀 Running HolmesGPT API E2E tests..."
	cd docker/holmesgpt-api && make -f Makefile.test test-e2e

.PHONY: test-holmesgpt-api-ci
test-holmesgpt-api-ci: ## Run HolmesGPT API tests for CI
	@echo "🤖 Running HolmesGPT API CI tests..."
	cd docker/holmesgpt-api && make -f Makefile.test test-ci

# Update existing test targets
.PHONY: test-all
test-all: validate-integration test test-integration test-e2e test-holmesgpt-api-unit test-holmesgpt-api-integration ## Run all tests including HolmesGPT API
	@echo "All test suites completed"

.PHONY: test-ci
test-ci: ## Run tests suitable for CI environment including HolmesGPT API
	@echo "🚀 Running CI test suite with hybrid strategy..."
	make test
	make test-integration-kind-ci
	make test-holmesgpt-api-ci
	@echo "✅ CI tests completed successfully"
```

### **GitHub Actions Integration**

Update `.github/workflows/` to include HolmesGPT API testing:

```yaml
# Add to existing workflow or create new holmesgpt-api.yml
- name: Test HolmesGPT API Unit
  run: make test-holmesgpt-api-unit

- name: Test HolmesGPT API Integration
  run: make test-holmesgpt-api-integration
  env:
    CI: true
    USE_MOCK_LLM: true
```

## 🚀 Usage Examples

### **Local Development**
```bash
# Quick development cycle
make test-holmesgpt-api-unit                    # 30 seconds
make test-holmesgpt-api-integration             # 5 minutes (adaptive LLM)
make test-holmesgpt-api-e2e                     # 15 minutes

# LLM-specific testing
make test-holmesgpt-api-with-real-llm           # Test with real LLM (Ollama/LocalAI)
make test-holmesgpt-api-with-mock-llm           # Test with mock LLM only

# Test with specific LLM configurations
LLM_ENDPOINT=http://localhost:11434 LLM_PROVIDER=ollama make test-holmesgpt-api-integration
LLM_ENDPOINT=http://localhost:8080 LLM_PROVIDER=localai make test-holmesgpt-api-integration

# Debug specific issues
cd docker/holmesgpt-api
make -f Makefile.test debug-logs
make -f Makefile.test debug-port-forward
make -f Makefile.test test-llm-integration      # Test LLM integration only
```

### **CI/CD Pipeline**
```bash
# Reliable CI testing
make test-holmesgpt-api-ci                      # Mock LLM, real K8s
```

### **Full Validation**
```bash
# Complete test suite
make test-all                                   # Includes HolmesGPT API tests
```

## 📊 Test Coverage Integration

The HolmesGPT API tests contribute to overall project coverage:

- **Unit Tests**: `docker/holmesgpt-api/tests/test_*.py` (129 tests)
- **Integration Tests**: `docker/holmesgpt-api/tests/integration/` (OAuth2 + K8s)
- **E2E Tests**: `docker/holmesgpt-api/tests/e2e/` (Full ecosystem)

## 🔧 Infrastructure Reuse

Following project guidelines, the HolmesGPT API tests **reuse existing infrastructure**:

### **Kind Cluster**
- Uses `scripts/setup-kind-cluster.sh`
- Leverages existing Kind configuration
- Reuses monitoring stack setup

### **Database Services**
- Uses `test/integration/scripts/bootstrap-integration-tests.sh`
- Leverages PostgreSQL + Vector DB containers
- Follows existing connection patterns

### **Test Patterns**
- Follows `test/integration/shared/config.go` patterns
- Uses similar environment variable configuration
- Matches existing test organization

## 🎯 Business Requirements Coverage

The HolmesGPT API tests validate **BR-HAPI-045** (OAuth 2 resource server) and **BR-HAPI-046** (Adaptive LLM support):

| **Component** | **Coverage** | **Test Level** |
|---------------|--------------|----------------|
| K8s Token Validation | ✅ | Integration |
| RBAC-to-Scope Mapping | ✅ | Integration |
| Bearer Token Support | ✅ | Integration |
| API Server Integration | ✅ | E2E |
| Performance | ✅ | E2E |

### **LLM Integration Features** *(BR-HAPI-046)*
- **Real LLM Support**: Integrates with Ollama (port 11434) and LocalAI (port 8080)
- **Auto-Detection**: Automatically detects LLM provider based on endpoint availability
- **Mock Fallback**: Falls back to mock LLM for CI/CD reliability when real LLM unavailable
- **Provider-Specific Tests**: Validates functionality across different LLM providers
- **Performance Testing**: Compares response times between real and mock LLM
- **Error Handling**: Tests graceful degradation when LLM services are unavailable

## 🔄 Development Workflow

### **Adding New Tests**
1. **Unit Tests**: Add to `tests/test_*.py`
2. **Integration Tests**: Add to `tests/integration/`
3. **E2E Tests**: Add to `tests/e2e/`

### **Debugging Failures**
1. Check unit tests first: `make test-holmesgpt-api-unit`
2. Debug integration: `make debug-logs debug-port-forward`
3. Manual testing: `make test-manual`

### **Performance Testing**
```bash
# Performance benchmarks
cd docker/holmesgpt-api
PYTHONPATH=./src python3 -m pytest tests/integration/ -m "slow" -v
```

## 📚 Documentation References

- **Test Plan**: [TEST_PLAN.md](./TEST_PLAN.md)
- **Business Requirements**: [BUSINESS_REQUIREMENTS_COVERAGE.md](./BUSINESS_REQUIREMENTS_COVERAGE.md)
- **Project Guidelines**: [../../docs/development/project guidelines.md](../../docs/development/project%20guidelines.md)

## 🏆 Success Metrics

- **✅ Unit Tests**: 129/129 passing
- **🔄 Integration Tests**: OAuth2 + K8s + LLM scenarios
- **🔄 E2E Tests**: Full ecosystem validation with adaptive LLM
- **📊 Coverage**: BR-HAPI-045 & BR-HAPI-046 requirements 100%
- **🧠 LLM Support**: Real LLM integration + Mock fallback tested

The HolmesGPT API testing infrastructure is designed to seamlessly integrate with the existing Kubernaut project while maintaining the established patterns and principles.
