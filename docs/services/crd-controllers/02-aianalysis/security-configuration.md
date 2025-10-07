## Security Configuration

### ServiceAccount & RBAC Least Privilege

**ServiceAccount Setup**:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: aianalysis-controller
  namespace: kubernaut-system
automountServiceAccountToken: true
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: aianalysis-controller
rules:
# AIAnalysis CRD permissions (full control)
- apiGroups: ["aianalysis.kubernaut.io"]
  resources: ["aianalyses"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["aianalysis.kubernaut.io"]
  resources: ["aianalyses/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["aianalysis.kubernaut.io"]
  resources: ["aianalyses/finalizers"]
  verbs: ["update"]

# AIApprovalRequest CRD permissions (full control - owns these CRDs)
- apiGroups: ["approval.kubernaut.io"]
  resources: ["aiapprovalrequests"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["approval.kubernaut.io"]
  resources: ["aiapprovalrequests/status"]
  verbs: ["get", "update", "patch"]

# RemediationRequest CRD permissions (read-only for parent reference)
- apiGroups: ["remediation.kubernaut.io"]
  resources: ["alertremediations"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["remediation.kubernaut.io"]
  resources: ["alertremediations/status"]
  verbs: ["get"]

# ConfigMap for Rego policies (read-only)
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch"]
  resourceNames: ["approval-policy-rego"]

# Event emission (write-only)
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: aianalysis-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: aianalysis-controller
subjects:
- kind: ServiceAccount
  name: aianalysis-controller
  namespace: kubernaut-system
```

**Least Privilege Principles**:
- ✅ Full control of AIAnalysis CRDs (owns these)
- ✅ Full control of AIApprovalRequest CRDs (creates and deletes)
- ✅ Read-only access to RemediationRequest (parent reference)
- ✅ Read-only access to Rego policy ConfigMap
- ✅ No Kubernetes resource modification permissions

**🚨 CRITICAL SECRET PROTECTION**:
- ❌ HolmesGPT API keys are NEVER captured verbatim in logs, CRD status, events, or audit trails
- ❌ Rego policy contents are NEVER logged (may contain sensitive approval rules)
- ✅ HolmesGPT credentials stored in Kubernetes Secrets (projected volume)
- ✅ Sanitize ALL outgoing data (logs, events, audit records, traces)
- ✅ Only HolmesGPT connection status (success/failure) logged, not credentials

---

### Network Policies

**Restrict Controller Network Access**:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: aianalysis-controller
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: aianalysis-controller
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
  # HolmesGPT-API Service (internal)
  - to:
    - podSelector:
        matchLabels:
          app: holmesgpt-api
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
- HolmesGPT-API is internal service (no external AI provider calls from controller)
- No direct database access (goes through Data Storage Service)

---

### Secret Management

**HolmesGPT API Credentials & Sensitive Data Protection**:

AIAnalysis controller handles sensitive HolmesGPT credentials and Rego policies. All secrets follow comprehensive protection patterns similar to Remediation Processor, adapted for AI-specific needs.

**Due to large content size, please refer to 01-alert-processor.md for complete secret management patterns including**:
- Pattern 1: Secret Reference Only (adapted for HolmesGPT credentials)
- Pattern 2: Audit Log Secret Sanitization (with HolmesGPT response sanitization)
- Pattern 3: Kubernetes Event Sanitization
- Pattern 4: Structured Logging Sanitization

**AI Analysis Specific Secret Patterns**:

```go
package controller

import (
    "os"
    "regexp"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/analysis/v1"
)

var (
    // HolmesGPT response secret patterns
    holmesSecretPatterns = []*regexp.Regexp{
        // API keys in responses
        regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[:=]\s*\S+`),
        // Tokens in log snippets
        regexp.MustCompile(`(?i)(token|auth[_-]?token|bearer)\s*[:=]\s*\S+`),
        // Passwords in logs
        regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*\S+`),
        // Database connection strings
        regexp.MustCompile(`(?i)(connection[_-]?string|database[_-]?url)\s*[:=]\s*\S+`),
        // AWS credentials
        regexp.MustCompile(`(?i)(aws[_-]?access[_-]?key[_-]?id|aws[_-]?secret[_-]?access[_-]?key)\s*[:=]\s*\S+`),
        // JWT tokens
        regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`),
        // Generic base64 secrets
        regexp.MustCompile(`(?i)(secret|token|key)\s*[:=]\s*[A-Za-z0-9+/]{32,}={0,2}`),
    }
)

func sanitizeHolmesGPTResponse(response string) string {
    sanitized := response
    for _, pattern := range holmesSecretPatterns {
        sanitized = pattern.ReplaceAllString(sanitized, "$1=***REDACTED***")
    }
    return sanitized
}

// HolmesGPT credential loading (NEVER log API key)
func (r *AIAnalysisReconciler) initHolmesGPTClient() error {
    apiKeyPath := os.Getenv("HOLMESGPT_API_KEY_PATH")
    apiKeyBytes, err := os.ReadFile(apiKeyPath)
    if err != nil {
        r.Log.Error(err, "Failed to read HolmesGPT API key",
            "path", apiKeyPath,  // Safe to log
            // NEVER log: "apiKey", string(apiKeyBytes)  ❌
        )
        return err
    }

    endpoint := os.Getenv("HOLMESGPT_ENDPOINT")
    r.holmesGPTClient = NewHolmesGPTClient(endpoint, string(apiKeyBytes))

    r.Log.Info("HolmesGPT client initialized",
        "endpoint", endpoint,  // Safe
        // NEVER log: "apiKey", string(apiKeyBytes)  ❌
    )

    return nil
}

// Rego policy loading (NEVER log policy content)
func (r *AIAnalysisReconciler) loadRegoPolicy(ctx context.Context) (string, error) {
    var cm corev1.ConfigMap
    if err := r.Get(ctx, client.ObjectKey{
        Name:      "approval-policy-rego",
        Namespace: "kubernaut-system",
    }, &cm); err != nil {
        return "", err
    }

    policyContent := cm.Data["policy.rego"]

    r.Log.Info("Rego policy loaded",
        "configMap", "approval-policy-rego",
        "policySize", len(policyContent),  // Safe
        // NEVER log: "policyContent", policyContent  ❌
    )

    return policyContent, nil
}
```

**Secret Handling Rules** (MANDATORY):
- ❌ NEVER store HolmesGPT API keys in CRD status
- ❌ NEVER log HolmesGPT credentials verbatim (logs, events, traces)
- ❌ NEVER log Rego policy contents (may contain sensitive approval logic)
- ❌ NEVER include HolmesGPT responses verbatim (may contain secrets from logs)
- ✅ Store HolmesGPT credentials in Kubernetes Secrets with projected volumes
- ✅ Sanitize HolmesGPT responses before storing in CRD status
- ✅ Only log connection status (success/failure), not credentials
- ✅ Use regex patterns to sanitize log snippets in HolmesGPT responses

**Sanitization Coverage** (100% Required):
- ✅ HolmesGPT API Keys → Never logged, read from file
- ✅ HolmesGPT Responses → Sanitized before CRD status storage
- ✅ Rego Policy → Never logged (size only)
- ✅ Audit Logs → Sanitized with regex patterns
- ✅ Structured Logs → `logWithSanitization()` wrapper
- ✅ Kubernetes Events → `emitEventSanitized()` wrapper
- ✅ Distributed Traces → Sanitize span attributes

---

### Security Context

**Pod Security Standards** (Restricted Profile):

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aianalysis-controller
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: aianalysis-controller
  template:
    metadata:
      labels:
        app: aianalysis-controller
    spec:
      serviceAccountName: aianalysis-controller
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        runAsGroup: 65532
        fsGroup: 65532
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: manager
        image: aianalysis-controller:latest
        securityContext:
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
        env:
        - name: HOLMESGPT_API_KEY_PATH
          value: "/var/run/secrets/holmesgpt/api-key"
        - name: HOLMESGPT_ENDPOINT
          value: "http:// holmesgpt-api.kubernaut-system.svc:8080"
        volumeMounts:
        - name: tmp
          mountPath: /tmp
        - name: cache
          mountPath: /.cache
        - name: holmesgpt-credentials
          mountPath: "/var/run/secrets/holmesgpt"
          readOnly: true
      volumes:
      - name: tmp
        emptyDir: {}
      - name: cache
        emptyDir: {}
      - name: holmesgpt-credentials
        projected:
          sources:
          - secret:
              name: holmesgpt-api-credentials
              items:
              - key: api-key
                path: api-key
```

**Why These Settings**:
- **runAsNonRoot**: Prevents privilege escalation
- **readOnlyRootFilesystem**: Immutable container filesystem
- **drop ALL capabilities**: Minimal Linux capabilities
- **seccompProfile**: Syscall filtering
- **Projected volume**: Secure HolmesGPT credential mounting

---

