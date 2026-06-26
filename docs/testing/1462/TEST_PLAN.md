# Test Plan: Fix #1462 AF Tool Does Not Pass Required Parameters to DS Remediation History Endpoint

**Issue**: [#1462](https://github.com/jordigilh/kubernaut/issues/1462)
**Service Type**: [x] Stateless HTTP API (API Frontend)
**Date**: 2026-06-26
**Status**: Active

---

## Business Requirements

| BR ID | Description |
|---|---|
| BR-HAPI-016 | Remediation history context for LLM prompt enrichment |

## FedRAMP Control Objectives

| Control | Objective | How This Fix Maps |
|---------|-----------|-------------------|
| SI-10 (Information Input Validation) | AF must validate required parameters before sending to DS | Validation rejects empty kind, name, spec_hash with descriptive errors |
| AU-3 (Content of Audit Records) | Spec hash enables causal chain integrity | Wiring spec_hash enables hash-chain regression detection in audit trail |

---

## Test Scenario Naming Convention

**Format**: `{TIER}-{SERVICE}-1462-{SEQUENCE}`

- `AF` = API Frontend

---

## Component 1: HandleGetRemediationHistory (AF Tool Handler)

### Unit Tests — Input Validation (SI-10)

| ID | Scenario | Expected Outcome |
|---|---|---|
| UT-AF-1462-001 | Empty `kind` in args → validation error | Error returned containing "kind is required"; DS client NOT called |
| UT-AF-1462-002 | Empty `name` in args → validation error | Error returned containing "name is required"; DS client NOT called |
| UT-AF-1462-003 | Empty `spec_hash` in args → validation error | Error returned containing "spec_hash is required"; DS client NOT called |

### Unit Tests — Happy Path (AU-3)

| ID | Scenario | Expected Outcome |
|---|---|---|
| UT-AF-1462-004 | All params valid including `spec_hash` → DS client receives all params | `opts.SpecHash == "sha256:abc123"` in captured DS client call; result returned without error |
| UT-AF-1462-005 | Empty `namespace` is allowed (cluster-scoped resources) | No error when namespace is empty; DS client called with empty namespace |
| UT-AF-1462-006 | `since` parameter pass-through | `opts.Since == "24h"` in captured DS client call |

---

## Component 2: OgenClient.GetRemediationHistory (DS Client)

### Unit Tests — Parameter Wiring (AU-3)

| ID | Scenario | Expected Outcome |
|---|---|---|
| UT-AF-1462-007 | `CurrentSpecHash` set in ogen query params | HTTP request to DS contains `currentSpecHash=sha256:abc123` in query string |

---

## Integration Tests (Wiring)

| ID | Scenario | Expected Outcome |
|---|---|---|
| IT-AF-1462-008 | Full tool → DS round-trip with all params | 200 response from DS with non-empty `tier1.chain`; spec hash used for hash-chain correlation |

---

## Existing Tests to Update

The following existing tests must be updated to include `spec_hash` in args to remain valid:

| ID | Required Change |
|---|---|
| UT-AF-122-001 | Add `SpecHash: "sha256:test"` to args |
| UT-AF-122-002 | Add `SpecHash: "sha256:test"` to args |
| UT-AF-122-003 | Add `SpecHash: "sha256:test"` to args |
| UT-AF-122-004 | Add `SpecHash: "sha256:test"` to args |

---

## Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|---|---|---|---|
| SpecHash in HistoryOpts | OgenClient.GetRemediationHistory | pkg/apifrontend/ds/ogen_client.go | IT-AF-1462-008 |
| Validation in HandleGetRemediationHistory | MCP tool registration | pkg/apifrontend/tools/ds_tools.go | IT-AF-1462-008 |

---

## Test Execution Summary

| Test Category | Tests | Status |
|---|---|---|
| AF Unit Tests — Validation (UT-AF-1462-001..003) | 3 | Pending |
| AF Unit Tests — Happy Path (UT-AF-1462-004..006) | 3 | Pending |
| AF Unit Tests — Client Wiring (UT-AF-1462-007) | 1 | Pending |
| AF Integration Tests (IT-AF-1462-008) | 1 | Pending |
| Existing Test Updates (UT-AF-122-001..004) | 4 | Pending |
| **Total** | **12** | **Pending** |
