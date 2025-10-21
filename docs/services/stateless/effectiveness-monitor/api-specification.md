# Effectiveness Monitor Service - API Specification

**Version**: v1.0
**Last Updated**: October 6, 2025
**Service Type**: Stateless HTTP API Service (Assessment & Analysis)
**HTTP Port**: 8080
**Metrics Port**: 9090

---

## 📋 API Overview

**Base URL**: `http://effectiveness-monitor-service.kubernaut-system.svc.cluster.local:8080`

**Authentication**:
- **API endpoints** (`/api/v1/assess/*`): Kubernetes TokenReviewer (Bearer token required)
- **Health endpoints** (`/health`, `/ready`): No authentication
- **Metrics endpoint** (`/metrics`): TokenReviewer authentication required

---

## 🔐 HTTP Endpoints

### **1. POST `/api/v1/assess/effectiveness`**

**Purpose**: Request effectiveness assessment for a specific remediation action

**Authentication**: Required (TokenReviewer)

**Request Headers**:
```
Content-Type: application/json
Authorization: Bearer <kubernetes-serviceaccount-token>
```

**Request Body**:
```json
{
  "action_id": "act-abc123",
  "action_type": "restart-pod",
  "namespace": "prod-payment-service",
  "cluster": "us-west-2",
  "wait_for_stabilization": true,
  "assessment_interval": "10m"
}
```

**Request Parameters**:
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `action_id` | string | Yes | Unique identifier for the remediation action |
| `action_type` | string | Yes | Type of action (e.g., `restart-pod`, `scale-deployment`) |
| `namespace` | string | No | Kubernetes namespace (for context) |
| `cluster` | string | No | Cluster identifier (for context) |
| `wait_for_stabilization` | boolean | No | Wait for system stabilization before assessment (default: `false`) |
| `assessment_interval` | string | No | Time window for metrics correlation (default: `10m`) |

**Response** (200 OK - Week 13+, Full Assessment):
```json
{
  "assessment_id": "assess-xyz789",
  "action_id": "act-abc123",
  "action_type": "restart-pod",
  "traditional_score": 0.87,
  "environmental_impact": {
    "memory_improvement": 0.25,
    "cpu_impact": -0.05,
    "network_stability": 0.92
  },
  "confidence": 0.85,
  "status": "assessed",
  "side_effects_detected": false,
  "side_effect_severity": "none",
  "trend_direction": "improving",
  "pattern_insights": [
    "Similar actions successful in 87% of production cases",
    "Effectiveness stable across business hours and off-hours"
  ],
  "assessed_at": "2025-10-06T10:15:30Z"
}
```

**Response** (200 OK - Week 5, Insufficient Data):
```json
{
  "assessment_id": "assess-xyz789",
  "action_id": "act-abc123",
  "action_type": "restart-pod",
  "status": "insufficient_data",
  "confidence": 0.25,
  "message": "Effectiveness assessment requires 8+ weeks of historical data. Current: 0 weeks.",
  "estimated_availability": "2025-11-19T00:00:00Z",
  "assessed_at": "2025-10-06T10:15:30Z"
}
```

**Error Responses**:
- `400 Bad Request`: Invalid `action_id` or `action_type`
- `401 Unauthorized`: Missing or invalid authentication token
- `403 Forbidden`: ServiceAccount lacks required RBAC permissions
- `404 Not Found`: Action not found in Data Storage
- `429 Too Many Requests`: Rate limit exceeded (50 req/s)
- `500 Internal Server Error`: Internal assessment error
- `503 Service Unavailable`: Data Storage unavailable (critical dependency)

---

### **2. GET `/api/v1/assess/effectiveness/:actionID`**

**Purpose**: Retrieve existing effectiveness assessment by action ID

**Authentication**: Required (TokenReviewer)

**Request Headers**:
```
Authorization: Bearer <kubernetes-serviceaccount-token>
```

**URL Parameters**:
| Parameter | Type | Description |
|-----------|------|-------------|
| `actionID` | string | Unique identifier for the remediation action |

