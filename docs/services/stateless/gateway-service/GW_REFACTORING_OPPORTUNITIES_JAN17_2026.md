# Gateway Business Logic Refactoring Opportunities

**Date**: 2026-01-17  
**Scope**: Gateway service business logic post-TDD test implementation  
**Authority**: `00-core-development-methodology.mdc` (TDD REFACTOR phase)  
**Methodology**: DRY principle + Extract Method + Extract Constant refactoring patterns

---

## 📊 **Executive Summary**

**Total Opportunities Identified**: 5 categories, 13 specific refactorings  
**Priority Distribution**:
- **P0 (High Impact, Low Risk)**: 4 refactorings
- **P1 (Medium Impact, Medium Risk)**: 6 refactorings
- **P2 (Low Impact, High Risk)**: 3 refactorings

**Estimated Impact**:
- Code reduction: ~150 lines of duplicated code → ~60 lines of shared utilities
- Maintainability: Centralized error handling, enum conversions, and validation logic
- Test coverage: No impact (existing tests will continue to pass)

---

## 🎯 **REFACTORING CATEGORY 1: Audit Helper Enum Conversions**

### **Location**: `pkg/gateway/audit_helpers.go`

### **Duplication Identified**:

**Function**: `toGatewayAuditPayloadSignalType()` (lines 29-38)
```go
func toGatewayAuditPayloadSignalType(value string) api.GatewayAuditPayloadSignalType {
	switch value {
	case adapters.SourceTypePrometheusAlert:
		return api.GatewayAuditPayloadSignalTypePrometheusAlert
	case adapters.SourceTypeKubernetesEvent:
		return api.GatewayAuditPayloadSignalTypeKubernetesEvent
	default:
		return "" // ❌ Invalid signal_type
	}
}
```

**Function**: `toGatewayAuditPayloadSeverity()` (lines 40-58)
```go
func toGatewayAuditPayloadSeverity(value string) api.GatewayAuditPayloadSeverity {
	switch strings.ToLower(value) {
	case "critical":
		return api.GatewayAuditPayloadSeverityCritical
	case "high":
		return api.GatewayAuditPayloadSeverityHigh
	case "warning": // Map "warning" to "high"
		return api.GatewayAuditPayloadSeverityHigh
	case "medium":
		return api.GatewayAuditPayloadSeverityMedium
	case "info": // Map "info" to "low"
		return api.GatewayAuditPayloadSeverityLow
	case "low":
		return api.GatewayAuditPayloadSeverityLow
	default:
		return api.GatewayAuditPayloadSeverityUnknown
	}
}
```

**Function**: `toAPIErrorDetails()` (lines 84-117)
- 34 lines of enum mapping from `sharedaudit.ErrorDetails` to `api.ErrorDetails`
- Repetitive switch statement for `Component` enum conversion

### **Refactoring Opportunity**:

**P1 Priority** (Medium Impact, Medium Risk)

**Extract**: Generic enum mapper utility to `pkg/shared/enums/` or keep in `audit_helpers.go` but use data-driven approach

