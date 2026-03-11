# ADR-043: Workflow Schema Definition Standard

**Status**: Approved
**Date**: 2025-11-28
**Deciders**: Architecture Team
**Related**: DD-WORKFLOW-003, DD-WORKFLOW-011, DD-NAMING-001, ADR-041, BR-WORKFLOW-006, ADR-058, DD-WORKFLOW-017
**Version**: 1.4

---

## Changelog

### Version 1.4 (2026-03-08)
- **BREAKING**: Schema now uses Kubernetes CRD envelope format (`apiVersion`/`kind`/`metadata`/`spec`). Flat format no longer supported.
- **Issue #292**: `apiVersion: kubernaut.ai/v1alpha1` and `kind: RemediationWorkflow` re-introduced as CRD envelope. All operational fields moved under `spec`.
- **Issue #299**: Registration is via `RemediationWorkflow` CRD applied with `kubectl apply`. AuthWebhook forwards inline schema to DS internal API. OCI-based registration (`schemaImage`) removed.
- **BR-WORKFLOW-004 v1.2**: `signalType`/`signalName` removed from labels. Discovery is by `actionType` (DD-WORKFLOW-016).
- **Labels**: `severity` and `environment` are now arrays (e.g., `severity: [critical, high]`). `component` and `priority` remain scalar strings.
- Schema file location: Inline in `RemediationWorkflow` CRD `spec` (no longer extracted from OCI bundles). OCI bundles retained for execution only (`spec.execution.bundle`).
- See: BR-WORKFLOW-006, ADR-058, DD-WORKFLOW-017

### Version 1.3 (2026-02-20)
- **Issue #131**: Added `detectedLabels` as optional field for workflow-author-declared infrastructure requirements
- **DD-WORKFLOW-001 v2.0**: 8 supported fields: `gitOpsManaged`, `gitOpsTool`, `pdbProtected`, `hpaEnabled`, `stateful`, `helmManaged`, `networkIsolated`, `serviceMesh`
- **Validation**: Boolean fields accept only `"true"`; string fields accept specific values or `"*"` wildcard; unknown fields rejected
- See test plan: `docs/testing/ADR-043/TEST_PLAN.md`

### Version 1.2 (2026-02-13)
- **BR-WORKFLOW-004**: Removed `apiVersion` and `kind` fields (plain config file, not K8s resource) -- **Superseded by v1.4**
- **BR-WORKFLOW-004**: Renamed field names to camelCase (e.g., `signal_type` -> `signalType`, `workflow_id` -> `workflowId`)
- **BR-WORKFLOW-004**: Promoted `actionType` from labels to top-level field
- **BR-WORKFLOW-004**: Made `description` a structured object (`what`, `whenToUse`, `whenNotToUse`, `preconditions`)
- **BR-WORKFLOW-004**: Deprecated `riskTolerance` (never stored in DB, removed from schema)
- See `docs/requirements/BR-WORKFLOW-004-workflow-schema-format.md` for authoritative format specification

### Version 1.1 (2026-02-05)
- Added `"job"` (Kubernetes Job) as a V1 execution engine value alongside `"tekton"`
- Updated V1/V2 engine value tables to reflect BR-WE-014 (K8s Job execution backend)

### Version 1.0 (2025-11-28)
- Initial ADR creation
- Approved separate `workflow-schema.yaml` file approach
- Defined schema format aligned with industry standards (Helm, GitHub Actions)
- Rejected Tekton-only parameter extraction approach

---

## Context

Kubernaut requires a standardized way to define remediation workflow metadata, parameters, and discovery labels. Two approaches were evaluated:

### Problem Statement

When operators define remediation workflows for Kubernaut, we need to:

1. **Discover workflows** by action type, severity, and other labels
2. **Guide LLM** with parameter constraints (enum, pattern, min/max)
3. **Validate parameters** before execution
4. **Support multiple execution engines** (Tekton, Kubernetes Job, Ansible)
5. **Enable GitOps registration** via `kubectl apply` of `RemediationWorkflow` CRDs (#292, #299)

### Options Evaluated

**Option A: Extract from Tekton Pipeline directly**
- Parse `spec.params` from `pipeline.yaml`
- Use Kubernetes labels/annotations for metadata

**Option B: Separate schema file (APPROVED)**
- Define `/workflow-schema.yaml` with Kubernaut-specific contract
- Execution engine agnostic
- Rich validation support

---

## Decision

**APPROVED**: Use a separate `workflow-schema.yaml` file as the authoritative parameter and metadata contract for all Kubernaut remediation workflows.

**Confidence**: 95%

---

## Rationale

### Why NOT Extract from Tekton Pipeline (Option A)

| Limitation | Impact |
|------------|--------|
| No `enum` support | LLM cannot know allowed values |
| No `pattern` support | No regex validation |
| K8s label limits (63 chars) | Cannot store rich labels |
| Tekton-coupled | Cannot support Ansible/Lambda in V2 |
| Higher complexity | Parse Tekton structs, handle API changes |

**Effort**: 3-4 days
**Maintenance**: Higher (Tekton API version changes)

### Why Separate Schema File (Option B) ⭐

| Benefit | Impact |
|---------|--------|
| Rich validation | `enum`, `pattern`, `min/max` supported |
| Label freedom | No character limits |
| Engine agnostic | Supports Tekton, Ansible, Lambda |
| Simpler parsing | Standard YAML, known location |
| Industry alignment | Helm, GitHub Actions, Ansible patterns |

**Effort**: 2-3 days
**Maintenance**: Lower (stable Kubernaut-controlled schema)

---

## Industry Standards Alignment

| Industry Tool | Pattern | Kubernaut Alignment |
|---------------|---------|---------------------|
| **Helm** | `values.schema.json` separate file | ✅ `workflow-schema.yaml` |
| **GitHub Actions** | `action.yml` input definitions | ✅ YAML format, `inputs` section |
| **Ansible Galaxy** | `meta/argument_specs.yml` | ✅ Parameter specs with choices |
| **AWS Step Functions** | Separate state machine JSON | ✅ Decoupled from implementation |
| **Argo Workflows** | WorkflowTemplate with params | ⚠️ Inline but supports enum |

**Confidence**: 95% - Aligns with dominant industry patterns

---

## Specification

### Schema Location

As of v1.4, the workflow schema is embedded inline in the `RemediationWorkflow` CRD `spec` field and registered via `kubectl apply`:

```yaml
# Applied directly to the cluster:
kubectl apply -f my-workflow.yaml
```

The AuthWebhook intercepts the CREATE admission and forwards the inline schema to the Data Storage internal API (ADR-058). OCI bundles are retained for **execution** only (referenced by `spec.execution.bundle`).

**Format**: YAML 1.2 with Kubernetes CRD envelope
**Encoding**: UTF-8

### Schema Definition

```yaml
# RemediationWorkflow CRD Format (v1.4)
# Authority: ADR-043, BR-WORKFLOW-004, BR-WORKFLOW-006
# CRD definition: config/crd/bases/kubernaut.ai_remediationworkflows.yaml
# Parser: pkg/datastorage/schema/parser.go

# ============================================
# CRD ENVELOPE (Required)
# ============================================
apiVersion: kubernaut.ai/v1alpha1     # REQUIRED - determines SchemaVersion
kind: RemediationWorkflow             # REQUIRED - must be "RemediationWorkflow"
metadata:
  name: string                        # REQUIRED - K8s resource name

# ============================================
# SPEC (Required) - all operational fields
# ============================================
spec:
  # ============================================
  # WORKFLOW IDENTITY (Required)
  # Workflow name comes from metadata.name
  # ============================================
  version: string     # REQUIRED - semantic version (e.g., "1.0.0")
  description:        # REQUIRED - structured description for LLM and operators
    what: string        # REQUIRED - one sentence describing what the workflow does
    whenToUse: string   # REQUIRED - root cause conditions
    whenNotToUse: string  # OPTIONAL - exclusion conditions
    preconditions: string # OPTIONAL - conditions to verify
  maintainers:        # OPTIONAL
    - name: string
      email: string

  # ============================================
  # ACTION TYPE (Required)
  # Primary matching key for workflow discovery (DD-WORKFLOW-016)
  # ============================================
  actionType: string  # REQUIRED - PascalCase, from action_type_taxonomy

  # ============================================
  # DISCOVERY LABELS (Required)
  # Used by MCP search to match workflows to incidents
  # ============================================
  labels:
    severity: [string]     # REQUIRED - array: [critical, high, medium, low]
    environment: [string]  # REQUIRED - array: [production, staging, "*"]
    component: string      # REQUIRED - K8s resource type (pod, deployment, node)
    priority: string       # REQUIRED - P0, P1, P2, P3, or "*"

  # ============================================
  # CUSTOM LABELS (Optional)
  # Operator-defined key-value labels for additional filtering (#212)
  # ============================================
  customLabels:            # OPTIONAL
    [key]: string

  # ============================================
  # DETECTED LABELS (Optional)
  # Author-declared infrastructure requirements (ADR-043 v1.3)
  # DD-WORKFLOW-001 v2.0: matched against incident DetectedLabels
  # ============================================
  detectedLabels:          # OPTIONAL
    gitOpsManaged: string    # "true" - requires GitOps management
    gitOpsTool: string       # "argocd", "flux", "*"
    pdbProtected: string     # "true"
    hpaEnabled: string       # "true"
    stateful: string         # "true"
    helmManaged: string      # "true"
    networkIsolated: string  # "true"
    serviceMesh: string      # "istio", "linkerd", "*"

  # ============================================
  # EXECUTION (Required)
  # Engine and bundle configuration
  # ============================================
  execution:
    engine: string   # "tekton" (default), "job", "ansible"
    bundle: string   # OCI image (tekton/job) or Git URL (ansible), with sha256 digest
    engineConfig:    # Engine-specific config (BR-WE-016, x-kubernetes-preserve-unknown-fields)
      # For ansible: playbookPath, commitDigest, jobTemplateName, inventoryName

  # ============================================
  # DEPENDENCIES (Optional, DD-WE-006)
  # Infrastructure resources required in execution namespace
  # ============================================
  dependencies:
    secrets:
      - name: string
    configMaps:
      - name: string

  # ============================================
  # PARAMETERS (Required)
  # At least one parameter must be defined
  # ============================================
  parameters:
    - name: string        # REQUIRED - UPPER_SNAKE_CASE (DD-WORKFLOW-003)
      type: string        # REQUIRED - string, integer, boolean, array, float
      required: boolean   # REQUIRED
      description: string # REQUIRED - human-readable for LLM
      enum: [string]      # OPTIONAL - allowed values
      pattern: string     # OPTIONAL - regex pattern
      minimum: number     # OPTIONAL - min value
      maximum: number     # OPTIONAL - max value
      default: any        # OPTIONAL - default value
      dependsOn: [string] # OPTIONAL - parameter dependencies

  # ============================================
  # ROLLBACK PARAMETERS (Optional)
  # ============================================
  rollbackParameters:     # OPTIONAL
    - name: string
      type: string
      required: boolean
      description: string
```

### Complete Example

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationWorkflow
metadata:
  name: oomkill-scale-down
spec:
  version: "1.0.0"
  description:
    what: "Scale down deployment replicas to reduce memory pressure"
    whenToUse: "When pods are OOMKilled and temporary capacity reduction is acceptable"
    whenNotToUse: "When OOM is caused by memory leaks requiring code fix"
    preconditions: "Pod is managed by a Deployment or StatefulSet"
  maintainers:
    - name: Platform Team
      email: platform@example.com

  actionType: ScaleReplicas

  labels:
    severity: [critical, high]
    environment: [production, staging]
    component: deployment
    priority: P1

  detectedLabels:
    hpaEnabled: "true"
    stateful: "true"

  execution:
    engine: job
    bundle: quay.io/kubernaut/workflow-oomkill-scale-down:v1.0.0@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890

  parameters:
    - name: TARGET_RESOURCE_KIND
      type: string
      required: true
      enum: [Deployment, StatefulSet, DaemonSet]
      description: Kubernetes resource type to modify

    - name: TARGET_RESOURCE_NAME
      type: string
      required: true
      pattern: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
      description: Name of the Kubernetes resource to scale

    - name: TARGET_NAMESPACE
      type: string
      required: true
      pattern: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
      description: Kubernetes namespace containing the resource

    - name: SCALE_TARGET_REPLICAS
      type: integer
      required: true
      minimum: 0
      maximum: 100
      description: Number of replicas to scale to (0 = scale to zero)

    - name: WAIT_FOR_ROLLOUT
      type: boolean
      required: false
      default: true
      description: Wait for deployment rollout to complete before finishing

    - name: TIMEOUT_SECONDS
      type: integer
      required: false
      default: 300
      minimum: 30
      maximum: 3600
      description: Maximum time to wait for scale operation

  rollbackParameters:
    - name: ORIGINAL_REPLICAS
      type: integer
      required: true
      description: Original replica count to restore on rollback
```

---

## Validation Rules

Validation is performed in two stages by the parser at `pkg/datastorage/schema/parser.go`:

### Stage 1: CRD Envelope Validation

| Field | Rule |
|-------|------|
| `apiVersion` | Required. Must be a supported version (currently `kubernaut.ai/v1alpha1`). |
| `kind` | Required. Must be `RemediationWorkflow`. |
| `metadata.name` | Required. Standard K8s resource name. |

### Stage 2: Spec Validation (BR-WORKFLOW-004)

| Field | Rule |
|-------|------|
| `metadata.name` | Required. Workflow name; lowercase alphanumeric with hyphens. |
| `spec.version` | Required. Semantic version string. |
| `spec.description.what` | Required. |
| `spec.description.whenToUse` | Required. |
| `spec.actionType` | Required. PascalCase from action type taxonomy. |
| `spec.labels.severity` | Required. Array of severity values. |
| `spec.labels.environment` | Required. Array of environment values. |
| `spec.labels.component` | Required. K8s resource type string. |
| `spec.labels.priority` | Required. Normalized to uppercase. |
| `spec.execution.bundle` | Required. Must include `@sha256:<64 hex>` digest for tekton/job engines. |
| `spec.parameters` | At least one required. Each must have `name`, `type`, `description`. |
| `spec.detectedLabels` | Optional. Boolean fields accept only `"true"`; string fields accept specific values or `"*"`. |
| `spec.dependencies` | Optional. Secret/ConfigMap names validated if present. |
| `spec.execution.engineConfig` | Required when `engine: ansible`. Must include `playbookPath` (BR-WE-016). |

---

## Migration Path

### Phase 1: Schema Standard (Complete)

1. ✅ Create ADR-043 (this document)
2. ✅ Implement schema parser in `pkg/datastorage/schema/parser.go`
3. ✅ Schema validation during registration
4. ✅ `execution.engine: "job"` support (BR-WE-014)
5. ✅ `execution.engine: "ansible"` support with `engineConfig` (Issue #45, BR-WE-016)
6. ✅ `detectedLabels` support (ADR-043 v1.3)

### Phase 2: CRD Format Migration (Complete -- #292)

1. ✅ Restructure `workflow-schema.yaml` to `apiVersion`/`kind`/`metadata`/`spec` CRD format
2. ✅ Parser handles CRD envelope, derives `SchemaVersion` from `apiVersion`
3. ✅ All test fixtures and demo scenarios migrated
4. ✅ Flat format removed (no backward compatibility)

### Phase 3: CRD-Based Registration (Complete -- #299)

1. ✅ `RemediationWorkflow` CRD registered in cluster
2. ✅ AuthWebhook validates CREATE/DELETE and bridges to DS internal API
3. ✅ OCI-based registration (`schemaImage`) removed from DS API
4. ✅ GitOps-compatible: `kubectl apply -f workflow.yaml`

### Phase 4: Future

1. ⏳ Schema versioning (`kubernaut.ai/v1beta1`, `kubernaut.ai/v1`)
2. ⏳ JSON Schema generation for IDE support
3. ⏳ `kubernaut workflow validate` CLI command

---

## Consequences

### Positive

1. ✅ **Simpler implementation** - 2-3 days vs 3-4 days
2. ✅ **Rich validation** - enum, pattern, min/max supported
3. ✅ **Engine agnostic** - Future-proof for V2 multi-engine
4. ✅ **Industry aligned** - Helm, GitHub Actions patterns
5. ✅ **Label freedom** - No K8s character limits
6. ✅ **Single source of truth** - No confusion about where metadata lives
7. ✅ **LLM guidance** - Enum and description fields guide parameter population

### Negative

1. ⚠️ **DS availability dependency** - AuthWebhook requires DS to be reachable during `kubectl apply` (mitigated by `failurePolicy: Fail`)
2. ⚠️ **Stale CRD status** - Admin-only operations (`deprecated`, `archived`) via DS REST API do not update CRD `.status`

### Mitigations

**For DS availability:**
- `failurePolicy: Fail` ensures no orphaned CRDs
- Operational runbook for troubleshooting (see `docs/operations/runbooks/workflow-registration-runbook.md`)

**For stale status:**
- Documented as accepted trade-off in ADR-058
- `deprecated`/`archived` states are rare admin operations

---

## Related Documents

- **BR-WORKFLOW-004**: Workflow Schema Format Specification (authoritative format)
- **BR-WORKFLOW-006**: RemediationWorkflow CRD Definition (CRD lifecycle and fields)
- **ADR-058**: Webhook-Driven Workflow Registration (AuthWebhook bridge architecture)
- **DD-WORKFLOW-017**: Workflow Lifecycle Component Interactions (end-to-end flow)
- **DD-WORKFLOW-016**: Action-Type Workflow Indexing (discovery matching)
- **DD-WORKFLOW-003**: Parameterized Remediation Actions (parameter naming)
- **DD-WORKFLOW-011**: Tekton OCI Bundles (execution bundle structure)
- **DD-WORKFLOW-001**: Mandatory Label Schema (required labels)
- **DD-NAMING-001**: Workflow Terminology (naming conventions)
- **ADR-041**: LLM Prompt/Response Contract (how LLM uses schema)
- **BR-WE-014**: Kubernetes Job Execution Backend
- **BR-WE-016**: Ansible Engine Config (discriminator pattern)
- **Issue #292**: Schema CRD format migration
- **Issue #299**: CRD-based workflow registration

---

## Approval

**Status**: ✅ Approved
**Date**: 2025-11-28 (original), 2026-03-08 (v1.4)
**Authority**: Authoritative

**Implementation Status**: Complete through Phase 3 (CRD-based registration). See `pkg/datastorage/schema/parser.go` for the authoritative parser implementation.

