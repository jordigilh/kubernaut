# Questions for Gateway Team

**From**: SignalProcessing Team
**Date**: December 1, 2025
**Status**: ⏳ Awaiting Response

---

## Context

SignalProcessing enriches alerts with Kubernetes context. We need to understand how `CustomLabels` from user-defined Rego policies will be passed through to HolmesGPT-API.

---

## Questions

### Q1: CustomLabels Field Order in Prompt

**Context**: SignalProcessing outputs `CustomLabels` as `map[string][]string`:
```json
{
  "customLabels": {
    "constraint": ["cost-constrained", "stateful-safe"],
    "team": ["name=platform"],
    "region": ["zone=us-east-1"]
  }
}
```

**Question**: In the HolmesGPT prompt template, what order are these labels presented? Does ordering affect LLM interpretation?

**Options**:
- [ ] A) Alphabetical by key - consistent ordering
- [ ] B) Insertion order - varies per request
- [ ] C) Custom ordering based on label category (e.g., `constraint` first)
- [ ] D) Doesn't matter - LLM is order-agnostic

---

### Q2: CustomLabels Size Limits

**Context**: Rego policies can theoretically generate unlimited custom labels.

**Question**: Is there a size limit for `CustomLabels` in the HolmesGPT prompt? Should SignalProcessing enforce limits?

**Current SignalProcessing Limits** (proposed):
- Max 10 custom label keys
- Max 5 values per key
- Max 100 chars per value

**Options**:
- [ ] A) Limits are correct - enforce at SignalProcessing
- [ ] B) Different limits needed: _______________
- [ ] C) No limits - Gateway/HolmesGPT handles truncation
- [ ] D) Limits enforced elsewhere - specify location

---

### Q3: CustomLabels Validation

**Context**: `CustomLabels` come from user Rego policies (sandboxed but user-defined).

**Question**: Does HolmesGPT-API validate/sanitize custom label content before including in prompts?

**Concern**: Malicious Rego could output labels designed to manipulate LLM behavior (prompt injection).

**Options**:
- [ ] A) Yes, validation at Gateway layer
- [ ] B) Yes, validation at HolmesGPT-API layer
- [ ] C) No validation - SignalProcessing should sanitize
- [ ] D) No validation needed - Rego sandbox is sufficient

---

## Response Section

### Gateway Team Response

**Date**:
**Respondent**:

**Q1 (Field order)**:
- [ ] Option A / B / C / D
- Notes:

**Q2 (Size limits)**:
- [ ] Option A / B / C / D
- Notes:

**Q3 (Validation)**:
- [ ] Option A / B / C / D
- Notes:

---

**SignalProcessing Team**


