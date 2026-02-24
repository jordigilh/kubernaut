# Notification Service - Integration Points

**Version**: 2.0
**Last Updated**: November 21, 2025
**Service Type**: CRD Controller (NotificationRequest CRD)
**Status**: ✅ PRODUCTION-READY with ADR-034 Audit Integration

---

## 📋 Overview

Integration points for the Notification Service, documenting all upstream clients, downstream dependencies, and data flows.

### **Key Integrations (v2.0)**

1. **Upstream**: Other CRD controllers create `NotificationRequest` CRDs
2. **Downstream**: Slack/Console/Email delivery services
3. **Audit**: ADR-034 unified `audit_events` table via fire-and-forget writes
4. **DLQ**: Redis Streams for audit write error recovery
5. **Observability**: Prometheus metrics + structured logging

---

## 🔗 **Upstream Clients** (Services Calling Notification)

**Business Requirements**: BR-NOT-026 to BR-NOT-037 (Escalation notifications with comprehensive context)

### **1. Remediation Orchestrator** (CRD Controller)

**Use Case**: Timeout escalations when remediation exceeds SLA
**Related BRs**: BR-NOT-026, BR-NOT-032 (Actionable next steps from last 5 escalation events)

**Integration Pattern**:
```go
// internal/controller/remediationorchestrator/escalation.go
package controller

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
)

func (r *Remediation OrchestratorReconciler) EscalateTimeout(ctx context.Context, req *RemediationRequest) error {
    payload := map[string]interface{}{
        "recipient": "sre-oncall@company.com",
        "channels":  []string{"email", "slack", "pagerduty"},
        "payload": map[string]interface{}{
            "alert": map[string]interface{}{
                "name":     req.Spec.AlertName,
                "severity": "critical",
            },
            // ... full escalation payload
        },
    }

    body, _ := json.Marshal(payload)
    httpReq, _ := http.NewRequest("POST",
        "http://notification-service.kubernaut-system.svc.cluster.local:8080/api/v1/notify/escalation",
        bytes.NewReader(body))

    httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", r.getServiceAccountToken()))
    httpReq.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(httpReq)
    if err != nil {
        return fmt.Errorf("escalation failed: %w", err)
    }

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("escalation rejected: %d", resp.StatusCode)
    }

    return nil
}
```

**Trigger**: RemediationRequest exceeds timeout (e.g., 15 minutes)

---

### **2. AI Analysis Controller** (CRD Controller)

**Use Case**: Analysis-triggered escalations (low confidence, multiple hypotheses)
**Related BRs**: BR-NOT-028, BR-NOT-029 (Root cause analysis with justification and confidence)

**Integration Pattern**:
```go
// internal/controller/aianalysis/notification.go
func (r *AIAnalysisReconciler) NotifyLowConfidence(ctx context.Context, analysis *AIAnalysis) error {
    if analysis.Spec.Confidence < 0.7 {
        // Escalate low-confidence analysis
        return r.notificationClient.SendEscalation(ctx, &EscalationRequest{
            Recipient: "ai-team@company.com",
            Channels:  []string{"email", "slack"},
            Payload:   BuildEscalationPayload(analysis),
        })
    }
    return nil
}
```

**Trigger**: AI confidence < 70% or >3 alternative hypotheses

---

### **3. Workflow Execution Controller** (CRD Controller)

**Use Case**: Workflow execution failures or safety violations
**Related BRs**: BR-NOT-027, BR-NOT-030, BR-NOT-031 (Impacted resources, recommended remediations with pros/cons)

**Integration Pattern**:
```go
// internal/controller/workflowexecution/escalation.go
func (r *WorkflowExecutionReconciler) HandleExecutionFailure(ctx context.Context, workflow *WorkflowExecution) error {
    return r.notificationClient.SendEscalation(ctx, &EscalationRequest{
        Recipient: "workflow-ops@company.com",
        Channels:  []string{"slack", "pagerduty"},
        Payload:   BuildWorkflowFailurePayload(workflow),
    })
}
```

**Trigger**: Workflow step fails after 3 retries

---

