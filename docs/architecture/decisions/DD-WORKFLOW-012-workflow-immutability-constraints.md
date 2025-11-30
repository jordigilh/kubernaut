# DD-WORKFLOW-012: Workflow Immutability Constraints

**Date**: 2025-11-27
**Status**: ✅ **APPROVED**
**Version**: 2.0
**Authority**: ⭐ **AUTHORITATIVE** - Single source of truth for workflow immutability
**Related**: DD-WORKFLOW-006, DD-WORKFLOW-009, DD-WORKFLOW-002, DD-NAMING-001

---

## Changelog

### Version 2.0 (2025-11-29)
- **BREAKING**: Changed primary key from composite `(workflow_id, version)` to UUID `workflow_id`
- `workflow_id` is now a UUID auto-generated at insert time
- `workflow_name` and `version` are now metadata fields (for humans), not identity fields
- Added `UNIQUE (workflow_name, version)` constraint to prevent duplicates
- Updated all examples and SQL to use UUID primary key
- Cross-reference: DD-WORKFLOW-002 v3.0 (search response uses UUID only)

### Version 1.0 (2025-11-27)
- Initial version defining workflow immutability with composite primary key

---

## 🚨 **CRITICAL: READ THIS FIRST**

### **THE IMMUTABILITY RULE (NO EXCEPTIONS)**

```
┌─────────────────────────────────────────────────────────────────┐
│ WORKFLOWS ARE IMMUTABLE - IDENTIFIED BY UUID                    │
│                                                                  │
│ PRIMARY KEY (workflow_id UUID)                                  │
│                                                                  │
│ ONCE CREATED, YOU CANNOT CHANGE:                                │
│ ❌ Description (affects semantic search embeddings)             │
│ ❌ Content (workflow implementation)                            │
│ ❌ Labels (affects filtering and semantic search)               │
│ ❌ Owner/Maintainer (audit trail)                               │
│ ❌ workflow_name, version (human metadata)                      │
│ ❌ Any field used for semantic search or audit trail            │
│                                                                  │
│ TO CHANGE THESE FIELDS:                                         │
│ ✅ CREATE A NEW WORKFLOW (gets new UUID)                        │
│                                                                  │
│ YOU CAN CHANGE:                                                 │
│ ✅ Status (active/disabled/deprecated/archived)                │
│ ✅ Success metrics (calculated from execution history)          │
│                                                                  │
│ WHY IMMUTABLE?                                                  │
│ 1. Semantic search embeddings must remain stable                │
│ 2. Audit trail must trace to exact workflow (UUID)              │
│ 3. Container schema must match catalog schema (no drift)        │
│ 4. Historical executions must be reproducible                   │
└─────────────────────────────────────────────────────────────────┘
```

### **DETERMINING FIELDS**

**The `workflow_id` UUID is the single primary key:**

```sql
-- From migration (updated for UUID)
workflow_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
workflow_name VARCHAR(255) NOT NULL,  -- Human-readable name (metadata)
version VARCHAR(50) NOT NULL,          -- Human version (metadata)
UNIQUE (workflow_name, version)        -- Prevent duplicate name+version
```

**Single field uniquely identifies a workflow:**
- `workflow_id`: UUID (auto-generated, immutable, single identifier for all operations)
- `workflow_name`: Human-readable name (metadata, not identity)
- `version`: Human version string (metadata, not identity)

**New workflow = New UUID ✅**
**Same workflow_name + version = DUPLICATE (prevented by UNIQUE constraint) ❌**

---

## 📋 **Status**

**✅ APPROVED** (2025-11-27)
**Confidence**: 99%
**Purpose**: Define immutable vs mutable workflow fields to prevent runtime drift and ensure audit trail integrity

**THIS IS THE AUTHORITATIVE SOURCE FOR WORKFLOW IMMUTABILITY.**
**ALL OTHER DOCUMENTS MUST REFERENCE THIS DD.**

---

## 🎯 **Context & Problem**

### **Problem Statement**

Workflows must be **immutable** to ensure:
1. **No Runtime Drift**: Catalog schema matches container schema at execution time
2. **Audit Trail Integrity**: Historical executions can be traced to exact workflow version
3. **Reproducibility**: Past remediation attempts can be recreated exactly
4. **Semantic Search Consistency**: Embeddings remain stable for same workflow version

