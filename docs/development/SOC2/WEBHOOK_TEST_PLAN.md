# Webhook Test Plan - Comprehensive Testing Strategy

**Date**: January 6, 2026
**Status**: ✅ **APPROVED** - Ready for TDD Implementation
**Purpose**: Define comprehensive test strategy for single consolidated webhook
**Authority**: DD-AUTH-001, DD-TESTING-001, SOC2 CC8.1
**Testing Tiers**: Unit (80%) → Integration (15%) → E2E (5%)
**Owner**: Webhook Team

---

## 🎯 **Testing Philosophy**

### **Defense-in-Depth Testing Approach**

```
Unit Tests (80%)           Integration Tests (15%)      E2E Tests (5%)
────────────────────────   ──────────────────────────   ────────────────────────
✅ Fast (<1s total)        ✅ Medium (~10s total)        ✅ Slow (~60s total)
✅ No K8s cluster          ✅ No K8s cluster             ✅ Real K8s (Kind)
✅ Pure Go logic           ✅ Webhook server running     ✅ Complete deployment
✅ handler.Handle()        ✅ HTTP POST to webhook       ✅ kubectl operations
✅ Mock admission.Request  ✅ TLS + auth flow            ✅ Real controllers
✅ 60+ tests               ✅ 10+ tests                  ✅ 10+ tests

Focus: Handler logic      Focus: HTTP integration      Focus: Business flows
```

### **TDD Workflow (MANDATORY)**

