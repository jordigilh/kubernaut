# DD-WORKFLOW-007: Manual Workflow Registration - FINAL

> **ARCHIVED** (2026-03-04): This document has been moved to `docs/archived/` because manual registration is now handled via `RemediationWorkflow` CRD. See [ADR-058](../architecture/decisions/ADR-058-webhook-driven-workflow-registration.md) and [BR-WORKFLOW-006](../requirements/BR-WORKFLOW-006-remediation-workflow-crd.md).

**Date**: 2025-11-15
**Status**: **SUPERSEDED** and **ARCHIVED** -- Replaced by CRD-based registration (ADR-058, BR-WORKFLOW-006). CLI deferred to V1.2
**Related**: DD-WORKFLOW-003, DD-WORKFLOW-006, DD-WORKFLOW-008, DD-WORKFLOW-012 (Workflow Immutability)

> **Note**: This document is superseded by DD-WORKFLOW-017, which defines the authoritative V1.0 workflow lifecycle with `action_type`-based design (DD-WORKFLOW-016). The CLI approach described here remains as historical reference for V1.2 planning.

---

## 🔗 **Workflow Immutability Reference**

**CRITICAL**: Workflow registration creates immutable workflow versions.

**Authority**: **DD-WORKFLOW-012: Workflow Immutability Constraints**
- Once registered, workflow content cannot be changed
- To update workflow, register a new version
- Registration extracts immutable schema from container

**Cross-Reference**: All registration operations (CLI, CRD, API) create immutable workflows per DD-WORKFLOW-012.

---

---

## ⚠️ **IMPORTANT: VERSION ROADMAP UPDATE**

**V1.1 Decision**: CRD-based registration (not CLI)
- Operators create `RemediationWorkflow` CRDs
- Workflow Registry Controller watches CRDs
- Controller calls Data Storage REST API

**V1.2 Plan**: CLI registration tool (as alternative to CRD)
- CLI command: `kubernaut workflow register <image>`
- CLI creates RemediationWorkflow CRD automatically

**This document describes the CLI approach planned for v1.2.**

---

## Original Document (CLI Approach - V1.2)

---

## Critical Correction

**User Clarification**:
> "Playbooks must be registered manually. There is no pre-pull or webhook. This is an operation initiated by an operator."

**Impact**: Complete architecture change - operator-initiated, not automated

---

## Revised Requirement

**Manual Registration Flow**:
1. Operator builds workflow container with `/playbook-schema.json`
2. Operator pushes container to registry
3. **Operator manually registers playbook** (CLI command or API call)
4. Platform pulls image, extracts schema, validates
5. Platform updates catalog
6. Workflow ready for use

**Key**: Operator controls when registration happens, not automated webhook.

---

## FINAL Solution: CLI-Initiated Registration with Validation

**Confidence: 99%**

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ 1. Operator Builds & Pushes Container                       │
│    docker build -t playbook-oomkill:v1.0.0 .                │
│    docker push quay.io/kubernaut/playbook-oomkill:v1.0.0    │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ Manual action
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Operator Registers Workflow (MANUAL)                     │
│    kubernaut workflow register \                            │
│      quay.io/kubernaut/playbook-oomkill:v1.0.0              │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ Triggers validation
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Platform Validates & Extracts                            │
│    - Pulls image (validates access) ✅                       │
│    - Extracts /playbook-schema.json                         │
│    - Validates schema format                                │
│    - Records image digest                                   │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ Validation success
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. Platform Updates Catalog                                 │
│    - Stores schema (from container)                         │
│    - Stores image digest                                    │
│    - Marks as validated                                     │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ Registration complete
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 5. Operator Confirmation                                    │
│    Workflow 'oomkill-cost-optimized' registered             │
│    Version: 1.0.0                                           │
│    Image: quay.io/kubernaut/playbook-oomkill:v1.0.0         │
│    Digest: sha256:abc123...                                 │
│    Status: Ready for use                                    │
└─────────────────────────────────────────────────────────────┘
```

---

## Implementation Options

### Option 1: CLI Command ⭐⭐⭐ RECOMMENDED

**Confidence: 99%**

#### CLI Tool

```bash
# Register playbook
kubernaut workflow register quay.io/kubernaut/playbook-oomkill:v1.0.0

