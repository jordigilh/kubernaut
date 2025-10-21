# Production Runbook: Stuck Notifications

**Runbook ID**: NOTIF-RB-002
**Version**: 1.0.0
**Last Updated**: 2025-10-20
**Severity**: P2 - High
**MTTR Target**: 20 minutes

---

## 📊 Trigger Condition

**Prometheus Alert**:
```yaml
alert: StuckNotifications
expr: |
  histogram_quantile(0.99,
    sum(rate(notification_delivery_duration_seconds_bucket[5m])) by (le, namespace)
  ) > 600
for: 5m
labels:
  severity: high
  component: notification-controller
  runbook: STUCK_NOTIFICATIONS
annotations:
  summary: "Notifications stuck in Delivering phase in {{ $labels.namespace }}"
  description: "P99 notification delivery time is {{ $value }}s (threshold: 600s/10min)"
  dashboard: "https://grafana.example.com/d/notification-controller"
```

**Threshold**: P99 delivery latency >600 seconds (10 minutes)
**Business Impact**: Delayed operator notifications, missed approval windows, increased MTTR for incidents

---

## 🚨 Symptoms

### User-Visible Symptoms
- ✅ Operators report delayed Slack notifications (>10 minutes after incident)
- ✅ AIApprovalRequest CRDs show `waitingForApproval` for extended periods
- ✅ Dashboard shows notifications stuck in "Delivering" phase
- ✅ Approval workflows timing out due to notification delays

### System-Level Symptoms
- ✅ `notification_delivery_duration_seconds{quantile="0.99"}` >600s
- ✅ NotificationRequest CRDs with `status.phase=Delivering` for >10 minutes
- ✅ `notification_stuck_duration_seconds` histogram showing high values
- ✅ Controller reconciliation loop appears slow or blocked

---

## 🔍 Diagnostic Queries

### 1. List Stuck Notifications
```bash
# Show notifications in Delivering phase for >10 minutes
kubectl get notificationrequest -A -o json | \
  jq '.items[] |
    select(.status.phase == "Delivering" and
    (.metadata.creationTimestamp | fromdateiso8601) < (now - 600)) |
    {name: .metadata.name, namespace: .metadata.namespace,
     age: ((now - (.metadata.creationTimestamp | fromdateiso8601)) / 60 | floor),
     attempts: .status.attemptCount}'

# Simple list (field-selector not yet supported for custom fields)
kubectl get notificationrequest -A --field-selector status.phase=Delivering
```

### 2. Check P99 Delivery Latency
```promql
# P99 delivery duration by namespace
histogram_quantile(0.99,
  sum(rate(notification_delivery_duration_seconds_bucket[5m])) by (le, namespace)
)

# P50, P95, P99 comparison (should see P99 >> P95)
quantile_over_time(0.50, notification_delivery_duration_seconds[10m])  # P50
quantile_over_time(0.95, notification_delivery_duration_seconds[10m])  # P95
quantile_over_time(0.99, notification_delivery_duration_seconds[10m])  # P99

# Expected healthy state: P50 <5s, P95 <30s, P99 <120s
# Stuck state: P99 >600s
```

### 3. Identify Delivery Bottlenecks
```promql
# Stuck duration by namespace (>10min)
sum(notification_stuck_duration_seconds_bucket{le="600"}) by (namespace)

# Delivery attempts per notification (high retries indicate issues)
avg(notification_retry_count) by (namespace)

# Notifications in Delivering phase right now
sum(notification_phase{phase="Delivering"}) by (namespace)
```

### 4. Check Controller Health
```bash
# Controller pod status and age
kubectl get pods -n kubernaut-system -l app=notification-controller \
  -o custom-columns=NAME:.metadata.name,STATUS:.status.phase,AGE:.metadata.creationTimestamp

# Controller reconciliation rate (should be >0.5 req/sec normally)
kubectl logs -n kubernaut-system -l app=notification-controller --tail=100 | \
  grep "Reconciling NotificationRequest" | wc -l

# Check for controller errors
kubectl logs -n kubernaut-system -l app=notification-controller --tail=200 | \
  grep -E "error|Error|ERROR|panic" | tail -20

# Check controller memory/CPU usage
kubectl top pods -n kubernaut-system -l app=notification-controller
```

---

## 🧰 Root Cause Analysis

### Slack API Slow Response - 50% of cases

**Symptoms**:
- Controller logs show: `"Slack API response time: 15.2s"` or similar high latencies
- Slack API not returning errors, just slow
- Console notifications succeed quickly, only Slack is slow

