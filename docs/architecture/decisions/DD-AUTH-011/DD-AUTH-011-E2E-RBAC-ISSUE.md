# DD-AUTH-011: E2E RBAC Issue - Notification E2E Tests

**Date**: January 26, 2026
**Status**: 🚨 **CRITICAL ISSUE FOUND**
**Priority**: HIGH (Blocks E2E test execution with real auth)

---

## 🚨 **ISSUE DISCOVERED**

### **Problem**

The Notification E2E tests use a **different architecture** than assumed in `client-rbac-v2.yaml`:

**Assumed** (in `client-rbac-v2.yaml`):
```yaml
# RoleBinding for Notification E2E
subjects:
  - kind: ServiceAccount
    name: notification-controller
    namespace: kubernaut-system  # ❌ WRONG
```

**Actual** (in E2E tests):
```yaml
# Notification E2E actual deployment
namespace: notification-e2e  # Dynamic namespace
serviceAccount: notification-controller
dataStorageURL: http://datastorage.notification-e2e.svc.cluster.local:8080
```

---

## 🔍 **ROOT CAUSE ANALYSIS**

### **Evidence 1: E2E Test Suite**

**File**: `test/e2e/notification/notification_e2e_suite_test.go:67`

```go
controllerNamespace string = "notification-e2e"
```

### **Evidence 2: ServiceAccount**

**File**: `test/e2e/notification/manifests/notification-rbac.yaml:5`

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: notification-controller  # ✅ Correct
  # Deployed via: kubectl apply -f rbac.yaml -n notification-e2e
```

### **Evidence 3: DataStorage Access Required**

**File**: `test/e2e/notification/manifests/notification-configmap.yaml:92`

```yaml
infrastructure:
  data_storage_url: "http://datastorage.${NAMESPACE}.svc.cluster.local:8080"
```

**Result after envsubst**:
```yaml
data_storage_url: "http://datastorage.notification-e2e.svc.cluster.local:8080"
```

### **Evidence 4: DataStorage Deployment Location**

**File**: `test/infrastructure/notification_e2e.go:334`

```go
// Deploy Data Storage infrastructure in notification-e2e namespace
func DeployNotificationDataStorageServices(ctx context.Context, namespace, kubeconfigPath, dataStorageImage string, writer io.Writer) error {
    // DataStorage deployed in SAME namespace as Notification controller
    return DeployDataStorageTestServicesWithNodePort(ctx, namespace, ...)
}
```

---

## 📊 **ARCHITECTURE COMPARISON**

### **Production** (kubernaut-notifications)

```
┌──────────────────────────────────────┐
│  kubernaut-notifications namespace   │
│                                      │
│  ┌────────────────────────────────┐ │
│  │ SA: notification-controller    │ │
│  └────────────────────────────────┘ │
│              │                       │
└──────────────┼───────────────────────┘
               │ Token
               ↓
┌──────────────────────────────────────┐
│  kubernaut-system namespace          │
│                                      │
│  ┌────────────────────────────────┐ │
│  │ RoleBinding:                   │ │
│  │  notification-data-storage-    │ │
│  │  client                        │ │
│  │                                │ │
│  │  Subjects:                     │ │
│  │  - SA: notification-controller │ │
│  │    NS: kubernaut-notifications │ │
│  └────────────────────────────────┘ │
│              ↓                       │
│  ┌────────────────────────────────┐ │
│  │ DataStorage Service            │ │
│  └────────────────────────────────┘ │
└──────────────────────────────────────┘
```

### **E2E Tests** (notification-e2e)

```
┌──────────────────────────────────────┐
│  notification-e2e namespace          │
│                                      │
│  ┌────────────────────────────────┐ │
│  │ SA: notification-controller    │ │
│  └────────────────────────────────┘ │
│              │                       │
│              │ Token                 │
│              ↓                       │
│  ┌────────────────────────────────┐ │
│  │ RoleBinding:                   │ │
│  │  ❌ MISSING!                    │ │
│  │                                │ │
│  │  Needs:                        │ │
│  │  - SA: notification-controller │ │
│  │    NS: notification-e2e        │ │
│  │  - Access to DataStorage       │ │
│  └────────────────────────────────┘ │
│              ↓                       │
│  ┌────────────────────────────────┐ │
│  │ DataStorage Service            │ │
│  │ (in same namespace)            │ │
│  └────────────────────────────────┘ │
└──────────────────────────────────────┘
```

---

## ✅ **SOLUTION**

### **Option A: Dynamic RoleBinding Creation** ⭐ **RECOMMENDED**

E2E infrastructure should create the RoleBinding programmatically when deploying audit infrastructure.

**File**: `test/infrastructure/notification_e2e.go` (update `DeployNotificationAuditInfrastructure`)

```go
func DeployNotificationAuditInfrastructure(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
    // ... existing deployment code ...

    // DD-AUTH-011: Create RoleBinding for Notification controller to access DataStorage
    _, _ = fmt.Fprintf(writer, "🔐 Creating DataStorage access RoleBinding for Notification controller...\n")
    if err := createDataStorageAccessRoleBinding(ctx, namespace, kubeconfigPath, writer); err != nil {
        return fmt.Errorf("failed to create DataStorage access RoleBinding: %w", err)
    }

    return nil
}

