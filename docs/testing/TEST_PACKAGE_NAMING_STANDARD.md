# Test Package Naming Standard

**Status**: ✅ **AUTHORITATIVE**
**Date**: May 15, 2026
**Authority**: Project-wide standard
**Version**: 2.0

---

## 📋 **Version History**

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v2.0** | 2026-05-15 | Migrated to black-box colocated tests per Issue #50. Unit tests now live alongside production code in `pkg/` and `internal/`. White-box (same-package) testing is deprecated. | ✅ **CURRENT** |
| **v1.1** | 2025-11-19 | Updated Template Compliance section | Superseded |
| **v1.0** | 2025-11-13 | Initial authoritative standard created | Superseded |

---

## 🎯 **Standard: Black-Box Testing, Colocated with Code**

**MANDATORY**: All unit test files in Kubernaut MUST:

1. **Be colocated** with the production code they test (e.g., `pkg/gateway/server_test.go` alongside `pkg/gateway/server.go`)
2. **Use the `_test` suffix** in the package declaration (e.g., `package gateway_test`) for black-box testing of exported APIs

### **Correct Pattern**

```go
// ✅ CORRECT: Black-box, colocated unit test
// File: pkg/gateway/server_test.go
package gateway_test

import (
    "github.com/jordigilh/kubernaut/pkg/gateway"
)

// ✅ CORRECT: Integration test (separate directory)
// File: test/integration/gateway/server_test.go
package gateway
```

### **Incorrect Pattern**

```go
// ❌ WRONG: Tests in separate test/unit/ directory
// File: test/unit/gateway/server_test.go
package gateway

// ❌ WRONG: White-box test accessing unexported symbols
// File: pkg/gateway/server_test.go
package gateway  // Should be gateway_test
```

---

## 📋 **Rationale**

### **Why Black-Box Testing?**

1. **Refactoring resilience**: Tests validate public API; internals can change freely
2. **Go idiomatic**: Standard Go convention for testing exported interfaces
3. **API surface validation**: Tests document the public contract
4. **Discoverability**: Tests live next to the code they test

### **Why Colocated?**

1. **`go test ./pkg/...`** discovers all unit tests naturally
2. **IDE navigation**: One-click toggle between code and tests
3. **Go community standard**: Familiar to all Go developers
4. **Tooling compatibility**: Coverage, refactoring, and linting work seamlessly

### **Exceptions: White-Box Testing**

White-box tests (`package <name>` without `_test` suffix) are permitted ONLY when:
- Testing unexported functions that cannot be exercised through the public API
- The file includes a comment explaining why white-box access is needed

Example: `pkg/datastorage/server/shutdown_test.go` uses `package server` to test unexported `isShuttingDown` fields.

---

## 🔍 **Verification**

### **Check Test Location**

```bash
# Unit tests should be in pkg/ or internal/, NOT test/unit/
find test/unit -name "*_test.go" 2>/dev/null
# Expected: No results (test/unit/ should not exist)

# Unit tests should be colocated
find pkg internal -name "*_test.go" | head -20
# Expected: Test files alongside production code
```

### **Check Package Naming**

```bash
# Unit tests should use _test suffix (black-box)
grep -r "^package.*_test$" pkg/ internal/ --include="*_test.go" | head -10
# Expected: All test files use _test suffix
```

---

## ✅ **Compliance Checklist**

Before committing test files:

- [ ] Unit test file is **colocated** with production code in `pkg/` or `internal/`
- [ ] Test file uses **`_test` suffix** in package declaration (e.g., `package gateway_test`)
- [ ] Test file name ends with `_test.go`
- [ ] Imports reference the package being tested via its full import path
- [ ] Each package directory has exactly **one** `RunSpecs` call (in `suite_test.go`)

---

## 🔧 **Test File Layout**

```
pkg/gateway/
├── server.go                    # Production code
├── server_test.go               # Unit test (package gateway_test)
├── suite_test.go                # Ginkgo suite runner (one per package)
├── config/
│   ├── config.go
│   ├── config_test.go
│   ├── suite_test.go
│   └── testdata/                # Test fixtures
│       ├── valid-config.yaml
│       └── invalid-config.yaml
└── middleware/
    ├── cors.go
    ├── cors_test.go
    └── suite_test.go

test/integration/gateway/        # Integration tests (separate)
test/e2e/gateway/                # E2E tests (separate)
```

---

## 📚 **Related Standards**

- [Testing Strategy](.cursor/rules/03-testing-strategy.mdc) - Overall testing approach
- [Issue #50](https://github.com/jordigilh/kubernaut/issues/50) - Migration tracking issue

---

**Document Status**: ✅ **AUTHORITATIVE**
**Enforcement**: **MANDATORY** for all new code
**Exceptions**: White-box tests only when accessing unexported symbols (must be documented)
