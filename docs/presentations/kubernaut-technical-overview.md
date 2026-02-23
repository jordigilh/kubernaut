# Kubernaut Technical Overview

**Audience**: Technical  
**Date**: February 2026  
**Format**: One slide per service, pipeline order

---

## Slide 1: Pipeline Overview

**Kubernaut** is a Kubernetes-native autonomous remediation platform. When a monitoring alert fires, Kubernaut ingests it, enriches it with cluster context, asks an LLM to diagnose the root cause and select a remediation workflow, executes that workflow safely, and then measures whether the fix actually worked.

```
Alert (Prometheus / K8s Event)
  |
  v
Gateway ──> RemediationRequest CRD
  |
  v
Signal Processing ──> SignalProcessing CRD
  |                    (enrichment + Rego classification)
  v
AI Analysis ──> AIAnalysis CRD
  |              (HolmesGPT RCA + workflow selection + Rego approval)
  v
Remediation Orchestrator ──> coordinates lifecycle
  |
  v
Workflow Execution ──> WorkflowExecution CRD
  |                     (Tekton PipelineRun / K8s Job)
  v
Effectiveness Monitor ──> EffectivenessAssessment CRD
  |                        (health + alerts + metrics + spec hash)
  v
Notification ──> NotificationRequest CRD
                  (Slack, Email, Console)
```

**Supporting services**: DataStorage (audit trail + workflow catalog), HolmesGPT-API (LLM orchestration), AuthWebhook (SOC2 identity injection).

---

## Slide 2: Gateway

**Purpose**: Webhook receiver that ingests alerts from monitoring systems, normalizes them, deduplicates, and creates RemediationRequest CRDs in Kubernetes. The single entry point for all signals.

**Architecture**: HTTP server (not a controller). Stateless, K8s-native.

**CRD created**: `RemediationRequest`

