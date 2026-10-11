# TP-2493-FLEET-v1.0 — Organic Fleet Alert Fixture Readiness

**Date:** 2026-10-11

**Status:** Approved 2026-10-11; local TDD and real Fleet verification complete;
normal follow-up publication and exact-SHA CI pending

**Approval:** User selected "Approve fix (Recommended)": test-only secure scope
precondition, real organic/full Fleet verification, one normal follow-up push
and exact-SHA CI monitoring. Additional host capacity requires separate approval.
**Capacity approval:** User subsequently selected "Approve guarded window
(Recommended)" for temporary Helios08 `maxkeys=512` during this owned Fleet
validation only, restoring `200` even on failure/interruption. `maxbytes=20000`
and unrelated workloads/evidence remain unchanged. At that approval the window
was not yet active; the later authorized change to `512` is now in effect.
**Superseding instruction:** When asked about pausing the retained owned Fleet
node to free standard test ports, the user explicitly permitted deleting that
named node and said to **keep the maxkeys changes; restoration is not needed**.
Therefore leave `maxkeys=512` in place after this run, including on failure;
`maxbytes=20000` remains unchanged. Prefer pausing/restarting the named retained
node instead of deletion so its previous evidence is preserved. This permission
does not authorize deleting any other retained cluster or changing other limits.
The first guarded attempt subsequently found that the retained node's saved API
endpoint was stale/unreachable and its restart was blocked by an unrelated
root-owned manager on port `18081`. The manager is untouched. Under the explicit
delete permission, retain the named old node **stopped** instead of deleting it;
do not make its restart a prerequisite by interfering with that unrelated work.
The original container, volume, source snapshot and evidence remain preserved.

