# 🧪 **LLM Integration Testing Scenarios**

**Business Requirement**: BR-HAPI-046 - Integration tests with adaptive LLM support

Following **Project Guidelines**: Modular testing, infrastructure reuse, business requirement alignment

---

## 🎯 **Three Testing Scenarios Overview**

### **Scenario 1: LLM-Only Testing** *(Independent)*
- **Purpose**: Test LLM functionality without K8s dependencies
- **Use Case**: LLM provider validation, connectivity testing, performance benchmarking
- **Infrastructure**: None (pure LLM testing)
- **Files**: `test_llm_only.py`

### **Scenario 2: K8s + Mock LLM** *(K8s Focus)*
- **Purpose**: Test K8s authentication/authorization without real LLM dependency
- **Use Case**: CI/CD pipelines, RBAC validation, ServiceAccount testing
- **Infrastructure**: Kind cluster + Mock LLM
- **Files**: `test_k8s_mock_llm.py`

### **Scenario 3: Full Integration** *(Complete Ecosystem)*
- **Purpose**: Test complete ecosystem with real K8s + real/mock LLM
- **Use Case**: End-to-end validation, performance testing, production readiness
- **Infrastructure**: Kind cluster + Real LLM (with mock fallback)
- **Files**: `test_full_integration.py`

---

## 🚀 **Usage Examples**

### **Scenario 1: LLM-Only Testing**

```bash
# Test against Ollama at remote endpoint
LLM_ENDPOINT=http://192.168.1.169:8080 LLM_PROVIDER=ollama USE_MOCK_LLM=false \
PYTHONPATH=./src python3 -m pytest tests/integration/test_llm_only.py -v

# Test with auto-detection (falls back to mock if unavailable)
LLM_ENDPOINT=http://localhost:8080 LLM_PROVIDER=auto-detect \
PYTHONPATH=./src python3 -m pytest tests/integration/test_llm_only.py -v

# Test with mock LLM only
USE_MOCK_LLM=true \
PYTHONPATH=./src python3 -m pytest tests/integration/test_llm_only.py -v
```

### **Scenario 2: K8s + Mock LLM**

```bash
# Prerequisites: Kind cluster running
# ./scripts/setup-kind-cluster.sh

# Test K8s authentication with mock LLM (fast, reliable)
KUBECONFIG=~/.kube/config USE_MOCK_LLM=true \
PYTHONPATH=./src python3 -m pytest tests/integration/test_k8s_mock_llm.py -v

# Test with specific namespace
TEST_NAMESPACE=holmesgpt-test USE_MOCK_LLM=true \
PYTHONPATH=./src python3 -m pytest tests/integration/test_k8s_mock_llm.py -v
```

### **Scenario 3: Full Integration**

```bash
# Prerequisites: Kind cluster + LLM endpoint available
# ./scripts/setup-kind-cluster.sh

# Test complete ecosystem with real LLM
LLM_ENDPOINT=http://192.168.1.169:8080 LLM_PROVIDER=ollama \
KUBECONFIG=~/.kube/config \
PYTHONPATH=./src python3 -m pytest tests/integration/test_full_integration.py -v

# Test with auto-detection (graceful fallback to mock if LLM unavailable)
LLM_ENDPOINT=http://192.168.1.169:8080 LLM_PROVIDER=auto-detect \
KUBECONFIG=~/.kube/config \
PYTHONPATH=./src python3 -m pytest tests/integration/test_full_integration.py -v

# CI/CD mode (forces mock LLM)
CI=true KUBECONFIG=~/.kube/config \
PYTHONPATH=./src python3 -m pytest tests/integration/test_full_integration.py -v
```

---

## 🔧 **Configuration Variables**

### **LLM Configuration**
- `LLM_ENDPOINT`: LLM server endpoint (default: `http://localhost:8080`)
- `LLM_PROVIDER`: `ollama`, `localai`, `mock`, or `auto-detect` (default: `auto-detect`)
- `LLM_MODEL`: Model name (default: `granite3.1-dense:8b`)
- `USE_MOCK_LLM`: Force mock LLM usage (`true`/`false`, default: `false`)

### **Kubernetes Configuration**
- `KUBECONFIG`: Path to kubeconfig file
- `TEST_NAMESPACE`: K8s namespace for testing (default: `holmesgpt`)
- `USE_FAKE_K8S_CLIENT`: Use fake K8s client (`true`/`false`, default: `false`)

### **Test Configuration**
- `CI`: CI/CD mode flag (forces mock LLM for reliability)
- `GITHUB_ACTIONS`: GitHub Actions CI detection
- `TEST_TIMEOUT`: Test timeout in seconds (default: `120`)
- `LOG_LEVEL`: Logging level (default: `INFO`)

