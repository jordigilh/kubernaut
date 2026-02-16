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
- Pluggable adapters: Prometheus AlertManager (`/api/v1/signals/prometheus`), Kubernetes Events (`/api/v1/signals/kubernetes-event`)
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
- Phase flow: Pending > Enriching > Classifying > Categorizing > Completed
- K8s enrichment: namespace, pod, deployment, owner chain, node, PDB, HPA
- 5 Rego classifiers: Environment, Priority, Severity, Business, CustomLabels
- Detected labels: GitOps, Helm, PDB-protected, HPA-managed, service mesh, network isolation
- Signal mode: predictive vs reactive
- Recovery context propagation for retries
- Degraded mode: partial enrichment when target resource is not found

**Rego Policy -- Priority Classification** (`deploy/signalprocessing/policies/priority.rego`):

```rego
package signalprocessing.priority

import rego.v2

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
- Phase flow: Pending > Investigating > Analyzing > Completed
- HolmesGPT integration: async session (submit > poll > result)
- Workflow selection: HAPI returns selected workflow (ID, image, parameters, rationale)
- Rego approval policy: approved / manual_review_required / denied
- Affected resource: LLM can identify a different target than the signal source (e.g., Deployment instead of Pod)
- Recovery flow: uses previous execution history to avoid repeating failed remediations
- Execution engine: supports `tekton` and `job` backends
- WorkflowNotNeeded: LLM can determine the problem self-resolved (no workflow created)

**Rego Policy -- Approval** (`config/rego/aianalysis/approval.rego`):

```rego
package aianalysis.approval

import rego.v2

default require_approval := false

# Production environment requires manual approval
require_approval if {
    is_production
}

# Multiple recovery attempts (3+) require approval in ANY environment
require_approval if {
    is_multiple_recovery
}

# Production + unvalidated target requires approval
require_approval if {
    is_production
    not target_validated
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
- Three API endpoints: incident analysis, recovery analysis, post-execution analysis
- 3-step workflow discovery protocol against DataStorage:
  1. `list_available_actions` (what can I do?)
  2. `list_workflows` (which workflows implement this action?)
  3. `get_workflow` (fetch the selected workflow's schema and parameters)
- Remediation history enrichment: queries DataStorage for past remediations on the same target, injects into prompt so the LLM avoids repeating failed approaches
- Three-way hash comparison: detects if the target spec drifted since last remediation
- Session-based async API: submit investigation > poll for result
- Mock LLM mode: deterministic responses for testing without a real LLM
- Auth: K8s ServiceAccount tokens (TokenReview + SubjectAccessReview)

**Pipeline Integration**:
- AIAnalysis controller calls HAPI for incident/recovery analysis
- HAPI calls DataStorage for workflow catalog and remediation history
- HAPI returns: root cause, affected resource, selected workflow, approval context

---

## Slide 6: Remediation Orchestrator

**Purpose**: Coordinates the full remediation lifecycle by creating and watching child CRDs (SignalProcessing, AIAnalysis, WorkflowExecution, Notification, EffectivenessAssessment). The conductor of the pipeline.

**Architecture**: Kubernetes controller (owns `RemediationRequest` CRD).

**CRD**: `RemediationRequest` (owns all child CRDs via owner references)

**Key Features**:
- Phase state machine: Pending > Processing > Analyzing > AwaitingApproval > Executing > Completed / Failed / TimedOut
- Child CRD lifecycle: creates SP, AA, WE, NR, EA at the right phase transitions
- Approval flow: creates `RemediationApprovalRequest` when Rego policy requires human approval
- Routing engine: scope blocking, consecutive failure blocking, deduplication
- Timeout handling: global (1h default) + per-phase (Processing 5m, Analyzing 10m, Executing 30m)
- Pre-remediation spec hash: captures SHA-256 of target resource `.spec` before workflow runs
- EffectivenessAssessment creation on all terminal phases (Completed, Failed, TimedOut)
- AI-resolved target: uses LLM-identified `AffectedResource` (e.g., Deployment) instead of signal-sourced target (e.g., Pod)
- Atomic status updates with optimistic concurrency (RetryOnConflict)
- Full audit trail to DataStorage at every phase transition

---

## Slide 7: Workflow Execution

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

## Slide 8: Effectiveness Monitor

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

## Slide 9: Notification

**Purpose**: Delivers notifications to configured channels with retries, label-based routing, and circuit breakers. Informs operators about remediation outcomes, approvals, and escalations.

**Architecture**: Kubernetes controller (watches `NotificationRequest` CRDs).

**CRD**: `NotificationRequest`

**Key Features**:
- Phase flow: Pending > Sending > Retrying > Sent / PartiallySent / Failed
- Delivery channels: Slack (webhook with circuit breaker), Console, File, Log
- Label-based routing: ConfigMap-driven rules with hot-reload (no restart required)
- Exponential backoff retries
- Circuit breaker: prevents cascading failures to external webhooks
- Notification types: approval requests, timeout escalations, completion summaries
- Audit events: message.sent, message.failed
- Atomic status updates per delivery attempt

---

## Slide 10: DataStorage

**Purpose**: Centralized HTTP API that stores and serves audit events, the workflow catalog, and remediation history. The only service that talks directly to PostgreSQL. Every other service reads and writes through its REST API.

**Architecture**: HTTP server (OpenAPI-driven, ogen-generated clients). Not a controller.

**Key Features**:
- Audit events API: batch write, query by correlation ID, hash chain verification (SOC2 compliance)
- Workflow catalog API: CRUD, action-type taxonomy, version lifecycle (active/deprecated)
- Workflow discovery API: three-step protocol for LLM-driven remediation selection
- Remediation history API: two-tier windowing (24h recent + 90d historical) with three-way hash comparison
- OpenAPI-first: `data-storage-v1.yaml` spec with auto-generated Go and Python clients
- PostgreSQL primary storage, Redis dead letter queue for failed audit writes
- K8s auth: TokenReview + SubjectAccessReview for service-to-service authentication
- Graceful shutdown with connection draining

---

## Slide 11: AuthWebhook

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