**Example Request**:
```http
GET /api/v1/assess/effectiveness/act-abc123
Authorization: Bearer eyJhbGciOiJSUzI1NiIs...
```

**Response** (200 OK - Assessment Found):
```json
{
  "assessment_id": "assess-xyz789",
  "action_id": "act-abc123",
  "action_type": "restart-pod",
  "traditional_score": 0.87,
  "environmental_impact": {
    "memory_improvement": 0.25,
    "cpu_impact": -0.05,
    "network_stability": 0.92
  },
  "confidence": 0.85,
  "status": "assessed",
  "side_effects_detected": false,
  "side_effect_severity": "none",
  "trend_direction": "improving",
  "pattern_insights": [
    "Similar actions successful in 87% of production cases"
  ],
  "assessed_at": "2025-10-06T10:15:30Z"
}
```

**Response** (404 Not Found - No Assessment):
```json
{
  "error": "assessment_not_found",
  "message": "No effectiveness assessment found for action act-abc123",
  "action_id": "act-abc123"
}
```

**Error Responses**:
- `400 Bad Request`: Invalid `action_id` format
- `401 Unauthorized`: Missing or invalid authentication token
- `403 Forbidden`: ServiceAccount lacks required RBAC permissions
- `404 Not Found`: Assessment not found
- `500 Internal Server Error`: Internal retrieval error
- `503 Service Unavailable`: Data Storage unavailable

---

### **3. POST `/api/v1/assess/batch`**

**Purpose**: Request batch effectiveness assessments for multiple actions

**Authentication**: Required (TokenReviewer)

**Request Headers**:
```
Content-Type: application/json
Authorization: Bearer <kubernetes-serviceaccount-token>
```

**Request Body**:
```json
{
  "actions": [
    {
      "action_id": "act-001",
      "action_type": "restart-pod"
    },
    {
      "action_id": "act-002",
      "action_type": "scale-deployment"
    },
    {
      "action_id": "act-003",
      "action_type": "delete-pod"
    }
  ],
  "wait_for_stabilization": false,
  "assessment_interval": "10m"
}
```

**Request Parameters**:
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `actions` | array | Yes | Array of action objects (max 50 per request) |
| `actions[].action_id` | string | Yes | Unique identifier for each action |
| `actions[].action_type` | string | Yes | Type of action |
| `wait_for_stabilization` | boolean | No | Apply to all actions (default: `false`) |
| `assessment_interval` | string | No | Apply to all actions (default: `10m`) |

**Response** (200 OK - Batch Results):
```json
{
  "batch_id": "batch-xyz123",
  "total_requested": 3,
  "successful_assessments": 2,
  "failed_assessments": 1,
  "results": [
    {
      "action_id": "act-001",
      "assessment_id": "assess-001",
      "status": "assessed",
      "traditional_score": 0.87,
      "confidence": 0.85
    },
    {
      "action_id": "act-002",
      "assessment_id": "assess-002",
      "status": "assessed",
      "traditional_score": 0.92,
      "confidence": 0.90
    },
    {
      "action_id": "act-003",
      "error": "action_not_found",
      "message": "Action act-003 not found in Data Storage"
    }
  ],
  "processed_at": "2025-10-06T10:15:30Z"
}
```

**Error Responses**:
- `400 Bad Request`: Invalid batch request (empty `actions`, exceeds max 50)
- `401 Unauthorized`: Missing or invalid authentication token
- `403 Forbidden`: ServiceAccount lacks required RBAC permissions
- `429 Too Many Requests`: Rate limit exceeded
- `500 Internal Server Error`: Internal batch processing error
- `503 Service Unavailable`: Data Storage unavailable

---

### **4. GET `/api/v1/assess/data-availability`**

**Purpose**: Check data availability for effectiveness assessments

**Authentication**: Required (TokenReviewer)

**Request Headers**:
```
Authorization: Bearer <kubernetes-serviceaccount-token>
```

**Example Request**:
```http
GET /api/v1/assess/data-availability
Authorization: Bearer eyJhbGciOiJSUzI1NiIs...
```

