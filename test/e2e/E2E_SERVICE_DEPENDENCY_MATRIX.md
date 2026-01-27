# E2E Service Dependency Matrix

**Status**: ✅ AUTHORITATIVE  
**Last Updated**: January 26, 2026  
**Purpose**: Defines all service dependencies for E2E test infrastructure setup

---

## Quick Reference

This matrix documents the **actual dependencies** deployed by each E2E test suite, extracted from the infrastructure bootstrap code in `test/infrastructure/*_e2e*.go`.

### Critical Dependencies

- **Data Storage (DS)**: ALL services depend on DS for audit trail storage (SOC2 compliance)
- **AuthWebhook (AW)**: Required by services that need user attribution (SOC2 CC8.1)
- **HolmesGPT API + Mock LLM**: Required for AI/ML analysis services

---

## Service Dependency Matrix

| Service | Service Images | Shared Services | External Dependencies | Kind Cluster Config | Notes |
|---------|---------------|-----------------|----------------------|---------------------|-------|
| **datastorage** | `datastorage` | None (standalone) | PostgreSQL 16, Redis | `kind-datastorage-config.yaml` | Core dependency for all other services |
| **gateway** | `gateway`, `datastorage` | None | PostgreSQL 16, Redis | `kind-gateway-config.yaml` | Ingestion service with DS for audit |
| **aianalysis** | `aianalysis`, `datastorage`, `holmesgpt-api`, `mock-llm` | None | PostgreSQL 16, Redis | `kind-aianalysis-config.yaml` | AI analysis with HAPI + Mock LLM |
| **authwebhook** | `authwebhook`, `datastorage` | None | PostgreSQL 16, Redis | `kind-authwebhook-config.yaml` | User attribution for SOC2 CC8.1 |
| **notification** | `notification`, `authwebhook` | DS (shared deployment) | None | `kind-notification-config.yaml` | Uses shared DS, requires AW |
| **remediationorchestrator** | `remediationorchestrator`, `datastorage`, `authwebhook` | None | PostgreSQL 16, Redis | `kind-remediationorchestrator-config.yaml` | Requires DS + AW (SOC2) |
| **signalprocessing** | `signalprocessing`, `datastorage` | None | PostgreSQL 16, Redis | `kind-signalprocessing-config.yaml` | Signal enrichment with DS audit |
| **workflowexecution** | `workflowexecution`, `datastorage`, `authwebhook` | None | PostgreSQL 16, Redis, Tekton v1.7.0 | `kind-workflowexecution-config.yaml` | Requires DS + AW + Tekton |
| **holmesgpt-api** | `holmesgpt-api`, `datastorage`, `mock-llm` | None | PostgreSQL 16, Redis | (uses workflowexecution config) | HAPI with Mock LLM + DS audit |

---

## Dependency Graph

```
datastorage (standalone)
    ├─ PostgreSQL 16
    └─ Redis

gateway
    └─ datastorage (audit trails)

aianalysis
    ├─ datastorage (audit trails)
    ├─ holmesgpt-api (AI analysis)
    └─ mock-llm (cost-free LLM simulation)

authwebhook
    └─ datastorage (audit trails)

notification
    ├─ datastorage (shared deployment, audit trails)
    └─ authwebhook (user attribution, SOC2 CC8.1)

remediationorchestrator
    ├─ datastorage (audit trails)
    └─ authwebhook (user attribution, SOC2 CC8.1)

signalprocessing
    └─ datastorage (audit trails)

workflowexecution
    ├─ datastorage (audit trails)
    ├─ authwebhook (user attribution, SOC2 CC8.1)
    └─ Tekton v1.7.0 (workflow engine)

holmesgpt-api
    ├─ datastorage (audit trails)
    └─ mock-llm (cost-free LLM simulation)
```

---

## Image Build Details

### Dockerfile Locations

