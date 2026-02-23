# BR-PLATFORM-002: Demo Scenario Specification and Authoring Guide

**Business Requirement ID**: BR-PLATFORM-002
**Category**: Platform
**Priority**: P1
**Target Version**: V1.0
**Status**: Pending
**Date**: 2026-02-23

---

## Business Need

### Problem Statement

Kubernaut has 22 runnable demo scenarios under `deploy/demo/scenarios/`, each showcasing different aspects of the remediation lifecycle. These scenarios follow a consistent but **undocumented** structure. New contributors have no formal specification for what a scenario must include, how to integrate it with the shared infrastructure, or what quality bar to meet.

**Current Limitations**:
- No formal specification for scenario deliverables or directory structure
- No documented contract for `run.sh`, `cleanup.sh`, or `README.md` content
- No checklist for workflow image lifecycle (build, push, make public, seed)
- Inconsistencies across scenarios (missing `inject-*` scripts, missing VHS tapes, stale `kind-config.yaml` references in `run.sh`)
- The scenario catalog in `deploy/demo/README.md` is outdated (says 17 scenarios; actual count is 22)

**Impact**:
- Contributors may produce incomplete or structurally inconsistent scenarios
- Scenario reviews lack a concrete checklist to validate against
- Missing deliverables (e.g., `cleanup.sh`, VHS tape) are not caught until late
- Onboarding time for new scenario authors is unnecessarily high

---

## Business Objective

Establish a formal specification for demo scenario structure, deliverables, and integration so that any contributor can author a new scenario that is structurally consistent with the existing 22 scenarios.

### Success Criteria
1. Every new scenario can be validated against a concrete deliverables checklist
2. The specification is derived from the patterns already established by the existing scenarios
3. Existing scenarios can be audited for compliance gaps

---

## Use Cases

### Use Case 1: Adding a New Workflow Scenario

**Scenario**: A contributor wants to add a new scenario that demonstrates automated remediation of a Kubernetes failure mode (e.g., `image-pull-backoff`).

**Current Flow**:
```
1. Contributor looks at an existing scenario for reference
2. Copies files, guesses which are mandatory vs optional
3. Forgets to register in workflow-mappings.sh
4. Forgets to build/push/seed the workflow image
5. PR review catches structural issues late
```

**Desired Flow with BR-PLATFORM-002**:
```
1. Contributor reads BR-PLATFORM-002 for the specification
2. Creates the mandatory directory structure and deliverables
3. Follows the integration checklist (workflow-mappings.sh, image build, seed)
4. Optionally creates a VHS tape from template.tape
5. PR review validates against the formal checklist
```

### Use Case 2: Adding a Behavioral (No-Workflow) Scenario

**Scenario**: A contributor wants to add a scenario that tests platform behavior (e.g., deduplication, escalation) without a dedicated remediation workflow.

**Current Flow**:
```
1. Contributor is unsure whether workflow/ directory is needed
2. No guidance on how to classify the scenario type
```

**Desired Flow with BR-PLATFORM-002**:
```
1. BR-PLATFORM-002 defines three scenario types (Workflow, No-Action, Behavioral)
2. Contributor identifies their scenario type and follows the appropriate deliverables
3. Workflow/ directory is clearly marked as conditional on scenario type
```

---

## Functional Requirements

### FR-PLATFORM-002-01: Scenario Directory Structure

**Requirement**: Every demo scenario SHALL be a self-contained directory under `deploy/demo/scenarios/<scenario-name>/` with a kebab-case name.

**Mandatory files** (all scenario types):

| File | Purpose |
|------|---------|
| `README.md` | BDD specification, acceptance criteria, prerequisites |
| `run.sh` | Single entry point for automated execution |
| `cleanup.sh` | Teardown: delete namespace, restart AlertManager |
| `manifests/namespace.yaml` | Namespace with required labels |
| `manifests/prometheus-rule.yaml` | PrometheusRule defining the alert |

**Acceptance Criteria**:
- Every scenario directory contains all mandatory files
- `run.sh` is executable (`chmod +x`)
- `cleanup.sh` is executable and idempotent (safe to run multiple times)

