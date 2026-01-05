# Kubernaut Must-Gather Diagnostic Collection Tool

**Version**: 1.0.0
**Status**: Production-Ready
**Business Requirement**: [BR-PLATFORM-001](../../docs/requirements/BR-PLATFORM-001-must-gather-diagnostic-collection.md)

---

## 📋 Overview

Kubernaut Must-Gather is an enterprise diagnostic collection tool following the OpenShift must-gather pattern. It collects comprehensive diagnostic data from Kubernaut platform deployments for support and troubleshooting.

### What Does It Collect?

- ✅ **6 CRD Types**: RemediationRequests, RemediationApprovalRequests, SignalProcessings, AIAnalyses, WorkflowExecutions, NotificationRequests
- ✅ **8 Service Logs**: Gateway, Data Storage, HolmesGPT API, Notification, Signal Processing, AI Analysis, Workflow Execution, Remediation Orchestrator
- ✅ **Tekton Resources**: PipelineRuns, TaskRuns, Pipelines, Tasks, Operator logs (CRITICAL for V1.0)
- ✅ **Database Infrastructure**: PostgreSQL and Redis logs, configurations
- ✅ **DataStorage REST API**: Workflow catalog (50 workflows), Audit events (1000 events, last 24h)
- ✅ **Cluster State**: RBAC, Storage (PVCs, PVs), Network (Services, NetworkPolicies)
- ✅ **Kubernetes Events**: Last 24h from all Kubernaut namespaces
- ✅ **Metrics**: Prometheus snapshots, ServiceMonitor configurations
- ✅ **Sanitization**: Automated redaction of passwords, tokens, certificates, PII

### Output

Compressed tarball: `kubernaut-must-gather-<timestamp>.tar.gz` (typically <500MB)

---

## 🚀 Usage

### Prerequisites

- `kubectl` with cluster admin access
- Kubernetes cluster with Kubernaut V1.0 deployed
- Cluster connectivity

### Execution Methods

#### Method 1: OpenShift-style (oc adm must-gather)

```bash
oc adm must-gather --image=quay.io/kubernaut/must-gather:latest
```

#### Method 2: Kubernetes-style (kubectl debug)

```bash
kubectl debug node/<node-name> \
  --image=quay.io/kubernaut/must-gather:latest \
  --image-pull-policy=Always -- /usr/bin/gather
```

#### Method 3: Direct pod execution (fallback)

```bash
kubectl run kubernaut-must-gather \
  --image=quay.io/kubernaut/must-gather:latest \
  --rm --attach --restart=Never \
  --serviceaccount=kubernaut-must-gather-sa \
  -- /usr/bin/gather
```

---

## ⚙️ Configuration Options

### Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--since=DURATION` | `24h` | Log collection timeframe (e.g., 24h, 48h, 7d) |
| `--dest-dir=PATH` | `/must-gather` | Output directory path |
| `--no-sanitize` | (sanitization enabled) | **Internal use only** - Disable automatic sanitization |
| `--max-size=MB` | `500` | Maximum collection size in MB |
| `--help`, `-h` | - | Show usage information |

### Examples

```bash
# Default collection (24h logs, 500MB limit)
/usr/bin/gather

# Collect last 48 hours of logs
/usr/bin/gather --since=48h

# Increase size limit to 1GB
/usr/bin/gather --max-size=1000

# Custom output directory
/usr/bin/gather --dest-dir=/tmp/diagnostics
```

---

## 📦 Output Structure

```
kubernaut-must-gather-20260104-123456/
├── cluster-scoped/
│   ├── nodes/
│   │   ├── nodes.yaml
│   │   └── nodes-describe.txt
│   ├── rbac/
│   │   ├── clusterroles.yaml
│   │   ├── clusterrolebindings.yaml
│   │   └── serviceaccounts-*.yaml
│   ├── storage/
│   │   ├── persistentvolumes.yaml
│   │   └── storageclasses.yaml
│   ├── network/
│   │   ├── services-*.yaml
│   │   └── networkpolicies-*.yaml
│   └── config/
│       ├── kubernetes-version.yaml
│       └── webhookconfigurations.yaml
├── crds/
│   ├── remediationrequests/
│   │   ├── crd-definition.yaml
│   │   └── all-instances.yaml
│   ├── remediationapprovalrequests/
│   ├── signalprocessings/
│   ├── aianalyses/
│   ├── workflowexecutions/
│   └── notificationrequests/
├── logs/
│   ├── kubernaut-system/
│   │   ├── gateway-*/
│   │   │   ├── current.log
│   │   │   ├── previous.log (if exists)
│   │   │   └── describe.txt
│   │   ├── datastorage-*/
│   │   ├── holmesgpt-api-*/
│   │   ├── signalprocessing-controller-*/
│   │   ├── aianalysis-controller-*/
│   │   ├── workflowexecution-controller-*/
│   │   └── remediationorchestrator-controller-*/
│   └── kubernaut-notifications/
│       └── notification-controller-*/
├── tekton/
│   ├── pipelineruns/
│   │   ├── all-pipelineruns.yaml
│   │   └── <pipelinerun-name>/
│   │       ├── spec.yaml
│   │       └── logs.txt
│   ├── taskruns/
│   │   └── all-taskruns.yaml
│   ├── pipelines/
│   ├── tasks/
│   └── operator/
│       ├── operator-pods.yaml
│       ├── configmaps.yaml
│       └── *.log
├── datastorage/
│   ├── workflows.json (50 workflows)
│   └── audit-events.json (1000 events, 24h)
├── database/
│   ├── postgresql/
│   │   ├── *.log
│   │   ├── configmaps.yaml
│   │   └── version.txt
│   └── redis/
│       ├── *.log
│       ├── info.txt
│       └── memory-stats.txt
├── events/
│   ├── events-kubernaut-system.yaml
│   ├── events-kubernaut-notifications.yaml
│   ├── events-kubernaut-workflows.yaml
│   └── events-cluster-wide.yaml
├── metrics/
│   ├── gateway-metrics.txt
│   ├── datastorage-metrics.txt
│   ├── servicemonitor-*.yaml
│   └── pods-resource-usage-*.txt
├── collection-metadata.json
├── version-info.yaml
├── sanitization-report.txt
└── SHA256SUMS
```

