# ADR-043: Workflow Schema Definition Standard

**Status**: Approved
**Date**: 2025-11-28
**Deciders**: Architecture Team
**Related**: DD-WORKFLOW-003, DD-WORKFLOW-011, DD-NAMING-001, ADR-041
**Version**: 1.2

---

## Changelog

### Version 1.2 (2026-02-13)
- **BR-WORKFLOW-004**: Removed `apiVersion` and `kind` fields (plain config file, not K8s resource)
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

When operators create remediation workflows as OCI bundles containing Tekton Pipelines, we need to:

1. **Discover workflows** by signal type, severity, and other labels
2. **Guide LLM** with parameter constraints (enum, pattern, min/max)
3. **Validate parameters** before execution
4. **Support future execution engines** (Ansible, Lambda, etc.)

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

### File Location

```
<oci-bundle>/
├── pipeline.yaml           # Tekton Pipeline (execution)
└── workflow-schema.yaml    # Kubernaut Schema (discovery + validation)
```

**Path**: `/workflow-schema.yaml` (root of OCI bundle)
**Format**: YAML 1.2
**Encoding**: UTF-8

### Schema Definition

```yaml
# Kubernaut Workflow Schema v1.0
# Authority: ADR-043
# See: docs/architecture/decisions/ADR-043-workflow-schema-definition-standard.md
# Format spec: docs/requirements/BR-WORKFLOW-004-workflow-schema-format.md

# ============================================
# METADATA (Required)
# ============================================
metadata:
  # Unique workflow identifier (used in catalog and LLM responses)
  # Format: lowercase alphanumeric with hyphens
  # Example: "oomkill-scale-down", "disk-cleanup-v2"
  workflowId: string  # REQUIRED

  # Semantic version (SemVer 2.0)
  # Format: MAJOR.MINOR.PATCH
  # Example: "1.0.0", "2.1.3"
  version: string  # REQUIRED

  # Human-readable description (shown to LLM and operators)
  # Max length: 500 characters
  description: string  # REQUIRED

  # Optional maintainer information
  maintainers:  # OPTIONAL
    - name: string
      email: string

# ============================================
# DISCOVERY LABELS (Required)
# Used by MCP search to match workflows to signals
# ============================================
labels:
  # Signal type this workflow handles
  # Must match DD-WORKFLOW-001 mandatory labels
  signalType: string  # REQUIRED - e.g., "OOMKilled", "CrashLoopBackOff"

  # Severity level this workflow is designed for
  # Values: "critical", "high", "medium", "low"
  severity: string  # REQUIRED

  # DEPRECATED: riskTolerance removed (BR-WORKFLOW-004)

  # Business category for filtering
  # Values: "cost-management", "performance", "availability", "security"
  business_category: string  # OPTIONAL

  # Additional custom labels (operator-defined)
  # Example: "team: platform", "region: us-east-1"
  # No character limits (unlike K8s labels)
  [custom_key]: string  # OPTIONAL

# ============================================
# EXECUTION HINT (Optional for V1, Required for V2)
# Specifies which execution engine runs this workflow
# ============================================
execution:
  # Execution engine type
  # V1 values: "tekton", "job" (Kubernetes Job - per BR-WE-014)
  # V2 values: "tekton", "job", "ansible", "lambda", "shell"
  engine: string  # OPTIONAL (default: "tekton")

  # Container image or bundle reference
  # For Tekton: OCI bundle URL
  # For Ansible: Git repo or container with playbook
  bundle: string  # OPTIONAL (can be derived from OCI bundle)

# ============================================
# PARAMETERS (Required)
# Define inputs for workflow execution
# Format: JSON Schema compatible subset
# ============================================
parameters:
  - name: string  # REQUIRED - Parameter name (UPPER_SNAKE_CASE per DD-WORKFLOW-003)
    type: string  # REQUIRED - "string", "integer", "boolean", "array"
    required: boolean  # REQUIRED - Whether parameter must be provided
    description: string  # REQUIRED - Human-readable description for LLM

    # Validation constraints (all OPTIONAL)
    enum: [string]  # Allowed values (for type: string)
    pattern: string  # Regex pattern (for type: string)
    minimum: number  # Minimum value (for type: integer)
    maximum: number  # Maximum value (for type: integer)
    default: any  # Default value if not provided

    # Parameter dependencies (OPTIONAL)
    # References other parameter names that must be set first
    depends_on: [string]

# ============================================
# ROLLBACK PARAMETERS (Optional)
# Parameters needed to rollback this workflow
# ============================================
rollback_parameters:  # OPTIONAL
  - name: string
    type: string
    required: boolean
    description: string
```

### Complete Example

```yaml
# /workflow-schema.yaml
# OOMKilled Scale Down Workflow - Production Ready

metadata:
  workflowId: oomkill-scale-down
  version: "1.0.0"
  description: >-
    Scale down deployment replicas to reduce memory pressure when OOMKilled
    events are detected. Suitable for non-critical workloads where temporary
    capacity reduction is acceptable.
  maintainers:
    - name: Platform Team
      email: platform@example.com

labels:
  signalType: OOMKilled
  severity: critical
  # DEPRECATED: riskTolerance removed (BR-WORKFLOW-004)
  business_category: cost-management
  team: platform
  environment: production

execution:
  engine: tekton
  bundle: quay.io/kubernaut/workflow-oomkill-scale-down:v1.0.0

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

rollback_parameters:
  - name: ORIGINAL_REPLICAS
    type: integer
    required: true
    description: Original replica count to restore on rollback
```

---

## Validation Rules

### Required Fields Validation

