# BR-HAPI-260: Dedicated Remediation History Tool

**Business Requirement ID**: BR-HAPI-260
**Category**: HolmesGPT API Service
**Priority**: P2
**Target Version**: V1
**Status**: Approved
**Date**: 2026-03-25
**Related Issue**: [#529](https://github.com/jordigilh/kubernaut/issues/529)

---

## Business Need

### Problem Statement

Remediation history is currently fetched as a side effect of the `get_namespaced_resource_context` tool. The LLM cannot independently query history for a specific resource without also triggering owner chain resolution, spec hash computation, and label detection. This tight coupling limits LLM reasoning flexibility during investigation.

### Business Objective

Provide a dedicated `get_remediation_history` LLM tool that decouples history queries from the resource context flow. The LLM can call it independently during Phase 1 (RCA) for any resource to inform its root cause analysis.

---

## Acceptance Criteria

1. A dedicated `get_remediation_history` tool is registered and available to the LLM during investigation
2. The tool accepts `kind`, `name`, and optional `namespace` parameters
3. The tool internally resolves the K8s owner chain to find the root owner before querying DS
4. The tool returns the remediation history chain (same format as DD-HAPI-016) to the LLM
5. The tool logs queried resources for audit purposes (structured logging)
6. The tool does NOT enforce that the LLM must call it (enforcement dropped per BR-HAPI-262 removal)
7. Owner chain resolution failure degrades gracefully: query DS with the provided resource

---

## Design References

- **DD-HAPI-016**: Remediation History Context Enrichment (DS endpoint contract)
- **ADR-055 v1.5**: LLM-Driven Context Enrichment (tool registration pattern)
- **Issue #529**: RCA Flow Redesign (three-phase architecture)

---

## Scope

**In scope**: Dedicated history tool, owner chain resolution within tool, DS query, audit logging.

**Out of scope**: History verification enforcement (dropped -- see BR-HAPI-262 note). HAPI's EnrichmentService provides authoritative history in Phase 2 regardless of whether the LLM called this tool.

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-25 | Initial requirement: dedicated history tool with audit tracking (enforcement dropped) |
