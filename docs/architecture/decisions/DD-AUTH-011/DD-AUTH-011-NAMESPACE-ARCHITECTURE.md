# DD-AUTH-011: Namespace Architecture for DataStorage RBAC

**Date**: January 26, 2026
**Status**: ✅ **CORRECTED** - Notification namespace fixed
**Authority**: DD-AUTH-011 (Granular RBAC implementation)

---

## 🎯 **NAMESPACE ARCHITECTURE**

### **Production Services**

| Service | Namespace | ServiceAccount Name | Notes |
|---------|-----------|---------------------|-------|
| **Gateway** | `kubernaut-system` | `gateway-sa` | Main API gateway |
| **AIAnalysis** | `kubernaut-system` | `aianalysis-sa` | AI analysis controller |
| **WorkflowExecution** | `kubernaut-system` | `workflowexecution-sa` | Workflow execution controller |
| **RemediationOrchestrator** | `kubernaut-system` | `remediationorchestrator-sa` | Orchestration controller |
| **Notification** | `kubernaut-notifications` | `notification-controller` | ⚠️ **Different namespace** |
| **AuthWebhook** | `kubernaut-system` | `authwebhook-sa` | Auth webhook handler |
| **SignalProcessing** | `kubernaut-system` | `signalprocessing-sa` | Signal processing |
| **HolmesGPT API** | `kubernaut-system` | `holmesgpt-api-sa` | HolmesGPT API service |
| **DataStorage** | `kubernaut-system` | `data-storage-sa` | DataStorage service (self-audit) |

---

## 🚨 **CRITICAL CORRECTION**

### **Notification Service Namespace**

**Incorrect Assumption** (Initial DD-AUTH-011):
```yaml
# ❌ WRONG
subjects:
  - kind: ServiceAccount
    name: notification-sa
    namespace: kubernaut-system
```

**Correct Configuration**:
```yaml
# ✅ CORRECT
subjects:
  - kind: ServiceAccount
    name: notification-controller
    namespace: kubernaut-notifications
```

**Evidence**:
- `deploy/notification/00-namespace.yaml`: Creates `kubernaut-notifications` namespace
- `deploy/notification/01-rbac.yaml`: ServiceAccount named `notification-controller` in `kubernaut-notifications`
- `deploy/notification/02-deployment.yaml`: Deploys controller to `kubernaut-notifications`

---

## 📋 **UPDATED RBAC CONFIGURATION**

### **RoleBinding for Notification Service**

**File**: `deploy/data-storage/client-rbac-v2.yaml:130-149`

```yaml
---
# RoleBinding: Grant Notification ServiceAccount access to DataStorage
# NOTE: Notification controller is deployed in kubernaut-notifications namespace
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: notification-data-storage-client
  namespace: kubernaut-system
  labels:
    app: notification-controller
    component: rbac
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: data-storage-client
subjects:
  - kind: ServiceAccount
    name: notification-controller
    namespace: kubernaut-notifications  # Cross-namespace binding
```

**Key Points**:
1. ✅ **RoleBinding is in `kubernaut-system`** (where DataStorage is deployed)
2. ✅ **Subject references `kubernaut-notifications` namespace** (where Notification controller is)
3. ✅ **Cross-namespace binding allowed** (ClusterRole + RoleBinding pattern)

---

## 🔍 **CROSS-NAMESPACE ACCESS PATTERN**

### **How It Works**

```
┌─────────────────────────────────────┐
│  kubernaut-notifications namespace  │
│                                     │
│  ┌──────────────────────────────┐  │
│  │ ServiceAccount:              │  │
│  │  notification-controller     │  │
│  └──────────────────────────────┘  │
│                │                    │
└────────────────┼────────────────────┘
                 │
                 │ Uses Token
                 ↓
┌─────────────────────────────────────┐
│  kubernaut-system namespace         │
│                                     │
│  ┌──────────────────────────────┐  │
│  │ RoleBinding:                 │  │
│  │  notification-data-storage-  │  │
│  │  client                      │  │
│  │                              │  │
│  │  Subjects:                   │  │
│  │  - SA: notification-         │  │
│  │    controller                │  │
│  │    Namespace: kubernaut-     │  │
│  │    notifications             │  │
│  └──────────────────────────────┘  │
│                │                    │
│                ↓                    │
│  ┌──────────────────────────────┐  │
│  │ OAuth2-Proxy Sidecar         │  │
│  │ (DataStorage)                │  │
│  │                              │  │
│  │ SAR Check:                   │  │
│  │  Can SA notification-        │  │
│  │  controller CREATE           │  │
│  │  service/data-storage-       │  │
│  │  service?                    │  │
│  │                              │  │
│  │  ✅ YES (via RoleBinding)    │  │
│  └──────────────────────────────┘  │
│                │                    │
│                ↓                    │
│  ┌──────────────────────────────┐  │
│  │ DataStorage Service          │  │
│  │ (Port 8081)                  │  │
│  └──────────────────────────────┘  │
└─────────────────────────────────────┘
```

