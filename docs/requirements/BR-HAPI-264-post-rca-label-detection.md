# BR-HAPI-264: Post-RCA Infrastructure Label Detection via EnrichmentService

**Business Requirement ID**: BR-HAPI-264
**Category**: HolmesGPT API Service
**Priority**: P2
**Target Version**: V1
**Status**: Approved
**Date**: 2026-03-25
**Related Issue**: [#529](https://github.com/jordigilh/kubernaut/issues/529)

---

## Business Need

### Problem Statement

Under the current architecture (ADR-056 v1.6), infrastructure labels (gitOpsManaged, pdbProtected, hpaEnabled, stateful, etc.) are detected by the `get_namespaced_resource_context` tool during the LLM's investigation. Labels are stored in `session_state` and injected into workflow discovery queries.

With the three-phase architecture (#529), this creates two problems:

1. Labels are detected for the resource the LLM investigated (via the tool), not necessarily the resource HAPI resolves as the root owner in Phase 2. The LLM might investigate a Pod but HAPI resolves to a Deployment -- labels should describe the Deployment.

2. `root_owner` and `detected_labels` writes in `session_state` by the resource context tool conflict with HAPI's Phase 2 resolution, which is authoritative.

### Business Objective

Move label detection from the resource_context tool to HAPI's EnrichmentService (Phase 2). Labels are always detected for the K8s-verified root owner, ensuring they accurately describe the resource that will be remediated.

---

## Acceptance Criteria

1. EnrichmentService detects labels for the resolved root owner (output of owner chain resolution)
2. Labels are provided to the LLM via the enrichment prompt in Phase 3
3. Resource context tools no longer write `detected_labels` to `session_state`
4. Resource context tools no longer write `root_owner` to `session_state`
5. `inject_detected_labels` works with labels from EnrichmentResult (not session_state)
6. If label detection fails after retries, HAPI fails hard with `rca_incomplete`

---

## Design References

- **ADR-056 v1.7**: Post-RCA Label Computation (updated for EnrichmentService)
- **DD-HAPI-018**: DetectedLabels Detection Specification (detection contract unchanged)
- **Issue #529**: RCA Flow Redesign

---

## Supersedes

This BR supersedes the label detection behavior described in ADR-056 v1.4-v1.6 (labels in resource_context tool). The detection specification (DD-HAPI-018) and label schema are unchanged; only the execution location moves.

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-25 | Initial requirement: labels via EnrichmentService for resolved root owner |
