# DD-SP-001: Remove Classification Confidence Scores from SignalProcessing

**Design Decision ID**: DD-SP-001
**Status**: ✅ **APPROVED** - Pre-Release Simplification + Security Fix
**Version**: 1.2
**Created**: 2025-12-14
**Last Updated**: 2025-12-14
**Author**: User Feedback + Architecture Review
**Context**: Pre-release product - no backwards compatibility required
**Security Impact**: Removes untrusted signal-labels source

---

## Changelog

### Version 1.2 (2025-12-14) - SECURITY UPDATE
- 🚨 **SECURITY**: Remove `signal-labels` fallback (untrusted external source)
- ✅ Reduced to 3 sources: `namespace-labels`, `rego-inference`, `default`
- ✅ Added security rationale for source restrictions
- ✅ Updated BR-SP-080 V2.0 to remove signal-labels acceptance criteria

### Version 1.1 (2025-12-14)
- ✅ **APPROVED** status (user confirmed pre-release = no breaking changes)
- ✅ Updated risks section (removed migration concerns)
- ✅ Simplified timeline (immediate implementation, no deprecation period)
- ✅ Added implementation scope and effort estimates
- ✅ Updated BR-SP-080 to reflect source-based approach (V2.0)

### Version 1.0 (2025-12-14)
- 📝 Initial proposal
- 🔍 Investigation results
- 📊 Impact analysis
- 💡 Alternatives considered

---

## Context & Problem

SignalProcessing CRD currently includes confidence scores (0.0-1.0) for environment and priority classification per **BR-SP-080**:

```go
type EnvironmentClassification struct {
    Environment  string      `json:"environment"`
    Confidence   float64     `json:"confidence"`      // ← Question: Is this needed?
    Source       string      `json:"source"`
    ClassifiedAt metav1.Time `json:"classifiedAt"`
}
```

**User Question**: "I'm not sure what's the value of this confidence if it's something that is a factual record retrieved from any valid source. As long as it's valid, it's just confidence 100%."

**Problem**: Confidence scores are **not used in any business logic** and are redundant with the `source` field.

---

## Investigation Results

### ✅ **Where Confidence IS Used**

| Location | Purpose | Impact |
|----------|---------|--------|
| **Audit Events** | `eventData["environment_confidence"]` | Observability only |
| **Metrics** | `signalprocessing_classification_confidence` | Monitoring only |
| **Tests** | `Expect(confidence).To(BeNumerically(">=", 0.95))` | Test assertions |

### ❌ **Where Confidence is NOT Used**

- **NOT** used in workflow selection
- **NOT** used in approval decisions
- **NOT** used in routing logic
- **NOT** used in prioritization
- **NOT** used in any conditional branches

**Critical Finding**: Environment/Priority classification confidence is **pure metadata** with no business logic impact.

---

## Analysis: Is Confidence Redundant?

### **Confidence Mapping (Historical — REMOVED)**

Confidence scores were 100% derivable from `source` and have been removed per this decision.

| Source | Former Confidence | Status |
|--------|-------------------|--------|
| `namespace-labels` | 1.0 | Active |
| `rego-inference` | 0.8 | Active (renamed from `configmap`) |
| ~~`signal-labels`~~ | ~~0.8~~ | **REMOVED** (security risk) |
| `default` | 0.0 | Active |

### **What Source Tells Us (IMPLEMENTED)**

| Source | Meaning | Trust Level |
|--------|---------|-------------|
| `namespace-labels` | Operator explicitly labeled via RBAC | ✅ High |
| `rego-inference` | Rego policy pattern matching (deterministic) | ✅ Medium |
| `default` | No detection succeeded | ⚠️ Low |

**Conclusion**: The `source` field provides **all the information** needed to understand classification quality. Confidence scores have been removed.

---

## Decision

### **APPROVED: Remove Confidence Field + Restrict Sources (Security)**

**Rationale**:
1. ✅ **Redundant**: Confidence is 100% derivable from `source`
2. ✅ **Simpler API**: Fewer fields to understand/maintain
3. ✅ **No Logic Impact**: No business logic uses confidence
4. ✅ **Still Observable**: `source` provides same insight for operators
5. ✅ **Clearer Intent**: Pattern matching IS deterministic (as user correctly noted)
6. 🚨 **SECURITY**: Remove `signal-labels` source (untrusted external source)

### **SECURITY: Remove Signal Labels Fallback**

**Problem**: Current implementation trusts `signal.Labels["kubernaut.ai/environment"]` for environment classification.

**Security Risk**:
- ❌ Signals originate from **untrusted external sources** (Prometheus, K8s events)
- ❌ Attacker could inject labels into Prometheus alerts
- ❌ **Privilege escalation**: Staging alert → labeled "production" → triggers production workflow
- ❌ No validation of signal label authenticity

**Example Attack**:
```yaml
# Attacker modifies Prometheus alerting rule to inject label
- alert: StagingPodCrash
  labels:
    kubernaut.ai/environment: production  # ← ATTACKER INJECTED THIS
    severity: critical
```
Result: System treats staging issue as production, potentially executing dangerous workflows.

