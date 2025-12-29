# BR-PLATFORM-001: Must-Gather Diagnostic Collection

**Document Version**: 1.0
**Date**: December 17, 2025
**Status**: Business Requirements Specification
**Category**: Platform & Diagnostics
**Priority**: P1 - Required for Production Supportability

---

## 1. Business Purpose

### 1.1 Problem Statement
Production support and troubleshooting of Kubernaut requires comprehensive diagnostic data collection from Kubernetes clusters. Manual collection is error-prone, incomplete, and time-consuming, leading to:
- Extended incident resolution times (MTTR)
- Missed diagnostic information critical for root cause analysis
- Inconsistent support data quality across incidents
- Privacy and security risks from ad-hoc data collection

### 1.2 Business Value
A standardized must-gather capability provides:
- **Reduced MTTR**: 50-70% reduction in diagnostic data collection time
- **Complete Diagnostics**: Comprehensive capture of all Kubernaut-related cluster state
- **Support Efficiency**: Consistent, predictable data format for support teams
- **Privacy Compliance**: Automated sanitization of sensitive information
- **Production Readiness**: Industry-standard diagnostic capability for enterprise deployments

### 1.3 Industry Standards Compliance
This capability follows established Kubernetes ecosystem best practices:
- **OpenShift must-gather**: Industry-leading diagnostic collection pattern
- **Kubernetes debug patterns**: Standard `kubectl` debug workflows
- **CNCF best practices**: Cloud-native diagnostic collection standards
- **Enterprise support requirements**: Expected capability for production Kubernetes operators

---

## 2. Mandatory Requirements

### 2.1 Core Must-Gather Capabilities

#### BR-PLATFORM-001.1: Container Image Distribution
**MUST** provide a container image for must-gather execution following industry standards:
- **Image Repository**: `quay.io/kubernaut/must-gather:latest`
- **Versioned Tags**: Semantic versioning aligned with Kubernaut releases
- **Multi-Architecture**: Support for `amd64` and `arm64` architectures
- **Minimal Base**: Security-focused base image (UBI9 minimal or distroless)
- **Tool Dependencies**: Include `kubectl`, `jq`, `tar`, `gzip` utilities

**Rationale**: Container-based must-gather enables consistent execution across diverse Kubernetes distributions (OpenShift, vanilla K8s, managed K8s services).

**Reference**: OpenShift must-gather pattern - https://docs.openshift.com/container-platform/latest/support/gathering-cluster-data.html

---

#### BR-PLATFORM-001.2: CRD Resource Collection
**MUST** collect all Kubernaut Custom Resource Definitions and instances:

**CRDs to Collect**:
- `remediationrequests.kubernaut.ai`
- `signalprocessings.kubernaut.ai`
- `aianalyses.kubernaut.ai`
- `workflowexecutions.kubernaut.ai`
- `remediationapprovalrequests.kubernaut.ai`
- `notificationrequests.kubernaut.ai`

**Collection Scope**:
- **All Namespaces**: Collect CRD instances across all namespaces
- **Cluster-Scoped**: Include cluster-scoped CRD instances
- **Full Spec & Status**: Capture complete resource definitions including status fields
- **Output Format**: YAML format with metadata preservation

**Rationale**: CRD instances contain complete orchestration state, remediation history, and workflow execution context essential for troubleshooting.

---

#### BR-PLATFORM-001.3: Service Pod Logs Collection
**MUST** collect logs from all Kubernaut service pods:

**Services to Collect**:
- Gateway Service (`gateway-*` pods)
- RemediationOrchestrator (`remediationorchestrator-*` pods)
- WorkflowExecution (`workflowexecution-*` pods)
- AIAnalysis (`aianalysis-*` pods)
- SignalProcessing (`signalprocessing-*` pods)
- Notification Service (`notification-*` pods)
- DataStorage Service (`datastorage-*` pods)
- HolmesGPT API (`holmesgpt-api-*` pods)
- Any operator/controller pods

