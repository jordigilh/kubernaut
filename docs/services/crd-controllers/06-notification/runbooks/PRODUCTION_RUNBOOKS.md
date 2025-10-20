# Notification Controller Production Runbooks

**Version**: 1.0 (v3.1 Enhancement)
**Last Updated**: 2025-10-18
**Purpose**: Operational runbooks for common Notification Controller failure scenarios

---

## Overview

This document provides operational runbooks for the most common Notification Controller issues encountered in production. Each runbook includes investigation steps, resolution procedures, and escalation criteria.

**Related Metrics**:
- `notification_failure_rate` - Current notification failure rate (percentage)
- `notification_stuck_duration_seconds` - Time notifications spend in Delivering phase

---

## Runbook 1: High Notification Failure Rate (>10%)

### Severity
⚠️ **HIGH** - Indicates systemic delivery issues

### Trigger Conditions
- Notification failure rate exceeds 10% for more than 30 minutes
- Alert: `notification_failure_rate{namespace="*"} > 10`

### Investigation Steps

1. **Check NotificationRequest failures**:
   ```bash
   kubectl get notificationrequest -A --field-selector status.phase=Failed
   ```

   Expected output: List of failed notifications with reasons

2. **Check Slack webhook health**:
   ```bash
   curl -X POST <webhook-url> -d '{"text":"health check"}'
   ```

   Expected output: `200 OK` with `"ok": true` response

3. **Check controller logs**:
   ```bash
   kubectl logs -n kubernaut-system deployment/notification-controller --tail=100 | grep -i error
   ```

   Look for patterns:
   - `Slack delivery failed permanently` (Category C errors)
   - `Slack delivery failed, will retry` (Category B errors)
   - `Permanent failure for channel` (Invalid webhook configuration)

4. **Check delivery attempts distribution**:
   ```bash
   kubectl get notificationrequest -A -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.totalAttempts}{"\n"}{end}' | sort -k2 -rn | head -20
   ```

   High attempt counts (>5) indicate persistent retry loops

### Common Root Causes

| Symptom | Root Cause | Resolution |
|---------|-----------|------------|
| All Slack deliveries fail with 401/403 | **Invalid webhook URL** | Update webhook URL in NotificationRequest spec or global config |
| All deliveries fail with timeout | **Network connectivity issue** | Check firewall rules, proxy configuration, DNS resolution |
| Intermittent 429 errors | **Rate limiting** | Reduce notification frequency, implement token bucket rate limiter |
| Failures after controller restart | **Configuration not loaded** | Verify ConfigMap/Secret mounts, check controller logs |

### Resolution Procedures

#### If Slack webhook invalid:
```bash
# Update webhook URL in NotificationRequest
kubectl patch notificationrequest <name> -n <namespace> --type=merge -p '{"spec":{"channels":{"slack":{"webhookURL":"https://hooks.slack.com/services/NEW/URL"}}}}'

# Or update global configuration
kubectl edit configmap notification-controller-config -n kubernaut-system
```

#### If rate limiting:
```bash
# Check current rate limit config
kubectl get configmap notification-controller-config -n kubernaut-system -o yaml | grep -A 5 rateLimit

# Adjust rate limit (example: reduce to 5 msg/min)
kubectl patch configmap notification-controller-config -n kubernaut-system --type=merge -p '{"data":{"rateLimit":"5"}}'

# Restart controller to apply changes
kubectl rollout restart deployment/notification-controller -n kubernaut-system
```

#### If transient errors:
```bash
# Verify retry backoff configuration
kubectl get configmap notification-controller-config -n kubernaut-system -o yaml | grep -A 10 retry

# Check if exponential backoff is working (should see: 30s → 60s → 120s → 240s → 480s)
kubectl logs -n kubernaut-system deployment/notification-controller --tail=100 | grep "will retry"
```

### Escalation Criteria
- **Escalate to Platform Team if**: Failure rate >15% for >1 hour
- **Escalate to Security Team if**: 401/403 errors across all notifications (potential credential compromise)
- **Escalate to Network Team if**: Timeout errors across all notifications (network infrastructure issue)

### Prevention
- Set up Prometheus alert: `alert: NotificationFailureRateHigh`
- Monitor webhook URL validity in pre-deployment checks
- Implement canary deployments for configuration changes

---

## Runbook 2: Stuck Notifications (>10min)

### Severity
⚠️ **MEDIUM** - Indicates delivery latency issues

### Trigger Conditions
- Notifications stuck in "Delivering" phase for more than 10 minutes
- Alert: `notification_stuck_duration_seconds{quantile="0.95"} > 600`

### Investigation Steps

1. **Identify stuck notifications**:
   ```bash
   kubectl get notificationrequest -A --field-selector status.phase=Delivering
   ```

   Expected output: List of notifications currently being delivered

2. **Check delivery attempts**:
   ```bash
   kubectl get notificationrequest <name> -n <namespace> -o jsonpath='{.status.deliveryAttempts}' | jq .
   ```

   Look for:
   - High attempt count (>5) indicates persistent failures
   - Long time between attempts indicates incorrect backoff
   - Empty attempts array indicates delivery hasn't started

3. **Check Slack API latency**:
   ```bash
   curl -w "%{time_total}" -X POST <webhook-url> -d '{"text":"latency check"}'
   ```

   Expected output: < 2 seconds for healthy API

   If >5 seconds, Slack API is slow or unreachable

4. **Check controller reconciliation loop**:
   ```bash
   kubectl logs -n kubernaut-system deployment/notification-controller --tail=100 | grep "Reconciling NotificationRequest"
   ```

   Look for:
   - Missing reconciliation logs (controller not picking up changes)
   - Excessive reconciliation (infinite loop)
   - Error logs during reconciliation

