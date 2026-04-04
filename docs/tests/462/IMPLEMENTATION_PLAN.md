# Implementation Plan: Forward signalAnnotations to KA + Anti-Confirmation-Bias Guardrail

**Issue**: #462
**Test Plan**: [TP-462-v1.0](TEST_PLAN.md)
**Branch**: `development/v1.4`
**Created**: 2026-03-04

---

## Overview

This plan implements two complementary changes across the full investigation pipeline:

- **Part A**: Propagate `signalAnnotations` through 7 layers: CRD type → RO creator → OpenAPI spec → request builder → KA handler → KA types → prompt template
- **Part B**: Add anti-confirmation-bias guardrails to the investigation prompt template

### Data flow (Part A)

```
RR.Spec.SignalAnnotations (map[string]string, exists today)
  → AIAnalysis.Spec.AnalysisRequest.SignalContext.SignalAnnotations (NEW CRD field)
    → IncidentRequest.SignalAnnotations (NEW OpenAPI field)
      → katypes.SignalContext.SignalAnnotations (NEW runtime field)
        → prompt.SignalData.SignalAnnotations (NEW prompt field)
          → incident_investigation.tmpl (NEW template section)
```

### Files to modify

| # | File | Change |
|---|------|--------|
| 1 | `api/aianalysis/v1alpha1/aianalysis_types.go` | Add `SignalAnnotations` to `SignalContextInput` |
| 2 | `pkg/remediationorchestrator/creator/aianalysis.go` | Copy `rr.Spec.SignalAnnotations` in `buildSignalContext` |
| 3 | `api/kubernautagent/openapi.yaml` | Add `signal_annotations` to `IncidentRequest` schema |
| 4 | `pkg/agentclient/` | Regenerate from OpenAPI spec |
| 5 | `pkg/aianalysis/handlers/request_builder.go` | Map annotations from AA CRD to `IncidentRequest` |
| 6 | `internal/kubernautagent/server/handler.go` | Map from `IncidentRequest` to `SignalContext` |
| 7 | `internal/kubernautagent/types/types.go` | Add `SignalAnnotations` to `SignalContext` |
| 8 | `internal/kubernautagent/prompt/builder.go` | Add to `SignalData`, render in template data, sanitize |
| 9 | `internal/kubernautagent/prompt/templates/incident_investigation.tmpl` | Render annotations section + Part B guardrails |

---

## Phase 1: TDD RED — Failing Tests

**Goal**: Write all tests that fail because the annotation pipeline and guardrails don't exist yet.

### Phase 1.1: RO creator unit tests (RED)

**File**: `test/unit/remediationorchestrator/creator/aianalysis_creator_test.go`

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| UT-RO-462-001 | `buildSignalContext(rr, sp)` returns `SignalAnnotations` matching `rr.Spec.SignalAnnotations` | `SignalContextInput` has no `SignalAnnotations` field |
| UT-RO-462-002 | `buildSignalContext(rr, sp)` with nil annotations returns empty/nil without panic | Same: field doesn't exist |

**Preconditions**: Existing test file may need a Ginkgo Describe block for `buildSignalContext`.

### Phase 1.2: Request builder unit tests (RED)

**File**: `test/unit/aianalysis/request_builder_test.go`

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| UT-AA-462-001 | `BuildIncidentRequest(aa)` with signalAnnotations → `req.SignalAnnotations` populated | `IncidentRequest` has no `SignalAnnotations` field |
| UT-AA-462-002 | `BuildIncidentRequest(aa)` without signalAnnotations → `req.SignalAnnotations` nil/empty | Same |

### Phase 1.3: KA handler unit tests (RED)

**File**: `test/unit/kubernautagent/server/handler_test.go`

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| UT-KA-462-001 | `mapIncidentRequestToSignal(req)` maps `signal_annotations` to `SignalContext.SignalAnnotations` | `katypes.SignalContext` has no `SignalAnnotations` field |

### Phase 1.4: Prompt builder unit tests (RED)

**File**: `test/unit/kubernautagent/prompt/builder_test.go`

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| UT-KA-462-002 | `RenderInvestigation()` with annotations → output contains "Signal Annotations" section with description + summary | `SignalData` has no `SignalAnnotations` field; template has no section |
| UT-KA-462-003 | `RenderInvestigation()` without annotations → output does NOT contain "Signal Annotations" section | Same |
| UT-KA-462-004 | `RenderInvestigation()` with partial annotations (description only) → section rendered with only description | Same |
| UT-KA-462-005 | `RenderInvestigation()` with injection in annotation value → `[REDACTED]` in output | Sanitizer doesn't process `SignalAnnotations` |
| UT-KA-462-006 | `RenderInvestigation()` reactive mode → output contains "Exhaustive Verification" and "Contradicting Evidence Search" | Guardrails already present in template (may pass — validates contract) |
| UT-KA-462-007 | `RenderInvestigation()` proactive mode → output contains same guardrails | Same as above |

