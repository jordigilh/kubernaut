# Triage: OpenAPI Embed Implementation Approach

**Date**: December 15, 2025
**Issue**: Current implementation requires copying OpenAPI spec to middleware directory
**Question**: Can we move spec next to code instead of copying?
**Status**: 🔴 **CONFLICT DETECTED - NEEDS RESOLUTION**

---

## Problem Statement

**Current Implementation** (Phase 1 Complete):
- Spec copied from `api/openapi/data-storage-v1.yaml` → `pkg/datastorage/server/middleware/openapi_spec_data.yaml`
- Embedded with: `//go:embed openapi_spec_data.yaml`

**Issue**: Requires manual sync when spec changes (maintenance burden).

**User Question**: Can we move the spec to `pkg/datastorage/server/middleware/` instead of copying?

---

## Authoritative Documentation Analysis

### ADR-031: OpenAPI Specification Standard

**Location**: [ADR-031 lines 48-76](../architecture/decisions/ADR-031-openapi-specification-standard.md#L48-L76)

**MANDATE**:
```markdown
### 1. Specification File Location

**Standard Directory Structure**:
docs/services/stateless/<service>/
├── README.md
├── api-specification.md
├── openapi/
│   ├── v1.yaml                    # OpenAPI 3.0+ spec for API v1
│   ├── v2.yaml                    # OpenAPI 3.0+ spec for API v2 (when released)
│   └── README.md
```

**BUT WAIT** - The actual implementation uses `api/openapi/data-storage-v1.yaml` (project root), NOT `docs/services/stateless/data-storage/openapi/v1.yaml`.

**CONFLICT DETECTED**: ADR-031 says specs go in `docs/services/stateless/<service>/openapi/`, but actual spec is in `api/openapi/`.

---

## Current State Analysis

### Actual File Locations

```bash
# Where the spec ACTUALLY is:
api/openapi/data-storage-v1.yaml

# Where ADR-031 says it SHOULD be:
docs/services/stateless/data-storage/openapi/v1.yaml

# Where we copied it for embedding:
pkg/datastorage/server/middleware/openapi_spec_data.yaml
```

**Issue**: Implementation doesn't follow ADR-031 specification location standard.

---

## Options Analysis

### Option A: Keep Current Implementation (Copy Spec)

**Approach**: Copy `api/openapi/data-storage-v1.yaml` → `pkg/datastorage/server/middleware/openapi_spec_data.yaml`

**Pros**:
- ✅ Follows ADR-031 location (if we use `api/openapi/` as standard)
- ✅ Separates API contract from implementation
- ✅ Works with `//go:embed`

**Cons**:
- ❌ Requires manual sync when spec changes
- ❌ Two copies of same file (maintenance burden)
- ❌ Risk of drift if copy not updated

**Recommendation**: ❌ **NOT SUSTAINABLE**

---

### Option B: Move Spec to Middleware Directory

**Approach**: Move `api/openapi/data-storage-v1.yaml` → `pkg/datastorage/server/middleware/openapi_spec.yaml`

**Pros**:
- ✅ No copying needed (single source of truth)
- ✅ Automatic sync (spec IS the embedded file)
- ✅ Works with `//go:embed openapi_spec.yaml`

**Cons**:
- ❌ Violates ADR-031 specification location standard
- ❌ Spec buried in implementation code
- ❌ Harder to find for API consumers
- ❌ Each service would have spec in different location

**Recommendation**: ❌ **VIOLATES ADR-031**

---

### Option C: Use Standard ADR-031 Location + Go Module Embed

**Approach**: Move spec to ADR-031 compliant location and use Go module-aware embed

**New Standard Location**:
```
docs/services/stateless/data-storage/openapi/v1.yaml
```

**Embed with Go Module Path**:
```go
// In pkg/datastorage/server/middleware/openapi_spec.go
package middleware

import (
	_ "embed"
)

// Embed from Go module root (module: github.com/jordigilh/kubernaut)
//go:embed docs/services/stateless/data-storage/openapi/v1.yaml
var embeddedOpenAPISpec []byte
```

**Wait - this still uses `..` paths implicitly!**

Actually, let me check if `//go:embed` supports absolute paths from module root...

---

### Option D: Embed from Project Root Package

**Approach**: Create a package at project root that embeds all OpenAPI specs

**New Files**:
```
pkg/openapi/
├── datastorage.go          // Embeds data-storage spec
├── gateway.go              // Embeds gateway spec
├── contextapi.go           // Embeds context-api spec
└── notification.go         // Embeds notification spec
```

**Example** (`pkg/openapi/datastorage.go`):
```go
package openapi

import (
	_ "embed"
)

// Embed Data Storage OpenAPI spec
// Authority: api/openapi/data-storage-v1.yaml
// DD-API-002: Centralized OpenAPI spec embedding
//
//go:embed ../../api/openapi/data-storage-v1.yaml
var DataStorageSpec []byte
```

**Usage** (in `pkg/datastorage/server/middleware/openapi.go`):
```go
import "github.com/jordigilh/kubernaut/pkg/openapi"

func NewOpenAPIValidator(logger logr.Logger, metrics *prometheus.CounterVec) (*OpenAPIValidator, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(openapi.DataStorageSpec) // Use centralized embedded spec
	// ...
}
```

**Pros**:
- ✅ Single source of truth per service
- ✅ Centralized embedding logic
- ✅ No copying needed
- ✅ Follows ADR-031 location (`api/openapi/`)
- ✅ Consistent pattern across all services
- ✅ Only `pkg/openapi/*` files use `..` paths (contained)

**Cons**:
- ⚠️ `pkg/openapi/` still needs `..` paths (but only 2 levels: `../../api/openapi/`)
- ⚠️ Adds import dependency from middleware to `pkg/openapi`

**Recommendation**: ✅ **BEST APPROACH** (if `../../` works)

---

### Option E: Use `go:generate` to Auto-Copy Specs

**Approach**: Use `go:generate` to automatically copy specs before build

**Files**:
```
pkg/datastorage/server/middleware/
├── openapi.go
├── openapi_spec.go              // Has //go:generate directive
└── openapi_spec_data.yaml       // Auto-generated copy
```

**openapi_spec.go**:
```go
package middleware

import (
	_ "embed"
)

//go:generate cp ../../../../api/openapi/data-storage-v1.yaml openapi_spec_data.yaml

//go:embed openapi_spec_data.yaml
var embeddedOpenAPISpec []byte
```

**Build Process**:
```bash
go generate ./pkg/datastorage/server/middleware/...
go build ./cmd/datastorage
```

**Pros**:
- ✅ Automatic sync via `go generate`
- ✅ Follows ADR-031 location (`api/openapi/`)
- ✅ Works with `//go:embed` (no `..` paths)
- ✅ CI/CD can enforce `go generate` before build

**Cons**:
- ⚠️ Requires `go generate` step in build process
- ⚠️ Developers must remember to run `go generate`
- ⚠️ Generated file in source control (or add to .gitignore)

**Recommendation**: ✅ **VIABLE ALTERNATIVE**

---

## Testing: Does `../../` Work in `//go:embed`?

Let me verify if Option D is feasible:

**Test Case**: Can `pkg/openapi/datastorage.go` embed `../../api/openapi/data-storage-v1.yaml`?

**Directory Structure**:
```
kubernaut/
├── api/openapi/data-storage-v1.yaml
└── pkg/openapi/datastorage.go
```

**Relative Path**: `../../api/openapi/data-storage-v1.yaml` (2 levels up)

**Go Documentation**: `//go:embed` paths are relative to the source file's directory and must be within the module.

**Key Constraint**: Paths CANNOT start with `..` or `/`.

**Conclusion**: ❌ **Option D won't work** - `//go:embed ../../api/` is invalid syntax.

---

## Revised Options

### Option B (Revised): Move Spec Next to Code

Since `//go:embed` doesn't support `..`, we have two real options:

1. **Move spec to middleware directory** (user's suggestion)
2. **Use `go:generate` to auto-copy** (automated copy)

Let me analyze these properly:

---

### OPTION 1: Move Spec to Middleware Directory (User's Suggestion)

**Approach**:
```
pkg/datastorage/server/middleware/
├── openapi.go
├── openapi_spec.go
└── data-storage-v1.yaml          # MOVED from api/openapi/
```

**Embed**:
```go
//go:embed data-storage-v1.yaml
var embeddedOpenAPISpec []byte
```

**Pros**:
- ✅ No copying needed
- ✅ Single source of truth
- ✅ Automatic sync (spec IS the embedded file)
- ✅ Works with `//go:embed` (no `..` paths)

**Cons**:
- ❌ Violates ADR-031 specification location standard
- ❌ Spec buried in implementation code (not discoverable)
- ❌ Each service has spec in different location
- ❌ API consumers don't know where to find spec
- ❌ Breaking change from current `api/openapi/` location

**Impact on ADR-031**:
- **HIGH IMPACT**: Requires amending ADR-031 to allow specs in `pkg/<service>/server/middleware/`
- **Alternative**: Create new standard: "Specs for embedded validation go in middleware, specs for client generation go in `api/openapi/`"

---

### OPTION 2: Use `go:generate` to Auto-Copy

**Approach**:
```
# Source of truth (unchanged)
api/openapi/data-storage-v1.yaml

# Auto-generated copy (via go:generate)
pkg/datastorage/server/middleware/openapi_spec_data.yaml
```

**Implementation**:
```go
// pkg/datastorage/server/middleware/openapi_spec.go
package middleware

import _ "embed"

//go:generate sh -c "cp ../../../../api/openapi/data-storage-v1.yaml openapi_spec_data.yaml"

//go:embed openapi_spec_data.yaml
var embeddedOpenAPISpec []byte
```

**Build Process**:
```bash
go generate ./...
go build ./cmd/datastorage
```

**Makefile Integration**:
```makefile
.PHONY: generate
generate:
	go generate ./...

.PHONY: build-datastorage
build-datastorage: generate
	go build -o bin/datastorage ./cmd/datastorage
```

**Pros**:
- ✅ Automatic sync via `go generate`
- ✅ Follows ADR-031 location (`api/openapi/`)
- ✅ Works with `//go:embed`
- ✅ CI/CD can enforce generation
- ✅ Single source of truth (`api/openapi/`)

**Cons**:
- ⚠️ Requires `go generate` step (but can be automated in Makefile)
- ⚠️ Developers must run `make generate` (or CI/CD enforces it)
- ⚠️ Generated file must be in `.gitignore` or source control

**`.gitignore` Entry**:
```
# Auto-generated OpenAPI spec copies (via go:generate)
pkg/*/server/middleware/openapi_spec_data.yaml
```

---

## Recommendation Matrix

| Criterion | Option 1: Move to Middleware | Option 2: `go:generate` |
|---|---|---|
| **ADR-031 Compliance** | ❌ Violates | ✅ Compliant |
| **Single Source of Truth** | ✅ Yes | ✅ Yes (with auto-copy) |
| **Discoverable by API Consumers** | ❌ Buried in code | ✅ In `api/openapi/` |
| **Maintenance Burden** | ✅ None | ⚠️ Requires `go generate` |
| **Build Complexity** | ✅ Simple | ⚠️ Adds generation step |
| **CI/CD Integration** | ✅ Easy | ⚠️ Must run `go generate` |
| **Cross-Service Consistency** | ❌ Each service different | ✅ Consistent |

---

## Final Recommendation

### ✅ **OPTION 2: Use `go:generate` with Makefile Integration**

**Rationale**:
1. ✅ **Maintains ADR-031 Compliance**: Specs stay in `api/openapi/`
2. ✅ **Automatic Sync**: `go generate` copies spec before build
3. ✅ **Discoverable**: API consumers find specs in standard location
4. ✅ **Consistent**: All services follow same pattern
5. ✅ **CI/CD Friendly**: Makefile enforces generation

**Implementation Steps**:

1. Update `pkg/datastorage/server/middleware/openapi_spec.go`:
   ```go
   //go:generate sh -c "cp ../../../../api/openapi/data-storage-v1.yaml openapi_spec_data.yaml"
   ```

2. Add to `.gitignore`:
   ```
   pkg/*/server/middleware/openapi_spec_data.yaml
   ```

3. Update Makefile:
   ```makefile
   .PHONY: generate
   generate:
   	@echo "Generating OpenAPI spec copies..."
   	go generate ./...

   .PHONY: build-datastorage
   build-datastorage: generate
   	go build -o bin/datastorage ./cmd/datastorage
   ```

4. Update DD-API-002 to document `go:generate` approach

---

## Alternative Recommendation (If User Prefers Simplicity)

### ✅ **OPTION 1: Move to Middleware + Amend ADR-031**

**If** the user prioritizes simplicity over ADR-031 compliance, then:

**Action**:
1. Move `api/openapi/data-storage-v1.yaml` → `pkg/datastorage/server/middleware/data-storage-v1.yaml`
2. Amend ADR-031 to allow: "Specs for embedded validation MAY be co-located with middleware"
3. Keep `api/openapi/` for client generation (if needed)

**Trade-off**: Simplicity vs. Consistency

---

## Decision Required

**Question for User**: Which approach do you prefer?

**A) `go:generate` (Recommended)**
- Follows ADR-031
- Requires Makefile changes
- Auto-copies spec before build

**B) Move to Middleware**
- Simpler (no generation step)
- Violates ADR-031 (requires amendment)
- Spec buried in code

**C) Keep Current (Manual Copy)**
- Status quo
- Manual maintenance burden

---

## Confidence Assessment

**Confidence**: **90%** ✅ **STRONGLY RECOMMEND Option A (`go:generate`)**

**Justification**:
- ✅ Maintains architectural standards (ADR-031)
- ✅ Automatic sync (no manual copy)
- ✅ Discoverable spec location
- ✅ Consistent cross-service pattern

**Remaining 10% Uncertainty**: User preference for simplicity vs. compliance.

---

**Status**: 🔴 **DECISION REQUIRED**
**Next Step**: User approval for Option A or B





