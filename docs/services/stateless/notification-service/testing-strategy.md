# Notification Service - Testing Strategy

**Version**: 1.0
**Last Updated**: October 6, 2025
**Service Type**: Stateless HTTP API Service
**Status**: ⚠️ NEEDS IMPLEMENTATION

---

## 📋 Overview

Comprehensive testing strategy for the Notification Service following APDC-Enhanced TDD methodology and defense-in-depth testing pyramid.

---

## 🧪 Testing Pyramid

### **Target Coverage Distribution**

```
         /\
        /  \  E2E Tests (10-15%)
       /____\
      /      \  Integration Tests (>50%)
     /________\
    /          \  Unit Tests (70%+)
   /____________\
```

| Test Type | Target Coverage | Focus |
|-----------|----------------|-------|
| **Unit Tests** | 70%+ | Business logic, sanitization, adapters |
| **Integration Tests** | >50% | External channels, HTTP API, CRD controller integration, cross-service escalation |
| **E2E Tests** | 10-15% | Complete notification flow |

---

## 🎯 **Unit Tests** (70%+ Coverage)

### **Test Framework**: Ginkgo + Gomega

### **1. Sanitization Tests** (BR-NOT-034)

**File**: `test/unit/notification/sanitizer_test.go`

```go
package notification_test

import (
<<<<<<< HEAD
=======
    "bytes"
    "context"
    "time"

>>>>>>> crd_implementation
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/notification"
)

var _ = Describe("Sanitizer", func() {
    var sanitizer *notification.Sanitizer

    BeforeEach(func() {
        sanitizer = notification.NewSanitizer()
    })

    // BR-NOT-034: API Key Redaction
    Context("API Key Sanitization", func() {
        It("should redact API keys from content", func() {
            content := "API_KEY=sk_live_1234567890abcdef"
            sanitized, actions := sanitizer.Sanitize(content)

            Expect(sanitized).To(ContainSubstring("[REDACTED_API_KEY]"))
            Expect(sanitized).NotTo(ContainSubstring("sk_live"))
            Expect(actions).To(HaveLen(1))
        })

        It("should handle multiple API keys", func() {
            content := "key1=abc123xyz789 and key2=def456uvw012"
            sanitized, actions := sanitizer.Sanitize(content)

            Expect(actions).To(HaveLen(1))
            Expect(actions[0]).To(ContainSubstring("Redacted 2 api_key patterns"))
        })
    })

    // BR-NOT-034: Password Redaction
    Context("Password Sanitization", func() {
        It("should redact passwords", func() {
            content := "PASSWORD=SuperSecret123!"
            sanitized, _ := sanitizer.Sanitize(content)

            Expect(sanitized).To(ContainSubstring("[REDACTED_PASSWORD]"))
            Expect(sanitized).NotTo(ContainSubstring("SuperSecret"))
        })
    })

    // BR-NOT-034: Database Connection Strings
    Context("Database Connection String Sanitization", func() {
        It("should redact connection strings", func() {
            content := "postgres://user:pass@host:5432/db"
            sanitized, _ := sanitizer.Sanitize(content)

            Expect(sanitized).To(ContainSubstring("[REDACTED_DB_CONN]"))
            Expect(sanitized).NotTo(ContainSubstring("user:pass"))
        })
    })

    // BR-NOT-034: PII Redaction
    Context("PII Sanitization", func() {
        It("should redact email addresses", func() {
            content := "Contact user@example.com for help"
            sanitized, actions := sanitizer.Sanitize(content)

            Expect(sanitized).To(ContainSubstring("[REDACTED_EMAIL]"))
            Expect(actions).To(ContainElement(ContainSubstring("email")))
        })

        It("should redact phone numbers", func() {
            content := "Call +1234567890 for support"
            sanitized, _ := sanitizer.Sanitize(content)

            Expect(sanitized).To(ContainSubstring("[REDACTED_PHONE]"))
        })
    })
})
```

---

### **2. Channel Adapter Tests** (BR-NOT-036)

**File**: `test/unit/notification/adapters_test.go`

