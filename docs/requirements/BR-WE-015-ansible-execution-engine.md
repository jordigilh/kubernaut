# BR-WE-015: Ansible Execution Engine Backend (AWX/AAP)

**Business Requirement ID**: BR-WE-015
**Category**: Workflow Engine Service
**Priority**: **P1 (HIGH)** - Enterprise Execution Backend
**Target Version**: **V1.0**
**Status**: Active
**Date**: March 2, 2026
**Related ADRs**: ADR-043 (Execution Engine Schema), ADR-044 (Engine Portability)
**Related BRs**: BR-WE-014 (K8s Job Backend), BR-WE-016 (EngineConfig Discriminator), BR-WORKFLOW-004 (Schema Format)
**GitHub Issue**: [#45](https://github.com/jordigilh/kubernaut/issues/45)

---

## Business Need

### Problem Statement

Organizations running Red Hat Ansible Automation Platform (AAP) or AWX have existing investments in Ansible playbooks, inventories, credentials, and RBAC. Kubernaut currently supports only Tekton PipelineRuns and Kubernetes Jobs as execution backends, forcing these organizations to repackage their Ansible playbooks into OCI container images — losing integration with AAP's credential management, inventory systems, and execution audit trail.

### Impact Without This BR

- Organizations with existing AAP/AWX infrastructure cannot leverage their Ansible investments through Kubernaut
- Remediation playbooks must be containerized even when they are already maintained as Ansible Projects in AWX
- No visibility into task-level execution details that AWX provides (stdout, artifacts, task timing)
- The `ansible` engine value in the workflow schema (ADR-043) remains unused

---

## Business Objective

**WorkflowExecution Controller SHALL support Ansible (AWX/AAP) as a third execution backend, using the AWX REST API to launch Job Templates and track execution status.**

### Success Criteria

1. WorkflowExecution CRD accepts `spec.executionEngine: "ansible"` and dispatches to `AnsibleExecutor`
2. `AnsibleExecutor` implements the `Executor` interface (`Create`, `GetStatus`, `Cleanup`, `Engine`)
3. `Create` launches an AWX Job Template via `POST /api/v2/job_templates/{id}/launch/` with workflow parameters as `extra_vars`
4. `GetStatus` polls AWX job status and maps AWX states to WFE phases:
   - AWX `pending`/`waiting` -> WFE `Pending`
   - AWX `running` -> WFE `Running`
   - AWX `successful` -> WFE `Completed`
   - AWX `failed`/`error`/`canceled` -> WFE `Failed`
5. `Cleanup` handles AWX job cancellation when WFE is deleted
6. AWX connection configuration (API URL, auth token) is in WE controller config, not in workflow schemas
7. Workflow parameters (`map[string]string`) are converted to typed AWX `extra_vars` JSON with correct type coercion
8. Resource locking (BR-WE-009), audit trail (BR-WE-005), and failure details (BR-WE-003) apply to Ansible executions identically to Tekton and Job backends

---

## Use Cases

### Use Case 1: Ansible Playbook Remediation via AWX

**Scenario**: A deployment enters CrashLoopBackOff. Kubernaut detects the event, analyzes it, and selects an Ansible rollback playbook maintained as an AWX Project.

```
1. Signal: Pod CrashLoopBackOff detected
2. Gateway -> SP -> RO -> AIAnalysis selects workflow with engine: ansible
3. RO creates WorkflowExecution CRD:
   spec:
     executionEngine: "ansible"
     workflowRef:
       workflowId: crashloop-rollback-ansible
       executionBundle: "https://github.com/org/playbooks.git"
       executionBundleDigest: "a1b2c3d4"
       engineConfig:
         playbookPath: "playbooks/rollback-deployment.yml"
         inventoryName: "k8s-inventory"
4. WE Controller dispatches to AnsibleExecutor
5. AnsibleExecutor:
   a. Reads engineConfig from WFE CRD
   b. Finds/creates AWX Project pointing to Git repo
   c. Finds/creates AWX Job Template with playbook path + inventory
   d. Launches job with parameters as extra_vars
   e. Polls job status until terminal state
6. WFE transitions: Pending -> Running -> Completed
7. RO creates EffectivenessAssessment
```

### Use Case 2: AWX Job Template by Name

**Scenario**: An operator pre-configures AWX Job Templates and references them by name in workflow schemas.

```yaml
execution:
  engine: ansible
  bundle: https://github.com/org/playbooks.git
  bundleDigest: a1b2c3d4
  engineConfig:
    playbookPath: playbooks/rollback-deployment.yml
    jobTemplateName: "kubernaut-rollback"
```

When `jobTemplateName` is set, the executor launches the existing template directly instead of creating one ad-hoc.

---

## Technical Requirements

### TR-1: AnsibleExecutor Implementation

The `AnsibleExecutor` SHALL implement the `Executor` interface defined in `pkg/workflowexecution/executor/executor.go`:

- `Engine() string` — returns `"ansible"`
- `Create(ctx, wfe, namespace) error` — launches AWX job
- `GetStatus(ctx, wfe, namespace) (ExecutionStatus, error)` — polls AWX job status
- `Cleanup(ctx, wfe, namespace) error` — cancels AWX job if running

### TR-2: AWX REST API Client

The executor SHALL use the AWX REST API v2:

- `POST /api/v2/job_templates/{id}/launch/` — launch job with `extra_vars`
- `GET /api/v2/jobs/{id}/` — poll job status
- `POST /api/v2/jobs/{id}/cancel/` — cancel running job
- `GET /api/v2/projects/` — list/find projects
- `POST /api/v2/projects/` — create project from Git repo

Authentication via Bearer token from WE controller config.

### TR-3: Extra Vars Conversion

Workflow parameters (`map[string]string`) SHALL be converted to typed `extra_vars` JSON:

```go
// "3" -> 3 (integer), "true" -> true (boolean), "[1,2]" -> [1,2] (array)
// Plain strings remain strings
```

### TR-4: WE Controller Configuration

```yaml
ansible:
  apiURL: "https://awx.example.com"
  tokenSecretRef:
    name: awx-credentials
    key: token
```

### TR-5: Error Handling

- Transient AWX API failures (network, 5xx) SHALL be retried with exponential backoff
- AWX authentication failures (401/403) SHALL be reported as non-retryable errors
- AWX job failures SHALL populate `FailureDetails` in WFE status

---

## Acceptance Criteria

```gherkin
Given a WorkflowExecution CRD with executionEngine "ansible" and valid engineConfig
When the WE controller reconciles the CRD
Then the AnsibleExecutor launches an AWX job with correct extra_vars
And the WFE phase transitions from Pending to Running to Completed
And the audit trail captures execution start, completion, and duration

Given a WorkflowExecution CRD with executionEngine "ansible"
When the AWX job fails
Then the WFE phase transitions to Failed
And FailureDetails contains the AWX failure reason and message

Given a WorkflowExecution CRD with executionEngine "ansible"
When the AWX API is unreachable
Then the executor retries with exponential backoff
And reports a transient error after retry exhaustion
```

---

## Dependencies

- **BR-WE-014**: Executor interface and Strategy pattern (prerequisite, landed)
- **BR-WE-016**: EngineConfig discriminator pattern (co-requisite)
- **BR-WORKFLOW-004**: Workflow schema format (engine and engineConfig fields)
- **#44**: K8s Job execution backend (prerequisite, landed)

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-02 | Initial BR |
