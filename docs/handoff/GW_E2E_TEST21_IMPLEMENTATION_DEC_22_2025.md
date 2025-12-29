# Gateway E2E Test 21 - CRD Lifecycle Operations
**Date**: December 22, 2025
**Status**: 🔄 **IN PROGRESS** (Test Running)
**Priority**: P0 - Core Functionality + Validation
**Service**: Gateway (GW)

---

## 🎯 Test Overview

**Test ID**: Test 21
**Test Name**: CRD Lifecycle Operations
**Business Requirements**: BR-GATEWAY-068, BR-GATEWAY-076, BR-GATEWAY-077
**Coverage Target**: `pkg/gateway/validation/*` + `pkg/gateway/processing/*` + `pkg/gateway/k8s/*` (+30% estimated)

---

## 📋 Test Scenarios (4 Specs)

### Test 21a: Malformed JSON Rejection
**Business Outcome**: Invalid JSON payloads rejected with proper error response

**Test Steps**:
1. Send malformed JSON to Gateway (missing closing braces)
2. Verify HTTP 400 Bad Request response
3. Validate RFC7807 Problem Details error format

**Expected Behavior**:
- HTTP 400 Bad Request
- RFC7807 fields present: `type`, `title`, `status`, `detail`

**Coverage**:
- `pkg/gateway/validation/json_validation.go`
- `pkg/gateway/errors/rfc7807.go`

---

### Test 21b: Valid Alert Creates CRD
**Business Outcome**: Valid alerts result in RemediationRequest CRDs in Kubernetes

**Test Steps**:
1. Send valid Prometheus alert to Gateway
2. Verify HTTP 201 Created response
3. **Query Kubernetes for created CRD** (using controller-runtime client)
4. Validate CRD spec fields match alert data

**Expected Behavior**:
- HTTP 201 Created
- Exactly 1 CRD created in test namespace
- CRD spec fields:
  - `SignalName` = "HighCPUUsage"
  - `Severity` = "critical"
  - `TargetResource.Namespace` = test namespace

**Coverage**:
- `pkg/gateway/processing/crd_creator.go`
- `pkg/gateway/k8s/client.go` (K8s client interaction)
- Gateway informer caching of RemediationRequest CRDs

**Key Validation**: This test verifies Gateway's K8s API server interaction through its informer.

---

### Test 21c: Missing Required Field Validation
**Business Outcome**: Alerts with missing required fields are rejected

**Test Steps**:
1. Send alert without `alertname` field
2. Verify HTTP 400 Bad Request response
3. Validate error response contains field validation details

**Expected Behavior**:
- HTTP 400 Bad Request
- Error response explains missing field

**Coverage**:
- `pkg/gateway/validation/field_validation.go`

---

### Test 21d: Invalid Content-Type Rejection
**Business Outcome**: Non-JSON Content-Type headers rejected

**Test Steps**:
1. Send valid alert payload with `Content-Type: text/plain`
2. Verify HTTP 415 Unsupported Media Type response

**Expected Behavior**:
- HTTP 415 Unsupported Media Type

**Coverage**:
- `pkg/gateway/middleware/content_type.go`

---

## 🔑 Key Design Decisions

### Using Controller-Runtime Client
**Rationale**: Gateway E2E tests use controller-runtime `client.Client` pattern for K8s queries

**Pattern**:
```go
k8sClient = getKubernetesClient()  // From deduplication_helpers.go

crdList := &remediationv1alpha1.RemediationRequestList{}
err := k8sClient.List(testCtx, crdList, client.InNamespace(testNamespace))
```

**Why Not Typed Clients**: Gateway E2E suite uses controller-runtime for consistency with controller testing patterns.

### Why Test K8s CRD Creation?
**User Feedback**: "the gw accessess the api server for RR CRD catching with the informer"

**Justification**: Gateway uses an informer to cache RemediationRequest CRDs from the Kubernetes API server. Test 21b validates that:
1. Gateway's K8s client successfully creates CRDs
2. CRDs contain correct spec data from alert payload
3. Gateway's informer interaction with API server works end-to-end