### **4. Kubernetes Executor** (CRD Controller)

**Use Case**: Critical action execution failures or safety check violations
**Related BRs**: BR-NOT-026, BR-NOT-033 (Comprehensive context, formatted for quick decision-making)

**Integration Pattern**:
```go
// internal/controller/kubernetesexecutor/notification.go
func (r *KubernetesExecutorReconciler) NotifySafetyViolation(ctx context.Context, action *KubernetesAction) error { // DEPRECATED - ADR-025
    return r.notificationClient.SendEscalation(ctx, &EscalationRequest{
        Recipient: "platform-ops@company.com",
        Channels:  []string{"email", "slack", "pagerduty"},
        Severity:  "critical",
        Payload:   BuildSafetyViolationPayload(action),
    })
}
```

**Trigger**: Safety check fails or dangerous action detected

---

## 🔽 **Downstream Dependencies** (External Services)

**Business Requirements**: BR-NOT-001 to BR-NOT-005 (Multi-channel delivery), BR-NOT-036 (Channel-specific formatting), BR-NOT-062 (Unified Audit), BR-NOT-063 (Graceful Degradation)

### **1. Data Storage Service** (Audit Integration) - NEW v2.0

**Business Requirements**: BR-NOT-062 (Unified Audit Table), BR-NOT-063 (Graceful Degradation)
**Service**: Data Storage Service (ADR-034 unified audit table)
**Endpoint**: `http://datastorage-service.kubernaut-system.svc.cluster.local:8080/api/v1/audit/events`
**Authentication**: Service account token
**Integration Pattern**: Fire-and-forget async writes via buffered audit store
**Performance**: <1ms overhead (non-blocking)
**Reliability**: DLQ fallback with Redis Streams (zero audit loss)

**Audit Events Generated**:
- `notification.message.sent` - Successful delivery
- `notification.message.failed` - Delivery failure
- `notification.message.acknowledged` - User acknowledgment
- `notification.message.escalated` - Priority escalation

**Configuration**:
```go
// cmd/notification/main.go
auditStore := audit.NewBufferedStore(
    audit.NewHTTPDataStorageClient(dataStorageURL, httpClient),
    audit.Config{
        BufferSize:    1000,
        BatchSize:     10,
        FlushInterval: 100 * time.Millisecond,
        MaxRetries:    3,
    },
    "notification",
    logger,
)
```

**Integration Details**: See [DD-NOT-001](./DD-NOT-001-ADR034-AUDIT-INTEGRATION-v2.0-FULL.md)

---

### **2. Email (SMTP)**

**Business Requirements**: BR-NOT-001 (Email notifications with rich formatting), BR-NOT-036 (1MB limit)
**Service**: Company SMTP server
**Endpoint**: `smtp.company.com:587` (TLS)
**Authentication**: Username/password from Secret
**Rate Limit**: 10 emails/minute
**Payload Size**: 1MB max

**Configuration**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: notification-email-config
  namespace: kubernaut-system
type: Opaque
stringData:
  smtp-host: "smtp.company.com"
  smtp-port: "587"
  smtp-username: "alerts@company.com"
  smtp-password: "<redacted>"
```

---

### **2. Slack**

**Business Requirements**: BR-NOT-002 (Slack integration for team collaboration), BR-NOT-036 (40KB limit)
**Service**: Slack Incoming Webhooks
**Endpoint**: `https://hooks.slack.com/services/<webhook-id>`
**Authentication**: Webhook URL (secret)
**Rate Limit**: 1 message/second
**Payload Size**: 40KB max

**Configuration**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: notification-slack-config
  namespace: kubernaut-system
type: Opaque
stringData:
  webhook-url: "https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXX"
```

---

### **3. Microsoft Teams**

**Business Requirements**: BR-NOT-005 (Teams & chat platforms integration), BR-NOT-036 (28KB limit)
**Service**: Teams Incoming Webhooks
**Endpoint**: `https://outlook.office.com/webhook/<webhook-id>`
**Authentication**: Webhook URL (secret)
**Rate Limit**: 4 messages/second
**Payload Size**: 28KB max

---

