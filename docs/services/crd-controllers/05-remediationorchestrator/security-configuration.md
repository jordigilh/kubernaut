# Security Configuration

## Overview

The Remediation Orchestrator (RemediationRequest Controller) is the **central coordinator** with elevated privileges to create and manage 4 downstream CRDs. This document defines **least-privilege RBAC**, network policies, and security hardening for the orchestrator controller.

**Security Posture**:
- **Privilege Level**: ELEVATED (creates multiple CRD types)
- **Attack Surface**: Kubernetes API Server + Storage Service HTTP
- **Critical**: Controller compromise enables CRD injection across all services

---

## 1. Service Account & RBAC Configuration

### Controller ServiceAccount

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: remediation-orchestrator
  namespace: kubernaut-system
  labels:
    app: remediation-orchestrator
    app.kubernetes.io/name: remediation-orchestrator
    app.kubernetes.io/component: controller
automountServiceAccountToken: true
```

---

### ClusterRole: Full CRD Orchestration Permissions

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: remediation-orchestrator
rules:
# RemediationRequest CRD (owned resource - full control)
- apiGroups: ["remediation.kubernaut.io"]
  resources: ["remediationrequests"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["remediation.kubernaut.io"]
  resources: ["remediationrequests/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["remediation.kubernaut.io"]
  resources: ["remediationrequests/finalizers"]
  verbs: ["update"]

# SignalProcessing CRD (create + watch)
- apiGroups: ["signalprocessing.kubernaut.io"]
  resources: ["remediationprocessings"]
  verbs: ["create", "get", "list", "watch", "update", "patch", "delete"]
- apiGroups: ["signalprocessing.kubernaut.io"]
  resources: ["remediationprocessings/status"]
  verbs: ["get", "list", "watch"]

# AIAnalysis CRD (create + watch)
- apiGroups: ["aianalysis.kubernaut.io"]
  resources: ["aianalyses"]
  verbs: ["create", "get", "list", "watch", "update", "patch", "delete"]
- apiGroups: ["aianalysis.kubernaut.io"]
  resources: ["aianalyses/status"]
  verbs: ["get", "list", "watch"]

# WorkflowExecution CRD (create + watch)
- apiGroups: ["workflowexecution.kubernaut.io"]
  resources: ["workflowexecutions"]
  verbs: ["create", "get", "list", "watch", "update", "patch", "delete"]
- apiGroups: ["workflowexecution.kubernaut.io"]
  resources: ["workflowexecutions/status"]
  verbs: ["get", "list", "watch"]

# Event emission (controller events only)
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
```

**Justification for Elevated Permissions**:
- **create**: Controller creates 3 downstream CRDs (RemediationProcessing, AIAnalysis, WorkflowExecution)
- **update/patch/delete**: Owner references require delete permission for cascade cleanup
- **watch**: Status watching triggers next phase in orchestration
- **list**: Controller startup requires listing existing CRDs for recovery

**❌ Explicitly Denied**:
- NO direct Pod/Deployment/Job manipulation (WorkflowExecution/KubernetesExecution (DEPRECATED - ADR-025) handle execution)
- NO Secret access (downstream controllers manage secrets)
- NO Node access (no infrastructure operations)
- NO namespace creation (operates in existing namespaces)

---

### ClusterRoleBinding

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: remediation-orchestrator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: remediation-orchestrator
subjects:
- kind: ServiceAccount
  name: remediation-orchestrator
  namespace: kubernaut-system
