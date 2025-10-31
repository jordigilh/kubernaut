# Context API Service - Integration Points

**Version**: 1.1
**Last Updated**: October 31, 2025
**Service Type**: Stateless HTTP API Service (Read-Only)
**Architecture Decision**: [DD-CONTEXT-001](../../architecture/decisions/DD-CONTEXT-001-Context-Enrichment-Placement.md) - LLM-Driven Tool Call Pattern

---

## 🔗 Upstream Clients (Services Calling Context API)

### **1. RemediationProcessing Controller** (Port 8080) ✅ **PRIMARY CONSUMER (Alternative 2)**

**Use Case**: Historical context for workflow failure recovery analysis
**Business Requirement**: BR-WF-RECOVERY-011
**Integration Pattern**: Query Context API → Store in RemediationProcessing.status → Remediation Orchestrator creates AIAnalysis CRD with all contexts
**Design Reference**: [`PROPOSED_FAILURE_RECOVERY_SEQUENCE.md`](../../../architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md) (Alternative 2)

**Note**: Per [DD-CONTEXT-001](../../architecture/decisions/DD-CONTEXT-001-Context-Enrichment-Placement.md), AIAnalysis Controller does NOT call Context API directly. Context enrichment happens via HolmesGPT API tool calls (see Section 2 below).

#### Recovery Context Integration (Alternative 2)

When a workflow fails, Remediation Orchestrator creates a NEW RemediationProcessing CRD (recovery). The RemediationProcessing Controller enriches it with:
- **Fresh monitoring context** (current CPU/memory/pod status)
- **Fresh business context** (current ownership/runbooks)
- **Recovery context from Context API** (historical failures, patterns, strategies)

Then Remediation Orchestrator watches RemediationProcessing completion and creates AIAnalysis with ALL enriched contexts.

```go
// internal/remediationprocessing/context_client.go
package remediationprocessing

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"

    processingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"
)

type ContextAPIClient interface {
    GetRemediationContext(ctx context.Context, remediationRequestID string) (*ContextAPIResponse, error)
}

type ContextAPIClientImpl struct {
    BaseURL    string
    HTTPClient *http.Client
}

func (c *ContextAPIClientImpl) GetRemediationContext(
    ctx context.Context,
    remediationRequestID string,
) (*ContextAPIResponse, error) {

    url := fmt.Sprintf("%s/api/v1/context/remediation/%s", c.BaseURL, remediationRequestID)

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", getServiceAccountToken()))
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.HTTPClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("context API request failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("context API returned status %d", resp.StatusCode)
    }

    var contextResp ContextAPIResponse
    if err := json.NewDecoder(resp.Body).Decode(&contextResp); err != nil {
        return nil, fmt.Errorf("failed to decode response: %w", err)
    }

    return &contextResp, nil
}

// Usage during recovery enrichment (Alternative 2)
func (r *RemediationProcessingReconciler) reconcileEnriching(
    ctx context.Context,
    rp *processingv1.RemediationProcessing,
) (ctrl.Result, error) {

    // ALWAYS enrich monitoring + business context (gets FRESH data)
    enrichmentResults, err := r.ContextService.GetContext(ctx, rp.Spec.Alert)
    if err != nil {
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    rp.Status.EnrichmentResults = enrichmentResults

    // IF recovery attempt, ALSO query Context API for recovery context
    if rp.Spec.IsRecoveryAttempt {
        recoveryCtx, err := r.enrichRecoveryContext(ctx, rp)
        if err != nil {
            // Graceful degradation: use fallback context
            recoveryCtx = r.buildFallbackRecoveryContext(rp)
        }

        // Add to enrichment results
        rp.Status.EnrichmentResults.RecoveryContext = recoveryCtx
    }

    rp.Status.Phase = "classifying"
    return ctrl.Result{Requeue: true}, r.Status().Update(ctx, rp)
}

func (r *RemediationProcessingReconciler) enrichRecoveryContext(
    ctx context.Context,
    rp *processingv1.RemediationProcessing,
) (*processingv1.RecoveryContext, error) {

    // Query Context API
    contextResp, err := r.ContextAPIClient.GetRemediationContext(
        ctx,
        rp.Spec.RemediationRequestRef.Name,
    )

    if err != nil {
        return nil, fmt.Errorf("Context API query failed: %w", err)
    }

    // Convert and store in RemediationProcessing.status
    return convertToRecoveryContext(contextResp), nil
}
```

