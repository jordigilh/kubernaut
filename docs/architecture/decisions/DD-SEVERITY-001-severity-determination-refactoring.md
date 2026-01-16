# DD-SEVERITY-001: Severity Determination Refactoring

## Status
**✅ APPROVED** (2026-01-09)
**Last Reviewed**: 2026-01-15 (v1.1 - Severity value alignment)
**Confidence**: 95%
**Priority**: P0 (Blocks customer onboarding with custom severity schemes)
**Version**: 1.1

---

## 📝 **Changelog**

### **v1.1 (2026-01-15) - Severity Value Alignment**
**Change**: Updated normalized severity values from `critical/warning/info/unknown` to `critical/high/medium/low/unknown`

**Rationale**:
- ✅ **Infrastructure Alignment**: HAPI LLM prompts and workflow catalog already use `critical/high/medium/low`
- ✅ **Zero Infrastructure Changes**: No HAPI prompt updates or workflow catalog migrations needed
- ✅ **Better Semantic Range**: 5-level granularity (critical > high > medium > low > unknown) provides clearer severity distinctions
- ✅ **Bounded Cardinality**: Still maintains acceptable Prometheus cardinality (5 values)
- ✅ **Operator Flexibility**: Rego policies can still map ANY external scheme (Sev1-4, P0-P4) to these 5 normalized values

**Impact**:
- CRD enum updates required (AIAnalysis, SignalProcessing Status)
- Rego policy examples updated to output `high/medium/low` instead of `warning/info`
- SignalProcessing controller validation updated
- No HAPI code changes required (already using these values)

### **v1.0 (2026-01-09) - Initial Approval**
- Approved SignalProcessing Rego-based severity determination
- Defined 4-week implementation plan
- Established Gateway pass-through architecture

---

## 📋 **Executive Summary**

**Problem**: Gateway hardcodes severity mappings, preventing customers with custom severity schemes (Sev1-4, P0-P4, etc.) from onboarding.

**Root Cause**: Three CRD enum validations block non-standard severity values:
1. `RemediationRequest.Spec.Severity` → `+kubebuilder:validation:Enum=critical;warning;info`
2. `SignalProcessing.Spec.Signal.Severity` → `+kubebuilder:validation:Enum=critical;warning;info`
3. `AIAnalysis.SignalContextInput.Severity` → `+kubebuilder:validation:Enum=critical;warning;info`

**Approved Solution**: SignalProcessing Rego-based severity determination (moves policy logic from Gateway to SignalProcessing)

**Implementation**: 4-week refactoring plan + 1-week buffer (5 weeks total)

---

## Context & Problem

### **Current Architecture Violation**

```
Customer Prometheus       Gateway Adapter           CRD Validation           SignalProcessing
┌──────────────────┐     ┌──────────────────┐     ┌──────────────┐        ┌────────────────┐
│ labels:          │     │ determineSeverity│     │ RR.Spec:     │        │ NO REGO FOR    │
│   severity:      │────>│ ❌ HARDCODED:    │────>│   severity   │───X───>│ SEVERITY       │
│   "Sev1"         │ X   │ switch {         │  X  │   ENUM:      │ REJECTED │              │
│   "P0"           │     │   case critical  │     │   critical,  │        │ Uses Gateway's │
│   "HIGH"         │     │   case warning   │     │   warning,   │        │ hardcoded      │
└──────────────────┘     │   case info      │     │   info       │        │ decision       │
                         │   default:warning│     └──────────────┘        └────────────────┘
                         │ }                │              │
                         └──────────────────┘              ▼
                                                   ❌ KUBERNETES API REJECTS "Sev1"
```

### **Key Requirements**

1. **Customer Extensibility**: Accept ANY severity scheme (Sev1-4, P0-P4, Critical/High/Medium/Low, etc.)
2. **Separation of Concerns**: Gateway extracts, SignalProcessing determines
3. **Architectural Consistency**: Severity follows same Rego pattern as environment/priority
4. **Operator Control**: All policy logic configurable via Rego ConfigMaps
5. **Backward Compatibility**: Existing deployments continue working with default 1:1 mapping

### **Business Requirements**

- **[BR-GATEWAY-111](../../services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md)**: Gateway Signal Pass-Through Architecture (P0)
- **[BR-SP-105](../../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md)**: Severity Determination via Rego Policy (P0)

### **Historical Context**

- **[TRIAGE-SEVERITY-EXTENSIBILITY.md](TRIAGE-SEVERITY-EXTENSIBILITY.md)**: Problem analysis, 7 layers of hardcoding identified
- **Original Approach Explored**: Gateway ConfigMap mapping (REJECTED - policy logic at wrong layer)
- **Final Approach**: SignalProcessing Rego determination (APPROVED - consistent with environment/priority)

---

## Alternatives Considered

### **Alternative 1: Gateway ConfigMap Mapping** [REJECTED]

**Approach**: Add `pkg/gateway/severity/mapper.go` with ConfigMap-based severity mapping at Gateway layer.

**Pros**:
- ✅ Non-breaking: Existing customers continue using `critical/warning/info`
- ✅ Customer-friendly: ConfigMap-based, no code changes required
- ✅ Hot-reload: Update ConfigMap → Gateway auto-reloads

**Cons**:
- ❌ **Violates separation of concerns**: Policy logic at Gateway layer (should be in SignalProcessing)
- ❌ **Inconsistent architecture**: Environment/Priority use Rego, but Severity uses ConfigMap
- ❌ **Split context**: Gateway lacks full signal context for policy decisions
- ❌ **Harder to maintain**: Two policy mechanisms (Rego + ConfigMap)

**Confidence**: 40% (solves immediate problem but creates architectural debt)

---

