# PR #2493 — Remaining CI Remediation: Execution Record

**Date:** 2026-10-11 UTC

**Plan:** [TP-2493-CI-v2.0](REMAINING_CI_FAILURES_PLAN.md)

**Requirements:** BR-SECURITY-2493; BR-PLATFORM-001.6a

**Starting published head:** `c659819f3f3d402c560583bf2779b678292c3060`

**Status:** Source and regression gates verified; focused publication requested.
The user explicitly requested "push them and monitor pipeline" before remaining
local image checks completed. This record accompanies the requested batched
publication; exact new-SHA CI results remain a separate acceptance gate.

## Delivered locally

- Root Go **1.26.9**, selected `golang.org/x/net` **v0.60.0**, all **16**
  active workflow Go selections, and an explicit Go 1.26.9 floor for the
  isolated goose build. No other selected dependency or UBI digest changed.
- Guarded Trivy output capture under inherited Bash `-e`: preserve original
  stdout/stderr and exit status, fail closed on findings/configuration/command
  failures, and retain existing bounded transient-DB retries and backoff.
- External curl fixtures now implement endpoint routing, response output files,
  HTTP write-out and nonzero network errors. Assertions retain explicit collector
  failure, secure authorities, and valid error evidence rather than weakening
  the production collector to return success.
- The actual Must-Gather workflow compares source/image collector names and
  checks executability, including `fleet-service-path.sh`, instead of expecting
  a stale count of ten. A real gather test proves sanitized partial archives
  survive DataStorage failure while other collected logs remain available.

## TDD and regression evidence

Evidence is retained in `../kubernaut-pr-2493-rca/`.

| Check | Result / evidence |
|---|---|
| RED: initial compiler/action/inventory contracts | 9 expected failures / 1 pass; `remaining-ci-contract-red-v20.{json,log}` |
| RED: curl output/write-out contract | Expected failure; `remaining-ci-curl-contract-red-v20.{json,log}` |
| RED: existing DataStorage Bats cases | 9 failures / 11 cases; `remaining-ci-datastorage-bats-red-v20.log` |
| GREEN/REFACTOR: infrastructure race suite | **292/292**, including **12** new contracts; `remaining-ci-infrastructure-final-v21.{json,log}` |
| Real TLS/token/error/privacy collector proofs | Existing IT-AF-2501-002/003/004 pass within that unfiltered infrastructure suite |
| Full production `make test` | **11,281 passing specs / 114 suites**; `remaining-ci-unit-make-warm-v21.log` |
| Native arm64 containerized shell suite after refactor | **73/73**, including IT-MG-2493-002 partial archive; `remaining-ci-must-gather-container-final-arm64-v21.log` |
| Source vulnerability gate | CI-pinned govulncheck **v1.3.0**, exit **0**; `remaining-ci-govulncheck-gate-green-v20.log` |
| Raw source scan reconciliation | None of the **11** recorded blockers remains; only previously accepted GO-2023-1901 is callable; `remaining-ci-govulncheck-analysis-v21.json` |
| Build after refactor | `go build ./...` passes; `remaining-ci-build-refactor-v21.log` |
| All-suite compilation after refactor | `go test ./... -run=^$ -timeout=30s` passes; `remaining-ci-all-suites-compile-refactor-v21.log` |
| Exact CI lint scope | **0 issues**; `remaining-ci-lint-ci-scope-v21.log` |
| Completed full lint | Exactly **22** pre-existing docs/hack/script diagnostics, **0 new**; `remaining-ci-lint-completed-comparison-v21.json` |
| Workflow validation | actionlint passes; `remaining-ci-actionlint-refactor-v21.log` |
| Shellcheck baseline comparison | Same three pre-existing helper findings, **0 new**; `remaining-ci-shellcheck-comparison-v21.json` |
| Actual cross-build paths | All **12 services × 2 architectures** build; `remaining-ci-cross-build-results-v20.json` |
| Actual cross-built binary metadata | Go 1.26.9 and linked x/net v0.60.0, **24 binaries**; `remaining-ci-service-binary-metadata-v21.json` |
| Scoped cross-built binary scan | **24 detected Go targets**, zero fixable HIGH/CRITICAL; `remaining-ci-service-binaries-scoped-v21.json` |
| Real retained vulnerable-image action control | Exit **1**, visible CVE/stdout/stderr report, one attempt; `remaining-ci-trivy-wrapper-real-control-v20.json` |
| Actual arm64 image inventory run block | Pass, required names/executable files; `remaining-ci-real-inventory-arm64-v21.log` |
| Actual arm64 delivered scripts | Gather + all **11** collectors match source SHA-256, including DataStorage/Fleet; `remaining-ci-real-collector-delivery-arm64-v21.json` |
| Actual arm64 Must-Gather scan | Exit **0**, detected kubectl, zero unaccepted HIGH/CRITICAL and zero secrets; `remaining-ci-must-gather-artifact-arm64-v21.json` |