**APDC-Enhanced TDD**:
1. **Analysis**: Study CRD schemas, identify auth fields
2. **Plan**: Design test cases, expected behaviors
3. **Do-RED**: Write failing tests (handlers don't exist yet)
4. **Do-GREEN**: Implement minimal handler logic to pass tests
5. **Do-REFACTOR**: Add validation, error handling, logging
6. **Check**: Verify SOC2 compliance, performance

**Result**: Tests written BEFORE implementation, catching issues early

---

## 📊 **Test Coverage Matrix**

| Component | Unit Tests | Integration Tests | E2E Tests | Total |
|-----------|------------|-------------------|-----------|-------|
| **Common Auth** | 10 | 0 | 0 | 10 |
| **WE Handler** | 20 | 3 | 3 | 26 |
| **RAR Handler** | 20 | 3 | 3 | 26 |
| **NR Handler** | 20 | 3 | 3 | 26 |
| **Multi-CRD Flows** | 0 | 2 | 2 | 4 |
| **Security** | 0 | 0 | 2 | 2 |
| **Performance** | 0 | 0 | 1 | 1 |
| **TOTAL** | **70** | **11** | **14** | **95** |

---

## 📊 **Code Coverage Targets (Per TESTING_GUIDELINES.md v2.5.0)**

### **Defense-in-Depth Coverage Strategy**

**Principle**: 50%+ of webhook code is tested in **ALL 3 tiers**, ensuring authentication vulnerabilities must slip through multiple defense layers.

| Tier | Coverage Target | What It Validates | Measurement |
|------|----------------|-------------------|-------------|
| **Unit** | **70%+** | Handler logic, auth extraction, validation | `go test -v -p 4 -cover pkg/authwebhook/...` |
| **Integration** | **50%** | HTTP admission flow, TLS, webhook server | `go test -v -p 4 -cover test/integration/webhooks/...` |
| **E2E** | **50%** | Deployed webhook, K8s API, CRD operations | Binary coverage (`GOCOVERDIR`) |

### **Critical Path Validation**

**Example**: `WorkflowExecutionAuthHandler.Handle()`

| Tier | Coverage | What's Tested |
|------|----------|---------------|
| **Unit** | 100% | All code paths (success, errors, edge cases) |
| **Integration** | 80% | HTTP admission request handling, TLS handshake |
| **E2E** | 60% | Real CRD update, K8s webhook call, controller integration |

**Result**: Handler logic is validated at **3 different levels** - unit correctness, HTTP integration, and production deployment.

### **Coverage Collection Commands**

```bash
# Unit coverage (parallel execution per DD-TEST-002)
go test -v -p 4 -cover -coverprofile=unit-coverage.txt ./pkg/authwebhook/...
go tool cover -html=unit-coverage.txt -o unit-coverage.html

# Integration coverage (parallel execution per DD-TEST-002)
go test -v -p 4 -cover -coverprofile=integration-coverage.txt ./test/integration/webhooks/...
go tool cover -html=integration-coverage.txt -o integration-coverage.html

# E2E coverage (Go 1.20+)
GOFLAGS=-cover go build -o bin/webhooks-controller ./cmd/authwebhook/
# Deploy to Kind with GOCOVERDIR=/coverdata
# Run E2E tests (parallel execution per DD-TEST-002)
go test -v -p 4 ./test/e2e/webhooks/...
# Gracefully shutdown webhook
kubectl scale deployment kubernaut-auth-webhook --replicas=0 -n kubernaut-system
# Extract coverage
kind cp webhook-e2e:/tmp/webhook-coverdata ./coverdata
go tool covdata percent -i=./coverdata
go tool covdata textfmt -i=./coverdata -o e2e-coverage.txt
go tool cover -html=e2e-coverage.txt -o e2e-coverage.html
```

### **Coverage Validation (CI)**

```bash
# CI pipeline enforces minimum coverage targets (parallel per DD-TEST-002)
go test -v -p 4 -cover ./pkg/authwebhook/... | grep "coverage:" | awk '{if ($4 < 70.0) exit 1}'
go test -v -p 4 -cover ./test/integration/webhooks/... | grep "coverage:" | awk '{if ($4 < 50.0) exit 1}'
# E2E: Manual verification (binary coverage extraction)
```

**Key Insight**: With 70%/50%/50% code coverage targets, **50%+ of webhook code is tested in ALL 3 tiers** - ensuring authentication bugs must slip through multiple defense layers to reach production!

---

## 🧪 **Unit Tests (28 Tests, <1s Total)**

### **Component: Common Auth (`test/unit/authwebhook/`)**

**28 tests total (10 original + 5 missing + 13 additional edge cases), <100ms total**

**Framework**: Ginkgo/Gomega BDD (per project standard)
**Pattern**: `DescribeTable` + `Entry` for similar scenarios

---

## **📊 Comprehensive Test Case ID Reference (AUTH-001 to AUTH-023)**

### **Authenticator Tests (AUTH-001 to AUTH-012)**

| Test ID | Description | Category | BR | SOC2 | Status |
|---------|-------------|----------|----|----|---|
| **AUTH-001** | Extract Valid User Info | Authenticator | BR-AUTH-001 | CC8.1 | ✅ Implemented |
| **AUTH-002** | Reject Missing Username | Authenticator | BR-AUTH-001 | CC8.1 | ✅ Implemented |
| **AUTH-003** | Reject Empty UID | Authenticator | BR-AUTH-001 | CC8.1 | ✅ Implemented |
| **AUTH-004** | Extract Multiple Groups | Authenticator | BR-AUTH-001 | CC8.1 | ⬜ **To Implement** |
| **AUTH-009** | Extract User with No Groups | Authenticator | BR-AUTH-001 | CC8.1 | ⬜ **To Implement** |
| **AUTH-010** | Extract Service Account User | Authenticator | BR-AUTH-001 | CC8.1 | ⬜ **To Implement** |
| **AUTH-011** | Format Operator Identity | Authenticator | BR-AUTH-001 | CC8.1 | ✅ Implemented |
| **AUTH-012** | Reject Malformed Requests | Authenticator | BR-AUTH-001 | CC8.1 | ✅ Implemented |

### **Validator Tests - Reason (AUTH-005 to AUTH-016)**

| Test ID | Description | Category | BR | SOC2 | Status |
|---------|-------------|----------|----|----|---|
| **AUTH-005** | ValidateReason - Accept Valid | Validator | BR-AUTH-001 | CC7.4 | ✅ Implemented |
| **AUTH-006** | ValidateReason - Reject Empty | Validator | BR-AUTH-001 | CC7.4 | ✅ Implemented |
| **AUTH-007** | ValidateReason - Reject Too Long | Validator | BR-AUTH-001 | CC7.4 | ⬜ **To Implement** |
| **AUTH-008** | ValidateReason - Accept Max Length | Validator | BR-AUTH-001 | CC7.4 | ⬜ **To Implement** |
| **AUTH-013** | ValidateReason - Reject Vague | Validator | BR-AUTH-001 | CC7.4 | ✅ Implemented |
| **AUTH-014** | ValidateReason - Reject Single Word | Validator | BR-AUTH-001 | CC7.4 | ✅ Implemented |
| **AUTH-015** | ValidateReason - Reject Negative Min | Validator | BR-AUTH-001 | CC7.4 | ✅ Implemented |
| **AUTH-016** | ValidateReason - Reject Zero Min | Validator | BR-AUTH-001 | CC7.4 | ✅ Implemented |

### **Validator Tests - Timestamp (AUTH-017 to AUTH-023)**

| Test ID | Description | Category | BR | SOC2 | Status |
|---------|-------------|----------|----|----|---|
| **AUTH-017** | ValidateTimestamp - Accept Recent | Validator | BR-AUTH-001 | CC8.1 | ✅ Implemented |
| **AUTH-018** | ValidateTimestamp - Accept Boundary | Validator | BR-AUTH-001 | CC8.1 | ✅ Implemented |
| **AUTH-019** | ValidateTimestamp - Reject Future | Validator | BR-AUTH-001 | CC8.1 | ✅ Implemented |
| **AUTH-020** | ValidateTimestamp - Reject Slightly Future | Validator | BR-AUTH-001 | CC8.1 | ✅ Implemented |
| **AUTH-021** | ValidateTimestamp - Reject Stale | Validator | BR-AUTH-001 | CC8.1 | ✅ Implemented |
| **AUTH-022** | ValidateTimestamp - Reject Very Old | Validator | BR-AUTH-001 | CC8.1 | ✅ Implemented |
| **AUTH-023** | ValidateTimestamp - Reject Zero | Validator | BR-AUTH-001 | CC8.1 | ✅ Implemented |

### **Implementation Summary**

| Category | Total | Implemented | To Implement |
|----------|-------|-------------|--------------|
| **Authenticator** | 8 | 5 | 3 (AUTH-004, 009, 010) |
| **Validator (Reason)** | 8 | 6 | 2 (AUTH-007, 008) |
| **Validator (Timestamp)** | 7 | 7 | 0 |
| **TOTAL** | **23** | **18** | **5** |

**Note**: AUTH-011 to AUTH-023 are additional tests discovered during implementation that provide critical edge case coverage for SOC2 compliance (CC7.4 audit completeness, CC8.1 replay attack prevention).

---

#### **AUTH-001: Extract Valid User Info**

**Test Plan Reference**: Test 1
**BR**: BR-AUTH-001 (Operator Attribution)
**Expected**: Returns UserInfo with username, UID, and groups

```go
var _ = Describe("BR-AUTH-001: Authenticated User Extraction", func() {
    var (
        authenticator *authwebhook.Authenticator
        ctx           context.Context
    )

    BeforeEach(func() {
        authenticator = authwebhook.NewAuthenticator()
        ctx = context.Background()
    })

    Describe("ExtractUser - SOC2 CC8.1 Operator Attribution", func() {
        Context("when operator provides valid authentication", func() {
            It("AUTH-001: should extract username, UID, and groups", func() {
                // Test Plan: Test 1 - Extract Valid User Info
                req := &admissionv1.AdmissionRequest{
                    UserInfo: authv1.UserInfo{
                        Username: "operator@kubernaut.ai",
                        UID:      "k8s-user-abc-123",
                        Groups:   []string{"system:authenticated", "operators"},
                    },
                }

                authCtx, err := authenticator.ExtractUser(ctx, req)

                Expect(err).ToNot(HaveOccurred())
                Expect(authCtx.Username).To(Equal("operator@kubernaut.ai"))
                Expect(authCtx.UID).To(Equal("k8s-user-abc-123"))
                Expect(authCtx.Groups).To(HaveLen(2))
                Expect(authCtx.Groups).To(ConsistOf("system:authenticated", "operators"))
            })
        })
    })
})
```

---

#### **AUTH-002, AUTH-003, AUTH-004: User Extraction Validation (DescribeTable)**

**Pattern**: Use `DescribeTable` for similar test scenarios per project convention

```go
Context("when validating user extraction scenarios", func() {
    // Per TESTING_GUIDELINES.md: Use DescribeTable for similar test scenarios

    DescribeTable("AUTH-002 to AUTH-004: User extraction validation",
        func(testID, username, uid string, groups []string, shouldSucceed bool, businessOutcome string) {
            req := &admissionv1.AdmissionRequest{
                UserInfo: authv1.UserInfo{
                    Username: username,
                    UID:      uid,
                    Groups:   groups,
                },
            }

            authCtx, err := authenticator.ExtractUser(ctx, req)

            if shouldSucceed {
                Expect(err).ToNot(HaveOccurred(), businessOutcome)
                Expect(authCtx).ToNot(BeNil())
            } else {
                Expect(err).To(HaveOccurred(), businessOutcome)
                Expect(authCtx).To(BeNil())
            }
        },

        // AUTH-002: Reject Missing Username
        Entry("AUTH-002: Reject Missing Username",
            "AUTH-002",
            "", // Empty username
            "k8s-user-123",
            []string{"system:authenticated"},
            false,
            "SOC2 CC8.1 violation: Cannot attribute action without operator username"),

        // AUTH-003: Reject Empty UID
        Entry("AUTH-003: Reject Empty UID",
            "AUTH-003",
            "operator@kubernaut.ai",
            "", // Empty UID
            []string{"system:authenticated"},
            false,
            "SOC2 CC8.1 violation: Cannot uniquely identify operator without UID"),

        // AUTH-004: Extract Multiple Groups
        Entry("AUTH-004: Extract Multiple Groups",
            "AUTH-004",
            "operator@kubernaut.ai",
            "k8s-user-123",
            []string{"system:authenticated", "operators", "admins", "sre-team"},
            true,
            "All groups preserved for RBAC audit trail"),
    )
})
```

---

#### **AUTH-005, AUTH-006: ValidateReason Tests (DescribeTable)**

```go
Describe("ValidateReason - SOC2 CC7.4 Audit Completeness", func() {
    // Per TESTING_GUIDELINES.md: Use DescribeTable for similar test scenarios

    DescribeTable("AUTH-005 to AUTH-006: Reason validation",
        func(testID, reason string, minWords int, shouldSucceed bool, businessOutcome string) {
            err := authwebhook.ValidateReason(reason, minWords)

            if shouldSucceed {
                Expect(err).ToNot(HaveOccurred(), businessOutcome)
            } else {
                Expect(err).To(HaveOccurred(), businessOutcome)
            }
        },

        // AUTH-005: ValidateReason - Accept Valid Input
        Entry("AUTH-005: Accept valid justification meeting minimum standard",
            "AUTH-005",
            "Clearing block due to confirmed fix deployment in production environment",
            10,
            true,
            "SOC2 CC7.4: Valid justification enables audit completeness"),

        // AUTH-006: ValidateReason - Reject Empty Reason
        Entry("AUTH-006: Reject empty justification to enforce mandatory documentation",
            "AUTH-006",
            "",
            10,
            false,
            "SOC2 CC7.4 violation: Mandatory justification for audit completeness"),
    )
})

---

#### **AUTH-007, AUTH-008: ValidateReason Length Boundaries**

```go
Context("when validating reason length boundaries", func() {
    DescribeTable("AUTH-007 to AUTH-008: Length boundary validation",
        func(testID, reason string, minWords int, shouldSucceed bool, businessOutcome string) {
            err := authwebhook.ValidateReason(reason, minWords)

            if shouldSucceed {
                Expect(err).ToNot(HaveOccurred(), businessOutcome)
            } else {
                Expect(err).To(HaveOccurred(), businessOutcome)
            }
        },

        // AUTH-007: ValidateReason - Reject Overly Long Reason
        Entry("AUTH-007: Reject overly long justification (>100 words)",
            "AUTH-007",
            strings.Repeat("word ", 101), // 101 words
            10,
            false,
            "SOC2 CC7.4: Prevent excessively verbose justifications"),

        // AUTH-008: ValidateReason - Accept Reason at Max Length
        Entry("AUTH-008: Accept justification at maximum length boundary (100 words)",
            "AUTH-008",
            strings.Repeat("word ", 100), // Exactly 100 words
            10,
            true,
            "SOC2 CC7.4: Boundary validation for maximum length"),
    )
})
```

---

#### **AUTH-009, AUTH-010: Special User Scenarios**

```go
Context("when handling special user scenarios", func() {
    DescribeTable("AUTH-009 to AUTH-010: Special authentication scenarios",
        func(testID, username, uid string, groups []string, shouldSucceed bool, businessOutcome string) {
            req := &admissionv1.AdmissionRequest{
                UserInfo: authv1.UserInfo{
                    Username: username,
                    UID:      uid,
                    Groups:   groups,
                },
            }

            authCtx, err := authenticator.ExtractUser(ctx, req)

            if shouldSucceed {
                Expect(err).ToNot(HaveOccurred(), businessOutcome)
                Expect(authCtx).ToNot(BeNil())

                // Test-specific validations
                if testID == "AUTH-009" {
                    Expect(authCtx.Groups).To(BeEmpty(), "Empty groups list preserved")
                } else if testID == "AUTH-010" {
                    Expect(authCtx.Username).To(ContainSubstring("serviceaccount"), "Service account identified")
                    Expect(authCtx.Groups).To(ContainElement("system:serviceaccounts"), "SA groups preserved")
                }
            } else {
                Expect(err).To(HaveOccurred(), businessOutcome)
            }
        },

        // AUTH-009: Extract User with No Groups
        Entry("AUTH-009: Extract user with empty groups list",
            "AUTH-009",
            "operator@kubernaut.ai",
            "k8s-user-123",
            []string{}, // Empty groups
            true,
            "SOC2 CC8.1: Empty groups list is acceptable for audit attribution"),

        // AUTH-010: Extract Service Account User
        Entry("AUTH-010: Extract Kubernetes ServiceAccount user identity",
            "AUTH-010",
            "system:serviceaccount:kubernaut-system:webhook-controller",
            "sa-uid-789",
            []string{"system:serviceaccounts", "system:authenticated"},
            true,
            "SOC2 CC8.1: Service account identities supported for audit trail"),
    )
})
```

---

### **Component: WorkflowExecution Handler (`pkg/authwebhook/workflowexecution_handler_test.go`)**

**20 tests, <200ms total**

#### **Test 11: Populate clearedBy When Block Clearance Requested**
```go
func TestWEHandler_PopulateClearedBy(t *testing.T) {
    handler := &webhooks.WorkflowExecutionAuthHandler{}

    wfe := &workflowexecutionv1.WorkflowExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "test-wfe",
            Namespace: "default",
        },
        Status: workflowexecutionv1.WorkflowExecutionStatus{
            BlockClearanceRequest: &workflowexecutionv1.BlockClearanceRequest{
                Reason:    "Confirmed fix deployed",
                ClearedBy: "", // Empty = request being made
            },
        },
    }

    req := createMockAdmissionRequest(wfe, "operator@example.com")
    resp := handler.Handle(context.TODO(), req)

    assert.True(t, resp.Allowed)

    // Decode patched object
    patchedWFE := &workflowexecutionv1.WorkflowExecution{}
    _ = json.Unmarshal(resp.Patch, patchedWFE)

    assert.Equal(t, "operator@example.com", patchedWFE.Status.BlockClearanceRequest.ClearedBy)
    assert.NotNil(t, patchedWFE.Status.BlockClearanceRequest.ClearedAt)
}
```

**BR**: BR-WE-013 (Audit-Tracked Block Clearing)
**Expected**: clearedBy populated with authenticated user

---

#### **Test 12: Populate clearedAt Timestamp**
```go
func TestWEHandler_PopulateClearedAt(t *testing.T) {
    handler := &webhooks.WorkflowExecutionAuthHandler{}

    wfe := &workflowexecutionv1.WorkflowExecution{
        Status: workflowexecutionv1.WorkflowExecutionStatus{
            BlockClearanceRequest: &workflowexecutionv1.BlockClearanceRequest{
                Reason:    "Test",
                ClearedBy: "",
            },
        },
    }

    req := createMockAdmissionRequest(wfe, "operator@example.com")
    resp := handler.Handle(context.TODO(), req)

    patchedWFE := &workflowexecutionv1.WorkflowExecution{}
    _ = json.Unmarshal(resp.Patch, patchedWFE)

    assert.NotNil(t, patchedWFE.Status.BlockClearanceRequest.ClearedAt)
    assert.WithinDuration(t, time.Now(), patchedWFE.Status.BlockClearanceRequest.ClearedAt.Time, 5*time.Second)
}
```

**BR**: BR-WE-013 (Audit-Tracked Block Clearing)
**Expected**: clearedAt timestamp is current time

---

#### **Test 13: Reject Clearance Without Reason**
```go
func TestWEHandler_RejectMissingReason(t *testing.T) {
    handler := &webhooks.WorkflowExecutionAuthHandler{}

    wfe := &workflowexecutionv1.WorkflowExecution{
        Status: workflowexecutionv1.WorkflowExecutionStatus{
            BlockClearanceRequest: &workflowexecutionv1.BlockClearanceRequest{
                Reason:    "", // Missing
                ClearedBy: "",
            },
        },
    }

    req := createMockAdmissionRequest(wfe, "operator@example.com")
    resp := handler.Handle(context.TODO(), req)

    assert.False(t, resp.Allowed)
    assert.Contains(t, resp.Result.Message, "reason is required")
}
```

**BR**: BR-WE-013 (Audit-Tracked Block Clearing)
**Expected**: Missing reason rejected

---

#### **Test 14: Reject Clearance With Overly Long Reason**
```go
func TestWEHandler_RejectLongReason(t *testing.T) {
    handler := &webhooks.WorkflowExecutionAuthHandler{}

    wfe := &workflowexecutionv1.WorkflowExecution{
        Status: workflowexecutionv1.WorkflowExecutionStatus{
            BlockClearanceRequest: &workflowexecutionv1.BlockClearanceRequest{
                Reason:    strings.Repeat("a", 501), // Too long
                ClearedBy: "",
            },
        },
    }

    req := createMockAdmissionRequest(wfe, "operator@example.com")
    resp := handler.Handle(context.TODO(), req)

    assert.False(t, resp.Allowed)
    assert.Contains(t, resp.Result.Message, "exceeds maximum length")
}
```

**BR**: BR-WE-013 (Audit-Tracked Block Clearing)
**Expected**: Overly long reason rejected

---

#### **Test 15: Reject Unauthenticated Requests**
```go
func TestWEHandler_RejectUnauthenticated(t *testing.T) {
    handler := &webhooks.WorkflowExecutionAuthHandler{}

    wfe := &workflowexecutionv1.WorkflowExecution{
        Status: workflowexecutionv1.WorkflowExecutionStatus{
            BlockClearanceRequest: &workflowexecutionv1.BlockClearanceRequest{
                Reason:    "Test",
                ClearedBy: "",
            },
        },
    }

    req := createMockAdmissionRequest(wfe, "") // No username
    resp := handler.Handle(context.TODO(), req)

    assert.False(t, resp.Allowed)
    assert.Contains(t, resp.Result.Message, "authentication required")
}
```

**BR**: BR-AUTH-001 (Operator Attribution)
**Expected**: Unauthenticated request rejected

---

#### **Test 16: Do Nothing When No Block Clearance Requested**
```go
func TestWEHandler_NoBlockClearance(t *testing.T) {
    handler := &webhooks.WorkflowExecutionAuthHandler{}

    wfe := &workflowexecutionv1.WorkflowExecution{
        Status: workflowexecutionv1.WorkflowExecutionStatus{
            BlockClearanceRequest: nil, // No clearance
        },
    }

    req := createMockAdmissionRequest(wfe, "operator@example.com")
    resp := handler.Handle(context.TODO(), req)

    assert.True(t, resp.Allowed)
    assert.Nil(t, resp.Patch) // No mutation
}
```

**BR**: BR-WE-013 (Audit-Tracked Block Clearing)
**Expected**: No changes when no clearance requested

---

#### **Test 17: Handle Malformed WorkflowExecution**
```go
func TestWEHandler_MalformedObject(t *testing.T) {
    handler := &webhooks.WorkflowExecutionAuthHandler{}

    req := admission.Request{
        AdmissionRequest: admissionv1.AdmissionRequest{
            Object: runtime.RawExtension{
                Raw: []byte("{invalid json}"),
            },
        },
    }

    resp := handler.Handle(context.TODO(), req)

    assert.False(t, resp.Allowed)
    assert.Equal(t, http.StatusBadRequest, int(resp.Result.Code))
}
```

**BR**: BR-AUTH-001 (Operator Attribution)
**Expected**: Malformed JSON rejected gracefully

---

#### **Test 18: Preserve Existing ClearedBy**
```go
func TestWEHandler_PreserveExistingClearedBy(t *testing.T) {
    handler := &webhooks.WorkflowExecutionAuthHandler{}

    wfe := &workflowexecutionv1.WorkflowExecution{
        Status: workflowexecutionv1.WorkflowExecutionStatus{
            BlockClearanceRequest: &workflowexecutionv1.BlockClearanceRequest{
                Reason:    "Test",
                ClearedBy: "existing@example.com", // Already set
            },
        },
    }

    req := createMockAdmissionRequest(wfe, "new@example.com")
    resp := handler.Handle(context.TODO(), req)

    assert.True(t, resp.Allowed)

    patchedWFE := &workflowexecutionv1.WorkflowExecution{}
    _ = json.Unmarshal(resp.Patch, patchedWFE)

    // Should NOT overwrite existing clearedBy
    assert.Equal(t, "existing@example.com", patchedWFE.Status.BlockClearanceRequest.ClearedBy)
}
```

**BR**: BR-WE-013 (Audit-Tracked Block Clearing)
**Expected**: Existing clearedBy preserved

---

#### **Test 19-30: Additional WE Handler Tests**
- Test: Handle CREATE vs UPDATE operations
- Test: Handle WFE with multiple status updates
- Test: Validate reason contains only printable characters
- Test: Handle WFE in different namespaces
- Test: Handle concurrent clearance requests
- Test: Handle WFE with large status block
- Test: Validate ClearedAt timestamp format
- Test: Handle service account as operator
- Test: Reject clearance with special characters in username
- Test: Handle WFE with finalizers
- Test: Validate webhook doesn't modify non-clearance fields
- Test: Handle WFE with owner references

*(Similar pattern for RAR and NR handlers - 20 tests each)*

---

### **Component: RemediationApprovalRequest Handler**

**20 tests** (similar to WE handler):
- Test: Populate approvedBy when decision is Approved
- Test: Populate rejectedBy when decision is Rejected
- Test: Populate decidedAt timestamp
- Test: Reject decision without reason
- Test: Reject invalid decision (not Approved/Rejected)
- Test: Reject unauthenticated requests
- Test: Do nothing when no decision made
- Test: Handle malformed RAR
- Test: Preserve existing approvedBy/rejectedBy
- Test: Validate decision reason length
- *(10 more tests covering edge cases)*

---

### **Component: NotificationRequest Handler**

**20 tests** (similar to WE handler):
- Test: Add cancellation annotations on DELETE
- Test: Populate cancelled-by annotation
- Test: Populate cancelled-at timestamp
- Test: Reject unauthenticated DELETE requests
- Test: Ignore non-DELETE operations
- Test: Handle NR without existing annotations
- Test: Handle malformed NR
- Test: Preserve existing annotations
- Test: Validate annotation key format
- Test: Handle DELETE with finalizers
- *(10 more tests covering edge cases)*

---

## 🔗 **Integration Tests (11 Tests, ~10s Total)**

**Infrastructure**: Webhook server running in test, NO K8s cluster

### **Component: WorkflowExecution Integration (`test/integration/webhooks/workflowexecution_webhook_test.go`)**

**3 tests, ~3s total**

#### **Integration Test 1: E2E Block Clearance Attribution**
```go
var _ = Describe("WorkflowExecution Webhook Integration", func() {
    var (
        webhookURL string
        httpClient *http.Client
    )

    BeforeEach(func() {
        webhookURL = "https://127.0.0.1:9443/mutate-workflowexecution"
        httpClient = &http.Client{
            Transport: &http.Transport{
                TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
            },
        }
    })

    It("should populate clearedBy on block clearance request", func() {
        wfe := &workflowexecutionv1.WorkflowExecution{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "test-wfe",
                Namespace: "default",
            },
            Status: workflowexecutionv1.WorkflowExecutionStatus{
                BlockClearanceRequest: &workflowexecutionv1.BlockClearanceRequest{
                    Reason:    "Integration test clearance",
                    ClearedBy: "",
                },
            },
        }

        admissionReview := createAdmissionReview(wfe, "operator@example.com")

        resp, err := httpClient.Post(webhookURL, "application/json", admissionReview)
        Expect(err).ToNot(HaveOccurred())
        Expect(resp.StatusCode).To(Equal(http.StatusOK))

        // Parse response
        var reviewResp admissionv1.AdmissionReview
        _ = json.NewDecoder(resp.Body).Decode(&reviewResp)

        Expect(reviewResp.Response.Allowed).To(BeTrue())
        Expect(reviewResp.Response.Patch).ToNot(BeNil())

        // Verify patch contains clearedBy
        patchJSON := base64.StdEncoding.DecodeString(reviewResp.Response.Patch)
        Expect(string(patchJSON)).To(ContainSubstring("operator@example.com"))
    })
})
```

**BR**: BR-WE-013 (Audit-Tracked Block Clearing)
**Expected**: HTTP POST to webhook returns patched object with clearedBy

---

#### **Integration Test 2: Reject Unauthenticated Requests**
```go
It("should reject unauthenticated block clearance", func() {
    wfe := &workflowexecutionv1.WorkflowExecution{
        Status: workflowexecutionv1.WorkflowExecutionStatus{
            BlockClearanceRequest: &workflowexecutionv1.BlockClearanceRequest{
                Reason:    "Test",
                ClearedBy: "",
            },
        },
    }

    admissionReview := createAdmissionReview(wfe, "") // No username

    resp, err := httpClient.Post(webhookURL, "application/json", admissionReview)
    Expect(err).ToNot(HaveOccurred())

    var reviewResp admissionv1.AdmissionReview
    _ = json.NewDecoder(resp.Body).Decode(&reviewResp)

    Expect(reviewResp.Response.Allowed).To(BeFalse())
    Expect(reviewResp.Response.Result.Message).To(ContainSubstring("authentication required"))
})
```

**BR**: BR-AUTH-001 (Operator Attribution)
**Expected**: Unauthenticated request rejected via HTTP

---

#### **Integration Test 3: TLS Certificate Validation**
```go
It("should require valid TLS certificates", func() {
    // Use strict TLS client (no InsecureSkipVerify)
    strictClient := &http.Client{
        Transport: &http.Transport{
            TLSClientConfig: &tls.Config{
                RootCAs: loadTestCA(),
            },
        },
    }

    wfe := &workflowexecutionv1.WorkflowExecution{
        Status: workflowexecutionv1.WorkflowExecutionStatus{
            BlockClearanceRequest: &workflowexecutionv1.BlockClearanceRequest{
                Reason:    "Test",
                ClearedBy: "",
            },
        },
    }

    admissionReview := createAdmissionReview(wfe, "operator@example.com")

    resp, err := strictClient.Post(webhookURL, "application/json", admissionReview)
    Expect(err).ToNot(HaveOccurred())
    Expect(resp.StatusCode).To(Equal(http.StatusOK))
})
```

**BR**: BR-SECURITY-001 (TLS Encryption)
**Expected**: Webhook requires valid TLS certificates

---

### **Component: RemediationApprovalRequest Integration**

**3 tests** (similar to WE integration):
- Test: Approve RAR with authenticated user
- Test: Reject RAR with authenticated user
- Test: Reject unauthenticated approval

---

### **Component: NotificationRequest Integration**

**3 tests** (similar to WE integration):
- Test: DELETE NR with authenticated user
- Test: Reject unauthenticated DELETE
- Test: Verify annotations added on DELETE

---

### **Component: Multi-CRD Flows**

**2 tests**:

#### **Integration Test 10: Multiple CRDs in Sequence**
```go
It("should handle multiple CRD types in sequence", func() {
    // 1. Create WFE with block clearance
    weResp := postWebhook("/mutate-workflowexecution", createWFE("operator1@example.com"))
    Expect(weResp.Response.Allowed).To(BeTrue())

    // 2. Create RAR with approval
    rarResp := postWebhook("/mutate-remediationapprovalrequest", createRAR("operator2@example.com"))
    Expect(rarResp.Response.Allowed).To(BeTrue())

    // 3. DELETE NR
    nrResp := postWebhook("/validate-notificationrequest-delete", createNRDelete("operator3@example.com"))
    Expect(nrResp.Response.Allowed).To(BeTrue())
})
```

**BR**: BR-AUTH-001 (Operator Attribution)
**Expected**: All 3 CRD types handled by same webhook

---

#### **Integration Test 11: Concurrent Requests**
```go
It("should handle concurrent webhook requests", func() {
    var wg sync.WaitGroup
    results := make(chan bool, 10)

    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(i int) {
            defer wg.Done()
            wfe := createWFE(fmt.Sprintf("operator%d@example.com", i))
            resp := postWebhook("/mutate-workflowexecution", wfe)
            results <- resp.Response.Allowed
        }(i)
    }

    wg.Wait()
    close(results)

    successCount := 0
    for allowed := range results {
        if allowed {
            successCount++
        }
    }

    Expect(successCount).To(Equal(10))
})
```

**BR**: BR-PERFORMANCE-001 (Webhook Performance)
**Expected**: Webhook handles concurrent requests without errors

---

## 🚀 **E2E Tests (14 Tests, ~60s Total)**

**Infrastructure**: Real K8s cluster (Kind), complete deployment

### **Component: WorkflowExecution E2E (`test/e2e/webhooks/10_webhook_auth_test.go`)**

**3 tests, ~20s total**

#### **E2E Test 1: Complete Block Clearance Flow**
```go
var _ = Describe("E2E: WorkflowExecution Block Clearance", func() {
    It("should capture operator identity in complete flow", func() {
        ctx := context.Background()

        // 1. Create WorkflowExecution (via kubectl)
        wfe := &workflowexecutionv1.WorkflowExecution{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "e2e-wfe",
                Namespace: "default",
            },
            Spec: workflowexecutionv1.WorkflowExecutionSpec{
                WorkflowName: "test-workflow",
            },
        }
        Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

        // 2. Wait for controller to detect block (simulated)
        Eventually(func() string {
            _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), wfe)
            return wfe.Status.Phase
        }, 30*time.Second, 1*time.Second).Should(Equal("Blocked"))

        // 3. Operator clears block (triggers webhook)
        wfe.Status.BlockClearanceRequest = &workflowexecutionv1.BlockClearanceRequest{
            Reason:    "E2E test clearance",
            ClearedBy: "", // Will be populated by webhook
        }
        Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())

        // 4. Verify webhook populated clearedBy
        Eventually(func() string {
            _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), wfe)
            return wfe.Status.BlockClearanceRequest.ClearedBy
        }, 10*time.Second, 1*time.Second).ShouldNot(BeEmpty())

        Expect(wfe.Status.BlockClearanceRequest.ClearedBy).To(ContainSubstring("@"))
        Expect(wfe.Status.BlockClearanceRequest.ClearedAt).ToNot(BeNil())

        // 5. Verify controller processed clearance
        Eventually(func() string {
            _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), wfe)
            return wfe.Status.Phase
        }, 30*time.Second, 1*time.Second).Should(Equal("Running"))

        // 6. Verify audit event emitted
        auditEvents := queryAuditEvents("workflowexecution.block.cleared", wfe.Name)
        Expect(auditEvents).To(HaveLen(1))
        Expect(auditEvents[0].ActorId).To(Equal(wfe.Status.BlockClearanceRequest.ClearedBy))
    })
})
```

**BR**: BR-WE-013 (Audit-Tracked Block Clearing)
**Expected**: Complete flow from kubectl → webhook → controller → audit

---

#### **E2E Test 2: Verify Controller Detects Webhook Changes**
```go
It("should trigger controller reconciliation after webhook mutation", func() {
    ctx := context.Background()

    wfe := &workflowexecutionv1.WorkflowExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "e2e-controller-detect",
            Namespace: "default",
        },
        Spec: workflowexecutionv1.WorkflowExecutionSpec{
            WorkflowName: "test-workflow",
        },
        Status: workflowexecutionv1.WorkflowExecutionStatus{
            Phase: "Blocked",
        },
    }
    Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

    // Update status to trigger clearance (webhook will populate clearedBy)
    wfe.Status.BlockClearanceRequest = &workflowexecutionv1.BlockClearanceRequest{
        Reason: "Test",
    }
    Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())

    // Verify controller reconciled (phase changed)
    Eventually(func() string {
        _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), wfe)
        return wfe.Status.Phase
    }, 30*time.Second, 1*time.Second).Should(Equal("Running"))
})
```

**BR**: BR-WE-013 (Audit-Tracked Block Clearing)
**Expected**: Controller reacts to webhook mutations

---

#### **E2E Test 3: Verify Audit Event Contains Correct Actor**
```go
It("should emit audit event with webhook-populated actor", func() {
    ctx := context.Background()

    wfe := createAndClearWFE(ctx, "e2e-audit-actor")

    // Query audit events
    auditEvents := queryAuditEvents("workflowexecution.block.cleared", wfe.Name)
    Expect(auditEvents).To(HaveLen(1))

    // Verify actor matches webhook-populated field
    Expect(auditEvents[0].ActorId).To(Equal(wfe.Status.BlockClearanceRequest.ClearedBy))
    Expect(auditEvents[0].ActorType).To(Equal("user"))
    Expect(auditEvents[0].EventOutcome).To(Equal("success"))
})
```

**BR**: BR-AUDIT-001 (Audit Attribution)
**Expected**: Audit event reflects webhook-populated actor

---

### **Component: RemediationApprovalRequest E2E**

**3 tests** (similar to WE E2E):
- Test: Complete approval flow with attribution
- Test: Complete rejection flow with attribution
- Test: Verify audit events for approval/rejection

---

### **Component: NotificationRequest E2E**

**3 tests** (similar to WE E2E):
- Test: Complete cancellation flow (DELETE) with attribution
- Test: Verify annotations added before finalizer cleanup
- Test: Verify audit event for cancellation

---

### **Component: Multi-CRD E2E Flows**

**2 tests**:

#### **E2E Test 10: Complete SOC2 Attribution Across All CRDs**
```go
It("should attribute all operator actions to authenticated users", func() {
    ctx := context.Background()

    // 1. Block clearance
    wfe := createAndClearWFE(ctx, "soc2-wfe")
    Expect(wfe.Status.BlockClearanceRequest.ClearedBy).ToNot(BeEmpty())

    // 2. Approval decision
    rar := createAndApproveRAR(ctx, "soc2-rar")
    Expect(rar.Status.ApprovedBy).ToNot(BeEmpty())

    // 3. Notification cancellation
    nr := createAndDeleteNR(ctx, "soc2-nr")
    Expect(nr.Annotations["kubernaut.ai/cancelled-by"]).ToNot(BeEmpty())

    // 4. Verify all 3 audit events have correct actors
    weAudit := queryAuditEvents("workflowexecution.block.cleared", wfe.Name)
    rarAudit := queryAuditEvents("orchestrator.approval.approved", rar.Name)
    nrAudit := queryAuditEvents("notification.request.cancelled", nr.Name)

    Expect(weAudit).To(HaveLen(1))
    Expect(rarAudit).To(HaveLen(1))
    Expect(nrAudit).To(HaveLen(1))

    // SOC2 CC8.1: All operator actions attributed
    Expect(weAudit[0].ActorId).ToNot(BeEmpty())
    Expect(rarAudit[0].ActorId).ToNot(BeEmpty())
    Expect(nrAudit[0].ActorId).ToNot(BeEmpty())
})
```

**BR**: BR-AUDIT-001 (SOC2 CC8.1 Attribution)
**Expected**: All operator actions have authenticated actors

---

#### **E2E Test 11: Verify Unauthenticated Requests Rejected**
```go
It("should reject unauthenticated kubectl operations", func() {
    // Use unauthenticated K8s client (if possible in test env)
    // OR verify webhook rejects requests with empty UserInfo

    // This test may require custom K8s client config
    // to simulate unauthenticated requests
})
```

**BR**: BR-SECURITY-001 (Authentication Enforcement)
**Expected**: Unauthenticated operations rejected

---

### **Component: Security E2E**

**2 tests**:

#### **E2E Test 12: Verify TLS Enforcement**
```go
It("should enforce TLS for webhook communication", func() {
    // Attempt HTTP (not HTTPS) connection to webhook
    httpClient := &http.Client{}
    _, err := httpClient.Post("http://kubernaut-auth-webhook:9443/mutate-workflowexecution", "application/json", nil)

    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("tls"))
})
```

**BR**: BR-SECURITY-001 (TLS Encryption)
**Expected**: HTTP rejected, HTTPS required

---

#### **E2E Test 13: Verify RBAC Permissions**
```go
It("should require correct RBAC permissions for webhook service account", func() {
    ctx := context.Background()

    // Verify webhook service account has correct ClusterRole
    sa := &corev1.ServiceAccount{}
    Expect(k8sClient.Get(ctx, types.NamespacedName{
        Name:      "kubernaut-auth-webhook",
        Namespace: "kubernaut-system",
    }, sa)).To(Succeed())

    // Verify ClusterRoleBinding exists
    crb := &rbacv1.ClusterRoleBinding{}
    Expect(k8sClient.Get(ctx, types.NamespacedName{
        Name: "kubernaut-auth-webhook",
    }, crb)).To(Succeed())

    Expect(crb.RoleRef.Name).To(Equal("kubernaut-auth-webhook"))
})
```

**BR**: BR-SECURITY-001 (RBAC Enforcement)
**Expected**: Webhook has minimal required permissions

---

### **Component: Performance E2E**

**1 test**:

#### **E2E Test 14: Verify Webhook Latency < 50ms**
```go
It("should complete webhook mutations in < 50ms", func() {
    ctx := context.Background()

    latencies := []time.Duration{}

    for i := 0; i < 10; i++ {
        wfe := &workflowexecutionv1.WorkflowExecution{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("perf-wfe-%d", i),
                Namespace: "default",
            },
            Status: workflowexecutionv1.WorkflowExecutionStatus{
                BlockClearanceRequest: &workflowexecutionv1.BlockClearanceRequest{
                    Reason: "Performance test",
                },
            },
        }

        start := time.Now()
        Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())
        latency := time.Since(start)

        latencies = append(latencies, latency)
    }

    // Calculate average latency
    var total time.Duration
    for _, l := range latencies {
        total += l
    }
    avgLatency := total / time.Duration(len(latencies))

    Expect(avgLatency).To(BeNumerically("<", 50*time.Millisecond))
})
```

**BR**: BR-PERFORMANCE-001 (Webhook Performance)
**Expected**: Webhook latency < 50ms

---

## ✅ **Test Execution Plan**

### **Quick Reference: Make Targets**

All webhook tests can be executed using standardized make targets that follow the existing kubernaut pattern (see `docs/development/SOC2/WEBHOOK_MAKEFILE_TRIAGE.md` for details):

```bash
# Run all test tiers (Unit + Integration + E2E)
make test-all-authwebhook