### **Alternative 2: Remove CRD Enum, Use String Validation** [REJECTED]

**Approach**: Remove `+kubebuilder:validation:Enum` from all CRDs, accept any string, validate in webhook.

**Pros**:
- ✅ Simplest implementation (just remove enum)
- ✅ Maximum flexibility (any severity value accepted)

**Cons**:
- ❌ **No validation**: Typos/invalid values pass through
- ❌ **No normalization**: Downstream services see inconsistent values
- ❌ **Lost type safety**: No compile-time validation
- ❌ **No policy control**: Operators cannot define mappings

**Confidence**: 30% (too permissive, no customer value)

---

### **Alternative 3: SignalProcessing Rego Determination** [APPROVED]

**Approach**: Gateway passes through raw severity → SignalProcessing Rego maps external → normalized → Write to Status field.

**Pros**:
- ✅ **Architectural consistency**: Matches environment/priority Rego pattern
- ✅ **Separation of concerns**: Gateway = dumb pipe, SignalProcessing = policy owner
- ✅ **Full context**: SP Rego has complete signal context for policy decisions
- ✅ **Operator control**: All policy logic in ONE place (Rego ConfigMaps)
- ✅ **Customer extensibility**: Operators define any severity mapping
- ✅ **Backward compatible**: Default 1:1 Rego policy shipped with deployment

**Cons**:
- ⚠️ **CRD changes required**: Remove enums from RR/SP, add Status field to SP - **Mitigation**: Pre-release product, no migration needed
- ⚠️ **Consumer updates required**: AA/RO read from new Status field - **Mitigation**: Clear 4-week plan with phased rollout

**Confidence**: **95%** (best architectural fit, enables customer requirements)

---

## Decision

**APPROVED: Alternative 3 - SignalProcessing Rego Determination**

### **Rationale**

1. **Architectural Consistency**: All policy logic (environment, priority, severity, business) in SignalProcessing Rego
2. **Separation of Concerns**: Gateway extracts data, SignalProcessing interprets data
3. **Full Context**: SignalProcessing has complete signal context for policy decisions
4. **Customer Extensibility**: Operators configure ANY severity scheme via Rego
5. **Maintainability**: One policy mechanism (Rego), not two (Rego + ConfigMap)

### **Key Insight**

The critical insight from [TRIAGE-SEVERITY-EXTENSIBILITY.md](TRIAGE-SEVERITY-EXTENSIBILITY.md): **"Gateway adapter runs BEFORE CRD creation"**, so severity mapping MUST happen either:
1. At Gateway (violates separation of concerns) ❌
2. At SignalProcessing via Rego (correct architectural layer) ✅

### **Approved Design Decisions**

#### **Q1: Notification/WorkflowExecution Message Severity**
**Decision**: **Option A** - Use external severity (`rr.Spec.Severity` = "Sev1")

**Rationale**: Operators configured "Sev1", they should see "Sev1" in messages for familiarity and understanding.

#### **Q2: Audit Event Severity Fields**
**Decision**: **Option C** - Include both external + normalized severity

**Rationale**: Complete traceability for debugging Rego mappings and customer support.

---

## Implementation

### **4-Week Implementation Plan + 1-Week Buffer**

#### **Week 1: CRD Schema Changes**

**Files to Modify**:
1. `api/remediation/v1alpha1/remediationrequest_types.go`
2. `api/signalprocessing/v1alpha1/signalprocessing_types.go`
3. `api/aianalysis/v1alpha1/aianalysis_types.go`

**Changes**:

**1. RemediationRequest (Remove Enum)**:
```go
// api/remediation/v1alpha1/remediationrequest_types.go
type RemediationRequestSpec struct {
    // Signal Classification
    // Severity level (external value from signal provider)
    // Examples: "Sev1", "P0", "critical", "HIGH", "warning"
    // SignalProcessing will normalize via Rego policy
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=50
    Severity string `json:"severity"` // ← REMOVE: +kubebuilder:validation:Enum=critical;warning;info

    // ... other fields
}
```

**2. SignalProcessing (Remove Spec Enum, Add Status Field)**:
```go
// api/signalprocessing/v1alpha1/signalprocessing_types.go
type SignalData struct {
    // Severity level (external value copied from RemediationRequest)
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=50
    Severity string `json:"severity"` // ← REMOVE: +kubebuilder:validation:Enum=critical;warning;info

    // ... other fields
}

type SignalProcessingStatus struct {
    // ... existing fields (Phase, Conditions, etc.)

    EnvironmentClassification *EnvironmentClassification `json:"environmentClassification,omitempty"`
    PriorityAssignment        *PriorityAssignment        `json:"priorityAssignment,omitempty"`
    BusinessClassification    *BusinessClassification    `json:"businessClassification,omitempty"`

    // Normalized severity determined by Rego policy
    // Valid values: critical, high, medium, low, unknown (aligned with HAPI/workflow catalog)
    // Consumers (AIAnalysis, RemediationOrchestrator) MUST read this field
    // +optional
    Severity string `json:"severity,omitempty"` // ← ADD THIS
}
```

**3. AIAnalysis (Update Enum to Match HAPI/Workflow Catalog)**:
```go
// api/aianalysis/v1alpha1/aianalysis_types.go
type SignalContextInput struct {
    // Signal severity (normalized by SignalProcessing Rego)
    // Valid values: critical, high, medium, low, unknown (aligned with HAPI/workflow catalog per v1.1)
    // +kubebuilder:validation:Enum=critical;high;medium;low;unknown
    Severity string `json:"severity"`

    // ... other fields
}
```

**Validation**:
- [ ] Run `make generate` to regenerate CRDs
- [ ] Run `make manifests` to update YAML manifests
- [ ] Verify Kubernetes API accepts "Sev1" in RemediationRequest
- [ ] Unit tests for CRD validation

