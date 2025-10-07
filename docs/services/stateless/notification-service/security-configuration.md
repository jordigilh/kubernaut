# Notification Service - Security Configuration

**Version**: 1.0  
**Last Updated**: October 6, 2025  
**Service Type**: Stateless HTTP API Service  
**Status**: ⚠️ NEEDS IMPLEMENTATION

---

## 📋 Overview

Security configuration for the Notification Service, covering authentication, authorization, sensitive data handling, and secure secret management.

---

## 🔐 Authentication

### **Kubernetes TokenReviewer** (Bearer Token)

**Pattern**: Consistent with all other Kubernaut services

```go
package notification

import (
    "context"
    "fmt"
    "net/http"
    "strings"

    authv1 "k8s.io/api/authentication/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes"
)

// AuthMiddleware validates Bearer tokens using Kubernetes TokenReviewer
func (s *NotificationService) AuthMiddleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Extract Bearer token
            authHeader := r.Header.Get("Authorization")
            if authHeader == "" {
                http.Error(w, "Missing Authorization header", http.StatusUnauthorized)
                return
            }

            if !strings.HasPrefix(authHeader, "Bearer ") {
                http.Error(w, "Invalid Authorization format", http.StatusUnauthorized)
                return
            }

            token := strings.TrimPrefix(authHeader, "Bearer ")

            // Validate with Kubernetes TokenReviewer
            review := &authv1.TokenReview{
                Spec: authv1.TokenReviewSpec{
                    Token: token,
                },
            }

            result, err := s.kubeClient.AuthenticationV1().TokenReviews().Create(
                context.TODO(), review, metav1.CreateOptions{},
            )

            if err != nil {
                http.Error(w, "Token validation failed", http.StatusUnauthorized)
                return
            }

            if !result.Status.Authenticated {
                http.Error(w, "Token not authenticated", http.StatusUnauthorized)
                return
            }

            // Add user info to context
            ctx := context.WithValue(r.Context(), "user", result.Status.User)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

---

## 🔒 Authorization (RBAC)

### **Required Permissions** (Notification Service)

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: notification-service
rules:
# TokenReviewer for authentication
- apiGroups: ["authentication.k8s.io"]
  resources: ["tokenreviews"]
  verbs: ["create"]

# SubjectAccessReview for RBAC checks (BR-NOT-037 - optional)
- apiGroups: ["authorization.k8s.io"]
  resources: ["subjectaccessreviews"]
  verbs: ["create"]

# Read ConfigMaps for channel configuration
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list"]

# Read Secrets for channel credentials
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get", "list"]
```

### **Client Permissions** (Services calling Notification Service)

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: notification-client
rules:
# Ability to get ServiceAccount tokens for authentication
- apiGroups: [""]
  resources: ["serviceaccounts/token"]
  verbs: ["create"]
```

---

## 🔐 **CRITICAL: Sensitive Data Sanitization** (BR-NOT-034)

### **Sanitization Pipeline**

**MANDATORY**: All notification payloads MUST pass through sanitization before delivery

```go
package notification

import (
    "regexp"
    "strings"
)

type Sanitizer struct {
    patterns map[string]*regexp.Regexp
}

func NewSanitizer() *Sanitizer {
    return &Sanitizer{
        patterns: map[string]*regexp.Regexp{
            // API Keys and tokens
            "api_key":     regexp.MustCompile(`(?i)(api[_-]?key|apikey|token)["\s:=]+([a-zA-Z0-9_\-]{20,})`),
            "bearer":      regexp.MustCompile(`(?i)Bearer\s+([a-zA-Z0-9_\-\.]{20,})`),
            
            // Passwords
            "password":    regexp.MustCompile(`(?i)(password|passwd|pwd)["\s:=]+([^\s"']{8,})`),
            
            // Database connection strings
            "db_conn":     regexp.MustCompile(`(?i)(postgres|mysql|mongodb)://[^@]+@[^\s]+`),
            
            // AWS credentials
            "aws_key":     regexp.MustCompile(`(?i)(AKIA[0-9A-Z]{16})`),
            "aws_secret":  regexp.MustCompile(`(?i)(aws[_-]?secret[_-]?access[_-]?key)["\s:=]+([a-zA-Z0-9/+=]{40})`),
            
            // Email addresses (PII)
            "email":       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
            
            // Phone numbers (PII)
            "phone":       regexp.MustCompile(`\+?[1-9]\d{1,14}`),
            
            // Credit cards
            "credit_card": regexp.MustCompile(`\b(?:\d{4}[-\s]?){3}\d{4}\b`),
        },
    }
}

