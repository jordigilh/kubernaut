# BR-WORKFLOW-004: Workflow Schema Format Specification

**Business Requirement ID**: BR-WORKFLOW-004
**Category**: Workflow Catalog Service
**Priority**: P0
**Target Version**: V1.0
**Status**: Active
**Date**: February 12, 2026

**Authority**: This is the authoritative specification for the `workflow-schema.yaml` file format. All implementations, tests, and documentation must conform to this BR.

**Related**:
- [DD-WORKFLOW-016](../architecture/decisions/DD-WORKFLOW-016-action-type-workflow-indexing.md) -- Action type taxonomy and structured descriptions
- [DD-WORKFLOW-017](../architecture/decisions/DD-WORKFLOW-017-workflow-lifecycle-component-interactions.md) -- OCI-based workflow registration (pullspec-only)
- [ADR-043](../architecture/decisions/ADR-043-workflow-schema-definition-standard.md) -- Original schema standard (superseded by this BR for format)

---

## Business Need

### Problem Statement

Kubernaut remediation workflows are packaged as OCI container images. Each image must contain a `/workflow-schema.yaml` file that provides all metadata needed for catalog registration, discovery, and LLM-assisted workflow selection. A clear, authoritative format specification is required to ensure:

- Operators know exactly what to include in their workflow images
- Data Storage can reliably parse and validate the schema
- The LLM receives structured, consistent information for decision-making
- Tests have a single source of truth for fixture generation

### Design Principles

1. **Plain configuration file** -- `workflow-schema.yaml` is not a Kubernetes resource. It has no `apiVersion` or `kind` fields. It is a configuration file with a defined schema.
2. **camelCase field names** -- Consistent with kubernaut configuration conventions.
3. **Single source of truth** -- All workflow metadata is extracted from this file during OCI-based registration. The operator does not provide metadata separately.
4. **Structured descriptions** -- Descriptions use a structured format (`what`, `whenToUse`, `whenNotToUse`, `preconditions`) that is useful for both operators and the LLM.

---

## Schema Format

### Complete Example

```yaml
metadata:
  workflowId: oomkill-restart-pod
  version: "1.0.0"
  description:
    what: "Delete and recreate a pod to recover from transient OOMKill failures"
    whenToUse: "OOMKilled with transient root cause, such as a temporary traffic spike or undersized resource limits"
    whenNotToUse: "When OOM is caused by a memory leak in application code"
    preconditions: "Pod is managed by a controller (Deployment, StatefulSet, DaemonSet)"
  maintainers:
    - name: "Platform Team"
      email: "platform@example.com"

actionType: RestartPod

labels:
  signalType: OOMKilled
  severity: critical
  environment: production
  component: pod
  priority: p1

customLabels:
  team: platform
  costCenter: ops

execution:
  engine: tekton
  bundle: quay.io/kubernaut/oomkill-restart:v1.0.0

parameters:
  - name: NAMESPACE
    type: string
    required: true
    description: "Target namespace containing the pod"
  - name: POD_NAME
    type: string
    required: true
    description: "Name of the pod to restart"
  - name: GRACE_PERIOD
    type: integer
    required: false
    description: "Graceful shutdown period in seconds"
    default: 30
    minimum: 0
    maximum: 300

rollbackParameters:
  - name: SNAPSHOT_ID
    type: string
    required: false
    description: "Snapshot to restore if restart fails"
```

---

## Field Specification

### Top-Level Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `metadata` | object | Yes | Workflow identification and description |
| `actionType` | string | Yes | Action type from the taxonomy (PascalCase, e.g., `RestartPod`, `ScaleReplicas`). Must match a valid entry in `action_type_taxonomy`. |
| `labels` | object | Yes | Mandatory matching/filtering criteria for workflow discovery |
| `customLabels` | map[string]string | No | Operator-defined key-value labels for additional filtering |
| `execution` | object | No | Execution engine configuration |
| `parameters` | array | Yes | Workflow input parameters (at least one required) |
| `rollbackParameters` | array | No | Parameters needed for rollback |

### `metadata` Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `workflowId` | string | Yes | Unique workflow identifier. Format: lowercase alphanumeric with hyphens (e.g., `oomkill-restart-pod`). Max 255 characters. |
| `version` | string | Yes | Semantic version (e.g., `1.0.0`). Max 50 characters. |
| `description` | object | Yes | Structured description (see below) |
| `maintainers` | array | No | Maintainer contact information |

### `metadata.description` Fields (Structured)

The description uses the same structured format as `action_type_taxonomy.description` (DD-WORKFLOW-016). This information is provided to the LLM during workflow selection to help it choose the right workflow for the incident.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `what` | string | Yes | What this workflow concretely does. One sentence. |
| `whenToUse` | string | Yes | Root cause conditions under which this workflow is appropriate. |
| `whenNotToUse` | string | No | Specific exclusion conditions for this workflow. Only include when there is a genuinely useful exclusion. Do not include failure-based exclusions (handled by remediation history, DD-HAPI-016). |
| `preconditions` | string | No | Conditions that must be verified through investigation that cannot be determined by catalog label filtering. |