**Deliverables**:
- Updated CRD manifests in `deploy/`
- Updated Go types in `api/*/v1alpha1/`
- CRD validation unit tests

---

#### **Week 2: SignalProcessing Rego Implementation**

**Files to Create**:
1. `deploy/signalprocessing/policies/severity.rego` (NEW)
2. `pkg/signalprocessing/classifier/severity.go` (NEW)

**Files to Modify**:
3. `internal/controller/signalprocessing/signalprocessing_controller.go`
4. `pkg/signalprocessing/audit/client.go`

**Changes**:

**1. Default Rego Policy** (`deploy/signalprocessing/policies/severity.rego`):
```rego
package signalprocessing.severity

import rego.v1

# 1:1 mapping for standard severity values (backward compatibility)
result := {"severity": "critical", "source": "rego-policy"} if {
    lower(input.signal.severity) == "critical"
}

result := {"severity": "high", "source": "rego-policy"} if {
    lower(input.signal.severity) == "high"
}

result := {"severity": "medium", "source": "rego-policy"} if {
    lower(input.signal.severity) == "medium"
}

result := {"severity": "low", "source": "rego-policy"} if {
    lower(input.signal.severity) == "low"
}

# Fallback: unmapped severity → unknown
default result := {"severity": "unknown", "source": "fallback"}
```

**Operator Customization Example**:
```rego
package signalprocessing.severity

import rego.v1

# Enterprise "Sev" scheme
result := {"severity": "critical", "source": "rego-policy"} if {
    input.signal.severity in ["Sev1", "SEV1", "sev1"]
}

result := {"severity": "high", "source": "rego-policy"} if {
    input.signal.severity in ["Sev2", "SEV2", "sev2"]
}

result := {"severity": "medium", "source": "rego-policy"} if {
    input.signal.severity in ["Sev3", "SEV3", "sev3"]
}

result := {"severity": "low", "source": "rego-policy"} if {
    input.signal.severity in ["Sev4", "SEV4", "sev4"]
}

# PagerDuty "P" scheme
result := {"severity": "critical", "source": "rego-policy"} if {
    input.signal.severity in ["P0", "P1"]
}

result := {"severity": "high", "source": "rego-policy"} if {
    input.signal.severity in ["P2"]
}

result := {"severity": "medium", "source": "rego-policy"} if {
    input.signal.severity in ["P3"]
}

result := {"severity": "low", "source": "rego-policy"} if {
    input.signal.severity in ["P4"]
}

default result := {"severity": "unknown", "source": "fallback"}
```

**2. Severity Classifier** (`pkg/signalprocessing/classifier/severity.go`):
```go
package classifier

import (
    "context"
    "fmt"

    "github.com/open-policy-agent/opa/rego"
    signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

type SeverityClassifier struct {
    regoEngine *rego.Rego
    logger     logr.Logger
}

type SeverityResult struct {
    Severity string `json:"severity"` // critical, warning, info, or unknown
    Source   string `json:"source"`   // rego-policy or fallback
}

func NewSeverityClassifier(policyPath string, logger logr.Logger) (*SeverityClassifier, error) {
    // Load severity.rego policy (similar to environment/priority classifiers)
    // ... implementation
}

func (c *SeverityClassifier) ClassifySeverity(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) (SeverityResult, error) {
    input := map[string]interface{}{
        "signal": map[string]interface{}{
            "severity": sp.Spec.Signal.Severity, // External value (e.g., "Sev1")
        },
    }

    // Evaluate Rego policy
    results, err := c.regoEngine.Eval(ctx, rego.EvalInput(input))
    if err != nil {
        c.logger.Error(err, "Severity Rego evaluation failed", "externalSeverity", sp.Spec.Signal.Severity)
        // Fallback to "unknown" on Rego failure
        return SeverityResult{Severity: "unknown", Source: "fallback-error"}, nil
    }

    // Parse result
    // ... implementation

    return result, nil
}
```

**3. Controller Integration** (`internal/controller/signalprocessing/signalprocessing_controller.go`):
```go
func (r *SignalProcessingReconciler) reconcileClassifying(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) (ctrl.Result, error) {
    // ... existing environment classification
    // ... existing priority assignment

    // NEW: Severity determination
    severityResult, err := r.severityClassifier.ClassifySeverity(ctx, sp)
    if err != nil {
        return ctrl.Result{}, fmt.Errorf("severity classification failed: %w", err)
    }

    // Write to Status
    sp.Status.Severity = severityResult.Severity

    // Emit audit event if fallback used
    if severityResult.Source == "fallback" || severityResult.Source == "fallback-error" {
        r.auditClient.RecordSeverityFallback(ctx, sp, severityResult)
    }

    // Emit metrics
    severityDeterminationTotal.WithLabelValues(
        sp.Spec.Signal.Severity, // external
        severityResult.Severity,  // normalized
        severityResult.Source,    // rego-policy/fallback
    ).Inc()

    // ... continue with business classification
}
```

**4. Audit Client Update** (`pkg/signalprocessing/audit/client.go`):
```go
// BEFORE (Line 84):
payload.Severity.SetTo(toSignalProcessingAuditPayloadSeverity(sp.Spec.Signal.Severity)) // ❌ External

// AFTER:
payload.Severity.SetTo(toSignalProcessingAuditPayloadSeverity(sp.Status.Severity)) // ✅ Normalized
```

**Validation**:
- [ ] Unit tests for `SeverityClassifier` (default policy)
- [ ] Unit tests for custom operator policies (Sev1-4, P0-P4)
- [ ] Integration tests: "Sev1" → Status.Severity = "critical"
- [ ] Audit event emitted when fallback to "unknown"
- [ ] Metrics emitted for severity determination

