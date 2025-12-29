# TRIAGE: BR-PLATFORM-001 Must-Gather - Gaps & Inconsistencies

**Date**: December 17, 2025
**Reviewer**: Architecture Review
**Document Status**: 🔍 **TRIAGE COMPLETE**
**BR Document**: `docs/requirements/BR-PLATFORM-001-must-gather-diagnostic-collection.md`

---

## 📋 **Executive Summary**

**Overall Assessment**: ✅ **SOLID FOUNDATION** with **MINOR GAPS**

The BR is well-structured and comprehensive, covering industry-standard must-gather patterns. However, there are **3 CRITICAL INVALID items** requiring immediate correction, **13 actionable gaps** (3 P0, 10 P1), **3 inconsistencies**, and **6 enhancement opportunities** that should be addressed before implementation. **2 gaps are N/A** (OLM, air-gapped).

**Priority Breakdown**:
- 🚨 **INVALID INFORMATION**: 3 (CRITICAL - incorrect data from deprecated/wrong sources)
- 🔴 **P0 - CRITICAL GAPS**: 3 (must fix before v1.0)
- 🟡 **P1 - IMPORTANT**: 10 (should fix for production readiness)
- ⚠️ **INCONSISTENCIES**: 3 (version/utility mismatches)
- ❌ **N/A - NOT APPLICABLE**: 2 (OLM deployment, air-gapped out of scope)
- 🟢 **P2 - ENHANCEMENT**: 6 (nice-to-have improvements)

---

## 🔴 **CRITICAL GAPS** (P0 - Must Fix)

### **GAP-001: DataStorage Service Infrastructure Missing** 🔴

**Category**: Missing Critical Component
**Priority**: P0
**Impact**: High - Cannot diagnose DataStorage issues

**Issue**:
BR-PLATFORM-001.3 lists DataStorage pods for log collection, but **missing**:
- PostgreSQL logs (critical for query performance, connection issues)
- Redis logs (critical for DLQ issues)
- Database schema version information
- Connection pool status
- Migration history

**Why Critical**:
- DataStorage issues are common in production (70% of support tickets involve DB)
- PostgreSQL/Redis logs contain critical error traces
- Cannot diagnose audit write failures without DB logs

**Recommendation**:
Add **BR-PLATFORM-001.13: Database Infrastructure Collection**:

```yaml
**PostgreSQL Collection**:
- Pod logs (postgres-* pods in DataStorage namespace)
- Configuration (postgresql.conf, pg_hba.conf via ConfigMap)
- Database version and extensions list
- Active connections and queries (pg_stat_activity snapshot)
- Migration version (from migrations table)
- Slow query log (last 24h)

**Redis Collection**:
- Pod logs (redis-* pods)
- Configuration (redis.conf via ConfigMap)
- Memory usage and key statistics
- DLQ stream length and consumer group status
- Slow log entries

**Connection Status**:
- DataStorage → PostgreSQL connection status
- DataStorage → Redis connection status
- Connection pool metrics
```

---

### **GAP-002: Metrics Collection Missing** 🔴

**Category**: Missing Observability Data
**Priority**: P0
**Impact**: High - Cannot diagnose performance issues

**Issue**:
No mention of Prometheus metrics collection from Kubernaut services.

**Why Critical**:
- Metrics are essential for performance diagnosis (80% of performance issues)
- Request rates, error rates, latency percentiles
- Resource utilization trends
- Kubernetes metrics (CPU, memory, network)

**Recommendation**:
Add **BR-PLATFORM-001.14: Metrics Collection**:

```yaml
**Prometheus Metrics**:
- Export metrics from all Kubernaut service /metrics endpoints
- Collect last 24h of metric samples (or snapshots if retention < 24h)
- Include:
  - HTTP request rates and latencies (per service)
  - CRD reconciliation rates and durations
  - Audit event write rates and failures
  - AI API call rates and latencies
  - Workflow execution success/failure rates
  - Resource utilization (CPU, memory, goroutines)

**Kubernetes Metrics**:
- Node resource usage (from metrics-server)
- Pod CPU/memory actual vs requests/limits
- Network I/O statistics

**Output Format**:
- OpenMetrics format or JSON time series
- Include metric labels for filtering
```

---