### `labels` Fields (Mandatory Matching Criteria)

These fields are used by the three-step discovery protocol (DD-HAPI-017) to filter workflows for a given incident context. They are stored in the `labels` JSONB column of `remediation_workflow_catalog`.

| Field | Type | Required | Valid Values | Description |
|-------|------|----------|--------------|-------------|
| `signalType` | string | Yes | Any (e.g., `OOMKilled`, `CrashLoopBackOff`, `NodeNotReady`) | The signal type this workflow handles |
| `severity` | string | Yes | `critical`, `high`, `medium`, `low` | Severity level this workflow is designed for |
| `component` | string | Yes | Any (e.g., `pod`, `deployment`, `node`, `service`) | Kubernetes resource type this workflow remediates |
| `environment` | string | Yes | Any (e.g., `production`, `staging`, `*` for all) | Target environment |
| `priority` | string | Yes | `p0`, `p1`, `p2`, `p3`, `p4`, `*` for all | Business priority level |

### `execution` Fields

| Field | Type | Required | Valid Values | Description |
|-------|------|----------|--------------|-------------|
| `engine` | string | No | `tekton`, `ansible`, `lambda`, `shell` | Execution engine type. Defaults to `tekton`. |
| `bundle` | string | No | OCI image reference | Execution bundle or container image |

### `parameters` and `rollbackParameters` Fields

Each parameter is an object with the following fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Parameter name (UPPER_SNAKE_CASE per DD-WORKFLOW-003) |
| `type` | string | Yes | One of: `string`, `integer`, `boolean`, `array` |
| `required` | boolean | Yes | Whether the parameter must be provided |
| `description` | string | Yes | Human-readable description (shown to LLM) |
| `enum` | array of strings | No | Allowed values (for string type) |
| `pattern` | string | No | Regex pattern for validation (for string type) |
| `minimum` | integer | No | Minimum value (for integer type) |
| `maximum` | integer | No | Maximum value (for integer type) |
| `default` | any | No | Default value if not provided |
| `dependsOn` | array of strings | No | Parameter names that must be set first |

### `maintainers` Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Maintainer name |
| `email` | string | Yes | Maintainer email address |

---

## Validation Rules

### Registration Validation (Data Storage)

When a workflow is registered via `POST /api/v1/workflows` (OCI pullspec-only), Data Storage:

1. Pulls the OCI image
2. Extracts `/workflow-schema.yaml` from the image layers
3. Parses and validates the YAML against this specification
4. Validates `actionType` against `action_type_taxonomy` (FK constraint)
5. Extracts and stores all fields in `remediation_workflow_catalog`

### Required Field Validation

- All fields marked "Required: Yes" must be present and non-empty
- `metadata.description.what` and `metadata.description.whenToUse` are always required
- `metadata.description.whenNotToUse` and `metadata.description.preconditions` are optional
- At least one parameter is required in `parameters`
- Each parameter must have `name`, `type`, and `description`

### Action Type Validation

- `actionType` must reference a valid entry in the `action_type_taxonomy` table
- Action type values use PascalCase (e.g., `RestartPod`, `ScaleReplicas`)
- If the action type is not found, registration fails with an error listing valid action types

---

## Deprecated Fields

The following fields from the previous schema format (ADR-043) are removed:

| Field | Reason |
|-------|--------|
| `apiVersion` | No Kubernetes association. This is a plain configuration file. |
| `kind` | No Kubernetes association. |
| `labels.riskTolerance` | Never stored in the database, never queried, never used in discovery. Dead code. If needed in the future, use `customLabels`. |
| `labels.businessCategory` | Moved to `customLabels` (operator-defined, not a mandatory matching criterion). |

---

## Naming Conventions

| Context | Convention | Example |
|---------|-----------|---------|
| YAML field names | camelCase | `workflowId`, `signalType`, `actionType` |
| Action type values | PascalCase | `RestartPod`, `ScaleReplicas` |
| Parameter names | UPPER_SNAKE_CASE | `NAMESPACE`, `POD_NAME` |
| JSONB keys (DB storage) | camelCase | `{"signalType": "OOMKilled"}` |
| PostgreSQL columns | snake_case | `workflow_name`, `action_type` |

---

## Acceptance Criteria

1. Data Storage can parse and validate a `workflow-schema.yaml` file conforming to this specification
2. All required fields are validated; missing fields produce clear error messages
3. `actionType` is validated against `action_type_taxonomy`; unknown types produce an error with valid options
4. Structured description fields (`what`, `whenToUse`) are required; optional fields are accepted when present
5. Labels are stored in the `labels` JSONB column with camelCase keys
6. Parameters are extracted and stored for LLM consumption
7. The schema format is documented and used consistently across all test fixtures
