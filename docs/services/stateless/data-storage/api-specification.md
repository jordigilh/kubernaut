# Data Storage Service - REST API Specification

**Version**: v1.0
**Last Updated**: October 6, 2025
**Base URL**: `http://data-storage.kubernaut-system:8080`
**Authentication**: Bearer Token (Kubernetes ServiceAccount)

---

## Table of Contents

1. [API Overview](#api-overview)
2. [Authentication](#authentication)
3. [Remediation Audit API](#remediation-audit-api)
4. [AI Analysis Audit API](#ai-analysis-audit-api)
5. [Workflow Audit API](#workflow-audit-api)
6. [Execution Audit API](#execution-audit-api)
7. [Health & Metrics](#health--metrics)
8. [Error Responses](#error-responses)

---

## API Overview

### Base URL
```
http://data-storage.kubernaut-system:8080
```

### API Version
All endpoints are prefixed with `/api/v1/`

### Content Type
```
Content-Type: application/json
```

### Rate Limiting
- **Per Service**: 500 writes/second
- **Burst**: 750 writes
- **Response Header**: `X-RateLimit-Remaining: 499`

---

## Authentication

### Bearer Token Authentication

All API requests require a valid Kubernetes ServiceAccount token:

```bash
curl -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "remediation-abc123", ...}' \
  http://data-storage.kubernaut-system:8080/api/v1/audit/remediation
```

**See**: [Kubernetes TokenReviewer Authentication](../../../../architecture/KUBERNETES_TOKENREVIEWER_AUTH.md)

---

## Remediation Audit API

### Create Remediation Audit Record

**Purpose**: Persist remediation request lifecycle event to audit trail

#### Request

```
POST /api/v1/audit/remediation
```

#### Request Body

```json
{
  "name": "remediation-abc123",
  "namespace": "production",
  "signalType": "prometheus",
  "signalName": "HighMemoryUsage",
  "signalNamespace": "production",
  "targetType": "deployment",
  "targetName": "api-server",
  "targetNamespace": "production",
  "environment": "production",
  "priority": "P0",
  "fingerprint": "sha256:abc123...",
  "phase": "Completed",
  "message": "Remediation completed successfully",
  "reason": "ScaledUp",
  "createdAt": "2025-10-06T10:00:00Z",
  "startedAt": "2025-10-06T10:00:05Z",
  "completedAt": "2025-10-06T10:00:45Z",
  "correlationId": "req-2025-10-06-abc123",
  "labels": {
    "app": "api-server",
    "tier": "backend"
  },
  "originalPayload": "base64encodeddata=="
}
```

#### Response (201 Created)

```json
{
  "id": 12345,
  "embeddingId": 67890,
  "createdAt": "2025-10-06T10:00:46Z",
  "message": "Audit record created successfully"
}
```

#### Go Implementation

```go
// pkg/datastorage/handlers/remediation_audit.go
package handlers

import (
    "encoding/json"
    "net/http"
    "time"

    "github.com/go-playground/validator/v10"
    "go.uber.org/zap"

    "github.com/jordigilh/kubernaut/pkg/datastorage/models"
    "github.com/jordigilh/kubernaut/pkg/datastorage/services"
    "github.com/jordigilh/kubernaut/pkg/correlation"
)

type RemediationAuditHandler struct {
    auditService *services.AuditService
    validator    *validator.Validate
    logger       *zap.Logger
}

func (h *RemediationAuditHandler) CreateRemediationAudit(w http.ResponseWriter, r *http.Request) {
    correlationID := correlation.FromContext(r.Context())
    log := h.logger.With(
        zap.String("correlation_id", correlationID),
        zap.String("handler", "CreateRemediationAudit"),
    )

    // Parse request body
    var req models.RemediationAuditRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        log.Warn("Invalid JSON", zap.Error(err))
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    // Validate request
    if err := h.validator.Struct(&req); err != nil {
        log.Warn("Validation failed", zap.Error(err))
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    log.Info("Creating remediation audit record",
        zap.String("name", req.Name),
        zap.String("namespace", req.Namespace),
        zap.String("phase", req.Phase),
    )

    // Create audit record (writes to PostgreSQL + Vector DB)
    result, err := h.auditService.CreateRemediationAudit(r.Context(), &req)
    if err != nil {
        log.Error("Failed to create audit record", zap.Error(err))
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    // Return 201 Created
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("X-Correlation-ID", correlationID)
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(result)

    log.Info("Audit record created",
        zap.Int64("id", result.ID),
        zap.Int64("embedding_id", result.EmbeddingID),
    )
}
```

---

## AI Analysis Audit API

### Create AI Analysis Audit Record

**Purpose**: Persist AI analysis results and investigation details

#### Request

```
POST /api/v1/audit/aianalysis
```

#### Request Body

```json
{
  "name": "ai-remediation-abc123",
  "namespace": "production",
  "remediationRequestRef": "remediation-abc123",
  "signalType": "prometheus",
  "signalContext": {
    "alertname": "HighMemoryUsage",
    "namespace": "production",
    "pod": "api-server-abc123"
  },
  "llmProvider": "openai",
  "llmModel": "gpt-4",
  "temperature": 0.7,
  "rootCause": "Memory leak in cache layer causing unbounded growth",
  "confidence": 0.85,
  "recommendedAction": "Increase memory limits and implement cache eviction policy",
  "requiresApproval": false,
  "investigationId": "inv-xyz789",
  "tokensUsed": 1250,
  "investigationTimeSeconds": 5,
  "phase": "Completed",
  "message": "AI analysis completed with high confidence",
  "createdAt": "2025-10-06T10:00:10Z",
  "startedAt": "2025-10-06T10:00:12Z",
  "completedAt": "2025-10-06T10:00:17Z",
  "correlationId": "req-2025-10-06-abc123"
}
```

#### Response (201 Created)

```json
{
  "id": 12346,
  "embeddingId": 67891,
  "createdAt": "2025-10-06T10:00:18Z",
  "message": "AI analysis audit record created successfully"
}
```

---

## Workflow Audit API

### Create Workflow Execution Audit Record

**Purpose**: Persist workflow execution history and step details

#### Request

```
POST /api/v1/audit/workflow
```

#### Request Body

```json
{
  "name": "wf-remediation-abc123",
  "namespace": "production",
  "remediationRequestRef": "remediation-abc123",
  "aiAnalysisRef": "ai-remediation-abc123",
  "workflowName": "scale-up",
  "isAutoApproved": true,
  "requiresApproval": false,
  "currentStep": 3,
  "stepCount": 3,
  "completedCount": 3,
  "failedCount": 0,
  "skippedCount": 0,
  "stepStatuses": [
    {
      "name": "validate-resources",
      "phase": "Completed",
      "startedAt": "2025-10-06T10:00:20Z",
      "completedAt": "2025-10-06T10:00:25Z",
      "retryCount": 0,
      "message": "Validation successful"
    },
    {
      "name": "scale-deployment",
      "phase": "Completed",
      "startedAt": "2025-10-06T10:00:26Z",
      "completedAt": "2025-10-06T10:00:40Z",
      "retryCount": 0,
      "message": "Scaled from 2 to 4 replicas"
    },
    {
      "name": "verify-health",
      "phase": "Completed",
      "startedAt": "2025-10-06T10:00:41Z",
      "completedAt": "2025-10-06T10:00:45Z",
      "retryCount": 0,
      "message": "Health check passed"
    }
  ],
  "phase": "Completed",
  "message": "Workflow completed successfully",
  "createdAt": "2025-10-06T10:00:18Z",
  "startedAt": "2025-10-06T10:00:20Z",
  "completedAt": "2025-10-06T10:00:45Z",
  "correlationId": "req-2025-10-06-abc123"
}
```

#### Response (201 Created)

```json
{
  "id": 12347,
  "embeddingId": 67892,
  "createdAt": "2025-10-06T10:00:46Z",
  "message": "Workflow audit record created successfully"
}
```

---

## Execution Audit API

### Create Kubernetes Execution Audit Record

**Purpose**: Persist Kubernetes action execution details and results

#### Request

```
POST /api/v1/audit/execution
```

#### Request Body

```json
{
  "name": "exec-wf-remediation-abc123",
  "namespace": "production",
  "workflowExecutionRef": "wf-remediation-abc123",
  "stepName": "scale-deployment",
  "actionType": "scale",
  "actionData": {
    "replicas": "4",
    "previousReplicas": "2"
  },
  "targetType": "deployment",
  "targetName": "api-server",
  "targetNamespace": "production",
  "enableDryRun": true,
  "enableValidation": true,
  "enableRollback": true,
  "actionResult": "success",
  "resourceVersion": "12345",
  "previousVersion": "12344",
  "dryRunResult": "Deployment would be updated successfully",
  "validationResult": "PASSED",
  "validationWarnings": [],
  "isRolledBack": false,
  "phase": "Completed",
  "message": "Deployment scaled successfully",
  "createdAt": "2025-10-06T10:00:26Z",
  "startedAt": "2025-10-06T10:00:28Z",
  "completedAt": "2025-10-06T10:00:40Z",
  "correlationId": "req-2025-10-06-abc123"
}
```

#### Response (201 Created)

```json
{
  "id": 12348,
  "embeddingId": 67893,
  "createdAt": "2025-10-06T10:00:41Z",
  "message": "Execution audit record created successfully"
}
```

---

## Health & Metrics

### Health Check

```
GET /healthz
```

**Response**: 200 OK if healthy

```json
{
  "status": "healthy",
  "timestamp": "2025-10-06T10:15:30Z",
  "dependencies": {
    "postgresql": "healthy",
    "vectordb": "healthy",
    "redis": "healthy"
  }
}
```

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

**Key Metrics**:
- `datastorage_audit_writes_total{type="remediation"}` - Total writes
- `datastorage_audit_write_duration_seconds` - Write latency histogram
- `datastorage_embedding_cache_hits_total` - Embedding cache hits
- `datastorage_embedding_generation_duration_seconds` - Embedding latency
- `datastorage_database_write_errors_total` - Write errors

---

## Error Responses

### Standard Error Format

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Missing required field: name",
    "details": {
      "field": "name",
      "constraint": "required"
    }
  },
  "timestamp": "2025-10-06T10:15:30Z",
  "path": "/api/v1/audit/remediation",
  "correlationId": "req-2025-10-06-abc123"
}
```

### HTTP Status Codes

| Code | Meaning | Example |
|------|---------|---------|
| 201 | Created | Audit record created successfully |
| 400 | Bad Request | Missing required field |
| 401 | Unauthorized | Invalid token |
| 409 | Conflict | Duplicate audit record |
| 429 | Too Many Requests | Rate limit exceeded |
| 500 | Internal Server Error | Database write failure |
| 503 | Service Unavailable | Database unavailable |

---

### Common Error Codes

| Error Code | Description | Resolution |
|------------|-------------|------------|
| `VALIDATION_ERROR` | Request validation failed | Check required fields |
| `DUPLICATE_AUDIT` | Audit record already exists | Idempotent operation - safe to ignore |
| `DATABASE_ERROR` | PostgreSQL write failed | Retry with exponential backoff |
| `EMBEDDING_ERROR` | Embedding generation failed | Retry or proceed without embedding |
| `TRANSACTION_ERROR` | Transaction rollback | Retry entire operation |

---

## Request Validation Rules

### Common Rules (All Endpoints)

| Field | Type | Required | Validation |
|-------|------|----------|------------|
| `name` | string | Yes | Max 255 chars, matches `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` |
| `namespace` | string | Yes | Max 255 chars, valid K8s namespace |
| `phase` | string | Yes | Enum: 'Pending', 'Processing', 'Completed', 'Failed' |
| `correlationId` | string | Yes | Format: `^[a-z]+-\d{4}-\d{2}-\d{2}-[a-z0-9]{6}$` |
| `createdAt` | timestamp | Yes | ISO 8601 format |
| `startedAt` | timestamp | No | Must be >= createdAt if present |
| `completedAt` | timestamp | No | Must be >= startedAt if present |

---

### Remediation-Specific Rules

| Field | Validation |
|-------|------------|
| `signalType` | Enum: 'prometheus', 'kubernetes-event' |
| `targetType` | Enum: 'deployment', 'statefulset', 'daemonset', 'pod' |
| `priority` | Enum: 'P0', 'P1', 'P2' |
| `environment` | Enum: 'production', 'staging', 'development' |
| `fingerprint` | SHA256 hash format (64 hex chars) |

---

### AI Analysis-Specific Rules

| Field | Validation |
|-------|------------|
| `confidence` | Float between 0.0 and 1.0 |
| `temperature` | Float between 0.0 and 2.0 |
| `tokensUsed` | Positive integer |
| `llmProvider` | Enum: 'openai', 'anthropic', 'local' |

---

### Workflow-Specific Rules

| Field | Validation |
|-------|------------|
| `stepCount` | Positive integer, >= 1 |
| `currentStep` | Integer, 0 <= currentStep <= stepCount |
| `completedCount` | Non-negative, <= stepCount |
| `failedCount` | Non-negative, <= stepCount |
| `skippedCount` | Non-negative, <= stepCount |
| `completedCount + failedCount + skippedCount` | <= stepCount |

---

## Idempotency

### Duplicate Write Handling

**Strategy**: Use **name + namespace** as natural key for idempotency

**Behavior**:
- First write: Creates record, returns 201 Created
- Duplicate write: Returns existing record, 201 Created (idempotent)
- Updated write (different phase): Creates new version, 201 Created

**Implementation**:
```sql
INSERT INTO remediation_requests (name, namespace, ...)
VALUES ($1, $2, ...)
ON CONFLICT (name, namespace, created_at)
DO UPDATE SET
    phase = EXCLUDED.phase,
    message = EXCLUDED.message,
    updated_at = CURRENT_TIMESTAMP
RETURNING id;
```

---

## Performance Considerations

### Write Latency Breakdown

| Operation | Target Latency | Notes |
|-----------|----------------|-------|
| Request validation | < 5ms | In-memory |
| Embedding generation | 50-150ms | LLM API call, cacheable |
| PostgreSQL write | 10-30ms | Single INSERT |
| Vector DB write | 20-50ms | Embedding INSERT |
| Response generation | < 5ms | JSON serialization |
| **Total (cached embedding)** | **< 100ms (p50)** | Cache hit path |
| **Total (fresh embedding)** | **< 250ms (p95)** | Cache miss path |

---

### Throughput Optimization

**Per Replica**:
- Target: 500 writes/second
- Peak: 750 writes/second (burst)

**Scaling**:
- 2 replicas: 1000 writes/second
- 3 replicas: 1500 writes/second

**Bottlenecks**:
1. PostgreSQL write throughput (mitigated by connection pooling)
2. Embedding generation (mitigated by caching)
3. Vector DB write throughput (mitigated by batch inserts in future)

---

**Document Status**: ✅ Complete
**Last Updated**: October 6, 2025
**Version**: 1.0