**Note**: UT-KA-462-006 and UT-KA-462-007 may pass immediately if the guardrails are already in the template (they were added earlier in this branch). These tests lock the behavioral contract.

### Phase 1 Checkpoint

- [ ] All tests compile
- [ ] UT-RO-462-001, UT-RO-462-002: FAIL (field missing on CRD type)
- [ ] UT-AA-462-001, UT-AA-462-002: FAIL (field missing on IncidentRequest)
- [ ] UT-KA-462-001: FAIL (field missing on SignalContext)
- [ ] UT-KA-462-002 through 005: FAIL (field missing on SignalData / template)
- [ ] UT-KA-462-006, UT-KA-462-007: PASS or FAIL (guardrails may already exist)
- [ ] Zero lint errors

---

## Phase 2: TDD GREEN — Minimal Implementation

**Goal**: Make all failing tests pass with minimal code changes.

### Phase 2.1: CRD type change

**File**: `api/aianalysis/v1alpha1/aianalysis_types.go`

Add to `SignalContextInput`:
```go
// Signal annotations from the original alert (e.g., description, summary from AlertManager).
// Untrusted content — sanitized before LLM prompt injection.
// +optional
SignalAnnotations map[string]string `json:"signalAnnotations,omitempty"`
```

Regenerate CRDs: `make generate manifests`

**Verification**: `go build ./api/...`

### Phase 2.2: RO creator wiring

**File**: `pkg/remediationorchestrator/creator/aianalysis.go`

In `buildSignalContext`, add after `EnrichmentResults`:
```go
SignalAnnotations: rr.Spec.SignalAnnotations,
```

**Verification**: UT-RO-462-001, UT-RO-462-002 pass

### Phase 2.3: OpenAPI spec update + code regeneration

**File**: `api/kubernautagent/openapi.yaml`

Add `signal_annotations` to `IncidentRequest` schema:
```yaml
signal_annotations:
  type: object
  additionalProperties:
    type: string
  nullable: true
  description: "Signal annotations from the original alert (e.g., description, summary)"
```

Regenerate: `make generate-agent-client` (or equivalent ogen command)

**Verification**: `go build ./pkg/agentclient/...`

### Phase 2.4: Request builder wiring

**File**: `pkg/aianalysis/handlers/request_builder.go`

In `BuildIncidentRequest`, add after `SignalMode`:
```go
if len(spec.SignalAnnotations) > 0 {
    req.SignalAnnotations.SetTo(agentclient.IncidentRequestSignalAnnotations(spec.SignalAnnotations))
}
```

**Verification**: UT-AA-462-001, UT-AA-462-002 pass

### Phase 2.5: KA types + handler

**File**: `internal/kubernautagent/types/types.go`

Add to `SignalContext`:
```go
SignalAnnotations map[string]string `json:"signal_annotations,omitempty"`
```

**File**: `internal/kubernautagent/server/handler.go`

In `mapIncidentRequestToSignal`, add:
```go
if v, ok := req.SignalAnnotations.Get(); ok {
    sc.SignalAnnotations = map[string]string(v)
}
```

**Verification**: UT-KA-462-001 passes

### Phase 2.6: Prompt builder + template

**File**: `internal/kubernautagent/prompt/builder.go`

Add to `SignalData`:
```go
SignalAnnotations map[string]string
```

Add to `investigationTemplateData`:
```go
SignalAnnotations string
```

In `RenderInvestigation`, add after description handling:
```go
if len(sanitized.SignalAnnotations) > 0 {
    var annots []string
    for k, v := range sanitized.SignalAnnotations {
        annots = append(annots, fmt.Sprintf("- **%s**: %s", sanitizeField(k), sanitizeField(v)))
    }
    sort.Strings(annots)
    data.SignalAnnotations = strings.Join(annots, "\n")
}
```

In `sanitizeSignal`, add:
```go
sanitized := SignalData{
    // ... existing fields ...
    SignalAnnotations: signal.SignalAnnotations, // values sanitized during rendering
}
```

**File**: `internal/kubernautagent/prompt/templates/incident_investigation.tmpl`

