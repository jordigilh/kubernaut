# Bounded PR #2637 review repair — 2026-11-16

Status: repairs 2/3 verified. The historical holds on decisions 1/4 below are
superseded by the user's explicit 2026-11-16 amended implementation approval,
recorded in DD-FLEET-015 and TEST_PLAN. No new implementation evidence is implied.
Historical baseline:
`13f629ccabbc15ed4680d89eff166b7dd2f03535`; remote main independently verified as
`741f7d8569fef9a5eed2d374c759b42298c0a6b8`. No main merge is authorized here.

## Active amended implementation — in progress, not publication-ready

Approved working base is `2cda8b1ecfdfe51c4ac8e00ce0c1136dd84eaa7d`.
DD-FLEET-015 and TEST_PLAN v1.6 were written before implementation. No new
branch, worktree, subagent, dependency, operator change or CI dispatch was used.
The complete enum-only/coordinated batch is **not implemented or committed**.

Owned-WATCH work currently replaces both informer lifecycles with the shared
dynamic-client owner; removes LIST-based Probe recovery and the readiness
refresher assertion; keeps AF/EM/SP prober installation; adds unhealthy/stopped
WithCurrent exclusion, joins Stop before event closure, paginated private LIST,
bounded WATCH establishment, quiet recovery, stream retirement and cancellable
backoff. FMC now connects SIGTERM/SIGINT cancellation before dependency startup,
returns startup errors through cleanup, preserves bootstrap health, and removes
only the dynamic WATCH client's inherited whole-request 30-second timeout.

Reproduced RED:

- `2593-owned-watch-red.log`: both registry initial Start calls incorrectly
  succeeded from LIST while WATCH was denied (two failures).
- `2593-startup-signal-red.log`: the actual compiled FMC command kept running
  after SIGTERM during successful-LIST/denied-WATCH startup. An earlier test-runner
  child process was discarded as invalid evidence because Ginkgo also owns
  SIGTERM; the recorded final RED launches the production executable instead.
- `2593-enum-scope-red.log`: three expected failures remain: missing target
  inherits managed namespace; missing namespace becomes verified unmanaged;
  ScopeResult serializes managed/reason/retryable. No typed GREEN is claimed.

Current WATCH proof and regression checks (logs in the checkout's temporary
parent directory):

- `2593-owned-watch-all-fleet-race.log`: every `./pkg/fleet/...` and
  `./cmd/fleetmetadatacache/...` race suite passes, including the new external
  HTTP registry → prober → Gate recovery journeys and production SIGTERM test.
  New selected source cases comprise six HTTP recovery/error journeys, seven
  private LIST/closed-WATCH cases and one production-command startup journey.
  These are execution cases, not an inflated 26-contract denominator.
- `2593-owned-watch-integration-final.log`: standard envtest 1.36.2 / real local
  Valkey suite passes **12/12 selected cases with -race**. Focus is
  `IT-REG-GW-005|FMC observed repair wiring`; 23 other cases are focus-excluded,
  not claimed as passing. This preserves registration revisions, namespace
  isolation, lost-write replay, deferred-target and repair wiring regressions.
  Uses per-command `CONTAINER_CONNECTION=podman-machine-default-root` and the
  installed envtest assets; no business overlay or persistent config change.
- `2593-owned-watch-build-final.log`, `2593-owned-watch-compile-final.log`,
  `2593-owned-watch-lint-final.log`: full repository build, repository test
  compilation and full lint (**zero issues**) pass. One concurrent build hit
  its harness timeout; the standalone rerun passes. This is not a source failure
  attribution and does not qualify the incomplete typed phase.

Refactor content: shared lifecycle state/publication exclusion; separate WATCH
establishment from stream consumption; isolate page validation; reuse UID-checked
deletion for disabled/untrackable registrations; retire observed stream failure
before transport teardown. No lint suppression or gate reduction was introduced.

Engram semantic evidence located the old LIST-only hold, DD-FLEET-013 revision
contract and the approved amendment. Exact references confirm production registry
Start callers in FMC/AF/EM/SP and ClusterRegistryProber installation in
`cmd/apifrontend/backend_deps.go:714`, `cmd/effectivenessmonitor/main.go:691`,
and `cmd/signalprocessing/main.go:468`. One semantic call failed with
`ZVEC_INDEX_FAILED: zvec-grep MCP closed without a JSON-RPC response`; later
semantic calls succeeded. Go method references are flat `Start`/`IsManagedResource`
(not guessed receiver/name paths); overloaded prober lookup uses `Probe[1]`.

Remaining work is explicit: complete typed RED matrix, implement enum-only
shared/local/FMC/cache/ACM/factory/consumer paths atomically, migrate every owned
mock/fixture/E2E decoder, complete FMC service-readiness/close proof and all eleven
wiring rows, assess all 26 critical contracts, run make test and final affected
checks, then commit/normal-push one coordinated batch. No final matrix numerator,
new CI qualification, full GREEN, independent-review readiness or merge claim.

### Replacement-agent recovery and pre-push gate (2026-11-16)

The replacement preserved the complete dirty checkout at the same base and
branch, read predecessor session `ses_e1985a566ffeHEb8txxhWgR5Zd` through the
harness, and confirmed no predecessor test/build/lint process remained. No
production source was changed by the replacement. An additional W04 external
HTTP MODIFIED/Disabled case was added to `owned_watch_test.go`; it passed without
an implementation change. The initial hypothesis that Disabled dispatch was
missing was disproved: `KuadrantRegistry.applyWatchEvent` already removes it.
This is regression proof, not a newly reproduced defect or extra denominator.

**Superseding user gate:** no push until the actual first-failing tests and
relevant affected suites pass on `helios08` using the exact complete candidate
and documented operator/dependency provenance. Local passes alone no longer
authorize publication. No commit, push, CI dispatch or cluster deployment was
performed by the replacement. A requested delegation attempt failed with
`Subagent depth limit reached (1)`; no child agent ran or edited this checkout.

Current PR-head CI is run **43963264751**, head
`2cda8b1ecfdfe51c4ac8e00ce0c1136dd84eaa7d`, testing synthetic merge
`d08b6f8be99fd723400967f63ae0f07797c9614a` onto
`741f7d8569fef9a5eed2d374c759b42298c0a6b8`. Fleet and FullPipeline passed on
that run, not on this uncommitted amendment. The current failures are:

- FMC IT job `133643031828`: **IT-FLEET-VALKEY-007**, line 208, expects nil
  error from the superseded boolean adapter for a Node target; actual error is
  `cluster_scoped_target_unsupported`. This is the old deferred-target test
  expectation, not evidence that the cache authorized a Node.
- FMC Kuadrant/EAIGW E2E jobs `133643031864` / `133643031928`:
  **E2E-FMC-1683-016**, `shared/tls_port_split.go:64–65`, queries Namespace
  `kube-system` as a target and expects HTTP 200. Actual HTTP 503 is consistent
  with the deferred cluster-scoped target guard through the boolean adapter.
  This proves the CA-verified request reached an application response; it is
  not evidence of a failed TLS handshake. Do not relax TLS assertions or allow
  the deferred target to make the fixture green.
- AF Fleet E2E job `133643031844`: **E2E-AF-FLEET-2462-002** observes 409-byte
  decision text, not the expected >512-byte grounded result; **2462-004** misses
  the expected `session_active` guidance. Sanitized AF/FMC artifact logs show
  `partition_unavailable` at 22:12:22.826/22:12:22.925/22:12:23.115 for the
  structured target, followed by AF scope refusal and the grounding guard's
  `no groundable investigation content available` override. The second caller
  similarly sees scope unavailability at 22:13:03.344/03.444/03.656. It is not
  justified to diagnose transport truncation or change the session/grounding
  guards from these failed assertions.

The underlying AF `partition_unavailable` predicate is **not established**.
`Client.checkFencedStatus` collapses `PartitionReady` errors; possible causes
include global prerequisites, fence/revision/generation mismatch and a non-clean
partition. The artifact lacks the internal failing predicate and repair-transition
trace. Ten-second repair cadence is present in the configuration, but timing
correlation is not proof of a repair-window or historical fencing RCA. RCA
confidence is **85%**; affected implementation/publication is paused under the
user's <90% escalation rule. Recommended next investigation is a bounded AF
Fleet reproduction capturing the internal readiness predicate and repair
transitions, without weakening proof, adding public diagnostics, changing retry
budgets, or redesigning fencing/availability.

Current-run operator artifact `14275224298` resolves merged operator source
`59e99fc9ffde0700251059bd77e1ac071b544f0b`, profile `generic-kind-v1.6`, amd64
digest `sha256:9bd454780f3a79138eacd70b79720ad237675caa7b8db59b5742dec73c360c32`
and arm64 digest
`sha256:d277ac08ae9a83da7eb2126fd7f945076c2e28a6fa9f32835a63e6243a3223b9`.
Artifact provenance is downloaded under `../2593-current-ci-operator`; this is
not a claim that any candidate operand image was built or remotely qualified.

Remote read-only preflight found root Podman 5.8.8, existing unrelated AA
PostgreSQL/Redis/mock-LLM containers and old envtest processes; these remain
untouched. No root Kind cluster was present. Work is isolated at
`/var/tmp/kubernaut-2593-recovery.3KCsUnY0` on `helios08` (no shared checkout
modified). Source transfer contains 5,882 tracked/current-approved files, no
`.git`, ignored files, local credentials or unrelated untracked files. Immutable
recovered-partial snapshot hashes (before this evidence amendment):

- source archive SHA-256:
  `3e3b4f6f3ef7decc687f68aa7d394191ec43b5c1f6e6a38cb075f13ceebc561b`
