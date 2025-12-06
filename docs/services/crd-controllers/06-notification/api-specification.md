# Notification Service - API Specification

**Version**: 2.1
**Last Updated**: 2025-12-06
**Service Type**: CRD Controller (NotificationRequest CRD)
**Architecture**: Declarative Kubernetes-native notification delivery
**Audit Integration**: ADR-034 Unified Audit Table (v2.0)

---

## 📋 API Overview

**Primary API**: Kubernetes CRD (`NotificationRequest`)
**Metrics Port**: 9090 (Prometheus metrics)

**Interaction Model**:
- **Create Notifications**: Apply `NotificationRequest` CRD to Kubernetes
- **Track Status**: Watch CRD `.status` field for delivery state
- **Audit Trail**: All events written to ADR-034 unified `audit_events` table

**Audit Events Generated**:
- `notification.message.sent` - Successful delivery to channel
- `notification.message.failed` - Delivery failure
- `notification.message.acknowledged` - User acknowledgment
- `notification.message.escalated` - Priority escalation

---

## 🔐 HTTP Endpoints

### **1. POST `/api/v1/notify/escalation`** (BR-NOT-026 through BR-NOT-037)

**Purpose**: Send comprehensive escalation notification with all required context

**Authentication**: Required (TokenReviewer)

**Request Body**:
```json
{
  "recipient": "sre-oncall@company.com",
  "channels": ["email", "slack"],
  "payload": {
    "alert": {
      "name": "PodOOMKilled",
      "severity": "warning",
      "timestamp": "2025-10-02T14:30:00Z",
      "fingerprint": "a1b2c3d4e5f6",
      "labels": {
        "alertname": "PodOOMKilled",
        "namespace": "production",
        "pod": "webapp-5f9c7d8b6-xyz12"
      },
      "annotations": {
        "summary": "Pod webapp has been OOMKilled 3 times",
        "description": "Memory limit reached repeatedly over 1 hour"
      }
    },
    "impactedResources": [
      {
        "kind": "Pod",
        "name": "webapp-5f9c7d8b6-xyz12",
        "namespace": "production",
        "state": {
          "phase": "Running",
          "restartCount": 3
        }
      }
    ],
    "rootCauseAnalysis": {
      "summary": "Chronic memory insufficiency",
      "confidence": 0.88,
      "detailedAnalysis": "Pattern Analysis: 3 OOMs in 1h indicates sustained memory pressure...",
      "methodology": "HolmesGPT + Pattern Analysis"
    },
    "analysisJustification": {
      "whyThisRootCause": "Repeated OOMs with linear memory growth pattern indicates insufficient allocation...",
      "alternativeHypotheses": [
        {
          "hypothesis": "Memory leak in application",
          "confidence": 0.15,
          "rejected": "Heap growth is linear and bounded, not exponential"
        },
        {
          "hypothesis": "External memory pressure",
          "confidence": 0.12,
          "rejected": "Node memory is adequate, only this pod affected"
        }
      ]
    },
    "recommendedRemediations": [
      {
        "rank": 1,
        "confidence": 0.88,
        "timeToResolution": "15-30 min",
        "riskLevel": "low",
        "combinedScore": 0.92,
        "action": "increase-memory-limit",
        "description": "Increase memory limit from 512Mi to 1Gi",
        "pros": [
          "Fixes root cause directly",
          "Low risk of side effects",
          "Well-established pattern"
        ],
        "cons": [
          "Requires GitOps PR review (15-30 min delay)",
          "Increased resource allocation"
        ],
        "tradeoffs": "Slower resolution vs maintaining Git as source of truth"
      },
      {
        "rank": 2,
        "confidence": 0.65,
        "timeToResolution": "5 min",
        "riskLevel": "medium",
        "combinedScore": 0.75,
        "action": "restart-pod",
        "description": "Restart pod to clear memory",
        "pros": [
          "Immediate relief",
          "Simple operation"
        ],
        "cons": [
          "Temporary fix only",
          "Brief service interruption",
          "Problem will recur"
        ],
        "tradeoffs": "Fast resolution vs temporary fix"
      }
    ],
    "nextSteps": {
      "escalationHistory": {
        "recentEvents": [
          {
            "timestamp": "2025-10-02T13:30:00Z",
            "alert": "PodOOMKilled (first occurrence)",
            "action": "Restart only",
            "outcome": "Temporary relief for 45 minutes"
          },
          {
            "timestamp": "2025-10-02T14:15:00Z",
            "alert": "PodOOMKilled (second occurrence)",
            "action": "No action taken",
            "outcome": "Automatic restart by kubelet"
          }
        ],
        "historicalSummary": "3 OOM events in past 1 hour, pattern established"
      },
      "gitopsPRLink": "https://github.com/company/k8s-manifests/pull/456",
      "monitoringLinks": [
        {
          "type": "Grafana Dashboard",
          "url": "https://grafana.company.com/d/webapp-memory",
          "description": "Memory usage trends"
        },
        {
          "type": "Prometheus Query",
          "url": "https://prometheus.company.com/graph?g0.expr=container_memory_usage_bytes",
          "description": "Container memory metrics"
        },
        {
          "type": "Kubernetes Dashboard",
          "url": "https://k8s.company.com/#!/pod/production/webapp-5f9c7d8b6-xyz12",
          "description": "Pod details"
        }
      ]
    }
  }
}
```

