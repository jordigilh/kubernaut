# Package Naming Convention Violation Analysis

**Date**: October 21, 2025
**Issue**: Test files using `package xxx_test` instead of `package xxx`

---

## 📋 Correct Convention

**File Naming**: `component_test.go` ✅
**Package Declaration**: `package component` ✅ (NO `_test` suffix)

**Example**:
```go
// File: test/unit/gateway/prometheus_adapter_test.go
package gateway  // ✅ CORRECT - Internal test package

import (
    "testing"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)
```

---

## 🔍 Violation Summary

**Total Violating Files**: 104 files use `package xxx_test`

### Major Services Affected

| Service | Violating Files | Plan Has Correct Convention? | Action Required |
|---------|----------------|------------------------------|-----------------|
| **Gateway** | 10 files | ✅ YES (plan fixed) | Fix actual code |
| **Notification** | 4 files | ❌ NO (plan uses `notification_test`) | Fix plan + code |
| **Toolset** | 13 files | ⚠️ NO PLAN FOUND | Fix code only |
| **Data Storage** | 6 files | ❌ NO (plan uses `datastorage_test`) | Fix plan + code |
| **Context API** | 13 files | ✅ YES (plan uses `contextapi`) | Fix actual code |
| **AI/HolmesGPT** | 17 files | ⚠️ NOT CHECKED | Check plan + fix code |
| **Workflow Engine** | 27 files | ⚠️ NOT CHECKED | Check plan + fix code |
| **Remediation** | 8 files | ⚠️ NOT CHECKED | Check plan + fix code |
| **Webhook** | 6 files | ⚠️ NOT CHECKED | Check plan + fix code |

---

## 📁 Detailed Violation Breakdown

### 1. Gateway Service ✅ PLAN FIXED

**Implementation Plan**: `docs/services/stateless/gateway-service/IMPLEMENTATION_PLAN_V1.0.md`
**Plan Status**: ✅ **CORRECT** - Uses `package gateway` (recently fixed)

**Violating Test Files** (10 files):
```
test/unit/gateway/remediation_path_test.go         → package gateway_test ❌
test/unit/gateway/crd_metadata_test.go             → package gateway_test ❌
test/unit/gateway/suite_test.go                    → package gateway_test ❌
test/unit/gateway/storm_detection_test.go          → package gateway_test ❌
test/unit/gateway/signal_ingestion_test.go         → package gateway_test ❌
test/unit/gateway/priority_classification_test.go  → package gateway_test ❌
test/unit/gateway/k8s_event_adapter_test.go        → package gateway_test ❌
test/unit/gateway/adapters/validation_test.go      → package gateway_test ❌
test/unit/gateway/adapters/suite_test.go           → package gateway_test ❌
test/unit/gateway/adapters/prometheus_adapter_test.go → package gateway_test ❌
```

**Required Fix**: Update actual test files to use `package gateway`

---

### 2. Notification Service ❌ PLAN HAS ERROR

**Implementation Plan**: `docs/services/crd-controllers/06-notification/implementation/IMPLEMENTATION_PLAN_V3.0.md`
**Plan Status**: ❌ **INCORRECT** - Uses `package notification_test` in examples

**Plan Examples with Error**:
```go
// Line 272 - Test example
package notification_test  // ❌ INCORRECT in plan

// Line 854 - Test example
package notification_test  // ❌ INCORRECT in plan

// Line 1469 - Test example
package notification_test  // ❌ INCORRECT in plan

// Line 1762 - Test example
package notification_test  // ❌ INCORRECT in plan

// Line 2490 - Test example
package notification_test  // ❌ INCORRECT in plan

// Line 3051 - Test example
package notification_test  // ❌ INCORRECT in plan
```

**Violating Test Files** (4 files):
```
test/unit/notification/sanitization_test.go      → package notification_test ❌
test/unit/notification/status_test.go            → package notification_test ❌
test/unit/notification/slack_delivery_test.go    → package notification_test ❌
test/unit/notification/retry_test.go             → package notification_test ❌
test/unit/notification/controller_edge_cases_test.go → package notification_test ❌
```

**Required Fix**:
1. ✅ Update implementation plan examples to use `package notification`
2. ✅ Update actual test files to use `package notification`

---

### 3. Toolset (Dynamic Toolset) Service ⚠️ NO PLAN FOUND

**Implementation Plan**: `docs/services/stateless/dynamic-toolset/implementation/IMPLEMENTATION_PLAN_ENHANCED.md`
**Plan Status**: ⚠️ **NO GO CODE EXAMPLES** - Plan has no package declarations

