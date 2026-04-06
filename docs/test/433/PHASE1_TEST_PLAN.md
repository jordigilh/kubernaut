# Phase 1 Test Plan: Wire Registry + HAPI-Specific Tool Parity + Prometheus Parity

**Issue**: #433 — Kubernaut Agent Go Rewrite
**Phase**: 1 of 3
**Date**: 2026-03-04
**Status**: Active
**Authority**: DD-TEST-006

---

## Scope

Phase 1 wires real tool execution into the investigator loop, fixes all HAPI-specific custom tools, adds Prometheus parity, implements TodoWrite, and updates enrichment types. After this phase, the investigator calls `registry.Execute()` with real tools instead of returning stub responses.

### Components Under Test

- `internal/kubernautagent/types/` — SignalContext propagation via context.Context
- `pkg/kubernautagent/tools/custom/` — 5 HAPI-specific tools (resource context x2, workflow discovery x3)
- `pkg/kubernautagent/tools/prometheus/` — 8 Prometheus tools (6 existing + 2 new)
- `pkg/kubernautagent/tools/investigation/` — TodoWrite tool (new)
- `pkg/kubernautagent/tools/registry/` — Registry wiring
- `internal/kubernautagent/enrichment/` — Updated interfaces + adapters
- `internal/kubernautagent/investigator/` — Registry integration, real tool execution
- `cmd/kubernautagent/main.go` — Production wiring

---

## Test Scenario Naming Convention

Format: `{TIER}-KA-433-{SEQUENCE}`

- `UT` — Unit Test (fake clients, mock interfaces)
- `IT` — Integration Test (real DataStorage, mock LLM)
- `E2E` — End-to-End (Kind + Mock LLM + real DS)

---

## Unit Tests

### 1A. Signal Context Propagation

| ID | Description | Status |
|----|-------------|--------|
| UT-KA-433-101 | WithSignalContext stores signal in context, SignalContextFromContext retrieves it | |
| UT-KA-433-102 | SignalContextFromContext returns false for context without signal | |
| UT-KA-433-103 | Signal context carries all fields (Name, Namespace, Severity, Environment, Priority, etc.) | |

### 1B. Resource Context Tools

| ID | Description | Status |
|----|-------------|--------|
| UT-KA-433-110 | get_namespaced_resource_context returns root_owner from owner chain last element | |
| UT-KA-433-111 | get_namespaced_resource_context returns {kind, name, namespace} as root_owner when chain empty | |
| UT-KA-433-112 | get_namespaced_resource_context includes remediation_history (non-empty list) | |
| UT-KA-433-113 | get_namespaced_resource_context includes empty remediation_history as [] | |
| UT-KA-433-114 | get_namespaced_resource_context includes detected_infrastructure when labels detected | |
| UT-KA-433-115 | get_namespaced_resource_context omits detected_infrastructure when nil | |
| UT-KA-433-116 | get_namespaced_resource_context includes quota_details when quotas exist | |
| UT-KA-433-117 | get_namespaced_resource_context omits quota_details when nil | |
| UT-KA-433-118 | get_namespaced_resource_context does NOT include owner_chain or spec_hash in response | |
| UT-KA-433-119 | get_namespaced_resource_context has valid JSON parameter schema (kind, name, namespace required) | |
| UT-KA-433-120 | get_cluster_resource_context returns root_owner without namespace | |
| UT-KA-433-121 | get_cluster_resource_context includes remediation_history, no detected_infrastructure/quota_details | |
| UT-KA-433-122 | get_cluster_resource_context has valid JSON parameter schema (kind, name required) | |

### 1C. Enrichment Types and Adapters

| ID | Description | Status |
|----|-------------|--------|
| UT-KA-433-130 | DetectedLabels struct serializes all bool/string fields correctly | |
| UT-KA-433-131 | DetectedLabels struct includes failedDetections as []string | |
| UT-KA-433-132 | EnrichmentResult.QuotaDetails serializes as map[string]string | |
| UT-KA-433-133 | DataStorageClient adapter maps GetRemediationHistory to ogen GetRemediationHistoryContext with kind + specHash | |

### 1C-Labels. Label Detection

