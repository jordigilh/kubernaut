# Authentication Webhook - Implementation Ready
**Date**: January 6, 2026
**Status**: ✅ **READY TO BEGIN TDD DAY 1**
**Commits**: d73487cb1, 0dbb02f63, 229cd9ffe, 6cb7108b0, 21624857f, d18fecdef

---

## ✅ **INFRASTRUCTURE COMPLETE**

### **1. Port Allocations** (DD-TEST-001 v2.1)

| Tier | PostgreSQL | Redis | Data Storage | Status |
|------|-----------|-------|--------------|--------|
| **Integration** | 15442 | 16386 | 18099 | ✅ Conflict-free |
| **E2E** | 25442 | 26386 | 28099 | ✅ Conflict-free |

**Authority**: `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md` v2.1

### **2. Make Targets** (Added to Makefile)

```bash
make test-unit-authwebhook              # Unit tests (coverage enabled)
make test-integration-authwebhook       # Integration tests (coverage enabled)
make test-e2e-authwebhook               # E2E tests (coverage enabled)
make test-all-authwebhook               # All 3 tiers
make clean-authwebhook-integration      # Cleanup
```

**Verification**:
```bash
$ make help | grep authwebhook
  test-unit-authwebhook          Run authentication webhook unit tests
  test-integration-authwebhook   Run webhook integration tests (envtest + real CRDs)
  test-e2e-authwebhook           Run webhook E2E tests (Kind cluster)
  test-all-authwebhook           Run all webhook test tiers (Unit + Integration + E2E)
  clean-authwebhook-integration  Clean webhook integration test infrastructure
```

### **3. Architecture Decisions** (Approved)

| Decision | Status | Document |
|----------|--------|----------|
| **Webhook Updates Status Directly** | ✅ Confirmed | DD-AUTH-001, ADR-051 |
| **Mutual Exclusion Pattern** | ✅ Approved | ADR-051 lines 337-650 |
| **Single Consolidated Webhook** | ✅ Approved | DD-AUTH-001 |
| **3 CRDs (WE, RAR, NR)** | ✅ Approved | DD-WEBHOOK-001 v1.2 |

**Key Architectural Clarification**:
- ✅ Webhook DOES update status fields directly (not via controller)
- ✅ Webhook has access to `req.UserInfo` (authenticated context)
- ✅ ValidateUpdate() enforces mutual exclusion (controller vs operator vs webhook fields)
- ✅ This is the CORRECT pattern for authentication (SOC2 CC8.1)

---

## 📋 **TDD DAY 1: UNIT TESTS**

### **What to Implement**

#### **File**: `pkg/authwebhook/authenticator.go`

```go
package authwebhook

import (
    "context"
    "fmt"

    admissionv1 "k8s.io/api/admission/v1"
    authv1 "k8s.io/api/authentication/v1"
)

// AuthContext holds authenticated user information
type AuthContext struct {
    Username string
    UID      string
}

// String returns formatted authentication string
func (a *AuthContext) String() string {
    return fmt.Sprintf("%s (UID: %s)", a.Username, a.UID)
}

// Authenticator extracts authenticated user identity from admission requests
type Authenticator struct {}

// NewAuthenticator creates a new authenticator
func NewAuthenticator() *Authenticator {
    return &Authenticator{}
}

// ExtractUser extracts authenticated user from admission request
// Returns error if user info is missing or invalid
func (a *Authenticator) ExtractUser(ctx context.Context, req *admissionv1.AdmissionRequest) (*AuthContext, error) {
    // TDD: Write tests first!
    panic("implement me")
}
```

#### **File**: `pkg/authwebhook/validator.go`

```go
package authwebhook

import (
    "fmt"
    "time"
)

// ValidateReason validates clearance/approval reason
// min = 10 words minimum for audit trail
func ValidateReason(reason string, minWords int) error {
    // TDD: Write tests first!
    panic("implement me")
}

// ValidateTimestamp validates request timestamp is not in future
// and not older than 5 minutes (replay attack prevention)
func ValidateTimestamp(ts time.Time) error {
    // TDD: Write tests first!
    panic("implement me")
}
```

