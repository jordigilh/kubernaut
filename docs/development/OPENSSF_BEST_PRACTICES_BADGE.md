# OpenSSF Best Practices Badge — Self-Assessment Draft

Reference draft for submitting Kubernaut to the [OpenSSF Best Practices Badge](https://www.bestpractices.dev/)
(formerly CII Best Practices), "Passing" tier. Create an account at bestpractices.dev, add a new
project pointing at `https://github.com/jordigilh/kubernaut`, and copy the answers below into the
web form. Criterion IDs match the form's internal field names so you can find each one quickly.

**Do not just copy-paste blindly** — a few criteria (marked ⚠️ below) require your personal
attestation (e.g., "I know how to design secure software") or actual historical data the form
computes/you must judge (e.g., issue response rate) that I can't verify on your behalf.

Legend: **Met** / **Unmet** / **N/A** — matches the three answer choices the form itself uses.

## Important: this is a *different* badge from the OpenSSF Scorecard badge

Don't confuse the two — both are now in progress for this repo, but they work differently:


|                             | OpenSSF **Scorecard**                                               | OpenSSF **Best Practices** (this doc, formerly "CII")                     |
| --------------------------- | ------------------------------------------------------------------- | ------------------------------------------------------------------------- |
| How it's scored             | Fully automated, re-run weekly by `.github/workflows/scorecard.yml` | Self-certified by you, one-time form                                      |
| What it measures            | ~18 supply-chain heuristics (pinning, permissions, SAST, etc.)      | ~50 broader FLOSS best-practice criteria (docs, testing, crypto, process) |
| README badge already added? | Yes                                                                 | No — added after you complete the steps below                             |




## How to submit

1. Go to [https://www.bestpractices.dev/en/projects/new](https://www.bestpractices.dev/en/projects/new) and click **"Log in with GitHub"**
  (uses your existing `jordigilh` GitHub account — no new credentials to manage).
2. Enter the repository URL: `https://github.com/jordigilh/kubernaut`. The form auto-detects a few
  fields (license, repo URL) from the repo itself.
3. **Fast path — automation proposal links**: `bestpractices.dev` supports pre-filling form fields
  via URL query parameters ([automation-proposals.md](https://github.com/ossf/best-practices-badge/blob/main/docs/automation-proposals.md)).
  Project `13485` already exists with 24% auto-detected by the site's own "Chief" analysis
  (`name`, `contribution`, `license_location`, `floss_license(_osi)`, `documentation_basics`,
  `repo_public/track/distributed`, `release_notes`, `report_process`, `build(_common_tools)`,
  `delivery_mitm`, `discussion`, `maintained` — all already **Met**, no need to touch them). The 6
  links below cover every *other* non-⚠️ criterion that's still blank (46 of 62), regenerated
  directly against the project's live JSON state so nothing is redundant. Proposals only *pre-fill
  blank fields* — nothing is saved until you review and click Save on each page:

  1. [Basics](https://www.bestpractices.dev/en/projects/13485/passing/edit?description_good_status=Met&description_good_justification=README+opens+with+a+one-line+description+%28AIOps+Platform+for+Intelligent+Kubernetes+Remediation%29+and+a+Why+section+explaining+the+problem+it+solves.+https%3A%2F%2Fgithub.com%2Fjordigilh%2Fkubernaut%23readme&interact_status=Met&interact_justification=README+links+to+Issues%2C+Discussions%2C+and+CONTRIBUTING.md%3B+install+docs+link+to+the+docs+site.&contribution_requirements_status=Met&contribution_requirements_justification=CONTRIBUTING.md+Code+Standards+and+Business+Requirements+sections+specify+required+conventions.+https%3A%2F%2Fgithub.com%2Fjordigilh%2Fkubernaut%2Fblob%2Fmain%2FCONTRIBUTING.md%23code-standards&documentation_interface_status=Met&documentation_interface_justification=Each+service%27s+OpenAPI%2FCRD+schema+documented+per-service+under+docs%2Fservices%2F%3B+MCP%2FA2A+protocol+surface+documented+in+the+docs+site.&english_status=Met&english_justification=All+docs%2C+issues%2C+and+code+comments+are+in+English.&repo_interim_status=Met&repo_interim_justification=Commits+between+releases+are+pushed+continuously+via+PRs%2C+not+squashed+into+release-only+snapshots+%28see+https%3A%2F%2Fgithub.com%2Fjordigilh%2Fkubernaut%2Fcommits%2Fmain%29.&version_unique_status=Met&version_unique_justification=SemVer+tags+%28v1.5.2%2C+etc.%29+via+.github%2Fworkflows%2Frelease.yml.&version_semver_status=Met&version_semver_justification=SemVer.&version_tags_status=Met&version_tags_justification=git+tags+per+release+%28vX.Y.Z%29.)
  2. [Release notes + Reporting + Quality, part 1](https://www.bestpractices.dev/en/projects/13485/passing/edit?release_notes_vulns_status=N%2FA&release_notes_vulns_justification=No+publicly+known+runtime+vulnerability+in+Kubernaut%27s+own+code+has+required+a+fix+release+to+date%3B+tracked+OSV+entries+%28.govulncheck-ignore.yaml%29+are+in+upstream+deps+%28Tekton%2C+Prometheus%29%2C+not+Kubernaut+code%2C+with+no+released+fix.&report_tracker_status=Met&report_tracker_justification=GitHub+Issues.&report_archive_status=Met&report_archive_justification=GitHub+Issues+is+a+public%2C+searchable%2C+permanent+archive.&vulnerability_report_process_status=Met&vulnerability_report_process_justification=SECURITY.md+Reporting+a+Vulnerability+section.+https%3A%2F%2Fgithub.com%2Fjordigilh%2Fkubernaut%2Fblob%2Fmain%2FSECURITY.md%23reporting-a-vulnerability&vulnerability_report_private_status=Met&vulnerability_report_private_justification=SECURITY.md+specifies+private+email+reporting%2C+explicitly+asking+reporters+not+to+open+a+public+issue.&build_floss_tools_status=Met&build_floss_tools_justification=Go+compiler%2C+golangci-lint%2C+make+are+all+FLOSS.&test_status=Met&test_justification=Ginkgo%2FGomega+BDD+suite+%28FLOSS%29%2C+documented+in+README+and+CONTRIBUTING.md.+https%3A%2F%2Fgithub.com%2Fjordigilh%2Fkubernaut%2Fblob%2Fmain%2FCONTRIBUTING.md%23test&test_invocation_status=Met&test_invocation_justification=go+test+.%2F...+%2F+make+test-tier-unit%2C+standard+for+Go.&test_most_status=Met&test_most_justification=AGENTS.md+mandates+100%25+unit+coverage+of+business+logic%3B+CI+gate+enforces+via+scripts%2Fcoverage%2Fcoverage_report.py.&test_continuous_integration_status=Met&test_continuous_integration_justification=.github%2Fworkflows%2Fci-pipeline.yml+runs+on+every+push%2FPR.)
  3. [Quality, part 2 + Crypto, part 1](https://www.bestpractices.dev/en/projects/13485/passing/edit?test_policy_status=Met&test_policy_justification=AGENTS.md+mandates+strict+TDD+%28RED-GREEN-REFACTOR%29+for+all+changes%3B+restated+in+CONTRIBUTING.md.+https%3A%2F%2Fgithub.com%2Fjordigilh%2Fkubernaut%2Fblob%2Fmain%2FAGENTS.md%23tdd-workflow&tests_are_added_status=Met&tests_are_added_justification=Evidenced+by+recent+history%3A+native+fuzz+tests+added+for+untrusted-input+parsers+came+with+new+coverage%3B+CI%27s+Test+Suite+Summary+required+check+blocks+merges+without+it.&tests_documented_added_status=Met&tests_documented_added_justification=AGENTS.md+TDD+Workflow+and+AI+Agent+Checkpoints+sections+document+the+policy%2C+referenced+from+CONTRIBUTING.md.&warnings_status=Met&warnings_justification=golangci-lint+%28.golangci.yml%29+with+15%2B+linters+enabled+%28gosec%2C+staticcheck%2C+govet%2C+errcheck%2C+gocyclo%2C+etc.%29.&warnings_fixed_status=Met&warnings_fixed_justification=CI%27s+Lint+%28Go+Services%29+is+a+required+status+check%3B+zero-warning+policy+in+AGENTS.md+GA+Readiness+Audit.&warnings_strict_status=Met&warnings_strict_justification=.golangci.yml+enables+complexity%2Fmaintainability+linters+%28gocyclo%2C+gocognit%2C+nestif%2C+maintidx%2C+funlen%29+beyond+defaults.&crypto_published_status=Met&crypto_published_justification=TLS+%28crypto%2Ftls%29%2C+JWT%2FJOSE+%28go-jose%2Fgo-jose%2Fv4%29%2C+and+Sigstore%2FCosign+keyless+signing+--+all+standard%2C+published%2C+peer-reviewed+protocols.+No+custom+crypto.&crypto_call_status=Met&crypto_call_justification=No+reimplemented+cryptographic+primitives%3B+all+crypto+goes+through+Go%27s+standard+library+or+well-known+libraries+%28go-jose%2C+cosign%29.)
  4. [Crypto, part 2 + Delivery](https://www.bestpractices.dev/en/projects/13485/passing/edit?crypto_floss_status=Met&crypto_floss_justification=Go+stdlib+crypto%2C+go-jose%2C+and+Sigstore%2FCosign+are+all+FLOSS.&crypto_keylength_status=Met&crypto_keylength_justification=TLS+via+crypto%2Ftls+defaults+%28RSA+2048%2B+%2F+ECDSA+P-256%2B%29%3B+no+configuration+path+exposes+shorter+keys.&crypto_working_status=Met&crypto_working_justification=The+one+non-default+SHA-1+use+%28pkg%2Fshared%2Fuuid%2Fuuid.go%29+is+RFC+4122+UUID+v5+name-based+generation%2C+which+mandates+SHA-1+by+spec%3B+not+a+security+mechanism.+No+MD5%2FDES%2FRC4+usage+found.&crypto_weaknesses_status=Met&crypto_weaknesses_justification=No+SHA-1%2FCBC+used+in+any+actual+security+mechanism+%28see+crypto_working%29.&crypto_pfs_status=Met&crypto_pfs_justification=Go%27s+crypto%2Ftls+negotiates+ECDHE+cipher+suites+by+default+%28TLS+1.2%2B%29%2C+providing+forward+secrecy.&crypto_password_storage_status=N%2FA&crypto_password_storage_justification=Kubernaut+does+not+store+end-user+passwords%3B+authentication+is+delegated+to+OIDC+providers.&crypto_random_status=Met&crypto_random_justification=No+direct+use+of+math%2Frand+for+security-sensitive+values%3B+Cosign%2FTLS%2FJWT+libraries+use+crypto%2Frand+internally.&delivery_unsigned_status=Met&delivery_unsigned_justification=Release+images+are+Cosign-signed+%28keyless%2C+Sigstore%29+and+carry+SLSA+provenance+%2B+SBOM+attestations%2C+not+just+a+bare+hash+over+HTTP.+https%3A%2F%2Fgithub.com%2Fjordigilh%2Fkubernaut%2Fblob%2Fmain%2FSECURITY.md%23supply-chain-security)
  5. [Vulnerabilities + Static Analysis, part 1](https://www.bestpractices.dev/en/projects/13485/passing/edit?vulnerabilities_fixed_60_days_status=Met&vulnerabilities_fixed_60_days_justification=3+tracked+OSV+entries+%28.govulncheck-ignore.yaml%29+are+upstream+Go-vulndb+false+positives+%28fix+filed+at+golang%2Fvulndb%235797%29+or+a+low-severity+%28CVSS+3.7%29+Tekton+issue+with+no+upstream+fix+yet%3B+each+has+a+dated+2026-10-01+re-review+commitment.&vulnerabilities_critical_fixed_status=Met&vulnerabilities_critical_fixed_justification=No+unaddressed+critical+vulnerabilities%3B+govulncheck+gated+in+CI+on+every+push%2FPR+%28scripts%2Fci%2Fgovulncheck-gated.sh%29.&no_leaked_credentials_status=Met&no_leaked_credentials_justification=gosec+%28hardcoded-credential+detection%29+runs+in+CI+via+golangci-lint%3B+no+secrets+found+in+the+repository.&static_analysis_status=Met&static_analysis_justification=golangci-lint+%2815%2B+linters%29+%2B+CodeQL.+https%3A%2F%2Fgithub.com%2Fjordigilh%2Fkubernaut%2Fblob%2Fmain%2F.github%2Fworkflows%2Fcodeql.yml&static_analysis_common_vulnerabilities_status=Met&static_analysis_common_vulnerabilities_justification=CodeQL%27s+default+Go+query+suite+targets+known+vulnerability+classes+%28injection%2C+path+traversal%2C+etc.%29%3B+gosec+covers+Go-specific+issues.&static_analysis_fixed_status=Met&static_analysis_fixed_justification=CI-gated%3B+AGENTS.md+GA+Readiness+Audit+mandates+zero+lint%2FSAST+findings+before+merge.&static_analysis_often_status=Met&static_analysis_often_justification=CodeQL+and+golangci-lint+run+on+every+push%2FPR%2C+plus+CodeQL%27s+weekly+scheduled+scan.)
  6. [Dynamic Analysis (fuzzing)](https://www.bestpractices.dev/en/projects/13485/passing/edit?dynamic_analysis_status=Met&dynamic_analysis_justification=Native+Go+fuzz+tests+%28func+FuzzXxx%28f+%2Atesting.F%29%29+target+untrusted-input+parsers+%28JWT+validation%2C+webhook+payload+decoding%2C+YAML+config+parsing%29.+https%3A%2F%2Fgithub.com%2Fjordigilh%2Fkubernaut%2Fblob%2Fmain%2FAGENTS.md%23exception-go-native-fuzz-tests&dynamic_analysis_unsafe_status=N%2FA&dynamic_analysis_unsafe_justification=Go+is+memory-safe%3B+no+C%2FC%2B%2B+in+the+codebase.&dynamic_analysis_enable_assertions_status=Met&dynamic_analysis_enable_assertions_justification=Go%27s+runtime+panics+on+nil-pointer+dereference%2C+index-out-of-range%2C+etc.+are+assertions+that+fuzzing+exercises+directly%3B+none+compiled+out.&dynamic_analysis_fixed_status=Met&dynamic_analysis_fixed_justification=Fuzzing+pkg%2Fnotification%2Frouting.ParseConfig+found+a+real+nil-pointer+panic+%28malformed+YAML+with+null+list+entries%29%2C+fixed+immediately+with+the+crash+input+retained+as+a+permanent+regression+corpus+entry.)

  Click each in order, review the yellow-highlighted (🤖) proposed fields on the resulting edit
  page, then **Save and continue** before clicking the next link — this keeps you logged into the
  same edit session. If a link ever 404s (e.g. project ID changes), re-derive it from the criteria
  list further down using the `field_status=Met&field_justification=...` pattern per
  [automation-proposals.md](https://github.com/ossf/best-practices-badge/blob/main/docs/automation-proposals.md).
4. The remaining ⚠️-flagged criteria (5 of 62) aren't in the links above on purpose — they're
  personal attestations/historical judgments only you can answer honestly. Fill those in manually
  on the form using your own knowledge/judgment rather than copying my suggested text verbatim.
5. The form shows a live "percentage complete" indicator. Once every MUST criterion is answered
  **Met** or a justified **N/A**, the passing badge is awarded automatically — no manual review
   or waiting period, unlike Scorecard's CI-based scoring.
6. Once awarded, the project page (`https://www.bestpractices.dev/en/projects/<your-id>`) shows a
  "Badges" / embed section with the exact markdown to paste into the README, in the form:
   `[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/<id>/badge)](https://www.bestpractices.dev/projects/<id>)`.
   Send me that snippet (or the project ID) once you have it and I'll add it to the README badge row.
7. **Do not pursue Silver/Gold as a solo maintainer** — verified against the live criteria list
  (`bestpractices.dev/en/criteria`): Gold *requires* (MUST) `two_person_review` (≥50% of changes
   reviewed by someone other than the author), `contributors_unassociated` (≥2 unaffiliated
   significant contributors), and `bus_factor` ≥ 2 — none of which a solo maintainer can satisfy
   without recruiting collaborators, and there's no N/A escape hatch for `two_person_review` or
   `contributors_unassociated`. Silver only has `bus_factor` as a SHOULD (not required) plus an
   `access_continuity` criterion (a documented succession/emergency-access plan), so Silver is
   *technically* reachable solo if you write that plan — but there's no need to chase it now.
   **Passing is the right target and has zero structural solo-maintainer blockers** — confirmed
   by reading the full criteria list; none of the above three appear before Gold.

Estimated time: with the automation links doing most of the work, this should be a 10–15 minute
review-and-click session (6 pages) plus a few minutes to answer the 5 self-certify criteria
yourself, rather than a manual copy-paste research project.

### "Other general comments about the project"

This is a free-text field on the project's Basics page (not a scored criterion), added manually.
It accepts Markdown. Text used:

```markdown
**Kubernaut** is an AIOps platform for automated, policy-governed Kubernetes remediation —
combining LLM-driven root-cause analysis with deterministic safety guardrails (RBAC-scoped
actions, dry-run preview, human approval gates) before executing any cluster-changing operation.

A few notes for reviewers:

- **Solo maintainer** — the project is currently maintained by a single person, which is why it
  targets the **Passing** tier rather than Silver/Gold. Those tiers require a two-person review
  process and multiple unaffiliated contributors that a solo maintainer cannot satisfy.
- **Supply-chain security** — releases are signed with Sigstore/Cosign (keyless) and carry SLSA
  provenance + SBOM attestations (see `SECURITY.md`).
  <!-- NOTE: an OpenSSF Scorecard workflow + badge exists on the security/ci-hardening-scorecard
       branch but is not yet merged to main as of 2026-07-04. Add "The repo also holds a
       weekly-recomputed [OpenSSF Scorecard](https://scorecard.dev) badge (see README)." back to
       this bullet, and re-save the general-comments field on bestpractices.dev, once merged and
       the badge is confirmed live. -->
- **Compliance alignment, not certification** — the audit-trace design is built to align with
  SOC2 Trust Service Criteria and FedRAMP control families. **No certification is held or
  claimed.** The internal mapping is documented under
  [`docs/architecture/decisions/`](https://github.com/jordigilh/kubernaut/tree/main/docs/architecture/decisions)
  for teams building their own compliance case.
```

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
- **[vulnerability_report_response]** ⚠️ Self-certify —c `SECURITY.md` *states* a 48-hour
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