### **4. SMS (Twilio)**

**Business Requirements**: BR-NOT-004 (SMS notifications for critical alerts), BR-NOT-036 (160 char limit)
**Service**: Twilio SMS API
**Endpoint**: `https://api.twilio.com/2010-04-01/Accounts/<account-sid>/Messages.json`
**Authentication**: Account SID + Auth Token
**Rate Limit**: 10 SMS/second
**Payload Size**: 160 characters

---

### **5. PagerDuty**

**Business Requirements**: BR-NOT-002 (Team collaboration integration), BR-NOT-036 (Channel-specific formatting)
**Service**: PagerDuty Events API v2
**Endpoint**: `https://events.pagerduty.com/v2/enqueue`
**Authentication**: Routing Key
**Rate Limit**: 120 events/minute
**Payload Size**: Unlimited (JSON)

---

## 📊 **Data Flow Diagram**

```
┌─────────────────────────────────────────────────┐
│          Upstream CRD Controllers               │
│  ┌──────────────┐  ┌──────────────┐            │
│  │ Remediation  │  │ AI Analysis  │            │
│  │ Orchestrator │  │ Controller   │            │
│  └──────┬───────┘  └──────┬───────┘            │
│         │                  │                     │
│         │  ┌───────────┐  │                     │
│         └──┤ Workflow  │──┘                     │
│            │ Execution │                         │
│            └─────┬─────┘                         │
│                  │                               │
│         ┌────────┴────────┐                     │
│         │   Kubernetes    │                     │
│         │   Executor      │                     │
│         └────────┬────────┘                     │
└──────────────────┼──────────────────────────────┘
                   │ HTTP POST /api/v1/notify/escalation
                   │ (Bearer Token Auth)
                   ▼
┌─────────────────────────────────────────────────┐
│         Notification Service (Port 8080)        │
│  ┌───────────────────────────────────────────┐ │
│  │ 1. Authentication (TokenReviewer)         │ │
│  │ 2. Sanitization (BR-NOT-034)              │ │
│  │ 3. Channel Selection                      │ │
│  │ 4. Adapter Formatting (BR-NOT-036)        │ │
│  │ 5. Delivery                               │ │
│  └───────────────────────────────────────────┘ │
└──────────────────┬────────────────────┬─────────┘
                   │                    │
                   ▼                    ▼
┌──────────────────────────────────────────────────┐
│       Downstream External Channels                │
│  ┌────────┐  ┌────────┐  ┌────────┐  ┌────────┐│
│  │ Email  │  │ Slack  │  │ Teams  │  │  SMS   ││
│  │ (SMTP) │  │(Webhook│  │(Webhook│  │(Twilio)││
│  └────────┘  └────────┘  └────────┘  └────────┘│
│  ┌────────┐                                      │
│  │PagerDuty│                                     │
│  │ (API)   │                                     │
│  └────────┘                                      │
└──────────────────────────────────────────────────┘
```

---

## 🔐 **Authentication & Authorization**

### **Inbound (From CRD Controllers)**

**Method**: Kubernetes TokenReviewer
**Required**: ServiceAccount token in `Authorization: Bearer` header

**RBAC** (CRD controllers need):
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: notification-client
rules:
- apiGroups: [""]
  resources: ["serviceaccounts/token"]
  verbs: ["create"]
```

### **Outbound (To External Channels)**

**Method**: Per-channel authentication (secrets)

- **Email**: SMTP username/password
- **Slack**: Webhook URL (contains auth token)
- **Teams**: Webhook URL (contains auth token)
- **SMS**: Twilio Account SID + Auth Token
- **PagerDuty**: Routing Key

---

## 📦 **Configuration Management**

### **ConfigMap** (Non-Sensitive Config)

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: notification-config
  namespace: kubernaut-system
data:
  # Channel enablement
  slack.enabled: "true"
  email.enabled: "true"
  teams.enabled: "false"
  sms.enabled: "true"
  pagerduty.enabled: "true"

  # Rate limits
  email.rateLimit: "10" # per minute
  slack.rateLimit: "60" # per minute
  sms.rateLimit: "600" # per minute

  # Email settings (non-sensitive)
  email.smtpHost: "smtp.company.com"
  email.smtpPort: "587"
  email.from: "alerts@company.com"
```