**Diagnostic Commands**:
```bash
# Check Slack API latency from controller pod
POD=$(kubectl get pods -n kubernaut-system -l app=notification-controller \
  -o jsonpath='{.items[0].metadata.name}')

# Test Slack webhook latency
kubectl exec -n kubernaut-system $POD -- \
  time curl -X POST "$SLACK_WEBHOOK_URL" \
  -H 'Content-Type: application/json' \
  -d '{"text":"Latency test"}' \
  --max-time 30 -w "\nTime: %{time_total}s\n"

# Expected: <2s for healthy Slack API
# Stuck state: >10s or timeout
```

**External Verification**:
- Check Slack Status Page: https://status.slack.com/
- Check Slack API latency from external monitoring tool
- Compare with other services calling Slack API

**Fix**:
1. **Immediate**: Set lower timeout for Slack API calls (currently 30s → reduce to 10s)
2. **Short-term**: Implement circuit breaker to fail-fast on slow Slack API
3. **Long-term**: Add fallback notification channel (Console-only mode)

**Configuration Change**:
```bash
# Reduce Slack API timeout
kubectl set env deployment/notification-controller -n kubernaut-system \
  SLACK_API_TIMEOUT=10s  # Default: 30s
```

---

### Controller Reconciliation Loop Blocked - 30% of cases

**Symptoms**:
- Controller logs frozen (no new log entries)
- Multiple notifications stuck simultaneously
- Controller pod shows high CPU or memory usage
- Restart count increasing

**Diagnostic Commands**:
```bash
# Check controller resource usage
kubectl top pods -n kubernaut-system -l app=notification-controller

# Check if controller is in CrashLoopBackoff
kubectl get pods -n kubernaut-system -l app=notification-controller \
  -o jsonpath='{.items[*].status.containerStatuses[*].state}'

# Check controller events
kubectl get events -n kubernaut-system --field-selector involvedObject.name=<POD_NAME>

# Check for goroutine leaks (requires debug endpoint)
POD=$(kubectl get pods -n kubernaut-system -l app=notification-controller -o jsonpath='{.items[0].metadata.name}')
kubectl port-forward -n kubernaut-system $POD 8080:8080 &
curl http://localhost:8080/debug/pprof/goroutine?debug=1 | grep "^goroutine" | wc -l
# Expected: <100 goroutines
# Leak: >1000 goroutines
```

**Fix**:
1. **Immediate**: Restart controller pod
   ```bash
   kubectl rollout restart deployment/notification-controller -n kubernaut-system
   ```
2. **Investigation**: Capture debug pprof profile before restart
   ```bash
   curl http://localhost:8080/debug/pprof/heap > heap-$(date +%s).prof
   curl http://localhost:8080/debug/pprof/goroutine > goroutine-$(date +%s).prof
   ```
3. **Long-term**: Analyze profiles to identify goroutine/memory leak root cause

---

### Status Update Conflicts (Category D) - 15% of cases

**Symptoms**:
- Controller logs: `"Category D: Status update conflict"` or `"Conflict updating status"`
- Notifications retry multiple times but don't progress
- High rate of status update attempts

**Diagnostic Commands**:
```promql
# Check status update conflict rate
rate(notification_status_update_conflicts_total[5m])

# Retry count distribution (high conflicts → high retries)
histogram_quantile(0.99, sum(rate(notification_retry_count_bucket[5m])) by (le))
```

```bash
# Check for concurrent controllers (should be only 1 replica)
kubectl get deployment -n kubernaut-system notification-controller \
  -o jsonpath='{.status.replicas}'

# Expected: 1
# Issue: >1 (multiple controllers competing)
```

**Fix**:
1. Verify only 1 controller replica is running:
   ```bash
   kubectl scale deployment/notification-controller -n kubernaut-system --replicas=1
   ```
2. Check if status update retry logic is working (should auto-recover within 3 retries)
3. Increase retry count if conflicts persist:
   ```bash
   kubectl set env deployment/notification-controller -n kubernaut-system \
     STATUS_UPDATE_RETRIES=5  # Default: 3
   ```

---

### Database/API Server Latency - 5% of cases

**Symptoms**:
- All controllers slow, not just notification controller
- `kubectl` commands slow
- API server metrics show high latency

**Diagnostic Commands**:
```bash
# Check API server response time
time kubectl get nodes

# Check etcd latency (if access available)
kubectl get --raw /metrics | grep etcd_request_duration_seconds

# Check controller-manager metrics
kubectl top pods -n kube-system -l component=kube-controller-manager
```

