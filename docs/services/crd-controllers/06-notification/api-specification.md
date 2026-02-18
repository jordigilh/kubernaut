# Notification Service - API Specification

**Version**: 2.3
**Last Updated**: 2025-12-11
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

### **NotificationRequest CRD Types**

```go
// NotificationRequest is the Schema for the notificationrequests API
type NotificationRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NotificationRequestSpec   `json:"spec,omitempty"`
	Status NotificationRequestStatus `json:"status,omitempty"`
}

type NotificationRequestSpec struct {
	// Type: "escalation" | "simple" | "approval" | "manual-review"
	Type string `json:"type"`

	// Recipient: email address or user identifier
	Recipient string `json:"recipient,omitempty"`

	// Channels: explicit channel list (if empty, uses routing rules)
	Channels []string `json:"channels,omitempty"`

	// Priority: "critical" | "high" | "normal" | "low"
	Priority string `json:"priority,omitempty"`

	// Subject: notification subject line
	Subject string `json:"subject,omitempty"`

	// Body: notification body content
	Body string `json:"body"`

	// Metadata: additional structured data
	Metadata *runtime.RawExtension `json:"metadata,omitempty"`
}

type NotificationRequestStatus struct {
	// Phase: Current lifecycle phase
	// Values: "Pending" | "Sending" | "Sent" | "PartiallySent" | "Failed"
	Phase string `json:"phase,omitempty"`

	// DeliveryAttempts: Record of all delivery attempts
	DeliveryAttempts []DeliveryAttempt `json:"deliveryAttempts,omitempty"`

	// FailureReason: Human-readable failure reason
	FailureReason string `json:"failureReason,omitempty"`

	// LastAttemptTime: Timestamp of most recent delivery attempt
	LastAttemptTime *metav1.Time `json:"lastAttemptTime,omitempty"`

	// Conditions: Standard Kubernetes conditions for status reporting
	// Supported conditions:
	//   - RoutingResolved (BR-NOT-069): Routing rule resolution status
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type DeliveryAttempt struct {
	Channel   string       `json:"channel"`
	Timestamp metav1.Time  `json:"timestamp"`
	Success   bool         `json:"success"`
	Error     string       `json:"error,omitempty"`
	Attempt   int          `json:"attempt"`
}
```

#### **Kubernetes Conditions (BR-NOT-069)**

The `Status.Conditions` field follows standard Kubernetes conventions:

**RoutingResolved Condition**:
- **Type**: `RoutingResolved`
- **Status**: `True` | `False` | `Unknown`
- **Reason**: `RoutingRuleMatched` | `RoutingFallback` | `RoutingError`
- **Message**: Human-readable description (e.g., "Matched rule 'critical-alerts' → channels: [pagerduty, slack]")

**Example kubectl Output**:
```bash
$ kubectl describe notificationrequest notif-abc123
...
Status:
  Phase: Sent
  Conditions:
    Type:                RoutingResolved
    Status:              True
    Last Transition Time: 2025-12-11T10:30:00Z
    Reason:              RoutingRuleMatched
    Message:             Matched rule 'critical-alerts' → channels: [pagerduty, slack]
    Observed Generation: 1
```

**Related Documentation**:
- Full specification: [BR-NOT-069](../../../requirements/BR-NOT-069-routing-rule-visibility-conditions.md)
- Routing rules: BR-NOT-065 (Channel Routing Based on Spec Fields)

---

## 🏷️ Routing Spec Fields (BR-NOT-065, Issue #91)

The Notification Service supports **spec-field-based routing** for notifications. When `NotificationRequest.spec.channels` is NOT specified, the service uses spec fields and `spec.metadata` to determine which channels to route to.

**Issue #91**: `kubernaut.ai/*` metadata labels were migrated to immutable CRD spec fields. Routing config keys are simplified (e.g., `severity` not `kubernaut.ai/severity`). Field selectors (`+kubebuilder:selectablefield`) replace label-based filtering.

### **Supported Routing Attributes**

Routing uses spec fields and `spec.metadata` keys. Config match keys use simplified names (no `kubernaut.ai/` prefix).

| Spec Field / Metadata Key | Config Match Key | Purpose | Values | Example |
|---------------------------|------------------|---------|--------|---------|
| `spec.type` | `type` | Notification type routing | `escalation`, `approval`, `completion`, `manual-review`, `status-update` | Route approvals to PagerDuty |
| `spec.severity` | `severity` | Severity-based routing | `critical`, `high`, `medium`, `low` | Route critical to PagerDuty |
| `spec.metadata["environment"]` | `environment` | Environment-based routing | `production`, `staging`, `development`, `test` | Route prod to oncall |
| `spec.priority` | `priority` | Priority-based routing | `critical`, `high`, `medium`, `low` | Route P0 to all channels |
| `spec.metadata["namespace"]` | `namespace` | Namespace-based routing | Any Kubernetes namespace | Route payment-ns to finance |
| `spec.phase` | `phase` | Phase that triggered notification | `signal-processing`, `ai-analysis`, `workflow-execution`, etc. | Route by phase |
| `spec.reviewSource` | `review-source` | Manual review source | `WorkflowResolutionFailed`, `ExhaustedRetries`, etc. | Route by review trigger |
| `spec.remediationRequestRef` | (correlation) | Parent remediation link | ObjectReference | ownerRef/spec ref sufficient |
| `spec.metadata["skip-reason"]` | `skip-reason` | WFE skip reason routing | `PreviousExecutionFailed`, `ExhaustedRetries`, `ResourceBusy`, `RecentlyRemediated` | Route execution failures to PagerDuty |
| `spec.metadata["investigation-outcome"]` | `investigation-outcome` | HolmesGPT outcome (BR-HAPI-200) | `resolved`, `inconclusive`, `workflow_selected` | Route inconclusive to ops for review |

