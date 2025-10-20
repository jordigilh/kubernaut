# Effectiveness Monitor Service - Security Configuration

**Version**: 1.0
**Last Updated**: October 6, 2025
**Service Type**: Stateless HTTP API Service (Assessment & Analysis)
**Port**: 8080 (REST + Health), 9090 (Metrics)

---

## 📋 Overview

Security configuration for Effectiveness Monitor Service, providing **assessment and analysis** of remediation action effectiveness with multi-dimensional evaluation.

---

## 🔐 Authentication

### **Kubernetes TokenReviewer** (Bearer Token)

```go
package effectiveness

import (
    "context"
    "net/http"
    "strings"

    authv1 "k8s.io/api/authentication/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes"
    "go.uber.org/zap"
)

func (s *EffectivenessMonitorService) AuthMiddleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            authHeader := r.Header.Get("Authorization")
            if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
                s.logger.Warn("Missing or invalid Authorization header",
                    zap.String("method", r.Method),
                    zap.String("path", r.URL.Path),
                )
                http.Error(w, "Missing or invalid Authorization header", http.StatusUnauthorized)
                return
            }

            token := strings.TrimPrefix(authHeader, "Bearer ")
            review := &authv1.TokenReview{
                Spec: authv1.TokenReviewSpec{Token: token},
            }

            result, err := s.kubeClient.AuthenticationV1().TokenReviews().Create(
                context.TODO(), review, metav1.CreateOptions{},
            )

            if err != nil || !result.Status.Authenticated {
                s.logger.Warn("Token authentication failed",
                    zap.Error(err),
                    zap.Bool("authenticated", result.Status.Authenticated),
                )
                http.Error(w, "Token authentication failed", http.StatusUnauthorized)
                return
            }

            s.logger.Debug("Token authenticated successfully",
                zap.String("username", result.Status.User.Username),
                zap.Strings("groups", result.Status.User.Groups),
            )

            ctx := context.WithValue(r.Context(), "user", result.Status.User)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

---

## 🤖 HolmesGPT API Authentication

### **Service Account Token for AI Analysis**

**Purpose**: Authenticate to HolmesGPT API for post-execution analysis when AI decision logic triggers.

**Token Source**: Kubernetes ServiceAccount mounted in pod

```go
// pkg/monitor/holmesgpt_client.go
package monitor

import (
    "context"
    "crypto/tls"
    "fmt"
    "net/http"
    "os"
)

type HolmesGPTClient struct {
    baseURL    string
    httpClient *http.Client
    token      string
}

// NewHolmesGPTClient creates client with ServiceAccount token
func NewHolmesGPTClient(baseURL string) (*HolmesGPTClient, error) {
    // Read ServiceAccount token from mounted volume
    tokenPath := "/var/run/secrets/kubernetes.io/serviceaccount/token"
    tokenBytes, err := os.ReadFile(tokenPath)
    if err != nil {
        return nil, fmt.Errorf("failed to read service account token: %w", err)
    }

    // Create HTTP client with TLS
    client := &http.Client{
        Transport: &http.Transport{
            TLSClientConfig: &tls.Config{
                MinVersion: tls.VersionTLS12,
            },
        },
        Timeout: 30 * time.Second,
    }

    return &HolmesGPTClient{
        baseURL:    baseURL,
        httpClient: client,
        token:      string(tokenBytes),
    }, nil
}

// AnalyzePostExecution calls HolmesGPT API with authentication
func (c *HolmesGPTClient) AnalyzePostExecution(ctx context.Context, req PostExecRequest) (*PostExecResponse, error) {
    url := fmt.Sprintf("%s/api/v1/postexec/analyze", c.baseURL)

    httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
    if err != nil {
        return nil, err
    }

    // Add Bearer token for authentication
    httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
    httpReq.Header.Set("Content-Type", "application/json")

    // Execute request
    resp, err := c.httpClient.Do(httpReq)
    // ... (handle response)
}
```

### **ServiceAccount Configuration**

```yaml
# deploy/effectiveness-monitor/serviceaccount.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: effectiveness-monitor
  namespace: prometheus-alerts-slm