### **Test Files to Create**

#### **File**: `test/unit/authwebhook/authenticator_test.go`

```go
package authwebhook_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/authwebhook"
    admissionv1 "k8s.io/api/admission/v1"
    authv1 "k8s.io/api/authentication/v1"
)

var _ = Describe("Authenticator", func() {
    var authenticator *authwebhook.Authenticator

    BeforeEach(func() {
        authenticator = authwebhook.NewAuthenticator()
    })

    Describe("ExtractUser", func() {
        Context("when admission request has valid user info", func() {
            It("should extract username and UID", func() {
                req := &admissionv1.AdmissionRequest{
                    UserInfo: authv1.UserInfo{
                        Username: "admin@example.com",
                        UID:      "abc-123-def",
                    },
                }

                authCtx, err := authenticator.ExtractUser(ctx, req)
                Expect(err).ToNot(HaveOccurred())
                Expect(authCtx.Username).To(Equal("admin@example.com"))
                Expect(authCtx.UID).To(Equal("abc-123-def"))
                Expect(authCtx.String()).To(Equal("admin@example.com (UID: abc-123-def)"))
            })
        })

        Context("when username is missing", func() {
            It("should return error", func() {
                req := &admissionv1.AdmissionRequest{
                    UserInfo: authv1.UserInfo{
                        UID: "abc-123",
                    },
                }

                _, err := authenticator.ExtractUser(ctx, req)
                Expect(err).To(HaveOccurred())
                Expect(err.Error()).To(ContainSubstring("username is required"))
            })
        })

        Context("when UID is missing", func() {
            It("should return error", func() {
                req := &admissionv1.AdmissionRequest{
                    UserInfo: authv1.UserInfo{
                        Username: "admin@example.com",
                    },
                }

                _, err := authenticator.ExtractUser(ctx, req)
                Expect(err).To(HaveOccurred())
                Expect(err.Error()).To(ContainSubstring("UID is required"))
            })
        })
    })
})
```

#### **File**: `test/unit/authwebhook/validator_test.go`

```go
package authwebhook_test

import (
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/authwebhook"
)

var _ = Describe("Validator", func() {
    Describe("ValidateReason", func() {
        Context("when reason has sufficient words", func() {
            It("should pass validation", func() {
                reason := "Investigation complete after root cause analysis confirmed memory leak in payment service"
                err := authwebhook.ValidateReason(reason, 10)
                Expect(err).ToNot(HaveOccurred())
            })
        })

        Context("when reason is too short", func() {
            It("should return error", func() {
                reason := "Fixed it"
                err := authwebhook.ValidateReason(reason, 10)
                Expect(err).To(HaveOccurred())
                Expect(err.Error()).To(ContainSubstring("minimum 10 words required"))
            })
        })

        Context("when reason is empty", func() {
            It("should return error", func() {
                err := authwebhook.ValidateReason("", 10)
                Expect(err).To(HaveOccurred())
                Expect(err.Error()).To(ContainSubstring("reason cannot be empty"))
            })
        })
    })

    Describe("ValidateTimestamp", func() {
        Context("when timestamp is current", func() {
            It("should pass validation", func() {
                ts := time.Now().Add(-30 * time.Second)
                err := authwebhook.ValidateTimestamp(ts)
                Expect(err).ToNot(HaveOccurred())
            })
        })

        Context("when timestamp is in the future", func() {
            It("should return error", func() {
                ts := time.Now().Add(1 * time.Hour)
                err := authwebhook.ValidateTimestamp(ts)
                Expect(err).To(HaveOccurred())
                Expect(err.Error()).To(ContainSubstring("timestamp cannot be in the future"))
            })
        })

        Context("when timestamp is too old", func() {
            It("should return error (replay attack prevention)", func() {
                ts := time.Now().Add(-10 * time.Minute)
                err := authwebhook.ValidateTimestamp(ts)
                Expect(err).To(HaveOccurred())
                Expect(err.Error()).To(ContainSubstring("timestamp too old"))
            })
        })
    })
})
```

