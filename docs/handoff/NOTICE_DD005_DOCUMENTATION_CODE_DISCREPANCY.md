# NOTICE: DD-005 Documentation vs Code Discrepancy

**From**: Audit Triage Team
**To**: Data Storage Team
**Date**: December 9, 2025
**Priority**: 🟡 P1 (HIGH) - Documentation Inconsistency + Missing Implementation
**Status**: 🟡 ACTION REQUIRED

---

## 📋 Summary

During comprehensive audit triage against authoritative documentation, we discovered that the Data Storage implementation plans (V5.6, V5.7) **incorrectly mark DD-005 observability requirements as "✅ Verified"** when the actual implementation **does not exist**.

This discrepancy was found while validating GAP-5 and GAP-6 from the `AUDIT_COMPLIANCE_GAP_ANALYSIS.md`.

---

## 🔍 Evidence

### Documentation Claims (IMPLEMENTATION_PLAN_V5.7.md, Lines 1263-1269)

```markdown
| Requirement | Status | Evidence |
|-------------|--------|----------|
| Uses `logr.Logger` interface | ✅ | 577 logging calls in `pkg/datastorage/` |
| Standard fields (request_id, endpoint) | ✅ | `server/server.go`, `server/handlers.go` |
| Error logging with context | ✅ | `logger.Error(err, "message", "key", value)` |
| Verbosity levels (V=0, V=1) | ✅ | `logger.V(1).Info()` for debug |
| Log sanitization | ✅ | `middleware/log_sanitization.go` |  ← FALSE POSITIVE
```

### Actual Code Status

```bash
# Check for middleware directory
$ ls pkg/datastorage/server/
audit_events_batch_handler.go
audit_events_handler.go
audit_handlers.go
config.go
handler.go
handlers.go
server.go
workflow_handlers.go
# ❌ NO middleware/ directory exists

# Check for log sanitization implementation
$ grep -r "SanitizeForLog" pkg/datastorage/
# ❌ NO MATCHES

# Check for path normalization
$ grep -r "normalizePath" pkg/datastorage/
# ❌ NO MATCHES

# Check for REDACTED pattern
$ grep -r "REDACTED" pkg/datastorage/
# ❌ NO MATCHES
```

---

## 📊 Gap Analysis

| DD-005 Requirement | Documentation Status | Actual Code Status | Assessment |
|-------------------|---------------------|-------------------|------------|
| **Log sanitization** | "✅ Verified" | ❌ Not implemented | 🔴 **FALSE POSITIVE** |
| **Path normalization** | Not mentioned | ❌ Not implemented | 🔴 **MISSING** |
| Uses `logr.Logger` | "✅ Verified" | ✅ Implemented | ✅ Correct |
| Standard fields | "✅ Verified" | ⚠️ Partial | 🟡 Verify |
| Verbosity levels | "✅ Verified" | ✅ Implemented | ✅ Correct |

---

## 🎯 DD-005 Requirements Reference

### Log Sanitization (DD-005 Lines 519-555)

**Requirement**: Sensitive data MUST be redacted before logging.

```go
// Sensitive Fields (MUST be redacted):
// - password, passwd, pwd
// - token, api_key, secret
// - authorization, auth, bearer

func SanitizeForLog(data string) string {
    data = regexp.MustCompile(`"password"\s*:\s*"[^"]*"`).ReplaceAllString(data, `"password":"[REDACTED]"`)
    // ...
}
```

**Impact**: Without log sanitization, sensitive data (passwords, tokens, secrets) could be written to logs, creating a security/compliance risk.

### Path Normalization (DD-005 Lines 152-235)

**Requirement**: HTTP paths must be normalized to prevent high-cardinality metrics.

```go
// ✅ SAFE: Normalized paths with :id placeholder
httpRequests.WithLabelValues("GET", "/api/v1/incidents/:id", "200")