| Service | Dockerfile Path | Image Name | Coverage Support |
|---------|-----------------|------------|------------------|
| datastorage | `docker/data-storage.Dockerfile` | `kubernaut/datastorage` | ✅ Yes (E2E_COVERAGE) |
| gateway | `docker/gateway-ubi9.Dockerfile` | `kubernaut/gateway` | ✅ Yes (E2E_COVERAGE) |
| aianalysis | `docker/aianalysis.Dockerfile` | `kubernaut/aianalysis-controller` | ✅ Yes (E2E_COVERAGE) |
| authwebhook | `docker/authwebhook.Dockerfile` | `authwebhook` | ✅ Yes (E2E_COVERAGE) |
| notification | `docker/notification-controller-ubi9.Dockerfile` | `kubernaut-notification` | ❌ No |
| remediationorchestrator | `docker/remediationorchestrator-controller.Dockerfile` | `kubernaut/remediationorchestrator-controller` | ✅ Yes (E2E_COVERAGE) |
| signalprocessing | `docker/signalprocessing-controller.Dockerfile` | `kubernaut/signalprocessing-controller` | ✅ Yes |
| workflowexecution | `docker/workflowexecution-controller.Dockerfile` | `kubernaut/workflowexecution-controller` | ✅ Yes (disabled on ARM64) |
| holmesgpt-api | `holmesgpt-api/Dockerfile` | `kubernaut/holmesgpt-api` | ❌ No (Python service) |
| mock-llm | `test/services/mock-llm/Dockerfile` | `kubernaut/mock-llm` | ❌ No (test fixture) |

---

## External Dependencies

### PostgreSQL 16
- **Used by**: ALL services (via DataStorage)
- **Purpose**: Audit event storage, workflow catalog
- **Deployment**: Kind cluster (StatefulSet or Deployment)
- **Port**: NodePort 30432 (varies by service)

### Redis
- **Used by**: ALL services (via DataStorage)
- **Purpose**: DLQ fallback, caching
- **Deployment**: Kind cluster (Deployment or Sentinel HA)
- **Port**: NodePort varies by service

### Tekton Pipelines v1.7.0
- **Used by**: workflowexecution only
- **Purpose**: Workflow execution engine
- **Deployment**: Kind cluster (Tekton CRDs + controllers)
- **Note**: Uses `ghcr.io` (no auth required, unlike gcr.io since 2025)

---

## Build Order for CI/CD

Based on dependencies, the optimal build order for parallel execution:

### Tier 1: Standalone (no service dependencies)
```bash
datastorage
mock-llm
```

### Tier 2: Depends on Tier 1
```bash
gateway (depends on datastorage)
authwebhook (depends on datastorage)
signalprocessing (depends on datastorage)
holmesgpt-api (depends on datastorage + mock-llm)
```

### Tier 3: Depends on Tier 2
```bash
notification (depends on datastorage + authwebhook)
remediationorchestrator (depends on datastorage + authwebhook)
workflowexecution (depends on datastorage + authwebhook)
aianalysis (depends on datastorage + holmesgpt-api + mock-llm)
```

**CI/CD Strategy**: Build Tier 1 first, then Tier 2 & 3 in parallel (no cross-tier dependencies in 2 & 3).

---

## Image Registry Strategy

### ⚠️ **IMPORTANT: ghcr.io is CI/CD ONLY (Not Production)**

**GitHub Container Registry (ghcr.io)**: Used ONLY for CI/CD ephemeral images
- ✅ **Purpose**: E2E testing in GitHub Actions
- ✅ **Benefits**: Saves ~60% disk space on runners, faster than local builds
- ✅ **Retention**: Auto-cleanup after 14 days (GitHub policy for untagged images)
- ❌ **NOT for production releases** (use Quay.io for production)

**Quay.io**: Reserved for production releases (separate workflow, future)
- ✅ **Purpose**: Long-term stable releases, Red Hat ecosystem integration
- ✅ **Benefits**: Advanced security scanning, OperatorHub compatibility
- ⏭️ **Status**: Not yet implemented (will be added when needed)

---

### CI/CD Registry Configuration (ghcr.io)