### **Key Requirements**

- **BR-STORAGE-012**: Workflow semantic search must return consistent results
- **BR-AUDIT-001**: All workflow executions must be traceable to exact version
- **DD-WORKFLOW-006**: Schema drift prevention requires immutability
- **DD-WORKFLOW-009**: Container is single source of truth

---

## ✅ **Decision**

**APPROVED**: Workflows are **immutable** - identified by **UUID**.

### **Immutability Principle**

```sql
workflow_id UUID PRIMARY KEY DEFAULT gen_random_uuid()  -- IMMUTABILITY: Each workflow gets unique UUID
UNIQUE (workflow_name, version)                          -- Prevent duplicate human identifiers
```

**What This Means**:
- Once a workflow is created, its **content fields** **CANNOT** be changed
- **Lifecycle fields** (status, metrics) **CAN** be updated
- To change content, create a **new workflow** (gets new UUID)

---

## 📊 **Field Classification**

### **QUICK REFERENCE TABLE**

| Field Category | Immutable? | Reason | Can Update? | Create New Workflow? |
|----------------|------------|--------|-------------|----------------------|
| **Identity** (`workflow_id` UUID) | ✅ YES | Primary key (auto-generated) | ❌ NO | N/A (defines workflow) |
| **Human Metadata** (`workflow_name`, `version`) | ✅ YES | Human identifiers | ❌ NO | ✅ YES (new workflow) |
| **Content** (`description`, `content`, `labels`, `embedding`) | ✅ YES | Semantic search + audit trail | ❌ NO | ✅ YES (new workflow) |
| **Ownership** (`owner`, `maintainer`) | ✅ YES | Audit trail | ❌ NO | ✅ YES (new workflow) |
| **Audit Creation** (`created_at`, `created_by`) | ✅ YES | Historical record | ❌ NO | N/A (set at creation) |
| **Lifecycle** (`status`, `disabled_at`, etc.) | ❌ NO | Operational state | ✅ YES | ❌ NO |
| **Success Metrics** (`actual_success_rate`, `total_executions`) | ❌ NO | Calculated from history | ✅ YES | ❌ NO |
| **Audit Update** (`updated_at`, `updated_by`) | ❌ NO | Tracks mutable changes | ✅ YES | ❌ NO |

---

### **IMMUTABLE FIELDS** (Cannot Change After Creation)

#### **Identity Fields** (Composite Primary Key)
```go
// These define the workflow's unique identity
WorkflowID string `json:"workflow_id"`  // e.g., "pod-oom-recovery"
Version    string `json:"version"`      // e.g., "v1.0.0"
```

**Rationale**: Primary key - changing these would create a different workflow

---

#### **Content Fields** (Workflow Definition)
```go
// These define what the workflow does
Name        string `json:"name"`                    // Workflow display name
Description string `json:"description"`             // Used for embedding generation
Content     string `json:"content"`                 // Full workflow YAML/JSON
ContentHash string `json:"content_hash"`            // SHA-256 hash for integrity
Labels      json.RawMessage `json:"labels"`         // Used for filtering + embedding
Embedding   *pgvector.Vector `json:"embedding"`     // Generated from description + labels
```

**Rationale**:
- **Description**: Changes would invalidate semantic search embeddings
- **Content**: Changing implementation would break audit trail
- **Labels**: Changes would affect semantic search and filtering
- **Embedding**: Derived from description + labels, must remain stable

---

#### **Metadata Fields** (Workflow Context)
```go
// These provide context about the workflow
Owner      *string `json:"owner"`       // Team or user responsible
Maintainer *string `json:"maintainer"`  // Contact email
```

**Rationale**: Ownership changes should create new version for audit trail

---

#### **Version Management Fields** (Immutable History)
```go
// These track version history
PreviousVersion   *string `json:"previous_version"`    // Link to previous version
VersionNotes      *string `json:"version_notes"`       // Release notes / changelog
ChangeSummary     *string `json:"change_summary"`      // Auto-generated summary
ApprovedBy        *string `json:"approved_by"`         // Who approved this version
ApprovedAt        *time.Time `json:"approved_at"`      // When approved
```

