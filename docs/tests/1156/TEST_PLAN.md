# Test Plan: AF SOC2 Audit Normalization (Issue #1156)

**IEEE 829 Test Plan — Version 1.0**

| Field | Value |
|---|---|
| Test Plan ID | TP-AF-1156-001 |
| Issue | [#1156](https://github.com/jordigilh/kubernaut/issues/1156) |
| Service | ApiFrontend (AF) |
| Date | 2026-05-19 |
| Status | Draft |
| Author | AI Agent |
| Business Requirement | BR-AUDIT-005 (SOC2 AU-2 Compliance) |

---

## 1. Introduction

### 1.1 Purpose

This test plan defines the testing strategy for normalizing the AF audit system from a private `BufferedEmitter` to the shared `pkg/audit.AuditStore`, with per-event typed OpenAPI payloads for all 30 AF auditable event types.

### 1.2 Scope

**In scope:**
- Store adapter: `Event` -> `AuditEventRequest` conversion with typed payloads (PR1)
- 16 already-emitted event types: correct mapping, payload construction, field population
- 14 newly-wired event types: emission, payload construction, field population (PR2)
- Event consolidation: `rbac.denied` + `mcp.tool_denied` -> `auth.access_denied`; `tool.invoked` + `mcp.tool_invoked` -> `tool.executed`
- Dead code removal: `BufferedEmitter`, `Writer`, `LogEmitter`, `EmitFromContext`, `WriteAuditEvents`
- Integration with shared `BufferedAuditStore` in `cmd/apifrontend/main.go`

**Out of scope:**
- `session.tampered` detection (v1.6)
- DataStorage backend changes
- Other services' audit systems

### 1.3 References

| Document | Path |
|---|---|
| Predecessor plan | `.cursor/plans/af_soc2_audit_normalization_9865d54a.plan.md` |
| Implementation plan | `.cursor/plans/af_soc2_audit_prs_e6b9c794.plan.md` |
| Audit store interface | `pkg/audit/store.go` |
| AF audit types | `pkg/apifrontend/audit/audit.go` |
| OpenAPI spec | `api/openapi/data-storage-v1.yaml` |
| DD-AUDIT-002 | `docs/architecture/decisions/DD-AUDIT-002-audit-shared-library-design.md` |
| DD-AUDIT-003 | `docs/architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md` |
| 100 Go Mistakes | https://100go.co |

---

## 2. Test Items

### 2.1 PR1 — Normalize AF Audit to Shared Store

| Item | Component | Type |
|---|---|---|
| TI-01 | `pkg/apifrontend/audit/store_adapter.go` | NEW — Core adapter |
| TI-02 | `pkg/apifrontend/audit/audit.go` | MODIFIED — EventType constants, CorrelationID |
| TI-03 | `cmd/apifrontend/main.go` | MODIFIED — Shared store wiring |
| TI-04 | `api/openapi/data-storage-v1.yaml` | MODIFIED — 19 new payload schemas |
| TI-05 | `pkg/datastorage/ogen-client/*.go` | REGENERATED — Codegen |

### 2.2 PR2 — Wire Missing Events + Enrichment

| Item | Component | Type |
|---|---|---|
| TI-06 | `pkg/apifrontend/tools/af_create_rr.go` | MODIFIED — rr.created/deduplicated |
| TI-07 | `pkg/apifrontend/tools/ka_tools.go` | MODIFIED — ka.delegated/result_received, user.decision |
| TI-08 | `pkg/apifrontend/severity/triage.go` | MODIFIED — severity_triage.completed/failed |
| TI-09 | `pkg/apifrontend/auth/dynamic_impersonation.go` | MODIFIED — impersonation.created |
| TI-10 | `pkg/apifrontend/auth/jwt_delegation.go` | MODIFIED — jwt.delegation |
| TI-11 | `pkg/apifrontend/handler/mcp.go` | MODIFIED — mcp.session_init |
| TI-12 | `pkg/apifrontend/session/statemachine.go` | MODIFIED — session.completed |
| TI-13 | `pkg/apifrontend/launcher/launcher.go` | MODIFIED — triage.started/completed |
| TI-14 | `pkg/apifrontend/session/service.go` | MODIFIED — session.created enrichment |
| TI-15 | `pkg/apifrontend/agent/root.go` | MODIFIED — auth.access_denied, tool.executed |

---

## 3. Features to Be Tested

### 3.1 Adapter Event Mapping (PR1)

| Feature | Acceptance Criteria |
|---|---|
| F-01: Event type prefix | `AuditEventRequest.EventType` = `"apifrontend." + Event.Type` |
| F-02: Event category | `AuditEventRequest.EventCategory` = `"apifrontend"` (always) |
| F-03: Event action mapping | Each EventType maps to correct `event_action` string |
| F-04: Event outcome derivation | Success-path events = `success`; failure-path = `failure` |
| F-05: Actor attribution | `UserID` present -> `actor_type: "user"`, `actor_id: UserID`; absent -> `actor_type: "service"`, `actor_id: "apifrontend"` |
| F-06: CorrelationID cascade | Priority: `CorrelationID` > `RequestID` > synthetic UUID |
| F-07: Typed payload construction | Each EventType constructs the correct ogen payload struct |
| F-08: Event consolidation | Old `rbac.denied`/`mcp.tool_denied` -> `auth.access_denied`; old `tool.invoked`/`mcp.tool_invoked` -> `tool.executed` |
| F-09: Close delegation | `StoreAdapter.Close()` calls `AuditStore.Close()` |

### 3.2 Missing Event Wiring (PR2)

| Feature | Acceptance Criteria |
|---|---|
| F-10: circuitbreaker.trip | Emitted when circuit breaker trips, payload has `circuit_name`, `failure_count` |
| F-11: impersonation.created | Emitted when K8s impersonated client created, payload has `target_user`, `groups` |
| F-12: jwt.delegation | Emitted when JWT forwarded to KA, payload has `target_service` |
| F-13: severity_triage.completed | Emitted after triage pipeline succeeds, payload has `severity`, `source_tier` |
| F-14: severity_triage.failed | Emitted after triage pipeline fails, payload has `error`, `failed_tier` |
| F-15: mcp.session_init | Emitted when MCP session initialized, payload has `session_id`, `protocol_version` |
| F-16: session.completed | Emitted at terminal phase, payload has `session_id`, `total_duration_ms` |
| F-17: triage.started | Emitted at agent entry, payload has `session_id`, `persona` |
| F-18: triage.completed | Emitted at agent completion, payload has `session_id`, `outcome`, `duration_ms` |
| F-19: rr.created | Emitted after RR creation, payload has `rr_name`, `namespace` |
| F-20: rr.deduplicated | Emitted at dedup detection, payload has `fingerprint` |
| F-21: ka.delegated | Emitted after KA Analyze call, payload has `session_id` |
| F-22: ka.result_received | Emitted on KA result, payload has `session_id`, `result_type` |
| F-23: user.decision | Emitted after workflow selection, payload has `session_id`, `decision` |

### 3.3 Enrichment (PR2)

| Feature | Acceptance Criteria |
|---|---|
| F-24: session.created enrichment | Detail includes `a2a_task_id`, `join_mode`, `user_identity`, `rr_ref` |
| F-25: auth.access_denied enrichment | Detail includes `user_role`, `required_roles`, `endpoint` |
| F-26: tool.executed enrichment | Detail includes `session_id`, `execution_duration_ms`, `tool_outcome` |
| F-27: TTL event enrichment | Detail includes `session_id` |

---

## 4. Features Not Tested

| Feature | Reason |
|---|---|
| DataStorage ingestion | DS handles JSONB storage; tested by DS's own test suite |
| OpenAPI validation in DS | `ValidateAuditEventRequest` tested in `pkg/audit/` |
| Other services' audit | Out of scope for AF normalization |
| `session.tampered` | Deferred to v1.6 |

---

## 5. Approach

### 5.1 Test Pyramid

| Tier | Prefix | Target | Coverage Goal |
|---|---|---|---|
| Unit | UT-AF-1156 | Adapter logic, payload construction, field mapping | >= 80% of adapter code |
| Integration | IT-AF-1156 | Adapter + real BufferedAuditStore + DS round-trip | >= 80% of wiring code |
| E2E | E2E-AF-1156 | Full A2A flow -> DS audit query | Behavioral assurance on critical path |

### 5.2 Testing Framework

- **Ginkgo/Gomega BDD** (mandatory per project rules)
- Table-driven tests for adapter (30 event types, per Go mistake #85)
- `Eventually()` for async assertions (no `time.Sleep`, per Go mistake #86)
- Race flag enabled (`-race`) in CI (per Go mistake #83)
- No `XIt`/pending tests (per project TDD rules)
- Fresh state per test via `BeforeEach` (no test pollution)

### 5.3 Mock Strategy

| Dependency | Mock? | Rationale |
|---|---|---|
| `audit.AuditStore` | YES (unit) | Capture events without DS |
| DataStorage | NO (integration) | Real DS for round-trip validation |
| Kubernetes API | YES | Use `fake.NewClientBuilder()` for session/CRD tests |
| LLM / HolmesGPT | YES | Not relevant to audit emission |

---

## 6. Test Scenarios

### 6.1 Unit Tests — PR1 Adapter (UT-AF-1156-001..040)

#### 6.1.1 Event Type Mapping (UT-AF-1156-001..030)

Table-driven test: for each of the 30 event types, verify:
- `AuditEventRequest.EventType` equals `"apifrontend." + eventType`
- `AuditEventRequest.EventData` contains correct payload struct type
- Payload `event_type` field matches discriminator

| Test ID | Event Type | Expected Payload Struct |
|---|---|---|
| UT-AF-1156-001 | `auth.success` | `ApifrontendAuthSuccessPayload` (NEW) |
| UT-AF-1156-002 | `auth.failure` | `ApifrontendAuthFailurePayload` (NEW) |
| UT-AF-1156-003 | `ratelimit.denied` | `ApifrontendRatelimitDeniedPayload` (NEW) |
| UT-AF-1156-004 | `session.created` | `ApifrontendSessionCreatedPayload` (existing) |
| UT-AF-1156-005 | `session.phase_changed` | `ApifrontendSessionPhaseChangedPayload` (NEW) |
| UT-AF-1156-006 | `session.deleted` | `ApifrontendSessionDeletedPayload` (NEW) |
| UT-AF-1156-007 | `session.auto_cancelled` | `ApifrontendSessionAutoCancelledPayload` (NEW) |
| UT-AF-1156-008 | `session.retention_deleted` | `ApifrontendSessionRetentionDeletedPayload` (NEW) |
| UT-AF-1156-009 | `session.completed` | `ApifrontendSessionCompletedPayload` (existing) |
| UT-AF-1156-010 | `a2a.task_started` | `ApifrontendA2ATaskStartedPayload` (NEW) |
| UT-AF-1156-011 | `a2a.task_completed` | `ApifrontendA2ATaskCompletedPayload` (NEW) |
| UT-AF-1156-012 | `a2a.task_failed` | `ApifrontendA2ATaskFailedPayload` (NEW) |
| UT-AF-1156-013 | `mcp.tool_failed` | `ApifrontendMCPToolFailedPayload` (NEW) |
| UT-AF-1156-014 | `mcp.session_init` | `ApifrontendMCPSessionInitPayload` (NEW) |
| UT-AF-1156-015 | `config.reloaded` | `ApifrontendConfigReloadedPayload` (NEW) |
| UT-AF-1156-016 | `config.rejected` | `ApifrontendConfigRejectedPayload` (NEW) |
| UT-AF-1156-017 | `circuitbreaker.trip` | `ApifrontendCircuitbreakerTripPayload` (NEW) |
| UT-AF-1156-018 | `impersonation.created` | `ApifrontendImpersonationCreatedPayload` (NEW) |
| UT-AF-1156-019 | `jwt.delegation` | `ApifrontendJWTDelegationPayload` (NEW) |
| UT-AF-1156-020 | `severity_triage.completed` | `ApifrontendSeverityTriageCompletedPayload` (NEW) |
| UT-AF-1156-021 | `severity_triage.failed` | `ApifrontendSeverityTriageFailedPayload` (NEW) |
| UT-AF-1156-022 | `triage.started` | `ApifrontendTriageStartedPayload` (existing) |
| UT-AF-1156-023 | `triage.completed` | `ApifrontendTriageCompletedPayload` (existing) |
| UT-AF-1156-024 | `rr.created` | `ApifrontendRRCreatedPayload` (existing) |
| UT-AF-1156-025 | `rr.deduplicated` | `ApifrontendRRDeduplicatedPayload` (existing) |
| UT-AF-1156-026 | `ka.delegated` | `ApifrontendKADelegatedPayload` (existing) |
| UT-AF-1156-027 | `ka.result_received` | `ApifrontendKAResultReceivedPayload` (existing) |
| UT-AF-1156-028 | `user.decision` | `ApifrontendUserDecisionPayload` (existing) |
| UT-AF-1156-029 | `auth.access_denied` | `ApifrontendAuthAccessDeniedPayload` (existing) |
| UT-AF-1156-030 | `tool.executed` | `ApifrontendToolExecutedPayload` (existing) |

#### 6.1.2 Cross-Cutting Adapter Behavior (UT-AF-1156-031..040)

| Test ID | Feature | Description | Pass Criteria |
|---|---|---|---|
| UT-AF-1156-031 | F-03 | event_action mapping | Each event type produces correct action string (table-driven) |
| UT-AF-1156-032 | F-04 | event_outcome for success-path | `auth.success` -> `success`, `session.created` -> `success` |
| UT-AF-1156-033 | F-04 | event_outcome for failure-path | `auth.failure` -> `failure`, `a2a.task_failed` -> `failure` |
| UT-AF-1156-034 | F-05 | Actor: user present | UserID="alice" -> `actor_type:"user"`, `actor_id:"alice"` |
| UT-AF-1156-035 | F-05 | Actor: system event | UserID="" -> `actor_type:"service"`, `actor_id:"apifrontend"` |
| UT-AF-1156-036 | F-06 | CorrelationID: primary | `CorrelationID` set -> used as-is |
| UT-AF-1156-037 | F-06 | CorrelationID: fallback | `CorrelationID` empty, `RequestID` set -> `RequestID` used |
| UT-AF-1156-038 | F-06 | CorrelationID: synthetic | Both empty -> UUID generated (non-empty, valid format) |
| UT-AF-1156-039 | F-02 | event_category | Always `"apifrontend"` regardless of event type |
| UT-AF-1156-040 | F-09 | Close delegation | `Close()` calls underlying `AuditStore.Close()` |

### 6.2 Unit Tests — PR2 Missing Events (UT-AF-1156-050..075)

#### 6.2.1 New Event Emission (UT-AF-1156-050..063)

| Test ID | Event | Component | Trigger | Verified Fields |
|---|---|---|---|---|
| UT-AF-1156-050 | `circuitbreaker.trip` | circuit breaker setup | AuditFunc callback | `circuit_name`, `failure_count` |
| UT-AF-1156-051 | `impersonation.created` | `DynamicClientFactory` | Client created for user | `target_user`, `groups` |
| UT-AF-1156-052 | `jwt.delegation` | `JWTDelegationTransport` | JWT forwarded | `target_service` |
| UT-AF-1156-053 | `severity_triage.completed` | `severity/triage.go` | Triage succeeds | `severity`, `source_tier` |
| UT-AF-1156-054 | `severity_triage.failed` | `severity/triage.go` | Triage fails | `error`, `failed_tier` |
| UT-AF-1156-055 | `mcp.session_init` | `handler/mcp.go` | New MCP session | `mcp_session_id`, `protocol_version` |
| UT-AF-1156-056 | `session.completed` | `session/statemachine.go` | `IsTerminal(to)` | `session_id`, `total_duration_ms` |
| UT-AF-1156-057 | `triage.started` | `session/service.go` | Session created | `session_id`, `persona` |
| UT-AF-1156-058 | `triage.completed` | `launcher/launcher.go` | AfterExecuteCallback | `session_id`, `triage_outcome`, `triage_duration_ms` |
| UT-AF-1156-059 | `rr.created` | `tools/af_create_rr.go` | RR created | `rr_name`, `rr_namespace` |
| UT-AF-1156-060 | `rr.deduplicated` | `tools/af_create_rr.go` | Dedup detected | `fingerprint`, `existing_rr_name` |
| UT-AF-1156-061 | `ka.delegated` | `tools/ka_tools.go` | KA Analyze called | `session_id`, `ka_correlation_id` |
| UT-AF-1156-062 | `ka.result_received` | `tools/ka_tools.go` | KA result received | `session_id`, `result_type` |
| UT-AF-1156-063 | `user.decision` | `tools/ka_tools.go` | Workflow selected | `session_id`, `decision` |

#### 6.2.2 Enrichment Tests (UT-AF-1156-070..075)

| Test ID | Event | Enriched Fields | Pass Criteria |
|---|---|---|---|
| UT-AF-1156-070 | `session.created` | `a2a_task_id`, `join_mode`, `user_identity`, `rr_ref` | All fields populated from `CreateConfig` |
| UT-AF-1156-071 | `auth.access_denied` | `user_role`, `required_roles`, `endpoint` | All fields from RBAC context |
| UT-AF-1156-072 | `tool.executed` | `session_id`, `execution_duration_ms`, `tool_outcome` | Duration > 0, outcome matches |
| UT-AF-1156-073 | `session.auto_cancelled` | `session_id` (was `session` key) | Normalized key name |
| UT-AF-1156-074 | `session.retention_deleted` | `session_id` (was `session` key) | Normalized key name |
| UT-AF-1156-075 | `session.created` CorrelationID | `CorrelationID` | Set from `RemediationRequestRef.Name` |

### 6.3 Integration Tests (IT-AF-1156-001..012)

#### 6.3.1 PR1 Integration (IT-AF-1156-001..003)

| Test ID | Feature | Description | Infrastructure |
|---|---|---|---|
| IT-AF-1156-001 | F-07 | Session created -> flush -> query DS -> typed payload | Real DS, `BufferedAuditStore`, `StoreAdapter` |
| IT-AF-1156-002 | F-07 | Auth success -> flush -> query DS -> typed payload | Real DS |
| IT-AF-1156-003 | F-09 | Graceful shutdown: `Close()` flushes all buffered events | Real DS |

#### 6.3.2 PR2 Integration (IT-AF-1156-010..012)

| Test ID | Feature | Description | Infrastructure |
|---|---|---|---|
| IT-AF-1156-010 | F-16 | session.completed emits `duration_ms` with real K8s CreationTimestamp (envtest) | envtest + real K8s API |
| IT-AF-1156-011 | F-* | Multi-event pipeline: 5 different event types round-trip through BufferedAuditStore | Capturing DS client |
| IT-AF-1156-012 | F-19 | circuitbreaker.trip event includes state transition and dependency details | Capturing DS client |

### 6.4 E2E Tests (E2E-AF-1156-001)

| Test ID | Feature | Description | Infrastructure |
|---|---|---|---|
| E2E-AF-1156-001 | Full SOC2 trace | Full A2A flow -> query DS -> >= 5 distinct `apifrontend.*` event types | Kind cluster + real DS |

**Note**: E2E-FP-AF-001 requires AF+DEX deployed in the FP cluster (Issue #1189). The test gracefully skips if AF is not available. Once #1189 deploys AF as the 14th FP service, this test validates the full-stack audit trace.

---

## 7. Pass/Fail Criteria

### 7.1 Per-Tier Pass Criteria

| Tier | Criterion |
|---|---|
| Unit | All UT-AF-1156-* pass; >= 80% line coverage of `store_adapter.go` |
| Integration | All IT-AF-1156-* pass; typed payloads verified in DS |
| E2E | E2E-AF-1156-001 passes; >= 5 distinct AF event types in DS |

### 7.2 Overall Pass Criteria

- `go build ./...` succeeds with zero errors
- `go vet ./...` clean
- `golangci-lint run` no new warnings
- All 30 event types have typed payloads in OpenAPI spec
- SOC2 compliance matrix (AU-2, AU-3, AU-6, AU-8, AU-10, CC6.1, CC7.2, A1.2) satisfied
- No `time.Sleep()` in test code
- No `XIt`/pending tests
- No assertion-free `It` blocks

---

## 8. Test Deliverables

| Deliverable | Path |
|---|---|
| Test plan (this document) | `docs/tests/1156/TEST_PLAN.md` |
| Unit tests — adapter | `pkg/apifrontend/audit/store_adapter_test.go` |
| Unit tests — wiring | Per-component `*_test.go` files |
| Integration tests | `test/integration/apifrontend/audit_normalization_test.go` |
| E2E tests | `test/e2e/apifrontend/audit_sink_test.go` (extended) |

---

## 9. Test Schedule

| Phase | PR | TDD Stage | Tests |
|---|---|---|---|
| Phase 3 | PR1 | RED | UT-AF-1156-001..040 (failing) |
| Phase 4 | PR1 | GREEN | UT-AF-1156-001..040 (passing) |
| Phase 5 | PR1 | REFACTOR | Quality + 100-go-mistakes |
| Phase 8 | PR1 | RED | IT-AF-1156-001..003 (failing) |
| Phase 9 | PR1 | GREEN | IT-AF-1156-001..003 (passing) |
| Phase 12 | PR2 | RED | UT-AF-1156-050..075 (failing) |
| Phase 13 | PR2 | GREEN | UT-AF-1156-050..075 (passing) |
| Phase 14 | PR2 | REFACTOR | Quality + 100-go-mistakes |
| Phase 15 | PR2 | IT+E2E | IT-AF-1156-010..012, E2E-AF-1156-001 |

---

## 10. Go Testing Anti-Patterns Avoided

| Anti-Pattern | Mitigation |
|---|---|
| `time.Sleep()` in tests (#86) | Use `Eventually()` with timeout/polling |
| Missing race detection (#83) | `-race` flag in CI |
| No table-driven tests (#85) | 30-entry table for adapter event mapping |
| Not using httptest (#88) | Used for HTTP-level integration |
| Pending/skipped tests | No `XIt`, `Skip()`, or `Pending()` |
| Assertion-free tests | Every `It` block has at least one `Expect()` |
| Test pollution | `BeforeEach` for fresh state; no shared mutable globals |
| Hardcoded ports | Ephemeral ports or test infrastructure constants |

---

## 11. Risks and Mitigations

| Risk | Severity | Mitigation |
|---|---|---|
| Ogen codegen diff large | MEDIUM | Verify no unexpected changes; review generated code |
| 30-case switch in adapter | LOW | Table-driven tests catch every case; linter catches unhandled cases |
| ~23 test call sites need update | MEDIUM | Mechanical: pass `nil` auditor; compile-time verification |
| Integration tests require real DS | LOW | Existing test infrastructure supports real DS |

---

## 12. Approvals

| Role | Name | Date | Status |
|---|---|---|---|
| Author | AI Agent | 2026-05-19 | Draft |
| Reviewer | | | Pending |
| Approver | | | Pending |
