# Dynamic Toolset Service - Security Configuration

**Version**: v1.0
**Last Updated**: October 6, 2025
**Service Type**: Stateless HTTP API + Kubernetes Controller
**Port**: 8080 (REST API + Health), 9090 (Metrics)

---

## Table of Contents

1. [Security Overview](#security-overview)
2. [Authentication](#authentication)
3. [Authorization](#authorization)
4. [Kubernetes RBAC](#kubernetes-rbac)
5. [Network Security](#network-security)
6. [Secrets Management](#secrets-management)
7. [Audit Logging](#audit-logging)
8. [Security Checklist](#security-checklist)

---

## Security Overview

### **Security Principles**

Dynamic Toolset Service implements defense-in-depth security for service discovery:

1. **Authentication**: Kubernetes TokenReviewer for service identity validation
2. **Authorization**: RBAC for discovery and ConfigMap management
3. **Kubernetes RBAC**: Read-only cluster access for service discovery, write access for ConfigMap
4. **Network Security**: Network policies, TLS encryption
5. **Audit Trail**: Comprehensive logging of all discovery and ConfigMap operations

### **Threat Model**

| Threat | Mitigation |
|--------|------------|
| **Unauthorized Service Discovery** | TokenReviewer authentication + RBAC |
| **Unauthorized ConfigMap Modification** | RBAC for ConfigMap write access |
| **Service Spoofing** | Service health validation before inclusion |
| **ConfigMap Deletion** | Reconciliation controller recreates ConfigMap |
| **Man-in-the-Middle** | mTLS for all service-to-service communication |

---

## Authentication

### **Kubernetes TokenReviewer**

**Implementation**: `pkg/auth/tokenreviewer.go`

```go
package auth

import (
    "context"
    "fmt"

    authv1 "k8s.io/api/authentication/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes"
    "go.uber.org/zap"
)

type TokenReviewer struct {
    client *kubernetes.Clientset
    logger *zap.Logger
}

func NewTokenReviewer(client *kubernetes.Clientset, logger *zap.Logger) *TokenReviewer {
    return &TokenReviewer{
        client: client,
        logger: logger,
    }
}

func (tr *TokenReviewer) ValidateToken(ctx context.Context, token string) (*authv1.UserInfo, error) {
    review := &authv1.TokenReview{
        Spec: authv1.TokenReviewSpec{
            Token: token,
        },
    }

    result, err := tr.client.AuthenticationV1().TokenReviews().Create(ctx, review, metav1.CreateOptions{})
    if err != nil {
        tr.logger.Error("Token review failed", zap.Error(err))
        return nil, fmt.Errorf("token review failed: %w", err)
    }

    if !result.Status.Authenticated {
        tr.logger.Warn("Token authentication failed",
            zap.String("error", result.Status.Error),
        )
        return nil, fmt.Errorf("token not authenticated")
    }

    tr.logger.Info("Token validated successfully",
        zap.String("username", result.Status.User.Username),
        zap.Strings("groups", result.Status.User.Groups),
    )

    return &result.Status.User, nil
}
```

### **HTTP Middleware**

```go
package middleware

import (
    "context"
    "net/http"
    "strings"

    "go.uber.org/zap"
)

func AuthenticationMiddleware(tokenReviewer *auth.TokenReviewer, logger *zap.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Skip authentication for health checks
            if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
                next.ServeHTTP(w, r)
                return
            }

            // Extract Bearer token
            authHeader := r.Header.Get("Authorization")
            if authHeader == "" {
                logger.Warn("Missing Authorization header")
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }

            token := strings.TrimPrefix(authHeader, "Bearer ")
            if token == authHeader {
                logger.Warn("Invalid Authorization header format")
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }

            // Validate token with Kubernetes
            userInfo, err := tokenReviewer.ValidateToken(r.Context(), token)
            if err != nil {
                logger.Warn("Token validation failed", zap.Error(err))
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }

            // Store user info in context for authorization
            ctx := context.WithValue(r.Context(), "userInfo", userInfo)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

---

## Authorization

### **RBAC Configuration**

**ServiceAccount**: `dynamic-toolset-sa`

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: dynamic-toolset-sa
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: dynamic-toolset-role
rules:
# Service discovery (read-only)
- apiGroups: [""]
  resources: ["services", "endpoints"]
  verbs: ["get", "list", "watch"]
# ConfigMap management (read + write)
- apiGroups: [""]
  resources: ["configmaps"]
  resourceNames: ["holmesgpt-toolset"]
  verbs: ["get", "create", "update", "patch"]
# ConfigMap watch (for reconciliation)
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["list", "watch"]
# Leader election
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "create", "update"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: dynamic-toolset-rolebinding
subjects:
- kind: ServiceAccount
  name: dynamic-toolset-sa
  namespace: kubernaut-system
roleRef:
  kind: ClusterRole
  name: dynamic-toolset-role
  apiGroup: rbac.authorization.k8s.io
```

### **Authorization Middleware**

```go
package middleware

import (
    "net/http"

    authv1 "k8s.io/api/authentication/v1"
    "go.uber.org/zap"
)

func AuthorizationMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            userInfo, ok := r.Context().Value("userInfo").(*authv1.UserInfo)
            if !ok {
                logger.Error("User info not found in context")
                http.Error(w, "Forbidden", http.StatusForbidden)
                return
            }

            // Check if user is authorized for service discovery operations
            if !isAuthorized(userInfo, r.Method, r.URL.Path) {
                logger.Warn("User not authorized",
                    zap.String("username", userInfo.Username),
                    zap.String("method", r.Method),
                    zap.String("path", r.URL.Path),
                )
                http.Error(w, "Forbidden", http.StatusForbidden)
                return
            }

            next.ServeHTTP(w, r)
        })
    }
}

func isAuthorized(userInfo *authv1.UserInfo, method, path string) bool {
    // Dynamic Toolset Service: Only allow discovery requests from admin or HolmesGPT API
    authorizedServiceAccounts := []string{
        "system:serviceaccount:kubernaut-system:holmesgpt-api-sa",
        "system:serviceaccount:kubernaut-system:admin-sa",
    }

    for _, sa := range authorizedServiceAccounts {
        if userInfo.Username == sa {
            return true
        }
    }

    return false
}
```

---

## Kubernetes RBAC

### **Service Discovery Permissions**

```yaml
# Read-only access for service discovery
rules:
- apiGroups: [""]
  resources: ["services"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["endpoints"]
  verbs: ["get", "list", "watch"]
```

### **ConfigMap Management Permissions**

```yaml
# Read + Write access for ConfigMap reconciliation
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  resourceNames: ["holmesgpt-toolset"]
  verbs: ["get", "create", "update", "patch"]
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["list", "watch"]  # For reconciliation loop
```

### **Leader Election Permissions**

```yaml
# Leader election for multi-replica deployments
rules:
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  resourceNames: ["dynamic-toolset-leader"]
  verbs: ["get", "create", "update"]
```

---

## Network Security

### **Network Policies**

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: dynamic-toolset-netpol
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: dynamic-toolset
  policyTypes:
  - Ingress
  - Egress
  ingress:
  # Allow from authorized services
  - from:
    - podSelector:
        matchLabels:
          app: holmesgpt-api
    - podSelector:
        matchLabels:
          app: admin-console
    ports:
    - protocol: TCP
      port: 8080
  # Allow Prometheus metrics scraping
  - from:
    - namespaceSelector:
        matchLabels:
          name: monitoring
    ports:
    - protocol: TCP
      port: 9090
  egress:
  # Allow to Kubernetes API server
  - to:
    - namespaceSelector: {}
      podSelector:
        matchLabels:
          component: kube-apiserver
    ports:
    - protocol: TCP
      port: 6443
  # Allow to discovered services (for health checks)
  - to:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 9090  # Prometheus
    - protocol: TCP
      port: 3000  # Grafana
    - protocol: TCP
      port: 16686 # Jaeger
    - protocol: TCP
      port: 9200  # Elasticsearch
  # Allow DNS
  - to:
    - namespaceSelector: {}
    ports:
    - protocol: UDP
      port: 53
```

---

## Secrets Management

### **No Secrets Required**

Dynamic Toolset Service does **not** require secrets for normal operation:
- ✅ Service discovery uses in-cluster Kubernetes client (ServiceAccount token)
- ✅ ConfigMap management uses in-cluster Kubernetes client
- ✅ No external API keys needed

### **Optional: Service Health Check Credentials**

If discovered services require authentication for health checks:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: service-health-check-credentials
  namespace: kubernaut-system
type: Opaque
stringData:
  prometheus_user: <PROMETHEUS_USER>
  prometheus_password: <PROMETHEUS_PASSWORD>
  grafana_token: <GRAFANA_API_TOKEN>
```

```go
package discovery

import (
    "context"
    "fmt"
    "net/http"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes"
    "go.uber.org/zap"
)

type ServiceHealthChecker struct {
    client *kubernetes.Clientset
    logger *zap.Logger
}

func (hc *ServiceHealthChecker) CheckPrometheusHealth(ctx context.Context, endpoint string) (bool, error) {
    // Load credentials from secret (if needed)
    secret, err := hc.client.CoreV1().Secrets("kubernaut-system").Get(ctx, "service-health-check-credentials", metav1.GetOptions{})
    if err != nil {
        hc.logger.Warn("Health check credentials not found, using no auth", zap.Error(err))
        return hc.checkHealthNoAuth(endpoint)
    }

    username := string(secret.Data["prometheus_user"])
    password := string(secret.Data["prometheus_password"])

    // Check health with basic auth
    req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/-/healthy", endpoint), nil)
    if err != nil {
        return false, err
    }
    req.SetBasicAuth(username, password)

    client := &http.Client{Timeout: 5 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return false, err
    }
    defer resp.Body.Close()

    return resp.StatusCode == http.StatusOK, nil
}
```

---

## Audit Logging

### **Security Event Logging**

```go
package logging

import (
    "go.uber.org/zap"
)

func LogSecurityEvent(logger *zap.Logger, event string, fields ...zap.Field) {
    allFields := append([]zap.Field{
        zap.String("event_type", "security"),
        zap.String("event", event),
    }, fields...)

    logger.Info("Security event", allFields...)
}

// Usage examples:
func (s *DynamicToolsetService) HandleDiscoveryRequest(ctx context.Context, req *DiscoveryRequest) error {
    userInfo := ctx.Value("userInfo").(*authv1.UserInfo)

    LogSecurityEvent(s.logger, "discovery_request_received",
        zap.String("username", userInfo.Username),
        zap.String("namespace", req.Namespace),
    )

    // ... discovery logic ...

    LogSecurityEvent(s.logger, "discovery_completed",
        zap.String("username", userInfo.Username),
        zap.Int("services_discovered", len(services)),
    )

    return nil
}
```

### **ConfigMap Reconciliation Logging**

```go
package controllers

import (
    "context"

    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/reconcile"
    "go.uber.org/zap"
)

func (r *ConfigMapReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
    r.logger.Info("ConfigMap reconciliation started",
        zap.String("configmap", req.Name),
        zap.String("namespace", req.Namespace),
    )

    // Check if ConfigMap exists
    configMap, err := r.client.CoreV1().ConfigMaps(req.Namespace).Get(ctx, req.Name, metav1.GetOptions{})
    if err != nil {
        if errors.IsNotFound(err) {
            LogSecurityEvent(r.logger, "configmap_deleted_detected",
                zap.String("configmap", req.Name),
            )
            // Recreate ConfigMap
            return r.recreateConfigMap(ctx, req)
        }
        return reconcile.Result{}, err
    }

    LogSecurityEvent(r.logger, "configmap_reconciliation_success",
        zap.String("configmap", req.Name),
    )

    return reconcile.Result{RequeueAfter: 5 * time.Minute}, nil
}
```

---

## Security Checklist

### **Pre-Deployment**

- [ ] TokenReviewer authentication implemented and tested
- [ ] RBAC roles and bindings created (dynamic-toolset-sa)
- [ ] ClusterRole limited to read-only service discovery
- [ ] ConfigMap write access limited to specific ConfigMap name
- [ ] Leader election configured for multi-replica deployments
- [ ] Network policies restrict ingress to authorized services
- [ ] NetworkPolicies configured for ingress/egress control
- [ ] Security event logging implemented for all operations
- [ ] Service health check validation implemented

### **Runtime Security**

- [ ] Monitor failed authentication attempts (alert if > 10/min)
- [ ] Monitor unauthorized ConfigMap modification attempts
- [ ] Audit logs reviewed regularly for suspicious activity
- [ ] ConfigMap reconciliation working (recreates after deletion)
- [ ] Leader election working correctly (only one active reconciler)

### **Kubernetes RBAC**

- [ ] Service discovery is read-only (no write access to Services)
- [ ] ConfigMap access limited to `holmesgpt-toolset` only
- [ ] No access to Secrets (except optional health check credentials)
- [ ] No access to Pods or Deployments

---

## Reference Documentation

- **TokenReviewer Auth**: `docs/architecture/KUBERNETES_TOKENREVIEWER_AUTH.md`
- **Logging Standard**: `docs/architecture/LOGGING_STANDARD.md`
- **API Specification**: `docs/services/stateless/dynamic-toolset/api-specification.md`
- **Overview**: `docs/services/stateless/dynamic-toolset/overview.md`

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Security Review**: Pending

