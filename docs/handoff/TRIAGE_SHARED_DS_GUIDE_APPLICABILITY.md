# TRIAGE: Shared DataStorage Configuration Guide Applicability to Gateway Integration Tests

**Date**: 2025-12-12
**Type**: Applicability Analysis
**Priority**: 🔴 **CRITICAL** - Determines if guide solves Gateway's issues
**Status**: ✅ **COMPLETE**

---

## 🎯 **USER QUESTION**

> "triage if this can help solve the current issues"
>
> Referring to: `docs/handoff/SHARED_DATASTORAGE_CONFIGURATION_GUIDE.md`

---

## ✅ **TRIAGE RESULT: NOT APPLICABLE** ❌

**Summary**: The Shared DataStorage Configuration Guide is **NOT applicable** to Gateway's current integration test issues because it targets a **different test tier** with **different infrastructure**.

---

## 🔍 **DETAILED ANALYSIS**

### **What the Guide Covers**

**Target Tier**: **E2E Tests**
**Infrastructure**: **Kubernetes (Kind clusters)**
**Pattern**: **ConfigMaps + Secrets + volumeMounts + K8s Deployments**

**From Guide**:
```yaml
# E2E Test Pattern (Kubernetes)
apiVersion: v1
kind: ConfigMap
metadata:
  name: datastorage-config
data:
  config.yaml: |
    database:
      host: postgresql    # K8s Service name
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
spec:
  template:
    spec:
      containers:
      - name: datastorage
        volumeMounts:
        - name: config
          mountPath: /etc/datastorage
      volumes:
      - name: config
        configMap:
          name: datastorage-config
```

**Key Characteristics**:
- ✅ Kubernetes native (ConfigMaps, Secrets, Deployments)
- ✅ Kind cluster deployment
- ✅ K8s service discovery (`postgresql`, `redis`)
- ✅ Helm/kubectl for orchestration
- ✅ Production-like infrastructure

---

### **What Gateway Integration Tests Actually Use**

**Target Tier**: **Integration Tests**
**Infrastructure**: **envtest + Podman containers**
**Pattern**: **podman-compose + wrapper functions**

**From** `test/integration/gateway/suite_test.go`:
```go
// Gateway Integration Test Setup
suiteLogger.Info("Gateway Integration Test Suite - envtest Setup")
suiteLogger.Info("  • envtest (in-memory K8s API server)")           // ← NOT Kind
suiteLogger.Info("  • RemediationRequest CRD (cluster-wide)")
suiteLogger.Info("  • Data Storage infrastructure (PostgreSQL + Redis + DS)")
suiteLogger.Info("  • Using shared infrastructure pattern")

// Current (broken) approach
dsInfra, err := infrastructure.StartDataStorageInfrastructure(nil, GinkgoWriter)  // ← Programmatic Podman
```

**Key Characteristics**:
- ✅ envtest (not Kind)
- ✅ Podman containers (not K8s deployments)
- ❌ Currently using programmatic Podman (broken)
- ✅ Should use podman-compose (AIAnalysis pattern)

---

## 📊 **TEST TIER COMPARISON**

| Aspect | E2E Tests (Guide) | Integration Tests (Gateway) |
|--------|------------------|----------------------------|
| **Infrastructure** | Kind cluster | envtest + Podman |
| **K8s Components** | Full cluster | In-memory API only |
| **DataStorage Deploy** | K8s Deployment | Podman container |
| **Config Method** | ConfigMap | podman-compose YAML |
| **Service Discovery** | K8s DNS | localhost ports |
| **Pattern** | ADR-030 K8s Config | AIAnalysis podman-compose |
| **Startup Time** | 2-5 minutes | 15-60 seconds |
| **Guide Applicable** | ✅ YES | ❌ NO |

---

## 🚨 **WHY GUIDE DOESN'T APPLY**

### **Infrastructure Mismatch**

**Guide Uses**:
```yaml
# Kubernetes Deployment (E2E)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
spec:
  template:
    spec:
      containers:
      - name: datastorage
        env:
        - name: CONFIG_PATH
          value: /etc/datastorage/config.yaml
        volumeMounts:
        - name: config
          mountPath: /etc/datastorage
```

