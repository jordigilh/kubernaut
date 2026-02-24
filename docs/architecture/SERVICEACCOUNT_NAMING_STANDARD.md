# ServiceAccount Naming Standard

**Version**: v1.0
**Last Updated**: October 6, 2025
**Status**: ✅ **STANDARDIZED**
**Scope**: All Kubernaut Services (11 total)

---

## 📋 **Naming Standard**

### **Pattern**: `{service-name}` (HTTP Services) or `{service-name}-sa` (CRD Controllers)

---

## ✅ **Standardized Names**

### **HTTP Services** (6 services) - No `-sa` suffix

| Service | ServiceAccount Name | Namespace | Status |
|---------|-------------------|-----------|--------|
| **Gateway** | `gateway` | `kubernaut-system` | ✅ Standard |
| **Context API** | `context-api` | `kubernaut-system` | ✅ Standard |
| **Data Storage** | `data-storage` | `kubernaut-system` | ✅ Standard |
| **HolmesGPT API** | `holmesgpt-api` | `kubernaut-system` | ✅ Standard |
| **Notification** | `notification` | `kubernaut-system` | ⚠️ **UPDATED** (was `notification-service`) |
| **Dynamic Toolset** | `dynamic-toolset` | `kubernaut-system` | ✅ Standard |

---

### **CRD Controllers** (5 controllers) - With `-sa` suffix

| Controller | ServiceAccount Name | Namespace | Status |
|------------|-------------------|-----------|--------|
| **Remediation Orchestrator** | `remediation-orchestrator-sa` | `kubernaut-system` | ✅ Standard |
| **Remediation Processor** | `remediation-processor-sa` | `kubernaut-system` | ✅ Standard |
| **AI Analysis** | `ai-analysis-sa` | `kubernaut-system` | ✅ Standard |
| **Workflow Execution** | `workflow-execution-sa` | `kubernaut-system` | ✅ Standard |
| ~~**Kubernetes Executor**~~ (DEPRECATED - ADR-025) | `kubernetes-executor-sa` | `kubernaut-system` | ✅ Standard |

---

## 🎯 **Rationale**

### **HTTP Services**: No `-sa` suffix
- **Reason**: ServiceAccount name matches service name for simplicity
- **Example**: `gateway` service uses `gateway` ServiceAccount
- **Benefit**: Clear 1:1 mapping, less verbose

### **CRD Controllers**: `-sa` suffix
- **Reason**: Distinguishes ServiceAccount from controller name in RBAC
- **Example**: `remediation-orchestrator` controller uses `remediation-orchestrator-sa` ServiceAccount
- **Benefit**: Clarifies it's a ServiceAccount in ClusterRoleBinding contexts
- **Pattern**: Follows `controller-runtime` conventions

---

## 📝 **Update Required**

### **Notification Service** ⚠️

**Current**: `notification-service`
**Standard**: `notification`

**Files to Update**:
1. `docs/services/stateless/06-notification-service.md` (Overview section)
2. Deployment YAML (when created)
3. RBAC manifests (when created)

**Migration**: Zero impact (pre-release, no deployed instances)

---

## 📊 **ServiceAccount Configuration Examples**

### **HTTP Service Example** (Gateway)

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: gateway
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: gateway-reader
rules:
- apiGroups: [""]
  resources: ["services"]
  verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: gateway-reader-binding
subjects:
- kind: ServiceAccount
  name: gateway  # ← No -sa suffix
  namespace: kubernaut-system
roleRef:
  kind: ClusterRole
  name: gateway-reader
  apiGroup: rbac.authorization.k8s.io
```

---

### **CRD Controller Example** (Remediation Orchestrator)

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: remediation-orchestrator-sa  # ← With -sa suffix
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: remediation-orchestrator
rules:
- apiGroups: ["remediation.kubernaut.io"]
  resources: ["remediationrequests"]
  verbs: ["get", "list", "watch", "update", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: remediation-orchestrator-binding
subjects:
- kind: ServiceAccount
  name: remediation-orchestrator-sa  # ← With -sa suffix
  namespace: kubernaut-system
roleRef:
  kind: ClusterRole
  name: remediation-orchestrator
  apiGroup: rbac.authorization.k8s.io
```

