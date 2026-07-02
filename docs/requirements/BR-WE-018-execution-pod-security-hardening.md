# BR-WE-018: Execution Pod Security Hardening

**Business Requirement ID**: BR-WE-018
**Category**: Workflow Execution Service — Security & Compliance
**Priority**: **P1 (HIGH)** — Closes a HIGH-severity GA readiness gap
**Target Version**: **V1.5**
**Status**: ✅ Implemented
**Date**: June 30, 2026
**Related ADRs**: None
**Related BRs**: BR-WE-014 (Kubernetes Job Execution Backend)
**GitHub Issues**: [#1505](https://github.com/jordigilh/kubernaut/issues/1505) (GAP-03)

---

## Business Need

### Problem Statement

The WorkflowExecution (WE) controller's own pod runs under a restricted `SecurityContext` (`runAsNonRoot`, `seccompProfile: RuntimeDefault`, `readOnlyRootFilesystem`, dropped capabilities — see `docs/services/crd-controllers/03-workflowexecution/security-configuration.md`). However, the pods the controller *spawns* to actually execute remediation workflows — Kubernetes Jobs (`JobExecutor.buildJob`) and Tekton PipelineRuns (`TektonExecutor.BuildPipelineRun`) — set no `SecurityContext` at all. This is an inconsistency: the component doing the actual remediation work (with cluster-write RBAC) is less hardened than the controller orchestrating it.

This was identified as **GAP-03 (HIGH severity)** in the GA Readiness Audit (issue #1505).

### Impact Without This BR

- Spawned execution pods can run as root, with privilege escalation allowed, a writable root filesystem, and the full default Linux capability set — a materially larger attack surface than necessary for a pod that exists only to apply a Kubernetes patch or run a scripted remediation step.
- Inconsistent with **FedRAMP AC-6** (Least Privilege) and **CM-7** (Least Functionality): the system does not uniformly enforce minimal privilege across all pods it creates.
- A compromised or malicious workflow image (supply-chain risk, complementary to GAP-01's image-signing work) has more room to escalate or persist if the pod it runs in is unrestricted.

---

## Decision: Hardcoded Restricted Profile (No Configurability)

**All spawned Job and Tekton execution pods are unconditionally hardened to the Kubernetes "restricted" Pod Security Standard.** There is no CRD field or other mechanism to opt out to a relaxed ("baseline") profile.

This was a deliberate choice, not an oversight:

1. **No demonstrated need**: existing workflow catalog images already run as non-root (`USER 1001`, see `test/fixtures/workflows/*/Dockerfile`) and do no filesystem writes outside of scratch space. Adding an unused escape-hatch field would be speculative (YAGNI) per the Go Anti-Pattern Checklist.
2. **AI-driven CR creation risk**: `WorkflowExecution` specs are populated by an AI/LLM-driven selection pipeline. A configurable `securityProfile` field would be a lever a compromised or malicious workflow/prompt-injection could manipulate to escape the restricted sandbox. Removing the field entirely closes that vector — any future change to pod hardening requires a reviewed code change (PR), not a runtime configuration value.
3. **Simpler compliance posture**: a fixed, code-enforced profile is materially easier to assess for FedRAMP/SOC2 than a configurable one, which would require verifying configuration state in every environment rather than verifying a single code path once.

### Asymmetric Hardening: Job vs. Tekton

| Backend | Pod-level `SecurityContext` | Container-level `SecurityContext` |
|---|---|---|
| Kubernetes Job | ✅ Applied | ✅ Applied |
| Tekton PipelineRun | ✅ Applied (via `TaskRunTemplate.PodTemplate.SecurityContext`) | ❌ Not applicable |

This asymmetry is an accepted, API-level constraint, not a gap: Tekton's `PipelineRunSpec.TaskRunTemplate.PodTemplate` (`github.com/tektoncd/pipeline/pkg/apis/pipeline/pod.PodTemplate`, v1.13.1) exposes only `SecurityContext *corev1.PodSecurityContext` (pod-level). Container-level settings belong to the `Task` spec resolved from the OCI bundle at execution time, which is outside the WE controller's authoring control. Pod-level hardening (`runAsNonRoot`, `seccompProfile: RuntimeDefault`) is still enforced for Tekton-executed workflows.

### Restricted Profile Applied

Pod-level (`Job` + `Tekton`):
- `runAsNonRoot: true`
- `seccompProfile.type: RuntimeDefault`

Container-level (`Job` only — the `"workflow"` container):
- `allowPrivilegeEscalation: false`
- `readOnlyRootFilesystem: true`
- `runAsNonRoot: true`
- `capabilities.drop: [ALL]`

A `tmp` `emptyDir` volume is mounted at `/tmp` (with `HOME=/tmp`, `TMPDIR=/tmp`) on the Job container so tools that expect writable scratch space (e.g. `kubectl`'s discovery cache) continue to function under `readOnlyRootFilesystem: true`.

This mirrors the existing `kubernaut.podSecurityContext` / `kubernaut.containerSecurityContext` Helm helpers (`charts/kubernaut/templates/_helpers.tpl`) already applied to the WE controller's own pod, so the spawned-pod profile is consistent with the controller's own posture.

---

## Defense in Depth: Pod Security Admission Backstop

In addition to the controller authoring the `SecurityContext` in code, the `kubernaut-workflows` namespace is labeled with the Kubernetes built-in Pod Security Admission (PSA) `restricted` standard:

```yaml
labels:
  pod-security.kubernetes.io/enforce: restricted
  pod-security.kubernetes.io/audit: restricted
  pod-security.kubernetes.io/warn: restricted
```

Since every pod built per this BR already satisfies `restricted`, this is a no-op under normal operation. Its purpose is to provide an independent, API-server-enforced backstop: even if a future code change in `buildJob`/`BuildPipelineRun` regressed the `SecurityContext`, the API server would still reject the resulting pod at admission time, rather than relying solely on the correctness of the Go code path.

This same hardening is tracked as a follow-up for the `kubernaut-operator` repository (which constructs its own `Namespace` object independently of this Helm chart), targeted for milestone v1.6.

---

## Success Criteria

1. `JobExecutor.buildJob()` sets the documented pod-level and container-level `SecurityContext`, plus the `/tmp` scratch volume and `HOME`/`TMPDIR` env vars.
2. `TektonExecutor.BuildPipelineRun()` sets the documented pod-level `SecurityContext` via `TaskRunTemplate.PodTemplate`.
3. The `kubernaut-workflows` Namespace (Helm-managed) carries the PSA `restricted` labels.
4. Unit tests assert the exact `SecurityContext` field values for both backends (no `BeNil()`/`BeEmpty()`-style weak assertions, per `.golangci.yml` `forbidigo` guidance).
5. FedRAMP/SOC2 control mapping: **AC-6** (Least Privilege), **CM-7** (Least Functionality) — both satisfied by a structural (code-enforced + admission-enforced), non-configurable guarantee.

---

## Related Documents

- [BR-WE-014: Kubernetes Job Execution Backend](./BR-WE-014-kubernetes-job-execution-backend.md)
- [WorkflowExecution Security Configuration](../services/crd-controllers/03-workflowexecution/security-configuration.md)

---

**Document Version**: 1.0
**Last Updated**: June 30, 2026
**Maintained By**: Kubernaut Architecture Team