**Gateway Needs**:
```yaml
# podman-compose (Integration)
version: '3.8'
services:
  datastorage:
    image: localhost/kubernaut-datastorage:latest
    ports:
      - "58080:8080"
    environment:
      - CONFIG_PATH=/etc/datastorage/config.yaml
    volumes:
      - ./config.yaml:/etc/datastorage/config.yaml
```

**Key Difference**:
- Guide = **Kubernetes Deployment** (kubectl apply)
- Gateway = **Podman container** (podman-compose up)

---

### **Test Tier Mismatch**

**Guide's Test Tier** (E2E):
```
┌─────────────────────────────────┐
│ Kind Cluster (Full K8s)         │
│                                 │
│  ┌────────────────────────┐    │
│  │ PostgreSQL Deployment   │    │
│  │ Service: postgresql     │    │
│  └────────────────────────┘    │
│                                 │
│  ┌────────────────────────┐    │
│  │ DataStorage Deployment  │    │
│  │ ConfigMap: config.yaml  │    │
│  │ Secret: db-secrets      │    │
│  └────────────────────────┘    │
│                                 │
│  ┌────────────────────────┐    │
│  │ Gateway Deployment      │    │
│  └────────────────────────┘    │
└─────────────────────────────────┘
```

**Gateway's Test Tier** (Integration):
```
┌─────────────────────────────────┐
│ envtest (In-Memory K8s API)     │
│  - RemediationRequest CRD       │
│  - No deployments, services     │
└─────────────────────────────────┘
         ↓
┌─────────────────────────────────┐
│ Podman Containers (Host)        │
│  - postgres:16-alpine           │
│  - redis:7-alpine               │
│  - datastorage:latest           │
│  (Started via podman-compose)   │
└─────────────────────────────────┘
```

---

## ✅ **WHAT GATEWAY ACTUALLY NEEDS**

### **Correct Pattern: AIAnalysis podman-compose**

**From** `test/integration/aianalysis/podman-compose.yml`:
```yaml
version: '3.8'
services:
  postgres:
    image: postgres:16-alpine
    ports: ["15434:5432"]
    environment:
      POSTGRES_USER: slm_user
      POSTGRES_PASSWORD: test_password
      POSTGRES_DB: action_history
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U slm_user -d action_history"]
      interval: 5s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    ports: ["16380:6379"]
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]

  datastorage:
    image: localhost/kubernaut-datastorage:latest
    depends_on: [postgres, redis]
    ports: ["18091:8080"]
    environment:
      CONFIG_PATH: /etc/datastorage/config.yaml
    volumes:
      - ./config/config.yaml:/etc/datastorage/config.yaml
      - ./config/db-secrets.yaml:/etc/datastorage/secrets/db-secrets.yaml
```

**Wrapper Function** (from `test/infrastructure/aianalysis.go`):
```go
func StartAIAnalysisIntegrationInfrastructure(writer io.Writer) error {
    composeFile := filepath.Join(projectRoot, "test/integration/aianalysis/podman-compose.yml")

    cmd := exec.Command("podman-compose",
        "-f", composeFile,
        "-p", "aianalysis-test",
        "up", "-d", "--build")

    if err := cmd.Run(); err != nil {
        return fmt.Errorf("failed to start podman-compose: %w", err)
    }

    // Wait for health
    return waitForHTTPHealth("http://localhost:18091/health", 60*time.Second)
}
```

---

## 📋 **APPLICABILITY SUMMARY**

| Question | Answer | Evidence |
|----------|--------|----------|
| **Does guide apply to Gateway integration tests?** | ❌ **NO** | Different test tier (E2E vs Integration) |
| **Does guide solve PostgreSQL race condition?** | ❌ **NO** | Uses K8s readiness probes, not Podman health checks |
| **Does guide solve migration path issues?** | ❌ **NO** | K8s uses init containers, not podman-compose |
| **Does guide solve pgvector cleanup?** | ❌ **NO** | K8s doesn't use pgvector for Gateway |
| **Does guide solve programmatic Podman issues?** | ❌ **NO** | K8s doesn't use Podman at all |
| **What DOES guide apply to?** | ✅ **E2E Tests** | Kind cluster deployments for E2E testing |

---

## 🎯 **CORRECT SOLUTION FOR GATEWAY**

### **Gateway Must Use: AIAnalysis Pattern (podman-compose)**