- complete per-file manifest SHA-256:
  `af9410f74b2bd98ceaf3f239baf8567c28e1c9661384dcaec98a47ad26f7d284`
- tracked binary diff SHA-256:
  `03ef1c958b767dad2d28ecf441cfd4e2c54aed09d1e62f6cc07035de19e1c0a3`

Remote verification checked archive hashes and every extracted file hash before
execution. **These are recovered-partial source hashes, not final-candidate
qualification.** Completed remote checks, with logs copied back into
`../2593-recovered-candidate`:

- `go test -count=1 -v ./pkg/shared/scope -ginkgo.focus='Enum-only scope'
  -ginkgo.no-color`: the same **3/3 expected RED failures** (S05/S06/S13).
- `go test -race -count=1 ./pkg/fleet/... ./cmd/fleetmetadatacache/...`:
  **all packages pass**, including both external HTTP recovery lanes and the
  actual production-signal test. This does not prove the still-unimplemented
  typed paths or all eleven wiring rows.
- `go test -timeout=5m -race -count=1 -v
  ./test/integration/fleetmetadatacache -ginkgo.focus='IT-FLEET-VALKEY-007'
  -ginkgo.no-color`: **same first-failing CI case reproduced**, exit 1, with
  the exact `cluster_scoped_target_unsupported` error. 34 other cases were
  focus-excluded, not claimed as passing. An initial attempt exited before
  specs because isolated HOME selected Go 1.26.8 with `GOTOOLCHAIN=local`;
  explicit per-command `GOTOOLCHAIN=go1.26.11` corrected only that prerequisite.

The IT used isolated HOME/TMPDIR, shared module/build caches only, explicit
envtest `1.37.0-linux-amd64` (matching the current CI run), and the repository's
unchanged `redis:7-alpine` harness. Observed Redis image digest:
`sha256:e4ee3e03ce8f75471f40937a7c6b2b0e500390b6812fcdf5cb32c01295ded446`.
Only the suite's own fixed-name containers were created/removed, after absence
and port checks. All commands have completed; no replacement-owned test process
or cluster remains running. Full remote E2E, enum GREEN, 26/26 and 11/11 proof,
make test, final gates, independent review and publication remain incomplete.

### Artifact-only AF RCA continuation (2026-11-17 UTC)

The next bounded agent preserved the complete dirty WATCH/enum checkout. Latest
applicable CI was independently refreshed: run **43963264751, attempt 1**, head
`2cda8b1ecfdfe51c4ac8e00ce0c1136dd84eaa7d`. Its synthetic merge
`d08b6f8be99fd723400967f63ae0f07797c9614a` has the **same complete Git tree**
`79dde012c57bcb266a43af26a5a5a7679b7e7238` as the PR head. These CI images do
**not** contain the unpublished WATCH/enum amendment.

**Superseding user conditions:** download/reuse CI/CD images, including implicit
fallback paths; no new production image builds. Keep the isolated reproduction
cluster and its services live for warm debugging. A new production image needed
for a fix or internal diagnostics requires a separately approved bounded
exception. The original exact-final-candidate remote pre-push gate remains.

Remote preflight again found no root Kind cluster and only the three existing
unrelated AA containers, which remain untouched. Work is under the existing
isolated recovery directory:
`helios08:/var/tmp/kubernaut-2593-recovery.3KCsUnY0/af-rca`.
Local evidence is `../2593-af-rca`. Remote `gh` is unauthenticated; the already
authenticated local `gh run download` route downloaded the selected original
artifacts and SCP transferred them without copying any authentication material.

The repository's `.github/actions/load-ci-images` Docker/OCI archive →
`podman load` route is reused. Only archive tag metadata was rewritten to the
private tag `2593-af-rca-43963264751`, avoiding replacement of existing host
`pr-2637` tags; every image config/layer is unchanged and the config digest was
verified after loading. The first AF load exposed the archive's additional OCI
index annotations, so that previously absent AF `pr-2637` tag was also created;
the loader was corrected before loading the remaining images. No pre-existing
tag was replaced. Full original archive hashes, API artifact digests, tags and
config identities are in `images.json` (SHA-256
`980a9bba338bb0f3edebf1b4e14e76d96fa9f559ce722159928fa154d775aa6d`).

| CI artifact | Artifact ID | Verified image config SHA-256 |
|---|---|---|
| image-apifrontend-amd64 | 14274708750 | 11c1b2407f294413226d38fff991ef55e5cd79690cda30388d537a01592d6fb9 |
| image-datastorage-amd64 | 14274739309 | e2ac222e3377f6275b38bc2358bfb8fa88e6675b8ff0815385b14f6c60238bf7 |
| image-kubernautagent-amd64 | 14275945739 | 022d17599d81290574a0313c4154c9d18506e48e1bb148cc56701a51fad17700 |
| image-fleetmetadatacache-amd64 | 14274404233 | 50d96acfdc1bfddebbd775625d201101a6bac7af68c6867591fe20d2b76839a5 |
| image-db-migrate-amd64 | 14275427659 | 77901bd82805dcbee453e2191ca23d6b1f984a2bfdf280f4c41fd7d7c57b86f0 |
| image-mock-llm-amd64 | 14275053531 | 34b512f287254c0c24a559d5c4b1085301087c7a378fd7da37ebae17cf66a67b |
| image-must-gather-amd64 | 14274259123 | 536d0e035814e9954646b788b77cc98a6be1932bc4a60eb6e62ac7509015d72a |

The identical previously downloaded operator artifact/provenance was transferred
and its archive SHA-256 `9bd454780f3a79138eacd70b79720ad237675caa7b8db59b5742dec73c360c32`
verified. **This value is an archive checksum, not a runtime image/config digest.**
The AF Fleet lane uses raw manifests, not the operator; merely transferring its
artifact is not evidence that the operator ran in this reproduction.

The original CI merge source archive SHA-256 is
`22bce7252c27a401e0615fc3247329d9702ed613fd4be572952659811966b594`.
Only two temporary **test-harness** lines differ, outside this checkout, in
`source-ci/test/e2e/apifrontend/e2e_suite_test.go`:

- Existing `AF_E2E_SKIP_TEARDOWN=true` now also suppresses binary-coverage
  collection, which otherwise scales the still-needed AF deployment to zero.
  No binary-coverage qualification is claimed for this diagnostic run.
- Retained Fleet-only setup skips the standalone/local setup block, which
  otherwise deletes Fleet-only Prometheus rules from that same retained cluster.

Assertions, production source/images, scope/grounding/session guards and retry
budgets are unchanged. `harness-only.patch` and complete per-file
`source-ci-manifest.json` (SHA-256
`66ac403139d0c187cb72a2235398d7eb88f6fa25a90372da1879f401fd6ea633`)
record these changes. Only the race-instrumented test runner and Ginkgo CLI were
compiled with Go 1.26.11; no application image was built. Per-command PATH guards
reject image build/prune/removal and Kind cluster deletion.

First focused actual case **E2E-AF-FLEET-2462-002** is launched via
`bash .../af-rca/run-first.sh`; a private, read-only Redis `EVAL_RO` observer
samples server TIME, status/index/target-record PTTL, UID, revision, generation,
full writer tuple and bounded proof fields without repair tokens, resource
bodies or arbitrary labels. Cluster: `af-fleet-2593-rca`; kubeconfig path:
`.../af-rca/home/.kube/af-fleet-2593-rca-config`. `AF_E2E_SKIP_TEARDOWN=true`
preserves it. **Execution result and exact internal predicate remain pending;
no new RCA confidence, GREEN or final-candidate qualification is claimed.**

Engram CODE and CORPUS semantic searches and exact references confirm that
`partition_unavailable` is returned before `ReadVerified`: it may mean a missing
partition or any `PartitionReady` rejection. Existing `/readyz` collapses these
to a boolean; the compiled artifact has no identified private snapshot/reason
endpoint. Redis proof captures must not be presented as a direct observation of
the process-local global snapshot. If existing telemetry cannot establish the
internal predicate, the no-new-image condition requires escalation, not invented
provenance or speculative repair.

#### Completed artifact-only executions and bounded RCA result

Both actual remote executions completed; there was **no production change,
application-image build, commit or push**:

1. Fresh isolated infrastructure, one process, actual
   `E2E-AF-FLEET-2462-002`: **1/1 selected PASS**, 585.061 seconds including
   infrastructure. The other 234 specs were focus/label-excluded, not passing
   claims. `af-first.log` SHA-256:
   `d5d7f339c43f34a7ff680c4350799a8048901ca49ca7e9b72180a872d8304d6d`.
2. Same live cluster, complete Fleet AF label, original CI **4 processes and
   seed 1794866481**, unchanged spec assertions: **8 PASS / 3 FAIL / 0 pending**,
   11/235 selected, 179.122 seconds. The 224 remaining specs are label exclusions.
   `af-suite.log` SHA-256:
   `8b7f876fd96a429a8c2069438e4668407b43b590bdc2bfe9eda39f75db5e4144`.

The full warm run reproduced **2462-002 at the original line 144, exactly
409 versus >512 bytes**, with preceding scope refusal and the grounding guard
override. `2462-003` additionally failed line 198 (missing early RCA).
`2462-004` failed **line 256**, because the first caller never reached KA
action=start; this differs from CI's line 274 second-caller guidance assertion
and must not be described as the identical assertion failure.

**Warm-state limitation:** this was diagnostic reuse, not a fresh final-candidate
qualification. It retained the first run's structured-target RR
`rr-cdf9a2dddf06-8b3f1f07` and its InvestigationSession, created at 01:47:32Z;
the warm structured request had a new context ID. No records were deleted or
reset. `warm-journey-metadata.json` records bounded identity/lifecycle metadata.
The session-active target had no prior journey from the first focused test.
The reproduced scope errors occur before the refused RR creation; retained
journey state is not used to claim an exclusive historical internal predicate.

