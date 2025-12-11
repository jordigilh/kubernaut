# WorkflowExecution Troubleshooting Guide

**Version**: v1.0
**Last Updated**: 2025-12-06
**Service**: WorkflowExecution Controller
**Related**: [Runbook](../../operations/runbooks/workflowexecution-runbook.md)

---

## 📋 Quick Diagnostic Commands

```bash
# Check WorkflowExecution CRDs
kubectl get workflowexecutions -A

# Check WorkflowExecution status
kubectl describe workflowexecution <name> -n <namespace>

# Check PipelineRuns in execution namespace
kubectl get pipelineruns -n kubernaut-workflows

# Check controller logs
kubectl logs -l app=workflowexecution-controller -n kubernaut-system --tail=100

# Check Tekton controller status
kubectl get pods -n tekton-pipelines
```

---

## 🔍 Common Issues and Solutions

### Issue 1: WorkflowExecution Stuck in "Pending" Phase

**Symptoms**:
- WorkflowExecution remains in `Pending` phase
- No PipelineRun created in `kubernaut-workflows` namespace

**Diagnostic Commands**:
```bash
# Check WFE status
kubectl get wfe <name> -n <namespace> -o yaml

# Check if there's an existing PipelineRun for same target
kubectl get pipelineruns -n kubernaut-workflows -l kubernaut.ai/target-resource=<target>

# Check controller logs for skip reasons
kubectl logs -l app=workflowexecution-controller -n kubernaut-system | grep -i "skip\|pending"
```

**Common Causes**:

| Cause | Solution |
|-------|----------|
| **Resource Busy** (BR-WE-009) | Another WFE is running for same target. Wait for completion. |
| **Cooldown Active** (BR-WE-010) | Recent remediation on same target. Check `status.skipDetails.recentRemediation`. |
| **Exhausted Retries** (BR-WE-012) | Max consecutive failures reached. Check `status.consecutiveFailures`. |
| **Previous Execution Failed** (BR-WE-012) | Execution failure blocks retries. Manual review needed. |
| **Tekton CRDs Missing** | Controller crashes if Tekton not installed. Install Tekton Pipelines. |

**Resolution**:
```bash
# Check skip reason
kubectl get wfe <name> -n <namespace> -o jsonpath='{.status.skipDetails}'

# If ResourceBusy, find the running WFE
kubectl get wfe -A -o json | jq '.items[] | select(.status.phase=="Running")'

# If RecentlyRemediated, check cooldown expiry
kubectl get wfe <name> -n <namespace> -o jsonpath='{.status.skipDetails.recentRemediation}'
```

---

### Issue 2: WorkflowExecution Stuck in "Running" Phase

**Symptoms**:
- WorkflowExecution in `Running` phase for extended time
- PipelineRun may or may not exist

**Diagnostic Commands**:
```bash
# Check PipelineRun status
kubectl get pipelinerun -n kubernaut-workflows -l kubernaut.ai/workflowexecution=<wfe-name>

# Check PipelineRun details
kubectl describe pipelinerun <pr-name> -n kubernaut-workflows

# Check TaskRun status (for detailed failure info)
kubectl get taskruns -n kubernaut-workflows -l tekton.dev/pipelineRun=<pr-name>

# Check pod logs for the running task
kubectl logs -l tekton.dev/pipelineRun=<pr-name> -n kubernaut-workflows --all-containers
```

**Common Causes**:

| Cause | Solution |
|-------|----------|
| **Task stuck pulling image** | Check image exists, registry credentials, network connectivity |
| **Task waiting for resources** | Check resource quotas in `kubernaut-workflows` namespace |
| **Task execution timeout** | Workflow may have long-running operations. Check Pipeline timeout settings. |
| **ServiceAccount missing** | Verify `kubernaut-workflow-runner` SA exists with correct permissions |
| **OCI bundle resolution failed** | Check bundle URL, registry authentication |

**Resolution**:
```bash
# Check TaskRun conditions
kubectl get taskrun -n kubernaut-workflows -l tekton.dev/pipelineRun=<pr-name> -o yaml

# Check pod events
kubectl get events -n kubernaut-workflows --field-selector involvedObject.kind=Pod

# Check if SA exists
kubectl get sa kubernaut-workflow-runner -n kubernaut-workflows
```