```go
package notification_test

import (
<<<<<<< HEAD
=======
    "bytes"
    "context"
    "time"

>>>>>>> crd_implementation
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/notification"
)

var _ = Describe("Channel Adapters", func() {
    var payload *notification.EscalationPayload

    BeforeEach(func() {
        payload = &notification.EscalationPayload{
            Alert: notification.Alert{
                Name:     "HighMemoryUsage",
                Severity: "critical",
            },
            RootCauseAnalysis: notification.RootCauseAnalysis{
                Summary:    "Memory limit exceeded",
                Confidence: 0.88,
            },
        }
    })

    // BR-NOT-036: Email Adapter
    Context("Email Adapter", func() {
        It("should format HTML email with 1MB limit", func() {
            adapter := notification.NewEmailAdapter()
            email, err := adapter.Format(payload)

            Expect(err).ToNot(HaveOccurred())
            Expect(email.Subject).To(ContainSubstring("HighMemoryUsage"))
            Expect(email.Body).To(ContainSubstring("<html>"))
            Expect(len(email.Body)).To(BeNumerically("<", 1024*1024)) // 1MB
        })

        It("should truncate payload exceeding 1MB", func() {
            // Create large payload
            largePayload := *payload
            largePayload.RootCauseAnalysis.DetailedAnalysis = strings.Repeat("x", 2*1024*1024)

            adapter := notification.NewEmailAdapter()
            email, err := adapter.Format(&largePayload)

            Expect(err).ToNot(HaveOccurred())
            Expect(len(email.Body)).To(BeNumerically("<", 1024*1024))
            Expect(email.Body).To(ContainSubstring("[TRUNCATED]"))
        })
    })

    // BR-NOT-036: Slack Adapter
    Context("Slack Adapter", func() {
        It("should format Slack Block Kit with 40KB limit", func() {
            adapter := notification.NewSlackAdapter()
            message, err := adapter.Format(payload)

            Expect(err).ToNot(HaveOccurred())
            Expect(message.Blocks).ToNot(BeEmpty())
            Expect(len(message.Blocks)).To(BeNumerically(">", 0))

            // Verify size limit
            jsonBytes, _ := json.Marshal(message)
            Expect(len(jsonBytes)).To(BeNumerically("<", 40*1024)) // 40KB
        })
    })

    // BR-NOT-036: Teams Adapter
    Context("Teams Adapter", func() {
        It("should format Adaptive Card with 28KB limit", func() {
            adapter := notification.NewTeamsAdapter()
            card, err := adapter.Format(payload)

            Expect(err).ToNot(HaveOccurred())
            Expect(card.Type).To(Equal("AdaptiveCard"))

            // Verify size limit
            jsonBytes, _ := json.Marshal(card)
            Expect(len(jsonBytes)).To(BeNumerically("<", 28*1024)) // 28KB
        })
    })

    // BR-NOT-036: SMS Adapter
    Context("SMS Adapter", func() {
        It("should format ultra-short SMS with 160 char limit", func() {
            adapter := notification.NewSMSAdapter()
            sms, err := adapter.Format(payload)

            Expect(err).ToNot(HaveOccurred())
            Expect(len(sms.Body)).To(BeNumerically("<=", 160))
            Expect(sms.Body).To(ContainSubstring("CRITICAL"))
            Expect(sms.Body).To(ContainSubstring("HighMemoryUsage"))
        })
    })
})
```

---

### **3. Data Freshness Tests** (BR-NOT-035)

**File**: `test/unit/notification/freshness_test.go`

```go
package notification_test

import (
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/notification"
)

var _ = Describe("Data Freshness Tracker", func() {
    var tracker *notification.FreshnessTracker

    BeforeEach(func() {
        tracker = notification.NewFreshnessTracker()
    })

    // BR-NOT-035: Freshness Validation
    Context("Freshness Validation", func() {
        It("should mark fresh data as fresh (< 60 seconds)", func() {
            gatheredAt := time.Now().Add(-30 * time.Second)
            isFresh, age := tracker.IsFresh(gatheredAt)

            Expect(isFresh).To(BeTrue())
            Expect(age).To(BeNumerically("<", 60))
        })

        It("should mark stale data as not fresh (> 60 seconds)", func() {
            gatheredAt := time.Now().Add(-90 * time.Second)
            isFresh, age := tracker.IsFresh(gatheredAt)

            Expect(isFresh).To(BeFalse())
            Expect(age).To(BeNumerically(">", 60))
        })

        It("should generate staleness warnings", func() {
            gatheredAt := time.Now().Add(-5 * time.Minute)
            warning := tracker.GenerateWarning(gatheredAt)

            Expect(warning).To(ContainSubstring("stale"))
            Expect(warning).To(ContainSubstring("5 minutes"))
        })
    })
})
```

---

## 🔗 **Integration Tests** (>50% Coverage)

### **Why >50% for Microservices Architecture**

Notification Service is a critical component in Kubernaut's **microservices architecture**, requiring extensive integration testing for:
- **CRD controller integration**: Remediation Orchestrator sends escalation notifications when remediation fails
- **Cross-service coordination**: Multiple controllers trigger notifications during remediation workflows
- **Multi-channel delivery**: Email, Slack, PagerDuty, webhook integrations must be validated
- **External API reliability**: Integration with external notification services (SendGrid, Slack API, PagerDuty API)

**Per project spec** (`.cursor/rules/03-testing-strategy.mdc` line 72):
> "**Coverage Mandate**: **>50% of total business requirements due to microservices architecture**"

### **Test Framework**: Ginkgo + Gomega + Real HTTP

### **1. HTTP API Integration Tests**

**File**: `test/integration/notification/api_test.go`

