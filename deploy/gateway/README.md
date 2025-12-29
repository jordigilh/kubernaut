# Gateway Service Deployment

**Version**: v1.0
**Status**: ✅ Production-Ready (SOC2 Compliant)
**Last Updated**: December 20, 2025

---

## Overview

The Gateway Service provides signal ingestion from Prometheus AlertManager and Kubernetes Events with Kubernetes-native state management for deduplication and RemediationRequest CRD creation.

**V1.0 Features**:
- **K8s-Native Deduplication** (DD-GATEWAY-011): Uses RemediationRequest status fields for deduplication state
- **CRD-Based State Management** (DD-GATEWAY-012): No external dependencies - all state in Kubernetes CRDs
- **SOC2-Compliant Audit Traces**: Full audit trail for enterprise compliance
- **Multi-Signal Support**: Prometheus AlertManager webhooks and Kubernetes Events

**Architecture Changes**:
- ❌ **Redis Removed** (DD-GATEWAY-012): Migrated to K8s-native state management
- ❌ **Storm Detection Removed** (DD-GATEWAY-015): Simplified V1.0 architecture

---

## 📋 **Prerequisites**

- **Kubernetes cluster** (v1.24+)
- **kubectl** CLI configured
- **Data Storage service** (for audit events)

---

## 🚀 **Quick Start**

### **Deploy Gateway Service**

```bash
# Deploy to Kubernetes
kubectl apply -k deploy/gateway/base/

# Verify deployment
kubectl get pods -n kubernaut-system -l app.kubernetes.io/component=gateway
kubectl logs -n kubernaut-system -l app.kubernetes.io/component=gateway --tail=50

# Check readiness
kubectl exec -n kubernaut-system $(kubectl get pod -n kubernaut-system -l app.kubernetes.io/component=gateway -o jsonpath='{.items[0].metadata.name}') -- curl localhost:8080/ready
```

---

## 📁 **Directory Structure**

```
deploy/gateway/base/
├── kustomization.yaml              # Kustomize configuration
├── 00-namespace.yaml               # kubernaut-system namespace
├── 01-rbac.yaml                    # ServiceAccount + ClusterRole + Binding
├── 02-configmap.yaml               # Gateway configuration
├── 03-deployment.yaml              # Gateway deployment
├── 04-service.yaml                 # ClusterIP service (8080, 9090)
└── 06-servicemonitor.yaml          # Prometheus metrics
```

**Note**: V1.0 provides standard Kubernetes manifests. Platform-specific overlays (OpenShift SCC, etc.) will be added in V1.1 based on user feedback.

---

## 🔧 **Configuration**

### **Environment Variables**

Gateway configuration is managed via ConfigMap (`gateway-config`):

| Variable | Purpose | Example |
|----------|---------|---------|
| `DATA_STORAGE_URL` | Audit event destination | `http://datastorage.kubernaut-system.svc.cluster.local:8080` |
| `LOG_LEVEL` | Logging verbosity | `info`, `debug`, `error` |
| `METRICS_PORT` | Prometheus metrics port | `9090` |

### **Customization**

To customize configuration:

1. Edit `base/02-configmap.yaml`
2. Redeploy: `kubectl apply -k deploy/gateway/base/`
3. Restart: `kubectl rollout restart deployment/gateway -n kubernaut-system`

---

## 🏗️ **Architecture**

### **Components**

| Component | Purpose | Port |
|---|---|---|
| **Gateway** | Signal ingestion + CRD creation | 8080 (HTTP), 9090 (metrics) |

**State Management**: All deduplication state is stored in RemediationRequest CRD status fields (DD-GATEWAY-011). No external dependencies.

### **API Endpoints**

- `POST /api/v1/signals/prometheus` - Prometheus AlertManager webhooks
- `POST /api/v1/signals/kubernetes-event` - Kubernetes Events
- `GET /health` - Health check (liveness probe)
- `GET /ready` - Readiness probe
- `GET /metrics` - Prometheus metrics (port 9090)

---

## 📊 **Monitoring**

### **Prometheus Metrics**

Gateway exposes metrics at `:9090/metrics`:

**Core Metrics**:
- `gateway_signals_received_total` - Total signals received
- `gateway_signals_deduplicated_total` - Signals deduplicated via CRD status
- `gateway_crds_created_total` - RemediationRequests created
- `gateway_crd_updates_total` - Deduplication updates to existing CRDs
- `gateway_processing_duration_seconds` - Signal processing latency

**SOC2 Audit Metrics**:
- `gateway_audit_events_emitted_total` - Audit events sent to Data Storage
- `gateway_audit_failures_total` - Audit event emission failures

### **Health Checks**