### **Secret** (Sensitive Config)

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: notification-secrets
  namespace: kubernaut-system
type: Opaque
stringData:
  slack-webhook-url: "<redacted>"
  smtp-password: "<redacted>"
  teams-webhook-url: "<redacted>"
  twilio-auth-token: "<redacted>"
  pagerduty-routing-key: "<redacted>"
```

---

## 🔄 **Error Handling & Retry**

### **Circuit Breaker Pattern**

```go
type CircuitBreaker struct {
    maxFailures   int
    resetTimeout  time.Duration
    state         string // "closed", "open", "half-open"
    failureCount  int
    lastFailTime  time.Time
}

// Wrap channel delivery with circuit breaker
func (s *NotificationService) DeliverWithCircuitBreaker(channel string, payload interface{}) error {
    cb := s.circuitBreakers[channel]

    if cb.IsOpen() {
        return fmt.Errorf("circuit breaker open for channel %s", channel)
    }

    err := s.deliverToChannel(channel, payload)
    if err != nil {
        cb.RecordFailure()
        return err
    }

    cb.RecordSuccess()
    return nil
}
```

### **Retry Strategy**

- **Max Retries**: 3
- **Backoff**: Exponential (1s, 2s, 4s)
- **Timeout**: 10s per attempt
- **Fallback**: Try alternative channel if primary fails

---

## 📊 **Integration Health Monitoring**

### **Prometheus Metrics**

```go
var (
    channelDeliverySuccess = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "notification_channel_delivery_success_total",
            Help: "Successful channel deliveries",
        },
        []string{"channel"}, // "email", "slack", etc.
    )

    channelDeliveryFailure = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "notification_channel_delivery_failure_total",
            Help: "Failed channel deliveries",
        },
        []string{"channel", "reason"},
    )

    circuitBreakerState = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "notification_circuit_breaker_state",
            Help: "Circuit breaker state (0=closed, 1=open, 2=half-open)",
        },
        []string{"channel"},
    )
)
```

---

## 🎯 **Integration Testing**

### **Mock Upstream Clients**

```go
// test/integration/notification/upstream_mock.go
type MockCRDController struct {
    sentRequests []*EscalationRequest
}

func (m *MockCRDController) SendEscalation(ctx context.Context, req *EscalationRequest) error {
    m.sentRequests = append(m.sentRequests, req)
    return nil
}
```

### **Mock Downstream Channels**

```go
// test/integration/notification/downstream_mock.go
type MockSlackServer struct {
    receivedMessages []SlackMessage
    server           *httptest.Server
}

func NewMockSlackServer() *MockSlackServer {
    mock := &MockSlackServer{receivedMessages: []SlackMessage{}}
    mock.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        var msg SlackMessage
        json.NewDecoder(r.Body).Decode(&msg)
        mock.receivedMessages = append(mock.receivedMessages, msg)
        w.WriteHeader(http.StatusOK)
    }))
    return mock
}
```

---

## ✅ **Integration Checklist**

### **Upstream Integration**
- [ ] Remediation Orchestrator calls notification service
- [ ] AI Analysis Controller calls notification service
- [ ] Workflow Execution calls notification service
- [ ] Kubernetes Executor calls notification service
- [ ] All clients use Bearer token authentication

### **Downstream Integration**
- [ ] Email (SMTP) configured and tested
- [ ] Slack webhook configured and tested
- [ ] Teams webhook configured and tested
- [ ] SMS (Twilio) configured and tested
- [ ] PagerDuty routing key configured and tested

### **Configuration**
- [ ] ConfigMap deployed with channel settings
- [ ] Secrets deployed with credentials
- [ ] RBAC roles created for clients
- [ ] Network policies configured

### **Monitoring**
- [ ] Prometheus metrics exposed
- [ ] Grafana dashboards created
- [ ] Alerts configured for channel failures
- [ ] Circuit breaker metrics tracked

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Status**: ✅ Complete Specification

