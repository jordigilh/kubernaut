# CRD API Group Rationale

**Date**: October 6, 2025
**Status**: ✅ **APPROVED**
**API Group**: `kubernaut.io`
**Version**: `/v1`

---

## 📋 Overview

All Kubernaut Custom Resource Definitions (CRDs) use the **`kubernaut.io`** API group with **`/v1`** versioning.

---

## 🎯 API Group: `kubernaut.io`

### **Selected API Group**
```yaml
apiVersion: kubernaut.io/v1
kind: RemediationRequest
```

### **Rationale**

#### 1. **Project-Scoped Grouping**
- **`kubernaut.io`** clearly identifies all CRDs as part of the Kubernaut project
- Avoids confusion with upstream Prometheus Alert Manager CRDs
- Provides namespace isolation from other projects

#### 2. **Domain Ownership**
- `kubernaut.io` domain is project-specific
- No conflicts with existing Kubernetes ecosystem CRDs
- Clear ownership and provenance

#### 3. **Kubernetes Convention Compliance**
- Follows Kubernetes API group naming conventions: `<domain>/<version>`
- Uses reverse DNS notation for uniqueness
- Aligns with examples: `monitoring.coreos.com`, `cert-manager.io`, `tekton.dev`

#### 4. **Simplicity & Clarity**
- Short, memorable domain name
- Easy to type in kubectl commands: `kubectl get remediationrequests.kubernaut.io`
- No ambiguity about project identity

---

## 📦 CRD Inventory

All Kubernaut CRDs use `kubernaut.io/v1`:

| CRD Kind | API Group | Full Name | Controller |
|----------|-----------|-----------|------------|
| **RemediationRequest** | `kubernaut.io/v1` | `remediationrequests.kubernaut.io` | RemediationOrchestrator |
| **RemediationProcessing** | `kubernaut.io/v1` | `remediationprocessings.kubernaut.io` | RemediationProcessor |
| **AIAnalysis** | `kubernaut.io/v1` | `aianalysis.kubernaut.io` | AIAnalysis |
| **WorkflowExecution** | `kubernaut.io/v1` | `workflowexecutions.kubernaut.io` | WorkflowExecution |
| **KubernetesExecution** | `kubernaut.io/v1` | `kubernetesexecutions.kubernaut.io` | KubernetesExecutor |

---

## 🔄 Versioning Strategy

### **V1 (Current)**
- **Version**: `/v1`
- **Stability**: General Availability (GA)
- **Scope**: V1 feature set (Kubernetes-only, single-cluster)
- **Backwards Compatibility**: None required (pre-release product)

### **V2 (Future)**
When V2 features are added (multi-cloud, multi-cluster):

**Option A: In-Place Upgrade** (Recommended)
- Keep `kubernaut.io/v1`
- Add new fields to existing CRDs
- Use optional fields for V2 features
- Rationale: Simpler for users, no migration needed

**Option B: New API Version**
- Introduce `kubernaut.io/v2`
- Dual support for v1 and v2 during migration
- Conversion webhooks for compatibility
- Rationale: Clean separation, explicit migration

**Decision**: Defer until V2 feature set finalized

---

## 🚫 Alternatives Considered

### **Alternative 1: `prometheus-alerts-slm.io`**
❌ **Rejected**
- **Too long**: Cumbersome in kubectl commands
- **Misleading**: Project scope extends beyond Prometheus alerts
- **Not generic**: Doesn't reflect multi-signal sources (K8s events, Datadog, etc.)
- **Coupling**: Ties identity to specific monitoring tool

### **Alternative 2: `alerts.kubernaut.io`**
❌ **Rejected**
- **Too specific**: Not all CRDs are alert-related (WorkflowExecution, KubernetesExecution are action-focused)
- **Limiting**: Doesn't reflect remediation orchestration scope
- **Redundant**: `kubernaut.io` is sufficient, no need for subdomain

