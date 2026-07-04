# OpenSSF Best Practices Badge — Self-Assessment Draft

Reference draft for submitting Kubernaut to the [OpenSSF Best Practices Badge](https://www.bestpractices.dev/)
(formerly CII Best Practices), "Passing" tier. Create an account at bestpractices.dev, add a new
project pointing at `https://github.com/jordigilh/kubernaut`, and copy the answers below into the
web form. Criterion IDs match the form's internal field names so you can find each one quickly.

**Do not just copy-paste blindly** — a few criteria (marked ⚠️ below) require your personal
attestation (e.g., "I know how to design secure software") or actual historical data the form
computes/you must judge (e.g., issue response rate) that I can't verify on your behalf.

Legend: **Met** / **Unmet** / **N/A** — matches the three answer choices the form itself uses.

---

## Basics

### Basic project website content

- **[description_good]** Met — README.md opens with a one-line description ("AIOps Platform for
  Intelligent Kubernetes Remediation") followed by a "Why" section explaining the problem it solves.
  URL: `https://github.com/jordigilh/kubernaut#readme`
- **[interact]** Met — README links to Issues, Discussions, and CONTRIBUTING.md; installation
  instructions link to the docs site.
- **[contribution]** Met — `CONTRIBUTING.md` documents the fork → branch → PR flow explicitly.
  URL: `https://github.com/jordigilh/kubernaut/blob/main/CONTRIBUTING.md`
- **[contribution_requirements]** Met — `CONTRIBUTING.md` "Code Standards" and "Business
  Requirements" sections specify required conventions (Ginkgo/Gomega, error handling, BR-tagging).
  URL: `https://github.com/jordigilh/kubernaut/blob/main/CONTRIBUTING.md#code-standards`

### FLOSS license

- **[floss_license]** Met — Apache License 2.0.
- **[floss_license_osi]** Met — Apache-2.0 is OSI-approved.
- **[license_location]** Met — `LICENSE` at repo root.
  URL: `https://github.com/jordigilh/kubernaut/blob/main/LICENSE`

### Documentation

- **[documentation_basics]** Met — README "What It Does" / "Installation" sections plus the full
  docs site.
  URL: `https://jordigilh.github.io/kubernaut-docs/`
- **[documentation_interface]** Met — Each service's OpenAPI/CRD schema is documented per-service
  under `docs/services/`; MCP/A2A protocol surface documented in the docs site.

### Other

- **[sites_https]** Met — GitHub and GitHub Pages (docs site) both enforce HTTPS.
- **[discussion]** Met — GitHub Discussions is enabled and used alongside Issues.
  URL: `https://github.com/jordigilh/kubernaut/discussions`
- **[english]** Met — all docs, issues, and code comments are in English.
- **[maintained]** Met — active commit history (check the form's own auto-detected activity).

## Change Control

### Public version-controlled source repository

- **[repo_public]** Met — public GitHub repo, git.
- **[repo_track]** Met — full git history with authorship and timestamps.
- **[repo_interim]** Met — commits between releases are pushed continuously, not squashed into
  release-only snapshots.
- **[repo_distributed]** Met — git.

### Unique version numbering

- **[version_unique]** Met — SemVer tags (`v1.5.2`, etc.) via `.github/workflows/release.yml`.
- **[version_semver]** Met — SemVer.
- **[version_tags]** Met — git tags per release (`vX.Y.Z`).

### Release notes

- **[release_notes]** Met — `CHANGELOG.md` follows Keep a Changelog format with human-written
  Added/Changed/Fixed sections per version, not raw git log output.
  URL: `https://github.com/jordigilh/kubernaut/blob/main/CHANGELOG.md`
- **[release_notes_vulns]** N/A — no publicly known runtime vulnerability in Kubernaut's own code
  has required a fix release to date (justification: the vulnerabilities currently tracked in
  `.govulncheck-ignore.yaml` are in upstream dependencies — Tekton, Prometheus — not in
  Kubernaut's own code, and none has a released fix).

## Reporting

### Bug-reporting process

- **[report_process]** Met — GitHub Issues with templates.
  URL: `https://github.com/jordigilh/kubernaut/blob/main/.github/ISSUE_TEMPLATE/bug_report.md`
- **[report_tracker]** Met — GitHub Issues.
- **[report_responses]** ⚠️ Self-certify — you'll need to judge whether you've acknowledged most
  issues opened in the last 2–12 months. Check your own Issues tab response history.
- **[enhancement_responses]** ⚠️ Self-certify — same as above, for feature-request issues.
- **[report_archive]** Met — GitHub Issues is a public, searchable, permanent archive.

### Vulnerability report process

- **[vulnerability_report_process]** Met — `SECURITY.md` "Reporting a Vulnerability" section.
  URL: `https://github.com/jordigilh/kubernaut/blob/main/SECURITY.md#reporting-a-vulnerability`
- **[vulnerability_report_private]** Met — `SECURITY.md` specifies private email reporting
  (`jgil@redhat.com`), explicitly asking reporters not to open a public issue.
- **[vulnerability_report_response]** ⚠️ Self-certify — `SECURITY.md` *states* a 48-hour
  acknowledgment target, which would satisfy the ≤14-day requirement, but the criterion asks about
  actual reports received in the last 6 months. If you've had zero reports, most projects answer
  "Met" on the stated policy; if you've had reports that took longer, answer honestly.

## Quality

### Working build system

- **[build]** Met — `go build ./...`, orchestrated via `Makefile` targets (`make build-all`).
- **[build_common_tools]** Met — standard Go toolchain + `make`.
- **[build_floss_tools]** Met — Go compiler, `golangci-lint`, `make` are all FLOSS.

### Automated test suite

- **[test]** Met — Ginkgo/Gomega BDD suite (FLOSS), documented in README "Development" section and
  `CONTRIBUTING.md`.
  URL: `https://github.com/jordigilh/kubernaut/blob/main/CONTRIBUTING.md#test`
- **[test_invocation]** Met — `go test ./...` / `make test-tier-unit`, standard for Go.
- **[test_most]** Met — AGENTS.md mandates 100% unit coverage of business logic; CI gate enforces
  it via `scripts/coverage/coverage_report.py`.
- **[test_continuous_integration]** Met — `.github/workflows/ci-pipeline.yml` runs on every push/PR.

### New functionality testing

- **[test_policy]** Met — `AGENTS.md` mandates strict TDD (RED-GREEN-REFACTOR) for all changes;
  `CONTRIBUTING.md` restates it.
  URL: `https://github.com/jordigilh/kubernaut/blob/main/AGENTS.md#tdd-workflow`
- **[tests_are_added]** Met — evidenced by this session alone: the native fuzz tests added for
  untrusted-input parsers came with new coverage, and CI's `Test Suite Summary` required check
  blocks merges without it.
- **[tests_documented_added]** Met — `AGENTS.md` "TDD Workflow" and "AI Agent Checkpoints" sections
  document the policy explicitly, referenced from `CONTRIBUTING.md`.

### Warning flags

- **[warnings]** Met — `golangci-lint` (`.golangci.yml`) with 15+ linters enabled (`gosec`,
  `staticcheck`, `govet`, `errcheck`, `gocyclo`, etc.).
- **[warnings_fixed]** Met — CI's `Lint (Go Services)` is a required status check; zero-warning
  policy stated in `AGENTS.md` GA Readiness Audit.
- **[warnings_strict]** Met — `.golangci.yml` enables complexity/maintainability linters
  (`gocyclo`, `gocognit`, `nestif`, `maintidx`, `funlen`) beyond the defaults.

## Security

### Secure development knowledge

- **[know_secure_design]** ⚠️ Self-certify — this is a personal attestation about the primary
  developer(s), not something derivable from the repo.
- **[know_common_errors]** ⚠️ Self-certify — same; `AGENTS.md`'s "Go Anti-Pattern Checklist" and
  SOC2/FedRAMP control mapping are good supporting evidence to cite in your answer.

### Use basic good cryptographic practices

- **[crypto_published]** Met — TLS (`crypto/tls`), JWT/JOSE (`go-jose/go-jose/v4`), and Sigstore/
  Cosign (keyless signing) — all standard, published, peer-reviewed protocols. No custom crypto.
- **[crypto_call]** Met — no reimplemented cryptographic primitives; all crypto goes through Go's
  standard library or well-known libraries (`go-jose`, `cosign`).
- **[crypto_floss]** Met — Go stdlib crypto, `go-jose`, and Sigstore/Cosign are all FLOSS.
- **[crypto_keylength]** Met — TLS via `crypto/tls` defaults (RSA 2048+ / ECDSA P-256+), no
  configuration path exposes shorter keys.
- **[crypto_working]** Met — the one non-default use of SHA-1 (`pkg/shared/uuid/uuid.go`) is RFC
  4122 UUID v5 name-based generation, which mandates SHA-1 by spec — it is not a security
  mechanism (no confidentiality/integrity/authentication claim), so this doesn't count against the
  criterion. No MD5/DES/RC4 usage found.
- **[crypto_weaknesses]** Met — see above; no SHA-1/CBC used in any actual security mechanism.
- **[crypto_pfs]** Met — Go's `crypto/tls` negotiates ECDHE cipher suites by default (TLS 1.2+),
  providing forward secrecy.
- **[crypto_password_storage]** N/A — Kubernaut does not store end-user passwords; authentication
  is delegated to OIDC providers.
- **[crypto_random]** Met — no direct use of `math/rand` found for security-sensitive values;
  Cosign/TLS/JWT libraries use `crypto/rand` internally.

### Secured delivery against MITM attacks

- **[delivery_mitm]** Met — git+https/ssh for source; container images pulled over HTTPS from
  Quay/GHCR.
- **[delivery_unsigned]** Met — release images are Cosign-signed (keyless, Sigstore) and carry
  SLSA provenance + SBOM attestations, not just a bare hash over HTTP.
  URL: `https://github.com/jordigilh/kubernaut/blob/main/SECURITY.md#supply-chain-security`

### Publicly known vulnerabilities fixed

- **[vulnerabilities_fixed_60_days]** Met — the 3 currently-tracked OSV entries
  (`.govulncheck-ignore.yaml`) are either upstream Go-vulndb false positives (already fixed
  upstream; a correction is filed at golang/vulndb#5797) or a low-severity (CVSS 3.7) Tekton issue
  requiring cluster-level RBAC to exploit, with no upstream fix available yet. Each entry has a
  dated re-review commitment (2026-10-01) enforcing periodic reassessment — this is a materially
  stronger process than a bare "no known vulnerabilities" claim.
- **[vulnerabilities_critical_fixed]** Met — no unaddressed critical vulnerabilities; `govulncheck`
  gated in CI on every push/PR (`scripts/ci/govulncheck-gated.sh`).

### Other security issues

- **[no_leaked_credentials]** Met — `gosec` (hardcoded-credential detection) runs in CI via
  `golangci-lint`; no secrets have been found in the repository. ⚠️ Consider running a full-history
  scan (`gitleaks detect --source . --log-opts="--all"` or GitHub's own secret scanning, which is
  free and automatic for public repos) once before submitting, just to be certain about historical
  commits.

## Analysis

### Static code analysis

- **[static_analysis]** Met — `golangci-lint` (15+ linters) + CodeQL.
  URL: `https://github.com/jordigilh/kubernaut/blob/main/.github/workflows/codeql.yml`
- **[static_analysis_common_vulnerabilities]** Met — CodeQL's default Go query suite targets
  known vulnerability classes (injection, path traversal, etc.); `gosec` does the same for
  Go-specific issues.
- **[static_analysis_fixed]** Met — CI-gated; `AGENTS.md` GA Readiness Audit mandates zero
  lint/SAST findings before merge.
- **[static_analysis_often]** Met — CodeQL and `golangci-lint` both run on every push/PR, plus
  CodeQL's weekly scheduled scan.

### Dynamic code analysis

- **[dynamic_analysis]** Met — native Go fuzz tests (`func FuzzXxx(f *testing.F)`) targeting
  untrusted-input parsers (JWT validation, webhook payload decoding, YAML config parsing, etc.).
  URL: `https://github.com/jordigilh/kubernaut/blob/main/AGENTS.md#exception-go-native-fuzz-tests`
- **[dynamic_analysis_unsafe]** N/A — Go is memory-safe; no C/C++ in the codebase.
- **[dynamic_analysis_enable_assertions]** Met — Go's runtime panics on nil-pointer dereference,
  index-out-of-range, etc. are all "assertions" that fuzzing exercises directly; no assertions are
  compiled out.
- **[dynamic_analysis_fixed]** Met — concrete evidence: fuzzing `pkg/notification/routing.ParseConfig`
  found a real nil-pointer panic (malformed YAML with null list entries), which was fixed
  immediately and the crash input retained as a permanent regression corpus entry
  (`pkg/notification/testdata/fuzz/FuzzRoutingParseConfig/`).

---

## One unrelated note found while researching this

`CONTRIBUTING.md` still says "Go 1.25.6+" under Prerequisites, but `go.mod` was bumped to 1.26.4
during this hardening pass. Worth a quick fix so new contributors don't install a stale toolchain
version — not part of the badge criteria, just noticed it in passing.
