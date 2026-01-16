# SignalProcessing Integration Actual Root Cause: DataStorage 400 Error - January 15, 2026

## Executive Summary

**Status**: 🔴 **CRITICAL - Previous Analysis Was Wrong**
**Issue**: Events being DROPPED due to DataStorage HTTP 400 errors, not timing issues
**Impact**: 1 real failure + 4 cascades (94% → 100% blocked by validation errors)

---

## 🚨 **Critical Discovery: Wrong Root Cause**

### **Previous (Incorrect) Analysis**
❌ **Theory**: Background timer flush not working, timing race between flush and controller emission
❌ **Solution**: Move flush inside `Eventually()` loop
❌ **Rationale**: "More frequent explicit flushes will catch events"

### **Actual Root Cause (Confirmed via Logs)**
✅ **Reality**: DataStorage returns HTTP 400 "invalid data", audit store DROPS entire batch
✅ **Evidence**: `{"level":"error","ts":"2026-01-15T21:04:07-05:00","logger":"audit-store","msg":"Dropping audit batch due to non-retryable error (invalid data)","batch_size":8,"is_4xx_error":true}`
✅ **Impact**: Events never reach DataStorage (dropped before persistence)

---

## 📊 **Evidence: Failing Test Timeline**

### **Test**: `should emit audit event with policy-defined fallback severity`
**Correlation ID**: `test-policy-fallback-audit-rr-1768529044618313000`

```
21:04:07.000 - Controller buffers 8 events (including classification.decision)
21:04:07.516 - Background writer timer tick fires (1-second interval working correctly)
21:04:07.516 - Attempts to flush 8-event batch to DataStorage
21:04:07.516 - ❌ DataStorage returns HTTP 400: "invalid data"
21:04:07.516 - ❌ Audit store DROPS entire batch (4xx = non-retryable)
21:04:07.628 - Test queries DataStorage: 0 events found (events were dropped)
21:04:09.631 - Test queries again: 0 events found (events never persisted)
21:04:11.662 - Test queries again: 0 events found (events never persisted)
... (continues until 60s timeout)
```

### **Key Log Excerpt**

```json
{"level":"info","ts":"2026-01-15T21:04:07-05:00","logger":"audit-store","msg":"⏰ Timer tick received","tick_number":3,"batch_size_before_flush":8,"buffer_utilization":0,"expected_interval":1,"actual_interval":1.000908875,"drift":0.000908875,"tick_time":"2026-01-15T21:04:07.516650875-05:00"}

{"level":"error","ts":"2026-01-15T21:04:07-05:00","logger":"audit-store","msg":"Failed to write audit batch","attempt":1,"batch_size":8,"error":"Data Storage Service returned status 400: HTTP 400 error from Data Storage API"}

{"level":"error","ts":"2026-01-15T21:04:07-05:00","logger":"audit-store","msg":"Dropping audit batch due to non-retryable error (invalid data)","batch_size":8,"is_4xx_error":true}

✅ Audit store flushed successfully  // ← Explicit flush (empty buffer after drop)
⏳ No events yet for signalprocessing.classification.decision (correlation_id=test-policy-fallback-audit-rr-1768529044618313000)
[21:04:07.628] classification.decision audit events found: 0 (expected: 1, correlation_id: test-policy-fallback-audit-rr-1768529044618313000)
```