#### **File**: `test/unit/authwebhook/suite_test.go`

```go
package authwebhook_test

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestAuthWebhookUnit(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "AuthWebhook Unit Suite")
}
```

---

## 🚀 **BEGIN TDD DAY 1**

### **Step 1: Create Directory Structure**

```bash
mkdir -p pkg/authwebhook
mkdir -p test/unit/authwebhook
```

### **Step 2: Create Test Suite**

```bash
# Copy suite_test.go from above
```

### **Step 3: RED Phase (Tests First)**

```bash
# Copy authenticator_test.go and validator_test.go from above

# Run tests (should FAIL)
make test-unit-authwebhook

# Expected output:
# ❌ FAIL: panic: implement me
```

### **Step 4: GREEN Phase (Minimal Implementation)**

```bash
# Implement pkg/authwebhook/authenticator.go (minimal to pass tests)
# Implement pkg/authwebhook/validator.go (minimal to pass tests)

# Run tests (should PASS)
make test-unit-authwebhook

# Expected output:
# ✅ PASS: 10 tests passing, >80% coverage
```

### **Step 5: REFACTOR Phase (Enhance)**

```bash
# Add error messages, edge cases, sophisticated validation
# Run tests continuously
make test-unit-authwebhook
```

---

## 📊 **EXPECTED TDD DAY 1 TIMELINE**

| Phase | Duration | Tasks | Deliverable |
|-------|----------|-------|-------------|
| **RED** | 10-15 min | Write failing tests | 10 test specs (all failing) |
| **GREEN** | 15-20 min | Minimal implementation | All tests passing, ~70% coverage |
| **REFACTOR** | 20-30 min | Enhance implementation | Clean code, >80% coverage |
| **Total** | **45-65 min** | Complete unit tests | Ready for Day 2 integration |

---

## ✅ **SUCCESS CRITERIA - DAY 1**

- [ ] All 10 unit tests passing
- [ ] Coverage >70% for `pkg/authwebhook/`
- [ ] No linter errors
- [ ] Builds successfully: `make build-authwebhook` (when cmd/authwebhook/ exists)
- [ ] Tests run in parallel: `make test-unit-authwebhook` uses 4 procs

---

## 📚 **REFERENCE DOCUMENTS**

| Document | Purpose |
|----------|---------|
| `DD-AUTH-001` | Webhook architecture and patterns |
| `DD-WEBHOOK-001` | CRD requirements matrix |
| `DD-TEST-001 v2.1` | Port allocations |
| `WEBHOOK_IMPLEMENTATION_PLAN.md` | 5-6 day roadmap |
| `WEBHOOK_TEST_PLAN.md` | Complete test strategy |
| `ADR-051` | Webhook scaffolding with mutual exclusion |

---

## 🎯 **READY TO START**

✅ **All infrastructure complete**
✅ **Make targets available**
✅ **Port allocations conflict-free**
✅ **Architecture decisions confirmed**
✅ **TDD Day 1 plan documented**

**Next Command**:
```bash
make test-unit-authwebhook
# Should output: "no test files" (expected - we haven't created them yet)

# Create test/unit/authwebhook/suite_test.go
# Create test/unit/authwebhook/authenticator_test.go
# Run again - tests should FAIL (RED phase)
# Implement pkg/authwebhook/authenticator.go
# Run again - tests should PASS (GREEN phase)
```

---

**Status**: ✅ **IMPLEMENTATION READY - BEGIN TDD DAY 1**
**Date**: 2026-01-06
**Owner**: Webhook Team

