# ADR-066: Migrate to 4-Level Severity Model

## Status

Proposed

## Date

2026-06-12

## Context

Kubernaut currently uses a 5-level severity model: `critical > high > medium > low > info`. This model has two problems:

1. **Vocabulary mismatch with Prometheus**: Prometheus alerts use `critical`, `warning`, and `info` as standard severity labels. Kubernaut's `medium` does not map to any Prometheus vocabulary, causing ranking bugs where `warning` (from Prometheus) was ranked at 0 (unknown) — below `info`.

2. **Redundant levels**: `low` is never produced by Prometheus alerting rules and rarely meaningful in incident response. Having both `medium` and `low` creates confusion without adding decision-making value.

3. **External system alignment**: PagerDuty uses `critical/error/warning/info`, ServiceNow uses P1-P4, OpsGenie uses P1-P5. A 4-level model maps cleanly to all of these.

## Decision

Migrate from the current 5-level model to a 4-level canonical model:

```
critical > high > warning > info
```

- Replace `medium` with `warning` (Prometheus-standard vocabulary)
- Remove `low` (merge into `info` for stored data)
- Maintain backward-compatible reads during migration

## Scope

| Area | Estimated Changes |
|------|-------------------|
| `severityRank` / `validSeverities` | Already forward-compatible (#1412) |
| CRD schemas (`api/`) | 4 files |
| Generated OAS code (`ogen-client/`) | Regenerate |
| DataStorage stored records | Migration: `medium` → `warning`, `low` → `info` |
| SignalProcessing evaluator | ~15 files |
| Notification routing | ~5 files |
| Approval thresholds | ~3 files |
| LLM triage prompts | ~5 files |
| Mock LLM scenarios | ~10 files |
| E2E/IT/UT fixtures | ~60 files |
| **Total** | ~100+ files, ~350 occurrences |

## Migration Strategy

### Phase 1: Forward Compatibility (DONE — #1412)
- Add `"warning": 3` alongside `"medium": 3` in `severityRank`
- Both vocabularies work simultaneously

### Phase 2: Production Emission
- Update all code that emits `"medium"` to emit `"warning"` instead
- Update all code that emits `"low"` to emit `"info"` instead
- `BuildTriagePrompt` responds with `critical, high, warning, info`

### Phase 3: Backward-Compatible Reads
- DataStorage read layer: stored `"medium"` → return `"warning"`
- DataStorage read layer: stored `"low"` → return `"info"`
- CRD validation accepts both old and new vocabularies

### Phase 4: Schema Updates
- Update CRD enum values
- Regenerate OAS code
- Update JSON schemas

### Phase 5: Cleanup
- Remove `"medium"` and `"low"` from `severityRank`
- Remove backward-compat read layer after data migration
- `NormalizeSeverity` default becomes `"warning"` (was `"medium"`)

## External Compatibility Matrix

| External System | Kubernaut Mapping |
|----------------|-------------------|
| Prometheus | critical → critical, warning → warning, info → info |
| PagerDuty | critical → critical, error → high, warning → warning, info → info |
| ServiceNow | P1 → critical, P2 → high, P3 → warning, P4 → info |
| OpsGenie | P1 → critical, P2 → high, P3 → warning, P4/P5 → info |

## Consequences

### Positive
- Eliminates Prometheus vocabulary mismatch
- Simplifies mental model for operators
- Aligns with industry standards
- Reduces enum-explosion in CRD schemas

### Negative
- Large migration scope (~100+ files)
- Requires DataStorage migration for stored records
- Breaking change for any external consumers of the current 5-level model
- Needs phased rollout to avoid service disruption

### Risks
- Stored historical data referencing `medium`/`low` must remain queryable
- External integrations may depend on specific severity strings

## Related

- #1412: Forward-compatible fix (added `warning` to `severityRank`)
- #1416: Console alert_selection artifact
- #1417: Tracking issue for this ADR
