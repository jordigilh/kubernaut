# WorkflowExecution - Production Runbooks

**Parent Document**: [IMPLEMENTATION_PLAN_V3.0.md](./IMPLEMENTATION_PLAN_V3.0.md)
**Version**: v1.0
**Last Updated**: 2025-12-03
**Status**: ✅ Ready for Implementation

---

## Document Purpose

This appendix contains production runbooks for the WorkflowExecution Controller, aligned with the Day 12 Production Readiness deliverables in [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](../../../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md).

---

## 📚 Runbook Index

| ID | Runbook | Triggers On | Automation |
|----|---------|-------------|------------|
| RB-WE-001 | High Error Rate | `workflowexecution_errors_total` spike | Alert + Dashboard |
| RB-WE-002 | PipelineRun Stuck | Phase=Running > 60min | Alert |
| RB-WE-003 | Resource Lock Not Releasing | PipelineRun exists > cooldown | Alert |
| RB-WE-004 | Tekton Unavailable | Controller crash loop | Alert |
| RB-WE-005 | High Skip Rate | `workflowexecution_skip_total` spike | Dashboard |

---

## RB-WE-001: High WorkflowExecution Error Rate

### Alert Definition

```yaml
# prometheus/rules/workflowexecution.yaml
groups:
  - name: workflowexecution
    rules:
      - alert: WorkflowExecutionHighErrorRate
        expr: |
          (
            sum(rate(workflowexecution_errors_total[5m])) 
            / 
            sum(rate(workflowexecution_phase_transitions_total[5m]))
          ) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High WorkflowExecution error rate"
          description: "Error rate is {{ $value | humanizePercentage }} (threshold: 10%)"
          runbook_url: "https://docs.kubernaut.ai/runbooks/RB-WE-001"
```

### Symptoms

- Alert: `WorkflowExecutionHighErrorRate` firing
- Metric: `workflowexecution_errors_total` increasing rapidly
- Logs: Multiple `level=error` entries in workflow-execution pod

### Diagnosis Steps

```bash
# Step 1: Check error breakdown by category
kubectl exec -it -n kubernaut-system deploy/workflow-execution -- \
  curl -s localhost:9090/metrics | grep workflowexecution_errors_total

# Expected output:
# workflowexecution_errors_total{category="validation",reason="ConfigurationError"} 5
# workflowexecution_errors_total{category="external",reason="TektonUnavailable"} 12

# Step 2: Check recent WFE failures
kubectl get wfe -A -o custom-columns=\
  NAME:.metadata.name,\
  PHASE:.status.phase,\
  REASON:.status.failureDetails.reason,\
  AGE:.metadata.creationTimestamp

# Step 3: Check controller logs
kubectl logs -n kubernaut-system deploy/workflow-execution --since=10m | grep -i error
```

### Resolution by Error Category

| Category | Resolution |
|----------|------------|
| `validation` | Check RemediationOrchestrator creating valid specs |
| `external` | Check Tekton health (see RB-WE-004) |
| `permission` | Fix RBAC (check ClusterRoleBinding) |
| `execution` | Investigate PipelineRun failures |

### Post-Incident

- [ ] Verify error rate returned to baseline
- [ ] Update runbook if new error pattern discovered
- [ ] Create incident report if customer-impacting

---

## RB-WE-002: PipelineRun Stuck in Running

### Alert Definition

```yaml
- alert: WorkflowExecutionStuck
  expr: |
    (
      time() - workflowexecution_phase_start_timestamp{phase="Running"}
    ) > 3600
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "WorkflowExecution stuck in Running phase"
    description: "WFE {{ $labels.name }} has been Running for > 1 hour"
```

### Symptoms

- WFE in `Running` phase for > 60 minutes
- Associated PipelineRun also stuck
- No status updates in WFE

### Diagnosis Steps

```bash
# Step 1: Find stuck WFEs
kubectl get wfe -A -o custom-columns=\
  NAME:.metadata.name,\
  NAMESPACE:.metadata.namespace,\
  PHASE:.status.phase,\
  STARTED:.status.startTime \
  | grep Running

# Step 2: Find associated PipelineRun
WFE_NAME="<stuck-wfe-name>"
TARGET=$(kubectl get wfe $WFE_NAME -o jsonpath='{.spec.targetResource}')
PR_NAME=$(echo -n "$TARGET" | sha256sum | cut -c1-16 | xargs printf "wfe-%s")
kubectl get pipelinerun -n kubernaut-workflows $PR_NAME -o yaml

# Step 3: Check PipelineRun TaskRuns
kubectl get taskrun -n kubernaut-workflows -l tekton.dev/pipelineRun=$PR_NAME

# Step 4: Check TaskRun logs
kubectl logs -n kubernaut-workflows -l tekton.dev/pipelineRun=$PR_NAME --all-containers
```

