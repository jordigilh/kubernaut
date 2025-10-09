# Notification Service - Payload Schemas

**Date**: October 5, 2025
**Status**: ✅ **AUTHORITATIVE SCHEMA**
**Purpose**: Unified notification payload for all CRD controllers
**Confidence**: 90%

---

## 🎯 Overview

This document defines the **authoritative notification payload schema** used by all CRD controllers when sending escalation notifications to the Notification Service.

**Consumers**:
- Remediation Orchestrator (timeout escalations)
- AIAnalysis Controller (rejection escalations)
- WorkflowExecution Controller (failure escalations)
- RemediationProcessor (validation failure escalations)

**Provider**:
- Notification Service (`POST /api/v1/notify/escalation`)

---

## 📋 Escalation Notification Payload

### HTTP Endpoint

**URL**: `POST http://notification-service:8080/api/v1/notify/escalation`
**Content-Type**: `application/json`
**Authentication**: Bearer token (JWT via Kubernetes TokenReviewer)
**Timeout**: 5 seconds
**Retry**: 3 attempts with exponential backoff

---

### Request Schema

```go
// pkg/notification/types.go
package notification

import (
    "time"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EscalationRequest is the unified payload for escalation notifications
// Sent by: Remediation Orchestrator, AIAnalysis, WorkflowExecution, RemediationProcessor
// Received by: Notification Service
type EscalationRequest struct {
    // ========================================
    // SOURCE CONTEXT (REQUIRED)
    // Identifies which remediation is being escalated
    // ========================================

    // Name of the RemediationRequest CRD
    RemediationRequestName string `json:"remediationRequestName" validate:"required"`

    // Namespace of the RemediationRequest CRD
    RemediationRequestNamespace string `json:"remediationRequestNamespace" validate:"required"`

    // Which controller is sending this escalation
    EscalatingController string `json:"escalatingController" validate:"required,oneof=central-controller ai-analysis workflow-execution remediation-processor"`

    // ========================================
    // ALERT CONTEXT (REQUIRED)
    // Original signal information
    // ========================================

    // Unique alert fingerprint for correlation
    AlertFingerprint string `json:"alertFingerprint" validate:"required"`

    // Human-readable alert name
    AlertName string `json:"alertName" validate:"required"`

    // Severity level
    Severity string `json:"severity" validate:"required,oneof=critical warning info"`

    // Environment classification
    Environment string `json:"environment" validate:"required,oneof=prod staging dev"`

    // Gateway-assigned priority
    Priority string `json:"priority" validate:"required,oneof=P0 P1 P2"`

    // Signal type
    SignalType string `json:"signalType" validate:"required"`

    // ========================================
    // RESOURCE CONTEXT (REQUIRED)
    // What resource is affected
    // ========================================

    // Target namespace
    Namespace string `json:"namespace" validate:"required"`

    // Target resource
    Resource ResourceIdentifier `json:"resource" validate:"required"`

    // ========================================
    // ESCALATION CONTEXT (REQUIRED)
    // Why are we escalating
    // ========================================

    // Reason for escalation
    EscalationReason string `json:"escalationReason" validate:"required,oneof=timeout rejected failed validation-error"`

    // Which phase escalated
    EscalationPhase string `json:"escalationPhase" validate:"required,oneof=remediation-processing ai-analysis workflow-execution kubernetes-execution"`

    // When escalation occurred
    EscalationTime time.Time `json:"escalationTime" validate:"required"`

    // Additional escalation details (phase-specific)
    EscalationDetails map[string]interface{} `json:"escalationDetails,omitempty"`

    // ========================================
    // TEMPORAL CONTEXT (REQUIRED)
    // Timeline information
    // ========================================

    // When the original alert started firing
    AlertFiringTime time.Time `json:"alertFiringTime" validate:"required"`

    // When Gateway received the alert
    AlertReceivedTime time.Time `json:"alertReceivedTime" validate:"required"`

    // How long remediation has been running
    RemediationDuration string `json:"remediationDuration" validate:"required"` // e.g., "15m30s"

    // ========================================
    // AI ANALYSIS CONTEXT (OPTIONAL)
    // AI-generated insights (if available)
    // ========================================

    // AI analysis summary (if AIAnalysis phase completed)
    AIAnalysis *AIAnalysisSummary `json:"aiAnalysis,omitempty"`

    // ========================================
    // RECOMMENDED ACTIONS (OPTIONAL)
    // Suggested next steps
    // ========================================

    // Actions the human operator should consider
    RecommendedActions []RecommendedAction `json:"recommendedActions,omitempty"`

    // ========================================
    // NOTIFICATION ROUTING (REQUIRED)
    // How to deliver this notification
    // ========================================

    // Which channels to use
    Channels []string `json:"channels" validate:"required,min=1,dive,oneof=slack email pagerduty msteams"`

    // Notification urgency level
    Urgency string `json:"urgency" validate:"required,oneof=critical high normal low"`

    // ========================================
    // EXTERNAL LINKS (OPTIONAL)
    // Helpful debugging links
    // ========================================

    // Link to Alertmanager (for Prometheus alerts)
    AlertmanagerURL string `json:"alertmanagerURL,omitempty"`

    // Link to Grafana dashboard
    GrafanaURL string `json:"grafanaURL,omitempty"`

    // Link to Kubernetes resource in UI
    KubernetesUIURL string `json:"kubernetesUIURL,omitempty"`
}

// ResourceIdentifier identifies a Kubernetes resource
type ResourceIdentifier struct {
    Kind      string `json:"kind" validate:"required"`
    Name      string `json:"name" validate:"required"`
    Namespace string `json:"namespace" validate:"required"`
}

// AIAnalysisSummary contains AI-generated insights
type AIAnalysisSummary struct {
    // Root cause identified by AI
    RootCause string `json:"rootCause" validate:"required"`

    // Confidence level (0.0-1.0)
    Confidence float64 `json:"confidence" validate:"required,gte=0,lte=1"`

    // Top recommendation from AI
    TopRecommendation string `json:"topRecommendation" validate:"required"`

    // Why AI is confident in this recommendation
    Justification string `json:"justification,omitempty"`

    // Alternative hypotheses considered
    AlternativeHypotheses []string `json:"alternativeHypotheses,omitempty"`
}

// RecommendedAction is a suggested next step
type RecommendedAction struct {
    // Action description
    Action string `json:"action" validate:"required"`

    // Action category
    Category string `json:"category" validate:"required,oneof=investigate remediate escalate rollback"`

    // Expected time to complete
    EstimatedTime string `json:"estimatedTime,omitempty"` // e.g., "5m", "30m"

    // Risk level of taking this action
    RiskLevel string `json:"riskLevel" validate:"required,oneof=low medium high"`

    // Confidence in this recommendation
    Confidence float64 `json:"confidence" validate:"gte=0,lte=1"`
}
```

