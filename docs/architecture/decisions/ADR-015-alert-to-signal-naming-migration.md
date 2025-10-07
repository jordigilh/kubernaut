# ADR-015: Migrate from "Alert" to "Signal" Naming Convention

**Status**: Proposed
**Date**: 2025-10-07
**Deciders**: Development Team, Architecture Review
**Context**: The Kubernaut system is designed to handle multiple signal types (Prometheus alerts, Kubernetes events, AWS CloudWatch alarms, custom webhooks), but core interfaces and types still use "Alert" prefix, creating semantic confusion and blocking evolution.

---

## Problem Statement

### **Architectural Reality**
Kubernaut's architecture explicitly supports multiple signal types:
1. **Prometheus Alerts** (current primary source)
2. **Kubernetes Events** (Gateway Service integration)
3. **AWS CloudWatch Alarms** (RemediationOrchestrator CRD field)
4. **Custom Webhooks** (Gateway Service testing strategy)
5. **Future Signal Sources** (extensible design)

### **Naming Reality**
Core service interfaces and types use "Alert" prefix:
- `AlertService` interface
- `AlertProcessorService` interface
- `AlertContext` struct
- `AlertMetrics` struct
- `ProcessAlert()`, `ValidateAlert()`, `EnrichAlert()` methods
- `GetAlertHistory()`, `GetAlertMetrics()` methods

### **Semantic Mismatch**

When developers encounter this code:
```go
type AlertService interface {
    ProcessAlert(ctx context.Context, alert types.Alert) (*ProcessResult, error)
    ValidateAlert(alert types.Alert) (*ValidationResult, error)
}
```

**They reasonably assume**:
- ✅ This handles Prometheus alerts
- ❌ This does NOT handle Kubernetes events (incorrect!)
- ❌ This does NOT handle AWS CloudWatch alarms (incorrect!)
- ❌ Adding new signal types requires creating parallel services (architectural mistake!)

### **CRD Evolution Already Complete**

The CRD layer has **already migrated** to signal-agnostic naming:
- ✅ `RemediationRequest` CRD uses `SignalType` field
- ✅ `RemediationProcessing` (renamed from `AlertProcessing`)
- ✅ `RemediationOrchestration` (renamed from `AlertRemediation`)

**Evidence from RemediationOrchestrator CRD**:
```yaml
SignalType:   string # "prometheus", "kubernetes-event", "aws-cloudwatch", etc.
SignalSource: string # Adapter name (e.g., "prometheus-adapter")
```

**The service/interface layer lags behind the CRD layer's evolution.**

---

## Decision

**Migrate all active service interfaces, types, and methods from "Alert" prefix to "Signal" prefix** to align naming with the multi-signal architecture documented in CRDs and Gateway Service.

### **Core Changes**

| Current Name | New Name | Justification |
|--------------|----------|---------------|
| `AlertService` | `SignalProcessorService` | Processes ANY signal type, not just alerts |
| `AlertProcessorService` | `SignalProcessorService` | Same as above (duplicate interface) |
| `AlertContext` | `SignalContext` | AI analyzes ALL signal types |
| `AlertMetrics` | `SignalProcessingMetrics` | Metrics must distinguish signal types |
| `AlertHistoryResult` | `SignalHistoryResult` | History includes all signal types |
| `ProcessAlert()` | `ProcessSignal()` | Method handles any signal |
| `ValidateAlert()` | `ValidateSignal()` | Validation works for any signal |
| `EnrichAlert()` | `EnrichSignal()` | Enrichment applies to any signal |
| `GetAlertHistory()` | `GetSignalHistory()` | History query spans all signals |
| `GetAlertMetrics()` | `GetProcessingMetrics()` | Metrics cover all signal processing |

---

## Rationale

### **1. Aligns with Documented Architecture**

