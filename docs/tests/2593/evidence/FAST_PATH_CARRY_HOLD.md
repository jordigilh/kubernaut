# #2593 Fast-Path Carry/Hold Manifest

**Date:** 2026-11-13  
**Implementation worktree:** `/private/var/folders/r7/gktmmltd1zq7wqhsjjslwsm80000gn/T/opencode/2593-fmc-implementation`  
**Branch:** `work/2593-fmc-presence`  
**Baseline:** `7a4ed304580298802520e292cad0a7c4efccd17b`  
**Original draft worktree:** `/Users/jgil/.local/share/opencode/worktree/d1eaa5e864fe7029869e1624207d8e393b28d5ca/v16-acm-inventory` (`triage/v16-acm-inventory`)

**Boundary status:** COMPLETE — 2026-11-13. The copied test-plan/review metadata now
identifies this clean worktree as the active RED baseline; the functional scenarios and
held ownership boundaries were not broadened.

This is a scope/ownership record, not a qualification or release approval. The
original draft worktree was not reset, cleaned, deleted, or overwritten. Its
pre-split inventory remains `72` modified tracked paths and `23` standard
untracked paths.

## Carried into the clean worktree

- #2593 authority artifacts: `docs/tests/2593/`, `DD-FLEET-011`, and its design-decision index entry.
- Typed MCP observation bridge and strict pagination/content validation: `pkg/fleet/observation/`, `pkg/fleet/mcpclient/observation.go`, the MCP client/tests, and mock-gateway structured-content support.
- FMC synchronization foundation: `pkg/fleet/fmc/syncer.go`, its tests, presence-aware writer/read path, and the FMC broker-readiness fixture.
- Internal Valkey/Lease lifecycle foundation: `pkg/fleet/scopecache/`, including the embedded `fmc_v2_atomic.lua` internal script.
- Registration incarnation/revision fencing: `pkg/fleet/registry/` types, registries, and lifecycle tests.
- Typed scope/GVK and fail-closed label behavior: `pkg/shared/scope/`, federated scope checks, Gateway/RO propagation, signal-processing persistence/enrichment/owner-chain propagation, and matching tests/artifacts.
- Canonical identity validation: `pkg/fleet/inventory/`, FMC exact-read validation, and the minimum affected fixture updates. No ACM production adapter was carried.

## Held in the original worktree

These paths remain preserved draft/separate work and were not copied:

```text
api/openapi/data-storage-v1.yaml
pkg/apifrontend/policyread/history_bounds_2584_red_test.go
pkg/apifrontend/policyread/local_inventory.go
pkg/apifrontend/policyread/service.go
pkg/apifrontend/policyread/service_2584_test.go
pkg/datastorage/ogen-client/oas_json_gen.go
pkg/datastorage/ogen-client/oas_schemas_gen.go
pkg/datastorage/ogen-client/oas_validators_gen.go
pkg/fleet/acm/client.go
pkg/fleet/acm/inventory.go
pkg/fleet/fmc/handler.go
pkg/fleet/fmc/handler_test.go
pkg/fleet/fmc/http_client.go
pkg/fleet/fmc/http_client_test.go
pkg/remediationorchestrator/audit/manager.go
pkg/remediationorchestrator/audit/manager_test.go
pkg/shared/auth/middleware.go
test/e2e/fleetmetadatacache/shared/helpers.go
test/e2e/fleetmetadatacache/suite_test.go
test/infrastructure/fleet_api_diagnostics_test.go
test/infrastructure/fleet_e2e.go
test/integration/fleetmetadatacache/fmc_http_api_test.go
test/integration/remediationorchestrator/ro_fleet_scope_test.go
pkg/fleet/fmc/r13_scope_test.go
pkg/shared/auth/middleware_security_event_test.go
```

The held set includes all current public v2 route/media drafts, ACM/AF inventory
work, separately owned RO/audit/security-event work, generated external-gate
artifacts, and ambiguous changes. No item in the held set is discarded.

## Boundary checks

- Public FMC boundary remains `/api/v1/scope/check`, ordinary `application/json`, and `managed`.
- No public v2 route or media registration was carried.
- The original worktree's tracked and untracked path inventories match the pre-split capture.
- `go build ./...` passes in the clean worktree.
- Focused carried-package tests pass; full qualification and GREEN/REFACTOR evidence remain pending.