---

### Response Schema

```go
// EscalationResponse is returned by Notification Service
type EscalationResponse struct {
    // Unique notification ID for tracking
    NotificationID string `json:"notificationId"`

    // Status of notification
    Status string `json:"status"` // "queued", "sent", "failed"

    // Channels successfully notified
    ChannelsNotified []string `json:"channelsNotified"`

    // Channels that failed
    ChannelsFailed []ChannelFailure `json:"channelsFailed,omitempty"`

    // When notification was processed
    ProcessedAt time.Time `json:"processedAt"`
}

type ChannelFailure struct {
    Channel string `json:"channel"`
    Error   string `json:"error"`
}
```

---

## 📝 Field Definitions

### Required Fields

All escalations **MUST** include:

| Field | Type | Description | Validation |
|-------|------|-------------|------------|
| `remediationRequestName` | string | CRD name | required |
| `remediationRequestNamespace` | string | CRD namespace | required |
| `escalatingController` | string | Which controller | required |
| `alertFingerprint` | string | Alert identifier | required |
| `alertName` | string | Human-readable name | required |
| `severity` | string | Severity level | `critical\|warning\|info` |
| `environment` | string | Environment | `prod\|staging\|dev` |
| `priority` | string | Priority level | `P0\|P1\|P2` |
| `signalType` | string | Signal source | required |
| `namespace` | string | Target namespace | required |
| `resource` | ResourceIdentifier | Target resource | required |
| `escalationReason` | string | Why escalating | enum |
| `escalationPhase` | string | Which phase | enum |
| `escalationTime` | time.Time | When escalated | required |
| `alertFiringTime` | time.Time | Alert start time | required |
| `alertReceivedTime` | time.Time | Gateway receive time | required |
| `remediationDuration` | string | How long running | required |
| `channels` | []string | Notification channels | min=1 |
| `urgency` | string | Urgency level | enum |

