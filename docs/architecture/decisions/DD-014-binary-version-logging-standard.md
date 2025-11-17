# DD-014: Binary Version Logging Standard

## Status
**✅ APPROVED** (2025-11-17)
**Last Reviewed**: 2025-11-17
**Confidence**: 95%

## Context & Problem

**Problem**: When troubleshooting production issues, we need to know the exact version of code running in each container. Without this information:
- Cannot trace runtime behavior back to specific source code commits
- Difficult to verify if latest fixes are deployed
- No audit trail for version history
- Cannot correlate issues with specific code changes

**Key Requirements**:
- Every service binary must log its version on startup
- Version must include: semantic version, git commit hash, and build timestamp
- Version info must be embedded at build time (not runtime)
- Must work across all services (Gateway, Controllers, APIs, etc.)
- Must avoid environment variable collisions (e.g., UBI Go toolset's `VERSION`)

## Alternatives Considered

### Alternative 1: Runtime Version File
**Approach**: Store version info in a file bundled with the container

**Pros**:
- ✅ Simple to implement
- ✅ No build-time complexity

**Cons**:
- ❌ File can be lost or modified
- ❌ Not embedded in binary
- ❌ Requires file I/O at startup
- ❌ Separate artifact to manage

**Confidence**: 30% (rejected - not robust enough)

---

### Alternative 2: Build-Time ldflags Injection (APPROVED)
**Approach**: Inject version info into binary via Go's `-ldflags -X` at build time

**Pros**:
- ✅ Version embedded directly in binary
- ✅ Immutable (cannot be changed post-build)
- ✅ Standard Go practice
- ✅ No runtime dependencies
- ✅ Works with `--version` flag
- ✅ Logged automatically on startup

**Cons**:
- ⚠️ Requires build-time arguments - **Mitigation**: Standardized Dockerfile pattern
- ⚠️ Must avoid env var collisions - **Mitigation**: Use `APP_VERSION` instead of `VERSION`

**Confidence**: 95% (approved)

---

### Alternative 3: Git Describe at Runtime
**Approach**: Execute `git describe` at container startup

**Pros**:
- ✅ Always accurate

**Cons**:
- ❌ Requires git in container (bloat)
- ❌ Requires .git directory (security risk)
- ❌ Slow startup
- ❌ Not production-ready

**Confidence**: 20% (rejected - not suitable for production)

---

## Decision

**APPROVED: Alternative 2** - Build-Time ldflags Injection

**Rationale**:
1. **Immutability**: Version info embedded in binary, cannot be tampered with
2. **Standard Practice**: Go's `-ldflags -X` is the industry standard for version injection
3. **Zero Runtime Cost**: No file I/O, no git execution, instant startup logging
4. **Audit Trail**: Every container log shows exact version on startup
5. **Traceability**: Git commit hash enables precise source code correlation

**Key Insight**: Build-time injection provides immutable, zero-cost version tracking that's essential for production troubleshooting and audit compliance.

## Implementation

### Standard Pattern for All Services

#### 1. Binary Code Pattern (`cmd/<service>/main.go`)

```go
package main

import (
	"flag"
	"fmt"
	"os"
	
	"go.uber.org/zap"
)

var (
	// version is the semantic version, set at build time via -ldflags
	version = "dev"
	// gitCommit is the git commit hash, set at build time via -ldflags
	gitCommit = "dev"
	// buildDate is the build timestamp, set at build time via -ldflags
	buildDate = "dev"
)

func main() {
	// Parse command-line flags
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("<Service Name> %s-%s (built: %s)\n", version, gitCommit, buildDate)
		os.Exit(0)
	}

	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = logger.Sync()
	}()

	// Log version on startup (MANDATORY)
	logger.Info("Starting <Service Name>",
		zap.String("version", version),
		zap.String("git_commit", gitCommit),
		zap.String("build_date", buildDate))

	// ... rest of service initialization
}
```

#### 2. Dockerfile Pattern

```dockerfile
FROM registry.access.redhat.com/ubi9/go-toolset:1.24 AS builder

# Build arguments for multi-architecture support
ARG TARGETOS=linux
ARG TARGETARCH

# Build version arguments
# IMPORTANT: Use APP_VERSION to avoid collision with Go toolset's VERSION env var
ARG APP_VERSION=dev
ARG GIT_COMMIT=dev
ARG BUILD_DATE=dev

# ... copy source code ...

# Build with version injection
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
	-ldflags="-w -s -extldflags '-static' -X main.version=${APP_VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildDate=${BUILD_DATE}" \
	-a -installsuffix cgo \
	-o <service-binary> \
	./cmd/<service>

# ... final stage ...
```

**CRITICAL**: Use `APP_VERSION` not `VERSION` to avoid collision with UBI Go toolset's `VERSION` environment variable (which is set to Go version like `1.24.6`).

#### 3. Build Command Pattern

```bash
#!/bin/bash
# Build script for all services

GIT_COMMIT=$(git log -1 --format=%h)
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
APP_VERSION="v0.1.0"  # Or from VERSION file

podman build \
  -f docker/<service>-ubi9.Dockerfile \
  --build-arg APP_VERSION=${APP_VERSION} \
  --build-arg GIT_COMMIT=${GIT_COMMIT} \
  --build-arg BUILD_DATE=${BUILD_DATE} \
  -t quay.io/jordigilh/<service>:latest \
  .
```

### Expected Log Output

**Startup log (JSON format):**
```json
{
  "level": "info",
  "ts": 1763404816.39772,
  "caller": "gateway/main.go:69",
  "msg": "Starting Gateway Service",
  "version": "v0.1.0",
  "git_commit": "1c357782",
  "build_date": "2025-11-17T18:37:31Z",
  "config_path": "/etc/gateway/config.yaml",
  "listen_addr": ":8080"
}
```

**Version flag output:**
```bash
$ gateway --version
Gateway Service v0.1.0-1c357782 (built: 2025-11-17T18:37:31Z)
```

### Reference Implementation

**Service**: Gateway Service
**Files**:
- `cmd/gateway/main.go` (lines 36-43, 54-56, 69-75)
- `docker/gateway-ubi9.Dockerfile` (lines 10-13, 41-45)

**Commits**:
- `730674f9` - feat(gateway): Add version, git commit, and build date to binary
- `edcce6f0` - fix(gateway): Fix ldflags quoting to properly expand build arguments
- `1c357782` - fix(gateway): Use APP_VERSION to avoid collision with Go toolset VERSION env var

## Consequences

### Positive
- ✅ **Full Traceability**: Every container logs exact git commit on startup
- ✅ **Zero Runtime Cost**: No file I/O, no git execution
- ✅ **Immutable**: Version info cannot be changed post-build
- ✅ **Audit Compliance**: Complete version history in logs
- ✅ **Troubleshooting**: Can correlate issues with specific code changes
- ✅ **Deployment Verification**: Confirm correct version is deployed

### Negative
- ⚠️ **Build Complexity**: Requires passing build args - **Mitigation**: Standardized build scripts
- ⚠️ **Variable Naming**: Must use `APP_VERSION` not `VERSION` - **Mitigation**: Documented in this DD

### Neutral
- 🔄 **All Services Must Adopt**: Requires updating all service Dockerfiles and main.go files
- 🔄 **CI/CD Updates**: Build pipelines must pass version arguments

## Validation Results

**Gateway Service Validation** (2025-11-17):
- ✅ Binary shows correct version: `v0.1.0-1c357782 (built: 2025-11-17T18:37:31Z)`
- ✅ Startup logs include all version fields
- ✅ Git commit hash matches source: `1c357782`
- ✅ Build timestamp accurate
- ✅ No collision with Go toolset VERSION env var

**Confidence Assessment Progression**:
- Initial assessment: 85% confidence
- After Gateway implementation: 95% confidence
- After validation: 95% confidence

## Rollout Plan

### Phase 1: Gateway Service (COMPLETED)
- ✅ Implement pattern in Gateway
- ✅ Validate in production
- ✅ Document lessons learned

### Phase 2: Controllers (NEXT)
- [ ] RemediationProcessor Controller
- [ ] AIAnalysis Controller
- [ ] WorkflowExecution Controller
- [ ] RemediationOrchestrator Controller
- [ ] NotificationRequest Controller

### Phase 3: Stateless Services
- [ ] HolmesGPT API
- [ ] Context API
- [ ] Workflow Catalog Service
- [ ] Data Storage Service

### Phase 4: CI/CD Integration
- [ ] Update build scripts to pass version args
- [ ] Add version validation to CI pipeline
- [ ] Document build process

## Related Decisions
- **Builds On**: ADR-027 (Multi-Architecture Build Strategy)
- **Builds On**: ADR-028 (Container Registry Policy)
- **Supports**: DD-005 (Observability Standards)

## Review & Evolution

**When to Revisit**:
- If Go build tooling changes ldflags behavior
- If container base image changes affect env vars
- If new services require different patterns

**Success Metrics**:
- 100% of services log version on startup
- Zero incidents of "unknown version" in production
- <5 seconds to identify deployed version from logs

---

## Quick Reference Checklist

### For Each Service Implementation:

**Code Changes** (`cmd/<service>/main.go`):
- [ ] Add `version`, `gitCommit`, `buildDate` variables (default to `"dev"`)
- [ ] Add `--version` flag handler
- [ ] Log version info on startup with `zap.String()` fields

**Dockerfile Changes** (`docker/<service>-ubi9.Dockerfile`):
- [ ] Add `ARG APP_VERSION=dev` (NOT `VERSION`)
- [ ] Add `ARG GIT_COMMIT=dev`
- [ ] Add `ARG BUILD_DATE=dev`
- [ ] Update `go build` ldflags: `-X main.version=${APP_VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildDate=${BUILD_DATE}`

**Build Script**:
- [ ] Set `GIT_COMMIT=$(git log -1 --format=%h)`
- [ ] Set `BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")`
- [ ] Set `APP_VERSION` from VERSION file or hardcode
- [ ] Pass `--build-arg APP_VERSION=... --build-arg GIT_COMMIT=... --build-arg BUILD_DATE=...`

**Validation**:
- [ ] Run `<service> --version` - should show `v0.1.0-<hash> (built: <timestamp>)`
- [ ] Check startup logs - should include `version`, `git_commit`, `build_date` fields
- [ ] Verify git commit hash matches source
- [ ] Verify build timestamp is accurate

---

**Priority**: FOUNDATIONAL - Critical for production operations and troubleshooting