**Why**:
1. **Test Tier Match**: Integration tests → podman-compose (not K8s)
2. **Proven**: 4 services successfully use it
3. **Fast**: <60 seconds vs K8s 2-5 minutes
4. **Simple**: Declarative infrastructure vs programmatic
5. **Authoritative**: Per ADR-016 + TRIAGE_INTEGRATION_TEST_INFRASTRUCTURE_PATTERNS.md

**Implementation**:
1. Create `test/integration/gateway/podman-compose.gateway.test.yml`
2. Create `infrastructure.StartGatewayIntegrationInfrastructure()` wrapper
3. Update `suite_test.go` to call wrapper
4. Remove `StartDataStorageInfrastructure()` usage

**Time**: ~1 hour
**Confidence**: 100% (proven across 4 services)

---

## 📖 **WHEN TO USE THE GUIDE**

### **✅ Guide IS Applicable For**:

| Service | Test Tier | Infrastructure | Applicability |
|---------|-----------|----------------|---------------|
| **Gateway E2E** | E2E | Kind cluster | ✅ **YES** - Use guide for E2E tests |
| **AIAnalysis E2E** | E2E | Kind cluster | ✅ **YES** - Already uses guide pattern |
| **WorkflowExecution E2E** | E2E | Kind cluster | ✅ **YES** - Already uses guide pattern |

### **❌ Guide NOT Applicable For**:

| Service | Test Tier | Infrastructure | Reason |
|---------|-----------|----------------|--------|
| **Gateway Integration** | Integration | envtest + Podman | ❌ Different infrastructure (not K8s) |
| **AIAnalysis Integration** | Integration | Podman only | ❌ No K8s (uses podman-compose) |
| **SignalProcessing Integration** | Integration | envtest + Podman | ❌ Uses podman-compose pattern |

---

## 🔗 **CORRECT DOCUMENTATION FOR GATEWAY**

### **For Integration Tests** (Current Issue):
- **Pattern**: AIAnalysis podman-compose
- **Doc**: `docs/handoff/TRIAGE_INTEGRATION_TEST_INFRASTRUCTURE_PATTERNS.md` ⭐
- **Reference**: `test/integration/aianalysis/podman-compose.yml`
- **Authority**: ADR-016 (Service-Specific Infrastructure)

### **For E2E Tests** (Future):
- **Pattern**: K8s ConfigMaps + Deployments
- **Doc**: `docs/handoff/SHARED_DATASTORAGE_CONFIGURATION_GUIDE.md` ⭐
- **Reference**: `test/infrastructure/datastorage.go` (K8s deployment functions)
- **Authority**: ADR-030 (Service Configuration Management)

---

## ✅ **FINAL RECOMMENDATION**

**For Gateway's Current Integration Test Issues**:

| Guide | Applicability | Reason |
|-------|--------------|--------|
| **SHARED_DATASTORAGE_CONFIGURATION_GUIDE.md** | ❌ **NOT APPLICABLE** | E2E/K8s pattern, not Integration/Podman |
| **TRIAGE_INTEGRATION_TEST_INFRASTRUCTURE_PATTERNS.md** | ✅ **AUTHORITATIVE** | Integration/Podman pattern ⭐ |

**Action**:
1. ❌ DON'T follow K8s ConfigMap guide (wrong tier)
2. ✅ DO follow AIAnalysis podman-compose pattern
3. ✅ DO create `podman-compose.gateway.test.yml`
4. ✅ DO create `StartGatewayIntegrationInfrastructure()` wrapper

---

## 📊 **CONCLUSION**

**Question**: "Can SHARED_DATASTORAGE_CONFIGURATION_GUIDE help solve Gateway integration test issues?"

**Answer**: **NO** ❌

**Reason**:
- Guide targets **E2E tests with Kind/K8s**
- Gateway integration tests use **envtest + Podman containers**
- Different infrastructure requires different pattern

**Correct Solution**:
- Use **AIAnalysis podman-compose pattern**
- Documented in: **TRIAGE_INTEGRATION_TEST_INFRASTRUCTURE_PATTERNS.md**

**Confidence**: 100% (proven pattern analysis)

---

**Document Status**: ✅ **COMPLETE**
**Recommendation**: Use AIAnalysis pattern, not E2E K8s pattern
**Next Step**: Create `podman-compose.gateway.test.yml`