---

## 🎯 **Test Markers**

Use pytest markers to run specific test categories:

```bash
# Run only LLM-only tests
pytest -m "llm_only"

# Run only K8s tests with mock LLM
pytest -m "mock_llm"

# Run only full integration tests
pytest -m "llm and k8s"

# Run all LLM-related tests
pytest -m "llm or llm_only"

# Skip slow tests
pytest -m "not slow"
```

---

## 🧩 **Key Features**

### **✅ Graceful Degradation** *(BR-HAPI-046.4)*
- **Real LLM Available**: Uses specified provider (`ollama`, `localai`)
- **Real LLM Unavailable**: Automatically falls back to mock LLM
- **Explicit Mock**: Always uses mock when `USE_MOCK_LLM=true`

### **✅ Auto-Detection** *(BR-HAPI-046.3)*
- **Port 11434**: Auto-detects Ollama
- **Port 8080**: Auto-detects LocalAI
- **Unavailable**: Falls back to mock
- **Override**: Explicit `LLM_PROVIDER` setting honored

### **✅ Environment Awareness**
- **CI/CD Mode**: Automatically uses mock LLM for reliability
- **Development Mode**: Uses real LLM when available
- **Hybrid Mode**: Real LLM with mock fallback

### **✅ Independent Testing**
- **LLM-Only**: No K8s dependencies
- **K8s-Only**: No real LLM dependencies
- **Full Integration**: Complete ecosystem validation

---

## 🔍 **Business Requirements Coverage**

| Requirement | Scenario 1 | Scenario 2 | Scenario 3 |
|-------------|------------|------------|------------|
| **BR-HAPI-046.1** Real LLM Integration | ✅ | ❌ | ✅ |
| **BR-HAPI-046.2** Mock LLM Fallback | ✅ | ✅ | ✅ |
| **BR-HAPI-046.3** Auto-Detection | ✅ | ❌ | ✅ |
| **BR-HAPI-046.4** Graceful Degradation | ✅ | ✅ | ✅ |
| **BR-HAPI-046.5** Performance Testing | ✅ | ❌ | ✅ |
| **BR-HAPI-045.x** K8s Integration | ❌ | ✅ | ✅ |

---

## 📋 **Development Workflow**

### **Local Development**
1. **LLM Development**: Use Scenario 1 for fast LLM iteration
2. **K8s Development**: Use Scenario 2 for K8s auth/RBAC work
3. **Integration Testing**: Use Scenario 3 for end-to-end validation

### **CI/CD Pipeline**
1. **Unit Tests**: Fast feedback loop
2. **Scenario 1**: LLM functionality (mock fallback)
3. **Scenario 2**: K8s integration (mock LLM)
4. **Scenario 3**: Full integration (mock LLM for reliability)

### **Production Validation**
1. **Staging**: Scenario 3 with real LLM endpoint
2. **Pre-production**: All scenarios with real infrastructure
3. **Production**: Health checks and monitoring

---

## 🛠️ **Troubleshooting**

### **Common Issues**

#### **LLM Endpoint Unavailable**
```
Warning: LLM endpoint http://192.168.1.169:8080 not available, falling back to mock
✅ Graceful degradation: ollama → mock (endpoint unavailable)
```
**Solution**: This is expected behavior. System gracefully degrades to mock LLM.

#### **K8s Authentication Failures**
```
Cannot connect to Kubernetes: Invalid kube-config file
```
**Solution**:
- Ensure Kind cluster is running: `./scripts/setup-kind-cluster.sh`
- Check KUBECONFIG path: `echo $KUBECONFIG`
- For LLM-only testing, use Scenario 1

#### **Service Account Token Issues**
```
No ServiceAccount tokens available for authentication testing
```
**Solution**:
- Wait for SA token creation (may take 30s after cluster setup)
- Check SA creation: `kubectl get sa -n holmesgpt`
- For K8s 1.24+, tokens are created on-demand

---

## 📈 **Performance Expectations**

| Scenario | Setup Time | Test Duration | Resource Usage |
|----------|------------|---------------|----------------|
| **LLM-Only** | < 1s | 2-5s (mock) / 10-30s (real) | Minimal |
| **K8s + Mock** | 30-60s (cluster) | 5-15s | Medium |
| **Full Integration** | 30-60s (cluster) | 15-45s | High |

**Note**: First Kind cluster setup takes longer (~2-3 minutes). Subsequent runs reuse existing cluster.

---

*Following Project Guidelines: Test business requirements, avoid implementation testing, ensure graceful error handling, maximize infrastructure reuse.*

