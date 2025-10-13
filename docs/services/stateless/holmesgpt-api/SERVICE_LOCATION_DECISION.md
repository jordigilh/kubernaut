# HolmesGPT API Service - Location Decision

**Decision Date**: October 13, 2025
**Status**: ✅ Final Location Determined
**Location**: `holmesgpt-api/` (root-level)

---

## Final Project Structure

```
kubernaut/                               # Main repository
│
├── cmd/                                 # Go binaries
│   ├── kubernaut/
│   ├── gateway/
│   └── ...
│
├── pkg/                                 # Go packages
│   ├── ai/
│   ├── workflow/
│   ├── platform/
│   └── ...
│
├── internal/                            # Go internal packages
│   ├── config/
│   ├── database/
│   └── ...
│
├── holmesgpt-api/                      # ✅ Python service (root-level, repo-ready)
│   ├── src/                            # Application code
│   │   ├── main.py
│   │   ├── api/v1/
│   │   ├── services/
│   │   ├── models/
│   │   └── config/
│   ├── tests/                          # Test suite
│   │   ├── unit/
│   │   ├── integration/
│   │   └── e2e/
│   ├── deploy/                         # Self-contained deployment
│   │   ├── kubernetes/
│   │   │   ├── deployment.yaml
│   │   │   ├── service.yaml
│   │   │   └── configmap.yaml
│   │   └── docker-compose.yml
│   ├── docs/                           # Service-specific docs
│   │   ├── DD-001-to-DD-010.md
│   │   ├── HANDOFF_SUMMARY.md
│   │   ├── API_REFERENCE.md
│   │   └── DEPLOYMENT_GUIDE.md
│   ├── Dockerfile
│   ├── requirements.txt
│   ├── pyproject.toml
│   └── README.md
│
├── dependencies/                        # External dependencies
│   └── holmesgpt/                      # HolmesGPT SDK submodule
│
├── docs/                                # Project-wide documentation
│   ├── architecture/
│   ├── requirements/
│   └── services/
│
├── config/                              # Shared configuration
├── deploy/                              # Monorepo deployment (Go services)
├── test/                                # Go test suite
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## Why Root-Level Location?

### 1. Only Non-Go Service ✅
**Observation**: HolmesGPT API is the **only Python service** in the entire codebase.

**Decision**: No need for intermediate grouping directory (`services/`, `python-services/`, etc.)

**Benefit**: Simpler structure, obvious separation from Go code

---

### 2. Clear Separation from Go Codebase ✅
**Observation**: Python code should not be in Go package structure (`pkg/`, `internal/`, `cmd/`)

**Decision**: Place at root level, clearly separate from all Go directories

**Benefit**: No confusion about language boundaries, obvious it's external

---

### 3. Repository-Ready Structure ✅
**Observation**: Service will eventually be hosted in its own repository

**Decision**: Structure it at root level, already formatted as standalone repo

**Benefit**: Extraction is trivial - just copy entire directory

```bash
# Future extraction (extremely simple)
cp -r holmesgpt-api/ /path/to/new-repo/
cd /path/to/new-repo/
git init
# Done! Already structured as standalone repository
```

---

### 4. Self-Contained Service ✅
**Observation**: Microservice should be independently deployable and documentable

**Decision**: Include own `deploy/`, `docs/`, `tests/` directories within service

**Benefit**: All service artifacts travel together, no external dependencies for deployment

---

## Alternative Locations Rejected

### ❌ `pkg/services/holmesgpt-api/`
**Why Rejected**: Python code doesn't belong in Go `pkg/` structure
**User Feedback**: "provide a different location for holmesgpt-api than the one you suggested, since holmesgpt-api is the only non go code"

---

### ❌ `services/holmesgpt-api/`
**Why Rejected**: Unnecessary intermediate directory for single service
**User Feedback**: Implied by request for "different location" - single service doesn't need grouping directory

---

### ❌ `python-services/holmesgpt-api/`
**Why Rejected**: Over-engineering for single Python service
**Reasoning**: Creates directory hierarchy for future services that don't exist

---

### ❌ `docker/holmesgpt-api/`
**Why Rejected**: Legacy location, not following new template methodology
**Reasoning**: Legacy code never tested in production, rebuilding from scratch

---

## Implementation Commands

### Local Development
```bash
cd holmesgpt-api
pip install -r requirements.txt
pytest
uvicorn src.main:app --reload
```

### Docker Build
```bash
cd holmesgpt-api
docker build -t holmesgpt-api:v1.0 .
```

### Kubernetes Deployment
```bash
kubectl apply -f holmesgpt-api/deploy/kubernetes/
```

### Testing
```bash
cd holmesgpt-api
pytest tests/unit/        # Unit tests (70% coverage)
pytest tests/integration/ # Integration tests (20% coverage)
pytest tests/e2e/         # E2E tests (10% coverage)
```

---

## Integration with Kubernaut

### How Go Services Will Consume HolmesGPT API

**AI Analysis Service** (Phase 3, Go code):
```go
// pkg/ai/analysis/holmesgpt_client.go
package analysis