Qualifications: the initial cold concurrent unit run timed out; its failure is
retained, not deleted or reclassified. The unchanged production Makefile passed
with warm caches and one suite at a time, without increasing test timeouts.
The full unit result includes **two existing kubeconfig-dependent skips** in
AIAnalysis nil-safety specs; no new skip or pending test was added. The first
broad binary-directory scan also detected old local Ginkgo development executables;
the scoped artifact scan explicitly excludes those non-runtime tools and proves
coverage of every one of the 24 expected application binaries.

REFACTOR content: split the new contracts by responsibility to support focused
commits; remove duplicate Bats fixture setup, a stale empty heading and an unused
mock parameter local; preserve clearer original-status/backoff diagnostics.
Plain compiler/module/version selections are **N/A** for refactor: no logic to
improve. Build, compilation and regression results above prove preservation.

## Actual image verification and capacity

The local Podman VM exhausted build disk space during a certified APIFrontend
build. This failed attempt is retained in
`remaining-ci-af-certified-build-arm64-v20.log`. No unrelated image/container was
deleted to make room.

The user separately approved temporary Helios08 `kernel.keys.maxkeys=512`,
restoring **200 even on failure**, with `maxbytes=20000` and unrelated workloads
unchanged. A fresh task-owned directory
`/home/kubernaut-e2e-af/pr2493-remediation-20261011-v21` contains the transferred
snapshot. All **5,914** recorded source/binary hashes were verified before starting
the guarded, serial build runner. This does not overwrite retained sources/images
or the independently advancing #2496 checkout.

The runner built and saved **30/32** planned images, verifying actual compiler,
linked-module and both certified AF metadata contracts for those artifacts.
On the user's immediate-publication instruction, only this task-owned runner
and its verified child build were interrupted (exit **143**). The final two local
images and native amd64 shell-suite run did not complete; cancellation is not
an artifact security PASS or a diagnosed security failure.

`remaining-ci-publication-quota-restoration-v22.json` confirms the runner exited,
`maxkeys=200` was restored, and `maxbytes=20000` stayed unchanged before publication.
Partial build/metadata logs remain in `helios-artifacts-v21/`; remote image
archives and prior evidence were retained, not deleted.

Image acceptance remains outstanding until actual image Go-binary/secret scans,
the newly clean real-action control, native amd64 shell suite and exact amd64
workflow inventory step pass. Declared versions, metadata and the binary-only
scan are not substitutes. The explicitly requested new-SHA pipeline will assess
these remaining gates; no incomplete or skipped check is labelled GREEN.

Trivy **v0.72.0** uses the owned cache `remaining-ci-trivy-cache-v20`, with DB
updated **2026-10-11T07:05:14.103977313Z**. Controlled comparisons use that DB
with `--skip-db-update`, HIGH/CRITICAL, `--ignore-unfixed` and exit policy **1**.
Default secret detection is retained for actual image scans.

## Preserved policy and limitations

- `.govulncheck-ignore.yaml`, `.gitleaks.toml`, the 16-entry Must-Gather exception
  file, diagnostic Dockerfile, real gather/collector scripts and certified AF
  Dockerfile remain byte-identical to the starting published head. See
  `remaining-ci-preserved-production-contracts-v21.json`.
- Kubectl **v1.36.2** findings are still **accepted risk, not patched flaws**,
  expiring **2027-04-11**; issue #1662 remains open. No application exception was
  added and the archive-checksum allowance remains exact-rule/path/value only.