**Key Features**:
- Pluggable adapters: Prometheus AlertManager (`/api/v1/signals/prometheus`), Kubernetes Events (`/api/v1/signals/kubernetes-event`); RR.Spec.SignalType="alert" (Issue #166)
- Status-based deduplication using RR fingerprint lookups (no Redis)
- Replay prevention: header-first with body-fallback freshness validation per adapter
- Scope filtering: label-based opt-in (`kubernaut.ai/managed=true`) so only managed resources are processed
- Distributed locking: K8s Lease-based locking for multi-replica safety
- Circuit breaker: protects against K8s API cascading failures
- Buffered audit events to DataStorage (signal received, deduplicated, CRD created)
- Graceful shutdown with readiness probe returning 503 during drain

**Signal Flow**:
1. HTTP POST arrives at adapter route
2. Middleware: concurrency throttle, replay validation, Content-Type check
3. Adapter parses payload into `NormalizedSignal`
4. Scope check, distributed lock, deduplication check
5. New signal: create `RemediationRequest` CRD (201). Duplicate: update status (202).

---

## Slide 3: Signal Processing

**Purpose**: Enriches raw signals with Kubernetes context (namespace labels, pod status, owner chain, HPA, PDB) and classifies them using operator-defined Rego policies. Transforms a raw alert into a fully contextualized, prioritized signal.

**Architecture**: Kubernetes controller (watches `SignalProcessing` CRDs).

**CRD**: `SignalProcessing`

**Key Features**:
- K8s enrichment: namespace, pod, deployment, owner chain, node, PDB, HPA
- 5 Rego classifiers: Environment, Priority, Severity, Business, CustomLabels
- Detected labels: GitOps, Helm, PDB-protected, HPA-managed, service mesh, network isolation
- Signal mode: predictive vs reactive

**Rego Policy -- Priority Classification** (`deploy/signalprocessing/policies/priority.rego`):

```rego
package signalprocessing.priority

import rego.v1

# Severity rank: higher = more urgent
severity_rank := 3 if { lower(input.signal.severity) == "critical" }
severity_rank := 2 if { lower(input.signal.severity) == "warning" }
severity_rank := 1 if { lower(input.signal.severity) == "info" }
default severity_rank := 0

# Environment rank: higher = more sensitive
env_rank := 3 if { lower(input.environment) == "production" }
env_rank := 2 if { lower(input.environment) == "staging" }
env_rank := 1 if { lower(input.environment) == "development" }
default env_rank := 0

# Combined score -> priority (N + M rules instead of N * M)
score := severity_rank + env_rank

result := {"priority": "P0", "policy_name": "score-based"} if { score >= 6 }
result := {"priority": "P1", "policy_name": "score-based"} if { score >= 4; score < 6 }
result := {"priority": "P2", "policy_name": "score-based"} if { score >= 2; score < 4 }
default result := {"priority": "P3", "policy_name": "default-catch-all"}
```

---

## Slide 4: AI Analysis

**Purpose**: Runs AI-powered root cause analysis via HolmesGPT-API, selects a remediation workflow from the catalog, and evaluates whether human approval is required using a Rego policy. The "brain" of the remediation pipeline.

**Architecture**: Kubernetes controller (watches `AIAnalysis` CRDs).

**CRD**: `AIAnalysis`

**Key Features**:
- Workflow selection: HAPI returns selected workflow (ID, image, parameters, rationale)
- Rego approval policy: approved / manual_review_required / denied
- Affected resource: LLM can identify a different target than the signal source (e.g., Deployment instead of Pod)

**Rego Policy -- Approval** (`config/rego/aianalysis/approval.rego`):

```rego
package aianalysis.approval

import rego.v1

default require_approval := false

# Production environment requires manual approval
require_approval if {
    is_production
}

# Multiple recovery attempts (3+) require approval in ANY environment
require_approval if {
    is_multiple_recovery
}

# Missing affected resource: default-deny (ADR-055)
require_approval if {
    not has_affected_resource
}

# Production + failed detections (GitOps, PDB) requires approval
require_approval if {
    is_production
    has_failed_detections
}

# Production + stateful workload requires approval
require_approval if {
    is_production
    is_stateful
}
```

---

## Slide 5: HolmesGPT-API (HAPI)

**Purpose**: Internal Python service that wraps the HolmesGPT SDK and orchestrates LLM-driven incident analysis, recovery proposals, and workflow discovery. The interface between the Kubernetes controllers and the LLM.

**Architecture**: HTTP service (FastAPI). Not a controller.

**Key Features**:
- 3-step workflow discovery protocol against DataStorage (see Slide 6 for taxonomy and concrete example):
  1. `list_available_actions` (what can I do?)
  2. `list_workflows` (which workflows implement this action?)
  3. `get_workflow` (fetch the selected workflow's schema and parameters)
- Remediation history enrichment: queries DataStorage for past remediations on the same target, injects into prompt so the LLM avoids repeating failed approaches
- Three-way hash comparison: detects if the target spec drifted since last remediation

**Pipeline Integration**:
- AIAnalysis controller calls HAPI for incident/recovery analysis
- HAPI calls DataStorage for workflow catalog and remediation history
- HAPI returns: root cause, affected resource, selected workflow, approval context

---

## Slide 6: Action Type Taxonomy and Workflow Discovery

**Purpose**: The action type taxonomy is a curated set of remediation actions that Kubernaut knows how to perform. It powers the three-step discovery protocol that the LLM uses to find and select the right workflow for a given incident. Operators extend it by registering new workflows under existing or new action types.

**V1.0 Action Type Taxonomy** (DD-WORKFLOW-016, 10 types):

| Action Type | What It Does |
|---|---|
| **ScaleReplicas** | Horizontally scale a workload by adjusting the replica count |
| **RestartPod** | Kill and recreate one or more pods |
| **IncreaseCPULimits** | Increase CPU resource limits on containers |
| **IncreaseMemoryLimits** | Increase memory resource limits on containers |
| **RollbackDeployment** | Revert a deployment to its previous stable revision |
| **DrainNode** | Drain and cordon a node, evicting all pods |
| **CordonNode** | Cordon a node to prevent new scheduling without eviction |
| **RestartDeployment** | Rolling restart of all pods in a workload |
| **CleanupNode** | Reclaim disk space by purging temp files and unused images |
| **DeletePod** | Delete pods stuck in a terminal state |

Each action type also carries structured `whenToUse`, `whenNotToUse`, and `preconditions` descriptions that the LLM evaluates during workflow selection.

**Three-Step Discovery Example** (OOMKill scenario):

```
Step 1: list_available_actions(severity=critical, environment=staging)
  LLM asks: "What remediation actions are available for this context?"
  -> Returns: IncreaseMemoryLimits, ScaleReplicas, RestartPod, RollbackDeployment, ...

Step 2: list_workflows(action_type=IncreaseMemoryLimits, severity=critical)
  LLM asks: "Which workflows implement IncreaseMemoryLimits?"
  -> Returns: oomkill-increase-memory-v1 (engine: job), oomkill-increase-memory-v1 (engine: tekton)

Step 3: get_workflow(workflow_id=oomkill-increase-memory-v1, version=1.0.0)
  LLM asks: "Give me the full schema so I can populate the parameters."
  -> Returns: full workflow-schema.yaml (see Slide 9)
```

The LLM populates the workflow parameters (e.g., `MEMORY_LIMIT_NEW=128Mi`) based on its root cause analysis and returns the selected workflow to the AIAnalysis controller.

---

## Slide 7: Remediation Orchestrator

**Purpose**: Coordinates the full remediation lifecycle by creating and watching child CRDs (SignalProcessing, AIAnalysis, WorkflowExecution, Notification, EffectivenessAssessment). The conductor of the pipeline.

**Architecture**: Kubernetes controller (owns `RemediationRequest` CRD).

**CRD**: `RemediationRequest` (owns all child CRDs via owner references)

**Key Features**:
- Child CRD lifecycle: creates SP, AA, WE, NR, EA at the right phase transitions
- Approval flow: creates `RemediationApprovalRequest` when Rego policy requires human approval
- Routing engine: scope blocking, consecutive failure blocking, deduplication
- Timeout handling: global (1h default) + per-phase (Processing 5m, Analyzing 10m, Executing 30m)
- Pre-remediation spec hash: captures SHA-256 of target resource `.spec` before workflow runs
- AI-resolved target: uses LLM-identified `AffectedResource` (e.g., Deployment) instead of signal-sourced target (e.g., Pod)

---

## Slide 8: Workflow Execution

**Purpose**: Executes remediation workflows by creating Tekton PipelineRuns or Kubernetes Jobs, monitors their progress, and reports structured success/failure details back to the orchestrator.

**Architecture**: Kubernetes controller (watches `WorkflowExecution` CRDs).

**CRD**: `WorkflowExecution`

**Key Features**:
- Pure executor: RO makes all routing decisions (cooldown, blocking, backoff) before WFE creation (DD-RO-002)
- Executor registry: `tekton` (PipelineRun with OCI bundle resolver) and `job` (Kubernetes Job)
- Deterministic resource naming: prevents duplicate executions for the same target
- Resource locking: deterministic name-based locking from `targetResource` identity
- Structured failure details: TaskFailed, OOMKilled, DeadlineExceeded, ImagePullBackOff
- External deletion handling: detects when PipelineRun/Job is deleted outside the controller
- Dedicated execution namespace (`kubernaut-workflows`): isolation from application workloads
- Audit events: workflow.started (with parameters), workflow.completed/failed

---

## Slide 9: Workflow Schema (workflow-schema.yaml)

**Purpose**: Every remediation workflow ships as an OCI image containing a `/workflow-schema.yaml` that declares what the workflow does, when to use it, and what parameters it needs. This schema is extracted during registration and stored in the workflow catalog for LLM-driven discovery.

**Container Image Reference** (DD-WORKFLOW-002 v2.4):
- V1.0 requires **tag + digest** for immutability and audit compliance
- Full pullspec: `container_image@container_digest`
- The digest guarantees that the exact image bytes are auditable and reproducible

**Example**: `workflow-schema.yaml` for the OOMKill increase-memory workflow:

```yaml
metadata:
  workflowId: oomkill-increase-memory-v1
  version: "1.0.0"
  description:
    what: "Increases memory limits for pods experiencing OOMKill events"
    whenToUse: "When pods are OOMKilled due to insufficient memory limits"
    whenNotToUse: "When OOM is caused by a memory leak requiring code fix"
    preconditions: "Pod is managed by a Deployment or StatefulSet"

actionType: IncreaseMemoryLimits

labels:
  signalName: OOMKilled
  severity: [critical, high]
  environment: [production, staging, test]
  component: "*"
  priority: "*"

execution:
  engine: job
  # V1.0: tag + SHA256 digest for immutability and audit trail
  containerImage: quay.io/kubernaut-cicd/workflows/oomkill-increase-memory:v1.0.0@sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4

# Operator-defined labels for additional filtering (separate from mandatory labels)
customLabels:
  team: platform-sre
  cost-profile: memory-intensive
  change-risk: low

parameters:
  - name: TARGET_RESOURCE_KIND
    type: string
    required: true
    description: "Kubernetes resource kind (Deployment, StatefulSet, DaemonSet)"
  - name: TARGET_RESOURCE_NAME
    type: string
    required: true
    description: "Name of the resource to patch"
  - name: TARGET_NAMESPACE
    type: string
    required: true
    description: "Namespace of the resource"
  - name: MEMORY_LIMIT_NEW
    type: string
    required: true
    description: "New memory limit to apply (e.g., 128Mi, 256Mi, 1Gi)"
```

**Key Design Decisions**:
- `containerImage` with tag + digest ensures the exact workflow binary is traceable in the audit trail
- `actionType` maps to the action taxonomy used by the LLM's three-step discovery protocol (list actions > list workflows > get schema)
- `labels` (mandatory) enable catalog filtering: severity, environment, component, priority
- `customLabels` (optional) are operator-defined key-value pairs for team/org-specific filtering -- stored separately from mandatory labels in their own JSONB column, passed through the discovery protocol for additional matching
- `description` fields (`what`, `whenToUse`, `whenNotToUse`, `preconditions`) are shown to the LLM during workflow selection
- `parameters` are populated by the LLM based on the incident context

---

## Slide 10: Effectiveness Monitor

**Purpose**: Assesses whether the remediation actually worked by running four independent checks after a stabilization window, then emitting structured audit events. Closes the feedback loop.

**Architecture**: Kubernetes controller (watches `EffectivenessAssessment` CRDs created by the Orchestrator).

**CRD**: `EffectivenessAssessment`

**Key Features**:
- 4 assessment components, each scored 0.0-1.0:
  - **Health**: pod status, readiness, restarts, CrashLoopBackOff, OOMKilled
  - **Alert resolution**: queries AlertManager for signal resolution
  - **Metrics**: 5 PromQL queries (CPU, memory, latency p95, error rate, throughput) pre vs post
  - **Spec hash**: SHA-256 comparison of pre/post remediation target `.spec`
- Kind-aware health checks: Deployment/RS/SS/DS use label-based pod listing, Pod uses direct lookup, ConfigMap/Secret/Node report "not applicable"
- Stabilization window: configurable wait before assessment (default 5m)
- Validity window: maximum time to complete assessment (default 30m)
- Spec drift guard: invalidates assessment if target spec changes during assessment
- Assessment reasons: `full`, `partial`, `no_execution`, `expired`, `spec_drift`, `metrics_timed_out`
- 5 typed audit events emitted to DataStorage

---

## Slide 11: Notification

**Purpose**: Delivers notifications to configured channels with retries, spec-field-based routing, and circuit breakers. Informs operators about remediation outcomes, approvals, and escalations.

**Architecture**: Kubernetes controller (watches `NotificationRequest` CRDs).

**CRD**: `NotificationRequest`

**Key Features**:
- Delivery channels: Slack (webhook with circuit breaker), Console, File, Log
- Spec-field-based routing: ConfigMap-driven rules with hot-reload (no restart required)
- Exponential backoff retries
- Circuit breaker: prevents cascading failures to external webhooks
- Notification types: approval requests, timeout escalations, completion summaries

---

## Slide 12: DataStorage

**Purpose**: Centralized HTTP API that stores and serves audit events, the workflow catalog, and remediation history. The only service that talks directly to PostgreSQL. Every other service reads and writes through its REST API.

**Architecture**: HTTP server (OpenAPI-driven, ogen-generated clients). Not a controller.

**Key Features**:
- Audit events API: batch write, query by correlation ID, hash chain verification (SOC2 compliance)
- Workflow catalog API: CRUD, action-type taxonomy, version lifecycle (active/deprecated)
- Workflow discovery API: three-step protocol for LLM-driven remediation selection
- Remediation history API: two-tier windowing (24h recent + 90d historical) with three-way hash comparison
- OpenAPI-first: `data-storage-v1.yaml` spec with auto-generated Go and Python clients
- PostgreSQL primary storage, Redis dead letter queue for failed audit writes

---

## Slide 13: AuthWebhook

**Purpose**: Kubernetes admission webhook that injects authenticated user identity into CRD status updates for SOC2 CC8.1 audit compliance. Ensures every remediation action is traceable to a human or service account.

**Architecture**: Mutating + Validating admission webhook.

**Key Features**:
- Mutating webhooks for identity injection:
  - WorkflowExecution: `status.initiatedBy`, `status.approvedBy`
  - RemediationApprovalRequest: `status.approvedBy`, `status.rejectedBy`
  - RemediationRequest: `status.lastModifiedBy`, `status.lastModifiedAt`
- Validating webhook: audit event before NotificationRequest deletion
- Forgery detection: overwrites user-provided identity fields and logs tampering attempts
- Decision validation: enforces Approved/Rejected/Expired for approval decisions
- Namespace selector: only processes namespaces with `kubernaut.ai/audit-enabled=true`
- mTLS via cert-manager; failure policy `Fail` (rejects requests if webhook is down)

---

## Coverage Snapshot (PR #90, February 2026)

| Service | Unit | Integration | E2E | All Tiers |
|---------|------|-------------|-----|-----------|
| Signal Processing | 87.3% | 61.4% | 58.2% | 85.4% |
| AI Analysis | 80.0% | 73.6% | 53.8% | 87.6% |
| Workflow Execution | 74.0% | 67.9% | 56.0% | 82.9% |
| Remediation Orchestrator | 79.9% | 59.8% | 49.1% | 82.1% |
| Notification | 75.5% | 57.6% | 49.5% | 73.3% |
| Effectiveness Monitor | 72.1% | 64.9% | 68.8% | 81.9% |
| Gateway | 65.5% | 42.5% | 59.0% | 81.5% |
| DataStorage | 60.1% | 34.9% | 48.7% | 65.4% |
| HolmesGPT-API | 79.0% | 62.1% | 59.1% | 94.1% |
| AuthWebhook | 50.0% | 49.0% | 41.6% | 78.4% |

---

## Live Demo: OOMKill Remediation

**Scenario**: A `memory-eater` pod is deployed with a 50Mi memory limit. The application needs 60Mi, so it gets OOMKilled by the kernel and enters CrashLoopBackOff.

**What Kubernaut Does** (autonomously, no human intervention):

1. Gateway ingests the OOMKill Kubernetes event
2. Signal Processing enriches it (owner chain, labels, Rego classification)
3. AI Analysis sends it to the LLM via HolmesGPT-API
4. LLM diagnoses "memory limit too low" with 0.95 confidence
5. LLM selects the `oomkill-increase-memory` workflow
6. Rego approval policy evaluates (auto-approved in dev environment)
7. Workflow Execution runs a K8s Job that patches the Deployment's memory limit from 50Mi to 128Mi
8. Kubernetes rolls out a new pod with the higher limit
9. Pod starts successfully and stays Running
10. Effectiveness Monitor assesses the fix (health, alerts, metrics, spec hash)
11. Notification sends a Slack message with the outcome

**Key Observation**: The entire pipeline runs autonomously -- no human intervention from alert to fix. Full audit trail in DataStorage at every step.

---

## Q&A

**github.com/jordigilh/kubernaut**
