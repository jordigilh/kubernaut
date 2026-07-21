# DD-PLATFORM-003: Must-Gather kubectl Version Bump Policy (CVE Remediation)

**Date**: July 13, 2026
**Status**: ✅ **APPROVED**
**Confidence**: 90%
**Last Reviewed**: July 13, 2026
**Related**: Issue #1663 (Trivy security scan blocking `v1.6.0-rc1` release), Issue #1662 (follow-up to re-triage residual CVEs before exception expiration), Issue #1315 (Trivy/SBOM release gate), `cmd/must-gather/Dockerfile`

---

## 🎯 **DECISION**

**Bump the `kubectl` binary bundled in the `must-gather` image from `v1.31.0`
to the latest published release (`v1.36.2`) to resolve the majority of the
23 CVEs (1 CRITICAL, 22 HIGH) flagged by the release pipeline's Trivy scan
(GitHub Actions run 29058480982, job 86711219020), even though this exceeds
kubectl's officially documented +/-1 minor version-skew support window for
the image's stated target server versions (1.30-1.32). Compatibility was
validated empirically via a kind-cluster spike (see below) before approval.
The small number of CVEs with no available fix in any current kubectl
release (because upstream Kubernetes has not yet adopted the necessary
`golang.org/x/net`/Go-toolchain patch versions) are suppressed via a
time-bound `.trivyignore` exception that expires and resurfaces the finding
for re-triage.**

---

## 📊 **Context & Problem**

The `v1.6.0-rc1` release pipeline's Trivy scan of the `must-gather` image
failed with `exit-code: 1` because `/usr/local/bin/kubectl` (a prebuilt
binary downloaded from `dl.k8s.io` in `cmd/must-gather/Dockerfile`) reported
23 fixable CVEs:

- 1 **CRITICAL**: `CVE-2025-68121` (`crypto/tls` certificate validation, Go stdlib)
- 22 **HIGH**: `golang.org/x/net` (4), `golang.org/x/oauth2` (1),
  `github.com/moby/spdystream` (1), Go stdlib (16 more)

### Investigation

Downloaded and inspected (`go version -m`) every currently published kubectl
release from `v1.31.14` (latest 1.31.x patch) through `v1.36.2` (latest
stable overall, and even `kubernetes/kubernetes@master`, unreleased). Finding:
**no published kubectl release fully resolves all 23 CVEs** — upstream
Kubernetes has not yet picked up `golang.org/x/net >= 0.55.0` (needed for
4 CVEs) or a Go toolchain >= 1.25.12/1.26.5 (needed for 1 CVE;
`v1.36.2` ships Go 1.26.4). `v1.36.2` is the best currently achievable target,
resolving 18 of 23 CVEs (including the sole CRITICAL).

## Alternatives Considered

### Alternative A: Build kubectl from source with force-patched transitive deps

**Approach**: Multi-stage Docker build; a small Go module requiring
`k8s.io/kubectl@v0.31.14` (preserving v1.31 client behavior) with
`golang.org/x/net`, `golang.org/x/oauth2`, and `github.com/moby/spdystream`
force-upgraded past their fixed versions via `go get`, compiled with Go
1.26.5. Validated via local spike — builds and runs successfully, resolves
100% of the 23 CVEs while keeping the documented 1.30-1.32 skew window.

**Pros**:
- ✅ Resolves all 23 CVEs immediately, no residual exceptions
- ✅ Preserves official kubectl version-skew compatibility (client stays v1.31.x)

**Cons**:
- ❌ New build pattern (Go builder stage) for an otherwise Bash-only image
- ❌ Ships dependency combinations upstream Kubernetes has never tested/released
- ❌ Ongoing maintenance burden to keep force-pinned versions current

**Confidence**: 85% (rejected — user preferred simpler approach + explicit tracking over a bespoke unreleased dependency combination)

---

### Alternative B: Bump to latest published kubectl release + kind-validate + time-bound exception (CHOSEN)

**Approach**: Change one line in the Dockerfile (`v1.31.0` → `v1.36.2`).
Validate empirically against kind clusters running the documented supported
server versions (1.30, 1.31, 1.32) by diffing every kubectl invocation
pattern used by `cmd/must-gather/collectors/*.sh` between the old and new
client. Suppress the small number of CVEs with no available upstream fix via
a `.trivyignore` file with an explicit expiration date so the exception
resurfaces for re-triage rather than being silently permanent.

**Pros**:
- ✅ Minimal, easily reviewable Dockerfile change
- ✅ Resolves 18/23 CVEs (100% of CRITICAL) using only released, upstream-tested artifacts
- ✅ Empirically validated (not just policy-assumed) against all 3 documented server versions
- ✅ Residual risk is time-boxed and self-resurfacing, not permanently hidden

**Cons**:
- ❌ Exceeds kubectl's official +/-1 minor version-skew policy for servers 1.30-1.32 (mitigated by empirical validation, not just wishful skipping)
- ❌ Leaves 5 CVEs technically "fixed upstream but unavailable in kubectl" until Kubernetes ships a newer patch release

**Confidence**: 90% (approved by user)

---

### Alternative C: Keep `v1.31.0`, blanket-ignore all 23 CVEs pending upstream fix

**Approach**: No Dockerfile change; suppress the whole Trivy finding set for `must-gather`.

**Pros**:
- ✅ Zero engineering effort

