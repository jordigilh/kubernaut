# HolmesGPT API - Authentication & Integration Test Plan

**Status**: 🎯 **ACTIVE** - OAuth 2 Resource Server Integration Testing
**Framework**: TDD with pytest + existing K8s infrastructure
**Following**: [Project Guidelines](../../docs/development/project%20guidelines.md)

---

## 📋 **Test Plan Overview**

This test plan validates the **OAuth 2 resource server integration** with Kubernetes API server authentication for the `holmesgpt-api` module, leveraging existing infrastructure to maximize reusability and follow established patterns.

### **Business Requirement Focus**
- **BR-HAPI-045**: OAuth 2 resource server compatible with Kubernetes API server
  - K8s Token Validation
  - K8s API Server Integration
  - RBAC-to-Scope Mapping
  - Scope-based Authorization
  - Bearer Token Support
  - Service Account Integration
  - JWT Token Validation
  - Resource Server Pattern
- **BR-HAPI-046**: Integration tests with adaptive LLM support
  - Real LLM Integration (when available)
  - Mock LLM Fallback (for CI/CD reliability)
  - LLM Provider Detection
  - Graceful Degradation

---

## 🏗️ **Test Architecture Strategy**

### **Infrastructure Reuse Pattern** *(Following Project Guidelines)*
```
┌─────────────────────────────────────────────────────┐
│ Existing Infrastructure (REUSE)                    │
├─────────────────────────────────────────────────────┤
│ • Kind Cluster Setup (scripts/setup-kind-cluster.sh│
│ • PostgreSQL + Vector DB (bootstrap script)        │
│ • Service Account RBAC (test/manifests/)           │
│ • Integration Test Framework (test/integration/)    │
│ • Mock Infrastructure (test/shared/)                │
└─────────────────────────────────────────────────────┘
                        ↓
┌─────────────────────────────────────────────────────┐
│ HolmesGPT API Integration Layer (NEW)              │
├─────────────────────────────────────────────────────┤
│ • OAuth2 Resource Server Tests                     │
│ • K8s ServiceAccount Token Validation              │
│ • Real K8s API Integration                          │
│ • Scope-based Authorization Tests                   │
└─────────────────────────────────────────────────────┘
```

---

## 🧪 **Test Levels & Scope**

### **Level 1: Unit Tests** *(CURRENT - ✅ 129 tests passing)*
- **Location**: `docker/holmesgpt-api/tests/`
- **Scope**: Business logic validation with mocks
- **Status**: Complete and passing
- **Infrastructure**: pytest + mock dependencies

### **Level 2: Component Integration Tests** *(NEW - THIS PLAN)*
- **Location**: `docker/holmesgpt-api/tests/integration/`
- **Scope**: holmesgpt-api ↔ Real K8s API server
- **Infrastructure**: Kind cluster + Real K8s tokens

### **Level 3: System Integration Tests** *(EXTENDED)*
- **Location**: `test/integration/holmesgpt_api/`
- **Scope**: holmesgpt-api ↔ Full Kubernaut ecosystem
- **Infrastructure**: Kind + PostgreSQL + Vector DB + Mock LLM

### **Level 4: End-to-End Tests** *(PLANNED)*
- **Location**: `test/e2e/holmesgpt_api/`
- **Scope**: Production-like scenarios with OpenShift
- **Infrastructure**: OCP cluster + Real components

---

## 🎯 **Test Scenarios & Business Requirements**

### **Authentication Test Scenarios**

#### **Scenario 1: K8s ServiceAccount Token Validation**
```yaml
Business Requirement: BR-HAPI-045.1 - K8s Token Validation
Test Level: Component Integration
Infrastructure: Kind cluster with real ServiceAccounts

Test Cases:
- Valid ServiceAccount token accepted
- Invalid token format rejected
- Expired token rejected
- Non-ServiceAccount subject rejected
- Token with insufficient permissions rejected
- Multi-namespace token validation
```

