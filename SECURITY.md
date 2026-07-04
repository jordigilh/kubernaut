# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.5.x   | :white_check_mark: |
| 1.4.x   | :white_check_mark: |
| 1.3.x   | :white_check_mark: |
| 1.2.x   | :x:                |
| < 1.2   | :x:                |

## Reporting a Vulnerability

If you discover a security vulnerability in Kubernaut, please report it responsibly.

**Do NOT open a public GitHub issue for security vulnerabilities.**

Instead, please email **jgil@redhat.com** with:

1. A description of the vulnerability
2. Steps to reproduce
3. Potential impact
4. Any suggested fix (optional)

### What to Expect

- **Acknowledgment** within 48 hours of your report
- **Assessment** within 5 business days
- **Fix timeline** communicated after assessment, typically within 30 days for critical issues
- **Credit** in the release notes (unless you prefer to remain anonymous)

## Security Considerations

Kubernaut operates with elevated Kubernetes RBAC permissions to perform remediation actions. When deploying:

- Follow the principle of least privilege for service accounts
- Use approval gates (`requiresApproval: true`) for destructive remediation workflows
- Review workflow schemas before registering them in the catalog
- Restrict access to the DataStorage and HAPI APIs
- Rotate LLM provider credentials regularly

## Supply Chain Security

All container images built by Kubernaut's CI and Release pipelines are signed keylessly with [Cosign](https://github.com/sigstore/cosign) using GitHub Actions OIDC as the signing identity (no long-lived private keys). SBOMs (CycloneDX format) are generated for every image and, for Release images, cryptographically bound to the image digest via Cosign attestation.

| Pipeline | Workflow | Registry | Lifecycle |
|---|---|---|---|
| CI | `.github/workflows/ci-pipeline.yml` | `ghcr.io/jordigilh/kubernaut/*` | Ephemeral (14-day retention), used by integration/E2E tests |
| Release | `.github/workflows/release.yml` | `quay.io/kubernaut-ai/*` | Production, tagged `v*` releases |

### Verifying image signatures

Verify a CI image (signed on every push/PR build):

```bash
cosign verify \
  --certificate-identity-regexp "^https://github.com/jordigilh/kubernaut/\.github/workflows/ci-pipeline\.yml@.*$" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/jordigilh/kubernaut/<service>:<tag>
```

Verify a Release image (signed on `v*` tag push):

```bash
cosign verify \
  --certificate-identity-regexp "^https://github.com/jordigilh/kubernaut/\.github/workflows/release\.yml@.*$" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  quay.io/kubernaut-ai/<service>:<version>
```

### Verifying SBOM provenance

Release images carry their CycloneDX SBOM as a Cosign attestation, bound to the image digest:

```bash
cosign verify-attestation \
  --type cyclonedx \
  --certificate-identity-regexp "^https://github.com/jordigilh/kubernaut/\.github/workflows/release\.yml@.*$" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  quay.io/kubernaut-ai/<service>:<version>
```

### Verifying SLSA build provenance

Release images also carry SLSA v1.0 build provenance:

```bash
cosign verify-attestation \
  --type slsaprovenance \
  --certificate-identity-regexp "^https://github.com/jordigilh/kubernaut/\.github/workflows/release\.yml@.*$" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  quay.io/kubernaut-ai/<service>:<version>
```

## Continuous Security Scanning

Every push and pull request is automatically scanned by:

| Tool | Workflow | What it catches |
|---|---|---|
| [CodeQL](https://codeql.github.com/) | `.github/workflows/codeql.yml` | Static analysis (SAST) for common vulnerability classes (injection, path traversal, etc.) |
| [govulncheck](https://go.dev/security/vuln/) | `scripts/ci/govulncheck-gated.sh` | Known vulnerabilities (OSV) in Go dependencies, gated on actual call-graph reachability |
| [gitleaks](https://github.com/gitleaks/gitleaks) | `.github/workflows/gitleaks.yml` | Hardcoded secrets/credentials, scanning full git history on every push, PR, and weekly schedule |
| [OpenSSF Scorecard](https://scorecard.dev/) | `.github/workflows/scorecard.yml` | Supply-chain security posture (pinning, permissions, branch protection, etc.) |

Known-benign matches (e.g., fake credentials in the sanitization/redaction test suites, which necessarily contain secret-shaped strings as test input) are tracked in `.gitleaks.toml` with a documented rationale for each -- see `docs/development/OPENSSF_BEST_PRACTICES_BADGE.md` for the full triage.

## Disclosure Policy

We follow coordinated disclosure. We ask that you give us reasonable time to address the vulnerability before public disclosure.