| ID | Description | Status |
|----|-------------|--------|
| UT-KA-433-140 | GitOps detection: ArgoCD tracking-id on pod annotation returns gitOpsManaged=true, gitOpsTool=argocd | |
| UT-KA-433-141 | GitOps detection: Flux label on deployment returns gitOpsTool=flux | |
| UT-KA-433-142 | GitOps detection: ArgoCD on namespace annotation returns gitOpsTool=argocd | |
| UT-KA-433-143 | GitOps detection: no matching annotations returns gitOpsManaged=false | |
| UT-KA-433-144 | PDB detection: matching PDB selector returns pdbProtected=true | |
| UT-KA-433-145 | PDB detection: empty pod labels skips LIST, returns pdbProtected=false (no failedDetection) | |
| UT-KA-433-146 | PDB detection: LIST error appends "pdbProtected" to failedDetections | |
| UT-KA-433-147 | HPA detection: scaleTargetRef matches deployment name returns hpaEnabled=true | |
| UT-KA-433-148 | HPA detection: scaleTargetRef matches owner chain entry returns hpaEnabled=true | |
| UT-KA-433-149 | HPA detection: empty targets skips LIST, returns hpaEnabled=false | |
| UT-KA-433-150 | Stateful detection: owner chain with StatefulSet kind returns stateful=true | |
| UT-KA-433-151 | Helm detection: managed-by=Helm label returns helmManaged=true | |
| UT-KA-433-152 | Helm detection: helm.sh/chart label exists returns helmManaged=true | |
| UT-KA-433-153 | NetworkPolicy detection: any netpol in namespace returns networkIsolated=true | |
| UT-KA-433-154 | NetworkPolicy detection: empty list returns networkIsolated=false | |
| UT-KA-433-155 | ServiceMesh detection: istio sidecar annotation returns serviceMesh=istio | |
| UT-KA-433-156 | ServiceMesh detection: linkerd proxy annotation returns serviceMesh=linkerd | |
| UT-KA-433-157 | ResourceQuota detection: quota exists returns resourceQuotaConstrained=true + quota_summary | |
| UT-KA-433-158 | ResourceQuota detection: empty list returns false, no failedDetection, nil summary | |
| UT-KA-433-159 | ResourceQuota detection: LIST error appends failedDetection, nil summary | |
| UT-KA-433-160 | Full detect_labels with nil k8s_context returns nil, nil | |
| UT-KA-433-161 | Partial failure: one check fails, others succeed, failedDetections populated | |

### 1D. Custom Tool Fixes

| ID | Description | Status |
|----|-------------|--------|
| UT-KA-433-170 | list_workflows accepts action_type (required), offset, limit params | |
| UT-KA-433-171 | list_workflows has valid JSON parameter schema with required action_type | |
| UT-KA-433-172 | list_available_actions extracts signal context from ctx (Severity, Component, Environment, Priority) | |
| UT-KA-433-173 | list_available_actions has valid JSON parameter schema | |
| UT-KA-433-174 | get_workflow accepts workflow_id param (not id) | |
| UT-KA-433-175 | get_workflow has valid JSON parameter schema with required workflow_id | |
| UT-KA-433-176 | All 5 custom tools have non-empty JSON parameter schemas (not {}) | |

### 1E. PhaseToolMap

| ID | Description | Status |
|----|-------------|--------|
| UT-KA-433-180 | DefaultPhaseToolMap assigns resource context tools to PhaseRCA | |
| UT-KA-433-181 | DefaultPhaseToolMap assigns workflow discovery tools to PhaseWorkflowDiscovery | |
| UT-KA-433-182 | DefaultPhaseToolMap assigns todo_write to all phases (RCA, WorkflowDiscovery, Validation) | |

### 1F. Prometheus Tool Parity

| ID | Description | Status |
|----|-------------|--------|
| UT-KA-433-190 | All 8 Prometheus tools have non-empty JSON parameter schemas | |
| UT-KA-433-191 | list_prometheus_rules tool calls GET /api/v1/rules | |
| UT-KA-433-192 | get_series tool calls GET /api/v1/series with match[] param | |
| UT-KA-433-193 | AllToolNames contains 8 entries | |

### 1-TodoWrite. TodoWrite Tool

| ID | Description | Status |
|----|-------------|--------|
| UT-KA-433-200 | TodoWrite creates items from {id, content, status} array | |
| UT-KA-433-201 | TodoWrite merges items by id on subsequent calls | |
| UT-KA-433-202 | TodoWrite returns JSON summary with count by status | |
| UT-KA-433-203 | TodoWrite has valid JSON parameter schema | |
| UT-KA-433-204 | TodoWrite accepts all status values: pending, in_progress, completed, cancelled | |

### 1G. Registry Wiring in Investigator

| ID | Description | Status |
|----|-------------|--------|
| UT-KA-433-210 | toolDefinitionsForPhase returns real Name/Description/Parameters from registry | |
| UT-KA-433-211 | runLLMLoop calls registry.Execute for each tool call | |
| UT-KA-433-212 | Tool execution error returns JSON error to LLM (not fatal) | |
| UT-KA-433-213 | Tool execution result is passed as tool message content to LLM | |

---

## Integration Tests

| ID | Description | Status |
|----|-------------|--------|
| IT-KA-433-301 | list_available_actions returns real action types from DataStorage | |
| IT-KA-433-302 | list_workflows with action_type returns workflows from DataStorage | |
| IT-KA-433-303 | get_workflow by workflow_id returns workflow definition from DataStorage | |
| IT-KA-433-304 | Investigator with mock LLM calls registry.Execute and returns real tool output | |
| IT-KA-433-305 | Enrichment adapter resolves remediation history from real DataStorage | |

---

## E2E Tests

| ID | Description | Status |
|----|-------------|--------|
| E2E-KA-433-401 | 3-step discovery: resource context + list_available_actions + list_workflows + get_workflow against real DS in Kind | |
| E2E-KA-433-402 | Investigation produces real tool output (not stub {"status":"ok"}) | |

---

## Acceptance Criteria

1. `go build ./...` passes
2. `golangci-lint run --timeout=5m` passes
3. All UT pass with `-race -count=1`
4. All IT pass against real DS containers
5. E2E 3-step discovery passes in Kind
6. 8 Prometheus tools registered (verified by UT-KA-433-193)
7. TodoWrite registered in all investigation phases (verified by UT-KA-433-182)
8. No stub responses remain in runLLMLoop
9. All custom + Prometheus tools have non-empty parameter schemas
