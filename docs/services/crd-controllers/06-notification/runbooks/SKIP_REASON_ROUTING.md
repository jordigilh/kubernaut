# Runbook: Skip-Reason Based Notification Routing

**Version**: 1.0
**Last Updated**: 2025-12-06
**Status**: ✅ Production-Ready
**Related BRs**: BR-NOT-065, BR-NOT-066
**Related DDs**: DD-WE-004 v1.1

---

## 📋 Overview

This runbook documents the skip-reason based notification routing system, which routes notifications differently based on WorkflowExecution skip reasons defined in DD-WE-004.

**Key Principle**: Different skip reasons have different severity implications:
- `PreviousExecutionFailed` → **CRITICAL** (cluster state unknown)
- `ExhaustedRetries` → **HIGH** (infrastructure issues)
- `ResourceBusy` / `RecentlyRemediated` → **LOW** (temporary, auto-resolves)

---

## 🔴 PreviousExecutionFailed (CRITICAL)

### What It Means

A workflow **ran and failed** during execution. The cluster state is **UNKNOWN** because non-idempotent actions may have partially executed.

### Example Scenario

```
Workflow: "increase-replicas"
  Step 1: kubectl patch deployment --replicas +1  ← EXECUTED ✅
  Step 2: kubectl apply memory limits             ← FAILED ❌

Result: Replicas = original + 1 (cluster modified, but not as expected)
```

### Routing Configuration

```yaml
- match:
    kubernaut.ai/skip-reason: PreviousExecutionFailed
  receiver: pagerduty-oncall-critical
  group_wait: 0s  # Send immediately
  repeat_interval: 15m
```

### Operator Actions

1. **Investigate** the failed WorkflowExecution CRD:
   ```bash
   kubectl get workflowexecution <name> -n kubernaut-system -o yaml
   ```

2. **Verify** cluster state manually - check what actions were executed:
   ```bash
   kubectl describe workflowexecution <name> -n kubernaut-system
   kubectl get events --field-selector involvedObject.name=<target-resource>
   ```

3. **Clear** the execution block using annotation:
   ```bash
   kubectl annotate workflowexecution <name> \
     kubernaut.ai/clear-execution-block="acknowledged-by-<operator>" \
     -n kubernaut-system
   ```

4. **Retry** manually if appropriate (after verifying cluster state)

### Prometheus Alert

```yaml
- alert: WorkflowExecutionFailed
  expr: increase(workflow_execution_skip_total{reason="PreviousExecutionFailed"}[5m]) > 0
  for: 0m
  labels:
    severity: critical
  annotations:
    summary: "Workflow execution failed - cluster state unknown"
    description: "WorkflowExecution {{ $labels.name }} failed during execution. Manual intervention required."
```

### SLO Target

- **Time to Acknowledge**: < 15 minutes
- **MTTR Target**: < 30 minutes

---

## 🟠 ExhaustedRetries (HIGH)

### What It Means

5+ **pre-execution** failures have occurred. Infrastructure issues are preventing workflow execution, but the cluster state is **KNOWN** (unchanged - no actions executed).

### Common Causes

| Cause | Frequency | Diagnostic |
|-------|-----------|------------|
| Image pull failures | 40% | `kubectl describe pod <name>` - Events section |
| Resource quota exceeded | 25% | `kubectl describe resourcequota -n <namespace>` |
| Validation webhook failures | 20% | `kubectl logs -n kubernaut-system -l app=validation-webhook` |
| Network policy blocking | 15% | `kubectl get networkpolicy -n <namespace>` |

### Routing Configuration

```yaml
- match:
    kubernaut.ai/skip-reason: ExhaustedRetries
  receiver: slack-ops-high
  group_wait: 1m
  repeat_interval: 1h
```

### Operator Actions

1. **Check** the recent failures in WFE status:
   ```bash
   kubectl get workflowexecution <name> -o jsonpath='{.status.skipDetails}'
   ```

2. **Investigate** infrastructure issues:
   ```bash
   # Check resource quotas
   kubectl describe resourcequota -A | grep -A5 "exceeded"

   # Check image pull errors
   kubectl get events --field-selector reason=Failed -A | grep -i pull

   # Check validation webhooks
   kubectl logs -n kubernaut-system -l app=validation-webhook --tail=100
   ```

3. **Fix** the underlying infrastructure problem

4. **Clear** the exhausted retries state (automatic on next successful WFE)

### Prometheus Alert

```yaml
- alert: WorkflowRetryExhausted
  expr: increase(workflow_execution_skip_total{reason="ExhaustedRetries"}[5m]) > 0
  for: 0m
  labels:
    severity: high
  annotations:
    summary: "Workflow retry exhausted - infrastructure issue"
    description: "WorkflowExecution {{ $labels.name }} exhausted retries. Check infrastructure."
```

### SLO Target

- **Time to Acknowledge**: < 30 minutes
- **MTTR Target**: < 2 hours

---

## 🟢 ResourceBusy (LOW)

### What It Means

Another WorkflowExecution is currently running on the same target resource. This is a **temporary** condition that auto-resolves when the current WFE completes.

### Routing Configuration

```yaml
- match:
    kubernaut.ai/skip-reason: ResourceBusy
  receiver: console-only-bulk
  group_wait: 5m
  group_interval: 30m
```

