# Health Check Standard - Kubernaut Services

**Version**: v1.0
**Last Updated**: October 6, 2025
**Status**: ✅ **STANDARDIZED**
**Scope**: All 11 Services (6 HTTP + 5 CRD Controllers)

---

## 📋 **Standard Health Check Endpoints**

### **All Services**

| Endpoint | Purpose | Authentication | Port |
|----------|---------|----------------|------|
| `/healthz` | **Liveness** probe | ❌ None | 8080 |
| `/readyz` | **Readiness** probe | ❌ None | 8080 |

---

## 🎯 **Health vs Readiness**

### **/healthz (Liveness)**

**Purpose**: Is the service process healthy?
**Criteria**: Service process is running and not deadlocked
**Failure Action**: Kubernetes restarts the pod

**Check Logic**:
```go
func healthzHandler(w http.ResponseWriter, r *http.Request) {
    // Simple: Is the process responsive?
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{
        "status": "healthy",
    })
}
```

**Response** (200 OK):
```json
{
  "status": "healthy",
  "timestamp": "2025-10-06T10:15:30Z"
}
```

---

### **/readyz (Readiness)**

**Purpose**: Is the service ready to accept traffic?
**Criteria**: Service is healthy AND all dependencies are available
**Failure Action**: Kubernetes removes pod from load balancer

**Check Logic**:
```go
func readyzHandler(w http.ResponseWriter, r *http.Request) {
    // Complex: Check all dependencies
    status := checkReadiness()

    if status.Ready {
        w.WriteHeader(http.StatusOK)
    } else {
        w.WriteHeader(http.StatusServiceUnavailable)
    }

    json.NewEncoder(w).Encode(status)
}
```

**Response** (200 OK - Ready):
```json
{
  "status": "ready",
  "timestamp": "2025-10-06T10:15:30Z",
  "dependencies": {
    "postgresql": {
      "status": "healthy",
      "latency": "2ms"
    },
    "redis": {
      "status": "healthy",
      "latency": "1ms"
    },
    "vectordb": {
      "status": "healthy",
      "latency": "3ms"
    }
  }
}
```

**Response** (503 Service Unavailable - Not Ready):
```json
{
  "status": "not_ready",
  "timestamp": "2025-10-06T10:15:30Z",
  "dependencies": {
    "postgresql": {
      "status": "healthy",
      "latency": "2ms"
    },
    "redis": {
      "status": "unhealthy",
      "error": "connection timeout",
      "latency": "5000ms"
    },
    "vectordb": {
      "status": "degraded",
      "error": "slow query performance",
      "latency": "150ms"
    }
  },
  "reason": "redis unhealthy"
}
```

---

## 🔧 **Go Implementation**

### **HTTP Services** (Gateway, Context API, Data Storage, etc.)

```go
// pkg/health/health.go
package health

import (
    "context"
    "encoding/json"
    "net/http"
    "time"

    "go.uber.org/zap"
)

// Checker defines the interface for dependency health checks
type Checker interface {
    Check(ctx context.Context) error
    Name() string
}

// HealthHandler provides health check endpoints
type HealthHandler struct {
    checkers []Checker
    logger   *zap.Logger
}

func NewHealthHandler(checkers []Checker, logger *zap.Logger) *HealthHandler {
    return &HealthHandler{
        checkers: checkers,
        logger:   logger,
    }
}

// Healthz handles liveness probe
func (h *HealthHandler) Healthz(w http.ResponseWriter, r *http.Request) {
    response := map[string]interface{}{
        "status":    "healthy",
        "timestamp": time.Now().UTC().Format(time.RFC3339),
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(response)
}

// Readyz handles readiness probe
func (h *HealthHandler) Readyz(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()

    dependencies := make(map[string]interface{})
    allHealthy := true
    var notReadyReason string

    for _, checker := range h.checkers {
        start := time.Now()
        err := checker.Check(ctx)
        latency := time.Since(start)

        depStatus := map[string]interface{}{
            "latency": latency.String(),
        }

        if err != nil {
            allHealthy = false
            depStatus["status"] = "unhealthy"
            depStatus["error"] = err.Error()
            notReadyReason = checker.Name() + " unhealthy"

            h.logger.Warn("Dependency unhealthy",
                zap.String("dependency", checker.Name()),
                zap.Error(err),
                zap.Duration("latency", latency),
            )
        } else if latency > 100*time.Millisecond {
            depStatus["status"] = "degraded"
            depStatus["warning"] = "high latency"

            h.logger.Warn("Dependency degraded",
                zap.String("dependency", checker.Name()),
                zap.Duration("latency", latency),
            )
        } else {
            depStatus["status"] = "healthy"
        }

        dependencies[checker.Name()] = depStatus
    }

    response := map[string]interface{}{
        "timestamp":    time.Now().UTC().Format(time.RFC3339),
        "dependencies": dependencies,
    }

    if allHealthy {
        response["status"] = "ready"
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
    } else {
        response["status"] = "not_ready"
        response["reason"] = notReadyReason
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusServiceUnavailable)
    }

    json.NewEncoder(w).Encode(response)
}
```