# Run specific tier
make test-unit-authwebhook
make test-integration-authwebhook
make test-e2e-authwebhook

# Run with coverage collection
make test-coverage-authwebhook              # Unit coverage
make test-coverage-integration-authwebhook  # Integration coverage
make test-coverage-e2e-authwebhook          # E2E coverage (binary coverage)
make test-coverage-all-authwebhook          # All tiers with coverage

# Cleanup
make clean-authwebhook-integration
```

**Benefits of Make Targets**:
- ✅ Auto-detects CPU cores for parallel execution (DD-TEST-002)
- ✅ Consistent timeout values per tier
- ✅ Programmatic infrastructure setup (follows AIAnalysis pattern)
- ✅ Coverage collection with proper flags
- ✅ Follows existing kubernaut patterns

---

### **Day 1: Common Auth Tests** (30 min)
```bash
# Run unit tests for common auth logic
cd pkg/authwebhook/auth
go test -v -cover

# Expected: 10 tests passing, >90% coverage
```

---

### **Day 2: WorkflowExecution Tests** (2 hours)
```bash
# Unit tests
cd pkg/authwebhook
go test -v -run TestWEHandler -cover

# Integration tests
cd test/integration/webhooks
ginkgo -v --label-filter="integration && workflowexecution"

# Expected: 20 unit + 3 integration tests passing
```

---

### **Day 3: RAR Tests** (2 hours)
```bash
# Similar to Day 2
go test -v -run TestRARHandler -cover
ginkgo -v --label-filter="integration && rar"

