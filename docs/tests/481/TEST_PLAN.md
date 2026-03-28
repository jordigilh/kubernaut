# Test Plan: Issue #481 - Per-Workflow ServiceAccount Reference

| Field | Value |
|---|---|
| **Issue** | #481 |
| **Decision** | DD-WE-005 v2.0 |
| **Business Requirements** | BR-WE-007, BR-SECURITY-001 |
| **Status** | Active |
| **Date** | 2025-03-04 |

## Overview

This test plan covers the per-workflow ServiceAccount reference feature, which
replaces platform-managed RBAC (Issue #186) with a user-provided ServiceAccount
declared in the workflow schema and propagated through the full pipeline.

## Test Scenarios

### DataStorage (DS) - Schema & Models

| ID | Description | Type | Status |
|---|---|---|---|
| UT-DS-481-001 | Parse serviceAccountName from execution section | Unit | Implemented |
| UT-DS-481-002 | Accept schema without serviceAccountName (optional) | Unit | Implemented |
| UT-DS-481-003 | ExtractServiceAccountName returns nil when absent | Unit | Implemented |
| UT-DS-481-004 | ExtractServiceAccountName returns pointer when present | Unit | Implemented |
| UT-DS-481-005 | WorkflowDiscoveryEntry includes SA when workflow has SA | Unit | Implemented |
| UT-DS-481-006 | WorkflowDiscoveryEntry omits SA when workflow has no SA | Unit | Implemented |

### HolmesGPT API (HAPI) - Validation & Parsing

| ID | Description | Type | Status |
|---|---|---|---|
| UT-HAPI-481-001 | ValidationResult carries SA name from catalog | Unit | Implemented |
| UT-HAPI-481-002 | ValidationResult SA is None when absent | Unit | Implemented |
| UT-HAPI-481-003 | SA injected into selected_workflow dict | Unit | Implemented |

### AIAnalysis (AA) - Response Processor

| ID | Description | Type | Status |
|---|---|---|---|
| UT-AA-481-001 | GetStringFromMap extracts service_account_name | Unit | Implemented |
| UT-AA-481-002 | GetStringFromMap returns empty when SA absent | Unit | Implemented |

### Remediation Orchestrator (RO) - WFE Creator

| ID | Description | Type | Status |
|---|---|---|---|
| UT-RO-481-001 | Propagate SA to ExecutionConfig when present | Unit | Implemented |
| UT-RO-481-002 | Leave ExecutionConfig nil when no SA and no timeout | Unit | Implemented |

### Workflow Execution (WE) - Executors

| ID | Description | Type | Status |
|---|---|---|---|
| UT-WE-481-001 | JobExecutor uses SA from ExecutionConfig | Unit | Implemented |
| UT-WE-481-002 | JobExecutor uses empty SA when ExecutionConfig nil | Unit | Implemented |
| UT-WE-481-003 | TektonExecutor uses SA from ExecutionConfig | Unit | Implemented |
| UT-WE-481-004 | TektonExecutor uses empty SA when ExecutionConfig nil | Unit | Implemented |

## Propagation Chain

```
Schema (workflow-schema.yaml)
  └─ serviceAccountName (optional string)
      └─ DataStorage DB (TEXT nullable)
          └─ HAPI ValidationResult.validated_service_account_name
              └─ HAPI result_parser → selected_workflow["service_account_name"]
                  └─ AA ResponseProcessor → SelectedWorkflow.ServiceAccountName
                      └─ RO Creator → ExecutionConfig.ServiceAccountName
                          └─ WE Executor → PodSpec.ServiceAccountName / TaskRunTemplate.ServiceAccountName
```

## Superseded Plans

- Issue #186 Test Plan (`docs/tests/186/TEST_PLAN.md`) - Superseded by this plan
