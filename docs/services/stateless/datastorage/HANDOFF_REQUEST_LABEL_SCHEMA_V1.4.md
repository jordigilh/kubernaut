# Handoff Request: Label Schema Update (DD-WORKFLOW-001 v1.4)

**From**: Architecture Team
**To**: Data Storage Service Team
**Date**: November 30, 2025
**Priority**: P2 (Documentation alignment)
**Status**: 🟡 LOW IMPACT - Implementation already correct

---

## Document Version

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| **2.0** | Nov 30, 2025 | AI Analysis Team | Updated to 5 mandatory labels, `risk_tolerance` now customer-derived |
| 1.0 | Nov 30, 2025 | Architecture Team | Initial: 6 mandatory labels |

---

## 📢 Changelog (v2.0)

### ⚠️ SCHEMA CHANGE

**Mandatory labels reduced from 6 to 5**. Both `risk_tolerance` AND `business_category` are now customer-derived via Rego policies.

| Label | Before (v1.3) | After (v1.4) |
|-------|---------------|--------------|
| `risk_tolerance` | Mandatory | **Customer-derived via Rego** |
| `business_category` | Optional | **Customer-derived via Rego** |

**Rationale**:
- `risk_tolerance`: Customers define what their environments mean (e.g., "uat" could be high risk for one team, low for another)
- `business_category`: Not all workloads have a business category (e.g., infrastructure pods)

---

## Summary

DD-WORKFLOW-001 has been updated to **v1.4**, reducing mandatory labels from 7 to **5**. Both `risk_tolerance` and `business_category` are now **customer-derived via Rego policies**.

**Good News**: Your implementation already treats these as optional (JSONB with `omitempty`). This is primarily a documentation alignment task.

---

## What Changed

| Aspect | Before (v1.2) | After (v1.4) |
|--------|---------------|--------------|
| **Mandatory Labels** | 7 | **5** |
| **`risk_tolerance`** | Mandatory | **Customer-derived via Rego** |
| **`business_category`** | Mandatory → Optional | **Customer-derived via Rego** |
| **Structured Columns** | 7 columns | 5 columns + JSONB |
| **Wildcard Support** | `environment`, `priority` | `environment`, `priority` |

---

## Label Schema (V1.4)

### 5 Mandatory Labels (System-Controlled)

| Group | Label | Type | Wildcard |
|-------|-------|------|----------|
| **A: Auto-Populated** | `signal_type` | TEXT | ❌ |
| **A: Auto-Populated** | `severity` | ENUM | ❌ |
| **A: Auto-Populated** | `component` | TEXT | ❌ |
| **B: System-Classified** | `environment` | ENUM | ✅ `'*'` |
| **B: System-Classified** | `priority` | ENUM | ✅ `'*'` |

### Customer-Derived Labels (via Rego → JSONB)

| Label | Type | Description |
|-------|------|-------------|
| `risk_tolerance` | ENUM | Customer interprets environment risk (critical/high/medium/low) |
| `business_category` | TEXT | Business domain - optional, not all workloads have one |
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
// DD-WORKFLOW-001 v1.4: 5 mandatory labels + customer-derived labels via Rego
```

### 2. Clarify risk_tolerance as Customer-Derived

```go
// pkg/datastorage/models/workflow_schema.go

// BEFORE:
// RiskTolerance is the risk tolerance level for filtering (MANDATORY)

// AFTER:
// RiskTolerance is customer-derived via Rego policies
// Per DD-WORKFLOW-001 v1.4: Customers define what environments mean for risk
// Values: "critical", "high", "medium", "low"
RiskTolerance string `yaml:"risk_tolerance,omitempty" json:"risk_tolerance,omitempty" validate:"omitempty"`
```

### 3. Clarify business_category as Customer-Derived

```go
// pkg/datastorage/models/workflow_schema.go

// BEFORE (line 104-106):
// BusinessCategory is the business category for filtering (OPTIONAL)
// Values: "cost-management", "performance", "availability", "security"
BusinessCategory string `yaml:"business_category,omitempty" json:"business_category,omitempty" validate:"omitempty"`

// AFTER (add note about customer-derived):
// BusinessCategory is customer-derived via Rego policies (optional)
// Per DD-WORKFLOW-001 v1.4: Not all workloads have a business category (e.g., infra pods)
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

- **DD-WORKFLOW-001 v1.4**: [docs/architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md](../../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md)
- **DD-NAMING-001**: [docs/architecture/decisions/DD-NAMING-001-remediation-workflow-terminology.md](../../../../architecture/decisions/DD-NAMING-001-remediation-workflow-terminology.md) - "Workflow" is authoritative terminology
- **SignalProcessing Rego Handoff**: [HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md](../../crd-controllers/01-signalprocessing/HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md)

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