**Gateway Service explicitly states** (from `docs/services/stateless/gateway-service/overview.md:25`):
> "Gateway Service is the **single entry point** for all external signals (**Prometheus alerts and Kubernetes events**) into the Kubernaut intelligent remediation system."

Gateway Service already treats alerts and events as interchangeable "signals".

### **2. Prevents Architectural Mistakes**

**Current naming encourages developers to**:
- Create parallel services: `EventService`, `CloudWatchService`, etc.
- Duplicate processing pipelines for each signal type
- Fragment metrics and observability across signal types
- Miss opportunities for unified signal correlation

**Signal-agnostic naming guides developers to**:
- Reuse existing `SignalProcessorService` for all signal types
- Implement polymorphic signal handling with `SignalType` discrimination
- Maintain unified metrics with per-signal-type breakdown
- Enable cross-signal correlation (e.g., alert + event = root cause)

### **3. Enables Signal-Type-Aware Metrics**

**Current `AlertMetrics`**:
```go
type AlertMetrics struct {
    AlertsIngested  int     // Cannot distinguish alerts from events
    ProcessingRate  float64 // alerts per minute (what about events?)
}
```

**New `SignalProcessingMetrics`**:
```go
type SignalProcessingMetrics struct {
    SignalsIngested  map[string]int // {"prometheus-alert": 150, "kubernetes-event": 300}
    ProcessingRate   map[string]float64 // Per signal type
    TotalSignals     int
}
```

**Impact**: Operators can now:
- Monitor per-signal-type SLOs
- Compare alert vs event processing rates
- Detect signal-type-specific issues
- Build Grafana dashboards with signal type dimensions

### **4. Consistency with CRD Layer**

CRDs already use `SignalType` field pattern. Service layer should match.

**Before (inconsistent)**:
- CRD: `RemediationRequest.Spec.SignalType` ✅
- Service: `AlertService.ProcessAlert()` ❌

**After (consistent)**:
- CRD: `RemediationRequest.Spec.SignalType` ✅
- Service: `SignalProcessorService.ProcessSignal()` ✅

---

## Migration Strategy

### **Phase 1: Core Type Definitions** (Week 1)
1. Create `pkg/signal/types.go` with new Signal-prefixed types
2. Add backward-compatible type aliases to prevent breaking changes:
   ```go
   // DEPRECATED: Use SignalProcessorService instead
   // This alias will be removed in V2
   type AlertService = SignalProcessorService

   // DEPRECATED: Use ProcessSignal instead
   func (s *Service) ProcessAlert(ctx context.Context, alert types.Alert) (*ProcessResult, error) {
       return s.ProcessSignal(ctx, signalFromAlert(alert))
   }
   ```
3. Update CRD reconcilers to use new types (internal change, no API break)

### **Phase 2: Interface Migration** (Week 2)
1. Update all service implementations to use `SignalProcessorService`
2. Update all callers to use new method names (`ProcessSignal`, `ValidateSignal`)
3. Run full test suite and fix breaks
4. Update mocks in `pkg/testutil/`

### **Phase 3: Test Migration** (Week 3)
1. Update test fixtures to use Signal types
2. Add signal-type-specific test cases (Kubernetes events, CloudWatch alarms)
3. Fix all failing tests
4. Validate E2E tests with multiple signal types

### **Phase 4: Documentation Cleanup** (Week 4)
1. Update all active documentation to use Signal terminology
2. Add deprecation notice to archive documents
3. Update API specifications and developer guides
4. Update code examples in README files

### **Phase 5: Type Alias Removal** (V2 Release)
1. Remove backward-compatible type aliases
2. Remove deprecated methods (`ProcessAlert`, etc.)
3. Final cleanup and validation
4. Update CHANGELOG with breaking changes

---

## Consequences

### **Positive**

1. **Semantic Clarity** ✅
   - Developers immediately understand the system handles multiple signal types
   - Interface names accurately reflect capabilities
   - Reduces onboarding confusion for new team members

