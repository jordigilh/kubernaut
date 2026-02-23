# Adapter Naming Consistency - SignalSource vs SignalType

**Date**: November 21, 2025
**Status**: ✅ **CONSISTENT IMPLEMENTATION**
**Confidence**: 100%

> **📋 Design Decision**: [DD-GATEWAY-010](decisions/DD-GATEWAY-010-adapter-naming-convention.md) | ✅ **Approved Design** | Confidence: 100%
>
> This document provides a quick reference. For complete decision details, alternatives considered, and implementation guidance, see [DD-GATEWAY-010](decisions/DD-GATEWAY-010-adapter-naming-convention.md).

---

## Revision: Issue #166 (2026-02)

**RR.Spec.SignalType values** are now normalized to `"alert"` (generic). The source-specific values `"prometheus-alert"` and `"kubernetes-event"` below were superseded. **Adapter identity** for audit/metrics uses `signal.Source` (e.g., `"prometheus"`, `"kubernetes-events"`).

---

## 🎯 **TRIAGE SUMMARY**

### **Current Implementation**

| Adapter | SignalSource (Monitoring System) | SignalType (RR.Spec) | Consistent? |
|---------|----------------------------------|----------------------|-------------|
| **Prometheus** | `"prometheus"` | `"alert"` | ✅ YES |
| **Kubernetes Event** | `"kubernetes-events"` | `"alert"` | ✅ YES |

### **Verdict**: ✅ **IMPLEMENTATION IS CORRECT AND CONSISTENT**

---

## 📋 **RATIONALE: Singular vs Plural**

### **SignalSource (Monitoring System) - Use System Name As-Is**

**Purpose**: Identifies the **monitoring system** that generated the signal
**Used By**: LLM for selecting investigation tools
**Naming Convention**: **Use the actual system name**

| Adapter | SignalSource | Rationale |
|---------|--------------|-----------|
| Prometheus | `"prometheus"` | ✅ System name (Prometheus) |
| Kubernetes Event | `"kubernetes-events"` | ✅ System name (Kubernetes Events API - plural) |

**Why "kubernetes-events" (plural)?**
- The monitoring system is called "Kubernetes **Events**" (plural)
- The K8s API resource is `events.v1.core` (plural)
- `kubectl get events` (plural command)
- Matches K8s API naming convention

**LLM Usage**:
```yaml
signal_source: "kubernetes-events"
→ LLM knows to use: kubectl get events, kubectl describe event
```

---

### **SignalType (Event Type) - Generic (Issue #166)**

**Purpose**: Identifies the **type of signal** received (generic classification)
**Used By**: RR.Spec, metrics, logging, signal classification
**Naming Convention**: **Generic** — `"alert"` for all adapters (Issue #166)

| Adapter | SignalType | Rationale |
|---------|------------|-----------|
| Prometheus | `"alert"` | ✅ Generic (source identity via SignalSource) |
| Kubernetes Event | `"alert"` | ✅ Generic (source identity via SignalSource) |

**Why Generic?**
- RR.Spec.SignalType is normalized to `"alert"` for all adapters
- Adapter identity for audit/metrics uses `signal.Source` (e.g., `"prometheus"`, `"kubernetes-events"`)

---

## ✅ **TEST FIX VALIDATION**

### **Original Test Expectation** (INCORRECT)

```go
// ❌ WRONG: Expected singular for SignalSource
Expect(crd.Spec.SignalSource).To(Equal("kubernetes-event"))
```

### **Corrected Test Expectation** (CORRECT)

```go
// ✅ CORRECT: Expect plural for SignalSource (monitoring system)
Expect(crd.Spec.SignalSource).To(Equal("kubernetes-events"))
```

**Verdict**: ✅ **Test was wrong, production code was correct**

---

**Status**: ✅ **RESOLVED**
**Action**: Test expectation corrected