### Optional Fields

Controllers **MAY** include:

| Field | Type | Description | When to Include |
|-------|------|-------------|-----------------|
| `escalationDetails` | map | Phase-specific data | Always recommended |
| `aiAnalysis` | AIAnalysisSummary | AI insights | If AIAnalysis completed |
| `recommendedActions` | []RecommendedAction | Suggested steps | If actions available |
| `alertmanagerURL` | string | Prometheus link | For Prometheus alerts |
| `grafanaURL` | string | Dashboard link | If available |
| `kubernetesUIURL` | string | K8s UI link | If UI deployed |

---

## 🔍 Usage Examples

### Example 1: Remediation Orchestrator Timeout Escalation

```go
// pkg/remediationorchestrator/escalation.go
package remediationorchestrator

func (r *RemediationRequestReconciler) sendTimeoutEscalation(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    phase string,
) error {
    payload := notification.EscalationRequest{
        // Source context
        RemediationRequestName:      remediation.Name,
        RemediationRequestNamespace: remediation.Namespace,
        EscalatingController:        "central-controller",

        // Alert context (from CRD)
        AlertFingerprint: remediation.Spec.SignalFingerprint,
        AlertName:        remediation.Spec.AlertName,
        Severity:         remediation.Spec.Severity,
        Environment:      remediation.Spec.Environment,
        Priority:         remediation.Spec.Priority,
        SignalType:       remediation.Spec.SignalType,

        // Resource context
        Namespace: remediation.Spec.Namespace,
        Resource:  convertResourceIdentifier(remediation.Spec.Resource),

        // Escalation context
        EscalationReason: "timeout",
        EscalationPhase:  phase, // "remediation-processing", "ai-analysis", etc.
        EscalationTime:   time.Now(),
        EscalationDetails: map[string]interface{}{
            "timeoutDuration": "30m",
            "phase":           phase,
            "retryCount":      remediation.Status.RetryCount,
        },

        // Temporal context
        AlertFiringTime:   remediation.Spec.FiringTime.Time,
        AlertReceivedTime: remediation.Spec.ReceivedTime.Time,
        RemediationDuration: time.Since(remediation.CreationTimestamp.Time).String(),

        // Notification routing
        Channels: determineChannels(remediation.Spec.Priority),
        Urgency:  mapPriorityToUrgency(remediation.Spec.Priority),

        // External links
        AlertmanagerURL: remediation.Spec.AlertmanagerURL,
        GrafanaURL:      remediation.Spec.GrafanaURL,
    }

    return r.notificationClient.SendEscalation(ctx, payload)
}

func determineChannels(priority string) []string {
    switch priority {
    case "P0":
        return []string{"pagerduty", "slack", "email"}
    case "P1":
        return []string{"slack", "email"}
    case "P2":
        return []string{"email"}
    default:
        return []string{"email"}
    }
}

func mapPriorityToUrgency(priority string) string {
    switch priority {
    case "P0":
        return "critical"
    case "P1":
        return "high"
    case "P2":
        return "normal"
    default:
        return "low"
    }
}
```