# Expected: 20 unit + 3 integration tests passing
```

---

### **Day 4: NR Tests** (2 hours)
```bash
# Similar to Day 2/3
go test -v -run TestNRHandler -cover
ginkgo -v --label-filter="integration && notificationrequest"

# Expected: 20 unit + 3 integration tests passing
```

---

### **Day 5-6: E2E Tests** (4 hours)
```bash
# Start Kind cluster
kind create cluster --name webhook-e2e

# Deploy webhook service
kubectl apply -k deploy/webhooks/

# Run E2E tests
cd test/e2e/webhooks
ginkgo -v --label-filter="e2e && webhook"

# Expected: 14 E2E tests passing
```

---

## 📊 **Test Success Criteria**

| Tier | Target | Status | Evidence |
|------|--------|--------|----------|
| **Unit Tests** | 70 passing, >80% coverage | ⬜ | `go test -cover` output |
| **Integration Tests** | 11 passing, <10s total | ⬜ | `ginkgo` output |
| **E2E Tests** | 14 passing, <60s total | ⬜ | Kind cluster logs |
| **SOC2 Compliance** | 100% attribution | ⬜ | Audit event validation |
| **Performance** | <50ms latency | ⬜ | E2E performance test |

---

## 🚨 **Test Failure Response**

**When tests fail**:
1. **DO NOT** skip tests
2. **DO NOT** change expectations to pass
3. **INVESTIGATE** root cause
4. **FIX** implementation or test (as appropriate)
5. **DOCUMENT** fix in commit message

**Example Failure Investigation**:
```bash
# Test failed: TestWEHandler_PopulateClearedBy

# Step 1: Read test output
go test -v -run TestWEHandler_PopulateClearedBy

# Step 2: Check handler implementation
cat pkg/authwebhook/workflowexecution_handler.go

# Step 3: Debug with print statements
# (Add logging to handler)

# Step 4: Re-run test
go test -v -run TestWEHandler_PopulateClearedBy

# Step 5: Commit fix
git commit -m "fix(webhook): populate clearedBy correctly"
```

---

## 📚 **References**

- **DD-TESTING-001**: Audit Event Validation Standards
- **DD-AUTH-001**: Shared Authentication Webhook
- **BR-WE-013**: Audit-Tracked Block Clearing
- **SOC2 CC8.1**: Change Control - Attribution
- **Ginkgo/Gomega**: https://onsi.github.io/ginkgo/
- **controller-runtime testing**: https://book.kubebuilder.io/cronjob-tutorial/writing-tests.html

---

**Status**: ✅ **READY FOR TDD IMPLEMENTATION**
**Test Count**: 95 tests (70 unit + 11 integration + 14 E2E)
**Timeline**: 5-6 days (aligned with implementation plan)
**Owner**: Webhook Team
**Next Step**: Begin Day 1 unit tests (common auth logic)