The sampler captured **1,729 complete warm-run snapshots** and correlated all
**15/15 scope errors** with Redis `state=repairing`, `clean_commit=0`. Maximum
nearest server-TIME sample distance is **50.906 ms**. This is contemporaneous
proof state, not a cadence-only inference. Generation 28 was observed repairing
01:49:26.824690Z → clean 01:49:30.095976Z; generation 31 repairing
01:49:56.810279Z → clean 01:50:00.016373Z. These are sampled transition bounds,
not exact in-process transition timestamps.

For the reproduced structured case's first scope error at
**01:49:29.420519363Z**:

- Nearest atomic Redis server TIME: `(1794880169, 401645)`, 18.755 ms before the
  logged event; status generation **28**, target-record generation **27**.
- Status/index/target PTTL: **57370 / 47370 / 20399 ms**; status expiry
  `1794880226771`, target expiry `1794880189800`.
- Registration UID **33bbc5df-dde6-4360-9b7b-d7e98f876786**, expected revision **1**.
- Full writer tuple: holder **0fa5fa80-ae50-40cf-94b8-37baf61d4185**, session
  **319cb9e2-ed72-45b2-8374-e268dfa82b7e**, lease epoch **1**, fence **1**.
  This tuple and UID/revision remained unchanged across the captured repairs.

For the session-active target's first scope error at **01:49:58.648924481Z**:
server TIME `(1794880198, 672252)` is 23.628 ms after the event; status generation
**31**, target-record generation **30**; PTTL **58099 / 48099 / 21514 ms**;
same UID/revision/full writer tuple. The first event-triggered capture also
observed the existing `/readyz` body `not ready` with HTTP 503. It does not
provide individual global readiness flags.

The old target generation during a new repair is **not evidence of a stale
writer**: approved in-place `begin` advances status before source collection and
record replacement. The captures show positive TTLs, not expiry. Engram exact
definitions/references verify `Syncer.beginRepair` → `BeginPartitionRepair`
(non-clean state), collection/commit → `PublishPartitionClean`, and
`Client.checkFencedStatus` → `PartitionReady` before `ReadVerified`. The AF
refusal → `enforceGroundingGuard` path accounts for the 409-byte replacement;
there is no evidence supporting SSE truncation, a parser relaxation, a foreign
writer or an unsupported cluster-scoped target as this AF failure's cause.

**Confidence and stop boundary:** **95%** for the observed repair-window
unavailability mechanism and scope-refusal/grounding chain. **85%** for the
exclusive first internal `PartitionReady` return predicate: the same-instant
process-local global prerequisites, active writer view, registration snapshot
and partition snapshot are still not exported by these CI images. Redis state
does not substitute for that memory snapshot; an earlier global rejection or
concurrent snapshot boundary cannot be claimed ruled out. Therefore the user's
**<90% exact-RCA escalation gate remains active**. No implementation is justified
by this report, and no speculative fence/readiness/retry/grounding change was
made. Direct private predicate/global+partition capture needs an approved bounded
diagnostic-image exception or an explicitly approved alternate diagnostic plan;
the user currently prohibits any new production image build.

Evidence files under `../2593-af-rca` and the remote `af-rca` directory:

- `suite-redis-proof.jsonl`, SHA-256
  `542df27c723c6bd5ce7d78539a3ed0446a87284c493bdee03c17bcfd1fca8a24`.
- `scope-failure-correlations.json`, SHA-256
  `100391f0860726db025dc1b97a464f2c5de2c5315ce9892d2c582f8ce9f3f051`;
  `suite-repair-transitions.json`, operational AF/FMC logs and
  `suite-first-scope-error.json` retain timestamps and bounded proof fields.
- `node-image-config-verification.json`, SHA-256
  `89b474d1ff6a02dcc7e47370a9faac8e89b8a57d6eac663f3521fa528635f833`:
  CRI inspection verifies all **five deployed first-party images** against the
  exact artifact config hashes. Must-gather uses the CI artifact on the host;
  the db-migrate image was loaded but not used by this lane's host Goose path.
- `live-pod-images.json` and `deployment-runtime-conditions.json` record runtime
  identities, restart counts and resource limits. Actual kube-mcp-server pin is
  `sha256:c289505b06079d7f265a6fef977ce1abbfbbb4271772db17db5b58edbf44c255`;
  standalone Valkey 8.1 resolves
  `sha256:8f836008e0986c36adf50da714c7060cf689dbb335f61fe077939d90fa650809`.
  No producer tools/list schema or empty/nonempty response-body proof is claimed.

Race-instrumented test runner SHA-256:
`22c4d19ae299147d56e69ae85146aa1bcc3ef7499a5a5f541189f36fb2cb6022`;
Ginkgo CLI SHA-256:
`477eb94084840f65f00f3736a2eb96ab953e5ee82ca1b0672695c900cbe06d9a`.
Existing host `localhost/db-migrate:pr-2637` remains the pre-existing config
`a806913b54c030132162b58da0a11ee5cfa2efd632a86ea40c9d9f51a9d5904b`;
the private CI-loaded tag uses the independently verified `77901bd...` image.

**Handoff: KEEP `af-fleet-2593-rca` LIVE.** No cluster teardown, shared-host
prune or service restart was performed outside normal initial/warm setup of this
owned test cluster. AF and FMC remain running; readiness can still flap during
the captured repair windows. Reuse (diagnostic warm state, not fresh qualification):

```bash
ssh helios08 'bash /var/tmp/kubernaut-2593-recovery.3KCsUnY0/af-rca/run-warm.sh E2E-AF-FLEET-2462-002'
```

The full warm-suite command is `bash .../af-rca/run-suite-warm.sh`; review/reset
only explicitly owned journey state before treating any subsequent run as a
fresh scenario qualification. Do **not** rerun `run-first.sh`: its infrastructure
path is not a retained-cluster setup. Build and deletion guards remain installed.
The three unrelated AA host containers remain up. All RCA test/sampler/follower
commands have completed; no diagnostic mutation, new API field or hidden image
build was used. Exact-final-candidate remote gates, enum GREEN, 26/26, 11/11 and
publication remain incomplete.

A complete comparison against the preserved recovery manifest checked all 5,882
approved source files: the only differences are this evidence document and the
pre-existing TEST_PLAN documentation amendment. All business/test source from
the partial WATCH/enum candidate remains byte-identical to the earlier remotely
verified partial snapshot. This preserves the earlier source-attributed race
evidence; it does not turn the enum RED cases or old CI images into final GREEN.

### Approved single private diagnostic image (2026-11-17 UTC)

The user explicitly approved **one** internal-logging FMC image from the exact CI
source, solely for this retained isolated cluster. This supersedes the earlier
no-build condition for that one image only. All authorization, fencing, readiness,
repair, retry, parser and grounding behavior remains unchanged. No endpoint, public
response/header, privilege, registry publication or additional application image
was introduced. No production behavior repair or publication is authorized here.

Source provenance under remote `af-rca/private-diagnostic`:

- Baseline: `2cda8b1ecfdfe51c4ac8e00ce0c1136dd84eaa7d`; original PR-head archive
  SHA-256 `e3704d9f374e018a1de7a9ca7d1b90b266e1bd47215fb10fddda28babda1c49c`.
  Local Git-object comparison checked **all 5,877 baseline files**, with exactly
  three diagnostic differences and zero extra source files. The dirty WATCH/enum
  candidate was not the image input. Source was extracted in a scoped remote
  temporary directory, without a Git branch/worktree or shared-checkout edit.
- Instrumentation diff SHA-256:
  `9745dbb9d8266ed437c1bf281182f5e6f0fa16a939018c40aecae6d8035d8ea9`.
  Files: `pkg/fleet/scopecache/{readiness,client}.go` and
  `pkg/fleet/fmc/handler.go`. The handler adds a private per-request logger UUID;
  the client passes the context logger to the same ordered readiness predicates;
  only rejection paths log scalar proof metadata under the existing read lock.
  Existing return values/reasons and predicate order are unchanged. No bodies,
  labels, credentials, bearer/repair tokens or arbitrary request headers are logged.
- Source manifest SHA-256:
  `026cc5946749d05f5f6ebe0192bb06b3a4b7431e6e87b0566ae0b16d5bd90d7a`.
  Existing `go test -count=1 ./pkg/fleet/scopecache ./pkg/fleet/fmc` passes both
  package suites on this isolated source (`diagnostic-regression.log`). This is
  instrumentation regression evidence, not a functional-fix RED/GREEN claim.
- Exactly one `/usr/bin/podman build` succeeded using the existing
  `docker/fleetmetadatacache.Dockerfile`, target `development`, architecture
  `amd64`, `GOFLAGS=-cover`, `TARGETARCH=amd64`, and
  `GIT_COMMIT=2cda8b1ecfdfe51c4ac8e00ce0c1136dd84eaa7d`. Default
  `APP_VERSION=v1.6.0` / `BUILD_DATE=unknown` and pinned builder/runtime inputs
  remain in that recipe. Exact command and complete build output are retained.
  The initial SSH wrapper timed out, but the original remote build continued;
  process-exit notification confirmed success. **No second build was started.**
- Private tag `localhost/fleetmetadatacache:2593-private-diag-2cda8b1ec`;
  image config `a317e1882a5ffdcd909d0eda7b75ff26c7358ff2a110c527ccecb81892920d4c`;
  local manifest digest
  `sha256:228b0635d9e93c8a7026c656a6d5e65d01724396fc32048f146d3df0c91f0ec8`.
  Original CI and diagnostic binaries both report
  `go1.27.3 (Red Hat 1.27.3-1.el10_3)`; dependency build-info entries are identical.
  This verifies the actual compiler after the recipe's `dnf update`, rather than
  assuming the race-runner's Go 1.26.11 is also the application compiler.

