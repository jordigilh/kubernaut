# BR-SEVERITY-001: Standardized Severity Levels

**Business Requirement ID**: BR-SEVERITY-001
**Category**: Cross-Cutting (All Services)
**Priority**: P0
**Target Version**: V1.0
**Status**: ✅ Approved
**Date**: 2026-02-10
**Last Updated**: 2026-02-15

**Related Design Decisions**:
- [DD-SEVERITY-001: Severity Determination Refactoring](../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md)
- [DD-WORKFLOW-001: Mandatory Workflow Label Schema](../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) — Severity stored as JSONB array in workflow labels (v2.7)

**Related Business Requirements**:
- **BR-SP-105**: Severity Determination via Rego Policy
- **BR-GATEWAY-111**: Gateway Signal Pass-Through Architecture
- **BR-KA-197**: Human Review Required Flag
- **BR-KA-212**: RCA Target Resource in Root Cause Analysis

---

## 📋 **Business Need**

### **Problem Statement**

Kubernaut uses severity levels across multiple components and boundaries:

1. **External systems** (Prometheus, PagerDuty, custom alerting) produce severity values in arbitrary schemes (Sev1-4, P0-P4, Critical/High/Medium/Low, etc.); these are source values, not yet canonical Kubernaut classifications
2. **SignalProcessing** normalizes external severity to an internal canonical set via Rego policy (DD-SEVERITY-001)
3. **Kubernaut Agent (KA)** receives the SignalProcessing classification and MUST preserve it in the RCA; the LLM does not independently classify or override severity
4. **AIAnalysis CRD** validates severity against a `kubebuilder:validation:Enum`
5. **Workflow catalog** uses severity as a search filter label

Without a single authoritative definition of what each canonical severity level **means**, with concrete examples, the system risks:

- LLM replacing the SignalProcessing classification with a severity inferred from RCA evidence
- Inconsistent interpretation across components (one component treats `"high"` differently than another)
- Ambiguity for operators writing Rego policies about which level to map to
- Drift between prompt definitions and CRD validation enums

### **Business Value**

| Benefit | Impact |
|---------|--------|
| **Consistency** | All components use the same severity taxonomy with the same semantics |
| **Severity Integrity** | KA preserves SignalProcessing's Rego classification through RCA and workflow selection |
| **Operator Clarity** | Operators writing Rego policies have unambiguous mapping targets |
| **Technical Documentation** | Single source of truth for severity definitions across all docs |
| **CRD Validation** | Enum values aligned with documented levels; no surprises at admission time |

---

## 🎯 **Requirement**

### **Canonical Severity Levels**

Kubernaut defines exactly **five** canonical severity levels. Canonical classification fields (including SP outputs, AIA context, KA context, workflow labels, metrics, and audit events) MUST use one of these values. Raw external source values may remain in the incoming RR severity only until SP Rego normalizes them (DD-AF-016).

The levels are ordered from most to least severe:

| Level | Action Required | User Impact Threshold | Response Time Expectation |
|-------|----------------|----------------------|--------------------------|
| **critical** | Immediate remediation required | >50% of users affected | Minutes |
| **high** | Urgent remediation needed | 10-50% of users affected | < 1 hour |
| **medium** | Remediation recommended | <10% of users affected | Hours |
| **low** | Remediation optional | No user impact | Days / next maintenance window |
| **unknown** | Human triage required | Cannot be determined | Depends on triage outcome |

---

## 📖 **Severity Level Definitions**

### **critical** — Immediate remediation required

The system is experiencing a condition that causes **complete service unavailability, active data loss, or an actively exploited security breach**. Immediate automated or manual remediation is required to restore service.

**Characteristics**:
- Production service completely unavailable
- Data loss or corruption actively occurring
- Security breach actively being exploited
- SLA violation in progress
- Revenue-impacting outage
- Affects >50% of users

**Kubernetes Examples**:
- A Deployment with `replicas: 3` has all 3 pods in `CrashLoopBackOff` — the service has zero available endpoints and incoming requests fail with `503 Service Unavailable`
- A StatefulSet pod running PostgreSQL is `OOMKilled` repeatedly, causing the database to be unreachable and all dependent services to fail with connection errors
- A Node enters `NotReady` state and it is the only node in a zone running a critical service without cross-zone redundancy — all pods on that node are evicted with no capacity to reschedule
- A PersistentVolumeClaim enters `Lost` state on a volume containing the only replica of production data, and writes are failing with `I/O error`

---

### **high** — Urgent remediation needed

The system is experiencing **significant degradation** that is escalating toward critical impact. The service is partially functional but operating well outside acceptable parameters.

**Characteristics**:
- Significant service degradation (>50% performance loss)
- High error rate (>10% of requests failing)
- Production issue escalating toward critical
- Affects 10-50% of users
- SLA at risk

