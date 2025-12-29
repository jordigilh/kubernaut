# Response: Notification Team - Integration Test Mock Violations

**Date**: December 10, 2025
**From**: Notification Team
**Re**: NOTICE_INTEGRATION_TEST_MOCK_VIOLATIONS.md
**Status**: ✅ **ACCEPTABLE WITH CLARIFICATION**

---

## 🎯 Decision

- [x] Acceptable (document rationale)

---

## 📋 Analysis

### Issue Identified

The notice identifies `test/integration/notification/suite_test.go` (lines 171-173) using `NewTestAuditStore()` instead of real Data Storage.

### Defense-in-Depth Testing Architecture

The Notification service uses a **layered audit testing strategy** per 03-testing-strategy.mdc:

| Layer | Location | Purpose | Uses Real DS? |
|-------|----------|---------|---------------|
| **Layer 1** (Unit) | `pkg/audit/*` | Audit library core functions | N/A |
| **Layer 2** (Unit) | `test/unit/notification/audit_test.go` | Audit helper methods (46 tests) | No |
| **Layer 3** (Integration) | `audit_integration_test.go` | AuditStore → DataStorage → PostgreSQL | **YES** ✅ |
| **Layer 4** (Integration) | `controller_audit_emission_test.go` | Controller → Audit emission at lifecycle | No (mock) |
| **Layer 5** (E2E) | `01_notification_lifecycle_audit_test.go` | Full chain with Kind cluster | **YES** ✅ |

### Why TestAuditStore is Acceptable in `suite_test.go`

The `TestAuditStore` is used specifically for **Layer 4** tests (`controller_audit_emission_test.go`) which verify:
- Controller emits `notification.message.sent` at correct lifecycle point
- Controller emits `notification.message.failed` at correct lifecycle point
- Events contain correct correlation_id
- Events contain correct ADR-034 fields

**This is NOT testing Data Storage integration** - that's done in Layer 3 and Layer 5.

### Real Data Storage Integration Exists

**Layer 3** (`audit_integration_test.go`) already tests with real infrastructure:
```go
// Uses DD-TEST-001 ports
dataStorageURL = "http://localhost:18090"  // Real Data Storage
postgresURL = "postgres://slm_user:slm_password@localhost:15433/action_history"  // Real PostgreSQL
```

**Layer 5** (E2E) uses:
```go
// Deployed via DeployNotificationAuditInfrastructure()
dataStorageNodePort = 30090  // Real Data Storage in Kind
```

---

## 🔴 Issue Found During Triage

**`audit_integration_test.go` contains Skip() calls which violate TESTING_GUIDELINES.md!**

```go
// Line 92 - VIOLATION
Skip(fmt.Sprintf("PostgreSQL not available: %v ...", err))

// Line 97 - VIOLATION
Skip(fmt.Sprintf("PostgreSQL not reachable: %v", err))
```

Per TESTING_GUIDELINES.md lines 420-549: **Skip() is ABSOLUTELY FORBIDDEN**.

**Action Required**: Replace Skip() with Fail() in audit_integration_test.go.

---

## ✅ Summary

| Aspect | Status |
|--------|--------|
| TestAuditStore in suite_test.go | ✅ Acceptable (Layer 4 - controller behavior) |
| Real DS in audit_integration_test.go | ✅ Exists (Layer 3) |
| Real DS in E2E tests | ✅ Exists (Layer 5) |
| Skip() violations found | 🔴 **NEEDS FIX** |

---

## 📋 Action Items

| # | Task | Priority | Status |
|---|------|----------|--------|
| 1 | Fix Skip() calls in audit_integration_test.go | HIGH | ✅ **COMPLETE** |
| 2 | Document defense-in-depth testing layers | DONE | ✅ This document |

### Skip() Fix Details

Replaced 3 Skip() calls in `audit_integration_test.go` with proper `Fail()` / `Expect()` assertions:
- Line 85: Data Storage availability check
- Line 92: PostgreSQL connection check
- Line 97: PostgreSQL ping check

All now fail with clear error messages per TESTING_GUIDELINES.md.

---

**Contact**: Notification Team

