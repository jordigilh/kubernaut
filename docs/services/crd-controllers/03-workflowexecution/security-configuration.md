## Security Configuration

**Version**: 4.0
**Last Updated**: 2025-12-03
**CRD API Group**: `kubernaut.ai/v1alpha1`
**Status**: ✅ Updated for Dedicated Execution Namespace (DD-WE-002)

---

## Changelog

### Version 4.1 (2026-02-18)
- ✅ **Issue #91**: Removed `kubernaut.ai/component` label from Namespace example; `kubernaut.ai/workflow-execution` KEPT on PipelineRun (external resource)

### Version 4.0 (2025-12-03)
- ✅ **Added**: Dedicated execution namespace RBAC (DD-WE-002)
- ✅ **Added**: kubernaut-workflow-runner ClusterRole for cross-namespace operations
- ✅ **Updated**: All PipelineRuns run in `kubernaut-workflows` namespace

### Version 3.1 (2025-12-02)
- ✅ **Removed**: All KubernetesExecution RBAC and code references
- ✅ **Updated**: RBAC to use Tekton PipelineRun permissions
- ✅ **Updated**: Code examples for Tekton-based architecture

---

## Two ServiceAccounts Required

| ServiceAccount | Namespace | Purpose | RBAC |
|----------------|-----------|---------|------|
| `workflowexecution-controller` | `kubernaut-system` | Controller operations | ClusterRole |
| `kubernaut-workflow-runner` | `kubernaut-workflows` | PipelineRun execution | ClusterRole |

---

## 1. PipelineRun Execution RBAC (DD-WE-002)

**ServiceAccount for workflow execution with cross-namespace permissions**:

```yaml
# Dedicated namespace for all PipelineRuns
apiVersion: v1
kind: Namespace
metadata:
  name: kubernaut-workflows
---
# ServiceAccount for PipelineRun execution
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubernaut-workflow-runner
  namespace: kubernaut-workflows
---
# ClusterRole with cross-namespace remediation permissions
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernaut-workflow-runner
rules:
  # Workload remediation (all namespaces)
  - apiGroups: ["apps"]
    resources: ["deployments", "statefulsets", "daemonsets", "replicasets"]
    verbs: ["get", "list", "patch", "update"]
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "delete"]
  # Node operations (cluster-scoped)
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "patch"]
  # ConfigMaps/Secrets for workflow data (read-only)
  - apiGroups: [""]
    resources: ["configmaps", "secrets"]
    verbs: ["get", "list"]
  # Events for workflow logging
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["create", "patch"]
---
# Bind ClusterRole to ServiceAccount
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernaut-workflow-runner
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kubernaut-workflow-runner
subjects:
- kind: ServiceAccount
  name: kubernaut-workflow-runner
  namespace: kubernaut-workflows
```

**Why ClusterRole (not namespace-scoped Role)**:
- PipelineRuns remediate resources in ANY namespace
- Cluster-scoped resources (Nodes) require cluster-level access
- Industry standard pattern (Crossplane, AWX, Argo)

---

## 2. Controller RBAC

### ServiceAccount & RBAC Least Privilege

**ServiceAccount Setup**:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: workflowexecution-controller
  namespace: kubernaut-system