#### Complete Recovery Flow (Alternative 2)

```
1. Workflow fails
   ↓
2. Remediation Orchestrator:
   - Evaluates recovery viability
   - Creates NEW RemediationProcessing CRD (recovery)
   - Sets phase: "recovering"
   ↓
3. RemediationProcessing Controller:
   - Enriches monitoring context (FRESH!)
   - Enriches business context (FRESH!)
   - Queries Context API for recovery context (FRESH!) ← THIS FILE
   - Stores ALL in RemediationProcessing.status.enrichmentResults
   - Sets phase: "completed"
   ↓
4. Remediation Orchestrator:
   - Watches RemediationProcessing completion
   - Creates AIAnalysis with ALL contexts
   - Normal flow continues (analyzing → executing)
```

#### Key Endpoints Used

- **`GET /api/v1/context/remediation/{remediationRequestId}`** - Retrieve historical recovery context
  - Returns: Previous failures, related alerts, historical patterns, successful strategies
  - Response format: JSON with `contextQuality` indicator ("complete", "partial", "minimal")
  - Graceful degradation: If unavailable, RemediationProcessing Controller uses fallback data

#### Integration Benefits (Alternative 2)

✅ **Fresh Monitoring Context**: Recovery attempts see current cluster state (not 10min old)
✅ **Fresh Business Context**: Current ownership/runbooks (may change between attempts)
✅ **Fresh Recovery Context**: Latest historical data from Context API
✅ **Immutable Audit Trail**: Each RemediationProcessing CRD is separate and immutable
✅ **Consistent Enrichment**: ALL enrichment in one place (RemediationProcessing Controller)
✅ **Pattern Reuse**: Recovery follows same pattern as initial (watch → enrich → complete)

---

### **2. HolmesGPT API Service** (Port 8080) 🔵 **SECONDARY CONSUMER**

**Use Case**: Dynamic context for AI investigations via LLM tool calls
**Business Requirements**: BR-HAPI-046 to BR-HAPI-050
**Integration Pattern**: LLM-driven tool call (NOT direct HTTP from AIAnalysis Controller)
**Architecture Decision**: [DD-CONTEXT-001](../../architecture/decisions/DD-CONTEXT-001-Context-Enrichment-Placement.md) - Approach B (LLM-Driven Tool Call Pattern)

#### LLM-Driven Tool Call Pattern

**Flow**:
```
AIAnalysis Controller
    ↓ (send investigation request)
HolmesGPT API Service
    ↓ (LLM decides if context needed)
LLM (Claude/Vertex AI)
    ↓ (tool call: get_context)
HolmesGPT API → Context API Service
    ↓ (return context to LLM)
LLM continues investigation with context
```

**Key Benefits**:
- ✅ **Multiple Context Queries**: LLM can query historical success rates for MULTIPLE remediation actions (not just pre-selected one)
- ✅ **Comparative Analysis**: LLM compares success rates across different strategies (e.g., restart-pod 85%, scale-deployment 72%, rollback-deployment 91%)
- ✅ **Higher Confidence**: LLM increases confidence assessment by evaluating alternatives with real historical data
- ✅ **Adaptive Investigation**: LLM can request additional context mid-investigation if initial approach seems insufficient
- ✅ **Cost Optimization**: 36% token cost reduction ($910/year) - context only when LLM requests it
- ✅ **LLM Autonomy**: LLM decides when context is needed, not forced pre-enrichment

**Trade-off**: +500ms-1s latency for tool call round-trip (acceptable for cost savings and improved decision quality)

#### Tool Call Implementation

