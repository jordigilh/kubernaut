# Test Plan: KA Fleet Overlay and Interactive Flow Integrity

**Test Plan Identifier**: TP-2417-v1
**Feature**: Fail-closed fleet overlay routing and signal-context propagation across autonomous, interactive, discovery, selection, and reconstruction flows.
**Version**: 1.0
**Created**: 2026-09-16
**Author**: Kubernaut maintainers
**Status**: Active
**Branch**: `fix/2417-ka-fleet-overlay-all-flows`

## 1. Purpose

Issue #2417 consolidates fleet-routing gaps where a target-cluster investigation could silently use hub tools, where signal resolution errors were downgraded to empty context, and where interactive selection or reconstruction lost the target cluster. This plan verifies the business outcome: every fleet-targeted KA path either uses the target overlay or stops with an observable error, while hub-local behavior remains unchanged.

## 2. Authority

- BR-INTEGRATION-1489: cluster-transparent fleet tool exposure.
- BR-INTEGRATION-054 and BR-INTEGRATION-065: fleet boundary enforcement and observability.
- BR-INTERACTIVE-003, BR-INTERACTIVE-004, BR-INTERACTIVE-005, and BR-INTERACTIVE-008: interactive lifecycle, selection, and reconstruction.
- BR-AI-056: detected-label propagation and scoring context.
- BR-AUDIT-005: correlation-queryable audit reconstruction.
- DD-FLEET-005: cluster-transparent tool exposure and fail-closed overlay resolution.
- Issue #2417 and related closed issues #1834, #2306, #2312, #2343, and #2345.

## 3. Risks and Mitigations

| ID | Risk | Mitigation |
|---|---|---|
| R1 | Empty or failed overlay causes investigation against the hub. | UT and IT assert non-nil errors, no overlay context, and the correct fleet failure audit event. |
| R2 | Signal resolver failures silently erase fleet target information. | Interactive message and discovery tests require resolver errors to surface. |
| R3 | Selection enrichment queries unscoped remediation history. | UT captures `EnrichRequest.ClusterID`; production route wiring passes the resolver. |
| R4 | Disconnect reconstruction loses fleet routing context. | Metadata round-trip UT and reconstruction context assertion; existing disconnect/E2E journeys remain regression coverage. |
| R5 | Failed label categories are incomplete. | Authoritative 13-category enumeration test includes `gitOpsTool`. |

## 4. Scope

### 4.1 Tested

- `prescopeFleetOverlay` nil resolver, resolver error, empty overlay, success, and hub-local no-op behavior.
- `InvestigateTool` start, message, and discovery signal resolution.
- `select_workflow` signal-aware enrichment and production wiring.
- Interactive lease metadata capture and reconstruction context restoration.
- All 13 `DetectedLabels` failure categories.
- Existing autonomous, interactive, discovery, reconstruction, and fleet gateway journeys.

### 4.2 Not Tested

- Gateway or kube-mcp-server implementation internals.
- New cluster discovery protocols; those remain covered by existing fleet gateway E2E tests.

## 5. Wiring Manifest

| Component | Production Entry Point | Wiring Location | IT/E2E Coverage |
|---|---|---|---|
| Fail-closed fleet overlay | `Investigator.Investigate`, `RunInteractiveTurn` | `investigator/fleet_overlay.go`, `investigator.go` | `IT-KA-FLEET-020`, `IT-KA-FLEET-029`, `E2E-FLEET-018` |
| Interactive signal resolution | `kubernaut_investigate` message/discovery/start | `mcp/tools/investigate_takeover.go`, `investigate_discovery.go`, `investigate_start.go` | `IT-KA-DISC-*`, `E2E-FLEET-018` |
| Selection cluster scoping | `kubernaut_select_workflow` | `cmd/kubernautagent/routes.go` and `mcp/tools/select_workflow.go` | `IT-KA-DISC-*`, `E2E-FLEET-018` |
| RCA discovery prescoping | `Investigator.RunWorkflowDiscoveryFromRCA` | `investigator/investigator_discovery.go` | `IT-KA-FLEET-2417` |
| Reconstruction signal context | disconnect callback to `RunReconTurn` | `cmd/kubernautagent/routes.go`, `mcp/reconstruct.go` | `IT-KA-TAKE-*`, `E2E-FLEET-018`, reconstruction E2E suite |
| Complete label category set | enrichment failure handling | `enrichment/label_detector.go` | label detector UT/IT suites and fleet post-RCA E2E |

## 6. Scenario Matrix

| BR | Tier | Scenario | Expected Outcome |
|---|---|---|---|
| BR-INTEGRATION-1489 / BR-INTEGRATION-054 | Unit | `UT-KA-FLEET-028` resolver unavailable, failed, or empty | Error is returned and the matching audit event is emitted. |
| BR-INTEGRATION-1489 | Integration | `IT-KA-FLEET-020`, `IT-KA-FLEET-029` | Production autonomous and interactive entry points fail closed. |
| BR-INTEGRATION-1489 / BR-AI-056 | Integration | `IT-KA-FLEET-2417` | Automatic RCA workflow discovery prescopes the remote overlay, detects `gitOpsManaged=true` and `gitOpsTool=argocd`, and selects the GitOps workflow. |
| BR-INTEGRATION-1489 / BR-INTEGRATION-065 | E2E | `E2E-FLEET-018` | Real interactive A2A message reaches the remote cluster, not hub tools. |
| BR-INTERACTIVE-003 / BR-INTERACTIVE-004 | Unit | `UT-KA-2417-001`, `UT-KA-TAKE-008`, `UT-KA-2417-002` | Resolver errors surface; signal metadata survives disconnect reconstruction. |
| BR-INTERACTIVE-005 | Unit/Integration | Selection enrichment cluster propagation | `EnrichRequest.ClusterID` and signal incident ID are authoritative. |
| BR-INTERACTIVE-008 | Integration/E2E | Existing reconstruction and disconnect journeys | Reconstructed turns retain target signal context and correlation. |
| BR-AI-056 | Unit | `UT-KA-433-131` category enumeration | All 13 authoritative labels, including `gitOpsTool`, are tracked. |
| BR-AUDIT-005 | Integration/E2E | Fleet failure and reconstruction audit traces | Events remain queryable by remediation correlation ID. |

## 7. Pass Criteria

1. All affected unit and integration tests pass with zero pending or skipped scenarios.
2. `go build ./...` succeeds.
3. `golangci-lint run --timeout=5m` reports no new findings.
4. Existing fleet E2E scenarios remain valid; `E2E-FLEET-018` proves the real interactive remote-cluster journey.
5. Changed business logic has complete unit coverage and every new production seam has a wiring test.
6. No fleet-target path silently falls back to hub tools after overlay or signal-resolution failure.