**Log Collection Requirements**:
- **Timeframe**: Last 24 hours of logs (configurable via `--since` flag)
- **All Containers**: Include init containers and sidecar containers
- **Previous Instances**: Collect logs from crashed/restarted pods (`--previous` flag)
- **Tail Lines**: Last 10,000 lines per container (configurable)
- **Timestamps**: Preserve RFC3339 timestamps

**Rationale**: Service logs contain error traces, API interactions, and runtime behavior critical for debugging production issues.

---

#### BR-PLATFORM-001.4: Kubernetes Events Collection
**MUST** collect Kubernetes events for troubleshooting context:

**Event Scope**:
- **Kubernaut Namespaces**: All events from Kubernaut service namespaces
- **Related Namespaces**: Events from namespaces where remediation actions execute
- **System Events**: Node events, cluster-level events impacting Kubernaut
- **Timeframe**: Last 24 hours (configurable)

**Event Filtering**:
- **Resource Types**: Filter by Kubernaut CRD types and service pods
- **Event Types**: Include `Warning`, `Normal`, and `Error` events
- **Output Format**: Structured JSON for automated analysis

**Rationale**: Events provide audit trail of Kubernetes API interactions, scheduling failures, and resource state changes.

---

#### BR-PLATFORM-001.5: Configuration & Secrets Collection
**MUST** collect ConfigMaps and Secrets (sanitized) for configuration analysis:

**ConfigMaps**:
- **Service Configurations**: All Kubernaut service ConfigMaps
- **Feature Flags**: Feature toggle configurations
- **HolmesGPT Configuration**: Endpoint URL, version compatibility matrix, timeout settings
- **Full Content**: Capture complete ConfigMap data

**Secrets** (Sanitized):
- **Metadata Only**: Collect Secret names, namespaces, and labels
- **Keys List**: Enumerate Secret keys without values
- **No Sensitive Data**: **NEVER** collect actual Secret values
- **Privacy Compliance**: Follow GDPR/CCPA data minimization principles

**Rationale**: Configuration issues are common production problems. Sanitized collection enables diagnosis without exposing credentials.

---

#### BR-PLATFORM-001.6: Cluster State Collection
**MUST** collect relevant cluster state information:

**Node Information**:
- Node resource capacity and allocations
- Node conditions and taints
- Kubelet version and runtime information

**Namespace Information**:
- Kubernaut service namespaces
- Resource quotas and limit ranges
- RBAC policies affecting Kubernaut

**API Server Information**:
- Kubernetes version
- Enabled feature gates
- API server flags (non-sensitive)

**Webhook Configurations** (cluster-scoped):
- ValidatingWebhookConfigurations
- MutatingWebhookConfigurations
- Webhook service endpoints and CA bundles
- Webhook failure policies and timeout settings

**RBAC Resources**:
- **Service Accounts**: All ServiceAccounts in Kubernaut namespaces
- **ServiceAccount Tokens**: Token secrets (metadata only, no token values)
- **ClusterRoles/ClusterRoleBindings**: Filter by `kubernaut.ai` labels
- **Roles/RoleBindings**: In Kubernaut namespaces
- **RBAC Validation**: `kubectl auth can-i` checks for key operations

**Storage Resources**:
- **PersistentVolumeClaims**: In Kubernaut namespaces
- **PersistentVolumes**: Bound to Kubernaut PVCs
- **StorageClasses**: Used by Kubernaut
- **VolumeSnapshots**: If applicable
- **Storage Metrics**: Capacity and usage information

**Network Resources**:
- **Services**: All Kubernaut service objects
- **Endpoints/EndpointSlices**: Connectivity status
- **NetworkPolicies**: In Kubernaut namespaces
- **Ingresses/Routes**: If applicable (OpenShift)