**Coverage Impact**: Tests `pkg/gateway/k8s/*` client code paths (+10-15% coverage)

---

## 📊 Expected Coverage Impact

### Before Test 21
| Package | Before | Target |
|---|---|---|
| `pkg/gateway/validation/*` | ~40% | ~70% |
| `pkg/gateway/processing/*` | 41.3% | ~60% |
| `pkg/gateway/k8s/*` | 22.2% | ~50% |

### After Test 21
**Total Gain**: +25-30% across validation, processing, and K8s client packages

---

## 🔍 Implementation Details

### Test Structure
- **Type**: `Ordered` Ginkgo test suite
- **Namespace**: Unique per test run (`crd-lifecycle-{process}-{timestamp}`)
- **Cleanup**: Namespace deleted on success, preserved on failure
- **Timeout**: 5 minutes per test context

### Dependencies
- Controller-runtime client (`sigs.k8s.io/controller-runtime/pkg/client`)
- RemediationRequest CRD API (`api/remediation/v1alpha1`)
- Gateway URL from suite setup (`gatewayURL`)
- K8s client helper (`getKubernetesClient()`)

### Test Isolation
- Each test uses unique namespace
- No shared state between specs
- CRD creation in Test 21b isolated to test namespace

---

## 🐛 Issues Resolved During Implementation

### Issue 1: Wrong API Import Path
**Symptom**: `cannot find module providing package github.com/jordigilh/kubernaut/api/v1alpha1`
**Root Cause**: Used incorrect CRD API import path
**Fix**: Changed to `github.com/jordigilh/kubernaut/api/remediation/v1alpha1`

### Issue 2: Missing K8s Client Access
**Symptom**: `undefined: k8sClient` and `undefined: remediationClient`
**Root Cause**: Attempted to use non-existent typed clients
**Fix**: Use controller-runtime client pattern from existing tests (`getKubernetesClient()`)

### Issue 3: Initial Oversimplification
**Symptom**: First version only tested HTTP validation, not K8s CRD creation
**User Feedback**: "the gw accessess the api server for RR CRD catching with the informer"
**Fix**: Added Test 21b to validate actual CRD creation in Kubernetes

---

## 🎯 Business Value

### Security (BR-GATEWAY-076, BR-GATEWAY-077)
- Validates malformed requests are rejected (prevents injection attacks)
- Ensures proper error handling prevents information leakage
- RFC7807 error format provides consistent error responses

### Reliability (BR-GATEWAY-068)
- Confirms CRDs are actually created for valid alerts
- Validates CRD spec data correctness
- Tests end-to-end Gateway → K8s API server flow

### Operations
- Content-Type validation prevents misconfigured clients
- Field validation prevents incomplete CRDs
- K8s client validation ensures informer caching works

---

## 🔗 Related Documents

- **Coverage Extension Plan**: `docs/handoff/GW_E2E_COVERAGE_EXTENSION_TRIAGE_DEC_22_2025.md`
- **Phase 1 Complete**: `docs/handoff/GW_PHASE1_COMPLETE_TRIAGE_DEC_22_2025.md`
- **CRD API Definition**: `api/remediation/v1alpha1/remediationrequest_types.go`
- **K8s Client Helpers**: `test/e2e/gateway/deduplication_helpers.go`

---

## ✅ Success Criteria

- [ ] All 4 specs pass (21a, 21b, 21c, 21d)
- [ ] Test 21b successfully queries Kubernetes for created CRD
- [ ] CRD spec fields validated correctly
- [ ] RFC7807 error format validated
- [ ] HTTP status codes correct (400, 415, 201)
- [ ] Test execution time < 2 minutes (after infrastructure setup)

---

## 📝 Next Steps

1. **Validate Test Results**: Confirm all 4 specs pass
2. **Measure Coverage Impact**: Run coverage report to confirm +25-30% gain
3. **Commit Test 21**: Add to Phase 2 completion
4. **Implement Test 22**: Gateway behavior under K8s API failures (retry logic, error handling)

---

**Document Status**: 🔄 In Progress
**Test Execution**: Running
**Expected Completion**: ~7 minutes from start
**Current Phase**: Infrastructure setup (image building)









