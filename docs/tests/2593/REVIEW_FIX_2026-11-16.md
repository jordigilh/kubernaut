# Bounded PR #2637 review repair — 2026-11-16

Status: repairs 2/3 verified under the user's standing review/fix authorization;
decisions 1/4 held. Baseline:
`13f629ccabbc15ed4680d89eff166b7dd2f03535`; remote main independently verified as
`741f7d8569fef9a5eed2d374c759b42298c0a6b8`. No main merge is authorized here.

## Authority and disposition

The current `TEST_PLAN.md`, DD-FLEET-011/012/013/014, BR-SCOPE-001 and
BR-INTEGRATION-065 govern this batch. No public API, HA, RBAC or inventory gate
change is proposed. AC-4/AC-6 and SI-10 are the affected safety objectives;
there are no new audit events.

1. Registry recovery: the permanent latch is confirmed in both registries and
   FMC omits the prober used by other commands. However `probeClusters` only
   LISTs; `Probe` then clears the watch error without proving watch recovery.
   Wiring it blindly would claim watch health from LIST alone. Implementation
   held for clarification of this health boundary; no new watch design invented.
2. Mutation replay: confirmed. Pinned go-redis v9.23.0 defaults to three retries,
   including EOF. TEST_PLAN lines 1359–1365 permit replay only after definitive
   NOSCRIPT and require ambiguous transport results to enter repair. Enforce
   this at the script command boundary, including injected clients.
3. Cluster-scoped targets: confirmed. TEST_PLAN boundary table prohibits
   verified-managed cluster-scoped remediation until FR-SCOPE-004 approval.
   Reject such *target checks* without deleting Namespace observations or
   preventing internal namespace-label inheritance reads.
4. Typed factory: confirmed unreachable for production FMC/ACM adapters, but
   existing boolean ingestion is not thereby broken. Public typed extensions
   remain approval-gated (TEST_PLAN boundary table/section 15). Neither a bool
   nor generic HTTP failure supplies observed label/backend provenance. Hold
   implementation for the exact typed evidence contract; preserve DD-AF-018's
   closed ACM inventory gate.

## TDD and success criteria

RED (~25 minutes): Ginkgo/Gomega scenarios below must fail on baseline.
GREEN (~15 minutes): no-replay EVALSHA command + ambiguous error classification;
target eligibility guard before both cache-read paths. REFACTOR (~10 minutes):
share first/after-NOSCRIPT script execution and error classification, not new
business components. Build/lint/regression checks are validation, not refactor.

| Scenario | Contract | Critical |
|---|---|---|
| UT-FMC-REPLAY-001 | Executed begin reply lost: one invocation, ErrAmbiguousWrite, no success | yes |
| UT-FMC-REPLAY-002 | Injected retry-enabled client cannot replay the mutation | yes |
| UT-FMC-REPLAY-003/004 | Lost reply after NOSCRIPT is not replayed; definitive NOSCRIPT alone still recovers once | yes |
| UT-FMC-DEFERRED-001 | Node/PV/Namespace/custom empty-namespace target never yields managed | yes |
| IT-FMC-REPLAY-001 | Real Syncer/store/readiness/Check fails closed after lost mutation reply; fresh repair recovers | yes |
| IT-FMC-DEFERRED-001 | Complete observed Node repair remains unavailable over real handler/client; Namespace still supports Pod inheritance | yes |

These are additive scoped scenarios, not a rewrite of the original plan's
denominators. All defined critical scenarios and both wiring points must pass.
Tests control external Redis transport and MCP only; business logic remains real.

The UT denominator is **9 critical cases** (four replay/recovery entries and five
deferred-target entries); the IT denominator is **3 critical cases** (lost begin,
lost commit and the Node/Namespace HTTP journey).

## Wiring manifest

| Component | Production entry | Wiring location | Proving IT |
|---|---|---|---|
| EVALSHA no-replay/error classification | FMC Syncer | atomic_script.go ScriptManager.eval, status_store.go BeginRepair | IT-FMC-REPLAY-001 |
| Deferred target guard | FMC HTTP API | scopecache.Client.Check -> fmc.Handler -> HTTPClient | IT-FMC-DEFERRED-001 |

## Validation, risks and rollback

Run affected UTs and real local Valkey production-composition tests, full build,
full lint and repository test compile; run affected regressions. No gate weakening
or pending specs. Existing external suite prerequisites and final exact-artifact
CI remain separately reported. Lost-reply assertions must observe the server's
completed reply before dropping it, and allow reconnect to expose hidden replay.
Rollback, if needed, is an ordinary reviewed revert of this bounded commit,
never a reset/discard. Do not claim resolution of held findings 1/4 or historical
CI/producer RCA. Preflight confidence for repairs 2/3: 96%; held decisions do
not receive implementation/coverage credit.

## Execution evidence

RED: `2593-review-red-ut.log` records **8 failures / 9 selected cases** on
unchanged production code: five target checks returned verified-managed; standalone
and injected Redis clients executed begin twice; after NOSCRIPT they executed it
three times instead of two. The existing permitted NOSCRIPT recovery case passed.
The native envtest 1.36.2 / Valkey 9.1.1 harness ran the unchanged new IT source:
`2593-native-review-red.log` records both mutation replays and the authorized Node.

