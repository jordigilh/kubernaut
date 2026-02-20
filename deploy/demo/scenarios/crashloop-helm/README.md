# Scenario #135: CrashLoopBackOff with Helm-Managed Workload

## Overview

Same fault as #120 (bad ConfigMap) but workload deployed via Helm chart. Helm sets `app.kubernetes.io/managed-by: Helm`, triggering `helmManaged: true` detection. Remediation uses `helm rollback` instead of `kubectl rollout undo`.

## Signal

- **Alert**: KubernautCrashLoopDetected
- **Condition**: Container restarts > 3 in 10 minutes in `demo-crashloop-helm` namespace

## Remediation

- **Workflow**: helm-rollback-v1
- **Action Type**: HelmRollback
- **Detection**: `helmManaged: true` (from `app.kubernetes.io/managed-by: Helm` label)

## BDD Spec

```gherkin
Feature: Helm-managed CrashLoopBackOff remediation

  Scenario: Bad config via helm upgrade triggers CrashLoopBackOff
    Given a Helm-managed deployment in namespace "demo-crashloop-helm"
    And the deployment has label "app.kubernetes.io/managed-by: Helm"
    And the workload is healthy with 2 replicas
    When an invalid nginx config is applied via "helm upgrade"
    Then the worker pods enter CrashLoopBackOff
    And the KubernautCrashLoopDetected alert fires
    And the pipeline detects helmManaged=true

  Scenario: Helm rollback remediates CrashLoopBackOff
    Given the worker deployment is in CrashLoopBackOff
    And the Helm release has a previous healthy revision
    When the helm-rollback-v1 workflow executes
    Then "helm rollback" is invoked to the previous revision
    And the worker pods recover and become Ready
    And the deployment has 2/2 replicas ready
```

## Acceptance Criteria

- [ ] Helm chart deploys worker deployment with `app.kubernetes.io/managed-by: Helm` label
- [ ] `helm upgrade` with bad nginx config causes pods to crash on startup
- [ ] PrometheusRule fires KubernautCrashLoopDetected when restarts > 3 in 10m
- [ ] HAPI/LLM detects `helmManaged: true` and selects helm-rollback-v1 workflow
- [ ] helm-rollback-v1 job runs `helm rollback` to previous revision
- [ ] After rollback, pods are Running and deployment has desired replicas ready