**Rationale**: Cluster state provides context for resource contention, scheduling issues, version compatibility problems, permission issues, storage failures, and network connectivity problems.

---

#### BR-PLATFORM-001.6a: DataStorage Service API Collection
**MUST** collect workflow and data via DataStorage REST API:

**Workflow Catalog** (via DataStorage REST API):
- **Endpoint**: `GET /api/v1/workflows`
- **Storage**: PostgreSQL (label-only matching in V1.0, per DD-WORKFLOW-015)
- **NO pgvector**: V1.0 uses label-only architecture (semantic search deferred to V1.1+)
- **Collection Method**: Query DataStorage service REST API
- **Sample Data**: Collect up to 50 workflow records (name, description, labels, status)

**Audit Events** (via DataStorage REST API):
- **Endpoint**: `GET /api/v1/audit/events?since=24h&limit=1000`
- **Storage**: PostgreSQL audit_events table
- **Collection Method**: Query DataStorage service REST API with time filter
- **Sample Data**: Collect recent audit events (past 24h) for cross-service activity tracing

**Rationale**: Workflows are stored in DataStorage service (PostgreSQL), NOT ConfigMaps (per DD-WORKFLOW-009, DD-WORKFLOW-015). REST API access enables diagnostic analysis without direct database access.

---

#### BR-PLATFORM-001.6b: DataStorage Infrastructure Collection
**MUST** collect DataStorage backend infrastructure for database diagnostics:

**PostgreSQL Collection**:
- **Pod Logs**: `postgres-*` pods in DataStorage namespace (last 24h)
- **Configuration**: `postgresql.conf`, `pg_hba.conf` (via ConfigMap)
- **Database Version**: PostgreSQL version and installed extensions list
- **Active Connections**: Snapshot of `pg_stat_activity` (active queries)
- **Migration Status**: Database migration version from migrations table
- **Slow Query Log**: Last 24h of slow queries (if enabled)

**Redis Collection**:
- **Pod Logs**: `redis-*` pods (last 24h)
- **Configuration**: `redis.conf` (via ConfigMap)
- **Memory Usage**: Memory statistics and key space information
- **DLQ Status**: DLQ stream length and consumer group status
- **Slow Log**: Recent slow log entries

**Connection Health**:
- DataStorage → PostgreSQL connection status
- DataStorage → Redis connection status
- Connection pool metrics (active, idle, waiting connections)

**Rationale**: DataStorage issues are common in production (~70% of support tickets involve database). PostgreSQL/Redis logs contain critical error traces needed for diagnosing audit write failures, query performance issues, and DLQ problems.

---

#### BR-PLATFORM-001.6c: Prometheus Metrics Collection
**MUST** collect Prometheus metrics for performance diagnosis:

**Service Metrics** (from `/metrics` endpoints):
- HTTP request rates and latencies (per service, per endpoint)
- CRD reconciliation rates and durations
- Audit event write rates and failure counts
- AI API call rates and latencies
- Workflow execution success/failure rates
- Resource utilization (CPU, memory, goroutines, open file descriptors)

**Kubernetes Metrics** (from metrics-server):
- Node resource usage (CPU, memory, disk, network)
- Pod CPU/memory actual vs requests/limits
- Network I/O statistics per pod

**Collection Method**:
- Query Prometheus `/api/v1/query_range` for last 24h samples
- Export as OpenMetrics format or JSON time series
- Include metric labels for filtering and correlation

**Rationale**: Metrics are essential for performance diagnosis (~80% of performance issues). Request rates, error rates, latency percentiles, and resource utilization trends are critical for root cause analysis.

---

#### BR-PLATFORM-001.6d: Namespace Discovery Specification
**MUST** specify how to discover Kubernaut namespaces dynamically:

**Discovery Methods** (in priority order):
1. **Label Selector**: `app.kubernetes.io/part-of=kubernaut`
2. **Name Pattern**: `kubernaut-*` (e.g., `kubernaut-system`, `kubernaut-workflows`)
3. **Environment Label**: `kubernaut.ai/environment` (collect all matching)
4. **Fallback**: If no labeled namespaces found, use hardcoded list:
   - `kubernaut-system`
   - `kubernaut-workflows`
   - `kubernaut-monitoring` (if exists)

**Multi-Tenancy Support**:
- Support `--tenant-label <key>=<value>` flag for tenant-specific collection
- Collect only namespaces matching tenant label selector

**Rationale**: Implementation cannot proceed without clear namespace discovery specification. Risk of missing namespaces or collecting too much data in multi-tenancy scenarios.

---

#### BR-PLATFORM-001.6e: Helm Release Information Collection
**MUST** collect Helm release information for deployment diagnostics:

**Helm Releases**:
- **Release List**: All Helm releases in Kubernaut namespaces (`helm list -n <namespace>`)
- **Release Status**: Current status (deployed, pending, failed)
- **Release History**: Last 5 revisions per release (`helm history <release>`)
- **Release Values**: Deployed values for each release (`helm get values <release>`)
- **Release Manifests**: Rendered Kubernetes manifests (`helm get manifest <release>`)

**Helm Configuration**:
- Helm version (`helm version`)
- Helm repositories configured
- Chart versions deployed

**Rationale**: Kubernaut v1.0 is deployed via Helm (not OLM operator). Helm release information is critical for diagnosing deployment issues, configuration drift, failed upgrades, and rollback scenarios.

---

#### BR-PLATFORM-001.6f: Tekton Pipeline Resources Collection
**MUST** collect Tekton pipeline resources for workflow execution diagnostics:

**Tekton Workflow Executions** (v1.0 primary execution engine per ADR-035, ADR-044):
- **PipelineRuns**: All PipelineRuns in Kubernaut namespaces (last 24h)
- **TaskRuns**: All TaskRuns in Kubernaut namespaces (last 24h)
- **Pipeline Definitions**: Referenced by WorkflowExecution CRDs
- **Task Definitions**: `kubernaut-action` generic meta-task
- **PipelineRun Logs**: All step logs from PipelineRuns
- **TaskRun Logs**: Action container output from TaskRuns

**Tekton Infrastructure**:
- **Operator Pods**: Tekton operator pods in `tekton-pipelines` namespace
- **Operator Logs**: Tekton operator logs (last 24h)
- **Webhook Configuration**: Tekton webhook configurations
- **Tekton ConfigMaps**: Feature flags, defaults, config settings

**Tekton Status**:
- **PipelineRun Conditions**: Status conditions, timestamps, durations
- **TaskRun Conditions**: Status conditions, failure reasons
- **Execution Metadata**: Retry attempts, timeout settings

**Rationale**: Per ADR-035 and ADR-044, Tekton Pipelines is the **primary remediation execution engine** for Kubernaut v1.0 (highest priority, replacing KubernetesExecutor). Without Tekton diagnostics, workflow execution failures cannot be diagnosed. This is **CRITICAL P1** for v1.0 production support.

---

### 2.2 Execution & Packaging Requirements

#### BR-PLATFORM-001.7: Command-Line Interface
**MUST** support standard must-gather execution patterns:

**Execution Methods**:
```bash
# Method 1: OpenShift-style (oc adm must-gather)
oc adm must-gather --image=quay.io/kubernaut/must-gather:latest

# Method 2: Kubernetes-style (kubectl debug)
kubectl debug node/<node-name> \
  --image=quay.io/kubernaut/must-gather:latest \
  --image-pull-policy=Always -- /usr/bin/gather

# Method 3: Direct pod execution (fallback)
kubectl run kubernaut-must-gather \
  --image=quay.io/kubernaut/must-gather:latest \
  --rm --attach -- /usr/bin/gather
```