# Output:
# Pulling image quay.io/kubernaut/playbook-oomkill:v1.0.0...
# ✓ Image pulled successfully (digest: sha256:abc123...)
# Extracting schema from /playbook-schema.json...
# ✓ Schema extracted (workflow_id: oomkill-cost-optimized, version: 1.0.0)
# Validating schema format...
# ✓ Schema valid
# Updating catalog...
# ✓ Catalog updated
#
# Workflow registered successfully:
#   ID: oomkill-cost-optimized
#   Version: 1.0.0
#   Parameters: 5
#   Labels: signal_type=OOMKilled, severity=high, priority=P1
#   Status: Ready for use
```

#### CLI Implementation (Go)

```go
package cmd

import (
    "context"
    "fmt"

    "github.com/google/go-containerregistry/pkg/crane"
    "github.com/spf13/cobra"
)

var registerCmd = &cobra.Command{
    Use:   "register IMAGE",
    Short: "Register a workflow container",
    Long: `Register a workflow container by pulling the image, extracting the schema,
and updating the workflow catalog.

Example:
  kubernaut workflow register quay.io/kubernaut/playbook-oomkill:v1.0.0`,
    Args: cobra.ExactArgs(1),
    RunE: runRegister,
}

func runRegister(cmd *cobra.Command, args []string) error {
    image := args[0]
    ctx := context.Background()

    // Step 1: Pull image (validates access)
    fmt.Printf("Pulling image %s...\n", image)

    img, err := crane.Pull(image)
    if err != nil {
        return fmt.Errorf("failed to pull image: %w\nCheck:\n  1. Image exists\n  2. Registry credentials configured\n  3. Network connectivity", err)
    }

    digest, err := img.Digest()
    if err != nil {
        return fmt.Errorf("failed to get image digest: %w", err)
    }

    fmt.Printf("✓ Image pulled successfully (digest: %s)\n", digest)

    // Step 2: Extract schema
    fmt.Println("Extracting schema from /playbook-schema.json...")

    schema, err := extractSchema(img)
    if err != nil {
        return fmt.Errorf("failed to extract schema: %w", err)
    }

    fmt.Printf("✓ Schema extracted (workflow_id: %s, version: %s)\n",
        schema.PlaybookID, schema.Version)

    // Step 3: Validate schema
    fmt.Println("Validating schema format...")

    if err := validateSchema(schema); err != nil {
        return fmt.Errorf("schema validation failed: %w", err)
    }

    fmt.Println("✓ Schema valid")

    // Step 4: Update catalog
    fmt.Println("Updating catalog...")

    catalogEntry := &PlaybookCatalogEntry{
        PlaybookID:      schema.PlaybookID,
        Version:         schema.Version,
        ContainerImage:  image,
        ContainerDigest: digest.String(),
        Parameters:      schema.Parameters,
        Labels:          schema.Labels,
        Validated:       true,
        ValidatedAt:     time.Now(),
    }

    if err := updateCatalog(ctx, catalogEntry); err != nil {
        return fmt.Errorf("failed to update catalog: %w", err)
    }

    fmt.Println("✓ Catalog updated")
    fmt.Println()
    fmt.Println("Playbook registered successfully:")
    fmt.Printf("  ID: %s\n", schema.PlaybookID)
    fmt.Printf("  Version: %s\n", schema.Version)
    fmt.Printf("  Parameters: %d\n", len(schema.Parameters))
    fmt.Printf("  Labels: %s\n", formatLabels(schema.Labels))
    fmt.Printf("  Status: Ready for use\n")

    return nil
}

func extractSchema(img v1.Image) (*PlaybookSchema, error) {
    // Export image filesystem
    fs := mutate.Extract(img)

    // Read /playbook-schema.json
    schemaFile, err := fs.Open("/playbook-schema.json")
    if err != nil {
        return nil, fmt.Errorf("schema file not found in container. Ensure /playbook-schema.json exists")
    }
    defer schemaFile.Close()

    var schema PlaybookSchema
    if err := json.NewDecoder(schemaFile).Decode(&schema); err != nil {
        return nil, fmt.Errorf("invalid JSON in schema file: %w", err)
    }

    return &schema, nil
}
```

#### Benefits

1. **Simple** (99%): Single command
2. **Immediate Feedback** (99%): Operator sees results instantly
3. **Error Handling** (98%): Clear error messages
4. **Validates Access** (99%): Pulls image, guarantees availability
5. **Operator Control** (100%): Operator decides when to register

---

### Option 2: kubectl apply (CRD)

**Confidence: 95%**

#### Usage

```bash
# Create PlaybookRegistration resource
cat <<EOF | kubectl apply -f -
apiVersion: kubernaut.io/v1alpha1
kind: PlaybookRegistration
metadata:
  name: oomkill-cost-optimized
  namespace: kubernaut-system
