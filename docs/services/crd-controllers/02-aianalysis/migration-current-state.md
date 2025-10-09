## Current State & Code Reuse Strategy

---

## ⚠️ NAMING DEPRECATION NOTICE

**ALERT PREFIX DEPRECATED**: This document contains type definitions using **"Alert" prefix** (e.g., `AlertContext`), which is **DEPRECATED** and being migrated to **"Signal" prefix** to reflect multi-signal architecture.

**Why Deprecated**: AI analysis should work on ALL signal types (Prometheus alerts, Kubernetes events, AWS CloudWatch alarms), not just alerts.

**Migration Decision**: [ADR-015: Alert to Signal Naming Migration](../../../../architecture/decisions/ADR-015-alert-to-signal-naming-migration.md)

**Current Naming Standard**: `AlertContext` → **`SignalContext`**

**⚠️ When implementing**: Use `SignalContext` instead of `AlertContext`. The Alert-prefixed types shown below are **for migration reference only**.

---

### Existing Code Assessment

**Total Reusable Code**: ~23,468 lines in `pkg/ai/`

#### HolmesGPT Integration (`pkg/ai/holmesgpt/` - 7,956 lines) ✅ HIGH REUSE

**Files**:
- `client.go` (2,184 lines) - HolmesGPT API client
- `ai_orchestration_coordinator.go` (1,460 lines) - Investigation orchestration
- `dynamic_toolset_manager.go` (727 lines) - Toolset management
- `toolset_deployment_client.go` (558 lines) - Toolset deployment
- `toolset_generators.go` (562 lines) - Toolset generation
- `cached_context_manager.go` (424 lines) - Context caching
- `service_integration.go` (370 lines) - Service integration
- `toolset_cache.go` (373 lines) - Toolset caching
- `toolset_template_engine.go` (298 lines) - Template engine

**Reuse Strategy**: ✅ **WRAP & INTEGRATE**
- Wrap existing HolmesGPT client in `pkg/ai/analysis/integration/holmesgpt.go`
- Reuse investigation orchestration logic in investigation phase
- Leverage toolset management for dynamic HolmesGPT capabilities

#### LLM Client (`pkg/ai/llm/` - 2,967 lines) ✅ MEDIUM REUSE

**Files**:
- `client.go` (2,468 lines) - Generic LLM client
- `providers.go` (212 lines) - Provider implementations
- `provider.go` (108 lines) - Provider abstraction
- `context_optimizer.go` (73 lines) - Context optimization

**Reuse Strategy**: ✅ **ADAPT FOR V2**
- V1: HolmesGPT only, minimal LLM client usage
- V2: Multi-provider support will leverage this infrastructure
- Current: May use for response validation and quality checks

#### AI Insights (`pkg/ai/insights/` - 6,295 lines) ✅ MEDIUM REUSE

**Files**:
- `assessor.go` (3,500 lines) - AI assessment logic
- `service.go` (1,453 lines) - Insights service
- `model_training_methods.go` (1,147 lines) - Training methods
- `model_trainer.go` (195 lines) - Model trainer

**Reuse Strategy**: ✅ **SELECTIVE EXTRACTION**
- Extract confidence scoring algorithms
- Reuse assessment patterns for recommendation ranking
- Adapt quality assurance validation logic

#### AI Context (`pkg/ai/context/` - 258 lines) ✅ HIGH REUSE

**Files**:
- `complexity_classifier.go` (258 lines) - Complexity classification

**Reuse Strategy**: ✅ **DIRECT REUSE**
- Use for investigation scope determination
- Integrate into analysis phase for context enrichment

### Migration Decision Matrix

| Component | Existing Code | Action | Effort | Business Value |
|-----------|--------------|--------|--------|---------------|
| **HolmesGPT Client** | `pkg/ai/holmesgpt/client.go` | WRAP | Low | High - Investigation core |
| **Investigation Orchestration** | `pkg/ai/holmesgpt/ai_orchestration_coordinator.go` | REFACTOR | Medium | High - Phase logic |
| **Toolset Management** | `pkg/ai/holmesgpt/dynamic_toolset_manager.go` | INTEGRATE | Low | High - Dynamic capabilities |
| **Context Caching** | `pkg/ai/holmesgpt/cached_context_manager.go` | REUSE | Low | Medium - Performance |
| **Confidence Scoring** | `pkg/ai/insights/assessor.go` | EXTRACT | Medium | High - Recommendation ranking |
| **Response Validation** | `pkg/ai/llm/client.go` | ADAPT | Medium | High - Quality assurance |
| **LLM Multi-Provider** | `pkg/ai/llm/providers.go` | V2 FUTURE | N/A | Low (V1 HolmesGPT only) |

### Recommended Migration Strategy

#### Phase 1: Core Controller Setup (Week 1, Days 1-2)
**Effort**: 2 days

1. Create package structure (`pkg/ai/analysis/`)
2. Define `Analyzer` interface (no "Service" suffix)
3. Implement `AIAnalysisReconciler` (Kubebuilder scaffold)
4. Setup watches for RemediationProcessing CRD
5. Implement finalizers and owner references

**Deliverable**: Basic controller with empty phase handlers

