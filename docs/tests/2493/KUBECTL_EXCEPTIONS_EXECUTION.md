# PR #2493: Six-Month Kubectl Exceptions — Execution Record

**Date**: 2026-10-11 UTC
**Requirement**: BR-SECURITY-2493; BR-PLATFORM-001
**Plan**: [TP-2493-SEC-v1.1](SECURITY_REMEDIATION_PLAN.md)
**Decision**: [DD-PLATFORM-003 amendment](../../architecture/decisions/DD-PLATFORM-003-must-gather-kubectl-version-policy.md#2026-10-11-amendment-explicit-six-month-risk-acceptance)
**Status**: Local exception step verified; publication requested, CI outcome tracked in PR #2493
**Starting head**: `fd0e07825b98077d6196901640f443fdbbe40ef5`

## Approved outcome

The user explicitly requested six-month kubectl CVE exceptions. Renewed the
thirteen expired entries and added CVE-2026-78667, CVE-2026-78669 and
CVE-2026-97031 in `cmd/must-gather/.trivyignore`. Exactly **sixteen IDs** now
expire on **2027-04-11**, six calendar months from the UTC approval date.

The installed released kubectl remains **v1.36.2**. This is **accepted risk,
not vulnerability remediation**: nineteen HIGH package findings remain in that
binary. Newer published v1.36.5/v1.37.1 fix thirteen IDs but retain the three
new HIGH IDs. The comments and decision record now acknowledge that distinction
instead of falsely claiming that no published fixes exist.

Owner: Kubernaut Platform Team. **Issue #1662 remains a re-triage obligation**;
exceptions must be removed when resolved, and renewal requires fresh approval.

## TDD and verification

| Check | Result |
|---|---|
| RED: UT-SEC-2493-007/008/009 | Seed `1791688000`: two expected failures (old expiry and missing risk-acceptance record), two existing-routing passes; other specs filtered, none pending |
| RED: IT-SEC-2493-003 image scan | Same retained must-gather image, expired ignore file: exit **1**, **19 HIGH findings / 16 IDs**, detected `usr/local/bin/kubectl` |
| GREEN: all infrastructure regressions | `go test -race ./test/infrastructure`, seed `1791688600`: **276/276 passed**, zero failed/pending/filtered |
| GREEN: IT-SEC-2493-003 image scan | Same image and DB, approved ignore file: exit **0**, zero unaccepted findings, positive kubectl binary-analysis coverage, zero secrets |
| Application-image negative control | Retained APIFrontend image, **without** the must-gather ignore file: exit **1**, CVE-2026-78669 remains visible |
| Production configuration routing | UT-SEC-2493-009 proves CI infrastructure and both release-architecture inputs remain conditional on **must-gather only** |
| Build | `GOTOOLCHAIN=go1.26.6 go build ./...`: PASS |
| Affected-package lint | `golangci-lint run --timeout=5m ./test/infrastructure/...`: **0 issues** |
| Formatting / whitespace | New Go test is gofmt-clean; `git diff --check`: PASS |
| REFACTOR | **N/A**: plain exception configuration and documentation; no new production logic to improve |

The Go1.26.6 selection above intentionally validates this exception-only step
on the unchanged application toolchain. It is not an application vulnerability
fix. Previously reproduced full-repository lint debt remains separate from
the clean affected-package result.

## Scan provenance and limitations

- Trivy **v0.72.0**, explicit owned cache with DB updated
  **2026-10-11T01:13:45.784565337Z**. Before/after use the same cached DB with
  `--skip-db-update` for a controlled comparison.
- Severity **CRITICAL,HIGH**, `--ignore-unfixed`, exit policy **1**, and scanners
  are unchanged. Only the requested enumerated exceptions change filtering.
- Retained **amd64** image archive checksums (SHA-256), re-verified against
  their original records:

  | Image | Archive SHA-256 |
  |---|---|
  | must-gather | `83434d9a9ab1c0ce80d2ec86df9b6991e50205b3e995c4560c909619989b88a0` |
  | APIFrontend negative control | `48145738263e06f371db6c1ac0557aae6a9d7bb1a209cf0364711e519a2f9c4b` |

- These are retained pre-rebase verification images, **not newly rebuilt
  images or claimed exact historical CI images**. The exception-aware scan
  does not recover hidden historical CI scanner output.
- Both architecture workflow inputs are covered by routing contracts; a new
  arm64 artifact scan or collector compatibility matrix is **not claimed**.
  No delivered binary, collector, Dockerfile or runtime behavior changed.
- APIFrontend's existing Red Hat Go-version parse warning remains: its visible
  x/net finding is a negative control, not complete stdlib scan evidence.
- The scanner-version record distinguishes its unrelated default-cache display
  from the explicitly selected scan-cache metadata; the old default DB is
  **not** the DB used for these scans.

## Preserved scope and compliance

`go.mod`, `go.sum`, `cmd/must-gather/Dockerfile`, `.govulncheck-ignore.yaml`,
and the Trivy action implementation are byte-identical to the starting head.
No new application exception, kubectl source builder/upgrade, host setting,
container launch, retained-image replacement, or #2496 write was performed.
At completion of this local verification, no commit, push, or new CI run had
occurred. The user subsequently requested logical commits, publication and CI
monitoring; those delivery results must be distinguished from local GREEN.

SOC2 CC8.1: approval and RED/GREEN evidence are recorded for this configuration
change. FedRAMP SI-2: the accepted kubectl flaws **remain unremediated**; do not
mark them resolved merely because filtering passes. TLS/FIPS, RBAC, replay,
audit event/control mappings and Fleet readiness behavior are unchanged.

The eleven actionable application Go advisories and the scanner wrapper's
inherited-errexit observability defect remain separate draft remediation work.
Overall PR merge readiness and remote integration/E2E GREEN are not established.

## Evidence

Owned RCA directory: `../kubernaut-pr-2493-rca/`:
- `kubectl-exceptions-red-v15.{json,log}`
- `kubectl-exceptions-image-red-record-v15.json`
- `kubectl-exceptions-infrastructure-green-v15.{json,log}`
- `kubectl-exceptions-scan-verification-v15.json`
- `kubectl-exceptions-verification-v15.json`
- `kubectl-exceptions-{build,lint}-v15.log`

**Confidence: 98%** in the exact CVE scope, calendar expiry, routing and local
exception-aware scan behavior, justified by failing-before/passing-after BDD
and actual same-image scans. This is not confidence that the CVEs are fixed or
that the overall PR is merge-ready.