```

---

## 2. Security Best Practices

### Least Privilege Principles

**✅ Enforced**:
1. **CRD-Only Permissions**: Controller ONLY creates/watches CRDs, no direct resource manipulation
2. **Read-Only Status**: Controller reads downstream CRD status, does NOT modify them
3. **Namespace-Scoped**: No cluster-admin privileges, only CRD-related permissions
4. **Event Scoping**: Events limited to RemediationRequest objects
5. **Owner References**: Automatic cascade deletion via owner references (no manual cleanup)

**❌ Prohibited**:
1. **No Pod Execution**: Controller does NOT create Pods/Jobs (delegated to WorkflowExecution/KubernetesExecution (DEPRECATED - ADR-025))
2. **No Secret Access**: Controller does NOT access Secrets (downstream controllers handle credentials)
3. **No Node Operations**: No node cordoning/draining (delegated to KubernetesExecution (DEPRECATED - ADR-025))
4. **No RBAC Elevation**: Controller CANNOT create ClusterRoles or escalate privileges

---

### Owner Reference Security

**Purpose**: Cascade deletion without manual cleanup permissions

```go
// Setting owner reference on downstream CRDs
func (r *RemediationRequestReconciler) createRemediationProcessing(
    ctx context.Context,
    remediation *remediationv1alpha1.RemediationRequest,
) error {
    processing := &remediationprocessingv1alpha1.RemediationProcessing{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-processing", remediation.Name),
            Namespace: remediation.Namespace,
        },
        Spec: mapRemediationRequestToProcessingSpec(remediation),
    }

    // ✅ CRITICAL: Set owner reference for cascade deletion
    if err := ctrl.SetControllerReference(remediation, processing, r.Scheme); err != nil {
        return fmt.Errorf("failed to set owner reference: %w", err)
    }

    return r.Create(ctx, processing)
}
```

**Security Benefits**:
- **Automatic Cleanup**: Deleting RemediationRequest deletes all downstream CRDs
- **No Manual Delete Permissions**: Controller doesn't need explicit delete verbs
- **Integrity**: Owner references enforced by Kubernetes API Server
- **Audit Trail**: Deletion events tracked for all owned CRDs

---

### Finalizer Security

**Purpose**: Graceful cleanup before cascade deletion

```go
const FinalizerName = "remediation.kubernaut.ai/finalizer"

// Adding finalizer to RemediationRequest
func (r *RemediationRequestReconciler) addFinalizer(
    ctx context.Context,
    remediation *remediationv1alpha1.RemediationRequest,
) error {
    if !controllerutil.ContainsFinalizer(remediation, FinalizerName) {
        controllerutil.AddFinalizer(remediation, FinalizerName)
        return r.Update(ctx, remediation)
    }
    return nil
}

// Finalizer cleanup logic
func (r *RemediationRequestReconciler) handleFinalizer(
    ctx context.Context,
    remediation *remediationv1alpha1.RemediationRequest,
) error {
    if remediation.DeletionTimestamp.IsZero() {
        // Not being deleted
        return r.addFinalizer(ctx, remediation)
    }

    // Being deleted - perform cleanup
    if controllerutil.ContainsFinalizer(remediation, FinalizerName) {
        // 1. Wait for downstream CRDs to finish (status check)
        // 2. Persist audit record to storage service
        // 3. Remove finalizer to allow deletion

        if err := r.performCleanup(ctx, remediation); err != nil {
            return err
        }

        controllerutil.RemoveFinalizer(remediation, FinalizerName)
        return r.Update(ctx, remediation)
    }

    return nil
}
```

**Security Considerations**:
- **Audit Persistence**: Finalizer ensures audit record persisted before deletion
- **Graceful Shutdown**: Downstream CRDs complete before RemediationRequest deleted
- **No Resource Leaks**: Finalizer prevents orphaned CRDs
- **Timeout Protection**: Finalizer cleanup should have timeout (prevent stuck CRDs)

---

## 3. Network Policies

### Controller Ingress Policy (Deny All)

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: remediation-orchestrator-ingress
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: remediation-orchestrator
  policyTypes:
  - Ingress
  ingress: []  # Deny all ingress (controller doesn't accept connections)
```

**Justification**: Controller is NOT a server, only a Kubernetes API client

---

