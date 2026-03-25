# BR-HAPI-265: Infrastructure Labels in Workflow Discovery Context

**Business Requirement ID**: BR-HAPI-265
**Category**: HolmesGPT API Service
**Priority**: P2
**Target Version**: V1
**Status**: Approved
**Date**: 2026-03-25
**Related Issue**: [#529](https://github.com/jordigilh/kubernaut/issues/529)

---

## Business Need

### Problem Statement

The LLM selects workflows without explicit awareness of infrastructure labels (gitOpsManaged, hpaEnabled, pdbProtected, etc.). While labels are transparently injected into DS queries for filtering, the LLM doesn't see them when making its selection. This can lead to suboptimal choices -- e.g., selecting a direct-patch workflow for a GitOps-managed resource.

### Business Objective

Include detected infrastructure labels in the enrichment context provided to the LLM in Phase 3 (Workflow Selection). The LLM can reason about labels when selecting workflows and providing rationale.

---

## Acceptance Criteria

1. The enrichment prompt (Phase 3) includes detected labels in a structured, human-readable format
2. Labels are included in the HAPI response via `inject_detected_labels` for downstream consumers (AIAnalysis Rego policies, audit)
3. `failedDetections` are stripped from labels before including in the enrichment prompt (consistent with existing behavior)
4. The LLM does not manage or pass labels as parameters -- they are read-only context

---

## Design References

- **ADR-056 v1.3**: Labels surfaced as read-only cluster_context
- **DD-HAPI-017**: Three-Step Workflow Discovery Integration
- **Issue #529**: RCA Flow Redesign

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-25 | Initial requirement: labels in enrichment context for workflow selection |