### Resolution

```bash
# Option A: Wait for Tekton timeout (if configured)
# PipelineRun will fail after Pipeline.spec.timeout

# Option B: Manual cancellation (if blocking)
kubectl patch pipelinerun -n kubernaut-workflows $PR_NAME \
  --type=merge -p '{"spec":{"status":"Cancelled"}}'

# Option C: Delete PipelineRun (releases lock immediately)
# WARNING: This may leave cluster in inconsistent state
kubectl delete pipelinerun -n kubernaut-workflows $PR_NAME
```

### Prevention

- Ensure `pipeline.spec.timeout` is set in workflow bundles
- Configure RO 60-minute timeout as outer boundary
- Review workflows for potential infinite loops

---

## RB-WE-003: Resource Lock Not Releasing

### Alert Definition

```yaml
- alert: WorkflowExecutionLockStuck
  expr: |
    (
      workflowexecution_phase_transitions_total{phase="Completed"} > 0
    ) and (
      time() - workflowexecution_completion_timestamp > 600
    ) and (
      workflowexecution_lock_released == 0
    )
  for: 5m
  labels:
    severity: warning
```

### Symptoms

- WFE completed/failed but lock not released
- New WFEs for same target stuck in Skipped/ResourceBusy
- PipelineRun still exists after cooldown expired

### Diagnosis Steps

```bash
# Step 1: Check WFE status
WFE_NAME="<wfe-name>"
kubectl get wfe $WFE_NAME -o yaml | grep -A20 "status:"

# Step 2: Check if PipelineRun still exists
TARGET=$(kubectl get wfe $WFE_NAME -o jsonpath='{.spec.targetResource}')
PR_NAME=$(echo -n "$TARGET" | sha256sum | cut -c1-16 | xargs printf "wfe-%s")
kubectl get pipelinerun -n kubernaut-workflows $PR_NAME

# Step 3: Check cooldown timing
COMPLETION=$(kubectl get wfe $WFE_NAME -o jsonpath='{.status.completionTime}')
echo "Completed at: $COMPLETION"
echo "Cooldown: 5 minutes (default)"
echo "Current time: $(date -u +%Y-%m-%dT%H:%M:%SZ)"

# Step 4: Check controller logs for deletion attempts
kubectl logs -n kubernaut-system deploy/workflow-execution | grep "delete.*PipelineRun"
```

### Resolution

```bash
# Option A: Wait for controller to retry deletion
# Controller will retry on next reconcile

# Option B: Manual PipelineRun deletion
kubectl delete pipelinerun -n kubernaut-workflows $PR_NAME

# Option C: Restart controller (resets all watches)
kubectl rollout restart -n kubernaut-system deploy/workflow-execution
```

### Prevention

- Ensure Tekton TTLSecondsAfterFinished is configured as backup
- Monitor `workflowexecution_lock_released` metric
- Review finalizer logic for edge cases

---

## RB-WE-004: Tekton Unavailable

### Alert Definition

```yaml
- alert: WorkflowExecutionTektonUnavailable
  expr: |
    up{job="workflow-execution"} == 0
    and on() (count(kube_deployment_status_replicas_ready{deployment="tekton-pipelines-controller"}) == 0)
  for: 2m
  labels:
    severity: critical
  annotations:
    summary: "WorkflowExecution controller down - Tekton unavailable"
    description: "Controller crashed because Tekton CRDs are not installed"
```

### Symptoms

- workflow-execution pod in CrashLoopBackOff
- Pod logs show: "Required dependency check failed"
- Tekton Pipelines controller not running

### Diagnosis Steps

```bash
# Step 1: Check workflow-execution pod status
kubectl get pods -n kubernaut-system -l app=workflow-execution

# Step 2: Check pod logs
kubectl logs -n kubernaut-system -l app=workflow-execution --previous

# Step 3: Check Tekton installation
kubectl get crd | grep tekton.dev
kubectl get pods -n tekton-pipelines

# Step 4: Check Tekton controller
kubectl logs -n tekton-pipelines deploy/tekton-pipelines-controller --tail=50
```

### Resolution

```bash
# Option A: Install Tekton Pipelines
kubectl apply -f https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml

# Option B: Wait for Tekton to recover
kubectl rollout status -n tekton-pipelines deploy/tekton-pipelines-controller

# After Tekton is available:
# workflow-execution will auto-recover via Kubernetes restart
```