**Violating Test Files** (13 files):
```
test/unit/toolset/service_discoverer_test.go   → package toolset_test ❌
test/unit/toolset/server_test.go               → package toolset_test ❌
test/unit/toolset/custom_detector_test.go      → package toolset_test ❌
test/unit/toolset/auth_middleware_test.go      → package toolset_test ❌
test/unit/toolset/suite_test.go                → package toolset_test ❌
test/unit/toolset/prometheus_detector_test.go  → package toolset_test ❌
test/unit/toolset/metrics_test.go              → package toolset_test ❌
test/unit/toolset/jaeger_detector_test.go      → package toolset_test ❌
test/unit/toolset/grafana_detector_test.go     → package toolset_test ❌
test/unit/toolset/generator_test.go            → package toolset_test ❌
test/unit/toolset/elasticsearch_detector_test.go → package toolset_test ❌
test/unit/toolset/detector_utils_test.go       → package toolset_test ❌
test/unit/toolset/configmap_builder_test.go    → package toolset_test ❌
```

**Required Fix**: Update actual test files to use `package toolset`

---

### 4. Data Storage Service ❌ PLAN HAS ERROR

**Implementation Plan**: `docs/services/stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V4.1.md`
**Plan Status**: ❌ **INCORRECT** - Uses `package datastorage_test` in examples

**Plan Examples with Error**:
```go
// Line 590 - Test example
package datastorage_test  // ❌ INCORRECT in plan

// Line 777 - Test example
package datastorage_test  // ❌ INCORRECT in plan

// Line 1059 - Test example
package datastorage_test  // ❌ INCORRECT in plan

// Line 1309 - Test example
package datastorage_test  // ❌ INCORRECT in plan

// Line 1515 - Test example
package datastorage_test  // ❌ INCORRECT in plan

// Line 1750 - Test example
package datastorage_test  // ❌ INCORRECT in plan

// Line 1923 - Test example
package datastorage_test  // ❌ INCORRECT in plan
```

**Violating Test Files** (6 files):
```
test/unit/datastorage/suite_test.go           → package datastorage_test ❌
test/unit/datastorage/metrics_test.go         → package datastorage_test ❌
test/unit/datastorage/dualwrite_context_test.go → package datastorage_test ❌
test/unit/datastorage/query_test.go           → package datastorage_test ❌
test/unit/datastorage/validation_test.go      → package datastorage_test ❌
test/unit/datastorage/embedding_test.go       → package datastorage_test ❌
test/unit/datastorage/dualwrite_test.go       → package datastorage_test ❌
test/unit/datastorage/sanitization_test.go    → package datastorage_test ❌
test/unit/datastorage_query_test.go           → package datastorage_test ❌
```

**Required Fix**:
1. ✅ Update implementation plan examples to use `package datastorage`
2. ✅ Update actual test files to use `package datastorage`

---

### 5. Context API Service ✅ PLAN CORRECT

**Implementation Plan**: `docs/services/stateless/context-api/implementation/IMPLEMENTATION_PLAN_V2.0.md`
**Plan Status**: ✅ **CORRECT** - Uses `package contextapi` throughout

**Plan Examples (Correct)**:
```go
// Line 2520 - Test example
package contextapi  // ✅ CORRECT in plan

// Line 2597 - Test example
package contextapi  // ✅ CORRECT in plan

// Line 3256 - Test example
package contextapi  // ✅ CORRECT in plan

// Line 4045 - Test example
package contextapi  // ✅ CORRECT in plan

// Line 4208 - Test example
package contextapi  // ✅ CORRECT in plan

// Line 5870 - Test example
package contextapi  // ✅ CORRECT in plan
```

**Violating Test Files** (13 files):
```
test/unit/contextapi/sqlbuilder/builder_schema_test.go → package contextapi ✅ CORRECT!
test/unit/contextapi/cache_manager_test.go    → package contextapi ✅ CORRECT!
test/unit/contextapi/sql_unicode_test.go      → package contextapi ✅ CORRECT!
test/unit/contextapi/cache_thrashing_test.go  → package contextapi ✅ CORRECT!
test/unit/contextapi/config_yaml_test.go      → package contextapi ✅ CORRECT!
test/unit/contextapi/cache_size_limits_test.go → package contextapi ✅ CORRECT!
test/unit/contextapi/cached_executor_test.go  → package contextapi ✅ CORRECT!
test/unit/contextapi/router_test.go           → package contextapi ✅ CORRECT!
test/unit/contextapi/suite_test.go            → package contextapi ✅ CORRECT!
test/unit/contextapi/client_test.go           → package contextapi ✅ CORRECT!
test/unit/contextapi/vector_test.go           → package contextapi ✅ CORRECT!
```