**Deliverables**:
- Default `severity.rego` policy in `deploy/`
- `SeverityClassifier` implementation
- Controller integration with Status field population
- Unit + integration tests

---

#### **Week 3: Gateway Refactoring (Severity + Priority Cleanup)**

**Files to Modify**:
1. `pkg/gateway/adapters/prometheus_adapter.go`
2. `pkg/gateway/adapters/kubernetes_event_adapter.go`
3. `docs/services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md`

**Changes**:

**1. Remove Severity Hardcoding** (`pkg/gateway/adapters/prometheus_adapter.go`):
```go
// BEFORE (Lines 234-241):
func determineSeverity(labels map[string]string) string {
    severity := labels["severity"]
    switch severity {
    case "critical", "warning", "info":
        return severity
    default:
        return "warning" // Default to warning for unknown severities
    }
}

// AFTER:
// REMOVED - Gateway now passes through raw severity without transformation
```

**Update Prometheus Alert Processing**:
```go
// BEFORE:
severity := determineSeverity(alert.Labels)

// AFTER:
severity := alert.Labels["severity"] // Pass through as-is (e.g., "Sev1")
if severity == "" {
    severity = "unknown" // Only default if missing entirely
}
```

**2. Remove Kubernetes Event Severity Mapping** (`pkg/gateway/adapters/kubernetes_event_adapter.go`):
```go
// BEFORE:
func mapSeverity(eventType, reason string) string {
    // Hardcoded mapping logic
}

// AFTER:
// REMOVED - Pass through event Type/Reason as-is
// SignalProcessing Rego will map k8s event types to severity
```

**3. Priority Cleanup (BR-GATEWAY-007)**:
```go
// REMOVE ALL priority determination logic from Gateway adapters
// Gateway should NOT determine priority (SignalProcessing owns this via priority.rego)
```

**4. Update Business Requirements**:
```markdown
### **BR-GATEWAY-007: Priority Assignment** [DEPRECATED]
**Status**: ⛔ DEPRECATED (2026-01-09)
**Reason**: Priority determination moved to SignalProcessing Rego (BR-SP-070)
**Replacement**: Gateway passes through raw priority hints, SignalProcessing determines final priority
**Migration**: Remove priority determination logic from Gateway adapters
```

**Validation**:
- [ ] Gateway writes "Sev1" to RemediationRequest (no transformation)
- [ ] Gateway writes "P0" to RemediationRequest (no transformation)
- [ ] Gateway no longer defaults unknown severity to "warning"
- [ ] Gateway no longer determines priority
- [ ] Integration tests: Prometheus alert "Sev1" → RR.Spec.Severity = "Sev1"
- [ ] Gateway audit events include raw external severity

**Deliverables**:
- Gateway code cleaned of hardcoded severity/priority logic
- BR-GATEWAY-007 marked DEPRECATED
- Integration tests for pass-through behavior

---

#### **Week 4: Consumer Updates + DataStorage Triage**

**Files to Modify**:
1. `pkg/remediationorchestrator/creator/aianalysis.go`
2. `pkg/remediationorchestrator/creator/notification.go` (NO CHANGE - keeps external)
3. `pkg/remediationorchestrator/handler/workflowexecution.go` (NO CHANGE - keeps external)
4. `docs/services/crd-controllers/03-remediationorchestrator/BUSINESS_REQUIREMENTS.md`

**Changes**:

**1. AIAnalysis Creator** (`pkg/remediationorchestrator/creator/aianalysis.go:171`):
```go
// BEFORE:
return aianalysisv1.SignalContextInput{
    Fingerprint:      rr.Spec.SignalFingerprint,
    Severity:         rr.Spec.Severity, // ❌ External "Sev1"
    SignalType:       rr.Spec.SignalType,
    Environment:      environment,
    BusinessPriority: priority,
    // ... other fields
}

// AFTER:
return aianalysisv1.SignalContextInput{
    Fingerprint:      rr.Spec.SignalFingerprint,
    Severity:         sp.Status.Severity, // ✅ Normalized "critical" from Rego
    SignalType:       rr.Spec.SignalType,
    Environment:      environment,
    BusinessPriority: priority,
    // ... other fields
}
```

**2. Notification Creator** (NO CHANGE - Approved Decision Q1):
```go
// pkg/remediationorchestrator/creator/notification.go:110,127,224,559
// KEEP: rr.Spec.Severity (external "Sev1")
// Rationale: Operators want to see their own severity values in notifications
```

**3. WorkflowExecution Handler** (NO CHANGE - Approved Decision Q1):
```go
// pkg/remediationorchestrator/handler/workflowexecution.go:447
// KEEP: rr.Spec.Severity (external "Sev1")
// Rationale: Operators want to see their own severity values in failure messages
```

**4. Audit Events** (IMPLEMENT - Approved Decision Q2):
```go
// All audit event constructors: Include BOTH external + normalized severity
type AuditEventPayload struct {
    SeverityExternal   string `json:"severity_external"`   // "Sev1"
    SeverityNormalized string `json:"severity_normalized"` // "critical"
    // ... other fields
}
```

**5. DataStorage Triage**:
- **Task**: Check if DataStorage needs SignalProcessing severity
- **Current**: WorkflowSearch uses `critical, high, medium, low` (different domain - workflows, not signals)
- **Decision**: Keep separate unless integration need discovered
- **Action**: Document decision in DataStorage BUSINESS_REQUIREMENTS.md

