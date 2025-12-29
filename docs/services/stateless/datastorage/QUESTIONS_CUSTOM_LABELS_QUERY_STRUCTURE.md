# Questions: Custom Labels Query Structure

**Date**: 2025-11-30
**From**: Data Storage Team
**To**: SignalProcessing Team
**Re**: HANDOFF_CUSTOM_LABELS_QUERY_STRUCTURE.md v1.0
**Status**: ✅ ANSWERED

---

## Context

We received the custom labels query structure handoff and have identified several clarifications needed before implementation.

---

## Questions

### Question 1: Subdomain Naming

**Are subdomains fixed (e.g., `constraint`, `team`, `region`) or customer-defined?**

The handoff shows examples like:
```json
{
  "constraint": ["cost-constrained", "stateful-safe"],
  "team": ["name=payments"],
  "region": ["zone=us-east-1"]
}
```

- **Option A**: Fixed set of subdomains (Data Storage validates against allowed list)
- **Option B**: Customer-defined (Data Storage accepts any subdomain)

**Impact**: Determines if we need validation logic or just pass-through.

**✅ ANSWER: Option B - Customer-defined**

Operators define their own subdomains via Rego policies. Kubernaut does NOT validate or restrict subdomain names. This is a core design principle: **operator freedom**.

The examples (`constraint`, `team`, `region`) are **documentation conventions only**, not enforced rules. An operator could use `compliance`, `cost-center`, `business-unit`, or any string they choose.

**Data Storage Action**: Accept any subdomain string. No validation needed. Just pass-through to JSONB storage/query.

---

### Question 2: Prefix Stripping

**Who is responsible for stripping the `kubernaut.io/` prefix?**

Previous understanding had full keys like:
- `kubernaut.io/team`
- `constraint.kubernaut.io/cost-constrained`

New structure has subdomains like:
- `team`
- `constraint`

**Options**:
- **Option A**: SignalProcessing strips prefix before sending to HolmesGPT-API
- **Option B**: HolmesGPT-API strips prefix before sending to Data Storage
- **Option C**: Data Storage strips prefix on receipt

**Impact**: Determines if Data Storage needs prefix-stripping logic.

**✅ ANSWER: Option A - SignalProcessing strips prefix**

SignalProcessing is responsible for the extraction. The full label format `<subdomain>.kubernaut.io/<key>[:<value>]` is an internal implementation detail hidden from all downstream services.

**Data flow**:
```
Rego Output                          SignalProcessing                   Downstream
───────────────────────              ────────────────                   ──────────
constraint.kubernaut.io/cost:true →  Extracts subdomain + key      →   {"constraint": ["cost"]}
team.kubernaut.io/name:payments   →  Hides .kubernaut.io/ suffix   →   {"team": ["name=payments"]}
```

**Data Storage Action**: No prefix stripping needed. You receive clean subdomain keys.

---

### Question 3: Workflow Storage Format

**Should workflows store custom labels in the same `map[string][]string` format?**

The handoff shows search queries use:
```json
"custom_labels": {
  "constraint": ["cost-constrained"],
  "team": ["name=payments"]
}
```

**Question**: When registering a workflow via `POST /api/v1/workflows`, should the workflow's `custom_labels` field use the same structure?

```json
// Workflow registration - same format?
{
  "workflow_name": "oomkill-increase-memory",
  "custom_labels": {
    "constraint": ["cost-constrained"],
    "team": ["name=payments", "name=platform"]
  }
}
```

**Impact**: Ensures consistency between storage and query formats.

**✅ ANSWER: Yes, same `map[string][]string` format**

Workflow registration and search queries should use the **identical structure** for consistency.

**Storage schema**:
```sql
custom_labels JSONB  -- {"constraint": ["cost-constrained"], "team": ["name=payments"]}
```

**Matching logic**: A workflow matches if the signal's custom labels are present in the workflow's custom labels (per subdomain). Multiple values in workflow = OR match.

**Example**:
- Signal: `{"team": ["name=payments"]}`
- Workflow A: `{"team": ["name=payments", "name=platform"]}` → ✅ MATCH
- Workflow B: `{"team": ["name=sre"]}` → ❌ NO MATCH

