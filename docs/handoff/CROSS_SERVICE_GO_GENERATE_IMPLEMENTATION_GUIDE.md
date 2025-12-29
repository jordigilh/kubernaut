# Cross-Service go:generate Implementation Guide

**To**: All Service Teams
**From**: Data Storage Team
**Date**: December 15, 2025
**Authority**: [DD-API-002: OpenAPI Spec Loading Standard](../architecture/decisions/DD-API-002-openapi-spec-loading-standard.md)
**Status**: ✅ **REFERENCE IMPLEMENTATION COMPLETE** (Data Storage + Audit Library)

---

## 🎯 **Executive Summary**

**What**: Implement `go:generate` to automatically sync OpenAPI specs and clients

**Why**: Eliminate manual copy/regeneration work, prevent spec drift, ensure build-time safety

**Who Should Implement**:
- ✅ **Gateway Team**: Generate Data Storage client
- ✅ **SignalProcessing Team**: Generate Data Storage client
- ✅ **RemediationOrchestrator Team**: Generate Data Storage client
- ✅ **WorkflowExecution Team**: Generate Data Storage client
- ✅ **Notification Team**: Generate Data Storage client
- ✅ **AIAnalysis Team**: Generate HAPI client

**Time Required**: 15-20 minutes per service

**Reference Implementation**: Data Storage service (validation middleware) + Audit Library (spec embedding)

---

## 📚 **Table of Contents**