**Validation**:
- [ ] AIAnalysis receives normalized severity from SP Status
- [ ] AIAnalysis LLM prompts use consistent severity values
- [ ] Notifications show external severity ("Sev1")
- [ ] WE failure messages show external severity ("Sev1")
- [ ] Audit events include both external + normalized
- [ ] E2E test: "Sev1" → SP determines "critical" → AA receives "critical"

**Deliverables**:
- RemediationOrchestrator consumer updates
- Audit event dual-severity fields
- DataStorage triage decision documented
- E2E tests for full severity flow

---

### **Week 5: Testing + Buffer**

**Integration Testing**:
- [ ] E2E test: Prometheus "Sev1" → RR → SP Rego → AA with "critical"
- [ ] E2E test: PagerDuty "P0" → SP Rego → AA with "critical"
- [ ] E2E test: Unknown "MyCustomSev" → SP fallback → "unknown"
- [ ] E2E test: Notification shows external "Sev1"
- [ ] E2E test: WE failure message shows external "Sev1"
- [ ] E2E test: Audit events include both severities

**Operator Testing**:
- [ ] Deploy custom `severity.rego` ConfigMap
- [ ] Verify hot-reload without pod restart
- [ ] Verify custom mapping "Sev1" → "critical"

**Backward Compatibility Testing**:
- [ ] Existing deployments with "critical/warning/info" continue working
- [ ] Default Rego policy provides 1:1 mapping

**Buffer Week**:
- Fix any issues discovered during testing
- Documentation updates
- Migration guide for operators

---

### **Data Flow Diagram (Approved Architecture)**

```
Step 1: Gateway (Pass-Through)
┌──────────────────────────┐
│ PrometheusAdapter        │
│ severity := alert.Labels │
│   ["severity"]           │────────> "Sev1" (raw, no transformation)
│ # No switch/case         │
│ # No normalization       │
└──────────────────────────┘
                │
                ▼
Step 2: RemediationRequest (No Enum)
┌──────────────────────────────┐
│ RR.Spec.Severity: "Sev1"     │
│ # No enum validation         │
│ # Accepts any string         │
└──────────────────────────────┘
                │
                ▼
Step 3: SignalProcessing (Copy + Rego)
┌──────────────────────────────┐        ┌──────────────────────────────┐
│ SP.Spec.Signal.Severity      │        │ severity.rego (ConfigMap)    │
│   "Sev1" (copied from RR)    │───────>│                              │
│                               │        │ result := {                  │
│                               │        │   "severity": "critical"     │
│                               │        │ } if {                       │
│                               │        │   input.signal.severity in   │
│                               │        │   ["Sev1", "SEV1", "sev1"]   │
│                               │        │ }                            │
│                               │        │                              │
│                               │        │ default result := {          │
│                               │        │   "severity": "unknown"      │
│                               │        │ }                            │
└──────────────────────────────┘        └──────────────────────────────┘
                                                      │
                                                      ▼
                                        ┌──────────────────────────────┐
                                        │ SP.Status.Severity           │
                                        │   "critical" (determined)    │
                                        └──────────────────────────────┘
                                                      │
                ┌─────────────────────────────────────┼─────────────────────────────────────┐
                │                                     │                                     │
                ▼                                     ▼                                     ▼
Step 4: Consumers
┌────────────────────┐            ┌────────────────────┐            ┌────────────────────┐
│ AIAnalysis         │            │ Notifications      │            │ WorkflowExecution  │
│ Read:              │            │ Read:              │            │ Failure Messages:  │
│   sp.Status.       │            │   rr.Spec.Severity │            │   rr.Spec.Severity │
│   Severity         │            │   "Sev1"           │            │   "Sev1"           │
│   "critical" ✅    │            │   (external) ✅    │            │   (external) ✅    │
└────────────────────┘            └────────────────────┘            └────────────────────┘
```

---

## Consequences

### **Positive**

- ✅ **Customer Extensibility**: Operators can use ANY severity scheme (Sev1-4, P0-P4, etc.)
- ✅ **Architectural Consistency**: All policy logic (severity, priority, environment, business) in SignalProcessing Rego
- ✅ **Separation of Concerns**: Gateway extracts, SignalProcessing determines, consumers use
- ✅ **Operator Control**: Severity mapping fully configurable via Rego ConfigMaps
- ✅ **Backward Compatible**: Default 1:1 Rego policy works for existing deployments
- ✅ **Traceability**: Audit events include both external + normalized severity
- ✅ **Observability**: Metrics track severity determination success/fallback rates
- ✅ **Hot-Reload**: ConfigMap updates apply without pod restarts

### **Negative**

- ⚠️ **CRD Changes Required**: Remove enums from RR/SP, add Status field to SP
  **Mitigation**: Pre-release product, no migration needed

- ⚠️ **Consumer Updates Required**: AIAnalysis/RO must read from new Status field
  **Mitigation**: Clear 4-week plan with phased rollout, comprehensive testing

- ⚠️ **Rego Policy Complexity**: Operators must learn Rego for custom mappings
  **Mitigation**: Provide example policies for common schemes (Sev1-4, P0-P4, Critical/High/Medium/Low)

- ⚠️ **Potential for Misconfiguration**: Operator could map all severities to "critical"
  **Mitigation**: Validation webhook for Rego policies (V2.0 enhancement)

### **Neutral**

- 🔄 **Priority Cleanup**: Gateway priority logic removed as part of same refactoring (approved)
- 🔄 **DataStorage Enum**: Workflow severity (`critical, high, medium, low`) kept separate (different domain)
- 🔄 **Message Severity**: Notifications/WE show external severity (operator familiarity vs consistency trade-off)

---

## Validation Results

### **Confidence Assessment Progression**

- **Initial assessment**: 40% confidence (Gateway ConfigMap approach - architectural debt)
- **After triage analysis**: 85% confidence (SignalProcessing Rego approach - architectural fit)
- **After user approval**: 95% confidence (Q1: external in messages, Q2: both in audit)