**Response** (200 OK):
```json
{
  "notificationId": "notif-abc123",
  "status": "delivered",
  "channels": {
    "email": {
      "status": "delivered",
      "timestamp": "2025-10-02T14:30:05Z",
      "payloadSize": "35KB"
    },
    "slack": {
      "status": "delivered",
      "timestamp": "2025-10-02T14:30:06Z",
      "payloadSize": "38KB",
      "threadTs": "1696260606.123456"
    }
  },
  "sanitizationApplied": [
    "Redacted 2 API keys from alert labels",
    "Masked 1 database connection string from logs"
  ],
  "dataFreshness": {
    "gatheredAt": "2025-10-02T14:30:05Z",
    "ageSeconds": 0,
    "isFresh": true
  }
}
```

**Error Responses**:
- `400 Bad Request`: Invalid payload structure
- `401 Unauthorized`: Missing or invalid authentication token
- `403 Forbidden`: Recipient not found or no notification permissions
- `429 Too Many Requests`: Rate limit exceeded (100 req/min per recipient)
- `500 Internal Server Error`: Notification delivery failed
- `503 Service Unavailable`: External channel unavailable (e.g., Slack API down)

---

### **2. POST `/api/v1/notify/simple`**

**Purpose**: Send simple notifications (non-escalation, general purpose)

**Authentication**: Required (TokenReviewer)

**Request Body**:
```json
{
  "recipient": "user@company.com",
  "channels": ["email"],
  "subject": "Workflow Completed",
  "message": "Your workflow 'deploy-app' completed successfully at 2025-10-02T14:35:00Z",
  "severity": "info",
  "metadata": {
    "workflowId": "wf-12345",
    "duration": "5m30s"
  }
}
```

**Response** (200 OK):
```json
{
  "notificationId": "notif-xyz789",
  "status": "delivered",
  "channels": {
    "email": {
      "status": "delivered",
      "timestamp": "2025-10-02T14:35:00Z"
    }
  }
}
```

**Error Responses**: Same as escalation endpoint

---

### **3. GET `/health`**

**Purpose**: Health check for Kubernetes liveness probe

**Port**: 8080
**Authentication**: None (public)

**Response** (200 OK):
```json
{
  "status": "OK",
  "timestamp": "2025-10-02T14:30:00Z"
}
```

**Response** (503 Service Unavailable):
```json
{
  "status": "UNHEALTHY",
  "reason": "Channel adapters initialization failed",
  "timestamp": "2025-10-02T14:30:00Z"
}
```

---

### **4. GET `/ready`**

**Purpose**: Readiness check for Kubernetes readiness probe

**Port**: 8080
**Authentication**: None (public)

**Response** (200 OK):
```json
{
  "status": "READY",
  "timestamp": "2025-10-02T14:30:00Z",
  "channels": {
    "slack": "healthy",
    "email": "healthy",
    "teams": "healthy",
    "sms": "healthy",
    "webhook": "healthy"
  }
}
```

**Response** (503 Service Unavailable):
```json
{
  "status": "NOT_READY",
  "timestamp": "2025-10-02T14:30:00Z",
  "channels": {
    "slack": "degraded",
    "email": "healthy",
    "teams": "healthy",
    "sms": "unhealthy",
    "webhook": "healthy"
  },
  "reason": "SMS channel unavailable"
}
```

---

### **5. GET `/metrics`**