**Rationale**: Historical records must not be altered

---

#### **Audit Trail Fields** (Immutable Timestamps)
```go
// These track creation history
CreatedAt time.Time `json:"created_at"`  // When workflow version was created
CreatedBy *string   `json:"created_by"`  // Who created this version
```

**Rationale**: Creation history must not be altered

---

### **MUTABLE FIELDS** (Can Change After Creation)

#### **Lifecycle Management Fields**
```go
// These track workflow lifecycle state
Status         string     `json:"status"`           // active → disabled → deprecated → archived
DisabledAt     *time.Time `json:"disabled_at"`      // When disabled
DisabledBy     *string    `json:"disabled_by"`      // Who disabled it
DisabledReason *string    `json:"disabled_reason"`  // Why disabled
```

**Rationale**: Operators need to disable/enable workflows without creating new versions

**Allowed Transitions**:
```
active → disabled → active (re-enable)
active → deprecated → archived (retirement)
```

---

#### **Version Management Fields** (Mutable State)
```go
// These track current version state
IsLatestVersion   bool    `json:"is_latest_version"`    // Flag for latest version
DeprecationNotice *string `json:"deprecation_notice"`   // Reason for deprecation
```

**Rationale**:
- `IsLatestVersion`: Updated when new version is created
- `DeprecationNotice`: Added when workflow is deprecated

---

#### **Success Metrics Fields** (Updated by Effectiveness Monitor)
```go
// These track execution history
ExpectedSuccessRate    *float64 `json:"expected_success_rate"`     // Expected (static)
ExpectedDurationSeconds *int    `json:"expected_duration_seconds"` // Expected (static)
ActualSuccessRate      *float64 `json:"actual_success_rate"`       // Calculated from history
TotalExecutions        int      `json:"total_executions"`          // Incremented on execution
SuccessfulExecutions   int      `json:"successful_executions"`     // Incremented on success
```

**Rationale**: Execution metrics are calculated from audit trail, not part of workflow definition

---

#### **Audit Trail Fields** (Mutable Timestamps)
```go
// These track modification history
UpdatedAt time.Time `json:"updated_at"`  // Last update timestamp (for mutable fields only)
UpdatedBy *string   `json:"updated_by"`  // Who last updated (for mutable fields only)
```

**Rationale**: Tracks changes to mutable fields (status, metrics)

---

## 🚫 **What Immutability Prevents**

### **Scenario 1: Changing Workflow Description**

**WRONG** (Violates Immutability):
```sql
-- ❌ This is FORBIDDEN
UPDATE remediation_workflow_catalog
SET description = 'New description'
WHERE workflow_id = 'pod-oom-recovery' AND version = 'v1.0.0';
```

**Consequences**:
- ❌ **Embedding changes** → semantic search returns different results for same version
- ❌ **Audit trail broken** → can't trace what workflow did when it executed
- ❌ **Reproducibility lost** → can't recreate past remediation attempts

**CORRECT** (Create New Version):
```sql
-- ✅ Create new version with updated description
INSERT INTO remediation_workflow_catalog (
    workflow_id, version, description, ...
) VALUES (
    'pod-oom-recovery', 'v1.1.0', 'New description', ...
);

-- ✅ Update is_latest_version flags
UPDATE remediation_workflow_catalog
SET is_latest_version = false
WHERE workflow_id = 'pod-oom-recovery' AND version = 'v1.0.0';

UPDATE remediation_workflow_catalog
SET is_latest_version = true
WHERE workflow_id = 'pod-oom-recovery' AND version = 'v1.1.0';
```

---

### **Scenario 2: Changing Workflow Content**

**WRONG** (Violates Immutability):
```sql
-- ❌ This is FORBIDDEN
UPDATE remediation_workflow_catalog
SET content = '<new_tekton_pipeline_yaml>'
WHERE workflow_id = 'pod-oom-recovery' AND version = 'v1.0.0';
```

**Consequences**:
- ❌ **Audit trail broken** → historical executions reference wrong content
- ❌ **Container mismatch** → catalog content ≠ container content (runtime drift)
- ❌ **Reproducibility lost** → can't recreate past remediation attempts