**Analysis**:
- ✅ Background timer working correctly (1.000908875s interval, <1ms drift)
- ✅ Batch flushed on schedule (8 events ready at tick #3)
- ❌ DataStorage validation rejected the batch
- ❌ Entire batch dropped (4xx errors are non-retryable per ADR-032)
- ✅ Explicit flush succeeds (empty buffer after drop)
- ❌ Test queries find 0 events (events never reached DataStorage)

---

## 🔍 **Why Moving Flush Inside Eventually Won't Help**

**User's Correct Intuition**: "Once the audit has been sent to the DS service, the flush makes no difference."

**Analysis**:
1. **Background Writer IS Working**: Timer fires every 1s and attempts flush
2. **Flush Frequency Irrelevant**: Events are DROPPED due to validation errors, not timing
3. **Moving Flush Won't Fix**: Explicit flush will also get HTTP 400 → drop batch
4. **Real Problem**: DataStorage OpenAPI validator rejects the data schema

### **Why My Analysis Was Wrong**

I focused on **timing patterns** from successful tests without investigating **WHY events weren't in DataStorage**. I assumed:
- ❌ Events were buffered but not flushed
- ❌ Background flush interval (1s) was too slow vs. polling interval (2s)
- ❌ More frequent flushes would solve the problem

**Reality**:
- ✅ Events WERE flushed on schedule
- ✅ Flush frequency WAS sufficient (1s < 2s polling)
- ✅ Events were DROPPED due to validation errors

---

## 🚨 **Root Cause: OpenAPI Spec Mismatch (Again)**

### **History**
1. **DD-SEVERITY-001 v1.1**: Changed severity values from `critical/warning/info` to `critical/high/medium/low/unknown`
2. **Jan 15 (earlier)**: Updated OpenAPI spec (`api/openapi/data-storage-v1.yaml`) and regenerated Ogen client
3. **Jan 15 (earlier)**: Ran `go generate ./pkg/audit/...` to update embedded spec
4. **Result**: Embedded spec (`pkg/audit/openapi_spec_data.yaml`) has correct enums

### **Why We're Still Getting 400 Errors**

**Hypothesis 1**: Test binary not recompiled with updated embedded spec
- **Evidence**: Previous session had same issue, fixed by `go generate` + clean build
- **Status**: Running clean build now with reverted flush interval

**Hypothesis 2**: DataStorage service using outdated spec
- **Evidence**: Possible if DataStorage API also embeds the spec for validation
- **Status**: Need to verify DataStorage service's embedded spec

**Hypothesis 3**: Ogen client regeneration incomplete
- **Evidence**: Previous issues with `go mod vendor` after Ogen client regeneration
- **Status**: Low probability (Ogen client verified in previous session)

---

## ✅ **Correct Fix Actions**

### **1. Reverted Flush Interval (COMPLETED)**
```go
// test/integration/signalprocessing/suite_test.go:295
// BEFORE (WRONG FIX):
auditConfig.FlushInterval = 1 * time.Second // Reduce write contention

// AFTER (REVERTED):
auditConfig.FlushInterval = 100 * time.Millisecond // Faster flush for tests
```

**Rationale**: 1-second flush interval was a red herring. The problem is validation errors, not flush timing.

---

### **2. Verified Embedded Spec (COMPLETED)**
```bash
$ grep -B 3 -A 5 "enum:" pkg/audit/openapi_spec_data.yaml | grep -A 5 "severity"
        severity:
          type: string
          enum: [critical, high, medium, low, unknown]  # ✅ CORRECT
          description: Normalized severity level (DD-SEVERITY-001 v1.1)
          example: "critical"
```

**Status**: ✅ Embedded spec has correct enum values

---

### **3. Clean Rebuild Integration Tests (IN PROGRESS)**
```bash
go clean -testcache && make test-integration-signalprocessing
```

**Rationale**: Ensure test binary is compiled with updated embedded spec from `pkg/audit/openapi_spec_data.yaml`

---

## 📈 **Expected Outcome**

### **If Clean Build Fixes It**
- **Pass Rate**: 89/89 (100%) ✅
- **Root Cause**: Test binary was using stale embedded spec
- **Lesson**: Always clean build after `go generate` changes

### **If Clean Build Still Fails**
- **Next Step**: Investigate DataStorage service's embedded spec
- **Root Cause**: DataStorage API may have outdated embedded spec for validation
- **Fix**: Run `go generate` on DataStorage service, restart service

---

## 🎯 **Key Lessons Learned**

1. **Always Check Logs First**: Don't theorize timing issues without evidence
2. **4xx Errors = Data Problem**: HTTP 400 means validation failure, not timing
3. **Background Flush Works**: Timer logs show consistent 1-second intervals
4. **User's Intuition Was Correct**: "Once sent to DS, flush makes no difference"
5. **Clean Build After go generate**: Embedded specs require binary recompilation

---

## 📚 **Related Issues**

1. **DD-SEVERITY-001 v1.1**: Severity value migration (critical/high/medium/low/unknown)
2. **Jan 15 (earlier)**: Same OpenAPI validation issue, fixed by `go generate` + clean build
3. **Jan 15 (earlier)**: `go mod vendor` error after Ogen client regeneration

---

## 🔄 **Next Steps**

1. ✅ **COMPLETED**: Revert flush interval to 100ms
2. ✅ **COMPLETED**: Verify embedded spec has correct enum values
3. ⏳ **IN PROGRESS**: Clean build integration tests
4. ⏳ **PENDING**: Triage test results
5. ⏳ **IF NEEDED**: Investigate DataStorage service's embedded spec

---

**Analysis Completed By**: AI Assistant (corrected by user feedback)
**Date**: January 15, 2026
**Status**: Running clean build to verify hypothesis
**Confidence**: 85% (high probability clean build will fix, but DataStorage service spec is unknown)