---

### FR-PLATFORM-002-02: Namespace Conventions

**Requirement**: Every scenario namespace SHALL follow these conventions:

| Label | Value | Required |
|-------|-------|----------|
| Name | `demo-<scenario-slug>` | YES |
| `kubernaut.ai/managed` | `"true"` | YES |
| `kubernaut.ai/environment` | `"production"` or appropriate | YES |

**Example** (`manifests/namespace.yaml`):
```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: demo-crashloop
  labels:
    kubernaut.ai/managed: "true"
    kubernaut.ai/environment: "production"
```

**Acceptance Criteria**:
- Namespace name matches `demo-<scenario-slug>` pattern
- Both `kubernaut.ai/managed` and `kubernaut.ai/environment` labels are present

---

### FR-PLATFORM-002-03: `run.sh` Contract

**Requirement**: Every `run.sh` SHALL source the shared helper scripts and call the platform setup functions in this order:

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-<scenario-slug>"

source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/kind-config.yaml" "${1:-}"

source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack

source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform

# For workflow scenarios only:
seed_scenario_workflow "<scenario-name>"

# Deploy manifests
kubectl apply -f "${SCRIPT_DIR}/manifests/"

# Fault injection (if applicable)
bash "${SCRIPT_DIR}/inject-<fault>.sh"

# Print monitoring instructions
echo "Monitor: kubectl get rr,sp,aa,wfe,ea -n ${NAMESPACE} -w"
```

**Acceptance Criteria**:
- `run.sh` is idempotent (safe to run against an existing cluster)
- `run.sh` prints monitoring instructions at the end
- All scenario-specific dependencies (cert-manager, Linkerd, etc.) are installed by `run.sh` before deploying manifests

---

### FR-PLATFORM-002-04: `cleanup.sh` Contract

**Requirement**: Every `cleanup.sh` SHALL perform these steps:

1. Delete scenario-specific resources (PrometheusRule first to stop alerts)
2. Delete the scenario namespace
3. Wait for namespace deletion to complete
4. Restart AlertManager to clear stale notification state

**Example**:
```bash
#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete namespace demo-<scenario-slug> --ignore-not-found --wait=true

while kubectl get ns demo-<scenario-slug> &>/dev/null; do sleep 2; done

kubectl rollout restart statefulset/alertmanager-kube-prometheus-stack-alertmanager -n monitoring
kubectl rollout status statefulset/alertmanager-kube-prometheus-stack-alertmanager -n monitoring --timeout=60s
```

**Acceptance Criteria**:
- `cleanup.sh` is idempotent (safe to run when scenario was never deployed)
- Uses `--ignore-not-found` on all deletions
- Restarts AlertManager to avoid stale `repeat_interval` suppression

---

### FR-PLATFORM-002-05: `README.md` Contract

**Requirement**: Every scenario `README.md` SHALL include these sections in order:

1. **Title**: `# Scenario #<NUMBER>: <Title>`
2. **Overview**: 1-2 paragraphs describing the scenario narrative
3. **Signal and Outcome**: Alert name and expected pipeline outcome
4. **Prerequisites table**: Kind cluster, services, dependencies, workflow catalog
5. **Automated Run**: `./deploy/demo/scenarios/<name>/run.sh`
6. **Cleanup**: `./deploy/demo/scenarios/<name>/cleanup.sh`
7. **BDD Specification**: Gherkin `Given/When/Then` block
8. **Acceptance Criteria**: Checklist of verifiable outcomes

**Acceptance Criteria**:
- BDD spec uses Gherkin syntax
- Acceptance criteria are checkboxes (`- [ ]`)
- Prerequisites table lists all external dependencies (cert-manager, Linkerd, etc.)

---

### FR-PLATFORM-002-06: Workflow Deliverables (Conditional)

**Requirement**: Scenarios that include a remediation workflow SHALL provide:

| File | Purpose |
|------|---------|
| `workflow/workflow-schema.yaml` | Workflow schema in ADR-043 format |
| `workflow/Dockerfile.exec` | Dockerfile for the exec image |
| `workflow/remediate.sh` | Remediation logic executed by the Job |