### **Alternative 3: `k8s.kubernaut.io`**
❌ **Rejected**
- **Confusing**: Implies official Kubernetes project (`k8s.io` is reserved)
- **Misleading**: V2 will include non-Kubernetes infrastructure (AWS, Azure, GCP)
- **Unnecessary**: `kubernaut.io` already implies Kubernetes context

### **Alternative 4: No API Group (Core Kubernetes)**
❌ **Rejected**
- **Not Allowed**: Only Kubernetes upstream can use core API groups
- **No Custom Domain**: Would require contributing to kubernetes/kubernetes
- **Loss of Identity**: No clear project ownership

---

## 🛠️ Usage Examples

### **kubectl Commands**
```bash
# List all Kubernaut CRDs
kubectl get crds | grep kubernaut.io

# List RemediationRequests
kubectl get remediationrequests.kubernaut.io
kubectl get rr  # Short name

# Describe a specific CRD
kubectl describe remediationrequest my-alert

# Get all Kubernaut resources
kubectl api-resources | grep kubernaut.io
```

### **CRD YAML Example**
```yaml
apiVersion: kubernaut.io/v1
kind: RemediationRequest
metadata:
  name: prometheus-cpu-high-20250106-1230
  namespace: kubernaut-system
spec:
  signal:
    type: prometheus
    alertname: HighCPUUsage
    namespace: production
  priority: P1
  environment: production
status:
  phase: processing
```

### **Go Client Example**
```go
import (
    kubernautv1 "github.com/jordigilh/kubernaut/api/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Create RemediationRequest
rr := &kubernautv1.RemediationRequest{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "prometheus-cpu-high",
        Namespace: "kubernaut-system",
    },
    Spec: kubernautv1.RemediationRequestSpec{
        Signal: kubernautv1.SignalSource{
            Type:      "prometheus",
            Alertname: "HighCPUUsage",
        },
    },
}
```

---

## 📐 Design Principles

### **1. Simplicity**
- Short, memorable API group
- Easy to type and remember
- No deep nesting or complex hierarchies

### **2. Clarity**
- Clear project identification
- No ambiguity about ownership
- Self-documenting in kubectl output

### **3. Scalability**
- Accommodates V2 expansion (multi-cloud, multi-cluster)
- Not tied to specific features or monitoring tools
- Generic enough for future growth

### **4. Convention Adherence**
- Follows Kubernetes API group conventions
- Uses reverse DNS notation
- Matches ecosystem patterns

---

## 🔗 Related Standards

### **Kubernetes API Conventions**
- [API Group Naming](https://kubernetes.io/docs/reference/using-api/api-concepts/#api-groups)
- [CRD Versioning](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/)

### **Ecosystem Examples**
- **Prometheus Operator**: `monitoring.coreos.com`
- **Cert Manager**: `cert-manager.io`
- **ArgoCD**: `argoproj.io`
- **Tekton**: `tekton.dev`
- **Istio**: `networking.istio.io`

---

## 🎯 Future Considerations

### **V2 Multi-Cloud Expansion**
When adding AWS, Azure, GCP infrastructure:
- ✅ **`kubernaut.io/v1`** is generic enough to accommodate
- ✅ CRD fields can be extended without API group change
- ✅ No need for provider-specific API groups

### **Potential Subgroups** (If Needed)
If CRDs become too numerous:
```yaml
# Core orchestration
kubernaut.io/v1

# Cloud-specific (only if necessary)
cloud.kubernaut.io/v1  # For AWS/Azure/GCP-specific CRDs
```

**Current Decision**: Not needed for V1 or V2. Stick with `kubernaut.io/v1`.

---

## ✅ Approval Status

**Decision**: Use `kubernaut.io` as the API group for all Kubernaut CRDs

**Approvers**:
- Architecture Team: ✅ Approved
- Development Team: ✅ Approved
- Operations Team: ✅ Approved

**Date**: October 6, 2025
**Confidence**: 95% (Well-established pattern, follows conventions, scalable)

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Status**: ✅ **APPROVED - PRODUCTION READY**