1. [Use Case 1: Embedding Specs for Validation Middleware](#use-case-1-embedding-specs-for-validation-middleware)
2. [Use Case 2: Generating Clients from Specs](#use-case-2-generating-clients-from-specs)
3. [Step-by-Step Implementation](#step-by-step-implementation)
4. [Verification Steps](#verification-steps)
5. [Troubleshooting](#troubleshooting)
6. [FAQ](#faq)

---

## Use Case 1: Embedding Specs for Validation Middleware

**Applies To**: Services that PROVIDE REST APIs with validation middleware

**Current Services**: Data Storage (✅ IMPLEMENTED)

**Future Services**: Context API, Notification (if adding OpenAPI validation in V2.0)

### Reference Implementation

**File**: `pkg/datastorage/server/middleware/openapi_spec.go`

```go
package middleware

import _ "embed"

// Auto-generate OpenAPI spec copy before build
// DD-API-002: OpenAPI Spec Loading Standard
//
//go:generate sh -c "cp ../../../../api/openapi/data-storage-v1.yaml openapi_spec_data.yaml"

// Embed auto-generated copy
//go:embed openapi_spec_data.yaml
var embeddedOpenAPISpec []byte
```

**Usage in Middleware**: `pkg/datastorage/server/middleware/openapi.go`

```go
func NewOpenAPIValidator(logger logr.Logger, metrics *prometheus.CounterVec) (*OpenAPIValidator, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(embeddedOpenAPISpec) // Use embedded spec
	// ...
}
```

**See Full Implementation**: [DS_OPENAPI_EMBED_GO_GENERATE_COMPLETE.md](./DS_OPENAPI_EMBED_GO_GENERATE_COMPLETE.md)

---

## Use Case 2: Generating Clients from Specs

**Applies To**: Services that CONSUME other services' REST APIs

**Current Services**: Gateway, SignalProcessing, RemediationOrchestrator, WorkflowExecution, Notification, AIAnalysis

### Reference Implementation

**For Data Storage Clients** (Gateway, SP, RO, WE, Notification):

**File**: `pkg/<your-service>/client/datastorage/generate.go` (NEW)

```go
package datastorage

// Auto-generate Data Storage client from OpenAPI spec
// DD-API-002: OpenAPI Spec Loading Standard
// Authority: api/openapi/data-storage-v1.yaml
//
// Usage: go generate ./... (or make generate)
//
//go:generate oapi-codegen -package datastorage -generate types,client ../../../../api/openapi/data-storage-v1.yaml -o client.go
```

**For HAPI Client** (AIAnalysis):

**File**: `pkg/aianalysis/client/generated/generate.go` (NEW)

```go
package generated

// Auto-generate HAPI client from OpenAPI spec
// DD-API-002: OpenAPI Spec Loading Standard
// Authority: holmesgpt-api/api/openapi.json
//
// Usage: go generate ./... (or make generate)
//
//go:generate ogen --package generated --target . --clean ../../../../../holmesgpt-api/api/openapi.json
```

---

## 📋 **Step-by-Step Implementation**

### Step 1: Determine Your Use Case

**Question**: Does your service PROVIDE or CONSUME REST APIs?

| Your Service | Use Case | Implementation Path |
|--------------|----------|-------------------|
| Gateway | CONSUME (Data Storage) | [Go to Step 2A](#step-2a-client-generation---data-storage-consumers) |
| SignalProcessing | CONSUME (Data Storage) | [Go to Step 2A](#step-2a-client-generation---data-storage-consumers) |
| RemediationOrchestrator | CONSUME (Data Storage) | [Go to Step 2A](#step-2a-client-generation---data-storage-consumers) |
| WorkflowExecution | CONSUME (Data Storage) | [Go to Step 2A](#step-2a-client-generation---data-storage-consumers) |
| Notification | CONSUME (Data Storage) | [Go to Step 2A](#step-2a-client-generation---data-storage-consumers) |
| AIAnalysis | CONSUME (HAPI) | [Go to Step 2B](#step-2b-client-generation---hapi-consumer) |

---

### Step 2A: Client Generation - Data Storage Consumers

**Applies To**: Gateway, SignalProcessing, RemediationOrchestrator, WorkflowExecution, Notification

#### 2A.1. Create `generate.go` File

```bash
# Identify your client directory
# Example for Gateway: pkg/gateway/client/datastorage/
# Example for SignalProcessing: pkg/signalprocessing/client/datastorage/

# Create generate.go file
mkdir -p pkg/<your-service>/client/datastorage
cat > pkg/<your-service>/client/datastorage/generate.go <<'EOF'
package datastorage

// Auto-generate Data Storage client from OpenAPI spec
// DD-API-002: OpenAPI Spec Loading Standard
// Authority: api/openapi/data-storage-v1.yaml
//
// Usage: go generate ./... (or make generate)
//
//go:generate oapi-codegen -package datastorage -generate types,client ../../../../api/openapi/data-storage-v1.yaml -o client.go
EOF
```

**Path Calculation**:
- From `pkg/<service>/client/datastorage/generate.go`
- To `api/openapi/data-storage-v1.yaml`
- Path: `../../../../api/openapi/data-storage-v1.yaml`

#### 2A.2. Add to Makefile

**File**: `Makefile`

Find your service's `generate` target and add client generation:

```makefile
.PHONY: generate
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./api/..."
	@echo "📋 Generating OpenAPI clients (DD-API-002)..."
	@go generate ./pkg/<your-service>/client/...
```

**If no `generate` target exists**, add one:

```makefile
.PHONY: generate
generate:
	@echo "📋 Generating OpenAPI clients (DD-API-002)..."
	@go generate ./pkg/<your-service>/client/...

.PHONY: build-<your-service>
build-<your-service>: generate  # Make build depend on generate
	go build -o bin/<your-service> ./cmd/<your-service>
```

#### 2A.3. Add to .gitignore

**File**: `.gitignore`

Add auto-generated client to ignore list:

```gitignore
# Auto-generated OpenAPI clients (via go:generate)
# DD-API-002: These are auto-generated from api/openapi/*.yaml
pkg/*/client/*/client.go
```

#### 2A.4. Test Generation

```bash
# Test go:generate directly
go generate ./pkg/<your-service>/client/...

# Verify client was generated
ls -lh pkg/<your-service>/client/datastorage/client.go

# Test via Makefile
make generate

# Test full build
make build-<your-service>
```

#### 2A.5. Update CI/CD (Optional but Recommended)

**File**: `.github/workflows/<your-service>.yml` (if exists)

Ensure `go generate` runs before build:

```yaml
- name: Generate Code
  run: make generate

- name: Build Service
  run: make build-<your-service>
```

---

### Step 2B: Client Generation - HAPI Consumer

**Applies To**: AIAnalysis

#### 2B.1. Create `generate.go` File

**File**: `pkg/aianalysis/client/generated/generate.go` (NEW)

```go
package generated

// Auto-generate HAPI client from OpenAPI spec
// DD-API-002: OpenAPI Spec Loading Standard
// Authority: holmesgpt-api/api/openapi.json
//
// Usage: go generate ./... (or make generate)
//
//go:generate ogen --package generated --target . --clean ../../../../../holmesgpt-api/api/openapi.json
```

**Path Calculation**:
- From `pkg/aianalysis/client/generated/generate.go`
- To `holmesgpt-api/api/openapi.json`
- Path: `../../../../../holmesgpt-api/api/openapi.json`

#### 2B.2. Add to Makefile

**File**: `Makefile`

```makefile
.PHONY: generate
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./api/..."
	@echo "📋 Generating OpenAPI clients (DD-API-002)..."
	@go generate ./pkg/aianalysis/client/generated/...
```

#### 2B.3. Add to .gitignore

**File**: `.gitignore`

```gitignore
# Auto-generated OpenAPI clients (via go:generate)
pkg/aianalysis/client/generated/oas_*.go
```

#### 2B.4. Test Generation

```bash
# Test generation
go generate ./pkg/aianalysis/client/generated/...

# Verify 18 generated files
ls -1 pkg/aianalysis/client/generated/oas_*.go | wc -l
# Expected: 18

# Test build
make build-aianalysis
```

---

## ✅ **Verification Steps**

### For ALL Services

#### Step 1: Test go:generate

```bash
# Remove existing generated files
rm pkg/<your-service>/client/*/client.go

# Run generation
go generate ./pkg/<your-service>/client/...

# Verify files created
ls -lh pkg/<your-service>/client/*/client.go
```

**Expected**: Client file(s) created successfully

---

#### Step 2: Test Makefile Integration

```bash
# Remove generated files
rm pkg/<your-service>/client/*/client.go

# Run make generate
make generate

# Verify files created
ls -lh pkg/<your-service>/client/*/client.go
```

**Expected**: Client file(s) created successfully

---

#### Step 3: Test Build

```bash
# Full build (should auto-generate)
make build-<your-service>
```

**Expected**: Build succeeds with no errors

---

#### Step 4: Verify Checksums (If Embedding Specs)

```bash
# Compare source and auto-generated copy
md5 api/openapi/<service>-v1.yaml pkg/<service>/server/middleware/openapi_spec_data.yaml
```

**Expected**: Identical MD5 checksums

---

#### Step 5: Run Tests

```bash
# Unit tests
go test ./pkg/<your-service>/...

# Integration tests (if applicable)
make test-<your-service>-integration
```

**Expected**: All tests pass

---

## 🔧 **Troubleshooting**

### Issue 1: go:generate Command Not Found

**Error**:
```
-bash: oapi-codegen: command not found
```

**Solution**:
```bash
# Install oapi-codegen
go install github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen@latest

# Or for ogen (AIAnalysis)
go install github.com/ogen-go/ogen/cmd/ogen@latest

# Verify installation
which oapi-codegen
```

---

### Issue 2: Invalid Pattern Syntax

**Error**:
```
pattern ../../../../api/openapi/data-storage-v1.yaml: invalid pattern syntax
```

**Cause**: Using `..` in `//go:embed` directive (not supported)

**Solution**: Use `//go:generate` to copy file first (as shown in examples above)

---

### Issue 3: Client File Not Generated

**Symptom**: `go generate` runs but no client.go created

**Debug Steps**:
```bash
# Run with verbose output
go generate -v ./pkg/<your-service>/client/...

# Check working directory
pwd

# Verify relative path
ls ../../../../api/openapi/data-storage-v1.yaml
```

**Solution**: Verify relative path is correct from `generate.go` location

---

### Issue 4: Build Fails with Missing Types

**Error**:
```
undefined: datastorage.AuditEventRequest
```

**Cause**: Generated client not being imported or regenerated

**Solution**:
```bash
# Force regeneration
rm pkg/<your-service>/client/datastorage/client.go
make generate
make build-<your-service>
```

---

### Issue 5: Git Merge Conflicts on Generated Files

**Symptom**: Merge conflicts in `client.go`

**Solution**: Add to `.gitignore` (recommended)

```gitignore
# Auto-generated OpenAPI clients
pkg/*/client/*/client.go
```

Then regenerate locally:
```bash
make generate
```

---

## ❓ **FAQ**

### Q1: Do I commit generated files to Git?

**Answer**: **NO** (Recommended)

**Rationale**:
- Generated files cause merge conflicts
- Bloat Git history with large diffs
- CI/CD can regenerate automatically

**Implementation**:
1. Add to `.gitignore`
2. Ensure CI/CD runs `make generate`
3. Developers run `make generate` locally before build

---

### Q2: What if the OpenAPI spec changes?

**Answer**: **Automatic regeneration on next build**

**Workflow**:
1. Data Storage team updates `api/openapi/data-storage-v1.yaml`
2. Commits and pushes change
3. Your service's next build runs `make generate`
4. Client auto-regenerates with new spec
5. Compilation errors caught immediately if breaking changes

**No manual action needed!**

---

### Q3: How do I know if my client is out of date?

**Answer**: **Build will fail**

**Example**:
```bash
make build-gateway
# If client outdated:
# Error: undefined: datastorage.NewFieldName
```

**Solution**: `make generate` and update your code

---

### Q4: Can I customize the generated client?

**Answer**: **YES, but avoid editing generated files directly**

**Recommended Approach**:
1. Create a wrapper/adapter in a separate file
2. Keep generated `client.go` pristine
3. Use adapter to customize behavior

**Example**:
```go
// pkg/gateway/client/datastorage/adapter.go (NOT generated)
package datastorage

func NewClientWithRetry(baseURL string) (*Client, error) {
    client, err := NewClient(baseURL) // Generated function
    // Add custom retry logic
    return client, err
}
```

---

### Q5: What if I need to debug the generation process?

**Answer**: Run `go generate` with verbose flag

```bash
go generate -v ./pkg/<your-service>/client/...
```

Or run the command directly:
```bash
cd pkg/<your-service>/client/datastorage
oapi-codegen -package datastorage -generate types,client ../../../../api/openapi/data-storage-v1.yaml -o client.go
```

---

### Q6: How often should I regenerate?

**Answer**: **Before every build (automated via Makefile)**

**Manual Regeneration**: Only needed if:
- Testing spec changes locally
- Troubleshooting generation issues
- Verifying custom changes

**Otherwise**: Let Makefile handle it automatically

---

### Q7: What if my service consumes multiple APIs?

**Answer**: Add multiple `go:generate` directives

**Example**:
```go
// pkg/<your-service>/client/generate.go
package client

// Generate Data Storage client
//go:generate oapi-codegen -package datastorage -generate types,client ../../../api/openapi/data-storage-v1.yaml -o datastorage/client.go

// Generate Context API client
//go:generate oapi-codegen -package contextapi -generate types,client ../../../api/openapi/context-api-v1.yaml -o contextapi/client.go
```

---

## 📊 **Implementation Checklist**

Use this checklist to track your implementation:

### For Your Service

- [ ] **Step 1**: Created `generate.go` file with `//go:generate` directive
- [ ] **Step 2**: Verified relative path to OpenAPI spec is correct
- [ ] **Step 3**: Added generation to Makefile `generate` target
- [ ] **Step 4**: Made build target depend on `generate`
- [ ] **Step 5**: Added generated files to `.gitignore`
- [ ] **Step 6**: Tested `go generate` command directly
- [ ] **Step 7**: Tested `make generate` command
- [ ] **Step 8**: Tested `make build-<service>` command
- [ ] **Step 9**: Verified all unit tests pass
- [ ] **Step 10**: Verified integration tests pass (if applicable)
- [ ] **Step 11**: Updated CI/CD to run `make generate` before build
- [ ] **Step 12**: Documented changes in service README
- [ ] **Step 13**: Notified team of new workflow

---

## 🎯 **Success Metrics**

Your implementation is successful when:

1. ✅ `make generate` creates client automatically
2. ✅ `make build-<service>` succeeds with auto-generation
3. ✅ All tests pass with regenerated client
4. ✅ No manual client regeneration commands needed
5. ✅ Team understands new workflow

---

## 📚 **References**

- [DD-API-002: OpenAPI Spec Loading Standard](../architecture/decisions/DD-API-002-openapi-spec-loading-standard.md)
- [ADR-031: OpenAPI Specification Standard](../architecture/decisions/ADR-031-openapi-specification-standard.md)
- [DS_OPENAPI_EMBED_GO_GENERATE_COMPLETE.md](./DS_OPENAPI_EMBED_GO_GENERATE_COMPLETE.md) - Reference implementation
- [TRIAGE_GO_GENERATE_CROSS_SERVICE_APPLICABILITY.md](./TRIAGE_GO_GENERATE_CROSS_SERVICE_APPLICABILITY.md) - Analysis
- [Go generate documentation](https://go.dev/blog/generate)
- [oapi-codegen documentation](https://github.com/deepmap/oapi-codegen)
- [ogen documentation](https://github.com/ogen-go/ogen)

---

## 💬 **Support**

### Questions or Issues?

1. **Check**: This guide's [Troubleshooting](#troubleshooting) and [FAQ](#faq) sections
2. **Reference**: Data Storage implementation in `pkg/datastorage/server/middleware/`
3. **Ask**: Post in `#architecture` Slack channel with:
   - Your service name
   - Error message (if any)
   - What you've tried

### Report Issues with This Guide

- **File**: `docs/handoff/CROSS_SERVICE_GO_GENERATE_IMPLEMENTATION_GUIDE.md`
- **Owner**: Data Storage Team
- **Contact**: Via `#data-storage` Slack channel

---

**Status**: ✅ **READY FOR IMPLEMENTATION**
**Reference Implementation**: Data Storage + Audit Library (COMPLETE)
**Your Action**: Follow Step-by-Step Implementation for your service