### Prevention

- Include Tekton in cluster prerequisites
- Add Tekton health to readiness probe dependencies
- Document Tekton version requirements

---

## RB-WE-005: High Skip Rate

### Alert Definition

```yaml
- alert: WorkflowExecutionHighSkipRate
  expr: |
    (
      sum(rate(workflowexecution_skip_total[5m])) 
      / 
      sum(rate(workflowexecution_phase_transitions_total{phase="Pending"}[5m]))
    ) > 0.5
  for: 10m
  labels:
    severity: info
  annotations:
    summary: "High WorkflowExecution skip rate"
    description: "{{ $value | humanizePercentage }} of WFEs are being skipped"
```

### Symptoms

- Many WFEs in `Skipped` phase
- SkipReason mostly `ResourceBusy` or `RecentlyRemediated`
- May indicate alert storm or duplicate signals

### Diagnosis Steps

```bash
# Step 1: Check skip breakdown
kubectl exec -it -n kubernaut-system deploy/workflow-execution -- \
  curl -s localhost:9090/metrics | grep workflowexecution_skip_total

# Step 2: List skipped WFEs by reason
kubectl get wfe -A -o custom-columns=\
  NAME:.metadata.name,\
  PHASE:.status.phase,\
  SKIP_REASON:.status.skipDetails.reason,\
  TARGET:.spec.targetResource \
  | grep Skipped

# Step 3: Check if same target getting many requests
kubectl get wfe -A -o jsonpath='{range .items[*]}{.spec.targetResource}{"\n"}{end}' | sort | uniq -c | sort -rn | head -10
```

### Resolution

| Skip Reason | Resolution |
|-------------|------------|
| `ResourceBusy` | Normal - lock working as expected |
| `RecentlyRemediated` | Normal - cooldown working as expected |
| High volume of both | Check Gateway deduplication, may need tuning |

### Prevention

- Review Gateway storm aggregation configuration
- Consider increasing cooldown if too many duplicate requests
- Monitor source of duplicate RemediationRequests

---

## 📊 Metrics Reference

### Key Metrics for Runbooks

| Metric | Type | Used In |
|--------|------|---------|
| `workflowexecution_errors_total` | Counter | RB-WE-001 |
| `workflowexecution_phase_transitions_total` | Counter | RB-WE-001, RB-WE-005 |
| `workflowexecution_phase_duration_seconds` | Histogram | RB-WE-002 |
| `workflowexecution_skip_total` | Counter | RB-WE-005 |
| `workflowexecution_lock_released` | Gauge | RB-WE-003 |
| `up{job="workflow-execution"}` | Gauge | RB-WE-004 |

### Dashboard Queries

```promql
# Error rate (for RB-WE-001)
sum(rate(workflowexecution_errors_total[5m])) by (category, reason)

# Phase duration P95 (for RB-WE-002)
histogram_quantile(0.95, sum(rate(workflowexecution_phase_duration_seconds_bucket[5m])) by (le, phase))

# Skip rate (for RB-WE-005)
sum(rate(workflowexecution_skip_total[5m])) by (reason)
```

---

## 🔄 Rollback Plan

### Rollback Triggers

| Trigger | Threshold | Action |
|---------|-----------|--------|
| Error rate spike | >5% increase from baseline | Initiate rollback |
| Controller crash loop | >3 restarts in 5 min | Immediate rollback |
| Lock not releasing | Any | Investigate, consider rollback |
| Data corruption | Any | Immediate rollback + incident |

### Rollback Procedure

```bash
# Step 1: Rollback deployment
kubectl rollout undo deployment/workflow-execution -n kubernaut-system

# Step 2: Verify rollback
kubectl rollout status deployment/workflow-execution -n kubernaut-system

# Step 3: Check logs
kubectl logs -n kubernaut-system deploy/workflow-execution --tail=50

# Step 4: Verify metrics recovering
kubectl exec -it -n kubernaut-system deploy/workflow-execution -- \
  curl -s localhost:9090/metrics | grep workflowexecution
```

### Post-Rollback

- [ ] Create incident report
- [ ] Schedule root cause analysis
- [ ] Plan fix and re-deployment

---

## References

- [Production Runbooks Template](../../../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md#-production-runbooks-template-day-12-deliverable)
- [DD-WE-001: Resource Locking Safety](../../../../architecture/decisions/DD-WE-001-resource-locking-safety.md)
- [DD-WE-003: Lock Persistence](../../../../architecture/decisions/DD-WE-003-resource-lock-persistence.md)
- [metrics-slos.md](../metrics-slos.md)