**Configuration Flags**:
- `--since <duration>`: Log collection timeframe (default: 24h)
- `--dest-dir <path>`: Output directory path (default: `/must-gather`)
- `--namespaces <list>`: Specific namespaces to collect (default: all Kubernaut namespaces)
- `--no-sanitize`: Disable automatic sanitization (for internal debugging only)

**Rationale**: Compatibility with existing Kubernetes administrator workflows and tooling.

---

#### BR-PLATFORM-001.8: Output Packaging & Organization
**MUST** produce organized, compressed archive output:

**Directory Structure**:
```
must-gather-<timestamp>/
├── cluster-scoped/
│   ├── nodes.yaml
│   ├── clusterroles.yaml
│   └── clusterrolebindings.yaml
├── namespaces/
│   ├── kubernaut-system/
│   │   ├── pods.yaml
│   │   ├── deployments.yaml
│   │   ├── configmaps.yaml
│   │   ├── secrets-metadata.yaml (sanitized)
│   │   └── logs/
│   │       ├── gateway-<pod-id>/
│   │       │   ├── current.log
│   │       │   └── previous.log (if exists)
│   │       └── ...
│   └── ...
├── crds/
│   ├── remediationrequests/
│   │   └── all-instances.yaml
│   ├── signalprocessings/
│   │   └── all-instances.yaml
│   └── ...
├── events/
│   └── all-events.json
├── version-info.txt
└── collection-metadata.json
```

**Archive Format**:
- **Compression**: gzip compressed tarball (`.tar.gz`)
- **Naming**: `kubernaut-must-gather-<cluster-name>-<timestamp>.tar.gz`
- **Max Size**: 500MB uncompressed (configurable with warnings)
- **Integrity**: Include SHA256 checksum file (`SHA256SUMS`)

**Size Limit Enforcement**:
- **Real-time Monitoring**: Track collection size during execution
- **Warning Threshold**: Warn at 400MB (80% of default limit)
- **Truncation Strategy**: Truncate logs at limit (oldest first, preserve metadata)
- **Truncation Reporting**: Document truncation in `collection-metadata.json`
- **Configurable Limit**: Support `--max-size <MB>` flag (default: 500MB)
- **Emergency Stop**: Hard stop at 2x limit (1000MB) to prevent disk exhaustion

**Rationale**: Standardized structure enables automated analysis tools and consistent support workflows. Size limits prevent disk exhaustion in constrained environments.

---

#### BR-PLATFORM-001.9: Privacy & Sanitization
**MUST** implement comprehensive data sanitization:

**Sensitive Data Removal**:
- **Secrets**: Remove all Secret values, preserve metadata only
- **Passwords**: Redact password fields in ConfigMaps and logs
- **API Keys**: Redact authentication tokens and API keys
- **Certificates**: Remove TLS certificates and private keys
- **PII**: Redact personally identifiable information (emails, names)

**Sanitization Patterns**:
```
password: ********
apiKey: [REDACTED]
token: [REDACTED-32-CHARS]
certificate: [CERTIFICATE-REDACTED]
privateKey: [PRIVATE-KEY-REDACTED]
email: user@[REDACTED]
```

**Opt-Out** (Internal Use Only):
- `--no-sanitize` flag for internal debugging
- **Warning**: Display clear warning about sensitive data exposure
- **Audit**: Log unsanitized collection attempts

**Rationale**: Privacy compliance (GDPR, CCPA) and security best practices for production data collection.

---

#### BR-PLATFORM-001.10: Collection Metadata & Versioning
**MUST** include metadata about the collection process:

**Metadata File** (`collection-metadata.json`):
```json
{
  "collection_time": "2025-12-17T15:30:00Z",
  "kubernaut_version": "v1.0.0",
  "must_gather_version": "v1.0.0",
  "kubernetes_version": "v1.28.3",
  "cluster_name": "prod-us-east-1",
  "collection_duration_seconds": 127,
  "namespaces_collected": ["kubernaut-system", "kubernaut-workflows"],
  "crds_collected": 6,
  "pods_collected": 23,
  "logs_collected": 45,
  "events_collected": 1247,
  "sanitization_enabled": true,
  "collection_flags": ["--since=24h"],
  "errors": []
}
```