```go
package notification_test

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/notification"
)

var _ = Describe("Notification HTTP API Integration", func() {
    var server *httptest.Server
    var service *notification.NotificationService

    BeforeEach(func() {
        service = notification.NewNotificationService(/* deps */)
        server = httptest.NewServer(service.Handler())
    })

    AfterEach(func() {
        server.Close()
    })

    Context("POST /api/v1/notify/escalation", func() {
        It("should accept valid escalation notification", func() {
            payload := map[string]interface{}{
                "recipient": "sre@company.com",
                "channels":  []string{"email"},
                "payload": map[string]interface{}{
                    "alert": map[string]interface{}{
                        "name":     "HighMemoryUsage",
                        "severity": "critical",
                    },
                },
            }

            body, _ := json.Marshal(payload)
            req, _ := http.NewRequest("POST", server.URL+"/api/v1/notify/escalation", bytes.NewReader(body))
            req.Header.Set("Authorization", "Bearer test-token")
            req.Header.Set("Content-Type", "application/json")

            resp, err := http.DefaultClient.Do(req)

            Expect(err).ToNot(HaveOccurred())
            Expect(resp.StatusCode).To(Equal(http.StatusOK))
        })

        It("should reject missing authentication", func() {
            payload := map[string]interface{}{}
            body, _ := json.Marshal(payload)
            req, _ := http.NewRequest("POST", server.URL+"/api/v1/notify/escalation", bytes.NewReader(body))

            resp, err := http.DefaultClient.Do(req)

            Expect(err).ToNot(HaveOccurred())
            Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
        })
    })
})
```

---

### **2. External Channel Integration Tests**

**File**: `test/integration/notification/channels_test.go`

```go
package notification_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/notification"
)

var _ = Describe("External Channel Integration", func() {
    // NOTE: Uses mock servers for external channels

    Context("Slack Integration", func() {
        It("should deliver notification to Slack webhook", func() {
            // Mock Slack server
            slackMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                w.WriteStatus(http.StatusOK)
                w.Write([]byte("ok"))
            }))
            defer slackMock.Close()

            adapter := notification.NewSlackAdapter(slackMock.URL)
            err := adapter.Deliver(testPayload)

            Expect(err).ToNot(HaveOccurred())
        })
    })

    Context("Email Integration", func() {
        It("should deliver notification via SMTP", func() {
            // Mock SMTP server
            smtpMock := StartMockSMTP()
            defer smtpMock.Stop()

            adapter := notification.NewEmailAdapter(smtpMock.Address())
            err := adapter.Deliver(testPayload)

            Expect(err).ToNot(HaveOccurred())
            Expect(smtpMock.ReceivedCount()).To(Equal(1))
        })
    })
})
```

### **3. CRD Controller Integration Tests** (NEW - Critical for >50% Coverage)

**File**: `test/integration/notification/crd_controller_test.go`

```go
package notification_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("CRD Controller Integration", func() {
    It("should receive escalation from Remediation Orchestrator", func() {
        // Simulate Remediation Orchestrator calling Notification Service
        escalationReq := &EscalationRequest{
            RemediationRequestID: "rr-abc123",
            AlertName:            "HighMemoryUsage",
            Namespace:            "production",
            Severity:             "P0",
            Reason:               "Remediation failed after 3 attempts",
            Context: map[string]string{
                "cluster":   "us-west-2",
                "namespace": "production",
            },
        }

        response, err := notificationClient.SendEscalation(escalationReq)

        Expect(err).ToNot(HaveOccurred())
        Expect(response.Status).To(Equal("delivered"))
        Expect(response.ChannelsNotified).To(ContainElement("email"))
        Expect(response.ChannelsNotified).To(ContainElement("pagerduty"))
    })

    It("should deliver multi-channel notifications for critical alerts", func() {
        // Test P0 alert escalation to all channels
        channels := []string{"email", "slack", "pagerduty"}

        result, err := notificationClient.SendMultiChannel(
            &Alert{Severity: "P0", Name: "CriticalFailure"},
            channels,
        )

        Expect(err).ToNot(HaveOccurred())
        Expect(result.SuccessfulChannels).To(HaveLen(3))
    })

    It("should handle partial channel failures gracefully", func() {
        // Simulate one channel failing (Slack API down)
        notificationClient.SimulateChannelFailure("slack")

        result, err := notificationClient.SendMultiChannel(
            &Alert{Severity: "P1", Name: "HighMemoryUsage"},
            []string{"email", "slack", "pagerduty"},
        )

        Expect(err).ToNot(HaveOccurred())
        Expect(result.SuccessfulChannels).To(HaveLen(2)) // Email + PagerDuty
        Expect(result.FailedChannels).To(ContainElement("slack"))
    })

    It("should deliver notifications with sanitized sensitive data", func() {
        // Test data sanitization in escalation
        escalationReq := &EscalationRequest{
            RemediationRequestID: "rr-xyz789",
            AlertName:            "DatabaseConnectionFailed",
            Context: map[string]string{
                "password":        "super-secret-password", // Should be sanitized
                "api_key":         "sk-1234567890",        // Should be sanitized
                "connection_host": "db.example.com",       // Should be visible
            },
        }

        response, err := notificationClient.SendEscalation(escalationReq)

        Expect(err).ToNot(HaveOccurred())

        // Verify sensitive data was sanitized in notification
        deliveredNotification := response.DeliveredContent
        Expect(deliveredNotification).ToNot(ContainSubstring("super-secret-password"))
        Expect(deliveredNotification).ToNot(ContainSubstring("sk-1234567890"))
        Expect(deliveredNotification).To(ContainSubstring("db.example.com"))
        Expect(deliveredNotification).To(ContainSubstring("***REDACTED***"))
    })

    It("should enforce rate limits for notification delivery", func() {
        // Test rate limiting (100 notifications per minute)
        for i := 0; i < 120; i++ {
            _, err := notificationClient.SendEscalation(&EscalationRequest{})
            if i < 100 {
                Expect(err).ToNot(HaveOccurred())
            } else {
                // Should hit rate limit
                Expect(err).To(MatchError(ContainSubstring("rate limit exceeded")))
            }
        }
    })

    It("should track notification delivery metrics", func() {
        // Send notifications and verify metrics
        notificationClient.SendEscalation(&EscalationRequest{Severity: "P0"})
        notificationClient.SendEscalation(&EscalationRequest{Severity: "P1"})

        metrics, err := notificationClient.GetMetrics()

        Expect(err).ToNot(HaveOccurred())
        Expect(metrics.TotalDelivered).To(BeNumerically(">=", 2))
        Expect(metrics.DeliveryRate).To(BeNumerically(">", 0.9))
    })
})
```

