# Data Storage Service - Security Configuration

**Version**: v1.0
**Last Updated**: October 6, 2025
**Service Type**: Stateless HTTP API Service (Write-Focused)
**Port**: 8080 (REST API + Health), 9090 (Metrics)

---

## Table of Contents

1. [Security Overview](#security-overview)
2. [Authentication](#authentication)
3. [Authorization](#authorization)
4. [Database Security](#database-security)
5. [Network Security](#network-security)
6. [Secrets Management](#secrets-management)
7. [Audit Logging](#audit-logging)
8. [Security Checklist](#security-checklist)

---

## Security Overview

### **Security Principles**

Data Storage Service implements defense-in-depth security for write operations:

1. **Authentication**: Kubernetes TokenReviewer for service identity validation
2. **Authorization**: RBAC for write permission enforcement
3. **Database Security**: Encrypted connections, credentials in secrets
4. **Network Security**: Network policies, TLS encryption
5. **Audit Trail**: Comprehensive logging of all write operations

### **Threat Model**

| Threat | Mitigation |
|--------|------------|
| **Unauthorized Writes** | TokenReviewer authentication + RBAC |
| **SQL Injection** | Prepared statements + input validation |
| **Data Tampering** | Write-only API, immutable audit trail |
| **Credential Exposure** | Kubernetes secrets, no hardcoded credentials |
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
    "net/http"
    "strings"

    "go.uber.org/zap"
)

func AuthenticationMiddleware(tokenReviewer *auth.TokenReviewer, logger *zap.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

**ServiceAccount**: `data-storage-sa`

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: data-storage-sa
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: data-storage-role
  namespace: kubernaut-system
rules:
- apiGroups: [""]
  resources: ["secrets"]
  resourceNames: ["postgres-credentials", "vectordb-credentials"]
  verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: data-storage-rolebinding
  namespace: kubernaut-system
subjects:
- kind: ServiceAccount
  name: data-storage-sa
  namespace: kubernaut-system
roleRef:
  kind: Role
  name: data-storage-role
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

            // Check if user is authorized for write operations
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
    // Data Storage Service: Only allow writes from authorized services
    authorizedServiceAccounts := []string{
        "system:serviceaccount:kubernaut-system:gateway-sa",
        "system:serviceaccount:kubernaut-system:aianalysis-controller-sa",
        "system:serviceaccount:kubernaut-system:workflowexecution-controller-sa",
        "system:serviceaccount:kubernaut-system:kubernetes-executor-sa",  // DEPRECATED - ADR-025
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

## Database Security

### **PostgreSQL Connection Security**

```go
package database

import (
    "database/sql"
    "fmt"

    _ "github.com/lib/pq"
    "go.uber.org/zap"
)

type PostgresConfig struct {
    Host     string
    Port     int
    Database string
    User     string
    Password string
    SSLMode  string // require, verify-ca, verify-full
}

func NewPostgresConnection(config PostgresConfig, logger *zap.Logger) (*sql.DB, error) {
    // Build connection string with SSL enabled
    connStr := fmt.Sprintf(
        "host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
        config.Host,
        config.Port,
        config.Database,
        config.User,
        config.Password,
        config.SSLMode, // Always use SSL
    )

    db, err := sql.Open("postgres", connStr)
    if err != nil {
        logger.Error("Failed to open PostgreSQL connection", zap.Error(err))
        return nil, err
    }

    // Test connection
    if err := db.Ping(); err != nil {
        logger.Error("Failed to ping PostgreSQL", zap.Error(err))
        return nil, err
    }

    logger.Info("PostgreSQL connection established",
        zap.String("host", config.Host),
        zap.Int("port", config.Port),
        zap.String("database", config.Database),
    )

    return db, nil
}
```

### **SQL Injection Prevention**

**Implementation**: Always use prepared statements

```go
package storage

import (
    "context"
    "database/sql"
)

func (s *DataStorageService) WriteRemediationAudit(ctx context.Context, audit *RemediationAudit) error {
    // ✅ CORRECT: Prepared statement with parameters
    query := `
        INSERT INTO remediation_audit (
            id, remediation_request_id, alert_name, namespace, cluster,
            action_type, status, timestamp, embedding
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
    `

    _, err := s.db.ExecContext(ctx, query,
        audit.ID,
        audit.RemediationRequestID,
        audit.AlertName,
        audit.Namespace,
        audit.Cluster,
        audit.ActionType,
        audit.Status,
        audit.Timestamp,
        audit.Embedding,
    )

    return err
}
```

---

## Network Security

### **Network Policies**

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: data-storage-netpol
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: data-storage
  policyTypes:
  - Ingress
  - Egress
  ingress:
  # Allow from authorized services only
  - from:
    - podSelector:
        matchLabels:
          app: gateway
    - podSelector:
        matchLabels:
          app: aianalysis-controller
    - podSelector:
        matchLabels:
          app: workflowexecution-controller
    - podSelector:
        matchLabels:
          app: kubernetes-executor  # DEPRECATED - ADR-025
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
  # Allow to PostgreSQL
  - to:
    - podSelector:
        matchLabels:
          app: postgresql
    ports:
    - protocol: TCP
      port: 5432
  # Allow to Vector DB (PostgreSQL with pgvector)
  - to:
    - podSelector:
        matchLabels:
          app: vectordb
    ports:
    - protocol: TCP
      port: 5433
  # Allow DNS
  - to:
    - namespaceSelector: {}
    ports:
    - protocol: UDP
      port: 53
```

### **Why No Service Mesh?**

**Decision**: Service mesh (Istio/Linkerd) is **not required** for Data Storage Service because:
- ✅ Authentication via Kubernetes TokenReviewer (sufficient for service-to-service auth)
- ✅ NetworkPolicies provide pod-to-pod security
- ✅ Services operate within controlled Kubernetes environment
- ✅ Reduces operational complexity and resource overhead
- ✅ TLS can be configured directly via Kubernetes secrets if needed

---

## Secrets Management

### **Database Credentials**

**Kubernetes Secret**:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: postgres-credentials
  namespace: kubernaut-system
type: Opaque
stringData:
  host: postgresql.kubernaut-system.svc.cluster.local
  port: "5432"
  database: kubernaut_audit
  username: data_storage_user
  password: <STRONG_PASSWORD_FROM_VAULT>
  sslmode: require
---
apiVersion: v1
kind: Secret
metadata:
  name: vectordb-credentials
  namespace: kubernaut-system
type: Opaque
stringData:
  host: vectordb.kubernaut-system.svc.cluster.local
  port: "5433"
  database: kubernaut_vectors
  username: data_storage_user
  password: <STRONG_PASSWORD_FROM_VAULT>
  sslmode: require
```

### **Loading Secrets in Go**

```go
package config

import (
    "context"
    "fmt"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes"
    "go.uber.org/zap"
)

func LoadDatabaseCredentials(ctx context.Context, client *kubernetes.Clientset, logger *zap.Logger) (*database.PostgresConfig, error) {
    secret, err := client.CoreV1().Secrets("kubernaut-system").Get(ctx, "postgres-credentials", metav1.GetOptions{})
    if err != nil {
        logger.Error("Failed to load postgres credentials", zap.Error(err))
        return nil, err
    }

    config := &database.PostgresConfig{
        Host:     string(secret.Data["host"]),
        Port:     5432, // Parse from secret.Data["port"]
        Database: string(secret.Data["database"]),
        User:     string(secret.Data["username"]),
        Password: string(secret.Data["password"]),
        SSLMode:  string(secret.Data["sslmode"]),
    }

    logger.Info("Database credentials loaded successfully")
    return config, nil
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
func (s *DataStorageService) HandleWriteRequest(ctx context.Context, req *WriteRequest) error {
    userInfo := ctx.Value("userInfo").(*authv1.UserInfo)

    LogSecurityEvent(s.logger, "write_request_received",
        zap.String("username", userInfo.Username),
        zap.String("request_id", req.ID),
        zap.String("audit_type", req.Type),
    )

    // ... process request ...

    LogSecurityEvent(s.logger, "write_request_completed",
        zap.String("username", userInfo.Username),
        zap.String("request_id", req.ID),
        zap.String("status", "success"),
    )

    return nil
}
```

### **Failed Authentication Logging**

```go
package auth

import (
    "context"
    "fmt"

    authv1 "k8s.io/api/authentication/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "go.uber.org/zap"
)

func (tr *TokenReviewer) ValidateToken(ctx context.Context, token string) (*authv1.UserInfo, error) {
    review := &authv1.TokenReview{
        Spec: authv1.TokenReviewSpec{
            Token: token,
        },
    }

    result, err := tr.client.AuthenticationV1().TokenReviews().Create(ctx, review, metav1.CreateOptions{})
    if err != nil {
        LogSecurityEvent(tr.logger, "token_review_error",
            zap.Error(err),
        )
        return nil, fmt.Errorf("token review failed: %w", err)
    }

    if !result.Status.Authenticated {
        LogSecurityEvent(tr.logger, "authentication_failed",
            zap.String("error", result.Status.Error),
        )
        return nil, fmt.Errorf("token not authenticated")
    }

    return &result.Status.User, nil
}
```

---

## Security Checklist

### **Pre-Deployment**

- [ ] TokenReviewer authentication implemented and tested
- [ ] RBAC roles and bindings created for service account
- [ ] Database credentials stored in Kubernetes secrets (not hardcoded)
- [ ] SSL/TLS enabled for all database connections (sslmode=require)
- [ ] Network policies restrict ingress to authorized services only
- [ ] NetworkPolicies configured for ingress/egress control
- [ ] All SQL queries use prepared statements (no string concatenation)
- [ ] Security event logging implemented for all authentication/authorization events
- [ ] Secrets rotation policy documented

### **Runtime Security**

- [ ] Monitor failed authentication attempts (alert if > 10/min)
- [ ] Monitor unauthorized access attempts (alert immediately)
- [ ] Audit logs reviewed regularly for suspicious activity
- [ ] Database connection strings never logged
- [ ] Sensitive data sanitized in logs (passwords, tokens, etc.)

### **Compliance**

- [ ] Audit trail persisted for all write operations
- [ ] Immutable audit records (no updates, only inserts)
- [ ] Data retention policy enforced (90-day default)
- [ ] GDPR compliance for sensitive data (if applicable)

---

## Reference Documentation

- **TokenReviewer Auth**: `docs/architecture/KUBERNETES_TOKENREVIEWER_AUTH.md`
- **Logging Standard**: `docs/architecture/LOGGING_STANDARD.md`
- **Database Schema**: `docs/services/stateless/data-storage/database-schema.md`
- **API Specification**: `docs/services/stateless/data-storage/api-specification.md`

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Security Review**: Pending