```bash
# Health check (liveness)
kubectl exec -n kubernaut-system [gateway-pod] -- curl localhost:8080/health

# Readiness check
kubectl exec -n kubernaut-system [gateway-pod] -- curl localhost:8080/ready
```

---

## 🔍 **Troubleshooting**

### **Gateway Pod Not Starting**

**Symptom**: Pod in `CrashLoopBackOff` or `Error` state

**Check**:
```bash
kubectl describe pod -n kubernaut-system -l app.kubernetes.io/component=gateway
kubectl logs -n kubernaut-system -l app.kubernetes.io/component=gateway
```

**Common Issues**:
1. **DATA_STORAGE_URL not set**: Gateway requires Data Storage for audit events (ADR-032)
2. **RBAC permissions**: Check ClusterRole for CRD access
3. **Image pull errors**: Verify image availability

### **CRD Creation Failures**

**Symptom**: Signals received but no RemediationRequest CRDs created

**Check**:
```bash
# Check Gateway logs for errors
kubectl logs -n kubernaut-system -l app.kubernetes.io/component=gateway | grep ERROR

# Verify CRD permissions
kubectl auth can-i create remediationrequests.remediation.kubernaut.ai \
  --as=system:serviceaccount:kubernaut-system:gateway
```

### **Deduplication Not Working**

**Symptom**: Duplicate RemediationRequest CRDs for same signal

**Check**:
```bash
# Check RemediationRequest status for deduplication metadata
kubectl get remediationrequest -n [namespace] [rr-name] -o jsonpath='{.status.deduplication}'

# Verify fingerprint calculation
kubectl logs -n kubernaut-system -l app.kubernetes.io/component=gateway | grep fingerprint
```

**Note**: Deduplication state is stored in RemediationRequest status.deduplication field per DD-GATEWAY-011

### **Audit Events Not Emitted**

**Symptom**: No audit events in Data Storage

**Check**:
```bash
# Verify Data Storage connectivity
kubectl exec -n kubernaut-system [gateway-pod] -- curl http://datastorage.kubernaut-system.svc.cluster.local:8080/health

# Check audit failure metrics
kubectl exec -n kubernaut-system [gateway-pod] -- curl localhost:9090/metrics | grep gateway_audit_failures_total
```

---

## 🔄 **Upgrading**

```bash
# Update image tag in base/kustomization.yaml or base/03-deployment.yaml
# Then redeploy
kubectl apply -k deploy/gateway/base/

# Monitor rollout
kubectl rollout status deployment/gateway -n kubernaut-system

# Or use kubectl set image
kubectl set image deployment/gateway gateway=quay.io/jordigilh/kubernaut-gateway:v1.1.0 -n kubernaut-system
```

---

## 🗑️ **Uninstall**

```bash
# Delete Gateway service
kubectl delete -k deploy/gateway/base/

# Or delete namespace (removes all kubernaut resources)
# kubectl delete namespace kubernaut-system
```

---

## 📚 **References**

### **Architecture Decisions**
- [DD-GATEWAY-011 - Shared Status Deduplication](../../docs/architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md)
- [DD-GATEWAY-012 - Redis Removal Complete](../../docs/handoff/NOTICE_DD_GATEWAY_012_REDIS_REMOVAL_COMPLETE.md)
- [DD-GATEWAY-015 - Storm Detection Removal](../../docs/architecture/decisions/DD-GATEWAY-015-storm-detection-removal.md)

### **Implementation**
- [Gateway Service Documentation](../../docs/services/stateless/gateway-service/)
- [V1.0 Completion Summary](../../docs/handoff/GATEWAY_V1_0_COMPLETE_25_25_TESTS_PASSING_DEC_20_2025.md)
- [P0 Maturity Compliance](../../docs/handoff/GATEWAY_P0_MATURITY_COMPLIANCE_COMPLETE_DEC_20_2025.md)

### **Testing**
- **Tests**: 443 total (314 unit + 104 integration + 25 E2E)
- **Coverage**: 100% P0 requirements (6/6)
- **Status**: ✅ All tests passing

---

## 🎯 **V1.0 Status**

| Aspect | Status | Details |
|--------|--------|---------|
| **Production Ready** | ✅ Complete | 443 tests passing, 100% P0 compliance |
| **SOC2 Compliance** | ✅ Complete | Full audit trail, enterprise-ready |
| **K8s-Native** | ✅ Complete | No external dependencies (Redis removed) |
| **Architecture** | ✅ Simplified | Storm detection removed, CRD-based state |

**Maintainer**: Kubernaut Gateway Team
**Last Validated**: December 20, 2025
**Version**: v1.0 Production-Ready