#### **Scenario 2: RBAC-to-OAuth2 Scope Mapping**
```yaml
Business Requirement: BR-HAPI-045.3 - RBAC-to-Scope Mapping
Test Level: Component Integration
Infrastructure: Kind cluster with real RBAC

Test Cases:
- Default ServiceAccount → basic scopes
- Admin ServiceAccount → admin scopes
- HolmesGPT ServiceAccount → investigation scopes
- Namespace-scoped permissions mapping
- Cluster-scoped permissions mapping
- Scope hierarchy validation
```

#### **Scenario 3: Bearer Token Authorization**
```yaml
Business Requirement: BR-HAPI-045.5 - Bearer Token Support
Test Level: Component Integration
Infrastructure: Real HTTP requests with K8s tokens

Test Cases:
- Authorization header parsing
- Bearer token extraction
- Token validation flow
- Scope-based endpoint access
- Missing token rejection
- Invalid Bearer format rejection
```

#### **Scenario 4: K8s API Server Integration**
```yaml
Business Requirement: BR-HAPI-045.2 - K8s API Server Integration
Test Level: System Integration
Infrastructure: Kind cluster + TokenReview API

Test Cases:
- TokenReview API validation
- API server connectivity
- Token audience validation
- Service account information extraction
- Permission discovery via K8s API
- Error handling for API failures
```

#### **Scenario 5: Adaptive LLM Integration**
```yaml
Business Requirement: BR-HAPI-046 - Integration tests with adaptive LLM support
Test Level: System Integration
Infrastructure: Real LLM (when available) + Mock LLM fallback

Test Cases:
- Real LLM integration when endpoint available
- Mock LLM fallback for CI/CD environments
- LLM provider auto-detection (ollama, localai, mock)
- Investigation quality with real vs mock LLM
- Chat functionality with real vs mock LLM
- Performance comparison between LLM types
- Error handling for LLM unavailability
- Graceful degradation scenarios
```

---

## 🚀 **Test Implementation Plan**

### **Phase 1: Component Integration Tests** *(Priority: HIGH)*

#### **1.1 Setup Test Infrastructure**
```bash
# Leveraging existing infrastructure
Location: docker/holmesgpt-api/tests/integration/
Base: scripts/setup-kind-cluster.sh + existing patterns

Components:
- pytest-kubernetes plugin
- Real K8s ServiceAccount creation
- Test namespace with proper RBAC
- holmesgpt-api deployment in Kind
```

#### **1.2 Authentication Test Suite**
```python
# docker/holmesgpt-api/tests/integration/test_k8s_auth_integration.py
class TestK8sAuthenticationIntegration:
    """Integration tests for OAuth2 + K8s authentication"""

    def test_valid_serviceaccount_token_authentication(self):
        """BR-HAPI-045.1: Valid K8s SA tokens should authenticate successfully"""

    def test_rbac_to_oauth2_scope_mapping(self):
        """BR-HAPI-045.3: K8s RBAC should map to OAuth2 scopes correctly"""

    def test_bearer_token_endpoint_authorization(self):
        """BR-HAPI-045.5: Endpoints should authorize based on Bearer tokens"""
```

#### **1.3 Test Data & Fixtures**
```yaml
# Reusing existing patterns from test/fixtures/
ServiceAccounts:
- test-admin-sa (cluster-admin permissions)
- test-viewer-sa (read-only permissions)
- holmesgpt-sa (investigation permissions)
- test-restricted-sa (minimal permissions)

Test Tokens:
- Valid tokens for each ServiceAccount
- Expired tokens for negative testing
- Malformed tokens for validation testing
```

### **Phase 2: System Integration Tests** *(Priority: MEDIUM)*

#### **2.1 Full Ecosystem Integration**
```bash
# Leveraging test/integration/ infrastructure
Location: test/integration/holmesgpt_api/
Base: Existing Go integration test patterns adapted for Python API

Components:
- Real Kind cluster (via setup-kind-cluster.sh)
- Real PostgreSQL + Vector DB (via bootstrap script)
- Mock LLM (for test reliability)
- holmesgpt-api deployed as K8s service
```