### **Key Validation Points**

- ✅ **Architectural Alignment**: Matches environment/priority Rego pattern (BR-SP-051, BR-SP-070)
- ✅ **Separation of Concerns**: Gateway extracts, SignalProcessing determines (BR-GATEWAY-111, BR-SP-105)
- ✅ **Customer Requirements**: Enables ANY severity scheme (Sev1-4, P0-P4, etc.)
- ✅ **Backward Compatibility**: Default 1:1 Rego policy for existing deployments
- ✅ **Traceability**: Audit events include both external + normalized (Q2 decision)
- ✅ **Operator Familiarity**: Messages show external severity (Q1 decision)

### **CRD Enum Audit Findings**

**Three Enum Validations Found (All Blocking)**:
1. ✅ `RemediationRequest.Spec.Severity` → Enum removal planned (Week 1)
2. ✅ `SignalProcessing.Spec.Signal.Severity` → Enum removal planned (Week 1)
3. ✅ `AIAnalysis.SignalContextInput.Severity` → Enum kept (receives normalized values only)

**Severity Field References**:
- **Gateway**: 2 locations (Prometheus, K8s event adapters) - Refactoring to remove hardcoding (Week 3)
- **RemediationOrchestrator**: 9 locations - 1 update (AIAnalysis creator), 8 keep external (Week 4)
- **SignalProcessing**: 3 locations (audit events) - Update to normalized (Week 2)

---

## Related Decisions

### **Builds On**:
- **[BR-SP-051](../../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md)**: Environment Classification via Rego (established pattern)
- **[BR-SP-070](../../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md)**: Priority Assignment via Rego (established pattern)
- **[DD-CATEGORIZATION-001](DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md)**: Gateway vs SignalProcessing responsibility split

### **Supports**:
- **[BR-GATEWAY-111](../../services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md)**: Gateway Signal Pass-Through Architecture (NEW)
- **[BR-SP-105](../../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md)**: Severity Determination via Rego Policy (NEW)

### **Supersedes**:
- **[BR-GATEWAY-007](../../services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md)**: Priority Assignment (DEPRECATED - moved to SP)
- **[TRIAGE-SEVERITY-EXTENSIBILITY.md](TRIAGE-SEVERITY-EXTENSIBILITY.md)**: Problem analysis (RESOLVED via this DD)

---

## Review & Evolution

### **When to Revisit**

- If customer requests UI-based severity mapping (vs. Rego YAML editing)
- If Rego policy validation becomes necessary (prevent misconfiguration)
- If DataStorage needs to integrate signal severity (currently separate domains)
- If additional policy mechanisms needed (beyond Rego)

### **Success Metrics**

- **Customer Onboarding**: 100% of customers can use their existing severity schemes (Sev1-4, P0-P4, etc.)
- **Rego Policy Adoption**: 90% of operators use default policy, 10% customize
- **Severity Fallback Rate**: <5% of signals fall back to "unknown" (indicates good mapping coverage)
- **Architectural Consistency**: 100% of policy logic (severity, priority, environment, business) in SignalProcessing Rego

---

## Appendix: Example Operator Configurations

### **Example 1: Enterprise "Sev" Scheme**

```rego
package signalprocessing.severity

import rego.v1

result := {"severity": "critical", "source": "rego-policy"} if {
    input.signal.severity in ["Sev1", "SEV1", "sev1"]
}

result := {"severity": "high", "source": "rego-policy"} if {
    input.signal.severity in ["Sev2", "SEV2", "sev2"]
}

result := {"severity": "medium", "source": "rego-policy"} if {
    input.signal.severity in ["Sev3", "SEV3", "sev3"]
}

result := {"severity": "low", "source": "rego-policy"} if {
    input.signal.severity in ["Sev4", "SEV4", "sev4"]
}

default result := {"severity": "unknown", "source": "fallback"}
```

### **Example 2: PagerDuty "P" Scheme**

```rego
package signalprocessing.severity

import rego.v1

result := {"severity": "critical", "source": "rego-policy"} if {
    input.signal.severity in ["P0", "P1"]
}

result := {"severity": "high", "source": "rego-policy"} if {
    input.signal.severity in ["P2"]
}

result := {"severity": "medium", "source": "rego-policy"} if {
    input.signal.severity in ["P3"]
}

result := {"severity": "low", "source": "rego-policy"} if {
    input.signal.severity in ["P4"]
}

default result := {"severity": "unknown", "source": "fallback"}
```

### **Example 3: Multi-Scheme Support**

```rego
package signalprocessing.severity

import rego.v1

# Critical severity mappings
result := {"severity": "critical", "source": "rego-policy"} if {
    input.signal.severity in [
        "Sev1", "SEV1", "sev1",           # Enterprise
        "P0", "P1",                        # PagerDuty
        "critical", "CRITICAL", "Critical" # Standard
    ]
}

# High severity mappings
result := {"severity": "high", "source": "rego-policy"} if {
    input.signal.severity in [
        "Sev2", "SEV2", "sev2",           # Enterprise
        "P2",                              # PagerDuty
        "high", "HIGH", "High"            # Standard
    ]
}

# Medium severity mappings
result := {"severity": "medium", "source": "rego-policy"} if {
    input.signal.severity in [
        "Sev3", "SEV3", "sev3",           # Enterprise
        "P3",                              # PagerDuty
        "medium", "MEDIUM", "Medium"      # Standard
    ]
}

# Low severity mappings
result := {"severity": "low", "source": "rego-policy"} if {
    input.signal.severity in [
        "Sev4", "SEV4", "sev4",           # Enterprise
        "P4",                              # PagerDuty
        "low", "LOW", "Low"               # Standard
    ]
}

default result := {"severity": "unknown", "source": "fallback"}
```