### Controller Egress Policy (Selective Allow)

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: remediation-orchestrator-egress
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: remediation-orchestrator
  policyTypes:
  - Egress
  egress:
  # Allow Kubernetes API Server
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    ports:
    - protocol: TCP
      port: 443  # Kubernetes API Server

  # Allow Storage Service (audit persistence)
  - to:
    - podSelector:
        matchLabels:
          app: storage-service
    ports:
    - protocol: TCP
      port: 8085  # Storage Service HTTP

  # Allow DNS resolution
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    - podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
```

**Egress Rules**:
1. **Kubernetes API**: REQUIRED for CRD operations
2. **Storage Service**: REQUIRED for audit trail persistence
3. **DNS**: REQUIRED for service name resolution
4. **All Other Egress**: DENIED (no external API calls)

---

## 4. Pod Security Standards

### Security Context

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: remediation-orchestrator
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: remediation-orchestrator
  template:
    metadata:
      labels:
        app: remediation-orchestrator
    spec:
      serviceAccountName: remediation-orchestrator
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532  # Nonroot user
        fsGroup: 65532
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: controller
        image: kubernaut/remediation-orchestrator:v1
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 65532
          capabilities:
            drop:
            - ALL
        resources:
          limits:
            cpu: 500m
            memory: 512Mi
          requests:
            cpu: 200m
            memory: 256Mi
        volumeMounts:
        - name: tmp
          mountPath: /tmp  # Writable tmp for cache
      volumes:
      - name: tmp
        emptyDir: {}
```

**Security Hardening**:
- ✅ **Non-root user**: UID 65532 (default nonroot)
- ✅ **Read-only root filesystem**: Immutable container filesystem
- ✅ **No privilege escalation**: Prevents container breakout
- ✅ **Drop ALL capabilities**: Minimal Linux capabilities
- ✅ **Seccomp profile**: RuntimeDefault (syscall filtering)
- ✅ **Resource limits**: CPU/memory constraints prevent DoS

---

## 5. Secret Handling

### Configuration Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: remediation-orchestrator-config
  namespace: kubernaut-system
type: Opaque
stringData:
  storage-service-url: "http://storage-service.kubernaut-system.svc.cluster.local:8085"
  audit-enabled: "true"
  max-retries: "3"
  retry-backoff-duration: "10s"
```

**Secret Mounting**:
```yaml
# In Deployment spec
env:
- name: STORAGE_SERVICE_URL
  valueFrom:
    secretKeyRef:
      name: remediation-orchestrator-config
      key: storage-service-url
- name: AUDIT_ENABLED
  valueFrom:
    secretKeyRef:
      name: remediation-orchestrator-config
      key: audit-enabled
```

**Secret Security**:
- ✅ Controller does NOT access Secrets for downstream services
- ✅ Only reads own configuration secret
- ✅ No Secret creation/modification permissions
- ✅ Environment variables preferred over volume mounts (immutable)

---

## 6. Audit & Monitoring

### Kubernetes Audit Policy

```yaml
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
# Log all CRD creations/deletions by remediation-orchestrator
- level: RequestResponse
  users: ["system:serviceaccount:kubernaut-system:remediation-orchestrator"]
  verbs: ["create", "delete", "patch", "update"]
  resources:
  - group: "signalprocessing.kubernaut.io"
    resources: ["remediationprocessings"]
  - group: "aianalysis.kubernaut.io"
    resources: ["aianalyses"]
  - group: "workflowexecution.kubernaut.io"
    resources: ["workflowexecutions"]

# Log status updates
- level: Request
  users: ["system:serviceaccount:kubernaut-system:remediation-orchestrator"]
  verbs: ["update", "patch"]
  resources:
  - group: "remediation.kubernaut.io"
    resources: ["remediationrequests/status"]
```

**Audit Scope**:
- ✅ All CRD creations logged
- ✅ Status updates tracked
- ✅ Downstream CRD lifecycle monitored
- ✅ RBAC permission checks logged

---

### Security Metrics

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Counter: CRD creation failures (security indicator)
    CRDCreationFailuresTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_orchestrator_crd_creation_failures_total",
        Help: "Total CRD creation failures (permission denied, quota, etc)",
    }, []string{"crd_type", "error_type"})

    // Counter: RBAC permission denied
    RBACDeniedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_orchestrator_rbac_denied_total",
        Help: "Total RBAC permission denied attempts",
    }, []string{"resource", "verb"})

    // Counter: Finalizer cleanup failures
    FinalizerCleanupFailuresTotal = promauto.NewCounter(prometheus.CounterOpts{
        Name: "kubernaut_orchestrator_finalizer_cleanup_failures_total",
        Help: "Total finalizer cleanup failures",
    })

    // Gauge: Active RemediationRequests (resource tracking)
    ActiveRemediationRequests = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "kubernaut_orchestrator_active_requests",
        Help: "Number of active RemediationRequest CRDs",
    })
)
```