`kind load docker-image` loaded only that image into **existing**
`af-fleet-2593-rca`; host and node config identities match. The deployment's
existing RollingUpdate policy could overlap two unsupported process-local writer
coordinators. Instead, the owned FMC deployment was explicitly scaled to zero,
the old pod's deletion and zero FMC pods were confirmed, its image alone was
patched, and one replica was started. Deployment UID
`ee8c0ed2-b629-4976-94a1-92c1bd4b53df`, original strategy and exact commands are in
`diagnostic-transition.jsonl`. All other observed pod UIDs, images and restart
counts were unchanged at transition completion. This intentional singleton
restart changes warm process/writer state; it is not hidden as an unchanged run.

All **seven** original CI host-image config hashes and the original `images.json`
checksum were independently rechecked unchanged. The operator archive checksum
remains `9bd454780f3a79138eacd70b79720ad237675caa7b8db59b5742dec73c360c32`.
Operator source `59e99fc9ffde0700251059bd77e1ac071b544f0b` has Go-module core
`v1.6.0-rc9.0.20261116145412-8c36d5628d88`, independently resolved to core source
`8c36d5628d8865f78c7dba14f6051685954539ea`. The AF Fleet lane still uses raw
manifests; this is dependency provenance, not a claim the operator was deployed.

#### Completed diagnostic executions

Exactly two diagnostic test executions completed, without assertion changes:

1. `python3 .../private-diagnostic/2593-private-runs.py` invokes the preserved warm
   environment through `private-diagnostic/run-focused-warm.sh
   'E2E-AF-FLEET-2462-00(2|4)'`: **1 PASS / 1 FAIL / 0 pending**, 2/235 selected,
   168.828 seconds, one process, seed **1794881682**. `2462-002` passes;
   `2462-004` fails its **first-caller action=start prerequisite at line 256**,
   not the original CI line 274 second-caller assertion. The 233 other specs are
   focus/label exclusions. `focused-runner.log` SHA-256:
   `082f7880a8cccc0851ed1266241e448ea2c5ed98cd057e4ab5a06f2909be79a7`.
2. The same driver then invokes `private-diagnostic/run-suite-warm.sh` with
   original CI **4 processes / seed 1794866481**, label `fleet-mode-af`:
   **10 PASS / 1 FAIL / 0 pending**, 11/235 selected, 121.102 seconds.
   `2462-003` fails **line 198**, missing early RCA; `2462-002` and `2462-004`
   pass. The 224 other specs are label exclusions. `af-suite.log` SHA-256:
   `e3d4aa2c73cc11745517fb18744ec398f6a9cba4df716ea328441be28450a8dc`.

These are **warm diagnostic executions**, not fresh qualification. The original
structured-target RR/InvestigationSession from 01:47:32Z remained in place. The
full run created the session-active target RR/InvestigationSession at 02:18:32Z;
no journey records were removed/reset. Final bounded metadata is recorded in
`journey-metadata-final.json` (captured after the tests and original-image restore).
The original retained-cluster BeforeSuite restarts the owned **mock-LLM pod** to
refresh its scenario configuration in each execution. This was not a new agent
service mutation; its exact CI image/config identity remained unchanged. Eight
other non-FMC pod UIDs/images/restart counts remained unchanged. The first
collector's stricter same-pod assertion identified this expected harness restart;
collection was corrected to record it explicitly and verify the unchanged image.
No test was rerun because of that evidence-collector correction.

#### Exact predicate and contemporaneous proof

The read-only sampler completed normally with **2,813 atomic server-TIME samples**.
All **9/9 observed FMC scope-check errors** have paired diagnostic/handler log
entries sharing the same private `diag_request_id`. All nine directly observe
`diag_reason="partition is not clean"`, `diag_stage="before_read_verified"`.
The same-lock snapshot and nearest Redis proof agree on UID, revision, generation,
writer tuple and `repairing` state; maximum nearest sample distance is **43.897 ms**.
There are no observed earlier-global or stale-fence/revision/generation rejections.

- All nine process-local snapshots have global readiness, MCP/registry initial
  sync, watch health, script readiness, Valkey readiness, active lease validity
  and lease heartbeat validity **true**. Partition count and registered count are
  **1**, registration/membership revision **1**, both fence comparisons true,
  expected/current generation equal, lease invalid flags false. The partition
  is `repairing`, `clean_commit=false`. `cleanProofComplete` short-circuits first
  on `State != PartitionClean`; its cleared proof-validity flags are **not** a
  direct observation of Redis TTL expiry.
- Focused `2462-004`: three rejected requests at **02:15:35.983961Z →
  02:15:36.317611Z**, generation **186**, UID
  `33bbc5df-dde6-4360-9b7b-d7e98f876786`, holder
  `b0b6b039-a365-4088-9311-88004820af9c`, session
  `b9dc7f52-86de-4e71-9c7b-ae6224561003`, epoch/fence **1/1**. First private
  request ID `e3d4331e-3647-48e0-bb7e-5bcf271f301b`; nearest Redis TIME
  `(1794881735,947544)`, **36.221 ms before** the diagnostic timestamp;
  status/index/target PTTL **58262 / 48263 / 21670 ms**. Target-record generation
  **185** is expected during the new in-place repair, not proof of a foreign
  writer. Sampled repair bounds: **02:15:34.295357Z → 02:15:37.502132Z clean**.
- Full run: three no-rules-target rejections in generation **202**, followed by
  three progressive-investigation target rejections in generation **203**.
  The no-rules spec itself passes; nine FMC rejections do not mean nine failed
  tests. First progressive request ID
  `2d021099-1e20-44d8-886c-faf3c8a922e8` at **02:18:26.700258Z**;
  server TIME `(1794881906,713323)`, **13.200 ms after**; status/index PTTL
  **57496 / 47496 ms**. Generation 203 sampled repairing **02:18:24.240553Z →
  02:18:27.373145Z clean**. The existing sampler targets only the structured and
  session-active records: **no target-record PTTL is claimed** for the no-rules
  or progressive target. All nine status/index PTTLs are positive, minima
  **57185 / 47185 ms**; all three sampled session-active record PTTLs are positive.
- These UUIDs correlate each HTTP handler invocation to its exact internal
  predicate. They are private logger IDs, **not** a new wire header or a claimed
  propagated AF trace ID. AF correlation uses the same distinct target and
  timestamps: final FMC attempt → AF `scope validation failed` at
  **02:15:36.317Z**, **02:18:16.530Z**, **02:18:27.009Z**. No new
  `grounding-guard overriding` event was captured during these two executions.

Evidence: `predicate-correlations.json` SHA-256
`508bd37990c7eef088b1947f2aee8ecb24dda52e3e0130d208e74b7e04bf97cf`;
`redis-proof.jsonl` SHA-256
`a61f325798769c534b00eec7d81093c095656173eb47a48e2df476f96328eddf`;
FMC operational log SHA-256
`d4fc98b41e8dc2bdcf941ea14519c76d6b3311e779c77ec47c67da7ac352a185`.
`repair-transitions.json`, AF operational logs and `capture-summary.json` retain
the complete bounded evidence. Logging is synchronous under the existing read
lock only on rejection and adds a per-request UUID/context; it is not zero-cost.
No extra backend calls or retry loops were added by the diagnostic source.

#### Reconstructed flow and contract disposition

Engram CODE semantic searches, CORPUS recalls and documentation-scoped semantic
searches located the proof lifecycle and relevant contract. Exact symbol and
caller lookups verified `PartitionReady`, `BeginPartitionRepair`,
`checkFencedStatus`, `ReadVerified`, `groundFleetAFSession` and the AF refusal/
grounding guards. Intermittent Engram timeouts/503s were not treated as negative
search evidence; bounded source reads supplied the exact definitions.

1. `Syncer.Run`'s periodic `syncAll` calls `SyncCluster` → `repairPartition` →
   `beginRepair` before source collection. The guarded status-store begin advances
   generation and sets Redis repairing; `BeginPartitionRepair` replaces local
   clean proof with repairing state and clears clean eligibility.
2. `Client.Check` → `checkFencedStatus` obtains registry identity/current partition
   generation, then `PartitionReady`. For the nine captured requests, all earlier
   global/fence/registration/generation predicates pass; `partitionClean` rejects
   `State=repairing`. `ReadVerified` is **not reached**, so neither its script's
   record checks nor namespace fallback can be the first rejection in these calls.
3. `Check` returns unavailable `partition_unavailable`; `IsManagedResource`
   returns an error; the unchanged FMC handler returns **503/no managed decision**.
   The unchanged HTTP client exhausts three attempts roughly **0.3 seconds** apart
   in total, inside the sampled approximately three-second repair window. Its
   two-second ceiling is not a claim that it waits two seconds before exhaustion.
4. AF `checkRRScope` refuses RR creation before KA action=start. This accounts for
   focused `2462-004` line 256 and full `2462-003` line 198. Source inspection of
   `enforceGroundingGuard` explains the previous artifact-only 409-byte replacement
   after refused grounding, but that replacement **did not recur** in the new runs.

This observed partition rejection is **intended fail-closed behavior**, not a
demonstrated stale-writer/global-publication bug. Authority: BR-SCOPE-001 /
BR-INTEGRATION-065; `TEST_PLAN.md:1121–1135` explicitly requires begin-before-source
collection and clean only after complete validation/commit;
`readiness.go:48–50,278–301,407–436,503–515` rejects repairing/uncertain proofs;
DD-FLEET-012 preserves singleton coordination and fencing; DD-FLEET-013 preserves
complete proof ownership/membership and unavailable missing targets. Removing
that guard, serving the old generation, changing retry budgets or changing
grounding to admit failed investigation is **not** an ordinary evidence-backed fix.
No such change, readiness assertion weakening, new DD or fix test was made.

