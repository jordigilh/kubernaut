# TRIAGE: Severity Level Extensibility [RESOLVED]

**Status**: ✅ **RESOLVED** (2026-01-09)
**Date**: 2026-01-09
**Triaged By**: Architecture Review
**Impact**: ⚠️ **BLOCKING** - Prevents customer onboarding with custom severity schemes (WAS)
**Severity**: **P0** - Multi-tenancy blocker (WAS)
**Updated**: 2026-01-09 (15:00) - **RESOLVED** via BR-GATEWAY-111 + BR-SP-105

---

## 🎯 **RESOLUTION SUMMARY**

**Decision Date**: 2026-01-09
**Resolution**: Implement **SignalProcessing Rego-based severity determination** (NOT Gateway ConfigMap mapping)

### **Approved Solution Architecture**
```
Gateway (Pass-Through)  →  SignalProcessing (Rego Determination)  →  AA/RO (Consume SP Status)
┌──────────────────┐       ┌───────────────────────────────┐       ┌────────────────────┐
│ • No hardcoding  │       │ • severity.rego policy        │       │ • Read Status.     │
│ • Preserve raw   │──────>│ • Map external → normalized   │──────>│   Severity         │
│   severity value │       │ • Write to Status field       │       │ • NOT Spec.Severity│
│ BR-GATEWAY-111   │       │ BR-SP-105                     │       └────────────────────┘
└──────────────────┘       └───────────────────────────────┘
```

### **Authoritative Documentation**
- **[BR-GATEWAY-111](../../services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md)**: Gateway Signal Pass-Through Architecture
- **[BR-SP-105](../../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md)**: Severity Determination via Rego Policy
- **[DD-SEVERITY-001](DD-SEVERITY-001-severity-determination-refactoring.md)**: Severity Determination Refactoring Plan (4-week implementation)

### **Key Design Decisions**
1. ✅ **Gateway = Dumb Pipe**: NO policy logic, just extract and preserve external severity
2. ✅ **SignalProcessing = Policy Owner**: ALL determination logic via operator-configurable Rego
3. ✅ **Fallback = "unknown"**: NOT "warning" (per stakeholder requirement)
4. ✅ **Observability**: Emit event/log when Rego fails to map severity
5. ✅ **Status Field**: `Status.SeverityClassification` (matches env/priority pattern)
6. ✅ **Priority Cleanup**: Also remove deprecated Gateway priority code (BR-GATEWAY-007)

### **Why NOT Gateway ConfigMap Mapping** (Options A/B/C Below)
This triage originally explored **Gateway-side mapping** (ConfigMap approach). However, the final architecture decision moved policy logic to **SignalProcessing Rego** for:
- **Architectural Consistency**: Environment/Priority already use Rego, severity should match
- **Separation of Concerns**: Gateway extracts, SignalProcessing determines
- **Full Context**: SP has complete signal context for policy decisions
- **Operator Control**: All policy logic in one place (Rego ConfigMaps)

---

## 📚 **Historical Analysis** (Context for Decision)

---

## 📋 **Executive Summary**

**Root Cause**: Gateway violates separation of concerns by hardcoding severity determination logic that should belong to SignalProcessing Rego policies.

**Correct Architecture** (per stakeholder clarification):
1. ✅ **Gateway**: Normalize external signal → CRD format (preserve raw severity)
2. ✅ **SignalProcessing Rego**: DETERMINE correct severity (map external → internal)
3. ✅ **AIAnalysis & RemediationOrchestrator**: CONSUME SP-determined severity