**Response** (200 OK - Week 5, Insufficient Data):
```json
{
  "data_weeks": 0,
  "sufficient_data": false,
  "minimum_required_weeks": 8,
  "estimated_full_capability": "2025-11-19T00:00:00Z",
  "current_capability": "insufficient_data_responses_only",
  "message": "System deployed on 2025-10-06. Collecting data for 8 weeks until full capability."
}
```

**Response** (200 OK - Week 13+, Full Capability):
```json
{
  "data_weeks": 10,
  "sufficient_data": true,
  "minimum_required_weeks": 8,
  "estimated_full_capability": "2025-10-06T00:00:00Z",
  "current_capability": "full_assessment",
  "confidence_level": "high",
  "message": "Full effectiveness assessment capability operational."
}
```

**Error Responses**:
- `401 Unauthorized`: Missing or invalid authentication token
- `403 Forbidden`: ServiceAccount lacks required RBAC permissions
- `500 Internal Server Error`: Failed to query data availability
- `503 Service Unavailable`: Data Storage unavailable

---

## 🤖 Decision Logic API (Internal)

### **Internal Method: `shouldCallAI()`**

**Purpose**: Determines if AI analysis adds value beyond automated assessment

**Function Signature**:
```go
func (s *EffectivenessMonitorService) shouldCallAI(
    workflow *WorkflowExecution,
    basicScore float64,
    anomalies []string,
) bool
```

**Decision Table**:

| Trigger Type | Condition | Frequency/Day | Annual Calls | AI Called? |
|--------------|-----------|---------------|--------------|------------|
| **P0 Failures** | `Priority == "P0" && !Success` | 50 | 18,250 | ✅ YES |
| **New Action Types** | `IsNewActionType == true` | 10 | 3,650 | ✅ YES |
| **Anomalies Detected** | `len(anomalies) > 0` | 5 | 1,825 | ✅ YES |
| **Oscillations** | `IsRecurringFailure == true` | 5 | 1,825 | ✅ YES |
| **Routine Successes** | `Success && !anomalies` | 10,000 | 3,650,000 | ❌ NO |
| **TOTAL** | - | **10,070** | **3,675,550** | **0.7% AI** |

**Volume Impact**:
- **Total Assessments**: 3.65M/year (~10K/day)
- **AI Analysis Calls**: 25.5K/year (~70/day)
- **AI Usage Rate**: 0.7% (selective, high-value only)

**Cost Impact**:
- **Always-AI Approach**: $1,825,000/year (100% AI)
- **Hybrid Approach**: $12,775/year (0.7% AI)
- **Annual Savings**: $1,812,225/year (99.3% cost reduction)

**Business Rationale**:
- ✅ Critical failures (P0) always get AI root cause analysis
- ✅ New action types build knowledge base with AI insights
- ✅ Anomalies trigger investigation for pattern learning
- ✅ Oscillations analyzed for recurring failure patterns
- ❌ Routine successes handled efficiently with automation (no AI cost)

**Return Value**:
- `true`: AI analysis warranted → Call HolmesGPT API `/api/v1/postexec/analyze`
- `false`: Automated assessment sufficient → Store basic metrics only

---

## 📊 AI Analysis Metrics

### Prometheus Metrics

```go
// AI trigger tracking
effectiveness_ai_trigger_total{trigger_type="p0_failure"}          // P0 failures analyzed
effectiveness_ai_trigger_total{trigger_type="new_action_type"}     // New action types analyzed
effectiveness_ai_trigger_total{trigger_type="anomaly_detected"}    // Anomalies investigated
effectiveness_ai_trigger_total{trigger_type="oscillation"}         // Recurring failures analyzed
effectiveness_ai_trigger_total{trigger_type="routine_skipped"}     // Routine successes (no AI)

// AI call tracking
effectiveness_ai_calls_total{status="success"}                     // Successful AI analyses
effectiveness_ai_calls_total{status="failure"}                     // Failed AI API calls
effectiveness_ai_calls_total{status="timeout"}                     // Timeout AI calls

// Cost tracking
effectiveness_ai_cost_total_dollars                                // Estimated total AI cost ($0.50/call)

// AI call duration
effectiveness_ai_call_duration_seconds_bucket                      // Histogram of AI call latency
```

