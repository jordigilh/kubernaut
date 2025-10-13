# Error Handling Philosophy - Notification Controller

**Date**: 2025-10-12  
**Status**: Production-Ready  
**BR Coverage**: BR-NOT-052 (Automatic Retry), BR-NOT-055 (Graceful Degradation)

---

## 🎯 **Core Principles**

### **1. Classify Before Acting**
Every error must be classified as **transient** (retryable) or **permanent** (not retryable) before deciding on action.

### **2. Fail Gracefully**
Partial success is acceptable. Console delivery succeeding while Slack fails is better than failing everything.

### **3. Protect the System**
Circuit breakers prevent cascading failures. An unhealthy channel should not block other channels.

### **4. Transparent Failures**
All failures are recorded in the CRD status with timestamps, error messages, and attempt counts.

---

## 📊 **Error Classification**

### **Transient Errors (Retryable)**
These errors are temporary and likely to succeed on retry:

| Error Type | HTTP Code | Retry Strategy | Example |
|-----------|-----------|----------------|---------|
| **Network Timeout** | - | Retry with backoff | DNS resolution failure, connection timeout |
| **Rate Limiting** | 429 | Retry with exponential backoff | Slack rate limit exceeded |
| **Service Unavailable** | 503 | Retry with backoff | Slack service temporarily down |
| **Internal Server Error** | 500 | Retry with backoff | Slack internal error |
| **Bad Gateway** | 502 | Retry with backoff | Proxy/gateway failure |
| **Gateway Timeout** | 504 | Retry with backoff | Upstream timeout |
| **Request Timeout** | 408 | Retry with backoff | Client timeout |

**Action**: Retry up to 5 times with exponential backoff (30s, 60s, 120s, 240s, 480s)

---

### **Permanent Errors (Not Retryable)**
These errors indicate a configuration or authorization problem that won't resolve with retries:

| Error Type | HTTP Code | Action | Example |
|-----------|-----------|--------|---------|
| **Unauthorized** | 401 | Mark as Failed immediately | Invalid Slack webhook token |
| **Forbidden** | 403 | Mark as Failed immediately | Insufficient permissions |
| **Not Found** | 404 | Mark as Failed immediately | Slack webhook URL doesn't exist |
| **Bad Request** | 400 | Mark as Failed immediately | Invalid JSON payload |
| **Unprocessable Entity** | 422 | Mark as Failed immediately | Schema validation failure |

**Action**: Mark notification as `Failed` immediately, record error in status

---

## 🔄 **Retry Policy**

### **Exponential Backoff**
```
Attempt 0: 30 seconds
Attempt 1: 60 seconds (30 * 2^1)
Attempt 2: 120 seconds (30 * 2^2)
Attempt 3: 240 seconds (30 * 2^3)
Attempt 4: 480 seconds (30 * 2^4, capped at max)
```

**Configuration**:
- **Max Attempts**: 5 per channel
- **Base Backoff**: 30 seconds
- **Max Backoff**: 480 seconds (8 minutes)
- **Multiplier**: 2.0

**Rationale**: Exponential backoff prevents overwhelming failing services while giving them time to recover.

---

## 🔌 **Circuit Breaker**

### **Purpose**
Prevent cascading failures by temporarily blocking requests to unhealthy channels.

### **States**

| State | Behavior | Transition |
|-------|----------|------------|
| **Closed** (Normal) | All requests allowed | → Open after 5 consecutive failures |
| **Open** (Failing) | All requests blocked | → Half-Open after 60s timeout |
| **Half-Open** (Testing) | Limited requests allowed | → Closed after 2 successes, → Open on failure |

### **Configuration**:
- **Failure Threshold**: 5 consecutive failures
- **Success Threshold**: 2 consecutive successes
- **Timeout**: 60 seconds

**Rationale**: Prevents wasting resources on known-failing channels while periodically testing recovery.

---

## 🎯 **Per-Channel Isolation**

### **Principle**
Each delivery channel (Console, Slack, Email, etc.) has:
- **Independent retry counters**
- **Independent circuit breaker state**
- **Independent failure tracking**

### **Example Scenario**
```
Notification: "Database connection failed"
Channels: [Console, Slack]

Timeline:
t=0s:   Console ✅ delivered, Slack ❌ failed (503)
t=30s:  Slack retry 1 ❌ failed (503)
t=90s:  Slack retry 2 ❌ failed (503)
t=210s: Slack retry 3 ❌ failed (503)
t=450s: Slack retry 4 ❌ failed (503)
t=930s: Slack retry 5 ❌ failed (503) - max retries reached

Result: NotificationPhase = PartiallySent
Status:
  SuccessfulDeliveries: 1 (Console)
  FailedDeliveries: 5 (Slack)
  CompletionTime: t=930s
```

**Benefit**: Console delivery succeeded immediately. Slack failures didn't block console.

---

## 📝 **User Notification Patterns**