**Current Architecture** (INCOMPLETE):
1. ❌ **Gateway**: Hardcodes severity mapping (`Sev1` → `warning` default)
2. ❌ **SignalProcessing**: NO severity.rego policy exists (only env/priority/business policies)
3. ❌ **SignalProcessing Status**: NO severity field (only EnvironmentClassification, PriorityAssignment, BusinessClassification)
4. ❌ **Result**: Severity from Gateway flows through unchanged to AA/RO (Gateway's hardcoded decision wins)

**Impact**:
- 🚫 **Customer Cannot Onboard**: Custom severity schemes (Sev1-4, P0-P4) rejected at Gateway
- 🚫 **CRD Validation Blocks**: Kubernetes API rejects non-standard severity values
- 🚫 **Policy Logic Split**: Gateway makes decisions that should be in Rego
- 🚫 **Inconsistent Pattern**: Environment/Priority determined by Rego, but Severity hardcoded in Gateway

**Current State**: ❌ **ARCHITECTURE VIOLATION** (Gateway owns policy logic)
**Required State**: ✅ **SEPARATION OF CONCERNS** (SignalProcessing owns all policy via Rego)

---

## 🏗️ **Architecture Comparison: Current vs Correct**

### **❌ CURRENT ARCHITECTURE (INCORRECT - Policy Logic in Gateway)**

```
Customer Prometheus       Gateway Adapter           CRD Created           SignalProcessing
┌──────────────────┐     ┌──────────────────┐     ┌──────────────┐      ┌────────────────┐
│ labels:          │     │ determineSeverity│     │ spec:        │      │ Rego Policies  │
│   severity:      │────>│ ❌ HARDCODED:    │────>│   severity:  │─────>│ • Priority     │
│   "Sev1"         │ X   │ switch {         │  X  │   "warning"  │      │ • Environment  │
│   "P0"           │     │   case critical  │     │   (WRONG!)   │      │ ❌ NOT Severity│
│   "HIGH"         │     │   case warning   │     └──────────────┘      └────────────────┘
└──────────────────┘     │   case info      │              │                     │
                         │   default:warning│              ▼                     ▼
                         │ }                │     ┌──────────────────────────────────┐
                         └──────────────────┘     │ AIAnalysis & RemediationOrch     │
                                                  │ • Consume WRONG severity         │
                                                  │ • Customer intent LOST           │
                                                  └──────────────────────────────────┘

🚫 PROBLEMS:
1. Gateway makes policy decisions (should be in Rego)
2. Customer's "Sev1" → Gateway's "warning" (information loss)
3. Rego policies never see original severity
4. Inconsistent: environment/priority via Rego, but severity hardcoded
```

### **✅ CORRECT ARCHITECTURE (Policy Logic in SignalProcessing Rego)**

```
Customer Prometheus       Gateway Adapter           CRD Created           SignalProcessing Rego
┌──────────────────┐     ┌──────────────────┐     ┌──────────────┐      ┌────────────────────┐
│ labels:          │     │ ✅ PASS-THROUGH  │     │ spec:        │      │ severity.rego      │
│   severity:      │────>│ • No hardcode    │────>│   severity   │─────>│ ✅ DETERMINE       │
│   "Sev1"         │ ✓   │ • Preserve value │  ✓  │   External:  │      │ severity:          │
│   "P0"           │     │ • Validate format│     │   "Sev1"     │      │                    │
│   "HIGH"         │     │ • CRD compatible │     └──────────────┘      │ result := {        │
└──────────────────┘     └──────────────────┘              │            │   "severity":      │
                                                            │            │   "critical"       │
                                                            ▼            │ } if {             │
                                                   ┌────────────────┐   │   input.signal.    │
                                                   │ SignalProcessing│   │   severity_ext ==  │
                                                   │ Status:         │   │   "Sev1"           │
                                                   │   severity:     │   │ }                  │
                                                   │   "critical"    │<──┘                    │
                                                   │   (determined)  │   └────────────────────┘
                                                   └────────────────┘            │
                                                            │                    │
                                                            ▼                    ▼
                                                   ┌────────────────────────────────┐
                                                   │ AIAnalysis & RemediationOrch   │
                                                   │ • Consume SP-determined        │
                                                   │   severity: "critical"         │
                                                   │ • Customer intent PRESERVED    │
                                                   └────────────────────────────────┘

✅ BENEFITS:
1. Gateway = dumb pipe (no policy logic)
2. ALL policy logic in ONE place (SignalProcessing Rego)
3. Customer intent preserved (AA/RO see correct severity)
4. Consistent pattern: environment, priority, AND severity via Rego
5. Operator controls severity mapping via ConfigMap
```

### **Architectural Responsibilities (Correct Model)**

| Component | Current Role (WRONG) | Correct Role | Configurable? |
|-----------|---------------------|--------------|---------------|
| **Gateway Adapters** | ❌ Determine severity via hardcoded map | ✅ Pass through external severity | N/A (no logic) |
| **CRD Schema** | ❌ Enforce `enum: [critical, warning, info]` | ✅ Accept string (or dual field) | YES (CRD update) |
| **SignalProcessing Rego** | ❌ Only consume Gateway's decision | ✅ **DETERMINE** severity via policy | ✅ YES (operator ConfigMap) |
| **SP Status** | ❌ Not written | ✅ Write determined severity | N/A (code change) |
| **AIAnalysis** | ❌ Read from Spec | ✅ Read from SP Status | N/A (code change) |
| **RemediationOrchestrator** | ❌ Read from Spec | ✅ Read from SP Status | N/A (code change) |

---

## 🔍 **Current Architecture Analysis**

### **Severity Hardcoding Locations**

| Layer | Location | Hardcoded Values | Enforcement Mechanism |
|-------|----------|------------------|----------------------|
| **1. External Source** | Customer Prometheus | `critical, warning, info` | AlertManager labels |
| **2. Gateway Adapter** | `pkg/gateway/adapters/prometheus_adapter.go:234-241` | `critical, warning, info` (defaults unknown → `warning`) | Go switch statement |
| **3. CRD Validation** | `api/signalprocessing/v1alpha1/signalprocessing_types.go:86` | `critical, warning, info` | Kubebuilder enum: `+kubebuilder:validation:Enum=critical;warning;info` |
| **4. OpenAPI Schema** | `api/openapi/data-storage-v1.yaml:2307` | `critical, warning, info` | OpenAPI enum for SignalProcessing |
| **5. Database Enum** | DD-WORKFLOW-001:1158 | `critical, high, medium, low` | PostgreSQL `CREATE TYPE severity_enum` |
| **6. Workflow Search** | `pkg/datastorage/models/workflow.go` | `critical, high, medium, low` | Go struct tag validation |
| **7. LLM Prompt** | ADR-039:229-253 | `critical, high, medium, low` | Prompt engineering specification |

### **Severity Flow Diagram**

```
┌──────────────────────────────────────────────────────────────────────┐
│                    CURRENT ARCHITECTURE (RIGID)                     │
└──────────────────────────────────────────────────────────────────────┘

Customer Prometheus Alert                    Kubernaut System
┌────────────────────┐                      ┌──────────────────────┐
│ severity: "Sev1"   │──────────X──────────>│ Gateway Adapter      │
│ severity: "P0"     │      REJECTED        │ • Expects: critical/ │
│ severity: "HIGH"   │                      │   warning/info       │
└────────────────────┘                      │ • Default: warning   │
                                             └──────────────────────┘
                                                      │
                                                      ▼
                                             ┌──────────────────────┐
                                             │ CRD Validation       │
                                             │ • Kubebuilder Enum   │
                                             │ • REJECTS non-std    │
                                             └──────────────────────┘
                                                      │
                                                      ▼
                                             ┌──────────────────────┐
                                             │ Database Enum        │
                                             │ • PostgreSQL enum    │
                                             │ • Fixed values       │
                                             └──────────────────────┘
```

---

## 🚨 **Critical Issues**

### **Issue 1: CRD Validation Blocking**

**Location**: `api/signalprocessing/v1alpha1/signalprocessing_types.go:85-86`

```go
// Severity level
// +kubebuilder:validation:Enum=critical;warning;info
Severity string `json:"severity"`
```

**Problem**: Kubernetes API server **rejects** RemediationRequest CRs if severity is not exactly `critical`, `warning`, or `info`.

**Customer Impact**:
```bash
# Customer's Prometheus alert
- alert: DatabaseDown
  labels:
    severity: "Sev1"  # Customer's standard

# Result: CRD rejected by Kubernetes API server
Error from server (Invalid): error when creating "remediationrequest.yaml":
RemediationRequest.signalprocessing.kubernaut.ai "rr-abc123" is invalid:
spec.severity: Unsupported value: "Sev1": supported values: "critical", "warning", "info"
```

**Workaround**: ❌ **NONE** - Customer must modify all Prometheus alerts

---

### **Issue 2: Gateway Default Fallback**

**Location**: `pkg/gateway/adapters/prometheus_adapter.go:234-241`

```go
func determineSeverity(labels map[string]string) string {
    severity := labels["severity"]
    switch severity {
    case "critical", "warning", "info":
        return severity
    default:
        return "warning" // Default to warning for unknown severities
    }
}
```

**Problem**: All unknown severity values (e.g., "Sev1", "P0", "HIGH") silently default to `"warning"`, **losing critical severity information**.

**Customer Impact**:
- Customer's P0/Sev1 alerts → Kubernaut "warning" (incorrect severity downgrade)
- Loss of business context (severity semantics are customer-specific)

---

### **Issue 3: Database Enum Lock-In**

**Location**: DD-WORKFLOW-001:1158

```sql
CREATE TYPE severity_enum AS ENUM ('critical', 'high', 'medium', 'low');
```

**Problem**: PostgreSQL enum types are **immutable** and **require database migration** to add new values. Adding customer-specific severities would require:
1. Stop database writes
2. Run `ALTER TYPE severity_enum ADD VALUE` (locks table)
3. Restart all services
4. Repeat for every customer's custom severity

**Scalability**: ❌ **NOT FEASIBLE** for multi-tenant SaaS

---

### **Issue 4: No Normalization/Mapping Layer**

**Current**: Direct passthrough of severity from external source → CRD → database

**Missing**: Configurable severity mapping layer that allows:
```yaml
# Customer configures mapping
severity_mappings:
  # Customer's severity → Kubernaut internal severity
  "Sev1": "critical"
  "Sev2": "high"
  "Sev3": "medium"
  "Sev4": "low"
  "P0": "critical"
  "P1": "high"
  "P2": "medium"
```

---

## 📊 **Impact Analysis**

### **Affected Services**

| Service | Impact | Affected Components |
|---------|--------|---------------------|
| **Gateway** | 🔴 **HIGH** | Prometheus adapter, K8s event adapter, severity determination |
| **SignalProcessing** | 🔴 **HIGH** | CRD validation, severity field in spec |
| **DataStorage** | 🟡 **MEDIUM** | Database enum, workflow search filters |
| **AIAnalysis** | 🟡 **MEDIUM** | LLM prompt severity assessment |
| **WorkflowExecution** | 🟢 **LOW** | Workflow matching logic |
| **RemediationOrchestrator** | 🟢 **LOW** | Decision logic based on severity |

### **Customer Personas Impacted**

| Customer Type | Severity Scheme | Impact |
|---------------|----------------|---------|
| **Enterprise SRE** | "Sev1", "Sev2", "Sev3", "Sev4" | 🚫 **CANNOT ONBOARD** - Must rewrite all alerts |
| **AWS-Based** | "Critical", "High", "Medium", "Low" (capitalized) | 🚫 **BLOCKED** - Case-sensitive validation |
| **PagerDuty Users** | "P0", "P1", "P2", "P3", "P4" | 🚫 **BLOCKED** - Priority-based naming |
| **Custom Monitoring** | "CRITICAL_PROD", "WARN_STAGE", "INFO_DEV" | 🚫 **BLOCKED** - Environment-scoped severity |

---

## 🎯 **Solution Options** [HISTORICAL - FOR REFERENCE ONLY]

> **NOTE**: These options were explored during triage but **NOT implemented**. The final solution (BR-SP-105) uses SignalProcessing Rego determination instead of Gateway mapping.

### **Option A: Gateway ConfigMap Mapping** [REJECTED]

**Why Rejected**: This approach placed policy logic at the Gateway layer, violating separation of concerns. Final architecture moved policy logic to SignalProcessing Rego (BR-SP-105) for consistency with environment/priority patterns.

**Approach**: Add `severityExternal` field to CRD, create `severity.rego` policy in SignalProcessing, write determined severity to SP Status.

**Architecture**:
```
┌─────────────────────────────────────────────────────────────────────────┐
│                    CORRECT ARCHITECTURE IMPLEMENTATION                  │
└─────────────────────────────────────────────────────────────────────────┘

Step 1: Gateway (No Logic)          Step 2: CRD (Dual Fields)
┌──────────────────────┐            ┌──────────────────────────────┐
│ PrometheusAdapter    │            │ SignalProcessing Spec:       │
│ severity := labels   │───────────>│   severityExternal: "Sev1"   │
│   ["severity"]       │            │   # Customer's original value│
│ # No switch/case     │            └──────────────────────────────┘
│ # No normalization   │                          │
└──────────────────────┘                          │
                                                  ▼
Step 3: SignalProcessing Rego      ┌──────────────────────────────┐
┌──────────────────────────────┐   │ severity.rego (ConfigMap)    │
│ package signalprocessing.    │   │                              │
│   severity                    │   │ result := {                  │
│                               │   │   "severity": "critical"     │
│ result := {                   │   │ } if {                       │
│   "severity": "critical",     │   │   lower(input.signal.        │
│   "source": "mapping"         │   │     severity_external) ==    │
│ } if {                        │   │   "sev1"                     │
│   lower(input.signal.         │   │ }                            │
│     severity_external) ==     │   │                              │
│   "sev1"                      │   │ # Operator adds mappings:    │
│ }                             │   │ # "p0" → "critical"          │
│                               │   │ # "high" → "critical"        │
│ # Default fallback            │   │ # etc.                       │
│ default result := {           │   └──────────────────────────────┘
│   "severity": "warning",      │                 │
│   "source": "default"         │                 │
│ }                             │                 ▼
└──────────────────────────────┘   ┌──────────────────────────────┐
                 │                  │ SignalProcessing Status:     │
                 └─────────────────>│   severity: "critical"       │
                                    │   severitySource: "mapping"  │
                                    └──────────────────────────────┘
                                                  │
                                                  ▼
Step 4: AA/RO Consume              ┌──────────────────────────────┐
┌──────────────────────────────┐   │ AIAnalysis reads:            │
│ AIAnalysis:                   │   │   sp.Status.Severity         │
│   severity := sp.Status.      │   │   # NOT sp.Spec.Severity     │
│     Severity                  │   │                              │
│                               │   │ RemediationOrchestrator:     │
│ RemediationOrchestrator:      │   │   severity := sp.Status.     │
│   severity := sp.Status.      │   │     Severity                 │
│     Severity                  │   └──────────────────────────────┘
└──────────────────────────────┘
```

**Key Design Points**:
1. **Gateway = Pass-Through**: No severity determination logic
2. **CRD = Dual Field**: `severityExternal` (customer's value) + internal field (deprecated)
3. **Rego = Policy**: Operators configure severity mapping via ConfigMap
4. **SP Status = Source of Truth**: AA/RO read determined severity from Status
5. **Consistent Pattern**: Matches how environment/priority already work

**Implementation**:

1. **ConfigMap Schema** (`config/samples/severity-mapping.yaml`):
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: severity-mappings
  namespace: kubernaut-system
data:
  mappings.yaml: |
    # Customer-defined severity mappings
    # Format: external_severity: internal_severity
    # Internal severity must be: critical, warning, info

    # Example: Enterprise SRE scheme
    "Sev1": "critical"
    "Sev2": "warning"
    "Sev3": "info"
    "Sev4": "info"

    # Example: PagerDuty scheme
    "P0": "critical"
    "P1": "critical"
    "P2": "warning"
    "P3": "info"
    "P4": "info"

    # Case-insensitive fallback
    "CRITICAL": "critical"
    "HIGH": "critical"
    "MEDIUM": "warning"
    "LOW": "info"

    # Default: unmapped severities → warning
    default: "warning"
```

2. **Gateway Code Changes**:
```go
// pkg/gateway/severity/mapper.go (NEW)
package severity

import (
    "strings"
    "gopkg.in/yaml.v3"
)

type SeverityMapper struct {
    mappings map[string]string
    defaultSeverity string
}

func NewSeverityMapperFromConfigMap(configMapData string) (*SeverityMapper, error) {
    var config struct {
        Mappings map[string]string `yaml:"mappings"`
        Default  string            `yaml:"default"`
    }

    if err := yaml.Unmarshal([]byte(configMapData), &config); err != nil {
        return nil, err
    }

    return &SeverityMapper{
        mappings: config.Mappings,
        defaultSeverity: config.Default,
    }, nil
}

func (m *SeverityMapper) Map(externalSeverity string) string {
    // 1. Exact match (case-sensitive)
    if internal, ok := m.mappings[externalSeverity]; ok {
        return internal
    }

    // 2. Case-insensitive match
    lowerExternal := strings.ToLower(externalSeverity)
    for ext, internal := range m.mappings {
        if strings.ToLower(ext) == lowerExternal {
            return internal
        }
    }

    // 3. Default fallback
    return m.defaultSeverity
}

func (m *SeverityMapper) Validate(internalSeverity string) bool {
    validSeverities := map[string]bool{
        "critical": true,
        "warning":  true,
        "info":     true,
    }
    return validSeverities[internalSeverity]
}
```

3. **Prometheus Adapter Integration**:
```go
// pkg/gateway/adapters/prometheus_adapter.go (UPDATED)
func (a *PrometheusAdapter) determineSeverity(labels map[string]string) string {
    externalSeverity := labels["severity"]

    // Use severity mapper (injected via dependency)
    internalSeverity := a.severityMapper.Map(externalSeverity)

    // Validate mapped severity
    if !a.severityMapper.Validate(internalSeverity) {
        a.logger.Warn("Invalid mapped severity, using default",
            "external", externalSeverity,
            "mapped", internalSeverity,
            "fallback", "warning")
        return "warning"
    }

    return internalSeverity
}
```

**Pros**:
- ✅ **Zero code changes** for customers (configure once, use forever)
- ✅ **Hot-reload**: Update ConfigMap → Gateway auto-reloads
- ✅ **Multi-tenant**: Each customer gets their own namespace ConfigMap
- ✅ **Backward compatible**: Default mapping works out-of-box
- ✅ **Audit trail**: ConfigMap changes tracked in Git

**Cons**:
- ⚠️ **Initial setup**: Customers must create ConfigMap during onboarding
- ⚠️ **Documentation**: Requires clear migration guide

**Effort**: **3-5 days**
- 1 day: Implement `SeverityMapper` + tests
- 1 day: Integrate with Prometheus/K8s adapters
- 1 day: Add ConfigMap hot-reload
- 1 day: Documentation + migration guide
- 1 day: Integration tests

**Confidence**: **95%**

---

### **Why Rego Policies Alone Can't Solve This** [CLARIFIED]

> **UPDATE (2026-01-09)**: The final solution (BR-SP-105) **DOES use Rego**, but requires dual-field CRD architecture. This section explains why Rego alone (without CRD changes) doesn't work.

**Question**: "Can we just let Rego policies handle severity mapping?"

**Answer**: ❌ **No (without CRD changes)**, because:

1. **Severity is INPUT to Rego, not OUTPUT**:
   ```rego
   # Rego CONSUMES severity (can't define it)
   result := {"priority": "P0"} if {
       input.signal.severity == "critical"  # ← Must already be normalized
   }
   ```

2. **CRD Validation Happens BEFORE Rego**:
   ```go
   // +kubebuilder:validation:Enum=critical;warning;info
   Severity string `json:"severity"`
   ```
   - Kubernetes API server rejects "Sev1" before Rego ever sees it

3. **Gateway Adapter Runs BEFORE CRD Creation**:
   ```
   Prometheus → Gateway Adapter → CRD Created → Rego Evaluated
                ↑ MAPPING MUST HAPPEN HERE
   ```

**Architectural Layers (Evaluation Order)**:
| Layer | When It Runs | Can Map Severity? | Reason |
|-------|--------------|-------------------|--------|
| **Gateway Adapter** | First (ingestion) | ✅ **YES** | Before CRD validation |
| **CRD Validation** | Second (API server) | ❌ **NO** | Enforces enum, rejects invalid |
| **Rego Policies** | Third (SignalProcessing controller) | ❌ **NO** | Consumes severity as input |

**Conclusion**: Severity mapping MUST happen at Gateway adapter layer.

---

### **Option B: Remove CRD Enum, Use String Validation** [REJECTED]

**Why Rejected**: No validation, database enum still rigid, lost type safety. Final solution (BR-SP-105) uses dual-field approach (external + determined) instead.

**Approach**: Remove `+kubebuilder:validation:Enum` from CRD, accept any string, validate in webhook.

**Changes**:
```go
// api/signalprocessing/v1alpha1/signalprocessing_types.go (UPDATED)
// Severity level (validated by webhook)
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=50
Severity string `json:"severity"`
```

**Pros**:
- ✅ **Simplest implementation** (just remove enum)
- ✅ **Maximum flexibility** (any severity value accepted)

**Cons**:
- ❌ **No validation**: Typos/invalid values pass through
- ❌ **Database enum still rigid**: PostgreSQL enum must be updated
- ❌ **Workflow matching breaks**: `severity_enum` mismatch
- ❌ **Lost type safety**: No compile-time validation

**Effort**: **1-2 days**
**Confidence**: **40%** (solves CRD but not database/workflow issues)

---

### **Option C: Dual Severity Fields (External + Internal)** [PARTIALLY ADOPTED]

**Status**: The dual-field concept was adopted, but determination logic moved to SignalProcessing Rego (BR-SP-105) instead of Gateway ConfigMap.

**Approach**: Store both customer severity and normalized internal severity.

**CRD Changes**:
```go
// External severity (customer's original value)
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=50
SeverityExternal string `json:"severityExternal"`

// Internal severity (Kubernaut normalized)
// +kubebuilder:validation:Enum=critical;warning;info
Severity string `json:"severity"`
```

**Pros**:
- ✅ **Preserves customer context**: Original severity retained for audit
- ✅ **Maintains type safety**: Internal severity still validated
- ✅ **Enables analytics**: Compare external vs internal severity mappings

**Cons**:
- ❌ **Breaking change**: New CRD field
- ❌ **Storage overhead**: Duplicate severity data
- ❌ **API complexity**: Two fields for same concept

**Effort**: **5-7 days** (includes migration)
**Confidence**: **70%**

---

## 🏆 **Recommendation** [HISTORICAL - SUPERSEDED]

> **UPDATE (2026-01-09)**: The recommendation below was **NOT implemented**. The final approved solution uses **SignalProcessing Rego determination** (BR-SP-105) instead of Gateway ConfigMap mapping. See [DD-SEVERITY-001](DD-SEVERITY-001-severity-determination-refactoring.md) for the approved implementation plan.

### **~~Implement Option A: Configurable Severity Mapping~~** [REJECTED]

**Original Rationale** (Gateway ConfigMap approach):
1. **Non-breaking**: Existing customers continue using `critical/warning/info`
2. **Customer-friendly**: ConfigMap-based, no code changes required
3. **Multi-tenant ready**: Per-namespace ConfigMaps for isolation
4. **Maintainable**: Centralized mapping logic, easy to test
5. **Future-proof**: Foundation for other label normalizations (priority, environment)

**Why Rejected**: Placed policy logic at Gateway layer. Final architecture moved policy logic to SignalProcessing Rego for architectural consistency.

**Original Implementation Plan** (NOT IMPLEMENTED):

#### **~~Phase 1: Core Severity Mapper (Week 1)~~** [CANCELLED]
- [ ] ~~Create `pkg/gateway/severity/mapper.go` with mapping logic~~
- [ ] ~~Add unit tests for mapping scenarios (exact match, case-insensitive, default)~~
- [ ] ~~Add validation for internal severity values~~
- [ ] ~~Add ConfigMap watcher for hot-reload~~

#### **~~Phase 2: Gateway Integration (Week 2)~~** [CANCELLED]
- [ ] ~~Integrate `SeverityMapper` into `PrometheusAdapter`~~
- [ ] ~~Integrate `SeverityMapper` into `KubernetesEventAdapter`~~
- [ ] ~~Add telemetry (metrics for mapped severities)~~
- [ ] ~~Add structured logging (external → internal severity)~~

#### **~~Phase 3: Documentation & Testing (Week 3)~~** [CANCELLED]
- [ ] ~~Create `docs/operations/SEVERITY_MAPPING_GUIDE.md`~~
- [ ] ~~Add ConfigMap examples for common schemes (Sev1-4, P0-4, Critical/High/Medium/Low)~~
- [ ] ~~Integration tests with custom ConfigMap~~
- [ ] ~~Update Gateway deployment YAML to mount ConfigMap~~

#### **~~Phase 4: Migration & Rollout (Week 4)~~** [CANCELLED]
- [ ] ~~Add default ConfigMap (1:1 mapping for backward compatibility)~~
- [ ] ~~Helm chart updates (optional ConfigMap values)~~
- [ ] ~~Customer onboarding guide~~
- [ ] ~~Beta testing with 2-3 pilot customers~~

### **✅ APPROVED IMPLEMENTATION PLAN**
See **[DD-SEVERITY-001](DD-SEVERITY-001-severity-determination-refactoring.md)** for the 4-week implementation plan:
- **Phase 1**: CRD Schema Changes (Remove `Spec.Severity` enum, Add `Status.SeverityClassification`)
- **Phase 2**: SignalProcessing Rego Implementation (`severity.rego`, Status field population)
- **Phase 3**: Gateway Refactoring (Remove hardcoded `determineSeverity()` logic)
- **Phase 4**: Gateway Priority Cleanup (Remove deprecated BR-GATEWAY-007 code)
- **Phase 5**: Consumer Updates (AIAnalysis, RemediationOrchestrator read from SP Status)
- **Phase 6**: DataStorage Triage (Database enum if needed)

---

## 📚 **Related Documentation**

### **Authoritative References**
- **ADR-039**: LLM Prompt Response Contract (severity assessment)
- **DD-WORKFLOW-001**: Mandatory Label Schema (severity enum)
- **DD-CATEGORIZATION-001**: Gateway vs Signal Processing Split (severity mapping)
- **PROMETHEUS_ALERTRULES.md**: Alert severity levels

### **Affected Files**
- `api/signalprocessing/v1alpha1/signalprocessing_types.go` (CRD enum)
- `pkg/gateway/adapters/prometheus_adapter.go` (severity determination)
- `pkg/gateway/adapters/kubernetes_event_adapter.go` (severity mapping)
- `api/openapi/data-storage-v1.yaml` (OpenAPI enum)
- `pkg/datastorage/models/workflow.go` (workflow search validation)
- `docs/architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md` (database enum)

---

## 🚀 **Next Steps** [RESOLVED]

### **✅ COMPLETED ACTIONS (2026-01-09)**
1. ✅ **Architecture Review**: Stakeholder feedback received → SignalProcessing Rego approach approved
2. ✅ **Decision Gate**: Approved SignalProcessing Rego (BR-SP-105) + Gateway Pass-Through (BR-GATEWAY-111)
3. ✅ **Business Requirements Created**: BR-GATEWAY-111 (v1.6), BR-SP-105 (v1.3)
4. ✅ **Implementation Plan**: DD-SEVERITY-001 (4-week plan, includes Priority cleanup)

### **🔜 NEXT ACTIONS**
See **[DD-SEVERITY-001](DD-SEVERITY-001-severity-determination-refactoring.md)** for detailed implementation plan:
1. **Week 1**: CRD schema changes (dual-field: external + Status.SeverityClassification)
2. **Week 2**: SignalProcessing Rego implementation (`severity.rego` + Status field)
3. **Week 3**: Gateway refactoring (remove hardcoded severity/priority logic)
4. **Week 4**: Consumer updates (AIAnalysis, RemediationOrchestrator) + DataStorage triage

### **RESOLVED Questions**
1. ~~Should severity mappings be cluster-wide or per-namespace?~~ → **Per-namespace Rego ConfigMaps (same as environment/priority)**
2. ~~Should we extend this pattern to environment mapping?~~ → **Already solved via BR-SP-051 (environment.rego)**
3. ~~Should we provide a UI for severity mapping?~~ → **V2.0 enhancement (not blocking)**

### **Architectural Consistency Achieved**
1. ✅ **Priority mapping** → Operators configure via `priority.rego` (BR-SP-070)
2. ✅ **Environment classification** → Operators configure via `environment.rego` (BR-SP-051)
3. ✅ **Severity determination** → Operators configure via `severity.rego` (BR-SP-105) ← **NOW CONSISTENT**
4. ✅ **Business classification** → Operators configure via `business.rego` (BR-SP-080)

---

## ✅ **Success Criteria**

**MVP (V1.0)**:
- [ ] Customer can define severity mappings via ConfigMap
- [ ] Gateway correctly maps external → internal severity
- [ ] CRD validation passes with internal severity
- [ ] Backward compatibility: Default 1:1 mapping works
- [ ] Hot-reload: ConfigMap updates apply without restart

**V1.1 Enhancements**:
- [ ] Multi-tenant: Namespace-scoped ConfigMaps
- [ ] Telemetry: Metrics for severity mapping hit/miss
- [ ] Admin UI: View/edit severity mappings
- [ ] Validation: Webhook rejects invalid internal severity mappings

---

## 📊 **Document Status**

**Status**: ✅ **RESOLVED** (2026-01-09)
**Resolution**: SignalProcessing Rego-based severity determination (BR-SP-105) + Gateway pass-through (BR-GATEWAY-111)
**Implementation Plan**: [DD-SEVERITY-001](DD-SEVERITY-001-severity-determination-refactoring.md) (4-week plan)
**Decision Maker**: Product + Engineering Leadership
**Approved By**: Architecture Review + Stakeholder Feedback### **Document Purpose**
This document serves as **historical context** for the severity extensibility architecture decision. It captures:
- The original problem analysis (7 layers of hardcoding)
- Explored options (Gateway ConfigMap vs SignalProcessing Rego)
- Why SignalProcessing Rego was chosen (architectural consistency, separation of concerns)
- Links to authoritative documentation (BR-GATEWAY-111, BR-SP-105, DD-SEVERITY-001)**For current implementation details**, see:
- **[BR-GATEWAY-111](../../services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md)**: Gateway requirements
- **[BR-SP-105](../../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md)**: SignalProcessing requirements
- **[DD-SEVERITY-001](DD-SEVERITY-001-severity-determination-refactoring.md)**: Implementation plan (PENDING CREATION)