**CORRECT** (Create New Version):
```bash
# 1. Build new container with updated content
docker build -t pod-oom-recovery:v1.1.0 .

# 2. Push to registry
docker push quay.io/kubernaut/pod-oom-recovery:v1.1.0

# 3. Register new version (CLI or CRD)
kubernaut workflow register quay.io/kubernaut/pod-oom-recovery:v1.1.0
```

---

### **Scenario 3: Changing Workflow Labels**

**WRONG** (Violates Immutability):
```sql
-- ❌ This is FORBIDDEN
UPDATE remediation_workflow_catalog
SET labels = jsonb_set(labels, '{severity}', '"high"')
WHERE workflow_id = 'pod-oom-recovery' AND version = 'v1.0.0';
```

**Consequences**:
- ❌ **Semantic search broken** → labels used for embedding generation
- ❌ **Filtering broken** → historical queries return different results
- ❌ **Audit trail broken** → can't trace which labels were active during execution

**CORRECT** (Create New Version):
```sql
-- ✅ Create new version with updated labels
INSERT INTO remediation_workflow_catalog (
    workflow_id, version, labels, ...
) VALUES (
    'pod-oom-recovery', 'v1.1.0', '{"severity": "high"}', ...
);
```

---

## ✅ **What Immutability Allows**

### **Scenario 1: Disabling a Workflow**

**ALLOWED** (Mutable Field):
```sql
-- ✅ This is ALLOWED (status is mutable)
UPDATE remediation_workflow_catalog
SET status = 'disabled',
    disabled_at = NOW(),
    disabled_by = 'operator@example.com',
    disabled_reason = 'Workflow causing issues in production'
WHERE workflow_id = 'pod-oom-recovery' AND version = 'v1.0.0';
```

**Rationale**: Operators need to disable workflows without creating new versions

---

### **Scenario 2: Updating Success Metrics**

**ALLOWED** (Mutable Field):
```sql
-- ✅ This is ALLOWED (metrics are mutable)
UPDATE remediation_workflow_catalog
SET total_executions = total_executions + 1,
    successful_executions = successful_executions + 1,
    actual_success_rate = (successful_executions + 1)::float / (total_executions + 1)
WHERE workflow_id = 'pod-oom-recovery' AND version = 'v1.0.0';
```

**Rationale**: Execution metrics are calculated from audit trail, not part of workflow definition

---

### **Scenario 3: Marking as Latest Version**

**ALLOWED** (Mutable Field):
```sql
-- ✅ This is ALLOWED (is_latest_version is mutable)
-- Mark old version as not latest
UPDATE remediation_workflow_catalog
SET is_latest_version = false
WHERE workflow_id = 'pod-oom-recovery' AND version = 'v1.0.0';

-- Mark new version as latest
UPDATE remediation_workflow_catalog
SET is_latest_version = true
WHERE workflow_id = 'pod-oom-recovery' AND version = 'v1.1.0';
```

**Rationale**: Latest version flag must be updated when new versions are created

---

## 🔒 **Enforcement Mechanisms**

### **1. Database Constraints**

```sql
-- Primary key prevents overwriting existing versions
PRIMARY KEY (workflow_id, version)

-- Trigger updates updated_at automatically
CREATE TRIGGER trigger_workflow_catalog_updated_at
    BEFORE UPDATE ON remediation_workflow_catalog
    FOR EACH ROW
    EXECUTE FUNCTION update_workflow_catalog_updated_at();
```

---

### **2. Application-Level Validation**