---
# deploy/effectiveness-monitor/clusterrole.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: effectiveness-monitor-holmesgpt-client
rules:
# No additional permissions needed for HolmesGPT API calls
# (Authentication via ServiceAccount token, authorization handled by HolmesGPT API)
---
# deploy/effectiveness-monitor/deployment.yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      serviceAccountName: effectiveness-monitor
      containers:
      - name: effectiveness-monitor
        env:
        - name: HOLMESGPT_API_URL
          value: "http://holmesgpt-api.prometheus-alerts-slm.svc.cluster.local:8080"
```

### **HolmesGPT API Authorization**

**Authorization handled by HolmesGPT API** (not Effectiveness Monitor):

- HolmesGPT API validates ServiceAccount token via Kubernetes TokenReview
- HolmesGPT API checks RBAC permissions for `/api/v1/postexec/analyze` endpoint
- Effectiveness Monitor only needs valid ServiceAccount token

**Required RBAC on HolmesGPT side** (defined in HolmesGPT API service):

```yaml
# HolmesGPT API RBAC (not Effectiveness Monitor)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: holmesgpt-api-postexec-analyzer
rules:
- apiGroups: ["holmesgpt.kubernaut.io"]
  resources: ["postexecanalyses"]
  verbs: ["create"]
---
# HolmesGPT API ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: effectiveness-monitor-holmesgpt-access
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: holmesgpt-api-postexec-analyzer
subjects:
- kind: ServiceAccount
  name: effectiveness-monitor
  namespace: prometheus-alerts-slm
```

### **Security Best Practices**

**Token Rotation**:
- ServiceAccount tokens auto-rotate (Kubernetes projected volume)
- Client automatically picks up new token on next read

**TLS/SSL**:
- All HolmesGPT API calls use HTTPS (TLS 1.2+)
- Certificate validation enabled

**Error Handling**:
```go
// Graceful degradation on authentication failure
resp, err := holmesgptClient.AnalyzePostExecution(ctx, req)
if err != nil {
    logger.Warn("HolmesGPT API authentication failed, using automated assessment",
        zap.Error(err),
        zap.String("workflow_id", workflow.ID),
    )
    // Fallback to automated assessment (no AI)
    return automatedAssessment, nil
}
```

**Rate Limiting**:
- HolmesGPT API enforces rate limits per ServiceAccount
- Effectiveness Monitor respects rate limits with exponential backoff

---

## 🔒 RBAC Permissions

### **Effectiveness Monitor Service Permissions**

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: effectiveness-monitor-service
rules:
# TokenReviewer for authentication
- apiGroups: ["authentication.k8s.io"]
  resources: ["tokenreviews"]
  verbs: ["create"]

# Read ConfigMaps for configuration
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list"]
```

### **Client Permissions** (Services calling Effectiveness Monitor)

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: effectiveness-monitor-client
rules:
- apiGroups: [""]
  resources: ["serviceaccounts/token"]
  verbs: ["create"]
```

---

## 🔐 Data Access Control

### **Assessment Service** (BR-INS-Security)

Effectiveness Monitor has **limited write capabilities**:
- ✅ Reads from Data Storage (action history, effectiveness data)
- ✅ Queries Infrastructure Monitoring (metrics correlation)
- ✅ Writes assessment results to Data Storage (audit trail)
- ❌ **NEVER** modifies Kubernetes resources
- ❌ **NEVER** executes remediation actions

### **Security Implications**

**Medium Risk Profile**:
- Assessment results inform decision-making
- Writes to audit trail (tamper-proof logging required)
- No Kubernetes write permissions
- Cannot execute actions directly

**Rate Limiting** (BR-INS-Performance):
```go
package effectiveness

import (
    "net/http"

    "golang.org/x/time/rate"
    "go.uber.org/zap"
)