---

## 🌐 **E2E Tests** (10-15% Coverage)

### **Test Framework**: Ginkgo + Real Kubernetes + Real Channels

### **Complete Notification Flow**

**File**: `test/e2e/notification/escalation_flow_test.go`

```go
package notification_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("E2E: Complete Escalation Notification Flow", func() {
    It("should deliver escalation notification end-to-end", func() {
        // 1. Setup: Deploy notification service to test cluster
        // 2. Trigger: Send escalation request from remediation controller
        // 3. Verify: Check notification delivered to channels
        // 4. Validate: Confirm sanitization applied
        // 5. Cleanup: Remove test notification

        // This test requires:
        // - Real Kubernetes cluster (Kind/Minikube)
        // - Mock external channels (Slack webhook, SMTP)
        // - Real Notification Service deployment

        By("deploying notification service")
        DeployNotificationService(testCluster)

        By("sending escalation notification request")
        response := SendEscalationRequest(/* payload */)
        Expect(response.Status).To(Equal("delivered"))

        By("verifying sanitization was applied")
        Expect(response.SanitizationApplied).ToNot(BeEmpty())

        By("confirming channel delivery")
        Eventually(func() int {
            return mockSlack.ReceivedCount()
        }, "10s").Should(Equal(1))
    })
})
```

---

## 🎯 **Test Coverage by Business Requirement**

### **Primary Requirements: Escalation Notifications** (BR-NOT-026 to BR-NOT-037)

| Business Requirement | Unit Tests | Integration Tests | E2E Tests | Total Coverage |
|---------------------|------------|-------------------|-----------|----------------|
| **BR-NOT-026** (Comprehensive alert context) | ✅ | ✅ | ✅ | 100% |
| **BR-NOT-027** (Impacted resources) | ✅ | ✅ | ✅ | 100% |
| **BR-NOT-028** (Root cause analysis) | ✅ | ✅ | ✅ | 100% |
| **BR-NOT-029** (Analysis justification) | ✅ | ✅ | - | 90% |
| **BR-NOT-030** (Recommended remediations) | ✅ | ✅ | - | 90% |
| **BR-NOT-031** (Remediation pros/cons) | ✅ | ✅ | - | 90% |
| **BR-NOT-032** (Actionable next steps) | ✅ | ✅ | ✅ | 100% |
| **BR-NOT-033** (Quick decision formatting) | ✅ | ✅ | - | 90% |
| **BR-NOT-034** (Sensitive data sanitization) | ✅✅✅ | ✅ | ✅ | 100% |
| **BR-NOT-035** (Data freshness indicators) | ✅✅ | ✅ | - | 90% |
| **BR-NOT-036** (Channel-specific formatting) | ✅✅✅✅ | ✅✅ | ✅ | 100% |
| **BR-NOT-037** (External service action links) | ✅ | ✅ | - | 80% |

### **Secondary Requirements: Multi-Channel Delivery** (BR-NOT-001 to BR-NOT-005)

