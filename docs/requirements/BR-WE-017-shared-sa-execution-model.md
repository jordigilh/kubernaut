# BR-WE-017: Shared ServiceAccount Execution Model (v1.1) with Per-Workflow SA Transition (v1.2)

**Business Requirement ID**: BR-WE-017
**Category**: Workflow Execution Service — Security & RBAC
**Priority**: **P0 (CRITICAL)** — Blocking production Ansible playbook execution
**Target Version**: **V1.1** (shared SA), **V1.2** (per-workflow SA via #501)
**Status**: ✅ Implemented (v1.1 shared SA)
**Date**: March 4, 2026
**Related ADRs**: ADR-044 (Engine Portability), DD-WE-002 (Dedicated Namespace), DD-WE-005 (Workflow-Scoped RBAC), DD-WE-007 (Ansible Playbook RBAC Rules)
**Related BRs**: BR-WE-015 (Ansible Execution Engine), BR-WE-014 (K8s Job Backend)
**GitHub Issues**: [#551](https://github.com/jordigilh/kubernaut/issues/551), [#552](https://github.com/jordigilh/kubernaut/issues/552), [#501](https://github.com/jordigilh/kubernaut/issues/501)

---

## Business Need

### Problem Statement

Ansible playbooks executed through AWX/AAP require Kubernetes API access to perform remediation tasks (e.g., creating Jobs for database migrations, discovering StorageClasses, checking Endpoints). The WE controller injects the controller's in-cluster SA token as an AWX credential so `kubernetes.core` Ansible modules can authenticate.

In v1.1, all workflow executions share a single ServiceAccount (`kubernaut-workflow-runner`) with a ClusterRole granting permissions to all resource types needed by any playbook. This is a pragmatic, ship-blocking requirement for v1.1 that trades blast radius for delivery speed.

### Impact Without This BR

- Ansible playbooks fail with `403 Forbidden` on any resource not in the shared ClusterRole
- Each new playbook requiring a new resource type forces a Helm chart release
- No audit trail of which playbook needs which permissions

---

## V1.1 Model: Shared ServiceAccount

**All Ansible workflow executions authenticate to the Kubernetes API using the `kubernaut-workflow-runner` ServiceAccount**, whose ClusterRole accumulates permissions for all production playbooks.

### Architecture

```
AWX/AAP Execution Environment (pod)
  └── kubeconfig injected by WE controller (#552)
      └── authenticates as: kubernaut-workflow-runner SA
          └── ClusterRole: union of all playbook permissions
              ├── apps: deployments, statefulsets, daemonsets (get, list, patch, update)
              ├── core: pods, configmaps, nodes, services, secrets, PVCs, endpoints ...
              ├── kubernaut.ai: workflowexecutions (get)
              ├── storage.k8s.io: storageclasses (get, list)
              ├── batch: jobs (get, list, create, delete)
              ├── ... (see DD-WE-007 for full list)
              └── [Ansible playbook requirements (#551)]
```

### Success Criteria (v1.1)

1. The `kubernaut-workflow-runner` ClusterRole includes all RBAC rules required by the 2 production Ansible playbooks (`gitops-migrate-postgres-emptydir-to-pvc.yml`, `gitops-update-memory-limits.yml`)
2. New playbooks requiring new resource types are handled by adding rules to the ClusterRole in the Helm chart (manual process, documented in playbook development guide)
3. The WE controller injects credentials using kubeconfig-file injection (#552), which takes precedence over in-cluster config in AAP execution environments

### Known Limitations (v1.1)

| Limitation | Impact | Accepted Risk |
|------------|--------|---------------|
| All playbooks share identical permissions | Medium — blast radius is full cluster for supported resource types | Accepted for v1.1 delivery. Mitigated in v1.2 by per-workflow SA. |
| Permission creep as playbooks grow | Low — only 2 production playbooks in v1.1 | Will not scale; DD-WE-005 addresses this in v1.2. |
| No per-execution audit of intended vs actual permissions | Medium — security reviews cannot verify least-privilege per workflow | DD-WE-005 adds schema-declared RBAC for auditability in v1.2. |
| ClusterRole is cluster-scoped | Medium — Job create/delete in any namespace | Consistent with existing model (PVC, Secret create/delete already cluster-wide). |

---

## V1.2 Transition: Per-Workflow ServiceAccount (#501)

**In v1.2, each workflow execution will run with a dedicated, short-lived ServiceAccount scoped to the permissions declared in the workflow schema.** This replaces the shared ClusterRole entirely.

### Architecture (v1.2 — planned)

```
WFE Created
  ├── 1. Read rbac.rules from workflow schema (DD-WE-005)
  ├── 2. Create SA "wfe-<hash>" in kubernaut-workflows
  ├── 3. Create scoped Role/ClusterRole with ONLY declared rules
  ├── 4. Create RoleBinding/ClusterRoleBinding
  ├── 5. TokenRequest API → short-lived token for SA
  ├── 6. Inject token as AWX credential (kubeconfig)
  ├── 7. Launch AWX Job with scoped token
  │  ... workflow executes ...
  ├── 8. Cleanup: delete SA, Role, RoleBinding
  └── WFE Completed/Failed
```

### Success Criteria (v1.2 — planned)

1. Each workflow execution runs with a dedicated SA whose permissions match the workflow schema's `rbac.rules`
2. The shared `kubernaut-workflow-runner` ClusterRole is deprecated (kept as fallback for schemas without `rbac`)
3. TokenRequest API provides short-lived tokens (default: 1h) that are not refreshable
4. Security audits can verify per-workflow least-privilege from the schema catalog

### Tracking

- **Issue**: [#501](https://github.com/jordigilh/kubernaut/issues/501) — Per-workflow SA token via TokenRequest
- **Design Decision**: [DD-WE-005](../architecture/decisions/DD-WE-005-workflow-scoped-rbac.md) — Schema-declared RBAC

---

## Implementation History

| Version | Date | Issue | Change |
|---------|------|-------|--------|
| v1.1 | 2026-03-04 | #551 | Added 4 RBAC rules to `kubernaut-workflow-runner` ClusterRole for production Ansible playbooks (DD-WE-007) |
| v1.1 | 2026-03-04 | #552 | Fixed credential type injection: kubeconfig-file replaces env-var injection to override in-cluster config in AAP pods |
| v1.2 | Planned | #501 | Per-workflow SA via TokenRequest, schema-declared RBAC (DD-WE-005) |

---

## Related Documents

- [DD-WE-002: Dedicated Execution Namespace](../architecture/decisions/DD-WE-002-dedicated-execution-namespace.md)
- [DD-WE-005: Workflow-Scoped RBAC](../architecture/decisions/DD-WE-005-workflow-scoped-rbac.md)
- [DD-WE-007: Ansible Playbook RBAC Rules](../architecture/decisions/DD-WE-007-ansible-playbook-rbac-rules.md)
- [BR-WE-015: Ansible Execution Engine](./BR-WE-015-ansible-execution-engine.md)

---

**Document Version**: 1.0
**Last Updated**: March 4, 2026
**Maintained By**: Kubernaut Architecture Team
