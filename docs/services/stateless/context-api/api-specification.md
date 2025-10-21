# Context API - REST API Specification

**Version**: v2.0
**Last Updated**: October 20, 2025
**Base URL**: `http://context-api.kubernaut-system.svc.cluster.local:8091`
**Authentication**: Bearer Token (Kubernetes ServiceAccount)
**Namespace**: kubernaut-system

---

## Table of Contents

1. [API Overview](#api-overview)
2. [Authentication](#authentication)
3. [Environment Context API](#environment-context-api)
4. [Historical Query API](#historical-query-api)
5. [Recovery Context API](#recovery-context-api) ← **NEW (BR-WF-RECOVERY-011)**
6. [Success Rate API](#success-rate-api)
7. [Pattern Matching API](#pattern-matching-api)
8. [Health & Metrics](#health--metrics)
9. [Error Responses](#error-responses)

---

## API Overview

### Base URL
```
http://context-api.kubernaut-system.svc.cluster.local:8091
```

**Service Details**:
- **Port**: 8091 (HTTP API)
- **Metrics Port**: 9090 (Prometheus)
- **Namespace**: kubernaut-system

### API Version
All endpoints are prefixed with `/api/v1/`

### Content Type
```
Content-Type: application/json
```

### Rate Limiting
- **Per Service**: 1000 requests/second
- **Burst**: 1500 requests
- **Response Header**: `X-RateLimit-Remaining: 999`

### Structured Action Format Support (Data Provider Role)

**Business Requirements**: BR-LLM-021 to BR-LLM-026
**Context API Role**: Read-only data provider (NO LLM integration)

Context API provides enriched historical context that **HolmesGPT API consumes** to support structured action generation by its LLM. Context API is a **stateless HTTP REST service** that queries historical data and serves it to multiple clients.

**Client Integration**:
- **PRIMARY**: RemediationProcessing Controller (workflow recovery context)
- **SECONDARY**: HolmesGPT API Service (AI investigation context)
- **TERTIARY**: Effectiveness Monitor Service (historical trend analytics)

**Data Provided** (read-only queries of remediation_audit table):
- Historical action success rates by action type
- Environment-specific constraints and policies
- Previous remediation patterns and outcomes
- Resource health indicators and thresholds

**Note**: Context API ONLY provides data. HolmesGPT API (the client) uses this data with its LLM to generate action recommendations.

---

## Authentication

### Bearer Token Authentication

All API requests require a valid Kubernetes ServiceAccount token:

```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://context-api.kubernaut-system:8080/api/v1/context
```

### Token Validation

Context API uses **Kubernetes TokenReviewer API** to validate tokens.

**See**: [Kubernetes TokenReviewer Authentication](../../../../architecture/KUBERNETES_TOKENREVIEWER_AUTH.md)

---

## Environment Context API

### Get Environment Context

**Purpose**: Retrieve environment metadata and statistics for a target resource

#### Request

```
GET /api/v1/context
```

#### Query Parameters

| Parameter | Type | Required | Description | Example |
|-----------|------|----------|-------------|---------|
| `namespace` | string | Yes | Kubernetes namespace | `production` |
| `targetType` | string | Yes | Resource type | `deployment` |
| `targetName` | string | Yes | Resource name | `api-server` |
| `environment` | string | No | Filter by environment | `production` |
| `timeRange` | string | No | Time range for stats | `7d`, `30d`, `90d` |

#### Example Request

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "http://context-api:8080/api/v1/context?namespace=production&targetType=deployment&targetName=api-server&timeRange=7d"
```

#### Response (200 OK)

```json
{
  "namespace": "production",
  "targetType": "deployment",
  "targetName": "api-server",
  "environment": "production",
  "timeRange": "7d",
  "metadata": {
    "labels": {
      "app": "api-server",
      "tier": "backend",
      "environment": "production"
    },
    "annotations": {
      "kubernaut.io/priority": "P0",
      "kubernaut.io/criticality": "high"
    }
  },
  "statistics": {
    "totalRemediations": 15,
    "successfulRemediations": 13,
    "failedRemediations": 2,
    "successRate": 0.867,
    "avgRemediationDurationSeconds": 42.5,
    "priorityDistribution": {
      "P0": 5,
      "P1": 8,
      "P2": 2
    }
  },
  "commonFailures": [
    {
      "reason": "ImagePullBackOff",
      "count": 3,
      "lastOccurrence": "2025-10-05T14:30:00Z"
    },
    {
      "reason": "OOMKilled",
      "count": 2,
      "lastOccurrence": "2025-10-04T09:15:00Z"
    }
  ],
  "recentRemediations": [
    {
      "id": "remediation-abc123",
      "createdAt": "2025-10-05T14:30:00Z",
      "phase": "Completed",
      "workflow": "scale-up",
      "durationSeconds": 38
    }
  ],
  "cachedAt": "2025-10-06T10:15:30Z",
  "cacheExpiresAt": "2025-10-06T10:20:30Z"
}
```

#### Go Implementation

```go
// pkg/contextapi/handlers/environment_context.go
package handlers

import (
    "encoding/json"
    "net/http"
    "time"

    "github.com/gorilla/mux"
    "go.uber.org/zap"

    "github.com/jordigilh/kubernaut/pkg/contextapi/models"
    "github.com/jordigilh/kubernaut/pkg/contextapi/services"
    "github.com/jordigilh/kubernaut/pkg/correlation"
)

type EnvironmentContextHandler struct {
    contextService *services.ContextService
    logger         *zap.Logger
}

func (h *EnvironmentContextHandler) GetEnvironmentContext(w http.ResponseWriter, r *http.Request) {
    correlationID := correlation.FromContext(r.Context())
    log := h.logger.With(
        zap.String("correlation_id", correlationID),
        zap.String("handler", "GetEnvironmentContext"),
    )

    // Parse query parameters
    req := &models.ContextQueryRequest{
        Namespace:   r.URL.Query().Get("namespace"),
        TargetType:  r.URL.Query().Get("targetType"),
        TargetName:  r.URL.Query().Get("targetName"),
        Environment: r.URL.Query().Get("environment"),
        TimeRange:   r.URL.Query().Get("timeRange"),
    }

    // Validate required fields
    if err := req.Validate(); err != nil {
        log.Warn("Invalid request", zap.Error(err))
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Default time range to 7 days
    if req.TimeRange == "" {
        req.TimeRange = "7d"
    }

    log.Info("Fetching environment context",
        zap.String("namespace", req.Namespace),
        zap.String("targetType", req.TargetType),
        zap.String("targetName", req.TargetName),
    )

    // Fetch context from service
    ctx, err := h.contextService.GetEnvironmentContext(r.Context(), req)
    if err != nil {
        log.Error("Failed to fetch environment context", zap.Error(err))
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    // Return JSON response
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("X-Correlation-ID", correlationID)
    json.NewEncoder(w).Encode(ctx)

    log.Info("Environment context returned",
        zap.Int("total_remediations", ctx.Statistics.TotalRemediations),
        zap.Float64("success_rate", ctx.Statistics.SuccessRate),
    )
}
```

---

## Historical Query API

### Get Historical Patterns

**Purpose**: Retrieve historical remediation patterns for similar targets

#### Request

```
GET /api/v1/context/historical
```

#### Query Parameters

| Parameter | Type | Required | Description | Example |
|-----------|------|----------|-------------|---------|
| `namespace` | string | Yes | Kubernetes namespace | `production` |
| `targetType` | string | Yes | Resource type | `deployment` |
| `targetName` | string | No | Specific resource name | `api-server` |
| `signalType` | string | No | Filter by signal type | `prometheus` |
| `timeRange` | string | No | Time range | `30d`, `90d` |
| `limit` | int | No | Max results | `20` (default: 10) |
| `includeEmbeddings` | bool | No | Include vector search | `true` |

#### Example Request

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "http://context-api:8080/api/v1/context/historical?namespace=production&targetType=deployment&timeRange=30d&includeEmbeddings=true&limit=10"
```

#### Response (200 OK)

```json
{
  "namespace": "production",
  "targetType": "deployment",
  "timeRange": "30d",
  "includeEmbeddings": true,
  "totalIncidents": 45,
  "patterns": [
    {
      "pattern": "high-memory-usage",
      "occurrences": 12,
      "commonRootCause": "Memory leak in cache layer",
      "successfulRemediations": 10,
      "commonWorkflow": "increase-memory-limit",
      "avgDurationSeconds": 35
    },
    {
      "pattern": "pod-crash-loop",
      "occurrences": 8,
      "commonRootCause": "Configuration error",
      "successfulRemediations": 7,
      "commonWorkflow": "rollback-deployment",
      "avgDurationSeconds": 52
    }
  ],
  "similarIncidents": [
    {
      "id": "remediation-def456",
      "namespace": "production",
      "targetName": "payment-api",
      "createdAt": "2025-09-20T10:30:00Z",
      "rootCause": "Memory leak in cache",
      "workflow": "increase-memory-limit",
      "phase": "Completed",
      "durationSeconds": 38,
      "similarityScore": 0.92
    }
  ],
  "cachedAt": "2025-10-06T10:15:30Z"
}
```

#### Go Implementation

```go
// pkg/contextapi/handlers/historical_query.go
package handlers

import (
    "encoding/json"
    "net/http"
    "strconv"

    "go.uber.org/zap"

    "github.com/jordigilh/kubernaut/pkg/contextapi/models"
    "github.com/jordigilh/kubernaut/pkg/contextapi/services"
    "github.com/jordigilh/kubernaut/pkg/correlation"
)

type HistoricalQueryHandler struct {
    contextService *services.ContextService
    logger         *zap.Logger
}

func (h *HistoricalQueryHandler) GetHistoricalPatterns(w http.ResponseWriter, r *http.Request) {
    correlationID := correlation.FromContext(r.Context())
    log := h.logger.With(
        zap.String("correlation_id", correlationID),
        zap.String("handler", "GetHistoricalPatterns"),
    )

    // Parse query parameters
    req := &models.HistoricalQueryRequest{
        Namespace:         r.URL.Query().Get("namespace"),
        TargetType:        r.URL.Query().Get("targetType"),
        TargetName:        r.URL.Query().Get("targetName"),
        SignalType:        r.URL.Query().Get("signalType"),
        TimeRange:         r.URL.Query().Get("timeRange"),
        IncludeEmbeddings: r.URL.Query().Get("includeEmbeddings") == "true",
    }

    // Parse limit
    if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
        if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
            req.Limit = limit
        }
    }

    // Validate required fields
    if err := req.Validate(); err != nil {
        log.Warn("Invalid request", zap.Error(err))
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Defaults
    if req.TimeRange == "" {
        req.TimeRange = "30d"
    }
    if req.Limit == 0 {
        req.Limit = 10
    }

    log.Info("Fetching historical patterns",
        zap.String("namespace", req.Namespace),
        zap.Bool("includeEmbeddings", req.IncludeEmbeddings),
    )

    // Fetch historical patterns
    patterns, err := h.contextService.GetHistoricalPatterns(r.Context(), req)
    if err != nil {
        log.Error("Failed to fetch historical patterns", zap.Error(err))
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    // Return JSON response
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("X-Correlation-ID", correlationID)
    json.NewEncoder(w).Encode(patterns)

    log.Info("Historical patterns returned",
        zap.Int("total_incidents", patterns.TotalIncidents),
        zap.Int("patterns", len(patterns.Patterns)),
    )
}
```

---

## Recovery Context API

> **📋 Design Decision: DD-001 - Alternative 2**
> **Consumer**: RemediationProcessing Controller (NOT Remediation Orchestrator)
> **Status**: ✅ Approved Design | **Confidence**: 95%
> **See**: [DD-001](../../../architecture/DESIGN_DECISIONS.md#dd-001-recovery-context-enrichment-alternative-2)

### Get Remediation Recovery Context

**Purpose**: Retrieve historical context for workflow failure recovery analysis
**Business Requirement**: BR-WF-RECOVERY-011
**Consumer**: RemediationProcessing Controller (DD-001: Alternative 2)
**Design Reference**: [`PROPOSED_FAILURE_RECOVERY_SEQUENCE.md`](../../../architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md) (Version 1.2)

#### Overview

When a workflow fails and Remediation Orchestrator creates a recovery RemediationProcessing CRD, the **RemediationProcessing Controller** queries this endpoint to retrieve historical context about previous failures, related alerts, historical patterns, and successful strategies. This context is stored in `RemediationProcessing.status.enrichmentResults.recoveryContext` (Alternative 2 pattern), then copied to AIAnalysis CRD spec by Remediation Orchestrator, enabling the AIAnalysis controller to generate alternative remediation strategies with complete temporal consistency.

#### Request

```http
GET /api/v1/context/remediation/{remediationRequestId}
Authorization: Bearer {service_account_token}
```

**Path Parameters**:
- `remediationRequestId` (string, required): RemediationRequest CRD name (e.g., "rr-2025-001")

**Example**:
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://context-api.kubernaut-system:8080/api/v1/context/remediation/rr-2025-001
```

#### Response (200 OK)

```json
{
  "remediationRequestId": "rr-2025-001",
  "currentAttempt": 2,
  "contextQuality": "complete",
  "previousFailures": [
    {
      "workflowRef": "workflow-001",
      "attemptNumber": 1,
      "failedStep": 3,
      "action": "scale-deployment",
      "errorType": "timeout",
      "failureReason": "Operation timed out after 5m",
      "duration": "5m3s",
      "clusterState": {
        "deployment_replicas": 2,
        "pod_status": "CrashLoopBackOff",
        "memory_usage": "89%"
      },
      "resourceSnapshot": {
        "deployment_name": "payment-api",
        "namespace": "production",
        "current_replicas": 2,
        "desired_replicas": 5
      },
      "timestamp": "2025-10-08T09:55:12Z"
    }
  ],
  "relatedAlerts": [
    {
      "alertFingerprint": "alert-fp-456",
      "alertName": "HighMemoryUsage",
      "correlation": 0.87,
      "timestamp": "2025-10-08T09:50:00Z"
    },
    {
      "alertFingerprint": "alert-fp-789",
      "alertName": "PodCrashLooping",
      "correlation": 0.92,
      "timestamp": "2025-10-08T09:52:30Z"
    }
  ],
  "historicalPatterns": [
    {
      "pattern": "scale_timeout_on_crashloop",
      "occurrences": 12,
      "successRate": 0.25,
      "averageRecoveryTime": "8m30s"
    },
    {
      "pattern": "memory_pressure_scaling_delay",
      "occurrences": 8,
      "successRate": 0.38,
      "averageRecoveryTime": "6m15s"
    }
  ],
  "successfulStrategies": [
    {
      "strategy": "restart_pods_before_scale",
      "description": "Restart stuck pods before attempting to scale deployment",
      "successCount": 15,
      "lastUsed": "2025-10-07T14:23:00Z",
      "confidence": 0.85
    },
    {
      "strategy": "incremental_scale_with_health_check",
      "description": "Scale up one replica at a time with health checks between",
      "successCount": 22,
      "lastUsed": "2025-10-08T08:10:00Z",
      "confidence": 0.91
    }
  ]
}
```

#### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `remediationRequestId` | string | RemediationRequest CRD name |
| `currentAttempt` | integer | Current recovery attempt number |
| `contextQuality` | string | Quality indicator: "complete", "partial", "minimal" |
| `previousFailures` | array | List of previous workflow failure details |
| `previousFailures[].workflowRef` | string | Failed WorkflowExecution CRD name |
| `previousFailures[].attemptNumber` | integer | Recovery attempt number (1, 2, 3) |
| `previousFailures[].failedStep` | integer | Step index that failed (0-based) |
| `previousFailures[].action` | string | Action type that failed (e.g., "scale-deployment") |
| `previousFailures[].errorType` | string | Classified error type ("timeout", "permission_denied", etc.) |
| `previousFailures[].failureReason` | string | Human-readable failure reason |
| `previousFailures[].duration` | string | How long the step ran before failing |
| `previousFailures[].clusterState` | object | Cluster state snapshot at failure time |
| `previousFailures[].resourceSnapshot` | object | Target resource state at failure time |
| `previousFailures[].timestamp` | string | ISO 8601 timestamp of failure |
| `relatedAlerts` | array | Alerts correlated with this remediation |
| `relatedAlerts[].alertFingerprint` | string | Unique alert identifier |
| `relatedAlerts[].alertName` | string | Alert name |
| `relatedAlerts[].correlation` | float | Correlation score (0.0-1.0) |
| `relatedAlerts[].timestamp` | string | Alert timestamp |
| `historicalPatterns` | array | Historical failure patterns for this alert type |
| `historicalPatterns[].pattern` | string | Pattern name |
| `historicalPatterns[].occurrences` | integer | How many times this pattern occurred |
| `historicalPatterns[].successRate` | float | Success rate for this pattern (0.0-1.0) |
| `historicalPatterns[].averageRecoveryTime` | string | Average time to recover |
| `successfulStrategies` | array | Successful recovery strategies for similar failures |
| `successfulStrategies[].strategy` | string | Strategy name |
| `successfulStrategies[].description` | string | Strategy description |
| `successfulStrategies[].successCount` | integer | Times this strategy succeeded |
| `successfulStrategies[].lastUsed` | string | Last time strategy was used |
| `successfulStrategies[].confidence` | float | Confidence score (0.0-1.0) |

#### Context Quality Levels

| Quality | Description | Data Completeness |
|---------|-------------|-------------------|
| `complete` | Full historical context available | All fields populated |
| `partial` | Some historical data available | previousFailures or relatedAlerts missing |
| `minimal` | Limited context available | Only basic failure info available |
| `degraded` | Context API failed, fallback data | Built from WorkflowExecutionRefs |

#### Error Responses

**404 Not Found** - RemediationRequest not found:
```json
{
  "error": "RemediationRequest not found",
  "remediationRequestId": "rr-2025-001",
  "message": "No remediation request found with ID rr-2025-001"
}
```

**503 Service Unavailable** - Context API temporarily unavailable:
```json
{
  "error": "Context API temporarily unavailable",
  "message": "Vector database connection failed",
  "fallback": "Use Remediation Orchestrator fallback context"
}
```

#### Graceful Degradation (BR-WF-RECOVERY-011)

**Critical Requirement**: If Context API is unavailable, Remediation Orchestrator MUST proceed with fallback context (don't fail).

**Fallback Strategy** (implemented in Remediation Orchestrator):
1. Extract previous failures from `RemediationRequest.status.workflowExecutionRefs`
2. Create minimal `HistoricalContext` with `contextQuality: "degraded"`
3. Embed fallback context in AIAnalysis CRD spec
4. Log warning and emit event
5. Continue recovery flow

**Example Fallback Context**:
```json
{
  "contextQuality": "degraded",
  "previousFailures": [
    {
      "workflowRef": "workflow-001",
      "attemptNumber": 1,
      "failedStep": 3,
      "failureReason": "timeout",
      "timestamp": "2025-10-08T09:55:12Z"
    }
  ],
  "relatedAlerts": [],
  "historicalPatterns": [],
  "successfulStrategies": []
}
```

#### Usage Example (Remediation Orchestrator)

```go
// Query Context API and handle graceful degradation
func (r *RemediationRequestReconciler) getHistoricalContext(
    ctx context.Context,
    remediation *RemediationRequest,
) *HistoricalContext {

    // Attempt to fetch from Context API
    contextResp, err := r.ContextAPIClient.GetRemediationContext(ctx, remediation.Name)

    if err != nil {
        // Graceful degradation: build fallback context
        log.Warn("Context API unavailable, using fallback context",
            "error", err,
            "remediationRequest", remediation.Name)

        r.Recorder.Event(remediation, "Warning", "ContextAPIUnavailable",
            "Using fallback context from WorkflowExecutionRefs")

        return r.buildFallbackContext(remediation)
    }

    // Success: convert to embeddable format
    return r.convertToEmbeddableContext(contextResp)
}
```

#### Database Queries

Context API executes the following queries to build the response:

1. **Previous Failures**: Query `workflow_executions` table filtered by `remediation_request_id`
2. **Related Alerts**: Query `alert_correlations` table with semantic search
3. **Historical Patterns**: Query `failure_patterns` table filtered by `alert_name`
4. **Successful Strategies**: Query `remediation_strategies` table ordered by `success_count`

**Query Performance**: Target <500ms for 95th percentile

#### Metrics

Context API tracks the following metrics for this endpoint:

```
context_api_recovery_requests_total{status="success|error"}
context_api_recovery_request_duration_seconds{quantile="0.5|0.95|0.99"}
context_api_recovery_context_quality{quality="complete|partial|minimal"}
context_api_recovery_previous_failures_count (histogram)
```

#### Related Documentation

- **Design Decision**: [`OPTION_B_IMPLEMENTATION_SUMMARY.md`](../../../architecture/OPTION_B_IMPLEMENTATION_SUMMARY.md)
- **Remediation Orchestrator Integration**: [`05-remediationorchestrator/OPTION_B_CONTEXT_API_INTEGRATION.md`](../../crd-controllers/05-remediationorchestrator/OPTION_B_CONTEXT_API_INTEGRATION.md)
- **Business Requirements**: BR-WF-RECOVERY-011 in `docs/requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md`
- **Sequence Diagram**: [`PROPOSED_FAILURE_RECOVERY_SEQUENCE.md`](../../../architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md)

---

## Success Rate API

### Get Success Rates

**Purpose**: Calculate success rates for workflows targeting specific resources

#### Request

```
GET /api/v1/context/success-rates
```

#### Query Parameters

| Parameter | Type | Required | Description | Example |
|-----------|------|----------|-------------|---------|
| `namespace` | string | Yes | Kubernetes namespace | `production` |
| `targetType` | string | Yes | Resource type | `deployment` |
| `targetName` | string | No | Specific resource name | `api-server` |
| `workflow` | string | No | Filter by workflow name | `scale-up` |
| `timeRange` | string | No | Time range | `30d` (default) |
| `groupBy` | string | No | Group results by | `workflow`, `environment` |

#### Example Request

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "http://context-api:8080/api/v1/context/success-rates?namespace=production&targetType=deployment&targetName=api-server&timeRange=30d&groupBy=workflow"
```

#### Response (200 OK)

```json
{
  "namespace": "production",
  "targetType": "deployment",
  "targetName": "api-server",
  "timeRange": "30d",
  "groupBy": "workflow",
  "overall": {
    "totalExecutions": 45,
    "successful": 40,
    "failed": 5,
    "successRate": 0.889,
    "avgDurationSeconds": 42.5,
    "p50DurationSeconds": 38,
    "p95DurationSeconds": 67,
    "p99DurationSeconds": 120
  },
  "byWorkflow": [
    {
      "workflow": "scale-up",
      "totalExecutions": 20,
      "successful": 19,
      "failed": 1,
      "successRate": 0.950,
      "avgDurationSeconds": 35,
      "lastExecution": "2025-10-05T14:30:00Z",
      "trend": "stable"
    },
    {
      "workflow": "restart-pods",
      "totalExecutions": 15,
      "successful": 12,
      "failed": 3,
      "successRate": 0.800,
      "avgDurationSeconds": 52,
      "lastExecution": "2025-10-04T09:15:00Z",
      "trend": "declining"
    }
  ],
  "cachedAt": "2025-10-06T10:15:30Z"
}
```

#### Go Implementation

```go
// pkg/contextapi/handlers/success_rate.go
package handlers

import (
    "encoding/json"
    "net/http"

    "go.uber.org/zap"

    "github.com/jordigilh/kubernaut/pkg/contextapi/models"
    "github.com/jordigilh/kubernaut/pkg/contextapi/services"
    "github.com/jordigilh/kubernaut/pkg/correlation"
)

type SuccessRateHandler struct {
    contextService *services.ContextService
    logger         *zap.Logger
}

func (h *SuccessRateHandler) GetSuccessRates(w http.ResponseWriter, r *http.Request) {
    correlationID := correlation.FromContext(r.Context())
    log := h.logger.With(
        zap.String("correlation_id", correlationID),
        zap.String("handler", "GetSuccessRates"),
    )

    // Parse query parameters
    req := &models.SuccessRateRequest{
        Namespace:  r.URL.Query().Get("namespace"),
        TargetType: r.URL.Query().Get("targetType"),
        TargetName: r.URL.Query().Get("targetName"),
        Workflow:   r.URL.Query().Get("workflow"),
        TimeRange:  r.URL.Query().Get("timeRange"),
        GroupBy:    r.URL.Query().Get("groupBy"),
    }

    // Validate required fields
    if err := req.Validate(); err != nil {
        log.Warn("Invalid request", zap.Error(err))
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Defaults
    if req.TimeRange == "" {
        req.TimeRange = "30d"
    }
    if req.GroupBy == "" {
        req.GroupBy = "workflow"
    }

    log.Info("Calculating success rates",
        zap.String("namespace", req.Namespace),
        zap.String("groupBy", req.GroupBy),
    )

    // Calculate success rates
    rates, err := h.contextService.GetSuccessRates(r.Context(), req)
    if err != nil {
        log.Error("Failed to calculate success rates", zap.Error(err))
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    // Return JSON response
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("X-Correlation-ID", correlationID)
    json.NewEncoder(w).Encode(rates)

    log.Info("Success rates returned",
        zap.Int("total_executions", rates.Overall.TotalExecutions),
        zap.Float64("overall_success_rate", rates.Overall.SuccessRate),
    )
}
```

---

## Pattern Matching API

### Match Similar Incidents

**Purpose**: Find similar past incidents using vector embeddings

#### Request

```
POST /api/v1/context/match
```

#### Request Body

```json
{
  "namespace": "production",
  "targetType": "deployment",
  "targetName": "api-server",
  "description": "High memory usage causing pod restarts",
  "signalType": "prometheus",
  "labels": {
    "app": "api-server",
    "environment": "production"
  },
  "maxResults": 10,
  "minSimilarity": 0.7
}
```

#### Response (200 OK)

```json
{
  "queryEmbedding": "[0.123, -0.456, ...]",
  "totalMatches": 5,
  "matches": [
    {
      "id": "remediation-ghi789",
      "namespace": "production",
      "targetName": "payment-api",
      "description": "Memory leak causing OOM",
      "createdAt": "2025-09-15T12:00:00Z",
      "rootCause": "Unbounded cache growth",
      "workflow": "increase-memory-limit",
      "phase": "Completed",
      "similarityScore": 0.94,
      "matchedPatterns": ["memory", "cache", "oom"]
    }
  ],
  "processingTimeMs": 45
}
```

---

## Structured Context for AI (Data Provider - READ-ONLY)

**Business Requirements**: BR-LLM-021 to BR-LLM-026, BR-CTX-001 to BR-CTX-005
**Context API Role**: Read-only historical data provider (NO LLM processing)

**Source of Truth**: `docs/design/CANONICAL_ACTION_TYPES.md` (27 canonical action types)

### Get Structured Context for HolmesGPT API

**Purpose**: Provide enriched, AI-optimized historical data that **HolmesGPT API** (the client) uses with its LLM for structured action generation. Context API queries the `remediation_audit` table and returns historical success rates and constraints for all 27 canonical action types.

**Architectural Note**: Context API is a **stateless REST service** that provides data. HolmesGPT API (the consumer) performs LLM processing.

#### Request

```
GET /api/v1/context/structured
```

#### Query Parameters

| Parameter | Type | Required | Description | Example |
|-----------|------|----------|-------------|---------|
| `namespace` | string | Yes | Kubernetes namespace | `production` |
| `alertName` | string | Yes | Alert name | `HighMemoryUsage` |
| `resourceType` | string | Yes | Resource type | `deployment` |
| `resourceName` | string | Yes | Resource name | `api-server` |
| `timeRange` | string | No | Historical data range | `7d` (default) |
| `includeSuccessRates` | boolean | No | Include action success rates | `true` (default) |
| `includeConstraints` | boolean | No | Include environment constraints | `true` (default) |

#### Example Request

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "http://context-api:8080/api/v1/context/structured?namespace=production&alertName=HighMemoryUsage&resourceType=deployment&resourceName=api-server"
```

#### Response (200 OK)

```json
{
  "contextId": "ctx-abc123-20251007",
  "timestamp": "2025-10-07T10:00:00Z",
  "targetResource": {
    "namespace": "production",
    "resourceType": "deployment",
    "resourceName": "api-server",
    "currentState": {
      "replicas": 3,
      "availableReplicas": 3,
      "memoryUsage": "85%",
      "cpuUsage": "45%"
    }
  },
  "actionSuccessRates": {
    "restart_pod": {
      "successRate": 0.92,
      "totalExecutions": 45,
      "averageExecutionTime": "15s",
      "lastSuccessful": "2025-10-06T14:30:00Z",
      "applicableConditions": ["high_memory", "memory_leak"]
    },
    "increase_resources": {
      "successRate": 0.88,
      "totalExecutions": 28,
      "averageExecutionTime": "2m30s",
      "lastSuccessful": "2025-10-05T09:15:00Z",
      "applicableConditions": ["resource_constrained", "high_load"]
    },
    "rollback_deployment": {
      "successRate": 0.95,
      "totalExecutions": 12,
      "averageExecutionTime": "1m45s",
      "lastSuccessful": "2025-10-03T16:20:00Z",
      "applicableConditions": ["bad_deployment", "regression"]
    },
    "uncordon_node": {
      "successRate": 0.98,
      "totalExecutions": 18,
      "averageExecutionTime": "5s",
      "lastSuccessful": "2025-10-06T16:00:00Z",
      "applicableConditions": ["node_maintenance_complete", "node_healthy"]
    },
    "taint_node": {
      "successRate": 0.95,
      "totalExecutions": 23,
      "averageExecutionTime": "8s",
      "lastSuccessful": "2025-10-07T10:30:00Z",
      "applicableConditions": ["node_disk_issue", "node_isolation_required"]
    },
    "untaint_node": {
      "successRate": 0.96,
      "totalExecutions": 21,
      "averageExecutionTime": "5s",
      "lastSuccessful": "2025-10-07T11:15:00Z",
      "applicableConditions": ["issue_resolved", "node_healthy"]
    }
  },
  "_comment": "Context API provides success rates for all 29 canonical action types. Only 6 shown as example."
  "environmentConstraints": {
    "maxMemoryLimit": "8Gi",
    "maxCpuLimit": "4",
    "minReplicas": 2,
    "maxReplicas": 10,
    "allowedActions": [
      "restart_pod",
      "increase_resources",
      "scale_deployment",
      "rollback_deployment",
      "uncordon_node",
      "taint_node",
      "untaint_node",
      "cordon_node"
    ],
    "restrictedActions": [
      "drain_node"
    ],
    "_comment": "Allowed actions list can contain any of the 29 canonical action types based on environment constraints"
    "approvalRequired": [
      "rollback_deployment",
      "scale_deployment"
    ]
  },
  "historicalPatterns": [
    {
      "patternId": "pattern-mem-leak-001",
      "description": "Memory leak pattern detected in similar deployments",
      "frequency": 8,
      "lastOccurred": "2025-10-05T12:00:00Z",
      "successfulRemediation": {
        "primaryAction": "restart_pod",
        "secondaryAction": "increase_resources",
        "successRate": 0.90
      }
    },
    {
      "patternId": "pattern-cache-growth-002",
      "description": "Unbounded cache growth in Java applications",
      "frequency": 5,
      "lastOccurred": "2025-10-04T08:30:00Z",
      "successfulRemediation": {
        "primaryAction": "restart_pod",
        "configurationChange": "Set cache eviction policy",
        "successRate": 0.85
      }
    }
  ],
  "resourceHealthIndicators": {
    "memoryPressure": {
      "current": 0.85,
      "threshold": 0.80,
      "trend": "increasing",
      "recommendation": "Immediate action required"
    },
    "crashLoopBackoff": {
      "occurrences": 0,
      "lastOccurrence": null,
      "trend": "stable"
    },
    "deploymentHealth": {
      "score": 0.75,
      "factors": [
        "High memory usage",
        "All replicas available",
        "No recent failures"
      ]
    }
  },
  "recommendedActionPriorities": {
    "restart_pod": {
      "priority": "high",
      "confidence": 0.90,
      "reasoning": "Historical success rate 92%, matches known patterns"
    },
    "increase_resources": {
      "priority": "medium",
      "confidence": 0.85,
      "reasoning": "Memory threshold exceeded, successful in 88% of cases"
    }
  },
  "metadata": {
    "formatVersion": "v2-structured",
    "generatedAt": "2025-10-07T10:00:00Z",
    "dataTimeRange": "7d",
    "confidenceScore": 0.88
  }
}
```

#### Structured Context Schema

**Key Features for AI Consumption**:

1. **Action Success Rates**: Historical success rates for each predefined action type
2. **Environment Constraints**: Resource limits, allowed/restricted actions, approval requirements
3. **Historical Patterns**: Similar incidents and their successful remediations
4. **Health Indicators**: Current resource health with thresholds and trends
5. **Recommended Priorities**: AI-optimized action priorities based on historical data

#### Integration with HolmesGPT

**HolmesGPT API consumes structured context**:

```python
# In HolmesGPT API Service
async def enrich_investigation_context(
    alert_name: str,
    namespace: str,
    resource_type: str,
    resource_name: str
) -> Dict[str, Any]:
    """
    Enrich investigation with structured context from Context API
    """
    context_response = await context_api_client.get_structured_context(
        namespace=namespace,
        alert_name=alert_name,
        resource_type=resource_type,
        resource_name=resource_name,
        include_success_rates=True,
        include_constraints=True
    )

    return {
        "allowed_actions": context_response["environmentConstraints"]["allowedActions"],
        "action_success_rates": context_response["actionSuccessRates"],
        "historical_patterns": context_response["historicalPatterns"],
        "health_indicators": context_response["resourceHealthIndicators"],
        "constraints": context_response["environmentConstraints"]
    }

# Use in HolmesGPT prompt
async def generate_structured_prompt(
    alert: Alert,
    enriched_context: Dict[str, Any]
) -> str:
    """
    Generate structured prompt for HolmesGPT with context
    """
    prompt = f"""
    ALERT: {alert.name}
    RESOURCE: {alert.namespace}/{alert.resource_name}

    ALLOWED ACTIONS (use ONLY these action types):
    {', '.join(enriched_context['allowed_actions'])}

    ACTION SUCCESS RATES (historical data):
    {json.dumps(enriched_context['action_success_rates'], indent=2)}

    HISTORICAL PATTERNS:
    {json.dumps(enriched_context['historical_patterns'], indent=2)}

    ENVIRONMENT CONSTRAINTS:
    - Max Memory: {enriched_context['constraints']['maxMemoryLimit']}
    - Max CPU: {enriched_context['constraints']['maxCpuLimit']}
    - Approval Required: {', '.join(enriched_context['constraints']['approvalRequired'])}

    Generate structured remediation actions in JSON format...
    """
    return prompt
```

#### Configuration Requirements

**File**: `internal/config/config.go`

```go
type ContextAPIConfig struct {
    // Existing fields...

    // Structured context support (NEW)
    EnableStructuredContext  bool   `yaml:"enable_structured_context" envconfig:"ENABLE_STRUCTURED_CONTEXT"`
    IncludeActionRates       bool   `yaml:"include_action_rates" envconfig:"INCLUDE_ACTION_RATES"`
    IncludeEnvironmentConstraints bool `yaml:"include_environment_constraints" envconfig:"INCLUDE_ENVIRONMENT_CONSTRAINTS"`
    DefaultTimeRange         string `yaml:"default_time_range" envconfig:"DEFAULT_TIME_RANGE"`
}
```

**Configuration File** (`config/development.yaml`):

```yaml
contextApi:
  endpoint: "http://context-api.kubernaut-system.svc.cluster.local:8091"

  # Structured context support (NEW)
  enable_structured_context: true
  include_action_rates: true
  include_environment_constraints: true
  default_time_range: "7d"

  # Existing configuration...
  cache_ttl: "5m"
  max_results: 10
```

#### Testing Requirements

**Unit Tests** (`pkg/context/structured_context_test.go`):

```go
var _ = Describe("Structured Context Generation", func() {
    var contextService *ContextAPIService

    BeforeEach(func() {
        contextService = NewContextAPIService(config)
    })

    Context("Generating structured context", func() {
        It("should include action success rates", func() {
            ctx := context.Background()
            structuredCtx, err := contextService.GetStructuredContext(ctx, &ContextRequest{
                Namespace:    "production",
                AlertName:    "HighMemoryUsage",
                ResourceType: "deployment",
                ResourceName: "api-server",
            })

            Expect(err).ToNot(HaveOccurred())
            Expect(structuredCtx.ActionSuccessRates).ToNot(BeEmpty())

            // Verify restart_pod success rate
            restartPodRate := structuredCtx.ActionSuccessRates["restart_pod"]
            Expect(restartPodRate.SuccessRate).To(BeNumerically(">", 0.0))
            Expect(restartPodRate.TotalExecutions).To(BeNumerically(">", 0))
        })

        It("should include environment constraints", func() {
            ctx := context.Background()
            structuredCtx, err := contextService.GetStructuredContext(ctx, &ContextRequest{
                Namespace:    "production",
                ResourceType: "deployment",
                ResourceName: "api-server",
            })

            Expect(err).ToNot(HaveOccurred())
            Expect(structuredCtx.EnvironmentConstraints).ToNot(BeNil())
            Expect(structuredCtx.EnvironmentConstraints.AllowedActions).ToNot(BeEmpty())
        })
    })
})
```

**Integration Tests** (`test/integration/context/structured_context_test.go`):

```go
var _ = Describe("Structured Context Integration", func() {
    Context("HolmesGPT consuming structured context", func() {
        It("should provide context for action generation", func() {
            // Get structured context
            contextResp := getStructuredContext("production", "HighMemoryUsage", "deployment", "api-server")
            Expect(contextResp.StatusCode).To(Equal(200))

            // Verify format version
            Expect(contextResp.Metadata.FormatVersion).To(Equal("v2-structured"))

            // Verify action success rates present
            Expect(contextResp.ActionSuccessRates).To(HaveKey("restart_pod"))
            Expect(contextResp.ActionSuccessRates).To(HaveKey("increase_resources"))

            // Use in HolmesGPT investigation
            investigation := performHolmesGPTInvestigation(contextResp)
            Expect(investigation.StructuredActions).ToNot(BeEmpty())
        })
    })
})
```

**Test Coverage Target**: >85%

#### Benefits of Structured Context

| Aspect | Before | After (Structured) | Improvement |
|--------|--------|-------------------|-------------|
| **Context Accuracy** | Generic patterns | Environment-specific | 40% more relevant |
| **Action Validation** | Manual validation | Pre-validated constraints | 100% compliance |
| **Success Prediction** | No historical data | Historical success rates | Data-driven decisions |
| **Response Format** | Ad-hoc JSON | Schema-driven | Type-safe integration |

---

## Health & Metrics

### Health Check

```
GET /healthz
```

**Response**: 200 OK if healthy

### Readiness Check

```
GET /readyz
```

**Response**: 200 OK if ready to serve traffic

### Metrics

```
GET /metrics
```

**Format**: Prometheus text format
**Authentication**: Required (TokenReviewer)

---

## Error Responses

### Standard Error Format

```json
{
  "error": {
    "code": "INVALID_REQUEST",
    "message": "Missing required parameter: namespace",
    "details": {
      "parameter": "namespace",
      "expected": "string"
    }
  },
  "timestamp": "2025-10-06T10:15:30Z",
  "path": "/api/v1/context",
  "correlationId": "req-2025-10-06-abc123"
}
```

### HTTP Status Codes

| Code | Meaning | Example |
|------|---------|---------|
| 200 | Success | Query successful |
| 400 | Bad Request | Missing required parameter |
| 401 | Unauthorized | Invalid token |
| 404 | Not Found | No data found for query |
| 429 | Too Many Requests | Rate limit exceeded |
| 500 | Internal Server Error | Database error |
| 503 | Service Unavailable | Database unavailable |

---

**Document Status**: ✅ Complete
**Last Updated**: October 6, 2025
**Version**: 1.0