---

### Question 4: Empty Subdomain Handling

**How should Data Storage handle empty subdomains or empty value arrays?**

Examples:
```json
// Empty array - ignore subdomain?
{"constraint": []}

// Missing subdomain - no filter?
{"team": ["name=payments"]}  // No "constraint" key
```

**Impact**: Determines edge case handling in query generation.

**✅ ANSWER: Ignore empty arrays and missing subdomains**

| Case | Behavior | SQL Result |
|------|----------|------------|
| `{"constraint": []}` | Ignore subdomain | No WHERE clause for `constraint` |
| `{"team": ["name=payments"]}` (no `constraint`) | Only filter on present | `WHERE custom_labels->'team' ? 'name=payments'` |
| `{}` or missing field | No custom label filter | No custom label WHERE clauses |

**Rationale**: Empty = "no opinion on this subdomain". Only filter on explicitly provided values.

**Go implementation**:
```go
for subdomain, values := range customLabels {
    if len(values) == 0 {
        continue  // Skip empty arrays
    }
    // Add WHERE clause for this subdomain
}
```

---

### Question 5: Version References

**Can you confirm the document versions referenced?**

The handoff references:
- DD-WORKFLOW-001 v1.8
- DD-WORKFLOW-004 v2.2

Our current versions:
- DD-WORKFLOW-001 v1.8 (current)
- DD-WORKFLOW-004 v2.1

**Question**: Are v1.5 and v2.2 upcoming versions, or should we use the current versions?

**✅ ANSWER: v1.5 and v2.2 are current (just updated today)**

We updated the DDs today with the subdomain-based custom labels design:

| Document | Previous | Current | Key Change |
|----------|----------|---------|------------|
| DD-WORKFLOW-001 | v1.4 | **v1.5** | Subdomain extraction format |
| DD-WORKFLOW-004 | v2.1 | **v2.2** | CustomLabels as `map[string][]string` |

**Action**: Pull the latest from `docs/architecture/decisions/` to get v1.5 and v2.2.

---

## Summary Table

| # | Question | Options | Default Assumption | ✅ Answer |
|---|----------|---------|-------------------|-----------|
| 1 | Subdomain naming | Fixed / Customer-defined | Customer-defined | **Customer-defined** |
| 2 | Prefix stripping | SignalProcessing / HolmesGPT-API / Data Storage | SignalProcessing | **SignalProcessing** |
| 3 | Workflow storage format | Same `map[string][]string` | Yes, same format | **Yes, same format** |
| 4 | Empty handling | Ignore / Error | Ignore empty | **Ignore empty** |
| 5 | Document versions | v1.5/v2.2 or v1.4/v2.1 | Use current | **v1.5/v2.2 (just updated)** |

**✅ All default assumptions were correct.**

---

## Response Format

Please respond inline below each question or create a response document at:
`docs/services/crd-controllers/01-signalprocessing/RESPONSE_CUSTOM_LABELS_QUERY_STRUCTURE.md`

---

## Related Documents

- [HANDOFF_CUSTOM_LABELS_QUERY_STRUCTURE.md](./HANDOFF_CUSTOM_LABELS_QUERY_STRUCTURE.md)
- [FOLLOWUP_CUSTOM_LABELS_CLARIFICATION.md](./FOLLOWUP_CUSTOM_LABELS_CLARIFICATION.md)
- [DD-WORKFLOW-001 v1.8](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md)
- [DD-WORKFLOW-002 v3.2](../../../architecture/decisions/DD-WORKFLOW-002-MCP-WORKFLOW-CATALOG-ARCHITECTURE.md) - **UPDATED** with subdomain structure
- [DD-WORKFLOW-004 v2.2](../../../architecture/decisions/DD-WORKFLOW-004-hybrid-weighted-label-scoring.md)
- [HANDOFF_CUSTOM_LABELS_EXTRACTION_V1.md](../../crd-controllers/01-signalprocessing/HANDOFF_CUSTOM_LABELS_EXTRACTION_V1.md)

---

## Response Changelog

| Date | Responder | Changes |
|------|-----------|---------|
| 2025-11-30 | SignalProcessing Team | Answered Q1-Q5 inline |