automountServiceAccountToken: true
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: workflowexecution-controller
rules:
# WorkflowExecution CRD permissions (full control)
- apiGroups: ["kubernaut.ai"]
  resources: ["workflowexecutions"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["kubernaut.ai"]
  resources: ["workflowexecutions/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["kubernaut.ai"]
  resources: ["workflowexecutions/finalizers"]
  verbs: ["update"]

# Tekton PipelineRun permissions (create + watch for execution)
- apiGroups: ["tekton.dev"]
  resources: ["pipelineruns"]
  verbs: ["create", "get", "list", "watch", "delete"]
- apiGroups: ["tekton.dev"]
  resources: ["pipelineruns/status"]
  verbs: ["get", "list", "watch"]

# RemediationRequest CRD permissions (read-only for parent reference)
- apiGroups: ["remediation.kubernaut.ai"]
  resources: ["remediationrequests"]
  verbs: ["get", "list", "watch"]
# NOTE: NO status write permissions - Remediation Orchestrator Pattern (see below)

# Event emission (write-only)
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: workflowexecution-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: workflowexecution-controller
subjects:
- kind: ServiceAccount
  name: workflowexecution-controller
  namespace: kubernaut-system
```

**Least Privilege Principles**:
- ✅ Write access ONLY to WorkflowExecution CRDs
- ✅ Create access to Tekton PipelineRun (workflow execution)
- ✅ Watch access to PipelineRun status (completion monitoring)
- ✅ NO direct Kubernetes resource access (Tekton handles execution)
- ✅ Event creation scoped to WorkflowExecution events only

**Remediation Orchestrator Pattern - RBAC Justification**:

This controller follows the **Remediation Orchestrator Pattern** where:
- ✅ **This controller** updates ONLY `WorkflowExecution.status`
- ✅ **RemediationOrchestrator** watches `WorkflowExecution` and aggregates status
- ❌ **NO status write permissions** needed on `RemediationRequest` - watch-based coordination handles all status updates

**Why No RemediationRequest.status Write Access**:
1. **Architectural Separation**: Remediation Orchestrator Pattern decouples child controllers from orchestration
2. **Watch-Based Coordination**: RemediationOrchestrator watches this CRD for status changes (<1s latency)
3. **Single Writer**: Only RemediationOrchestrator updates `RemediationRequest.status` (prevents race conditions)
4. **Testability**: This controller can be tested in complete isolation without RemediationRequest dependency

**What This Controller CAN Do with RemediationRequest**:
- ✅ `get` - Read parent CRD for owner reference setup
- ✅ `list` - List parent CRDs for audit/tracing
- ✅ `watch` - Watch for parent lifecycle events (deletion)
- ❌ NO `update` or `patch` on `remediationrequests` or `remediationrequests/status`

**Reference**:
- See: [Remediation Orchestrator Architecture](../05-remediationorchestrator/overview.md)

**🚨 CRITICAL SECRET PROTECTION**:
- ❌ Secrets are NEVER captured verbatim in logs, CRD status, events, or audit trails
- ✅ Secret values are ALWAYS scrambled/sanitized before any storage or logging
- ✅ Only secret **references** (name, namespace, type) are stored
- ✅ Regex-based sanitization applied to ALL outgoing data (logs, events, audit records)

---

### Network Policies

**Restrict Controller Network Access**:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: workflowexecution-controller
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: workflowexecution-controller
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
      port: 8081  # Health/Ready (DD-TEST-001)
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
  # Kubernetes API server (for CRD and PipelineRun operations)
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

**Why These Restrictions**:
- No external network access (all dependencies internal or via API server)
- No direct database access (goes through Data Storage Service)
- No access to application namespaces (delegates execution to Tekton)
- No direct service-to-service communication (CRD-based coordination)

---

### Secret Management

**No Direct Secret Handling in WorkflowExecution**:

WorkflowExecution controller does NOT handle secrets directly. All secrets are:
- Referenced in workflow OCI bundles (defined by workflow authors)
- Passed through workflow parameters (LLM-selected)
- Referenced by name/namespace only (no secret values in CRD)

**Pattern 1: Workflow Parameter Sanitization**:
```go
package controller

import (
    "context"
    "fmt"
    "regexp"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1"
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

func sanitizeWorkflowPayload(payload string) string {
    sanitized := payload
    for _, pattern := range secretPatterns {
        sanitized = pattern.ReplaceAllString(sanitized, "$1=***REDACTED***")
    }
    return sanitized
}

func (r *WorkflowExecutionReconciler) recordAudit(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) error {
    // Sanitize parameters before audit logging
    sanitizedParams := sanitizeWorkflowPayload(fmt.Sprintf("%v", wfe.Spec.Parameters))

    auditRecord := &AuditRecord{
        WorkflowID:     wfe.Spec.WorkflowRef.WorkflowID,
        TargetResource: wfe.Spec.TargetResource,
        Parameters:     sanitizedParams,  // Sanitized version
        // ... other fields
    }

    return r.StorageClient.RecordAudit(ctx, auditRecord)
}
```

**Pattern 2: Kubernetes Event Sanitization**:
```go
package controller

import (
    "fmt"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1"

    "k8s.io/client-go/tools/record"
)

func (r *WorkflowExecutionReconciler) emitEventSanitized(
    wfe *workflowexecutionv1.WorkflowExecution,
    eventType string,
    reason string,
    message string,
) {
    // Sanitize message before emitting event
    sanitizedMessage := sanitizeWorkflowPayload(message)

    r.Recorder.Event(wfe, eventType, reason, sanitizedMessage)
}

// Example: PipelineRun creation event with sanitized details
func (r *WorkflowExecutionReconciler) emitPipelineRunCreationEvent(
    wfe *workflowexecutionv1.WorkflowExecution,
) {
    // Build message with potentially sensitive data
    message := fmt.Sprintf(
        "Creating PipelineRun: workflow=%s, target=%s, params=%v",
        wfe.Spec.WorkflowRef.WorkflowID,
        wfe.Spec.TargetResource,
        wfe.Spec.Parameters,  // May contain secrets
    )

    // Sanitize before emitting
    r.emitEventSanitized(wfe, "Normal", "PipelineRunCreating", message)
}
```

**Pattern 3: Structured Logging Sanitization**:
```go
package controller

import (
    "context"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1"

    "github.com/go-logr/logr"
)

func (r *WorkflowExecutionReconciler) logWithSanitization(
    log logr.Logger,
    message string,
    keysAndValues ...interface{},
) {
    // Sanitize all string values in keysAndValues
    sanitizedKVs := make([]interface{}, len(keysAndValues))
    for i, kv := range keysAndValues {
        if str, ok := kv.(string); ok {
            sanitizedKVs[i] = sanitizeWorkflowPayload(str)
        } else {
            sanitizedKVs[i] = kv
        }
    }

    log.Info(message, sanitizedKVs...)
}

// Example usage
func (r *WorkflowExecutionReconciler) createPipelineRun(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
    log logr.Logger,
) error {
    // Sanitize before logging
    r.logWithSanitization(log, "Creating Tekton PipelineRun",
        "workflowId", wfe.Spec.WorkflowRef.WorkflowID,
        "targetResource", wfe.Spec.TargetResource,
        "parameters", fmt.Sprintf("%v", wfe.Spec.Parameters),  // Will be sanitized
    )

    // ... PipelineRun creation logic
    return nil
}
```

**Pattern 4: PipelineRun Parameter Sanitization**:
```go
package controller

import (
    "context"
    "fmt"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1"
    tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *WorkflowExecutionReconciler) buildPipelineRun(
    wfe *workflowexecutionv1.WorkflowExecution,
) *tektonv1.PipelineRun {
    // Build parameters from spec
    params := make([]tektonv1.Param, 0, len(wfe.Spec.Parameters))
    for key, value := range wfe.Spec.Parameters {
        params = append(params, tektonv1.Param{
            Name:  key,
            Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: value},
        })
    }

    // Create PipelineRun with bundle resolver
    return &tektonv1.PipelineRun{
        ObjectMeta: metav1.ObjectMeta{
            Name:      wfe.Name,
            Namespace: wfe.Namespace,
            Labels: map[string]string{
                // Issue #91: KEPT - label on external K8s resource (PipelineRun) for WE-to-PipelineRun correlation
                "kubernaut.ai/workflow-execution": wfe.Name,
                "kubernaut.ai/workflow-id":        wfe.Spec.WorkflowRef.WorkflowID,
            },
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(wfe, workflowexecutionv1.GroupVersion.WithKind("WorkflowExecution")),
            },
        },
        Spec: tektonv1.PipelineRunSpec{
            PipelineRef: &tektonv1.PipelineRef{
                ResolverRef: tektonv1.ResolverRef{
                    Resolver: "bundles",
                    Params: []tektonv1.Param{
                        {Name: "bundle", Value: tektonv1.ParamValue{StringVal: wfe.Spec.WorkflowRef.ContainerImage}},
                        {Name: "name", Value: tektonv1.ParamValue{StringVal: "workflow"}},
                    },
                },
            },
            Params: params,  // Pass through as-is (Tekton handles securely)
        },
    }
}

func (r *WorkflowExecutionReconciler) createPipelineRunWithLogging(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) error {
    pr := r.buildPipelineRun(wfe)

    // Sanitize parameters ONLY for logging (not in PipelineRun)
    sanitizedParams := sanitizeWorkflowPayload(fmt.Sprintf("%v", wfe.Spec.Parameters))
    r.logWithSanitization(r.Log, "Creating PipelineRun",
        "workflowId", wfe.Spec.WorkflowRef.WorkflowID,
        "parameters", sanitizedParams,  // Sanitized for logs
    )

    return r.Create(ctx, pr)
}
```

**Secret Handling Rules** (MANDATORY):
- ❌ NEVER store secret values in CRD status
- ❌ NEVER log secret values verbatim (logs, events, traces)
- ❌ NEVER include secrets in audit records
- ❌ NEVER include secrets in Kubernetes Events
- ✅ Pass secrets through to PipelineRun params (Tekton handles securely)
- ✅ Sanitize ALL outgoing data (logs, events, audit records, traces)
- ✅ Use regex patterns for common secret formats
- ✅ Apply sanitization at controller boundaries (before any external output)

**Sanitization Coverage** (100% Required):
- ✅ CRD Status Updates → No secrets stored
- ✅ Audit Logs → `sanitizeWorkflowPayload()` applied
- ✅ Structured Logs → `logWithSanitization()` wrapper
- ✅ Kubernetes Events → `emitEventSanitized()` wrapper
- ✅ Distributed Traces → Sanitize span attributes
- ✅ PipelineRun Creation → Pass through (Tekton handles securely)

---

### Security Context

**Pod Security Standards** (Restricted Profile):

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: workflowexecution-controller
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: workflowexecution-controller
  template:
    metadata:
      labels:
        app: workflowexecution-controller
    spec:
      serviceAccountName: workflowexecution-controller
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
        image: workflowexecution-controller:latest
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
        - containerPort: 8081
          name: health
          protocol: TCP
        - containerPort: 9090
          name: metrics
          protocol: TCP
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 15
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
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

**Why These Settings**:
- **runAsNonRoot**: Prevents privilege escalation
- **readOnlyRootFilesystem**: Immutable container filesystem
- **drop ALL capabilities**: Minimal Linux capabilities
- **seccompProfile**: Syscall filtering for defense-in-depth
- **emptyDir volumes**: Writable directories for tmp files only

---