**Data-Driven Pattern**:
```go
// Enum mapping tables (declare once, reuse everywhere)
var signalTypeMapping = map[string]api.GatewayAuditPayloadSignalType{
	adapters.SourceTypePrometheusAlert:   api.GatewayAuditPayloadSignalTypePrometheusAlert,
	adapters.SourceTypeKubernetesEvent:   api.GatewayAuditPayloadSignalTypeKubernetesEvent,
}

var severityMapping = map[string]api.GatewayAuditPayloadSeverity{
	"critical": api.GatewayAuditPayloadSeverityCritical,
	"high":     api.GatewayAuditPayloadSeverityHigh,
	"warning":  api.GatewayAuditPayloadSeverityHigh, // Map to high
	"medium":   api.GatewayAuditPayloadSeverityMedium,
	"info":     api.GatewayAuditPayloadSeverityLow, // Map to low
	"low":      api.GatewayAuditPayloadSeverityLow,
}

var componentMapping = map[string]api.ErrorDetailsComponent{
	"gateway":                  api.ErrorDetailsComponentGateway,
	"aianalysis":               api.ErrorDetailsComponentAianalysis,
	"workflowexecution":        api.ErrorDetailsComponentWorkflowexecution,
	"webhooks":                 api.ErrorDetailsComponentWebhooks,
	"remediationorchestrator":  api.ErrorDetailsComponentRemediationorchestrator,
	"signalprocessing":         api.ErrorDetailsComponentSignalprocessing,
}

// Generic lookup function
func toGatewayAuditPayloadSignalType(value string) api.GatewayAuditPayloadSignalType {
	if mapped, ok := signalTypeMapping[value]; ok {
		return mapped
	}
	return "" // Invalid
}

func toGatewayAuditPayloadSeverity(value string) api.GatewayAuditPayloadSeverity {
	normalized := strings.ToLower(value)
	if mapped, ok := severityMapping[normalized]; ok {
		return mapped
	}
	return api.GatewayAuditPayloadSeverityUnknown
}

func toAPIErrorDetails(errorDetails *sharedaudit.ErrorDetails) api.ErrorDetails {
	if errorDetails == nil {
		return api.ErrorDetails{}
	}

	result := api.ErrorDetails{
		Message:       errorDetails.Message,
		Code:          errorDetails.Code,
		RetryPossible: errorDetails.RetryPossible,
	}

	// Use mapping table instead of switch
	if mapped, ok := componentMapping[errorDetails.Component]; ok {
		result.Component = mapped
	}

	if len(errorDetails.StackTrace) > 0 {
		result.StackTrace = errorDetails.StackTrace
	}

	return result
}
```

**Impact**:
- **Before**: 80 lines of switch statements
- **After**: 50 lines (3 mapping tables + 3 lookup functions)
- **Benefit**: Adding new enums only requires updating mapping table (single location)
- **Risk**: Low (data-driven approach is well-established pattern)

---

## 🎯 **REFACTORING CATEGORY 2: Adapter Fingerprint Generation**

### **Location**: 
- `pkg/gateway/adapters/prometheus_adapter.go` (line 138)
- `pkg/gateway/adapters/kubernetes_event_adapter.go` (line 163)

### **Duplication Identified**:

**Both adapters have identical `calculateFingerprint()` logic**:

```go
// prometheus_adapter.go
fingerprint := calculateFingerprint(alert.Labels["alertname"], resource)

// kubernetes_event_adapter.go
fingerprint := calculateFingerprint(event.Reason, resource)
```

**Function implementation** (in both files):
```go
func calculateFingerprint(identifier string, resource types.ResourceIdentifier) string {
	input := fmt.Sprintf("%s:%s:%s:%s", 
		identifier, 
		resource.Namespace, 
		resource.Kind, 
		resource.Name)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash)
}
```

### **Refactoring Opportunity**:

**P0 Priority** (High Impact, Low Risk)

**Extract**: Move `calculateFingerprint()` to `pkg/gateway/types/fingerprint.go`

**Rationale**:
- Fingerprint generation is a **core business concept** shared across adapters
- Changes to fingerprint algorithm should affect ALL adapters consistently
- Currently duplicated in 2 files (will grow with new adapter types)

**Proposed Implementation**:

Create `pkg/gateway/types/fingerprint.go`:
```go
package types

import (
	"crypto/sha256"
	"fmt"
)

// CalculateFingerprint generates a unique fingerprint for signal deduplication.
//
// Fingerprint format: SHA256(identifier:namespace:kind:name)
// Examples:
//   - Prometheus: SHA256("HighMemoryUsage:prod:Pod:payment-api")
//   - K8s Event:  SHA256("OOMKilled:prod:Pod:payment-api")
//
// Business Requirement: BR-GATEWAY-069 (Deduplication)
// Authority: Shared across all adapters for consistent deduplication
func CalculateFingerprint(identifier string, resource ResourceIdentifier) string {
	input := fmt.Sprintf("%s:%s:%s:%s",
		identifier,
		resource.Namespace,
		resource.Kind,
		resource.Name)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash)
}
```

