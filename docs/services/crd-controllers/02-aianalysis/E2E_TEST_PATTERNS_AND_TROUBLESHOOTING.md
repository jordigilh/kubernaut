# AIAnalysis E2E Test Patterns and Troubleshooting Guide

**Service**: AIAnalysis Controller
**Test Suite**: `test/e2e/aianalysis/`
**Version**: 1.0
**Last Updated**: December 13, 2025
**Status**: Active

---

## 📋 **Table of Contents**

1. [E2E Test Architecture](#e2e-test-architecture)
2. [Test Patterns](#test-patterns)
3. [Infrastructure Setup](#infrastructure-setup)
4. [Troubleshooting Guide](#troubleshooting-guide)
5. [Common Issues](#common-issues)
6. [Debugging Tools](#debugging-tools)
7. [Best Practices](#best-practices)

---

## 🏗️ **E2E Test Architecture**

### **Test Pyramid Position**

```
         /\
        /  \  E2E (10-15%) - Complete workflows
       /____\
      /      \  Integration (>50%) - Service interactions
     /________\
    /          \ Unit (70%+) - Business logic
   /____________\
```

**E2E Tests**: 22 tests covering critical user journeys with real infrastructure.

### **Cluster Architecture**

```
┌─────────────────────────────────────────────────────────┐
│ KIND Cluster: aianalysis-e2e                            │
│                                                          │
│  ┌────────────┐  ┌──────────────┐  ┌─────────────────┐ │
│  │ PostgreSQL │◄─┤ DataStorage  │◄─┤ AIAnalysis      │ │
│  │ (audit)    │  │ (workflows)  │  │ Controller      │ │
│  └────────────┘  └──────────────┘  └─────────────────┘ │
│                                              ▲           │
│  ┌────────────┐  ┌──────────────┐           │           │
│  │ Redis      │◄─┤ HolmesGPT-API│───────────┘           │
│  │ (cache)    │  │ (AI analysis)│ (mock LLM)            │
│  └────────────┘  └──────────────┘                       │
│                                                          │
│  NodePorts:                                              │
│  • AIAnalysis Health: 30284 → localhost:8184            │
│  • AIAnalysis Metrics: 30184 → localhost:9184           │
│  • AIAnalysis API: 30084 → localhost:8084               │
└─────────────────────────────────────────────────────────┘
```

### **Port Allocation (per DD-TEST-001)**

| Service | Internal Port | NodePort | Localhost | Purpose |
|---------|---------------|----------|-----------|---------|
| AIAnalysis API | 8084 | 30084 | 8084 | CRD API (unused in E2E) |
| AIAnalysis Health | 8081 | 30284 | 8184 | `/healthz`, `/readyz` |
| AIAnalysis Metrics | 9090 | 30184 | 9184 | `/metrics` (Prometheus) |
| DataStorage | 8080 | - | - | Internal only |
| HolmesGPT-API | 8080 | - | - | Internal only |
| PostgreSQL | 5432 | - | - | Internal only |
| Redis | 6379 | - | - | Internal only |

---

## 🎯 **Test Patterns**

### **Test Organization**

```
test/e2e/aianalysis/
├── suite_test.go                     # Setup/teardown, cluster lifecycle
├── 01_health_endpoints_test.go       # Health/readiness checks (6 tests)
├── 02_metrics_test.go                # Prometheus metrics (6 tests)
├── 03_full_flow_test.go              # Complete investigation flow (5 tests)
└── 04_recovery_flow_test.go          # Recovery attempt handling (5 tests)
```

**Naming Convention**: `NN_description_test.go` where NN is execution order.

### **Suite Setup Pattern (suite_test.go)**

```go
var _ = SynchronizedBeforeSuite(
    // Process 1: Create cluster (runs ONCE)
    func() []byte {
        clusterName = "aianalysis-e2e"
        kubeconfigPath = fmt.Sprintf("%s/.kube/aianalysis-e2e-config", homeDir)

        // Create KIND cluster with full dependency chain
        err := infrastructure.CreateAIAnalysisCluster(clusterName, kubeconfigPath, GinkgoWriter)
        Expect(err).ToNot(HaveOccurred())

        return []byte(kubeconfigPath) // Share with all processes
    },
    // All processes: Connect to cluster
    func(data []byte) {
        kubeconfigPath = string(data)
        os.Setenv("KUBECONFIG", kubeconfigPath)

        // Create Kubernetes client
        cfg, err := config.GetConfig()
        k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})

        // Wait for services to be ready
        Eventually(func() bool {
            return checkServicesReady()
        }, 3*time.Minute, 5*time.Second).Should(BeTrue())
    },
)
```

**Key Pattern**: Process 1 creates infrastructure, all processes share kubeconfig.

### **Cluster Preservation Pattern**

```go
var _ = SynchronizedAfterSuite(
    // All processes: Cleanup context
    func() {
        if cancel != nil {
            cancel()
        }
    },
    // Process 1: Conditional cleanup
    func() {
        // Preserve cluster if ANY test failed
        if anyTestFailed || os.Getenv("SKIP_CLEANUP") == "true" {
            logger.Info("⚠️  Keeping cluster alive for debugging")
            logger.Info(fmt.Sprintf("  export KUBECONFIG=%s", kubeconfigPath))
            return
        }

        // All passed - cleanup
        err := infrastructure.DeleteAIAnalysisCluster(clusterName, kubeconfigPath)
    },
)
```

**Key Pattern**: Preserve failed test clusters for forensic analysis.

### **Test Structure Pattern**

```go
var _ = Describe("Full Investigation Flow", func() {
    var (
        ctx       context.Context
        namespace string
        analysis  *aianalysisv1alpha1.AIAnalysis
    )

    BeforeEach(func() {
        ctx = context.Background()
        namespace = "kubernaut-system"

        // Create test AIAnalysis CRD
        analysis = &aianalysisv1alpha1.AIAnalysis{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("test-analysis-%s", randomSuffix()),
                Namespace: namespace,
            },
            Spec: aianalysisv1alpha1.AIAnalysisSpec{
                // ... spec fields
            },
        }
    })

    Context("When investigating production incident", func() {
        It("should complete investigation and select workflow", func() {
            // Create CRD
            Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

            // Wait for phase transitions
            Eventually(func() string {
                Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
                return analysis.Status.Phase
            }, 30*time.Second, 1*time.Second).Should(Equal("Investigating"))

            // Verify workflow selection
            Eventually(func() bool {
                Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
                return analysis.Status.SelectedWorkflow != nil
            }, 45*time.Second, 2*time.Second).Should(BeTrue())
        })
    })
})
```

**Key Patterns**:
- Use `randomSuffix()` for unique resource names (parallel execution)
- Use `Eventually` with reasonable timeouts for async operations
- Always verify CRD status fields, not just phase

---

## 🚀 **Infrastructure Setup**

### **Build Times (Important for Timeouts)**

| Service | Language | Build Time | Reason |
|---------|----------|------------|--------|
| DataStorage | Go | 2-3 min | Compilation |
| **HolmesGPT-API** | **Python (UBI9)** | **10-15 min** | **pip packages (bottleneck)** |
| AIAnalysis | Go | 2-3 min | Compilation |
| **Total Setup** | - | **15-20 min** | **Image builds + deployments** |

**Critical**: Ginkgo timeout must be ≥30 minutes to account for Python builds.

### **Current Makefile Configuration**

```makefile
# test/e2e/aianalysis target
test-e2e-aianalysis:
    ginkgo -v --timeout=30m --procs=4 ./test/e2e/aianalysis/...
```

**Rationale**: 30-minute timeout accounts for:
- 15-20 min: Image builds
- 5 min: Service startup
- 5 min: Test execution

### **Infrastructure Creation**

```go
// test/infrastructure/aianalysis.go
func CreateAIAnalysisCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
    // 1. Create KIND cluster (1 min)
    CreateKindCluster(clusterName, kubeconfigPath)

    // 2. Build images (15-20 min)
    //    - DataStorage (2-3 min)
    //    - HolmesGPT-API (10-15 min) ← BOTTLENECK
    //    - AIAnalysis (2-3 min)
    BuildImages(writer)

    // 3. Load images to cluster (2 min)
    LoadImages(clusterName)

    // 4. Deploy services (2-3 min)
    //    - PostgreSQL + Redis
    //    - DataStorage
    //    - HolmesGPT-API (with MOCK_LLM_MODE=true)
    //    - AIAnalysis Controller
    DeployServices(kubeconfigPath, writer)

    return nil
}
```

### **Environment Variables for HAPI Mock Mode**

```yaml
# Critical configuration for E2E tests
env:
- name: LLM_PROVIDER
  value: mock                    # Use mock LLM
- name: LLM_MODEL
  value: mock://test-model       # Mock model identifier
- name: MOCK_LLM_MODE
  value: "true"                  # Enable mock responses (CRITICAL)
- name: DATASTORAGE_URL
  value: http://datastorage:8080 # Internal service URL
```

**Critical**: `MOCK_LLM_MODE=true` must be set for deterministic test responses.

---

## 🔍 **Troubleshooting Guide**

### **Symptom 1: Tests Timeout During Setup**

**Error Message**:
```
Timed out after 20m while waiting for infrastructure
```

**Root Cause**: HolmesGPT-API Python image takes 10-15 minutes to build.

**Solution**:
1. Verify Ginkgo timeout: `--timeout=30m` (not 20m)
2. Check Podman machine resources: `podman machine inspect`
3. Consider image caching:
   ```bash
   # Pre-build image locally
   podman build -t localhost/kubernaut-holmesgpt-api:latest \
     -f holmesgpt-api/Dockerfile .
   ```

**Prevention**: Document expected build times in test output.

---

### **Symptom 2: "No workflow selected" Errors**

**Error Message (Controller Logs)**:
```
ERROR  No workflow selected - investigation may have failed
DEBUG  HAPI did not return recovery_analysis, skipping RecoveryStatus population
```

**Root Cause**: HAPI mock mode not activating (missing `selected_workflow` in response).

**Diagnostic Steps**:

1. **Verify HAPI pod is running**:
   ```bash
   kubectl get pods -n kubernaut-system -l app=holmesgpt-api
   ```

2. **Check HAPI logs for mock mode activation**:
   ```bash
   kubectl logs -n kubernaut-system deployment/holmesgpt-api | grep mock_mode
   ```

   **Expected** (if working):
   ```json
   {"event": "mock_mode_active", "incident_id": "..."}
   ```

   **Actual** (if broken):
   ```
   INFO:  10.244.1.6:49204 - "POST /api/v1/incident/analyze HTTP/1.1" 200 OK
   ```

3. **Verify environment variable in pod**:
   ```bash
   kubectl exec -n kubernaut-system deployment/holmesgpt-api -- env | grep MOCK
   ```

   **Expected**:
   ```
   MOCK_LLM_MODE=true
   LLM_PROVIDER=mock
   LLM_MODEL=mock://test-model
   ```

4. **Test HAPI manually from controller pod**:
   ```bash
   kubectl exec -n kubernaut-system deployment/aianalysis-controller -- \
     curl -s http://holmesgpt-api:8080/api/v1/incident/analyze -d '{
       "incident_id": "test-123",
       "signal_type": "OOMKilled",
       "severity": "critical"
     }'
   ```

**Solutions**:
- **If env var missing**: Check deployment YAML in `test/infrastructure/aianalysis.go`
- **If mock mode not activating**: Request enhanced logging from HAPI team
- **If response incomplete**: Verify HAPI mock response structure

---

### **Symptom 3: Network Connectivity Failures**

**Error Message**:
```
Failed to connect to holmesgpt-api:8080
```

**Diagnostic Steps**:

1. **Check service DNS resolution**:
   ```bash
   kubectl exec -n kubernaut-system deployment/aianalysis-controller -- \
     nslookup holmesgpt-api
   ```

2. **Check service endpoints**:
   ```bash
   kubectl get endpoints -n kubernaut-system holmesgpt-api
   ```

3. **Check network policies** (if any):
   ```bash
   kubectl get networkpolicies -n kubernaut-system
   ```

**Solution**: Services should be in same namespace with no network policies in E2E.

---

### **Symptom 4: Health Endpoint Not Ready**

**Error Message**:
```
Eventually failed: Service health check returned 503
```

**Diagnostic Steps**:

1. **Check pod readiness**:
   ```bash
   kubectl get pods -n kubernaut-system
   ```

2. **Check pod logs**:
   ```bash
   kubectl logs -n kubernaut-system deployment/aianalysis-controller --tail=100
   ```

3. **Check health endpoint directly**:
   ```bash
   curl http://localhost:8184/healthz
   curl http://localhost:8184/readyz
   ```

**Common Causes**:
- DataStorage dependency not ready
- Kubernetes API server connection issues
- Audit client initialization failures

---

## 🛠️ **Debugging Tools**

### **Cluster State Inspection**

```bash
# Set kubeconfig
export KUBECONFIG=~/.kube/aianalysis-e2e-config

# Check all pods
kubectl get pods -n kubernaut-system

# Check services
kubectl get svc -n kubernaut-system

# Check CRDs
kubectl get aianalyses -n kubernaut-system

# Describe specific AIAnalysis
kubectl describe aianalysis -n kubernaut-system test-analysis-123
```

### **Log Collection**

```bash
# AIAnalysis controller logs
kubectl logs -n kubernaut-system deployment/aianalysis-controller -f

# HolmesGPT-API logs (mock mode)
kubectl logs -n kubernaut-system deployment/holmesgpt-api -f

# DataStorage logs
kubectl logs -n kubernaut-system deployment/datastorage -f

# Save logs for offline analysis
kubectl logs -n kubernaut-system deployment/holmesgpt-api > /tmp/hapi-logs.txt
```

### **Manual API Testing**

```bash
# Test HAPI from controller pod
kubectl exec -n kubernaut-system deployment/aianalysis-controller -- \
  curl -s http://holmesgpt-api:8080/api/v1/incident/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "incident_id": "manual-test-001",
    "remediation_id": "rem-test-001",
    "signal_type": "OOMKilled",
    "severity": "critical",
    "signal_source": "prometheus",
    "resource_namespace": "production",
    "resource_kind": "Pod",
    "resource_name": "test-pod",
    "error_message": "OOM killed",
    "environment": "production",
    "priority": "P0",
    "risk_tolerance": "low",
    "business_category": "payments",
    "cluster_name": "prod-us-east"
  }'
```

### **Environment Variable Verification**

```bash
# Verify MOCK_LLM_MODE in HAPI pod
kubectl exec -n kubernaut-system deployment/holmesgpt-api -- env | grep MOCK

# Verify all HAPI environment variables
kubectl exec -n kubernaut-system deployment/holmesgpt-api -- env | sort
```

---

## 📋 **Common Issues**

### **Issue 1: Stale Podman Port Bindings**

**Symptom**: Port already in use errors during cluster creation.

**Solution**:
```bash
# Find and kill processes using ports
lsof -ti:8184,8084,9184 | xargs kill -9

# Or restart Podman machine
podman machine stop
podman machine start
```

### **Issue 2: Leftover KIND Clusters**

**Symptom**: Cluster creation fails with "already exists" error.

**Solution**:
```bash
# List all KIND clusters
kind get clusters

# Delete old E2E cluster
kind delete cluster --name aianalysis-e2e
```

### **Issue 3: Image Not Loaded to Cluster**

**Symptom**: Pod stuck in `ImagePullBackOff`.

**Solution**:
```bash
# Verify images in cluster
podman exec -it aianalysis-e2e-control-plane crictl images | grep kubernaut

# Reload image manually
kind load docker-image localhost/kubernaut-aianalysis:latest --name aianalysis-e2e
```

---

## ✅ **Best Practices**

### **Test Design**

1. ✅ **Use Unique Resource Names**: Add `randomSuffix()` for parallel execution
2. ✅ **Set Reasonable Timeouts**: Account for async operations (30-45s for investigation)
3. ✅ **Verify Status Fields**: Don't just check phase, verify business data
4. ✅ **Use Eventually, Not Sleep**: Ginkgo `Eventually` with polling, not fixed delays
5. ✅ **Cleanup in AfterEach**: Delete test CRDs to avoid state leakage

### **Infrastructure**

1. ✅ **Document Build Times**: Comment expected durations in infrastructure code
2. ✅ **Preserve Failed Clusters**: Keep cluster alive for debugging
3. ✅ **Verify Environment Variables**: Add checks that critical vars reach processes
4. ✅ **Use Mock Modes**: Never call real LLMs in E2E tests (cost + determinism)
5. ✅ **Health Checks Before Tests**: Wait for all services ready in BeforeSuite

### **Debugging**

1. ✅ **Capture Logs Early**: Start log tailing before test execution
2. ✅ **Test Components Individually**: Verify services work before integration
3. ✅ **Use Diagnostic Logging**: Add structured logs in mock mode paths
4. ✅ **Document Findings**: Create handoff docs for cross-team issues
5. ✅ **Share Kubeconfig**: Document how to access preserved clusters

---

## 📚 **Related Documentation**

- [testing-strategy.md](./testing-strategy.md) - Complete testing strategy
- [HANDOFF_AIANALYSIS_SERVICE_COMPLETE_2025-12-13.md](../../../handoff/HANDOFF_AIANALYSIS_SERVICE_COMPLETE_2025-12-13.md) - Current status
- [LESSONS_LEARNED_CROSS_TEAM_E2E_DEBUGGING.md](../../../handoff/LESSONS_LEARNED_CROSS_TEAM_E2E_DEBUGGING.md) - Cross-team debugging patterns
- [DD-TEST-001](../../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md) - Port allocation strategy

---

## 📞 **Support**

**E2E Test Issues**:
- Review this guide first
- Check [TROUBLESHOOTING.md](./TROUBLESHOOTING.md) (if exists)
- See handoff documents in `docs/handoff/`

**Cross-Team Integration Issues**:
- Create formal handoff document
- Use structured communication pattern
- Reference [LESSONS_LEARNED_CROSS_TEAM_E2E_DEBUGGING.md](../../../handoff/LESSONS_LEARNED_CROSS_TEAM_E2E_DEBUGGING.md)

---

**Document Version**: 1.0
**Created**: 2025-12-13
**Maintained By**: AIAnalysis Team
**Next Review**: After E2E tests reach 100% passing

---

**END OF DOCUMENT**


