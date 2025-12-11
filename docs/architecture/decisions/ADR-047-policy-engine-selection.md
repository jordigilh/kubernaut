# ADR-047: Policy Engine Selection

**Status**: 🔄 Proposed
**Date**: 2025-12-05
**Decision Makers**: Kubernaut Architecture Team
**Impact**: High - Affects multiple services (SignalProcessing, AIAnalysis, all future policy evaluation)

---

## Context

Kubernaut requires a policy evaluation engine for multiple use cases across SignalProcessing and AIAnalysis services. This is a **cross-cutting architectural decision** that:

1. **Affects multiple services** - SignalProcessing, AIAnalysis, and all future services requiring configurable policy evaluation
2. **Has long-term implications** - Foundational technology choice that will persist for years
3. **Sets a precedent** - All future policy evaluation in kubernaut follows this decision
4. **Involves significant trade-offs** - 8 technologies evaluated with different strengths/weaknesses

**Important Context**: Kubernaut has **NOT been released yet**, allowing full flexibility to evaluate and change technologies without impacting users.

### Use Cases Requiring Policy Evaluation

1. **Environment Classification** (BR-SP-051-053): Determine environment from labels with configurable rules
2. **Priority Assignment** (BR-SP-070-072): Assign priority using severity × environment matrix
3. **CustomLabels Extraction** (BR-SP-102): Extract customer-defined labels from K8s context
4. **Approval Policies** (BR-AI-026-028): Determine if remediation requires manual approval
5. **Mandatory Label Protection** (BR-SP-104): Prevent customer policies from overriding system labels

### Key Requirements

| Requirement ID | Description | Priority |
|----------------|-------------|----------|
| **BR-SP-070** | "Rego-based priority assignment" | P0 - Explicit |
| **BR-SP-102** | CustomLabels format `map[string][]string` | P0 - Critical |
| **BR-SP-104** | Sandboxed execution with security wrapper | P0 - Security |
| **BR-SP-072** | Hot-reload policies from ConfigMap | P1 - Operational |
| **BR-SP-080** | Confidence scoring (structured output) | P1 - Quality |
| **BR-AI-028** | Complex approval decision logic | P1 - Business |
| **DD-WORKFLOW-001 v1.9** | Sandboxed runtime (no network, no FS, timeout, memory) | P0 - Security |

---

## Decision

**Recommended: Rego (OPA)** with Starlark and Expr as documented alternatives.

Since kubernaut has **not been released**, we have flexibility to change. The architecture team should choose from:

| Option | Technology | Action | Timeline | Risk | Confidence |
|--------|------------|--------|----------|------|------------|
| **A** ⭐ | **Rego** | Stay current | 0 weeks | 🟢 Low | 92% |
| **B** | **Starlark** | Migrate | 3-4 weeks | 🟡 Medium | 88% |
| **C** | **Expr** | Migrate | 2-3 weeks | 🟡 Medium | 85% |

**Default Recommendation**: **Option A (Rego)** unless there's a strong reason to change.

---

## Alternatives Considered

This ADR evaluated **8 candidate technologies** for policy evaluation:

### Tier 1: Viable Candidates (≥85% fit)

| Technology | Score | Key Strengths | Key Weaknesses |
|------------|-------|---------------|----------------|
| **Rego (OPA)** | **92%** | CNCF standard, built-in sandbox, rule chaining, existing implementation | Learning curve, larger footprint |
| **Starlark** | **88%** | Python-like syntax, sandboxed by design, Google-proven | Less policy-specific tooling |
| **Expr** | **85%** | Fast, simple, Go-native, growing ecosystem | Less mature sandbox, no rule chaining |

### Tier 2: Possible with Effort (60-84% fit)

| Technology | Score | Key Strengths | Key Weaknesses |
|------------|-------|---------------|----------------|
| **Tengo** | 78% | Go-like syntax, sandboxed, fast | Smaller community |
| **Goja** | 65% | Familiar JS syntax, full-featured | Hard to sandbox, security risk |
| **Jsonnet** | 60% | Deterministic, Google-backed | Not designed for runtime policy |

### Tier 3: Not Recommended (<60% fit)

| Technology | Score | Key Strengths | Key Weaknesses |
|------------|-------|---------------|----------------|
| **CEL** | 45% | K8s native, fast | Cannot return `map[string][]string` |
| **Cue** | 40% | Strong validation | Not for runtime decisions |

### Why CEL is Not Viable

CEL **cannot return `map[string][]string`** - this is an **architectural blocker** for BR-SP-102 (CustomLabels). CEL is appropriate ONLY for CRD validation and simple field checks, not for kubernaut's policy evaluation needs.

