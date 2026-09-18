# Implementation Plan: Fleet Boundary and Audit Provenance Corrections

**Plan Identifier**: IP-2417-v1
**Feature**: Implement issues #2419, #2422, #2426, and #2430 with full pyramid coverage and the missing remote WE E2E proof.
**Created**: 2026-09-17
**Status**: Active
**Branch**: `fix/2417-ka-fleet-overlay-all-flows`
**Test plan**: `docs/tests/2417/TEST_PLAN.md`

## Preflight Decision

Confidence: 96%.

The production callers and test boundaries are known. #2419, #2422, and #2430
require small production changes. #2426 already has the production helper wired
to all target-associated events, so implementation is test-only unless the
persistence proof exposes a real defect. The former FMC E2E gap is already covered
by the real Kuadrant and EAIGW `SyncJourney` suites. The remaining missing E2E
coverage is the remote `JobExecutor.IsCompleted` path.

## Phase 1: RED

1. `UT-KA-2419-001` and `IT-KA-2419-002`: empty fleet overlays fail before the
   enrichment runner can use hub-local access.
2. `UT-AW-2422-001` and `IT-AW-2422-002`: validator-generated DELETE audit events
   carry the NotificationRequest's `Spec.ClusterID` through Data Storage.
3. `UT-AA-2426-001` and `IT-AA-2426-002`: target-associated AIAnalysis audit
   events persist `Spec.ClusterID`; hub-local events remain unset.
4. `UT-GW-2430-001` and `IT-GW-2430-002`: remote reader construction failures are
   returned to signal parsing and cannot select the hub resolver.
5. `E2E-FLEET-012`: a real WorkflowExecution collision causes the production
   reconciler to read Job state on the remote cluster through MCP.

## Phase 2: GREEN

1. Add `len(overlay) == 0` fail-closed validation in `runEnrichment`.
2. Add `audit.SetClusterID(auditEvent, nr.Spec.ClusterID)` to
   `NotificationRequestValidator.ValidateDelete`.
3. Preserve the existing `setAnalysisClusterID` helper and make no production
   change unless the new integration assertion fails.
4. Change `resolverForCluster` to return `(types.OwnerResolver, error)`, return
   a contextual error for unavailable remote reader infrastructure, and propagate
   errors through both `Parse` and `ParseBatch`.
5. Add only the E2E fixture/helper code required to trigger the real remote WE
   collision path; do not add a mock gateway or direct `IsCompleted` seam.

## Phase 3: Checkpoint W

- Verify every component in the test plan's wiring manifest has a production
  caller outside test files.
- Run the listed IT suites and confirm they exercise production dispatch paths.
- Confirm no new package component is orphaned.
- Confirm the new E2E test uses the deployed WorkflowExecution controller and
  remote MCP gateway, not a fake client for the path under proof.

## Phase 4: REFACTOR

- Improve error context and log fields for remote resolver failures.
- Update stale fallback comments and test descriptions.
- Keep the change minimal; #2426 remains test-only if production behavior is
  already correct.

## Phase 5: Verification

Run the project-required build, lint, unit, integration, E2E, and TDD compliance
checks. Inspect the final diff for business-requirement references, control
objective assertions, error handling, and the complete wiring manifest.