// Sanitize removes sensitive data from payload
func (s *Sanitizer) Sanitize(content string) (sanitized string, actions []string) {
    sanitized = content
    actions = []string{}

    for name, pattern := range s.patterns {
        matches := pattern.FindAllString(sanitized, -1)
        if len(matches) > 0 {
            actions = append(actions, fmt.Sprintf("Redacted %d %s patterns", len(matches), name))
            sanitized = pattern.ReplaceAllString(sanitized, fmt.Sprintf("[REDACTED_%s]", strings.ToUpper(name)))
        }
    }

    return sanitized, actions
}

// SanitizePayload sanitizes notification payload
func (s *Sanitizer) SanitizePayload(payload *EscalationPayload) *SanitizationResult {
    result := &SanitizationResult{
        Actions: []string{},
    }

    // Sanitize alert annotations
    for key, value := range payload.Alert.Annotations {
        sanitized, actions := s.Sanitize(value)
        if len(actions) > 0 {
            payload.Alert.Annotations[key] = sanitized
            result.Actions = append(result.Actions, actions...)
        }
    }

    // Sanitize alert labels
    for key, value := range payload.Alert.Labels {
        sanitized, actions := s.Sanitize(value)
        if len(actions) > 0 {
            payload.Alert.Labels[key] = sanitized
            result.Actions = append(result.Actions, actions...)
        }
    }

    // Sanitize root cause analysis
    sanitized, actions := s.Sanitize(payload.RootCauseAnalysis.DetailedAnalysis)
    if len(actions) > 0 {
        payload.RootCauseAnalysis.DetailedAnalysis = sanitized
        result.Actions = append(result.Actions, actions...)
    }

    // Sanitize remediation descriptions
    for i := range payload.RecommendedRemediations {
        sanitized, actions := s.Sanitize(payload.RecommendedRemediations[i].Description)
        if len(actions) > 0 {
            payload.RecommendedRemediations[i].Description = sanitized
            result.Actions = append(result.Actions, actions...)
        }
    }

    return result
}

type SanitizationResult struct {
    Actions []string
}
```

### **Sanitization Metrics**

```go
var (
    sanitizationActionsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "notification_sanitization_actions_total",
            Help: "Total number of sanitization actions applied",
        },
        []string{"type"}, // "api_key", "password", "email", etc.
    )

    sanitizationLatency = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name: "notification_sanitization_duration_seconds",
            Help: "Duration of sanitization operations",
            Buckets: prometheus.LinearBuckets(0.001, 0.001, 10), // 1ms to 10ms
        },
    )
)
```

---

## 🔐 **Secure Secret Management**

### **Secret Mounting Strategy Decision**

**Decision**: Use **Kubernetes Projected Volumes** (Option 3) for V1, with migration path to External Secrets + Vault (Option 4) for production.

**Date Confirmed**: October 2025  
**Status**: ✅ Approved

---

### **Option Comparison**

| Option | Security | Simplicity | Production Ready | V1 Recommendation |
|--------|----------|------------|------------------|-------------------|
| **Option 1: Environment Variables** | 4/10 | 10/10 | ❌ No | ❌ Not Recommended |
| **Option 2: Volume Mount (tmpfs)** | 7/10 | 9/10 | ✅ Yes | ⚠️ Basic |
| **Option 3: Projected Volume** | 9/10 | 10/10 | ✅ Yes | ✅ **RECOMMENDED** |
| **Option 4: External Secrets + Vault** | 9.5/10 | 6/10 | ✅ Yes | ⏳ V2 (Future) |

---

### **V1 Approach: Kubernetes Projected Volumes (Option 3)** ⭐

**Why Projected Volumes are Best for V1**:

| Aspect | Assessment | Score |
|--------|------------|-------|
| **Security** | tmpfs (RAM-only), read-only, file permissions 0400, token rotation | 9/10 |
| **Simplicity** | Kubernetes native, no external dependencies (Vault, AWS Secrets Manager) | 10/10 |
| **Token Rotation** | ServiceAccount token auto-rotates (1 hour TTL) | 10/10 |
| **Operational Complexity** | Low - just mount volume, no Vault setup required | 10/10 |
| **Production Ready** | Used by many production Kubernetes services | 9/10 |

**Overall Score**: **9.5/10** - Excellent for V1 and most production use cases

---

### **Deployment Configuration (Projected Volume)**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: notification-service
  namespace: kubernaut-system
spec:
  replicas: 2  # HA deployment
  selector:
    matchLabels:
      app: notification-service
  template:
    metadata:
      labels:
        app: notification-service
    spec:
      serviceAccountName: notification-service

      # Security hardening
      securityContext:
        runAsNonRoot: true
        runAsUser: 65534  # nobody user
        fsGroup: 65534
        seccompProfile:
          type: RuntimeDefault

      containers:
      - name: notification-service
        image: quay.io/jordigilh/notification-service:v1.0.0

        args:
        - "--config=/etc/config/config.yaml"
        - "--secrets-dir=/var/run/secrets/kubernaut"

        ports:
        - name: http
          containerPort: 8080
        - name: metrics
          containerPort: 9090

        # Container-level security
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop:
            - ALL

        volumeMounts:
        # Non-sensitive config (ConfigMap)
        - name: config
          mountPath: /etc/config
          readOnly: true

        # Secrets (Projected Volume) ← OPTION 3
        - name: credentials
          mountPath: /var/run/secrets/kubernaut
          readOnly: true

        # Writable tmpfs for runtime files
        - name: tmp
          mountPath: /tmp

        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi

        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10

        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5

      volumes:
      # ConfigMap for non-sensitive config
      - name: config
        configMap:
          name: notification-config
          defaultMode: 0444

      # OPTION 3: Projected Volume with ServiceAccount Token
      - name: credentials
        projected:
          defaultMode: 0400  # Read-only for owner only
          sources:
          # Secret data
          - secret:
              name: notification-credentials
              items:
              - key: smtp-password
                path: smtp/password
              - key: slack-bot-token
                path: slack/token
              - key: teams-webhook-url
                path: teams/webhook
              - key: twilio-auth-token
                path: twilio/token

          # ServiceAccount token (auto-rotates)
          - serviceAccountToken:
              path: sa-token
              expirationSeconds: 3600  # 1 hour
              audience: notification-service

      # Tmpfs for writable directories
      - name: tmp
        emptyDir:
          medium: Memory
          sizeLimit: 64Mi
```