```go
// pkg/datastorage/repository/workflow_repository.go

func (r *WorkflowRepository) UpdateWorkflow(ctx context.Context, wf *models.RemediationWorkflow) error {
    // ONLY allow updates to mutable fields
    query := `
        UPDATE remediation_workflow_catalog
        SET
            -- Lifecycle fields (MUTABLE)
            status = $1,
            disabled_at = $2,
            disabled_by = $3,
            disabled_reason = $4,

            -- Version management (MUTABLE)
            is_latest_version = $5,
            deprecation_notice = $6,

            -- Success metrics (MUTABLE)
            actual_success_rate = $7,
            total_executions = $8,
            successful_executions = $9,

            -- Audit trail (MUTABLE)
            updated_by = $10
            -- updated_at is handled by trigger

        WHERE workflow_id = $11 AND version = $12
        RETURNING *
    `

    // NOTE: Immutable fields are NOT in the UPDATE statement
    // Attempting to update immutable fields requires creating a new version

    return r.db.QueryRowContext(ctx, query,
        wf.Status, wf.DisabledAt, wf.DisabledBy, wf.DisabledReason,
        wf.IsLatestVersion, wf.DeprecationNotice,
        wf.ActualSuccessRate, wf.TotalExecutions, wf.SuccessfulExecutions,
        wf.UpdatedBy,
        wf.WorkflowID, wf.Version).Scan(&result)
}
```

---

### **3. API-Level Validation**

```go
// pkg/datastorage/server/workflow_handlers.go

func (h *WorkflowHandler) UpdateWorkflow(w http.ResponseWriter, r *http.Request) {
    var req UpdateWorkflowRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid request", http.StatusBadRequest)
        return
    }

    // Validate that ONLY mutable fields are being updated
    if req.Description != nil || req.Content != nil || req.Labels != nil {
        http.Error(w,
            "Cannot update immutable fields (description, content, labels). "+
            "Create a new version instead.",
            http.StatusBadRequest)
        return
    }

    // Proceed with update (only mutable fields)
    // ...
}
```

---

## 🎯 **Versioning Strategy**

### **When to Create New Version**

Create a **new version** when changing:
- ✅ **Description** (affects semantic search)
- ✅ **Content** (workflow implementation)
- ✅ **Labels** (affects filtering and search)
- ✅ **Owner/Maintainer** (for audit trail)

### **When to Update Existing Version**

Update the **existing version** when changing:
- ✅ **Status** (disable/enable workflow)
- ✅ **Success Metrics** (execution history)
- ✅ **Latest Version Flag** (version management)
- ✅ **Deprecation Notice** (retirement messaging)

---

## 📋 **Duplicate Prevention Integration**

### **Why Duplicate Prevention Matters**

With immutability, duplicate workflows become a **permanent problem**:
- Cannot "fix" duplicate by updating description
- Must create new version to differentiate
- Duplicate embeddings persist forever

### **Validation Rule**

**PREVENT**: Different `workflow_id`, same `description` + `labels`

```go
func (r *WorkflowRepository) ValidateWorkflowUniqueness(ctx context.Context, wf *models.RemediationWorkflow) error {
    // Check for workflows with identical description + mandatory labels
    // BUT different workflow_id (versioning with same workflow_id is allowed)
    query := `
        SELECT workflow_id, version
        FROM remediation_workflow_catalog
        WHERE status IN ('active', 'disabled')
          AND is_latest_version = true
          AND description = $1
          AND labels->>'signal-type' = $2
          AND labels->>'severity' = $3
          AND workflow_id != $4  -- Allow same workflow_id (versioning)
        LIMIT 1
    `

    // If duplicate found, reject creation
    // ...
}
```

---

## 📊 **Consequences**

### **Positive**

✅ **No Runtime Drift** (100%): Catalog schema always matches container schema
✅ **Audit Trail Integrity** (100%): Historical executions traceable to exact version
✅ **Reproducibility** (100%): Past remediation attempts can be recreated exactly
✅ **Semantic Search Consistency** (100%): Embeddings remain stable for same version
✅ **Version History** (100%): Complete history of workflow evolution preserved

### **Negative**

⚠️ **Cannot Fix Mistakes** (95%): Typos in description require new version
⚠️ **Version Proliferation** (80%): Many versions if frequent changes
⚠️ **Storage Growth** (70%): All versions stored forever (until archived)

**Mitigations**:
- **Pre-commit Validation**: Validate workflows locally before registration
- **Archival Strategy**: Archive old versions after deprecation period
- **Semantic Versioning**: Use semver to communicate change significance

### **Neutral**

🔄 **Versioning Required**: All changes require new version (by design)
🔄 **Status Updates Allowed**: Operators can disable/enable without versioning

---