**Purpose**: Prometheus metrics for observability

**Port**: 9090
**Authentication**: Required (TokenReviewer)

**Response** (200 OK):
```
# HELP notification_delivery_total Total number of notifications delivered
# TYPE notification_delivery_total counter
notification_delivery_total{channel="email",status="success"} 1523
notification_delivery_total{channel="slack",status="success"} 1421
notification_delivery_total{channel="teams",status="success"} 89
notification_delivery_total{channel="sms",status="success"} 234
notification_delivery_total{channel="email",status="failure"} 12
notification_delivery_total{channel="slack",status="failure"} 8

# HELP notification_delivery_duration_seconds Duration of notification delivery
# TYPE notification_delivery_duration_seconds histogram
notification_delivery_duration_seconds_bucket{channel="email",le="0.1"} 456
notification_delivery_duration_seconds_bucket{channel="email",le="0.5"} 1234
notification_delivery_duration_seconds_bucket{channel="email",le="1.0"} 1523
notification_delivery_duration_seconds_count{channel="email"} 1535
notification_delivery_duration_seconds_sum{channel="email"} 1234.56

# HELP notification_sanitization_applied_total Number of sanitization actions applied
# TYPE notification_sanitization_applied_total counter
notification_sanitization_applied_total{type="api_key"} 45
notification_sanitization_applied_total{type="password"} 23
notification_sanitization_applied_total{type="pii"} 67
notification_sanitization_applied_total{type="connection_string"} 12
```

---

## 📊 Go Type Definitions

### **Escalation Notification Request**

```go
package notification

import "time"

type EscalationNotificationRequest struct {
	Recipient string   `json:"recipient"`
	Channels  []string `json:"channels"` // ["email", "slack", "teams", "sms", "webhook"]
	Payload   EscalationPayload `json:"payload"`
}

type EscalationPayload struct {
	Alert                    Alert                    `json:"alert"`
	ImpactedResources        []Resource               `json:"impactedResources"`
	RootCauseAnalysis        RootCauseAnalysis        `json:"rootCauseAnalysis"`
	AnalysisJustification    AnalysisJustification    `json:"analysisJustification"`
	RecommendedRemediations  []Remediation            `json:"recommendedRemediations"`
	NextSteps                NextSteps                `json:"nextSteps"`
}

type Alert struct {
	Name        string            `json:"name"`
	Severity    string            `json:"severity"` // "critical", "warning", "info"
	Timestamp   time.Time         `json:"timestamp"`
	Fingerprint string            `json:"fingerprint"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

type Resource struct {
	Kind      string                 `json:"kind"`
	Name      string                 `json:"name"`
	Namespace string                 `json:"namespace"`
	State     map[string]interface{} `json:"state"`
}

type RootCauseAnalysis struct {
	Summary          string  `json:"summary"`
	Confidence       float64 `json:"confidence"`
	DetailedAnalysis string  `json:"detailedAnalysis"`
	Methodology      string  `json:"methodology"`
}

type AnalysisJustification struct {
	WhyThisRootCause        string                   `json:"whyThisRootCause"`
	AlternativeHypotheses   []AlternativeHypothesis  `json:"alternativeHypotheses"`
}

type AlternativeHypothesis struct {
	Hypothesis string  `json:"hypothesis"`
	Confidence float64 `json:"confidence"`
	Rejected   string  `json:"rejected"`
}

type Remediation struct {
	Rank             int      `json:"rank"`
	Confidence       float64  `json:"confidence"`
	TimeToResolution string   `json:"timeToResolution"`
	RiskLevel        string   `json:"riskLevel"` // "low", "medium", "high"
	CombinedScore    float64  `json:"combinedScore"`
	Action           string   `json:"action"`
	Description      string   `json:"description"`
	Pros             []string `json:"pros"`
	Cons             []string `json:"cons"`
	Tradeoffs        string   `json:"tradeoffs"`
}

type NextSteps struct {
	EscalationHistory EscalationHistory `json:"escalationHistory"`
	GitopsPRLink      string            `json:"gitopsPRLink,omitempty"`
	MonitoringLinks   []MonitoringLink  `json:"monitoringLinks"`
}

type EscalationHistory struct {
	RecentEvents       []EscalationEvent `json:"recentEvents"`
	HistoricalSummary  string            `json:"historicalSummary"`
}

type EscalationEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Alert     string    `json:"alert"`
	Action    string    `json:"action"`
	Outcome   string    `json:"outcome,omitempty"`
}

type MonitoringLink struct {
	Type        string `json:"type"` // "Grafana Dashboard", "Prometheus Query", "Kubernetes Dashboard"
	URL         string `json:"url"`
	Description string `json:"description"`
}
```

### **Escalation Notification Response**

```go
type EscalationNotificationResponse struct {
	NotificationID       string                 `json:"notificationId"`
	Status               string                 `json:"status"` // "delivered", "partial", "failed"
	Channels             map[string]ChannelStatus `json:"channels"`
	SanitizationApplied  []string               `json:"sanitizationApplied"`
	DataFreshness        DataFreshness          `json:"dataFreshness"`
}

type ChannelStatus struct {
	Status      string    `json:"status"` // "delivered", "failed", "skipped"
	Timestamp   time.Time `json:"timestamp"`
	PayloadSize string    `json:"payloadSize,omitempty"`
	ThreadTs    string    `json:"threadTs,omitempty"` // Slack thread timestamp
	Error       string    `json:"error,omitempty"`
}

type DataFreshness struct {
	GatheredAt time.Time `json:"gatheredAt"`
	AgeSeconds int       `json:"ageSeconds"`
	IsFresh    bool      `json:"isFresh"` // true if age < 60 seconds
}
```

### **Simple Notification Request**

```go
type SimpleNotificationRequest struct {
	Recipient string                 `json:"recipient"`
	Channels  []string               `json:"channels"`
	Subject   string                 `json:"subject"`
	Message   string                 `json:"message"`
	Severity  string                 `json:"severity"` // "info", "warning", "error"
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type SimpleNotificationResponse struct {
	NotificationID string                    `json:"notificationId"`
	Status         string                    `json:"status"`
	Channels       map[string]ChannelStatus  `json:"channels"`
}
```

---

## 🏷️ Routing Labels (BR-NOT-065)

The Notification Service supports **label-based routing** for notifications. When `NotificationRequest.spec.channels` is NOT specified, the service uses labels on the CRD to determine which channels to route to.

### **Supported Routing Labels**

All labels use the `kubernaut.ai/` domain (NOT `kubernaut.io/`).

| Label Key | Purpose | Values | Example |
|-----------|---------|--------|---------|
| `kubernaut.ai/notification-type` | Notification type routing | `escalation`, `approval_required`, `completed`, `failed`, `status_update` | Route approvals to PagerDuty |
| `kubernaut.ai/severity` | Severity-based routing | `critical`, `high`, `medium`, `low` | Route critical to PagerDuty |
| `kubernaut.ai/environment` | Environment-based routing | `production`, `staging`, `development`, `test` | Route prod to oncall |
| `kubernaut.ai/priority` | Priority-based routing | `P0`, `P1`, `P2`, `P3` | Route P0 to all channels |
| `kubernaut.ai/namespace` | Namespace-based routing | Any Kubernetes namespace | Route payment-ns to finance |
| `kubernaut.ai/component` | Source component routing | `remediation-orchestrator`, `workflow-execution`, etc. | Route by source |
| `kubernaut.ai/remediation-request` | Correlation routing | RemediationRequest CRD name | Link to parent remediation |
| `kubernaut.ai/skip-reason` | WFE skip reason routing | `PreviousExecutionFailed`, `ExhaustedRetries`, `ResourceBusy`, `RecentlyRemediated` | Route execution failures to PagerDuty |

### **Skip-Reason Label (DD-WE-004 Integration)**

The `kubernaut.ai/skip-reason` label enables fine-grained routing based on WorkflowExecution skip reasons:

| Skip Reason | Severity | Recommended Routing | Rationale |
|-------------|----------|---------------------|-----------|
| `PreviousExecutionFailed` | **CRITICAL** | PagerDuty (immediate) | Cluster state unknown - manual intervention required |
| `ExhaustedRetries` | HIGH | Slack (#ops channel) | Infrastructure issues - team awareness required |
| `ResourceBusy` | LOW | Console/Bulk | Temporary - auto-resolves |
| `RecentlyRemediated` | LOW | Console/Bulk | Temporary - auto-resolves |

**Example Routing Configuration** (Alertmanager-compatible per BR-NOT-066):
```yaml
route:
  routes:
    # CRITICAL: Execution failures → PagerDuty
    - match:
        kubernaut.ai/skip-reason: PreviousExecutionFailed
      receiver: pagerduty-oncall

    # HIGH: Exhausted retries → Slack
    - match:
        kubernaut.ai/skip-reason: ExhaustedRetries
      receiver: slack-ops

    # LOW: Temporary conditions → Console only
    - match_re:
        kubernaut.ai/skip-reason: "^(ResourceBusy|RecentlyRemediated)$"
      receiver: console-only

  receiver: default-slack

receivers:
  - name: pagerduty-oncall
    pagerduty_configs:
      - service_key: ${PAGERDUTY_KEY}
  - name: slack-ops
    slack_configs:
      - channel: '#kubernaut-ops'
  - name: console-only
    console_config:
      enabled: true
  - name: default-slack
    slack_configs:
      - channel: '#kubernaut-alerts'
```

### **Go Constants** (`pkg/notification/routing/labels.go`)

```go
// Label keys
const (
    LabelNotificationType   = "kubernaut.ai/notification-type"
    LabelSeverity          = "kubernaut.ai/severity"
    LabelEnvironment       = "kubernaut.ai/environment"
    LabelPriority          = "kubernaut.ai/priority"
    LabelNamespace         = "kubernaut.ai/namespace"
    LabelComponent         = "kubernaut.ai/component"
    LabelRemediationRequest = "kubernaut.ai/remediation-request"
    LabelSkipReason        = "kubernaut.ai/skip-reason"
)

// Skip reason values (DD-WE-004)
const (
    SkipReasonPreviousExecutionFailed = "PreviousExecutionFailed"  // CRITICAL
    SkipReasonExhaustedRetries        = "ExhaustedRetries"         // HIGH
    SkipReasonResourceBusy            = "ResourceBusy"             // LOW
    SkipReasonRecentlyRemediated      = "RecentlyRemediated"       // LOW
)
```

### **Routing Resolution Priority**

1. If `spec.channels` is specified → Use those channels directly
2. If `spec.channels` is empty → Resolve from routing rules based on labels
3. If no routing rules match → Use default receiver (console)

**Related Documentation**:
- [BR-NOT-065: Channel Routing Based on Labels](./BUSINESS_REQUIREMENTS.md#br-not-065-channel-routing-based-on-labels)
- [BR-NOT-066: Alertmanager-Compatible Configuration Format](./BUSINESS_REQUIREMENTS.md#br-not-066-alertmanager-compatible-configuration-format)
- [DD-WE-004: Exponential Backoff Cooldown](../../../architecture/decisions/DD-WE-004-exponential-backoff-cooldown.md)
- [Cross-Team Notice](../../../../handoff/NOTICE_WE_EXPONENTIAL_BACKOFF_DD_WE_004.md)

---

## 🔒 Authentication

**Method**: Kubernetes TokenReviewer

**Request Header**:
```
Authorization: Bearer <kubernetes-service-account-token>
```

**Token Validation**:
1. Extract Bearer token from `Authorization` header
2. Call Kubernetes TokenReview API
3. Verify token is valid and not expired
4. Extract authenticated user/service account
5. Allow request if valid, return 401 if invalid

**Implementation**:
```go
import (
	authv1 "k8s.io/api/authentication/v1"
	"k8s.io/client-go/kubernetes"
)

func (s *NotificationService) ValidateToken(token string) (*authv1.UserInfo, error) {
	tokenReview := &authv1.TokenReview{
		Spec: authv1.TokenReviewSpec{
			Token: token,
		},
	}

	result, err := s.kubeClient.AuthenticationV1().TokenReviews().Create(context.TODO(), tokenReview, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}

	if !result.Status.Authenticated {
		return nil, fmt.Errorf("token not authenticated")
	}

	return &result.Status.User, nil
}
```

---

## 📈 Rate Limiting

**Per Recipient**: 100 requests/minute
**Per Service Account**: 500 requests/minute
**Global**: 5,000 requests/minute

**Response Header** (on rate limit):
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1696260666
Retry-After: 60
```

---

## 🎯 Performance Targets

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Latency (p50)** | < 200ms | Time from request to response |
| **Latency (p95)** | < 500ms | Includes external channel delays |
| **Latency (p99)** | < 1s | Worst case with retries |
| **Throughput** | 100 req/s | Sustained load |
| **Availability** | 99.5% | Uptime target |

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: 2025-10-06
**Status**: ✅ Complete Specification