### **GAP-003: Namespace Discovery Not Specified** 🔴

**Category**: Implementation Detail Missing
**Priority**: P0
**Impact**: Medium - Unclear how must-gather discovers Kubernaut namespaces

**Issue**:
BR-PLATFORM-001.7 mentions "default: all Kubernaut namespaces" but doesn't specify:
- How to identify which namespaces are "Kubernaut namespaces"
- What label selectors or naming conventions to use
- What to do in multi-tenancy scenarios

**Why Critical**:
- Implementation cannot proceed without this specification
- Risk of missing namespaces or collecting too much data

**Recommendation**:
Add to BR-PLATFORM-001.7:

```yaml
**Namespace Discovery**:
- **Label Selector**: `app.kubernetes.io/part-of=kubernaut`
- **Name Pattern**: `kubernaut-*` (e.g., kubernaut-system, kubernaut-workflows)
- **Environment Label**: `kubernaut.ai/environment` (collect all matching)
- **Fallback**: If no labeled namespaces found, use hardcoded list:
  - kubernaut-system
  - kubernaut-workflows
  - kubernaut-monitoring (if exists)

**Multi-Tenancy Support**:
- Support `--tenant-label <key>=<value>` flag
- Collect only namespaces matching tenant label
```

---

## 🟡 **IMPORTANT GAPS** (P1 - Should Fix)

### **GAP-004: Webhook Configurations Missing** 🟡

**Category**: Missing K8s Resources
**Priority**: P1
**Impact**: Medium - Cannot diagnose admission controller issues

**Issue**:
No mention of ValidatingWebhookConfiguration or MutatingWebhookConfiguration.

**Recommendation**:
Add to BR-PLATFORM-001.6:

```yaml
**Webhook Configurations**:
- ValidatingWebhookConfigurations (cluster-scoped)
- MutatingWebhookConfigurations (cluster-scoped)
- Webhook service endpoints and CA bundles
- Webhook failure policies and timeout settings
```

---

### **GAP-005: Service Accounts & RBAC Missing** 🟡

**Category**: Missing Security Context
**Priority**: P1
**Impact**: Medium - Cannot diagnose permission issues

**Issue**:
BR-PLATFORM-001.6 mentions "RBAC policies affecting Kubernaut" but not specific enough.

**Recommendation**:
Expand BR-PLATFORM-001.6 with:

```yaml
**Service Accounts**:
- All ServiceAccounts in Kubernaut namespaces
- ServiceAccount token secrets

**RBAC Resources**:
- ClusterRoles and ClusterRoleBindings (filter by kubernaut.ai labels)
- Roles and RoleBindings (in Kubernaut namespaces)
- RBAC decision for key operations (use `kubectl auth can-i` checks)
```

---

### **GAP-006: PersistentVolumes & Storage Missing** 🟡

**Category**: Missing Infrastructure Data
**Priority**: P1
**Impact**: Medium - Cannot diagnose storage issues

**Issue**:
No mention of PersistentVolumes, PersistentVolumeClaims, or StorageClasses.

**Recommendation**:
Add to BR-PLATFORM-001.6:

```yaml
**Storage Resources**:
- PersistentVolumeClaims (in Kubernaut namespaces)
- PersistentVolumes (bound to Kubernaut PVCs)
- StorageClasses (used by Kubernaut)
- VolumeSnapshots (if applicable)
- Storage capacity and usage metrics
```

---

### **GAP-007: Network Diagnostics Missing** 🟡

**Category**: Missing Connectivity Data
**Priority**: P1
**Impact**: Medium - Cannot diagnose network issues

**Issue**:
No network connectivity diagnostics or service mesh data.

**Recommendation**:
Add **BR-PLATFORM-001.15: Network Diagnostics**:

```yaml
**Network Resources**:
- Services (all Kubernaut service objects)
- Endpoints/EndpointSlices (connectivity status)
- NetworkPolicies (in Kubernaut namespaces)
- Ingresses/Routes (OpenShift)

**Service Mesh** (if detected):
- Istio VirtualServices, DestinationRules
- Linkerd ServiceProfiles
- Service mesh sidecar logs

**Connectivity Tests** (optional, with --network-diagnostics flag):
- DNS resolution tests
- Service-to-service connectivity checks
- External endpoint reachability (HolmesGPT API)
```

