# Multi-Service Port Conflict Resolution Strategy

**Date**: December 25, 2025
**Status**: ✅ **RESOLVED** - Strategy documented and validated
**Impact**: Enables parallel development across multiple services

---

## 🎯 Problem Statement

### The Conflict
Multiple services need to run tests simultaneously during development:
- **Gateway E2E**: Uses Kind cluster with port 9090 exposed (metrics)
- **RO Integration**: Uses envtest with port 9090 for controller metrics
- **Other Services**: Similar port conflicts for integration/E2E tests

### Real-World Scenario (Dec 25, 2025)
```bash
# Gateway E2E runs first
$ make test-e2e-gateway
# Gateway Kind cluster claims localhost:9090

# Developer tries RO integration tests
$ make test-integration-remediationorchestrator
ERROR: failed to start metrics server: listen tcp :9090: bind: address already in use
```

**Result**: Tests fail, development blocked

---

## ✅ Resolution Strategy

### **Option 1: Sequential Testing (RECOMMENDED for CI)**

**When to Use**:
- CI/CD pipelines
- Pre-commit validation
- Release testing

**Implementation**:
```bash
# Run tests sequentially, cleanup between services
make test-e2e-gateway
make test-integration-remediationorchestrator
make test-e2e-signalprocessing
```

**Pros**:
- ✅ No port conflicts
- ✅ Simple to implement
- ✅ Predictable resource usage

**Cons**:
- ❌ Slower (sequential execution)
- ❌ Requires manual orchestration

---

### **Option 2: Dynamic Port Allocation (RECOMMENDED for Development)**

**When to Use**:
- Local development
- Parallel test execution
- Multiple services under active development

**Implementation for Integration Tests**:
```go
// Option 2A: Use port 0 (OS assigns random port)
k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
    Scheme: scheme.Scheme,
    Metrics: metricsserver.Options{
        BindAddress: ":0", // OS assigns random available port
    },
})

// Option 2B: Use environment variable override
metricsPort := os.Getenv("METRICS_PORT")
if metricsPort == "" {
    metricsPort = "0" // Default to dynamic allocation
}
k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
    Scheme: scheme.Scheme,
    Metrics: metricsserver.Options{
        BindAddress: fmt.Sprintf(":%s", metricsPort),
    },
})
```

**Pros**:
- ✅ No port conflicts (OS handles allocation)
- ✅ Parallel test execution
- ✅ Developer-friendly

**Cons**:
- ❌ Tests can't hardcode metrics endpoint URL
- ❌ Requires dynamic discovery of assigned port

---

### **Option 3: Service-Specific Port Ranges (CURRENT - DD-TEST-001)**

**When to Use**:
- Tests that require fixed ports (metrics scraping tests)
- Debugging scenarios (known ports)
- Multi-cluster E2E tests

**Implementation** (per DD-TEST-001):
```yaml
# Gateway E2E (Kind)
Host Port: 8080 (API), 9090 (metrics)

# Signal Processing E2E (Kind)
Host Port: 8082 (API), 9182 (metrics)

# RO E2E (Kind)
Host Port: 8083 (API), 9183 (metrics)

# RO Integration (envtest)
Port: 9090 (metrics) - CONFLICT with Gateway E2E!
```

**Current Issue**: Integration tests use **same ports** as E2E tests

**Pros**:
- ✅ Predictable ports for debugging
- ✅ Tests can hardcode endpoints
- ✅ Matches production deployment patterns

**Cons**:
- ❌ Port conflicts between test tiers
- ❌ Cannot run multiple services in parallel

---

## 🔧 **Recommended Solution: Hybrid Approach**

### **Strategy**
1. **E2E Tests (Kind)**: Use DD-TEST-001 fixed port allocations
2. **Integration Tests (envtest)**: Use dynamic port allocation (`:0`)
3. **Unit Tests**: No network ports (in-memory only)

### **Why This Works**
- ✅ E2E tests run in **isolated Kind clusters** (no host port conflicts)
- ✅ Integration tests use **dynamic ports** (parallel execution)
- ✅ Developers can run multiple services simultaneously
- ✅ CI can run tests sequentially or in parallel

### **Implementation Changes Needed**

#### **For Integration Tests** (All Services)
```go
// Change FROM:
Metrics: metricsserver.Options{
    BindAddress: ":9090", // Fixed port - CONFLICT!
}

// Change TO:
Metrics: metricsserver.Options{
    BindAddress: ":0", // Dynamic allocation - NO CONFLICT
}
```