**Confidence: 98% for the exact internal rejection of these nine requests and
the newly reproduced line-256/line-198 failures; 95% for the general scheduled
repair-window unavailability mechanism. Confidence remains 85% for the exclusive
internal predicate of the original CI 2462-002 line-144 / 2462-004 line-274
assertions.** Both diagnostic executions passed 2462-002; line 274 was not
reproduced. Direct same-lock proof from other requests cannot retroactively expose
the original process snapshot. Warm journey retention and the intentional FMC
process restart also prevent presenting the better pass count as a fix.

Therefore the original-assertion **<90% gate remains active**. Instrumentation was
sufficient for observed rejections, but the exact two requested CI assertion
failures did not recur with it. Stop rather than prescribe a speculative
production fix or repeat tests indefinitely. No production fix is justified by
this bounded round. A further exact-assertion capture using the already-built
private image would require an explicitly bounded next diagnostic decision;
changing intended repair availability would require a separate approved contract.
The actual-final-candidate first-failing/affected-suite remote pass gate is
unchanged and is not satisfied by these diagnostic runs.

#### Final retained state and reuse

All test runners and the sampler completed; sampler exit **0**. After saving
diagnostic logs, the **original CI FMC image was restored** using the same
zero-pod-confirmed singleton transition, with no rollout-policy change. At
**02:22:46Z**, the same deployment UID has one updated/ready/available replica,
image `localhost/fleetmetadatacache:2593-af-rca-43963264751`, node config
`sha256:50d96acfdc1bfddebbd775625d201101a6bac7af68c6867591fe20d2b76839a5`.
`restore-transition.jsonl` and `final-state.json` record exact commands/state.
The private diagnostic image is retained locally and in the node, not published
or currently deployed. Original readiness may still flap during periodic repair.

**KEEP `af-fleet-2593-rca` LIVE.** Kubeconfig remains
`/var/tmp/kubernaut-2593-recovery.3KCsUnY0/af-rca/home/.kube/af-fleet-2593-rca-config`.
The three unrelated AA host containers remain up; an unrelated
`/root/kubernaut-2592-val` RO integration runner observed during final inspection
was not stopped or modified. No RCA runner/sampler remains active. No cluster
rebuild/teardown, shared-host restart/prune, new branch/worktree, commit or push.

Existing original-image warm reuse command (only after the next bounded decision):

```bash
ssh helios08 'bash /var/tmp/kubernaut-2593-recovery.3KCsUnY0/af-rca/run-warm.sh E2E-AF-FLEET-2462-002'
```

The exact full warm command is retained in `private-diagnostic/run-suite-warm.sh`:
`ginkgo -v --procs=4 --seed=1794866481 --timeout=18m
--label-filter=fleet-mode-af --no-color .../af-e2e.test`, with the existing guarded
environment and `AF_E2E_SKIP_INFRA=true AF_E2E_SKIP_TEARDOWN=true E2E_COVERAGE=true`.
Use a **new output prefix** for any approved reuse; do not overwrite either prior
run's evidence or rerun `run-first.sh`. Reusing it now runs the restored CI FMC,
not the diagnostic image. The transition scripts deliberately refuse to overwrite
their existing history files. No additional execution is started automatically.

Final local preservation verification checked **all 19 pre-existing dirty paths
other than this evidence document**, including the deleted file and untracked
WATCH/enum tests: every file remains byte-identical/absent as originally observed.
There are no additional dirty checkout paths. HEAD/tree/branch remain
`2cda8b1ecfdfe51c4ac8e00ce0c1136dd84eaa7d` /
`79dde012c57bcb266a43af26a5a5a7679b7e7238` / `work/2593-fmc-presence`.
Preservation-manifest SHA-256:
`37d6bfb66d7a92fcb653546b304be0b8de2db7d2298fd94829c26abe77420ea6`.
`git diff --check` passes. Only this factual evidence appendix changed in the
checkout during the diagnostic round. Private evidence is retained remotely at
`helios08:/var/tmp/kubernaut-2593-recovery.3KCsUnY0/af-rca/private-diagnostic`
and copied locally under `../2593-fmc-private-diagnostic`.

### Approved bounded AF contract amendment — partial RED handoff (2026-11-17 UTC)

The user's unconditioned approval to record corrected contracts, implement
deterministic regression coverage and validate remotely alongside the approved
enum migration is recorded **before test edits** in TEST_PLAN v1.7 and
`docs/testing/2462/TEST_PLAN.md`. Engram semantic CODE/CORPUS searches and exact
`resolveInvestigationRR`/`checkRRScope` definitions/callers corroborated ADR-068's
new-RR admission versus existing-RR takeover distinction. No production source
was changed during this bounded amendment. Pre-existing WATCH/enum work remains
partial; this is **not an atomic GREEN or publication-ready handoff**.

New `IT-AF-FMC-2593-001` extends the existing observed-repair fixture. A
test-owned channel in the **external MCP list handler** holds actual collection
after real SyncCluster/BeginRepair. Assertions observe a new repairing generation
and cleared clean commit; real FMC HTTP returns 503 without a managed decision;
the real AF investigation handler creates no RR, starts no KA investigation and
returns no RCA/summary/session. Releasing collection completes the same guarded
generation/fence as clean, and separate direct and HTTP scope checks authorize
the verified observed Pod. There is no readiness mutation, sleep, reservation
API, investigation replay or production protocol change.

The final refusal assertion is deliberately **RED**: the old AF boolean consumer
says `resource not managed ... Add label kubernaut.ai/managed=true` instead of
unavailable. This is the already-approved DD-FLEET-015 consumer gap, not evidence
that all AF success failures share an exclusive RCA. GREEN belongs to the
incomplete atomic enum migration; no ad-hoc message-only production fix was made.

Execution evidence:

1. Local `2593-af-repair-amendment-red.log`: seed **1794884226**, **0 pass / 1
   fail / 0 pending**, 1/36 selected, **41.228s** suite time. Failure is the
   unavailable-message assertion at line 265, after all refusal/safety and clean
   recovery assertions passed. The 35 others are focus exclusions.
2. Remote `contract-amendment.GAnBWNfO/repair-wiring.log`: exact archived source,
   native `go test -race -count=1 ./test/integration/fleetmetadatacache
   -ginkgo.focus='FMC observed repair wiring' -ginkgo.seed=1794884226
   -ginkgo.no-color`, **8 pass / 1 fail / 0 pending**, 9/36 selected,
   **46.657s** suite time. Same line-265 RED; all eight existing observed-repair
   cases pass. New isolated HOME/TMPDIR/envtest 1.37.0 and actual Valkey; no
   application image build. Only the fixture's three named dependency containers
   were created/cleaned; the live AF cluster and unrelated RO/AA work were not
   stopped or changed by this IT.

`groundFleetAFSession` now retains and parses the stream instead of discarding
it. Before presentation, 002 requires current A2A context/task → RR metadata,
genuine investigation_summary content → same RR, and an exact RR-bound persisted
InvestigationSession with nonempty KA correlation. 003 uses the same
single-invocation admission/grounding proof before its original early-RCA and
summary assertions. The >512 structured-decision fixture, all three causal-chain
entries, workflow options, severity/confidence and positive auth/SSE media
assertions are unchanged. The shared SSE reader additionally rejects malformed
JSON and JSON-RPC errors; a failed terminal status cannot count as grounding.

First remote amended-test execution: **0 pass / 2 fail / 0 pending**, 2/235
selected, **74.330s**, one process, seed **1794866481**. Original CI application
images and retained warm journeys were used. 002 fails the new current-RR
admitted-success prerequisite before presentation (line 413), not the original
>512 assertion. 003 reaches a genuine RCA artifact but failed a newly added
overstrict test assertion excluding the emitter's explicitly supported
missing-causal-chain placeholder. That assertion was a **test defect**, not a
production defect: `ka_investigate_bridge.go:289–330` explicitly permits a genuine
KA RCA without chain entries. It was removed from the grounding prerequisite;
the separate structured fixture's three-entry assertion remains unchanged.
One source-corrected validation execution follows, not an investigation retry
loop or repeated test-until-green. Final results are recorded below.

The source-corrected execution also completed **0 pass / 2 fail / 0 pending**,
2/235 selected, **72.773s**, one process, seed **1794866481**. 003 fails the
current-RR admission prerequisite (line 416). 002 obtains content but exposes a
second **test-only mapping defect** (line 464): the direct-MCP signaler writes
synthetic `a2a-<rr_id>` into immutable IS `spec.a2aTaskID`, not the live stream's
task ID (`ka_investigate_mcp.go:647,731`). Actual read-only CRD inspection confirms
this for all three retained sessions. The final helper therefore keeps strict
live context/task/RR/content matching and follows that exact RR to its persisted
session/correlation, instead of misinterpreting the CRD field. A retained session
alone still cannot satisfy the current stream's genuine-content proof. No
production session field or lifecycle is changed. **This final mapping correction
has local compile/lint verification only; no third remote rerun was started.**
Neither remote failure is presented as new original-assertion RCA closure, and
the helper's two authoring errors are not attributed to production defects.

Second race-runner SHA-256:
`e7e789ea99fc464f051adfb8c3b49efccd0a01176f1191fad974325cba8984a1`;
`focused-grounding-finaltests.log` SHA-256:
`bce0639badf53d72f3311b26f65b7e65ecb361e4c316f8174b976c672ce8360e`.
The filenames `finaltests` identify that immutable second snapshot, **not** a
claim that the later mapping correction or complete production candidate was
remotely qualified. No full AF suite was run during this amendment: 004's required
correction and the atomic production migration are still incomplete.