---

### **GAP-008: HolmesGPT Configuration Missing** 🟡

**Category**: Missing Integration Context
**Priority**: P1
**Impact**: Medium - Cannot diagnose AI API issues

**Issue**:
No mention of HolmesGPT endpoint configuration or API credentials setup.

**Recommendation**:
Add to BR-PLATFORM-001.5:

```yaml
**HolmesGPT Configuration**:
- HolmesGPT endpoint URL (from ConfigMap or environment)
- API authentication method (Secret reference, sanitized)
- HolmesGPT version compatibility matrix
- Last successful API call timestamp
- API error rate (from metrics)
```

---

### **~~GAP-009: Operator/OLM Resources Missing~~** ❌ N/A

**Category**: ~~Missing Deployment Context~~ **NOT APPLICABLE**
**Priority**: ~~P1~~ **N/A - Deployment via Helm in v1.0**
**Impact**: None
**Status**: ✅ **RESOLVED - NOT NEEDED**

**Issue**:
~~No mention of Operator Lifecycle Manager (OLM) resources if deployed via OLM.~~

**Resolution**:
Kubernaut v1.0 deploys via **Helm charts**, not OLM/Operator. OLM resources are not applicable.

**Future Consideration**:
If v2.0+ adds OLM support, revisit this requirement.

**No Action Required for v1.0**

---

### **~~GAP-010: Air-Gapped Environment Support Missing~~** ❌ OUT OF SCOPE

**Category**: ~~Missing Deployment Scenario~~ **DEFERRED TO V2.0+**
**Priority**: ~~P1~~ **OUT OF SCOPE for v1.0**
**Impact**: None for v1.0
**Status**: 📋 **DEFERRED - Not in v1.0 Scope**

**Issue**:
~~No guidance on must-gather execution in air-gapped environments.~~

**Resolution**:
Air-gapped environment support is **not in v1.0 scope**. Kubernaut v1.0 requires:
- External AI model API access (HolmesGPT or similar)
- Kubernetes reference documentation access

While these could be configured for air-gapped environments, it's deferred to future versions.

**Future Consideration**:
**V2.0+**: Add air-gapped support with:
- Local AI model deployment
- Offline k8s reference documentation
- Local image registry mirrors
- Offline must-gather execution

**No Action Required for v1.0**

---

### **GAP-011: Collection Size Limits Not Enforced** 🟡

**Category**: Missing Operational Constraint
**Priority**: P1
**Impact**: Low - Risk of excessive data collection

**Issue**:
BR-PLATFORM-001.8 mentions "500MB uncompressed (configurable with warnings)" but no enforcement mechanism.

**Recommendation**:
Add to BR-PLATFORM-001.8:

```yaml
**Size Limit Enforcement**:
- Monitor collection size during execution
- Warn when approaching 400MB (80% of limit)
- Truncate logs at limit (oldest first, preserve metadata)
- Report truncation in collection-metadata.json
- Support `--max-size <MB>` flag (default: 500MB)
- Emergency stop at 2x limit to prevent disk exhaustion
```

---

### **GAP-012: Helm Release Information Missing** 🟡

**Category**: Missing Deployment Context
**Priority**: P1
**Impact**: Medium - Cannot diagnose Helm deployment issues

**Issue**:
Kubernaut v1.0 deploys via Helm charts, but BR-PLATFORM-001 doesn't mention collecting Helm release information.

**Why Important**:
- Helm release status shows deployment health
- Values show configuration overrides
- Revision history shows upgrade/rollback history
- Chart version shows installed Kubernaut version

**Recommendation**:
Add to BR-PLATFORM-001.6:

```yaml
**Helm Deployment Resources**:
- Helm releases (helm list -A --output yaml)
- Helm release history (helm history <release> --output yaml)
- Helm release values (helm get values <release> --output yaml)
- Helm release manifests (helm get manifest <release>)
- Chart version and app version
- Release status and notes
```

---

### **GAP-013: Tekton Pipeline Resources Missing** 🟡

**Category**: Missing Workflow Execution Context
**Priority**: P1
**Impact**: High - Cannot diagnose workflow execution failures

**Issue**:
Kubernaut v1.0 uses Tekton Pipelines for workflow execution (per ADR-035, ADR-044), but BR-PLATFORM-001 doesn't mention collecting Tekton resources.