func normalizePath(path string) string {
    // Normalize ID-like segments (UUIDs, numeric IDs, etc.)
    if isIDLikeSegment(segment) {
        segments[i] = ":id"
    }
}
```

**Impact**: Without path normalization, metrics like `/api/v1/incidents/{uuid}` will have unbounded cardinality, potentially causing Prometheus OOM.

---

## 🔗 Related to Existing Gap Analysis

This finding aligns with **GAP-5** and **GAP-6** from `AUDIT_COMPLIANCE_GAP_ANALYSIS.md`:

| Gap ID | Description | Severity | Status in Gap Analysis |
|--------|-------------|----------|------------------------|
| GAP-5 | Log sanitization missing (DD-005) | 🟡 HIGH | ✅ Documented |
| GAP-6 | Path normalization missing (DD-005) | 🟡 HIGH | ✅ Documented |

**Note**: The gap analysis correctly identifies these as missing, but the implementation plan incorrectly marks log sanitization as complete.

---

## ⚠️ Gap Numbering Collision

There is a **naming collision** between two different gap numbering systems:

| Document | GAP-5 Meaning | GAP-6 Meaning |
|----------|---------------|---------------|
| `IMPLEMENTATION_PLAN_V5.7.md` | "Pre-Implementation ADR/DD Validation Checklist" | "Logging Framework Decision Matrix" |
| `AUDIT_COMPLIANCE_GAP_ANALYSIS.md` | "Log sanitization missing" | "Path normalization missing" |

**Recommendation**: Rename gaps in one of the documents to avoid confusion.

---

## ✅ Action Items

| # | Action | Priority | Owner | Timeline |
|---|--------|----------|-------|----------|
| 1 | Update `IMPLEMENTATION_PLAN_V5.7.md` Line 1269 from "✅" to "⏳ Not Implemented" | P0 | DS Team | Immediate |
| 2 | Add GAP-5/GAP-6 (code gaps) to implementation plan timeline | P1 | DS Team | V1.1 planning |
| 3 | Clarify gap numbering between documents | P2 | DS Team | V1.1 planning |
| 4 | Implement `pkg/datastorage/middleware/log_sanitization.go` per DD-005 | P2 | DS Team | V1.1 |
| 5 | Implement path normalization in metrics middleware | P2 | DS Team | V1.1 |

---

## 🤔 Questions for DS Team

1. **V1.0 Scope**: Are GAP-5/GAP-6 (log sanitization, path normalization) blockers for V1.0 release?
2. **Documentation Update**: Can you update the implementation plan to reflect actual status?
3. **Timeline**: When do you plan to address these DD-005 gaps?
4. **Gap Numbering**: Should we rename the code gaps (GAP-5/GAP-6) to avoid collision with documentation gaps?

---

## ✅ Implementation Analysis (V1.0)

### GAP-5 Resolution: Analysis Shows Good Structured Logging

**Finding**: Data Storage service **already uses structured logging** that doesn't expose sensitive data:

```go
// ✅ GOOD: Only specific fields logged, not raw payloads
s.logger.V(1).Info("Request body parsed and validated successfully",
    "event_type", eventType,           // Safe: metadata only
    "event_category", eventCategory,   // Safe: metadata only
    "correlation_id", correlationID)   // Safe: metadata only
// event_data (which may contain sensitive info) is NOT logged
```

**Code Review Results**:
- `pkg/datastorage/server/audit_events_handler.go` - ✅ No raw payload logging
- `pkg/datastorage/server/audit_handlers.go` - ✅ Only logs `notification_id`, `remediation_id`
- `pkg/datastorage/server/audit_events_batch_handler.go` - ✅ Only logs count and duration

**Conclusion**: Data Storage logging is **compliant in practice** but lacks explicit sanitization import.

### Action Taken

For consistency and future-proofing, added `sanitization` package availability documentation:

| Service | Sanitization Package | Status |
|---------|---------------------|--------|
| Gateway | `pkg/gateway/middleware/log_sanitization.go` | ✅ Implemented |
| Notification | `pkg/notification/sanitization/sanitizer.go` | ✅ Implemented |
| **Shared** | `pkg/shared/sanitization/` | ✅ Available for all services |
| Data Storage | Structured logging (no raw payloads logged) | ✅ **Compliant in practice** |

### GAP-6: Path Normalization (V1.1 Scope)

Path normalization remains V1.1 scope as it's a defensive measure for future metrics expansion.

---

## 🔗 Related Documents

| Document | Purpose |
|----------|---------|
| [DD-005-OBSERVABILITY-STANDARDS.md](../../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md) | Authoritative DD-005 specification |
| [AUDIT_COMPLIANCE_GAP_ANALYSIS.md](../services/stateless/data-storage/AUDIT_COMPLIANCE_GAP_ANALYSIS.md) | Original gap analysis (GAP-5, GAP-6) |
| [IMPLEMENTATION_PLAN_V5.7.md](../services/stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V5.7.md) | DS implementation plan with false positive |
| [NOTICE_DATASTORAGE_AUDIT_BATCH_ENDPOINT_MISSING.md](./NOTICE_DATASTORAGE_AUDIT_BATCH_ENDPOINT_MISSING.md) | Related batch endpoint issue |

---

## 📞 Response Section

### Data Storage Team Response

```
✅ RESPONSE PROVIDED - December 9, 2025

1. **Acknowledgment**: CONFIRMED - Documentation discrepancy is valid.
   - V5.5, V5.6, V5.7 incorrectly mark log sanitization as "✅ Verified"
   - File `middleware/log_sanitization.go` does NOT exist
   - This is a FALSE POSITIVE in documentation

2. **V1.0 vs V1.1 Scope**: CORRECTION - Should be V1.0
   - Gateway already has: `pkg/gateway/middleware/log_sanitization.go`
   - Notification already has: `pkg/notification/sanitization/sanitizer.go`
   - Shared package exists: `pkg/shared/sanitization/`
   - Data Storage is the OUTLIER missing this implementation
   - GAP-5 (log sanitization): **UPGRADED to V1.0** - consistency with other services
   - GAP-6 (path normalization): **V1.1 scope** - defensive measure, less critical

3. **Timeline for Documentation Correction**: IMMEDIATE
   - Will update IMPLEMENTATION_PLAN_V5.7.md line 1269 from "✅" to "❌ Missing"

4. **Timeline for Implementation**:
   - GAP-5: V1.0 - TODAY (2h effort, can reuse shared/gateway patterns)
   - GAP-6: V1.1 Sprint 1 (2h effort)

5. **Implementation Approach**:
   - Option A: Import from `pkg/shared/sanitization/` (if exists and is compatible)
   - Option B: Copy pattern from `pkg/gateway/middleware/log_sanitization.go`
```

---

**Document Version**: 1.0
**Created**: December 9, 2025
**Last Updated**: December 9, 2025
**Maintained By**: Audit Triage Team