---

## Implementation Status

### **Service Impact Triage (January 16, 2026)**

**Services Analyzed**: 8 services triaged for DD-SEVERITY-001 impact

| Service | Impact | Code Changes | Unit Tests | Integration Tests | Status |
|---------|--------|--------------|------------|-------------------|--------|
| **Gateway** | ✅ Updated | Week 3 | ✅ Complete | 🟡 80% (2 pending) | 95% |
| **SignalProcessing** | ✅ Updated | Week 2 | ✅ Complete | ✅ Complete | 100% |
| **RemediationOrchestrator** | ✅ Updated | Week 4 | ✅ Complete | ⚠️ Missing | 70% |
| **AIAnalysis Controller** | ✅ No Impact | N/A | N/A | N/A | 100% |
| **Notification Controller** | ✅ No Impact | N/A | N/A | N/A | 100% |
| **WorkflowExecution Controller** | ✅ No Impact | N/A | N/A | N/A | 100% |
| **DataStorage** | ✅ Triaged | Separate domain | N/A | N/A | 100% |
| **HolmesGPT-API** | ✅ No Impact | Pass-through | N/A | N/A | 100% |

**Key Findings**:
- ✅ **AIAnalysis Controller**: No severity logic in reconciliation (just processes CRDs)
- ✅ **Notification Controller**: No severity logic in reconciliation (RO handles NT creation with external severity)
- ✅ **WorkflowExecution Controller**: No severity logic in reconciliation (RO handles WE creation with external severity)
- ✅ **HolmesGPT-API**: Accepts severity as string, no validation/transformation (passes to LLM prompt)
- ✅ **DataStorage**: Workflow severity is separate domain (decision documented in BUSINESS_REQUIREMENTS.md)

**Rationale**: CRD controllers (AA, NT, WE) don't make policy decisions about severity - they just process their respective CRDs. RemediationOrchestrator is responsible for:
- Creating AIAnalysis with **normalized** severity (from `sp.Status.Severity`)
- Creating Notification with **external** severity (from `rr.Spec.Severity`)
- Creating WorkflowExecution with **external** severity (from `rr.Spec.Severity`)

---

### **Week 1: CRD Schema Changes**
| Task | Status | Date | Tests | Files |
|------|--------|------|-------|-------|
| RemediationRequest enum removal | ✅ Complete | Jan 15, 2026 | CRD validation | `api/remediation/v1alpha1/remediationrequest_types.go:234` |
| SignalProcessing enum removal | ✅ Complete | Jan 15, 2026 | CRD validation | `api/signalprocessing/v1alpha1/signalprocessing_types.go` |
| SignalProcessing Status.Severity field | ✅ Complete | Jan 15, 2026 | CRD validation | `api/signalprocessing/v1alpha1/signalprocessing_types.go:235` |
| AIAnalysis enum update (v1.1) | ✅ Complete | Jan 15, 2026 | CRD validation | `api/aianalysis/v1alpha1/aianalysis_types.go:121,485` |

**Deliverables**: ✅ All CRD changes complete, RR accepts ANY severity string

---

### **Week 2: SignalProcessing Rego Implementation**
| Task | Status | Date | Tests | Files |
|------|--------|------|-------|-------|
| Default severity.rego policy | ✅ Complete | Jan 15, 2026 | Unit + Integration | `config/severity-policy-example.rego` |
| SeverityClassifier implementation | ✅ Complete | Jan 15, 2026 | Unit | `pkg/signalprocessing/classifier/severity.go` |
| Controller integration | ✅ Complete | Jan 15, 2026 | Integration | `internal/controller/signalprocessing/` |
| Audit client update | ✅ Complete | Jan 15, 2026 | Integration | `pkg/signalprocessing/audit/client.go:84,325` |
| Test fixtures created | ✅ Complete | Jan 16, 2026 | Documentation | `test/fixtures/severity/` |

**Unit Tests**:
- ✅ `test/unit/signalprocessing/severity_classifier_test.go` - Basic classification
- ✅ `test/unit/signalprocessing/severity_case_sensitivity_test.go` - Case handling

**Integration Tests**:
- ✅ SignalProcessing controller emits normalized severity in status

**Deliverables**: ✅ All SP changes complete, Rego policy functional

---

### **Week 3: Gateway Refactoring**
| Task | Status | Date | Tests | Files |
|------|--------|------|-------|-------|
| Remove determineSeverity hardcoding | ✅ Complete | Jan 16, 2026 | Unit | `pkg/gateway/adapters/prometheus_adapter.go` |
| Pass-through severity logic | ✅ Complete | Jan 16, 2026 | Unit + Integration | `pkg/gateway/adapters/*.go` |
| Prometheus adapter update | ✅ Complete | Jan 16, 2026 | Integration | `pkg/gateway/adapters/prometheus_adapter.go` |
| K8s event adapter update | ✅ Complete | Jan 16, 2026 | Integration | `pkg/gateway/adapters/kubernetes_event_adapter.go` |
| BR-GATEWAY-007 deprecated | ✅ Complete | Jan 16, 2026 | Documentation | `docs/services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md` |

**Unit Tests**:
- ✅ Gateway unit tests verify pass-through behavior