2. **Architectural Consistency** ✅
   - Service layer naming aligns with CRD layer (`SignalType` field)
   - Gateway Service and processing services use consistent terminology
   - Future signal types (AWS CloudWatch, custom webhooks) integrate cleanly

3. **Observability Improvement** ✅
   - Per-signal-type metrics enable precise monitoring
   - Operators can distinguish alert vs event processing rates
   - Grafana dashboards can show signal type breakdown
   - SLOs can be defined per signal type

4. **Prevents Technical Debt** ✅
   - Avoids creating parallel services for each signal type
   - Reduces code duplication across signal processors
   - Enables signal correlation (alert + event → root cause)
   - Scales cleanly to new signal types without architectural changes

5. **Testing Improvements** ✅
   - Test fixtures can be reused across signal types
   - Polymorphic signal handling simplifies test cases
   - Multi-signal scenarios become testable
   - Reduces mock complexity

6. **Business Value** ✅
   - Aligns code with product roadmap (Kubernetes events in V2)
   - Enables faster feature development (new signal types)
   - Reduces maintenance burden (single processing pipeline)

### **Negative**

1. **Migration Effort** ⚠️
   - **100+ files** need updates (interfaces, implementations, tests, docs)
   - **40-57 hours** estimated effort (~1-1.5 weeks)
   - Risk of breaking changes if not carefully managed
   - **Mitigation**: Use backward-compatible type aliases during Phases 1-3

2. **Learning Curve** ⚠️
   - Developers need to learn new interface names
   - Existing code reviews may reference old naming
   - Documentation search results may be stale temporarily
   - **Mitigation**: Clear migration guide, deprecation warnings, updated search indexes

3. **Test Churn** ⚠️
   - All tests referencing Alert* types will break
   - Mock factories need updates
   - Test fixtures need migration
   - **Mitigation**: Comprehensive test coverage validation after each phase

4. **Merge Conflicts** ⚠️
   - Active feature branches will have merge conflicts
   - Coordination needed across team
   - **Mitigation**: Coordinate migration timeline, use feature flags for gradual rollout

---

## Alternatives Considered

### **Alternative 1: Keep "Alert" Naming, Add "Event" Parallel Services**

**Approach**: Create separate `EventService`, `CloudWatchService`, etc. alongside `AlertService`.

**Rejected Because**:
- ❌ Duplicates processing pipeline logic across services
- ❌ Fragments metrics and observability
- ❌ Prevents signal correlation (alert + event analysis)
- ❌ Does not scale to new signal types (requires new service per type)
- ❌ Contradicts CRD layer's signal-agnostic design

### **Alternative 2: Use "Trigger" Instead of "Signal"**

**Approach**: Use `TriggerService`, `ProcessTrigger()` instead of `SignalService`, `ProcessSignal()`.

**Rejected Because**:
- ❌ "Trigger" implies causation, but signals may be informational (Kubernetes events)
- ❌ "Signal" already used in RemediationOrchestrator CRD (`SignalType` field)
- ❌ "Signal" is more generic and industry-standard terminology
- ❌ Would require changing CRD field names (`SignalType` → `TriggerType`)

### **Alternative 3: Use "Event" Instead of "Signal"**

**Approach**: Use `EventService`, `ProcessEvent()` instead of `SignalService`, `ProcessSignal()`.

**Rejected Because**:
- ❌ "Event" conflicts with Kubernetes Events (specific signal type)
- ❌ Prometheus sends "alerts", not "events" (terminology mismatch)
- ❌ "Event" implies temporal occurrence, but alerts can be stateful (firing duration)
- ❌ "Signal" is more neutral and encompasses both alerts and events

### **Alternative 4: Delay Migration Until V2**

**Approach**: Keep Alert naming in V1, migrate in V2 when Kubernetes Events adapter is implemented.

