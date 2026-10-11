# Test and Implementation Plan: PR #2493 Security Remediation

**Identifier**: TP-2493-SEC-v1.1
**Created**: 2026-10-11
**Status**: Kubectl exception step locally complete; remaining remediation plan is draft
**Delivery authorization**: User requested logical commits, push and CI monitoring on 2026-10-11
**Branch**: `fix/2493-ci-followups`
**Starting head**: `fd0e07825b98077d6196901640f443fdbbe40ef5`
**Base**: `origin/main` at `84c0af7859c802e2a2fd6a57933c4e1d52ccbfcf`
**Format**: IEEE 829-2008 hybrid; AGENTS.md overrides stale template coverage thresholds

## 1. Purpose, objectives, and measurable success

Remediate the actionable application Go/image findings blocking PR #2493 and
implement the user's explicitly approved six-month risk acceptance for the
current must-gather kubectl. Do not broaden exceptions to application artifacts,
change authentication, or lose diagnostic capability.

- Exact CI-pinned govulncheck gate passes; the eleven recorded actionable IDs
  disappear. Existing accepted GO-2023-1901 is not a new blocker.
- Actual affected application service/fixture images, on both supported
  architectures, have zero reportable fixable HIGH/CRITICAL findings; require
  positive binary-analysis coverage. Must-gather uses exactly sixteen enumerated
  kubectl exceptions expiring 2027-04-11, not a vulnerability-free claim.
- All new/changed build and scan wiring points have passing integration proof.
- Native certified APIFrontend FIPS configuration is preserved in produced
  binaries; no return to BoringCrypto or uncertified mode.
- Must-gather retains its collector/CLI/archive/privacy behavior, including
  compatibility with the documented 1.30/1.31/1.32 server range.
- All affected regression tests pass and no new lint diagnostics appear.

## 2. Authority and evidence

- [BR-SECURITY-2493](../../requirements/BR-SECURITY-2493-vulnerability-remediation.md)
- [BR-PLATFORM-001](../../requirements/BR-PLATFORM-001-must-gather-diagnostic-collection.md)
- [DD-PLATFORM-003](../../architecture/decisions/DD-PLATFORM-003-must-gather-kubectl-version-policy.md)
- [DD-AUTH-016](../../architecture/decisions/DD-AUTH-016-signed-user-identity-delegation.md)
- ADR-027/ADR-028 multi-architecture and approved base-image policies; AGENTS.md
- PR #2493, Issue #1662; already-approved #2499–#2502 plans remain in force
- Owned evidence: `../kubernaut-pr-2493-rca/rebased-origin-main-verification-v13.json`,
  `retained-images-trivy-recheck-summary-v13.json`, and
  `security-spike-v14/{released-kubectl-spike,patched-kubectl-spike}.json`

## 3. Preflight and resolved spikes

1. All five original PR patches rebased unchanged; new main changes only Fleet
   Helm infrastructure. Preserve their tested behavior and avoid unrelated cleanup.
2. Root Go/x/net declarations and **18** active CI/release Go selections are
   mapped. Existing service/mock-LLM builders use `GOTOOLCHAIN=auto` and the root
   module. The goose builder has an isolated module and needs its own floor.
3. Existing multi-stage goose build is the closest production pattern for an
   isolated, pinned diagnostic executable build. Existing Ginkgo infrastructure
   suites provide the contract/CLI-fixture test pattern.
4. **Go/FIPS spike — YES**: Go 1.26.9 builds actual APIFrontend with the existing
   certified module. No registry, cryptography architecture, or root module changed.
5. **Published kubectl bump alone — NO**: checksum-verified v1.36.5 and v1.37.1
   binaries each report four HIGH package findings / three CVEs with positive
   Go-binary analysis. Both use Go 1.26.8 and x/net older than v0.60.0.
6. **Patched same-minor source build — YES for build/scan viability**: published
   v0.36.5 modules + Go 1.26.9 + x/net v0.60.0 build with certified FIPS; real
   rootfs binary analysis passes without exceptions. Runtime compatibility remains
   a mandatory acceptance test, not an assumed property.