Remote evidence root:
`helios08:/var/tmp/kubernaut-2593-recovery.3KCsUnY0/af-rca/contract-amendment.GAnBWNfO`.
Original snapshot archive SHA-256
`bc8f3c9aa66fc41b10707551b9a310f39f325dae56707c10b8a321893aa6278e`;
all **5,882** extracted file hashes verified. Its manifest SHA-256 is
`884864e79ded226b586c128731c095370ec01ae6ab125f1050fe98afc81b7dd3`.
The original race-runner SHA-256 is
`8687129ecadc1f89f6cd1b30f51d083d9104d0156803a61c646b97f07b6f79fd`;
first focused log SHA-256
`b9b2a1785ec8fed93cae6ae8188ca9610a8e41b49d8e3115a8b7b257d587d1b7`.
An initial compile wrapper referenced absent `/usr/local/go/bin/go`; it ran no
compiler. The corrected command used installed `/usr/bin/go`, Go 1.26.11.

Corrected-test archive SHA-256
`3fd34a7d6adece5ed8ec4752bde823b207fbf3c8fcc87081880c07d9afd93623`;
manifest SHA-256
`e487cbfa9e0172b8c4515467fac035092126e3a1c409753ca11f7e53fcf79552`.
Both snapshots preserve complete source; the separate remote compile directories
apply only the **existing two-line retained-cluster harness overlay**, recorded
by before/after hashes: skip standalone setup in the Fleet-only retained lane,
and skip destructive binary-coverage collection under SKIP_TEARDOWN. No parser,
auth, business assertion, image or production source overlay is applied.

**Remaining contract work is explicit:** 004 is not yet corrected to use an
authoritative existing rr_id for caller two or to reject a completed refused
first stream promptly. Its existing action=start barrier is preserved, not
weakened. Its static LLM selector currently supplies resource arguments; the
mock's existing `$from_tool` interpolation resolves string fields from prior
tool responses within the same conversation, not an RR learned by another
concurrent request. Do not substitute a retained warm RR, accept arbitrary
unavailable/conflict as session_active, mutate the shared fixture during parallel
callers, or quietly add a new test-service image. Resolve the test-owned binding
mechanism before claiming this contract implemented.

No full affected-AF-suite qualification or 4/4 amendment qualification is claimed.
The atomic enum migration, 26 critical contracts and eleven wiring rows remain
incomplete. The nine prior same-request repairing refusals retain 98% confidence;
the original CI 002 line-144 / 004 line-274 exclusive RCA remains **85%**. No
new diagnostic capture with the private image was needed. The prior logging-only
image-build exception is consumed. Any functional candidate application/test-
service image needed for final qualification requires a new explicitly bounded
image exception **before building**; old CI images cannot qualify changed
production binaries. No commit, push, CI dispatch, new branch/worktree, subagent
or merge was performed. Keep `af-fleet-2593-rca` live.

Final local validation: `go build ./...` passes (no production source changed
afterward); final affected E2E/FMC IT package compilation passes;
`golangci-lint run --timeout=5m` reports **0 issues**; whitespace review passes.
Logs are `../2593-af-amendment-{build,compile-handoff,lint-handoff}.log`.
`make test` and complete exact-candidate AF qualification are **not** claimed.
REFACTOR: the existing shared stream/binding checks replace discarded grounding
bytes; no new business component was created. The first-round assertion defects
and their explicit corrections are preserved above rather than called GREEN.