#### **2.2 End-to-End Authentication Flow**
```python
# test/integration/holmesgpt_api/test_auth_e2e.py
class TestHolmesGPTAuthE2E:
    """End-to-end authentication tests in real K8s environment"""

    def test_investigation_with_k8s_token(self):
        """Full flow: K8s token → OAuth2 validation → Investigation API"""

    def test_chat_with_serviceaccount_permissions(self):
        """Full flow: SA token → Scope validation → Chat API"""

    def test_investigation_with_real_llm(self):
        """BR-HAPI-046: Investigation with real LLM when available"""

    def test_investigation_with_mock_llm_fallback(self):
        """BR-HAPI-046: Investigation with mock LLM fallback"""

    def test_llm_provider_auto_detection(self):
        """BR-HAPI-046: Automatic LLM provider detection and configuration"""
```

### **Phase 3: Performance & Resilience Tests** *(Priority: LOW)*

#### **3.1 Authentication Performance**
```python
# test/integration/holmesgpt_api/test_auth_performance.py
class TestAuthenticationPerformance:
    """Performance tests for OAuth2 + K8s integration"""

    def test_token_validation_latency(self):
        """Token validation should complete within SLA"""

    def test_concurrent_authentication_load(self):
        """System should handle concurrent auth requests"""
```

---

## 🛠️ **Infrastructure Integration**

### **Makefile Targets** *(Following existing patterns)*

```makefile
##@ HolmesGPT API Testing
.PHONY: test-holmesgpt-api-unit
test-holmesgpt-api-unit: ## Run HolmesGPT API unit tests
	@echo "🧪 Running HolmesGPT API unit tests..."
	cd docker/holmesgpt-api && PYTHONPATH=./src python3 -m pytest tests/ -v

.PHONY: test-holmesgpt-api-integration
test-holmesgpt-api-integration: ## Run HolmesGPT API integration tests with Kind (real LLM when available)
	@echo "🏗️ Running HolmesGPT API integration tests with Kind cluster..."
	@echo "  ├── Kubernetes: Real Kind cluster"
	@echo "  ├── Authentication: Real K8s ServiceAccount tokens"
	@echo "  ├── API: holmesgpt-api deployed in Kind"
	@echo "  ├── LLM: Real LLM (if available) or Mock fallback"
	@echo "  └── Purpose: OAuth2 + K8s + LLM integration validation"
	@echo ""
	./scripts/setup-kind-cluster.sh
	make deploy-holmesgpt-api-to-kind
	@echo "Running integration tests..."
	cd docker/holmesgpt-api && \
	KUBECONFIG=$$(kind get kubeconfig --name=prometheus-alerts-slm-test) \
	HOLMESGPT_API_ENDPOINT=http://localhost:8800 \
	LLM_ENDPOINT=$${LLM_ENDPOINT:-http://localhost:8080} \
	LLM_MODEL=$${LLM_MODEL:-granite3.1-dense:8b} \
	LLM_PROVIDER=$${LLM_PROVIDER:-auto-detect} \
	PYTHONPATH=./src python3 -m pytest tests/integration/ -v --tb=short
	@echo "Cleaning up..."
	./scripts/cleanup-kind-cluster.sh

.PHONY: test-holmesgpt-api-integration-mock-llm
test-holmesgpt-api-integration-mock-llm: ## Run HolmesGPT API integration tests with mock LLM only
	@echo "🤖 Running HolmesGPT API integration tests with mock LLM..."
	@echo "  ├── Kubernetes: Real Kind cluster"
	@echo "  ├── Authentication: Real K8s ServiceAccount tokens"
	@echo "  ├── API: holmesgpt-api deployed in Kind"
	@echo "  ├── LLM: Mock LLM (for CI/CD reliability)"
	@echo "  └── Purpose: OAuth2 + K8s integration with reliable LLM"
	@echo ""
	./scripts/setup-kind-cluster.sh
	make deploy-holmesgpt-api-to-kind
	@echo "Running integration tests with mock LLM..."
	cd docker/holmesgpt-api && \
	KUBECONFIG=$$(kind get kubeconfig --name=prometheus-alerts-slm-test) \
	HOLMESGPT_API_ENDPOINT=http://localhost:8800 \
	USE_MOCK_LLM=true \
	LLM_PROVIDER=mock \
	PYTHONPATH=./src python3 -m pytest tests/integration/ -v --tb=short
	@echo "Cleaning up..."
	./scripts/cleanup-kind-cluster.sh

.PHONY: test-holmesgpt-api-e2e
test-holmesgpt-api-e2e: integration-services-start ## Run HolmesGPT API E2E tests
	@echo "🚀 Running HolmesGPT API E2E tests..."
	./scripts/setup-kind-cluster.sh
	make deploy-holmesgpt-api-to-kind
	@echo "Running E2E tests..."
	cd docker/holmesgpt-api && \
	KUBECONFIG=$$(kind get kubeconfig --name=prometheus-alerts-slm-test) \
	DB_HOST=localhost DB_PORT=5433 \
	VECTOR_DB_HOST=localhost VECTOR_DB_PORT=5434 \
	HOLMESGPT_API_ENDPOINT=http://localhost:8800 \
	PYTHONPATH=./src python3 -m pytest tests/e2e/ -v --tb=short
	./scripts/cleanup-kind-cluster.sh
	make integration-services-stop

.PHONY: deploy-holmesgpt-api-to-kind
deploy-holmesgpt-api-to-kind: ## Deploy HolmesGPT API to Kind cluster for testing
	@echo "📦 Deploying HolmesGPT API to Kind cluster..."
	kubectl apply -f docker/holmesgpt-api/k8s/
	kubectl wait --for=condition=Available deployment/holmesgpt-api -n holmesgpt --timeout=300s
	kubectl port-forward svc/holmesgpt-api 8800:8000 -n holmesgpt &
	@echo "HolmesGPT API available at http://localhost:8800"
```