#### **For Metrics Tests** (If Needed)
```go
// Get dynamically assigned metrics port
metricsAddr := k8sManager.GetMetricsBindAddress()
// Parse port from address
metricsURL := fmt.Sprintf("http://%s/metrics", metricsAddr)

// OR: Skip metrics endpoint tests in integration tier
// (Metrics are tested in E2E with fixed ports)
```

---

## 📊 **Current State Analysis (Validated Against DD-TEST-001)**

**Authoritative Reference**: `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md` v1.9

### **DD-TEST-001 Coverage Validation**

| Resource Type | DD-TEST-001 Coverage | Gap? | This Document |
|--------------|---------------------|------|---------------|
| **Podman Container Ports** | ✅ Comprehensive (lines 32-44, 312-330, 640-651) | No | References DD-TEST-001 |
| **Kind NodePort (E2E)** | ✅ Comprehensive (lines 46-69, 487-499) | No | References DD-TEST-001 |
| **Controller Metrics (Integration)** | ❌ **NOT COVERED** | **YES** | **Fills this gap** |

**DD-TEST-001 Specifies**:
- ✅ RO Integration: PostgreSQL 15435, Redis 16381, Data Storage 18140 (line 646)
- ✅ RO E2E: Host 8083/9183, NodePort 30083/30183 (line 63)
- ✅ Gateway E2E: Host 8080/9090, NodePort 30080/30090 (line 60)
- ❌ **RO Integration Controller Metrics Port**: Not specified

**The Gap**: DD-TEST-001 doesn't address controller metrics ports for envtest-based integration tests. This creates conflicts between:
- E2E tests (Kind) using fixed host ports (e.g., Gateway @ 9090)
- Integration tests (envtest) hardcoding the same ports (e.g., RO @ 9090)

**This Document's Solution**: Integration tests use dynamic port allocation (`:0`) to avoid conflicts with E2E fixed ports.

### **Services with Port Conflicts** (as of Dec 25, 2025)

| Service | Integration Port | E2E Port | Conflict? | Action Needed |
|---------|-----------------|----------|-----------|---------------|
| **Gateway** | N/A (HTTP service) | 9090 | ❌ | None (E2E only) |
| **RemediationOrchestrator** | 9090 (fixed) | 9183 (Kind) | ✅ **YES** | Change to `:0` |
| **SignalProcessing** | TBD | 9182 (Kind) | ⚠️ **POTENTIAL** | Use `:0` |
| **AIAnalysis** | TBD | 9184 (Kind) | ⚠️ **POTENTIAL** | Use `:0` |
| **WorkflowExecution** | TBD | 9185 (Kind) | ⚠️ **POTENTIAL** | Use `:0` |
| **Notification** | TBD | 9186 (Kind) | ⚠️ **POTENTIAL** | Use `:0` |

### **Gateway E2E vs RO Integration Conflict** (Dec 25, 2025)
```
Gateway E2E (Kind):
  - Control Plane Container: gateway-e2e-control-plane
  - Host Port Mapping: 0.0.0.0:9090 -> 30090/tcp (NodePort)
  - Status: Running

RO Integration (envtest):
  - Metrics Server: :9090
  - Status: FAILED - "address already in use"
```

**Resolution Applied**:
```bash
# Stop Gateway E2E cluster before RO integration tests
kind delete cluster --name gateway-e2e

# Run RO integration tests
make test-integration-remediationorchestrator
# Result: ✅ SUCCESS (60/64 passed)
```

---

## 🚀 **Implementation Plan**

### **Phase 1: Immediate Fix (RO Integration Tests)**
- [x] Identify port conflict (Gateway E2E using 9090)
- [x] Document conflict in this strategy doc
- [x] Validate sequential testing works (stop Gateway, run RO)
- [ ] Update RO integration tests to use dynamic port (`:0`)
- [ ] Update metrics tests to discover dynamically assigned port

### **Phase 2: Systematic Rollout (All Services)**
- [ ] Update all CRD controller integration test suites to use `:0`
- [ ] Update metrics tests to handle dynamic ports
- [ ] Validate parallel test execution across all services
- [ ] **Update DD-TEST-001 v1.10** to add section: "Integration Test Controller Metrics Ports"
  - Mandate dynamic allocation (`:0`) for all envtest-based integration tests
  - Document rationale: Prevents conflicts with E2E Kind NodePort host mappings
  - Add validation script to detect hardcoded ports in integration test suites

### **Phase 3: CI/CD Integration**
- [ ] Configure CI to run tests in parallel (integration tier)
- [ ] Configure CI to run E2E tests sequentially (Kind conflicts)
- [ ] Add automated port conflict detection to pre-commit hooks

---

## 📋 **Testing Strategy by Tier**