**Base URL**: `ghcr.io/<owner>/kubernaut/<service>`

**Tagging Convention**:
```bash
# For Pull Requests (ephemeral, auto-cleanup after 14 days)
ghcr.io/jordigilh/kubernaut/datastorage:pr-123
ghcr.io/jordigilh/kubernaut/gateway:pr-123

# For main branch commits (ephemeral, auto-cleanup after 14 days)
ghcr.io/jordigilh/kubernaut/datastorage:main-abc1234
ghcr.io/jordigilh/kubernaut/gateway:main-abc1234
```

**GitHub Actions Configuration**:
```yaml
# Automatic in CI/CD pipeline
env:
  IMAGE_REGISTRY: ghcr.io/${{ github.repository_owner }}/kubernaut
  IMAGE_TAG: pr-${{ github.event.pull_request.number }}

permissions:
  packages: write  # Required for ghcr.io push
```

**E2E Test Configuration**:
```bash
# Environment variables for E2E tests (set automatically by CI)
IMAGE_REGISTRY=ghcr.io/jordigilh/kubernaut
IMAGE_TAG=pr-123

# E2E infrastructure checks these env vars:
# - If set: Pull from ghcr.io (CI/CD mode, fast)
# - If unset: Build locally (local dev mode, existing behavior)
```

---

## Infrastructure Bootstrap Patterns

### Hybrid Parallel Pattern (AUTHORITATIVE)
All services use the **hybrid parallel pattern** (per DD-TEST-002):

```
Phase 1: Build images IN PARALLEL (BEFORE cluster creation)
    └─ Eliminates cluster idle time (~18% faster)
Phase 2: Create Kind cluster (after builds complete)
Phase 3: Load images to cluster IN PARALLEL
Phase 4: Deploy services (PostgreSQL + Redis in parallel)
Phase 5: Wait for services ready
```

**Authority**: `docs/handoff/E2E_PATTERN_PERFORMANCE_ANALYSIS_JAN07.md`

**Reference Implementation**: 
- `test/infrastructure/remediationorchestrator_e2e_hybrid.go` (authoritative)
- `test/infrastructure/workflowexecution_e2e_hybrid.go`
- `test/infrastructure/gateway_e2e.go`

---

## E2E Test Port Allocation (DD-TEST-001)

Each service uses a unique port range to avoid conflicts in parallel E2E execution:

| Service | API Port (NodePort → Host) | Metrics Port | DataStorage Port |
|---------|----------------------------|--------------|------------------|
| datastorage | 30081 → 28090 | 30181 | N/A (self) |
| gateway | 30080 → 8080 | 9080 | 30081 |
| aianalysis | 30184 → 8184 | 9184 | 30081 |
| authwebhook | (varies) | (varies) | 30081 |
| notification | (varies) | (varies) | (shared DS) |
| remediationorchestrator | 30083 → (varies) | 30183 | 30081 |
| signalprocessing | 30082 → (varies) | 30182 | 30081 |
| workflowexecution | (varies) | 30185 → 9185 | 30081 |
| holmesgpt-api | 30088 → 8088 | (varies) | 30081 |

**Authority**: DD-TEST-001 (Port Allocation Standard)

---

## Coverage Collection (DD-TEST-007)

Services with coverage instrumentation support:
- ✅ datastorage
- ✅ gateway
- ✅ aianalysis
- ✅ authwebhook
- ✅ remediationorchestrator
- ✅ signalprocessing
- ✅ workflowexecution (disabled on ARM64 due to Go runtime crash)

**Coverage Directory**: `coverdata/` (mounted in Kind cluster at `/coverdata`)

**Environment Variable**: `E2E_COVERAGE=true` (enables build-time instrumentation)

---

## Usage Examples

### Local E2E Test Execution (Current Pattern)
```bash
# Run E2E tests with local image builds
make test-e2e-gateway
make test-e2e-aianalysis
make test-e2e-workflowexecution
```