7. Engram memory recall was used. The workspace code-search backend currently
   fails; exact file/symbol searches and direct source evidence were used instead.
8. Helios08's verification UID 1006 currently has **243 charged keys against
   maxkeys=200**, with maxbytes=20000. New rootless containers may be blocked.
   No quota change or retained-cluster cleanup is implicitly authorized.

**Preflight confidence: 95%** in the mapped implementation paths, not the safety
of leaving the accepted vulnerabilities unpatched. The configuration-only
exception step uses existing routing and retained image archives, so it needs
no new containers or host-capacity change. Application/build-wrapper changes
remain subject to plan approval and any necessary runner-capacity decision.

## 4. Approved exception decision and rejected alternatives

The user selected: **"let's add kubectl CVEs as exceptions for 6 months validity"**.
Approved on **2026-10-11 UTC**, expiring **2027-04-11**. Amend DD-PLATFORM-003
to record this risk acceptance without rewriting its historical investigation.

Retain kubectl v1.36.2; renew the thirteen expired IDs and add only
CVE-2026-78667, CVE-2026-78669 and CVE-2026-97031. Published v1.36.5/v1.37.1
fix thirteen IDs; therefore renewing all sixteen is accepted risk, not proof
of unavailable fixes. Issue #1662 remains open for re-triage before expiry.

The patched source-build spike was viable but **not selected**; no new Go
module, Dockerfile builder, downstream artifact or DD-PLATFORM-2493 is introduced.
Upgrading to v1.36.5 with only three residual exceptions would reduce exposure,
but is not part of the current configuration-only request and would need its
own collector compatibility validation.

Removing kubectl is not a drop-in alternative: BR-PLATFORM-001.1 explicitly
requires it and the current shell collectors depend on its resource/log/event
operations. Rewriting those collectors is outside this focused security fix.

**Approved TDD scope**: add Ginkgo contract tests for exact CVEs, six-calendar-month
expiry, truthful risk documentation and must-gather-only routing; observe RED;
update the existing ignore configuration and stale comments; prove GREEN with
the same real retained image and an application-image negative control.
REFACTOR is N/A for this plain configuration change. Full affected infrastructure
regressions, build, lint and whitespace checks remain required validation.

## 5. Scope and invariants

**Approved now**:
- `cmd/must-gather/.trivyignore`: exactly sixteen IDs, all expiring 2027-04-11,
  with truthful owner/approval/risk/re-triage commentary.
- Existing CI and release comments, DD-PLATFORM-003 amendment, requirement and
  this plan: distinguish accepted risk from remediation; preserve routing.
- UT-SEC-2493-007/008/009 plus IT-SEC-2493-003 retained-image before/after scan
  and unexcepted application-image negative control. No image rebuild is needed
  for an external scanner-configuration change.
