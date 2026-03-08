# Workflow Authoring Guide

**Version**: v1.1
**Last Updated**: 2026-03-04
**Audience**: Platform Engineers, SREs, DevOps Engineers
**Prerequisites**: Familiarity with Tekton Pipelines, Kubernetes, OCI registries

---

## 📋 Overview

This guide explains how to create, package, and deploy remediation workflows for Kubernaut. Workflows are Tekton Pipelines packaged as OCI bundles that execute automated remediation actions.

**Key Concepts**:
- **Workflow**: A Tekton Pipeline that performs remediation actions
- **OCI Bundle**: Container image containing the Pipeline definition
- **Parameters**: Dynamic values passed from AI Analysis to the workflow
- **Target Resource**: The Kubernetes resource being remediated

---

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Kubernaut Workflow Execution                      │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌──────────────┐    ┌──────────────────┐    ┌──────────────────┐  │
│  │ AIAnalysis   │───▶│ WorkflowExecution│───▶│ Tekton Pipeline  │  │
│  │ (selects     │    │ (creates         │    │ (executes in     │  │
│  │  workflow)   │    │  PipelineRun)    │    │  kubernaut-      │  │
│  └──────────────┘    └──────────────────┘    │  workflows ns)   │  │
│         │                    │               └──────────────────┘  │
│         │                    │                        │             │
│         ▼                    ▼                        ▼             │
│  ┌──────────────┐    ┌──────────────────┐    ┌──────────────────┐  │
│  │ OCI Bundle   │    │ Parameters       │    │ Target Resource  │  │
│  │ Reference    │    │ (from AI)        │    │ (remediated)     │  │
│  └──────────────┘    └──────────────────┘    └──────────────────┘  │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 🚀 Quick Start: Your First Workflow

### Step 1: Create a Simple Pipeline

Create a file named `restart-deployment.yaml`:

```yaml
apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  name: restart-deployment
  labels:
    kubernaut.ai/workflow-type: remediation
    kubernaut.ai/category: deployment
spec:
  description: |
    Restarts a deployment by triggering a rollout restart.
    Used for transient issues like memory leaks or stuck pods.

  params:
    - name: namespace
      type: string
      description: Namespace of the deployment
    - name: deployment-name
      type: string
      description: Name of the deployment to restart
    - name: dry-run
      type: string
      default: "false"
      description: If true, only validate without executing

  tasks:
    - name: validate-deployment
      taskRef:
        name: kubectl-validate
        kind: ClusterTask
      params:
        - name: resource
          value: "deployment/$(params.deployment-name)"
        - name: namespace
          value: "$(params.namespace)"

    - name: restart-deployment
      runAfter:
        - validate-deployment
      when:
        - input: "$(params.dry-run)"
          operator: in
          values: ["false"]
      taskRef:
        name: kubectl-restart
        kind: ClusterTask
      params:
        - name: resource
          value: "deployment/$(params.deployment-name)"
        - name: namespace
          value: "$(params.namespace)"

    - name: verify-rollout
      runAfter:
        - restart-deployment
      taskRef:
        name: kubectl-rollout-status
        kind: ClusterTask
      params:
        - name: resource
          value: "deployment/$(params.deployment-name)"
        - name: namespace
          value: "$(params.namespace)"
        - name: timeout
          value: "300s"
```

### Step 2: Package as OCI Bundle

```bash
# Install tkn CLI if not already installed
# See: https://tekton.dev/docs/cli/

# Package the pipeline as an OCI bundle
tkn bundle push ghcr.io/your-org/kubernaut-workflows/restart-deployment:v1.0.0 \
  -f restart-deployment.yaml

# Verify the bundle
tkn bundle list ghcr.io/your-org/kubernaut-workflows/restart-deployment:v1.0.0
```

### Step 3: Create a Workflow Schema

Create a `workflow-schema.yaml` (per BR-WORKFLOW-004) that declares the workflow's metadata, action type, labels, execution engine, and parameters. This schema describes the workflow to the LLM for discovery and selection.

### Step 4: Register via RemediationWorkflow CRD

Create a `RemediationWorkflow` CRD that embeds the workflow schema in its `.spec`:

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationWorkflow
metadata:
  name: restart-deployment-v1
  namespace: kubernaut-system
spec:
  metadata:
    workflowId: restart-deployment
    version: "1.0.0"
    description:
      what: "Restarts a deployment by triggering a rollout restart"
      whenToUse: "When pods are unhealthy due to transient issues like memory leaks"
      whenNotToUse: "When the issue is a configuration error that will recur after restart"
  actionType: RestartDeployment
  labels:
    severity: ["warning", "critical"]
    environment: ["production", "staging", "development"]
    component: Deployment
    priority: high
  execution:
    engine: tekton
    bundle: "ghcr.io/your-org/kubernaut-workflows/restart-deployment:v1.0.0@sha256:abc123..."
  parameters:
    - name: namespace
      type: string
      required: true
      description: "Namespace of the deployment"
    - name: deployment-name
      type: string
      required: true
      description: "Name of the deployment to restart"
    - name: dry-run
      type: string
      required: false
      description: "If true, only validate without executing"