| Business Requirement | Unit Tests | Integration Tests | E2E Tests | Total Coverage |
|---------------------|------------|-------------------|-----------|----------------|
| **BR-NOT-001** (Email notifications) | ✅✅ | ✅ | ✅ | 100% |
| **BR-NOT-002** (Slack integration) | ✅✅ | ✅ | ✅ | 100% |
| **BR-NOT-003** (Console/stdout) | ✅ | - | - | 70% |
| **BR-NOT-004** (SMS notifications) | ✅ | ✅ | - | 90% |
| **BR-NOT-005** (Teams & chat platforms) | ✅✅ | ✅ | - | 90% |

### **Supporting Requirements: Template Management** (BR-NOT-006 to BR-NOT-007)

| Business Requirement | Unit Tests | Integration Tests | E2E Tests | Total Coverage |
|---------------------|------------|-------------------|-----------|----------------|
| **BR-NOT-006** (Template rendering) | ✅ | ✅ | - | 85% |
| **BR-NOT-007** (Configurable templates) | ✅ | - | - | 70% |

**Note**: BR-NOT-038 to BR-NOT-040 are V2 Future requirements (excluded from V1 testing)

---

## 🔧 **Test Utilities**

### **Mock LLM Client** (Not Needed for Notification)

**N/A**: Notification Service does not use LLM

### **Mock External Channels**

```go
package testutil

type MockSlackServer struct {
    server       *httptest.Server
    receivedMsgs []SlackMessage
}

func NewMockSlackServer() *MockSlackServer {
    mock := &MockSlackServer{
        receivedMsgs: []SlackMessage{},
    }

    mock.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        var msg SlackMessage
        json.NewDecoder(r.Body).Decode(&msg)
        mock.receivedMsgs = append(mock.receivedMsgs, msg)
        w.WriteHeader(http.StatusOK)
    }))

    return mock
}

func (m *MockSlackServer) URL() string {
    return m.server.URL
}

func (m *MockSlackServer) ReceivedCount() int {
    return len(m.receivedMsgs)
}
```

---

## 📊 **Test Execution**

### **Unit Tests**
```bash
# Run all unit tests
make test-unit

# Run with coverage
go test -v -coverprofile=coverage.out ./test/unit/notification/...

# View coverage report
go tool cover -html=coverage.out
```

### **Integration Tests**
```bash
# Run integration tests (requires Docker)
make test-integration

# Run specific integration suite
go test -v ./test/integration/notification/...
```

### **E2E Tests**
```bash
# Run E2E tests (requires Kind cluster)
make test-e2e

# Setup test environment
make bootstrap-dev

# Cleanup after tests
make cleanup-dev
```

---

## 🎯 **Test Quality Metrics**

| Metric | Target | Current |
|--------|--------|---------|
| **Unit Test Coverage** | 70%+ | TBD |
| **Integration Test Coverage** | >50% | TBD |
| **E2E Test Coverage** | 10-15% | TBD |
| **Test Pass Rate** | 100% | TBD |
| **Test Execution Time** | < 5 min (unit) | TBD |

---

## ✅ **Test Checklist**

- [ ] All business requirements have corresponding tests
- [ ] Sanitization tested with 20+ patterns
- [ ] All channel adapters have size limit tests
- [ ] External channel mocks implemented
- [ ] CRD controller integration tests implemented
- [ ] Cross-service escalation tests pass
- [ ] E2E flow tests pass
- [ ] Coverage meets 70%+ unit / >50% integration / 10-15% E2E
- [ ] All tests pass in CI/CD pipeline

---

---

## 🎯 Test Level Selection: Maintainability First

**Principle**: Prioritize maintainability and simplicity when choosing between unit, integration, and e2e tests.

### Decision Framework

```mermaid
flowchart TD
    Start[New Notification Test] --> Question1{Can test with<br/>mock notifier?<br/>&lt;15 lines setup}

    Question1 -->|Yes| Question2{Testing routing logic<br/>or email/Slack<br/>delivery?}
    Question1 -->|No| Integration[Integration Test]

    Question2 -->|Routing Logic| Question3{Test readable<br/>and maintainable?}
    Question2 -->|Delivery| Integration

    Question3 -->|Yes| Unit[Unit Test]
    Question3 -->|No| Integration

    Unit --> Validate1[✅ Use Unit Test]
    Integration --> Validate2{Complete alert<br/>to notification<br/>pipeline with real<br/>SMTP/Slack?}

    Validate2 -->|Yes| E2E[E2E Test]
    Validate2 -->|No| Validate3[✅ Use Integration Test]

    E2E --> Validate4[✅ Use E2E Test]

    style Unit fill:#90EE90
    style Integration fill:#87CEEB
    style E2E fill:#FFB6C1
```

### Test at Unit Level WHEN

- ✅ Scenario can be tested with **mock notifier** (in-memory notification testing)
- ✅ Focus is on **routing logic** (channel selection, template rendering, severity mapping)
- ✅ Setup is **straightforward** (< 15 lines of mock configuration)
- ✅ Test remains **readable and maintainable** with mocking