import (
    "net/http"
    "encoding/json"
)

type HolmesGPTClient struct {
    baseURL string
    httpClient *http.Client
}

func (c *HolmesGPTClient) Investigate(ctx context.Context, alert Alert) (*InvestigationResult, error) {
    // Call HolmesGPT API at holmesgpt-api service endpoint
    resp, err := c.httpClient.Post(
        "http://holmesgpt-api.kubernaut.svc.cluster.local:8090/api/v1/investigate",
        "application/json",
        alert.ToJSON(),
    )
    // ... handle response
}
```

**Kubernetes Service Discovery**:
```yaml
# holmesgpt-api/deploy/kubernetes/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: holmesgpt-api
  namespace: kubernaut
spec:
  selector:
    app: holmesgpt-api
  ports:
  - name: http
    port: 8090
    targetPort: 8090
  - name: metrics
    port: 9091
    targetPort: 9091
```

**Go services access via**: `http://holmesgpt-api.kubernaut.svc.cluster.local:8090`

---

## Future Repository Extraction

### When Ready to Move to Separate Repository

**Step 1**: Copy entire directory
```bash
cp -r holmesgpt-api/ /path/to/holmesgpt-api-repo/
```

**Step 2**: Initialize new repository
```bash
cd /path/to/holmesgpt-api-repo/
git init
git add .
git commit -m "Initial commit: HolmesGPT API Service v1.0"
git remote add origin git@github.com:org/holmesgpt-api.git
git push -u origin main
```

**Step 3**: Update Kubernaut monorepo
```bash
# In kubernaut monorepo
git rm -r holmesgpt-api/
git submodule add git@github.com:org/holmesgpt-api.git holmesgpt-api
git commit -m "Move HolmesGPT API to separate repository"
```

**No restructuring needed** - service is already formatted correctly!

---

## Design Decision Reference

This location decision is documented as **DD-001** in the implementation plan:

### DD-001: Service Location - Root Level (Only Non-Go Service)
- **Rationale**: Only Python service in codebase, will eventually be own repository, no intermediate directory needed
- **Decision**: `holmesgpt-api/` at root level (simple, repo-ready, clearly separate from Go)
- **Benefits**:
  - Easy extraction to separate repository
  - Clear separation between Python and Go services
  - Self-contained with own deployment, docs, and tests
  - No over-engineering for single service

---

## Summary

| Aspect | Decision |
|--------|----------|
| **Location** | `holmesgpt-api/` (root-level) |
| **Rationale** | Only non-Go service, no intermediate directory needed |
| **Structure** | Self-contained with own deploy/, docs/, tests/ |
| **Access** | Via Kubernetes service: `holmesgpt-api.kubernaut.svc.cluster.local:8090` |
| **Future** | Trivial extraction to separate repository |
| **Commands** | All relative to `holmesgpt-api/` directory |

---

**Status**: ✅ **APPROVED - Ready for Implementation**
**Next Step**: Begin Day 1 Analysis Phase (APDC-A) following implementation plan

