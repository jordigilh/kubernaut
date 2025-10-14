# Dynamic Toolset Service - E2E Test Plan (V2)

**Date**: October 13, 2025
**Target Version**: V2 (In-Cluster Deployment)
**Status**: 📋 **PLANNED** (Deferred to V2)
**Current Version**: V1 (Out-of-Cluster Development Mode)

---

## Overview

This document defines the End-to-End (E2E) test plan for the Dynamic Toolset Service when deployed in-cluster (V2). E2E tests are deferred to V2 because the current V1 implementation runs out-of-cluster for development convenience, while E2E tests require in-cluster deployment to validate production scenarios.

---

## Why Defer E2E Tests to V2?

### V1: Out-of-Cluster Development Mode

**Current Architecture**:
```
Developer Machine
└── Dynamic Toolset Service (local process)
    └── KUBECONFIG → Remote Kubernetes Cluster
        ├── Service Discovery
        ├── ConfigMap Operations
        └── Authentication (TokenReview)
```

**Characteristics**:
- Service runs as local process on developer machine
- Accesses Kubernetes API via KUBECONFIG
- Suitable for development and testing
- **NOT representative of production deployment**

**Test Coverage**:
- ✅ Unit Tests: 194/194 (100%) - Component-level testing
- ✅ Integration Tests: 38/38 (100%) - Real K8s API via Kind cluster
- ⏸️ E2E Tests: Deferred - Require in-cluster deployment

### V2: In-Cluster Production Deployment

**Target Architecture**:
```
Kubernetes Cluster
└── kubernaut-system namespace
    └── dynamic-toolset Pod
        ├── ServiceAccount with RBAC
        ├── In-cluster config
        ├── Service Discovery (same cluster)
        ├── ConfigMap Operations (same namespace)
        └── Metrics/Health endpoints
```

**Characteristics**:
- Service runs as Pod in target cluster
- Uses in-cluster ServiceAccount for auth
- RBAC-restricted permissions
- **Production-representative deployment**

**E2E Test Scenarios Enabled**:
- ✅ Multi-cluster service discovery
- ✅ RBAC restriction validation
- ✅ Large-scale discovery (100+ services)
- ✅ Cross-namespace discovery with permissions
- ✅ ConfigMap reconciliation under load
- ✅ Network policy enforcement
- ✅ Resource limit validation

---

## V2 E2E Test Scenarios

### Scenario 1: Multi-Cluster Service Discovery

**Objective**: Validate service discovery across multiple Kubernetes clusters

**Test Setup**:
```bash
# Create 3-cluster Kind environment
kind create cluster --name test-cluster-1
kind create cluster --name test-cluster-2
kind create cluster --name test-cluster-3

# Deploy Dynamic Toolset to cluster-1
kubectl apply -k deploy/dynamic-toolset/ --context kind-test-cluster-1

# Deploy mock services to all clusters
kubectl apply -f test/e2e/fixtures/prometheus-service.yaml --context kind-test-cluster-1
kubectl apply -f test/e2e/fixtures/grafana-service.yaml --context kind-test-cluster-2
kubectl apply -f test/e2e/fixtures/jaeger-service.yaml --context kind-test-cluster-3
```

**Test Steps**:
1. Configure Dynamic Toolset with multi-cluster kubeconfigs
2. Trigger discovery across all 3 clusters
3. Verify ConfigMap contains services from all clusters
4. Validate endpoint URLs include correct cluster context

**Success Criteria**:
- ✅ All services from 3 clusters discovered
- ✅ ConfigMap updated with services from all clusters
- ✅ Endpoint URLs correctly reference cluster contexts
- ✅ Discovery completes in < 10 seconds for 30 services (10 per cluster)

**Test File**: `test/e2e/multi_cluster_discovery_test.go`

### Scenario 2: RBAC Restriction Testing

**Objective**: Validate service operates correctly with restricted RBAC permissions

**Test Setup**:
```yaml
# Restricted ClusterRole (can only list services in specific namespaces)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: dynamic-toolset-restricted
rules:
- apiGroups: [""]
  resources: ["services"]
  verbs: ["get", "list"]
  resourceNames: []  # Empty = all services
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "create", "update", "patch"]
- apiGroups: ["authentication.k8s.io"]
  resources: ["tokenreviews"]
  verbs: ["create"]
```

**Test Steps**:
1. Deploy Dynamic Toolset with restricted ClusterRole
2. Create services in multiple namespaces (some allowed, some forbidden)
3. Trigger discovery
4. Verify only allowed services are discovered
5. Validate error logging for forbidden namespaces

**Success Criteria**:
- ✅ Services in allowed namespaces discovered
- ✅ Services in forbidden namespaces skipped (not error)
- ✅ Error metrics incremented for RBAC denials
- ✅ ConfigMap contains only allowed services
- ✅ No pod crash or restart on RBAC denial