---

### Example 2: AIAnalysis Rejection Escalation

```go
// pkg/aianalysis/escalation.go
package aianalysis

func (r *AIAnalysisReconciler) sendRejectionEscalation(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    aiAnalysis *aianalysisv1.AIAnalysis,
    rejectionReason string,
) error {
    payload := notification.EscalationRequest{
        // Source context
        RemediationRequestName:      remediation.Name,
        RemediationRequestNamespace: remediation.Namespace,
        EscalatingController:        "ai-analysis",

        // Alert context
        AlertFingerprint: remediation.Spec.SignalFingerprint,
        AlertName:        remediation.Spec.AlertName,
        Severity:         remediation.Spec.Severity,
        Environment:      remediation.Spec.Environment,
        Priority:         remediation.Spec.Priority,
        SignalType:       remediation.Spec.SignalType,

        // Resource context
        Namespace: remediation.Spec.Namespace,
        Resource:  convertResourceIdentifier(remediation.Spec.Resource),

        // Escalation context
        EscalationReason: "rejected",
        EscalationPhase:  "ai-analysis",
        EscalationTime:   time.Now(),
        EscalationDetails: map[string]interface{}{
            "rejectionReason": rejectionReason,
            "operator":        aiAnalysis.Status.LastModifiedBy,
            "rejectionTime":   aiAnalysis.Status.RejectionTime,
        },

        // Temporal context
        AlertFiringTime:     remediation.Spec.FiringTime.Time,
        AlertReceivedTime:   remediation.Spec.ReceivedTime.Time,
        RemediationDuration: time.Since(remediation.CreationTimestamp.Time).String(),

        // AI Analysis summary
        AIAnalysis: &notification.AIAnalysisSummary{
            RootCause:         aiAnalysis.Status.RootCause,
            Confidence:        aiAnalysis.Status.Confidence,
            TopRecommendation: aiAnalysis.Status.TopRecommendation,
            Justification:     aiAnalysis.Status.Justification,
            AlternativeHypotheses: aiAnalysis.Status.AlternativeHypotheses,
        },

        // Recommended actions (from AI analysis)
        RecommendedActions: convertRecommendations(aiAnalysis.Status.Recommendations),

        // Notification routing
        Channels: []string{"slack", "email"},
        Urgency:  "high",

        // External links
        AlertmanagerURL: remediation.Spec.AlertmanagerURL,
        GrafanaURL:      remediation.Spec.GrafanaURL,
    }

    return r.notificationClient.SendEscalation(ctx, payload)
}
```

---

### Example 3: WorkflowExecution Failure Escalation