Add after the Error Details section:
```
{{ if .SignalAnnotations }}
## Signal Annotations (from Alert Source)

**IMPORTANT**: These annotations were authored by the alert rule creator and provide domain-specific context about the expected root cause. Use them as investigation leads — but verify independently.

{{ .SignalAnnotations }}

{{ end -}}
```

**Verification**: UT-KA-462-002 through UT-KA-462-005 pass

### Phase 2.7: Part B — Anti-confirmation-bias guardrails

The guardrails are already present in the template (lines 113-119 of `incident_investigation.tmpl`):
```
## Investigation Guardrails
1. **Exhaustive Verification**: ...
2. **Contradicting Evidence Search**: ...
```

If they are already present, UT-KA-462-006 and UT-KA-462-007 should already pass. If not, add them.

### Phase 2 Checkpoint

- [ ] All 13 tests pass (11 unit + 2 integration TBD)
- [ ] `go build ./...` succeeds
- [ ] `go vet ./...` clean
- [ ] CRD manifests regenerated and committed
- [ ] OpenAPI client regenerated and committed

---

## Phase 3: TDD REFACTOR — Code Quality

**Goal**: Improve code quality without changing behavior.

### Phase 3.1: Sort annotation keys deterministically

Ensure annotation rendering order is deterministic (sort keys alphabetically) for testability and prompt consistency.

### Phase 3.2: Extract annotation rendering helper

If annotation rendering logic in `RenderInvestigation` exceeds ~10 lines, extract to a `renderAnnotations(annots map[string]string) string` helper.

### Phase 3.3: Template section ordering audit

Verify the signal annotations section is positioned optimally in the prompt:
- After Error Details (so the LLM has the alert context)
- Before Investigation Workflow (so it guides the investigation)

### Phase 3.4: Sanitization coverage audit

Verify that ALL untrusted annotation keys AND values pass through `sanitizeField`. Currently keys are not fields that standard injection would target, but defense-in-depth requires sanitization.

### Phase 3 Checkpoint

- [ ] All 13 tests still pass
- [ ] No new lint errors
- [ ] Annotation rendering is deterministic (sorted keys)
- [ ] `go build ./...` clean

---

## Phase 4: Integration Tests (GREEN)

**Goal**: Validate end-to-end annotation flow with real components.

### Phase 4.1: IT-AA-462-001 — Annotations flow through

**File**: `test/integration/aianalysis/annotation_flow_test.go`

Create a real `AIAnalysis` CR with `signalAnnotations` populated, pass through `RequestBuilder.BuildIncidentRequest`, and assert annotations appear on the resulting `IncidentRequest`.

### Phase 4.2: IT-AA-462-002 — Backward compatibility

Same flow with empty/nil `signalAnnotations`. Assert no errors and the request is valid.

### Phase 4 Checkpoint

- [ ] All 13 tests pass (11 unit + 2 integration)
- [ ] Per-tier coverage >=80%
- [ ] `go build ./...` clean

---

## Phase 5: Due Diligence & Commit

### Phase 5.1: Comprehensive audit

- [ ] All CRD field additions have `+optional` and `omitempty`
- [ ] OpenAPI spec matches CRD schema
- [ ] Deep copy functions regenerated (`zz_generated.deepcopy.go`)
- [ ] No hardcoded strings that should be constants
- [ ] Prompt template renders correctly with 0, 1, and many annotations
- [ ] Injection patterns in annotation values are sanitized
- [ ] Existing tests unaffected

### Phase 5.2: Commit in logical groups

| Commit # | Scope | Files |
|----------|-------|-------|
| 1 | `test(#462): TDD RED — failing tests for signalAnnotations pipeline` | Test files only |
| 2 | `feat(#462): add SignalAnnotations to AIAnalysis CRD and RO wiring` | CRD types, RO creator, generated deepcopy, CRD manifests |
| 3 | `feat(#462): add signal_annotations to KA OpenAPI spec and client` | OpenAPI spec, generated agent client |
| 4 | `feat(#462): wire signalAnnotations through request builder, KA handler, and prompt` | Request builder, handler, types, prompt builder, template |
| 5 | `refactor(#462): deterministic annotation rendering and sanitization hardening` | Prompt builder refactoring |

---

## Estimated Effort

| Phase | Effort |
|-------|--------|
| Phase 1 (RED) | 0.5 day |
| Phase 2 (GREEN) | 1.5 days |
| Phase 3 (REFACTOR) | 0.5 day |
| Phase 4 (Integration) | 0.5 day |
| Phase 5 (Due Diligence) | 0.5 day |
| **Total** | **3.5 days** |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial implementation plan |
