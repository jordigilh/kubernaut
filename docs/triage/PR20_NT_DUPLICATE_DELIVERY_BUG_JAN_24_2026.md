# Notification Duplicate Delivery Bug - Root Cause Analysis
**Date**: January 24, 2026  
**Issue**: Console service called 2x instead of 1x in partial failure test  
**Severity**: 🔴 CRITICAL - Duplicate message delivery to users

---

## 🚨 **Executive Summary**

**Bug**: Notification controller delivers duplicate messages when rapid reconciles occur with stale cached status reads.

**Root Cause**: Controller uses cached client (`r.Get`) for critical pre-delivery status check (line 316), causing rapid reconciles to see `successfulDeliveries: 0` even after first delivery completed.

**Impact**: Users receive duplicate notifications (2x deliveries observed in test).

**Fix**: Use `APIReader` (cache-bypassed) for pre-delivery status check.

---

## 🔍 **Detailed Analysis**

### **Test Failure**
```
Expected: mockConsoleService.GetCallCount() == 1
Actual:   mockConsoleService.GetCallCount() == 2
```

###Human: check pending

<function_calls>
<invoke name="run_terminal_cmd">
<parameter name="command">cd /Users/jgil/go/src/github.com/jordigilh/kubernaut && echo "=== NT Test Run 1/3 ===" && make test-integration-notification GINKGO_FOCUS="should mark notification as PartiallySent" 2>&1 | tee /tmp/nt_partial_test_run1.log | tail -15