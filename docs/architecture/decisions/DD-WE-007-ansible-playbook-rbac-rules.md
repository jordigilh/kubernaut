# DD-WE-007: Ansible Playbook RBAC Rules for kubernaut-workflow-runner

**Version**: 1.0
**Date**: 2026-03-04
**Status**: ✅ APPROVED
**Author**: WorkflowExecution Team
**Priority**: P0 (Blocking)

---

## Context

The `kubernaut-workflow-runner` ClusterRole (Helm chart: `charts/kubernaut/templates/workflowexecution/workflowexecution.yaml`) grants RBAC permissions to the shared ServiceAccount used by all Ansible playbook executions via AWX/AAP.

During E2E testing of the `disk-pressure-emptydir` scenario on OCP with AAP (`v1.1.0-rc12`), the playbook passed 10/13 discovery tasks before hitting `403 Forbidden` on `workflowexecutions.kubernaut.ai`. An audit of all 6 Ansible playbooks in `kubernaut-test-playbooks` revealed 4 missing resource rules.

### Dependency on #552

These RBAC rules apply to the `kubernaut-workflow-runner` SA. If #552's credential type bug causes AAP to use in-cluster config (wrong SA), these rules are moot. Both #551 and #552 must land together in v1.1.

---

## Decision

**Add 4 new RBAC rules to the `kubernaut-workflow-runner` ClusterRole for resources required by the production Ansible playbooks.**

This is a v1.1 interim fix. In v1.2 (#501, DD-WE-005), per-workflow SA scoping will replace the shared ClusterRole with schema-declared RBAC rules per execution.

---

## Rules Added

| API Group | Resources | Verbs | Used By | Justification |
|-----------|-----------|-------|---------|---------------|
| `kubernaut.ai` | `workflowexecutions` | `get` | Both production playbooks | Read-only: extract ownerReferences for RR correlation |
| `storage.k8s.io` | `storageclasses` | `get`, `list` | `gitops-migrate-postgres-emptydir-to-pvc.yml` | Read-only: discover default StorageClass |
| _(core)_ | `endpoints` | `get`, `list` | `gitops-migrate-postgres-emptydir-to-pvc.yml` | Read-only: check if postgres pod is alive |
| `batch` | `jobs` | `get`, `list`, `create`, `delete` | `gitops-migrate-postgres-emptydir-to-pvc.yml` | pg_dump, verify, pg_restore Jobs lifecycle |

### Security note

`batch/v1 Job` create/delete is cluster-scoped via ClusterRole. This is consistent with the existing privilege model (PVC create/delete, Secret create/delete, Pod delete are already cluster-wide). The workflow runner is intentionally cross-namespace for remediation. In v1.2 (#501), per-workflow SA scoping (DD-WE-005) can tighten this.

---

## Playbook Audit

Audited all 6 playbooks in `kubernaut-test-playbooks`:

| Playbook | K8s API Calls | New Rules Needed |
|----------|---------------|-----------------|
| `gitops-migrate-postgres-emptydir-to-pvc.yml` | Yes: WFE, SC, Endpoints, Jobs | All 4 rules |
| `gitops-update-memory-limits.yml` | Yes: WFE (ownerRef lookup) | `workflowexecutions get` only |
| `test-success.yml` | No K8s API calls | None |
| `test-failure.yml` | No K8s API calls | None |
| `test-dep-configmap.yml` | No K8s API calls | None |
| `test-dep-secret.yml` | No K8s API calls | None |

---

## Implementation

**File changed**: `charts/kubernaut/templates/workflowexecution/workflowexecution.yaml`

4 new rule blocks added to the `kubernaut-workflow-runner` ClusterRole, marked with comment `# ── Ansible playbook requirements (#551)`.

---

## Risks

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Cluster-scoped Job create/delete | Medium | Low | Consistent with existing privilege model (PVC, Secret create/delete already cluster-wide). V1.2 (#501) scopes per-workflow. |
| No RBAC audit process for new playbooks | Low | Medium | Recommend adding RBAC audit step to playbook development guide. |
| Dependency on #552 | High | Certain | Both issues must land together. #552 ensures correct SA is used; #551 ensures SA has correct permissions. |

---

## Related Documents

- [DD-WE-005: Workflow-Scoped RBAC](./DD-WE-005-workflow-scoped-rbac.md) — v1.2 replacement for shared ClusterRole
- [DD-WE-002: Dedicated Execution Namespace](./DD-WE-002-dedicated-execution-namespace.md) — Establishes `kubernaut-workflows` namespace
- [BR-WE-017: Shared SA Model (v1.1) → Per-Workflow SA (v1.2)](../requirements/BR-WE-017-shared-sa-execution-model.md) — Business requirement for SA transition
- [Issue #551](https://github.com/jordigilh/kubernaut/issues/551) — RBAC rules bug
- [Issue #552](https://github.com/jordigilh/kubernaut/issues/552) — Credential type injection bug (dependency)
- [Issue #501](https://github.com/jordigilh/kubernaut/issues/501) — Per-workflow SA via TokenRequest (v1.2)

---

## Document Maintenance

| Date | Version | Changes |
|------|---------|---------|
| 2026-03-04 | 1.0 | Initial: 4 RBAC rules added for production Ansible playbooks |