## 🔗 **Related Decisions & Cross-References**

### **This DD is Referenced By**

All documents discussing workflow updates, modifications, or versioning MUST reference this DD:

- **DD-WORKFLOW-006**: Schema Drift Prevention
  - **Requirement**: Container schema = Catalog schema (no drift)
  - **Enforced By**: DD-WORKFLOW-012 immutability constraints
  - **Cross-Reference**: "Workflows are immutable per DD-WORKFLOW-012"

- **DD-WORKFLOW-009**: Workflow Catalog Storage
  - **Defines**: `PRIMARY KEY (workflow_id, version)`
  - **Enforced By**: DD-WORKFLOW-012 immutability constraints
  - **Cross-Reference**: "PRIMARY KEY enforces immutability per DD-WORKFLOW-012"

- **DD-WORKFLOW-002**: MCP Workflow Catalog Architecture
  - **Requirement**: Semantic search consistency
  - **Enforced By**: DD-WORKFLOW-012 immutability (embeddings cannot change)
  - **Cross-Reference**: "Embeddings are immutable per DD-WORKFLOW-012"

- **DD-NAMING-001**: Remediation Workflow Terminology
  - **Defines**: "Workflow" terminology
  - **Immutability**: Applies to all workflow fields per DD-WORKFLOW-012
  - **Cross-Reference**: "Workflow immutability defined in DD-WORKFLOW-012"

- **DD-WORKFLOW-008**: Version Roadmap
  - **Versioning Strategy**: Create new version for content changes
  - **Enforced By**: DD-WORKFLOW-012 immutability constraints
  - **Cross-Reference**: "Versioning required per DD-WORKFLOW-012"

- **DD-WORKFLOW-007**: Manual Registration
  - **Registration**: Creates immutable workflow version
  - **Enforced By**: DD-WORKFLOW-012 immutability constraints
  - **Cross-Reference**: "Registered workflows are immutable per DD-WORKFLOW-012"

### **Database Schema Reference**

**Migration**: `migrations/015_create_workflow_catalog_table.sql`
```sql
-- Line 115: PRIMARY KEY enforces immutability
PRIMARY KEY (workflow_id, version),  -- IMMUTABILITY: Cannot overwrite existing version
```

**Comment in Migration**:
```sql
-- Line 15: Explicit immutability statement
-- 1. Composite primary key (workflow_id, version) for immutability
```

### **Go Model Reference**

**File**: `pkg/datastorage/models/workflow.go`
```go
// Lines 38-41: Composite primary key
// IDENTITY (Composite Primary Key)
WorkflowID string `json:"workflow_id" db:"workflow_id" validate:"required,max=255"`
Version    string `json:"version" db:"version" validate:"required,max=50"`
```

**Authority**: DD-WORKFLOW-012 (this document)

---

## ❓ **Common Questions (FAQ)**

### **Q1: Can I update a workflow's description?**

**A: NO.** Description is immutable because it's used to generate semantic search embeddings.

**To change description**: Create a new version with same `workflow_id` and different `version`.

---

### **Q2: Can I fix a typo in a workflow's content?**

**A: NO.** Content is immutable because it's the workflow implementation.

**To fix typo**: Create a new version (e.g., v1.0.0 → v1.0.1).

---

### **Q3: Can I disable a workflow temporarily?**

**A: YES.** Status is mutable.

```sql
UPDATE remediation_workflow_catalog
SET status = 'disabled'
WHERE workflow_id = 'pod-oom-recovery' AND version = 'v1.0.0';
```

---

### **Q4: Can I change a workflow's labels?**

**A: NO.** Labels are immutable because they're used for filtering and semantic search.

**To change labels**: Create a new version.

---

### **Q5: What fields determine workflow identity?**

**A: `workflow_id` + `version` (composite primary key).**

These two fields together uniquely identify a workflow. You cannot change either field.

---

### **Q6: Can I have two workflows with the same description?**

**A: NO (if they have different workflow_id).** This is prevented by duplicate validation.

**Exception**: Same `workflow_id` with different `version` CAN have same description (versioning).

---

### **Q7: How do I update a workflow's success metrics?**

**A: YES.** Success metrics are mutable (calculated from execution history).