```

Apply it:

```bash
kubectl apply -f restart-deployment-rw.yaml
```

After successful admission, the AuthWebhook registers the workflow in the DataStorage catalog. Check registration status:

```bash
kubectl get remediationworkflow restart-deployment-v1 -n kubernaut-system -o yaml
# .status.workflowId should contain the DS-assigned UUID
# .status.catalogStatus should be "active"
```

To remove a workflow from the catalog (disable), delete the CRD:

```bash
kubectl delete remediationworkflow restart-deployment-v1 -n kubernaut-system
```

See [BR-WORKFLOW-006](../../requirements/BR-WORKFLOW-006-remediation-workflow-crd.md) and [ADR-058](../../architecture/decisions/ADR-058-webhook-driven-workflow-registration.md) for full CRD specification and architecture details.

---

## 📝 Pipeline Requirements

### Required Labels

All Kubernaut workflows MUST include these labels on the Tekton Pipeline resource. These are workflow catalog labels on Tekton Pipelines, distinct from CRD spec fields (e.g., `spec.signalType (value "alert" per Issue #166)`, `spec.severity` on RemediationRequest) that replaced routing labels in Issue #91.

```yaml
metadata:
  labels:
    kubernaut.ai/workflow-type: remediation    # REQUIRED
    kubernaut.ai/category: <category>          # REQUIRED: deployment|pod|node|service|config
    kubernaut.ai/severity: <severity>          # OPTIONAL: critical|high|medium|low
```

### Required Parameters

Every workflow MUST accept these parameters (injected by WorkflowExecution):

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `namespace` | string | Target resource namespace | `production` |
| `dry-run` | string | Validate without executing | `"true"` or `"false"` |

### Optional Standard Parameters

These are commonly used and recommended:

| Parameter | Type | Description |
|-----------|------|-------------|
| `deployment-name` | string | Deployment to remediate |
| `pod-name` | string | Pod to remediate |
| `node-name` | string | Node to remediate |
| `service-name` | string | Service to remediate |
| `timeout` | string | Operation timeout (e.g., `300s`) |
| `replicas` | string | Target replica count |

### Custom Parameters

AI Analysis can pass custom parameters. Define them in your Pipeline:

```yaml
spec:
  params:
    # Standard parameters
    - name: namespace
      type: string
    - name: dry-run
      type: string
      default: "false"

    # Custom parameters (AI will populate these)
    - name: memory-limit
      type: string
      default: "2Gi"
      description: New memory limit for the container
    - name: cpu-request
      type: string
      default: "500m"
      description: New CPU request for the container
```

---

## 🔧 Pipeline Patterns

### Pattern 1: Validation → Action → Verify

The recommended pattern for most remediations:

```yaml
tasks:
  # 1. Validate preconditions
  - name: validate
    taskRef:
      name: validate-resource

  # 2. Execute remediation
  - name: remediate
    runAfter: [validate]
    when:
      - input: "$(params.dry-run)"
        operator: in
        values: ["false"]
    taskRef:
      name: execute-action

  # 3. Verify success
  - name: verify
    runAfter: [remediate]
    taskRef:
      name: verify-health
```

### Pattern 2: With Rollback Capability

For risky operations, include rollback:

```yaml
tasks:
  - name: capture-state
    taskRef:
      name: capture-current-state

  - name: remediate
    runAfter: [capture-state]
    taskRef:
      name: execute-action

  - name: verify
    runAfter: [remediate]
    taskRef:
      name: verify-health

  # Rollback on failure (using Tekton finally)
  finally:
    - name: rollback-on-failure
      when:
        - input: "$(tasks.verify.status)"
          operator: in
          values: ["Failed"]
      taskRef:
        name: restore-state
      params:
        - name: state
          value: "$(tasks.capture-state.results.state)"
```

### Pattern 3: Multi-Step Sequential

For complex remediations requiring multiple steps:

```yaml
tasks:
  - name: step-1-cordon-node
    taskRef:
      name: kubectl-cordon

  - name: step-2-drain-pods
    runAfter: [step-1-cordon-node]
    taskRef:
      name: kubectl-drain

  - name: step-3-restart-kubelet
    runAfter: [step-2-drain-pods]
    taskRef:
      name: restart-kubelet

  - name: step-4-uncordon-node
    runAfter: [step-3-restart-kubelet]
    taskRef:
      name: kubectl-uncordon

  - name: step-5-verify
    runAfter: [step-4-uncordon-node]
    taskRef:
      name: verify-node-ready
```

---

## 📦 OCI Bundle Best Practices

### Versioning

Use semantic versioning for bundles:

```bash
# Initial release
tkn bundle push ghcr.io/org/workflows/restart-deployment:v1.0.0

# Patch (bug fix)
tkn bundle push ghcr.io/org/workflows/restart-deployment:v1.0.1

# Minor (new feature, backward compatible)
tkn bundle push ghcr.io/org/workflows/restart-deployment:v1.1.0

# Major (breaking changes)
tkn bundle push ghcr.io/org/workflows/restart-deployment:v2.0.0
```

### Tagging Strategy

```bash
# Version tags
ghcr.io/org/workflows/restart-deployment:v1.0.0   # Immutable
ghcr.io/org/workflows/restart-deployment:v1.0     # Latest patch
ghcr.io/org/workflows/restart-deployment:v1       # Latest minor
ghcr.io/org/workflows/restart-deployment:latest   # Latest (NOT recommended for production)
```

### Bundle Signing (Recommended)

For production environments, sign bundles with cosign:

```bash
# Generate key pair (one-time)
cosign generate-key-pair

# Sign the bundle
cosign sign --key cosign.key ghcr.io/org/workflows/restart-deployment:v1.0.0

# Verify signature
cosign verify --key cosign.pub ghcr.io/org/workflows/restart-deployment:v1.0.0
```

---

## 🧪 Testing Workflows Locally

### Option 1: Direct PipelineRun

```yaml
# test-pipelinerun.yaml
apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  name: test-restart-deployment
  namespace: kubernaut-workflows
spec:
  pipelineRef:
    resolver: bundles
    params:
      - name: bundle
        value: ghcr.io/org/workflows/restart-deployment:v1.0.0
      - name: name
        value: restart-deployment
      - name: kind
        value: Pipeline
  params:
    - name: namespace
      value: "test-namespace"
    - name: deployment-name
      value: "test-deployment"
    - name: dry-run
      value: "true"  # Always use dry-run for testing
  serviceAccountName: kubernaut-workflow-runner
```

```bash
# Apply and watch
kubectl apply -f test-pipelinerun.yaml
tkn pipelinerun logs test-restart-deployment -f -n kubernaut-workflows
```

### Option 2: Using tkn CLI

```bash
# Run directly from bundle
tkn pipeline start restart-deployment \
  --namespace kubernaut-workflows \
  --param namespace=test-namespace \
  --param deployment-name=test-deployment \
  --param dry-run=true \
  --serviceaccount kubernaut-workflow-runner \
  --use-pipelinerun restart-deployment-test \
  --showlog
```

### Option 3: Integration Test Setup

Create a test namespace with sample deployment:

```bash
# Create test environment
kubectl create namespace workflow-test
kubectl create deployment test-app --image=nginx:latest -n workflow-test

# Run workflow against test deployment
tkn pipeline start restart-deployment \
  --param namespace=workflow-test \
  --param deployment-name=test-app \
  --param dry-run=false \
  -n kubernaut-workflows \
  --showlog

# Cleanup
kubectl delete namespace workflow-test
```

---

## 🔒 Security Considerations

### ServiceAccount Permissions

Workflows run with `kubernaut-workflow-runner` ServiceAccount in `kubernaut-workflows` namespace. This ServiceAccount has cluster-wide permissions for:

- Deployments, StatefulSets, DaemonSets: get, list, patch, update
- Pods: get, list, delete
- Nodes: get, list, patch, cordon, uncordon
- ConfigMaps, Secrets: get, list (read-only)

### Least Privilege Principle

If your workflow needs additional permissions:

1. **DON'T** modify the default ServiceAccount
2. **DO** request a custom ServiceAccount with specific permissions
3. **DO** document required permissions in the workflow metadata

```yaml
metadata:
  annotations:
    kubernaut.ai/required-permissions: |
      - apiGroups: ["apps"]
        resources: ["deployments"]
        verbs: ["get", "patch"]
```

### Secrets Handling

- **NEVER** log secret values
- **NEVER** pass secrets as plain parameters
- **DO** use Kubernetes Secrets with `secretKeyRef`
- **DO** use workspaces for sensitive data

```yaml
# Bad: Secret in parameter
params:
  - name: api-key
    value: "my-secret-key"  # ❌ NEVER do this

# Good: Reference Kubernetes Secret
workspaces:
  - name: credentials
    secret:
      secretName: api-credentials  # ✅ Correct approach
```

---

## 📊 Workflow Metrics

WorkflowExecution exposes these metrics for your workflows:

| Metric | Description |
|--------|-------------|
| `workflowexecution_total{outcome,workflow}` | Total executions by outcome |
| `workflowexecution_duration_seconds{workflow}` | Execution duration histogram |
| `workflowexecution_skip_total{reason}` | Skipped executions by reason |

### Adding Custom Metrics

Emit custom metrics from your Tasks using Tekton results:

```yaml
tasks:
  - name: remediate
    taskRef:
      name: my-task
    # Results can be used for custom metrics
results:
  - name: items-processed
    description: Number of items processed
  - name: execution-time-ms
    description: Custom execution time
```

---

## 📚 Example Workflows

### Restart Deployment

**Use Case**: Deployment pods are unhealthy or experiencing transient issues.

```yaml
# See: ghcr.io/kubernaut/workflows/restart-deployment:v1.0.0
```

### Scale Deployment

**Use Case**: Deployment needs more/fewer replicas based on load.

```yaml
apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  name: scale-deployment
  labels:
    kubernaut.ai/workflow-type: remediation
    kubernaut.ai/category: deployment
spec:
  params:
    - name: namespace
      type: string
    - name: deployment-name
      type: string
    - name: replicas
      type: string
    - name: dry-run
      type: string
      default: "false"

  tasks:
    - name: validate
      taskRef:
        name: kubectl-validate
      params:
        - name: resource
          value: "deployment/$(params.deployment-name)"
        - name: namespace
          value: "$(params.namespace)"

    - name: scale
      runAfter: [validate]
      when:
        - input: "$(params.dry-run)"
          operator: in
          values: ["false"]
      taskRef:
        name: kubectl-scale
      params:
        - name: resource
          value: "deployment/$(params.deployment-name)"
        - name: namespace
          value: "$(params.namespace)"
        - name: replicas
          value: "$(params.replicas)"

    - name: verify
      runAfter: [scale]
      taskRef:
        name: kubectl-rollout-status
      params:
        - name: resource
          value: "deployment/$(params.deployment-name)"
        - name: namespace
          value: "$(params.namespace)"
```

### Delete Stuck Pod

**Use Case**: Pod is stuck in Terminating or CrashLoopBackOff.

```yaml
apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  name: delete-stuck-pod
  labels:
    kubernaut.ai/workflow-type: remediation
    kubernaut.ai/category: pod
spec:
  params:
    - name: namespace
      type: string
    - name: pod-name
      type: string
    - name: force
      type: string
      default: "false"
    - name: dry-run
      type: string
      default: "false"

  tasks:
    - name: check-pod-status
      taskRef:
        name: kubectl-get-pod
      params:
        - name: pod-name
          value: "$(params.pod-name)"
        - name: namespace
          value: "$(params.namespace)"

    - name: delete-pod
      runAfter: [check-pod-status]
      when:
        - input: "$(params.dry-run)"
          operator: in
          values: ["false"]
      taskRef:
        name: kubectl-delete-pod
      params:
        - name: pod-name
          value: "$(params.pod-name)"
        - name: namespace
          value: "$(params.namespace)"
        - name: force
          value: "$(params.force)"

    - name: verify-deletion
      runAfter: [delete-pod]
      taskRef:
        name: kubectl-wait-deleted
      params:
        - name: resource
          value: "pod/$(params.pod-name)"
        - name: namespace
          value: "$(params.namespace)"
```

---

## 🔗 Related Documentation

- [WorkflowExecution Service Spec](../../services/crd-controllers/03-workflowexecution/README.md)
- [CRD Schema](../../services/crd-controllers/03-workflowexecution/crd-schema.md)
- [Security Configuration](../../services/crd-controllers/03-workflowexecution/security-configuration.md)
- [ADR-044: Tekton Delegation](../../architecture/decisions/ADR-044-workflow-execution-engine-delegation.md)
- [ADR-043: OCI Bundle Standard](../../architecture/decisions/ADR-043-workflow-schema-definition-standard.md)
- [ADR-058: Webhook-Driven Registration](../../architecture/decisions/ADR-058-webhook-driven-workflow-registration.md)
- [BR-WORKFLOW-006: RemediationWorkflow CRD](../../requirements/BR-WORKFLOW-006-remediation-workflow-crd.md)
- [BR-WORKFLOW-004: Workflow Schema Format](../../requirements/BR-WORKFLOW-004-workflow-schema-format.md)

---

## 📞 Support

- **Issues**: Create a GitHub issue with label `workflow-authoring`
- **Questions**: See [Troubleshooting Guide](../../troubleshooting/service-specific/workflowexecution-issues.md)
- **Runbook**: See [WorkflowExecution Runbook](../../operations/runbooks/workflowexecution-runbook.md)

---

## 📝 Changelog

| Version | Date | Changes |
|---------|------|---------|
| v1.1 | 2026-03-04 | Added CRD-based registration (Step 3-4) via RemediationWorkflow CRD. Replaced "no registration needed" with explicit CRD creation instructions. Added links to ADR-058, BR-WORKFLOW-006, BR-WORKFLOW-004. |
| v1.0 | 2025-12-06 | Initial release |