```go
// pkg/workflow/validation/schema_validator.go

func ValidateWorkflowSchema(schema *WorkflowSchema) error {
    var errs []error

    // Metadata validation
    if schema.Metadata.WorkflowID == "" {
        errs = append(errs, errors.New("metadata.workflow_id is required"))
    }
    if !semverRegex.MatchString(schema.Metadata.Version) {
        errs = append(errs, errors.New("metadata.version must be valid semver"))
    }
    if schema.Metadata.Description == "" {
        errs = append(errs, errors.New("metadata.description is required"))
    }

    // Labels validation (DD-WORKFLOW-001 mandatory labels)
    if schema.Labels.SignalType == "" {
        errs = append(errs, errors.New("labels.signal_type is required"))
    }
    if schema.Labels.Severity == "" {
        errs = append(errs, errors.New("labels.severity is required"))
    }
    // DEPRECATED: Labels.RiskTolerance removed per BR-WORKFLOW-004 (never stored in DB)
    if schema.Labels.RiskTolerance == "" {
        errs = append(errs, errors.New("labels.risk_tolerance is required"))
    }

    // Parameters validation
    if len(schema.Parameters) == 0 {
        errs = append(errs, errors.New("at least one parameter is required"))
    }
    for i, param := range schema.Parameters {
        if param.Name == "" {
            errs = append(errs, fmt.Errorf("parameters[%d].name is required", i))
        }
        if !upperSnakeCaseRegex.MatchString(param.Name) {
            errs = append(errs, fmt.Errorf("parameters[%d].name must be UPPER_SNAKE_CASE", i))
        }
        if param.Type == "" {
            errs = append(errs, fmt.Errorf("parameters[%d].type is required", i))
        }
        if param.Description == "" {
            errs = append(errs, fmt.Errorf("parameters[%d].description is required", i))
        }
    }

    if len(errs) > 0 {
        return errors.Join(errs...)
    }
    return nil
}
```

### Parameter Value Validation

```go
// Validate parameter values against schema constraints
func ValidateParameterValue(param ParameterDef, value any) error {
    switch param.Type {
    case "string":
        str, ok := value.(string)
        if !ok {
            return fmt.Errorf("%s: expected string, got %T", param.Name, value)
        }
        if len(param.Enum) > 0 && !slices.Contains(param.Enum, str) {
            return fmt.Errorf("%s: value '%s' not in enum %v", param.Name, str, param.Enum)
        }
        if param.Pattern != "" {
            re := regexp.MustCompile(param.Pattern)
            if !re.MatchString(str) {
                return fmt.Errorf("%s: value '%s' does not match pattern '%s'",
                    param.Name, str, param.Pattern)
            }
        }

    case "integer":
        num, ok := value.(int)
        if !ok {
            return fmt.Errorf("%s: expected integer, got %T", param.Name, value)
        }
        if param.Minimum != nil && num < *param.Minimum {
            return fmt.Errorf("%s: value %d below minimum %d", param.Name, num, *param.Minimum)
        }
        if param.Maximum != nil && num > *param.Maximum {
            return fmt.Errorf("%s: value %d above maximum %d", param.Name, num, *param.Maximum)
        }

    case "boolean":
        if _, ok := value.(bool); !ok {
            return fmt.Errorf("%s: expected boolean, got %T", param.Name, value)
        }
    }

    return nil
}
```

---

## Migration Path

### Phase 1: V1.0 (Immediate)

1. ✅ Create ADR-043 (this document)
2. ⏳ Update DD-WORKFLOW-011 to reference `workflow-schema.yaml`
3. ⏳ Implement schema extractor in `pkg/workflow/extraction/`
4. ⏳ Update workflow registration to validate schema
5. ⏳ Update mock MCP server to use schema labels

### Phase 2: V1.1 (Future)

1. ⏳ Add schema drift detection (compare catalog vs bundle)
2. ⏳ Add JSON Schema generation for IDE support
3. ⏳ Add `kubernaut workflow validate` CLI command

### Phase 3: V2.0 (Multi-Engine Expansion)

1. ⏳ Add `execution.engine: ansible` support (per Issue #45, BR-WE-014 future scope)
2. ⏳ Add `execution.engine: lambda` support
3. ⏳ Schema versioning (apiVersion: kubernaut.io/v2)

**Note**: `execution.engine: "job"` (Kubernetes Job) was added to V1 per BR-WE-014.

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

1. ⚠️ **Two files per workflow** - `pipeline.yaml` + `workflow-schema.yaml`
2. ⚠️ **Potential drift** - Schema could drift from actual pipeline params

### Mitigations

**For two files:**
- Clear documentation and examples
- `kubernaut workflow init` scaffolding tool
- Pre-commit validation hook

**For drift:**
- Validation during registration
- Schema drift detection (V1.1)
- CI/CD pipeline checks

---

## Related Documents

- **DD-WORKFLOW-003**: Parameterized Remediation Actions (parameter naming)
- **DD-WORKFLOW-011**: Tekton OCI Bundles (bundle structure)
- **DD-NAMING-001**: Workflow Terminology (naming conventions)
- **ADR-041**: LLM Prompt/Response Contract (how LLM uses schema)
- **DD-WORKFLOW-001**: Mandatory Label Schema (required labels)
- **BR-WE-014**: Kubernetes Job Execution Backend (adds `"job"` to V1 engine values)
- **Issue #44**: K8s Job execution backend proposal

---

## Approval

**Status**: ✅ Approved
**Date**: 2025-11-28
**Authority**: Authoritative

**Next Steps**:
1. Update DD-WORKFLOW-011 to reference this ADR
2. Implement schema extractor in `pkg/workflow/extraction/`
3. Create example workflow with `workflow-schema.yaml`
4. Update documentation