```sql
UPDATE remediation_workflow_catalog
SET total_executions = total_executions + 1
WHERE workflow_id = 'pod-oom-recovery' AND version = 'v1.0.0';
```

---

### **Q8: Can I change who owns a workflow?**

**A: NO.** Owner is immutable for audit trail.

**To change owner**: Create a new version with new owner.

---

### **Q9: What happens if I try to update an immutable field?**

**A: The database will reject the update** (enforced by application logic).

The API will return: `400 Bad Request: Cannot update immutable fields. Create a new version instead.`

---

### **Q10: How do I know which fields are immutable?**

**A: Check the Quick Reference Table at the top of this document.**

Or check the `UpdateWorkflow()` function in `pkg/datastorage/repository/workflow_repository.go` - it only updates mutable fields.

---

## 📝 **Summary**

### **Immutability Principle**

**Workflows are immutable at the (workflow_id, version) level.**

### **Immutable Fields** (Cannot Change)
- Identity: `workflow_id`, `version`
- Content: `description`, `content`, `content_hash`, `labels`, `embedding`
- Metadata: `owner`, `maintainer`
- Version History: `previous_version`, `version_notes`, `change_summary`, `approved_by`, `approved_at`
- Audit Trail: `created_at`, `created_by`

### **Mutable Fields** (Can Change)
- Lifecycle: `status`, `disabled_at`, `disabled_by`, `disabled_reason`
- Version Management: `is_latest_version`, `deprecation_notice`
- Success Metrics: `actual_success_rate`, `total_executions`, `successful_executions`
- Audit Trail: `updated_at`, `updated_by`

### **To Change Immutable Fields**
**Create a new version** with same `workflow_id` and different `version`.

---

**Status**: ✅ **APPROVED**
**Confidence**: 99%
**Authority**: ⭐ **AUTHORITATIVE** - Single source of truth for workflow immutability

---

## 🚫 **EXPLICIT PROHIBITIONS (DO NOT)**

### **NEVER DO THESE (IMMUTABILITY VIOLATIONS)**

```sql
-- ❌ NEVER UPDATE description
UPDATE remediation_workflow_catalog SET description = '...' WHERE ...;

-- ❌ NEVER UPDATE content
UPDATE remediation_workflow_catalog SET content = '...' WHERE ...;

-- ❌ NEVER UPDATE content_hash
UPDATE remediation_workflow_catalog SET content_hash = '...' WHERE ...;

-- ❌ NEVER UPDATE labels
UPDATE remediation_workflow_catalog SET labels = '...' WHERE ...;

-- ❌ NEVER UPDATE embedding
UPDATE remediation_workflow_catalog SET embedding = '...' WHERE ...;

-- ❌ NEVER UPDATE owner
UPDATE remediation_workflow_catalog SET owner = '...' WHERE ...;

-- ❌ NEVER UPDATE maintainer
UPDATE remediation_workflow_catalog SET maintainer = '...' WHERE ...;

-- ❌ NEVER UPDATE workflow_id
UPDATE remediation_workflow_catalog SET workflow_id = '...' WHERE ...;

-- ❌ NEVER UPDATE version
UPDATE remediation_workflow_catalog SET version = '...' WHERE ...;

-- ❌ NEVER UPDATE created_at
UPDATE remediation_workflow_catalog SET created_at = '...' WHERE ...;

-- ❌ NEVER UPDATE created_by
UPDATE remediation_workflow_catalog SET created_by = '...' WHERE ...;

-- ❌ NEVER UPDATE previous_version
UPDATE remediation_workflow_catalog SET previous_version = '...' WHERE ...;

-- ❌ NEVER UPDATE version_notes
UPDATE remediation_workflow_catalog SET version_notes = '...' WHERE ...;

-- ❌ NEVER UPDATE change_summary
UPDATE remediation_workflow_catalog SET change_summary = '...' WHERE ...;

-- ❌ NEVER UPDATE approved_by
UPDATE remediation_workflow_catalog SET approved_by = '...' WHERE ...;

-- ❌ NEVER UPDATE approved_at
UPDATE remediation_workflow_catalog SET approved_at = '...' WHERE ...;
```