---

## Consequences

### Positive

- ✅ **100% BR compliance** - All explicit requirements satisfied
- ✅ **Unified policy architecture** - Single language across services
- ✅ **Production-ready security** - Built-in sandbox, no custom code
- ✅ **Investment protection** - Existing 2 Rego engines remain valid
- ✅ **Team efficiency** - No new language learning required
- ✅ **Industry alignment** - CNCF graduated, wide adoption

### Negative

- ⚠️ **OPA library size** (~5-10MB) - Acceptable for controller binaries
- ⚠️ **Rego learning curve** - Different from traditional languages (team already trained)

### Neutral

- 🔄 CEL remains for CRD validation (Kubernetes-native)
- 🔄 Policy files require ConfigMap management

---

## Related Decisions

| Decision | Relationship |
|----------|--------------|
| **DD-AIANALYSIS-001** | Builds on this ADR - Rego loading strategy implementation |
| **DD-WORKFLOW-001 v1.9** | Implements - Sandbox requirements |
| **ADR-041** | Aligns - Rego receives pre-fetched data |
| **BR-SP-070** | Satisfies - Explicit Rego requirement |
| **BR-SP-102** | Satisfies - CustomLabels format |

---

## Implementation

### Current Rego Implementation

| Component | File | Status |
|-----------|------|--------|
| **SignalProcessing Rego Engine** | `pkg/signalprocessing/rego/engine.go` | ✅ Implemented |
| **AIAnalysis Rego Evaluator** | `pkg/aianalysis/rego/evaluator.go` | ✅ Implemented |
| **CustomLabels Extraction** | `pkg/signalprocessing/rego/extractor.go` | ✅ Implemented |
| **Security Wrapper** | `pkg/signalprocessing/rego/engine.go:SystemLabels` | ✅ Designed |

### Unified Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         KUBERNAUT POLICY ARCHITECTURE                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────┐    ┌─────────────────────┐    ┌─────────────────┐  │
│  │  SIGNAL PROCESSING  │    │     AI ANALYSIS     │    │   CRD SCHEMAS   │  │
│  ├─────────────────────┤    ├─────────────────────┤    ├─────────────────┤  │
│  │ • Environment       │    │ • Approval Policies │    │ • Field         │  │
│  │ • Priority          │    │ • Risk Assessment   │    │   Validation    │  │
│  │ • CustomLabels      │    │ • Threshold Config  │    │ • Enum Values   │  │
│  │                     │    │                     │    │                 │  │
│  │   ┌─────────────┐   │    │   ┌─────────────┐   │    │   ┌─────────┐   │  │
│  │   │    REGO     │   │    │   │    REGO     │   │    │   │   CEL   │   │  │
│  │   │   ENGINE    │   │    │   │  EVALUATOR  │   │    │   │ (K8s)   │   │  │
│  │   └─────────────┘   │    │   └─────────────┘   │    │   └─────────┘   │  │
│  └─────────────────────┘    └─────────────────────┘    └─────────────────┘  │
│                                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                         SHARED REGO INFRASTRUCTURE                     │  │
│  ├───────────────────────────────────────────────────────────────────────┤  │
│  │  • Sandboxed Runtime (no network, no FS, 5s timeout, 128MB memory)    │  │
│  │  • ConfigMap Policy Loading                                           │  │
│  │  • Hot-Reload Capability                                              │  │
│  │  • Security Wrapper (mandatory label protection)                      │  │
│  │  • Prepared Query Caching                                             │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Technical Details

### Master Comparison Table (All 8 Technologies)