#### Phase 2: Integration Layer (Week 1, Days 3-4)
**Effort**: 2 days

1. Create `pkg/ai/analysis/integration/holmesgpt.go`
   - Wrap existing `pkg/ai/holmesgpt/client.go`
   - Expose `Investigate()`, `Recover()`, `SafetyAnalysis()` methods
2. Create `pkg/ai/analysis/integration/storage.go`
   - Wrap Data Storage Service HTTP client
   - Implement `GetHistoricalPatterns()`, `GetSuccessRate()`

**Deliverable**: Integration layer ready for phase handlers

#### Phase 3: Phase Handlers (Week 1, Day 5)
**Effort**: 1 day

1. Implement `pkg/ai/analysis/phases/investigating.go`
   - Use HolmesGPT integration layer
   - Extract root cause hypotheses (BR-AI-012)
2. Implement `pkg/ai/analysis/phases/analyzing.go`
   - Contextual analysis (BR-AI-001)
   - Response validation (BR-AI-021, BR-AI-023)
3. Implement `pkg/ai/analysis/phases/recommending.go`
   - Recommendation generation (BR-AI-006)
   - Historical success rate incorporation (BR-AI-008)
4. Implement `pkg/ai/analysis/phases/completed.go`
   - WorkflowExecution CRD creation
   - Investigation report generation (BR-AI-014)

**Deliverable**: Complete reconciliation logic

### No Main Application Migration

**Verification**: `cmd/ai-analysis-service/` does **NOT** exist ✅

**Action**: Create new binary entry point

```go
// cmd/aianalysis/main.go
package main

import (
    "flag"
    "os"

    "k8s.io/apimachinery/pkg/runtime"
    clientgoscheme "k8s.io/client-go/kubernetes/scheme"
    ctrl "sigs.k8s.io/controller-runtime"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/v1"
    "github.com/jordigilh/kubernaut/pkg/ai/analysis"
)

var (
    scheme   = runtime.NewScheme()
    setupLog = ctrl.Log.WithName("setup")
)

func init() {
    _ = clientgoscheme.AddToScheme(scheme)
    _ = aianalysisv1.AddToScheme(scheme)
}

func main() {
    var metricsAddr string
    var enableLeaderElection bool

    flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "Metrics endpoint")
    flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election")
    flag.Parse()

    mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
        Scheme:             scheme,
        MetricsBindAddress: metricsAddr,
        LeaderElection:     enableLeaderElection,
        LeaderElectionID:   "aianalysis.kubernaut.io",
    })
    if err != nil {
        setupLog.Error(err, "unable to start manager")
        os.Exit(1)
    }

    if err = (&analysis.AIAnalysisReconciler{
        Client:   mgr.GetClient(),
        Scheme:   mgr.GetScheme(),
        Recorder: mgr.GetEventRecorderFor("aianalysis-controller"),
    }).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create controller", "controller", "AIAnalysis")
        os.Exit(1)
    }

    setupLog.Info("starting manager")
    if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
        setupLog.Error(err, "problem running manager")
        os.Exit(1)
    }
}
```

### ✅ **TYPE SAFETY COMPLIANCE**

This CRD specification was **built with type safety from the start**—no refactoring needed:

| Type | Previous (Anti-Pattern) | Current (Type-Safe) | Benefit |
|------|------------------------|---------------------|---------|
| **AlertContext** | N/A (new service) | Structured type with 10+ fields | Compile-time safety, clear data contract |
| **InvestigationScope** | N/A (new service) | Structured type with resource scope | HolmesGPT targeting precision |
| **AnalysisRequest** | N/A (new service) | Structured type | Type-safe AI investigation request |
| **AnalysisResult** | N/A (new service) | Structured type with recommendations | Type-safe AI response handling |
| **RecommendationList** | N/A (new service) | Structured slice of Recommendation types | No `map[string]interface{}` in recommendations |

**Design Principle**: AIAnalysis service was designed after Remediation Processor type safety remediation, incorporating lessons learned.

**Key Type-Safe Components**:
- ✅ All HolmesGPT request/response types are structured
- ✅ Recommendation workflow types are fully structured
- ✅ No `map[string]interface{}` usage anywhere in CRD spec or status
- ✅ OpenAPI v3 validation enforces all types at API server level

**Type-Safe Patterns in Action**:
```go
// ✅ TYPE SAFE - Structured AlertContext
type AlertContext struct {
    Fingerprint      string `json:"fingerprint"`
    Severity         string `json:"severity"`
    Environment      string `json:"environment"`
    BusinessPriority string `json:"businessPriority"`

    // Resource targeting for HolmesGPT (NOT logs/metrics)
    Namespace    string `json:"namespace"`
    ResourceKind string `json:"resourceKind"`
    ResourceName string `json:"resourceName"`

    // Kubernetes context (small metadata ~8KB)
    KubernetesContext KubernetesContext `json:"kubernetesContext"`
}

// NOT this anti-pattern:
// AlertData map[string]interface{} `json:"alertData"` // ❌ WRONG
```

**Related Documents**:
- `ALERT_PROCESSOR_TYPE_SAFETY_TRIAGE.md` - Original type safety remediation that set the standard
- `02-ai-analysis.md` (this document) - Built type-safe from the start

---

