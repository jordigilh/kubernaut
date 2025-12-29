# Tekton Test Fixtures for WorkflowExecution E2E Testing

This directory contains Tekton Pipelines for testing the WorkflowExecution controller.

## Prerequisites

### 1. Install Tekton Pipelines

For **Kind cluster** (local development):

```bash
# Create Kind cluster with extra port mappings
kind create cluster --name kubernaut-test --config=- <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    extraPortMappings:
      - containerPort: 30085
        hostPort: 8085
        protocol: TCP
EOF

# Install Tekton Pipelines (latest stable)
kubectl apply --filename https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml

# Wait for Tekton to be ready
kubectl wait --for=condition=Ready pods --all -n tekton-pipelines --timeout=120s
```

For **OpenShift** (using the Tekton Operator):

```bash
# Install OpenShift Pipelines Operator from OperatorHub
# Or via CLI:
cat <<EOF | kubectl apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: openshift-pipelines-operator
  namespace: openshift-operators
spec:
  channel: latest
  name: openshift-pipelines-operator-rh
  source: redhat-operators
  sourceNamespace: openshift-marketplace
EOF

# Create TektonConfig instance
cat <<EOF | kubectl apply -f -
apiVersion: operator.tekton.dev/v1alpha1
kind: TektonConfig
metadata:
  name: config
spec:
  profile: all
  targetNamespace: openshift-pipelines
EOF
```

### 2. Install Tekton CLI (tkn)

```bash
# macOS
brew install tektoncd-cli

# Linux (manual)
curl -LO https://github.com/tektoncd/cli/releases/download/v0.35.1/tkn_0.35.1_Linux_x86_64.tar.gz
tar xvzf tkn_0.35.1_Linux_x86_64.tar.gz -C /usr/local/bin tkn
```

## Test Pipelines

### hello-world-pipeline.yaml

A simple pipeline that echoes parameters and exits successfully. Used to test:
- PipelineRun creation from OCI bundle
- Parameter passing (BR-WE-002)
- Status monitoring (BR-WE-003)
- Successful completion handling

**Parameters:**
| Name | Default | Description |
|------|---------|-------------|
| `TARGET_RESOURCE` | `test/deployment/hello` | Target resource identifier |
| `MESSAGE` | `Hello from Kubernaut...` | Message to echo |
| `DELAY_SECONDS` | `2` | Simulated work duration |

### failing-pipeline.yaml

An intentionally failing pipeline. Used to test:
- Failure detection (BR-WE-003)
- Failure details extraction (BR-WE-005)
- Natural language summary generation

**Parameters:**
| Name | Default | Description |
|------|---------|-------------|
| `TARGET_RESOURCE` | `test/deployment/failing` | Target resource identifier |
| `FAILURE_MODE` | `exit` | How to fail: `exit`, `timeout` |
| `FAILURE_MESSAGE` | `Intentional test failure` | Error message |

## Building OCI Bundles

### Local Testing (ttl.sh - temporary registry)

```bash
# Bundle hello-world (expires in 24h)
tkn bundle push ttl.sh/kubernaut-hello-world:1h \
  -f hello-world-pipeline.yaml

# Bundle failing pipeline
tkn bundle push ttl.sh/kubernaut-failing:1h \
  -f failing-pipeline.yaml
```

### Production (ghcr.io)

```bash
# Login to GitHub Container Registry
echo $GITHUB_TOKEN | docker login ghcr.io -u $GITHUB_USER --password-stdin

# Bundle and push hello-world
tkn bundle push ghcr.io/kubernaut/test-workflows/hello-world:v1.0.0 \
  -f hello-world-pipeline.yaml

# Bundle and push failing
tkn bundle push ghcr.io/kubernaut/test-workflows/failing:v1.0.0 \
  -f failing-pipeline.yaml
```

## Running Tests Manually

### 1. Create WorkflowExecution Namespace

```bash
kubectl create namespace kubernaut-workflows
kubectl create serviceaccount kubernaut-workflow-runner -n kubernaut-workflows
```

### 2. Apply RBAC for Workflow Runner

```bash
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernaut-workflow-runner
subjects:
  - kind: ServiceAccount
    name: kubernaut-workflow-runner
    namespace: kubernaut-workflows
roleRef:
  kind: ClusterRole
  name: cluster-admin  # Adjust for production
  apiGroup: rbac.authorization.k8s.io
EOF
```

### 3. Create Test WorkflowExecution

```bash
cat <<EOF | kubectl apply -f -
apiVersion: workflowexecution.kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: test-hello-world
  namespace: default
spec:
  targetResource: "default/deployment/test-app"
  workflowRef:
    workflowId: "hello-world"
    version: "v1.0.0"
    containerImage: "ttl.sh/kubernaut-hello-world:1h"
  parameters:
    TARGET_RESOURCE: "default/deployment/test-app"
    MESSAGE: "Testing WorkflowExecution controller"
    DELAY_SECONDS: "5"
  executionConfig:
    serviceAccountName: "kubernaut-workflow-runner"
    timeoutMinutes: 10
EOF
```

### 4. Watch Execution

```bash
# Watch WorkflowExecution status
kubectl get workflowexecution test-hello-world -w

# Watch PipelineRun in execution namespace
kubectl get pipelinerun -n kubernaut-workflows -w

# View PipelineRun logs
tkn pipelinerun logs -n kubernaut-workflows -f
```

## Verification Checklist

- [ ] Tekton Pipelines installed and running
- [ ] `tkn` CLI installed
- [ ] OCI bundles pushed (ttl.sh for local, ghcr.io for CI)
- [ ] `kubernaut-workflows` namespace created
- [ ] `kubernaut-workflow-runner` ServiceAccount with permissions
- [ ] WorkflowExecution CRD installed
- [ ] WorkflowExecution controller running

## Troubleshooting

### PipelineRun stuck in Pending

```bash
# Check events
kubectl describe pipelinerun -n kubernaut-workflows

# Common issues:
# - Missing ServiceAccount
# - OCI bundle not accessible
# - Tekton not fully started
```

### Bundle Resolution Failed

```bash
# Verify bundle is accessible
tkn bundle list ttl.sh/kubernaut-hello-world:1h

# Check Tekton resolver logs
kubectl logs -n tekton-pipelines -l app.kubernetes.io/component=resolvers
```