- No auth/RBAC/replay, audit ordering/correlation, verified TLS, Fleet readiness
  cadence/gate or diagnostic privacy contract was weakened. No new audit event
  or control mapping is introduced.
- SOC2 **CC8.1**: explicit approvals, failing-before/passing-after evidence and
  focused publication. FedRAMP **SI-2/SA-11**: actual patch and proving scans;
  **SC-8/SC-13**: preserve verified transport and certified cryptography.
- Historical #2501 event/query/row evidence, failure-time #2502 routing and the
  original 24 hidden scan reports remain unavailable. New controlled reproductions
  cannot retroactively recover those incident causes.
- A normal batched push and completed **new-SHA** CI cycle, including previously
  gated integration/E2E, are still required. This checkpoint is **not merge-ready**.

**Confidence: 95%** in the scoped source/fixture/wrapper changes, justified by
RED contracts, full affected regressions and observable real negative controls.
Remaining risks: actual image security gates and remote journey execution.

## Requested publication

The scoped code changes are assembled as:

- `ff0efd24c`: patched root/workflow/isolated compiler and selected x/net.
- `057ee7aa8`: observable fail-closed Trivy capture and actual-action contracts.
- `2415b7856`: faithful curl/error fixtures, partial archives and collector inventory.

The final pre-publication focused contracts pass **12/12**
(`remaining-ci-prepush-contracts-v22.{json,log}`). Publication uses a single normal
push to the existing PR branch, preserving merge `f492991e5` and both closing
references. The next pipeline result must be attributed to the resulting new SHA,
not the still-failing `c659819f` cycle. No local cancellation or absent image scan
is reclassified as a successful CI result.

## Published-head CI result and scoped Fleet follow-up

The normal batched publication reached
`2dafdc9ce4f714604cf690bac0a9fd9aaf268f98`. CI run **38144393386** completed with
**76 successful jobs, 3 failed, and 0 skipped**. Fleet E2E is the only underlying
failure (**46/47** specs); Test Suite Summary and Merge Gate are cascading failures.
All other integration/E2E lanes and all scan steps across the 28 scan-bearing jobs
passed. Same-head Must-Gather **38144393373**, Gitleaks **38144393370** and CodeQL
**38144393368** also passed. Native amd64 Must-Gather proved **73/73** cases,
including the partial archive and exact executable collector inventory. These
new-SHA results close the earlier CI artifact gates; the interrupted local
30/32-image runner remains partial evidence, not a retrospectively complete run.

The remaining Fleet failure and approved test-only correction are documented in
[FLEET_ORGANIC_ALERT_PLAN.md](FLEET_ORGANIC_ALERT_PLAN.md). Its observed sequence
is an organic alert emitted before fixture scope readiness is established,
followed by two unmanaged-resource rejections and no repeat notification during
the unchanged RR assertion. The exact historical FMC cache transition remains
unavailable. A real authenticated, CA-verified FMC precondition must pass before
the test injects its metric; no direct Gateway POST or production scope change.

At this checkpoint the correction passes its full 307-spec infrastructure race
suite, build, compilation, CI-scoped lint, focused real organic Fleet journey
and complete **47/47** real Fleet regression (**0 failed/pending/skipped**).
Both full-run production Must-Gather collections and final runner cleanup pass.
All 15 verified amd64 images were reused, not rebuilt; only the race-enabled
Fleet test binary was recompiled. The original and fresh full-run nodes are
retained stopped, unrelated workloads remain unchanged, and the authorized
`maxkeys=512` / unchanged `maxbytes=20000` are verified.

Evidence: `fleet-organic-full-real-v30.{json,log}`,
`fleet-organic-final-verification-v30.json` and
`fleet-organic-final-original-images-v30.json`. The actual organic dispatch proves
the test-harness wiring manifest; source-order assertions alone are not the proof.
The new-head publication/CI gate remains pending, so this is **not yet merge-ready**.
Existing `Closes #2501` / `Closes #2502` references and fast-forward history are
preserved. **Confidence: 98%** in the scoped correction, based on the above real
journeys, full regression and exact-source checks; CI-environment validation of
the new head remains outstanding.