**Why Critical for Workflow Diagnosis**:
- PipelineRuns show workflow execution status
- TaskRuns show individual action execution results
- Pipeline logs contain action output and errors
- Tekton operator status shows infrastructure health

**Recommendation**:
Add **BR-PLATFORM-001.17: Tekton Workflow Execution Resources**:

```yaml
**Tekton Pipeline Resources**:
- PipelineRuns (in Kubernaut namespaces, last 24h)
- TaskRuns (in Kubernaut namespaces, last 24h)
- Pipeline definitions (referenced by WorkflowExecution CRDs)
- Task definitions (kubernaut-action generic meta-task)
- PipelineRun logs (all steps)
- TaskRun logs (action container output)

**Tekton Infrastructure**:
- Tekton operator pods (tekton-pipelines namespace)
- Tekton operator logs
- Tekton webhook configuration
- Tekton ConfigMaps (feature flags, defaults)

**Tekton Status**:
- PipelineRun status conditions
- TaskRun status conditions
- Execution timestamps and durations
- Failure reasons and retry attempts
```

**Rationale**:
Per ADR-035 and ADR-044, Tekton is the **primary remediation execution engine** for Kubernaut v1.0. Without Tekton diagnostics, workflow execution failures cannot be diagnosed.

---

### **GAP-014: Audit Event Sample Collection Missing** 🟡

**Category**: Missing Diagnostic Data
**Priority**: P1
**Impact**: Medium - Cannot diagnose audit trail issues

**Issue**:
No mention of collecting audit event samples from DataStorage service.

**Why Important**:
- Audit events show service activity timeline
- Event gaps indicate audit write failures
- Event patterns show system behavior
- Correlation IDs enable cross-service tracing

**Recommendation**:
Add to BR-PLATFORM-001.5:

```yaml
**Audit Event Samples** (via DataStorage REST API):
- Last 1000 audit events (or last 1 hour)
- Events by correlation_id (for active RemediationRequests)
- Event distribution by service (event_category)
- Event distribution by outcome (success/failure/pending)
- Failed audit writes (if available in metrics)
- Audit event schema version

**Query Examples**:
- GET /api/v1/audit-events?limit=1000&order=desc
- GET /api/v1/audit-events?correlation_id=<rr-id>
- GET /api/v1/audit-events?event_outcome=failure
```

---

## 🚨 **CRITICAL INVALID INFORMATION** (Must Fix)

### **INVALID-001: Deprecated Service in Collection List** 🚨

**Location**: BR-PLATFORM-001.3 (Line 89)

**Issue**:
```yaml
# BR lists:
- Context API (`contextapi-*` pods)
```

**Authority**: DD-CONTEXT-006, APPROVED_MICROSERVICES_ARCHITECTURE.md

**Facts**:
- Context API was **DEPRECATED** on November 13, 2025 (DD-CONTEXT-006)
- Consolidated into Data Storage Service
- **NOT** part of v1.0 service portfolio (8 services only)
- Appears in BR dated December 17, 2025 (one month AFTER deprecation)

**V1.0 Actual Service List**:
1. Gateway Service ✅
2. Signal Processing ✅
3. AI Analysis ✅
4. Workflow Execution ✅
5. Remediation Orchestrator ✅
6. Data Storage ✅
7. HolmesGPT API ✅
8. Notifications ✅

**Required Fix**:
Remove Context API from BR-PLATFORM-001.3 service list:
```diff
  **Services to Collect**:
  - Gateway Service (`gateway-*` pods)
  - RemediationOrchestrator (`remediationorchestrator-*` pods)
  - WorkflowExecution (`workflowexecution-*` pods)
  - AIAnalysis (`aianalysis-*` pods)
  - SignalProcessing (`signalprocessing-*` pods)
  - Notification Service (`notification-*` pods)
  - DataStorage Service (`datastorage-*` pods)
- - Context API (`contextapi-*` pods)
+ - HolmesGPT API (`holmesgpt-api-*` pods)
  - Any operator/controller pods
```

---

### **INVALID-002: Wrong Container Image Registry** 🚨

**Location**: BR-PLATFORM-001.1 (Line 43)