**Integration steps** (mandatory for workflow scenarios):

1. Add mapping to `deploy/demo/scripts/workflow-mappings.sh`:
   ```bash
   "<scenario-name>:<image-name>"
   ```
2. Build exec + schema images:
   ```bash
   ./deploy/demo/scripts/build-demo-workflows.sh --scenario <scenario-name>
   ```
3. Push images to `quay.io/kubernaut-cicd/test-workflows/`
4. Ensure both images are **public** on the registry
5. Seed workflow via `seed_scenario_workflow` in `run.sh`

**Acceptance Criteria**:
- `workflow-schema.yaml` passes DataStorage schema validation
- `Dockerfile.exec` builds without errors
- `remediate.sh` is executable and handles failure gracefully
- Mapping exists in `workflow-mappings.sh`
- Both `<image-name>-exec:v1.0.0` and `<image-name>-schema:v1.0.0` exist in the registry

---

### FR-PLATFORM-002-07: Fault Injection Script (Conditional)

**Requirement**: Scenarios that inject a fault SHOULD provide an `inject-<fault-description>.sh` script.

**Naming convention**: `inject-<verb>-<noun>.sh` (e.g., `inject-bad-config.sh`, `inject-orphan-pvcs.sh`, `inject-deny-policy.sh`).

**Acceptance Criteria**:
- Script is executable
- Script is idempotent when possible
- Script prints what it is doing

---

### FR-PLATFORM-002-08: VHS Tape (Optional)

**Requirement**: Scenarios MAY include a VHS tape for recording terminal demos.

**Specification**:
- File: `<scenario-name>.tape`
- Derived from `deploy/demo/scenarios/template.tape`
- Three-act structure: The Problem, The Remediation, The Result
- Uses sentinel defense pattern (string concatenation + `clear`) for Wait+Screen reliability
- Uses bash comments (`# ...`) for visible annotations (not `echo`)
- Output formats: `.gif` and `.mp4`

**Acceptance Criteria**:
- Tape follows the template.tape structure
- Recording completes without manual intervention
- All sentinels use the two-defense pattern

---

### FR-PLATFORM-002-09: Display Helper Scripts (Optional)

**Requirement**: Scenarios with VHS tapes MAY include `show-*.sh` helper scripts for formatted display of pipeline state.

**Naming convention**:
- `show-alert.sh` -- display alert details from AlertManager
- `show-ai-analysis.sh` -- display RCA and workflow recommendation
- `show-effectiveness.sh` -- display EA score and explanations
- `show-approval-reason.sh` -- display why approval is required
- `show-dedup-status.sh` -- display deduplication state
- `approve-rar.sh` -- approve the RemediationApprovalRequest

---

### FR-PLATFORM-002-10: Scenario Classification

**Requirement**: Every scenario SHALL be classified as one of three types:

| Type | Description | Has `workflow/`? | Expected Outcome |
|------|-------------|------------------|------------------|
| **Workflow** | Full pipeline through WFE + EA | Yes | `RemediationSuccessful` or `RemediationFailed` |
| **No-Action** | Pipeline terminates at AA | No (intentionally) | `NoActionRequired` |
| **Behavioral** | Tests platform behavior (dedup, escalation, retry) | Varies | Scenario-specific |

Current classification (22 scenarios):
- **Workflow**: crashloop, crashloop-helm, memory-leak, stuck-rollout, slo-burn, hpa-maxed, pdb-deadlock, autoscale, pending-taint, node-notready, statefulset-pvc-failure, network-policy-block, mesh-routing-failure, gitops-drift, cert-failure, cert-failure-gitops, memory-escalation, remediation-retry (18)
- **No-Action**: disk-pressure (1)
- **Behavioral**: duplicate-alert-suppression, resource-quota-exhaustion, concurrent-cross-namespace (3)

---

## Non-Functional Requirements

### NFR-PLATFORM-002-01: Isolation

Each scenario MUST deploy into its own namespace and MUST NOT interfere with other scenarios running concurrently (unless the scenario explicitly tests cross-namespace behavior, e.g., `concurrent-cross-namespace`).