Bounded test-only amendment diff (three test files plus #2462 clarification):
`../2593-af-amendment-evidence/bounded-test-amendment.patch`, SHA-256
`ba6d517f4debe1a9c201876bd34835175f1cb910f4db38584a086111bb93cc05`.
It is generated against exact pre-round bytes; the pre-existing FMC IT file hash
matches the preserved recovered-partial archive before diffing. The #2593 plan
and this evidence document are intentionally separate documentation amendments.
The final helper file SHA-256 is
`84e2457335d38dc652c3fcb0c82c1c7871108800e941925f51be5c5633f5f8f1`;
relative to the second remote source snapshot only that helper, this evidence
document and the plan's handoff-status paragraph differ. **No final-helper remote
pass is implied.**

Final read-only remote inspection records ten service pods and **no owned test
runner**. All five first-party node image config hashes remain the original CI
artifacts, including FMC
`50d96acfdc1bfddebbd775625d201101a6bac7af68c6867591fe20d2b76839a5`
and AF `11c1b2407f294413226d38fff991ef55e5cd79690cda30388d537a01592d6fb9`.
Each standard retained BeforeSuite restarted only the owned mock-LLM pod with its
unchanged CI image/config; all retained RR/IS journeys remain present, including
the progressive RR created by the first amendment execution. This is warm state,
not a fresh journey qualification. No private diagnostic image was deployed.
Fourteen bounded evidence files were copied to
`../2593-af-amendment-evidence` and all hashes verified; kubeconfig, credentials
and raw must-gather pre-sanitize backups were not copied into this packet.
No unrelated host container/service was removed, pruned or restarted.

### 004 local completion, image inventory and pre-push conflict gate (2026-11-16)

**Disposition: 004's approved test-source correction is complete locally;
amendment qualification remains 0/4.** No production behavior was changed in
this phase. No commit, push, image build, cluster execution/change, teardown,
new branch/worktree, subagent, merge/rebase, stash/reset or discard occurred.
The live retained cluster was not contacted; prior pod/image records below are
historical provenance, not a new live-state claim. WATCH/enum work remains partial.

#### Ownership and actual-argument proof

`test/e2e/apifrontend/fleet_mode_e2e_test.go`, scenario 004, now sends caller one
with an explicit unique context. It incrementally consumes emitted contextId,
taskId and rr_id. JSON-RPC/message IDs are not task IDs. Only the exact namespaced
RR's InvestigationSession with a nonempty KACorrelationID releases the barrier;
the pre-invocation set of already-correlated RRs cannot supply new action=start
proof. The exact RR is then read back and checked for hub/Pod/fixture identity.
No target-matching retained session can authorize caller two. The original
15-second response-header and 90-second action-start budgets remain. Completed
refusal, failed/canceled/rejected status, malformed JSON, JSON-RPC error and EOF
fail promptly, without waiting 90 seconds for an impossible start.

The new `fleet_session_stream_test.go` owns the live reader. Cleanup cancels the
request, closes its body and requires the reader's done channel within five
seconds. Channel sends select cancellation; no assertion executes in the reader
goroutine. `helpers_test.go`'s unchanged wire shapes moved into this self-contained
helper file. The completed-stream reader uses the same strict parser. The former
substring-only session-active helper was replaced, not retained as an alternative
success path. A retained same-RR correlation is also rejected rather than silently
treated as this invocation's start; a future warm run needing a fresh journey must
declare/provision that state explicitly, not retry the investigation until green.

Caller two has a different explicit context and uses
`fleet lease contention investigate rr_id=<captured-name>`. This is not an attempt
to influence an unconstrained LLM by omitting resource words: the new external-LLM
scenario `af_fleet_existing_rr_2593` implements the existing
ScenarioWithContextConfig interface and emits **exactly**
`kubernaut_investigate({"rr_id":"<captured-name>"})`. It is registered in the
existing default registry before service execution. Its map is freshly allocated
per request. It reads only LastUserContent, requires AF caller scope and the actual
advertised tool, and validates the entire remaining marker as a plain Kubernetes
resource name. URLs, empty IDs, extra arguments/instructions and old-history-only
markers do not select it. There is no shared fixture mutation, cross-conversation
lookup, resource-argument fallback, network lookup or new production API.

Actual OpenAI HTTP handler output is captured for **24 concurrent distinct RRs**;
each has the exact singleton rr_id argument map. Actual Gemini HTTP handler output
with an older different RR in history contains only the last user's current RR.
These provider-level checks are in `test/services/mock-llm/af_existing_rr_2593_test.go`;
they prove the fixture's emitted tool arguments, not a live AF/KA invocation.
The E2E additionally requires caller-two live context/task, exact rr_id metadata
and visible current-RR session_active guidance. Arbitrary unavailable/conflict,
another caller/task/RR, failed stream, early_rca/investigation_summary/decision
artifacts and synthetic RCA/summary/confidence/causal-chain DataParts cannot pass.

Engram semantic CODE evidence corroborated the existing investigation flow;
CORPUS evidence includes ADR-068:435–440 and the approved TEST_PLAN:48 contract.
Exact Engram symbol/reference requests timed out in Serena; the permitted gopls
fallback found resolveInvestigationRR's production caller at
`ka_investigate_mcp.go:381` and the test export only. Source definitions were read:
resolveInvestigationRR:474–517 runs creation/scope only without rr_id;
rejectKnownActiveSession:394–397 precedes readiness barriers; successful
startKAInvestigation:418 precedes finalizeInvestigationStart:921–970, which writes
the KA correlation. EventBridge:407–445 emits status metadata, context/task and
merged exact-RR context. IS spec.a2aTaskID remains synthetic a2a-<rr_id>, and
investigation_summary.session_id remains the RR, not the KA correlation ID.
No missing causal-chain requirement was reintroduced into 002/003 grounding.

#### Local RED/GREEN and verification scope

New test files were added only after existing E2E/helper/mock-provider/registry
patterns and ScenarioWithContextConfig were inspected. This is approved bounded
test infrastructure, not a new business component or new production wiring row.
Its wiring is existing default scenario registry → real provider HTTP handler →
serialized external tool call; the live business dispatch remains unqualified.

* `../2593-004-mock-red.log`: seed 1794886758, **5 pass / 1 fail**, six selected;
  the existing provider handler emits no tool call for the new marker. This is a
  behavioral RED, not absence/compilation failure. An earlier cold compile command
  hit its 120-second command budget without a test result; it is not RED evidence.
* `../2593-004-stream-red.log`: seed 1794886828, **9 pass / 3 fail**. This local
  RED used the factored weak retained-correlation predicate (the safety-significant
  predicate after assuming old target selection), not a live reproduction of
  the entire old Kubernetes target-query loop. No live 004 failure is attributed
  from this isolated RED. No-current-binding, retained RR and wrong RR namespace
  all incorrectly passed that predicate; the exact-RR predicate fixes them.
* `../2593-004-stream-final5.log`: final exact-helper race execution passes all
  **15/15** helper precondition cases, seed 1794887557, 2.282 seconds suite time,
  with no pending/skipped specs. The final run includes explicit rr_id metadata
  and failed-stream rejection, plus existing-constant/error-message cleanup.
  These are helper scenarios, not additional journey/26-contract credit. The
  exact helper and spec files are copied byte-for-byte to
  `../2593-004-local-contracts`; a tiny standalone Ginkgo bootstrap plus verbatim
  existing wire constants runs them without E2E BeforeSuite, image building or
  cluster setup. They remain part of the ordinary E2E package as well.
* `../2593-004-mock-final.log`: **353/353** mock-service specs and **14/14** handler
  specs pass with -race, including all seven new fixture scenarios. No skips or
  pending tests. Build and affected E2E/FMC-IT compile-only checks pass. Lint after
  deduplication reports **0 issues**. The first lint invocation stopped because
  another unrelated linter held its process lock; --allow-parallel-runners runs
  the same rules without touching that process. The intermediate six new lint
  findings were fixed (existing constants, obsolete helper, meaningful namespace
  test), not suppressed. `make test`, live IT/E2E and atomic enum GREEN are not
  claimed. No integration harness or image builder was invoked.

Final source checks: `../2593-004-build-final5.log` (go build ./..., exit 0),
`../2593-004-compile-final5.log` (AF E2E/FMC IT compile-only, exit 0), and
`../2593-004-lint-final5.log` (**0 issues**, exit 0). The additional terminal-status
assertion exposed one more repeated `failed` literal after the earlier clean
lint; it was replaced by the existing constant, not suppressed. No source code
changed after these final checks. Documentation and generated manifests are
separate. `git diff --check` passes. The reconstructed exact **004-only** code
diff is `../2593-004-evidence/004-only-code.patch` (seven code/test paths); its
initial helper bytes are verified against the prior documented SHA-256
84e2457335d38dc652c3fcb0c82c1c7871108800e941925f51be5c5633f5f8f1.
`preservation.json` compares every prior snapshot path and confirms no change to
any pre-existing cmd/pkg/internal/api source or test, including the partial
WATCH/enum files. The separate broader delta explicitly includes the intervening
final 002/003 mapping correction; it is not mislabeled as this phase's work.

REFACTOR content: consolidate incremental/completed SSE parsing, move identical
wire shapes, replace duplicate weak guidance detection, and reuse constants;
the request-local fixture has no stored request state. Local source build,
compile/race/lint and whitespace checks are the safety net, not the refactor.
Source-only confidence: **95%**, from direct production control-flow/field reads,
actual provider HTTP argument proof, retained-state negatives, cancellation/join
proof and local regression checks. Exact-candidate runtime qualification is still
absent; no new original-CI exclusive RCA percentage is claimed. The earlier
85% original 002/004 RCA and 98% nine later repairing refusals are unchanged.

#### Atomic remaining work — do not freeze/build the current partial source

1. Complete the approved enum-only contract atomically. `pkg/shared/scope/checker.go`
   still exposes Managed, public Reason/Retryable JSON fields and the boolean
   ScopeChecker seam. `Manager.Check` still needs the approved presence/inheritance
   semantics. The three existing enum-contract REDs remain outside this phase.
2. Complete remote adapters/factory and public v1 together: FMC handler/client
   still use managed:boolean (including unknown-registration false), not strict
   state/provenance/status decoding. ACM and federated dispatch still retain
   boolean conversion paths. Preserve full fences, UID/revision/generation,
   completeness/TTL, no-replay and DD-AF-018's closed ACM Console inventory gate.
3. Migrate all GW/RO/AF action and exact-inventory consumers and their fixtures.
   Current callers include gateway/signal_ingestion.go,
   remediationorchestrator/routing/blocking.go, apifrontend/tools/af_create_rr.go,
   and apifrontend/policyread/service.go. Unavailable must not become unmanaged or
   label advice; existing-RR contention is not new-RR admission. Finish the
   required S01–S18 acceptance matrix, not only the error message.
4. Preserve and reverify the existing owned-WATCH candidate, W01–W08 and all
   eleven wiring rows as part of the complete resolved source. The pre-existing
   tests already exercise real dynamic HTTP clients, both registries, denied
   WATCH recovery and readiness.Gate; the command tests cover startup signals.
   They were not removed/replaced or falsely described as missing. Historical
   package/command passes do not qualify a later changed/resolved candidate.
5. Resolve the actual incoming conflict as below, then refresh the source/image
   inventory and run every required post-resolution gate. This phase did not
   finish the whole atomic migration or add unrelated cleanup.

#### Functional-image exception inventory — prepared, NOT approved

`../2593-004-evidence/source-sha256.json` records the exact current local tree
with deleted paths separately; `snapshot-status.json` explicitly says no atomic
freeze/no build approval. `af-only-service-source-closures.json` records repository
source files and hashes from actual go-list dependency closures for FMC, AF and
mock-LLM. These are inspection artifacts, not build instructions or a release
snapshot. Recreate them after atomic completion and conflict resolution.

| Future AF-only image | Why a changed artifact is needed | Current changed source boundary |
|---|---|---|
| FMC | Owned WATCH/startup candidate plus pending approved enum producer | cmd/fleetmetadatacache/main.go; pkg/fleet/registry/{owned_watch,eaigw_registry,kuadrant_registry,delete}.go; deleted registry/probe.go; pkg/fleet/readiness/probers.go; pending typed handler/cache/shared-scope changes |
| AF | Shared registry/readiness and pending enum consumers; old image cannot qualify them | shared registry/readiness closure above; pending pkg/shared/scope, pkg/fleet scope factory/adapters, AF tool/policyread changes |
| mock-LLM | New request-local 004 external LLM fixture | test/services/mock-llm/scenarios/{scenario_af_existing_rr.go,registry_default.go}; existing provider handlers unchanged |

For the current **pre-merge AF-only** source, reuse unchanged KA and DataStorage
artifacts and the unchanged migration inputs; do not silently build them. Historical
CI image config hashes from `../2593-af-amendment-evidence/final-live-state.json`:
AF `11c1b2407f294413226d38fff991ef55e5cd79690cda30388d537a01592d6fb9`;
FMC `50d96acfdc1bfddebbd775625d201101a6bac7af68c6867591fe20d2b76839a5`;
mock-LLM `34b512f287254c0c24a559d5c4b1085301087c7a378fd7da37ebae17cf66a67b`;
KA `022d17599d81290574a0313c4154c9d18506e48e1bb148cc56701a51fad17700`;
DS `e2ac222e3377f6275b38bc2358bfb8fa88e6675b8ff0815385b14f6c60238bf7`.
Those config identities are not the distinct imported manifest digests; both
forms and dependency pod records are retained in that JSON. No fresh inspection
or dependency migration execution was performed. Capture migration-job inputs/
image digest and must-gather artifacts at the later authorized qualification;
do not invent a migration identity from the DataStorage pod hash.

Operator artifact is provenance only in this raw-manifest AF lane, not deployed:
source `59e99fc9ffde0700251059bd77e1ac071b544f0b`, embedded core
`8c36d5628d8865f78c7dba14f6051685954539ea`, amd64 archive SHA-256
`9bd454780f3a79138eacd70b79720ad237675caa7b8db59b5742dec73c360c32`;
arm64 `d277ac08ae9a83da7eb2126fd7f945076c2e28a6fa9f32835a63e6243a3223b9`.
References: prior operator-boundary-provenance.json and ci-operator-provenance.json
under `../2593-fmc-private-diagnostic`. No operator image build is requested here.
EM/SP images enter scope only if their changed WATCH deployment path is actually
exercised. GW/RO images belong to later completed atomic-consumer qualification,
not this AF-only exception. No new build exception has been exercised.

The exact-artifact request must therefore be refreshed, not granted against
unspecified future source: finish the approved atomic work, resolve conflicts,
record the final source digest and per-service closure, request explicit bounded
functional build permission, then capture build flags, source/module/dependency
identities, binaries, archives and image config/manifest digests. Run first-failing
and full affected suites on that exact resolved candidate before any push.

#### Actual conflicts and safe reconciliation plan (read-only inspection)

Live GitHub reports PR #2637 OPEN, CONFLICTING / DIRTY. Head is exactly
`2cda8b1ecfdfe51c4ac8e00ce0c1136dd84eaa7d` on work/2593-fmc-presence.
The PR response's cached base SHA is
`741f7d8569fef9a5eed2d374c759b42298c0a6b8`; querying refs/heads/main directly
gives **`ed19d855c30423abbcf181efbb3d4974c9f9a871`**. The cached base is already
an ancestor and would falsely produce a no-conflict simulation. The actual
current-main object was fetched without changing branches/index/worktree. A
non-checkout `git merge-tree --write-tree --name-only HEAD <actual-main>` returns
one content conflict: **`.github/workflows/ci-pipeline.yml`**, operator native
cross-compile preparation around lines 778–786 in the simulated tree. No conflict
markers were written to the working tree. Raw output is
`../2593-004-current-main-merge-tree.txt` (simulation tree 8aec1e54839baf653a6436e2454e49fe7074a4d1).

Incoming changes since the merge base affect exactly workflow YAML, go.mod and
go.sum. The workflow conflict is our tag-preserving single-quoted regex versus
main's later #2648 support for tag **or digest-only** builders and corrected
literal BUILDPLATFORM handling. The recommended normal-merge resolution preserves
main's reviewed `[:@]` separator support and literal
variable behavior, plus this branch's preservation/negative fixture coverage.
Review/run the preparation-step tests for both tag/digest forms and unexpected
builders; never resolve by reverting to fixed 10.2, removing guards, or changing
operator artifact authority. No resolution was applied in this phase.

Main also updates kubernetes-mcp-server 0.1.2→0.1.3, OPA 1.22.3→1.22.4, goose
3.30.0→3.30.1 and indirect interpolate 0.0.2→0.2.0. Consequently post-merge reuse
of KA/DS/migration artifacts requires a new dependency-closure comparison; the
current pre-merge unchanged-artifact assumption cannot automatically carry
forward. Request explicit scope expansion if a dependency actually changes a
needed runtime artifact, rather than building extra services implicitly.

Safest next phase: leave all current dirty files intact; preserve/complete and
review the atomic candidate, obtain permission for its local commit, then refresh
main/head and perform an approved normal merge from a clean candidate tree. A
separate conflict-resolution agent should own that phase to avoid concurrent
edits. Review the real conflict/blast radius again. Re-inventory and rerun build,
lint, required UT/IT/E2E and helios08 first-failing/affected suites **after** the
resolution before push. No stash/reset/discard, rebase/force-push, PR merge or
public boolean compatibility resurrection is authorized. A new design/intent
conflict below 90% confidence must be escalated before editing.

### Atomic completion and normal-main reconciliation (2026-11-16; active)

This phase is explicitly authorized to complete the approved source, make local
candidate commits, and reconcile actual main with a normal merge from a clean
tree. It does not authorize publication, image builds or remote cluster changes.
No remote command has been run. The retained helios08 cluster is untouched.

Initial HEAD was `2cda8b1ecfdfe51c4ac8e00ce0c1136dd84eaa7d`; fetched actual main is
`ed19d855c30423abbcf181efbb3d4974c9f9a871`. All 28 incoming dirty paths (including
deletion and untracked files) were preserved in
`../2593-atomic-completion-evidence/predecessor-source.json`, `predecessor.patch`
and `predecessor-files/`. Every path belongs to the explicitly described WATCH,
enum, AF amendment or supporting evidence work; no unknown user work was found.
`004-preservation.json` verifies all **7/7** 004 code paths byte-identical.

Engram methodology/CORPUS recall, workspace semantic search, exact scope/checker,
factory/command/type definitions and Check references located the approved
contracts and production callers. Intermittent Engram 403/503 responses were not
treated as negative evidence; bounded source reads were used when necessary.
Authority remains DD-FLEET-015, TEST_PLAN v1.8, BR-SCOPE-001/002/010 and
BR-INTEGRATION-065. No new architecture choice or operator/CRD change is made.

Local source changes complete enum-only Check across Manager, cache, FMC HTTP,
ACM, federated factory, GW, RO and AF. State/provenance are the only wire fields;
legacy bool interfaces, adapters, Managed field, ScopeCheckResponse and existence
fallback are removed. Errors win over results. Missing observations cannot
inherit; namespace false retains namespace-label provenance. Fleet attribution,
unknown registry, strict GVK, bounded transient-only retry, status/media/body and
single-object validation are fail-closed. AF distinguishes unavailable from
unmanaged through the outer create-RR error as well as its guidance; only verified
unmanaged becomes AF exact NotFound. DD-AF-018 remains closed. Existing fencing,
begin-before-collection repair and ambiguous-write protections are unchanged.

New local TDD evidence in `../2593-atomic-completion-evidence`:

- `enum-red.log`: original production, local **0 pass/3 fail**; strict-wire
  **1 pass/11 fail** (seed 1794887911). Failures are unsafe missing evidence,
  public diagnostics/boolean and accepted malformed/legacy wire, not compile RED.
- `fmc-it-premerge.log`: **25 pass/1 fail**, 26 selected/36, seed 1794889037.
  Safety and guarded clean recovery pass; RED identifies the outer AF error prefix
  still saying not managed. `fmc-it-premerge-second.log` passes all 26 selected
  after the actual error-boundary fix, with real envtest 1.37.0 and Valkey/race.
  Ten cases are focus exclusions, not additional passes. A later namespace-false
  assertion must be executed again after reconciliation.
- `acm-label-red.log`: two malformed managed-label projections incorrectly inherit;
  `acm-label-green.log` passes the complete ACM package with race after strict
  managed-key parsing. Broader added assertions are regression characterization,
  not retroactive claims that each was a separately reproduced defect.
- `build-candidate.log` and `race-candidate.log`: full build plus all selected
  shared-scope/Fleet/GW/RO-routing/AF-policyread and AF/RO/FMC command, ACM factory,
  infrastructure and serving-helper packages pass with race. Command tests invoke
  real factories/HTTP and the real RO constructor and AF action/inventory paths.
  No full real-Kind or remote journey is implied by these local wiring tests.
- `af-scope-race-premerge.log`: focused scope refusal and IT-AF-2094-001 pass with
  race. The full AF package's earlier 790/791 run had an unchanged 30ms watcher
  observation failure under concurrent compilers; an earlier full run passed
  791/791. The separate MCP TLS timing failure was also in a source-unchanged
  component. These failures remain recorded, not suppressed or called fixes.

Concurrent build/compile wrappers hit 600s harness deadlines and one linter reached
its five-minute ceiling. Standalone full build/repository compile and full lint
then passed (`build-serial-premerge.log`, `compile-serial-premerge.log`,
`lint-serial-premerge.log`, zero lint issues). Validation will be serialized after
merge. No timeout, assertion, business guard or existing test is weakened.

Refactor content: local presence lookup uses early returns rather than fallback
and nested conditions; the strict codec shares bounded per-member validation with
serving-path helpers; label projection parsing rejects malformed/duplicate opt-in;
the AF refusal error boundary no longer conflates domain outcomes. Targeted nilerr
annotations document the explicitly required invalid-domain/nil-error contract;
they do not ignore an execution error. Post-refactor full build passes.

Current disposition: local full candidate ready for preservation commit/normal
merge; no final 26/26 or 11/11 numerator, post-resolution pass or artifact freeze
is claimed yet. Remote AF amendment remains **0/4**. Original exclusive 002/004 RCA
remains 85%; nine later same-request repairing refusals remain 98%. The consumed
logging-only image exception is not reused for functional builds.

#### Normal merge and final acceptance-boundary completion (2026-11-17)

Atomic local candidate commit is `552e47dd0566f8edad71fdd7310e4beee2bdde6c`.
Normal merge commit is `77b6b55bbfac504ceee194e2c6d693b85157d37c`, with actual
main parent `ed19d855c30423abbcf181efbb3d4974c9f9a871`. Both refs were refreshed
from origin immediately before the clean-tree merge; the published topic head
remains `2cda8b1ec`. Only workflow YAML conflicted. The resolution combines the
source-preserving single-quoted shell expression with main's `[:@]` tag/digest
support and literal `[$]BUILDPLATFORM` guard. It preserves both intentions rather
than choosing ours/theirs. The actual preparation step is executed with
`BUILDPLATFORM=must-not-expand`; all five old/current/tag+digest/digest-only/
unexpected-builder cases pass with race. `merge-workflow-red.log` first proves
digest-only failure against the candidate parent's colon-only expression;
`merge-workflow-green.log` proves the combined result. No image build is run.

The first complete **post-merge** serial validation round passes all nine gates
at merge source `77b6b55bb` (`postmerge-results.json`): full build, repository
test compilation, full lint (zero issues), affected race suites, full AF tools
and mock-provider race suites, EM/SP command race suites, selected real FMC IT,
`make test TEST_SUITE_PROCS=1`, and whitespace. Suite serialization changes no
assertion, timeout, test set or coverage gate. `make test` takes 685.473 seconds;
the real FMC IT takes 65.685 seconds. Earlier timing-sensitive AF/MCP failures
do not recur in this serial round without production changes to those paths.

Final scope-boundary review then reproduced **two actual remaining approved
acceptance gaps** against that merge source, not new architecture or extra
denominator credit. `final-boundary-red.log`, seed 1794892851, records two REDs:
direct FMC Check accepted missing explicit GVK from a successful backend; an
already-queued cross-namespace WATCH event allowed initial publication. GREEN
validates required remote GVK/identity before HTTP dispatch and validates WATCH
namespace before both pending-event publication and live event dispatch. Namespace
metadata is required on namespaced LIST/WATCH objects. Valid transport-fixture
requests now supply explicit GVK. The complete FMC/registry race packages and
full build pass (`final-boundary-green-final.log`, `build-final-boundary.log`).
A fresh serial gate round is required for this final post-merge source; the
earlier nine passes are retained under their original SHA, not relabeled.

Read-only local CI artifact inspection revalidates the original five application
archive/config hashes and extracts ELF build metadata without executing any
application binary (`original-ci-binary-inventory.json`, buildinfo and nm files).
Their compiler is Red Hat Go 1.27.3; host validation is Go 1.26.11. Source/module
closure inspection is provisional until final freeze. Incoming main's four module
updates do not automatically imply changes in every runtime binary. The original
db-migrate archive is independently reverified: its actual goose binary is
**v3.27.2**, built with Go 1.26.11 and interpolate v0.0.2, matching the unchanged
recipe's isolated explicit pin, not main go.mod's goose v3.30.1. This is artifact
input/provenance evidence, not migration execution or a new-image permission.

## Authority and disposition (historical bounded repair)

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