**Fix**:
1. **Platform issue** - escalate to cluster infrastructure team
2. No controller-level fix available
3. Monitor API server health dashboard

---

## 🔧 Remediation Steps

### Step 1: Identify Root Cause (5 minutes)
```bash
# Quick triage checklist
echo "=== Controller Health ==="
kubectl get pods -n kubernaut-system -l app=notification-controller

echo "=== Stuck Notifications Count ==="
kubectl get notificationrequest -A --field-selector status.phase=Delivering | wc -l

echo "=== Controller Logs (Recent Errors) ==="
kubectl logs -n kubernaut-system -l app=notification-controller --tail=50 | grep -i error

echo "=== Slack API Test ==="
POD=$(kubectl get pods -n kubernaut-system -l app=notification-controller -o jsonpath='{.items[0].metadata.name}')
kubectl exec -n kubernaut-system $POD -- time curl -X POST "$SLACK_WEBHOOK_URL" \
  -H 'Content-Type: application/json' -d '{"text":"Test"}' --max-time 10
```

**Interpretation**:
- Controller unhealthy + High CPU/Memory → **Reconciliation loop blocked** (30% of cases)
- Slack API test >10s → **Slack API slow** (50% of cases)
- Logs show "Conflict" errors → **Status update conflicts** (15% of cases)
- `kubectl` commands slow → **API server latency** (5% of cases)

### Step 2: Apply Quick Fix Based on Root Cause (10 minutes)

**If Slack API Slow**:
```bash
# Reduce Slack API timeout to fail-fast
kubectl set env deployment/notification-controller -n kubernaut-system \
  SLACK_API_TIMEOUT=10s

# Enable Console-only fallback for critical notifications
kubectl patch configmap -n kubernaut-system notification-config \
  --type merge -p '{"data":{"FALLBACK_TO_CONSOLE":"true"}}'
```

**If Reconciliation Loop Blocked**:
```bash
# Capture debug info first (optional, 2 min)
POD=$(kubectl get pods -n kubernaut-system -l app=notification-controller -o jsonpath='{.items[0].metadata.name}')
kubectl port-forward -n kubernaut-system $POD 8080:8080 &
curl http://localhost:8080/debug/pprof/goroutine > goroutine-$(date +%s).prof
kill %1  # Stop port-forward

# Restart controller
kubectl rollout restart deployment/notification-controller -n kubernaut-system
```

**If Status Update Conflicts**:
```bash
# Scale to single replica (if multiple)
kubectl scale deployment/notification-controller -n kubernaut-system --replicas=1

# Increase retry count
kubectl set env deployment/notification-controller -n kubernaut-system \
  STATUS_UPDATE_RETRIES=5
```

**If API Server Latency**:
```bash
# No controller-level fix - escalate to platform team
echo "Escalating to #platform-infrastructure - API server latency issue"
```

### Step 3: Clear Stuck Notifications (5 minutes)
```bash
# Manual intervention: Force requeue stuck notifications by annotation
kubectl get notificationrequest -A --field-selector status.phase=Delivering -o json | \
  jq -r '.items[] | "\(.metadata.namespace) \(.metadata.name)"' | \
  while read ns name; do
    echo "Requeuing $ns/$name"
    kubectl annotate notificationrequest -n $ns $name \
      kubernaut.ai/force-requeue="$(date +%s)" --overwrite
  done

# Alternative: Delete and recreate (only if notification is idempotent)
# kubectl delete notificationrequest -n <namespace> <name>
```

### Step 4: Verify Fix (5 minutes)
```bash
# Wait for controller restart
kubectl rollout status deployment/notification-controller -n kubernaut-system

# Check P99 latency decreasing
watch -n 5 'kubectl get --raw /metrics | grep notification_delivery_duration_seconds | grep "0.99"'

# Check stuck notifications clearing
watch -n 5 'kubectl get notificationrequest -A --field-selector status.phase=Delivering | wc -l'

# Expected: Count drops to 0 within 5 minutes
```

---

## 🤖 Automation & Prevention

