# Gateway Service - Security Configuration

> **📋 Changelog**
> | Version | Date | Changes | Reference |
> |---------|------|---------|-----------|
> | v1.1 | 2025-12-03 | Added RemediationRequest CRD write permissions, Redis secret access | API contract alignment |
> | v1.0 | 2025-10-04 | Initial security configuration | - |

---

## Authentication

### JWT Bearer Token Authentication

**Pattern**: Kubernetes TokenReviewer API (consistent with other services)

```go
// pkg/gateway/auth.go
package gateway

import (
    "context"
    "fmt"
    "net/http"
    "strings"

    authv1 "k8s.io/api/authentication/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes"
)

// createAuthMiddleware creates JWT authentication middleware
func (s *Server) createAuthMiddleware() func(http.Handler) http.Handler {
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

            result, err := s.k8sClientset.AuthenticationV1().TokenReviews().Create(
                r.Context(), review, metav1.CreateOptions{},
            )
            if err != nil {
                http.Error(w, "Token validation failed", http.StatusUnauthorized)
                return
            }

            if !result.Status.Authenticated {
                http.Error(w, "Token not authenticated", http.StatusUnauthorized)
                return
            }

            // Store user info in context for RBAC
            ctx := context.WithValue(r.Context(), "user", result.Status.User)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

### Alertmanager Configuration

```yaml
receivers:
  - name: kubernaut-gateway
    webhook_configs:
      - url: http://gateway-service.kubernaut-system:8080/api/v1/signals
        send_resolved: true
        http_config:
          bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
          # Optional: Explicitly declare source type (auto-detected by default)
          headers:
            X-Signal-Source: prometheus
```

---

## RBAC Configuration

### Gateway Service Permissions

```yaml
---
# ServiceAccount for Gateway
apiVersion: v1
kind: ServiceAccount
metadata:
  name: gateway
  namespace: kubernaut-system
---
# ClusterRole for Gateway operations
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: gateway-controller
rules:
# RemediationRequest CRD creation
- apiGroups: ["remediation.kubernaut.io"]
  resources: ["remediationrequests"]
  verbs: ["create", "get", "list", "watch"]
# Namespace labels for environment classification
- apiGroups: [""]
  resources: ["namespaces"]
  verbs: ["get", "list", "watch"]
# ConfigMap for environment overrides
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch"]
# Events for audit trail
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
# Kubernetes Events watching
- apiGroups: [""]
  resources: ["events"]
  verbs: ["list", "watch"]
---
# ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: gateway-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: gateway-controller
subjects:
- kind: ServiceAccount
  name: gateway
  namespace: kubernaut-system
```

### Alertmanager ServiceAccount Permissions

```yaml
---
# ServiceAccount for Alertmanager
apiVersion: v1
kind: ServiceAccount
metadata:
  name: alertmanager
  namespace: monitoring
---
# ClusterRole for signal submission
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: gateway-signal-submitter
rules:
- nonResourceURLs:
    - /api/v1/signals
  verbs:
    - post
---
# ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: alertmanager-gateway-access
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: gateway-alert-submitter
subjects:
- kind: ServiceAccount
  name: alertmanager
  namespace: monitoring
```

---

## Network Policies

```yaml
---
# Ingress: Allow Alertmanager + K8s API
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gateway-ingress
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: gateway
  policyTypes:
  - Ingress
  ingress:
  # Allow from Alertmanager
  - from:
    - namespaceSelector:
        matchLabels:
          name: monitoring
      podSelector:
        matchLabels:
          app: alertmanager
    ports:
    - protocol: TCP
      port: 8080
  # Allow from Prometheus (metrics)
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
---
# Egress: Allow K8s API + Redis + Monitoring
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gateway-egress
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: gateway
  policyTypes:
  - Egress
  egress:
  # Allow to K8s API server
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    ports:
    - protocol: TCP
      port: 443
  # Allow to Redis
  - to:
    - podSelector:
        matchLabels:
          app: redis
    ports:
    - protocol: TCP
      port: 6379
  # Allow DNS
  - to:
    - namespaceSelector: {}
      podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
```

---

## Secrets Management

### Redis Credentials

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: gateway-redis-credentials
  namespace: kubernaut-system
type: Opaque
stringData:
  redis-password: <REDIS_PASSWORD>
  redis-url: redis://redis.kubernaut-system:6379
```

### Rego Policy ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: gateway-priority-policy
  namespace: kubernaut-system
data:
  priority.rego: |
    package kubernaut.priority
    default priority = "P2"
    priority = "P0" {
        input.severity == "critical"
        input.environment == "prod"
        input.namespace in ["payment-service", "auth-service"]
    }
```

---

## Security Context

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: kubernaut-system
spec:
  template:
    spec:
      serviceAccountName: gateway
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
      - name: gateway
        image: kubernaut/gateway:v1.0
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop:
            - ALL
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: metrics
```

**Confidence**: 95%