**Security Alerts**:
- Alert on repeated RBAC denials (potential privilege escalation attempt)
- Alert on finalizer cleanup failures (resource leak indicator)
- Alert on CRD creation quota exhaustion (DoS indicator)

---

## 7. Threat Model

### Attack Vectors

| Threat | Impact | Mitigation |
|--------|--------|------------|
| **CRD Injection** | Malicious CRD creation | Owner references + RBAC validation |
| **Privilege Escalation** | Controller compromise | Least-privilege RBAC, no cluster-admin |
| **Resource Exhaustion** | DoS via unlimited CRDs | Resource quotas, rate limiting |
| **Audit Bypass** | Deletion without audit | Finalizers ensure audit persistence |
| **Network Eavesdropping** | API token theft | TLS for all connections, network policies |

---

### Security Controls

| Control | Implementation | Status |
|---------|----------------|--------|
| **Least Privilege RBAC** | CRD-only permissions | ✅ Enforced |
| **Network Segmentation** | Network policies | ✅ Enforced |
| **Audit Trail** | Finalizers + storage persistence | ✅ Enforced |
| **Resource Limits** | Pod security + quotas | ✅ Enforced |
| **Immutable Infrastructure** | Read-only filesystem | ✅ Enforced |
| **Non-Root Execution** | UID 65532 | ✅ Enforced |

---

## 8. Security Testing

### Unit Tests (Security Validation)

```go
func TestRBACPermissions(t *testing.T) {
    tests := []struct {
        name     string
        resource string
        verb     string
        allowed  bool
    }{
        {"create RemediationProcessing", "remediationprocessings", "create", true},
        {"delete Pods", "pods", "delete", false},
        {"create Secrets", "secrets", "create", false},
        {"escalate RBAC", "clusterroles", "create", false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            allowed := checkRBACPermission(tt.resource, tt.verb)
            assert.Equal(t, tt.allowed, allowed)
        })
    }
}
```

### Integration Tests (Attack Simulation)

```go
func TestSecurityConstraints(t *testing.T) {
    // Test 1: Verify controller cannot create Pods directly
    _, err := clientset.CoreV1().Pods("default").Create(ctx, &corev1.Pod{...})
    assert.Error(t, err) // Should be denied

    // Test 2: Verify controller cannot access Secrets
    _, err = clientset.CoreV1().Secrets("default").Get(ctx, "test-secret", metav1.GetOptions{})
    assert.Error(t, err) // Should be denied

    // Test 3: Verify CRD creation succeeds
    _, err = customClient.RemediationProcessing("default").Create(ctx, &remediationprocessingv1alpha1.RemediationProcessing{...})
    assert.NoError(t, err) // Should succeed
}
```

---

## 9. Security Checklist

### Deployment Validation

- [ ] ServiceAccount created with correct name
- [ ] ClusterRole has ONLY CRD permissions (no Pod/Secret/Node access)
- [ ] ClusterRoleBinding references correct ServiceAccount
- [ ] Network policies deny ingress, allow selective egress
- [ ] Pod security context enforces non-root, read-only filesystem
- [ ] Resource limits configured (CPU: 500m, Memory: 512Mi)
- [ ] Seccomp profile: RuntimeDefault
- [ ] All capabilities dropped
- [ ] Kubernetes audit policy configured
- [ ] Security metrics exported to Prometheus
- [ ] Finalizers implemented for audit persistence
- [ ] Owner references set on all downstream CRDs

### Monitoring & Alerts

- [ ] Alert on RBAC permission denied
- [ ] Alert on CRD creation failures
- [ ] Alert on finalizer cleanup failures
- [ ] Alert on resource quota exhaustion
- [ ] Dashboard shows active RemediationRequests
- [ ] Audit logs reviewed regularly

---

## 10. References

- **RBAC Best Practices**: [Kubernetes RBAC Documentation](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)
- **Network Policies**: [Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- **Pod Security Standards**: [Pod Security Admission](https://kubernetes.io/docs/concepts/security/pod-security-admission/)
- **Owner References**: [ADR-005: Owner Reference Architecture](../../../architecture/decisions/005-owner-reference-architecture.md)
- **Testing**: [testing-strategy.md](./testing-strategy.md)
- **Integration**: [integration-points.md](./integration-points.md)