**Notification Service Unit Test Examples**:
- Channel selection algorithms (severity → email/Slack/PagerDuty routing)
- Template rendering logic (Jinja2 expansion, variable substitution)
- Throttling calculations (rate limit enforcement, cooldown periods)
- Priority mapping (severity → notification priority)
- Message formatting (plain text, HTML, Markdown conversion)

---

### Move to Integration Level WHEN

- ✅ Scenario requires **real SMTP server** (testcontainers with MailHog)
- ✅ Validating **real Slack API** (test workspace webhooks)
- ✅ Unit test would require **excessive notifier mocking** (>40 lines of mock setup)
- ✅ Integration test is **simpler to understand** and maintain
- ✅ Testing **cross-service notification triggers** (AI Analysis → Notification → email sent)

**Notification Service Integration Test Examples**:
- Complete email delivery (template → SMTP → inbox verification)
- Real Slack webhook posting (message → Slack API → channel verification)
- PagerDuty incident creation (alert → PagerDuty API → incident verification)
- Notification retry logic (SMTP failure → retry → success)
- Cross-service coordination (Workflow Execution completes → Notification triggered → email sent)

---

### Move to E2E Level WHEN

- ✅ Testing **complete remediation notification flow** (alert → remediation → notification → stakeholder)
- ✅ Validating **end-to-end notification pipeline** (all services + real SMTP/Slack + real users)
- ✅ Lower-level tests **cannot reproduce full notification journey** (multiple channels + retries + acknowledgment)

**Notification Service E2E Test Examples**:
- Complete remediation notification (alert → AI analysis → workflow → remediation → notification → email + Slack)
- Multi-channel notification delivery (high severity → email + Slack + PagerDuty simultaneously)
- End-to-end delivery verification (notification sent → stakeholder acknowledges → tracking updated)

---

## 🧭 Maintainability Decision Criteria

**Ask these 5 questions before implementing a unit test:**

### 1. Mock Complexity
**Question**: Will SMTP/Slack mocking be >30 lines?
- ✅ **YES** → Consider integration test with testcontainers (MailHog, Slack mock)
- ❌ **NO** → Unit test acceptable

**Notification Service Example**:
```go
// ❌ COMPLEX: 70+ lines of SMTP mock setup
mockSMTP := &MockSMTPClient{
    // 70+ lines of connection mocking, TLS setup, message format
}
// BETTER: Integration test with MailHog testcontainer

// ✅ SIMPLE: Channel selection with mock notifier
mockNotifier := testutil.NewMockNotifier()
result := channelSelector.Select("critical", "production")
Expect(result).To(ConsistOf("email", "slack", "pagerduty"))
```

---

### 2. Readability
**Question**: Would a new developer understand this test in 2 minutes?
- ✅ **YES** → Unit test is good
- ❌ **NO** → Consider higher test level

**Notification Service Example**:
```go
// ✅ READABLE: Clear template rendering test
It("should render notification template with alert context", func() {
    alert := testutil.NewAlert("HighMemoryUsage", "production")

    rendered, err := templateRenderer.Render("email_template.html", alert)

    Expect(err).ToNot(HaveOccurred())
    Expect(rendered).To(ContainSubstring("High Memory Usage"))
    Expect(rendered).To(ContainSubstring("production"))
})
```

---

### 3. Fragility
**Question**: Does test break when SMTP/Slack configuration changes?
- ✅ **YES** → Move to integration test (testing implementation, not behavior)
- ❌ **NO** → Unit test is appropriate

**Notification Service Example**:
```go
// ❌ FRAGILE: Breaks if we change SMTP host/port
Expect(smtpClient.Host).To(Equal("smtp.gmail.com"))
Expect(smtpClient.Port).To(Equal(587))

// ✅ STABLE: Tests notification delivery outcome
notification, err := notifier.Send(ctx, message)
Expect(err).ToNot(HaveOccurred())
Expect(notification.Status).To(Equal("sent"))
Expect(notification.DeliveryTime).To(BeNumerically("<", 5*time.Second))
```

---

### 4. Real Value
**Question**: Is this testing routing logic or notification delivery?
- **Routing Logic** → Unit test with mock notifier
- **Notification Delivery** → Integration test with real SMTP/Slack

**Notification Service Decision**:
- **Unit**: Channel selection, template rendering, throttling (pure logic)
- **Integration**: SMTP delivery, Slack posting, PagerDuty incidents (infrastructure)

---

### 5. Maintenance Cost
**Question**: How much effort to maintain this vs integration test?
- **Lower cost** → Choose that option

**Notification Service Example**:
- **Unit test with 80-line SMTP mock**: HIGH maintenance (breaks on SMTP protocol changes)
- **Integration test with MailHog**: LOW maintenance (automatically adapts to SMTP evolution)

---

## 🎯 Realistic vs. Exhaustive Testing

**Principle**: Test realistic notification scenarios necessary to validate business requirements - not more, not less.