### CI/CD E2E Test Execution (Registry Pattern - Automatic)
```bash
# In GitHub Actions CI/CD:
# IMAGE_REGISTRY and IMAGE_TAG are set automatically by the workflow
# E2E infrastructure detects these and pulls from ghcr.io instead of building

# Example (manual test with registry):
export IMAGE_REGISTRY=ghcr.io/jordigilh/kubernaut
export IMAGE_TAG=pr-123
make test-e2e-gateway

# Example (local dev without registry):
unset IMAGE_REGISTRY IMAGE_TAG
make test-e2e-gateway  # Builds locally as before
```

---

## Related Documentation

### Design Decisions
- **DD-TEST-001**: Port Allocation Standard for E2E Tests
- **DD-TEST-002**: Parallel Test Execution Standard (Hybrid Pattern)
- **DD-TEST-007**: E2E Coverage Capture Standard
- **DD-TEST-008**: Disk Space Management (Image Export + Prune)
- **DD-TEST-009**: Build Timeout Prevention (15min timeout)

### Infrastructure Code
- `test/infrastructure/e2e_images.go` - Consolidated image build API
- `test/infrastructure/*_e2e*.go` - Service-specific infrastructure setup
- `test/infrastructure/shared_integration_utils.go` - Shared utilities

### Performance Analysis
- `docs/handoff/E2E_PATTERN_PERFORMANCE_ANALYSIS_JAN07.md` - Hybrid pattern validation

---

## Maintenance Notes

### Adding New E2E Service Dependencies

When a new service is added:

1. **Update this matrix** with service dependencies
2. **Create Dockerfile** in `docker/<service>.Dockerfile`
3. **Add infrastructure bootstrap** in `test/infrastructure/<service>_e2e.go`
4. **Follow hybrid parallel pattern** (build → cluster → load → deploy)
5. **Allocate unique ports** per DD-TEST-001
6. **Add to CI/CD pipeline** in `.github/workflows/ci-pipeline.yml`

### Dependency Changes

If a service's dependencies change:
1. Update this matrix
2. Update infrastructure bootstrap code
3. Update CI/CD build tier if necessary
4. Re-validate E2E tests pass with new dependencies

---

## GitHub Container Registry (ghcr.io) - CI/CD Details

### Authentication (Automatic in GitHub Actions)
```yaml
- name: Log in to GitHub Container Registry
  uses: docker/login-action@v3
  with:
    registry: ghcr.io
    username: ${{ github.actor }}
    password: ${{ secrets.GITHUB_TOKEN }}  # Automatically provided
```

### Image Pull in Kind Cluster (if using private images)
```bash
# Create imagePullSecret in Kind cluster
kubectl create secret docker-registry ghcr-secret \
  --docker-server=ghcr.io \
  --docker-username=${{ github.actor }} \
  --docker-password=${{ secrets.GITHUB_TOKEN }} \
  --namespace=kubernaut-system

# Reference in Deployment:
# spec.imagePullSecrets:
#   - name: ghcr-secret
```

### Disk Space Savings (ghcr.io vs Local Build)
| Operation | Local Build | ghcr.io Pull | Savings |
|-----------|-------------|--------------|---------|
| 10 Images | ~15GB | ~5GB | ~60% |
| Build Time | ~10-15 min | ~3-5 min | ~50% |
| Kind Load | Tar export required | Direct pull | Faster |

### Image Cleanup Policy
- **Untagged images**: Auto-deleted after 14 days (GitHub policy)
- **Tagged images**: Persist until manually deleted
- **PR images**: Untagged after PR merge → auto-cleanup in 14 days
- **Main images**: Untagged after new commit → auto-cleanup in 14 days

---

**Maintained By**: Platform Team  
**Last Review**: January 26, 2026  
**Next Review**: When new services are added or dependencies change

**Registry Status**:
- ✅ **ghcr.io**: Active (CI/CD only, ephemeral images)
- ⏭️ **Quay.io**: Planned (production releases, not yet implemented)
