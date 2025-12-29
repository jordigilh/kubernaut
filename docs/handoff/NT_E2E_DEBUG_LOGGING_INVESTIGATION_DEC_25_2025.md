# Notification E2E - Debug Logging Investigation

**Date**: December 25, 2025
**Status**: 🔄 **IN PROGRESS** - E2E tests running with comprehensive debug logging
**Purpose**: Trace exact code path to understand why retries aren't happening

---

## 🎯 **Investigation Goals**

1. **Confirm phase transitions**: Is notification staying in `Sending` or transitioning to `PartiallySent`?
2. **Verify backoff calculation**: Is `RequeueAfter` being calculated and returned correctly?
3. **Identify blocking logic**: Where does execution stop that prevents retries?
4. **Terminal phase check**: Is the `IsTerminal()` check causing early exit?

---

## 📝 **Debug Logging Added**

### **1. Reconcile Start** (Line ~178)
```go
log.Info("🔍 RECONCILE START DEBUG",
    "name", notification.Name,
    "generation", notification.Generation,
    "phase", notification.Status.Phase,
    "successfulDeliveries", notification.Status.SuccessfulDeliveries,
    "failedDeliveries", notification.Status.FailedDeliveries,
    "totalAttempts", notification.Status.TotalAttempts)
```

**What to Look For**:
- How many reconciles happen after initial failure
- What phase notification is in on each reconcile
- Are delivery counters incrementing?

---

### **2. Terminal Phase Check** (Line ~208)
```go
log.Info("🔍 TERMINAL CHECK #1 DEBUG",
    "phase", notification.Status.Phase,
    "isTerminal", notificationphase.IsTerminal(notification.Status.Phase))

if notificationphase.IsTerminal(notification.Status.Phase) {
    log.Info("❌ EXITING: NotificationRequest in terminal state")
    return ctrl.Result{}, nil
}
```

**What to Look For**:
- Is `PartiallySent` being detected as terminal?
- Is controller exiting early on second reconcile?

---

### **3. Batch Status Update** (Line ~278)
```go
log.Info("📝 BATCH STATUS UPDATE START",
    "attemptsToRecord", len(result.deliveryAttempts),
    "currentPhase", notification.Status.Phase)

// ... record attempts ...

log.Info("✅ BATCH STATUS UPDATE COMPLETE",
    "newSuccessful", notification.Status.SuccessfulDeliveries,
    "newFailed", notification.Status.FailedDeliveries)
```

**What to Look For**:
- How many attempts are being recorded in batch?
- Are counters updating correctly?
- Does this trigger a new reconcile?

---

### **4. Phase Transition Logic** (Line ~942)
```go
log.Info("🔍 PHASE TRANSITION LOGIC START",
    "currentPhase", notification.Status.Phase,
    "totalChannels", totalChannels,
    "totalSuccessful", totalSuccessful,
    "failureCount", result.failureCount)
```

**What to Look For**:
- What decision logic is being executed?
- Are we entering the partial success path?

---

### **5. Partial Success with Backoff** (Line ~1030)
```go
log.Info("⏰ PARTIAL SUCCESS WITH FAILURES → REQUEUE WITH BACKOFF",
    "backoff", backoff,
    "maxAttemptCount", maxAttemptCount,
    "currentPhase", notification.Status.Phase)

return ctrl.Result{RequeueAfter: backoff}, nil
```

**What to Look For**:
- Is backoff being calculated correctly (30s, 60s, 120s)?
- Is `RequeueAfter` being returned?
- Does a new reconcile happen after backoff period?

---

## 🔍 **Expected Log Sequence for Successful Retry**

### **Reconcile #1** (Initial Attempt)
```
🔍 RECONCILE START DEBUG: phase=Sending, successfulDeliveries=0, failedDeliveries=0
🔍 TERMINAL CHECK #1: isTerminal=false
📝 BATCH STATUS UPDATE START: attemptsToRecord=2
📝 Recorded attempt: channel=console, status=success
📝 Recorded attempt: channel=file, status=failed
✅ BATCH STATUS UPDATE COMPLETE: newSuccessful=1, newFailed=1
🔍 PHASE TRANSITION LOGIC: totalSuccessful=1, failureCount=1
⏰ PARTIAL SUCCESS → REQUEUE WITH BACKOFF: backoff=30s
```

