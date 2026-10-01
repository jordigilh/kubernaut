# BR-KA-017-007: Label Context for Action Type Selection

**Business Requirement ID**: BR-KA-017-007
**Category**: Kubernaut Agent (KA) Service
**Priority**: P1
**Target Version**: V1.6
**Status**: Approved
**Date**: 2026-09-30
**Related Issue**: [#2478](https://github.com/jordigilh/kubernaut/issues/2478)

---

## Business Need

KA must use the detected infrastructure labels of the remediation target when
presenting action families for workflow selection. Management-aware workflows
should receive deterministic catalog preference without removing generic
workflows that may be better supported by the RCA.

## Acceptance Criteria

1. `SignalContext.DetectedLabelsJSON` is forwarded to KA's informer-cache-backed
   Step 1 and Step 2 discovery filters when present.
2. Step 1 action families are ordered by the best matching workflow's existing
   score, descending, with `action_type` ascending as the deterministic tie
   breaker; rank is assigned before pagination.
3. The LLM receives only bounded advisory evidence: `rank`, `preferred`,
   `matchedDetectedLabels`, and `preferenceReason`.
4. Generic action families remain available for RCA-driven selection and remain
   subject to the existing workflow context/security gate.
5. Internal scores and best-workflow identities are retained in the correlated
   `workflow.catalog.actions_listed` audit event, not in the LLM response.
6. Malformed detected-label input fails the discovery call closed rather than
   silently weakening the context filters.

## Design References

- **DD-KA-017**: Three-Step Workflow Discovery Integration
- **DD-WORKFLOW-016 v1.5**: Action-Type Workflow Catalog Indexing
- **BR-KA-265**: Infrastructure Labels in Workflow Discovery Context
- **ADR-056**: Post-RCA label computation