### Notification Service: Requirement-Driven Coverage

**Business Requirement Analysis** (BR-NOTIF-001 to BR-NOTIF-005):

| Notification Dimension | Realistic Values | Test Strategy |
|---|---|---|
| **Channels** | email, Slack, PagerDuty (3 channels) | Test channel selection |
| **Severities** | info, warning, critical (3 levels) | Test severity routing |
| **Environments** | dev, staging, production (3 envs) | Test environment filtering |
| **Delivery Status** | sent, failed, retrying (3 states) | Test retry logic |

**Total Possible Combinations**: 3 × 3 × 3 × 3 = 81 combinations
**Distinct Business Behaviors**: 12 behaviors (per BR-NOTIF-001 to BR-NOTIF-005)
**Tests Needed**: ~28 tests (covering 12 distinct behaviors with edge cases)

---

### ✅ DO: Test Distinct Notification Behaviors Using DescribeTable

**BEST PRACTICE**: Use Ginkgo DescribeTable for channel selection and throttling.

```go
// ✅ GOOD: Tests distinct channel selection using DescribeTable
var _ = Describe("BR-NOTIF-001: Channel Selection", func() {
    DescribeTable("Notification channel routing based on severity and environment",
        func(severity string, environment string, expectedChannels []string) {
            alert := testutil.NewAlert("test-alert", severity, environment)

            channels := channelSelector.Select(alert)

            Expect(channels).To(ConsistOf(expectedChannels))
        },
        // BR-NOTIF-001.1: Critical production → all channels
        Entry("critical severity in production → email + slack + pagerduty",
            "critical", "production", []string{"email", "slack", "pagerduty"}),

        // BR-NOTIF-001.2: Warning production → email + slack
        Entry("warning severity in production → email + slack",
            "warning", "production", []string{"email", "slack"}),

        // BR-NOTIF-001.3: Info production → slack only
        Entry("info severity in production → slack only",
            "info", "production", []string{"slack"}),

        // BR-NOTIF-001.4: Critical staging → email + slack
        Entry("critical severity in staging → email + slack",
            "critical", "staging", []string{"email", "slack"}),

        // BR-NOTIF-001.5: Warning staging → slack only
        Entry("warning severity in staging → slack only",
            "warning", "staging", []string{"slack"}),

        // BR-NOTIF-001.6: Dev environment → slack only (all severities)
        Entry("any severity in dev → slack only",
            "critical", "dev", []string{"slack"}),
    )
})
```

**Why DescribeTable is Better for Notification Testing**:
- ✅ 6 routing scenarios in single test function (vs. 6 separate test functions)
- ✅ Change routing logic once, all scenarios tested
- ✅ Clear channel selection rules visible
- ✅ Easy to add new environments or channels
- ✅ Perfect for testing routing matrices

---

### ❌ DON'T: Test Redundant Notification Variations

```go
// ❌ BAD: Redundant tests that validate SAME routing logic
func TestEmailNotificationForAlert1() {
    // Tests email routing for alert1
}

func TestEmailNotificationForAlert2() {
    // Tests email routing for alert2
}

func TestEmailNotificationForAlert3() {
    // Tests email routing for alert3
}
// All 3 tests validate SAME routing logic with different alert names
// BETTER: One test for routing logic, one for edge case (empty recipients)

// ❌ BAD: Exhaustive template permutations
func TestEmailTemplateWithAlert1() {}
func TestEmailTemplateWithAlert2() {}
// ... 79 more combinations
// These don't test DISTINCT template rendering
```

---

### Decision Criteria: Is This Notification Test Necessary?

Ask these 4 questions:

1. **Does this test validate a distinct notification routing or delivery rule?**
   - ✅ YES: Throttling prevents duplicate notifications within 5 minutes (BR-NOTIF-003.1)
   - ❌ NO: Testing different alert names with same routing

2. **Does this notification scenario actually occur in production?**
   - ✅ YES: SMTP server temporary failure triggers retry
   - ❌ NO: 100 simultaneous PagerDuty incidents (unrealistic)

3. **Would this test catch a notification bug the other tests wouldn't?**
   - ✅ YES: Template rendering fails gracefully with missing variables
   - ❌ NO: Testing 20 different alert titles with same template

4. **Is this testing notification behavior or implementation variation?**
   - ✅ Notification: Severity determines channel selection
   - ❌ Implementation: SMTP connection pooling details

**If answer is "NO" to all 4 questions** → Skip the test, it adds maintenance cost without notification value

---

### Notification Service Test Coverage Example with DescribeTable

**BR-NOTIF-003: Throttling and Rate Limiting (6 distinct throttling scenarios)**

