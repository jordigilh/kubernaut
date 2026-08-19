# DD-TESTING-003: E2E Must-Gather Execution Mechanism

**Status**: ✅ **APPROVED** (2026-08-18)
**Last Reviewed**: 2026-08-18
**Confidence**: 90%

---

## Context & Problem

### The Challenge

E2E test suites (`test/e2e/*`) currently collect cluster diagnostics via `MustGatherPodLogs()`
in `test/infrastructure/datastorage.go` — an ad-hoc Ginkgo helper that shells out to `kubectl logs`
per-pod. This duplicates (poorly) what the production `must-gather` image
(`cmd/must-gather/`) already does comprehensively (CRDs, events, cluster state, Tekton, DataStorage
API, DB infra, metrics, sanitization, checksums, tarball).

**Real incident** (PR #2185, referenced in [#2036](https://github.com/jordigilh/kubernaut/issues/2036)):
E2E tests hit the suite timeout. `MustGatherPodLogs()` never ran because it is invoked from Ginkgo's
own `AfterSuite`/`AfterEach` hooks, which do not reliably execute after a timeout-triggered process
kill. Diagnostics were lost for the exact failure mode where they matter most.

Additionally, `main`'s `DeleteCluster()` deletes the Kind cluster whenever `testsFailed == false`,
which is also true on timeout/cancellation (tracked separately in
[#2192](https://github.com/jordigilh/kubernaut/issues/2192), already fixed in `release/v1.5` via #2191).
Even with that fixed, we still need a diagnostic mechanism that (a) matches production must-gather
output, and (b) can be invoked independently of Ginkgo's own hook lifecycle so it survives timeouts.

Migrating E2E to run the actual `cmd/must-gather` image (instead of the Ginkgo-native collector)
requires picking **how** that image gets executed against a Kind cluster from CI. This DD evaluates
two candidate mechanisms via time-boxed spikes and picks one.

### Business Requirements

- **BR-TESTING-001**: Integration/E2E tests MUST provide actionable diagnostics on failure
- **BR-PLATFORM-001.3.2**: Must-gather RBAC/execution (existing, `cmd/must-gather/templates/`)
- Supports the migration tracked in [#2036](https://github.com/jordigilh/kubernaut/issues/2036)

---

## Alternatives Considered

Both alternatives were prototyped against a real, disposable Kind cluster (`mg-spike`, kindest/node
v1.36.1, podman provider) with a representative workload (2 nginx pods + 1 Job in `kubernaut-system`)
and the actual `cmd/must-gather` image built unmodified from `main` HEAD.

### Alternative 1: Local Container on the Kind Podman Network ✅ PROPOSED

**Approach**: Run the must-gather image as a plain `podman run` container attached to the `kind`
podman network (same pattern as `cmd/must-gather/Makefile`'s existing `test-e2e-container` target),
with the cluster's kubeconfig and an output directory bind-mounted in.

```bash
podman run --rm \
  --network kind \
  -e KUBECONFIG=/kubeconfig/kubeconfig-internal.yaml \
  -v "$KUBECONFIG_INTERNAL":/kubeconfig/kubeconfig-internal.yaml:ro,Z \
  -v "$OUTPUT_DIR":/must-gather:Z \
  localhost/must-gather:<tag> --dest-dir=/must-gather
```

**Spike Evidence**:
- Used `kind get kubeconfig --internal` (server rewritten to the in-network Pod DNS name,
  e.g. `https://mg-spike-control-plane:6443`) — no RBAC objects needed beyond what the E2E
  harness's own admin kubeconfig already grants.
- Single command, no cluster-side setup or teardown.
- Full collection (CRDs, logs, events, cluster state, Tekton, DataStorage API, DB infra,
  metrics, sanitization, checksums, tarball) completed in **~18s** on this dev machine
  (includes QEMU cross-arch emulation overhead — amd64 image on arm64 host; CI runners are
  native amd64, so production timing will be lower).
- Output tarball + expanded directory landed **directly on the host filesystem** via bind
  mount — zero extraction step, available immediately for `actions/upload-artifact`.
- Verified pod logs (including the `Job`-owned pod) were correctly collected.

**Pros**:
- ✅ **1 step**: no image-load-into-cluster, no RBAC apply/cleanup, no extraction step
- ✅ Reuses the E2E harness's existing kubeconfig — no new RBAC surface to build or maintain
- ✅ Output available on host in real time (streams to bind mount as it's written)
- ✅ Can be invoked **independently of the Ginkgo process** (e.g., from a CI step, a signal
  handler, or a supervising script) — directly solves the "lost on timeout" problem, since it
  doesn't depend on an `AfterSuite`/`AfterEach` hook firing
- ✅ Matches an already-validated local-dev pattern (`cmd/must-gather/Makefile:test-e2e-container`)
- ✅ No cluster-scoped RBAC objects to collide across concurrent suites

**Cons**:
- ⚠️ Requires the invoking process to have podman access to the `kind` network by name
  (already true for anything using `kind_cluster_helpers.go`, which creates clusters via the
  same podman provider)
- ⚠️ Uses the cluster-admin-equivalent kubeconfig rather than a scoped ClusterRole (acceptable
  for E2E — the existing `MustGatherPodLogs()` already used the same admin kubeconfig via
  `kubectl`)

**Confidence**: 90%

---

### Alternative 2: In-Cluster Pod + RBAC + Tarball Extraction

**Approach**: Load the image into the Kind cluster's node, apply the existing
`cmd/must-gather/templates/{serviceaccount,clusterrole,clusterrolebinding}.yaml`, run the
image as a Pod using that ServiceAccount, wait for collection to finish, `kubectl cp` the
tarball out, then delete the Pod and RBAC objects.

**Spike Evidence**:
- `kind load docker-image` took **~18s** per load (one-time per cluster, but still a
  dependency that must happen before the first gather call).
- RBAC apply (3 manifests) was fast (<1s) and worked unmodified — templates already default
  to the `default` namespace matching E2E convention.
- The Pod needed a `sleep 120` appended to its command to stay alive long enough for
  `kubectl cp` to run before exiting — a real timing hack; a production version would need a
  poll-for-completion loop instead of a fixed sleep.
- Collection itself also completed in ~18s (identical collector code path).
- `kubectl cp` was stress-tested with a 150MB file (near the tool's practical range, well under
  the 500MB `--max-size` cap) — completed in 0.75s with a matching MD5 checksum. **No
  reliability concern found** at these sizes, contrary to some documented `kubectl cp` issues
  with very large transfers.
- Full sequence: image load → RBAC apply (×3) → Pod create → wait-ready → poll/sleep →
  `kubectl cp` → Pod delete → RBAC delete (×3) = **8 discrete steps**, each a potential
  failure point.

**Pros**:
- ✅ Uses the existing, already-written RBAC templates as designed for in-cluster execution
- ✅ `kubectl cp` proved reliable even at 150MB in this spike

**Cons**:
- ❌ 8 moving parts vs. 1 — more failure surface, more to get wrong under CI timeout pressure
- ❌ Image must be loaded into every ephemeral Kind cluster before first use (~18s tax)
- ❌ Requires a wait-for-completion mechanism (not a fixed sleep) to safely extract before Pod exit
- ❌ Still depends on the invoking process being alive to run the extraction/cleanup steps —
  does **not** meaningfully improve the "lost on timeout" failure mode over the status quo,
  since a killed Ginkgo process still can't run `kubectl cp` + cleanup
- ❌ Cluster-scoped `ClusterRole`/`ClusterRoleBinding` use a fixed name
  (`kubernaut-must-gather`) — fine for one-suite-per-cluster (today's convention) but a latent
  collision risk if that convention ever changes

**Confidence**: 55% (rejected — solves less of the actual problem, at higher operational cost)

---

## Decision

**APPROVED: Alternative 1 — Local Container on the Kind Podman Network**

### Rationale

1. **Directly fixes the motivating incident**: because it's a single external `podman run`
   invocation against the still-live Kind cluster, it can be triggered by a CI step or wrapper
   script that runs regardless of whether the Ginkgo process itself times out or is killed —
   the actual failure mode from PR #2185/#2036 does not reproduce with this mechanism.
2. **Fewer moving parts**: 1 step vs. 8. Fewer failure points under exactly the CI-timeout
   pressure that motivated this migration in the first place.
3. **Zero new RBAC surface**: reuses the kubeconfig the E2E harness already has; nothing new
   to apply, maintain, or clean up per test run.
4. **Already validated shape**: mirrors `cmd/must-gather/Makefile`'s `test-e2e-container`
   target, which the team already uses for local dev must-gather runs against Kind.
5. **No extraction step**: bind-mounted output is immediately available for
   `actions/upload-artifact`, with no tarball-copy race to get right.

**Key Insight**:
> "The mechanism must survive the death of the process that triggered it — that's the whole
> point of migrating off Ginkgo hooks. A container invocation keyed to the Kind network,
> not to the Ginkgo process's lifecycle, is what actually satisfies that requirement."

---

## Implementation

1. ✅ Added `cmd/must-gather/collectors/jobs.sh` (confirmed gap: `pkg/workflowexecution/executor/job.go`
   still creates `batchv1.Job`s in production; no dedicated collector existed before this).
2. ✅ Built `RunMustGatherImage(...)` + `BuildMustGatherImageForE2E(...)` helpers in
   `test/infrastructure/must_gather_image.go`, wrapping the `podman run` invocation validated in
   this spike (kubeconfig-internal generation via `stripKindProviderBanner`, bind mounts, cleanup).
   Extended with a repeatable `--extra-namespace` flag (`ExtraNamespaces []string` on
   `RunMustGatherImageOptions`) for suites needing diagnostics from infra namespaces outside
   `kubernaut-system` (e.g. `mcp-system`, `istio-system`, `envoy-ai-gateway-system`) — tracked
   as a production must-gather feature in [#2194](https://github.com/jordigilh/kubernaut/issues/2194).
3. ✅ Piloted on the `gateway` E2E suite; verified a real bundle collected via a focused
   integration smoke test against a live Kind cluster.
4. ✅ Rolled out to all remaining E2E suites: single-cluster (`apifrontend`, `authwebhook`,
   `effectivenessmonitor`, `notification`, `workflowexecution`, `fullpipeline`,
   `remediationorchestrator`, `kubernautagent`, `datastorage`, `signalprocessing`, `aianalysis`)
   and multi-cluster (`fleet`, `fleetmetadatacache`, `fleetmetadatacache/eaigw` — primary + remote
   cluster calls, using `ExtraNamespaces` for the mesh/gateway components each suite deploys).
   `signalprocessing`/`aianalysis` were missed in the initial rollout pass and briefly regressed to
   zero diagnostic collection once step 5 removed `DeleteCluster()`'s implicit
   `MustGatherPodLogs()` call — caught and fixed by the full-CI validation in step 7 below.
5. ✅ Removed `MustGatherPodLogs()` and its dedicated unit test (confirmed dead: every caller
   of `DeleteCluster()` now runs `RunMustGatherImage` in its own `AfterSuite` before calling
   `DeleteCluster`, which made the old inline kubectl-log-scraping call inside `DeleteCluster`'s
   CI/CD branch fully redundant). Also dropped the now-unused `namespace ...string` variadic
   parameter from `DeleteCluster()`. Updated `MUST_GATHER_ARTIFACT_COLLECTION.md`. Closes #2036.
6. ✅ Wired the must-gather image into `ci-pipeline.yml`'s existing artifact-based image handoff
   instead of each E2E job building it from source on its own failure teardown: added `must-gather`
   as a third `build-infra-images` matrix entry (alongside `db-migrate`/`mock-llm`), built once per
   workflow run and uploaded as the `image-must-gather-amd64` artifact. Every E2E job's existing
   `load-ci-images` step already globs `image-*-amd64` and `podman load`s whatever is there, so no
   change was needed there. `BuildMustGatherImageForE2E` now calls `resolvePrebuiltCIArtifact(ctx,
   "must-gather", writer)` first — the same `KUBERNAUT_CI_ARTIFACT_TAG` fast path `db-migrate`,
   `mock-llm`, `datastorage`, `kubernautagent`, and `BuildImageForKind` already use — falling back to
   a local `podman build` only for non-CI/local-dev runs where that env var isn't set.
7. ✅ Full-CI validation across all 15 E2E suites (2026-08-19), after being asked whether the
   collection path had actually been exercised anywhere but the `gateway` pilot. Rather than adding
   a permanent "always run must-gather" mode (rejected earlier in this same rollout — CI should only
   pay the must-gather cost on the failure path it exists for), the validation was done as two
   throwaway commits on this PR's branch: (1) an `E2E_FORCE_MUST_GATHER_VALIDATION=true` env var
   forcing `ResolveAnyFailure()`'s `anyFailure` to `true`, and (2) flipping the E2E job's
   `Collect/Upload must-gather logs` steps from `failure() || cancelled()` to `always()` — both
   reverted immediately after inspection, never intended to reach `main`. This caught two real bugs
   that UT/pattern-matching review had missed:
   - The `signalprocessing`/`aianalysis` gap from step 4 above.
   - A systemic context-cancellation bug in 11 of 15 suites: `SynchronizedAfterSuite`'s first
     closure (runs on ALL processes) calls `cancel()` on the suite's shared `ctx`/`harness.Ctx`
     *before* the second closure (process-1-only, where the must-gather call lives) ever runs —
     `exec.CommandContext` against an already-canceled context fails immediately with `context
     canceled`, so `RunMustGatherImage` silently produced nothing. The `gateway` pilot didn't catch
     this because gateway's suite deliberately does *not* cancel its context before teardown; the
     already-working `fleetmetadatacache` (Kuadrant lane) suite didn't hit it because it already
     used a fresh `bgCtx := context.Background()`. Fixed by applying that same fresh-context pattern
     to `signalprocessing`, `aianalysis`, `datastorage`, `remediationorchestrator`, `fleet`,
     `workflowexecution`, `fullpipeline`, `notification`, `effectivenessmonitor`, `authwebhook`, and
     `fleetmetadatacache/eaigw`.

   After both fixes, a second full-CI validation run confirmed real, correctly-structured must-gather
   bundles (collectors' `logs/`, `events/`, `jobs/`, `database/`, `tekton/`, `cluster-scoped/`,
   `metrics/`, `SHA256SUMS`, `version-info.yaml`) for 14/15 suites, including `fleet`'s
   `--extra-namespace` diagnostics (`logs/mcp-system/`, `logs/istio-system/` populated with real pod
   logs) across both its primary and remote clusters. The 15th (`fleetmetadatacache-kuadrant`) hit
   its own pre-existing 15-minute job timeout unrelated to must-gather (Kuadrant/Istio infra setup is
   slow) and was killed mid-test before `AfterSuite` ran; the CI-level `kind export logs` fallback
   still produced a bundle, confirming that degraded path also works as designed.

---

## Consequences

### Positive

- ✅ Diagnostics survive Ginkgo process timeout/kill (the actual bug that motivated this work)
- ✅ E2E diagnostics now match production must-gather output exactly (dogfooding)
- ✅ No new RBAC objects to maintain in E2E clusters

### Negative

- ⚠️ E2E harness needs podman network access by name (`kind`) — already implicit in how
  `test/infrastructure` creates clusters, but worth calling out as a coupling point
- ⚠️ Uses cluster-admin-equivalent kubeconfig rather than a scoped role (no regression vs.
  today's `MustGatherPodLogs()`, which already does the same)

### Neutral

- 🔄 `cmd/must-gather/templates/*.yaml` (built for Alternative 2) remain unused by E2E but
  stay relevant for the in-cluster production use case (`README.md` "Method 3") — not being
  retired, just not reused here

---

## Validation Results

Both mechanisms were prototyped end-to-end against a disposable `mg-spike` Kind cluster with a
representative `kubernaut-system` workload (2 Deployments + 1 Job), using the unmodified
`cmd/must-gather` image built from `main` HEAD.

| Dimension | Alt 1: Local Container | Alt 2: In-Cluster Pod |
|---|---|---|
| Setup steps | 0 (reuse kubeconfig) | 2 (image load + RBAC apply) |
| Execution steps | 1 | 1 (+ wait/poll hack) |
| Extraction steps | 0 (bind mount) | 1 (`kubectl cp`) |
| Cleanup steps | 0 (`--rm`) | 2 (Pod delete + RBAC delete) |
| **Total discrete steps** | **1** | **8** |
| Collection duration | ~18s | ~18s (identical collector code) |
| One-time tax per cluster | none | ~18s (`kind load docker-image`) |
| `kubectl cp` reliability @ 150MB | n/a | ✅ 0.75s, checksum-verified |
| Survives Ginkgo process kill | ✅ yes (external invocation) | ❌ no (still needs a live process to extract/cleanup) |

---

## Related Decisions

- **Builds On**: [DD-TESTING-002: Integration Test Diagnostics (Must-Gather Pattern)](DD-TESTING-002-integration-test-diagnostics-must-gather.md) — the podman-compose integration-tier analog; this DD covers the E2E/Kind tier instead
- **Resolves**: [#2036](https://github.com/jordigilh/kubernaut/issues/2036) — E2E must-gather migration
- **Related**: [#2192](https://github.com/jordigilh/kubernaut/issues/2192) — Kind cluster teardown race forward-port (independent fix, same incident family)

---

## References

- **Spike workload/scripts**: ad-hoc, run against disposable `mg-spike` Kind cluster (not committed)
- **Existing local-dev pattern**: [`cmd/must-gather/Makefile`](../../../cmd/must-gather/Makefile) `test-e2e-container` target
- **New E2E collector**: [`test/infrastructure/must_gather_image.go`](../../../test/infrastructure/must_gather_image.go) `RunMustGatherImage()` / `BuildMustGatherImageForE2E()`
- **RBAC templates (Alt 2, not adopted here)**: [`cmd/must-gather/templates/`](../../../cmd/must-gather/templates/)
- **Follow-up tracking**: [#2194](https://github.com/jordigilh/kubernaut/issues/2194) (extended must-gather features: jobs.sh collector, `--extra-namespace`), [#2196](https://github.com/jordigilh/kubernaut/issues/2196) (pre-existing `KUBERNAUT_NAMESPACES` bash array-export bug in `logs.sh`/`events.sh`/`metrics.sh`/`cluster-state.sh`, found during this migration, out of scope for this DD)

---

**Approved By**: Repository owner (2026-08-18)
**Implementation Status**: ✅ Complete — all 15 E2E suites migrated and validated end-to-end via a
temporary full-CI run (2026-08-19, reverted after inspection), old collector removed, #2036 resolved
