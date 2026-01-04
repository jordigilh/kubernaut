# OpenAPI Validation Strategy - Consistency Across Services

**Date**: December 31, 2025
**Issue**: Inconsistent OpenAPI management between HAPI and Data Storage
**Proposal**: Validate committed specs in CI, force developers to keep specs in sync

---

## Current State: Inconsistency

### HolmesGPT API (Code-First, FastAPI)
```yaml
# Current CI workflow:
- name: Export OpenAPI spec
  run: make export-openapi-holmesgpt-api

- name: Upload OpenAPI spec  # ❌ Just uploads, no validation
  uses: actions/upload-artifact@v4
  with:
    name: openapi-spec
    path: holmesgpt-api/api/openapi.json
```

**Problem**:
- ❌ No enforcement that committed spec is up-to-date
- ❌ Developer can forget to export and commit spec changes
- ❌ Spec drift can go unnoticed until production

---

### Data Storage (Spec-First, Manual YAML)
```yaml
# Current: No CI validation at all
# api/openapi/data-storage-v1.yaml exists but nothing checks it
```

**Problem**:
- ❌ No validation that Go code implements the spec
- ❌ No validation that spec is syntactically correct
- ❌ Manual spec updates can be forgotten

---

## Proposed Solution: **Validation, Not Artifacts**

### Core Principle
> **Git is the source of truth, CI enforces it stays in sync**

---

### Implementation: HAPI (Code-First Validation)

#### Step 1: Update Makefile Target (Simplified)

**Replace complex `.committed` file approach** with simple git diff:

```makefile
.PHONY: validate-openapi-holmesgpt-api
validate-openapi-holmesgpt-api: export-openapi-holmesgpt-api ## Validate OpenAPI spec is committed (CI - ADR-045)
	@echo "🔍 Validating OpenAPI spec is up-to-date..."
	@cd holmesgpt-api && \
	if ! git diff --quiet api/openapi.json; then \
		echo ""; \
		echo "❌ OpenAPI spec drift detected!"; \
		echo ""; \
		echo "The generated OpenAPI spec differs from the committed version."; \
		echo ""; \
		echo "📋 Changes:"; \
		git diff api/openapi.json | head -50; \
		echo ""; \
		echo "🔧 To fix:"; \
		echo "  1. Run: make export-openapi-holmesgpt-api"; \
		echo "  2. Review: git diff holmesgpt-api/api/openapi.json"; \
		echo "  3. Commit: git add holmesgpt-api/api/openapi.json"; \
		echo ""; \
		exit 1; \
	fi
	@echo "✅ OpenAPI spec is up-to-date and committed"
```

**Why this is better**:
- ✅ Uses standard git diff (no .committed file needed)
- ✅ Clear error messages with fix instructions
- ✅ Shows actual diff (first 50 lines)
- ✅ Simpler to understand and maintain

---

#### Step 2: Update CI Workflow

**Replace artifact upload with validation**:

```yaml
# In defense-in-depth-optimized.yml, integration-holmesgpt job:

# ❌ REMOVE: Artifact upload
# - name: Upload OpenAPI spec
#   uses: actions/upload-artifact@v4
#   with:
#     name: openapi-spec
#     path: holmesgpt-api/api/openapi.json

# ✅ ADD: Validation that fails CI if spec not committed
- name: Validate OpenAPI spec is committed (ADR-045)
  working-directory: holmesgpt-api
  run: |
    cd ..
    make validate-openapi-holmesgpt-api
```

---

### Implementation: Data Storage (Spec-First Validation)

#### Step 1: Add Makefile Target

```makefile
.PHONY: validate-openapi-datastorage
validate-openapi-datastorage: ## Validate Data Storage OpenAPI spec syntax (CI - ADR-031)
	@echo "🔍 Validating Data Storage OpenAPI spec..."
	@# Check YAML syntax
	@docker run --rm -v "$(PWD):/local" openapitools/openapi-generator-cli validate \
		-i /local/api/openapi/data-storage-v1.yaml || \
		(echo "❌ OpenAPI spec validation failed!" && exit 1)
	@echo "✅ Data Storage OpenAPI spec is valid"
```

#### Step 2: Add CI Validation

```yaml
# In defense-in-depth-optimized.yml, integration-datastorage job:

- name: Validate Data Storage OpenAPI spec (ADR-031)
  run: make validate-openapi-datastorage
```

---

## Benefits

### 1. **Consistency Across Services** ✅

| Service | Committed Spec | CI Validation |
|---------|---------------|---------------|
| **HAPI** | ✅ `holmesgpt-api/api/openapi.json` | ✅ Validates generated matches committed |
| **Data Storage** | ✅ `api/openapi/data-storage-v1.yaml` | ✅ Validates syntax is correct |