---

### **Secret Creation (Kubernetes Native)**

```yaml
# Create secret with channel credentials
apiVersion: v1
kind: Secret
metadata:
  name: notification-credentials
  namespace: kubernaut-system
type: Opaque
stringData:
  smtp-password: "changeme"
  slack-bot-token: "xoxb-changeme"
  teams-webhook-url: "https://outlook.office.com/webhook/changeme"
  twilio-auth-token: "changeme"
```

---

### **Application Secret Loading**

```go
// pkg/notification/config/loader.go
package config

import (
    "fmt"
    "os"
    "path/filepath"
)

type Config struct {
    Email EmailConfig `yaml:"email"`
    Slack SlackConfig `yaml:"slack"`
    Teams TeamsConfig `yaml:"teams"`
}

type EmailConfig struct {
    SMTPHost         string `yaml:"smtp_host"`
    SMTPPort         int    `yaml:"smtp_port"`
    SMTPUser         string `yaml:"smtp_user"`
    SMTPPasswordPath string `yaml:"smtp_password_path"`

    // Loaded at runtime from projected volume
    SMTPPassword     string `yaml:"-"`
}

type SlackConfig struct {
    BotTokenPath string `yaml:"bot_token_path"`
    BotToken     string `yaml:"-"`
}

type TeamsConfig struct {
    WebhookURLPath string `yaml:"webhook_url_path"`
    WebhookURL     string `yaml:"-"`
}

func LoadConfig(configPath, secretsDir string) (*Config, error) {
    // 1. Load non-sensitive config from ConfigMap
    cfg := &Config{}
    if err := loadYAML(configPath, cfg); err != nil {
        return nil, fmt.Errorf("load config: %w", err)
    }

    // 2. Load secrets from projected volume
    if err := loadSecrets(cfg, secretsDir); err != nil {
        return nil, fmt.Errorf("load secrets: %w", err)
    }

    return cfg, nil
}

func loadSecrets(cfg *Config, secretsDir string) error {
    // Load SMTP password
    smtpPasswordPath := filepath.Join(secretsDir, "smtp/password")
    smtpPassword, err := readSecretFile(smtpPasswordPath)
    if err != nil {
        return fmt.Errorf("read smtp password: %w", err)
    }
    cfg.Email.SMTPPassword = smtpPassword

    // Load Slack bot token
    slackTokenPath := filepath.Join(secretsDir, "slack/token")
    slackToken, err := readSecretFile(slackTokenPath)
    if err != nil {
        return fmt.Errorf("read slack token: %w", err)
    }
    cfg.Slack.BotToken = slackToken

    // Load Teams webhook URL
    teamsWebhookPath := filepath.Join(secretsDir, "teams/webhook")
    teamsWebhook, err := readSecretFile(teamsWebhookPath)
    if err != nil {
        return fmt.Errorf("read teams webhook: %w", err)
    }
    cfg.Teams.WebhookURL = teamsWebhook

    return nil
}

func readSecretFile(path string) (string, error) {
    // Verify file exists
    info, err := os.Stat(path)
    if err != nil {
        return "", fmt.Errorf("stat file: %w", err)
    }

    // Verify file permissions (should be 0400 - read-only for owner)
    if info.Mode().Perm() != 0400 {
        return "", fmt.Errorf("insecure file permissions: %v (expected 0400)", info.Mode().Perm())
    }

    // Read secret content
    data, err := os.ReadFile(path)
    if err != nil {
        return "", fmt.Errorf("read file: %w", err)
    }

    // Never log secret content
    return string(data), nil
}
```