### Cost Monitoring Queries

**Daily AI Analysis Cost**:
```promql
sum(increase(effectiveness_ai_cost_total_dollars[24h]))
```

**AI Trigger Breakdown (Last 7 Days)**:
```promql
sum by (trigger_type) (increase(effectiveness_ai_trigger_total[7d]))
```

**AI Call Success Rate**:
```promql
rate(effectiveness_ai_calls_total{status="success"}[24h])
/
rate(effectiveness_ai_calls_total[24h])
```

**P95 AI Call Duration**:
```promql
histogram_quantile(0.95, effectiveness_ai_call_duration_seconds_bucket)
```

**Monthly Cost Projection**:
```promql
sum(increase(effectiveness_ai_cost_total_dollars[30d]))
```

**AI Usage Rate** (% of assessments using AI):
```promql
sum(rate(effectiveness_ai_calls_total[24h]))
/
sum(rate(effectiveness_assessment_total[24h]))
```

### Cost Alerts

**Budget Exceeded** (>$15,000/year):
```promql
sum(increase(effectiveness_ai_cost_total_dollars[365d])) > 15000
```

**Unexpected AI Surge** (>100 calls/hour):
```promql
rate(effectiveness_ai_calls_total[1h]) > 100
```

**High Failure Rate** (>10% AI call failures):
```promql
sum(rate(effectiveness_ai_calls_total{status="failure"}[1h]))
/
sum(rate(effectiveness_ai_calls_total[1h])) > 0.10
```

---

## 🔍 Health & Readiness Endpoints

### **5. GET `/health`**

**Purpose**: Liveness probe (service is running)

**Authentication**: None

**Response** (200 OK):
```json
{
  "status": "healthy",
  "timestamp": "2025-10-06T10:15:30Z"
}
```

**Response** (503 Service Unavailable):
```json
{
  "status": "unhealthy",
  "reason": "data_storage_unreachable",
  "timestamp": "2025-10-06T10:15:30Z"
}
```

---

### **6. GET `/ready`**

**Purpose**: Readiness probe (service can accept traffic)

**Authentication**: None

**Response** (200 OK - Ready, Week 5):
```json
{
  "status": "ready",
  "data_weeks": 0,
  "full_capability": false,
  "current_capability": "insufficient_data_responses",
  "timestamp": "2025-10-06T10:15:30Z"
}
```

**Response** (200 OK - Ready, Week 13+):
```json
{
  "status": "ready",
  "data_weeks": 10,
  "full_capability": true,
  "current_capability": "full_assessment",
  "timestamp": "2025-10-06T10:15:30Z"
}
```

**Response** (503 Service Unavailable - Not Ready):
```json
{
  "status": "not_ready",
  "reason": "data_storage_unavailable",
  "timestamp": "2025-10-06T10:15:30Z"
}
```

---

## 📊 Metrics Endpoint

### **7. GET `/metrics`**

**Purpose**: Prometheus metrics scraping

**Authentication**: Required (TokenReviewer)

**Request Headers**:
```
Authorization: Bearer <kubernetes-serviceaccount-token>
```