**Version Information** (`version-info.txt`):
- Kubernaut component versions
- Kubernetes cluster version
- Must-gather tool version
- Collection timestamp

**Rationale**: Version context is critical for support teams to understand compatibility and reproduction of issues.

---

### 2.3 Non-Functional Requirements

#### BR-PLATFORM-001.11: Performance & Resource Limits
**MUST** execute efficiently with bounded resource usage:

**Resource Limits**:
- **Memory**: Max 512MB per collection (configurable)
- **CPU**: Max 1 CPU core (burstable to 2)
- **Execution Time**: Target 2-5 minutes for typical cluster
- **Network**: Minimize API server request volume via caching

**Optimization Requirements**:
- **Parallel Collection**: Collect logs and resources concurrently
- **API Pagination**: Use Kubernetes API pagination for large resource sets
- **Compression**: Stream compression during collection
- **Timeout Handling**: Graceful handling of slow API responses

**Rationale**: Must-gather should not impact cluster performance or stability during execution.

---

#### BR-PLATFORM-001.12: Error Handling & Resilience
**MUST** handle partial collection failures gracefully:

**Failure Scenarios**:
- **RBAC Denials**: Continue collection with warnings for inaccessible resources
- **API Timeouts**: Retry with exponential backoff, record failures
- **Missing Resources**: Log unavailable resources, continue collection
- **Disk Space**: Detect insufficient space, warn user

**Error Reporting**:
- **Summary**: Include error summary in `collection-metadata.json`
- **Partial Success**: Produce archive even with partial failures
- **Exit Codes**: Standard exit codes (0=success, 1=partial, 2=failure)

**Rationale**: Partial diagnostic data is better than no data. Must-gather should never fail completely due to transient issues.

---

## 3. Implementation Guidance

### 3.1 Technology Stack
- **Base Image**: `registry.access.redhat.com/ubi9/ubi-minimal:latest`
- **Runtime**: Bash scripts with `kubectl` binary
- **Utilities**: `jq` (JSON processing), `tar`, `gzip`, `sed`, `awk`, `sha256sum` (checksum generation)
- **Kubernetes Client**: kubectl 1.31+ (latest stable Kubernetes version)
  - **Version Detection**: Auto-detect cluster version and warn if kubectl version mismatch
  - **Compatibility**: Must-gather supports K8s 1.31+ (Kubernaut v1.0 requirement)
  - **Note**: Use `kubectl` for YAML operations (no `yq` dependency needed)

