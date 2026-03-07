# BR-WE-016: EngineConfig Discriminator Pattern

**Business Requirement ID**: BR-WE-016
**Category**: Workflow Engine Service
**Priority**: **P1 (HIGH)** - Schema Extensibility
**Target Version**: **V1.0**
**Status**: Active
**Date**: March 2, 2026
**Related ADRs**: ADR-043 (Execution Engine Schema), ADR-044 (Engine Portability)
**Related BRs**: BR-WE-014 (K8s Job Backend), BR-WE-015 (Ansible Engine), BR-WORKFLOW-004 (Schema Format)
**GitHub Issue**: [#45](https://github.com/jordigilh/kubernaut/issues/45)

---

## Business Need

### Problem Statement

The WorkflowRef in the WorkflowExecution CRD currently has OCI-specific fields (`executionBundle`, `executionBundleDigest`) that work for Tekton and K8s Jobs but do not accommodate engine-specific configuration. The Ansible engine requires additional fields (playbook path, inventory name, job template name) that have no place in the current schema. Adding flat top-level fields per engine creates namespace pollution and makes it impossible to validate "only set fields for your engine."

### Impact Without This BR

- No extensible mechanism for engine-specific configuration
- Each new engine would add fields to WorkflowRef, polluting the schema
- No way to validate that engine-specific fields match the declared engine
- Future engines (Argo, Flux, etc.) would require schema deprecation and migration

---

## Business Objective

**WorkflowRef SHALL support an `engineConfig` field using a discriminator pattern where the `executionEngine` field determines the shape of `engineConfig`, enabling engine-specific configuration without schema deprecation when future engines are added.**

### Success Criteria

1. `WorkflowRef` gains an optional `engineConfig` field stored as opaque JSON (`apiextensionsv1.JSON`)
2. The `executionEngine` field acts as the discriminator — its value determines how `engineConfig` is deserialized
3. For `ansible`: `engineConfig` contains `playbookPath` (required), `inventoryName` (optional), `jobTemplateName` (optional)
4. For `tekton` and `job`: `engineConfig` is nil/absent (no engine-specific config needed)
5. DS validates `engineConfig` contents at workflow registration time based on the declared engine
6. `engineConfig` flows through the full pipeline: DS catalog -> AIAnalysis -> RO creator -> WFE CRD -> Executor
7. Adding a future engine requires only a new Go struct and a switch case — no CRD schema changes, no version bump
8. Existing `tekton`/`job` workflows are fully backward compatible (zero changes)

---

## Technical Requirements

### TR-1: CRD Schema

```go
type WorkflowRef struct {
    WorkflowID            string              `json:"workflowId"`
    Version               string              `json:"version"`
    ExecutionBundle       string              `json:"executionBundle"`
    ExecutionBundleDigest string              `json:"executionBundleDigest,omitempty"`
    // +kubebuilder:pruning:PreserveUnknownFields
    // +optional
    EngineConfig          *apiextensionsv1.JSON `json:"engineConfig,omitempty"`
}
```

### TR-2: Two-Phase Unmarshal

```go
func ParseEngineConfig(engine string, raw json.RawMessage) (any, error) {
    switch engine {
    case "ansible":
        var cfg AnsibleEngineConfig
        return &cfg, json.Unmarshal(raw, &cfg)
    case "tekton", "job":
        return nil, nil
    default:
        return nil, fmt.Errorf("unknown engine: %s", engine)
    }
}
```

### TR-3: AnsibleEngineConfig

```go
type AnsibleEngineConfig struct {
    PlaybookPath    string `json:"playbookPath"`
    InventoryName   string `json:"inventoryName,omitempty"`
    JobTemplateName string `json:"jobTemplateName,omitempty"`
}
```

### TR-4: Workflow Schema

```yaml
execution:
  engine: ansible
  bundle: https://github.com/org/playbooks.git
  bundleDigest: abc123
  engineConfig:
    playbookPath: playbooks/restart-pod.yml
    inventoryName: k8s-inventory
```

### TR-5: Pipeline Pass-Through

`engineConfig` SHALL be stored as `JSONB` in the DS catalog, included in search results, passed through `AIAnalysis.Status.SelectedWorkflow.EngineConfig`, and propagated by the RO creator to `WorkflowExecution.Spec.WorkflowRef.EngineConfig`.

### TR-6: DS Validation

At workflow registration time, DS SHALL validate `engineConfig` based on the declared `engine`:
- `ansible`: require `playbookPath` to be non-empty
- `tekton`/`job`: reject `engineConfig` if present (or ignore it)
- Unknown engine: reject registration

---

## Acceptance Criteria

```gherkin
Given a workflow-schema.yaml with engine "ansible" and engineConfig containing playbookPath
When the workflow is registered in DS via OCI
Then the engineConfig is stored in the catalog as JSONB
And the engineConfig is returned in search results

Given a workflow selected by AIAnalysis with engineConfig
When RO creates the WorkflowExecution CRD
Then the WFE spec.workflowRef.engineConfig contains the same data

Given a WorkflowExecution CRD with executionEngine "ansible"
When the AnsibleExecutor reads the CRD
Then it deserializes engineConfig as AnsibleEngineConfig
And uses playbookPath to configure the AWX job

Given a workflow-schema.yaml with engine "tekton" and no engineConfig
When the workflow is registered and executed
Then the pipeline works identically to today (backward compatible)
```

---

## Dependencies

- **BR-WE-015**: Ansible execution engine (co-requisite — first consumer of engineConfig)
- **BR-WORKFLOW-004**: Workflow schema format (engineConfig field addition)
- **#292**: Schema restructuring (independent, does not block)

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-02 | Initial BR |