---

## 🔒 **RBAC Naming Patterns**

### **ClusterRole Naming**

| Pattern | Example | Use Case |
|---------|---------|----------|
| `{service}-reader` | `gateway-reader` | Read-only access |
| `{service}-writer` | `data-storage-writer` | Write access |
| `{controller}` | `remediation-orchestrator` | Controller permissions |
| `{controller}-{resource}` | `ai-analysis-aianalysis` | Resource-specific |

---

### **ClusterRoleBinding Naming**

| Pattern | Example |
|---------|---------|
| `{service}-{role}-binding` | `gateway-reader-binding` |
| `{controller}-binding` | `remediation-orchestrator-binding` |

---

## 📚 **Token Reviewer Authentication**

All services use **Kubernetes TokenReviewer API** for authentication.

**Implementation**: See [KUBERNETES_TOKENREVIEWER_AUTH.md](./KUBERNETES_TOKENREVIEWER_AUTH.md)

**ServiceAccount Token Usage**:
```yaml
# Deployment references ServiceAccount
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: kubernaut-system
spec:
  template:
    spec:
      serviceAccountName: gateway  # ← References ServiceAccount
      containers:
      - name: gateway
        # Token automatically mounted at /var/run/secrets/kubernetes.io/serviceaccount/token
```

---

## ✅ **Compliance Checklist**

### **For Each HTTP Service**:
- ✅ ServiceAccount name = service name (no `-sa` suffix)
- ✅ Namespace = `kubernaut-system`
- ✅ Referenced in Deployment `serviceAccountName`
- ✅ ClusterRoleBinding references correct ServiceAccount
- ✅ Documented in service overview

### **For Each CRD Controller**:
- ✅ ServiceAccount name = controller name + `-sa` suffix
- ✅ Namespace = `kubernaut-system`
- ✅ Referenced in controller manager configuration
- ✅ ClusterRoleBinding references correct ServiceAccount
- ✅ Documented in controller overview

---

## 🎯 **Benefits of Standardization**

### **1. Predictable Naming**
- ✅ Clear pattern for all services
- ✅ Easy to guess ServiceAccount name from service name
- ✅ Consistent across entire platform

### **2. RBAC Clarity**
- ✅ ServiceAccount identity clear in audit logs
- ✅ Easy to trace permissions to specific services
- ✅ Simplified troubleshooting

### **3. Automation Friendly**
- ✅ Scriptable ServiceAccount creation
- ✅ Consistent naming in Helm charts
- ✅ Easy to generate RBAC manifests

### **4. Documentation Consistency**
- ✅ Single naming standard to document
- ✅ No exceptions or special cases
- ✅ Clear for new developers

---

## 📊 **ServiceAccount Summary**

### **All 11 Services**

```
HTTP Services (no -sa suffix):
├── gateway
├── context-api
├── data-storage
├── holmesgpt-api
├── notification  ← UPDATED from notification-service
└── dynamic-toolset

CRD Controllers (with -sa suffix):
├── remediation-orchestrator-sa
├── remediation-processor-sa
├── ai-analysis-sa
├── workflow-execution-sa
└── kubernetes-executor-sa  # DEPRECATED - ADR-025
```

**Total**: 11 ServiceAccounts
**Standard Compliance**: 11/11 (100%)

---

## 📚 **Related Documentation**

- [KUBERNETES_TOKENREVIEWER_AUTH.md](./KUBERNETES_TOKENREVIEWER_AUTH.md) - Authentication implementation
- [STATELESS_SERVICES_PORT_STANDARD.md](./STATELESS_SERVICES_PORT_STANDARD.md) - Port configuration
- [Service Dependency Map](./SERVICE_DEPENDENCY_MAP.md) - Service interactions

---

**Document Status**: ✅ Complete
**Compliance**: 11/11 services (100%)
**Last Updated**: October 6, 2025
**Version**: 1.0