Both services now have:
- Committed OpenAPI specs in git
- CI validation that enforces sync
- Clear developer workflow

---

### 2. **Better Developer Experience** ✅

**Before** (current):
```
Developer changes HAPI endpoint → CI passes → spec is outdated ❌
```

**After** (proposed):
```
Developer changes HAPI endpoint → CI fails ❌
Error: "OpenAPI spec drift detected! Run 'make export-openapi-holmesgpt-api'"
Developer runs make → commits spec → CI passes ✅
```

---

### 3. **Single Source of Truth** ✅

**Before**: CI artifacts (30-day retention, hard to find)
**After**: Git repository (permanent, easy to find)

```bash
# Anyone can see current spec:
cat holmesgpt-api/api/openapi.json
cat api/openapi/data-storage-v1.yaml

# No need to download CI artifacts
```

---

### 4. **Contract Testing Foundation** ✅

With committed specs, we can add **contract testing**:

```yaml
# Future enhancement:
- name: Contract test - HAPI implements its spec
  run: make contract-test-holmesgpt-api

- name: Contract test - Data Storage implements its spec
  run: make contract-test-datastorage
```

**Contract testing**: Validate runtime API behavior matches OpenAPI spec
- HAPI: Does FastAPI app actually return what spec promises?
- DS: Does Go Chi router actually implement what spec defines?

---

## Implementation Plan

### Phase 1: HAPI Validation (Immediate) ✅

1. **Update Makefile** - Simplify `validate-openapi-holmesgpt-api` to use git diff
2. **Update CI** - Replace artifact upload with validation call
3. **Test** - Make intentional change without committing spec, verify CI fails
4. **Document** - Update ADR-045 with new validation approach

---

### Phase 2: Data Storage Validation (Immediate) ✅

1. **Add Makefile target** - `validate-openapi-datastorage` for syntax validation
2. **Add CI validation** - Call in `integration-datastorage` job
3. **Test** - Intentionally break YAML syntax, verify CI fails
4. **Document** - Update ADR-031 with validation requirement

---

### Phase 3: Contract Testing (Future V2.0) 🔮

1. **HAPI contract tests** - Use Schemathesis or similar
2. **DS contract tests** - Use OpenAPI validator middleware
3. **Add to CI** - After integration tests pass

---

## Migration Path

### Step 1: Ensure Specs Are Committed ✅

```bash
# HAPI - already committed
git ls-files holmesgpt-api/api/openapi.json  # ✅ exists

# Data Storage - already committed
git ls-files api/openapi/data-storage-v1.yaml  # ✅ exists
```

### Step 2: Update Makefile

```bash
# Apply simplified validation targets
# (see implementations above)
```

### Step 3: Update CI Workflows

```bash
# Remove artifact uploads
# Add validation calls
# (see implementations above)
```

### Step 4: Test

```bash
# HAPI test:
# 1. Change endpoint in src/
# 2. Don't run make export-openapi-holmesgpt-api
# 3. Push PR
# 4. CI should FAIL with clear instructions

# DS test:
# 1. Break YAML syntax in api/openapi/data-storage-v1.yaml
# 2. Push PR
# 3. CI should FAIL with validation error
```

---

## Answer to User's Question

> "Maybe what we should do is check if the generated openapi spec is different than the one in the commits and fail. This way we force the developer to always update the openapi spec in the PR before merging."

**Answer**: **YES, absolutely!** This is the correct approach.

### Current Problems It Solves

1. ❌ **HAPI**: Uploads to artifact but doesn't validate committed spec
2. ❌ **Data Storage**: No validation at all
3. ❌ **Inconsistency**: Two different approaches

### Your Proposal Solves This

1. ✅ **HAPI**: Validate generated spec matches committed (fail if not)
2. ✅ **Data Storage**: Validate committed spec syntax is correct
3. ✅ **Consistency**: Both use validation, both have committed specs
4. ✅ **Developer workflow**: CI forces spec updates before merge

### Implementation

**For HAPI** (code-first):
```bash
make export-openapi-holmesgpt-api  # Generate from code
git diff api/openapi.json          # Check if changed
exit 1 if changed                  # Fail CI
```

**For Data Storage** (spec-first):
```bash
openapi-generator-cli validate     # Check YAML syntax
exit 1 if invalid                  # Fail CI
```

Both approaches enforce that **git is the source of truth** and **CI validates it stays in sync**.

---

**Status**: ⏳ **READY FOR IMPLEMENTATION**
**Recommendation**: Implement both Phase 1 and Phase 2 immediately
**Benefit**: Consistent OpenAPI management across all services