**Removed** (Issue #91): `kubernaut.ai/component` (ownerRef sufficient), `kubernaut.ai/remediation-request` (use `spec.remediationRequestRef`).

### **Skip-Reason Routing (DD-WE-004 Integration)**

The `spec.metadata["skip-reason"]` attribute enables fine-grained routing based on WorkflowExecution skip reasons:

| Skip Reason | Severity | Recommended Routing | Rationale |
|-------------|----------|---------------------|-----------|
| `PreviousExecutionFailed` | **CRITICAL** | PagerDuty (immediate) | Cluster state unknown - manual intervention required |
| `ExhaustedRetries` | HIGH | Slack (#ops channel) | Infrastructure issues - team awareness required |
| `ResourceBusy` | LOW | Console/Bulk | Temporary - auto-resolves |
| `RecentlyRemediated` | LOW | Console/Bulk | Temporary - auto-resolves |

**Example Routing Configuration** (Alertmanager-compatible per BR-NOT-066, Issue #91):
```yaml
route:
  routes:
    # CRITICAL: Execution failures → PagerDuty
    - match:
        skip-reason: PreviousExecutionFailed
      receiver: pagerduty-oncall

    # HIGH: Exhausted retries → Slack
    - match:
        skip-reason: ExhaustedRetries
      receiver: slack-ops

    # LOW: Temporary conditions → Console only
    - match_re:
        skip-reason: "^(ResourceBusy|RecentlyRemediated)$"
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

### **Investigation-Outcome Routing (BR-HAPI-200)**

The `spec.metadata["investigation-outcome"]` attribute enables routing based on HolmesGPT investigation results:

| Investigation Outcome | Scenario | Recommended Routing | Rationale |
|-----------------------|----------|---------------------|-----------|
| `resolved` | Problem self-resolved before AI intervention | **Skip notification** (null-receiver) | No action needed - prevent alert fatigue |
| `inconclusive` | LLM cannot determine root cause | Slack (#ops channel) | Human review required |
| `workflow-selected` | Normal workflow execution | Continue to default routing | Standard flow |

**Example Routing Configuration** (Alertmanager-compatible, Issue #91):
```yaml
route:
  routes:
    # Self-resolved: Skip notification by default
    - match:
        investigation-outcome: resolved
      receiver: null-receiver  # No notification

    # Inconclusive: Route to ops for manual review
    - match:
        investigation-outcome: inconclusive
      receiver: slack-ops

    # Workflow selected: Fall through to normal routing
    - match:
        investigation-outcome: workflow-selected
      continue: true

  receiver: default-slack

receivers:
  - name: null-receiver  # Drops notifications silently
  - name: slack-ops
    slack_configs:
      - channel: '#kubernaut-ops'
  - name: default-slack
    slack_configs:
      - channel: '#kubernaut-alerts'
```

**Related Documentation**:
- [BR-HAPI-200](../../../../handoff/NOTICE_INVESTIGATION_INCONCLUSIVE_BR_HAPI_200.md) - Cross-team notice
- [BR-HAPI-197](../../../../docs/requirements/BR-HAPI-197-needs-human-review-field.md) - Parent requirement

---

### **Go Constants** (`pkg/notification/routing/attributes.go`)

```go
// Routing attribute keys (Issue #91: spec fields + spec.metadata)
const (
    AttrType                = "type"                 // spec.type
    AttrSeverity            = "severity"             // spec.severity
    AttrEnvironment         = "environment"         // spec.metadata["environment"]
    AttrPhase               = "phase"                // spec.phase
    AttrReviewSource        = "review-source"        // spec.reviewSource
    AttrPriority            = "priority"             // spec.priority
    AttrNamespace           = "namespace"            // spec.metadata["namespace"]
    AttrSkipReason          = "skip-reason"          // spec.metadata["skip-reason"]
    AttrInvestigationOutcome = "investigation-outcome" // spec.metadata["investigation-outcome"]
)

// Skip reason values (DD-WE-004)
const (
    SkipReasonPreviousExecutionFailed = "PreviousExecutionFailed"  // CRITICAL
    SkipReasonExhaustedRetries        = "ExhaustedRetries"         // HIGH
    SkipReasonResourceBusy            = "ResourceBusy"            // LOW
    SkipReasonRecentlyRemediated      = "RecentlyRemediated"       // LOW
)

// Investigation outcome values (BR-HAPI-200)
const (
    InvestigationOutcomeResolved         = "resolved"          // No action needed
    InvestigationOutcomeInconclusive     = "inconclusive"      // Human review required
    InvestigationOutcomeWorkflowSelected = "workflow_selected" // Normal flow
)
```

### **Routing Resolution Priority**

1. If `spec.channels` is specified → Use those channels directly
2. If `spec.channels` is empty → Resolve from routing rules based on spec fields and spec.metadata
3. If no routing rules match → Use default receiver (console)

### **Field Selectors**

NotificationRequest supports `+kubebuilder:selectablefield` for server-side filtering (replaces label-based filtering):
- `spec.remediationRequestRef.name`
- `spec.type`
- `spec.severity`

**Related Documentation**:
- [BR-NOT-065: Channel Routing Based on Spec Fields](./BUSINESS_REQUIREMENTS.md#br-not-065-channel-routing-based-on-spec-fields)
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