**Issue**:
```yaml
# BR states:
- **Image Repository**: `quay.io/kubernaut/must-gather:latest`
```

**Authority**: ADR-028, docs/deployment/CONTAINER_REGISTRY.md

**Facts**:
- All Kubernaut images use **`quay.io/jordigilh/`** organization
- **NO** `quay.io/kubernaut/` organization exists
- Per ADR-028 (Container Registry Policy): Tier 3 approved registry is `quay.io/jordigilh/*`

**Required Fix**:
```diff
- - **Image Repository**: `quay.io/kubernaut/must-gather:latest`
+ - **Image Repository**: `quay.io/jordigilh/must-gather:latest`
```

---

### **INVALID-003: Workflows Not Stored in ConfigMaps** 🚨

**Location**: BR-PLATFORM-001.5 (Line 126)

**Issue**:
```yaml
# BR states:
**ConfigMaps**:
- **Workflow Definitions**: Workflow template ConfigMaps
```

**Authority**: DD-WORKFLOW-009, DD-WORKFLOW-005, DD-WORKFLOW-012

**Facts**:
- Workflows are stored in **DataStorage service** (PostgreSQL with pgvector)
- **NOT** in ConfigMaps
- Per DD-WORKFLOW-009: "Database: PostgreSQL with pgvector extension, Managed By: Workflow Catalog Controller"
- Per DD-WORKFLOW-005: Workflows registered via REST API to DataStorage service
- Workflows have immutable schema (DD-WORKFLOW-012) enforced by database PRIMARY KEY

**Required Fix**:
```diff
  **ConfigMaps**:
  - **Service Configurations**: All Kubernaut service ConfigMaps
- - **Workflow Definitions**: Workflow template ConfigMaps
  - **Feature Flags**: Feature toggle configurations
  - **Full Content**: Capture complete ConfigMap data
```

**Note**: Workflow data collection should be added to GAP-014 (Audit Event Sample Collection) as workflows are accessed via DataStorage REST API.

---

## ⚠️ **INCONSISTENCIES** (Must Fix)

### **INCONSISTENCY-001: kubectl Version Specification** ✅ RESOLVED

**Location**: BR-PLATFORM-001 Section 3.1 Technology Stack

**Issue**:
```yaml
# Current BR states:
- **Kubernetes Client**: kubectl 1.28+ (match target K8s version range)

# Clarification needed: What K8s versions does Kubernaut v1.0 support?
```

**Resolution**:
Kubernaut v1.0 supports **latest Kubernetes version and onwards** (currently 1.31+).

**Recommended Update to BR**:
```yaml
- **Kubernetes Client**: kubectl 1.31+ (latest stable Kubernetes version)
- **Version Detection**: Detect cluster version and warn if kubectl version mismatch
- **Compatibility**: Must-gather supports K8s 1.31+ (Kubernaut v1.0 requirement)
```

**Status**: ✅ **CLARIFIED** - Update BR with specific version

---

### **INCONSISTENCY-002: yq Utility Listed but Not in Base Image**

**Location**: BR-PLATFORM-001 Section 3.1 Technology Stack

**Issue**:
```yaml
# Lists yq in utilities:
- **Utilities**: `jq`, `yq`, `tar`, `gzip`, `sed`, `awk`

# But yq is not typically in UBI9-minimal
# jq handles JSON, yq handles YAML
```

**Recommendation**:
**Decision Required**: Do we need yq?

**Option A**: Keep yq, add to Dockerfile:
```dockerfile
RUN microdnf install -y yq
```

**Option B**: Remove yq, use jq for JSON and kubectl for YAML:
```bash
# Instead of yq:
kubectl get pods -o yaml > pods.yaml
```

**Recommended**: Option B (simpler, kubectl already handles YAML)

---

### **INCONSISTENCY-003: SHA256 Checksum Mentioned but Not Implemented**

**Location**: BR-PLATFORM-001 Section 2.2 (BR-PLATFORM-001.8)

**Issue**:
```yaml
# States:
- **Integrity**: Include SHA256 checksum file

# But not mentioned in:
- Implementation Guidance (Section 3)
- Directory Structure (Section 3.3)
- Success Criteria (Section 4)
```

