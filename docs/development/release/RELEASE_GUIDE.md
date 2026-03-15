# Kubernaut Release Guide

How to cut a new release of Kubernaut.

## Releasing a New Version

### 1. Ensure main is ready

All changes intended for the release must be merged to `main` via PR. Direct pushes to `main` are blocked by branch protection (`enforce_admins: true`, required status checks: Lint + Test Suite Summary).

```bash
git checkout main
git pull origin main
```

Verify CI is green on the latest commit before proceeding.

### 2. Tag the release

Tag the `main` branch commit you want to release:

```bash
git tag -a v1.0.0 -m "Kubernaut v1.0.0"
```

### 3. Push the tag

Tags are not subject to branch protection — pushing a tag does not push to `main`:

```bash
git push origin v1.0.0
```

This triggers the release workflow (`.github/workflows/release.yml`).

### 4. Monitor the workflow

Watch progress at **Actions → Release** in the GitHub repository, or:

```bash
gh run watch
```

The workflow has three stages:

| Stage | What it does | Duration |
|-------|-------------|----------|
| **Build & Push Images** | Builds 11 services for amd64 + arm64, pushes to `quay.io/kubernaut-ai/`, creates multi-arch manifests, tags `latest` | ~60-90 min |
| **Publish Helm Chart** | Packages chart with release version, pushes to `oci://quay.io/kubernaut-ai/charts` | ~2 min |
| **Create GitHub Release** | Creates a GitHub Release with auto-generated notes | ~1 min |

### 5. Verify the release

After the workflow completes:

**Images** — Confirm all 11 images are available with the correct tag:

```bash
for svc in gateway signalprocessing aianalysis authwebhook \
           remediationorchestrator workflowexecution notification \
           datastorage effectivenessmonitor holmesgpt-api must-gather; do
  skopeo inspect --raw docker://quay.io/kubernaut-ai/$svc:1.0.0 | python3 -m json.tool | head -5
done
```

**Helm chart** — Confirm the chart is pullable:

```bash
helm show chart oci://quay.io/kubernaut-ai/charts/kubernaut --version 1.0.0
```

**GitHub Release** — Check the release page:

```bash
gh release view v1.0.0
```

### 6. Install

```bash
helm install kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut --version 1.0.0
```

## What Gets Released

### Container Images (11 services)

All images are published to `quay.io/kubernaut-ai/<service>:<version>` as multi-arch manifests (amd64 + arm64).

| Service | Type | Dockerfile |
|---------|------|-----------|
| gateway | Go | `docker/gateway.Dockerfile` |
| signalprocessing | Go | `docker/signalprocessing-controller.Dockerfile` |
| aianalysis | Go | `docker/aianalysis.Dockerfile` |
| authwebhook | Go | `docker/authwebhook.Dockerfile` |
| remediationorchestrator | Go | `docker/remediationorchestrator-controller.Dockerfile` |
| workflowexecution | Go | `docker/workflowexecution-controller.Dockerfile` |
| notification | Go | `docker/notification-controller.Dockerfile` |
| datastorage | Go | `docker/data-storage.Dockerfile` |
| effectivenessmonitor | Go | `docker/effectivenessmonitor-controller.Dockerfile` |
| holmesgpt-api | Python | `holmesgpt-api/Dockerfile` |
| must-gather | Bash | `cmd/must-gather/Dockerfile` |

`mock-llm` is **not** released — it is a test-only artifact.

### Helm Chart

Published to `oci://quay.io/kubernaut-ai/charts/kubernaut`. The chart's `version` and `appVersion` are set from the git tag automatically.

### GitHub Release

Created with auto-generated release notes (commit history since previous tag).

## Versioning

- Follow [SemVer](https://semver.org/): `MAJOR.MINOR.PATCH`
- Git tag format: `v<version>` (e.g. `v1.0.0`, `v1.1.0`, `v2.0.0`)
- The `v` prefix is stripped for image tags and chart versions (tag `v1.0.0` → images tagged `1.0.0`, chart version `1.0.0`)
- Both `<version>` and `latest` tags are pushed for images

## Build Strategy

- **Go services**: `CGO_ENABLED=0` cross-compilation via `GOARCH`. Builder stage uses `ubi10/go-toolset`, runtime uses `ubi10/ubi-minimal`.
- **Python service** (holmesgpt-api): `ubi10/python-312` for both builder and runtime.
- **must-gather**: `ubi10/ubi` base with kubectl and jq.
- **arm64 on amd64 runner**: QEMU user-space emulation (`qemu-user-static`). Go cross-compiles natively; QEMU handles the container base layer and any non-Go build steps.

## Version Injection

Every released image carries build-time version metadata, both embedded in Go binaries and exposed as OCI image labels.

### Go services

All 9 Go services import `internal/version` which exposes three variables (`Version`, `GitCommit`, `BuildDate`) set at build time via `-ldflags`:

```
-X github.com/jordigilh/kubernaut/internal/version.Version=<version>
-X github.com/jordigilh/kubernaut/internal/version.GitCommit=<sha>
-X github.com/jordigilh/kubernaut/internal/version.BuildDate=<timestamp>
```

Each service logs these values at startup.

### OCI labels

All 11 Dockerfiles include standard OCI labels:

| Label | Value |
|-------|-------|
| `org.opencontainers.image.version` | Release version (e.g. `1.0.0`) |
| `org.opencontainers.image.revision` | Git commit SHA |
| `org.opencontainers.image.created` | Build timestamp (RFC 3339) |
| `org.opencontainers.image.source` | Repository URL |
| `org.opencontainers.image.title` | Service name |

### How it flows

1. The release workflow extracts version from the git tag and captures `build_date` and `github.sha`.
2. These are passed as `APP_VERSION`, `GIT_COMMIT`, `BUILD_DATE` environment variables to `make image-build`.
3. The Makefile forwards them as `--build-arg` to each `podman build` invocation.
4. Inside each Dockerfile, `ARG` declarations receive the values, which are used in `ldflags` (Go) and `LABEL` directives (all).

For local builds, the Makefile defaults to `dev` / short git SHA / current timestamp.

## Troubleshooting

### Workflow fails at "Login to Quay.io"

Verify secrets are set:

```bash
gh secret list --repo jordigilh/kubernaut
```

Both `QUAY_ROBOT_USERNAME` and `QUAY_ROBOT_TOKEN` must be present.

### Image push fails with 403

The robot account lacks write permission on the target repository. Either:
- Add the robot to a team with **Creator** role (recommended), or
- Grant **Write** permission on the specific repository in Quay.io

### arm64 build fails or hangs

QEMU emulation can be slow or unstable for large builds. Check:
- `qemu-user-static` is installed on the runner
- `/proc/sys/fs/binfmt_misc/qemu-aarch64` exists

### Helm push fails

Ensure the robot account can create repositories in Quay.io (Creator team role). The `charts/kubernaut` repository is auto-created on first push.

## Related

- Issue [#80](https://github.com/jordigilh/kubernaut/issues/80) — Release: Helm chart creation, multi-arch images, and public publishing
- Issue [#257](https://github.com/jordigilh/kubernaut/issues/257) — release(ci): Multi-arch image build + Helm OCI publish workflow
- Issue [#273](https://github.com/jordigilh/kubernaut/issues/273) — Standardize version injection and OCI labels across all services
- `.github/workflows/release.yml` — Release workflow source
- `Makefile` — `image-build`, `image-push`, `image-manifest` targets
