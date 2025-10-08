# Context API Service - Integration Points

**Version**: 1.0
**Last Updated**: October 6, 2025
**Service Type**: Stateless HTTP API Service (Read-Only)

---

## 🔗 Upstream Clients (Services Calling Context API)

### **1. AI Analysis Service** (Port 8080)

**Use Case**: Historical context for remediation recommendations

```go
// internal/aianalysis/context_client.go
package aianalysis

import (
    "fmt"
    "net/http"
)

func (a *AIAnalysisService) GetHistoricalContext(alertName, namespace string) (*ContextData, error) {
    req, _ := http.NewRequest("GET",
        fmt.Sprintf("http://context-api-service:8080/api/v1/context/historical?alert=%s&namespace=%s", alertName, namespace),
        nil)
    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.getServiceAccountToken()))

    resp, err := http.DefaultClient.Do(req)
    // ... handle response
}
```

---

### **2. HolmesGPT API Service** (Port 8080)

**Use Case**: Dynamic context for AI investigations

```python
# holmesgpt-api/context_integration.py
from typing import Dict
import requests


def get_investigation_context(alert_id: str) -> Dict:
    headers = {"Authorization": f"Bearer {get_service_account_token()}"}
    response = requests.get(
        f"http://context-api-service:8080/api/v1/context/investigation/{alert_id}",
        headers=headers
    )
    return response.json()
```

---

### **3. Effectiveness Monitor Service** (Port 8087)

**Use Case**: Historical trends for effectiveness assessment

```go
package effectiveness

import (
    "fmt"
    "net/http"
)

func (e *EffectivenessMonitor) GetHistoricalTrends(actionType string) (*TrendData, error) {
    url := fmt.Sprintf("http://context-api-service:8080/api/v1/context/trends?actionType=%s&timeRange=90d", actionType)
    // ... call Context API
}
```

---

## 🔽 Downstream Dependencies (External Services)

### **1. PostgreSQL Database**

**Service**: PostgreSQL with pgvector extension
**Endpoint**: `postgresql-service:5432`
**Database**: `kubernaut`
**Authentication**: Username/password from Secret

**Queries**:
- Action history retrieval
- Success rate calculations
- Effectiveness data lookups
- Audit trail queries

---

### **2. Redis Cache**

**Service**: Redis
**Endpoint**: `redis-service:6379`
**Authentication**: Password from Secret

**Cache Keys**:
```
context:success_rate:restart-pod:production -> "0.87"
context:history:alert-abc123 -> JSON
context:similar:embedding-xyz -> JSON array
```

**TTL**: 5 minutes (configurable)

---

### **3. Vector Database** (pgvector)

**Service**: PostgreSQL with pgvector extension
**Endpoint**: `postgresql-service:5432`
**Purpose**: Semantic similarity search

**Queries**:
```sql
-- Find similar alerts
SELECT alert_id, embedding <-> $1 AS distance
FROM alert_embeddings
ORDER BY distance ASC
LIMIT 10;
```

---

## 📊 Data Flow Diagram

```
┌─────────────────────────────────────────────┐
│       Upstream Clients                      │
│  ┌────────────┐  ┌────────────┐            │
│  │ AI Analysis│  │ HolmesGPT  │            │
│  │  Service   │  │    API     │            │
│  └──────┬─────┘  └──────┬─────┘            │
│         │                │                   │
│         │  ┌─────────────┴──────┐           │
│         └──┤ Effectiveness      │           │
│            │ Monitor            │           │
│            └─────────┬──────────┘           │
└──────────────────────┼──────────────────────┘
                       │ HTTP GET (Bearer Token)
                       ▼
┌─────────────────────────────────────────────┐
│      Context API Service (Port 8080)        │
│  ┌───────────────────────────────────────┐ │
│  │ 1. Authentication (TokenReviewer)     │ │
│  │ 2. Cache Lookup (Redis)               │ │
│  │ 3. Database Query (PostgreSQL)        │ │
│  │ 4. Vector Search (pgvector)           │ │
│  │ 5. Response Formatting                │ │
│  └───────────────────────────────────────┘ │
└──────────────────┬────────────────┬─────────┘
                   │                │
                   ▼                ▼
┌──────────────────────────────────────────────┐
│       Downstream Dependencies                 │
│  ┌─────────┐  ┌────────┐  ┌──────────┐     │
│  │PostgreSQL│  │ Redis  │  │ pgvector │     │
│  │ (Action │  │(Cache) │  │(Similarity│     │
│  │ History)│  │        │  │  Search)  │     │
│  └─────────┘  └────────┘  └──────────┘     │
└──────────────────────────────────────────────┘
```