**Response** (200 OK - Prometheus Text Format):
```
# HELP effectiveness_assessment_duration_seconds Duration of effectiveness assessments
# TYPE effectiveness_assessment_duration_seconds histogram
effectiveness_assessment_duration_seconds_bucket{action_type="restart-pod",confidence_level="high",le="0.1"} 0
effectiveness_assessment_duration_seconds_bucket{action_type="restart-pod",confidence_level="high",le="0.5"} 142
effectiveness_assessment_duration_seconds_bucket{action_type="restart-pod",confidence_level="high",le="1"} 387
effectiveness_assessment_duration_seconds_bucket{action_type="restart-pod",confidence_level="high",le="5"} 498
effectiveness_assessment_duration_seconds_bucket{action_type="restart-pod",confidence_level="high",le="+Inf"} 500
effectiveness_assessment_duration_seconds_sum{action_type="restart-pod",confidence_level="high"} 456.789
effectiveness_assessment_duration_seconds_count{action_type="restart-pod",confidence_level="high"} 500

# HELP effectiveness_traditional_score Traditional effectiveness score distribution
# TYPE effectiveness_traditional_score histogram
effectiveness_traditional_score_bucket{action_type="restart-pod",environment="production",le="0"} 0
effectiveness_traditional_score_bucket{action_type="restart-pod",environment="production",le="0.2"} 12
effectiveness_traditional_score_bucket{action_type="restart-pod",environment="production",le="0.5"} 45
effectiveness_traditional_score_bucket{action_type="restart-pod",environment="production",le="0.8"} 387
effectiveness_traditional_score_bucket{action_type="restart-pod",environment="production",le="1"} 500
effectiveness_traditional_score_bucket{action_type="restart-pod",environment="production",le="+Inf"} 500

# HELP effectiveness_data_availability_weeks Number of weeks of historical data available
# TYPE effectiveness_data_availability_weeks gauge
effectiveness_data_availability_weeks 10

# HELP effectiveness_insufficient_data_responses_total Total number of insufficient_data responses
# TYPE effectiveness_insufficient_data_responses_total counter
effectiveness_insufficient_data_responses_total 0

# HELP effectiveness_side_effects_detected_total Total number of assessments with side effects detected
# TYPE effectiveness_side_effects_detected_total counter
effectiveness_side_effects_detected_total{severity="high"} 5
effectiveness_side_effects_detected_total{severity="low"} 23
effectiveness_side_effects_detected_total{severity="none"} 472

# HELP effectiveness_assessments_total Total number of effectiveness assessments by status
# TYPE effectiveness_assessments_total counter
effectiveness_assessments_total{status="assessed"} 500
effectiveness_assessments_total{status="insufficient_data"} 0
effectiveness_assessments_total{status="error"} 3
```

**Error Responses**:
- `401 Unauthorized`: Missing or invalid authentication token
- `403 Forbidden`: ServiceAccount lacks required RBAC permissions

---

## 🔐 Authentication Details

### **Kubernetes TokenReviewer**

All API endpoints (except `/health` and `/ready`) require Bearer token authentication validated via Kubernetes TokenReviewer API.

**Example Request**:
```http
GET /api/v1/assess/effectiveness/act-abc123
Authorization: Bearer eyJhbGciOiJSUzI1NiIsImtpZCI6IjEyMzQ1Njc4OTAifQ...
```

**TokenReviewer Validation**:
```go
review := &authv1.TokenReview{
    Spec: authv1.TokenReviewSpec{Token: token},
}

result, err := kubeClient.AuthenticationV1().TokenReviews().Create(
    context.TODO(), review, metav1.CreateOptions{},
)

if err != nil || !result.Status.Authenticated {
    return http.StatusUnauthorized
}
```

**Required RBAC** (Client ServiceAccount):
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: effectiveness-monitor-client
rules:
- apiGroups: [""]
  resources: ["serviceaccounts/token"]
  verbs: ["create"]
```

---

## ⏱️ Rate Limiting

### **Per-Client Rate Limits**

| Endpoint | Rate Limit | Burst |
|----------|-----------|-------|
| `/api/v1/assess/effectiveness` | 50 req/s | 100 |
| `/api/v1/assess/batch` | 10 req/s | 20 |
| `/api/v1/assess/data-availability` | 10 req/s | 20 |
| `/health`, `/ready` | No limit | N/A |

**Rate Limit Response** (429 Too Many Requests):
```json
{
  "error": "rate_limit_exceeded",
  "message": "Rate limit exceeded: 50 req/s, burst 100",
  "retry_after": "1s"
}
```

---

## 📊 Response Schemas

### **EffectivenessScore** (Week 13+ Full Assessment)

```go
package effectiveness

import (
    "time"
)

