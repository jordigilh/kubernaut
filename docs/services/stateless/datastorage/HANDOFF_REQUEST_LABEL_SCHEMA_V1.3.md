# Handoff Request: Label Schema Update (DD-WORKFLOW-001 v1.3)

**From**: Architecture Team
**To**: Data Storage Service Team
**Date**: November 30, 2025
**Priority**: P2 (Documentation alignment)
**Status**: 🟡 LOW IMPACT - Implementation already correct

---

## Summary

DD-WORKFLOW-001 has been updated from v1.2 to **v1.3**, reducing mandatory labels from 7 to **6**. The `business_category` label has been moved from mandatory to **optional custom label**.

**Good News**: Your implementation already treats `business_category` as optional (JSONB with `omitempty`). This is primarily a documentation alignment task.

---

## What Changed

| Aspect | Before (v1.2) | After (v1.3) |
|--------|---------------|--------------|
| **Mandatory Labels** | 7 | **6** |
| **`business_category`** | Mandatory | **Optional custom label** |
| **Structured Columns** | 7 columns | 6 columns + JSONB |
| **Wildcard Support** | `environment`, `priority`, `business_category` | `environment`, `priority` |

---

## Label Schema (V1.3)

### 6 Mandatory Labels

| Group | Label | Type | Wildcard |
|-------|-------|------|----------|
| **A: Auto-Populated** | `signal_type` | TEXT | ❌ |
| **A: Auto-Populated** | `severity` | ENUM | ❌ |
| **A: Auto-Populated** | `component` | TEXT | ❌ |
| **B: Rego-Configurable** | `environment` | ENUM | ✅ `'*'` |
| **B: Rego-Configurable** | `priority` | ENUM | ✅ `'*'` |
| **B: Rego-Configurable** | `risk_tolerance` | ENUM | ❌ |

### Optional Custom Labels (JSONB)

| Label | Type | Description |
|-------|------|-------------|
| `business_category` | TEXT | Business domain (payment-service, analytics) |
| `gitops_tool` | TEXT | GitOps preferences (argocd, flux) |
| `region` | TEXT | Geographic targeting (us-east-1) |
| `team` | TEXT | Team ownership (platform, sre) |

---

## Impact Assessment

### ✅ Already Correct (No Changes Needed)

| File | Status | Notes |
|------|--------|-------|
| `pkg/datastorage/models/workflow.go` | ✅ OK | Labels stored as JSONB |
| `pkg/datastorage/models/workflow_schema.go` | ✅ OK | `BusinessCategory` already has `omitempty` |
| `pkg/datastorage/schema/parser.go` | ✅ OK | Only adds `business_category` if present |
| `pkg/datastorage/server/workflow_handlers.go` | ✅ OK | Accepts as optional filter |

### ⚠️ Documentation Updates Required

| File | Line | Current | Should Be |
|------|------|---------|-----------|
| `pkg/datastorage/models/workflow_schema.go` | 50-51 | `// DD-WORKFLOW-001: 7 mandatory labels` | `// DD-WORKFLOW-001 v1.3: 6 mandatory labels` |
| `pkg/datastorage/models/workflow.go` | 77 | `//   "business_category": "payments",` | Move to custom_labels example or keep as optional example |

---

## Recommended Changes

### 1. Update Comment References

```go
// pkg/datastorage/models/workflow_schema.go

// BEFORE (line 50-51):
// Labels contains discovery labels for MCP search
// DD-WORKFLOW-001: 7 mandatory labels

// AFTER:
// Labels contains discovery labels for MCP search
// DD-WORKFLOW-001 v1.3: 6 mandatory labels + optional custom labels
```

### 2. Clarify business_category as Optional

```go
// pkg/datastorage/models/workflow_schema.go

// BEFORE (line 104-106):
// BusinessCategory is the business category for filtering (OPTIONAL)
// Values: "cost-management", "performance", "availability", "security"
BusinessCategory string `yaml:"business_category,omitempty" json:"business_category,omitempty" validate:"omitempty"`

// AFTER (add note about custom labels):
// BusinessCategory is an optional custom label for business domain filtering
// Per DD-WORKFLOW-001 v1.3: Moved from mandatory to optional custom label
// Values: user-defined (e.g., "payment-service", "analytics", "infrastructure")
BusinessCategory string `yaml:"business_category,omitempty" json:"business_category,omitempty" validate:"omitempty"`
```

---

## No Database Migration Required

The current schema already uses JSONB for labels:

```sql
-- Current schema (already correct)
Labels json.RawMessage `json:"labels" db:"labels" validate:"required"`
```

Labels are stored as flexible JSONB, not structured columns. No migration needed.

---

## Testing Verification

Please verify these scenarios still work:

1. **Workflow creation without `business_category`** → Should succeed ✅
2. **Workflow creation with `business_category`** → Should succeed ✅
3. **Search filtering by `business_category`** → Should return matching workflows ✅
4. **Search without `business_category` filter** → Should return all workflows ✅

---

## Authority

- **DD-WORKFLOW-001 v1.3**: [docs/architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md](../../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md)
- **DD-PLAYBOOK-001 v1.2**: [docs/architecture/decisions/DD-PLAYBOOK-001-mandatory-label-schema.md](../../../../architecture/decisions/DD-PLAYBOOK-001-mandatory-label-schema.md)

---

## Timeline

| Task | Effort | Priority |
|------|--------|----------|
| Update code comments | 30 min | P3 |
| Verify test coverage | 1 hour | P2 |
| **Total** | **1.5 hours** | P2 |

---

**Contact**: Architecture Team
**Questions**: Create issue or reach out on #kubernaut-dev