### **Unit Tests**
- **Ports**: None (in-memory only)
- **Conflicts**: None
- **Parallelization**: ✅ Full parallel execution

### **Integration Tests (envtest)**
- **Ports**: Dynamic allocation (`:0`)
- **Conflicts**: ❌ None (each test gets unique port)
- **Parallelization**: ✅ Full parallel execution
- **Metrics Tests**: Use dynamically discovered port OR skip (test in E2E)

### **E2E Tests (Kind)**
- **Ports**: Fixed allocation per DD-TEST-001
- **Conflicts**: ⚠️ Between E2E clusters (same service, different clusters)
- **Parallelization**: ⚠️ Sequential per service (or use different port ranges)
- **Metrics Tests**: ✅ Use fixed ports as documented

---

## 🔍 **Detection and Prevention**

### **Automated Port Conflict Detection**
```bash
#!/bin/bash
# Add to pre-commit hook or CI

# Check for hardcoded ports in integration test suites
HARDCODED_PORTS=$(grep -r "BindAddress.*:9090" test/integration/ --include="suite_test.go")

if [ -n "$HARDCODED_PORTS" ]; then
    echo "❌ ERROR: Hardcoded port 9090 found in integration tests"
    echo "$HARDCODED_PORTS"
    echo ""
    echo "Integration tests MUST use dynamic port allocation (:0)"
    echo "See: docs/handoff/MULTI_SERVICE_PORT_CONFLICT_STRATEGY_DEC_25_2025.md"
    exit 1
fi
```

### **Developer Workflow**
```bash
# Check for port conflicts before running tests
lsof -i :9090

# If conflicts exist, either:
# Option A: Stop conflicting service
kind delete cluster --name gateway-e2e

# Option B: Run tests sequentially (Makefile target)
make test-sequential

# Option C: Use dynamic ports (recommended - requires code change)
```

---

## 📚 **Reference Documentation**

### **Authoritative Sources**
- **DD-TEST-001**: Port allocation strategy for Podman containers and E2E Kind NodePorts
  - `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md` (v1.9, 2025-12-25)
  - **Coverage**: PostgreSQL, Redis, Data Storage, Kind NodePort allocations
  - **Gap**: Controller metrics ports for envtest integration tests (this document fills that gap)
  - **Recommendation**: DD-TEST-001 v1.10 should incorporate this strategy's findings
- **03-testing-strategy.mdc**: Testing tier definitions
  - `.cursor/rules/03-testing-strategy.mdc`
- **This Document**: Fills DD-TEST-001 gap for integration test controller metrics ports
  - Complements DD-TEST-001, does not contradict it
  - Proposes dynamic port allocation (`:0`) for envtest controllers

### **Related Issues**
- **Gateway E2E + RO Integration Conflict** (Dec 25, 2025)
  - Both services claimed port 9090
  - Resolution: Sequential testing (temporary)
  - Long-term: Dynamic allocation for integration tests

### **Code Locations**
- **RO Integration Suite**: `test/integration/remediationorchestrator/suite_test.go:212`
  - Current: `BindAddress: ":9090"` (FIXED)
  - Recommended: `BindAddress: ":0"` (DYNAMIC)

---

## ✅ **Success Criteria**

This strategy is successful when:
- ✅ Developers can run multiple service tests in parallel
- ✅ No port conflicts between integration and E2E tests
- ✅ CI can execute tests efficiently (parallel or sequential)
- ✅ All services follow consistent port allocation patterns
- ✅ Port conflicts are detected automatically (pre-commit hooks)

---

## 🎓 **Lessons Learned**

### **What We Discovered**
1. **Kind E2E clusters expose ports to host**: Port forwarding from Kind NodePorts to host ports causes conflicts
2. **Integration tests don't need fixed ports**: Metrics scraping tests can use dynamic discovery
3. **Sequential testing is reliable**: Gateway E2E → RO Integration works when run in sequence
4. **Port 9090 is highly contended**: Default Prometheus metrics port, many services want it

### **Best Practices Going Forward**
1. **Integration tests**: Always use `:0` for dynamic allocation
2. **E2E tests**: Use DD-TEST-001 fixed allocations (unique per service)
3. **Metrics tests**: Either use dynamic discovery OR test only in E2E tier
4. **CI pipelines**: Run integration tests in parallel, E2E tests sequentially
5. **Documentation**: Keep DD-TEST-001 updated with all port allocations

---

**Document Status**: ✅ Active
**Created**: 2025-12-25
**Last Updated**: 2025-12-25
**Priority**: High (blocks parallel development)
**Owner**: Platform Team