**Cons**:
- ❌ Leaves the sole CRITICAL CVE (and 17 other HIGH CVEs that DO have a fix already published upstream) unresolved for no reason — a real regression in security posture with a trivial available fix
- ❌ Sets a bad precedent of reaching for suppression before exhausting available upstream fixes

**Confidence**: 20% (rejected)

---

## Decision

**APPROVED: Alternative B**

**Rationale**:
1. **Best available fix, no invented dependency graph**: Resolves the CRITICAL CVE and 17 of 22 HIGH CVEs using only an artifact Kubernetes itself has built, tested, and released — no bespoke/unreleased dependency combination to maintain.
2. **Empirically validated, not policy-assumed**: A kind-cluster spike executed every kubectl invocation pattern from all 6 must-gather collectors against live 1.30/1.31/1.32 API servers with both the old (v1.31.0) and new (v1.36.2) client and diffed the results. Zero return-code mismatches and zero functional output differences across all 3 versions; only cosmetic differences (client version banner, a `describe pod`/`describe node` label/format tweak) were observed.
3. **Residual risk is bounded and self-resurfacing**: The 5 CVEs with no available kubectl release fix are suppressed via `.trivyignore` entries with an explicit `exp:` expiration date (90 days), so the CI gate re-fails automatically for re-triage instead of the exception being silently permanent.

**Key Insight**: Kubectl's documented n-1/n+1 version-skew policy is a
*support* boundary, not a hard compatibility wall for the specific,
narrow set of read-only `get`/`describe`/`logs`/`top`/`api-resources`
operations must-gather performs. Empirical validation against the exact
command surface in use is a stronger signal than the general policy for
this specific tool.

### Implementation

**Primary Implementation Files**:
- `cmd/must-gather/Dockerfile` — kubectl download version bumped `v1.31.0` → `v1.36.2`
- `cmd/must-gather/.trivyignore` — time-bound exceptions for the 5 CVEs with no available upstream fix
- `.github/workflows/release.yml` — `trivyignores` input wired into both `security-scan-amd64`/`security-scan-arm64` Trivy steps for the `must-gather` matrix entry

**Validation performed** (not shipped as automated CI — see Consequences):
- kind clusters `v1.30.8`, `v1.31.9`, `v1.32.8` (kindest/node images)
- Representative resources per collector: namespaces, a CRD + CR instance, a
  Deployment/pod with logs, RBAC (Role/RoleBinding/ClusterRole/ClusterRoleBinding),
  PVC, Service, NetworkPolicy, Ingress, ConfigMap, webhook configs
- Every `kubectl` invocation pattern from `collectors/*.sh` executed with both
  `kubectl-baseline` (v1.31.0) and `kubectl-candidate` (v1.36.2) clients,
  diffed after normalizing volatile fields (timestamps, UIDs, resourceVersions)

### Consequences

**Positive**:
- ✅ Release pipeline's Trivy gate passes with a real, verified reduction in exposure (23 → 5 CVEs, CRITICAL eliminated)
- ✅ No new build pattern/technology introduced into an otherwise simple Bash image

**Negative**:
- ⚠️ 5 CVEs remain unresolved until Kubernetes ships a release with `golang.org/x/net >= 0.55.0` and Go >= 1.25.12/1.26.5 — **Mitigation**: time-bound `.trivyignore` (90-day expiration) + tracking issue filed to revisit
- ⚠️ kind-cluster compatibility validation was a manual, one-time spike, not an automated regression test — **Mitigation**: documented here for future re-validation; consider promoting to a CI job if kubectl is bumped again before 1.30-1.32 support is dropped

**Neutral**:
- 🔄 Future kubectl bumps for this image should repeat the kind-cluster validation spike documented here, especially if the target server-version range changes

### Validation Results

**Confidence Assessment Progression**:
- Initial assessment (before investigation): 60% — assumed a simple version bump would fully resolve the scan
- After dependency/toolchain investigation: 75% — confirmed no release fully resolves all CVEs, several alternatives viable
- After kind-cluster spike (3 versions, zero functional diffs): 90% — empirical evidence supports the skew-policy exception

**Key Validation Points**:
- ✅ `go version -m` inspection of actual released kubectl binaries (not just `go.mod`, which can diverge from `replace` directives) confirmed exact embedded dependency versions for v1.31.0 through v1.36.2
- ✅ kind-cluster diff spike: 0 RC mismatches, 0 functional output diffs across 1.30/1.31/1.32
- ✅ Local Trivy scan of the rebuilt image (see PR) confirms exactly the expected 5 residual CVEs

### Related Decisions
- **Builds On**: Issue #1315 (Trivy/SBOM release gate introduction)

### Review & Evolution

**When to Revisit**:
- When the `.trivyignore` 90-day exception expires (tracking issue filed)
- If Kubernetes ships a kubectl release with `golang.org/x/net >= 0.55.0` and Go >= 1.25.12/1.26.5 sooner (re-run this spike, drop the exception early)
- If the documented minimum supported server version for must-gather changes (re-run the kind-cluster validation spike against the new range)

**Success Metrics**:
- Trivy scan for `must-gather`: 0 CRITICAL, <=5 time-bound-exception HIGH (target: 0 once upstream catches up)
- No user-reported must-gather functional regressions against clusters in the 1.30-1.32 range post-release