```go
// pkg/workflowexecution/escalation.go
package workflowexecution

func (r *WorkflowExecutionReconciler) sendFailureEscalation(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    workflow *workflowexecutionv1.WorkflowExecution,
    failureReason string,
) error {
    payload := notification.EscalationRequest{
        // Source context
        RemediationRequestName:      remediation.Name,
        RemediationRequestNamespace: remediation.Namespace,
        EscalatingController:        "workflow-execution",

        // Alert context
        AlertFingerprint: remediation.Spec.SignalFingerprint,
        AlertName:        remediation.Spec.AlertName,
        Severity:         remediation.Spec.Severity,
        Environment:      remediation.Spec.Environment,
        Priority:         remediation.Spec.Priority,
        SignalType:       remediation.Spec.SignalType,

        // Resource context
        Namespace: remediation.Spec.Namespace,
        Resource:  convertResourceIdentifier(remediation.Spec.Resource),

        // Escalation context
        EscalationReason: "failed",
        EscalationPhase:  "workflow-execution",
        EscalationTime:   time.Now(),
        EscalationDetails: map[string]interface{}{
            "failureReason":  failureReason,
            "failedStep":     workflow.Status.FailedStep,
            "failedStepNum":  workflow.Status.FailedStepNum,
            "totalSteps":     workflow.Status.TotalSteps,
            "executionTime":  workflow.Status.ExecutionTime,
            "retryCount":     workflow.Status.RetryCount,
        },

        // Temporal context
        AlertFiringTime:     remediation.Spec.FiringTime.Time,
        AlertReceivedTime:   remediation.Spec.ReceivedTime.Time,
        RemediationDuration: time.Since(remediation.CreationTimestamp.Time).String(),

        // Recommended actions (from workflow)
        RecommendedActions: []notification.RecommendedAction{
            {
                Action:        "Review failed step logs",
                Category:      "investigate",
                EstimatedTime: "5m",
                RiskLevel:     "low",
                Confidence:    0.9,
            },
            {
                Action:        "Retry workflow execution",
                Category:      "remediate",
                EstimatedTime: "10m",
                RiskLevel:     "medium",
                Confidence:    0.6,
            },
        },

        // Notification routing
        Channels: []string{"slack", "email"},
        Urgency:  "high",

        // External links
        AlertmanagerURL:  remediation.Spec.AlertmanagerURL,
        GrafanaURL:       remediation.Spec.GrafanaURL,
        KubernetesUIURL:  buildKubernetesUIURL(remediation),
    }

    return r.notificationClient.SendEscalation(ctx, payload)
}
```

---

## 🔒 Security Considerations

### Sensitive Data Sanitization

**CRITICAL**: Controllers MUST sanitize sensitive data before sending notifications.

**Fields to Sanitize**:
- Alert labels/annotations (may contain API keys, tokens)
- Environment variables (may contain credentials)
- Command arguments (may contain passwords)
- ConfigMap/Secret references

**Implementation**:
```go
// pkg/notification/sanitizer.go
type Sanitizer struct {
    // Patterns to redact
    sensitivePatterns []string
}

func (s *Sanitizer) SanitizeEscalationRequest(req *EscalationRequest) {
    // Redact sensitive data in escalation details
    if req.EscalationDetails != nil {
        req.EscalationDetails = s.sanitizeMap(req.EscalationDetails)
    }

    // Redact sensitive data in AI analysis
    if req.AIAnalysis != nil {
        req.AIAnalysis.RootCause = s.redactSensitiveInfo(req.AIAnalysis.RootCause)
        req.AIAnalysis.TopRecommendation = s.redactSensitiveInfo(req.AIAnalysis.TopRecommendation)
    }
}

func (s *Sanitizer) redactSensitiveInfo(text string) string {
    // Redact patterns like: password=xxx, token=xxx, api_key=xxx
    for _, pattern := range s.sensitivePatterns {
        text = regexp.MustCompile(pattern).ReplaceAllString(text, "$1=REDACTED")
    }
    return text
}
```

---

## 📊 Payload Size Considerations

### Channel Limits

**CRITICAL**: Notification payload must respect channel limits to avoid truncation.

| Channel | Message Limit | Strategy |
|---------|--------------|----------|
| **Slack** | 40,000 characters (~40KB) | Use tiered notifications |
| **MS Teams** | 28KB | Use tiered notifications |
| **Email** | 10MB (best practice: <1MB) | Full payload OK |
| **PagerDuty** | 1024 characters | Summary only |
| **SMS** | 160 characters | Alert name + link only |

### Tiered Notification Strategy

**Level 1 (Inline)**: Critical info only (~5KB)
- Alert name, severity, priority
- Escalation reason
- Top recommendation
- Action buttons

**Level 2 (Expandable)**: Full analysis (~30KB)
- Complete AI analysis
- All recommended actions
- Escalation details
- Resource context

**Level 3 (Link)**: Detailed data (unlimited)
- Link to web UI with full context
- Complete CRD spec
- All historical data

