# Test Plan: Issue #1460 — status/subscribe SSE Endpoint

**Business Requirement**: BR-AF-1460
**Design Document**: DD-AF-008
**Date**: 2026-06-18

## Scope

Tests for the `POST /a2a/status` endpoint that delivers RR phase transitions
as an SSE stream.

## Unit Tests (logic)

| ID | Description | File |
|----|------------|------|
| UT-AF-1460-001 | Valid status/subscribe request parses correctly, extracts rr_id | handler/status_handler_test.go |
| UT-AF-1460-002 | Missing rr_id returns JSON-RPC error -32602 (invalid_params) | handler/status_handler_test.go |
| UT-AF-1460-003 | Wrong method returns -32601 (method_not_found) | handler/status_handler_test.go |
| UT-AF-1460-004 | Malformed JSON returns -32600 (invalid_request) | handler/status_handler_test.go |
| UT-AF-1460-005 | buildPhaseMetadata for Executing phase returns workflow_id, started_at | handler/status_types_test.go |
| UT-AF-1460-006 | buildPhaseMetadata for Verifying phase returns verification_deadline, started_at, ea_phase, stabilization_deadline | handler/status_types_test.go |
| UT-AF-1460-007 | buildPhaseMetadata for Blocked phase returns blocked_until, block_reason, block_message | handler/status_types_test.go |
| UT-AF-1460-008 | buildPhaseMetadata for terminal phases returns outcome/failure_reason/skip_reason | handler/status_types_test.go |
| UT-AF-1460-030 | HandleWatch uses effectivenessAssessmentRef when available | tools/kubernaut_watch_test.go |

## Integration Tests (wiring)

| ID | Description | File |
|----|------------|------|
| IT-AF-1460-010 | REQ-2: subscribe to existing non-terminal RR receives current phase as first SSE event | status_subscribe_test.go |
| IT-AF-1460-011 | Phase transition: RR Processing->Executing emits SSE event with workflow_id metadata | status_subscribe_test.go |
| IT-AF-1460-012 | EA sub-phase: Verifying with effectivenessAssessmentRef emits ea_phase in metadata | status_subscribe_test.go |
| IT-AF-1460-013 | Watch reconnection: handler re-establishes watch transparently on channel close | status_subscribe_test.go |
| IT-AF-1460-014 | status/closing: pre-warning emitted before token deadline kills stream | status_subscribe_test.go |
| IT-AF-1460-015 | Terminal phase: Completed emits final:true with outcome, stream closes | status_subscribe_test.go |
| IT-AF-1460-016 | rr_not_found: subscribe to non-existent RR returns -32001 error | status_subscribe_test.go |
| IT-AF-1460-017 | Already-terminal: subscribe to Completed RR sends single final event, closes | status_subscribe_test.go |
| IT-AF-1460-018 | Heartbeat: received within 15s on idle stream | status_subscribe_test.go |
| IT-AF-1460-019 | Blocked phase: transition emits blocked_until, block_reason in metadata | status_subscribe_test.go |
| IT-AF-1460-020 | Route dispatch: POST /a2a/status dispatches to StatusHandler through auth chain | router_http_test.go |
| IT-AF-1460-040 | RBAC parity: AF ClusterRole grants get, list, watch on effectivenessassessments | tools_crd_test.go |

## Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|------------|
| StatusHandler | RouterConfig.StatusHandler | handler/router.go:NewRouter | IT-AF-1460-020 |
| POST /a2a/status route | mux.Handle in NewRouter | handler/router.go | IT-AF-1460-020 |
| StatusHandler construction | main() deps wiring | cmd/apifrontend/main.go | IT-AF-1460-020 |
| EA watch via effectivenessAssessmentRef | handleSubscribe | handler/status_handler.go | IT-AF-1460-012 |
| Watch reconnection loop | handleSubscribe | handler/status_handler.go | IT-AF-1460-013 |
| status/closing pre-warning | handleSubscribe | handler/status_handler.go | IT-AF-1460-014 |
| HandleWatch EA-ref migration | HandleWatch | tools/crd_tools.go | UT-AF-1460-030 |
| RBAC EA watch/list | ClusterRole | deploy/apifrontend/base/02-rbac.yaml | IT-AF-1460-040 |

## Coverage Targets

- Unit: >=80% of status_handler.go and status_types.go
- Integration: >=80% of status_handler.go through production HTTP stack