type EffectivenessScore struct {
    AssessmentID        string                 `json:"assessment_id"`
    ActionID            string                 `json:"action_id"`
    ActionType          string                 `json:"action_type"`
    TraditionalScore    float64                `json:"traditional_score"`
    EnvironmentalImpact EnvironmentalMetrics   `json:"environmental_impact"`
    Confidence          float64                `json:"confidence"`
    Status              string                 `json:"status"` // "assessed", "insufficient_data", "error"
    SideEffectsDetected bool                   `json:"side_effects_detected"`
    SideEffectSeverity  string                 `json:"side_effect_severity"` // "high", "low", "none"
    TrendDirection      string                 `json:"trend_direction"` // "improving", "declining", "stable", "insufficient_data"
    PatternInsights     []string               `json:"pattern_insights"`
    AssessedAt          time.Time              `json:"assessed_at"`
}

type EnvironmentalMetrics struct {
    MemoryImprovement float64 `json:"memory_improvement"` // -1.0 to 1.0 (negative = degradation)
    CPUImpact         float64 `json:"cpu_impact"`         // -1.0 to 1.0 (negative = increase)
    NetworkStability  float64 `json:"network_stability"`  // 0.0 to 1.0 (1.0 = perfect stability)
}
```

### **InsufficientDataResponse** (Week 5)

```go
type InsufficientDataResponse struct {
    AssessmentID         string    `json:"assessment_id"`
    ActionID             string    `json:"action_id"`
    ActionType           string    `json:"action_type"`
    Status               string    `json:"status"` // "insufficient_data"
    Confidence           float64   `json:"confidence"` // 0.2-0.3
    Message              string    `json:"message"`
    EstimatedAvailability time.Time `json:"estimated_availability"`
    AssessedAt           time.Time `json:"assessed_at"`
}
```

---

## ✅ API Usage Examples

### **Example 1: Request Assessment (Week 13+)**

```bash
curl -X POST http://effectiveness-monitor-service:8080/api/v1/assess/effectiveness \
  -H "Authorization: Bearer eyJhbGciOiJSUzI1NiIs..." \
  -H "Content-Type: application/json" \
  -d '{
    "action_id": "act-abc123",
    "action_type": "restart-pod",
    "namespace": "prod-payment-service",
    "cluster": "us-west-2"
  }'
```

**Response**:
```json
{
  "assessment_id": "assess-xyz789",
  "action_id": "act-abc123",
  "traditional_score": 0.87,
  "confidence": 0.85,
  "status": "assessed",
  "trend_direction": "improving"
}
```

### **Example 2: Check Data Availability (Week 5)**

```bash
curl -X GET http://effectiveness-monitor-service:8080/api/v1/assess/data-availability \
  -H "Authorization: Bearer eyJhbGciOiJSUzI1NiIs..."
```

**Response**:
```json
{
  "data_weeks": 0,
  "sufficient_data": false,
  "minimum_required_weeks": 8,
  "estimated_full_capability": "2025-11-19T00:00:00Z",
  "current_capability": "insufficient_data_responses_only"
}
```

### **Example 3: Batch Assessment (Week 13+)**

```bash
curl -X POST http://effectiveness-monitor-service:8080/api/v1/assess/batch \
  -H "Authorization: Bearer eyJhbGciOiJSUzI1NiIs..." \
  -H "Content-Type: application/json" \
  -d '{
    "actions": [
      {"action_id": "act-001", "action_type": "restart-pod"},
      {"action_id": "act-002", "action_type": "scale-deployment"}
    ]
  }'
```

**Response**:
```json
{
  "batch_id": "batch-xyz123",
  "total_requested": 2,
  "successful_assessments": 2,
  "results": [
    {
      "action_id": "act-001",
      "assessment_id": "assess-001",
      "traditional_score": 0.87,
      "confidence": 0.85
    },
    {
      "action_id": "act-002",
      "assessment_id": "assess-002",
      "traditional_score": 0.92,
      "confidence": 0.90
    }
  ]
}
```

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Status**: ✅ Complete Specification