**Rejected Because**:
- ❌ V1 code will have incorrect semantics (AlertService already processes multiple types)
- ❌ Larger V2 migration effort (more code to change)
- ❌ V1 production code will be confusing to operators and developers
- ❌ Delays architectural consistency benefits
- ✅ **Better to migrate now** while codebase is smaller and team has context

---

## Implementation Checklist

### **Phase 1: Core Type Definitions** (Week 1)
- [ ] Create `pkg/signal/types.go` with Signal-prefixed types
- [ ] Create `pkg/signal/signal.go` with Signal struct definition
- [ ] Add backward-compatible type aliases in `pkg/alert/service.go`
- [ ] Update CRD reconcilers to use new types (internal only)
- [ ] Add deprecation comments to old types
- [ ] Validate builds pass with backward compatibility

### **Phase 2: Interface Migration** (Week 2)
- [ ] Update `pkg/remediationprocessing/service.go` to implement `SignalProcessorService`
- [ ] Update all callers in `pkg/workflow/`, `pkg/ai/`, `pkg/platform/`
- [ ] Update mocks in `pkg/testutil/mock_factory.go`
- [ ] Run unit tests and fix breaks
- [ ] Run integration tests and fix breaks

### **Phase 3: Test Migration** (Week 3)
- [ ] Update test fixtures in `test/unit/` to use Signal types
- [ ] Update test fixtures in `test/integration/` to use Signal types
- [ ] Add Kubernetes event test cases
- [ ] Add CloudWatch alarm test cases (mock)
- [ ] Validate test coverage maintained (70%+ unit, 20%+ integration)

### **Phase 4: Documentation Cleanup** (Week 4)
- [ ] Update `docs/services/crd-controllers/01-remediationprocessor/` docs
- [ ] Update `docs/services/crd-controllers/02-aianalysis/` docs
- [ ] Update `docs/services/stateless/gateway-service/` docs
- [ ] Add deprecation notice to archive README
- [ ] Update API specifications
- [ ] Update developer onboarding guides

### **Phase 5: Type Alias Removal** (V2 Release)
- [ ] Remove type aliases from `pkg/alert/service.go`
- [ ] Remove deprecated methods (`ProcessAlert`, etc.)
- [ ] Remove deprecated structs (`AlertContext`, etc.)
- [ ] Final validation of all tests
- [ ] Update CHANGELOG with breaking changes
- [ ] Release notes for V2

---

## Validation Criteria

Migration is **successful** when:
- ✅ All active service interfaces use `Signal` prefix (not `Alert`)
- ✅ All method names use `Signal` terminology (`ProcessSignal`, not `ProcessAlert`)
- ✅ Metrics are signal-type-aware (can distinguish alert vs event processing rates)
- ✅ Documentation consistently uses "signal" for multi-source events
- ✅ Zero production bugs introduced during migration
- ✅ Test coverage maintained at 70%+ (unit) + 20%+ (integration)
- ✅ Build passes on all target platforms (Linux, macOS, Windows)
- ✅ E2E tests validate multiple signal types (alerts + events)

---

## Monitoring and Rollback Plan

### **Monitoring**
- Track build failures during each migration phase
- Monitor test coverage regression (fail if drops below 70% unit)
- Review PR feedback for developer confusion
- Track time spent on migration vs estimate

### **Rollback Plan**
If migration causes critical production issues:
1. **Phase 1-3**: Rollback is safe (backward-compatible aliases in place)
2. **Phase 4**: Documentation rollback via Git revert
3. **Phase 5**: If aliases removed, revert commit and restore aliases

**Rollback Decision Criteria**:
- Critical production bug introduced by migration
- Test coverage drops below 60% unit
- Migration takes >2x estimated effort (80+ hours)

---

## Related Documents