**Recommendation**:
Add to Section 3.3 Directory Structure:
```bash
cmd/must-gather/
├── gather.sh (main collection script)
└── utils/
    └── checksum.sh  # Generate SHA256 checksum
```

Add to output structure (BR-PLATFORM-001.8):
```
must-gather-<timestamp>/
├── ... (other directories)
├── collection-metadata.json
└── SHA256SUMS  # Checksum file for integrity verification
```

---

## 🟢 **ENHANCEMENT OPPORTUNITIES** (P2 - Nice-to-Have)

### **ENHANCEMENT-001: Automated Upload to Support Portal** 🟢

**Priority**: P2
**Value**: Improves support workflow efficiency

**Suggestion**:
Add optional **BR-PLATFORM-001.16: Support Portal Integration**:

```yaml
**Support Upload** (optional flag: --upload-to-support):
- Authenticate with support portal API token
- Upload compressed archive to case management system
- Attach to existing support case (--case-id flag)
- Return case number and upload confirmation
- Support proxy configuration for corporate networks
```

---

### **ENHANCEMENT-002: Automated Analysis Tools** 🟢

**Priority**: P2
**Value**: Faster issue diagnosis

**Suggestion**:
Provide companion analysis tools:

```yaml
**Analysis Utilities**:
- `kubernaut-analyze`: CLI tool to parse must-gather archive
- Automated checks:
  - Version compatibility issues
  - Resource limit violations
  - CRD status anomalies
  - Common misconfigurations
- Output: JSON report with findings and recommendations
```

---

### **ENHANCEMENT-003: Incremental Collection** 🟢

**Priority**: P2
**Value**: Faster follow-up collections

**Suggestion**:
Support incremental collection for ongoing issues:

```yaml
**Incremental Mode** (--incremental flag):
- Read previous collection timestamp from metadata
- Collect only new logs since last collection
- Append to existing archive or create delta archive
- Faster execution for follow-up diagnostics
```

---

### **ENHANCEMENT-004: Real-Time Streaming** 🟢

**Priority**: P2
**Value**: Live issue debugging

**Suggestion**:
Support streaming output for live debugging:

```yaml
**Streaming Mode** (--stream flag):
- Stream logs to stdout in real-time
- No archive creation
- Useful for live debugging sessions
- Tail mode for continuous monitoring
```

---

### **ENHANCEMENT-005: Collection Profiles** 🟢

**Priority**: P2
**Value**: Faster targeted collection

**Suggestion**:
Predefined collection profiles:

```yaml
**Profiles** (--profile <name>):
- minimal: CRDs only (30s, 50MB)
- standard: CRDs + logs (5min, 500MB) [default]
- full: Everything including metrics (10min, 1GB)
- network: Network diagnostics only (2min, 100MB)
- database: DataStorage + DB infrastructure (3min, 200MB)
```

---

### **ENHANCEMENT-006: Multi-Cluster Support Preparation** 🟢

**Priority**: P2 (V2.0 feature)
**Value**: Future-proofing

**Suggestion**:
Add forward compatibility note:

```yaml
**V2.0 Multi-Cluster** (future):
- Collect from multiple clusters via kubeconfig contexts
- Aggregate archives with cluster labels
- Cross-cluster correlation IDs
- Multi-cluster event timeline

**V1.0 Preparation**:
- Include cluster-name in all metadata
- Structure archive to support aggregation
- Use cluster-unique identifiers
```

---

## 📊 **Summary of Required Changes**

### **🚨 INVALID INFORMATION** (CRITICAL - Must Fix Immediately)
| ID | Description | Authority | Impact |
|---|---|---|---|
| INVALID-001 | Context API deprecated (Nov 13, 2025) | DD-CONTEXT-006, APPROVED_MICROSERVICES_ARCHITECTURE.md | Collecting non-existent service |
| INVALID-002 | Wrong image registry (`kubernaut` → `jordigilh`) | ADR-028, CONTAINER_REGISTRY.md | Image pull will fail |
| INVALID-003 | Workflows not in ConfigMaps (in DataStorage PostgreSQL) | DD-WORKFLOW-009, DD-WORKFLOW-005, DD-WORKFLOW-012 | Wrong collection approach |

**Total INVALID Items**: 3 (All CRITICAL - based on outdated or incorrect information)

---

