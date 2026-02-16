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
- **BR-HAPI-197**: Human Review Required Flag
- **BR-HAPI-212**: RCA Target Resource in Root Cause Analysis

---

## 📋 **Business Need**

### **Problem Statement**

Kubernaut uses severity levels across multiple components and boundaries:

1. **External systems** (Prometheus, PagerDuty, custom alerting) produce severity values in arbitrary schemes (Sev1-4, P0-P4, Critical/High/Medium/Low, etc.)
2. **SignalProcessing** normalizes external severity to an internal canonical set via Rego policy (DD-SEVERITY-001)
3. **HAPI LLM prompts** instruct the LLM to assess root cause severity using canonical levels
4. **AIAnalysis CRD** validates severity against a `kubebuilder:validation:Enum`
5. **Workflow catalog** uses severity as a search filter label

Without a single authoritative definition of what each canonical severity level **means**, with concrete examples, the system risks:

- LLM returning values outside the allowed set (e.g., `"warning"`, `"info"`)
- Inconsistent interpretation across components (one component treats `"high"` differently than another)
- Ambiguity for operators writing Rego policies about which level to map to
- Drift between prompt definitions and CRD validation enums

### **Business Value**

| Benefit | Impact |
|---------|--------|
| **Consistency** | All components use the same severity taxonomy with the same semantics |
| **LLM Reliability** | Prompts with precise definitions and examples reduce hallucinated severity values |
| **Operator Clarity** | Operators writing Rego policies have unambiguous mapping targets |
| **Technical Documentation** | Single source of truth for severity definitions across all docs |
| **CRD Validation** | Enum values aligned with documented levels; no surprises at admission time |

---

## 🎯 **Requirement**

### **Canonical Severity Levels**

Kubernaut defines exactly **five** canonical severity levels. All internal components (CRDs, LLM prompts, workflow catalog labels, metrics, audit events) MUST use one of these values.

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

The investigation **could not determine the severity** due to insufficient data, ambiguous signals, or conflicting evidence. A human operator must review the situation and assign the appropriate severity.

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
| **HAPI Incident Prompt** | LLM instruction text | `critical`, `high`, `medium`, `low`, `unknown` | `holmesgpt-api/src/extensions/incident/prompt_builder.py` |
| **HAPI Recovery Prompt** | LLM instruction text | `critical`, `high`, `medium`, `low`, `unknown` | `holmesgpt-api/src/extensions/recovery/prompt_builder.py` |
| **SignalProcessing Rego** | Rego policy output | `critical`, `high`, `medium`, `low`, `unknown` | `config/rego/severity.rego` |
| **Workflow Catalog** | DataStorage label filter (JSONB array, ? operator) | `[critical, high, medium, low]` | `api/openapi/data-storage-v1.yaml` |
| **Prometheus Metrics** | Label cardinality | `critical`, `high`, `medium`, `low`, `unknown` | Various `metrics.go` files |

**Note**: The Workflow Catalog does not use `unknown` because workflows are authored for specific, known conditions. An `unknown` severity assessment triggers human review (BR-HAPI-197), not workflow execution.

**Workflow labels**: In the workflow catalog, severity is stored as a JSONB array (e.g., `["critical"]` or `["critical", "high"]`). A workflow can declare multiple severity levels to indicate it handles signals at any of those levels. The `*` wildcard is not used — to match any severity, list all four levels: `[critical, high, medium, low]`. Search queries use the JSONB `?` operator: `labels->'severity' ? $severity_filter`.

---

## 📐 **Design Constraints**

### Bounded Cardinality

The 5-level set is deliberately constrained to maintain acceptable Prometheus metric cardinality and operator cognitive load. Adding new levels requires a formal BR amendment.

### LLM Prompt Alignment

The severity definitions in this BR are the **single source of truth**. Both `incident/prompt_builder.py` and `recovery/prompt_builder.py` MUST reproduce these definitions verbatim (or by reference) so the LLM receives consistent instructions.

### Rego Policy Mapping Target

Operators writing Rego policies for SignalProcessing MUST map external severity values to one of these five canonical levels. The `unknown` level serves as the default fallback for unmapped values.

### CRD Validation

Any CRD field that stores a canonical severity value MUST use `+kubebuilder:validation:Enum=critical;high;medium;low;unknown` to ensure Kubernetes admission rejects invalid values before they enter the system.

---

## ✅ **Acceptance Criteria**

| # | Criterion | Verification |
|---|-----------|-------------|
| AC-1 | All five levels (`critical`, `high`, `medium`, `low`, `unknown`) are accepted by the AIAnalysis CRD | CRD validation test |
| AC-2 | HAPI incident prompt severity section matches this BR's definitions | Code review / prompt unit test |
| AC-3 | HAPI recovery prompt severity section matches this BR's definitions | Code review / prompt unit test |
| AC-4 | SignalProcessing default Rego policy maps to all five levels | Rego unit test |
| AC-5 | No component uses severity values outside this set (e.g., `warning`, `info`, `error`) | `grep` audit across codebase |
| AC-6 | DD-SEVERITY-001 references this BR as the canonical definition | Document cross-reference |
| AC-7 | Workflow catalog stores severity as a JSONB array in labels; no `*` wildcard; search uses the JSONB `?` operator (DD-WORKFLOW-001 v2.7) | Schema inspection + integration test |

---

## 📚 **References**

- [DD-SEVERITY-001: Severity Determination Refactoring](../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md) — Architectural decision for Rego-based severity normalization
- [DD-SEVERITY-001 Implementation Plan](../implementation/DD-SEVERITY-001-implementation-plan.md) — Week-by-week implementation status
- [BR-SP-105](../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md) — SignalProcessing Rego severity determination
- [BR-GATEWAY-111](../services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md) — Gateway severity pass-through

---

**Document Version**: 1.1
**Author**: AI Assistant (reviewed by Jordi Gil)
**Next Review**: After E2E severity scenarios (Sprint N+1)