**PR:** [#2493](https://github.com/jordigilh/kubernaut/pull/2493)

**Starting published head:** `2dafdc9ce4f714604cf690bac0a9fd9aaf268f98`

**Failure:** [E2E Fleet, job 114486586154](https://github.com/jordigilh/kubernaut/actions/runs/38144393386/job/114486586154)

**Format:** IEEE 829 hybrid; AGENTS.md overrides older template coverage guidance

## 1. Objective and evidence

Preserve E2E-FLEET-021's real Prometheus → AlertManager → Gateway journey while
making its newly created, managed fixture visible to the scope backend **before**
the alert can fire. Do not turn this into another direct Gateway POST test.

The completed pipeline has **76 successful jobs**, one underlying Fleet E2E
failure, and two cascading summary/merge-gate failures; no jobs were skipped.
All other integration/E2E lanes and all image-scanning steps passed. Must-Gather,
Gitleaks, and CodeQL passed in their separate workflows on the same head.

Retained failed-job logs and sanitized must-gather evidence establish:

| UTC | Observation |
|---|---|
| 14:12:09.574 | Organic target Deployment creation begins |
| 14:12:09.593 | Synthetic metric injected; no Fleet scope precondition |
| 14:12:17.068 | Gateway resolves the target, then rejects it as `unmanaged_resource` |
| 14:12:17.072 | AlertManager records `Notify success` for the same delivery |
| 14:12:22.153 | Second organic delivery again rejected as `unmanaged_resource` |
| 14:12:22.155 | AlertManager again records `Notify success` |
| 14:13:47.610 | RR assertion expires after its unchanged 90-second bound |
| 14:14:07 onward | Later deliveries find no target after deferred test cleanup |

The suite is **46/47 passing**. The later owner-not-found messages are not proof
that the initial fixture was missing. FMC's actual startup log reports a
**30-second** sync interval, not the 10 seconds in the suite's historical comment.
AlertManager's existing repeat interval is one hour; group flushes do not prove
another webhook notification occurred within the assertion window.

**Evidence limitation:** the artifact does not record the exact FMC cache
transition or distinguish a cache miss from a fail-safe scope transport response.
The directly observed failure is an alert emitted before the test establishes
scope eligibility, followed by no new notification during its assertion. A real
pre-fire scope check will establish readiness or expose a separate backend error;
do not claim the missing historical cache transition has been recovered.

External evidence: `remaining-ci-fleet-organic-rca-v25.json`, failed job log,
sanitized selected logs, and archive manifest under `../kubernaut-pr-2493-rca/`.
Preserve the original archive; do not inspect or publish credential-bearing
`.pre-sanitize` members.

## 2. Requirements, scope and existing patterns

- **BR-INTEGRATION-054 / ADR-068:** real federated remediation journey.
- **BR-INTEGRATION-065:** Fleet scope-check backend routing.
- **BR-SCOPE-002:** Gateway rejects unmanaged resources; never bypass this gate.
- **AC-4 / AU-3:** preserve the organic flow and explicit `cluster=hub` attribution.
- **SC-8 / AC-6:** CA-verified HTTPS and existing least-privilege authorization.
- **SOC2 CC8.1:** approval, TDD, verification, and normal reviewed publication.

Discovery found existing scope HTTP contracts in `pkg/fleet/fmc/handler.go`,
authenticated scope probes in `test/e2e/fleetmetadatacache/shared/helpers.go`,
bounded owned-node Service probes in `test/infrastructure/fleet_e2e.go`, and
Gateway's existing FMC client authorization in
`charts/kubernaut/templates/rbac/fmc-scope-check-client-rbac.yaml`.

Proposed changes are confined to the organic Fleet test, an existing
infrastructure helper location and its Ginkgo contracts, and verification docs.
No new production business component, dependency, or architectural decision.
Production wiring is unchanged; the manifest below covers test-harness wiring.

Excluded: production scope/readiness/auth changes, new RBAC grants or NodePorts,
cache seeding, direct Gateway POSTs, shorter shared AlertManager repeat intervals,
increased existing timeouts, skipped tests, vulnerability exceptions, unrelated
#2496 work, forced pushes, manual reruns, and automatic host-limit changes.

## 3. Recommended correction and alternatives

**A — Recommended: real pre-fire scope readiness.** After target creation, query
FMC's existing HTTPS Service for the exact hub Deployment until it returns an
explicit managed result. Only then inject the metric. Reuse the existing
owned-node Service-probe pattern, current inter-service CA trust, and Gateway's
already-authorized ServiceAccount identity. Send bearer material through stdin,
not logged command arguments or persisted evidence. Encode query parameters,
bound execution/output, and expose HTTP/TLS/auth/parse failures distinctly from
a valid `managed=false` response. No Service or RBAC mutation is needed.

This adds a bounded fixture precondition, not a longer RR assertion. The original
90-second RR assertion and zero test-initiated Gateway POST contract stay intact.
If the existing Service route cannot be queried securely, stop and reassess rather
than bypass TLS, auth or NetworkPolicy.

**B — Shorten AlertManager repeats in test fixtures:** could resend after cache
sync, but changes shared notification behavior, masks fixture ordering and adds
noise to other parallel scenarios. Not recommended.

**C — Change production rejection/retry behavior:** broader security/delivery
semantics; unsupported by the retained evidence. Not recommended for this fix.

**Confidence:** 95% in the observed failure sequence and recommended ordering
correction. Exact historical cache timing remains unknown. The secure probe's
real Service-path execution must be validated before declaring GREEN.

## 4. Test plan and wiring manifest

| Scenario | Tier | Required outcome |
|---|---|---|
| UT-FLEET-2493-001 | Ginkgo UT | Exact hub/GVK/namespace/name query; bounded execution; CA verification; credentials absent from argv/diagnostics |
| UT-FLEET-2493-002 | Ginkgo UT | Valid unmanaged result never reports ready; managed result permits progress; HTTP/auth/TLS/invalid JSON/cancellation errors remain observable |
| UT-FLEET-2493-003 | Source-order contract | Organic entry point requires scope before injection; no direct Gateway POST call; this fast check alone is not wiring proof |
| IT-FLEET-2493-001 | Harness integration, proved by E2E-FLEET-021 | Real authenticated FMC Service dispatch succeeds before injection; no cache seeding or test-initiated Gateway POST |
| E2E-FLEET-021 | Existing real E2E | Actual secure FMC precondition then real alert firing and AlertManager webhook creates a hub-attributed RR for the fixture |
| Existing Fleet suite | Regression | All 47 specs pass with normal parallelism and no skips or timeout increases |

Only external network/container/Kubernetes boundaries may be mocked in fast
contracts. Actual FMC scope/cache logic and the real organic E2E journey are not
replaced by mocked business outcomes. Fast contracts alone are not E2E proof.

| Test-harness component | Entry point | Wiring location | Passing proof |
|---|---|---|---|
| Secure scope probe | Organic target setup | Existing Fleet infrastructure helper location, called from `test/e2e/fleet/20_gw_organic_alert_test.go` before `InjectMetrics` | IT-FLEET-2493-001 and real E2E-FLEET-021 |

No new audit events are introduced; existing RR/audit attribution stays unchanged.

## 5. TDD, verification, delivery and rollback

1. **RED (~20–30 min):** preserve CI failure provenance; add failing Ginkgo
   probe/error/privacy and entry-point ordering contracts before implementation.
2. **GREEN (~20–40 min plus E2E):** minimal secure probe and pre-fire call;
   run contracts, focused real organic journey and all 47 Fleet specs. Verify the
   test-harness wiring manifest. No GREEN claim without the real journey.
3. **REFACTOR (~10 min):** consolidate repeated command/response diagnostics
   into existing bounded-output patterns if needed; otherwise mark N/A with a
   reason. Validate build and affected tests after any concrete refactor.
4. **Verification:** `go build ./...`, all-suite compilation, affected
   infrastructure/race contracts, existing lint scope, anti-pattern/privacy
   review, and real Fleet regression. Record current-source/image identities.
5. **Delivery:** after approval and local evidence, one focused normal follow-up
   push; monitor the exact new SHA through CI. Preserve history and existing
   `Closes #2501` / `Closes #2502` references. Do not trigger manual reruns.

**Capacity:** the former Helios08 artifact-verification window was closed with
`maxkeys=200` and `maxbytes=20000` verified restored. The separate approvals above
authorize this run and supersede its restore-to-200 rule: keep `maxkeys=512`,
retain `maxbytes=20000`, and preserve unrelated workloads. Only the named retained
owned Fleet node may be paused/deleted; retain it stopped when restarting would
interfere with the unrelated port owner documented above. Fresh task-owned Fleet
nodes and source/evidence must be inventoried before execution.

**Suspension:** unavailable capacity, unverified trust, credential leakage,
inability to observe real scope readiness, or any newly exposed production
failure. Retain evidence and escalate; do not remove the assertion or suppress
errors. Rollback only newly authored changes, never unrelated work or history.

**Acceptance:** real organic E2E and 47/47 Fleet pass, intact security/identity
contracts, no new lint findings or skips, and passing exact-SHA CI. The current
head remains not merge-ready; approval is not an implementation or passing proof.

## 6. Execution checkpoint — local GREEN complete; exact-SHA CI pending

- **RED:** five expected new readiness-contract failures with eight existing
  contracts passing (`fleet-organic-contracts-red-v26.{json,log}`). A separate
  redaction-expansion test failed before the bounded-output improvement
  (`fleet-organic-redaction-red-v26.{json,log}`).
- **Fast GREEN:** all **307/307** infrastructure contracts pass under `-race`,
  including 15 new organic readiness/privacy/error/ordering cases. No pending or
  skipped specs in the full run (`fleet-organic-infrastructure-race-v27.json`).
- **REFACTOR content:** consolidate credential redaction and post-redaction byte
  capping in the existing bounded-output component; the original diagnostic
  executor and the new scope probe now share this behavior. Explicitly exclude
  proxy routing for the in-node Service check. No new production component.
- **Validation:** Go 1.26.9 `go build ./...`, all-suite compilation and CI-scoped
  lint pass. Full lint matches the exact **22** previous baseline findings with
  zero new findings (`fleet-organic-lint-baseline-comparison-v27.json`).
- **Qualification:** an earlier concurrently overloaded infrastructure run passed
  306/307; its 100-ms diagnostic fixture expired before emitting a payload. The
  focused and full race runs passed without changing that test or its timeout.
  Preserve the failed run; do not relabel it as a pass.
- **Identity:** all 5,893 snapshot entries verified; the 15 required amd64 artifact
  identities match the prior verified build and all production build inputs are
  byte-identical. The new Go 1.26.9 Linux/amd64 race-enabled Fleet test binary is
  verified separately. All three tested Go files remain byte-identical locally.
- **Runner qualification:** v27 stopped at the stale saved endpoint before any
  node mutation. v28 failed before creating fresh nodes because the prebuilt
  Ginkgo binary's directory had no `go.mod` ancestor; its original-node restart
  failed on the unrelated port owner. Both attempts and the stopped original are
  retained. v29 changes only the external runner's binary placement and resource
  disposition, never the tested Go code, security gates, or test timeouts.
- **Real focused proof:** E2E-FLEET-021 passes under the native race-enabled
  binary with four processes and CI seed `1791727149`. It records valid
  `managed=false` replies at **22:48:35.984**, **22:48:38.397**, and
  **22:48:40.699 UTC**, then `managed=true` at **22:48:42.979 UTC** before metric
  injection. The real firing-rule/AlertManager journey produces the asserted
  hub-attributed RR (`fleet-organic-focused-real-v29.json`, `organic-focused.log`).
  These are new observations, not recovery of the original incident's cache
  transition. The focused run's 46 exclusions are its regex filter, not new
  pending tests or `Skip()` calls.
- **CHECKPOINT W:** the helper is called through the organic entry point and
  authenticates to the real FMC Service using the node CA and existing Gateway
  identity. IT-FLEET-2493-001 is proved by the passing real E2E-FLEET-021; the fast
  source-order assertion alone is not used as wiring proof. Production business
  wiring is unchanged.
- **Collection:** production Must-Gather succeeds on both fresh focused clusters,
  with original collection statuses retained. Both archives and the focused
  source/report/command evidence are preserved. Only those successful newly
  created clusters were deleted before the fresh full run.
- **Runner qualification:** transition to the full run found the external
  adapter's one-argument Podman `untag` form removed all names from each image.
  All 15 content-addressed image identities remained available; original owned
  aliases were restored and verified. A real adapter smoke test now proves the
  explicit two-argument form removes only the task alias. No source, layers,
  archives or unrelated workloads were deleted. v30 runs only the full suite;
  the successful focused setup is not repeated.
- **Full real regression:** v30 completes successfully with **47/47 passed,
  0 failed, 0 pending and 0 skipped**, native Go 1.26.9 race instrumentation,
  four processes, CI seed `1791727149` and the unchanged 50-minute suite bound
  (`fleet-organic-full-real-v30.{json,log}`, `fleet-organic-full-proof-v30.json`).
  The organic case observes valid unmanaged responses from **23:10:31.316 UTC**,
  then `managed=true` at **23:10:40.434 UTC** before injection. AlertManager's own
  webhook produces the hub-attributed RR. No direct test Gateway POST, cache
  seeding, new authorization, TLS bypass or RR-timeout increase.
- **Full collection and disposition:** both production Must-Gather invocations
  return zero and retain sanitized hub/spoke archives. Final runner exit is zero
  with no cleanup errors. All four pre-run container inventory entries are
  unchanged, including the original stopped node; both fresh full-suite nodes
  are retained stopped. The unrelated root-owned port `18081` manager remains
  untouched. All 15 original artifact identities and aliases remain verified;
  no application image was rebuilt. `maxkeys=512`, `maxbytes=20000` are verified
  (`fleet-organic-final-verification-v30.json`,
  `fleet-organic-final-original-images-v30.json`).
- **Safety review:** no new Go anti-pattern, production business wiring, security
  exception, skipped/pending test or timeout change. All three tested Go files
  still match the real-run source manifest; build/lint/race evidence applies to
  the exact proposed code. PR closing references and merge ancestry are intact.
- **Outstanding:** the approved normal follow-up publication and exact-new-SHA
  CI. Local GREEN and CHECKPOINT W are complete; merge readiness is **not yet
  established** by local results alone.

**Confidence: 98%** in the scoped correction: failing-before contracts, all
307 infrastructure race specs, both real organic journeys, complete 47-spec
regression, explicit collection success and source/image identity verification.
Remaining risk: the new-head CI environment has not yet run this correction;
historical cache-transition timing remains unavailable.