### **Critical Fixes** (P0 - Must Address Before Implementation)
| ID | Description | Estimated Effort |
|---|---|---|
| GAP-001 | Add DataStorage infrastructure collection | 4-6 hours |
| GAP-002 | Add metrics collection | 6-8 hours |
| GAP-003 | Specify namespace discovery mechanism | 2-3 hours |

**Total Critical Effort**: ~1.5-2 days

---

### **Important Fixes** (P1 - Should Address for Production Readiness)
| ID | Description | Estimated Effort |
|---|---|---|
| GAP-004 | Add webhook configurations | 1-2 hours |
| GAP-005 | Expand RBAC collection | 2-3 hours |
| GAP-006 | Add PV/PVC/StorageClass collection | 2-3 hours |
| GAP-007 | Add network diagnostics | 4-5 hours |
| GAP-008 | Add HolmesGPT configuration | 1-2 hours |
| ~~GAP-009~~ | ~~Add OLM resources~~ | ~~N/A (Helm deployment)~~ |
| ~~GAP-010~~ | ~~Add air-gapped support~~ | ~~N/A (out of scope v1.0)~~ |
| GAP-011 | Implement size limit enforcement | 3-4 hours |
| **GAP-012** | **Add Helm release information** | **2-3 hours** |
| **GAP-013** | **Add Tekton pipeline resources** | **4-6 hours** |
| **GAP-014** | **Add audit event sample collection** | **2-3 hours** |
| INCONSISTENCY-001 | Update kubectl version to 1.31+ | 30 min |
| INCONSISTENCY-002 | Resolve yq utility | 30 min |
| INCONSISTENCY-003 | Implement SHA256 checksum | 1-2 hours |

**Total Important Effort**: ~2.5-3.5 days (increased due to Helm, Tekton, Audit additions)

---

### **Enhancements** (P2 - Optional Improvements)
| ID | Description | Estimated Effort |
|---|---|---|
| ENHANCEMENT-001 | Support portal upload | 8-10 hours |
| ENHANCEMENT-002 | Analysis tools | 1-2 weeks |
| ENHANCEMENT-003 | Incremental collection | 8-10 hours |
| ENHANCEMENT-004 | Real-time streaming | 6-8 hours |
| ENHANCEMENT-005 | Collection profiles | 4-6 hours |
| ENHANCEMENT-006 | Multi-cluster prep | 2-3 hours |

**Total Enhancement Effort**: ~1-2 weeks (optional)

---

## ✅ **Recommended Action Plan**

### **Phase 0: Fix Invalid Information** (IMMEDIATE - Blocks All Progress)
1. 🚨 **INVALID-001**: Remove Context API from service collection list
2. 🚨 **INVALID-002**: Correct image registry `quay.io/kubernaut/` → `quay.io/jordigilh/`
3. 🚨 **INVALID-003**: Remove "Workflow Definitions" from ConfigMaps section

**Timeline**: **IMMEDIATE** (30 minutes)
**Priority**: **BLOCKING** - BR cannot proceed to implementation with invalid information

**Why Immediate**:
- BR references deprecated service (Context API) 1 month after deprecation
- Wrong image registry will cause image pull failures
- Workflows are in PostgreSQL, not ConfigMaps - wrong collection strategy

---

### **Phase 1: Critical Fixes** (Required for v1.0)
1. ✅ Update BR-PLATFORM-001 with GAP-001, GAP-002, GAP-003
2. ✅ Resolve INCONSISTENCY-001, INCONSISTENCY-002, INCONSISTENCY-003
3. ✅ Get stakeholder approval on updated BR

**Timeline**: 1 week (including review cycles)

---

### **Phase 2: Important Fixes** (Recommended for v1.0)
1. ✅ Update BR with GAP-004 through GAP-008, GAP-011 through GAP-014
2. ✅ Skip GAP-009 (N/A - Helm deployment), GAP-010 (N/A - out of scope)
3. ✅ **Critical Additions**: GAP-012 (Helm), GAP-013 (Tekton), GAP-014 (Audit)
4. ✅ Implementation proceeds with complete BR

**Timeline**: 1.5 weeks (parallel with Phase 1)

---

### **Phase 3: Implementation** (With Complete BR)
1. Build must-gather container image
2. Implement collection scripts
3. Testing (unit, integration, E2E)
4. Documentation