- The user additionally requested assessment/exception handling for review
  discussion_r4239946051 (Gitleaks alert #42). Recomputing the retained archive
  digest proves that finding is a public SHA-256, not a credential. Add only
  an exact-rule/path/digest AND allowance in `.gitleaks.toml`, with defaults
  retained and real-scanner negative controls. This is separate from CVE risk
  acceptance; never exempt the whole document or historical commit.

**Remaining draft remediation scope — not approved by the exception request**:
- `go.mod` / `go.sum`: Go 1.26.9 and the selected x/net v0.60.0; existing
  crypto/sys/term/text versions already satisfy x/net's published requirements.
- Five active workflow files: replace the eighteen Go 1.26.6 selections with
  1.26.9; preserve job dependencies, permissions, scanner versions, and policy.
- `docker/db-migrate.Dockerfile`: require the patched compiler for the isolated
  goose build; inspect output metadata rather than relying on the Red Hat suffix.
- `.github/actions/trivy-scan-with-retry/action.yml`: guard output/status capture
  against inherited `-e`, retaining fail-closed behavior and bounded retry.
- Ginkgo/Gomega contract, build/artifact integration and affected regression tests.

**Excluded**: unrelated #2496 code; historical RCA reconstruction claims; new
auth/RBAC/audit behavior; Fleet gate/network changes; blanket dependency updates;
exceptions beyond the sixteen accepted must-gather IDs; dropping server support;
new kubectl source builds or upgrades; arbitrary scanner upgrades;
unapproved host settings, image replacement, cleanup, force push, or manual CI reruns.

## 6. Risks, mitigation, and suspension

| Risk | Mitigation / proving scenarios |
|---|---|
| A declaration changes but a native/isolated/cross build keeps old Go | Actual binary metadata and real source/image gates: UT-001/002, IT-001/002/003 |
| Empty scans falsely look green, or Red Hat Go suffix prevents stdlib matching | Require detected Go binary targets; fail validation on version-parse warnings; IT-001/002/003 |
| Accepted kubectl vulnerabilities are mistaken for patched software | Explicit risk approval, expiry, owner, partial upstream fix evidence and Issue #1662 tracking; UT-007/008, IT-003 |
| Exceptions are broadened or leak to application scans | Exact CVE-set/expiry and workflow-routing contracts; real unexcepted application-image negative control; UT-007/009, IT-003 |
| Scanner wrapper silently exits or masks a real finding | Execute the production YAML run block under Bash `-e -o pipefail`; controlled external scanner responses; UT-004/005/006, IT-004 |
| Build graph drift in later application remediation | Minimal root dependency update and actual produced metadata; UT-002, IT-002 |
| Verification containers exhaust restored host quota | Explicit approval for temporary maxkeys=512 with restoration to200, or explicit owned-resource cleanup/alternate runner; never change maxbytes or unrelated workloads |
| Long CI feedback or independently advancing work overwritten | Locally verify first; unique source/image tags, hash manifests, preserve old images; one batched push after reassessing the exact remote head; no #2496 writes |

Suspend if compiler/certified module, image analysis, compatibility, or container
capacity fails. Capture actual errors and resolve or escalate; do not use XIt,
Skip, disabled gates, timeout-only fixes, or speculative stubs. Below90% confidence
in an unresolved implementation choice requires user escalation.

Rollback is an isolated revert of this new remediation commit only; never reset
unrelated work. Restoring a vulnerable artifact is a diagnostic rollback, not an
acceptable release. Restore any explicitly approved temporary host setting even
if verification fails, and retain failed-run evidence.

## 7. Wiring manifest and CHECKPOINT W

| Component / changed contract | Production entry point | Wiring location | Proving IT |
|---|---|---|---|
| Root patched compiler/modules | Service build and cross-build jobs | Root module; CI/release setup-go; existing service Dockerfiles / Makefile | IT-SEC-2493-001/002 |
| Isolated goose compiler floor | db-migrate image | `docker/db-migrate.Dockerfile` goose-builder stage | IT-SEC-2493-002 |
| Approved kubectl CVE exceptions | CI infrastructure scan and both release-architecture scans | Existing `trivyignores` conditional inputs in CI/release; `cmd/must-gather/.trivyignore` | UT-SEC-2493-009 + IT-SEC-2493-003 |
| Verified archive-checksum false positive | Existing Gitleaks history scan | `.github/workflows/gitleaks.yml` loads root `.gitleaks.toml`; existing fail-if-findings step unchanged | UT-SEC-2493-010/011 + IT-SEC-2493-005 |
| Visible, fail-closed scanner capture | Actual Build/service/infra image scan steps | `.github/actions/trivy-scan-with-retry/action.yml` scan run block; unchanged workflow uses | IT-SEC-2493-004 |

The approved exception step requires its routing and real image-scan row to pass.
The remaining rows belong to the draft application/build-wrapper work and are
not claimed GREEN. No test-only executable or source-only scan substitutes for
actual scanner analysis of the retained runtime binary.

## 8. Test scenarios and BR coverage

All scenarios map to **BR-SECURITY-2493**; kubectl/collection scenarios additionally
map to **BR-PLATFORM-001**. New behavior tests use Ginkgo/Gomega.

| ID | Tier | Observable contract / expected result |
|---|---|---|
| UT-SEC-2493-001 | UT | Root and active workflow build selections exclude vulnerable Go versions |
| UT-SEC-2493-002 | UT | Selected root x/net and independent compiler floors meet the patched contract; no reliance on go.sum's historical versions |
| UT-SEC-2493-004 | UT | Production Trivy block prints a genuine finding, exits nonzero immediately, and does not retry it under inherited errexit |
| UT-SEC-2493-005 | UT | Transient DB failures retry only within the existing bound; subsequent success passes; exhaustion prints the real error and fails |
| UT-SEC-2493-006 | UT | Successful scans pass and non-transient/config failures expose output without retries or suppression |
| UT-SEC-2493-007 | UT | Exactly the sixteen observed kubectl CVE IDs are accepted without duplicates/permanent entries; each expires six calendar months from the UTC approval date |
| UT-SEC-2493-008 | UT | Configuration records owner/approval/tracking and accepted risk, without claiming the CVEs are fixed or all published fixes are unavailable |
| UT-SEC-2493-009 | UT | CI and both release-architecture ignore-file inputs are conditional on must-gather only |
| UT-SEC-2493-010 | UT | Verified checksum allowance targets only generic-api-key and requires exact file AND extracted digest; defaults retained, no commit/stopword exemptions |
| UT-SEC-2493-011 | UT | Different values in the same file and the same value in other files are outside the allowance |
| IT-SEC-2493-001 | IT | Exact real CI govulncheck entry point passes after the root compiler/dependency change |
| IT-SEC-2493-002 | IT | Both supported-architecture service/fixture images use patched binaries and pass unchanged real image scans; AF certified FIPS metadata is preserved |
| IT-SEC-2493-003 | IT | Same retained image fails with expired exceptions and passes with the approved file, with detected kubectl binary coverage; unexcepted application image still fails on CVE-2026-78669 |
| IT-SEC-2493-004 | IT | Real scan report flows through the production action with correct visible exit status; workflow caller wiring remains intact |
| IT-SEC-2493-005 | IT | CI-pinned Gitleaks reproduces alert #42 on its original commit before the allowance and clears only that finding after; real synthetic-key/path negative controls still fail |

For UT-004/005/006, fake only external Trivy/sleep executables; run the actual
action's YAML script rather than a reimplementation. Every case uses owned
temporary paths and bounded contexts. IT/E2E use actual built binaries/images,
real scanners and real Kubernetes APIs. Missing tooling is a failed precondition,
not a pending or skipped test.

Unit business-logic coverage target is100%; integration coverage is100% of this
wiring manifest and applicable controls; E2E covers all in-scope control objectives.
Merged structural coverage follows the >=80% repository gate. Plain version-pin
and configuration changes do not invent line-coverage metrics. A new collector
compatibility/E2E matrix is N/A for the exception-only step: neither the delivered
CLI nor collectors/runtime security behavior changes. Existing #2501/#2502
journey evidence is preserved, not relabelled as a new exception-step E2E run.

## 9. Detailed execution contracts

- **Given** current vulnerable declarations and images, **when** the RED contracts
  and exact scanner paths run, **then** they fail for the identified vulnerability
  or observability defect. Capture reason, version, source/image identity and seed.
- **Given** an image, **when** scanners run with actual binary coverage,
  **then** exit0 means zero unaccepted findings, not merely zero analyzed targets.
  For must-gather, the nineteen underlying package findings remain accepted,
  not repaired; keep the unfiltered/expired-exception report alongside GREEN.
- **Given** the production must-gather entry point and a supported cluster,
  **when** all collectors execute, **then** required resource/log/event/audit data
  survives with unchanged scope, TLS, attribution, packaging and sanitization.
- **Given** a scanner error/finding, **when** the production action receives it,
  **then** the original output and status remain observable with correct classification.

## 10. TDD phases and estimates

**Approved exception step** (~20–30min): discover existing configuration and
routing; RED contracts + retained-image scan; GREEN exact IDs/expiry and amended
risk rationale + real image/control scans; REFACTOR N/A (plain configuration);
affected build/lint/regression validation. No container-capacity approval needed.

**Remaining draft application/build-wrapper phases**:
1. **DISCOVERY / readiness** (~15min plan review): approve the remaining scope
   and any required runner capacity; verify all13 readiness dimensions.
2. **RED** (~30–45min): add BDD contract/action tests, run them on unchanged
   implementation; capture real source/image gate failures and wiring preconditions.
3. **GREEN** (~45–90min + image time): minimal application/isolated compiler pins;
   fix status/output capture; wire all production paths and run real IT; CHECKPOINT W.
4. **REFACTOR** (~15min): improve existing scanner/test-fixture error reporting
   and cleanup if GREEN leaves awkward duplication; introduce no new components.
   Plain pins/deletions are N/A with reason. Build/test/lint are mandatory safety
   checks after any actual refactor, not the content of refactoring.
5. **Verification** (~60–120min, environment-dependent): full units/build/lint,
   affected regressions, image/multi-arch scans and confidence/compliance reconciliation.
6. **Delivery**: focused security follow-up commits in existing PR, exact-lease
   guarded rebase publication only after assessment; one batched CI cycle, no
   manual rerun or push while known gate blockers remain. CI outcome is separate
   from local GREEN and must be recorded before claiming merge readiness.

**Subsequent user-directed delivery**: publish the currently verified exception
changes in logical groups and monitor the resulting CI cycle, despite the known
application Go/lint blockers. This supersedes the earlier wait-before-push
restriction only; it does not authorize broader suppressions or unplanned fixes.
The remote's newer main-merge commit has an identical source tree to the tested
local rebase; preserve that commit and use a normal fast-forward push.

## 11. Environment and readiness audit

- Go1.26.9 via a scoped toolchain selection, existing CI-pinned govulncheck1.3.0
  and Trivy0.72.0 with a current recorded DB, existing Ginkgo/Gomega and lint tools.
- Approved UBI builder/runtime bases; owned unique image tags; no replacement or
  deletion of the retained green-v5 evidence images.
- Linux verification UID1006 / netns `kae2e-af`; serial image export/loading lanes;
  preserve the diagnostic node and unrelated #2496 workloads. Legacy compatibility
  clusters run one at a time with explicit ownership and capacity approval.
- Build/lint/unit/IT/wiring/BDD/IDs/compliance/anti-pattern/BR/regression/fail-closed/
  domain gates are all covered by the scenarios and final verification; no
  production-ready declaration on baseline lint debt or missing CI journeys.

## 12. Deliverables

Approved exception configuration, DD-PLATFORM-003 amendment, BDD tests, retained
image before/after and application-control reports, exact hashes/versions/seeds,
and a risk-acceptance execution record. Root/isolated compiler manifests,
application artifact metadata/scans and further issue/PR updates belong to the
remaining draft remediation step. No new source-built kubectl is delivered.

## 13. Execution record

The approved exception configuration is locally verified. RED seed1791688000:
two failing contracts (old expiry and missing risk acceptance), two passing
existing-routing scenarios. Actual retained-image RED exits1 with nineteen HIGH
package findings / sixteen IDs and positive kubectl binary analysis.

GREEN seed1791688600:276/276 infrastructure specs pass with race detection.
The same must-gather image passes the approved exception-aware scan; the
unexcepted application-image control remains blocked. Build, affected lint and
whitespace checks pass. Detailed results are in `KUBECTL_EXCEPTIONS_EXECUTION.md`.
Application/compiler/wrapper implementation
has not started; the isolated experiments are not production changes or plan
approval. No host setting, container, retained image or #2496 file is changed.

Gitleaks addition: RED seed1791717000 has four expected failing scope contracts;
the real scanner reproduces the exact historical alert at lines107–108. GREEN
seed1791717100 passes280/280 infrastructure specs with race detection. The
original-commit scan clears that false positive; different-value/same-file and
same-value/other-file real-scanner controls remain blocking. Build and affected
lint pass. Details: `GITLEAKS_CHECKSUM_EXECUTION.md`.