**Test File**: `test/e2e/rbac_restrictions_test.go`

### Scenario 3: Large-Scale Discovery (100+ Services)

**Objective**: Validate performance and stability with large number of services

**Test Setup**:
```bash
# Deploy 120 mock services across 4 namespaces
for i in {1..30}; do
  kubectl apply -f test/e2e/fixtures/prometheus-service-$i.yaml -n monitoring
  kubectl apply -f test/e2e/fixtures/grafana-service-$i.yaml -n observability
  kubectl apply -f test/e2e/fixtures/jaeger-service-$i.yaml -n tracing
  kubectl apply -f test/e2e/fixtures/custom-service-$i.yaml -n default
done
```

**Test Steps**:
1. Deploy Dynamic Toolset to Kind cluster
2. Deploy 120 discoverable services across 4 namespaces
3. Trigger discovery
4. Measure discovery latency
5. Verify ConfigMap size and structure
6. Validate all 120 services discovered

**Success Criteria**:
- ✅ All 120 services discovered correctly
- ✅ Discovery latency < 5 seconds
- ✅ ConfigMap size < 200KB (well below 1MB limit)
- ✅ Memory usage < 256Mi
- ✅ CPU usage < 0.5 cores average
- ✅ No pod restarts or OOMKills

**Test File**: `test/e2e/large_scale_discovery_test.go`

**Performance Benchmarks**:
```
Services | Discovery Time | ConfigMap Size | Memory Usage | CPU Usage
---------|---------------|----------------|--------------|----------
10       | < 1s          | ~10KB          | ~50Mi        | ~0.1 cores
50       | < 2s          | ~50KB          | ~100Mi       | ~0.2 cores
100      | < 5s          | ~100KB         | ~200Mi       | ~0.4 cores
120      | < 5s          | ~150KB         | ~256Mi       | ~0.5 cores
```

### Scenario 4: Cross-Namespace Discovery with Permissions

**Objective**: Validate namespace filtering and permission-aware discovery

**Test Setup**:
```yaml
# Namespace-scoped Role (only monitoring and observability)
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: dynamic-toolset-namespaced
  namespace: monitoring
rules:
- apiGroups: [""]
  resources: ["services"]
  verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: dynamic-toolset-namespaced
  namespace: observability
rules:
- apiGroups: [""]
  resources: ["services"]
  verbs: ["get", "list"]
```

**Test Steps**:
1. Deploy Dynamic Toolset with namespace-scoped Roles
2. Create services in `monitoring`, `observability`, `default`, `kube-system` namespaces
3. Configure Dynamic Toolset to discover only `monitoring` and `observability`
4. Trigger discovery
5. Verify only services from configured namespaces are discovered

**Success Criteria**:
- ✅ Services from `monitoring` and `observability` namespaces discovered
- ✅ Services from `default` and `kube-system` namespaces NOT discovered
- ✅ ConfigMap contains only services from allowed namespaces
- ✅ No errors logged for unconfigured namespaces
- ✅ Namespace filter configuration respected

**Test File**: `test/e2e/cross_namespace_discovery_test.go`

### Scenario 5: ConfigMap Reconciliation Under Load

**Objective**: Validate ConfigMap reconciliation with concurrent updates

**Test Setup**:
```bash
# Deploy Dynamic Toolset with 30-second reconciliation interval
kubectl apply -k deploy/dynamic-toolset/
kubectl set env deployment/dynamic-toolset DISCOVERY_INTERVAL=30s -n kubernaut-system

# Deploy tool to simulate concurrent ConfigMap updates
kubectl apply -f test/e2e/fixtures/configmap-updater.yaml
```

**Test Steps**:
1. Deploy Dynamic Toolset with 30-second reconciliation
2. Deploy 20 services
3. While discovery is running, manually update ConfigMap (add override)
4. Wait for reconciliation cycle
5. Verify override is preserved
6. Add 5 more services
7. Wait for reconciliation
8. Verify new services added, override still preserved

**Success Criteria**:
- ✅ Manual overrides preserved across reconciliation cycles
- ✅ New services added to ConfigMap within 2 seconds of reconciliation
- ✅ Stale services removed within 2 seconds
- ✅ ConfigMap structure remains valid throughout
- ✅ No lost updates or race conditions
- ✅ Reconciliation latency < 2 seconds

**Test File**: `test/e2e/reconciliation_under_load_test.go`

### Scenario 6: Network Policy Enforcement

**Objective**: Validate service operates correctly with network policies

**Test Setup**:
```yaml
# Network Policy: Allow Dynamic Toolset to access K8s API and services
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: dynamic-toolset-netpol
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: dynamic-toolset
  policyTypes:
  - Egress
  egress:
  - to:
    - namespaceSelector: {}  # Allow access to all namespaces
    ports:
    - protocol: TCP
      port: 443  # K8s API
    - protocol: TCP
      port: 9090  # Service discovery
  - to:
    - podSelector: {}
    ports:
    - protocol: TCP
      port: 53  # DNS
```