---

### **V2 Migration Path: External Secrets + Vault (Option 4)** ⏳

**Future Enhancement** for production environments requiring centralized secret management.

#### **Why Migrate to Option 4 in V2**

| Feature | Option 3 (V1) | Option 4 (V2) | Benefit |
|---------|---------------|---------------|---------|
| **Secret Storage** | Kubernetes Secrets | Vault/AWS Secrets Manager | Centralized management |
| **Audit Trail** | K8s API logs | Vault audit logs | Complete access history |
| **Rotation Frequency** | Manual (90 days) | Automatic (5 minutes) | Improved security |
| **Secret Versioning** | ❌ No | ✅ Yes | Rollback capability |
| **Fine-Grained Access** | K8s RBAC | Vault policies | Per-service control |
| **Encryption at Rest** | K8s etcd | Vault encryption | Additional security layer |

#### **Migration Strategy (V1 → V2)**

**Step 1**: Deploy External Secrets Operator

```yaml
# Install External Secrets Operator via Helm
helm repo add external-secrets https://charts.external-secrets.io
helm install external-secrets \
  external-secrets/external-secrets \
  -n external-secrets-system \
  --create-namespace
```

**Step 2**: Configure Vault Secret Store

```yaml
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: vault-backend
  namespace: kubernaut-system
spec:
  provider:
    vault:
      server: "https://vault.company.com"
      path: "secret"
      version: "v2"
      auth:
        kubernetes:
          mountPath: "kubernetes"
          role: "notification-service"
```

**Step 3**: Create ExternalSecret (syncs from Vault to K8s)

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: notification-credentials
  namespace: kubernaut-system
spec:
  refreshInterval: 5m  # Auto-refresh every 5 minutes
  secretStoreRef:
    name: vault-backend
    kind: SecretStore
  target:
    name: notification-credentials  # Same name as V1 secret
    creationPolicy: Owner
  data:
  - secretKey: smtp-password
    remoteRef:
      key: notification/smtp
      property: password
  - secretKey: slack-bot-token
    remoteRef:
      key: notification/slack
      property: bot_token
  - secretKey: teams-webhook-url
    remoteRef:
      key: notification/teams
      property: webhook_url
  - secretKey: twilio-auth-token
    remoteRef:
      key: notification/twilio
      property: auth_token
```

**Step 4**: No Application Code Changes Required

The application still reads from `/var/run/secrets/kubernaut/smtp/password` - same paths as V1. External Secrets Operator syncs Vault secrets to Kubernetes, and the Projected Volume mount works identically.

**Migration Confidence**: **95%** - Seamless migration path with no application changes required.

---

### **Security Comparison Summary**

| Security Aspect | Option 3 (V1) | Option 4 (V2) |
|-----------------|---------------|---------------|
| **Storage** | Kubernetes etcd | Vault encrypted storage |
| **Access Control** | K8s RBAC | Vault policies + K8s RBAC |
| **Audit Logging** | K8s API audit | Vault audit logs |
| **Secret Rotation** | Manual (ServiceAccount auto) | Automatic (configurable) |
| **Secret Versioning** | ❌ No | ✅ Yes (rollback support) |
| **Encryption at Rest** | K8s etcd encryption | Vault encryption + K8s |
| **Operational Complexity** | Low | Medium (requires Vault) |
| **External Dependencies** | None | Vault/AWS Secrets Manager |

**Recommendation**: 
- ✅ **V1**: Start with Option 3 (Projected Volume) - excellent security, zero external dependencies
- ⏳ **V2**: Migrate to Option 4 (External Secrets + Vault) when centralized secret management is required

---

### **Why NOT Use Option 1 (Environment Variables)**

| Issue | Description | Security Impact |
|-------|-------------|-----------------|
| **Visible in /proc** | Secrets visible in `/proc/[pid]/environ` | 🔴 HIGH |
| **Container Inspect** | `kubectl describe pod` shows env vars | 🔴 HIGH |
| **Crash Dumps** | Core dumps include environment | 🔴 HIGH |
| **Error Messages** | May leak in logs/errors | 🔴 MEDIUM |
| **No Rotation** | Requires pod restart | 🟡 MEDIUM |

**Verdict**: ❌ **NEVER use environment variables for secrets** - too many leak vectors.

---

### **Channel Credentials Storage (Legacy/Alternative)**

**ALTERNATIVE** (if Projected Volume not feasible): CSI Secret Driver (External Secrets Operator)

```yaml
apiVersion: v1
kind: SecretProviderClass
metadata:
  name: notification-service-secrets
