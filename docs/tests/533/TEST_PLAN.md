# Test Plan: SOC2 Audit Event Gaps in Phase 2 Enrichment Flow

**Feature**: Emit ADR-034 compliant audit events for Phase 2 enrichment operations
**Version**: 1.1
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/v1.1.0-rc10`

**Authority**:
- [BR-AUDIT-005 v2.0]: SOC2 compliance — all security-relevant operations must have audit trail
- [ADR-034]: Unified Audit Table Design (schema, envelope, event_data)
- [DD-AUDIT-005]: Hybrid Provider Data Capture (HAPI emits provider-perspective events)
- [Issue #533]: SOC2 audit event gaps in #529 three-phase RCA enrichment flow

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- [Audit Anti-Pattern](../development/business-requirements/TESTING_GUIDELINES.md#anti-pattern-direct-audit-infrastructure-testing)

---

## 1. Scope

### In Scope

- **OpenAPI payload schemas** (`api/openapi/data-storage-v1.yaml`): New `AIAgentEnrichmentCompletedPayload` and `AIAgentEnrichmentFailedPayload` types added to the discriminated union (`oneOf` + `discriminator.mapping`)
- **Python client regeneration** (`kubernaut-agent/src/clients/datastorage/`): Regenerate the OpenAPI Python client so Pydantic models for the new payload types exist
- **Pydantic re-exports** (`src/models/audit_models.py`): Re-export the new generated payload types as `EnrichmentCompletedEventData` and `EnrichmentFailedEventData`
- **Audit event factory functions** (`src/audit/events.py`): Two new factory functions: `create_enrichment_completed_event` and `create_enrichment_failed_event`
- **Module exports** (`src/audit/__init__.py`): Export new factory functions for consumption by business code
- **Audit emission in business flow** (`src/extensions/incident/llm_integration.py`): Emitting enrichment events at the correct points in the Phase 2 flow:
  - **GAP 1** (High): `aiagent.enrichment.completed` — emitted after successful Phase 2 (after line 835 in `llm_integration.py`)
  - **GAP 2** (High): `aiagent.enrichment.failed` — emitted on `EnrichmentFailure` (inside the `except EnrichmentFailure` block at line 813)

### Out of Scope

- **GAP 3** (Low): Phase 1 exhaustion — partially covered by existing validation attempt events. Assessed as low SOC2 risk; defer to future release.
- **GAP 4** (Low): Phase 1→3 data merge — field-level provenance attribution. The final `aiagent.response.complete` event captures the merged result. Defer to future release.
- **Audit infrastructure** (buffering, batching, persistence): Owned by `pkg/audit/` and DataStorage. NOT tested here per [Audit Anti-Pattern](../development/business-requirements/TESTING_GUIDELINES.md#anti-pattern-direct-audit-infrastructure-testing).
- **Go Data Storage handler changes**: The DS ogen handler accepts any `event_data` as JSONB; no handler changes required for new event types.
- **Go ogen client regeneration**: Only Python client is used by HAPI; Go ogen regeneration deferred to when a Go consumer needs typed enrichment event access.

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| New OpenAPI payload schemas first | Discriminated union requires typed payloads for client-side validation. Pydantic models in `audit_models.py` are re-exports from the generated client — schemas must exist in OpenAPI before regeneration, before factory functions can reference them. **Strict dependency chain: OpenAPI → regenerate → re-export → factory → test.** |
| Factory functions in `events.py` | Follows established pattern (same as `create_llm_request_event`, `create_aiagent_response_complete_event`). Pure logic, unit-testable. Uses `_create_adr034_event()` helper that wraps event_data in `AuditEventRequestEventData(actual_instance=...)`. |
| Emit in `llm_integration.py`, not `enrichment_service.py` | The enrichment service is a pure business component. Audit emission belongs at the orchestration layer where `audit_store` is available (via `get_audit_store()`). Consistent with how LLM request/response events are emitted from `llm_integration.py`, not from the SDK. |
| Flatten `EnrichmentResult` fields for audit payload | `EnrichmentResult.detected_labels` is a dict from `LabelDetector`. The audit payload captures a `detected_labels_summary` (the dict as-is), `root_owner_kind`/`name`/`namespace` (flattened from `root_owner` dict), and `remediation_history_fetched` (boolean derived from `remediation_history is not None`). This provides audit-friendly flat fields for SOC2 queries. |
| Defer GAP 3 and GAP 4 | Low SOC2 risk, high effort. GAP 3 is partially covered by validation exhaustion handler. GAP 4 would require schema changes for marginal audit value. |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of **unit-testable** code (factory functions in `events.py`, payload field mapping logic)
- **Integration**: Not applicable for this feature — see Tier Skip Rationale

### 2-Tier Minimum

This feature applies a unit-only tier strategy with documented rationale for skipping integration tests. The audit anti-pattern policy explicitly states integration tests must test business logic that emits audits, not audit infrastructure. The business logic (Phase 2 enrichment) is already integration-tested via existing EnrichmentService tests; we add unit tests for the new audit event factories and verify emission wiring via code review only.

### Business Outcome Quality Bar

Tests validate:
1. **Correctness**: Factory functions produce ADR-034 compliant events with correct envelope and payload fields
2. **Data accuracy**: Enrichment data (root_owner, detected_labels, owner_chain_length) is accurately captured in audit events
3. **Behavior**: Factory functions handle boundary conditions (partial enrichment data, missing fields) without errors
4. **Robustness**: Failed enrichment events capture the exact failure context (reason, detail, affected_resource)

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `kubernaut-agent/src/audit/events.py` | `create_enrichment_completed_event`, `create_enrichment_failed_event` | ~60 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `kubernaut-agent/src/extensions/incident/llm_integration.py` | 2 `audit_store.store_audit()` call sites in Phase 2 | ~6 |

**Note**: The emission call sites in `llm_integration.py` are I/O (they call `audit_store.store_audit()`), but testing them would require mocking the audit store — which falls into the [Audit Anti-Pattern](../development/business-requirements/TESTING_GUIDELINES.md#anti-pattern-direct-audit-infrastructure-testing). Emission wiring is verified via code review and E2E coverage.

### Non-Testable Infrastructure (owned by other components)

| File | Reason |
|------|--------|
| `api/openapi/data-storage-v1.yaml` | Schema definition — validated by OpenAPI tooling |
| `kubernaut-agent/src/clients/datastorage/` | Generated client — validated by generator |
| `kubernaut-agent/src/models/audit_models.py` | Re-exports only (2 lines per type) — no logic to test |
| `kubernaut-agent/src/audit/__init__.py` | Module exports — compilation validates correctness |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-AUDIT-005 | Enrichment completed event follows ADR-034 envelope structure | P0 | Unit | UT-HAPI-533-001 | Pending |
| BR-AUDIT-005 | Enrichment completed event captures root_owner correctly | P0 | Unit | UT-HAPI-533-002 | Pending |
| BR-AUDIT-005 | Enrichment completed event captures detected_labels summary | P0 | Unit | UT-HAPI-533-003 | Pending |
| BR-AUDIT-005 | Enrichment completed event captures failed_detections list | P1 | Unit | UT-HAPI-533-004 | Pending |
| BR-AUDIT-005 | Enrichment completed event captures remediation_history_fetched flag | P1 | Unit | UT-HAPI-533-005 | Pending |
| BR-AUDIT-005 | Enrichment completed event captures owner_chain_length | P1 | Unit | UT-HAPI-533-006 | Pending |
| BR-AUDIT-005 | Enrichment failed event follows ADR-034 envelope with outcome="failure" | P0 | Unit | UT-HAPI-533-007 | Pending |
| BR-AUDIT-005 | Enrichment failed event captures failure reason and detail | P0 | Unit | UT-HAPI-533-008 | Pending |
| BR-AUDIT-005 | Enrichment failed event captures the affected_resource attempted | P1 | Unit | UT-HAPI-533-009 | Pending |
| BR-AUDIT-005 | Enrichment failed event correlation_id uses remediation_id | P1 | Unit | UT-HAPI-533-010 | Pending |
| BR-AUDIT-005 | Enrichment completed event handles partial data (labels=None, history=None) | P1 | Unit | UT-HAPI-533-011 | Pending |
| BR-AUDIT-005 | Enrichment completed event handles cluster-scoped resource (no namespace) | P2 | Unit | UT-HAPI-533-012 | Pending |
| BR-AUDIT-005 | Enrichment failed event handles missing remediation_id gracefully | P2 | Unit | UT-HAPI-533-013 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit)
- **SERVICE**: `HAPI` (HolmesGPT API)
- **ISSUE_NUMBER**: `533`
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `kubernaut-agent/src/audit/events.py` — 2 new factory functions, targeting >=80% coverage of new code

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HAPI-533-001` | Enrichment completed event has ADR-034 envelope (version, event_category, event_type, timestamp, correlation_id, event_action, event_outcome) | Pending |
| `UT-HAPI-533-002` | Enrichment completed event captures root_owner kind, name, and namespace accurately | Pending |
| `UT-HAPI-533-003` | Enrichment completed event captures detected_labels summary (label key-value pairs from LabelDetector) | Pending |
| `UT-HAPI-533-004` | Enrichment completed event captures failed_detections list (labels that could not be detected) | Pending |
| `UT-HAPI-533-005` | Enrichment completed event captures remediation_history_fetched boolean (True when DS returned history) | Pending |
| `UT-HAPI-533-006` | Enrichment completed event captures owner_chain_length (number of K8s resources in owner chain) | Pending |
| `UT-HAPI-533-007` | Enrichment failed event has ADR-034 envelope with event_outcome="failure" and event_type="aiagent.enrichment.failed" | Pending |
| `UT-HAPI-533-008` | Enrichment failed event captures failure reason (e.g., "rca_incomplete") and detail string (e.g., retry context) | Pending |
| `UT-HAPI-533-009` | Enrichment failed event captures the affected_resource that was being processed when failure occurred | Pending |
| `UT-HAPI-533-010` | Enrichment failed event correlation_id matches the provided remediation_id for SOC2 traceability | Pending |
| `UT-HAPI-533-011` | Enrichment completed event handles partial data: detected_labels=None, remediation_history=None (label detector unavailable, DS unreachable) | Pending |
| `UT-HAPI-533-012` | Enrichment completed event handles cluster-scoped resource (namespace="" in root_owner) | Pending |
| `UT-HAPI-533-013` | Enrichment failed event handles missing remediation_id (falls back to "unknown" for correlation_id) | Pending |

