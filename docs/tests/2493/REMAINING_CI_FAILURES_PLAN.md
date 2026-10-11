# TP-2493-CI-v2.0 — Remaining CI Failures

**Date:** 2026-10-11
**Status:** Approved 2026-10-11; source/regression checks passed; publication requested
**Approval:** User selected "Approve plan (Recommended)" for the focused scope,
real artifact validation, batched publication and unchanged exception/host limits.
**Additional capacity approval:** After the local VM exhausted build disk space,
the user selected "Use Helios08 (Recommended)": temporarily set only
`kernel.keys.maxkeys=512`, restore `200` even on failure, retain `maxbytes=20000`
and all unrelated workloads/source/evidence. This supersedes the no-host-limit-
change default only for the explicitly approved verification window.
**Publication sequencing update:** The user subsequently requested "push them
and monitor pipeline" before the remaining local image checks completed. Publish
the verified source/fixture/wrapper changes in focused commits and one normal
push; use the exact new-SHA CI cycle to assess the outstanding image/journey
gates. Stop only the owned verification runner and restore the temporary quota
before publication. Unfinished image checks are not claimed GREEN.
**PR:** [#2493](https://github.com/jordigilh/kubernaut/pull/2493)
**Baseline:** `c659819f3f3d402c560583bf2779b678292c3060`
**Failed run:** [38134600484](https://github.com/jordigilh/kubernaut/actions/runs/38134600484)
**Format:** IEEE 829 hybrid; AGENTS.md is authoritative
**Parent plan:** [TP-2493-SEC-v1.1](SECURITY_REMEDIATION_PLAN.md)

## 1. Objective and scope

Implement the outstanding application/toolchain and scanner-wrapper work from
the parent draft, and close verification gaps left in the approved #2501/#2502
collector follow-ups. The earlier kubectl risk acceptance and verified checksum
allowance were delivered; **the eleven application Go advisories were not fixed**.

Requirements: **BR-SECURITY-2493**, **BR-PLATFORM-001.6a**, and existing #2501/#2502
contracts. No new audit events or application authorization behavior are introduced.

In scope:
1. Root `go.mod`/`go.sum`: Go **1.26.9**, selected `golang.org/x/net` **v0.60.0**.
2. The **sixteen** current Go selections across CI, release, CodeQL, chart release
   and Helm smoke workflows. Update active pins, not historical evidence.
3. The isolated goose compiler floor in `docker/db-migrate.Dockerfile`; preserve
   its existing goose version, driver restrictions, and approved UBI bases.
4. `.github/actions/trivy-scan-with-retry/action.yml`: expose original scanner
   output/status under inherited Bash `-e`, preserving fail-closed classification
   and the existing bounded retry policy.
5. Must-gather test mocks/assertions in `cmd/must-gather/test/` and the container
   verification block in `.github/workflows/must-gather-tests.yml`.
6. Ginkgo/Gomega contracts, existing shell-suite regression coverage, actual
   build/artifact security verification, focused commits and one batched CI cycle.

Excluded: wider vulnerability/secret suppressions; kubectl upgrades/source builds;
new registries or scanner versions; auth/RBAC/TLS/FIPS changes; audit schema,
pagination or Fleet readiness changes; unrelated #2496 edits; automatic host
limit changes, broad cleanup, forced pushes or manual CI reruns.

## 2. Preflight and evidence

**Architecture impact:** moderate build/test wiring; no new architectural pattern
or production business component. Existing production entry points are retained.

| Finding | Evidence | Decision |
|---|---|---|
| Eleven actionable Go IDs remain | Run 38134600484's govulncheck gate; root Go 1.26.6 and x/net v0.58.0 | Patch the compiler/module, not the ignore list |
| Patch can preserve certified FIPS | Existing isolated APIFrontend artifact reports Go 1.26.9 and `GOFIPS140=v1.0.0-c2097c7c` | YES; re-prove on newly built artifacts |
| No extra x/net dependency upgrades required | Published v0.60.0 module requires the crypto/sys/term/text versions already selected by this root module | Restrict dependency changes to the identified target |
| Service and mock-LLM builders follow the root floor | Existing `GOTOOLCHAIN=auto`, root module copy and production build paths | Retain the existing mechanism; verify actual binaries |
| Goose does not follow the root module | Its Dockerfile initializes a separate module | Set an explicit patched compiler floor and verify output |
| All 24 service image jobs fail without scanner details | Logs show attempt 1 then exit; action assigns captured output before checking status | Guard that assignment, print output, preserve actual failure |
| DataStorage curl mock is obsolete | `helpers.bash` prints one body to stdout; production uses `--output` and `--write-out` with distinct endpoints | Update external mocks; do not weaken production collection |
| Error-success assertions contradict approved behavior | Collector returns nonzero with bounded JSON; `gather.sh` warns and continues | Assert explicit collector failure and successful partial whole-archive collection separately |
| Namespace assertions use an obsolete authority suffix | Current collector uses verified `https://data-storage-service.<ns>.svc:8080` | Assert current secure authority, not an unrelated URL rewrite |
| Container verifier expects 10 but source/image contain 11 | `fleet-service-path.sh` is the eleventh script | Verify exact source/image inventory plus executability |

The nine shell-spec failures and inventory mismatch also occur on pre-exception
head `f492991e5`; they were not introduced by the two security exception commits.
They nevertheless remain gaps in the overall PR and must be fixed.

The earlier draft's eighteen-workflow-selection claim is superseded by this
fresh **sixteen-selection** inventory at the stated baseline.

**Preflight confidence: 95%.** Existing build paths, FIPS viability, and test drift
are evidenced. Remaining risks are actual artifact scans and verification capacity;
the precise causes of the 24 hidden scanner failures are not assumed resolved.

Tooling safety: Engram memory recall was used. Its host semantic search fails,
and relative code tools resolve to the primary checkout rather than this worktree
(the newly delivered exception test is absent there). Use explicit worktree paths
for fallback reads/edits; do not edit the independently advancing primary checkout.

## 3. Recommended approach and alternatives

**Recommended:** targeted Go/x/net patch, guarded scanner capture, and faithful
external test fixtures plus inventory validation. Preserve all existing security
and diagnostic contracts. No new technology choice or dependency family is needed.

Rejected alternatives:
- New application exceptions or disabled scanner gates: conceal fixable flaws.
- Return success from the collector on audit failure: contradicts approved #2501
  observability/privacy tests; whole-gather partial success is already supported.
- Delete the new Fleet collector or simply ignore the inventory mismatch: loses
  the approved #2502 diagnostics. Exact names and executable files are stronger
  evidence than a replacement magic count alone.

If visible scans reveal another blocker outside these targets, retain its report
and propose the smallest evidence-backed correction. Do not guess at all image
failures or broaden suppressions to make the next run green.

## 4. Wiring manifest

| Changed contract | Production entry point | Wiring location | Passing proof required |
|---|---|---|---|
| Patched root compiler/dependency | Real service builds and CI source gate | `go.mod`; five workflow files; existing service/mock-LLM builders | IT-SEC-2493-001/002 |
| Patched isolated compiler | db-migrate image's goose build | `docker/db-migrate.Dockerfile`, goose-builder | IT-SEC-2493-002 |
| Observable fail-closed image scan | Existing CI/release matrix scan calls | `.github/actions/trivy-scan-with-retry/action.yml`, actual YAML run block | IT-SEC-2493-004 |
| Current DataStorage shell regression contract | Existing containerized must-gather tests invoking real collector | `cmd/must-gather/Makefile:test-container`; existing helpers/specs; real `datastorage.sh` | IT-MG-2493-001/002 and existing IT-AF-2501-002/003/004 |
| Required collector files are delivered | Container Build's production verification step | `.github/workflows/must-gather-tests.yml:Verify container contents`; built must-gather image | IT-MG-2493-003 |

CHECKPOINT W requires real entry-point execution for every row. Text assertions
or mocked image commands alone do not establish artifact delivery/security.

## 5. Test scenarios and expected outcomes

All new Go behavior tests use Ginkgo/Gomega and reference the requirements above.
Only external scanner/sleep/network/container dependencies may be faked. Run the
actual action YAML shell block under Bash `-e -o pipefail`, not a rewritten model.
Enhance the existing Bats suite for its shell regressions; do not create another
framework or skip/disable any failing case.

| ID | Tier | Contract |
|---|---|---|
| UT-SEC-2493-001 | UT | Root and all sixteen active workflow selections require patched Go |
| UT-SEC-2493-002 | UT | Selected x/net and isolated goose compiler floor are patched; certified AF settings retained |
| UT-SEC-2493-004 | UT | Genuine finding exposes stdout/stderr and original exit status immediately under inherited errexit; exactly one attempt |
| UT-SEC-2493-005 | UT | Transient DB failure then success retries within the existing bound; exhaustion prints the error and fails |
| UT-SEC-2493-006 | UT | Success passes; non-transient/config/command failures expose output and do not retry or suppress |
| UT-MG-2493-001 | UT | External curl fixture respects output file, HTTP write-out, endpoint routing and query recording |
| UT-MG-2493-002 | UT | DataStorage errors remain nonzero with valid bounded diagnostic JSON; success/empty results are structured and namespace-aware |
| UT-MG-2493-003 | UT | Production inventory verifier rejects a missing/non-executable required collector, including Fleet diagnostics |
| IT-SEC-2493-001 | IT | Exact CI-pinned govulncheck gate passes; all eleven recorded blockers disappear without new exceptions |
| IT-SEC-2493-002 | IT | Real service/fixture artifacts use patched compiler/modules on amd64/arm64; AF certified module preserved; real scans detect binaries and pass |
| IT-SEC-2493-004 | IT | Real retained failing scan is visible through the repaired action; newly built clean image passes through the same entry point |
| IT-MG-2493-001 | IT | Existing full containerized shell suite passes through its production Makefile target with corrected fixtures |
| IT-MG-2493-002 | IT | Real gather entry point continues after DataStorage failure, preserves sanitized error evidence and produces partial archive |
| IT-MG-2493-003 | IT | Exact workflow verification block passes on built image; actual required collector names/files are present and executable |

Keep existing UT-SEC-2493-007/008/009/010/011 and collector TLS/token/pagination
tests green. Preserve six-month CVE scope/expiry, default secret detection and both
negative controls; do not reinterpret previous exception-aware GREEN as remediation.

## 6. TDD sequence and estimates

1. **DISCOVERY/readiness (~15 min):** approve this scope, preserve source/runner
   ownership, and check all thirteen AGENTS.md readiness dimensions.
2. **RED (~30–45 min):** add/extend failing Ginkgo contracts for compiler/module
   selections, actual scanner script and inventory verification; reproduce the
   current shell-suite failures and source/artifact security failures with provenance.
3. **GREEN (~60–90 min plus artifact time):** make minimal compiler/module/pin,
   scanner capture and fixture/inventory changes; execute all wiring-manifest IT;
   run CHECKPOINT W. Scanner changes should reveal remaining artifact failures
   before claiming their cause or completion.
4. **REFACTOR (~15 min):** consolidate repeated external fixture setup and improve
   test diagnostics/cleanup if GREEN leaves duplication. Do not create new business
   components. Plain version pins are N/A; post-refactor build/test/lint are the
   safety net, not a substitute for a named code-quality improvement.
5. **Verification (~60–120 min, capacity dependent):** build, all-suite compilation,
   full unit regressions, full lint and baseline comparison, affected infrastructure
   and collector tests, actual multi-architecture artifact metadata/scans, wiring,
   compliance and anti-pattern checks. No pending tests or silent scan omissions.
6. **Delivery:** logical commits, recheck exact remote ancestry, one normal batched
    push after local gates; monitor that new SHA to completion. Do not substitute an
    older run or skipped integration/E2E lanes for verification.
    The explicit publication sequencing update above permits publishing after
    source/regression gates while local image acceptance remains incomplete;
    it does not waive image/journey acceptance or broaden security exceptions.

## 7. Environment, risks and rollback

- Go 1.26.9 is available locally; retain CI-pinned govulncheck 1.3.0 and Trivy
  0.72.0. Record scanner DB timestamp and require positive Go-binary coverage.
- Approved UBI builder/runtime image digests and native certified FIPS remain
  unchanged. No Red Hat-version parsing warning is accepted as proof of a clean
  standard library; inspect the actual produced compiler metadata.
- Existing local rootless Podman VM is reachable (arm64, six CPUs). Use serial,
  task-owned resources and unique tags. Retain prior image/scan evidence.
- Helios08 UID 1006 remains over its restored key quota: **243/200** keys,
  **15,370/20,000** bytes. Do not launch there on an assumed capacity allowance.
  If that runner is needed, separately request a temporary `maxkeys=512` change
  with restoration to 200 even on failure; `maxbytes` and unrelated workloads stay
  untouched. Prefer the available local runner when feasible.
- Do not delete unrelated containers, images, clusters, worktrees or historical
  evidence to make room. Pause for a capacity decision if owned execution is blocked.
- Baseline lint debt is distinct from the current failing govulncheck step. Re-run
  full lint after the toolchain patch and correct in-scope findings; escalate wider
  unrelated changes rather than silently sweeping independently advancing work.
- Security build failures, certified-module changes, unexpected dependency churn
  or newly visible out-of-scope scanner findings suspend that portion until assessed.
- Rollback is a focused revert of newly authored remediation commits only. Never
  reset unrelated work; a restored vulnerable artifact is not release acceptance.

## 8. Acceptance and compliance

- The exact source gate has zero new blocking advisories; actual runtime artifacts
  have zero unaccepted fixable HIGH/CRITICAL findings with detected binary targets.
- Both supported architectures preserve compiler/module/FIPS contracts; declaration
  changes alone are insufficient evidence.
- Full affected tests and all wiring-manifest rows pass; no new lint diagnostics.
- Real Trivy failures are visible and fail closed; transient retries stay bounded.
- Existing auth/RBAC/replay, verified TLS, audit ordering/correlation, Fleet probe
  cadence/readiness gate, and collector privacy/partial-diagnostic behavior survive.
- SOC2 **CC8.1**: explicit approval and RED/GREEN/REFACTOR/verification provenance.
  FedRAMP **SI-2/SA-11**: actual patching and proving security tests; **SC-8/SC-13**:
  preserve verified TLS and certified cryptography. No new audit event mapping.
- Accepted kubectl CVEs remain tracked risk, not repaired vulnerabilities. Only a
  completed pushed-SHA CI cycle, including previously gated journeys, can establish
  overall merge readiness; missing/skipped evidence is never labelled GREEN.

## 9. Deliverables

Focused application-security, scanner-observability and collector-verification
commits; reproducible failing/passing contracts; source/image identities, actual
binary metadata and scan reports; full affected regression/lint results; completed
wiring/compliance/readiness checklist; pushed-SHA CI report with remaining unknowns.

At plan creation no implementation or additional push had occurred. The user
subsequently approved the scoped plan; implementation results are recorded
separately from approval and the preceding exception-only CI cycle.