---

## 🔧 **VERIFICATION COMMANDS**

### **Verify Notification Namespace**

```bash
# 1. Check Notification namespace exists
kubectl get namespace kubernaut-notifications
# Expected: NAME                      STATUS   AGE
#          kubernaut-notifications   Active   ...

# 2. Check Notification ServiceAccount
kubectl get serviceaccount -n kubernaut-notifications notification-controller
# Expected: NAME                     SECRETS   AGE
#          notification-controller   1         ...

# 3. Check Notification deployment
kubectl get deployment -n kubernaut-notifications notification-controller
# Expected: NAME                     READY   UP-TO-DATE   AVAILABLE   AGE
#          notification-controller   1/1     1            1           ...
```

### **Verify RBAC Binding**

```bash
# 4. Check RoleBinding in kubernaut-system (where DataStorage is)
kubectl get rolebinding -n kubernaut-system notification-data-storage-client -o yaml

# Expected output should show:
#   subjects:
#   - kind: ServiceAccount
#     name: notification-controller
#     namespace: kubernaut-notifications

# 5. Test Notification SA can access DataStorage
kubectl auth can-i create services/data-storage-service \
  --as=system:serviceaccount:kubernaut-notifications:notification-controller \
  -n kubernaut-system
# Expected: yes
```

---

## 🌐 **NETWORKPOLICY IMPLICATIONS**

### **DataStorage NetworkPolicy**

**File**: `deploy/data-storage/networkpolicy.yaml`

The NetworkPolicy must allow ingress from `kubernaut-notifications` namespace:

```yaml
ingress:
  # Allow from all kubernaut-system services
  - from:
      - podSelector: {}  # All pods in kubernaut-system
    ports:
      - protocol: TCP
        port: 8080  # oauth-proxy port
  
  # Allow from Notification controller (different namespace)
  - from:
      - namespaceSelector:
          matchLabels:
            app.kubernetes.io/name: kubernaut
            app.kubernetes.io/component: notification-controller
    ports:
      - protocol: TCP
        port: 8080
```

**Authority**: DD-AUTH-009 Phase 6 (NetworkPolicy Update)

---

## 📊 **RBAC SUMMARY**

### **Total RoleBindings Required**

| Category | Count | Namespaces |
|----------|-------|------------|
| **Production Services** | 8 | `kubernaut-system` (7) + `kubernaut-notifications` (1) |
| **E2E Test Suites** | 6 | `kubernaut-system` (dynamic) |
| **TOTAL** | **14** | - |

### **Special Cases**

1. **Notification Service**:
   - ✅ Only service NOT in `kubernaut-system`
   - ✅ Uses `notification-controller` (not `notification-sa`)
   - ✅ Requires cross-namespace RoleBinding

2. **DataStorage Self-Audit**:
   - ✅ DataStorage has RBAC to access itself
   - ✅ Used for self-auditing operations

---

## ✅ **VALIDATION CHECKLIST**

After applying `client-rbac-v2.yaml`:

- [ ] Notification RoleBinding references correct namespace (`kubernaut-notifications`)
- [ ] Notification RoleBinding references correct ServiceAccount (`notification-controller`)
- [ ] `kubectl auth can-i` confirms Notification SA has `create` permission
- [ ] NetworkPolicy allows ingress from `kubernaut-notifications` namespace
- [ ] All other services remain in `kubernaut-system` namespace

---

## 📚 **RELATED DECISIONS**

- **DD-AUTH-011**: Granular RBAC & SAR Verb Mapping
- **DD-AUTH-009**: OAuth2-Proxy Migration (NetworkPolicy section)
- **DD-TEST-001**: E2E Test Namespace Allocation

---

## 🎯 **KEY TAKEAWAYS**

1. **Notification is the ONLY service in a different namespace**
2. **ServiceAccount name is `notification-controller`** (not `notification-sa`)
3. **Cross-namespace RBAC works** with ClusterRole + RoleBinding pattern
4. **NetworkPolicy must explicitly allow** `kubernaut-notifications` namespace
5. **All other services are in `kubernaut-system`**

---

**Document Version**: 1.0  
**Last Updated**: January 26, 2026  
**Status**: ✅ CORRECTED