### Tier Skip Rationale

- **Integration**: Skipped per [Audit Anti-Pattern](../development/business-requirements/TESTING_GUIDELINES.md#anti-pattern-direct-audit-infrastructure-testing). Integration tests must test business logic that emits audits, not audit infrastructure. The enrichment business logic is already integration-tested via existing EnrichmentService tests. Adding integration tests that verify `audit_store.store_audit()` was called would test the audit client library, not the service's business logic. Unit tests on factory functions validate correctness; emission point wiring is verified via code review.
- **E2E**: Enrichment audit events will be verified as part of the full-pipeline E2E when the entire Phase 2 flow runs against a real cluster. Not separately tested for this issue.

---

## 6. Test Cases (Detail)

### UT-HAPI-533-001: Enrichment completed event ADR-034 envelope

**BR**: BR-AUDIT-005
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_audit_enrichment_events.py`

**Given**: A successful Phase 2 enrichment with root_owner={kind: "Deployment", name: "api-server", namespace: "production"}, 3 detected labels, remediation history fetched, owner chain of length 3
**When**: `create_enrichment_completed_event()` is called with incident_id="inc-123", remediation_id="rem-456", and the enrichment result data
**Then**: The returned `AuditEventRequest` has:
- `version` == "1.0"
- `event_category` == "aiagent"
- `event_type` == "aiagent.enrichment.completed"
- `event_timestamp` is a non-empty string (ISO 8601)
- `correlation_id` == "rem-456"
- `event_action` == "enrichment_completed"
- `event_outcome` == "success"
- `event_data` is not None and `event_data.actual_instance` is not None

**Acceptance Criteria**:
- All 8 ADR-034 envelope fields are present and non-None
- `event_category` matches the HAPI service constant ("aiagent")
- `actor_type` == "Service" and `actor_id` == "kubernaut-agent"

---

### UT-HAPI-533-002: Enrichment completed event root_owner accuracy

**BR**: BR-AUDIT-005
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_audit_enrichment_events.py`

**Given**: Phase 2 resolved root_owner={kind: "Deployment", name: "api-server", namespace: "production"}
**When**: `create_enrichment_completed_event()` is called
**Then**: `event_data.actual_instance` contains:
- `root_owner_kind` == "Deployment"
- `root_owner_name` == "api-server"
- `root_owner_namespace` == "production"

**Acceptance Criteria**:
- Root owner fields are flattened from the dict, not nested
- Values match the input exactly (no transformation)

---

### UT-HAPI-533-003: Enrichment completed event detected_labels summary

**BR**: BR-AUDIT-005
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_audit_enrichment_events.py`

**Given**: Phase 2 detected labels {"gitOpsManaged": True, "pdbProtected": False, "hpaEnabled": True}
**When**: `create_enrichment_completed_event()` is called with detected_labels={"gitOpsManaged": True, "pdbProtected": False, "hpaEnabled": True}
**Then**: `event_data.actual_instance.detected_labels_summary` == {"gitOpsManaged": True, "pdbProtected": False, "hpaEnabled": True}

**Acceptance Criteria**:
- Label dict is passed through as-is to the audit payload
- Both True and False values are preserved

---

### UT-HAPI-533-004: Enrichment completed event failed_detections

**BR**: BR-AUDIT-005
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_audit_enrichment_events.py`

**Given**: Phase 2 had failed detections ["networkIsolated", "serviceMesh"]
**When**: `create_enrichment_completed_event()` is called with failed_detections=["networkIsolated", "serviceMesh"]
**Then**: `event_data.actual_instance.failed_detections` == ["networkIsolated", "serviceMesh"]

**Acceptance Criteria**:
- List is preserved exactly
- Empty list `[]` is acceptable when all detections succeed

---

### UT-HAPI-533-005: Enrichment completed event remediation_history_fetched

**BR**: BR-AUDIT-005
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_audit_enrichment_events.py`

**Given**: Phase 2 successfully fetched remediation history from DataStorage
**When**: `create_enrichment_completed_event()` is called with remediation_history_fetched=True
**Then**: `event_data.actual_instance.remediation_history_fetched` is True

**Acceptance Criteria**:
- Boolean, not string or int
- True when DS returned history, False when DS was unreachable or returned no data

---

### UT-HAPI-533-006: Enrichment completed event owner_chain_length

**BR**: BR-AUDIT-005
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_audit_enrichment_events.py`

**Given**: Phase 2 resolved owner chain of length 3 (Pod → ReplicaSet → Deployment)
**When**: `create_enrichment_completed_event()` is called with owner_chain_length=3
**Then**: `event_data.actual_instance.owner_chain_length` == 3

**Acceptance Criteria**:
- Integer type
- 1 when the resource has no parent (root_owner == affected_resource)

---

### UT-HAPI-533-007: Enrichment failed event ADR-034 envelope

**BR**: BR-AUDIT-005
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_audit_enrichment_events.py`

**Given**: Phase 2 enrichment failed with reason="rca_incomplete", detail="resolve_owner_chain failed after 3 retries"
**When**: `create_enrichment_failed_event()` is called with incident_id="inc-123", remediation_id="rem-456", the failure data, and affected_resource
**Then**: The returned `AuditEventRequest` has:
- `version` == "1.0"
- `event_category` == "aiagent"
- `event_type` == "aiagent.enrichment.failed"
- `event_action` == "enrichment_failed"
- `event_outcome` == "failure"
- `actor_type` == "Service"
- `actor_id` == "kubernaut-agent"

**Acceptance Criteria**:
- Envelope structure matches the completed event except for outcome and action

---

### UT-HAPI-533-008: Enrichment failed event reason and detail

**BR**: BR-AUDIT-005
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_audit_enrichment_events.py`

**Given**: EnrichmentFailure with reason="rca_incomplete" and detail="resolve_owner_chain failed after 3 retries: ConnectionError"
**When**: `create_enrichment_failed_event()` is called
**Then**: `event_data.actual_instance` contains:
- `reason` == "rca_incomplete"
- `detail` == "resolve_owner_chain failed after 3 retries: ConnectionError"

**Acceptance Criteria**:
- Both fields are strings
- Detail preserves the full error context including the original exception message

---

### UT-HAPI-533-009: Enrichment failed event affected_resource

**BR**: BR-AUDIT-005
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_audit_enrichment_events.py`

**Given**: Enrichment was attempted for affected_resource={"kind": "Pod", "name": "api-xyz", "namespace": "prod"}
**When**: `create_enrichment_failed_event()` is called with the affected_resource dict
**Then**: `event_data.actual_instance.affected_resource_kind` == "Pod", `affected_resource_name` == "api-xyz", `affected_resource_namespace` == "prod"

**Acceptance Criteria**:
- Affected resource fields are flattened, consistent with root_owner fields in completed event
- Captures what was attempted, not what was resolved (since resolution failed)

---

### UT-HAPI-533-010: Enrichment failed event uses remediation_id as correlation

**BR**: BR-AUDIT-005
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_audit_enrichment_events.py`

**Given**: remediation_id="rem-abc-123"
**When**: `create_enrichment_failed_event()` is called
**Then**: `event.correlation_id` == "rem-abc-123"

**Acceptance Criteria**:
- Same correlation pattern as all other aiagent events (see `create_llm_request_event`)

---

### UT-HAPI-533-011: Enrichment completed event with partial data

**BR**: BR-AUDIT-005
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_audit_enrichment_events.py`

**Given**: Phase 2 succeeded but label detector was unavailable (detected_labels=None) and DS was unreachable (remediation_history_fetched=False), with root_owner={kind: "Deployment", name: "api", namespace: "default"} and owner_chain_length=2
**When**: `create_enrichment_completed_event()` is called with detected_labels=None, failed_detections=None, remediation_history_fetched=False
**Then**: Event is created successfully with:
- `event_outcome` == "success" (enrichment succeeded, just with partial data)
- `detected_labels_summary` is None
- `failed_detections` is None
- `remediation_history_fetched` is False
- `root_owner_kind`, `root_owner_name`, `root_owner_namespace` are populated

**Acceptance Criteria**:
- Factory function does not raise when optional fields are None
- Partial data is a valid production scenario (k8s available but DS is not)

---

### UT-HAPI-533-012: Enrichment completed event with cluster-scoped resource

**BR**: BR-AUDIT-005
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_audit_enrichment_events.py`

**Given**: Phase 2 resolved root_owner={kind: "Node", name: "worker-1"} (no namespace — cluster-scoped resource)
**When**: `create_enrichment_completed_event()` is called with root_owner_namespace=""
**Then**: `event_data.actual_instance.root_owner_namespace` == ""

**Acceptance Criteria**:
- Empty string for namespace is valid, not None
- Consistent with how `EnrichmentResult.root_owner` represents cluster-scoped resources

---

### UT-HAPI-533-013: Enrichment failed event with missing remediation_id

**BR**: BR-AUDIT-005
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_audit_enrichment_events.py`

**Given**: Enrichment failed but remediation_id is None (early failure before RR is created)
**When**: `create_enrichment_failed_event()` is called with remediation_id=None
**Then**: `event.correlation_id` == "unknown"

**Acceptance Criteria**:
- Follows the same fallback pattern as `create_aiagent_response_failed_event` which uses `remediation_id or "unknown"`
- Event is still created and storable even without a correlation ID

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: pytest (HAPI Python convention; class-based `TestXxx` with `test_` methods)
- **Mocks**: None required — factory functions are pure logic operating on Pydantic models
- **Location**: `kubernaut-agent/tests/unit/test_audit_enrichment_events.py`
- **Dependencies**: OpenAPI-generated Pydantic models must exist before tests can import them
- **conftest**: Uses existing `kubernaut-agent/tests/unit/conftest.py` (provides prometrix mock, config files, session reset)

### Integration Tests

- Not applicable (see Tier Skip Rationale in Section 5)

---

## 8. Execution Order (Dependency Chain)

The implementation follows a strict dependency chain. Each step must complete before the next:

```
Step 1: OpenAPI Schema (api/openapi/data-storage-v1.yaml)
  └─ Add AIAgentEnrichmentCompletedPayload schema
  └─ Add AIAgentEnrichmentFailedPayload schema
  └─ Add to oneOf list in AuditEventRequestEventData
  └─ Add discriminator mapping entries

Step 2: Regenerate Python DS Client
  └─ make generate-datastorage-client-python
  └─ Verify new models appear in datastorage/models/

Step 3: Re-export in audit_models.py
  └─ Import and alias new payload types

Step 4: TDD RED — Write failing tests (test_audit_enrichment_events.py)
  └─ Tests will fail with ImportError (factory functions don't exist yet)

Step 5: TDD GREEN — Implement factory functions (events.py)
  └─ Add create_enrichment_completed_event()
  └─ Add create_enrichment_failed_event()
  └─ Export from audit/__init__.py

Step 6: TDD VERIFY GREEN — All 13 tests pass

Step 7: Add emission call sites in llm_integration.py
  └─ After enrichment success (line ~835)
  └─ In EnrichmentFailure except block (line ~813)

Step 8: TDD REFACTOR — Clean up, verify consistency
```

---

## 9. Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| OpenAPI schema change breaks Go ogen build | Medium | High | Only add schemas + mappings; don't modify existing schemas. Run `go build ./...` after regeneration to confirm. |
| Python client regeneration introduces regressions | Low | High | Run full `make test-unit-kubernaut-agent` after regeneration to verify existing tests still pass. |
| New payload field names conflict with existing OpenAPI types | Low | Medium | Prefix with `AIAgentEnrichment*` to stay in the `aiagent.*` namespace. Check for name collisions before adding. |
| Partial enrichment data causes None-field serialization issues | Medium | Medium | UT-HAPI-533-011 explicitly tests this boundary. Factory function uses `Optional` for nullable fields. |
| `signalprocessing.enrichment.completed` discriminator value already exists | High | High | The existing discriminator has `'signalprocessing.enrichment.completed'` mapped to `SignalProcessingAuditPayload`. Our new events use `'aiagent.enrichment.completed'` and `'aiagent.enrichment.failed'` — different prefix, no collision. Verified in OpenAPI spec. |

---

## 10. Execution Commands

```bash
# Unit tests (all HAPI)
make test-unit-kubernaut-agent

# Specific test file
cd kubernaut-agent && source venv/bin/activate && python -m pytest tests/unit/test_audit_enrichment_events.py -v

# Specific test by ID
cd kubernaut-agent && source venv/bin/activate && python -m pytest tests/unit/test_audit_enrichment_events.py -k "ut_hapi_533_001" -v

# Regenerate Python DS client
make generate-datastorage-client-python

# Verify no Go build regression
go build ./...
```

---

## 11. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan for #533 SOC2 audit event gaps |
| 1.1 | 2026-03-04 | Risk mitigation pass: added 3 boundary tests (011-013), execution order dependency chain, risk register, OpenAPI schema collision analysis, clarified Pydantic model sourcing (generated not hand-coded), documented `signalprocessing.enrichment.completed` non-collision, added non-testable infrastructure inventory |
