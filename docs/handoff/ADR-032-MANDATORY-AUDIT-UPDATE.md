# ADR-032 Update: Mandatory Audit Requirements Now Authoritative

**Date**: December 17, 2025
**Updated Document**: `docs/architecture/decisions/ADR-032-data-access-layer-isolation.md`
**Version**: 1.2 → 1.3
**Status**: ✅ **AUTHORITATIVE REFERENCE** - All services MUST comply

---

## 🎯 **What Changed**

### **Problem**
- **ADR-032** contained mandatory audit requirements but they were buried in line 92-112
- Services violating audit mandate had no clear authoritative reference to cite
- Triage documents (e.g., `TRIAGE_AUDIT_FALLBACK_LOGIC_ALL_SERVICES.md`) identified violations but couldn't cite specific ADR sections

### **Solution**
- **Added prominent mandatory audit section** at document start (lines 11-158)
- **Structured as §1-4** for easy citation in code and documentation
- **Added explicit "No Fallback/Recovery" prohibition** per user request
- **Created service classification table** showing which services MUST crash vs MAY continue

---

## 📋 **New Authoritative Sections**

### **ADR-032 §1: Audit Mandate**

**Services MUST create audit entries for**:
1. ✅ Every remediation action (WorkflowExecution)
2. ✅ Every AI/ML decision (AIAnalysis)
3. ✅ Every workflow execution (WorkflowExecution)
4. ✅ Every effectiveness assessment (EffectivenessMonitor)
5. ✅ Every alert/signal processed (SignalProcessing, Gateway)
6. ✅ Every notification delivered (Notification)
7. ✅ Every orchestration phase transition (RemediationOrchestrator)

### **ADR-032 §2: Audit Completeness Requirements**

**1. No Audit Loss** (MANDATORY):
- ❌ Services MUST NOT implement "graceful degradation" that silently skips audit
- ❌ Services MUST NOT implement fallback/recovery mechanisms when audit client is nil
- ❌ Services MUST NOT continue execution if audit client is not initialized
- ✅ Services MUST fail immediately (return error, fail request, terminate operation) if audit store is nil
- ✅ Services MUST crash at startup if audit store cannot be initialized (for P0 services)

**2. No Recovery Allowed** (NEW - User Requested):
- ❌ Services MUST NOT catch audit initialization errors and continue
- ❌ Services MUST NOT implement retry loops to "wait" for audit to become available
- ❌ Services MUST NOT queue requests while audit is unavailable
- ✅ Services MUST fail fast and exit(1) if audit cannot be initialized
- ✅ Kubernetes will restart the pod (correct behavior - pod is misconfigured)

**Rationale**: Audit unavailability is a **deployment/configuration error**, not a transient failure. The correct response is to crash and let Kubernetes orchestration detect the misconfiguration.

### **ADR-032 §3: Service Classification**

| Service | Audit Mandatory? | Crash on Init Failure? | Graceful Degradation? | Reference |
|---------|------------------|------------------------|----------------------|-----------|
| **SignalProcessing** | ✅ MANDATORY | ✅ YES (P0) | ❌ NO | cmd/signalprocessing/main.go:161 |
| **RemediationOrchestrator** | ✅ MANDATORY | ✅ YES (P0) | ❌ NO | cmd/remediationorchestrator/main.go:126 |
| **WorkflowExecution** | ✅ MANDATORY | ✅ YES (P0) | ❌ NO | cmd/workflowexecution/main.go:170 |
| **Notification** | ✅ MANDATORY | ✅ YES (P0) | ❌ NO | cmd/notification/main.go:163 |
| **AIAnalysis** | ⚠️ OPTIONAL | ❌ NO (P1) | ✅ YES (by design) | cmd/aianalysis/main.go:155 |
| **DataStorage** | ✅ MANDATORY | ✅ YES (P0) | ❌ NO | pkg/datastorage/server/server.go:186 |
| **Gateway** | 🟡 PLANNED | 🟡 PENDING | 🟡 PENDING | DD-AUDIT-003 |