---

### **Dependency Checkers**

#### **PostgreSQL Checker**

```go
// pkg/health/checkers/postgres.go
package checkers

import (
    "context"
    "database/sql"
)

type PostgreSQLChecker struct {
    db *sql.DB
}

func NewPostgreSQLChecker(db *sql.DB) *PostgreSQLChecker {
    return &PostgreSQLChecker{db: db}
}

func (c *PostgreSQLChecker) Name() string {
    return "postgresql"
}

func (c *PostgreSQLChecker) Check(ctx context.Context) error {
    return c.db.PingContext(ctx)
}
```

---

#### **Redis Checker**

```go
// pkg/health/checkers/redis.go
package checkers

import (
    "context"

    "github.com/go-redis/redis/v8"
)

type RedisChecker struct {
    client *redis.Client
}

func NewRedisChecker(client *redis.Client) *RedisChecker {
    return &RedisChecker{client: client}
}

func (c *RedisChecker) Name() string {
    return "redis"
}

func (c *RedisChecker) Check(ctx context.Context) error {
    return c.client.Ping(ctx).Err()
}
```

---

#### **Kubernetes API Checker**

```go
// pkg/health/checkers/kubernetes.go
package checkers

import (
    "context"

    "k8s.io/client-go/kubernetes"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type KubernetesChecker struct {
    clientset *kubernetes.Clientset
}

func NewKubernetesChecker(clientset *kubernetes.Clientset) *KubernetesChecker {
    return &KubernetesChecker{clientset: clientset}
}

func (c *KubernetesChecker) Name() string {
    return "kubernetes_api"
}

func (c *KubernetesChecker) Check(ctx context.Context) error {
    _, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
    return err
}
```

---

### **CRD Controllers** (controller-runtime)

```go
// cmd/remediation-orchestrator/main.go
package main

import (
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/healthz"
)

func main() {
    mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
        HealthProbeBindAddress: ":8080",  // Port 8080 for health checks
        // ...
    })

    // Add health checks
    if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
        setupLog.Error(err, "unable to set up health check")
        os.Exit(1)
    }

    if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
        setupLog.Error(err, "unable to set up ready check")
        os.Exit(1)
    }

    // Start manager
    if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
        setupLog.Error(err, "problem running manager")
        os.Exit(1)
    }
}
```

---

## 📊 **Dependency Health Checks by Service**

### **Gateway Service**

**Dependencies**:
- ✅ Redis (deduplication, storm detection)
- ✅ Kubernetes API (CRD creation)
- ✅ Rego engine (policy evaluation)

**Readiness Logic**:
```go
checkers := []health.Checker{
    checkers.NewRedisChecker(redisClient),
    checkers.NewKubernetesChecker(k8sClientset),
    checkers.NewRegoChecker(regoEngine),
}
```

**Degraded State**: ❌ Not supported (all dependencies mandatory)

---

### **Context API Service**

**Dependencies**:
- ✅ PostgreSQL (audit trail queries)
- ✅ Redis (query caching)
- ✅ Vector DB (pgvector - semantic search)

**Readiness Logic**:
```go
checkers := []health.Checker{
    checkers.NewPostgreSQLChecker(pgDB),
    checkers.NewRedisChecker(redisClient),
    checkers.NewVectorDBChecker(vectorDB),  // Same as PostgreSQL with pgvector
}
```

**Degraded State**: ⚠️ Redis optional (reduced performance)
```go
if redisErr != nil && postgresqlOK && vectorDBOK {
    response["status"] = "degraded"
    response["reason"] = "redis cache unavailable, queries slower"
    w.WriteHeader(http.StatusOK)  // Still ready, just degraded
}
```

---

### **Data Storage Service**

**Dependencies**:
- ✅ PostgreSQL (audit writes)
- ✅ Redis (embedding cache)
- ✅ LLM API (embedding generation)

**Readiness Logic**:
```go
checkers := []health.Checker{
    checkers.NewPostgreSQLChecker(pgDB),
    checkers.NewRedisChecker(redisClient),
    checkers.NewHTTPChecker("llm_api", llmEndpoint+"/health"),
}
```