**Tool Definition** (from BR-HAPI-046):
```python
{
    "name": "get_context",
    "description": "Retrieve historical context for similar incidents. Use when investigation requires understanding of past similar alerts, success rates, or patterns.",
    "parameters": {
        "type": "object",
        "properties": {
            "alert_fingerprint": {
                "type": "string",
                "description": "Fingerprint of the current alert"
            },
            "similarity_threshold": {
                "type": "number",
                "description": "Minimum similarity score (0.0-1.0), default 0.70"
            },
            "context_types": {
                "type": "array",
                "items": {"enum": ["historical_remediations", "cluster_patterns", "success_rates"]},
                "description": "Types of context to retrieve"
            }
        },
        "required": ["alert_fingerprint"]
    }
}
```

**Tool Call Handler** (HolmesGPT API):
```python
# holmesgpt-api/tools/context_tool.py
from typing import Dict
import requests


class ContextTool:
    def __init__(self, context_api_url: str):
        self.context_api_url = context_api_url
        self.client = ContextAPIClient(context_api_url)

    def handle_tool_call(self, investigation_id: str, parameters: Dict[str, Any]) -> Dict[str, Any]:
        """Handle LLM tool call request"""
        try:
            # LLM can make MULTIPLE calls to compare remediation strategies
            alert_fingerprint = parameters.get("alert_fingerprint")
            similarity_threshold = parameters.get("similarity_threshold", 0.70)
            context_types = parameters.get("context_types")

            # Invoke Context API
            context = self.client.get_context(
                alert_fingerprint=alert_fingerprint,
                similarity_threshold=similarity_threshold,
                context_types=context_types
            )

            # Format response for LLM (ultra-compact JSON)
            return self._format_response(context)

        except Exception as e:
            # Graceful degradation
            return {"error": str(e), "fallback": "continue_without_context"}
```

**Example: LLM Making Multiple Context Queries**:
```python
# Investigation 1: LLM queries context for "restart-pod"
tool_call_1 = {
    "name": "get_context",
    "parameters": {
        "alert_fingerprint": "mem-spike-api-server-abc123",
        "context_types": ["success_rates"],
        "action_type": "restart-pod"  # LLM specifies action
    }
}
# Response: {"success_rate": 0.85, "total_executions": 120}

# Investigation 2: LLM queries context for "scale-deployment"
tool_call_2 = {
    "name": "get_context",
    "parameters": {
        "alert_fingerprint": "mem-spike-api-server-abc123",
        "context_types": ["success_rates"],
        "action_type": "scale-deployment"  # LLM tries alternative
    }
}
# Response: {"success_rate": 0.72, "total_executions": 45}

# Investigation 3: LLM queries context for "rollback-deployment"
tool_call_3 = {
    "name": "get_context",
    "parameters": {
        "alert_fingerprint": "mem-spike-api-server-abc123",
        "context_types": ["success_rates"],
        "action_type": "rollback-deployment"  # LLM tries another alternative
    }
}
# Response: {"success_rate": 0.91, "total_executions": 78}

# Result: LLM chooses "rollback-deployment" with 91% confidence based on comparative historical data
```

**Related Documentation**:
- [DD-CONTEXT-001](../../architecture/decisions/DD-CONTEXT-001-Context-Enrichment-Placement.md) - Architectural decision
- [BR-HAPI-046 to BR-HAPI-050](../holmesgpt-api/BR-HAPI-046-050-CONTEXT-API-TOOL.md) - HolmesGPT API tool integration

---

### **3. Effectiveness Monitor Service** (Port 8080)

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

## 🏗️ Architecture Clarification: AIAnalysis Integration

**Question**: Why doesn't AIAnalysis Controller call Context API directly?

**Answer**: Per [DD-CONTEXT-001](../../architecture/decisions/DD-CONTEXT-001-Context-Enrichment-Placement.md), context enrichment uses an **LLM-driven tool call pattern** instead of pre-enrichment.

### Pattern Comparison

#### ❌ Approach A: Pre-Enrichment (NOT USED)
```
AIAnalysis Controller
    ↓ (fetch context)
Context API Service
    ↓ (return enriched context)
AIAnalysis Controller
    ↓ (send investigation request with context)
HolmesGPT API Service
    ↓ (investigate with provided context)
LLM (Claude/Vertex AI)
```

