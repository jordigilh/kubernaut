# DD-PLATFORM-008: `startupProbe` for Fleet-Aware Services with Slow Cold Starts

**Date**: July 28, 2026
**Status**: ✅ **APPROVED**
**Confidence**: 92%
**Last Reviewed**: July 29, 2026
**Related**: Issue #1737 (E2E FP/Fleet Helm chart migration), Issue #1755 (DD-TEST-015 Fleet
E2E hardening), `pkg/fleet/registry` (`ClusterRegistry.Start`), `pkg/fleet/mcpclient`
(`NewResilient`), `cmd/workflowexecution/main.go` (`buildClientFactory`)

---

## 🎯 **DECISION**

**Every Kubernaut Helm chart component whose process can legitimately block
at boot for longer than its steady-state liveness/readiness budget allows
(a "slow, one-time startup dependency") SHALL get a `startupProbe` via the
new `kubernaut.startupProbe` helper (`_helpers.tpl`), rather than having its
steady-state liveness/readiness thresholds loosened to compensate. This is
the default remediation pattern going forward -- new components with the
same shape should adopt it, not reinvent probe tuning per-service.**

The helper defaults to `initialDelaySeconds: 5, periodSeconds: 5,
timeoutSeconds: 5, failureThreshold: 60` (305s total grace, updated from an
initial `30`/150s -- see the July 29 update below) against `/healthz` on the
`health` named port, fully overridable via its input `dict`. It only
**defers** liveness/readiness enforcement until the first successful check;
steady-state probe behavior for a healthy, already-started pod is
completely unchanged.

Applied to `apifrontend`, `effectivenessmonitor`, `workflowexecution`,
`gateway`, `remediationorchestrator`, `fleetmetadatacache`, and
`signalprocessing` -- the seven chart-managed services that build a fleet
MCP Gateway connection (`registry.ClusterRegistry` / `mcpclient.NewResilient`,
or an OAuth2 token fetch gating that connection) synchronously at process
startup.

**Update (July 29, 2026)**: two further gaps surfaced during live
re-validation of the DD-TEST-015 fleet E2E refactor (single `helm install`
with `global.fleet.enabled=true`, all seven services cold-starting
concurrently under a 12-way-parallel Ginkgo run):