### **Reconcile #2** (After 30s Backoff)
```
🔍 RECONCILE START DEBUG: phase=Sending, successfulDeliveries=1, failedDeliveries=1
🔍 TERMINAL CHECK #1: isTerminal=false
📝 BATCH STATUS UPDATE START: attemptsToRecord=1
📝 Recorded attempt: channel=file, status=failed (attempt 2)
✅ BATCH STATUS UPDATE COMPLETE: newSuccessful=1, newFailed=2
🔍 PHASE TRANSITION LOGIC: totalSuccessful=1, failureCount=1
⏰ PARTIAL SUCCESS → REQUEUE WITH BACKOFF: backoff=60s
```

### **Reconcile #3** (After 60s Backoff)
```
(Same pattern, backoff=120s)
```

---

## 🚨 **Failure Scenarios to Watch For**

### **Scenario A: Terminal Phase Exit**
```
🔍 RECONCILE START DEBUG: phase=PartiallySent  ← PROBLEM!
🔍 TERMINAL CHECK #1: isTerminal=true
❌ EXITING: NotificationRequest in terminal state
```
**Diagnosis**: Notification transitioned to `PartiallySent` prematurely

---

### **Scenario B: No Second Reconcile**
```
🔍 RECONCILE START DEBUG: phase=Sending (initial)
...
⏰ PARTIAL SUCCESS → REQUEUE WITH BACKOFF: backoff=30s
(No further logs after 30+ seconds)
```
**Diagnosis**: `RequeueAfter` not triggering new reconcile

---

### **Scenario C: Backoff Not Calculated**
```
🔍 PHASE TRANSITION LOGIC: totalSuccessful=1, failureCount=1
(No "⏰ PARTIAL SUCCESS" log)
```
**Diagnosis**: Code path not reaching backoff logic

---

## 📊 **Test Run Details**

**Command**:
```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
timeout 900 make test-e2e-notification 2>&1 | tee /tmp/nt-e2e-debug-logging.log
```

**Log File**: `/tmp/nt-e2e-debug-logging.log`
**Started**: ~10:40 AM EST
**Expected Duration**: ~10-15 minutes

---

## 🔧 **How to Analyze Logs After Test**

### **Extract Controller Logs**:
```bash
# Get controller pod logs
kubectl --kubeconfig ~/.kube/notification-e2e-config logs \
    -n notification-e2e \
    -l app=notification-controller \
    --tail=2000 > /tmp/controller-debug.log

# Filter for debug markers
grep -E "🔍|📝|⏰|✅|❌" /tmp/controller-debug.log > /tmp/debug-markers.log

# Check reconcile sequence
grep "RECONCILE START" /tmp/controller-debug.log

# Check backoff returns
grep "REQUEUE WITH BACKOFF" /tmp/controller-debug.log

# Check terminal exits
grep "EXITING.*terminal" /tmp/controller-debug.log
```

### **Analyze Test Output**:
```bash
# Check test results
grep -E "Ran [0-9]+ of [0-9]+ Specs" /tmp/nt-e2e-debug-logging.log

# Check specific failing test
grep -A 50 "Scenario 1: Failed delivery triggers retry" /tmp/nt-e2e-debug-logging.log
```

---

## 🎯 **Success Criteria**

Tests will pass if we see:
1. ✅ Multiple reconciles happening (Reconcile #1, #2, #3, #4, #5)
2. ✅ Backoff being calculated (30s, 60s, 120s, 240s, 480s)
3. ✅ At least 2 File channel attempts within 3 minutes
4. ✅ Notification staying in `Sending` phase until retries exhausted

---

## 💡 **Next Steps Based on Findings**

### **If Scenario A (Terminal Phase Exit)**:
- Fix: Ensure notification stays in `Sending` during retry loop
- Check where `PartiallySent` is being set
- Review `determinePhaseTransition` logic

### **If Scenario B (No Second Reconcile)**:
- Fix: Investigate controller-runtime requeue behavior
- Check if status update overrides `RequeueAfter`
- Consider moving status update to AFTER return

### **If Scenario C (Backoff Not Calculated)**:
- Fix: Debug why code isn't reaching partial success path
- Check `result.failureCount` value
- Verify `totalSuccessful` calculation

---

**Document Owner**: AI Assistant
**Status**: Waiting for test results (~10 minutes)
**Monitor**: `tail -f /tmp/nt-e2e-debug-logging.log`



