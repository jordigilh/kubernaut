# DD-FLEET-009: Gateway-Neutral MCP Read Recovery and Binding Leases

**Status**: Approved design; implementation source remains gated on plan review  
**Date**: 2026-10-16  
**Related**: Issue #2518, `DD-FLEET-005`, `ADR-068`, `DD-AUDIT-003`, `ADR-034`  
**Business requirements**: `BR-KA-016`, `BR-HAPI-261`, `BR-AUDIT-005`, `BR-KA-OBSERVABILITY-001`, `BR-INTEGRATION-054`, `BR-INTEGRATION-065`, `BR-INTEGRATION-1489`

## Context

KA Fleet enrichment can encounter an EOF/reset, expired session, or bounded
HTTP failure while a gateway has session-scoped tool selection. The current
BridgeTool can retain a session snapshot, the writer path can retry ambiguous
mutations, and evidence/audit status can be lost or misclassified during
re-enrichment. EAIGW prefix filtering and Kuadrant session selection are
different adapter mechanisms, but recovery must have one safe core contract.

Live read-only evidence recorded in the canonical issue plan shows EAIGW
discovery, prefix binding, reconnect rediscovery and concurrent listing, and
Kuadrant discovery, selection/list/call isolation, concurrent target calls and
fresh-session rediscovery. The repository also has make-driven FMC E2E lanes
for both gateways (`make test-e2e-fleetmetadatacache-kuadrant` and
`make test-e2e-fleetmetadatacache-eaigw`) plus the broader EAIGW full-pipeline
lane (`make test-e2e-fleet`). Those lanes cover real Keycloak/RFC8693,
kube-mcp-server, FMC, dynamic registration, cross-cluster isolation, and the
EAIGW KA pipeline; they are existing conformance evidence, not a missing lane.
The PR-validation workflow includes all three in its E2E matrix, invokes the
corresponding Make target, and makes the aggregate E2E result a `Merge Gate`
dependency. The latest merged PR #2513 passed CI run `38919125785` with
`E2E (fleet)`, `E2E (fleetmetadatacache-kuadrant)`,
`E2E (fleetmetadatacache-eaigw)`, `E2E (fullpipeline)`, and `Merge Gate`
successful. Lane availability and a passing merge-gate execution are therefore
established; future implementation commits must still record their own run.
The earlier targeted EAIGW remote-target probe failed at gateway-side RFC8693
issuer routing (`https://keycloak:8443`, no route), which is a prerequisite for
that particular probe and not a general claim that the FMC EAIGW lane is
blocked. The live gateway result still does not prove production KA overlay
wiring, mutation safety, cancellation release, or generation/CAS fencing; those
remain mandatory local merge gates.

## Decision

1. **Provider-owned session generation/CAS.** The provider owns opaque session
   handles, generation publication and close/reconnect fencing. A stale
   reconnect cannot replace a newer handle; a losing session is closed with the
   same bounded operation context. `Close` fences publication before closing.
2. **Gateway-neutral `ToolBindingLease`.** The core resolves a trusted logical
   tool and target to an immutable binding and calls only through its lease.
   The lease owns `Binding()`, `CallTool()` and idempotent `Release()` and
   fences session and binding generations.
3. **Adapter-owned selection isolation.** EAIGW may use a prefix binding with a
   generation guard. Kuadrant must isolate the session per target or hold a
   lease across `select_tools → ListTools → CallTool`; releasing after
   `ListTools` is invalid. The core never branches on gateway type.
4. **Trusted logical read replay only.** `ReplayRead` is assigned by a trusted
   registration allow-list (`resources_get`, `resources_list`, and separately
   reviewed context reads). Wire names, annotations and discovered tools cannot
   elevate replayability. Unknown tools fail closed.
5. **Mutation `ReplayNever`.** Create, update, delete, approval, patch and
   execution calls are sent at most once. A failed mutation may be reconciled
   by an explicit subsequent read, but is never automatically resent by read
   recovery. The proof must traverse the executor factory, not only WriterClient.
6. **Typed bounded evidence and audit propagation.** Recovery/source outcomes
   use closed server-owned types and target/source keys. Status travels through
   investigation, AgentSession and AIAnalysis terminal paths, including failed
   and cancelled sessions. Audit events use one canonical event ID, correlation,
   target/source, bounded attempts and redacted failure details. Metrics use a
   dependency-free observer with fixed labels only.

## Normative recovery policy

- Maximum three attempts including the initial call; maximum elapsed 10 seconds;
  100ms initial backoff, 2s maximum backoff, deterministic injectable jitter
  ±20%, and `Retry-After` cap 5 seconds.
- Effective deadline is the minimum of parent, caller/tool and policy deadlines
  and includes the call, reconnect handshake, lease refresh, close and sleep.
- Existing SDK resume is allowed only when the transport provides that contract.
- EOF/unexpected EOF/reset/no-progress/session closed/missing: reconnect,
  re-resolve and replay only an approved read.
- 429 and 5xx: bounded read retry; 401 permits one refresh/reconnect path.
- 400/403/404/422, MCP `CallToolResult.IsError`, malformed schema/response and
  unknown status/tool: immediate typed failure.
- Caller cancellation/deadline always wins and is never retried.
- Reconnect waiters have independent cancellation. A departing waiter does not
  cancel shared work while another remains; the last departing waiter cancels
  it. Close wakes all waiters with typed `SessionClosed`.

## Rejected alternatives

- **Retry every MCP tool** — can duplicate mutations and violates AC-6.
- **Gateway branches in the retry core** — couples resilience to current gateway
  implementations and prevents future adapter conformance.
- **Lease ending after `ListTools`** — permits Kuadrant selection interleaving
  between schema discovery and the actual call.
- **Universal SDK resume** — modern MCP transport does not guarantee in-flight
  `Last-Event-ID` resumability.
- **Mutation replay** — ambiguous side effects cannot be made safe by a client
  retry; use explicit read reconciliation.
- **Readiness-probe frequency as recovery** — does not repair an in-flight call
  or stale session-scoped selection.
- **Logs only / free-form review strings** — loses durable status and can map
  transport failure incorrectly to `WorkflowNotFound`.
- **Immediate new CRD enum** — additive typed optional metadata and existing API
  error categories should preserve compatibility unless tests prove otherwise.

## Consequences and verification

The core gains explicit provider, binding, replay and evidence contracts. The
adapters own only gateway-specific selection mechanics. This requires local
tests for generation/CAS, waiter cancellation, lease interleaving, mutation
non-replay, target merges, typed terminal paths, canonical audit IDs and
bounded metrics. The formal test contract is
`docs/testing/2518/TEST_PLAN.md`; its 15 UT scenarios and 12 IT wiring rows
are required before implementation is complete. The external
`E2E-FLEET-2518-001` Agent Router/Envoy journey remains a separate release gate.

Control mapping: FedRAMP AC-4/AC-6/SI-10/AU-2/AU-3 (with SC-8/AU-9/AU-11
regression boundaries), SOC 2 CC7.2, and OWASP ASVS V2.2.1–V2.3.1 and V16.2.x.