**Kubernetes Examples**:
- A Deployment with `replicas: 3` has 1 pod `OOMKilled` and restarting, leaving 2 healthy replicas — the service is degraded with increased latency and reduced throughput, and another failure would cause an outage
- A pod's liveness probe is failing intermittently, causing Kubernetes to restart it every few minutes — users experience transient errors during each restart cycle
- A HorizontalPodAutoscaler is at `maxReplicas` and CPU utilization is at 95% — the service cannot scale further and response times are increasing toward SLA breach
- An `ImagePullBackOff` on a canary deployment blocks a critical security patch from rolling out while the vulnerability is known and actively scanned

---

### **medium** — Remediation recommended

The system has a **non-urgent issue** that should be addressed but is not causing significant user impact. Left unattended, the issue may escalate.

**Characteristics**:
- Minor service degradation (<50% performance loss)
- Moderate error rate (1-10% of requests failing)
- Non-production critical issues
- Affects <10% of users
- Staging/development critical issues

**Kubernetes Examples**:
- A Deployment with `replicas: 5` has 1 pod in `CrashLoopBackOff` — 4 healthy replicas handle the load comfortably, but the failing pod consumes restart resources and reduces headroom
- A pod is nearing its memory limit (using 85% of `limits.memory`) without being OOMKilled — performance is stable but the pod is at risk under load spikes
- A CronJob is failing on every other execution due to a transient DNS resolution error — half the scheduled jobs succeed, but the failure pattern indicates a flaky dependency
- A staging environment Deployment is completely down due to an `ImagePullBackOff` — no production impact, but it blocks QA validation of an upcoming release

---

### **low** — Remediation optional

The system has a **minor or informational issue** that does not affect users or service quality. Remediation can be deferred to the next maintenance window or addressed opportunistically.

**Characteristics**:
- Informational issues
- Optimization opportunities
- Development environment issues
- No user impact
- Capacity planning alerts

**Kubernetes Examples**:
- A pod is using 40% of its `requests.cpu` but has `limits.cpu` set 10x higher — the resource is over-provisioned, wasting cluster capacity, but the service runs fine
- A Deployment has a `FailedScheduling` warning for a non-critical batch job because node affinity rules are too restrictive — the job runs when capacity becomes available
- A development namespace has pods in `Pending` state because the namespace resource quota is exhausted — no production impact, developers need to clean up old resources
- A container image tag `:latest` is used in a non-production Deployment — this is a best-practice violation but causes no immediate issue
- `PodDisruptionBudget` is configured with `minAvailable: 1` on a single-replica Deployment — there is no practical disruption budget, but the service is not critical

---

### **unknown** — Human triage required

The severity classification is **unavailable** due to insufficient data, ambiguous signals, conflicting evidence, or an unmapped external value. `unknown` remains accepted by CRD enums as a sentinel, but it is not a successful SP classification: SP MUST fail the Classifying phase, and the remediation pipeline MUST NOT route to a workflow or ask an LLM to invent a replacement. An operator must correct the policy/input before processing can continue.

**Characteristics**:
- Root cause could not be determined
- Conflicting signals prevent a confident assessment
- Insufficient monitoring data or logs to evaluate impact
- The condition is novel and has no precedent in the system
- External dependencies prevent full investigation (e.g., RBAC restrictions, API unavailability)

**Kubernetes Examples**:
- A pod is in `CrashLoopBackOff` but the container logs are empty and no events provide context — the investigator cannot determine whether this is a critical production outage or a misconfigured development workload
- A Node shows intermittent `NotReady` conditions lasting a few seconds each — it is unclear whether this is a transient network glitch or early signs of node hardware failure
- A Service has elevated error rates, but the investigation toolset lacks permission to read pod logs in the target namespace — the severity cannot be assessed without access to the relevant data
- An alert fires for a resource in a namespace that has no labels indicating environment or ownership — it is impossible to determine whether this is a production or test workload

---

## 🔗 **Component Alignment**

This table documents where each component enforces or references the canonical severity levels:

| Component | Mechanism | Levels Supported | Source File |
|-----------|-----------|-----------------|-------------|
| **AIAnalysis CRD** | `kubebuilder:validation:Enum` | `critical`, `high`, `medium`, `low`, `unknown` | `api/aianalysis/v1alpha1/aianalysis_types.go` |
| **Kubernaut Agent (KA) Incident Prompt** | Pass-through instruction for SP-classified input | Same severity values as SignalProcessing | `internal/kubernautagent/prompt/templates/incident_investigation.tmpl` |
| **KA Workflow Selection Prompt** | Pass-through instruction for SP-classified input | Same severity values as SignalProcessing | `internal/kubernautagent/prompt/templates/phase3_workflow_selection.tmpl` |
| **SignalProcessing Rego** | Rego policy output | `critical`, `high`, `medium`, `low`, `unknown` | `config/rego/severity.rego` |
| **API Frontend** | Requires and passes through explicit correlated alert/rule severity before RR creation; does not infer, default, or canonicalize it | Operator-defined source values until SP normalization | `pkg/apifrontend/severity/triage.go`; [DD-AF-016](../architecture/decisions/DD-AF-016-explicit-alert-severity-source.md) |
| **Workflow Catalog** | DataStorage label filter (JSONB array, ? operator) | `[critical, high, medium, low]` | `api/openapi/data-storage-v1.yaml` |
| **Prometheus Metrics** | Label cardinality | `critical`, `high`, `medium`, `low`, `unknown` | Various `metrics.go` files |