**Degraded State**: ⚠️ Redis optional (reduced performance)

---

### **HolmesGPT API Service**

**Dependencies**:
- ✅ LLM Provider (OpenAI, Claude, etc.)
- ✅ Dynamic Toolset ConfigMap (toolset configuration)

**Readiness Logic**:
```go
checkers := []health.Checker{
    checkers.NewHTTPChecker("llm_provider", llmEndpoint+"/health"),
    checkers.NewFileChecker("toolset_config", "/etc/kubernaut/toolsets/kubernetes-toolset.yaml"),
}
```

**Degraded State**: ❌ Not supported (LLM mandatory)

---

### **Notification Service**

**Dependencies**:
- ✅ External channels (Slack, Teams, Email, etc.) - **optional per channel**

**Readiness Logic**:
```go
// All channels are optional - service is always ready
// Individual channel health tracked separately
checkers := []health.Checker{
    checkers.NewHTTPChecker("slack_api", "https://slack.com/api/api.test", true),  // Optional
    checkers.NewHTTPChecker("teams_api", "https://graph.microsoft.com/v1.0/me", true),  // Optional
}
```

**Degraded State**: ✅ Supported (channels independently available)

---

### **Dynamic Toolset Service**

**Dependencies**:
- ✅ Kubernetes API (service discovery, ConfigMap management)

**Readiness Logic**:
```go
checkers := []health.Checker{
    checkers.NewKubernetesChecker(k8sClientset),
}
```

**Degraded State**: ❌ Not supported (Kubernetes API mandatory)

---

## 🎯 **Deployment Configuration**

### **HTTP Service Deployment**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: context-api
  namespace: kubernaut-system
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: context-api
        image: kubernaut/context-api:v1
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: metrics

        # Liveness probe - restart if unhealthy
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3

        # Readiness probe - remove from load balancer if not ready
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 5
          timeoutSeconds: 5
          failureThreshold: 2
```

---

### **CRD Controller Deployment**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: remediation-orchestrator
  namespace: kubernaut-system
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: manager
        image: kubernaut/remediation-orchestrator:v1
        ports:
        - containerPort: 8080
          name: health
        - containerPort: 9090
          name: metrics

        args:
        - --health-probe-bind-address=:8080
        - --metrics-bind-address=:9090
        - --leader-elect

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
```

---

## 📊 **Health Check Metrics**

```
# Health check results
{service}_health_check_total{dependency="postgresql",status="success"} 100
{service}_health_check_total{dependency="redis",status="failure"} 2

# Health check latency
{service}_health_check_duration_seconds{dependency="postgresql"} 0.002

# Service readiness
{service}_ready{} 1  # 1 = ready, 0 = not ready
```

---

## ✅ **Implementation Checklist**

### **For Each Service**:

1. ✅ **Liveness probe** (/healthz) - process health only
2. ✅ **Readiness probe** (/readyz) - dependency health
3. ✅ **Dependency checkers** for each critical dependency
4. ✅ **Timeout handling** (5s max for readiness checks)
5. ✅ **Structured responses** with dependency status
6. ✅ **Health metrics** for monitoring
7. ✅ **Deployment YAML** with probe configuration

---

## 🎯 **Testing Health Checks**

### **Test Script**

```bash
#!/bin/bash
# test-health-checks.sh

SERVICE_URL="http://context-api:8080"

# Test 1: Liveness (should always be 200)
curl -s "$SERVICE_URL/healthz" | jq

# Test 2: Readiness (may be 503 if dependencies down)
curl -s -w "\nHTTP Status: %{http_code}\n" "$SERVICE_URL/readyz" | jq

# Test 3: Simulate dependency failure
# (Stop PostgreSQL and check readiness)
kubectl scale deployment postgresql --replicas=0 -n kubernaut-system
sleep 10
curl -s -w "\nHTTP Status: %{http_code}\n" "$SERVICE_URL/readyz" | jq

# Expected: 503 Service Unavailable with postgresql: unhealthy
```

---

## 📚 **Related Documentation**

- [STATELESS_SERVICES_PORT_STANDARD.md](./STATELESS_SERVICES_PORT_STANDARD.md) - Health check ports
- [ERROR_RESPONSE_STANDARD.md](./ERROR_RESPONSE_STANDARD.md) - 503 error format
- [SERVICE_DEPENDENCY_MAP.md](./SERVICE_DEPENDENCY_MAP.md) - Service dependencies

---

**Document Status**: ✅ Complete
**Compliance**: 11/11 services covered
**Last Updated**: October 6, 2025
**Version**: 1.0