**P0 Services** (Business-Critical): **MUST crash** if audit cannot be initialized
**P1 Services** (Operational Visibility): **MAY** continue without audit (log warning)

### **ADR-032 §4: Enforcement**

**✅ CORRECT** (Mandatory Pattern):
```go
// Audit is MANDATORY per ADR-032 - controller will crash if not configured
auditStore, err := audit.NewBufferedStore(...)
if err != nil {
    setupLog.Error(err, "FATAL: failed to create audit store - audit is MANDATORY per ADR-032")
    os.Exit(1)  // Crash on init failure - NO RECOVERY
}

// Runtime nil check - returns error if nil (prevents silent audit loss)
func (r *Reconciler) recordAudit(ctx context.Context, event AuditEvent) error {
    if r.AuditStore == nil {
        err := fmt.Errorf("AuditStore is nil - audit is MANDATORY per ADR-032 §1")
        logger.Error(err, "CRITICAL: Cannot record audit event")
        return err  // Return error - NO FALLBACK
    }
    return r.AuditStore.StoreAudit(ctx, event)
}
```

**❌ WRONG** (Violates ADR-032):
```go
// ❌ VIOLATION #1: Graceful degradation silently skips audit
if r.AuditStore == nil {
    logger.V(1).Info("AuditStore not configured, skipping audit")
    return nil  // Violates ADR-032 §1 "No Audit Loss"
}

// ❌ VIOLATION #2: Fallback/recovery mechanism
if r.AuditStore == nil {
    logger.Warn("Audit not available, queueing for later")
    r.pendingAudits = append(r.pendingAudits, event)
    return nil  // Violates ADR-032 §2 "No Recovery Allowed"
}

// ❌ VIOLATION #3: Retry loop waiting for audit
if r.AuditStore == nil {
    for i := 0; i < 10; i++ {
        time.Sleep(1 * time.Second)
        if r.AuditStore != nil {
            break  // Violates ADR-032 §2 "No Recovery Allowed"
        }
    }
}
```

**Why These Are Wrong**:
1. **Violation #1**: Creates compliance gap - operations succeed without audit trail
2. **Violation #2**: Queuing implies audit is optional, violates mandatory requirement
3. **Violation #3**: Masks configuration error, delays failure detection

**Correct Behavior**: Service MUST crash at startup if audit cannot be initialized. **NO fallback, NO recovery, NO graceful degradation.**

---

## 📊 **Impact on Existing Services**

### **Services with ADR-032 Violations**

| Service | Violation Type | Location | Fix Required |
|---------|---------------|----------|--------------|
| **WorkflowExecution** | ❌ Graceful degradation (§1) | `workflowexecution_controller.go:1287` | Change `return nil` to `return err` |
| **RemediationOrchestrator** | ⚠️ Silent skip (§1) | `reconciler.go:1132` | Add error return if nil |
| **Gateway** | 🟡 No audit integration (§3) | `server.go:297` | Implement audit client |

### **Services Already Compliant**

| Service | Compliance Status | Evidence |
|---------|------------------|----------|
| **SignalProcessing** | ✅ COMPLIANT | Crashes on init failure + defensive nil checks |
| **Notification** | ✅ COMPLIANT | Crashes on init failure + no nil checks (fail-fast) |
| **AIAnalysis** | ✅ COMPLIANT | Optional by design (P1 service per §3) |
| **DataStorage** | ✅ COMPLIANT | Crashes on init failure + graceful nil checks |

---

## 🔧 **How to Use This ADR**

### **For Code Reviews**

**Cite ADR-032 sections when rejecting code**:
```
❌ REJECT: This code violates ADR-032 §1 "No Audit Loss"
The `return nil` on line 42 silently skips audit when AuditStore is nil.

Required fix per ADR-032 §4:
if r.AuditStore == nil {
    err := fmt.Errorf("AuditStore is nil - audit is MANDATORY per ADR-032 §1")
    logger.Error(err, "CRITICAL: Cannot record audit event")
    return err
}
```

### **For Implementation**

