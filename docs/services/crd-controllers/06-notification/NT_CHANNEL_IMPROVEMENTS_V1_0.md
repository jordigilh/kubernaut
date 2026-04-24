# Notification Service (NT) - Channel Improvements Plan

**Date**: December 22, 2025
**Last Updated**: March 4, 2026
**Version**: v2.0
**Status**: ✅ **ACTIVE** - Reflects v1.4 channel reality
**Priority**: P1 (Production Readiness)
**Target**: Production-grade notification channels with reliability and observability

---

## 🎯 Objective

Track and improve notification delivery channels across releases.

**Implemented Channels (v1.4)**: 6 (Console ✅, Slack ✅, File ✅, Log ✅, PagerDuty ✅, Teams ✅)
**Planned Channels (v1.6+)**: Email, Webhook, ServiceNow (#61), Jira (#53)

---

## 📊 Channel Status (as of v1.4)

### ✅ **Console Delivery** (PRODUCTION-READY)
**File**: `pkg/notification/delivery/console.go`
**Status**: ✅ PRODUCTION-READY
**Registration**: Static — `cmd/notification/main.go`
**LOC**: ~80 lines

**Features**:
- ✅ Structured JSON logging
- ✅ Non-blocking delivery
- ✅ Zero external dependencies
- ✅ 100% reliability (no network calls)

**No Improvements Needed**: Console delivery is simple, reliable, and well-tested.

---

### ✅ **Slack Delivery** (PRODUCTION-READY)
**File**: `pkg/notification/delivery/slack.go`
**Status**: ✅ PRODUCTION-READY
**Registration**: Dynamic — `internal/controller/notification/routing_handler.go` (per-receiver)
**LOC**: ~280 lines

**Current Features**:
- ✅ Slack Block Kit formatting
- ✅ Priority-based color coding (critical=red, high=orange, medium=yellow, low=gray)
- ✅ Webhook URL from environment variable
- ✅ HTTP timeout: 10s
- ⚠️ Basic error handling

**Identified Issues**:
1. **Pre-Existing Lint Warning** (P2 - LOW):
   - `pkg/notification/delivery/slack.go:71`: `Error return value of resp.Body.Close is not checked`
   - Impact: Potential resource leak (minor)
   - Fix: 5 minutes

2. **No Rate Limiting Protection** (P1 - HIGH):
   - Slack API rate limit: ~1 request/second
   - Current implementation: No rate limiting
   - Risk: 429 errors under burst traffic
   - Impact: Notification delivery failures during incidents

3. **No Retry on 429** (P1 - HIGH):
   - Slack returns 429 with `Retry-After` header
   - Current implementation: Treats 429 as permanent failure
   - Risk: Unnecessary failures during rate limiting

4. **Limited Observability** (P2 - MEDIUM):
   - No Slack-specific metrics (success rate, latency, rate limit errors)
   - No structured logging of Slack API responses
   - Difficult to debug Slack integration issues

5. **No Connection Pooling** (P2 - MEDIUM):
   - Creates new HTTP client for each request
   - Impact: Slower delivery, more TCP connections
   - Recommendation: Reuse HTTP client with connection pooling

---

### ✅ **File Delivery** (PRODUCTION-READY)
**File**: `pkg/notification/delivery/file.go`
**Status**: ✅ PRODUCTION-READY
**Registration**: Static — `cmd/notification/main.go`
**LOC**: ~200 lines

**Purpose**: File-based delivery for audit trails and compliance.

---

### ✅ **Log Delivery** (PRODUCTION-READY)
**File**: `pkg/notification/delivery/log.go`
**Status**: ✅ PRODUCTION-READY
**Registration**: Static — `cmd/notification/main.go`
**LOC**: ~200 lines

**Purpose**: Structured JSON logs to stdout for observability pipelines.

---

### ✅ **PagerDuty Delivery** (PRODUCTION-READY)
**File**: `pkg/notification/delivery/pagerduty.go`, `pagerduty_payload.go`
**Status**: ✅ PRODUCTION-READY (implemented in #60)
**Registration**: Dynamic — `internal/controller/notification/routing_handler.go` (per-receiver)
**LOC**: ~300 lines

**Features**:
- ✅ PagerDuty Events API V2 (`https://events.pagerduty.com/v2/enqueue`)
- ✅ Routing key from credential config
- ✅ Severity mapping from notification priority
- ✅ Circuit breaker support via `routing_handler.go`

---

### ✅ **Teams Delivery** (PRODUCTION-READY)
**File**: `pkg/notification/delivery/teams.go`, `teams_cards.go`
**Status**: ✅ PRODUCTION-READY (implemented in #593)
**Registration**: Dynamic — `internal/controller/notification/routing_handler.go` (per-receiver)
**LOC**: ~400 lines

**Features**:
- ✅ Power Automate Workflows incoming webhooks
- ✅ Adaptive Card formatting
- ✅ Circuit breaker support via `routing_handler.go`

---

## 🚨 Identified Channel Improvement Priorities

### **Priority 1: Slack Delivery Reliability** (P0 - CRITICAL for V1.0)

#### **Improvement 1.1: Fix Pre-Existing Lint Warning** (P2 - LOW)
**File**: `pkg/notification/delivery/slack.go:71`
**Priority**: P2 (LOW)
**Estimated Time**: 5 minutes
**Impact**: Prevents potential resource leak

**Current Code**:
```go
defer resp.Body.Close() // ← Lint warning: error return value not checked
```

**Proposed Fix**:
```go
defer func() {
    if closeErr := resp.Body.Close(); closeErr != nil {
        // Log but don't fail delivery on body close error
        // Per BR-NOT-053: Delivery succeeded if Slack API returned 200
        // Body close error is non-critical
    }
}()
```

**Success Criteria**:
- ✅ Lint warning resolved
- ✅ Resource leak prevented
- ✅ No functional change (delivery still succeeds)

---

#### **Improvement 1.2: Add Rate Limiting Protection** (P1 - HIGH)
**File**: `pkg/notification/delivery/slack.go`
**Priority**: P1 (HIGH)
**Estimated Time**: 2-3 hours
**Impact**: Prevents 429 errors during burst traffic

**Problem**:
- Slack API rate limit: ~1 request/second (~60 requests/minute)
- Current implementation: No rate limiting
- Risk: Burst of 10+ notifications → 429 errors

**Proposed Solution**: Token bucket rate limiter

```go
import (
    "golang.org/x/time/rate"
)

type SlackDeliveryService struct {
    webhookURL  string
    httpClient  *http.Client
    rateLimiter *rate.Limiter // ← NEW
}

func NewSlackDeliveryService(webhookURL string) *SlackDeliveryService {
    return &SlackDeliveryService{
        webhookURL:  webhookURL,
        httpClient:  &http.Client{Timeout: 10 * time.Second},
        rateLimiter: rate.NewLimiter(rate.Limit(1), 5), // ← 1/sec, burst 5
    }
}

func (s *SlackDeliveryService) Deliver(ctx context.Context, notif *notificationv1alpha1.NotificationRequest) error {
    // Wait for rate limiter permit (non-blocking, respects ctx timeout)
    if err := s.rateLimiter.Wait(ctx); err != nil {
        return fmt.Errorf("rate limiter timeout: %w", err)
    }

    // ... existing delivery logic
}
```

**Configuration**:
- **Rate**: 1 request/second (conservative, under Slack limit)
- **Burst**: 5 requests (allow small bursts without delay)
- **Justification**: Slack rate limit is ~1/sec per webhook, conservative limit prevents 429s

**Success Criteria**:
- ✅ Rate limiter prevents 429 errors
- ✅ Burst of 10 notifications handled gracefully (5 immediate, 5 delayed)
- ✅ Context timeout respected (if rate limiter blocks >10s, return error)
- ✅ Metrics exposed: `notification_delivery_rate_limited_total{channel="slack"}`

---

#### **Improvement 1.3: Retry on 429 Rate Limiting** (P1 - HIGH)
**File**: `pkg/notification/delivery/slack.go`
**Priority**: P1 (HIGH)
**Estimated Time**: 1-2 hours
**Impact**: Reduces unnecessary failures during rate limiting

**Problem**:
- Slack returns 429 with `Retry-After` header (seconds)
- Current implementation: Treats 429 as permanent failure
- Risk: Notifications marked failed instead of retried

**Proposed Solution**: Detect 429 and return retryable error

```go
func (s *SlackDeliveryService) Deliver(ctx context.Context, notif *notificationv1alpha1.NotificationRequest) error {
    // ... send request

    if resp.StatusCode == 429 {
        retryAfter := resp.Header.Get("Retry-After")
        // Return retryable error with hint for backoff
        return &RetryableError{
            Err:        fmt.Errorf("slack rate limited, retry after %s seconds", retryAfter),
            RetryAfter: parseRetryAfter(retryAfter), // ← Convert to time.Duration
        }
    }

    // ... existing error handling
}
```

**Integration with Retry Logic**:
```go
// In internal/controller/notification/retry_circuit_breaker_handler.go
func (r *NotificationRequestReconciler) handleRetryableError(ctx context.Context, notif *notificationv1alpha1.NotificationRequest, err error) (reconcile.Result, error) {
    var retryErr *RetryableError
    if errors.As(err, &retryErr) && retryErr.RetryAfter > 0 {
        // Use Slack's Retry-After hint instead of exponential backoff
        return reconcile.Result{RequeueAfter: retryErr.RetryAfter}, nil
    }

    // ... existing exponential backoff logic
}
```

**Success Criteria**:
- ✅ 429 errors classified as retryable (not permanent failure)
- ✅ Retry-After header respected (no unnecessary delays)
- ✅ CRD status shows retry reason: "rate limited"
- ✅ Metrics exposed: `notification_delivery_retries_total{channel="slack",reason="rate_limited"}`

---

#### **Improvement 1.4: Enhanced Observability** (P2 - MEDIUM)
**File**: `pkg/notification/delivery/slack.go`
**Priority**: P2 (MEDIUM)
**Estimated Time**: 2 hours
**Impact**: Improves debugging and operational visibility

**Proposed Enhancements**:

1. **Slack-Specific Metrics**:
```go
// In pkg/notification/metrics/metrics.go
const (
    // ... existing metrics
    MetricNameSlackDeliverySuccess = "kubernaut_notification_slack_delivery_success_total"
    MetricNameSlackDeliveryFailure = "kubernaut_notification_slack_delivery_failure_total"
    MetricNameSlackRateLimitHits   = "kubernaut_notification_slack_rate_limit_hits_total"
    MetricNameSlackDeliveryLatency = "kubernaut_notification_slack_delivery_latency_seconds"
)

type Metrics struct {
    // ... existing metrics
    SlackDeliverySuccess prometheus.Counter
    SlackDeliveryFailure *prometheus.CounterVec // labels: reason (timeout, 4xx, 5xx, rate_limited)
    SlackRateLimitHits   prometheus.Counter
    SlackDeliveryLatency prometheus.Histogram
}
```

2. **Structured Logging**:
```go
func (s *SlackDeliveryService) Deliver(ctx context.Context, notif *notificationv1alpha1.NotificationRequest) error {
    start := time.Now()

    // ... delivery logic

    s.logger.Info("slack delivery completed",
        "notification", notif.Name,
        "status_code", resp.StatusCode,
        "latency_ms", time.Since(start).Milliseconds(),
        "rate_limited", resp.StatusCode == 429,
        "response_headers", resp.Header, // ← Useful for debugging
    )

    // ... metrics recording
}
```

**Success Criteria**:
- ✅ Slack success rate metric exposed
- ✅ Slack failure reasons tracked (timeout, 4xx, 5xx, rate_limited)
- ✅ Slack delivery latency histogram (P50, P95, P99)
- ✅ Structured logs include Slack API response details

---

#### **Improvement 1.5: HTTP Connection Pooling** (P2 - MEDIUM)
**File**: `pkg/notification/delivery/slack.go`
**Priority**: P2 (MEDIUM)
**Estimated Time**: 1 hour
**Impact**: Reduces latency and TCP connection overhead

**Current Implementation**:
```go
// Creates new HTTP client for EACH notification (inefficient)
httpClient := &http.Client{Timeout: 10 * time.Second}
```

**Proposed Solution**: Reuse HTTP client with connection pooling

```go
var (
    // Shared HTTP client with connection pooling (singleton)
    sharedHTTPClient = &http.Client{
        Timeout: 10 * time.Second,
        Transport: &http.Transport{
            MaxIdleConns:        100,              // Max idle connections across all hosts
            MaxIdleConnsPerHost: 10,               // Max idle connections per host (Slack)
            IdleConnTimeout:     90 * time.Second, // Close idle connections after 90s
            TLSHandshakeTimeout: 10 * time.Second,
            ExpectContinueTimeout: 1 * time.Second,
        },
    }
)

type SlackDeliveryService struct {
    webhookURL  string
    httpClient  *http.Client // ← Reuse sharedHTTPClient
    rateLimiter *rate.Limiter
}

func NewSlackDeliveryService(webhookURL string) *SlackDeliveryService {
    return &SlackDeliveryService{
        webhookURL:  webhookURL,
        httpClient:  sharedHTTPClient, // ← Reuse
        rateLimiter: rate.NewLimiter(rate.Limit(1), 5),
    }
}
```

**Success Criteria**:
- ✅ HTTP connections reused across notifications
- ✅ Delivery latency reduced by ~50-100ms (TLS handshake saved)
- ✅ TCP connection count stable (<10 connections to Slack)

---

### **Priority 2: Channel Evaluation Status**

#### **Status Matrix (Updated v1.4)**:
| Channel | Business Priority | Status | Milestone | Notes |
|---|---|---|---|---|
| **Console** | P0 (Core) | ✅ Implemented | Pre-v1.0 | Static registration |
| **Slack** | P0 (Core) | ✅ Implemented | Pre-v1.0 | Dynamic per-receiver |
| **File** | P0 (Core) | ✅ Implemented | Pre-v1.0 | Static registration |
| **Log** | P0 (Core) | ✅ Implemented | Pre-v1.0 | Static registration |
| **PagerDuty** | P0 (Critical) | ✅ Implemented | v1.3 (#60) | Dynamic per-receiver, circuit breaker |
| **Teams** | P1 (High) | ✅ Implemented | v1.3 (#593) | Dynamic per-receiver, Adaptive Cards |
| **Email** | P1 (High) | ⏸️ Planned | TBD | SMTP integration |
| **Webhook** | P2 (Medium) | ⏸️ Planned | TBD | Generic HTTP POST |
| **ServiceNow** | P1 (High) | ⏸️ Planned | v1.6 (#61) | ITSM integration |
| **Jira** | P1 (High) | ⏸️ Planned | v1.6 (#53) | ITSM integration |
| **SMS (Twilio)** | P3 (Low) | ⏸️ Deferred | TBD | Low ROI |

---

#### **Channel 1: Email Delivery** (P0 - CRITICAL, RECOMMENDED FOR V1.0)

**Business Justification**:
- **Use Case**: Formal notifications, external stakeholders, compliance documentation
- **Priority**: P0 (Critical) - Many enterprises require email for audit trails
- **Precedent**: Slack is for real-time, Email is for formal record

**Implementation Complexity**: MEDIUM (2-3 days)

**Key Features**:
1. **SMTP Integration**:
   - Environment variables: `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`
   - TLS support (STARTTLS)
   - Authentication (PLAIN, LOGIN)

2. **HTML Email Templates**:
   - Subject: NotificationRequest.Subject
   - Body: NotificationRequest.Body (formatted as HTML)
   - Footer: "Sent by Kubernaut Notification Service"
   - Priority badge (critical=red, high=orange, etc.)

3. **Error Handling**:
   - Transient errors: SMTP connection failures, timeouts (retryable)
   - Permanent errors: Invalid email addresses, authentication failures (non-retryable)

4. **Rate Limiting**:
   - SMTP rate limit: ~10 emails/second (configurable)
   - Token bucket rate limiter (similar to Slack)

**Estimated LOC**: ~350 lines (email.go + templates)

**Recommendation**: ✅ **ADD TO V1.0** (Email is critical for enterprise deployments)

---

#### **Channel 2: PagerDuty Integration** — ✅ IMPLEMENTED (v1.3, #60)

**Files**: `pkg/notification/delivery/pagerduty.go`, `pagerduty_payload.go`
**Wired in**: `internal/controller/notification/routing_handler.go` (dynamic per-receiver)

**Implemented Features**:
1. **PagerDuty Events API V2** — `https://events.pagerduty.com/v2/enqueue`
2. **Routing key from credential config** (resolved per-receiver)
3. **Severity mapping** from notification priority
4. **Circuit breaker** integration via `routing_handler.go`
5. **Payload builder** with structured incident details (`pagerduty_payload.go`)

---

#### **Channel 3: Microsoft Teams** — ✅ IMPLEMENTED (v1.3, #593)

**Files**: `pkg/notification/delivery/teams.go`, `teams_cards.go`
**Wired in**: `internal/controller/notification/routing_handler.go` (dynamic per-receiver)

**Implemented Features**:
1. **Power Automate Workflows incoming webhooks**
2. **Adaptive Card formatting** (`teams_cards.go`)
3. **Circuit breaker** integration via `routing_handler.go`
4. **Configurable timeout** (default 10s)

---

#### **Channel 4: SMS (Twilio)** (P2 - MEDIUM, DEFER TO V2.0)

**Business Justification**:
- **Use Case**: Critical on-call alerts (backup channel)
- **Priority**: P2 (Medium) - Nice to have but not critical

**Implementation Complexity**: MEDIUM-HIGH (3-4 days)

**Challenges**:
- SMS cost (charged per message)
- Message length limits (160 characters)
- Twilio account setup + phone number provisioning
- International delivery complexities

**Recommendation**: ⏸️ **DEFER TO V2.0** (Not justified for V1.0, low ROI)

---

#### **Channel 5: Generic Webhook** (P2 - MEDIUM, DEFER TO V1.1)

**Business Justification**:
- **Use Case**: Custom integrations (ServiceNow, Jira, etc.)
- **Priority**: P2 (Medium) - Flexibility for future integrations

**Implementation Complexity**: LOW (1 day)

**Key Features**:
1. **Generic HTTP POST**:
   - Webhook URL from ConfigMap or NotificationRequest label
   - JSON payload: NotificationRequest spec + status
   - HTTP headers: Content-Type, Authorization (optional)

2. **Error Handling**:
   - Transient: 5xx errors, timeouts (retryable)
   - Permanent: 4xx errors (non-retryable)

**Estimated LOC**: ~150 lines (webhook.go)

**Recommendation**: ⏸️ **DEFER TO V1.1** (Simple to add, but no immediate use case)

---

## 📋 Channel Roadmap (Updated v1.4)

### **Pre-v1.0 (Delivered)**:
- ✅ **Console**: Static registration, production-ready
- ✅ **Slack**: Dynamic per-receiver registration, Block Kit formatting
- ✅ **File**: Static registration, file-based delivery for audit trails
- ✅ **Log**: Static registration, structured JSON to stdout

### **v1.3 (Delivered — #60, #593)**:
- ✅ **PagerDuty**: Dynamic per-receiver, Events API V2, circuit breaker
- ✅ **Teams**: Dynamic per-receiver, Adaptive Cards, circuit breaker

**v1.4 Channel Count**: 6 channels

---

### **v1.6 (Planned — ITSM Integration)**:
- ⏸️ **ServiceNow** (#61): Incident records via REST API, CMDB linkage
- ⏸️ **Jira** (#53): Ticket creation with RCA summaries

**v1.6 Channel Count**: 8 channels (projected)

---

### **Future (TBD)**:
- ⏸️ **Email**: SMTP integration for formal notifications
- ⏸️ **Webhook**: Generic HTTP POST for custom integrations
- ⏸️ **SMS (Twilio)**: Low priority, deferred pending customer demand

---

## ⏱️ Time Estimates

### **V1.0 Slack Improvements** (MANDATORY):
| Improvement | Priority | Time | Impact |
|---|---|---|---|
| 1.1: Fix lint warning | P2 | 5 min | Low |
| 1.2: Rate limiting | P1 | 2-3 hours | High |
| 1.3: Retry on 429 | P1 | 1-2 hours | High |
| 1.4: Observability | P2 | 2 hours | Medium |
| 1.5: Connection pooling | P2 | 1 hour | Medium |
| **TOTAL** | - | **~7 hours** | - |

---

### **Channel Implementation Status**:
| Channel | Priority | Status | Milestone |
|---|---|---|---|
| Console | P0 | ✅ Delivered | Pre-v1.0 |
| Slack | P0 | ✅ Delivered | Pre-v1.0 |
| File | P0 | ✅ Delivered | Pre-v1.0 |
| Log | P0 | ✅ Delivered | Pre-v1.0 |
| PagerDuty | P0 | ✅ Delivered | v1.3 (#60) |
| Teams | P1 | ✅ Delivered | v1.3 (#593) |
| ServiceNow | P1 | ⏸️ Planned | v1.6 (#61) |
| Jira | P1 | ⏸️ Planned | v1.6 (#53) |
| Email | P1 | ⏸️ Planned | TBD |
| Webhook | P2 | ⏸️ Planned | TBD |
| SMS | P3 | ⏸️ Deferred | TBD |

---

## ✅ Success Criteria (v1.4)

### **Delivered Channels**:
- [x] Console: Production-ready, static registration
- [x] Slack: Dynamic per-receiver, Block Kit formatting, circuit breaker
- [x] File: Static registration, file-based audit delivery
- [x] Log: Static registration, structured JSON to stdout
- [x] PagerDuty: Events API V2, dynamic per-receiver, circuit breaker (#60)
- [x] Teams: Adaptive Cards, dynamic per-receiver, circuit breaker (#593)
- [x] DB migration constraint matches implemented channels (migration 006)

### **Remaining Improvements** (Slack Reliability):
- [ ] Rate limiting prevents 429 errors under burst traffic
- [ ] 429 errors are retryable (not permanent failures)
- [ ] Slack-specific metrics exposed (success rate, latency, rate limit hits)
- [ ] HTTP connection pooling reduces latency

---

## 🎯 Recommendation

### **v1.4 Focus**: Slack Reliability Improvements
- Rate limiting, 429 retry, observability (~7 hours)

### **v1.6 Focus**: ITSM Integration
- ServiceNow delivery channel (#61)
- Jira delivery channel (#53)
- Aligns with multi-cluster federation scope (#54)

---

**Status**: ✅ **ACTIVE**
**Owner**: NT Team

---

## 📜 Changelog

| Version | Date | Changes |
|---|---|---|
| v1.0 | December 22, 2025 | Initial plan. Channels: Console, Slack, File. PagerDuty and Teams evaluated as future. |
| v2.0 | March 4, 2026 | Updated to reflect v1.4 reality: PagerDuty (#60) and Teams (#593) are now implemented and wired into production via `routing_handler.go`. Added Log as implemented channel. Updated roadmap with v1.6 ITSM targets (#53, #61). Corrected DB migration 006 CHECK constraint to match all 6 implemented channels. |