**Solution**: **ONLY trust namespace labels and Rego policy** (both operator-controlled, RBAC-protected)

### **New API Structure**

```go
type EnvironmentClassification struct {
    Environment  string      `json:"environment"`
    // Confidence field REMOVED (redundant with source)
    Source       string      `json:"source"`  // Valid: "namespace-labels", "rego-inference", "default"
    ClassifiedAt metav1.Time `json:"classifiedAt"`
}

type PriorityAssignment struct {
    Priority     string      `json:"priority"`
    // Confidence field REMOVED (redundant with source)
    Source       string      `json:"source"`  // Valid: "rego-policy", "severity-fallback", "default"
    AssignedAt   metav1.Time `json:"assignedAt"`
}
```

**Valid Source Values** (Security-Restricted):

**Environment Classification**:
- `"namespace-labels"`: Operator-defined `kubernaut.ai/environment` label (RBAC-protected) ✅
- `"rego-inference"`: Rego pattern matching from namespace name (deterministic) ✅
- `"default"`: No detection succeeded → "unknown" ✅
- ~~`"signal-labels"`~~: **REMOVED** - Untrusted external source 🚨

**Priority Assignment**:
- `"rego-policy"`: Rego matrix (environment × severity) ✅
- `"severity-fallback"`: Severity-only when environment unknown ✅
- `"default"`: No classification possible → "P3" ✅

---

## Consequences

### ✅ **Benefits**

1. **Simpler API**: 2 fewer fields in CRD status
2. **Clearer Semantics**: "Source" is more meaningful than "confidence" for deterministic classification
3. **Reduced Confusion**: No more questions like "what does 0.8 confidence mean?"
4. **Easier Testing**: No more arbitrary confidence threshold assertions
5. **Less Code**: Remove confidence calculation logic

### ⚠️ **Risks**

1. ~~**Breaking Change**: Existing CRs with confidence field will need migration~~ ✅ **N/A - Pre-release**
2. **Lost Observability**: Dashboards showing confidence scores will break (but no dashboards exist yet)
3. **BR-SP-080 Update Required**: Update business requirement document
4. **Future ML**: If we add ML classification, we'd need confidence back (can add then if needed)

### 🔧 **Migration Path**

✅ **NO MIGRATION NEEDED - Pre-release product**

Simply remove the field from:
- CRD types
- Controller logic
- Rego policies
- Audit events
- Tests (183 references)

---

## Alternatives Considered

### **Alternative 1: Keep Confidence for Observability** ❌

**Pros**:
- ✅ No breaking change
- ✅ Dashboards continue working
- ✅ BR-SP-080 compliant

**Cons**:
- ❌ Redundant with `source` field
- ❌ Misleading for deterministic processes
- ❌ More fields to maintain
- ❌ Doesn't address user's concern

**Verdict**: Rejected - Keeps the problem

---

### **Alternative 2: Document as Observability-Only** ❌

**Pros**:
- ✅ No breaking change
- ✅ Clarifies purpose

**Cons**:
- ❌ Still redundant with `source`
- ❌ Still confusing ("why have it if unused?")
- ❌ Technical debt remains

**Verdict**: Rejected - Doesn't solve the root issue

---

### **Alternative 3: Use Confidence for Approval Decisions** ❌

**Example**:
```go
if envClassification.Confidence < 0.7 {
    requireHumanApproval = true
}
```

**Cons**:
- ❌ Adds complexity for unclear benefit
- ❌ Pattern matching IS deterministic (as user noted)
- ❌ Source already captures detection method quality
- ❌ Creates arbitrary thresholds (why 0.7?)

**Verdict**: Rejected - Overengineering

---

## Impact Analysis

### **Files to Change**

1. **CRD Types**:
   - `api/signalprocessing/v1alpha1/signalprocessing_types.go`
   - Remove `Confidence float64` from `EnvironmentClassification` and `PriorityAssignment`
   - Update CRD comment: ~~`"signal-labels"`~~ → Remove from valid sources

2. **Controller Logic** (Security Fix):
   - `internal/controller/signalprocessing/signalprocessing_controller.go:742-749`
   - **REMOVE signal labels fallback** (lines 741-749):
     ```go
     // 🚨 SECURITY RISK - REMOVE THIS:
     if signal != nil && signal.Labels != nil {
         if env, ok := signal.Labels["kubernaut.ai/environment"]; ok && env != "" {
             result.Environment = env
             result.Confidence = 0.80
             result.Source = "signal-labels"  // ← UNTRUSTED SOURCE
             return result
         }
     }
     ```

3. **Classifier Logic** (Security Fix):
   - `pkg/signalprocessing/classifier/environment.go:171-196`
   - **REMOVE** `trySignalLabelsFallback()` method
   - **REMOVE** signal labels check in `Classify()` method

4. **Rego Policies**:
   - `deploy/signalprocessing/policies/environment.rego`
   - Remove `"confidence": 0.95` from policy results
   - Rename source from `"configmap"` → `"rego-inference"` (clearer)
   - `test/integration/signalprocessing/suite_test.go` (test policy)
   - Remove confidence, rename configmap → rego-inference