spec:
  provider: vault # or aws, azure, gcp
  parameters:
    vaultAddress: "https://vault.company.com"
    roleName: "notification-service"
    objects: |
      - objectName: "slack-webhook-url"
        secretKey: "url"
      - objectName: "smtp-password"
        secretKey: "password"
      - objectName: "twilio-auth-token"
        secretKey: "token"
      - objectName: "pagerduty-api-key"
        secretKey: "apiKey"
```

---

## 🔒 **Network Security**

### **Network Policies**

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: notification-service
  namespace: prometheus-alerts-slm
spec:
  podSelector:
    matchLabels:
      app: notification-service
  policyTypes:
  - Ingress
  - Egress
  
  ingress:
  # Allow from other Kubernaut services
  - from:
    - namespaceSelector:
        matchLabels:
          name: prometheus-alerts-slm
    ports:
    - protocol: TCP
      port: 8080
  
  # Allow from Prometheus for metrics scraping
  - from:
    - namespaceSelector:
        matchLabels:
          name: monitoring
      podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 9090
  
  egress:
  # Allow to Kubernetes API
  - to:
    - namespaceSelector: {}
      podSelector:
        matchLabels:
          component: apiserver
    ports:
    - protocol: TCP
      port: 443
  
  # Allow to external notification channels (Slack, Email, etc.)
  - to:
    - podSelector: {}
    ports:
    - protocol: TCP
      port: 443  # HTTPS for webhooks
    - protocol: TCP
      port: 587  # SMTP for email
```

---

## 🛡️ **Security Best Practices**

### **1. Input Validation**
- ✅ Validate all notification payloads against schema
- ✅ Reject payloads exceeding channel limits
- ✅ Sanitize all user-provided content

### **2. Output Encoding**
- ✅ HTML-encode for email
- ✅ JSON-encode for webhooks
- ✅ URL-encode for links

### **3. Rate Limiting**
- ✅ Per-recipient rate limits (100 req/min)
- ✅ Per-channel rate limits (Slack: 1 req/s, Email: 10/min)
- ✅ Global rate limits (5,000 req/min)

### **4. Secret Rotation**
- ✅ Channel credentials rotated every 90 days
- ✅ Kubernetes ServiceAccount tokens auto-rotated
- ✅ No hardcoded secrets in code

### **5. Audit Logging**
- ✅ Log all authentication attempts
- ✅ Log all sanitization actions
- ✅ Log all notification deliveries
- ✅ Log all failures

---

## 📊 **Security Metrics**

```go
var (
    authenticationAttempts = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "notification_auth_attempts_total",
            Help: "Total authentication attempts",
        },
        []string{"status"}, // "success", "failure"
    )

    sanitizationViolations = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "notification_sanitization_violations_total",
            Help: "Total sanitization violations detected",
        },
        []string{"type"},
    )

    rateLimitExceeded = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "notification_rate_limit_exceeded_total",
            Help: "Total rate limit violations",
        },
        []string{"recipient", "channel"},
    )
)
```

---

## 🎯 **Compliance**

### **BR-NOT-034: Sensitive Data Sanitization**
- ✅ All payloads sanitized before delivery
- ✅ Regex patterns for common secrets
- ✅ Semantic detection for PII
- ✅ Sanitization metrics tracked

### **BR-NOT-037: External Service Action Links**
- ✅ Links generated, not validated
- ✅ Authentication enforced by target service
- ✅ No RBAC permission checking for links
- ✅ Recipient responsible for access

---

**Document Maintainer**: Kubernaut Documentation Team  
**Last Updated**: October 6, 2025  
**Status**: ✅ Complete Specification