### **Pattern 1: All Deliveries Successful**
```yaml
status:
  phase: Sent
  reason: AllDeliveriesSucceeded
  message: "Successfully delivered to 2 channel(s)"
  deliveryAttempts:
    - channel: console
      status: success
      timestamp: "2025-10-12T19:30:00Z"
    - channel: slack
      status: success
      timestamp: "2025-10-12T19:30:01Z"
  completionTime: "2025-10-12T19:30:01Z"
```

---

### **Pattern 2: Partial Success (Graceful Degradation)**
```yaml
status:
  phase: PartiallySent
  reason: PartialDeliveryFailure
  message: "1 of 2 deliveries succeeded"
  deliveryAttempts:
    - channel: console
      status: success
      timestamp: "2025-10-12T19:30:00Z"
    - channel: slack
      status: failed
      error: "webhook returned 503 (retryable)"
      timestamp: "2025-10-12T19:30:01Z"
    - channel: slack
      status: failed
      error: "webhook returned 503 (retryable)"
      timestamp: "2025-10-12T19:30:31Z"
    # ... 3 more Slack retries
  successfulDeliveries: 1
  failedDeliveries: 5
  completionTime: "2025-10-12T19:45:00Z"
```

---

### **Pattern 3: Complete Failure**
```yaml
status:
  phase: Failed
  reason: MaxRetriesExceeded
  message: "All 2 deliveries failed"
  deliveryAttempts:
    - channel: console
      status: failed
      error: "service not initialized"
      timestamp: "2025-10-12T19:30:00Z"
    - channel: slack
      status: failed
      error: "webhook returned 401 (permanent failure)"
      timestamp: "2025-10-12T19:30:01Z"
  completionTime: "2025-10-12T19:30:01Z"
```

---

## 🛠️ **Operational Guidelines**

### **For Operators**

**Detecting Issues**:
```bash
# Find notifications stuck in Sending phase
kubectl get notificationrequests -A -o json | \
  jq '.items[] | select(.status.phase == "Sending") | .metadata.name'

# Check circuit breaker state (via controller logs)
kubectl logs -n kubernaut-notifications deployment/notification-controller | \
  grep "circuit_state"
```

**Troubleshooting Permanent Failures**:
1. Check notification status: `kubectl get notificationrequest <name> -o yaml`
2. Review `deliveryAttempts` array for error messages
3. Common fixes:
   - **401 Unauthorized**: Update Slack webhook secret
   - **404 Not Found**: Verify webhook URL
   - **400 Bad Request**: Check notification content format

**Recovering from Circuit Breaker Open**:
- Circuit auto-recovers after 60s timeout
- Manual recovery: Delete and recreate notification (forces new circuit state)

---

### **For Developers**

**Adding New Delivery Channels**:
1. Implement error classification in delivery service
2. Use `retry.HTTPError` for HTTP status codes
3. Let controller handle retry logic (don't retry in service)
4. Return errors immediately - controller will retry

**Example**:
```go
func (s *EmailDeliveryService) Deliver(ctx context.Context, notification *NotificationRequest) error {
    resp, err := s.httpClient.Post(s.apiURL, "application/json", payload)
    if err != nil {
        return err // Controller will classify as transient
    }
    
    if resp.StatusCode >= 400 {
        return &retry.HTTPError{
            StatusCode: resp.StatusCode,
            Message:    fmt.Sprintf("email API returned %d", resp.StatusCode),
        }
    }
    
    return nil
}
```

---

## 🧪 **Testing Strategy**

### **Unit Tests**
- **Error classification**: 12 table-driven tests (BR-NOT-052)
- **Retry policy logic**: Max attempts, backoff calculation
- **Circuit breaker**: State transitions, per-channel isolation

### **Integration Tests**
- **Delivery failure recovery**: Mock server returns 503 → 503 → 200
- **Graceful degradation**: Console succeeds, Slack fails → PartiallySent
- **Circuit breaker**: 5 failures → circuit opens → blocks requests

### **E2E Tests**
- **Real Slack outage**: Verify notification persists and retries when Slack recovers
- **Invalid credentials**: Verify immediate failure (no retries)

---

## 📊 **Success Metrics**

- **Retry Success Rate**: >80% of transient errors succeed on retry
- **Circuit Breaker Effectiveness**: <1% cascading failures
- **Partial Success Rate**: >95% of notifications deliver to at least one channel
- **Error Classification Accuracy**: 100% (permanent vs. transient)

---

## 🔗 **Related Documentation**

- [BR-NOT-052: Automatic Retry](../../requirements/BR-NOT-052-automatic-retry.md)
- [BR-NOT-055: Graceful Degradation](../../requirements/BR-NOT-055-graceful-degradation.md)
- [Retry Policy Implementation](../../../../pkg/notification/retry/policy.go)
- [Circuit Breaker Implementation](../../../../pkg/notification/retry/circuit_breaker.go)

---

**Version**: 1.0  
**Last Updated**: 2025-10-12  
**Status**: Production-Ready ✅

