# Dynamic Toolset Reference Cleanup - December 20, 2025

**Date**: December 20, 2025
**Status**: 🔄 **IN PROGRESS**
**Purpose**: Remove all Dynamic Toolset references from V1.0 documentation to avoid confusion

---

## 📋 **Executive Summary**

**Code Deletion**: ✅ Complete (commit 9f0c2c71)
- Deleted `pkg/toolset/` (~1,722 LOC)
- Deleted `cmd/dynamictoolset/main.go`
- Added deprecation notices

**Documentation Cleanup**: 🔄 In Progress
- **Main README.md**: 6 references found
- **Other docs**: 157 files contain references (mostly in `docs/services/stateless/dynamic-toolset/` - intentionally preserved)

---

## 🎯 **Cleanup Strategy**

### **What to Remove**
1. ✅ **Main README.md** - All references (service status, build commands, test counts)
2. ✅ **Makefile** - Build and test targets
3. ✅ **Go.mod** - Any toolset-specific dependencies (if any)
4. ⚠️ **docs/services/stateless/README.md** - Remove from service list
5. ⚠️ **docs/README.md** - Update service documentation navigation

### **What to Preserve**
- ✅ **docs/services/stateless/dynamic-toolset/** - All docs (historical reference, V2.0 planning)
- ✅ **deploy/dynamic-toolset/** - Deployment manifests (reference architecture)
- ✅ **DD-016** - Design decision (authoritative deferral rationale)
- ✅ **test/infrastructure/toolset.go** - May be used by other services (needs verification)

---

## 📝 **Main README.md Changes Required**

### **Issue #1: Service Status Table (Line 79)**
**Current**:
```markdown
| **~~Dynamic Toolset~~** | ❌ **Deferred to V2.0** | Service discovery (DD-016) | 8 BRs (redundant with HolmesGPT-API) |
```

**Fix**: **DELETE entire line**

**Rationale**: User wants to avoid confusion - service won't be in v1.x, reimplementation depends on V1.0 feedback

---

### **Issue #2: Service Count Comment (Line 100)**
**Current**:
```markdown
- 📊 **V1.0 Service Count**: 8 production-ready services (11 original - Context API deprecated - Dynamic Toolset deferred to V2.0 - Effectiveness Monitor deferred to V1.1)
```

**Fix**:
```markdown
- 📊 **V1.0 Service Count**: 8 production-ready services (10 original - Context API deprecated - Effectiveness Monitor deferred to V1.1)
```

**Rationale**: Update count math (10 original not 11, since Dynamic Toolset removed entirely)

---

### **Issue #3: Build Command (Line 125)**
**Current**:
```bash
# Build individual services
go build -o bin/gateway-service ./cmd/gateway
go build -o bin/dynamic-toolset ./cmd/dynamictoolset  # <-- DELETE
go build -o bin/data-storage ./cmd/datastorage
```

**Fix**: **DELETE dynamic-toolset build command line**

**Rationale**: cmd/dynamictoolset no longer exists

---

### **Issue #4: Service Documentation Navigation (Line 254)**
**Current**:
```markdown
- **[Stateless Services](docs/services/stateless/)**: Gateway Service, Dynamic Toolset, Data Storage Service, HolmesGPT API, Notification Service, Effectiveness Monitor
```

**Fix**:
```markdown
- **[Stateless Services](docs/services/stateless/)**: Gateway Service, Data Storage Service, HolmesGPT API, Notification Service, Effectiveness Monitor
```

**Rationale**: Remove from user-facing service list

---

### **Issue #5: Test Status Table (Line 278)**
**Current**:
```markdown
| **Dynamic Toolset** | - | - | - | **Deferred to V2.0** | **DD-016** |
```

**Fix**: **DELETE entire table row**

**Rationale**: Not part of V1.0 test status

---

### **Issue #6: Test Count Note (Line 285)**
**Current**:
```markdown
*Note: ... Dynamic Toolset (245 tests) deferred to V2.0 per DD-016.*
```

**Fix**: **DELETE "Dynamic Toolset (245 tests) deferred to V2.0 per DD-016." from note**

**Rationale**: Don't mention in V1.0 test status

---

## 🔧 **Makefile Changes Required**

**Search for**:
- `dynamictoolset` targets
- `toolset` build/test commands
- E2E test targets for toolset

**Expected Findings**:
```makefile
build-dynamictoolset:
test-unit-dynamictoolset:
test-integration-dynamictoolset:
test-e2e-dynamictoolset:
```

**Action**: Delete all Dynamic Toolset-specific targets

---

## 📂 **Other Documentation Files**

### **Priority Files to Update**

| File | Action | Reason |
|------|--------|--------|
| `docs/services/stateless/README.md` | Remove from service list | User-facing |
| `docs/README.md` | Update navigation | User-facing |
| `docs/DEVELOPER_GUIDE.md` | Remove build instructions | Developer-facing |
| `docs/architecture/KUBERNAUT_SERVICE_CATALOG.md` | Remove from catalog | Authoritative |
| `docs/architecture/SERVICE_DEPENDENCY_MAP.md` | Remove dependencies | Authoritative |

### **Files to Preserve (No Changes)**

| Directory | Reason |
|-----------|--------|
| `docs/services/stateless/dynamic-toolset/` | Historical reference, V2.0 planning |
| `docs/architecture/decisions/DD-016-*.md` | Authoritative design decision |
| `docs/architecture/DYNAMIC_TOOLSET_CONFIGURATION_ARCHITECTURE.md` | V2.0 reference |
| `docs/requirements/BR-TOOLSET-*.md` | V2.0 business requirements |

---

## ✅ **Validation Checklist**

After cleanup, verify:

- [ ] Main README.md has ZERO mentions of "Dynamic Toolset"
- [ ] Makefile has ZERO dynamictoolset targets
- [ ] `go build ./...` succeeds (no missing cmd/dynamictoolset)
- [ ] Service count accurate (8 production-ready, 10 original)
- [ ] Test count table excludes Dynamic Toolset
- [ ] Service navigation lists exclude Dynamic Toolset
- [ ] `docs/services/stateless/dynamic-toolset/` still exists (preserved)
- [ ] `deploy/dynamic-toolset/` still exists (preserved)
- [ ] DD-016 still exists (preserved)

---

## 🎯 **User Requirements**

**From User** (Dec 20, 2025):
> "triage for the Dynamic Toolset service references in the main README.md and other authoritative documents to remove references to it. It's reimplementation will depend on feedback of the current v1.0. For now I don't want to reference it in the main documentation to avoid confusion since it won't be in v1.x"

**Key Points**:
1. ✅ Remove from main README.md (avoid confusion)
2. ✅ Remove from authoritative docs
3. ✅ Keep implementation docs for V2.0 planning
4. ✅ Reimplementation depends on V1.0 feedback

---

## 📊 **Impact Assessment**

| Document Type | Files to Change | Estimated Time |
|---------------|-----------------|----------------|
| Main README.md | 1 file, 6 changes | 15 minutes |
| Makefile | 1 file | 10 minutes |
| Service docs | 3-5 files | 20 minutes |
| Architecture docs | 2-3 files | 15 minutes |
| **Total** | **7-10 files** | **~60 minutes** |

---

## 🚀 **Implementation Plan**

### **Phase 1: Main README.md** (Critical)
1. Delete service status table row (Line 79)
2. Update service count comment (Line 100)
3. Delete build command (Line 125)
4. Remove from service navigation (Line 254)
5. Delete test status table row (Line 278)
6. Update test count note (Line 285)

### **Phase 2: Makefile** (High Priority)
1. Search for all `dynamictoolset` targets
2. Delete build targets
3. Delete test targets
4. Verify `make build` still works

### **Phase 3: Service Documentation** (Medium Priority)
1. Update `docs/services/stateless/README.md`
2. Update `docs/README.md` navigation
3. Update `docs/DEVELOPER_GUIDE.md` if needed

### **Phase 4: Architecture Documentation** (Low Priority)
1. Update service catalog
2. Update service dependency map
3. Add notes about DD-016 deferral where needed

### **Phase 5: Validation** (Critical)
1. Build verification (`go build ./...`)
2. Makefile targets verification
3. Documentation consistency check

---

## 📝 **Status**

**Current**: Phase 1 in progress (Main README.md)
**Next**: Phase 2 (Makefile)
**ETA**: ~60 minutes total

---

**Created**: December 20, 2025
**Authority**: User guidance + DD-016
**Confidence**: 95% - Clear scope, straightforward cleanup
**Next Step**: Fix main README.md (6 changes)












