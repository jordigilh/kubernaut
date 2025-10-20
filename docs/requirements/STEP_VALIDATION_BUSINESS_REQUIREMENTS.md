# Step Validation Business Requirements

**Version**: 1.0
**Status**: ✅ Approved
**Design Decision**: [DD-002 - Per-Step Validation Framework](../architecture/DESIGN_DECISIONS.md#dd-002-per-step-validation-framework-alternative-2)
**Last Updated**: 2025-10-14

---

## Overview

This document defines business requirements for per-step precondition and postcondition validation in the remediation workflow. These requirements extend the existing workflow-level validation (BR-WF-015, BR-WF-016, BR-WF-050, BR-WF-051) to provide defense-in-depth validation at the step level.

**Business Context**:
- Current remediation effectiveness: 70%
- Target remediation effectiveness: 85-90% (+15-20%)
- Current cascade failure rate: 30%
- Target cascade failure rate: <10% (-20%)
- Current MTTR (failed remediation): 15 minutes
- Target MTTR: 8 minutes (-47%)

**Architectural Foundation**: [DD-002 - Per-Step Validation Framework](../architecture/DESIGN_DECISIONS.md#dd-002-per-step-validation-framework-alternative-2)

---

## WorkflowExecution Service Requirements

### BR-WF-016: Step Precondition Validation

**Category**: Workflow Validation
**Priority**: P1 (High Value)
**Service**: WorkflowExecution
**Design Decision**: DD-002 Alternative 2

**Description**:
The WorkflowExecution service MUST validate preconditions before executing each workflow step. Preconditions define the expected cluster state that must be true before a step can safely execute.

**Rationale**:
Prevent cascade failures where steps execute based on invalid state assumptions from previous steps. For example, if Step 2 "scale deployment from 3 to 5 replicas" assumes Step 1 successfully set replicas to 3, but Step 1 failed silently, the precondition check will halt execution before making invalid changes.

**Acceptance Criteria**:
1. MUST evaluate all `step.preConditions[]` before creating KubernetesExecution CRD for that step
2. MUST use Rego policy engine for condition evaluation (reuse BR-REGO-001 to BR-REGO-010)
3. MUST query current cluster state for condition input (e.g., deployment current replicas, pod status)
4. MUST block step execution if `condition.required=true` and condition fails
5. MUST log warning if `condition.required=false` and condition fails (non-blocking)
6. MUST record condition evaluation results in `status.stepStatuses[n].preConditionResults[]`
7. MUST include failure reason in status when precondition blocks execution
8. MUST support condition timeout for async checks (default 30 seconds)

**Integration Points**:
- **Rego Policy Engine**: Reuses existing policy evaluation infrastructure (BR-REGO-001)
- **Kubernetes API**: Queries cluster state for condition input
- **RemediationOrchestrator**: Receives status updates when precondition fails
- **KubernetesExecutor**: Only created after preconditions pass

**Example**:
```yaml
# WorkflowStep with preconditions
steps:
  - stepNumber: 2
    name: "Scale web deployment"
    action: "scale_deployment"
    parameters:
      deployment: "web-app"
      namespace: "production"
      replicas: 5
    preConditions:
      - type: deployment_exists
        description: "Deployment must exist before scaling"
        rego: |
          package precondition
          import future.keywords.if
          allow if { input.deployment_found == true }
        required: true
        timeout: "10s"

      - type: current_replicas_match
        description: "Current replicas should match baseline"
        rego: |
          package precondition
          import future.keywords.if
          allow if { input.current_replicas == 3 }
        required: false  # warning only, not blocking
        timeout: "5s"

# Status after evaluation
status:
  stepStatuses:
    - stepNumber: 2
      preConditionResults:
        - conditionType: deployment_exists
          evaluated: true
          passed: true
          evaluationTime: "2025-10-14T10:00:00Z"
        - conditionType: current_replicas_match
          evaluated: true
          passed: false
          errorMessage: "Current replicas: 1, expected: 3"
          evaluationTime: "2025-10-14T10:00:01Z"
      status: "blocked"
      errorMessage: "Precondition 'deployment_exists' required but not met"
```

**Testing Requirements**:
- Unit: Condition evaluation logic with various Rego policies
- Integration: Real Kubernetes cluster state queries
- E2E: Complete workflow with precondition failure scenarios

**Related BRs**:
- BR-WF-015: Workflow-level safety validation (parent requirement)
- BR-WF-053: Condition policy management (ConfigMap-based policies)
- BR-REGO-001 to BR-REGO-010: Rego policy evaluation

---

### BR-WF-052: Step Postcondition Verification

**Category**: Workflow Validation
**Priority**: P1 (High Value)
**Service**: WorkflowExecution
**Design Decision**: DD-002 Alternative 2

**Description**:
The WorkflowExecution service MUST verify postconditions after each workflow step completes. Postconditions define the expected cluster state that must be true for a step to be considered successful.

**Rationale**:
Verify that step execution achieved its intended effect. For example, after "scale deployment to 5 replicas", verify that 5 pods are actually running (not just that the kubectl command succeeded). This prevents "successful" workflows that didn't actually remediate the issue.

**Acceptance Criteria**:
1. MUST evaluate all `step.postConditions[]` after KubernetesExecution completes
2. MUST use Rego policy engine for condition evaluation
3. MUST query cluster state after step execution for verification
4. MUST mark step as failed if `condition.required=true` and postcondition fails
5. MUST log warning if `condition.required=false` and postcondition fails
6. MUST support async verification with configurable timeout (default 2 minutes)
7. MUST wait up to `condition.timeout` for state to converge (e.g., pods starting)
8. MUST record verification results in `status.stepStatuses[n].postConditionResults[]`
9. MUST trigger rollback if step marked failed and `rollbackStrategy=automatic`

**Integration Points**:
- **KubernetesExecutor**: Watches for completion before evaluating postconditions
- **Kubernetes API**: Queries cluster state for verification
- **RemediationOrchestrator**: Receives status updates when postcondition fails
- **Rollback Logic**: Triggered when required postcondition fails

**Example**:
```yaml
# WorkflowStep with postconditions
steps:
  - stepNumber: 2
    name: "Scale web deployment"
    action: "scale_deployment"
    parameters:
      deployment: "web-app"
      namespace: "production"
      replicas: 5
    postConditions:
      - type: desired_replicas_running
        description: "Desired replica count must be running"
        rego: |
          package postcondition
          import future.keywords.if
          allow if {
            input.running_pods >= input.target_replicas
            input.deployment_available == true
          }
        required: true
        timeout: "2m"  # wait for pods to start

      - type: no_crashloop_pods
        description: "No pods should be in CrashLoopBackOff"
        rego: |
          package postcondition
          import future.keywords.if
          allow if { count([p | p := input.pods[_]; p.status == "CrashLoopBackOff"]) == 0 }
        required: true
        timeout: "1m"

# Status after verification
status:
  stepStatuses:
    - stepNumber: 2
      postConditionResults:
        - conditionType: desired_replicas_running
          evaluated: true
          passed: true
          evaluationTime: "2025-10-14T10:02:30Z"
        - conditionType: no_crashloop_pods
          evaluated: true
          passed: true
          evaluationTime: "2025-10-14T10:02:31Z"
      status: "completed"
```

**Testing Requirements**:
- Unit: Async verification with timeout handling
- Integration: Real pod startup and convergence scenarios
- E2E: Postcondition failure triggering rollback

**Related BRs**:
- BR-WF-051: Workflow-level success validation (parent requirement)
- BR-WF-053: Condition policy management
- BR-WF-050: Workflow rollback handling

---

### BR-WF-053: Condition Policy Management

**Category**: Workflow Configuration
**Priority**: P2 (Medium Value)
**Service**: WorkflowExecution
**Design Decision**: DD-002 Alternative 2

**Description**:
The WorkflowExecution service MUST support centralized management of condition policies via ConfigMap, allowing operations teams to update validation logic without code changes.

**Rationale**:
Enable iterative tuning of condition strictness based on production telemetry. Initial conditions can be lenient (required=false) and gradually tightened (required=true) as confidence increases, without requiring service redeployment.

**Acceptance Criteria**:
1. MUST load condition policies from ConfigMap (`kubernaut-workflow-conditions`)
2. MUST support policy versioning in ConfigMap metadata
3. MUST reload policies when ConfigMap is updated (watch-based or periodic)
4. MUST provide default condition templates for common actions (scale_deployment, restart_pod, etc.)
5. MUST support policy override per workflow execution via `spec.conditionOverrides`
6. MUST validate Rego policy syntax before applying
7. MUST gracefully degrade if policy loading fails (use embedded defaults)
8. MUST log policy updates and validation errors

**Integration Points**:
- **ConfigMap**: Source of truth for condition policies
- **Rego Policy Engine**: Evaluates policies from ConfigMap
- **Kubernetes API**: Watches ConfigMap for updates

**Example**:
```yaml
# ConfigMap with condition policies
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-workflow-conditions
  namespace: kubernaut-system
  labels:
    version: "1.2.0"
data:
  scale_deployment_preconditions.rego: |
    package precondition.scale_deployment
    import future.keywords.if

    deployment_exists if {
      input.deployment_found == true
    }

    current_replicas_match if {
      input.current_replicas == input.expected_baseline
    }

  scale_deployment_postconditions.rego: |
    package postcondition.scale_deployment
    import future.keywords.if

    desired_replicas_running if {
      input.running_pods >= input.target_replicas
      input.deployment_available == true
    }

# Workflow with policy override
spec:
  workflowDefinition:
    steps:
      - stepNumber: 1
        action: "scale_deployment"
        preConditions:
          - type: deployment_exists
            rego: "kubernaut-workflow-conditions:scale_deployment_preconditions.rego:deployment_exists"
            required: true
  conditionOverrides:
    # Override default policy for this execution
    scale_deployment:
      preconditions:
        deployment_exists:
          required: false  # make it a warning for this workflow
```

**Testing Requirements**:
- Unit: ConfigMap parsing and validation
- Integration: Policy reload and hot-update scenarios
- E2E: Policy override in workflow execution

**Related BRs**:
- BR-WF-016: Step precondition validation (uses policies from ConfigMap)
- BR-WF-052: Step postcondition verification (uses policies from ConfigMap)
- BR-REGO-001: Rego policy evaluation

---

## KubernetesExecutor Service Requirements

### BR-EXEC-016: Action Precondition Framework

**Category**: Action Validation
**Priority**: P1 (High Value)
**Service**: KubernetesExecutor
**Design Decision**: DD-002 Alternative 2

**Description**:
The KubernetesExecutor service MUST validate action-specific preconditions before creating Kubernetes Jobs for action execution. This provides an additional validation layer beyond WorkflowExecution step preconditions.

**Rationale**:
Provide action-specific validation that's more detailed than workflow-level checks. For example, WorkflowExecution might check "deployment exists", but KubernetesExecutor can check "deployment has valid image pull secrets" before attempting to scale.

**Acceptance Criteria**:
1. MUST evaluate all `spec.preConditions[]` during validating phase (after parameter validation, before Job creation)
2. MUST use existing Rego policy engine (BR-REGO-001 to BR-REGO-010)
3. MUST integrate with existing dry-run validation (BR-EXEC-059, BR-EXEC-060)
4. MUST support action-specific condition types (resource_state, RBAC_permissions, capacity_check)
5. MUST block Job creation if required precondition fails
6. MUST record precondition results in `status.validationResults.preConditionResults[]`
7. MUST provide detailed failure reason for troubleshooting

**Integration Points**:
- **Existing Validation**: Extends current 5-step validation phase (BR-EXEC-001 to BR-EXEC-015)
- **Dry-Run Validation**: Preconditions evaluated before dry-run (BR-EXEC-059)
- **Rego Policy Engine**: Reuses existing policy infrastructure
- **WorkflowExecution**: Receives status updates when precondition blocks execution

**Example**:
```yaml
# KubernetesExecution with action preconditions
apiVersion: execution.kubernaut.io/v1alpha1
kind: KubernetesExecution
metadata:
  name: scale-web-app-step-2
spec:
  action: "scale_deployment"
  parameters:
    deployment: "web-app"
    namespace: "production"
    replicas: 5
  preConditions:
    - type: sufficient_cluster_capacity
      description: "Cluster must have capacity for additional pods"
      rego: |
        package precondition
        import future.keywords.if
        allow if {
          input.available_cpu >= input.required_cpu * input.new_replicas
          input.available_memory >= input.required_memory * input.new_replicas
        }
      required: true
      timeout: "10s"

    - type: image_pull_secrets_valid
      description: "Deployment must have valid image pull secrets"
      rego: |
        package precondition
        import future.keywords.if
        allow if { count(input.invalid_secrets) == 0 }
      required: true
      timeout: "5s"

status:
  phase: failed
  validationResults:
    preConditionResults:
      - conditionType: sufficient_cluster_capacity
        evaluated: true
        passed: false
        errorMessage: "Insufficient CPU: need 2 cores, have 0.5 cores available"
        evaluationTime: "2025-10-14T10:00:15Z"
```

**Testing Requirements**:
- Unit: Condition evaluation with various cluster states
- Integration: Real cluster capacity checks
- E2E: Precondition blocking Job creation

**Related BRs**:
- BR-EXEC-001 to BR-EXEC-015: Existing validation phase (preconditions extend this)
- BR-EXEC-059: Dry-run validation (preconditions evaluated first)
- BR-REGO-001 to BR-REGO-010: Rego policy evaluation

---

### BR-EXEC-036: Action Postcondition Verification

**Category**: Action Validation
**Priority**: P1 (High Value)
**Service**: KubernetesExecutor
**Design Decision**: DD-002 Alternative 2

**Description**:
The KubernetesExecutor service MUST verify action-specific postconditions after Kubernetes Job completes. This confirms that the action achieved its intended effect on cluster state.

**Rationale**:
Kubernetes Jobs can succeed even if the desired outcome wasn't achieved (e.g., `kubectl scale` succeeds but pods don't start). Postcondition verification catches these scenarios and marks the execution as failed, triggering appropriate workflow handling.

**Acceptance Criteria**:
1. MUST evaluate all `spec.postConditions[]` after `Job.status.succeeded = 1`
2. MUST query cluster state to validate intended outcome
3. MUST support async verification with configurable timeout (default 2 minutes)
4. MUST wait up to `condition.timeout` for state convergence
5. MUST mark execution as failed if required postcondition fails
6. MUST record verification results in `status.validationResults.postConditionResults[]`
7. MUST capture rollback information when postcondition fails
8. MUST transition to `rollback_ready` phase with postcondition failure details

**Integration Points**:
- **Job Monitoring**: Evaluates postconditions after Job completion
- **Kubernetes API**: Queries cluster state for verification
- **WorkflowExecution**: Receives status updates with postcondition results
- **Rollback Logic**: Captures rollback information on postcondition failure

**Example**:
```yaml
# KubernetesExecution with action postconditions
apiVersion: execution.kubernaut.io/v1alpha1
kind: KubernetesExecution
metadata:
  name: scale-web-app-step-2
spec:
  action: "scale_deployment"
  parameters:
    deployment: "web-app"
    namespace: "production"
    replicas: 5
  postConditions:
    - type: desired_replicas_running
      description: "All desired replicas must be running"
      rego: |
        package postcondition
        import future.keywords.if
        allow if {
          input.running_pods >= input.target_replicas
          input.ready_pods >= input.target_replicas
        }
      required: true
      timeout: "2m"

    - type: deployment_health_check
      description: "Deployment must be Available and Progressing"
      rego: |
        package postcondition
        import future.keywords.if
        allow if {
          input.conditions.Available == true
          input.conditions.Progressing == true
        }
      required: true
      timeout: "1m"

status:
  phase: failed
  executionResults:
    success: true  # Job succeeded
    jobName: "scale-web-app-job"
    duration: "15s"
  validationResults:
    postConditionResults:
      - conditionType: desired_replicas_running
        evaluated: true
        passed: false
        errorMessage: "Only 2 of 5 desired pods running (insufficient resources)"
        evaluationTime: "2025-10-14T10:02:45Z"
      - conditionType: deployment_health_check
        evaluated: true
        passed: false
        errorMessage: "Deployment condition 'Available' is False"
        evaluationTime: "2025-10-14T10:02:46Z"
  rollbackInformation:
    available: true
    rollbackAction: "scale_deployment"
    rollbackParameters:
      deployment: "web-app"
      namespace: "production"
      replicas: 3  # original value
```

**Testing Requirements**:
- Unit: Async verification with timeout scenarios
- Integration: Real pod startup and failure scenarios
- E2E: Postcondition failure triggering workflow rollback

**Related BRs**:
- BR-EXEC-070: Rollback information extraction (enhanced with postcondition failure details)
- BR-WF-052: Workflow postcondition verification (receives results from action postconditions)
- BR-REGO-001: Rego policy evaluation

---

## Cross-Service Integration

### Validation Flow

```
1. WorkflowExecution evaluates step.preConditions[]
   ↓ (if pass)
2. WorkflowExecution creates KubernetesExecution CRD
   ↓
3. KubernetesExecutor evaluates spec.preConditions[]
   ↓ (if pass)
4. KubernetesExecutor creates Kubernetes Job
   ↓
5. Job executes kubectl action
   ↓ (Job succeeds)
6. KubernetesExecutor evaluates spec.postConditions[]
   ↓ (if pass)
7. KubernetesExecutor updates status.phase = "completed"
   ↓
8. WorkflowExecution detects completion
   ↓
9. WorkflowExecution evaluates step.postConditions[]
   ↓
10. WorkflowExecution updates status with overall verification results
```

### Failure Handling

**Precondition Failure**:
- WorkflowExecution: Halt workflow, mark step as blocked, don't create KubernetesExecution
- KubernetesExecutor: Mark execution as failed, don't create Job

**Postcondition Failure**:
- KubernetesExecutor: Mark execution as failed, capture rollback information
- WorkflowExecution: Mark step as failed, trigger rollback if `rollbackStrategy=automatic`

---

## Implementation Priorities

### Phase 1: High-Value Actions (Weeks 1-2)
Implement conditions for top 5 actions covering 75% of remediation scenarios:
- scale_deployment
- restart_pod
- increase_resources
- rollback_deployment
- expand_pvc

**Success Metric**: 20% reduction in cascade failures for these 5 actions

### Phase 2: Medium-Value Actions (Weeks 3-4)
Extend to next 10 actions covering 90% of scenarios:
- Infrastructure: drain_node, cordon_node, uncordon_node, taint_node, untaint_node, quarantine_pod
- Storage: cleanup_storage, backup_data, compact_storage
- Application: update_hpa

**Success Metric**: 85% remediation effectiveness for all Phase 1+2 actions

### Phase 3: Remaining Actions (Weeks 5-6)
Complete coverage for all 27 canonical actions (100%):
- Security: rotate_secrets, audit_logs, update_network_policy
- Network: restart_network, reset_service_mesh
- Database: failover_database, repair_database
- Monitoring: enable_debug_mode, create_heap_dump, collect_diagnostics
- Resource: optimize_resources, migrate_workload, scale_statefulset, restart_daemonset
- Fallback: notify_only

**Success Metric**: <10% cascade failure rate, <15% false positive rate

---

## Success Metrics

| Metric | Baseline | Target | Measurement |
|--------|----------|--------|-------------|
| **Remediation Effectiveness** | 70% | 85-90% | % of workflows achieving intended outcome |
| **Cascade Failure Rate** | 30% | <10% | % of workflows failing due to invalid state |
| **MTTR (Failed Remediation)** | 15 min | <8 min | Time to diagnose failure root cause |
| **Manual Intervention** | 40% | 20% | % of remediations requiring human analysis |
| **False Positive Rate** | 0% (none exist) | <15% | % of workflows blocked by incorrect conditions |
| **Condition Adoption** | 0% | 80% | % of workflows using conditions (within 6 months) |

---

## References

- **Design Decision**: [DD-002 - Per-Step Validation Framework](../architecture/DESIGN_DECISIONS.md#dd-002-per-step-validation-framework-alternative-2)
- **Implementation Plan**: [Precondition/Postcondition Framework](../services/crd-controllers/standards/precondition-postcondition-framework.md)
- **APDC Methodology**: [.cursor/rules/00-core-development-methodology.mdc](../../.cursor/rules/00-core-development-methodology.mdc)
- **Rego Policy Guidelines**: [04-kubernetesexecutor/implementation/REGO_POLICY_INTEGRATION.md](../services/crd-controllers/04-kubernetesexecutor/implementation/REGO_POLICY_INTEGRATION.md)

