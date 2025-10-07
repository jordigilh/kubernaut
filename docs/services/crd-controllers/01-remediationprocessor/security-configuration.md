## Security Configuration

### ServiceAccount & RBAC Least Privilege

**ServiceAccount Setup**:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: alertprocessing-controller
  namespace: kubernaut-system
automountServiceAccountToken: true
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: alertprocessing-controller
rules:
# RemediationProcessing CRD permissions (full control)
- apiGroups: ["remediationprocessing.kubernaut.io"]
  resources: ["alertprocessings"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["remediationprocessing.kubernaut.io"]
  resources: ["alertprocessings/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["remediationprocessing.kubernaut.io"]
  resources: ["alertprocessings/finalizers"]
  verbs: ["update"]

# RemediationRequest CRD permissions (read-only for parent reference)
- apiGroups: ["remediation.kubernaut.io"]
  resources: ["alertremediations"]
  verbs: ["get", "list", "watch"]
# NOTE: NO status write permissions - Remediation Orchestrator Pattern (see below)

# Kubernetes core resources (read-only for enrichment)
- apiGroups: [""]
  resources: ["pods", "nodes", "namespaces", "configmaps", "secrets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["batch"]
  resources: ["jobs", "cronjobs"]
  verbs: ["get", "list", "watch"]

# Event emission (write-only)
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: alertprocessing-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: alertprocessing-controller
subjects:
- kind: ServiceAccount
  name: alertprocessing-controller
  namespace: kubernaut-system
```

**Least Privilege Principles**:
- ✅ Read-only access to Kubernetes resources (no modifications)
- ✅ Write access ONLY to RemediationProcessing CRDs
- ✅ No Secret modification permissions (read-only for enrichment metadata)
- ✅ Event creation scoped to RemediationProcessing events only

**Remediation Orchestrator Pattern - RBAC Justification**:

This controller follows the **Remediation Orchestrator Pattern** where:
- ✅ **This controller** updates ONLY `RemediationProcessing.status`
- ✅ **RemediationRequest Controller** (Remediation Orchestrator) watches `RemediationProcessing` and aggregates status
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
  name: alertprocessing-controller
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: alertprocessing-controller
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
  # Kubernetes API server
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    ports:
    - protocol: TCP
      port: 443
  # Context Service (internal)
  - to:
    - podSelector:
        matchLabels:
          app: context-service
    ports:
    - protocol: TCP
      port: 8080
  # Data Storage Service (audit)
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
- No external network access (all dependencies internal)
- No direct database access (goes through Data Storage Service)
- No access to application namespaces (reads via Kubernetes API only)

---

### Secret Management

**No Sensitive Data in RemediationProcessing CRDs**:

RemediationProcessing controller does NOT handle secrets directly. All sensitive data handling follows these patterns:

**Pattern 1: Secret Reference Only** (Recommended):
```go
package controller

import (
    "context"
    "fmt"

    alertprocessorv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"

    corev1 "k8s.io/api/core/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *RemediationProcessingReconciler) enrichWithSecretMetadata(
    ctx context.Context,
    ap *alertprocessorv1.RemediationProcessing,
) error {
    // Get Pod that references Secret
    var pod corev1.Pod
    if err := r.Get(ctx, client.ObjectKey{
        Name:      ap.Status.EnrichedAlert.ResourceName,
        Namespace: ap.Status.EnrichedAlert.Namespace,
    }, &pod); err != nil {
        return err
    }

    // Extract Secret reference (NOT content)
    secretRefs := []alertprocessorv1.SecretReference{}
    for _, volume := range pod.Spec.Volumes {
        if volume.Secret != nil {
            secretRefs = append(secretRefs, alertprocessorv1.SecretReference{
                Name:      volume.Secret.SecretName,
                Namespace: pod.Namespace,
                Type:      "volume",  // volume | env | imagePullSecret
            })
        }
    }

    // Store reference ONLY (never store actual secret data)
    ap.Status.EnrichedAlert.SecretReferences = secretRefs

    return r.Status().Update(ctx, ap)
}
```

**Pattern 2: Audit Log Secret Sanitization**:
```go
package controller

import (
    "fmt"
    "regexp"

    alertprocessorv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"
)

var (
    // Common secret patterns to sanitize
    secretPatterns = []*regexp.Regexp{
        regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*\S+`),
        regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[:=]\s*\S+`),
        regexp.MustCompile(`(?i)(token|auth)\s*[:=]\s*\S+`),
        regexp.MustCompile(`(?i)(secret)\s*[:=]\s*\S+`),
    }
)

func sanitizeAlertPayload(payload string) string {
    sanitized := payload
    for _, pattern := range secretPatterns {
        sanitized = pattern.ReplaceAllString(sanitized, "$1=***REDACTED***")
    }
    return sanitized
}

func (r *RemediationProcessingReconciler) recordAudit(
    ctx context.Context,
    ap *alertprocessorv1.RemediationProcessing,
) error {
    // Sanitize before audit logging
    sanitizedPayload := sanitizeAlertPayload(string(ap.Spec.Alert.Payload))

    auditRecord := &AuditRecord{
        AlertFingerprint: ap.Spec.Alert.Fingerprint,
        Payload:          sanitizedPayload,  // Sanitized version
        // ... other fields
    }

    return r.StorageClient.RecordAudit(ctx, auditRecord)
}
```

**Pattern 3: Kubernetes Event Sanitization**:
```go
package controller

import (
    "fmt"

    alertprocessorv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"

    "k8s.io/client-go/tools/record"
)

func (r *RemediationProcessingReconciler) emitEventSanitized(
    ap *alertprocessorv1.RemediationProcessing,
    eventType string,
    reason string,
    message string,
) {
    // Sanitize message before emitting event
    sanitizedMessage := sanitizeAlertPayload(message)

    r.Recorder.Event(ap, eventType, reason, sanitizedMessage)
}

// Example: Enrichment completed event with sanitized details
func (r *RemediationProcessingReconciler) emitEnrichmentEvent(
    ap *alertprocessorv1.RemediationProcessing,
) {
    // Build message with potentially sensitive data
    message := fmt.Sprintf(
        "Enrichment completed: namespace=%s, pod=%s, annotations=%v",
        ap.Status.EnrichedAlert.Namespace,
        ap.Status.EnrichedAlert.ResourceName,
        ap.Status.EnrichedAlert.Annotations,  // May contain secrets
    )

    // Sanitize before emitting
    r.emitEventSanitized(ap, "Normal", "EnrichmentCompleted", message)
}
```

**Pattern 4: Structured Logging Sanitization**:
```go
package controller

import (
    "context"

    alertprocessorv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"

    "github.com/go-logr/logr"
)

func (r *RemediationProcessingReconciler) logWithSanitization(
    log logr.Logger,
    message string,
    keysAndValues ...interface{},
) {
    // Sanitize all string values in keysAndValues
    sanitizedKVs := make([]interface{}, len(keysAndValues))
    for i, kv := range keysAndValues {
        if str, ok := kv.(string); ok {
            sanitizedKVs[i] = sanitizeAlertPayload(str)
        } else {
            sanitizedKVs[i] = kv
        }
    }

    log.Info(message, sanitizedKVs...)
}

// Example usage
func (r *RemediationProcessingReconciler) enrichAlert(
    ctx context.Context,
    ap *alertprocessorv1.RemediationProcessing,
    log logr.Logger,
) error {
    // Sanitize before logging
    r.logWithSanitization(log, "Starting alert enrichment",
        "fingerprint", ap.Spec.Alert.Fingerprint,
        "payload", string(ap.Spec.Alert.Payload),  // Will be sanitized
    )

    // ... enrichment logic
}
```

**Secret Handling Rules** (MANDATORY):
- ❌ NEVER store secret values in CRD status
- ❌ NEVER log secret values verbatim (logs, events, traces)
- ❌ NEVER include secrets in audit records
- ❌ NEVER include secrets in Kubernetes Events
- ✅ Store secret **references** only (name, namespace, type)
- ✅ Sanitize ALL outgoing data (logs, events, audit records, traces)
- ✅ Use regex patterns for common secret formats
- ✅ Apply sanitization at controller boundaries (before any external output)

**Sanitization Coverage** (100% Required):
- ✅ CRD Status Updates → No secrets stored
- ✅ Audit Logs → `sanitizeAlertPayload()` applied
- ✅ Structured Logs → `logWithSanitization()` wrapper
- ✅ Kubernetes Events → `emitEventSanitized()` wrapper
- ✅ Distributed Traces → Sanitize span attributes
- ✅ HTTP Responses → Sanitize before sending

**Common Secret Patterns** (Expanded):
```go
var secretPatterns = []*regexp.Regexp{
    // Passwords
    regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*\S+`),

    // API Keys
    regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[:=]\s*\S+`),

    // Tokens
    regexp.MustCompile(`(?i)(token|auth[_-]?token|bearer)\s*[:=]\s*\S+`),

    // Generic secrets
    regexp.MustCompile(`(?i)(secret|private[_-]?key)\s*[:=]\s*\S+`),

    // AWS credentials
    regexp.MustCompile(`(?i)(aws[_-]?access[_-]?key[_-]?id|aws[_-]?secret[_-]?access[_-]?key)\s*[:=]\s*\S+`),

    // Database connection strings
    regexp.MustCompile(`(?i)(connection[_-]?string|database[_-]?url)\s*[:=]\s*\S+`),

    // JWT tokens (base64 encoded with dots)
    regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`),

    // Generic base64 secrets (>32 chars)
    regexp.MustCompile(`(?i)(secret|token|key)\s*[:=]\s*[A-Za-z0-9+/]{32,}={0,2}`),
}
```

---

### Security Context

**Pod Security Standards** (Restricted Profile):

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: alertprocessing-controller
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: alertprocessing-controller
  template:
    metadata:
      labels:
        app: alertprocessing-controller
    spec:
      serviceAccountName: alertprocessing-controller
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
        image: alertprocessing-controller:latest
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

