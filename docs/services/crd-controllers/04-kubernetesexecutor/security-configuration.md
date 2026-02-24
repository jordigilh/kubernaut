## Security Configuration

> **DEPRECATED**: KubernetesExecution CRD and KubernetesExecutor service were eliminated by ADR-025 and replaced by Tekton TaskRun via WorkflowExecution. This documentation is retained for historical reference only. API types and CRD manifests have been removed from the codebase.

### ServiceAccount & RBAC Least Privilege

**Controller ServiceAccount Setup**:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubernetesexecution-controller
  namespace: kubernaut-system
automountServiceAccountToken: true
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernetesexecution-controller
rules:
# KubernetesExecution CRD permissions (full control)
- apiGroups: ["kubernetesexecution.kubernaut.io"]
  resources: ["kubernetesexecutions"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["kubernetesexecution.kubernaut.io"]
  resources: ["kubernetesexecutions/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["kubernetesexecution.kubernaut.io"]
  resources: ["kubernetesexecutions/finalizers"]
  verbs: ["update"]

# WorkflowExecution CRD permissions (read-only for parent reference)
- apiGroups: ["workflowexecution.kubernaut.io"]
  resources: ["workflowexecutions"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["workflowexecution.kubernaut.io"]
  resources: ["workflowexecutions/status"]
  verbs: ["get"]

# Kubernetes Job permissions (create + watch for execution)
- apiGroups: ["batch"]
  resources: ["jobs"]
  verbs: ["create", "get", "list", "watch", "delete"]
- apiGroups: ["batch"]
  resources: ["jobs/status"]
  verbs: ["get", "list", "watch"]

# ServiceAccount management (create per-action SAs)
- apiGroups: [""]
  resources: ["serviceaccounts"]
  verbs: ["create", "get", "list", "watch", "delete"]

# RBAC management (create per-action Roles/RoleBindings)
- apiGroups: ["rbac.authorization.k8s.io"]
  resources: ["roles", "rolebindings"]
  verbs: ["create", "get", "list", "watch", "delete"]

# Event emission (write-only)
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernetesexecution-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kubernetesexecution-controller
subjects:
- kind: ServiceAccount
  name: kubernetesexecution-controller
  namespace: kubernaut-system
```

**Least Privilege Principles**:
- ✅ Write access ONLY to KubernetesExecution CRDs
- ✅ Create Kubernetes Jobs (execution mechanism)
- ✅ Create ServiceAccounts + RBAC (per-action isolation)
- ✅ NO direct resource modification (Jobs handle that)
- ✅ Event creation scoped to KubernetesExecution events only

**🚨 CRITICAL: Per-Action RBAC Isolation**:
Each predefined action requires a dedicated ServiceAccount with **least-privilege permissions** for that action only.

---

### Per-Action ServiceAccounts (10 Predefined Actions)

**Action 1: Restart Pod** (`restart-pod`):
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: restart-pod-sa
  namespace: kubernaut-system
automountServiceAccountToken: true
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: restart-pod-role
  namespace: <TARGET_NAMESPACE>  # Created dynamically per action
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "delete"]  # ONLY delete permission for restart
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: restart-pod-binding
  namespace: <TARGET_NAMESPACE>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: restart-pod-role
subjects:
- kind: ServiceAccount
  name: restart-pod-sa
  namespace: kubernaut-system
```

**Action 2: Scale Deployment** (`scale-deployment`):
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: scale-deployment-sa
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: scale-deployment-role
  namespace: <TARGET_NAMESPACE>
rules:
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get", "patch"]  # ONLY patch for scale
- apiGroups: ["apps"]
  resources: ["deployments/scale"]
  verbs: ["get", "update"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: scale-deployment-binding
  namespace: <TARGET_NAMESPACE>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: scale-deployment-role
subjects:
- kind: ServiceAccount
  name: scale-deployment-sa
  namespace: kubernaut-system
```

**Action 3: Patch ConfigMap** (`patch-configmap`):
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: patch-configmap-sa
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: patch-configmap-role
  namespace: <TARGET_NAMESPACE>
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "patch"]  # ONLY patch for configuration updates
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: patch-configmap-binding
  namespace: <TARGET_NAMESPACE>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: patch-configmap-role
subjects:
- kind: ServiceAccount
  name: patch-configmap-sa
  namespace: kubernaut-system
```

**Action 4: Cordon Node** (`cordon-node`):
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: cordon-node-sa
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: cordon-node-role
rules:
- apiGroups: [""]
  resources: ["nodes"]
  verbs: ["get", "patch"]  # ONLY patch for cordon/uncordon
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: cordon-node-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cordon-node-role
subjects:
- kind: ServiceAccount
  name: cordon-node-sa
  namespace: kubernaut-system
```

**Action 5: Drain Node** (`drain-node`):
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: drain-node-sa
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: drain-node-role
rules:
- apiGroups: [""]
  resources: ["nodes"]
  verbs: ["get", "patch"]  # Patch for cordon
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "delete"]  # List + delete for eviction
- apiGroups: [""]
  resources: ["pods/eviction"]
  verbs: ["create"]  # Eviction API
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: drain-node-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: drain-node-role
subjects:
- kind: ServiceAccount
  name: drain-node-sa
  namespace: kubernaut-system
```

**Action 6: Delete Pod** (`delete-pod`):
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: delete-pod-sa
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: delete-pod-role
  namespace: <TARGET_NAMESPACE>
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "delete"]  # ONLY delete permission
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: delete-pod-binding
  namespace: <TARGET_NAMESPACE>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: delete-pod-role
subjects:
- kind: ServiceAccount
  name: delete-pod-sa
  namespace: kubernaut-system
```

**Action 7: Patch Deployment** (`patch-deployment`):
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: patch-deployment-sa
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: patch-deployment-role
  namespace: <TARGET_NAMESPACE>
rules:
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get", "patch"]  # ONLY patch for updates
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: patch-deployment-binding
  namespace: <TARGET_NAMESPACE>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: patch-deployment-role
subjects:
- kind: ServiceAccount
  name: patch-deployment-sa
  namespace: kubernaut-system
```

**Action 8: Rollback Deployment** (`rollback-deployment`):
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rollback-deployment-sa
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: rollback-deployment-role
  namespace: <TARGET_NAMESPACE>
rules:
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get", "patch"]
- apiGroups: ["apps"]
  resources: ["replicasets"]
  verbs: ["get", "list"]  # List previous ReplicaSets for rollback
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rollback-deployment-binding
  namespace: <TARGET_NAMESPACE>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rollback-deployment-role
subjects:
- kind: ServiceAccount
  name: rollback-deployment-sa
  namespace: kubernaut-system
```

**Action 9: Verify Deployment** (`verify-deployment`):
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: verify-deployment-sa
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: verify-deployment-role
  namespace: <TARGET_NAMESPACE>
rules:
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get"]  # ONLY get for verification (read-only)
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list"]  # List pods for health check
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: verify-deployment-binding
  namespace: <TARGET_NAMESPACE>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: verify-deployment-role
subjects:
- kind: ServiceAccount
  name: verify-deployment-sa
  namespace: kubernaut-system
```

**Action 10: Custom Script** (`custom-script`):
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: custom-script-sa
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: custom-script-role
  namespace: <TARGET_NAMESPACE>
rules:
# VERY LIMITED permissions for custom scripts (security critical)
- apiGroups: [""]
  resources: ["pods", "configmaps"]
  verbs: ["get", "list"]  # Read-only by default
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: custom-script-binding
  namespace: <TARGET_NAMESPACE>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: custom-script-role
subjects:
- kind: ServiceAccount
  name: custom-script-sa
  namespace: kubernaut-system
```

**Per-Action RBAC Mapping**:
| Action | ServiceAccount | Permissions | Scope |
|--------|----------------|-------------|-------|
| `restart-pod` | `restart-pod-sa` | pods: delete | Namespace |
| `scale-deployment` | `scale-deployment-sa` | deployments/scale: update | Namespace |
| `patch-configmap` | `patch-configmap-sa` | configmaps: patch | Namespace |
| `cordon-node` | `cordon-node-sa` | nodes: patch | Cluster |
| `drain-node` | `drain-node-sa` | nodes: patch, pods: delete | Cluster |
| `delete-pod` | `delete-pod-sa` | pods: delete | Namespace |
| `patch-deployment` | `patch-deployment-sa` | deployments: patch | Namespace |
| `rollback-deployment` | `rollback-deployment-sa` | deployments: patch, replicasets: list | Namespace |
| `verify-deployment` | `verify-deployment-sa` | deployments: get, pods: list | Namespace |
| `custom-script` | `custom-script-sa` | read-only (pods, configmaps, deployments) | Namespace |

---

### Network Policies

**Restrict Controller Network Access**:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kubernetesexecution-controller
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: kubernetesexecution-controller
  policyTypes:
  - Ingress
  - Egress
  ingress:
  # Health/readiness probes from kubelet
  - from:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    ports:
    - protocol: TCP
      port: 8080  # Health/Ready
  # Metrics scraping from Prometheus
  - from:
    - namespaceSelector:
        matchLabels:
          name: monitoring
      podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 9090  # Metrics
  egress:
  # Kubernetes API server (for Job/RBAC operations)
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    ports:
    - protocol: TCP
      port: 443
  # Data Storage Service (audit trail)
  - to:
    - podSelector:
        matchLabels:
          app: data-storage-service
    ports:
    - protocol: TCP
      port: 8080
  # DNS resolution
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
      podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
```

**Restrict Job Pod Network Access** (Applied to all execution Jobs):

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kubernetesexecution-job-pods
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: kubernetesexecution-job
  policyTypes:
  - Egress
  egress:
  # Kubernetes API server ONLY (no other network access)
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    ports:
    - protocol: TCP
      port: 443
  # DNS resolution (for API server discovery)
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
      podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
```

**Why These Restrictions**:
- Controller: No external network access (all dependencies internal or via API server)
- Job Pods: **ONLY Kubernetes API access** (no application services, no internet)
- No direct database access (goes through Data Storage Service)
- No access to other Kubernaut services (API-only execution)

---

### Secret Management

**kubectl Command Sanitization in Job Pods**:

**Pattern 1: Job Script with Secret Sanitization**:
```go
package controller

import (
    "context"
    "fmt"
    "regexp"
    "strings"

    kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetesexecution/v1"

    batchv1 "k8s.io/api/batch/v1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
    // Common secret patterns to sanitize
    secretPatterns = []*regexp.Regexp{
        regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*\S+`),
        regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[:=]\s*\S+`),
        regexp.MustCompile(`(?i)(token|auth)\s*[:=]\s*\S+`),
        regexp.MustCompile(`(?i)(secret)\s*[:=]\s*\S+`),
        // AWS credentials
        regexp.MustCompile(`(?i)(aws[_-]?access[_-]?key[_-]?id|aws[_-]?secret[_-]?access[_-]?key)\s*[:=]\s*\S+`),
        // Database connection strings
        regexp.MustCompile(`(?i)(connection[_-]?string|database[_-]?url)\s*[:=]\s*\S+`),
        // JWT tokens
        regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`),
        // Generic base64 secrets (>32 chars)
        regexp.MustCompile(`(?i)(secret|token|key)\s*[:=]\s*[A-Za-z0-9+/]{32,}={0,2}`),
    }
)

func sanitizeCommand(command string) string {
    sanitized := command
    for _, pattern := range secretPatterns {
        sanitized = pattern.ReplaceAllString(sanitized, "$1=***REDACTED***")
    }
    return sanitized
}

func (r *KubernetesExecutionReconciler) createExecutionJob(
    ctx context.Context,
    ke *kubernetesexecutionv1.KubernetesExecution,
) error {
    // Build kubectl command for action
    command := r.buildKubectlCommand(ke.Spec.Action, ke.Spec.Parameters)

    // Sanitize command ONLY for logging (not in Job spec)
    sanitizedCommand := sanitizeCommand(command)
    r.Log.Info("Creating Kubernetes Job",
        "action", ke.Spec.Action.Name,
        "command", sanitizedCommand,  // Sanitized for logs
    )

    // Create Job with UNSANITIZED command (actual execution needs real values)
    job := &batchv1.Job{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-job", ke.Name),
            Namespace: "kubernaut-system",
            Labels: map[string]string{
                "app": "kubernetesexecution-job",
                "kubernetesexecution": ke.Name,
            },
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(ke, kubernetesexecutionv1.GroupVersion.WithKind("KubernetesExecution")),
            },
        },
        Spec: batchv1.JobSpec{
            Template: corev1.PodTemplateSpec{
                Spec: corev1.PodSpec{
                    ServiceAccountName: r.getServiceAccountForAction(ke.Spec.Action.Name),
                    RestartPolicy:      corev1.RestartPolicyNever,
                    SecurityContext:    r.getJobPodSecurityContext(),
                    Containers: []corev1.Container{
                        {
                            Name:            "executor",
                            Image:           "bitnami/kubectl:latest",  // Official kubectl image
                            Command:         []string{"/bin/sh", "-c"},
                            Args:            []string{command},  // UNSANITIZED for actual execution
                            SecurityContext: r.getJobContainerSecurityContext(),
                        },
                    },
                },
            },
            BackoffLimit: ptr.To(int32(0)),  // No retries (controller handles retry logic)
            TTLSecondsAfterFinished: ptr.To(int32(300)),  // Clean up after 5 min
        },
    }

    return r.Create(ctx, job)
}

func (r *KubernetesExecutionReconciler) getServiceAccountForAction(actionName string) string {
    // Map action name to dedicated ServiceAccount
    saMap := map[string]string{
        "restart-pod":         "restart-pod-sa",
        "scale-deployment":    "scale-deployment-sa",
        "patch-configmap":     "patch-configmap-sa",
        "cordon-node":         "cordon-node-sa",
        "drain-node":          "drain-node-sa",
        "delete-pod":          "delete-pod-sa",
        "patch-deployment":    "patch-deployment-sa",
        "rollback-deployment": "rollback-deployment-sa",
        "verify-deployment":   "verify-deployment-sa",
        "custom-script":       "custom-script-sa",
    }
    return saMap[actionName]
}
```

**Pattern 2: Job Output Sanitization**:
```go
package controller

import (
    "context"
    "fmt"
    "io"

    kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetesexecution/v1"

    corev1 "k8s.io/api/core/v1"
    "k8s.io/client-go/kubernetes"
)

func (r *KubernetesExecutionReconciler) captureJobLogs(
    ctx context.Context,
    ke *kubernetesexecutionv1.KubernetesExecution,
    jobPodName string,
) (string, error) {
    // Get Job pod logs
    req := r.ClientSet.CoreV1().Pods("kubernaut-system").GetLogs(jobPodName, &corev1.PodLogOptions{})
    logs, err := req.Stream(ctx)
    if err != nil {
        return "", err
    }
    defer logs.Close()

    // Read logs
    logBytes, err := io.ReadAll(logs)
    if err != nil {
        return "", err
    }

    // Sanitize logs before storing in CRD status
    sanitizedLogs := sanitizeCommand(string(logBytes))

    // Update KubernetesExecution status with sanitized logs
    ke.Status.ExecutionLogs = sanitizedLogs  // Sanitized version only
    return sanitizedLogs, r.Status().Update(ctx, ke)
}
```

**Pattern 3: Audit Log Sanitization**:
```go
package controller

import (
    "context"

    kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetesexecution/v1"
)

func (r *KubernetesExecutionReconciler) recordAudit(
    ctx context.Context,
    ke *kubernetesexecutionv1.KubernetesExecution,
) error {
    // Sanitize action parameters before audit logging
    sanitizedParams := sanitizeCommand(ke.Spec.Parameters.String())

    auditRecord := &AuditRecord{
        ExecutionID:    ke.Name,
        ActionName:     ke.Spec.Action.Name,
        Parameters:     sanitizedParams,  // Sanitized version
        ExecutionLogs:  sanitizeCommand(ke.Status.ExecutionLogs),  // Sanitized
        // ... other fields
    }

    return r.StorageClient.RecordAudit(ctx, auditRecord)
}
```

**Secret Handling Rules** (MANDATORY):
- ❌ NEVER log kubectl commands verbatim (may contain secrets)
- ❌ NEVER store unsanitized Job outputs in CRD status
- ❌ NEVER include secrets in audit records
- ❌ NEVER include secrets in Kubernetes Events
- ✅ Sanitize ALL kubectl commands before logging
- ✅ Sanitize ALL Job outputs before storing in CRD
- ✅ Use regex patterns for common secret formats
- ✅ Apply sanitization at Job creation and log capture boundaries

**Sanitization Coverage** (100% Required):
- ✅ CRD Status Updates → Sanitized logs only
- ✅ Audit Logs → `sanitizeCommand()` applied
- ✅ Structured Logs → Sanitized kubectl commands
- ✅ Kubernetes Events → Sanitized action parameters
- ✅ Job Creation → Unsanitized (for execution), sanitized logging

---

### Security Context

**Controller Pod Security Standards** (Restricted Profile):

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubernetesexecution-controller
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kubernetesexecution-controller
  template:
    metadata:
      labels:
        app: kubernetesexecution-controller
    spec:
      serviceAccountName: kubernetesexecution-controller
      securityContext:
        # Pod-level security context
        runAsNonRoot: true
        runAsUser: 65532  # nonroot user
        runAsGroup: 65532
        fsGroup: 65532
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: manager
        image: kubernetesexecution-controller:latest
        securityContext:
          # Container-level security context
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 65532
          capabilities:
            drop:
            - ALL
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
        ports:
        - containerPort: 8080
          name: health
          protocol: TCP
        - containerPort: 9090
          name: metrics
          protocol: TCP
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 15
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        volumeMounts:
        - name: tmp
          mountPath: /tmp
        - name: cache
          mountPath: /.cache
      volumes:
      - name: tmp
        emptyDir: {}
      - name: cache
        emptyDir: {}
```

**Job Pod Security Standards** (Restricted Profile with kubectl):

```yaml
# Applied to all Kubernetes Jobs created by controller
securityContext:
  # Pod-level security context
  runAsNonRoot: true
  runAsUser: 65532  # nonroot user
  runAsGroup: 65532
  fsGroup: 65532
  seccompProfile:
    type: RuntimeDefault

containers:
- name: executor
  image: bitnami/kubectl:latest
  securityContext:
    # Container-level security context
    allowPrivilegeEscalation: false
    readOnlyRootFilesystem: true
    runAsNonRoot: true
    runAsUser: 65532
    capabilities:
      drop:
      - ALL
  resources:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      cpu: 200m
      memory: 256Mi
```

**Why These Settings**:
- **runAsNonRoot**: Prevents privilege escalation (both controller and Jobs)
- **readOnlyRootFilesystem**: Immutable container filesystem
- **drop ALL capabilities**: Minimal Linux capabilities (kubectl doesn't need special caps)
- **seccompProfile**: Syscall filtering for defense-in-depth
- **No volumeMounts in Jobs**: kubectl operates entirely via Kubernetes API (no file access needed)

**🚨 CRITICAL: Job Pod Security**:
- ✅ Job pods run as nonroot user (65532)
- ✅ Job pods have **no privileged capabilities** (drop ALL)
- ✅ Job pods have **read-only root filesystem** (no file writes)
- ✅ Job pods have **no host access** (network, PID, IPC namespaces isolated)
- ✅ Job pods have **restricted network policy** (API server access only)

---

### Per-Action RBAC Enforcement Pattern

**Runtime RBAC Validation**:
```go
package controller

import (
    "context"
    "fmt"

    kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetesexecution/v1"

    corev1 "k8s.io/api/core/v1"
    rbacv1 "k8s.io/api/rbac/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *KubernetesExecutionReconciler) ensureRBACForAction(
    ctx context.Context,
    ke *kubernetesexecutionv1.KubernetesExecution,
) error {
    actionName := ke.Spec.Action.Name
    targetNamespace := ke.Spec.TargetNamespace

    // 1. Ensure ServiceAccount exists
    sa := &corev1.ServiceAccount{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-sa", actionName),
            Namespace: "kubernaut-system",
        },
    }
    if err := r.Get(ctx, client.ObjectKeyFromObject(sa), sa); client.IgnoreNotFound(err) != nil {
        return err
    }
    if err != nil {
        // Create ServiceAccount
        if err := r.Create(ctx, sa); err != nil {
            return fmt.Errorf("failed to create ServiceAccount: %w", err)
        }
    }

    // 2. Ensure Role exists with least-privilege permissions
    role := r.getRoleForAction(actionName, targetNamespace)
    if err := r.Get(ctx, client.ObjectKeyFromObject(role), role); client.IgnoreNotFound(err) != nil {
        return err
    }
    if err != nil {
        // Create Role
        if err := r.Create(ctx, role); err != nil {
            return fmt.Errorf("failed to create Role: %w", err)
        }
    }

    // 3. Ensure RoleBinding exists
    binding := &rbacv1.RoleBinding{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-binding", actionName),
            Namespace: targetNamespace,
        },
        RoleRef: rbacv1.RoleRef{
            APIGroup: "rbac.authorization.k8s.io",
            Kind:     "Role",
            Name:     role.Name,
        },
        Subjects: []rbacv1.Subject{
            {
                Kind:      "ServiceAccount",
                Name:      sa.Name,
                Namespace: sa.Namespace,
            },
        },
    }
    if err := r.Get(ctx, client.ObjectKeyFromObject(binding), binding); client.IgnoreNotFound(err) != nil {
        return err
    }
    if err != nil {
        // Create RoleBinding
        if err := r.Create(ctx, binding); err != nil {
            return fmt.Errorf("failed to create RoleBinding: %w", err)
        }
    }

    return nil
}

func (r *KubernetesExecutionReconciler) getRoleForAction(actionName, targetNamespace string) *rbacv1.Role {
    // Define least-privilege permissions per action
    roleMap := map[string]rbacv1.Role{
        "restart-pod": {
            ObjectMeta: metav1.ObjectMeta{
                Name:      "restart-pod-role",
                Namespace: targetNamespace,
            },
            Rules: []rbacv1.PolicyRule{
                {
                    APIGroups: []string{""},
                    Resources: []string{"pods"},
                    Verbs:     []string{"get", "delete"},
                },
            },
        },
        "scale-deployment": {
            ObjectMeta: metav1.ObjectMeta{
                Name:      "scale-deployment-role",
                Namespace: targetNamespace,
            },
            Rules: []rbacv1.PolicyRule{
                {
                    APIGroups: []string{"apps"},
                    Resources: []string{"deployments"},
                    Verbs:     []string{"get", "patch"},
                },
                {
                    APIGroups: []string{"apps"},
                    Resources: []string{"deployments/scale"},
                    Verbs:     []string{"get", "update"},
                },
            },
        },
        // ... other actions
    }

    role := roleMap[actionName]
    return &role
}
```

**🚨 CRITICAL: RBAC Enforcement**:
- ✅ Every action has a **dedicated ServiceAccount**
- ✅ Every ServiceAccount has **least-privilege RBAC** (only required permissions)
- ✅ Roles are created **dynamically per execution** (target namespace specific)
- ✅ RoleBindings are created **per execution** (ephemeral isolation)
- ✅ Job pods **cannot escalate privileges** (restricted security context)

---