**Status**: ✅ **NO ACTION NEEDED** - Context API already follows correct convention!

---

### 6. Other Services (Need Analysis)

**Services Requiring Analysis**:
- AI/HolmesGPT (17 files with violations)
- Workflow Engine (27 files with violations)
- Remediation Controllers (8 files with violations)
- Webhook (6 files with violations)
- Security (2 files with violations)
- Platform (3 files with violations)
- Monitoring (2 files with violations)
- Infrastructure (2 files with violations)
- Adaptive Orchestration (4 files with violations)

---

## 🎯 Action Plan

### Phase 1: Fix Implementation Plans (Priority: HIGH)

#### 1.1 Notification Service Plan
**File**: `docs/services/crd-controllers/06-notification/implementation/IMPLEMENTATION_PLAN_V3.0.md`
**Action**: Replace all instances of `package notification_test` with `package notification`
**Lines to Fix**: 272, 854, 1469, 1762, 2490, 3051

#### 1.2 Data Storage Service Plan
**File**: `docs/services/stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V4.1.md`
**Action**: Replace all instances of `package datastorage_test` with `package datastorage`
**Lines to Fix**: 590, 777, 1059, 1309, 1515, 1750, 1923

### Phase 2: Fix Actual Test Files (Priority: MEDIUM)

**Approach**: Systematic update of all 104 violating test files

**Command to Fix All**:
```bash
# Find all test files with package xxx_test declaration
find test/ -name "*_test.go" -type f | while read file; do
    # Extract the package name from the file path
    dir=$(dirname "$file" | xargs basename)

    # Replace package xxx_test with package xxx
    sed -i '' "s/^package ${dir}_test$/package ${dir}/" "$file"
done
```

**Manual Verification**: Review changes for edge cases (e.g., nested packages)

### Phase 3: Update ADR Documentation (Priority: LOW)

**File**: `docs/architecture/decisions/ADR-004-fake-kubernetes-client.md`
**Action**: Update line 119 example to clarify correct convention

**Current** (line 119):
```go
package remediationprocessing_test  // ❌ Shows external test pattern
```

**Proposed**:
```go
package remediationprocessing  // ✅ Internal test package (preferred)
```

**Rationale**: ADR-004 currently shows the external test package pattern (`_test` suffix), which contradicts the project's preferred internal test package approach.

---

## 📊 Summary Statistics

| Category | Count | Status |
|----------|-------|--------|
| **Total Violating Files** | 104 | ❌ Need fixing |
| **Plans with Error** | 2 (Notification, Data Storage) | ❌ Fix first |
| **Plans with Correct Convention** | 2 (Gateway, Context API) | ✅ Reference models |
| **Files Already Correct** | 13 (Context API tests) | ✅ No action |

**Estimated Fix Time**:
- Phase 1 (Fix Plans): 30 minutes
- Phase 2 (Fix Code): 2-3 hours (with testing)
- Phase 3 (Update ADR): 15 minutes
- **Total**: ~3-4 hours

---

## 🔍 Root Cause Analysis

### Why This Happened

1. **ADR-004 Example**: Shows `package remediationprocessing_test` pattern (line 119)
2. **Mixed Conventions**: Codebase has both patterns (internal vs external test packages)
3. **Go Flexibility**: Both patterns are valid Go, causing confusion
4. **Template Propagation**: Early implementation plans were copied, spreading the error

### Prevention Strategy

1. ✅ Update ADR-004 to clarify preferred convention
2. ✅ Add linter rule to enforce `package xxx` (not `package xxx_test`)
3. ✅ Update implementation plan templates
4. ✅ Add to code review checklist
5. ✅ Document in `.cursor/rules/02-go-coding-standards.mdc`

---

## 📚 References

- **Context API Implementation Plan**: Best example of correct convention
- **Gateway Implementation Plan**: Recently fixed, good reference
- **Codebase Examples**: `test/unit/contextapi/*.go` files follow correct pattern

---

**Document Status**: 📋 Analysis Complete
**Next Step**: Fix Notification and Data Storage implementation plans
**Owner**: Development Team
**Priority**: HIGH (Consistency and maintainability)