1. **`signalprocessing` was missing entirely** from the original six-service
   rollout despite being fleet-aware (it builds the same
   `registry.ClusterRegistry` via its own MCP Gateway connection at boot,
   confirmed in `signalprocessing.yaml`'s `fleet.oauth2` config block) --
   an oversight in the original audit, not a "correctly excluded"
   non-fleet-aware service as this doc previously (incorrectly) listed it.
   It crash-looped (exit 137, `Liveness probe failed ... context deadline
   exceeded`) with the exact same signature AF/EM/WE had before their fix.
2. **`failureThreshold: 30` (150s) was insufficient** even for the six
   services that already had it: `effectivenessmonitor`,
   `remediationorchestrator`, and `workflowexecution` were all still being
   killed by their own `startupProbe` (not just the liveness probe it was
   meant to defer) under the same 12-way-parallel run. Direct evidence via
   cgroup v2 `cpu.stat` inspection (`kubectl exec <pod> -- cat
   /sys/fs/cgroup/cpu.stat`) on a freshly-restarted `effectivenessmonitor`
   pod showed `nr_throttled: 23` of `nr_periods: 24` (96%) -- near-constant
   CFS bandwidth-quota throttling against the default `500m` CPU limit
   during the cache-sync + MCP-client-connect cold-start burst. This is the
   well-documented Linux CFS bandwidth controller behavior where a CPU
   *limit* throttles bursty startup work in discrete quota periods even
   when the container's average utilization looks moderate
   (kubernetes/kubernetes#67577). `failureThreshold` raised from `30` to
   `60` (150s -> 305s), matching the 5-minute ceiling already adopted
   elsewhere in this same investigation for `mcpclient.NewResilient`'s own
   `MaxElapsedTime` backoff budget (`pkg/fleet/mcpclient/resilience.go`) --
   the probe should not give up before the client it's waiting on does.

**Update (July 28, 2026)**: extended from the original three (AF/EM/WE) to
`gateway` and `remediationorchestrator` after the first Fleet E2E run against
the fully chart-native deploy path (DD-TEST-015, `global.fleet.enabled=true`
on the *first* `helm install`, superseding the old kubectl-patch-based deploy
that never actually exercised this cold-boot race for these two services)
hit the identical liveness-probe-induced crash loop: Gateway's
`fleet.NewScopeChecker`/`ClusterRegistry.Start` and RO's equivalent both
block at boot exactly like AF/EM's, and were killed mid-startup
("connection refused" / "context deadline exceeded") the same way. Also
added to `fleetmetadatacache`, which is *always* fleet-enabled by
definition (its whole Deployment is gated on
`fleetmetadatacache.enabled=true`) and showed the same shape (OAuth2 token
source + MCP Gateway connection built synchronously in `main.go` before the
health server starts serving). This is exactly the scenario this DD's
"default remediation pattern going forward" language anticipated -- see
[Consequences](#-consequences) for the updated file list and the retired
gateway negative test.

---

## 📊 **Context & Problem**

While migrating the Fleet E2E suite (Issue #1737) from programmatic Go
deployment to `helm install`, all three of `apifrontend`,
`effectivenessmonitor`, and `workflowexecution` were observed crash-looping
for 10+ minutes immediately after being patched with fleet OAuth2 wiring and
restarted -- non-self-resolving via retries within the test run, but
succeeding instantly when the exact same patch+restart was replayed manually
minutes later once the cluster was idle.

Live debugging (pod exec, `kubectl describe`, log inspection) on a
resource-constrained Kind-on-podman cluster (6 vCPU, 11GB, per
`vfkit --cpus 6 --memory 11444`) found two related but distinct failure
modes, both fixed by this DD (a third, unrelated ordering bug -- AF/EM/WE
being wired to the fleet MCP Gateway before its Kuadrant AuthPolicy had
converged -- was fixed separately in `test/infrastructure/fleet_e2e.go` and
is out of scope here):

1. **apifrontend / effectivenessmonitor**: both build a
   `registry.ClusterRegistry` against the fleet MCP Gateway at boot
   (`buildFleetReaderDeps` -> `mcpclient.NewResilient` +
   `ClusterRegistry.Start`'s blocking `cache.WaitForCacheSync`). Under
   contention (multiple services restarting back-to-back at the tail of a
   full-chart E2E setup), the initial informer sync legitimately took longer
   than AF's liveness probe's ~60s kill budget (`initialDelaySeconds: 30,
   periodSeconds: 15, failureThreshold: 3` default) / EM's ~30s budget,
   causing kubelet to kill the pod mid-startup, before it ever had a chance
   to report ready.

2. **workflowexecution**: its own logs show the fleet MCP Gateway connect
   attempt (including an OAuth2 token fetch to Keycloak) took 83 seconds
   before failing -- but critically, WE's design is already fail-open for
   this specific case (`"readiness will report NotReady and keep retrying in
   the background"`, confirmed in `cmd/workflowexecution/main.go`). Despite
   that, its liveness probe still killed the pod: `/healthz` (bound to
   `controller-runtime`'s dependency-free `healthz.Ping`, confirmed via code
   read -- not an app-level bug) missed its 5s probe `timeoutSeconds`
   repeatedly, across multiple restarts, non-self-resolving. A trivial,
   synchronous, dependency-free handler missing a 5s deadline under load
   means the whole process was briefly unable to service *any* HTTP request
   -- CPU/scheduler starvation, not a code defect in the handler itself.

Both failure modes share the same shape and the same fix: the pod's
*legitimate* cold-start time, under real-world (not just this E2E
environment's) node contention, can exceed what a probe tuned for
steady-state response times tolerates. This is not exclusive to a
constrained local Kind VM -- production clusters see noisy-neighbor CPU
pressure too, especially during a multi-pod rolling restart (e.g. a chart
upgrade touching several fleet-aware services at once).

---

## 🔍 **Alternatives Considered**

### **Option A: Add a `startupProbe` via a shared `_helpers.tpl` helper** ✅ **CHOSEN**

A `startupProbe` is exactly Kubernetes' purpose-built mechanism for "slow to
start, fast at steady state": while it's failing, the kubelet does not
evaluate liveness/readiness at all, so a legitimately slow cold start is
never mistaken for a hang. Once it succeeds once, it stops being evaluated
and liveness/readiness resume unchanged. Implementing it as a chart-wide
`kubernaut.startupProbe` helper (parameterized by `path`/`port`/threshold
overrides) means the *fix* costs one line per Deployment template, and it
becomes the discoverable, documented default for any future component with
the same shape.

- ✅ Kubernetes-idiomatic: does exactly what a `startupProbe` exists for, no
  workaround semantics.
- ✅ Zero steady-state behavior change: a healthy pod that starts within its
  existing liveness `initialDelaySeconds` sees no difference at all.
- ✅ Fixes both observed failure modes (blocking `ClusterRegistry.Start` for
  AF/EM, and CPU-starvation-induced probe misses for WE) with one mechanism,
  since both are fundamentally "give the pod more time before killing it."
- ✅ Reusable: `kubernaut.startupProbe` is a one-line include, matching the
  existing pattern for `kubernaut.affinity` / `kubernaut.containerSecurityContext`
  / `kubernaut.mergedSecurityContext`.
- ➖ New probe type for the chart (no prior `startupProbe` usage) -- accepted
  because it's a standard, well-understood Kubernetes primitive, not a
  bespoke pattern.

### **Option B: Loosen liveness/readiness `initialDelaySeconds`/`failureThreshold` directly** ❌ REJECTED

Just increase the existing liveness probe's grace period for AF/EM/WE
(e.g. `failureThreshold: 30` directly on `livenessProbe`).

- ❌ Also slows down detection of a **genuinely** hung process during normal
  (fast) steady-state operation for the *entire* probe lifetime, not just
  cold start -- a real deadlock introduced later would take just as long to
  detect as a slow boot does today.
- ❌ Conflates two different concerns (slow boot vs. hung steady-state
  process) into one threshold, whereas `startupProbe` cleanly separates
  them.
- ➖ Simpler diff (no new probe type), but rejected because it's strictly
  worse observability for a problem `startupProbe` solves without that
  trade-off.

### **Option C: Test-harness-only retry/backoff around the patch+rollout-wait** ❌ REJECTED (for this DD's scope)

Keep the chart's probes as-is; add a retry loop in
`test/infrastructure/fleet_e2e.go` around
`patchDeploymentAddFleetOAuth2Volume`'s rollout-status wait.

- ❌ Doesn't fix the same risk in a real cluster -- a production chart
  upgrade that happens to restart several fleet-aware services under node
  contention would hit the identical crash loop, with no test harness to
  retry it.
- ➖ Smaller/more isolated diff (no chart change), but rejected because the
  underlying risk is a genuine production robustness gap, not an E2E-only
  artifact -- confirmed by WE's dependency-free `healthz.Ping` handler still
  missing its deadline under load, which is a node-resource-pressure signal
  independent of how the pod got restarted.

---

## ✅ **Consequences**

- `charts/kubernaut/templates/_helpers.tpl`: `kubernaut.startupProbe` named
  template. Input `dict` accepts `path`, `port`, `initialDelaySeconds`,
  `periodSeconds`, `timeoutSeconds`, `failureThreshold`, all with sensible
  defaults (`/healthz`, `health`, `5`, `5`, `5`, `60` -- `failureThreshold`
  raised from an initial `30` per the July 29 update above).
- `charts/kubernaut/templates/apifrontend/apifrontend.yaml`,
  `templates/effectivenessmonitor/effectivenessmonitor.yaml`,
  `templates/workflowexecution/workflowexecution.yaml`,
  `templates/gateway/gateway.yaml`,
  `templates/remediationorchestrator/remediationorchestrator.yaml`,
  `templates/fleetmetadatacache/fleetmetadatacache.yaml`,
  `templates/signalprocessing/signalprocessing.yaml`: each gets
  `{{- include "kubernaut.startupProbe" (dict "port" "health") | nindent 10 }}`
  immediately before its existing `readinessProbe`/`livenessProbe` block, with a
  comment documenting the specific blocking dependency and the confirmed
  incident. No other fields on the existing liveness/readiness probes changed.
- `charts/kubernaut/tests/startup_probe_test.yaml`: asserts the
  `startupProbe` renders with the expected path/port/thresholds on all
  seven services, and that the existing liveness/readiness
  `initialDelaySeconds` values are untouched. The original negative check
  ("`gateway` does *not* get one -- targeted rollout, not chart-wide") was
  retired and replaced with a positive assertion once Gateway was confirmed
  to share the same blocking dependency shape (see Update notes above);
  this remains a *targeted* rollout to services with the confirmed
  dependency shape, not an unconditional chart-wide default --
  non-fleet-aware services (Console, DataStorage, KubernautAgent,
  AIAnalysis, Notification, AuthWebhook) still correctly have no
  `startupProbe`.
- Validated: `helm lint --strict` clean, `helm template` output inspected
  for correct probe rendering/indentation on all seven services, full
  `helm-unittest` suite green (250/250 as of the July 29 update).
- **Follow-up (not implemented by this DD)**: `kubernaut-operator` is the
  production deployment path (the Helm chart is development/E2E-test-only,
  per existing project convention) and defines its own Deployment specs
  independently (`kubernaut-operator/internal/resources/deployments.go`,
  same split as DD-PLATFORM-004's anti-affinity/PDB parity gap). If
  `kubernaut-operator` deploys these same three fleet-aware services with
  the same startup dependency shape, it should get the equivalent
  `startupProbe` treatment there too -- tracked as a parity gap, not
  actioned in this DD.

## 🔗 Related Decisions

- DD-PLATFORM-004 (Anti-Affinity and PDB Enabled by Default): same
  "Operator-parity gap, chart-wide default via a `_helpers.tpl` helper"
  shape; this DD follows its precedent for how to introduce a new
  chart-wide default safely (helper + targeted call sites + helm-unittest
  coverage + DD doc).
- Issue #1737: umbrella Fleet/FullPipeline E2E Helm chart migration this
  fix was discovered during.