### Common Root Causes

| Symptom | Root Cause | Resolution |
|---------|-----------|------------|
| Stuck at 0 delivery attempts | **Controller not reconciling** | Restart controller, check watch connection |
| Stuck with 5+ attempts, all failed | **Max retries reached** | Force mark as failed, investigate underlying issue |
| Long delays between retries | **Incorrect backoff calculation** | Verify controller version, check backoff logic |
| Stuck after controller restart | **CRD not requeued** | Patch CRD status to trigger reconciliation |

### Resolution Procedures

#### If retry count >5 (max retries reached):
```bash
# Check failure reason
kubectl get notificationrequest <name> -n <namespace> -o jsonpath='{.status.message}'

# Force mark as failed to stop retry loop
kubectl patch notificationrequest <name> -n <namespace> --type=merge -p '{"status":{"phase":"Failed","reason":"ManualIntervention","message":"Manually failed after investigation"}}'

# Investigate underlying Slack API issues
kubectl logs -n kubernaut-system deployment/notification-controller --tail=200 | grep <name>
```

#### If Slack slow (>5s latency):
```bash
# Check Slack API status
curl https://status.slack.com/api/v2.0.0/current

# Increase timeout in controller config
kubectl patch configmap notification-controller-config -n kubernaut-system --type=merge -p '{"data":{"slackTimeout":"20s"}}'

# Restart controller
kubectl rollout restart deployment/notification-controller -n kubernaut-system
```

#### If stuck in queue (controller not reconciling):
```bash
# Check controller health
kubectl get pods -n kubernaut-system -l app=notification-controller

# Check controller logs for watch connection issues
kubectl logs -n kubernaut-system deployment/notification-controller --tail=100 | grep -i watch

# Restart notification-controller
kubectl rollout restart deployment/notification-controller -n kubernaut-system

# Force reconciliation by patching CRD
kubectl patch notificationrequest <name> -n <namespace> --type=merge -p '{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"'$(date +%Y-%m-%dT%H:%M:%S)'"}}}'
```

### Escalation Criteria
- **Escalate to Platform Team if**: >10 notifications stuck for >10 minutes
- **Escalate to Slack Team if**: Slack API latency >10s consistently
- **Escalate to Development Team if**: Controller reconciliation loop broken

### Prevention
- Set up Prometheus alert: `alert: NotificationStuckTooLong`
- Monitor `notification_stuck_duration_seconds` histogram
- Implement controller health checks with auto-restart
- Set up Slack API status monitoring

---

## Monitoring & Alerting

### Prometheus Metrics

```yaml
# notification_failure_rate gauge
- alert: NotificationFailureRateHigh
  expr: notification_failure_rate{namespace="*"} > 10
  for: 30m
  labels:
    severity: high
  annotations:
    summary: "High notification failure rate ({{ $value }}%)"
    description: "Notification failure rate has been above 10% for 30 minutes"
    runbook: "https://docs/notification/runbooks#runbook-1"

# notification_stuck_duration_seconds histogram
- alert: NotificationStuckTooLong
  expr: histogram_quantile(0.95, notification_stuck_duration_seconds) > 600
  for: 5m
  labels:
    severity: medium
  annotations:
    summary: "Notifications stuck for >10 minutes (p95)"
    description: "95th percentile stuck duration is {{ $value }}s"
    runbook: "https://docs/notification/runbooks#runbook-2"
```

### Dashboard Queries

```promql
# Failure rate over time
rate(notification_deliveries_total{status="failed"}[5m]) / rate(notification_deliveries_total[5m]) * 100

# Stuck notifications by namespace
count by (namespace) (notification_phase{phase="Delivering"}) unless (notification_stuck_duration_seconds < 600)

# Delivery latency (p50, p95, p99)
histogram_quantile(0.50, notification_delivery_duration_seconds_bucket)
histogram_quantile(0.95, notification_delivery_duration_seconds_bucket)
histogram_quantile(0.99, notification_delivery_duration_seconds_bucket)
```

---

## Quick Reference

### Useful Commands

```bash
# Check all failed notifications
kubectl get notificationrequest -A --field-selector status.phase=Failed

# Check stuck notifications
kubectl get notificationrequest -A --field-selector status.phase=Delivering

# Get notification details
kubectl describe notificationrequest <name> -n <namespace>

# Check controller logs
kubectl logs -n kubernaut-system deployment/notification-controller --tail=100

# Restart controller
kubectl rollout restart deployment/notification-controller -n kubernaut-system

# Force reconciliation
kubectl annotate notificationrequest <name> -n <namespace> kubectl.kubernetes.io/restartedAt="$(date +%Y-%m-%dT%H:%M:%S)" --overwrite
```

### Error Code Reference

| Error Category | Status Code | Retryable | Backoff |
|---------------|------------|-----------|---------|
| Category A | CRD Not Found | No | - |
| Category B | 429, 5xx | Yes | 30s → 480s |
| Category C | 401, 403, 4xx | No | - |
| Category D | Conflict | Yes | Immediate |
| Category E | Sanitization Error | Yes | Degraded |

---

## Related Documentation

- [Notification Controller Implementation](../controller-implementation.md)
- [v3.1 Enhancements](../implementation/IMPLEMENTATION_PLAN_V3.0.md#enhancement-1-notification-specific-error-handling)
- [Error Handling Categories](../implementation/IMPLEMENTATION_PLAN_V3.0.md#error-categories-for-notification-delivery)
- [Integration Tests](../../../../test/integration/notification/)

---

**Last Updated**: 2025-10-18
**Version**: 1.0 (v3.1)
**Maintained By**: Platform Team