5. **Audit Events**:
   - `pkg/signalprocessing/audit/client.go`
   - Remove `eventData["environment_confidence"]` lines
   - Remove `eventData["priority_confidence"]` lines

6. **Tests** (183 references):
   - `test/integration/signalprocessing/*.go`
   - Replace confidence assertions with source assertions
   - Change: `Expect(confidence).To(BeNumerically(">=", 0.95))`
   - To: `Expect(source).To(Equal("namespace-labels"))`

7. **CRD Manifest**:
   - `config/crd/bases/kubernaut.ai_signalprocessings.yaml:293`
   - Update description: Remove `signal-labels` from valid sources
   - Regenerate with `make manifests`

8. **Business Requirements**:
   - ✅ Already updated BR-SP-080 V2.0 with security rationale

---

## BR-SP-080 Status

### **Current Requirement**

```markdown
### BR-SP-080: Confidence Scoring

**Description**: The SignalProcessing controller MUST provide confidence
scores (0.0-1.0) for all categorization decisions.

**Acceptance Criteria**:
- [ ] Confidence 1.0: Explicit label match
- [ ] Confidence 0.8: Pattern match
- [ ] Confidence 0.6: Rego policy inference
- [ ] Confidence 0.4: Default fallback
```

### **Implemented Change** ✅

```markdown
### BR-SP-080: Classification Source Tracking (IMPLEMENTED)

**Description**: The SignalProcessing controller MUST track the source
of all categorization decisions.

**Acceptance Criteria**:
- [x] Source "namespace-labels": Explicit label match (highest trust)
- [x] Source "rego-inference": Rego policy pattern match (medium trust)
- [x] Source "default": No detection succeeded (lowest trust)
- [x] ~~Source "signal-labels"~~: REMOVED (security risk)

**Rationale**: Source provides clear, actionable information about detection
method without introducing arbitrary confidence scores for deterministic
processes.
```

---

## Testing Impact

### **Before (Confidence-Based)**

```go
It("should classify environment with high confidence", func() {
    Expect(final.Status.EnvironmentClassification.Environment).To(Equal("production"))
    Expect(final.Status.EnvironmentClassification.Confidence).To(BeNumerically(">=", 0.95))
})
```

### **After (Source-Based)**

```go
It("should classify environment from namespace label", func() {
    Expect(final.Status.EnvironmentClassification.Environment).To(Equal("production"))
    Expect(final.Status.EnvironmentClassification.Source).To(Equal("namespace-labels"))
})
```

**Benefits**:
- ✅ Clearer intent ("where did this come from?")
- ✅ No arbitrary thresholds (0.95 vs 0.8?)
- ✅ Tests validate actual detection method

---

## Recommendation

### **Action Required** (Pre-Release - Immediate Implementation)

1. ✅ **Remove confidence field** from CRD types
2. ✅ **Update controller logic** to remove confidence calculation
3. ✅ **Update Rego policies** to remove confidence from results
4. ✅ **Update audit events** to remove confidence fields
5. ✅ **Update tests** (183 references) to use `source` instead of `confidence`
6. ✅ **Update BR-SP-080** to reflect source-based approach
7. ✅ **Regenerate CRD manifests** with `make manifests`

### **Timeline**

✅ **Immediate**: No deprecation period needed - pre-release product

**Estimated Effort**: 2-3 hours
- Remove field: 30 min
- Update tests: 90 min
- Update docs: 30 min

---

## User Feedback

**Original Question**: "I'm not sure what's the value of this confidence if it's something that is a factual record retrieved from any valid source."

**Response**: You're absolutely correct. For deterministic classification (labels, pattern matching), confidence scores add complexity without value. The `source` field already provides all the information needed to understand classification quality.

---

## References

- **BR-SP-080**: Confidence Scoring (current requirement)
- **BR-SP-051**: Environment Classification (Primary) - namespace labels
- **BR-SP-052**: Environment Classification (Fallback) - Rego inference
- **BR-SP-053**: Environment Classification (Default) - unknown fallback
- **User Feedback**: 2025-12-14 - "as long as it's valid, it's just confidence 100%"

---

## Status: IMPLEMENTED

**Pre-Release Context**: No backwards compatibility concerns, no migration needed.

**Implementation Steps**:
1. ✅ Remove `Confidence float64` from CRD types
2. ✅ Update controller classification logic
3. ✅ Update Rego policies (source renamed from `configmap` to `rego-inference`)
4. ✅ Update audit events
5. ✅ Fix test references
6. ✅ Update BR-SP-080 documentation
7. ✅ Regenerate CRD manifests
8. ✅ Remove dead pattern-match test and vestigial references (Issue #177)

**Approval**: User feedback confirms no value in confidence scores for deterministic classification.

---

**Document Status**: ✅ IMPLEMENTED
**Last Updated**: 2026-03-04
**Implementation**: Complete (ADR-060 consolidation + Issue #177 cleanup)