### Operator Actions

Usually **none required**. The system will automatically retry when the current WFE completes.

If needed, check what's currently running:
```bash
kubectl get workflowexecution -A --field-selector status.phase=Running
```

### SLO Target

- Auto-resolves within 15 minutes (typical WFE duration)
- No operator action expected

---

## 🟢 RecentlyRemediated (LOW)

### What It Means

A cooldown or backoff period is active for this target+workflow combination. This is a **temporary** condition that auto-resolves after the cooldown expires.

### Routing Configuration

```yaml
- match:
    kubernaut.ai/skip-reason: RecentlyRemediated
  receiver: console-only-bulk
  group_wait: 5m
  group_interval: 30m
```

### Operator Actions

Usually **none required**. The system will automatically retry when the cooldown expires.

Check cooldown status:
```bash
kubectl get workflowexecution <name> -o jsonpath='{.status.cooldownExpiresAt}'
```

### SLO Target

- Auto-resolves based on cooldown duration (configurable, default 15 minutes)
- No operator action expected

---

## 📊 Monitoring Dashboard

### Key Metrics

| Metric | PromQL | Description |
|--------|--------|-------------|
| Skip by Reason | `sum by(reason)(workflow_execution_skip_total)` | Total skips by reason |
| Critical Skips | `workflow_execution_skip_total{reason="PreviousExecutionFailed"}` | Execution failures |
| High Skips | `workflow_execution_skip_total{reason="ExhaustedRetries"}` | Exhausted retries |
| Skip Rate | `rate(workflow_execution_skip_total[5m])` | Skip rate over 5 minutes |

### Grafana Panel (JSON)

```json
{
  "title": "WFE Skips by Reason",
  "type": "timeseries",
  "targets": [
    {
      "expr": "sum by(reason)(rate(workflow_execution_skip_total[5m]))",
      "legendFormat": "{{reason}}"
    }
  ],
  "fieldConfig": {
    "defaults": {
      "custom": {
        "lineWidth": 2,
        "fillOpacity": 20
      },
      "color": {
        "mode": "palette-classic"
      }
    },
    "overrides": [
      {
        "matcher": {"id": "byName", "options": "PreviousExecutionFailed"},
        "properties": [{"id": "color", "value": {"fixedColor": "red", "mode": "fixed"}}]
      },
      {
        "matcher": {"id": "byName", "options": "ExhaustedRetries"},
        "properties": [{"id": "color", "value": {"fixedColor": "orange", "mode": "fixed"}}]
      }
    ]
  }
}
```

---

## 🔧 Troubleshooting

### Notifications Not Routing Correctly

1. **Check labels on NotificationRequest**:
   ```bash
   kubectl get notificationrequest <name> -n kubernaut-system -o yaml | grep -A20 labels
   ```

2. **Verify routing config loaded**:
   ```bash
   kubectl logs -f deployment/notification-controller -n kubernaut-system | grep -i "routing"
   ```

3. **Check routing decision in logs**:
   ```bash
   kubectl logs -f deployment/notification-controller -n kubernaut-system | grep "Resolved channels from routing"
   ```

### Skip-Reason Label Not Set

If `kubernaut.ai/skip-reason` label is missing:

1. **Check RO is setting labels** (per DD-WE-004 Q8):
   ```bash
   kubectl logs -f deployment/remediation-orchestrator -n kubernaut-system | grep -i "notification"
   ```

2. **Check WFE status has SkipDetails.Reason**:
   ```bash
   kubectl get workflowexecution <name> -o jsonpath='{.status.skipDetails}'
   ```

### Routing Config Not Loaded

1. **Check ConfigMap exists**:
   ```bash
   kubectl get configmap notification-routing-config -n kubernaut-system
   ```

2. **Verify config format**:
   ```bash
   kubectl get configmap notification-routing-config -n kubernaut-system -o yaml
   ```

3. **Check for parse errors**:
   ```bash
   kubectl logs -f deployment/notification-controller -n kubernaut-system | grep -i "error.*routing"
   ```

---

## 📋 Quick Reference

| Skip Reason | Severity | Default Routing | Operator Action | Auto-Resolves |
|-------------|----------|-----------------|-----------------|---------------|
| `PreviousExecutionFailed` | CRITICAL | PagerDuty | Manual investigation | No |
| `ExhaustedRetries` | HIGH | Slack #ops | Fix infrastructure | No |
| `ResourceBusy` | LOW | Console | None | Yes (15 min) |
| `RecentlyRemediated` | LOW | Console | None | Yes (cooldown) |

---

## 🔗 References

| Document | Description |
|----------|-------------|
| [DD-WE-004](../../../../architecture/decisions/DD-WE-004-exponential-backoff-cooldown.md) | Exponential Backoff Design |
| [BR-NOT-065](../BUSINESS_REQUIREMENTS.md#br-not-065-channel-routing-based-on-labels) | Channel Routing BR |
| [Cross-Team Notice](../../../../handoff/NOTICE_WE_EXPONENTIAL_BACKOFF_DD_WE_004.md) | DD-WE-004 Notice |
| [Example Config](../../../../../config/samples/notification_routing_config.yaml) | Routing Config Example |

---

**Document Version**: 1.0
**Last Updated**: 2025-12-06
**Status**: ✅ Production-Ready