**Note**: The Workflow Catalog does not use `unknown` because workflows are authored for specific, known conditions. An `unknown` severity assessment triggers human review (BR-KA-197), not workflow execution.

**Workflow labels**: In the workflow catalog, severity is stored as a JSONB array (e.g., `["critical"]` or `["critical", "high"]`). A workflow can declare multiple severity levels to indicate it handles signals at any of those levels. The `"*"` wildcard is supported (DD-WORKFLOW-001 v2.8) — `severity: ["*"]` matches any severity. Alternatively, listing all four levels `["critical", "high", "medium", "low"]` achieves the same result. Search queries use the JSONB `?` operator: `labels->'severity' ? $severity_filter OR labels->'severity' ? '*'`.

---

## 📐 **Design Constraints**

### Bounded Cardinality

The 5-level set is deliberately constrained to maintain acceptable Prometheus metric cardinality and operator cognitive load. Adding new levels requires a formal BR amendment.

### LLM Prompt Alignment

SignalProcessing Rego is the **source of the severity classification**. KA may receive a valid SP severity as read-only investigation context, but LLM prompts and response schemas MUST NOT ask the model to copy, produce, reassess, or override that classification. Any final severity-bearing result must be populated from trusted SP-derived server state.

### Rego Policy Mapping Target

Operators writing Rego policies for SignalProcessing MUST map external severity values to a concrete supported severity. If unmapped inputs are expected, the operator's Rego policy MUST define a concrete catch-all value appropriate to workflow routing. `unknown` is an enum-compatible sentinel, not a successful fallback; an empty or `unknown` policy result MUST fail SP classification. SP MUST NOT supply a Go fallback, and downstream LLMs MUST NOT guess.

### API Frontend Source-Severity Gate

Before SP runs, API Frontend MUST source a `RemediationRequest` severity from
an explicit, correlated Prometheus alert or alerting-rule `severity` label.
For a rule-only potential-issue investigation, matching candidate rules must
all provide the same non-empty raw severity; missing or conflicting values
fail closed and create no RR. Multiple alert instances in the selected
specificity/state bucket must likewise agree on one non-empty raw value. AF
MUST pass the raw source value unchanged:
it MUST NOT ask an LLM to infer severity, map an unrecognized value to
`warning`, or supply another local fallback. SP Rego then owns canonical
normalization and fails classification if it cannot produce usable required
outputs (DD-AF-016).

### CRD Validation

Any CRD field that stores a canonical severity value MUST use `+kubebuilder:validation:Enum=critical;high;medium;low;unknown` to ensure Kubernetes admission rejects invalid values before they enter the system.

---

## ✅ **Acceptance Criteria**

| # | Criterion | Verification |
|---|-----------|-------------|
| AC-1 | All five levels (`critical`, `high`, `medium`, `low`, `unknown`) are accepted by the AIAnalysis CRD | CRD validation test |
| AC-2 | KA incident prompt treats valid SP severity as read-only context and does not request it in model-authored RCA output | Prompt/schema unit test |
| AC-3 | KA workflow-selection prompt does not request or accept model-authored SP severity | Prompt/schema unit test |
| AC-4 | SignalProcessing Rego catch-all returns a concrete policy-defined value; empty or `unknown` output transitions SP to `PhaseFailed` rather than completing | Rego and controller integration tests |
| AC-5 | No canonical classification field uses a severity outside this set (e.g., `warning`, `info`, `error`); raw external source labels may remain arbitrary only until SP Rego normalization | `grep` audit across canonical fields and pass-through tests |
| AC-6 | DD-SEVERITY-001 references this BR as the canonical definition | Document cross-reference |
| AC-7 | Workflow catalog stores severity as a JSONB array in labels; `"*"` wildcard supported (DD-WORKFLOW-001 v2.8); search uses the JSONB `?` operator with wildcard fallback | Schema inspection + integration test |
| AC-8 | AF creates no RR without explicit, unambiguous correlated alert/rule severity; otherwise it passes the raw source value unchanged, leaving canonical mapping or failure to SP Rego | AF triage unit/integration tests and SP classification test |

---

## 📚 **References**

- [DD-SEVERITY-001: Severity Determination Refactoring](../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md) — Architectural decision for Rego-based severity normalization
- [DD-SEVERITY-001 Implementation Plan](../implementation/DD-SEVERITY-001-implementation-plan.md) — Week-by-week implementation status
- [BR-SP-105](../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md) — SignalProcessing Rego severity determination
- [BR-GATEWAY-111](../services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md) — Gateway severity pass-through
- [DD-AF-016](../architecture/decisions/DD-AF-016-explicit-alert-severity-source.md) — AF requires an explicit source label and never infers/defaults severity

---

**Document Version**: 1.3 (updated for #2467 / DD-AF-016, 2026-09-27)
**Author**: AI Assistant (reviewed by Jordi Gil)
**Next Review**: After E2E severity scenarios (Sprint N+1)
