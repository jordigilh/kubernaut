# Container Image Specification

## Authoritative References

The container image policy for kubernaut-apifrontend follows the standards defined in the
kubernaut repository:

- **ADR-027**: Multi-Architecture Build Strategy — mandates UBI10 base images, multi-arch
  (`linux/amd64`, `linux/arm64`), and migration from alpine/distroless.
- **ADR-028**: Container Registry Policy — approved registries, versioning, compliance
  checklist, and Go Dockerfile template.
- **DD-TEST-007**: E2E Coverage Capture Standard — conditional build flags for coverage
  instrumentation.
- **DD-TEST-001**: Unique Container Image Tags for parallel test isolation.
- **DD-TEST-002**: E2E Dockerfile Optimization — no `dnf update` in service Dockerfiles.

These documents live at:
```
../kubernaut/docs/architecture/decisions/ADR-027-multi-architecture-build-strategy.md
../kubernaut/docs/architecture/decisions/ADR-028-container-registry-policy.md
../kubernaut/docs/architecture/decisions/ADR-028-EXCEPTION-001-upstream-go-arm64.md
../kubernaut/docs/architecture/decisions/DD-TEST-001-unique-container-image-tags.md
../kubernaut/docs/architecture/decisions/DD-TEST-002-parallel-test-execution-standard.md
../kubernaut/docs/architecture/decisions/DD-TEST-007-e2e-coverage-capture-standard.md
```

## Apifrontend Image

| Property | Value |
|----------|-------|
| Binary | `cmd/apifrontend/main.go` |
| Dockerfile | `Dockerfile` (root) |
| Builder base | `registry.access.redhat.com/ubi10/go-toolset:1.25` |
| Production runtime | `scratch` (zero CVE surface) |
| Development runtime | `registry.access.redhat.com/ubi10/ubi-minimal:latest` |
| UID (production) | 65534 (nobody) |
| UID (development) | 1001 |
| Exposed ports | 8443 (API), 8081 (health), 9090 (metrics) |
| Entry point | `/apifrontend` |

## Build Targets

```bash
# Production (scratch, stripped binary)
podman build --target production -t apifrontend:v1.0 .

# Development/E2E (ubi-minimal, coverage support)
podman build --build-arg GOFLAGS=-cover -t apifrontend:dev .
```

## Build Arguments

| Arg | Description | Default |
|-----|-------------|---------|
| `APP_VERSION` | Semantic version injected via `-ldflags` | `v0.1.0` |
| `GIT_COMMIT` | Git SHA for traceability | `unknown` |
| `BUILD_DATE` | ISO 8601 build timestamp | `unknown` |
| `GOFLAGS` | Set to `-cover` for E2E coverage builds | (empty) |
| `TARGETARCH` | Target architecture (`amd64`, `arm64`) | Platform default |

## Labels

Both OCI (`org.opencontainers.image.*`) and Red Hat/OpenShift labels are applied:

- `org.opencontainers.image.source`, `.version`, `.revision`, `.created`, `.title`, `.description`, `.vendor`
- `name`, `vendor`, `summary`, `description`, `maintainer`, `component`, `part-of`
- `io.k8s.description`, `io.k8s.display-name`, `io.openshift.tags`

## Trust Chain (Production)

