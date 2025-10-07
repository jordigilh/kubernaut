# Context API Service - Security Configuration

**Version**: 1.0
**Last Updated**: October 6, 2025
**Service Type**: Stateless HTTP API Service (Read-Only)
**Port**: 8080 (REST + Health), 9090 (Metrics)

---

## 📋 Overview

Security configuration for Context API Service, a **read-only** historical intelligence provider.

---

## 🔐 Authentication

### **Kubernetes TokenReviewer** (Bearer Token)

```go
package context

import (
    "context"
    "net/http"
    "strings"

    authv1 "k8s.io/api/authentication/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes"
)

func (s *ContextAPIService) AuthMiddleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            authHeader := r.Header.Get("Authorization")
            if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
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
                http.Error(w, "Token authentication failed", http.StatusUnauthorized)
                return
            }

            ctx := context.WithValue(r.Context(), "user", result.Status.User)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

---

## 🔒 RBAC Permissions

### **Context API Service Permissions**

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: context-api-service
rules:
# TokenReviewer for authentication
- apiGroups: ["authentication.k8s.io"]
  resources: ["tokenreviews"]
  verbs: ["create"]

# Read ConfigMaps for environment classification
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list"]
```

### **Client Permissions** (Services calling Context API)

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: context-api-client
rules:
- apiGroups: [""]
  resources: ["serviceaccounts/token"]
  verbs: ["create"]
```

---

## 🔐 Data Access Control

### **Read-Only Service** (BR-CTX-Security)

Context API is **read-only** - it NEVER modifies data:
- ✅ Reads from PostgreSQL (action history, effectiveness data)
- ✅ Queries vector database (semantic search)
- ✅ Retrieves cached data (Redis)
- ❌ **NEVER** writes to databases
- ❌ **NEVER** modifies Kubernetes resources

### **Security Implications**

**Low Risk Profile**:
- No data mutation capabilities
- No Kubernetes write permissions
- Cannot execute actions or modify state
- Read-only database connections

**Rate Limiting** (BR-CTX-Performance):
```go
package context

import (
    "net/http"

    "golang.org/x/time/rate"
)

// Per-client rate limiting
rateLimiter := rate.NewLimiter(100, 200) // 100 req/s, burst 200

func (s *ContextAPIService) RateLimitMiddleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if !rateLimiter.Allow() {
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
  name: context-api-service
  namespace: prometheus-alerts-slm
spec:
  podSelector:
    matchLabels:
      app: context-api-service
  policyTypes:
  - Ingress
  - Egress

  ingress:
  # Allow from AI Analysis, HolmesGPT API, Effectiveness Monitor
  - from:
    - namespaceSelector:
        matchLabels:
          name: prometheus-alerts-slm
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
  # Allow to PostgreSQL
  - to:
    - podSelector:
        matchLabels:
          app: postgresql
    ports:
    - protocol: TCP
      port: 5432

  # Allow to Redis
  - to:
    - podSelector:
        matchLabels:
          app: redis
    ports:
    - protocol: TCP
      port: 6379
```

---

## 🔒 Secret Management

### **Database Credentials** (CSI Secret Driver)

```yaml
apiVersion: v1
kind: SecretProviderClass
metadata:
  name: context-api-secrets
spec:
  provider: vault
  parameters:
    vaultAddress: "https://vault.company.com"
    roleName: "context-api-service"
    objects: |
      - objectName: "postgres-password"
        secretKey: "password"
      - objectName: "redis-password"
        secretKey: "password"
```

### **Deployment Configuration**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: context-api-service
  namespace: prometheus-alerts-slm
spec:
  template:
    spec:
      serviceAccountName: context-api-service
      containers:
      - name: context-api
        image: quay.io/jordigilh/context-service:latest
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: metrics
        volumeMounts:
        - name: secrets
          mountPath: "/mnt/secrets"
          readOnly: true
      volumes:
      - name: secrets
        csi:
          driver: secrets-store.csi.k8s.io
          readOnly: true
          volumeAttributes:
            secretProviderClass: "context-api-secrets"
```

---

## 📊 Security Metrics

```go
package context

import (
    "github.com/prometheus/client_golang/prometheus"
)

var (
    authenticationAttempts = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "context_api_auth_attempts_total",
            Help: "Total authentication attempts",
        },
        []string{"status"}, // "success", "failure"
    )

    rateLimitExceeded = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "context_api_rate_limit_exceeded_total",
            Help: "Total rate limit violations",
        },
        []string{"client"},
    )

    unauthorizedAccess = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "context_api_unauthorized_access_total",
            Help: "Total unauthorized access attempts",
        },
    )
)
```

---

## ✅ Security Best Practices

### **1. Authentication**
- ✅ All endpoints require Bearer token
- ✅ Kubernetes TokenReviewer validates tokens
- ✅ User context propagated through request chain

### **2. Authorization**
- ✅ Minimal RBAC permissions (read-only)
- ✅ No write permissions to any resources
- ✅ Service account with least privilege

### **3. Network Security**
- ✅ NetworkPolicies restrict ingress/egress
- ✅ TLS/mTLS for all communications
- ✅ No direct external access

### **4. Data Protection**
- ✅ Read-only database connections
- ✅ No sensitive data in logs
- ✅ Secrets managed via CSI driver

### **5. Rate Limiting**
- ✅ Per-client rate limits (100 req/s)
- ✅ Global rate limits (5,000 req/s)
- ✅ Burst capacity (200 req burst)

---

## 🎯 Security Compliance

### **BR-CTX Security Requirements**
- ✅ BR-CTX-SEC-001: Token-based authentication
- ✅ BR-CTX-SEC-002: Read-only data access
- ✅ BR-CTX-SEC-003: Rate limiting per client
- ✅ BR-CTX-SEC-004: Network isolation
- ✅ BR-CTX-SEC-005: Secret rotation support

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Status**: ✅ Complete Specification