### **Kubernetes Manifests** *(New)*

```yaml
# docker/holmesgpt-api/k8s/namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: holmesgpt
  labels:
    name: holmesgpt

---
# docker/holmesgpt-api/k8s/serviceaccount.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: holmesgpt-api
  namespace: holmesgpt

---
# docker/holmesgpt-api/k8s/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: holmesgpt-api
  namespace: holmesgpt
spec:
  replicas: 1
  selector:
    matchLabels:
      app: holmesgpt-api
  template:
    metadata:
      labels:
        app: holmesgpt-api
    spec:
      serviceAccountName: holmesgpt-api
      containers:
      - name: holmesgpt-api
        image: holmesgpt-api:test
        ports:
        - containerPort: 8000
        env:
        - name: LLM_PROVIDER
          value: "mock"
        - name: LLM_MODEL
          value: "mock-model"
        - name: K8S_AUTH_MODE
          value: "serviceaccount"
```

### **Test Configuration** *(Following existing patterns)*

```python
# docker/holmesgpt-api/tests/integration/conftest.py
import pytest
import os
from kubernetes import client, config
from .k8s_client import K8sTestClient

@pytest.fixture(scope="session")
def k8s_integration_config():
    """Integration test configuration following existing patterns"""
    # LLM Configuration with auto-detection and fallback
    llm_endpoint = os.getenv("LLM_ENDPOINT", "http://localhost:8080")
    use_mock_llm = bool(os.getenv("USE_MOCK_LLM", False))

    # Auto-detect LLM provider if not specified
    llm_provider = os.getenv("LLM_PROVIDER", "auto-detect")
    if llm_provider == "auto-detect":
        llm_provider = detect_llm_provider(llm_endpoint, use_mock_llm)

    return {
        "kubeconfig": os.getenv("KUBECONFIG"),
        "namespace": "holmesgpt",
        "api_endpoint": os.getenv("HOLMESGPT_API_ENDPOINT", "http://localhost:8800"),
        "test_timeout": int(os.getenv("TEST_TIMEOUT", "120")),
        "use_real_k8s": not bool(os.getenv("USE_FAKE_K8S_CLIENT", False)),
        # LLM Configuration - BR-HAPI-046
        "llm_endpoint": llm_endpoint,
        "llm_model": os.getenv("LLM_MODEL", "granite3.1-dense:8b"),
        "llm_provider": llm_provider,
        "use_mock_llm": use_mock_llm,
        "llm_available": check_llm_availability(llm_endpoint, use_mock_llm)
    }

def detect_llm_provider(endpoint: str, use_mock: bool) -> str:
    """BR-HAPI-046: Auto-detect LLM provider based on endpoint and availability"""
    if use_mock:
        return "mock"

    # Try to detect provider based on port and availability
    if ":11434" in endpoint:
        return "ollama" if check_endpoint_available(endpoint) else "mock"
    elif ":8080" in endpoint:
        return "localai" if check_endpoint_available(endpoint) else "mock"
    else:
        return "mock"  # Fallback to mock for unknown endpoints

def check_llm_availability(endpoint: str, use_mock: bool) -> bool:
    """Check if LLM endpoint is available"""
    if use_mock:
        return True
    return check_endpoint_available(endpoint)

def check_endpoint_available(endpoint: str) -> bool:
    """Check if endpoint responds to health check"""
    try:
        import requests
        response = requests.get(f"{endpoint}/health", timeout=5)
        return response.status_code == 200
    except:
        return False

@pytest.fixture(scope="session")
def k8s_client(k8s_integration_config):
    """Real K8s client for integration testing"""
    if k8s_integration_config["kubeconfig"]:
        config.load_kube_config(k8s_integration_config["kubeconfig"])
    else:
        config.load_incluster_config()

    return K8sTestClient(
        namespace=k8s_integration_config["namespace"],
        timeout=k8s_integration_config["test_timeout"]
    )

@pytest.fixture
def test_serviceaccounts(k8s_client):
    """Create test ServiceAccounts with different permission levels"""
    return k8s_client.create_test_serviceaccounts([
        {"name": "test-admin", "cluster_role": "cluster-admin"},
        {"name": "test-viewer", "cluster_role": "view"},
        {"name": "test-holmesgpt", "cluster_role": "holmesgpt-investigator"}
    ])
```

