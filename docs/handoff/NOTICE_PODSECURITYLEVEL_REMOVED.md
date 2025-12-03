# NOTICE: PodSecurityLevel Field Removed from DetectedLabels

**Date**: December 3, 2025
**From**: SignalProcessing Team
**To**: All Service Teams (AIAnalysis, HolmesGPT-API, Data Storage, RO, Gateway)
**Priority**: Medium (Schema Change)
**Effective**: Immediately

---

## Summary

The `podSecurityLevel` field has been **removed** from the `DetectedLabels` schema in DD-WORKFLOW-001 v2.2.

---

## Rationale

| Issue | Impact |
|-------|--------|
| **PodSecurityPolicy (PSP) Deprecated** | PSP deprecated in K8s 1.21, removed in K8s 1.25 |
| **Pod Security Standards (PSS) Inconsistency** | PSS is enforced at **namespace-level**, not pod-level |
| **Detection Complexity** | Would require checking both namespace labels AND pod security context |
| **Unreliable Results** | Different clusters have different PSS enforcement modes |

**Decision**: Remove field entirely rather than provide unreliable detection.

---

## Schema Change

### Before (DD-WORKFLOW-001 v2.1)

```go
type DetectedLabels struct {
    // ... other fields ...
    PodSecurityLevel string `json:"podSecurityLevel,omitempty"` // REMOVED
    ServiceMesh      string `json:"serviceMesh,omitempty"`
}
```

### After (DD-WORKFLOW-001 v2.2)

```go
type DetectedLabels struct {
    // ... other fields ...
    // PodSecurityLevel REMOVED - PSP deprecated, PSS is namespace-level
    ServiceMesh string `json:"serviceMesh,omitempty"`
}
```

---

## Impact by Service

| Service | Action Required | Impact Level |
|---------|-----------------|--------------|
| **SignalProcessing** | ✅ Already updated | None (producer) |
| **AIAnalysis** | Remove any `podSecurityLevel` references | Low |
| **HolmesGPT-API** | Remove workflow filtering on `podSecurityLevel` | Low |
| **Data Storage** | Remove `podSecurityLevel` from workflow catalog schema | Low |
| **RO** | No action (pass-through only) | None |
| **Gateway** | No action (no DetectedLabels usage) | None |

---

## Migration Steps

### AIAnalysis Team

```go
// BEFORE
if labels.PodSecurityLevel == "restricted" {
    // handle restricted pods
}

// AFTER
// Remove this check - field no longer exists
```

### HolmesGPT-API Team

```sql
-- BEFORE (workflow catalog query)
WHERE detected_labels->>'podSecurityLevel' = 'restricted'

-- AFTER
-- Remove this WHERE clause
```

### Data Storage Team

```yaml
# BEFORE (workflow catalog schema)
detected_labels:
  podSecurityLevel:
    type: string
    enum: [privileged, baseline, restricted]

# AFTER
# Remove podSecurityLevel from schema
```

---

## Updated Field Count

| Version | Field Count | Fields |
|---------|-------------|--------|
| v2.1 | 9 fields | gitOpsManaged, gitOpsTool, pdbProtected, hpaEnabled, stateful, helmManaged, networkIsolated, **podSecurityLevel**, serviceMesh |
| v2.2 | 8 fields | gitOpsManaged, gitOpsTool, pdbProtected, hpaEnabled, stateful, helmManaged, networkIsolated, serviceMesh |

---

## References

- **DD-WORKFLOW-001 v2.2**: [DD-WORKFLOW-001-mandatory-label-schema.md](../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md)
- **Go Type**: `pkg/shared/types/enrichment.go`
- **SignalProcessing Plan**: `docs/services/crd-controllers/01-signalprocessing/IMPLEMENTATION_PLAN_V1.20.md`

---

## Questions?

Reply to this document or reach out to the SignalProcessing team directly.

---

## Acknowledgment

Please acknowledge receipt by adding your team name below:

| Team | Acknowledged | Date | Notes |
|------|--------------|------|-------|
| AIAnalysis | ✅ Done | 2025-12-03 | Updated crd-schema.md v2.4 (removed field + enum), README.md v2.6 |
| HolmesGPT-API | ✅ Done | 2025-12-03 | Removed from model, filters, context, tests (9 files). OpenAPI regenerated. |
| Data Storage | ✅ Acknowledged | 2025-12-03 | 3 files, ~21 lines to remove: models/workflow.go (field + constant), repository/workflow_repository.go (WHERE + boost), audit/workflow_search_event.go (audit field) |
| RO | ✅ Acknowledged | 2025-12-03 | No action required - pass-through only, no consumption of DetectedLabels fields |
| Gateway | ✅ Acknowledged | 2025-12-03 | No impact - Gateway does not use DetectedLabels schema |