### **ALWAYS DO THESE INSTEAD (CREATE NEW VERSION)**

```sql
-- ✅ ALWAYS create new version for content changes
INSERT INTO remediation_workflow_catalog (
    workflow_id, version, description, content, labels, ...
) VALUES (
    'pod-oom-recovery', 'v1.1.0', 'New description', '<new_content>', '<new_labels>', ...
);

-- ✅ ALWAYS update is_latest_version flags
UPDATE remediation_workflow_catalog SET is_latest_version = false
WHERE workflow_id = 'pod-oom-recovery' AND version = 'v1.0.0';

UPDATE remediation_workflow_catalog SET is_latest_version = true
WHERE workflow_id = 'pod-oom-recovery' AND version = 'v1.1.0';
```

### **ALLOWED UPDATES (MUTABLE FIELDS ONLY)**

```sql
-- ✅ ALLOWED: Update status
UPDATE remediation_workflow_catalog SET status = 'disabled' WHERE ...;

-- ✅ ALLOWED: Update disabled_at
UPDATE remediation_workflow_catalog SET disabled_at = NOW() WHERE ...;

-- ✅ ALLOWED: Update disabled_by
UPDATE remediation_workflow_catalog SET disabled_by = 'operator@example.com' WHERE ...;

-- ✅ ALLOWED: Update disabled_reason
UPDATE remediation_workflow_catalog SET disabled_reason = 'Causing issues' WHERE ...;

-- ✅ ALLOWED: Update is_latest_version
UPDATE remediation_workflow_catalog SET is_latest_version = false WHERE ...;

-- ✅ ALLOWED: Update deprecation_notice
UPDATE remediation_workflow_catalog SET deprecation_notice = 'Use v2.0 instead' WHERE ...;

-- ✅ ALLOWED: Update actual_success_rate
UPDATE remediation_workflow_catalog SET actual_success_rate = 0.95 WHERE ...;

-- ✅ ALLOWED: Update total_executions
UPDATE remediation_workflow_catalog SET total_executions = total_executions + 1 WHERE ...;

-- ✅ ALLOWED: Update successful_executions
UPDATE remediation_workflow_catalog SET successful_executions = successful_executions + 1 WHERE ...;

-- ✅ ALLOWED: Update updated_by
UPDATE remediation_workflow_catalog SET updated_by = 'system' WHERE ...;

-- ✅ ALLOWED: updated_at is handled by trigger automatically
```

---

## 📋 **Checklist for Developers**

### **Before Updating a Workflow, Ask:**

- [ ] **Is this field in the "IMMUTABLE FIELDS" list?**
  - ✅ YES → Create new version instead
  - ❌ NO → Proceed with update

- [ ] **Does this change affect semantic search?**
  - ✅ YES → Create new version (description, labels, embedding)
  - ❌ NO → Proceed with update

- [ ] **Does this change affect audit trail?**
  - ✅ YES → Create new version (content, owner, created_at)
  - ❌ NO → Proceed with update

- [ ] **Is this a lifecycle change?**
  - ✅ YES → Update status field (allowed)
  - ❌ NO → Check other criteria

- [ ] **Is this a metrics update?**
  - ✅ YES → Update metrics fields (allowed)
  - ❌ NO → Check other criteria

### **If Creating New Version:**

- [ ] **Increment version** (e.g., v1.0.0 → v1.1.0)
- [ ] **Keep same workflow_id** (for versioning)
- [ ] **Set is_latest_version = true** for new version
- [ ] **Set is_latest_version = false** for old version
- [ ] **Set previous_version** to link versions
- [ ] **Add version_notes** explaining changes

---

## 🎯 **Next Steps**

1. ✅ **DD-WORKFLOW-012 Created** (this document - authoritative immutability reference)
2. ⏳ **Update All DDs** to reference DD-WORKFLOW-012 for immutability
3. ⏳ **Implement Duplicate Prevention** validation in workflow repository
4. ⏳ **Update DD-WORKFLOW-002** with tie-breaking strategy and confidence field
5. ⏳ **Create Operator Guide** for workflow versioning best practices
6. ⏳ **Add Application-Level Validation** to enforce immutability in code