```go
var _ = Describe("BR-NOTIF-003: Throttling", func() {
    // ANALYSIS: 3 channels × 3 severities × 3 envs × 4 time windows = 108 combinations
    // REQUIREMENT ANALYSIS: Only 6 distinct throttling behaviors per BR-NOTIF-003
    // TEST STRATEGY: Use DescribeTable for 6 throttling scenarios + 2 edge cases

    DescribeTable("Notification throttling based on cooldown periods",
        func(channel string, lastSentMinutesAgo int, cooldownMinutes int, expectedThrottled bool) {
            lastSent := time.Now().Add(-time.Duration(lastSentMinutesAgo) * time.Minute)

            throttled := throttler.ShouldThrottle(channel, lastSent, cooldownMinutes)

            Expect(throttled).To(Equal(expectedThrottled))
        },
        // Scenario 1: Within cooldown → throttled
        Entry("email sent 2 minutes ago with 5 minute cooldown → throttled",
            "email", 2, 5, true),

        // Scenario 2: Beyond cooldown → allowed
        Entry("email sent 6 minutes ago with 5 minute cooldown → allowed",
            "email", 6, 5, false),

        // Scenario 3: PagerDuty shorter cooldown (critical incidents)
        Entry("pagerduty sent 1 minute ago with 2 minute cooldown → throttled",
            "pagerduty", 1, 2, true),

        // Scenario 4: Slack longer cooldown (non-critical)
        Entry("slack sent 8 minutes ago with 10 minute cooldown → throttled",
            "slack", 8, 10, true),

        // Scenario 5: Exact cooldown boundary
        Entry("email sent exactly 5 minutes ago with 5 minute cooldown → allowed",
            "email", 5, 5, false),

        // Scenario 6: First notification (never sent)
        Entry("never sent before → allowed",
            "email", 10000, 5, false),

        // Edge case 1: Zero cooldown (instant retry)
        Entry("zero cooldown → always allowed",
            "email", 0, 0, false),

        // Edge case 2: Just before cooldown expiry
        Entry("sent 4.9 minutes ago with 5 minute cooldown → throttled",
            "email", 4, 5, true),  // 4 minutes < 5 minutes
    )
})
```

**Benefits for Notification Throttling Testing**:
- ✅ **8 throttling scenarios tested in ~12 lines** (vs. ~240 lines with separate functions)
- ✅ **Single throttle engine** - changes apply to all channels
- ✅ **Clear throttling rules** - cooldown logic immediately visible
- ✅ **Easy to add channels** - new parameter for new notification types
- ✅ **90% less maintenance** for complex throttling testing

---

## ⚠️ Anti-Patterns to AVOID

### ❌ OVER-EXTENDED UNIT TESTS (Forbidden)

**Problem**: Excessive SMTP/Slack mocking (>40 lines) makes notification tests unmaintainable

```go
// ❌ BAD: 90+ lines of SMTP connection and message mocking
mockSMTP := &MockSMTPClient{
    // 90+ lines of TLS, authentication, message format mocking
}
// THIS SHOULD BE AN INTEGRATION TEST

// BETTER: Integration test with MailHog testcontainer
```

**Solution**: Move to integration test with testcontainers

```go
// ✅ GOOD: Integration test with MailHog (testcontainers)
mailhog, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
    ContainerRequest: testcontainers.ContainerRequest{
        Image:        "mailhog/mailhog:latest",
        ExposedPorts: []string{"1025/tcp", "8025/tcp"},
    },
    Started: true,
})
Expect(err).ToNot(HaveOccurred())

// Send email
err = notifier.SendEmail(ctx, notification)
Expect(err).ToNot(HaveOccurred())

// Verify in MailHog inbox
emails := mailhog.GetEmails()
Expect(emails).To(HaveLen(1))
Expect(emails[0].Subject).To(Equal("Critical Alert: HighMemoryUsage"))
```

---

### ❌ WRONG TEST LEVEL (Forbidden)

**Problem**: Testing real SMTP delivery in unit tests

```go
// ❌ BAD: Testing actual SMTP delivery in unit test
It("should send email via Gmail SMTP", func() {
    // Complex mocking of Gmail SMTP
    // Real SMTP connection - belongs in integration test
})
```

**Solution**: Use integration test for real SMTP

```go
// ✅ GOOD: Integration test for SMTP delivery
Describe("Integration: SMTP Delivery", func() {
    It("should deliver email to MailHog", func() {
        // Test with real SMTP testcontainer
    })
})
```

---

### ❌ REDUNDANT COVERAGE (Forbidden)

**Problem**: Testing same template rendering at multiple levels

```go
// ❌ BAD: Testing exact same template rendering at all 3 levels
// Unit test: Template rendering
// Integration test: Template rendering (duplicate)
// E2E test: Template rendering (duplicate)
// NO additional value
```

**Solution**: Test rendering in unit tests, test DELIVERY in higher levels

```go
// ✅ GOOD: Each level tests distinct aspect
// Unit test: Template rendering correctness
// Integration test: Template rendering + SMTP delivery
// E2E test: Template rendering + delivery + stakeholder acknowledgment
// Each level adds unique notification value
```

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Status**: ✅ Complete Specification