| Criterion | **Rego** | **CEL** | **Expr** | **Starlark** | **Cue** | **Jsonnet** | **Goja** | **Tengo** |
|-----------|----------|---------|----------|--------------|---------|-------------|----------|-----------|
| **Can return `map[string][]string`** | ✅ Yes | ❌ No | ✅ Yes | ✅ Yes | ⚠️ Limited | ✅ Yes | ✅ Yes | ✅ Yes |
| **Built-in sandbox** | ✅ Yes | ❌ No | ⚠️ Partial | ✅ Yes | ✅ Yes | ✅ Yes | ❌ No | ✅ Yes |
| **Rule chaining/fallbacks** | ✅ Native | ❌ Ternary | ⚠️ Via code | ✅ Functions | ⚠️ Via defaults | ✅ Functions | ✅ Functions | ✅ Functions |
| **Regex support** | ✅ Native | ⚠️ Extension | ✅ Native | ✅ Native | ❌ No | ❌ No | ✅ Native | ✅ Native |
| **Hot-reload** | ✅ Built-in | ⚠️ Manual | ✅ Built-in | ✅ Built-in | ⚠️ Manual | ⚠️ Manual | ✅ Built-in | ✅ Built-in |
| **Performance (single eval)** | ~50-500μs | ~1-5μs | ~10-100ns | ~1-10μs | ~10-100μs | ~100μs-1ms | ~1-10μs | ~1-10μs |
| **Memory footprint** | ~5-10MB | ~1-2MB | ~1MB | ~2-3MB | ~5MB | ~3-5MB | ~5-10MB | ~2-3MB |
| **Learning curve** | 🟡 Medium | 🟢 Low | 🟢 Low | 🟢 Low | 🟡 Medium | 🟢 Low | 🟢 Low | 🟢 Low |
| **Industry adoption** | ✅ CNCF Grad | ✅ K8s native | 🟡 Growing | ✅ Google/Bazel | 🟡 Growing | ✅ Google | 🟡 Moderate | 🟢 Small |
| **Testing framework** | ✅ `opa test` | ⚠️ Limited | ⚠️ Go tests | ⚠️ Go tests | ✅ Built-in | ⚠️ Go tests | ⚠️ Go tests | ⚠️ Go tests |
| **Current kubernaut usage** | ✅ 2 engines | ⚠️ CRD only | ❌ None | ❌ None | ❌ None | ❌ None | ❌ None | ❌ None |

### Decision Matrix (Weighted Scoring)

| Criterion | Weight | **Rego** | **Starlark** | **Expr** | CEL |
|-----------|--------|----------|--------------|----------|-----|
| **BR-SP-102 (CustomLabels)** | 20% | 10/10 | 10/10 | 9/10 | 0/10 |
| **Security/Sandbox** | 20% | 9/10 | 10/10 | 6/10 | 3/10 |
| **Structured output** | 15% | 9/10 | 9/10 | 8/10 | 2/10 |
| **Rule composition** | 10% | 9/10 | 8/10 | 6/10 | 4/10 |
| **Hot-reload** | 10% | 9/10 | 9/10 | 9/10 | 4/10 |
| **Performance** | 5% | 7/10 | 8/10 | 10/10 | 9/10 |
| **Existing investment** | 10% | 10/10 | 2/10 | 2/10 | 2/10 |
| **Learning curve** | 5% | 6/10 | 8/10 | 9/10 | 8/10 |
| **Industry adoption** | 5% | 10/10 | 8/10 | 7/10 | 8/10 |

**Weighted Total**:
- **Rego: 9.2/10** ⭐ (Recommended)
- **Starlark: 8.4/10** (Strong Alternative)
- **Expr: 7.1/10** (Performance Option)
- CEL: 3.1/10 (Not Viable)

### BR-Specific Evaluation (Tier 1 Candidates)

| Requirement | **Rego** | **Starlark** | **Expr** | CEL | Winner |
|-------------|----------|--------------|----------|-----|--------|
| **BR-SP-051-053**: Environment Classification | ✅ 95% | ✅ 90% | ⚠️ 80% | ⚠️ 60% | Rego |
| **BR-SP-070-072**: Priority Assignment | ✅ 95% | ✅ 90% | ⚠️ 75% | ❌ 0%* | Rego |
| **BR-SP-080-081**: Confidence Scoring | ✅ 90% | ✅ 90% | ⚠️ 75% | ⚠️ 40% | Tie |
| **BR-SP-102**: CustomLabels (`map[string][]string`) | ✅ 100% | ✅ 95% | ✅ 90% | ❌ 0%** | Rego |
| **BR-SP-104**: Security Wrapper | ✅ 95% | ✅ 95% | ⚠️ 70% | ❌ 20% | Tie |
| **BR-AI-026-028**: Approval Policies | ✅ 95% | ✅ 90% | ⚠️ 75% | ⚠️ 60% | Rego |
| **DD-WORKFLOW-001 v1.9**: Sandbox | ✅ Built-in | ✅ Built-in | ⚠️ Manual | ❌ None | Tie |

*\*BR-SP-070 currently states "Rego-based" - can be changed to "policy-based" since not released*
*\*\*CEL cannot return `map[string][]string` - architectural blocker*

---

## Risk Analysis

### Rego Risks (Recommended)

| Risk ID | Risk | Severity | Likelihood | Mitigation |
|---------|------|----------|------------|------------|
| **REGO-R1** | Policy complexity | 🟢 Low | 30% | `opa test` framework |
| **REGO-R2** | OPA library size (~5-10MB) | 🟢 Low | 100% | Acceptable trade-off |
| **REGO-R3** | Learning curve | 🟢 Low | 20% | Team already trained |
| **REGO-R4** | Performance overhead | 🟢 Low | 10% | <1ms per eval, acceptable |