Intermediate wiring RED (`2593-native-review-intermediate.log`) then proved that
disabling retries alone left prior readiness clean after a lost begin reply.
The fix invalidates the previous proof inside `Syncer.beginRepair`'s existing
registration guard, before another guarded clean publication can run. Lost commit
already uses the existing uncertain/repair lifecycle.

GREEN: all **9/9 critical UT cases**, **3/3 critical IT cases** and **2/2 wiring
rows** are covered. The native full selected source run passed (eight cases,
including existing broker/lifecycle regressions). The standard suite then passed
**11/11 selected cases with -race**, using the repository-documented per-command
`CONTAINER_CONNECTION=podman-machine-default-root` and envtest 1.36.2. The 24
other cases were excluded by the scoped focus expression, not claimed as passing.
No pending/skip was added. Standard-suite output:
`2593-review-standard-green-it.log`.
After the proxy context cleanup, the final standard `-race` rerun passed all
three new wiring cases (`2593-review-final-it.log`); the full build and repository
compile check also passed again (`2593-review-final-compile.log`).

Refactor content: both EVALSHA attempts share one no-replay execution/error
boundary; NOSCRIPT recovery is flattened with guard returns and checks the exact
typed Redis error code. The test transport carries caller context through listen
and dial rather than storing it. `go-redis`'s pinned `command.go:645–650` and
`redis.go:1619–1624` establish the per-command NoRetry behavior. A constructor-only
`MaxRetries:-1` setting would not protect injected retry-enabled clients; using
zero would incorrectly select the driver's default three retries
(`options.go:764–769`).

Passing affected checks: `go test -race ./pkg/fleet/...
./cmd/fleetmetadatacache/...`, `go build ./...`, repository compile-only
`go test ./... -run=^$ -timeout=30s`, full `golangci-lint run --timeout=5m`
(**0 issues**) and `git diff --check`. Logs are under the session's approved
temporary parent directory (`2593-review-{race,build,compile,lint-final}.log`).
An initial standard IT attempt failed in suite setup due to rootless Podman
`lchown /etc/shadow`; no business spec ran in that attempt. No persistent Podman
configuration or gate was changed to obtain the subsequent passing standard run.
The suite's separate pinned kube-mcp parser container could not start because
its existing invocation passes an unsupported `--port` flag. The selected tests
use the controlled external MCP endpoint, not that producer. Neither the CLI
failure nor these passing journeys prove the producer's structured-content schema.

This batch does not qualify a new deployed artifact, close findings 1/4, prove
the pinned producer's empty structured-content schema, or establish historical
CI causality. No DD was added: repairs enforce approved contracts, while the
unselected alternatives below remain approval-gated.

## Held decisions — evidence and alternatives

**Finding 1:** `cmd/fleetmetadatacache/main.go:116–118` consumes registry health,
but has no `ClusterRegistryProber` production caller. The exact Engram reference
lookup found callers in AF, EM and SP, not FMC. Both registries latch false
(`eaigw_registry.go:193`, `kuadrant_registry.go:147`). Their Probe implementations
call `probeClusters`, whose only API operation is LIST (`registry/probe.go:57`),
then set ready true and clear the watch error. Thus a successful LIST with a
still-broken WATCH can mask the requested watch-health condition. Existing
`UT-REG-EAIGW-010` explicitly allows LIST to restore readiness; the approved
DD-FLEET-013 generation fence only rejects an *intervening* health change.

Decision needed: (A) explicitly accept periodic authoritative-LIST health as the
FMC recovery boundary and wire the existing prober, or (B) require confirmed
informer/watch reestablishment before clearing watch health and approve that
lifecycle design. B preserves the literal watch-health claim; A is smaller but
must not be described as proof of a healthy watch. No code for either is included.

**Finding 4:** `scope_factory.go:128–159` constructs HTTPClient/ACM Client;
neither defines Check. `FederatedScopeChecker.Check:155–159` therefore cannot
obtain a remote typed checker from this factory. Existing production consumers
continue to call IsManagedResource (including `policyread/service.go:340`), so
this does not prove all remote ingestion is broken.

A strict status/bool wrapper cannot recover all typed evidence: FMC's handler
returns HTTP 200 `managed:false` for an unknown registration (`handler.go:108–112`)
and HTTPClient's bool decoder also accepts an omitted managed field as false
(`http_client.go:167–172`). Wrapping that result as verified-unmanaged would
invent authority. ACM has richer internal Search observations, but changing its
typed mapping/AF consumer does not authorize FMC wire semantics or Console ACM
inventory. TEST_PLAN's boundary table and section 15 retain the approval gate.

Decision needed: (A) approve a strictly validated existing-v1 status/bool adapter
with exact unavailable/unknown-cluster and legacy-provenance semantics (including
any required handler semantic correction), or (B) approve the precise typed-v1
schema and backend mapping before implementation. A avoids new response fields
but still needs an agreed evidence boundary; B conveys richer provenance but is
an explicit public contract change. Direct ACM typed scope must remain separate
from the still-closed DD-AF-018 Console inventory gate in either option.