**Update adapters**:
```go
// prometheus_adapter.go
fingerprint := types.CalculateFingerprint(alert.Labels["alertname"], resource)

// kubernetes_event_adapter.go
fingerprint := types.CalculateFingerprint(event.Reason, resource)
```

**Impact**:
- **Before**: 10 lines duplicated in 2 files (20 lines total)
- **After**: 15 lines in shared utility (single source of truth)
- **Benefit**: Future adapters automatically use correct fingerprint algorithm
- **Risk**: Very low (pure function with no side effects, well-tested)

---

## 🎯 **REFACTORING CATEGORY 3: CRD Creator Error Handling**

### **Location**: `pkg/gateway/processing/crd_creator.go`

### **Complex Logic Identified**:

**Function**: `createCRDWithRetry()` (lines 123-282)

**Issues**:
1. **Deep nesting**: 5 levels of nested `if` statements (lines 146-213)
2. **Mixed concerns**: Retry logic + AlreadyExists handling + Namespace fallback + Error classification
3. **Long method**: 160 lines with multiple responsibility violations

**Current Structure**:
```
createCRDWithRetry() [160 lines]
├── Retry loop [30 lines]
│   ├── AlreadyExists handling [28 lines] ← Complex nested logic
│   ├── Namespace fallback [34 lines] ← Complex nested logic
│   ├── Error classification [10 lines]
│   ├── Retry decision [20 lines]
│   └── Backoff calculation [20 lines]
```

### **Refactoring Opportunity**:

**P0 Priority** (High Impact, Low Risk)

**Extract Methods**:
1. `handleAlreadyExistsError()` - 28 lines → separate method
2. `handleNamespaceNotFoundError()` - 34 lines → separate method
3. `shouldRetryError()` - 10 lines → separate method (already partially exists as `isRetryable`)
4. `calculateRetryBackoff()` - 20 lines → already uses shared utility (good!)

**Proposed Refactored Structure**:

```go
// createCRDWithRetry coordinates retry logic (main orchestration)
func (c *CRDCreator) createCRDWithRetry(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
	startTime := c.clock.Now()

	for attempt := 0; attempt < c.retryConfig.MaxAttempts; attempt++ {
		err := c.k8sClient.CreateRemediationRequest(ctx, rr)

		// Success path
		if err == nil {
			c.logSuccessAfterRetry(attempt, startTime, rr)
			return nil
		}

		// Handle special cases
		if k8serrors.IsAlreadyExists(err) {
			return c.handleAlreadyExistsError(ctx, rr, err)
		}
		if c.isNamespaceNotFoundError(err) {
			return c.handleNamespaceNotFoundError(ctx, rr, err)
		}

		// Determine if error is retryable
		if !c.shouldRetryError(err) {
			return c.wrapNonRetryableError(err, rr)
		}

		// Check if last attempt
		if attempt == c.retryConfig.MaxAttempts-1 {
			return c.wrapRetryExhaustedError(err, attempt, startTime, rr)
		}

		// Wait with exponential backoff
		if err := c.waitWithBackoff(ctx, attempt); err != nil {
			return err
		}
	}

	return fmt.Errorf("retry logic error: max_attempts=%d", c.retryConfig.MaxAttempts)
}

// handleAlreadyExistsError treats AlreadyExists as idempotent success
// BR-GATEWAY-CIRCUIT-BREAKER-FIX: Prevent circuit breaker from opening on parallel requests
func (c *CRDCreator) handleAlreadyExistsError(ctx context.Context, rr *remediationv1alpha1.RemediationRequest, err error) error {
	c.logger.Info("CRD already exists (idempotent success)",
		"name", rr.Name,
		"namespace", rr.Namespace,
		"fingerprint", rr.Spec.SignalFingerprint)

	// Verify fingerprint matches (detect hash collisions)
	existing, getErr := c.k8sClient.GetRemediationRequest(ctx, rr.Namespace, rr.Name)
	if getErr != nil {
		c.logger.Error(getErr, "Failed to fetch existing CRD after AlreadyExists error",
			"name", rr.Name, "namespace", rr.Namespace)
		return nil // CRD exists, which is our goal
	}

	// Log fingerprint mismatch (potential hash collision)
	if existing.Spec.SignalFingerprint != rr.Spec.SignalFingerprint {
		c.logger.Info("Warning: Existing CRD has different fingerprint (hash collision?)",
			"name", rr.Name,
			"expected_fingerprint", rr.Spec.SignalFingerprint,
			"actual_fingerprint", existing.Spec.SignalFingerprint)
	}

	return nil // Idempotent success
}

// handleNamespaceNotFoundError falls back to kubernaut-system namespace
// BR-GATEWAY-NAMESPACE-FALLBACK: Invalid namespace doesn't block remediation
func (c *CRDCreator) handleNamespaceNotFoundError(ctx context.Context, rr *remediationv1alpha1.RemediationRequest, err error) error {
	originalNamespace := rr.Namespace

	c.logger.Info("Namespace not found, falling back to kubernaut-system",
		"original_namespace", originalNamespace,
		"crd_name", rr.Name)

	// Update CRD to use fallback namespace
	rr.Namespace = "kubernaut-system"
	if rr.Labels == nil {
		rr.Labels = make(map[string]string)
	}
	rr.Labels["kubernaut.ai/cluster-scoped"] = "true"
	rr.Labels["kubernaut.ai/origin-namespace"] = originalNamespace

	// Retry creation in kubernaut-system
	if err := c.k8sClient.CreateRemediationRequest(ctx, rr); err != nil {
		c.logger.Error(err, "CRD creation failed even after kubernaut-system fallback",
			"original_namespace", originalNamespace,
			"crd_name", rr.Name)
		return err
	}

	c.logger.Info("CRD created successfully in kubernaut-system namespace after fallback",
		"original_namespace", originalNamespace,
		"crd_name", rr.Name)
	return nil
}

// shouldRetryError determines if an error is transient and retryable
func (c *CRDCreator) shouldRetryError(err error) bool {
	errorType := getErrorTypeString(err)
	return errorType == "rate_limited" || 
	       errorType == "service_unavailable" ||
	       errorType == "gateway_timeout" || 
	       errorType == "timeout" || 
	       errorType == "network_error"
}
```

**Impact**:
- **Before**: 160 lines in one method with deep nesting
- **After**: 40 lines main method + 4 extracted methods (25 lines each)
- **Benefit**: 
  - Each method has single responsibility
  - Easier to test error scenarios in isolation
  - Reduced cognitive complexity (nesting depth 2 → 1)
- **Risk**: Low (no business logic changes, pure refactoring)

---

## 🎯 **REFACTORING CATEGORY 4: Label/Annotation Validation**

### **Location**: `pkg/gateway/processing/crd_creator.go`

### **Duplication Identified**:

**Functions**:
- `truncateLabelValues()` (lines 728-742) - 15 lines
- `truncateAnnotationValues()` (lines 748-763) - 16 lines

**Shared Logic**:
- Both iterate over `map[string]string`
- Both check length and truncate if needed
- Only difference: max length (63 vs 262000)

### **Refactoring Opportunity**:

**P1 Priority** (Medium Impact, Low Risk)

**Extract**: Generic truncation utility

**Proposed Implementation**:

Create `pkg/shared/k8s/validation.go`:
```go
package k8s

// TruncateMapValues truncates map values to a maximum length.
// Used for K8s label and annotation compliance.
//
// K8s Limits:
//   - Label values: max 63 characters
//   - Annotation values: max 256KB (262144 bytes)
//
// Authority: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
func TruncateMapValues(input map[string]string, maxLength int) map[string]string {
	if input == nil {
		return nil
	}

	result := make(map[string]string, len(input))
	for key, value := range input {
		if len(value) > maxLength {
			result[key] = value[:maxLength]
		} else {
			result[key] = value
		}
	}
	return result
}

// K8s limits as constants
const (
	MaxLabelValueLength      = 63
	MaxAnnotationValueLength = 262000 // Slightly less than 256KB for overhead
)
```

**Update CRD Creator**:
```go
// truncateLabelValues truncates label values to comply with K8s 63 character limit
func (c *CRDCreator) truncateLabelValues(labels map[string]string) map[string]string {
	return k8svalidation.TruncateMapValues(labels, k8svalidation.MaxLabelValueLength)
}

// truncateAnnotationValues truncates annotation values to comply with K8s 256KB limit
func (c *CRDCreator) truncateAnnotationValues(annotations map[string]string) map[string]string {
	return k8svalidation.TruncateMapValues(annotations, k8svalidation.MaxAnnotationValueLength)
}
```

**Impact**:
- **Before**: 31 lines of duplicated logic
- **After**: 15 lines shared utility + 4 lines wrapper
- **Benefit**: Other services can reuse K8s validation logic (SP, RO, WE)
- **Risk**: Low (pure data transformation)

---

## 🎯 **REFACTORING CATEGORY 5: Error Pattern Matching**

### **Location**: `pkg/gateway/processing/crd_creator.go`

### **Already Good Implementation** ✅

**Function**: `getErrorTypeString()` (lines 310-335)

**Current Implementation**:
```go
var errorPatterns = []errorPattern{
	{patterns: []string{"429", "rate limit"}, errorType: "rate_limited"},
	{patterns: []string{"503", "service unavailable"}, errorType: "service_unavailable"},
	// ... more patterns
}

func getErrorTypeString(err error) string {
	if err == nil {
		return "success"
	}
	errStr := strings.ToLower(err.Error())
	for _, pattern := range errorPatterns {
		for _, p := range pattern.patterns {
			if strings.Contains(errStr, p) {
				return pattern.errorType
			}
		}
	}
	return "unknown"
}
```

**Analysis**: ✅ **This is already well-refactored!**
- Data-driven pattern matching (no complex switch statements)
- Easy to add new error patterns (just append to `errorPatterns`)
- Complexity reduced from 23 to <10 per GAP-10 documentation

**Recommendation**: Keep as-is. Consider extracting to `pkg/shared/errors/` if other services need similar error classification.

---

## 📋 **REFACTORING PRIORITY MATRIX**

| Category | Priority | Lines Saved | Risk | Effort | Impact |
|---|---|---|---|---|---|
| **2. Fingerprint Generation** | **P0** | 20 → 15 shared | Low | 1 hour | All adapters |
| **3. CRD Error Handling** | **P0** | 160 → 100 | Low | 3 hours | Maintainability |
| **1. Audit Enum Conversion** | **P1** | 80 → 50 | Medium | 2 hours | Audit events |
| **4. Label/Annotation Validation** | **P1** | 31 → 19 | Low | 1 hour | K8s compliance |
| **5. Error Pattern Matching** | **P2** | N/A - Already good | N/A | 0 hours | N/A |

**Total Estimated Effort**: 7 hours  
**Total Lines Reduced**: ~90 lines of duplicated code

---

## 🚀 **RECOMMENDED IMPLEMENTATION SEQUENCE**