**Problems**:
- ❌ Forces context even when not needed
- ❌ Higher token cost (context in every request)
- ❌ Wastes LLM intelligence (pre-decides what LLM needs)
- ❌ Inflexible (cannot adapt to different investigation types)
- ❌ **Single context query** (only one remediation action evaluated)

---

#### ✅ Approach B: LLM-Driven Tool Call (APPROVED)
```
AIAnalysis Controller
    ↓ (send investigation request)
HolmesGPT API Service
    ↓ (LLM decides if context needed)
LLM (Claude/Vertex AI)
    ↓ (tool call: get_context)
HolmesGPT API → Context API Service
    ↓ (return context to LLM)
LLM continues investigation with context
```

**Benefits**:
- ✅ **36% token cost reduction** ($910/year savings)
- ✅ **LLM autonomy** - LLM decides when context is needed
- ✅ **Flexible** - LLM can request different context types
- ✅ **Aligned with HolmesGPT SDK** - Native tool call pattern
- ✅ **Multiple context queries** - LLM can compare multiple remediation strategies
- ✅ **Higher confidence** - LLM evaluates alternatives with real historical data

**Trade-off**: +500ms-1s latency for tool call round-trip (acceptable for cost savings and improved decision quality)

---

### Example: Multiple Context Queries for Comparative Analysis

**Scenario**: Memory spike in API server

**Approach A (Pre-Enrichment)**: AIAnalysis Controller pre-fetches context for "restart-pod" (pre-selected action) and sends to HolmesGPT API
- **Result**: LLM receives only "restart-pod has 85% success rate"
- **Problem**: LLM cannot evaluate alternatives, stuck with pre-selected action

**Approach B (Tool Call)**: LLM makes multiple tool calls to compare strategies
1. **Tool Call 1**: Query "restart-pod" → 85% success rate (120 executions)
2. **Tool Call 2**: Query "scale-deployment" → 72% success rate (45 executions)
3. **Tool Call 3**: Query "rollback-deployment" → 91% success rate (78 executions)
- **Result**: LLM chooses "rollback-deployment" with 91% confidence
- **Benefit**: Data-driven decision based on comparative historical analysis

---

### Integration Summary

| Service | Calls Context API? | Pattern | Rationale |
|---|---|---|---|
| **RemediationProcessing Controller** | ✅ **YES** (direct HTTP) | Recovery context enrichment | Workflow failure recovery (BR-WF-RECOVERY-011) |
| **HolmesGPT API** | ✅ **YES** (LLM tool calls) | LLM-driven on-demand queries | AI investigation context (BR-HAPI-046 to BR-HAPI-050) |
| **AIAnalysis Controller** | ❌ **NO** | Sends investigation request to HolmesGPT API | Per DD-CONTEXT-001, context enrichment via HolmesGPT API tool calls |
| **Effectiveness Monitor** | ✅ **YES** (direct HTTP) | Historical trend analytics | Effectiveness assessment (BR-MONITORING-002) |

---

### Related Documents

- [DD-CONTEXT-001](../../architecture/decisions/DD-CONTEXT-001-Context-Enrichment-Placement.md) - Architectural decision
- [BR-HAPI-046 to BR-HAPI-050](../holmesgpt-api/BR-HAPI-046-050-CONTEXT-API-TOOL.md) - HolmesGPT API tool integration
- [PROPOSED_FAILURE_RECOVERY_SEQUENCE.md](../../architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md) - Recovery context flow

---

## 📊 Data Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Upstream Clients                                 │
│                                                                     │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐ │
│  │ RemediationProc  │  │ HolmesGPT API    │  │ Effectiveness    │ │
│  │ Controller       │  │ (LLM Tool Calls) │  │ Monitor          │ │
│  └────────┬─────────┘  └────────┬─────────┘  └────────┬─────────┘ │
│           │                     │                     │             │
│           │ (PRIMARY)           │ (SECONDARY)         │ (TERTIARY)  │
│           │ Direct HTTP         │ LLM Tool Call       │ Direct HTTP │
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
  namespace: kubernaut-system
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
  namespace: kubernaut-system
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