**Integration Tests Created**:
- ✅ `[GW-INT-SEV-001]` - Preserve 'critical' severity (baseline)
- ✅ `[GW-INT-SEV-002]` - Preserve 'warning' severity
- ✅ `[GW-INT-SEV-003]` - Preserve 'info' severity
- ✅ `[GW-INT-SEV-004]` - Default to 'unknown' if missing
- ⚠️ `[GW-INT-SEV-005]` - Preserve 'Sev1' enterprise severity **[PENDING: Remove PIt/Skip]**
- ⚠️ `[GW-INT-SEV-006]` - Preserve 'P0' PagerDuty severity **[PENDING: Remove PIt/Skip]**
- ✅ `[GW-INT-SEV-007]` - Preserve K8s 'Warning' event type
- ✅ `[GW-INT-SEV-008]` - Preserve K8s 'Error' event type
- ✅ `[GW-INT-SEV-009]` - No hardcoded OOMKilled→critical mapping
- ✅ `[GW-INT-SEV-010]` - Accept ANY non-empty severity string

**Integration Tests Status**: 🟡 80% complete (8/10 passing, 2 pending `PIt()`/`Skip()` removal)

**Blocking Issue**: Tests 005 & 006 use `PIt()` + `Skip()` which is **FORBIDDEN** per TESTING_GUIDELINES.md lines 1104-1233

**Deliverables**: ✅ Gateway code complete, 🟡 Integration tests 80% complete

---

### **Week 4: Consumer Updates + DataStorage Triage**
| Task | Status | Date | Tests | Files |
|------|--------|------|-------|-------|
| AIAnalysis creator update | ✅ Complete | Jan 15, 2026 | ✅ Unit | `pkg/remediationorchestrator/creator/aianalysis.go:172` |
| Notification creator (no change) | ✅ Complete | Jan 15, 2026 | N/A | `pkg/remediationorchestrator/creator/notification.go` |
| WorkflowExecution handler (no change) | ✅ Complete | Jan 15, 2026 | N/A | `pkg/remediationorchestrator/handler/workflowexecution.go` |
| DataStorage triage | ✅ Complete | Jan 16, 2026 | Documentation | `docs/services/stateless/data-storage/BUSINESS_REQUIREMENTS.md` |
| Audit events (dual severity) | ✅ Complete | Jan 15, 2026 | Integration | `pkg/signalprocessing/audit/client.go` |

**Unit Tests**:
- ✅ `test/unit/remediationorchestrator/aianalysis_creator_test.go:200-237`
  - "should use normalized severity from SignalProcessing.Status.Severity (DD-SEVERITY-001)"
  - Verifies RO uses `sp.Status.Severity` (normalized "critical") not `rr.Spec.Severity` (external "Sev1")

**Integration Tests**:
- ⚠️ **MISSING**: No RO integration test verifies normalized severity propagation in K8s environment

**Rationale for Missing Integration Test**: RO integration tests focus on routing, approval, timeouts, notifications - but not severity propagation specifically. This is a **gap** that should be filled.

**Deliverables**: ✅ RO code complete, ✅ Unit tests complete, ⚠️ Integration tests missing

---

### **Week 5: Testing + Buffer**
| Task | Status | Date | Tests |
|------|--------|------|-------|
| Gateway integration tests | 🔄 In Progress | Jan 16, 2026 | 8/10 complete (2 pending) |
| RO integration tests | ⚠️ **PENDING** | - | Not started |
| E2E pipeline tests | ⚠️ **PENDING** | - | Not started |
| Test fixtures created | ✅ Complete | Jan 16, 2026 | `test/fixtures/severity/` |

**Test Fixtures**:
- ✅ `enterprise-sev-policy.rego` - Enterprise "Sev1-4" scheme
- ✅ `pagerduty-p-policy.rego` - PagerDuty "P0-P4" scheme
- ✅ `prometheus-alert-sev1.json` - Production outage with `severity="Sev1"`
- ✅ `prometheus-alert-p0.json` - Database outage with `severity="P0"`
- ✅ `README.md` - Complete usage guide with code examples

**E2E Tests** (Pending - Will use test fixtures):
- ⚠️ Full "Sev1" → "critical" pipeline test
- ⚠️ Full "P0" → "critical" pipeline test
- ⚠️ Custom severity hot-reload verification

---

### **Overall Progress: 85% Complete**

| Week | Component | Code | Unit Tests | Integration Tests | E2E Tests | Status |
|------|-----------|------|------------|-------------------|-----------|--------|
| **Week 1** | CRD Schema | ✅ 100% | ✅ 100% | N/A | N/A | ✅ 100% |
| **Week 2** | SignalProcessing | ✅ 100% | ✅ 100% | ✅ 100% | N/A | ✅ 100% |
| **Week 3** | Gateway | ✅ 100% | ✅ 100% | 🟡 80% (2 pending) | N/A | 🟡 95% |
| **Week 4** | RO + DataStorage | ✅ 100% | ✅ 100% | ⚠️ 0% (missing) | N/A | 🟡 70% |
| **Week 5** | E2E Pipeline | N/A | N/A | N/A | ⚠️ 0% (pending) | ⚠️ 0% |

**Blocking Items**:
1. ⚠️ **P1**: Enable Gateway tests 005 & 006 (remove `PIt()`/`Skip()` - TESTING_GUIDELINES.md violation)
2. ⚠️ **P2**: Create RO integration test for normalized severity propagation
3. ⚠️ **P3**: Create E2E pipeline test using test fixtures (full Gateway → RR → SP → RO → AA flow)

**Confidence Assessment**:
- **Code Implementation**: 100% complete (all services updated or triaged)
- **Unit Test Coverage**: 100% complete (all critical paths tested)
- **Integration Test Coverage**: 75% complete (Gateway 80%, SP 100%, RO 0%)
- **E2E Test Coverage**: 0% complete (pending Gateway Week 3 completion)

---

**Document Version**: 1.1
**Last Updated**: 2026-01-16 (Added Implementation Status section)
**Next Review**: After completing Gateway tests 005 & 006 and RO integration test