**Implementation**:
```go
func (ns *NotificationService) formatForChannel(
    channel string,
    req *EscalationRequest,
) string {
    switch channel {
    case "slack", "msteams":
        return ns.formatTiered(req) // Levels 1-2 with expandable sections
    case "pagerduty":
        return ns.formatSummary(req) // Level 1 only
    case "email":
        return ns.formatFull(req) // All levels
    case "sms":
        return ns.formatMinimal(req) // Alert name + link
    default:
        return ns.formatTiered(req)
    }
}
```

---

## ✅ Validation Rules

### Request Validation

```go
// pkg/notification/validator.go
type Validator struct {
    validator *validator.Validate
}

func (v *Validator) ValidateEscalationRequest(req *EscalationRequest) error {
    // Struct tag validation
    if err := v.validator.Struct(req); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }

    // Business logic validation
    if req.EscalationTime.Before(req.AlertFiringTime) {
        return errors.New("escalation time cannot be before alert firing time")
    }

    if req.AIAnalysis != nil {
        if req.AIAnalysis.Confidence < 0 || req.AIAnalysis.Confidence > 1 {
            return errors.New("AI confidence must be between 0 and 1")
        }
    }

    return nil
}
```

---

## 🔗 Integration Checklist

### For CRD Controllers

- [ ] Import `pkg/notification` package
- [ ] Create `NotificationClient` interface
- [ ] Implement `sendEscalation()` helper
- [ ] Sanitize sensitive data before sending
- [ ] Map `Priority` → `Urgency`
- [ ] Determine channels based on priority
- [ ] Include AI analysis summary (if available)
- [ ] Add recommended actions
- [ ] Include external links (Alertmanager, Grafana)
- [ ] Handle notification failures gracefully
- [ ] Log notification success/failure
- [ ] Update CRD status with notification status

### For Notification Service

- [ ] Implement `POST /api/v1/notify/escalation` endpoint
- [ ] Validate incoming requests against schema
- [ ] Implement tiered formatting per channel
- [ ] Respect channel size limits
- [ ] Return proper HTTP status codes
- [ ] Generate unique notification IDs
- [ ] Track notification delivery status
- [ ] Handle channel failures gracefully
- [ ] Implement retry logic for transient failures
- [ ] Log all notification attempts

---

## 📈 Metrics

### For Controllers (Recommended)

```go
var (
    escalationsSent = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kubernaut_escalations_sent_total",
            Help: "Total number of escalations sent to Notification Service",
        },
        []string{"controller", "reason", "priority", "status"},
    )

    escalationDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "kubernaut_escalation_send_duration_seconds",
            Help: "Time to send escalation notification",
        },
        []string{"controller", "status"},
    )
)
```

---

## 🎯 Success Criteria

- [x] Unified schema defined for all controllers
- [x] Required fields documented
- [x] Optional fields documented
- [x] Validation rules specified
- [x] Usage examples provided for each controller
- [x] Response schema defined
- [x] Security considerations documented
- [x] Payload size considerations addressed
- [x] Integration checklist created

---

## 📚 Related Documents

**CRD Controllers**:
- `docs/services/crd-controllers/05-remediationorchestrator/integration-points.md`
- `docs/services/crd-controllers/02-aianalysis/overview.md`
- `docs/services/crd-controllers/03-workflowexecution/integration-points.md`

**Notification Service**:
- `docs/services/stateless/06-notification-service.md`

**Requirements**:
- `docs/services/ESCALATION_NOTIFICATION_IMPACT_TRIAGE.md`
- `docs/services/crd-controllers/ESCALATION_NOTIFICATION_REQUIREMENTS_TRIAGE.md`

---

**Status**: ✅ **AUTHORITATIVE SCHEMA - READY FOR IMPLEMENTATION**
**Confidence**: 90% (Comprehensive schema covering all use cases)
**Next Step**: Update CRD controller integration docs to reference this schema
