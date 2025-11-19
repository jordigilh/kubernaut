# Test Package Naming Standard

**Status**: ✅ **AUTHORITATIVE**
**Date**: November 19, 2025
**Authority**: Project-wide standard
**Version**: 1.1

---

## 📋 **Version History**

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v1.1** | 2025-11-19 | Updated Template Compliance section - all templates now compliant | ✅ **CURRENT** |
| **v1.0** | 2025-11-13 | Initial authoritative standard created | Superseded |

---

## 🎯 **Standard: White-Box Testing (Same Package)**

**MANDATORY**: All test files in Kubernaut MUST use the **same package name** as the code being tested.

### **Correct Pattern**

```go
// ✅ CORRECT: White-box testing
// File: test/unit/contextapi/models_test.go
package contextapi

// File: test/integration/toolset/discovery_test.go
package toolset

// File: test/unit/datastorage/repository_test.go
package datastorage
```

### **Incorrect Pattern**

```go
// ❌ WRONG: Black-box testing with _test suffix
// File: test/unit/contextapi/models_test.go
package contextapi_test  // ❌ DO NOT USE

// File: test/integration/toolset/discovery_test.go
package toolset_test  // ❌ DO NOT USE

// File: test/unit/datastorage/repository_test.go
package datastorage_test  // ❌ DO NOT USE
```

---

## 📋 **Rationale**

### **Why White-Box Testing?**

1. **Access to Internal State**: Tests can validate internal fields and unexported functions
2. **Comprehensive Validation**: Can test implementation details when needed
3. **Project Consistency**: All services follow the same pattern
4. **Simpler Test Patterns**: No need for complex accessor methods

### **Why NOT Black-Box Testing?**

1. **Inconsistent with Project Convention**: Kubernaut uses white-box testing
2. **Limited Access**: Can't validate internal state or unexported functions
3. **Inefficient Patterns**: Forces creation of accessor methods just for testing
4. **Import Complexity**: Can cause circular import issues in some cases

---

## 🔍 **Verification**

### **Check Your Test Files**

```bash
# Find any incorrect _test package declarations
grep -r "^package.*_test$" test/ --include="*.go"

# Expected: No results (all tests use same package as code)
```

### **Correct Examples from Codebase**

**Dynamic Toolset** (Reference Implementation):
```bash
$ grep "^package" test/integration/toolset/*.go
test/integration/toolset/suite_test.go:package toolset
test/integration/toolset/graceful_shutdown_test.go:package toolset
test/integration/toolset/content_type_validation_test.go:package toolset
```

**Data Storage** (Reference Implementation):
```bash
$ grep "^package" test/integration/datastorage/*.go
test/integration/datastorage/audit_events_schema_test.go:package datastorage
```

**Context API** (Reference Implementation):
```bash
$ grep "^package" test/unit/contextapi/*.go
test/unit/contextapi/models_test.go:package contextapi
test/unit/contextapi/query_builder_test.go:package contextapi
```

---

## 📚 **Authoritative References**

### **Primary Authority**

1. **Context API COMMON-PITFALLS.md** (lines 288-310)
   - File: `docs/services/stateless/context-api/COMMON-PITFALLS.md`
   - Section: "Pitfall #8: Inconsistent Package Naming in Tests"
   - Explicitly states: "❌ WRONG: Black-box testing (package name_test)"
   - Explicitly states: "✅ CORRECT: White-box testing (same package)"

2. **Context API Day 1 Foundation Complete** (lines 338-340)
   - File: `docs/services/stateless/context-api/implementation/phase0/01-day1-foundation-complete.md`
   - States: "Correctly using `package contextapi` (not `contextapi_test`)"
   - States: "Follows project standards for test files in `test/` directories"

3. **Context API Implementation Plan V1.0** (line 11)
   - File: `docs/services/stateless/context-api/implementation/archive/IMPLEMENTATION_PLAN_V1.0.md`
   - Identifies: "CRITICAL GAP IDENTIFIED: Test package naming uses `_test` suffix incorrectly"

### **Reference Implementations**

1. **Dynamic Toolset**: `test/integration/toolset/` - Uses `package toolset`
2. **Data Storage**: `test/integration/datastorage/` - Uses `package datastorage`
3. **Context API**: `test/unit/contextapi/` - Uses `package contextapi`

---

## ✅ **Compliance Checklist**

Before committing test files:

- [ ] Test file uses **same package name** as code being tested
- [ ] Test file does **NOT** use `_test` suffix in package declaration
- [ ] Test file is in `test/unit/[service]/`, `test/integration/[service]/`, or `test/e2e/[service]/`
- [ ] Test file name ends with `_test.go`
- [ ] Imports reference the package being tested if needed

---

## 🚨 **Common Mistakes**

### **Mistake 1: Using Go Standard Practice**

```go
// ❌ WRONG for Kubernaut (but correct for standard Go)
package datastorage_test

import "github.com/jordigilh/kubernaut/pkg/datastorage"
```

**Why Wrong**: While this is standard Go practice for external black-box testing, **Kubernaut uses white-box testing** as a project convention.

**Correction**:
```go
// ✅ CORRECT for Kubernaut
package datastorage

// No import needed - same package
```

---

### **Mistake 2: Mixing Conventions**

```go
// ❌ WRONG: Inconsistent package naming
// File 1: test/unit/toolset/detector_test.go
package toolset_test

// File 2: test/unit/toolset/metrics_test.go
package toolset
```

**Why Wrong**: Inconsistency within the same service.

**Correction**: All test files for a service use the **same package name**.

---

## 🔧 **Migration Guide**

If you have existing tests with `_test` suffix:

### **Step 1: Update Package Declaration**

```diff
- package datastorage_test
+ package datastorage
```

### **Step 2: Remove Unnecessary Imports**

```diff
- import "github.com/jordigilh/kubernaut/pkg/datastorage"

  func TestRepository(t *testing.T) {
-     repo := datastorage.NewRepository()
+     repo := NewRepository()
  }
```

### **Step 3: Verify Tests Pass**

```bash
go test ./test/unit/[service]/...
go test ./test/integration/[service]/...
```

---

## 📖 **Related Standards**

- [Testing Strategy](../testing/03-testing-strategy.mdc) - Overall testing approach
- [Service Implementation Plan Template](../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md) - Must follow this standard
- [Context API Common Pitfalls](../services/stateless/context-api/COMMON-PITFALLS.md) - Detailed examples

---

## 📝 **Template Compliance**

**CRITICAL**: All templates MUST reflect this standard.

**Current Status** (as of 2025-11-19): ✅ **ALL TEMPLATES COMPLIANT**
- `SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md`: ✅ Uses `package [service]` consistently
- `FEATURE_EXTENSION_PLAN_TEMPLATE.md`: ✅ Uses `package [service]` consistently
- Both templates include mandatory Package Naming Conventions section
- All test examples (unit, integration, E2E) use correct format

**Template Updates** (commit d04c69cf):
- Fixed FEATURE_EXTENSION_PLAN_TEMPLATE.md violations (3 examples corrected)
- Added Package Naming Conventions section to both templates
- Clear reference to TEST_PACKAGE_NAMING_STANDARD.md (authoritative)

---

**Document Status**: ✅ **AUTHORITATIVE**
**Enforcement**: **MANDATORY** for all new code
**Exceptions**: None - all services must follow this standard

