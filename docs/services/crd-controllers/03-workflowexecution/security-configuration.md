## Security Configuration

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
- apiGroups: ["workflowexecution.kubernaut.io"]
  resources: ["workflowexecutions"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["workflowexecution.kubernaut.io"]
  resources: ["workflowexecutions/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["workflowexecution.kubernaut.io"]
  resources: ["workflowexecutions/finalizers"]
  verbs: ["update"]

# KubernetesExecution CRD permissions (create + watch for orchestration)
- apiGroups: ["kubernetesexecution.kubernaut.io"]
  resources: ["kubernetesexecutions"]
  verbs: ["create", "get", "list", "watch"]
- apiGroups: ["kubernetesexecution.kubernaut.io"]
  resources: ["kubernetesexecutions/status"]
  verbs: ["get", "list", "watch"]

# RemediationRequest CRD permissions (read-only for parent reference)
- apiGroups: ["remediation.kubernaut.io"]
  resources: ["alertremediations"]
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
- ✅ Create access to KubernetesExecution CRDs (orchestration)
- ✅ Watch access to child CRD status (step completion monitoring)
- ✅ NO direct Kubernetes resource access (Executor Service responsibility)
- ✅ Event creation scoped to WorkflowExecution events only

**Remediation Orchestrator Pattern - RBAC Justification**:

This controller follows the **Remediation Orchestrator Pattern** where:
- ✅ **This controller** updates ONLY `WorkflowExecution.status`
- ✅ **RemediationRequest Controller** (Remediation Orchestrator) watches `WorkflowExecution` and aggregates status
- ❌ **NO status write permissions** needed on `RemediationRequest` - watch-based coordination handles all status updates

**Why No RemediationRequest.status Write Access**:
1. **Architectural Separation**: Remediation Orchestrator Pattern decouples child controllers from orchestration
2. **Watch-Based Coordination**: RemediationRequest Controller watches this CRD for status changes (<1s latency)
3. **Single Writer**: Only RemediationRequest Controller updates `RemediationRequest.status` (prevents race conditions)
4. **Testability**: This controller can be tested in complete isolation without RemediationRequest dependency

**What This Controller CAN Do with RemediationRequest**:
- ✅ `get` - Read parent CRD for owner reference setup
- ✅ `list` - List parent CRDs for audit/tracing
- ✅ `watch` - Watch for parent lifecycle events (deletion)
- ❌ NO `update` or `patch` on `alertremediations` or `alertremediations/status`

**Reference**:
- See: [Remediation Orchestrator Architecture](../05-remediationorchestrator/overview.md)
- See: [Remediation Orchestrator Pattern Violation Analysis](../CENTRAL_CONTROLLER_VIOLATION_ANALYSIS.md)

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
  # Kubernetes API server (for CRD operations)
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
- No access to application namespaces (orchestrates via child CRDs only)
- No direct service-to-service communication (CRD-based coordination)

---

### Secret Management

**No Direct Secret Handling in WorkflowExecution**:

WorkflowExecution controller does NOT handle secrets directly. All secrets are:
- Embedded in workflow definitions (created by AIAnalysis)
- Passed through to KubernetesExecution CRDs (executor responsibility)
- Referenced by name/namespace only (no secret values)

**Pattern 1: Workflow Definition Sanitization**:
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
    we *workflowexecutionv1.WorkflowExecution,
) error {
    // Sanitize workflow definition before audit logging
    sanitizedWorkflow := sanitizeWorkflowPayload(we.Spec.WorkflowDefinition.String())

    auditRecord := &AuditRecord{
        WorkflowID:         we.Name,
        WorkflowDefinition: sanitizedWorkflow,  // Sanitized version
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
    we *workflowexecutionv1.WorkflowExecution,
    eventType string,
    reason string,
    message string,
) {
    // Sanitize message before emitting event
    sanitizedMessage := sanitizeWorkflowPayload(message)

    r.Recorder.Event(we, eventType, reason, sanitizedMessage)
}

// Example: Step execution event with sanitized details
func (r *WorkflowExecutionReconciler) emitStepExecutionEvent(
    we *workflowexecutionv1.WorkflowExecution,
    stepName string,
) {
    // Build message with potentially sensitive data
    message := fmt.Sprintf(
        "Executing step: %s, action=%s, parameters=%v",
        stepName,
        we.Spec.WorkflowDefinition.Steps[stepName].Action,
        we.Spec.WorkflowDefinition.Steps[stepName].Parameters,  // May contain secrets
    )

    // Sanitize before emitting
    r.emitEventSanitized(we, "Normal", "StepExecuting", message)
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
func (r *WorkflowExecutionReconciler) orchestrateStep(
    ctx context.Context,
    we *workflowexecutionv1.WorkflowExecution,
    stepName string,
    log logr.Logger,
) error {
    // Sanitize before logging
    r.logWithSanitization(log, "Orchestrating workflow step",
        "workflowID", we.Name,
        "stepName", stepName,
        "parameters", we.Spec.WorkflowDefinition.Steps[stepName].Parameters,  // Will be sanitized
    )

    // ... orchestration logic
}
```

**Pattern 4: Step Parameter Sanitization**:
```go
package controller

import (
    "context"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1"
    kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetesexecution/v1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *WorkflowExecutionReconciler) createKubernetesExecution(
    ctx context.Context,
    we *workflowexecutionv1.WorkflowExecution,
    step *workflowexecutionv1.WorkflowStep,
) error {
    // Create KubernetesExecution CRD for step
    ke := &kubernetesexecutionv1.KubernetesExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-%s", we.Name, step.Name),
            Namespace: we.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(we, workflowexecutionv1.GroupVersion.WithKind("WorkflowExecution")),
            },
        },
        Spec: kubernetesexecutionv1.KubernetesExecutionSpec{
            Action:     step.Action,
            Parameters: step.Parameters,  // Pass through as-is (executor sanitizes)
            TargetCluster: step.TargetCluster,
        },
    }

    // Sanitize step parameters ONLY for logging/events (not in CRD)
    sanitizedParams := sanitizeWorkflowPayload(step.Parameters.String())
    r.logWithSanitization(r.Log, "Creating KubernetesExecution CRD",
        "workflowID", we.Name,
        "stepName", step.Name,
        "parameters", sanitizedParams,  // Sanitized for logs
    )

    return r.Create(ctx, ke)
}
```

**Secret Handling Rules** (MANDATORY):
- ❌ NEVER store secret values in CRD status
- ❌ NEVER log secret values verbatim (logs, events, traces)
- ❌ NEVER include secrets in audit records
- ❌ NEVER include secrets in Kubernetes Events
- ✅ Pass secrets through to child CRDs (executor handles sanitization)
- ✅ Sanitize ALL outgoing data (logs, events, audit records, traces)
- ✅ Use regex patterns for common secret formats
- ✅ Apply sanitization at controller boundaries (before any external output)

**Sanitization Coverage** (100% Required):
- ✅ CRD Status Updates → No secrets stored
- ✅ Audit Logs → `sanitizeWorkflowPayload()` applied
- ✅ Structured Logs → `logWithSanitization()` wrapper
- ✅ Kubernetes Events → `emitEventSanitized()` wrapper
- ✅ Distributed Traces → Sanitize span attributes
- ✅ Child CRD Creation → Pass through (executor sanitizes)

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

**Why These Settings**:
- **runAsNonRoot**: Prevents privilege escalation
- **readOnlyRootFilesystem**: Immutable container filesystem
- **drop ALL capabilities**: Minimal Linux capabilities
- **seccompProfile**: Syscall filtering for defense-in-depth
- **emptyDir volumes**: Writable directories for tmp files only

---