---

## 🔐 Configuration

### **ConfigMap**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: context-api-config
  namespace: prometheus-alerts-slm
data:
  # Database
  postgres.host: "postgresql-service"
  postgres.port: "5432"
  postgres.database: "kubernaut"

  # Redis
  redis.host: "redis-service"
  redis.port: "6379"
  redis.ttl: "300" # 5 minutes

  # Performance
  cache.enabled: "true"
  connection.pool.size: "50"
  query.timeout: "5s"
```

### **Secret**

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: context-api-secrets
  namespace: prometheus-alerts-slm
type: Opaque
stringData:
  postgres-password: "<redacted>"
  redis-password: "<redacted>"
```

---

## 🔄 Error Handling

### **Circuit Breaker Pattern**

```go
package context

import (
    "database/sql"
    "fmt"
)

type CircuitBreaker struct {
    maxFailures int
    state       string // "closed", "open", "half-open"
}

func (s *ContextAPIService) QueryWithCircuitBreaker(query string) (interface{}, error) {
    if s.circuitBreaker.IsOpen() {
        return nil, fmt.Errorf("circuit breaker open for database")
    }

    result, err := s.db.Query(query)
    if err != nil {
        s.circuitBreaker.RecordFailure()
        return nil, err
    }

    s.circuitBreaker.RecordSuccess()
    return result, nil
}
```

### **Fallback Strategy**

```go
package context

import (
    "time"
)

func (s *ContextAPIService) GetSuccessRate(actionType string) (float64, error) {
    // Try cache first
    if cached, ok := s.cache.Get(cacheKey); ok {
        return cached.(float64), nil
    }

    // Try database
    rate, err := s.db.GetSuccessRate(actionType)
    if err != nil {
        // Fallback to default
        return 0.5, nil // 50% default success rate
    }

    // Cache result
    s.cache.Set(cacheKey, rate, 5*time.Minute)
    return rate, nil
}
```

---

## 📊 Integration Metrics

```go
package context

import (
    "github.com/prometheus/client_golang/prometheus"
)

var (
    databaseQueries = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "context_api_db_queries_total",
            Help: "Total database queries",
        },
        []string{"status"}, // "success", "failure"
    )

    cacheHitRate = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "context_api_cache_hit_rate",
            Help: "Cache hit rate",
        },
        []string{"cache_type"}, // "redis", "memory"
    )

    dependencyHealth = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "context_api_dependency_health",
            Help: "Dependency health status (1=healthy, 0=unhealthy)",
        },
        []string{"dependency"}, // "postgresql", "redis"
    )
)
```

---

## ✅ Integration Checklist

### **Upstream Integration**
- [ ] AI Analysis Service calls Context API
- [ ] HolmesGPT API calls Context API
- [ ] Effectiveness Monitor calls Context API
- [ ] All clients use Bearer token authentication

### **Downstream Integration**
- [ ] PostgreSQL connection configured
- [ ] Redis caching operational
- [ ] Vector DB queries working
- [ ] Connection pooling configured

### **Configuration**
- [ ] ConfigMap deployed
- [ ] Secrets deployed
- [ ] RBAC roles created
- [ ] Network policies configured

### **Monitoring**
- [ ] Prometheus metrics exposed
- [ ] Grafana dashboards created
- [ ] Circuit breaker metrics tracked
- [ ] Cache hit rate monitored

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Status**: ✅ Complete Specification