### Prometheus Alert with Auto-Remediation
```yaml
alert: StuckNotifications
expr: |
  histogram_quantile(0.99,
    sum(rate(notification_delivery_duration_seconds_bucket[5m])) by (le, namespace)
  ) > 600
for: 5m
labels:
  severity: high
  component: notification-controller
  runbook: STUCK_NOTIFICATIONS
  auto_remediate: "conditional"  # Only if reconciliation loop blocked
annotations:
  summary: "Notifications stuck in {{ $labels.namespace }}"
  remediation: |
    1. Check controller health: kubectl get pods -n kubernaut-system -l app=notification-controller
    2. Test Slack API: curl -X POST $SLACK_WEBHOOK_URL --max-time 10
    3. If Slack slow: kubectl set env deployment/notification-controller SLACK_API_TIMEOUT=10s
    4. If controller blocked: kubectl rollout restart deployment/notification-controller
```

### Auto-Restart on Controller Blocked (>20 minutes)
```yaml
alert: NotificationControllerBlocked
expr: |
  (
    time() - max(notification_last_reconciliation_timestamp_seconds) by (namespace)
  ) > 1200  # 20 minutes since last successful reconciliation
for: 2m
labels:
  severity: critical
  component: notification-controller
  auto_remediate: "true"  # Auto-restart controller
annotations:
  summary: "Notification controller blocked for >20 minutes"
  remediation: "kubectl rollout restart deployment/notification-controller -n kubernaut-system"
```

### Preventive Monitoring
```promql
# Alert on P95 latency approaching threshold (early warning)
histogram_quantile(0.95,
  sum(rate(notification_delivery_duration_seconds_bucket[5m])) by (le, namespace)
) > 300  # 5 minutes (before hitting 10 min P99 threshold)

# Alert on growing stuck notification count
sum(notification_phase{phase="Delivering"}) by (namespace) > 10

# Alert on increasing retry rates (indicates delivery issues)
avg(notification_retry_count) by (namespace) > 3
```

---

## 📊 Success Criteria

- ✅ P99 delivery latency drops below 120 seconds within 5 minutes of remediation
- ✅ No notifications stuck in "Delivering" phase for >5 minutes
- ✅ Controller reconciliation rate >0.5 req/sec
- ✅ `notification_stuck_duration_seconds{le="600"}` counter stops increasing
- ✅ Test notification delivered within 30 seconds

---

## 📞 Escalation Path

| Time Since Alert | Action | Contact |
|---|---|---|
| **0-10 min** | Auto-remediation attempt | Automated |
| **10-20 min** | Manual remediation by SRE on-call | #sre-oncall Slack channel |
| **20-30 min** | Escalate to Platform Engineering (if API server issue) | @platform-engineering |
| **30+ min** | Page Engineering Manager | PagerDuty: P2 incident → P1 after 30 min |

---

## 🔗 Related Runbooks

- [HIGH_FAILURE_RATE.md](./HIGH_FAILURE_RATE.md) - For high notification failure rates (>10%)
- Notification Controller Architecture: [docs/services/crd-controllers/06-notification/overview.md](../overview.md)
- Performance Tuning: [docs/services/crd-controllers/06-notification/controller-implementation.md](../controller-implementation.md)

---

## 📝 Post-Incident Actions

After resolving the incident:

1. **Analyze debug profiles** captured during incident (goroutine, heap)
2. **Document Slack API latency** patterns observed
3. **Review timeout configuration** (adjust SLACK_API_TIMEOUT if needed)
4. **Add monitoring** for newly discovered bottlenecks
5. **Update this runbook** with lessons learned

---

## 🔬 Advanced Debugging

### Capture Full Debug Profile
```bash
POD=$(kubectl get pods -n kubernaut-system -l app=notification-controller -o jsonpath='{.items[0].metadata.name}')
kubectl port-forward -n kubernaut-system $POD 8080:8080 &

# Goroutine profile (identify blocked goroutines)
curl http://localhost:8080/debug/pprof/goroutine?debug=2 > goroutine-full.txt

# Heap profile (identify memory leaks)
curl http://localhost:8080/debug/pprof/heap > heap.prof

# CPU profile (30s sample)
curl http://localhost:8080/debug/pprof/profile?seconds=30 > cpu.prof

# Analyze with pprof tool
go tool pprof -http=:9090 heap.prof
```

### Analyze Stuck Notification Details
```bash
# Get full details of a stuck notification
kubectl get notificationrequest -n <namespace> <name> -o yaml

# Check for common issues:
# - spec.channels contains invalid channel
# - status.deliveryResults shows repeated errors
# - status.attemptCount at max retries
# - metadata.ownerReferences points to deleted parent CRD
```

---

**Last Verified**: 2025-10-20
**Verified By**: Notification Controller v1.0.1
**MTTR Improvement**: 40 min → 20 min after v3.1 enhancements