---

## 🔒 Security & Privacy

### Automatic Sanitization (BR-PLATFORM-001.9)

Must-gather automatically redacts sensitive data:

- ✅ **Secrets**: Data values redacted, keys preserved
- ✅ **Passwords**: Replaced with `********`
- ✅ **API Keys/Tokens**: Replaced with `[REDACTED]`
- ✅ **Certificates**: Replaced with `[CERTIFICATE-REDACTED]`
- ✅ **Private Keys**: Replaced with `[PRIVATE-KEY-REDACTED]`
- ✅ **PII**: Email addresses replaced with `user@[REDACTED]`

**Warning**: The `--no-sanitize` flag is for **internal debugging only** and will expose sensitive data!

### RBAC Requirements

Must-gather requires a ServiceAccount with read-only cluster access:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernaut-must-gather
rules:
- apiGroups: [""]
  resources: ["pods", "pods/log", "events", "configmaps", "secrets", "nodes", "namespaces", "services", "endpoints", "persistentvolumeclaims", "persistentvolumes"]
  verbs: ["get", "list"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
  verbs: ["get", "list"]
- apiGroups: ["kubernaut.ai"]
  resources: ["*"]
  verbs: ["get", "list"]
- apiGroups: ["tekton.dev"]
  resources: ["pipelineruns", "taskruns", "pipelines", "tasks"]
  verbs: ["get", "list"]
- apiGroups: ["apiextensions.k8s.io"]
  resources: ["customresourcedefinitions"]
  verbs: ["get", "list"]
- apiGroups: ["rbac.authorization.k8s.io"]
  resources: ["clusterroles", "clusterrolebindings", "roles", "rolebindings"]
  verbs: ["get", "list"]
```

---

## 📊 Analyzing the Archive

### Extract the Archive

```bash
tar -xzf kubernaut-must-gather-20260104-123456.tar.gz
cd kubernaut-must-gather-20260104-123456
```

### Verify Integrity

```bash
sha256sum -c SHA256SUMS
```

### Review Metadata

```bash
cat collection-metadata.json
```

### Check Sanitization Report

```bash
cat sanitization-report.txt
```

### Common Analysis Tasks

#### View RemediationRequest CRDs
```bash
cat crds/remediationrequests/all-instances.yaml
```

#### Check Gateway Service Logs
```bash
ls logs/kubernaut-system/gateway-*/
cat logs/kubernaut-system/gateway-*/current.log | grep ERROR
```

#### Review Tekton PipelineRun Failures
```bash
cat tekton/pipelineruns/all-pipelineruns.yaml | grep -A 10 "phase: Failed"
```

#### Analyze Workflow Catalog
```bash
jq '.workflows[] | {name, status, signal_type}' datastorage/workflows.json
```

#### Count Audit Events by Type
```bash
jq '.data | group_by(.event_type) | map({event_type: .[0].event_type, count: length})' datastorage/audit-events.json
```

---

## 🏗️ Building the Image

### Build for Local Testing

```bash
cd cmd/must-gather
podman build -t kubernaut-must-gather:dev .
```

### Build for Production

```bash
# Multi-arch build
podman build --platform linux/amd64,linux/arm64 \
  -t quay.io/kubernaut/must-gather:v1.0.0 \
  -t quay.io/kubernaut/must-gather:latest \
  .

# Push to quay.io
podman push quay.io/kubernaut/must-gather:v1.0.0
podman push quay.io/kubernaut/must-gather:latest
```

---

## 🐛 Troubleshooting

### Must-Gather Container Fails to Start

**Problem**: Pod fails with `ImagePullBackOff`

**Solution**: Check image registry and credentials
```bash
kubectl describe pod kubernaut-must-gather
```

### Collection Times Out

**Problem**: Collection exceeds timeout limits

**Solution**: Reduce collection scope with `--since`
```bash
/usr/bin/gather --since=12h
```

### Archive Size Exceeds Limit

**Problem**: Collection stops at 500MB

**Solution**: Increase size limit or reduce timeframe
```bash
/usr/bin/gather --max-size=1000 --since=12h
```

### Permission Denied Errors

**Problem**: Cannot access resources

**Solution**: Verify RBAC permissions
```bash
kubectl auth can-i get pods --all-namespaces --as=system:serviceaccount:default:kubernaut-must-gather-sa
```

---

## 📚 References

- [BR-PLATFORM-001](../../docs/requirements/BR-PLATFORM-001-must-gather-diagnostic-collection.md) - Business requirements
- [OpenShift Must-Gather Documentation](https://docs.openshift.com/container-platform/latest/support/gathering-cluster-data.html)
- [Kubernetes Debug Documentation](https://kubernetes.io/docs/tasks/debug/debug-application/debug-running-pod/)

---

## 📄 License

Apache License 2.0

---

## 🤝 Support

For support with must-gather:
- **Issues**: [GitHub Issues](https://github.com/jordigilh/kubernaut/issues)
- **Documentation**: `docs/` directory in repository
- **Support Team**: Attach must-gather archive to support tickets

---

**Kubernaut Must-Gather** - Enterprise-grade diagnostic collection for Kubernetes AIOps platform troubleshooting.