---

### Issue 3: WorkflowExecution Failed - OCI Bundle Not Found

**Symptoms**:
- WorkflowExecution in `Failed` phase
- Error message: `failed to resolve bundle` or `image not found`

**Diagnostic Commands**:
```bash
# Check WFE failure details
kubectl get wfe <name> -n <namespace> -o jsonpath='{.status.failureDetails}'

# Check PipelineRun failure
kubectl describe pipelinerun <pr-name> -n kubernaut-workflows | grep -A 10 "Conditions"

# Test bundle resolution manually
tkn bundle list <bundle-url>
```

**Common Causes**:

| Cause | Solution |
|-------|----------|
| **Bundle URL typo** | Verify `spec.workflowRef.bundleRef` URL is correct |
| **Registry auth missing** | Add imagePullSecrets to ServiceAccount |
| **Bundle not pushed** | Push bundle with `tkn bundle push` |
| **Wrong tag** | Verify tag exists in registry |

**Resolution**:
```bash
# Verify bundle exists
crane manifest <bundle-url>

# Check registry auth
kubectl get sa kubernaut-workflow-runner -n kubernaut-workflows -o jsonpath='{.imagePullSecrets}'

# Add registry secret if missing
kubectl patch sa kubernaut-workflow-runner -n kubernaut-workflows \
  -p '{"imagePullSecrets": [{"name": "registry-credentials"}]}'
```

---

### Issue 4: WorkflowExecution Failed - Permission Denied

**Symptoms**:
- WorkflowExecution in `Failed` phase
- Error message contains `forbidden`, `unauthorized`, or `RBAC`

**Diagnostic Commands**:
```bash
# Check failure details
kubectl get wfe <name> -n <namespace> -o jsonpath='{.status.failureDetails.naturalLanguageSummary}'

# Check TaskRun that failed
kubectl describe taskrun <tr-name> -n kubernaut-workflows

# Check ServiceAccount permissions
kubectl auth can-i --list --as=system:serviceaccount:kubernaut-workflows:kubernaut-workflow-runner
```

**Common Causes**:

| Cause | Solution |
|-------|----------|
| **ClusterRole missing** | Apply `kubernaut-workflow-runner` ClusterRole |
| **ClusterRoleBinding missing** | Apply ClusterRoleBinding to SA |
| **Target namespace not accessible** | Add namespace to RBAC rules |
| **New resource type needed** | Extend ClusterRole with new permissions |

**Resolution**:
```bash
# Check current permissions
kubectl get clusterrolebinding kubernaut-workflow-runner -o yaml

# Verify ClusterRole exists
kubectl get clusterrole kubernaut-workflow-runner -o yaml

# Test specific permission
kubectl auth can-i patch deployments --as=system:serviceaccount:kubernaut-workflows:kubernaut-workflow-runner -n <target-namespace>
```

---

### Issue 5: Consecutive Failures and Exponential Backoff

**Symptoms**:
- WorkflowExecution being skipped repeatedly
- `status.skipDetails.reason: ExhaustedRetries` or `PreviousExecutionFailed`

**Diagnostic Commands**:
```bash
# Check consecutive failure count
kubectl get wfe -l kubernaut.ai/target-resource=<target> -o jsonpath='{.items[*].status.consecutiveFailures}'

# Check next allowed execution time
kubectl get wfe -l kubernaut.ai/target-resource=<target> -o jsonpath='{.items[*].status.nextAllowedExecution}'

# Check if it was an execution failure
kubectl get wfe <name> -o jsonpath='{.status.failureDetails.wasExecutionFailure}'
```

**Understanding Backoff Behavior** (per DD-WE-004):

| Failure Type | `wasExecutionFailure` | Backoff Behavior |
|--------------|----------------------|------------------|
| **Pre-Execution** (infra issues) | `false` | Exponential backoff: 10s → 20s → 40s → ... → 10min max |
| **Execution** (workflow failed) | `true` | **Immediate block** - no retries, manual review required |

**Resolution**:

For **Pre-Execution Failures** (transient issues):
```bash
# Wait for backoff to expire (check nextAllowedExecution)
kubectl get wfe <name> -o jsonpath='{.status.nextAllowedExecution}'

# Or fix the infrastructure issue and wait for next attempt
```

For **Execution Failures** (workflow logic failed):
```bash
# This requires manual review - the workflow itself has a bug
# 1. Check failure details
kubectl get wfe <name> -o jsonpath='{.status.failureDetails}'

# 2. Review and fix the workflow
# 3. Create a NEW WorkflowExecution (blocked ones won't retry automatically)
```

---

### Issue 6: Tekton Controller Not Ready

**Symptoms**:
- Controller crashes at startup
- Error: `no matches for kind "PipelineRun" in version "tekton.dev/v1"`

**Diagnostic Commands**:
```bash
# Check Tekton installation
kubectl get crd pipelineruns.tekton.dev

# Check Tekton controller
kubectl get pods -n tekton-pipelines

# Check controller logs
kubectl logs -l app=workflowexecution-controller -n kubernaut-system --previous
```

**Resolution**:
```bash
# Install Tekton Pipelines
kubectl apply -f https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml

# Wait for Tekton to be ready
kubectl wait --for=condition=available deployment/tekton-pipelines-controller -n tekton-pipelines --timeout=300s

# Restart WE controller
kubectl rollout restart deployment/workflowexecution-controller -n kubernaut-system
```

---

### Issue 7: PipelineRun Created But WorkflowExecution Not Updated

**Symptoms**:
- PipelineRun shows `Succeeded` or `Failed`
- WorkflowExecution still shows `Running`

**Diagnostic Commands**:
```bash
# Check if PipelineRun has owner reference
kubectl get pipelinerun <pr-name> -n kubernaut-workflows -o jsonpath='{.metadata.ownerReferences}'

# Check controller reconciliation
kubectl logs -l app=workflowexecution-controller -n kubernaut-system | grep <wfe-name>
```

**Common Causes**:

| Cause | Solution |
|-------|----------|
| **Watch not working** | Controller may need restart |
| **Namespace mismatch** | Verify PipelineRun is in `kubernaut-workflows` |
| **Label missing** | Check PipelineRun has `kubernaut.ai/workflowexecution` label |

**Resolution**:
```bash
# Force reconciliation by updating annotation
kubectl annotate wfe <name> -n <namespace> kubernaut.ai/force-reconcile=$(date +%s) --overwrite

# Or restart controller
kubectl rollout restart deployment/workflowexecution-controller -n kubernaut-system
```

---

## 📊 Metrics-Based Troubleshooting

### High Error Rate Alert

```promql
# Query to find error sources
sum by (reason) (rate(workflowexecution_total{outcome="Failed"}[5m]))
```

| Metric Pattern | Likely Cause |
|----------------|--------------|
| High `PipelineRunCreationFailed` | Bundle resolution or RBAC issues |
| High `TaskFailed` | Workflow logic or target resource issues |
| High `ExhaustedRetries` | Persistent infrastructure issues |
| High `PreviousExecutionFailed` | Workflow bugs requiring manual fix |

### Skip Rate Analysis

```promql
# Breakdown of skip reasons
sum by (reason) (rate(workflowexecution_skip_total[5m]))
```

| Skip Reason | Action |
|-------------|--------|
| `ResourceBusy` | Normal if many WFEs for same target; reduce if excessive |
| `RecentlyRemediated` | Normal; indicates cooldown working |
| `ExhaustedRetries` | Investigate infrastructure issues |
| `PreviousExecutionFailed` | Review and fix workflow, then create new WFE |

---

## 🔗 Related Documentation

- [Runbook: High Error Rate](../../operations/runbooks/workflowexecution-runbook.md#rb-we-001)
- [Runbook: Stuck PipelineRun](../../operations/runbooks/workflowexecution-runbook.md#rb-we-002)
- [DD-WE-004: Exponential Backoff](../../architecture/decisions/DD-WE-004-exponential-backoff-cooldown.md)
- [Workflow Author's Guide](../../guides/user/workflow-authoring.md)

---

## 📝 Changelog

| Version | Date | Changes |
|---------|------|---------|
| v1.0 | 2025-12-06 | Initial release with 7 common issues |