**Test Steps**:
1. Deploy Network Policy allowing only K8s API and service ports
2. Deploy Dynamic Toolset
3. Deploy services in various namespaces
4. Trigger discovery
5. Verify service discovery works with network policy
6. Deploy restrictive Network Policy (deny all egress)
7. Verify service handles network failures gracefully

**Success Criteria**:
- ✅ Service discovery works with permissive network policy
- ✅ K8s API calls succeed through network policy
- ✅ Service handles network policy denials gracefully (no crashes)
- ✅ Error metrics incremented for network failures
- ✅ Health check remains available
- ✅ Graceful degradation when network restricted

**Test File**: `test/e2e/network_policy_test.go`

### Scenario 7: Resource Limit Validation

**Objective**: Validate service operates within defined resource limits

**Test Setup**:
```yaml
# Deployment with strict resource limits
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dynamic-toolset
spec:
  template:
    spec:
      containers:
      - name: dynamic-toolset
        resources:
          requests:
            memory: "128Mi"
            cpu: "250m"
          limits:
            memory: "256Mi"
            cpu: "500m"
```

**Test Steps**:
1. Deploy Dynamic Toolset with strict resource limits
2. Deploy 100 services to stress the system
3. Trigger discovery repeatedly (every 30 seconds for 10 minutes)
4. Monitor memory and CPU usage
5. Verify no OOMKills or CPU throttling
6. Measure discovery latency under resource constraints

**Success Criteria**:
- ✅ Memory usage stays below 256Mi limit
- ✅ CPU usage stays below 500m limit
- ✅ No pod restarts due to OOMKill
- ✅ Discovery latency remains < 5 seconds under load
- ✅ Graceful handling of resource constraints
- ✅ No memory leaks over 10-minute test

**Test File**: `test/e2e/resource_limits_test.go`

---

## E2E Test Infrastructure Requirements

### Multi-Node Kind Cluster Setup

**Configuration** (`test/e2e/kind-config.yaml`):
```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: dynamic-toolset-e2e
nodes:
- role: control-plane
- role: worker
- role: worker
- role: worker
featureGates:
  NetworkPolicy: true
networking:
  disableDefaultCNI: false
  kubeProxyMode: "iptables"
```

**Setup Script** (`test/e2e/setup-cluster.sh`):
```bash
#!/bin/bash
set -e

# Create Kind cluster
kind create cluster --config test/e2e/kind-config.yaml

# Deploy CNI (Calico for Network Policy support)
kubectl apply -f https://docs.projectcalico.org/manifests/calico.yaml

# Wait for nodes ready
kubectl wait --for=condition=Ready nodes --all --timeout=300s

# Create test namespaces
kubectl create namespace kubernaut-system
kubectl create namespace monitoring
kubectl create namespace observability
kubectl create namespace tracing

# Deploy Dynamic Toolset
kubectl apply -k deploy/dynamic-toolset/

# Wait for Dynamic Toolset ready
kubectl wait --for=condition=Ready pod -l app=dynamic-toolset -n kubernaut-system --timeout=120s

echo "E2E cluster ready"
```

### Mock Service Deployment

**Service Generator** (`test/e2e/generate-services.sh`):
```bash
#!/bin/bash

# Generate N mock services
generate_services() {
  local count=$1
  local namespace=$2
  local service_type=$3

  for i in $(seq 1 $count); do
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  name: ${service_type}-${i}
  namespace: ${namespace}
  labels:
    app: ${service_type}
    kubernaut.io/discoverable: "true"
spec:
  ports:
  - port: 9090
    name: metrics
  selector:
    app: ${service_type}-${i}
EOF
  done
}

# Generate 30 Prometheus services
generate_services 30 monitoring prometheus

# Generate 30 Grafana services
generate_services 30 observability grafana

# Generate 30 Jaeger services
generate_services 30 tracing jaeger

# Generate 30 custom services
generate_services 30 default custom
```

### Test Execution Framework

**E2E Test Runner** (`test/e2e/run-e2e-tests.sh`):
```bash
#!/bin/bash
set -e

echo "Setting up E2E test cluster..."
./test/e2e/setup-cluster.sh

echo "Running E2E tests..."
go test -v ./test/e2e/... \
  -timeout 30m \
  -count 1 \
  --ginkgo.v \
  --ginkgo.progress

echo "Cleaning up E2E cluster..."
kind delete cluster --name dynamic-toolset-e2e

echo "E2E tests complete"
```

---

## Success Criteria

E2E test suite is **COMPLETE** when:

- ✅ All 7 scenarios implemented with Ginkgo/Gomega tests
- ✅ Multi-node Kind cluster setup automated
- ✅ Mock service generation automated
- ✅ Test execution framework complete
- ✅ 100% test pass rate in CI
- ✅ Test duration < 30 minutes total
- ✅ Cleanup automated (no leftover resources)

---

## Timeline & Effort Estimate

### Development Effort

| Phase | Effort | Description |
|-------|--------|-------------|
| **Infrastructure Setup** | 4 hours | Kind cluster, CNI, namespaces, cleanup |
| **Scenario 1-2** | 6 hours | Multi-cluster + RBAC tests |
| **Scenario 3-4** | 6 hours | Large-scale + cross-namespace tests |
| **Scenario 5-7** | 8 hours | Reconciliation + network + resources |
| **CI Integration** | 4 hours | GitHub Actions workflow |
| **Documentation** | 2 hours | Test documentation and troubleshooting |
| **Total** | **30 hours** | ~4 days of development |

### Timeline

**Prerequisites**:
- ✅ V2 in-cluster deployment manifests ready
- ✅ RBAC roles defined
- ✅ Resource limits configured
- ✅ Network policies defined

**Estimated Start**: After V2 deployment manifests complete
**Estimated Completion**: 4 days after start
**Target Version**: V2 (Q1 2026)

---

## Cost/Benefit Analysis

### Cost of E2E Tests

**Development**:
- 30 hours implementation
- 4 hours ongoing maintenance per month

**CI Resources**:
- ~30 minutes per test run
- 2-3 test runs per PR
- 10-15 PRs per week
- Estimated: 15-22.5 hours CI time per week

**Infrastructure**:
- Kind clusters (ephemeral, no cost)
- GitHub Actions minutes (~450-675 per week)

**Total Cost**: ~34 hours development + ~675 CI minutes/week

### Benefit of E2E Tests

**Bugs Prevented**:
- RBAC misconfigurations (high impact)
- Scale issues (100+ services)
- Network policy conflicts
- Resource limit violations
- Reconciliation race conditions

**Confidence Increase**:
- V1 with unit + integration: 95% confidence
- V2 with E2E: 98-99% confidence

**Production Parity**:
- V1: Development environment (good)
- V2 with E2E: Production-like environment (excellent)

**Regression Prevention**:
- Catch breaking changes before production
- Validate multi-cluster scenarios
- Ensure RBAC compliance

### Decision: Defer to V2

**Rationale**:
1. ✅ V1 has 100% unit + integration test coverage (232/232 passing)
2. ✅ Integration tests use real Kubernetes API (Kind cluster)
3. ✅ E2E tests require in-cluster deployment (not available in V1)
4. ✅ Cost/benefit favorable for V2, not V1
5. ✅ V1 confidence (95%) sufficient for initial release

**Risk Acceptance**:
- V1 may have undiscovered issues in production deployment scenarios
- Acceptable risk given comprehensive integration test coverage
- E2E tests will catch issues before V2 production release

---

## Deferral Justification

### Why V1 is Sufficient Without E2E

1. **Comprehensive Integration Tests**
   - 38 integration tests using real Kubernetes API
   - Real service discovery, ConfigMap operations, authentication
   - Kind cluster provides production-like environment

2. **Out-of-Cluster Limitations**
   - V1 runs as local process, not in-cluster Pod
   - E2E scenarios (RBAC, network policies) not applicable
   - In-cluster deployment required for meaningful E2E tests

3. **Cost/Benefit for V1**
   - High development cost (30 hours)
   - Low benefit (tests not applicable to V1 architecture)
   - Better to invest in V2 in-cluster deployment

4. **Risk Mitigation**
   - 232/232 tests passing (100%)
   - Real Kubernetes API integration
   - 95% confidence sufficient for V1

### When E2E Tests Become Critical

E2E tests become **MANDATORY** when:
- ✅ V2 in-cluster deployment is implemented
- ✅ RBAC roles are production-ready
- ✅ Network policies are defined
- ✅ Resource limits are set
- ✅ Production release is planned

**Trigger**: V2 in-cluster deployment complete → E2E tests MUST be implemented before production release

---

## References

- [01-integration-first-rationale.md](01-integration-first-rationale.md) - Integration test approach
- [TESTING_STRATEGY.md](TESTING_STRATEGY.md) - Overall testing strategy
- [README.md](../README.md) - Service overview and deployment
- Kind Documentation: https://kind.sigs.k8s.io/
- Kubernetes E2E Testing: https://kubernetes.io/blog/2019/03/22/kubernetes-end-to-end-testing-for-everyone/

---

**Document Status**: 📋 **PLANNED** (Deferred to V2)
**Last Updated**: October 13, 2025
**Next Action**: Implement after V2 in-cluster deployment
**Effort Estimate**: 30 hours development + 4 hours/month maintenance