spec:
  containerImage: quay.io/kubernaut/playbook-oomkill:v1.0.0
EOF

# Check status
kubectl get playbookregistration oomkill-cost-optimized -o yaml

# Output:
# status:
#   phase: Ready
#   playbookId: oomkill-cost-optimized
#   version: 1.0.0
#   containerDigest: sha256:abc123...
#   catalogRegistered: true
#   message: "Playbook registered successfully"
```

#### Benefits

1. **Kubernetes-native** (99%): Uses kubectl
2. **Declarative** (98%): YAML-based
3. **Status Tracking** (99%): CR status shows progress
4. **GitOps-friendly** (95%): Can be committed to Git

#### Trade-offs

- Requires operator controller (more complexity)
- Asynchronous (operator must check status)

---

### Option 3: API Call

**Confidence: 92%**

#### Usage

```bash
# Register via API
curl -X POST http://playbook-registry.kubernaut-system.svc:8080/playbooks/register \
  -H "Content-Type: application/json" \
  -d '{
    "container_image": "quay.io/kubernaut/playbook-oomkill:v1.0.0"
  }'

# Response:
# {
#   "status": "success",
#   "workflow_id": "oomkill-cost-optimized",
#   "version": "1.0.0",
#   "container_digest": "sha256:abc123...",
#   "message": "Playbook registered successfully"
# }
```

#### Benefits

1. **Simple** (95%): HTTP API
2. **Scriptable** (98%): Easy to automate
3. **No CLI needed** (90%): Just curl

#### Trade-offs

- Less user-friendly than CLI
- No built-in progress feedback

---

## Comparison Matrix

| Option | Confidence | User Experience | Kubernetes-Native | Recommended |
|--------|-----------|-----------------|-------------------|-------------|
| **CLI Command** | **99%** | ✅ **Excellent** | ⚠️ External tool | ⭐⭐⭐ **BEST** |
| **kubectl apply (CRD)** | 95% | ✅ Good | ✅ **Yes** | ⭐⭐ **STRONG** |
| **API Call** | 92% | ⚠️ Basic | ⚠️ No | ⭐ **GOOD** |

---

## FINAL RECOMMENDATION: CLI Command

**Confidence: 99%**

### Why CLI is Best

1. **Best User Experience** (99%)
   - Single command
   - Immediate feedback
   - Clear error messages
   - Progress indicators

2. **Validates Everything** (99%)
   - Pulls image (validates access)
   - Extracts schema (validates format)
   - Updates catalog (validates registration)

3. **Operator Control** (100%)
   - Operator decides when to register
   - No automatic triggers
   - Explicit action

4. **Simple** (98%)
   - No CRD/operator needed
   - No webhook infrastructure
   - Just a CLI binary

---

## Complete Manual Flow

```
┌─────────────────────────────────────────────────────────────┐
│ 1. Operator: Build Container                                │
│    cd playbooks/oomkill-cost-optimized/                     │
│    docker build -t playbook-oomkill:v1.0.0 .                │
│    (Container includes /playbook-schema.json)               │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Operator: Push to Registry                               │
│    docker push quay.io/kubernaut/playbook-oomkill:v1.0.0    │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Operator: Register Workflow (MANUAL)                     │
│    kubernaut workflow register \                            │
│      quay.io/kubernaut/playbook-oomkill:v1.0.0              │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. Platform: Pull & Validate                                │
│    - Pulls image (validates access) ✅                       │
│    - Extracts /playbook-schema.json                         │
│    - Validates schema format                                │
│    - Records digest                                         │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 5. Platform: Update Catalog                                 │
│    - Stores schema (from container)                         │
│    - Stores image digest                                    │
│    - Marks as validated                                     │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 6. Operator: Confirmation                                   │
│    ✓ Workflow registered successfully                       │
│    Ready for use by LLM                                     │
└─────────────────────────────────────────────────────────────┘
```

---

## Key Benefits of Manual Registration

### 1. **Operator Control** (100% confidence)
- Operator decides when workflow is ready
- Can test locally before registering
- No surprise registrations
- Explicit approval step

### 2. **Validates Access at Registration** (99% confidence)
- Pulls image during registration
- Fails fast if credentials wrong
- Guarantees image available for execution
- No surprises during incidents

### 3. **Single Source of Truth** (99% confidence)
- Container has schema
- Platform extracts during registration
- Catalog derived from container
- No drift possible

### 4. **Simple** (98% confidence)
- No webhook infrastructure
- No automated triggers
- No CRD/operator (for CLI option)
- Just a command

---

## Schema Drift Prevention (Maintained)

**Manual registration maintains drift prevention**:

1. Container has `/playbook-schema.json` (ONLY source)
2. Operator runs `kubernaut workflow register <image>`
3. Platform pulls image, extracts schema
4. Catalog updated with extracted schema
5. Catalog schema = Container schema (no drift)

**At execution time**:
- LLM uses catalog schema
- Tekton pulls container (same image)
- Container has same schema (guaranteed)
- No drift possible

---

## Failure Scenarios Prevented

### Scenario 1: Image Not Accessible
**Registration**:
```
$ kubernaut workflow register quay.io/private/playbook:v1
Pulling image quay.io/private/playbook:v1...
✗ Failed to pull image: unauthorized
Check:
  1. Image exists
  2. Registry credentials configured
  3. Network connectivity

