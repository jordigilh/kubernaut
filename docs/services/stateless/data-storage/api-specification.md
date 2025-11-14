# Data Storage Service - REST API Specification

**Version**: v2.0 (Phase 1: Read API + ADR-033 Success Rate Analytics ✅ Production-Ready)
**Last Updated**: November 5, 2025
**Base URL**: `http://data-storage.kubernaut-system:8080`
**Authentication**: Bearer Token (Kubernetes ServiceAccount) - *Phase 2*
**Implementation Status**: Days 1-15 Complete, 92 Tests (38 Unit, 54 Integration)

---

## Table of Contents

### Phase 1: Read API (✅ Production-Ready)
1. [Incidents Read API](#incidents-read-api-phase-1)
   - [List Incidents](#list-incidents)
   - [Get Incident by ID](#get-incident-by-id)
2. [Success Rate Analytics API](#success-rate-analytics-api-adr-033) **✨ NEW in v2.0**
   - [Get Success Rate by Incident Type](#get-success-rate-by-incident-type)
   - [Get Success Rate by Playbook](#get-success-rate-by-playbook)
   - [Get Multi-Dimensional Success Rate](#get-multi-dimensional-success-rate) **✨ NEW**
3. [Health & Metrics](#health--metrics)
4. [RFC 7807 Error Responses](#rfc-7807-error-responses)

### Phase 2: Write API (📋 Planned)
5. [Remediation Audit API](#remediation-audit-api-phase-2)
6. [AI Analysis Audit API](#ai-analysis-audit-api-phase-2)
7. [Workflow Audit API](#workflow-audit-api-phase-2)
8. [Execution Audit API](#execution-audit-api-phase-2)

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

## Incidents Read API (Phase 1)

**Status**: ✅ Production-Ready (Days 1-8 Complete)
**Business Requirements**: BR-STORAGE-021 through BR-STORAGE-028
**Test Coverage**: 75 tests (38 unit, 37 integration)

### List Incidents

Retrieve a filtered and paginated list of incidents from the `resource_action_traces` table.

#### Request

```http
GET /api/v1/incidents?severity=critical&limit=100&offset=0
```

#### Query Parameters

| Parameter | Type | Required | Default | Validation | Description |
|-----------|------|----------|---------|------------|-------------|
| `namespace` | string | No | - | Alphanumeric + hyphens | Filter by Kubernetes namespace |
| `severity` | string | No | - | Enum: `critical`, `high`, `medium`, `low` | Filter by alert severity |
| `cluster` | string | No | - | Alphanumeric + hyphens | Filter by cluster name |
| `action_type` | string | No | - | Alphanumeric + hyphens | Filter by remediation action type |
| `alert_name` | string | No | - | Alphanumeric + hyphens + underscore | Filter by alert name |
| `limit` | integer | No | 100 | 1-1000 | Maximum results per page |
| `offset` | integer | No | 0 | ≥0 | Number of records to skip |

#### Response (200 OK)

```json
{
  "data": [
    {
      "id": 12345,
      "action_history_id": 1,
      "action_id": "uuid-string",
      "alert_name": "HighMemoryUsage",
      "alert_severity": "critical",
      "action_type": "scale",
      "action_timestamp": "2025-11-01T10:00:00Z",
      "model_used": "gpt-4o",
      "model_confidence": 0.95,
      "execution_status": "completed"
    }
  ],
  "pagination": {
    "limit": 100,
    "offset": 0,
    "total": 1
  }
}
```

#### Example Request

```bash
# List critical incidents
curl "http://data-storage.kubernaut-system:8080/api/v1/incidents?severity=critical&limit=10"

# Filter by namespace and severity
curl "http://data-storage.kubernaut-system:8080/api/v1/incidents?namespace=production&severity=high"

# Paginate through results
curl "http://data-storage.kubernaut-system:8080/api/v1/incidents?limit=100&offset=200"
```

#### Security Features (BR-STORAGE-025)
- **SQL Injection Prevention**: Parameterized queries with PostgreSQL `$N` placeholders
- **Input Validation**: Severity enum validation, limit/offset boundary checks
- **Unicode Support**: Full UTF-8 support (BR-STORAGE-026)

#### Performance Characteristics (BR-STORAGE-027)
- **p95 Latency**: <100ms (exceeds <250ms target)
- **p99 Latency**: <200ms (exceeds <500ms target)
- **Large Result Sets (1000 records)**: p99 <500ms (exceeds <1s target)

---

### Get Incident by ID

Retrieve a single incident by its unique ID.

#### Request

```http
GET /api/v1/incidents/:id
```

#### Path Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | integer | Yes | Unique incident ID |

#### Response (200 OK)

```json
{
  "id": 12345,
  "action_history_id": 1,
  "action_id": "uuid-string",
  "alert_name": "HighMemoryUsage",
  "alert_severity": "critical",
  "action_type": "scale",
  "action_timestamp": "2025-11-01T10:00:00Z",
  "model_used": "gpt-4o",
  "model_confidence": 0.95,
  "execution_status": "completed"
}
```

#### Response (404 Not Found)

See [RFC 7807 Error Responses](#rfc-7807-error-responses).

#### Example Request

```bash
# Get specific incident
curl "http://data-storage.kubernaut-system:8080/api/v1/incidents/12345"
```

---

## RFC 7807 Error Responses

All errors follow [RFC 7807 Problem Details](https://datatracker.ietf.org/doc/html/rfc7807) standard (BR-STORAGE-024).

### 400 Bad Request - Invalid Parameters

```json
{
  "type": "https://kubernaut.io/errors/validation",
  "title": "Invalid Request Parameters",
  "status": 400,
  "detail": "Invalid severity value: 'super-critical'. Must be one of: critical, high, medium, low",
  "instance": "/api/v1/incidents"
}
```

### 404 Not Found - Incident Not Found

```json
{
  "type": "https://kubernaut.io/errors/not-found",
  "title": "Incident Not Found",
  "status": 404,
  "detail": "Incident with ID 99999 does not exist",
  "instance": "/api/v1/incidents/99999"
}
```

### 500 Internal Server Error - Database Error

```json
{
  "type": "https://kubernaut.io/errors/internal",
  "title": "Internal Server Error",
  "status": 500,
  "detail": "Database query failed",
  "instance": "/api/v1/incidents"
}
```

---

## Success Rate Analytics API (ADR-033)

**Status**: ✅ Production-Ready (Days 12-18 Complete)
**Business Requirements**: BR-STORAGE-031-01, BR-STORAGE-031-02, BR-STORAGE-031-05
**Test Coverage**: 23 integration tests (6 new multi-dimensional tests)
**OpenAPI Spec**: [v2.yaml](./openapi/v2.yaml)

**Purpose**: Multi-dimensional success tracking for AI-driven remediation effectiveness analysis.

### Overview

ADR-033 introduces multi-dimensional success tracking to enable AI learning and continuous improvement. The system tracks remediation effectiveness across three dimensions:

1. **PRIMARY**: Incident Type (e.g., "pod-oom-killer", "disk-pressure")
2. **SECONDARY**: Remediation Playbook (e.g., "pod-oom-recovery", "disk-cleanup")
3. **TERTIARY**: AI Execution Mode (catalog/chained/manual)

### Key Features

- **Confidence-Based Recommendations**: High/medium/low/insufficient_data confidence levels
- **AI Execution Mode Tracking**: 90-9-1 Hybrid Model (catalog/chained/manual escalation)
- **Playbook Breakdown**: See which playbooks work best for each incident type
- **Incident-Type Breakdown**: See which problems each playbook solves best
- **Time Range Filtering**: Analyze recent (1h) vs historical (30d) data
- **Minimum Sample Thresholds**: Prevent decisions based on insufficient data

---

### Get Success Rate by Incident Type

**PRIMARY DIMENSION**: Calculate remediation success rate for a specific incident type.

#### Request

```http
GET /api/v1/success-rate/incident-type?incident_type=pod-oom-killer&time_range=7d&min_samples=5
```

#### Query Parameters

| Parameter | Type | Required | Default | Validation | Description |
|-----------|------|----------|---------|------------|-------------|
| `incident_type` | string | **Yes** | - | Non-empty string | The incident type to analyze (e.g., "pod-oom-killer") |
| `time_range` | string | No | "7d" | Pattern: `^[0-9]+(h\|d)$` | Time window: "1h", "24h", "7d", "30d" |
| `min_samples` | integer | No | 5 | ≥1 | Minimum sample size for confidence calculation |

#### Response (200 OK)

```json
{
  "incident_type": "pod-oom-killer",
  "time_range": "7d",
  "total_executions": 150,
  "successful_executions": 135,
  "failed_executions": 15,
  "success_rate": 90.0,
  "confidence": "high",
  "min_samples_met": true,
  "ai_execution_mode": {
    "catalog_selected": 135,
    "chained": 12,
    "manual_escalation": 3
  },
  "playbook_breakdown": [
    {
      "playbook_id": "pod-oom-recovery",
      "playbook_version": "v1.2",
      "executions": 120,
      "success_rate": 92.5
    },
    {
      "playbook_id": "memory-limit-increase",
      "playbook_version": "v2.0",
      "executions": 30,
      "success_rate": 80.0
    }
  ]
}
```

#### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `incident_type` | string | The incident type being analyzed |
| `time_range` | string | Time window for this analysis |
| `total_executions` | integer | Total remediation attempts |
| `successful_executions` | integer | Number of successful attempts |
| `failed_executions` | integer | Number of failed attempts |
| `success_rate` | float | Success rate percentage (0-100%) |
| `confidence` | string | Confidence level: "high" (≥100 samples), "medium" (20-99), "low" (5-19), "insufficient_data" (<5 or below min_samples) |
| `min_samples_met` | boolean | Whether minimum sample threshold was met |
| `ai_execution_mode` | object | AI execution mode distribution (ADR-033 Hybrid Model) |
| `playbook_breakdown` | array | Breakdown by playbook showing which playbooks were tried |

#### AI Execution Mode (ADR-033 Hybrid Model)

| Field | Type | Expected % | Description |
|-------|------|-----------|-------------|
| `catalog_selected` | integer | 90-95% | Single playbook selected from catalog |
| `chained` | integer | 4-9% | Multiple playbooks chained together |
| `manual_escalation` | integer | <1% | Escalated to human operator |

#### Example Requests

```bash
# Basic query with defaults
curl "http://data-storage.kubernaut-system:8080/api/v1/success-rate/incident-type?incident_type=pod-oom-killer"

# Recent data (last 24 hours)
curl "http://data-storage.kubernaut-system:8080/api/v1/success-rate/incident-type?incident_type=disk-pressure&time_range=24h"

# Higher confidence threshold
curl "http://data-storage.kubernaut-system:8080/api/v1/success-rate/incident-type?incident_type=network-timeout&min_samples=20"

# Historical analysis (30 days)
curl "http://data-storage.kubernaut-system:8080/api/v1/success-rate/incident-type?incident_type=pod-oom-killer&time_range=30d"
```

#### Error Responses

**400 Bad Request** - Missing or invalid parameters

```json
{
  "type": "https://api.kubernaut.io/problems/validation-error",
  "title": "Validation Error",
  "status": 400,
  "detail": "incident_type query parameter is required",
  "instance": "/api/v1/success-rate/incident-type"
}
```

**500 Internal Server Error** - Database or repository error

```json
{
  "type": "https://api.kubernaut.io/problems/internal-error",
  "title": "Internal Server Error",
  "status": 500,
  "detail": "Failed to retrieve success rate data",
  "instance": "/api/v1/success-rate/incident-type?incident_type=pod-oom-killer"
}
```

#### Use Cases

1. **AI Playbook Selection**: Choose most effective playbook for an incident type
2. **Confidence-Based Decisions**: Only use high-confidence recommendations
3. **Trend Analysis**: Compare recent (7d) vs historical (30d) success rates
4. **Playbook Effectiveness**: Identify which playbooks work best for each problem

---

### Get Success Rate by Playbook

**SECONDARY DIMENSION**: Calculate remediation success rate for a specific playbook.

#### Request

```http
GET /api/v1/success-rate/playbook?playbook_id=pod-oom-recovery&playbook_version=v1.2&time_range=7d&min_samples=5
```

#### Query Parameters

| Parameter | Type | Required | Default | Validation | Description |
|-----------|------|----------|---------|------------|-------------|
| `playbook_id` | string | **Yes** | - | Non-empty string | The playbook identifier to analyze |
| `playbook_version` | string | No | null | Pattern: `^v[0-9]+\.[0-9]+(\.[0-9]+)?$` | Specific version (e.g., "v1.2") or omit for all versions |
| `time_range` | string | No | "7d" | Pattern: `^[0-9]+(h\|d)$` | Time window: "1h", "24h", "7d", "30d" |
| `min_samples` | integer | No | 5 | ≥1 | Minimum sample size for confidence calculation |

#### Response (200 OK)

```json
{
  "playbook_id": "pod-oom-recovery",
  "playbook_version": "v1.2",
  "time_range": "7d",
  "total_executions": 200,
  "successful_executions": 185,
  "failed_executions": 15,
  "success_rate": 92.5,
  "confidence": "high",
  "min_samples_met": true,
  "ai_execution_mode": {
    "catalog_selected": 180,
    "chained": 15,
    "manual_escalation": 5
  },
  "incident_type_breakdown": [
    {
      "incident_type": "pod-oom-killer",
      "executions": 150,
      "success_rate": 94.0
    },
    {
      "incident_type": "container-memory-pressure",
      "executions": 50,
      "success_rate": 88.0
    }
  ]
}
```

#### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `playbook_id` | string | The playbook identifier being analyzed |
| `playbook_version` | string\|null | Specific version or null if aggregated across all versions |
| `time_range` | string | Time window for this analysis |
| `total_executions` | integer | Total times this playbook was executed |
| `successful_executions` | integer | Number of successful executions |
| `failed_executions` | integer | Number of failed executions |
| `success_rate` | float | Success rate percentage (0-100%) |
| `confidence` | string | Confidence level (same as incident-type endpoint) |
| `min_samples_met` | boolean | Whether minimum sample threshold was met |
| `ai_execution_mode` | object | AI execution mode distribution |
| `incident_type_breakdown` | array | Breakdown by incident type showing which problems this playbook solves |

#### Example Requests

```bash
# All versions of a playbook
curl "http://data-storage.kubernaut-system:8080/api/v1/success-rate/playbook?playbook_id=pod-oom-recovery"

# Specific version (A/B testing)
curl "http://data-storage.kubernaut-system:8080/api/v1/success-rate/playbook?playbook_id=pod-oom-recovery&playbook_version=v2.0"

# Compare versions
curl "http://data-storage.kubernaut-system:8080/api/v1/success-rate/playbook?playbook_id=pod-oom-recovery&playbook_version=v1.2" > v1.json
curl "http://data-storage.kubernaut-system:8080/api/v1/success-rate/playbook?playbook_id=pod-oom-recovery&playbook_version=v2.0" > v2.json
diff v1.json v2.json

# Recent playbook effectiveness
curl "http://data-storage.kubernaut-system:8080/api/v1/success-rate/playbook?playbook_id=disk-cleanup&time_range=24h"
```

#### Error Responses

Same error response format as incident-type endpoint.

#### Use Cases

1. **Playbook Validation**: Verify new playbook versions are effective
2. **Version Comparison**: A/B test v1.2 vs v2.0
3. **Incident-Type Suitability**: Identify which problems a playbook solves best
4. **Playbook Catalog Optimization**: Remove low-performing playbooks

---

### Get Multi-Dimensional Success Rate

**CROSS-DIMENSIONAL AGGREGATION**: Calculate success rate across any combination of dimensions (incident type, playbook, action type).

**BR-STORAGE-031-05**: Multi-Dimensional Success Rate API

#### Request

```http
GET /api/v1/success-rate/multi-dimensional?incident_type=pod-oom-killer&playbook_id=pod-oom-recovery&playbook_version=v1.2&action_type=increase_memory&time_range=7d&min_samples=5
```

#### Query Parameters

| Parameter | Type | Required | Default | Validation | Description |
|-----------|------|----------|---------|------------|-------------|
| `incident_type` | string | No | - | Non-empty string | Filter to specific incident type |
| `playbook_id` | string | No | - | Non-empty string | Filter to specific playbook |
| `playbook_version` | string | No | - | Pattern: `^v[0-9]+\.[0-9]+(\.[0-9]+)?$` | Specific version (requires `playbook_id`) |
| `action_type` | string | No | - | Non-empty string | Filter to specific action type |
| `time_range` | string | No | "7d" | Pattern: `^[0-9]+(h\|d)$` | Time window: "1h", "24h", "7d", "30d", "90d" |
| `min_samples` | integer | No | 5 | ≥1 | Minimum sample size for confidence calculation |

**Validation Rules**:
- At least one dimension filter must be provided
- `playbook_version` requires `playbook_id` to be specified
- All dimension combinations are supported

#### Response (200 OK)

```json
{
  "dimensions": {
    "incident_type": "pod-oom-killer",
    "playbook_id": "pod-oom-recovery",
    "playbook_version": "v1.2",
    "action_type": "increase_memory"
  },
  "time_range": "7d",
  "total_executions": 50,
  "successful_executions": 45,
  "failed_executions": 5,
  "success_rate": 90.0,
  "confidence": "medium",
  "min_samples_met": true
}
```

#### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `dimensions` | object | Echo of query dimensions (empty string if not specified) |
| `dimensions.incident_type` | string | Incident type filter applied |
| `dimensions.playbook_id` | string | Playbook ID filter applied |
| `dimensions.playbook_version` | string | Playbook version filter applied |
| `dimensions.action_type` | string | Action type filter applied |
| `time_range` | string | Time window for this analysis |
| `total_executions` | integer | Total executions matching all dimension filters |
| `successful_executions` | integer | Number of successful executions |
| `failed_executions` | integer | Number of failed executions |
| `success_rate` | float | Success rate percentage (0-100%) |
| `confidence` | string | Confidence level (high/medium/low/insufficient_data) |
| `min_samples_met` | boolean | Whether minimum sample threshold was met |

#### Example Requests

```bash
# All three dimensions (most specific)
curl "http://data-storage.kubernaut-system:8080/api/v1/success-rate/multi-dimensional?incident_type=pod-oom-killer&playbook_id=pod-oom-recovery&playbook_version=v1.2&action_type=increase_memory"

# Two dimensions: incident + playbook (all actions)
curl "http://data-storage.kubernaut-system:8080/api/v1/success-rate/multi-dimensional?incident_type=pod-oom-killer&playbook_id=pod-oom-recovery&playbook_version=v1.2"

# Single dimension: incident type only
curl "http://data-storage.kubernaut-system:8080/api/v1/success-rate/multi-dimensional?incident_type=pod-oom-killer&time_range=30d"

# Single dimension: playbook only
curl "http://data-storage.kubernaut-system:8080/api/v1/success-rate/multi-dimensional?playbook_id=pod-oom-recovery"

# Action type effectiveness across all incidents
curl "http://data-storage.kubernaut-system:8080/api/v1/success-rate/multi-dimensional?action_type=increase_memory&time_range=7d"
```

#### Error Responses

**400 Bad Request** - Invalid parameters:
```json
{
  "type": "https://api.kubernaut.io/problems/validation-error",
  "title": "Invalid Query Parameter",
  "status": 400,
  "detail": "playbook_version requires playbook_id to be specified",
  "instance": "/api/v1/success-rate/multi-dimensional"
}
```

**400 Bad Request** - Invalid time_range:
```json
{
  "type": "https://api.kubernaut.io/problems/validation-error",
  "title": "Invalid Query Parameter",
  "status": 400,
  "detail": "invalid time_range: invalid (expected: 1h, 1d, 7d, 30d, 90d)",
  "instance": "/api/v1/success-rate/multi-dimensional"
}
```

**500 Internal Server Error** - Database failure:
```json
{
  "type": "https://api.kubernaut.io/problems/internal-error",
  "title": "Internal Server Error",
  "status": 500,
  "detail": "Failed to retrieve multi-dimensional success rate data",
  "instance": "/api/v1/success-rate/multi-dimensional"
}
```

#### Use Cases

1. **AI Learning**: "What's the success rate of playbook X for incident type Y?"
2. **Action Effectiveness**: "Does action Z work better for incident A or B?"
3. **Playbook Optimization**: "Which actions in this playbook are failing?"
4. **Version Comparison**: "Is v2.0 better than v1.2 for OOM incidents?"
5. **Incident-Specific Analysis**: "What's the best playbook+action combo for disk-full?"
6. **Trend Analysis**: "Is our overall success rate improving over time?"

#### Integration Examples

**AI/LLM Service** (Playbook Selection):
```go
// Query: "Best playbook for pod-oom-killer?"
resp, err := client.Get(fmt.Sprintf(
    "%s/api/v1/success-rate/multi-dimensional?incident_type=pod-oom-killer&time_range=30d",
    dataStorageURL,
))
// AI selects playbook with highest success_rate
```

**Effectiveness Monitor** (Trend Detection):
```go
// Compare last 7 days vs previous 7 days
current := getSuccessRate("incident_type=pod-oom-killer&time_range=7d")
previous := getSuccessRate("incident_type=pod-oom-killer&time_range=14d") // Approximate
trend := current.SuccessRate - previous.SuccessRate
```

**HolmesGPT API** (Direct Consumer):
```go
// HolmesGPT API calls Data Storage directly
func (s *HolmesGPTService) GetPlaybookSuccessRate(ctx context.Context, playbookID string) (*SuccessRate, error) {
    // Direct call to Data Storage with authentication
    resp, err := s.dataStorageClient.GetMultiDimensionalSuccessRate(ctx, playbookID)
    // Return response
}
```

---

### Confidence Levels

ADR-033 uses sample-size-based confidence levels to prevent decisions based on insufficient data:

| Confidence Level | Sample Size | Recommended Action |
|-----------------|-------------|-------------------|
| **high** | ≥100 samples | ✅ Safe to use for automated decisions |
| **medium** | 20-99 samples | ⚠️ Use with caution, consider manual review |
| **low** | 5-19 samples | ⚠️ Insufficient for automated decisions, manual review required |
| **insufficient_data** | <5 samples (or below `min_samples` threshold) | ❌ Do not use for decisions, collect more data |

### Performance Characteristics

- **p95 Latency**: <150ms (incident-type), <200ms (playbook)
- **p99 Latency**: <300ms (incident-type), <400ms (playbook)
- **Database Indexes**: Optimized for `incident_type`, `playbook_id`, `action_timestamp`
- **Cache Strategy**: No caching (real-time aggregation for accuracy)

### Security Features

- **SQL Injection Prevention**: Parameterized queries with PostgreSQL `$N` placeholders
- **Input Validation**: Time range pattern validation, min_samples boundary checks
- **Rate Limiting**: Same as other endpoints (500 req/sec per service)

---

## Health & Metrics

### Health Check Endpoints

```bash
# Liveness probe (DD-007 compatible)
GET /health/live
# Returns 200 OK if service is alive

# Readiness probe (DD-007 graceful shutdown integration)
GET /health/ready
# Returns 200 OK if ready to accept traffic
# Returns 503 Service Unavailable during graceful shutdown

# Combined health check
GET /health
# Returns 200 OK if service is healthy
```

### Prometheus Metrics

```bash
# Metrics endpoint
GET /metrics
# Exposes Prometheus metrics (port 9090)
```

**Key Metrics**:
- `http_requests_total{method="GET", path="/api/v1/incidents", status="200"}` - Request count
- `http_request_duration_seconds{method="GET", path="/api/v1/incidents"}` - Request latency histogram
- `database_query_duration_seconds{query="list_incidents"}` - Database query latency

---

## Phase 2: Write API (Planned)

The following endpoints are planned for Phase 2 implementation:

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

## Remediation Audit API (Phase 2)

**Status**: 📋 Planned for Phase 2

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