### Starlark Risks (Alternative B)

| Risk ID | Risk | Severity | Likelihood | Mitigation |
|---------|------|----------|------------|------------|
| **STAR-R1** | Migration effort | 🟡 Medium | 100% | 2-3 weeks planned |
| **STAR-R2** | No policy-specific tooling | 🟡 Medium | 100% | Use Go tests + custom helpers |
| **STAR-R3** | Smaller policy community | 🟢 Low | 80% | Strong Google backing |
| **STAR-R4** | Discards Rego investment | 🟡 Medium | 100% | Acceptable pre-release |

### Expr Risks (Alternative C)

| Risk ID | Risk | Severity | Likelihood | Mitigation |
|---------|------|----------|------------|------------|
| **EXPR-R1** | Weaker sandbox | 🟡 Medium | 100% | Custom environment restrictions |
| **EXPR-R2** | No rule chaining | 🟡 Medium | 100% | Handle in Go wrapper |
| **EXPR-R3** | Less mature for policy | 🟢 Low | 60% | Growing ecosystem |
| **EXPR-R4** | Migration effort | 🟡 Medium | 100% | 2-3 weeks planned |

---

## Quick Decision Guide

```
┌─────────────────────────────────────────────────────────────────┐
│                    POLICY ENGINE SELECTION                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Q1: Do you need map[string][]string output (CustomLabels)?      │
│      │                                                           │
│      ├── NO → Consider CEL for simple validation                 │
│      │                                                           │
│      └── YES → Continue to Q2                                    │
│                                                                  │
│  Q2: Is built-in sandbox critical (customer-defined policies)?   │
│      │                                                           │
│      ├── YES → Rego or Starlark                                  │
│      │   │                                                       │
│      │   ├── Prefer declarative rules? → Rego ⭐                 │
│      │   └── Prefer Python-like? → Starlark                      │
│      │                                                           │
│      └── NO → Expr (fastest, simpler)                            │
│                                                                  │
│  Q3: Already have Rego implementation?                           │
│      │                                                           │
│      ├── YES → Stay with Rego (lowest risk)                      │
│      └── NO → Evaluate Starlark vs Expr                          │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## Tier 1 Technology Examples

### Rego (Recommended)

```rego
package kubernaut.signalprocessing.classification

default environment := "development"

environment := env if {
    env := input.namespace_labels["environment"]
} else := env if {
    env := input.namespace_labels["env"]
} else := env if {
    env := input.namespace_labels["kubernaut.ai/environment"]
}

# Returns structured output
classification := {
    "environment": environment,
    "confidence": confidence,
    "custom_labels": extract_custom_labels(input)
}
```

### Starlark (Alternative B)

```python
# Starlark policy
def classify_environment(labels):
    """Classify environment from labels with fallback chain."""
    env_keys = ["environment", "env", "kubernaut.ai/environment"]

    for key in env_keys:
        if key in labels and labels[key]:
            return {
                "environment": labels[key],
                "confidence": 0.95,
                "source": "label:" + key
            }

    return {
        "environment": "development",
        "confidence": 0.4,
        "source": "default"
    }
```

### Expr (Alternative C)

```go
// Expr program
program := `{
    "environment": labels["environment"] ?? labels["env"] ?? labels["kubernaut.ai/environment"] ?? "development",
    "confidence": labels["environment"] != nil ? 0.95 : 0.4,
    "custom_labels": filter(labels, {# startsWith "constraint."})
}`

env := map[string]interface{}{
    "labels": namespaceLabels,
}
result, _ := expr.Run(program, env)
```

---

## When to Revisit

- If **Kubernetes adopts CEL for policy evaluation** (not just validation)
- If **CEL adds structured output support** (`map[string][]string`)
- If **performance becomes critical** (sub-microsecond requirements)
- If **V2.0 requires centralized policy management** (consider OPA Server)

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.2 | 2025-12-05 | **Promoted to ADR-047** from DD-POLICY-001 (cross-cutting architectural decision) |
| 1.1 | 2025-12-05 | **EXPANDED**: Added 6 additional alternatives (Expr, Starlark, Cue, Jsonnet, Goja, Tengo) |
| 1.0 | 2025-12-05 | Initial CEL vs Rego analysis |

---

**Document Version**: 1.2
**Last Updated**: 2025-12-05
**Status**: 🔄 **Proposed** - Awaiting team decision on Option A/B/C
**Authority**: ⭐ **AUTHORITATIVE** - Single source of truth for policy engine selection