Registration failed ❌
```

**Operator**: Fixes credentials, re-runs command
**Result**: Registration succeeds only when image accessible

### Scenario 2: Missing Schema
**Registration**:
```
$ kubernaut workflow register quay.io/kubernaut/playbook:v1
Pulling image...
✓ Image pulled
Extracting schema from /playbook-schema.json...
✗ Schema file not found in container

Ensure /playbook-schema.json exists in container.
See: https://docs.kubernaut.io/playbooks/schema

Registration failed ❌
```

**Operator**: Adds schema to Dockerfile, rebuilds, re-runs
**Result**: Registration succeeds only with valid schema

### Scenario 3: Invalid Schema Format
**Registration**:
```
$ kubernaut workflow register quay.io/kubernaut/playbook:v1
Pulling image...
✓ Image pulled
Extracting schema...
✓ Schema extracted
Validating schema format...
✗ Schema validation failed: missing required field 'workflow_id'

Registration failed ❌
```

**Operator**: Fixes schema, rebuilds, re-runs
**Result**: Registration succeeds only with valid schema

---

## CLI Commands

```bash
# Register playbook
kubernaut workflow register <image>

# List registered playbooks
kubernaut workflow list

# Get workflow details
kubernaut workflow get <playbook-id>

# Update workflow (re-register with new version)
kubernaut workflow register <image> --force

# Delete playbook
kubernaut workflow delete <playbook-id>

# Validate workflow without registering
kubernaut workflow validate <image>
```

---

## Confidence Assessment

### CLI Command Approach: 99%

**Strengths**:
1. **Operator Control** (100%): Manual, explicit
2. **Access Validation** (99%): Pulls image during registration
3. **Schema Extraction** (99%): From container (single source)
4. **Drift Prevention** (100%): Catalog derived from container
5. **User Experience** (99%): Simple, clear feedback

**Gap to 100% (1% risk)**:
- **Concurrent Registrations** (1%): Two operators register same workflow simultaneously
- **Mitigation**: Optimistic locking in catalog (version check)
- **Confidence after mitigation**: 99.5%

---

## Summary

**Problem**: Playbooks must be registered manually by operators

**Solution**: CLI command that pulls image, extracts schema, updates catalog

**Key Points**:
1. ✅ Operator-initiated (manual, not automated)
2. ✅ Validates access (pulls image during registration)
3. ✅ Single source of truth (container schema)
4. ✅ No drift (catalog derived from container)
5. ✅ Simple (single command)

**Confidence**: 99%
**Risk**: Very Low
**User Experience**: Excellent

---

**Status**: Final Decision
**Recommended**: CLI Command (`kubernaut workflow register`)
**Confidence**: 99%
**Ready for Implementation**: ✅ Yes