// createDataStorageAccessRoleBinding creates RoleBinding for Notification controller
// to access DataStorage service in the same namespace
func createDataStorageAccessRoleBinding(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
    clientset, err := getKubernetesClient(kubeconfigPath)
    if err != nil {
        return err
    }

    rb := &rbacv1.RoleBinding{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "notification-controller-datastorage-access",
            Namespace: namespace,  // Same namespace as DataStorage
            Labels: map[string]string{
                "app":       "notification-controller",
                "component": "rbac",
                "test":      "e2e",
            },
        },
        RoleRef: rbacv1.RoleRef{
            APIGroup: "rbac.authorization.k8s.io",
            Kind:     "ClusterRole",
            Name:     "data-storage-client",  // DD-AUTH-011: verb:"create"
        },
        Subjects: []rbacv1.Subject{
            {
                Kind:      "ServiceAccount",
                Name:      "notification-controller",
                Namespace: namespace,  // Same namespace
            },
        },
    }

    _, err = clientset.RbacV1().RoleBindings(namespace).Create(ctx, rb, metav1.CreateOptions{})
    if err != nil && !apierrors.IsAlreadyExists(err) {
        return fmt.Errorf("failed to create RoleBinding: %w", err)
    }

    _, _ = fmt.Fprintf(writer, "✅ RoleBinding created: notification-controller-datastorage-access\n")
    _, _ = fmt.Fprintf(writer, "   Namespace: %s\n", namespace)
    _, _ = fmt.Fprintf(writer, "   ServiceAccount: notification-controller\n")
    _, _ = fmt.Fprintf(writer, "   ClusterRole: data-storage-client (verb:create)\n")

    return nil
}
```

### **Option B: Static RBAC Manifest** (NOT RECOMMENDED)

Create a static manifest, but it would be **hardcoded** to `notification-e2e` namespace.

**Problem**: E2E tests should support dynamic namespaces (per DD-TEST-001).

---

## 📋 **ACTION ITEMS**

### **Immediate**

1. ✅ Remove static Notification E2E RoleBinding from `client-rbac-v2.yaml`
2. ⏳ Implement `createDataStorageAccessRoleBinding()` in `test/infrastructure/notification_e2e.go`
3. ⏳ Update `DeployNotificationAuditInfrastructure()` to call RBAC creation

### **Testing**

1. ⏳ Verify RoleBinding created in `notification-e2e` namespace
2. ⏳ Verify Notification controller can access DataStorage
3. ⏳ Verify audit events are written to DataStorage
4. ⏳ Run Notification E2E tests with real OAuth2-proxy (no pass-through)

---

## 🎯 **VALIDATION COMMANDS**

```bash
# 1. Check if RoleBinding exists in E2E namespace
kubectl get rolebinding -n notification-e2e notification-controller-datastorage-access

# Expected:
# NAME                                           ROLE                            AGE
# notification-controller-datastorage-access     ClusterRole/data-storage-client 1m

# 2. Verify RoleBinding references correct SA
kubectl get rolebinding -n notification-e2e notification-controller-datastorage-access -o yaml

# Expected subjects:
# subjects:
# - kind: ServiceAccount
#   name: notification-controller
#   namespace: notification-e2e

# 3. Test Notification controller SA can access DataStorage
kubectl auth can-i create services/datastorage \
  --as=system:serviceaccount:notification-e2e:notification-controller \
  -n notification-e2e

# Expected: yes
```

---

## ✅ **UPDATED client-rbac-v2.yaml**

**Remove** the static Notification E2E RoleBinding:

```yaml
# ❌ REMOVE THIS - E2E creates dynamically
---
# RoleBinding: Grant Notification E2E ServiceAccount access
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: notification-e2e-data-storage-client
  namespace: kubernaut-system
  ...
```

**Replace with comment**:

```yaml
# ========================================
# NOTE: Notification E2E RBAC
# ========================================
# Notification E2E tests deploy to dynamic namespaces (notification-e2e, test-*, etc.)
# and deploy DataStorage in the SAME namespace as the Notification controller.
#
# RoleBinding for DataStorage access is created PROGRAMMATICALLY by E2E infrastructure
# in test/infrastructure/notification_e2e.go during audit infrastructure deployment.
#
# Authority: DD-AUTH-011-E2E-RBAC-ISSUE.md
# ========================================
```

---

## 📚 **REFERENCES**

- **DD-AUTH-011**: Granular RBAC & SAR Verb Mapping
- **DD-TEST-001**: E2E Dynamic Namespace Allocation
- **ADR-032**: Data Storage Audit Integration

---

**Document Version**: 1.0  
**Last Updated**: January 26, 2026  
**Status**: 🚨 CRITICAL ISSUE - Solution documented, implementation required