The `scratch` production image copies only the statically-linked binary plus:
- CA certificates: `/etc/ssl/certs/ca-certificates.crt` (from UBI builder's `ca-certificates` package)
- Timezone data: `/usr/share/zoneinfo`
- Passwd: `/etc/passwd` (for numeric UID resolution)

## Compliance Checklist (ADR-028)

- [x] Base images from `registry.access.redhat.com` only
- [x] Go toolset pinned to minor (`go-toolset:1.25`)
- [x] Runtime uses `latest` tag (ubi-minimal) or scratch
- [x] Multi-arch support via `TARGETARCH` build arg
- [x] Non-root user at runtime
- [x] OCI + Red Hat labels applied
- [x] CGO_ENABLED=0 for static linking
- [x] No `dnf update` in non-builder stages (DD-TEST-002)
- [x] Coverage-conditional build (DD-TEST-007)

## Supply Chain Verification

Released images are cryptographically signed using [Cosign](https://docs.sigstore.dev/cosign/overview/)
with keyless signing (Fulcio + GitHub OIDC). No manual key management is required.

> **Note:** Git tags use `vX.Y.Z` but container image tags strip the `v` prefix (e.g. `1.5.0`).

### Prerequisites

- [Cosign](https://docs.sigstore.dev/cosign/system_config/installation/) v2.4+ installed
- Network access to Sigstore infrastructure (Rekor transparency log)
- Registry credentials if verifying from a private registry

### Verify image signature

```bash
cosign verify \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com \
  --certificate-identity-regexp="^https://github.com/jordigilh/kubernaut-apifrontend/.github/workflows/release.yml@refs/tags/" \
  quay.io/kubernaut-ai/apifrontend:1.5.0
```

### Verify SBOM attestation

The CycloneDX SBOM attestation is currently attached to the **amd64** image only.

```bash
cosign verify-attestation \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com \
  --certificate-identity-regexp="^https://github.com/jordigilh/kubernaut-apifrontend/.github/workflows/release.yml@refs/tags/" \
  --type cyclonedx \
  quay.io/kubernaut-ai/apifrontend:1.5.0-amd64
```

### What is verified

| Artifact | Format | Attached to |
|----------|--------|-------------|
| Image signature | Cosign keyless (Fulcio) | Multi-arch manifest + `:latest` (GA only) |
| SBOM attestation | CycloneDX JSON (in-toto) | amd64 image digest |

### Evidence for compliance auditors

Cosign keyless signing uses short-lived certificates from [Fulcio](https://docs.sigstore.dev/fulcio/overview/)
bound to the GitHub Actions OIDC identity. The signing event is recorded in the
[Rekor](https://docs.sigstore.dev/logging/overview/) transparency log, providing
a tamper-evident audit trail. The expected signer identity is:

```
Issuer:  https://token.actions.githubusercontent.com
Subject: https://github.com/jordigilh/kubernaut-apifrontend/.github/workflows/release.yml@refs/tags/vX.Y.Z
```

The CycloneDX SBOM is also uploaded as a GitHub Release artifact (`sbom-X.Y.Z`).

### Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `no matching signatures` | Image not signed or wrong tag | Verify tag matches a release (not `latest` for pre-releases) |
| `OIDC issuer mismatch` | Using wrong `--certificate-oidc-issuer` | Must be `https://token.actions.githubusercontent.com` |
| `identity mismatch` | Verifying fork-built image | Only images built by this repo's release workflow are signed |
| `network timeout` | Cannot reach Rekor/Sigstore | Check proxy/firewall allows `rekor.sigstore.dev`, `fulcio.sigstore.dev` |

### License compliance

CI runs `go-licenses check` against an approved allowlist:

```
Apache-2.0, BSD-2-Clause, BSD-3-Clause, MIT, ISC, MPL-2.0, Unlicense, CC0-1.0, 0BSD
```

Any dependency with a license not on this list will fail the CI build.

**If CI fails on a license check:**
1. Run `go-licenses report ./...` locally to identify the offending module
2. Evaluate whether the license is acceptable for the project
3. If approved: add to the allowlist in `.github/workflows/ci.yml` and update this doc
4. If not approved: find an alternative dependency or request a legal exception

## Makefile Integration

The `Makefile` should define targets following the kubernaut pattern:

```makefile
IMAGE_REGISTRY ?= quay.io/jordigilh
IMAGE_TAG ?= $(shell git describe --tags --always --dirty)
CONTAINER_TOOL ?= $(shell command -v podman 2>/dev/null || echo docker)

.PHONY: image-build
image-build:
	$(CONTAINER_TOOL) build \
		--target production \
		--build-arg APP_VERSION=$(IMAGE_TAG) \
		--build-arg GIT_COMMIT=$(shell git rev-parse HEAD) \
		--build-arg BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ") \
		-t $(IMAGE_REGISTRY)/apifrontend:$(IMAGE_TAG) .
```