### 3.2 RBAC Requirements
Must-gather requires a ClusterRole with:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernaut-must-gather
rules:
- apiGroups: [""]
  resources: ["pods", "pods/log", "events", "configmaps", "secrets", "nodes", "namespaces"]
  verbs: ["get", "list"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
  verbs: ["get", "list"]
- apiGroups: ["kubernaut.ai", "kubernetesexecution.kubernaut.io"]
  resources: ["*"]
  verbs: ["get", "list"]
- apiGroups: ["apiextensions.k8s.io"]
  resources: ["customresourcedefinitions"]
  verbs: ["get", "list"]
```

### 3.3 Directory Structure
```
cmd/must-gather/
├── Dockerfile
├── gather.sh (main collection script)
├── collectors/
│   ├── crds.sh
│   ├── logs.sh
│   ├── events.sh
│   ├── config.sh
│   ├── cluster-state.sh
│   ├── datastorage.sh
│   ├── metrics.sh
│   ├── helm.sh
│   └── tekton.sh
├── sanitizers/
│   ├── secrets.sh
│   ├── logs.sh
│   └── patterns.txt
├── utils/
│   ├── checksum.sh (SHA256 checksum generation)
│   ├── size-monitor.sh (collection size monitoring)
│   └── namespace-discovery.sh (namespace discovery logic)
└── templates/
    ├── clusterrole.yaml
    └── README.md (usage instructions)
```

**Checksum Implementation**:
The `utils/checksum.sh` script generates SHA256 checksums for archive integrity verification:
```bash
# Generate SHA256SUMS file for all collected files
cd must-gather-<timestamp>/
find . -type f -exec sha256sum {} \; > SHA256SUMS
```

### 3.4 Testing Requirements
- **Unit Tests**: Test individual collector scripts
- **Integration Tests**: Test against real Kubernetes cluster
- **E2E Tests**: Full must-gather execution in test cluster
- **Sanitization Tests**: Verify sensitive data removal
- **Performance Tests**: Validate resource usage limits

---

## 4. Success Criteria

### 4.1 Functional Validation
- ✅ Must-gather image builds and runs on OpenShift and vanilla Kubernetes
- ✅ Collects all 8 Kubernaut CRD types successfully
- ✅ Captures logs from all Kubernaut service pods
- ✅ Produces valid, compressed tarball output
- ✅ Sanitizes all sensitive data patterns
- ✅ Executes within 5 minutes for typical cluster

### 4.2 Support Team Validation
- ✅ Support engineers can extract and analyze archive
- ✅ Archive structure is intuitive and well-documented
- ✅ Metadata file provides sufficient context
- ✅ Missing data is clearly documented in error summary

### 4.3 Production Readiness
- ✅ Documented in user-facing documentation
- ✅ Integrated into support escalation procedures
- ✅ Tested on all target Kubernetes distributions (OpenShift 4.12+, K8s 1.26+)
- ✅ Privacy review completed and approved

---

## 5. Compliance & Standards

### 5.1 Industry Standards
- **OpenShift Must-Gather**: Follows Red Hat's established pattern
- **Kubernetes Debug**: Compatible with `kubectl debug` workflows
- **CNCF Best Practices**: Aligns with cloud-native diagnostic standards

### 5.2 Privacy & Security
- **GDPR Compliance**: Data minimization and PII redaction
- **CCPA Compliance**: Transparent data collection and sanitization
- **SOC 2**: Audit trail of collection operations
- **RBAC Least Privilege**: Minimal permissions required for collection

### 5.3 Documentation Requirements
- **User Documentation**: `docs/user-guide/must-gather.md`
- **Support Documentation**: `docs/support/analyzing-must-gather.md`
- **Developer Documentation**: `docs/development/must-gather-development.md`
- **README**: Include in must-gather image at `/README.md`

---

## 6. Related Requirements

- **BR-MONITORING-001**: Integration with cluster monitoring and alerting
- **BR-AUDIT-001**: Audit trail integration for diagnostic collection events
- **BR-SECURITY-001**: RBAC and security policy compliance
- **BR-K8S-001**: Kubernetes client connectivity requirements

---

## 7. References

### External Standards
- [OpenShift Must-Gather Documentation](https://docs.openshift.com/container-platform/latest/support/gathering-cluster-data.html)
- [Kubernetes Debug Documentation](https://kubernetes.io/docs/tasks/debug/debug-application/debug-running-pod/)
- [CNCF Best Practices](https://www.cncf.io/blog/2021/03/01/kubernetes-debugging-best-practices/)

### Internal Documentation
- Architecture Decision Records: ADR-042 (Must-Gather Design - to be created)
- Design Decisions: DD-010 (Must-Gather Implementation - to be created)
- Testing Strategy: 03-testing-strategy.mdc

---

**Document Status**: ✅ **APPROVED - READY FOR IMPLEMENTATION**
**Priority**: **P1 - Required for Production Support**
**Target Version**: **Kubernaut v1.0**
**Estimated Effort**: **2-3 weeks** (container image + collection scripts + testing + documentation)