**Reference ADR-032 in code comments**:
```go
// Audit is MANDATORY per ADR-032 §1: Audit writes are MANDATORY, not best-effort
// Per ADR-032 §2: No fallback/recovery allowed - fail fast at startup
auditStore, err := audit.NewBufferedStore(...)
if err != nil {
    setupLog.Error(err, "FATAL: ADR-032 §2 violation - audit initialization failed")
    os.Exit(1)
}
```

### **For Documentation**

**Cite ADR-032 in design docs**:
```markdown
### Audit Implementation

**Authoritative Reference**: ADR-032 §1-4

Per ADR-032 §3, this service is classified as **P0 (Business-Critical)** and MUST:
- ✅ Crash on audit init failure (ADR-032 §2)
- ✅ Return error if audit store is nil (ADR-032 §4)
- ❌ NO graceful degradation allowed (ADR-032 §1)
- ❌ NO fallback/recovery mechanisms (ADR-032 §2)
```

---

## 📚 **Related Documents**

### **Updated by This Change**

1. **ADR-032**: Now has authoritative §1-4 sections at document start
2. **TRIAGE_AUDIT_FALLBACK_LOGIC_ALL_SERVICES.md**: Can now cite ADR-032 §1-4 for violations

### **Complementary ADRs**

| ADR | Topic | Relationship |
|-----|-------|-------------|
| **ADR-034** | Unified Audit Table Design | Defines audit schema (what to store) |
| **ADR-038** | Async Buffered Audit Ingestion | Defines audit write pattern (how to store) |
| **ADR-032** | Mandatory Audit Requirements | Defines audit mandate (MUST store) ⭐ THIS ADR |

### **Design Decisions**

| DD | Topic | Relationship |
|----|-------|-------------|
| **DD-AUDIT-001** | Audit Responsibility Pattern | Service-specific audit requirements |
| **DD-AUDIT-002** | Audit Shared Library Design | Implementation of ADR-038 pattern |
| **DD-AUDIT-003** | Service Audit Trace Requirements | Which services need audit (references ADR-032 §3) |

---

## ✅ **Verification Checklist**

Before claiming ADR-032 compliance, verify:

- [ ] **Startup Behavior**: Service crashes with `os.Exit(1)` if audit init fails (P0 services)
- [ ] **Runtime Behavior**: Functions return error if AuditStore is nil (no silent skip)
- [ ] **No Fallback**: Zero fallback/recovery mechanisms when audit unavailable
- [ ] **No Queuing**: Zero pending audit queues or retry loops
- [ ] **Error Logging**: ERROR level logs when audit is unavailable
- [ ] **Code Comments**: ADR-032 §X cited in audit initialization code
- [ ] **Metrics**: Prometheus metrics for audit write success/failure
- [ ] **Alerts**: P1 alert configured for >1% audit write failure rate

---

## 🎯 **Key Takeaways**

### **For Service Owners**

1. ✅ **ADR-032 is now THE authoritative reference** for audit requirements
2. ✅ **Cite ADR-032 §1-4** in code comments and documentation
3. ❌ **No fallback/recovery allowed** - crash at startup if audit unavailable
4. ❌ **No graceful degradation** - return error if audit store is nil

### **For Platform Team**

1. ✅ **Use ADR-032 §X** when citing violations in code reviews
2. ✅ **Service classification in §3** defines P0 (MUST crash) vs P1 (MAY continue)
3. ✅ **Enforcement patterns in §4** provide correct/wrong code examples
4. ✅ **Related documents updated** to reference ADR-032 §1-4

### **For Compliance/Audit Team**

1. ✅ **ADR-032 §1** defines mandatory audit requirements
2. ✅ **ADR-032 §2** prohibits fallback/recovery (ensures completeness)
3. ✅ **ADR-032 §3** classifies services (P0 vs P1)
4. ✅ **Zero tolerance for audit loss** - services MUST fail if misconfigured

---

**Prepared by**: Jordi Gil
**Authority**: ADR-032 v1.3 (December 17, 2025)
**Status**: ✅ Authoritative - All services MUST comply
**Enforcement**: Immediate (violations MUST be fixed)