### NFR-PLATFORM-002-02: Idempotency

Both `run.sh` and `cleanup.sh` MUST be idempotent. Running them multiple times against the same cluster SHALL not produce errors or leave orphaned resources.

### NFR-PLATFORM-002-03: Self-Containment

Each scenario MUST be runnable independently. Platform and monitoring setup is handled by the shared helpers (`ensure_kind_cluster`, `ensure_monitoring_stack`, `ensure_platform`), but the scenario itself MUST NOT depend on another scenario having been run first.

---

## Dependencies

### Upstream Dependencies
- Shared helper scripts: `deploy/demo/scripts/kind-helper.sh`, `monitoring-helper.sh`, `platform-helper.sh`
- Workflow build pipeline: `deploy/demo/scripts/build-demo-workflows.sh`
- Workflow seeding: `deploy/demo/scripts/seed-workflows.sh`
- VHS tape template: `deploy/demo/scenarios/template.tape`
- Shared Dockerfile: `deploy/demo/scenarios/Dockerfile.schema`
- Kind cluster configs: `deploy/demo/scenarios/kind-config-singlenode.yaml`, `kind-config-multinode.yaml`

### Downstream Impacts
- `deploy/demo/README.md` -- scenario catalog must be updated when scenarios are added
- `deploy/demo/scripts/workflow-mappings.sh` -- must include mapping for workflow scenarios
- Container registry (`quay.io/kubernaut-cicd/test-workflows/`) -- images must be public

---

## Implementation Phases

### Phase 1: Specification (This BR)
- Define deliverables, contracts, and naming conventions
- Classify existing 22 scenarios

### Phase 2: Compliance Audit
- Audit all 22 scenarios against this specification
- File issues for compliance gaps (missing `cleanup.sh`, missing labels, stale references)

### Phase 3: Tooling (Future)
- Create a `scripts/new-scenario.sh` scaffold generator
- Add CI validation for scenario structure compliance

**Total Estimated Effort**: Phase 1: 1 day, Phase 2: 1 day, Phase 3: 2 days

---

## Success Metrics

### Scenario Consistency
- **Target**: 100% of scenarios pass the deliverables checklist
- **Measure**: Audit script or manual review

### Contributor Onboarding
- **Target**: New contributor can create a structurally compliant scenario without asking questions
- **Measure**: This BR + template.tape + deploy/demo/README.md are sufficient

---

## Alternatives Considered

### Alternative 1: Informal Wiki Page
**Approach**: Document scenario patterns in a wiki or informal guide outside the repo.
**Rejected Because**: Not version-controlled, not enforceable, diverges from repo state over time.

### Alternative 2: Cookiecutter / Scaffold Generator Only
**Approach**: Provide a `new-scenario.sh` generator without a formal specification.
**Rejected Because**: The generator would encode implicit rules; a formal spec is needed first so the generator (Phase 3) has a source of truth.

---

## Approval

**Status**: Pending
**Date**: 2026-02-23
**Decision**: Pending review
**Approved By**: --
**Related ADR**: ADR-037 (BR Template Standard), ADR-043 (Workflow Schema Format)

---

## References

### Related Business Requirements
- BR-PLATFORM-001: Platform infrastructure
- BR-SCOPE-010: RO Routing Scope Validation (namespace `kubernaut.ai/managed` label)

### Related Documents
- `deploy/demo/README.md`: Demo installation guide and scenario catalog
- `deploy/demo/scenarios/template.tape`: VHS tape template
- `deploy/demo/scripts/workflow-mappings.sh`: Scenario-to-image mapping
- `deploy/demo/scripts/build-demo-workflows.sh`: Workflow image build pipeline
- `docs/guides/user/workflow-authoring.md`: Workflow schema authoring guide
- `docs/architecture/decisions/ADR-043-workflow-schema-format.md`: Workflow schema format

### Shared Infrastructure (Not a Scenario)
- `deploy/demo/scenarios/gitops/`: Setup scripts for Gitea and ArgoCD, shared by `gitops-drift` and `cert-failure-gitops`

---

**Document Version**: 1.0
**Last Updated**: 2026-02-23
**Status**: Pending