- **Triage Report**: [ALERT_PREFIX_NAMING_TRIAGE.md](../../analysis/ALERT_PREFIX_NAMING_TRIAGE.md) - Detailed risk analysis
- **Gateway Service**: [gateway-service/overview.md](../../services/stateless/gateway-service/overview.md) - Multi-signal ingestion
- **RemediationOrchestrator CRD**: [crd-schema.md](../../services/crd-controllers/05-remediationorchestrator/crd-schema.md) - SignalType field
- **V2 Roadmap**: [V2_DECISION_FINAL.md](../V2_DECISION_FINAL.md) - Kubernetes events feature
- **CRD Schemas**: [CRD_SCHEMAS.md](../CRD_SCHEMAS.md) - Authoritative CRD definitions

---

## Stakeholder Impact

| Stakeholder | Impact | Mitigation |
|-------------|--------|------------|
| **Backend Developers** | Need to learn new interface names | Migration guide, code comments, pair programming sessions |
| **DevOps/SRE** | Metrics dashboard updates needed | Provide Grafana dashboard templates with signal type breakdown |
| **QA/Test Engineers** | Test fixtures need updates | Provide test utility helpers for multi-signal scenarios |
| **Technical Writers** | Documentation updates required | Provide documentation templates and style guide updates |
| **Product Management** | No user-facing impact | Inform of technical debt reduction and V2 readiness |

---

## Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Migration Time** | 40-57 hours (~1-1.5 weeks) | Track actual hours spent |
| **Test Coverage** | Maintain 70%+ unit, 20%+ integration | Run `go test -cover` |
| **Build Success** | 100% on all platforms | CI/CD pipeline status |
| **Production Bugs** | Zero bugs introduced | Monitor production alerts for 2 weeks post-migration |
| **Developer Satisfaction** | Positive feedback from team | Post-migration survey (5-point scale) |
| **Code Duplication** | Reduce by 20% (eliminate parallel services) | Static analysis tool |

---

**Decision Made By**: Development Team
**Approved By**: Architecture Review
**Date Proposed**: 2025-10-07
**Target Start Date**: [To be determined after team review]
**Target Completion Date**: [4 weeks from start date]
**Migration Owner**: [To be assigned]

---

## Appendix: Example Code Migration

### **Before Migration**
```go
// pkg/alert/service.go
type AlertService interface {
    ProcessAlert(ctx context.Context, alert types.Alert) (*ProcessResult, error)
    ValidateAlert(alert types.Alert) (*ValidationResult, error)
}

type alertServiceImpl struct {
    // ...
}

func (s *alertServiceImpl) ProcessAlert(ctx context.Context, alert types.Alert) (*ProcessResult, error) {
    // Process alert
    return &ProcessResult{}, nil
}
```

### **After Migration (Phase 1-3: Backward Compatible)**
```go
// pkg/signal/service.go
type SignalProcessorService interface {
    ProcessSignal(ctx context.Context, signal types.Signal) (*ProcessResult, error)
    ValidateSignal(signal types.Signal) (*ValidationResult, error)
}

type signalProcessorServiceImpl struct {
    // ...
}

func (s *signalProcessorServiceImpl) ProcessSignal(ctx context.Context, signal types.Signal) (*ProcessResult, error) {
    // Process signal (polymorphic based on signal.Type)
    switch signal.Type {
    case "prometheus-alert":
        return s.processPrometheusAlert(ctx, signal)
    case "kubernetes-event":
        return s.processKubernetesEvent(ctx, signal)
    case "cloudwatch-alarm":
        return s.processCloudWatchAlarm(ctx, signal)
    default:
        return nil, fmt.Errorf("unsupported signal type: %s", signal.Type)
    }
}

// pkg/alert/service.go (DEPRECATED - backward compatibility)
// DEPRECATED: Use SignalProcessorService instead
// This alias will be removed in V2
type AlertService = signal.SignalProcessorService
```

### **After Migration (Phase 5: Aliases Removed)**
```go
// pkg/alert/service.go is DELETED
// All code uses pkg/signal/service.go with SignalProcessorService
```