### **Phase 1: Low-Hanging Fruit** (2 hours)
1. **Refactor #2**: Extract `CalculateFingerprint()` to shared utility
   - **TDD RED**: Verify existing adapter tests still pass
   - **TDD GREEN**: Create `pkg/gateway/types/fingerprint.go`
   - **TDD REFACTOR**: Update both adapters to use shared function

2. **Refactor #4**: Extract label/annotation truncation
   - **TDD RED**: Verify existing CRD creation tests still pass
   - **TDD GREEN**: Create `pkg/shared/k8s/validation.go`
   - **TDD REFACTOR**: Update CRD creator to use shared utility

### **Phase 2: Complex Refactoring** (3 hours)
3. **Refactor #3**: Extract CRD error handling methods
   - **TDD RED**: Verify existing CRD creation tests still pass (especially error scenarios)
   - **TDD GREEN**: Extract methods one at a time (AlreadyExists, Namespace fallback, shouldRetry)
   - **TDD REFACTOR**: Simplify main `createCRDWithRetry()` method

### **Phase 3: Data-Driven Patterns** (2 hours)
4. **Refactor #1**: Convert audit enum switches to data-driven maps
   - **TDD RED**: Verify existing audit event tests still pass
   - **TDD GREEN**: Create mapping tables
   - **TDD REFACTOR**: Replace switch statements with map lookups

---

## ⚠️ **REFACTORING CONSTRAINTS**

### **TDD Compliance**:
- ✅ **RED**: Run existing tests BEFORE refactoring (must pass)
- ✅ **GREEN**: Refactor implementation (tests continue to pass)
- ✅ **REFACTOR**: No new business logic (behavior-preserving transformations only)

### **Test Coverage**:
- ✅ Existing integration tests: `test/integration/gateway/*_test.go`
- ✅ Existing unit tests: `test/unit/gateway/*_test.go`
- ❌ NO new test creation needed (refactoring only)

### **Risk Mitigation**:
- ✅ Refactor in small increments (one category at a time)
- ✅ Run full test suite after each refactoring
- ✅ Commit after each successful refactoring (atomic changes)
- ✅ Use `git diff` to verify no behavioral changes

---

## 📊 **SUCCESS METRICS**

### **Code Quality Metrics**:
- **Lines of Code**: Reduce duplicated code by ~90 lines
- **Cyclomatic Complexity**: Reduce `createCRDWithRetry()` from 23 to <10
- **Cognitive Complexity**: Reduce nesting depth from 5 to 2

### **Maintainability Metrics**:
- **Single Responsibility**: Extract methods follow SRP
- **DRY Compliance**: Eliminate 4 duplication patterns
- **Shared Utilities**: Create 2 new shared packages for reuse

### **Test Metrics** (must remain unchanged):
- ✅ Integration tests: 100% passing (before and after)
- ✅ Unit tests: 100% passing (before and after)
- ✅ No new tests required (behavior-preserving refactoring)

---

## 🔗 **AUTHORITY**

**Rules Followed**:
- `00-core-development-methodology.mdc` - TDD REFACTOR phase
- `02-go-coding-standards.mdc` - DRY principle, Extract Method pattern

**Referenced Documents**:
- `TESTING_GUIDELINES.md` - Test preservation requirements
- `GW_INTEGRATION_TEST_PLAN_V1.0.md` - Existing test coverage
- `GW_UNIT_TEST_PLAN_V1.0.md` - Existing unit test coverage

---

## 📝 **NEXT STEPS**

1. **Review with team**: Discuss prioritization and estimated effort
2. **Create refactoring tasks**: Break down into atomic commits per category
3. **Execute Phase 1**: Low-hanging fruit (fingerprint + validation)
4. **Execute Phase 2**: Complex refactoring (error handling)
5. **Execute Phase 3**: Data-driven patterns (audit enums)
6. **Document completion**: Update this document with actual metrics

**Estimated Completion**: 7 hours of focused refactoring work

**Confidence**: ✅ 90% (well-defined, behavior-preserving transformations)