// Per-client rate limiting
func (s *EffectivenessMonitorService) RateLimitMiddleware() func(http.Handler) http.Handler {
    limiter := rate.NewLimiter(50, 100) // 50 req/s, burst 100

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if !limiter.Allow() {
                s.logger.Warn("Rate limit exceeded",
                    zap.String("client_ip", r.RemoteAddr),
                    zap.String("path", r.URL.Path),
                )
                http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

---

## 🛡️ Network Security

### **Network Policies**

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: effectiveness-monitor-service
  namespace: prometheus-alerts-slm
spec:
  podSelector:
    matchLabels:
      app: effectiveness-monitor-service
  policyTypes:
  - Ingress
  - Egress

  ingress:
  # Allow from Context API, HolmesGPT API (assessment requests)
  - from:
    - namespaceSelector:
        matchLabels:
          name: prometheus-alerts-slm
      podSelector:
        matchLabels:
          app: context-api-service
    - namespaceSelector:
        matchLabels:
          name: prometheus-alerts-slm
      podSelector:
        matchLabels:
          app: holmesgpt-api-service
    ports:
    - protocol: TCP
      port: 8080

  # Allow from Prometheus for metrics
  - from:
    - namespaceSelector:
        matchLabels:
          name: monitoring
    ports:
    - protocol: TCP
      port: 9090

  egress:
  # Allow to Data Storage Service (action history, assessment storage)
  - to:
    - namespaceSelector:
        matchLabels:
          name: prometheus-alerts-slm
      podSelector:
        matchLabels:
          app: data-storage-service
    ports:
    - protocol: TCP
      port: 8085

  # Allow to Infrastructure Monitoring Service (metrics correlation)
  - to:
    - namespaceSelector:
        matchLabels:
          name: prometheus-alerts-slm
      podSelector:
        matchLabels:
          app: infrastructure-monitoring-service
    ports:
    - protocol: TCP
      port: 8094

  # Allow to Kubernetes API server
  - to:
    - namespaceSelector: {}
      podSelector:
        matchLabels:
          component: apiserver
    ports:
    - protocol: TCP
      port: 443

  # Allow DNS
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

**Why No Service Mesh?**

Kubernetes-native security features (TokenReviewer + NetworkPolicies) provide:
- Mutual TLS via pod-to-pod encryption (if CNI supports)
- Fine-grained network access control
- Kubernetes-native RBAC integration
- No additional operational complexity

---

## 🔐 Database Security

### **Data Storage Service Connection** (PostgreSQL + pgvector)

```go
package effectiveness

import (
    "context"
    "database/sql"
    "fmt"

    _ "github.com/lib/pq"
    "go.uber.org/zap"
)

func (s *EffectivenessMonitorService) ConnectToDataStorage(ctx context.Context) (*sql.DB, error) {
    // Credentials from Kubernetes Secret
    dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=require",
        s.config.DataStorage.Host,
        s.config.DataStorage.Port,
        s.config.DataStorage.Username,
        s.config.DataStorage.Password,
        s.config.DataStorage.Database,
    )

    db, err := sql.Open("postgres", dsn)
    if err != nil {
        return nil, fmt.Errorf("failed to connect to Data Storage: %w", err)
    }

    // Connection pooling
    db.SetMaxOpenConns(25)
    db.SetMaxIdleConns(10)
    db.SetConnMaxLifetime(5 * time.Minute)

    s.logger.Info("Connected to Data Storage Service",
        zap.String("host", s.config.DataStorage.Host),
        zap.Int("port", s.config.DataStorage.Port),
    )

    return db, nil
}
```

### **Credentials Management**

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: effectiveness-monitor-db-credentials
  namespace: prometheus-alerts-slm
type: Opaque
stringData:
  DB_USERNAME: effectiveness_monitor_user
  DB_PASSWORD: <generate-secure-password>
  DB_HOST: data-storage-service.prometheus-alerts-slm.svc.cluster.local
  DB_PORT: "5432"
  DB_NAME: kubernaut_audit
```

**Load Credentials in Main Application**:

```go
package main

import (
    "context"
    "fmt"
    "os"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes"
    "go.uber.org/zap"
)

func loadDatabaseCredentials(ctx context.Context, kubeClient *kubernetes.Clientset, logger *zap.Logger) (*DatabaseConfig, error) {
    secret, err := kubeClient.CoreV1().Secrets("prometheus-alerts-slm").Get(
        ctx, "effectiveness-monitor-db-credentials", metav1.GetOptions{},
    )
    if err != nil {
        logger.Error("Failed to load database credentials from secret", zap.Error(err))
        return nil, fmt.Errorf("failed to load database credentials: %w", err)
    }

    config := &DatabaseConfig{
        Host:     string(secret.Data["DB_HOST"]),
        Port:     5432, // Parse from DB_PORT if needed
        Username: string(secret.Data["DB_USERNAME"]),
        Password: string(secret.Data["DB_PASSWORD"]),
        Database: string(secret.Data["DB_NAME"]),
    }

    logger.Info("Database credentials loaded successfully")
    return config, nil
}
```

---

## 🔐 Sensitive Data Handling

### **Assessment Data Classification**

| Data Type | Sensitivity | Handling |
|-----------|-------------|----------|
| **Effectiveness Score** | Medium | Store in audit trail, do not log raw scores |
| **Side Effect Details** | High | Sanitize before logging, store encrypted |
| **Environmental Metrics** | Medium | Aggregate only, no raw infrastructure metrics in logs |
| **Pattern Insights** | Low | Safe to log and expose via API |

### **Logging Sensitive Data**

```go
package effectiveness

import (
    "go.uber.org/zap"
)

func (s *EffectivenessMonitorService) LogAssessmentResult(assessment *EffectivenessScore) {
    // DO NOT log raw scores - aggregate only
    s.logger.Info("Assessment completed",
        zap.String("assessment_id", assessment.AssessmentID),
        zap.String("action_type", assessment.ActionType),
        zap.String("confidence_level", s.confidenceBucket(assessment.Confidence)), // Bucketed
        zap.Bool("side_effects_detected", assessment.SideEffectsDetected),
        // DO NOT LOG: assessment.TraditionalScore, assessment.EnvironmentalImpact.CPUImpact
    )
}

func (s *EffectivenessMonitorService) confidenceBucket(confidence float64) string {
    if confidence >= 0.8 {
        return "high"
    } else if confidence >= 0.5 {
        return "medium"
    }
    return "low"
}
```

---

## 🛡️ Secrets Management

### **Service Account**

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: effectiveness-monitor-sa
  namespace: prometheus-alerts-slm
```

### **Deployment with ServiceAccount**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: effectiveness-monitor-service
  namespace: prometheus-alerts-slm
spec:
  replicas: 2
  selector:
    matchLabels:
      app: effectiveness-monitor-service
  template:
    metadata:
      labels:
        app: effectiveness-monitor-service
    spec:
      serviceAccountName: effectiveness-monitor-sa
      containers:
      - name: effectiveness-monitor
        image: effectiveness-monitor:v1.0.0
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: metrics
        env:
        - name: DB_USERNAME
          valueFrom:
            secretKeyRef:
              name: effectiveness-monitor-db-credentials
              key: DB_USERNAME
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: effectiveness-monitor-db-credentials
              key: DB_PASSWORD
        - name: DB_HOST
          valueFrom:
            secretKeyRef:
              name: effectiveness-monitor-db-credentials
              key: DB_HOST
        - name: DB_PORT
          valueFrom:
            secretKeyRef:
              name: effectiveness-monitor-db-credentials
              key: DB_PORT
        - name: DB_NAME
          valueFrom:
            secretKeyRef:
              name: effectiveness-monitor-db-credentials
              key: DB_NAME
```

---

## 📊 Metrics Security

### **Prometheus Metrics with Authentication**

```go
package effectiveness

import (
    "net/http"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "go.uber.org/zap"
)

var (
    assessmentDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "effectiveness_assessment_duration_seconds",
            Help:    "Duration of effectiveness assessments",
            Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
        },
        []string{"action_type", "confidence_level"},
    )

    effectivenessScore = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "effectiveness_traditional_score",
            Help:    "Traditional effectiveness score distribution",
            Buckets: prometheus.LinearBuckets(0, 0.1, 11),
        },
        []string{"action_type", "environment"},
    )
)

func init() {
    prometheus.MustRegister(assessmentDuration, effectivenessScore)
}

// Metrics endpoint with authentication
func (s *EffectivenessMonitorService) metricsHandler() http.Handler {
    return s.AuthMiddleware()(promhttp.Handler())
}
```

---

## ✅ Security Checklist

### **Pre-Deployment**

- [ ] Kubernetes TokenReviewer authentication implemented
- [ ] RBAC ClusterRole and ServiceAccount configured
- [ ] NetworkPolicy restricts ingress/egress
- [ ] Database credentials stored in Kubernetes Secret
- [ ] Rate limiting configured (50 req/s)
- [ ] Sensitive data sanitized in logs

### **Runtime Security**

- [ ] All API requests authenticated via Bearer token
- [ ] Assessment results stored in tamper-proof audit trail
- [ ] Metrics endpoint secured with TokenReviewer
- [ ] Connection to Data Storage uses TLS (sslmode=require)
- [ ] No raw effectiveness scores in logs

### **Monitoring**

- [ ] Authentication failures logged with context
- [ ] Rate limit violations tracked in metrics
- [ ] Database connection failures alerted
- [ ] Unauthorized access attempts monitored

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Status**: ✅ Complete Specification