---

## 📊 **Test Execution Strategy**

### **Continuous Integration** *(Following existing CI patterns)*

```yaml
# .github/workflows/holmesgpt-api-tests.yml
name: HolmesGPT API Tests
on: [push, pull_request]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Run Unit Tests
        run: make test-holmesgpt-api-unit

  integration-tests:
    runs-on: ubuntu-latest
    needs: unit-tests
    steps:
      - uses: actions/checkout@v4
      - name: Setup Kind
        uses: helm/kind-action@v1
      - name: Run Integration Tests
        run: make test-holmesgpt-api-integration
        env:
          CI: true
          USE_MOCK_LLM: true

  e2e-tests:
    runs-on: ubuntu-latest
    needs: integration-tests
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    steps:
      - uses: actions/checkout@v4
      - name: Run E2E Tests
        run: make test-holmesgpt-api-e2e
```

### **Local Development** *(Matching existing patterns)*

```bash
# Quick development cycle
make test-holmesgpt-api-unit                    # Fast feedback (30s)
make test-holmesgpt-api-integration             # Medium cycle (5min) - with real LLM if available
make test-holmesgpt-api-integration-mock-llm    # Medium cycle (5min) - with mock LLM only
make test-holmesgpt-api-e2e                     # Full validation (15min)

# Test with specific LLM configurations
LLM_ENDPOINT=http://localhost:11434 LLM_PROVIDER=ollama make test-holmesgpt-api-integration    # Ollama
LLM_ENDPOINT=http://localhost:8080 LLM_PROVIDER=localai make test-holmesgpt-api-integration   # LocalAI
USE_MOCK_LLM=true make test-holmesgpt-api-integration                                          # Mock LLM

# Debug specific scenarios
cd docker/holmesgpt-api
PYTHONPATH=./src python3 -m pytest tests/integration/test_k8s_auth_integration.py::test_valid_serviceaccount_token_authentication -v -s

# Debug LLM integration scenarios
PYTHONPATH=./src python3 -m pytest tests/integration/test_llm_integration.py::test_investigation_with_real_llm -v -s
```