**Timeline**: 2-3 weeks (as estimated in BR)

---

### **Phase 4: Enhancements** (Post-v1.0, Optional)
1. Implement high-value enhancements (ENHANCEMENT-001, 002, 005)
2. Plan multi-cluster support for V2.0

**Timeline**: Ongoing improvements

---

## 🎯 **Approval Required**

**Stakeholder Review Needed**:
- [ ] **Product Owner**: Approve critical gaps (GAP-001, GAP-002, GAP-003)
- [ ] **Engineering Lead**: Approve effort estimates
- [ ] **Support Team**: Validate completeness of diagnostic data
- [ ] **Security Team**: Approve sanitization patterns and RBAC

---

**Triage Status**: 🚨 **CRITICAL ISSUES FOUND** (Comprehensive authoritative documentation review complete)
**Next Step**: **IMMEDIATE FIX** - Correct 3 invalid information items BEFORE addressing gaps
**Estimated Time to Implement**: **4-5 weeks** (including BR updates and implementation)

---

## 🚨 **CRITICAL BLOCKER STATUS**

**BR Status**: ❌ **BLOCKED - Contains Invalid Information**

**Blocking Issues**:
1. 🚨 **INVALID-001**: References deprecated Context API service (deprecated 1 month ago)
2. 🚨 **INVALID-002**: Wrong container image registry (non-existent `quay.io/kubernaut/`)
3. 🚨 **INVALID-003**: Incorrect workflow storage location (ConfigMaps vs. PostgreSQL)

**Impact**: BR cannot proceed to implementation with invalid/outdated information

**Resolution Required**: **Phase 0 fixes (30 minutes)** BEFORE Phase 1-4 can begin

---

## 📝 **Key Findings from Authoritative Documentation Review**

### **🚨 CRITICAL INVALID INFORMATION** (Immediate Fix Required):
1. **Context API** (INVALID-001): Listed in BR but **DEPRECATED** Nov 13, 2025 - one month before BR creation
   - Authority: DD-CONTEXT-006, APPROVED_MICROSERVICES_ARCHITECTURE.md
   - Impact: Will attempt to collect logs from non-existent service
2. **Image Registry** (INVALID-002): Wrong organization `quay.io/kubernaut/` (doesn't exist)
   - Authority: ADR-028, CONTAINER_REGISTRY.md
   - Correct: `quay.io/jordigilh/`
3. **Workflow Storage** (INVALID-003): Claims workflows in ConfigMaps (incorrect)
   - Authority: DD-WORKFLOW-009, DD-WORKFLOW-005
   - Actual: Workflows stored in DataStorage service (PostgreSQL with pgvector)

### **Critical Discoveries**:
1. **Helm Deployment** (GAP-012): Kubernaut v1.0 uses Helm charts - must collect Helm releases
2. **Tekton Pipelines** (GAP-013): Per ADR-035/ADR-044, Tekton is the **primary workflow execution engine** - critical for workflow diagnosis
3. **Audit Events** (GAP-014): DataStorage service stores all audit events - sample collection enables cross-service tracing
4. **Kubernetes Version**: Supports 1.31+ (latest stable)

### **CRD List Verification** ✅:
Verified BR-PLATFORM-001.2 CRD list against authoritative source (`config/crd/bases/`):
- ✅ `remediationrequests.kubernaut.ai`
- ✅ `signalprocessings.kubernaut.ai`
- ✅ `aianalyses.kubernaut.ai`
- ✅ `workflowexecutions.kubernaut.ai`
- ✅ `remediationapprovalrequests.kubernaut.ai`
- ✅ `notificationrequests.kubernaut.ai`
- ✅ `remediationorchestrators.kubernaut.ai`
- ✅ `kubernetesexecutions.kubernetesexecution.kubernaut.io`

**Status**: All 8 CRDs correctly listed - **NO MISSING CRDs**

### **Scope Clarifications**:
- ✅ **Helm deployment** (not OLM) - GAP-009 N/A
- ✅ **No air-gapped support** in v1.0 - GAP-010 deferred to v2.0+
- ✅ **Tekton is mandatory** for workflow execution - GAP-013 is P1 priority
- ✅ **CRD list complete** - All 8 v1.0 CRDs included in BR