---

## 🎯 **Success Criteria & Validation**

### **Completion Criteria**
- [ ] **BR-HAPI-045** fully covered with integration tests
- [ ] **BR-HAPI-046** adaptive LLM support implemented and tested
- [ ] All test levels passing in CI/CD pipeline
- [ ] Real K8s ServiceAccount token validation working
- [ ] RBAC-to-OAuth2 scope mapping validated
- [ ] Real LLM integration working when available
- [ ] Mock LLM fallback working for CI/CD reliability
- [ ] LLM provider auto-detection functioning
- [ ] Performance benchmarks established for both LLM types
- [ ] Documentation updated with test procedures

### **Quality Gates**
- **Unit Tests**: 100% passing (✅ Current: 129/129)
- **Integration Tests**: 100% passing (🔄 To implement)
- **E2E Tests**: 100% passing (🔄 To implement)
- **Performance**: Token validation < 100ms (📏 To establish)
- **Coverage**: BR-HAPI-045 requirements 100% covered

### **Business Validation**
- ✅ Real K8s ServiceAccount tokens authenticate successfully
- ✅ RBAC permissions correctly map to OAuth2 scopes
- ✅ Scope-based authorization works for all API endpoints
- ✅ Real LLM integration provides quality responses when available
- ✅ Mock LLM fallback ensures reliable testing in CI/CD
- ✅ LLM provider auto-detection works across different endpoints
- ✅ Graceful degradation when LLM services are unavailable
- ✅ Integration with existing Kubernaut ecosystem validated
- ✅ No regression in existing functionality

---

## 🔄 **Implementation Timeline**

### **Week 1: Foundation** *(Priority: HIGH)*
- [ ] Create integration test infrastructure
- [ ] Implement basic K8s authentication tests
- [ ] Validate Kind cluster integration

### **Week 2: Core Features** *(Priority: HIGH)*
- [ ] Implement RBAC-to-scope mapping tests
- [ ] Create Bearer token authorization tests
- [ ] Add K8s API server integration tests

### **Week 3: System Integration** *(Priority: MEDIUM)*
- [ ] Implement full ecosystem E2E tests
- [ ] Add performance validation tests
- [ ] Create CI/CD pipeline integration

### **Week 4: Validation & Documentation** *(Priority: LOW)*
- [ ] Performance benchmarking
- [ ] Documentation updates
- [ ] Final validation and sign-off

---

## 📚 **References & Dependencies**

### **Project Guidelines Compliance**
- ✅ **Reuse Existing Infrastructure**: Leveraging Kind, PostgreSQL, test patterns
- ✅ **TDD Approach**: Test-first development with business requirements
- ✅ **No Duplication**: Building on existing test framework
- ✅ **Business Requirement Alignment**: All tests mapped to BR-HAPI-045
- ✅ **Integration Focus**: Tests validate real component interaction

### **Infrastructure Dependencies**
- **scripts/setup-kind-cluster.sh**: Kubernetes cluster provisioning
- **test/integration/scripts/bootstrap-integration-tests.sh**: Database services
- **test/shared/**: Mock and test utilities
- **Makefile**: Existing test execution patterns

### **Business Requirements**
- **BR-HAPI-045**: OAuth 2 resource server compatible with K8s API server
- **Project Guidelines**: Development and testing principles
- **Existing Test Coverage**: 129 unit tests (foundation)

---

**Status**: 📋 **READY FOR IMPLEMENTATION**
**Next Steps**: Begin Phase 1 implementation with existing infrastructure
