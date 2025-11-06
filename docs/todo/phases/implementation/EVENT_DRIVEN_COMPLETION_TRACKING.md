# Event-Driven Completion Tracking Implementation

**Document Version**: 1.0
**Date**: September 27, 2025
**Status**: **READY FOR TDD IMPLEMENTATION**
**Business Requirements**: BR-SP-021, BR-PA-009, BR-INS-001, BR-CTX-020, BR-NOTIF-001
**Methodology**: **MANDATORY TDD Workflow with AI Integration**

---

## 🎯 **BUSINESS OBJECTIVE**

Implement event-driven callback system to track remediation completion across Kubernaut microservices, enabling AI learning feedback loops, automated effectiveness assessment, and complete end-to-end workflow visibility.

**Current Architecture Gap**: No completion notification mechanism exists between services after remediation execution.

**Business Impact**:
- AI Analysis Service cannot learn from action outcomes (BR-PA-009)
- Effectiveness Monitor Service cannot trigger assessments (BR-INS-001)
- Notification Service lacks completion triggers (BR-NOTIF-001)
- Alert Processor cannot track lifecycle completion (BR-SP-021)

---

## 🚨 **MANDATORY TDD METHODOLOGY COMPLIANCE**

**CRITICAL**: This implementation MUST follow the complete TDD workflow per [00-core-development-methodology.mdc](mdc:.cursor/rules/00-core-development-methodology.mdc).

### **Phase Sequence - REQUIRED**
1. **Discovery Phase** (5-10 min): Search existing callback patterns in codebase
2. **TDD RED Phase** (15-20 min): Write failing tests for completion callbacks
3. **TDD GREEN Phase** (20-25 min): Minimal implementation + mandatory integration
4. **TDD REFACTOR Phase** (25-35 min): Enhance existing callback methods only

### **AI Integration Requirements - Rule 12 Compliance**
- **MANDATORY**: Use existing `llm.Client` interface from `pkg/ai/llm/`
- **FORBIDDEN**: Creating new AI interfaces for callbacks
- **REQUIRED**: Enhance existing AI client methods for learning feedback

### **Integration Requirements - Rule 01 Compliance**
- **MANDATORY**: All callback components MUST integrate with main applications in `cmd/`
- **VALIDATION**: `grep -r "CallbackHandler\|EventBus" cmd/ --include="*.go"` must show integration
- **FORBIDDEN**: Callback code that exists only in tests

---

## 🔄 **APPROVED ARCHITECTURE DECISIONS**

Based on user input on 5 critical decisions:

1. **Transport**: Hybrid in-process + HTTP callbacks
2. **Registration**: Runtime registration via REST API
3. **Reliability**: Retry with exponential backoff (3 attempts, 100ms-30s)
4. **Payloads**: Configurable detail levels (minimal/standard/detailed/custom)
5. **Migration**: Non-breaking changes with feature flags

---

## 📋 **TDD IMPLEMENTATION ROADMAP**

### **Discovery Phase: Existing Callback Infrastructure**

**Mandatory Search Commands**:
```bash
# Check existing callback patterns
grep -r "callback\|event\|notification" pkg/ --include="*.go" | grep -v "_test.go"

# Check progress reporting (found in codebase)
find pkg/orchestration/execution/ -name "*progress*" -type f

# Check AI health callbacks (found in codebase)
find pkg/ai/orchestration/ -name "*health*" -type f

# Main app integration check
grep -r "callback\|event" cmd/ --include="*.go"
```

**Discovery Findings** (from codebase analysis):
- ✅ Progress reporting exists: `pkg/orchestration/execution/progress_reporting.go`
- ✅ AI health callbacks exist: `pkg/ai/orchestration/health_monitor.go`
- ❌ **GAP**: No workflow/action completion callbacks
- ❌ **GAP**: No cross-service completion notifications

**Decision**: Enhance existing progress reporting patterns vs create new completion system.

---

### **TDD RED Phase: Failing Completion Tests**

**Business Requirement**: BR-SP-021, BR-PA-009, BR-INS-001

**Test Files to Create** (following TDD RED):

#### **File**: `test/unit/events/completion_event_bus_test.go`
```go
package completion_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "context"
    "time"

    "github.com/jordigilh/kubernaut/pkg/events/completion"
    "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("BR-SP-021: Completion Event Bus", func() {
    var (
        eventBus completion.EventBus
        ctx      context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        eventBus = completion.NewEventBus(nil) // Will fail - interface doesn't exist
    })

    It("should publish workflow completion events", func() {
        // BR-SP-021: Track alert states and lifecycle
        event := completion.WorkflowCompleteEvent{
            WorkflowID: "test-workflow-123",
            Status:     "success",
            Duration:   5 * time.Minute,
        }

        err := eventBus.Publish(ctx, event) // Will fail - method doesn't exist
        Expect(err).ToNot(HaveOccurred())
    })

    It("should register HTTP callbacks for cross-service notifications", func() {
        config := completion.HTTPCallbackConfig{
            ServiceName: "ai-analysis-service",
            Endpoint:    "http://ai-service:8082/callbacks/workflow-complete",
        }

        err := eventBus.RegisterHTTP("workflow.complete", config) // Will fail
        Expect(err).ToNot(HaveOccurred())
    })
})
```

#### **File**: `test/unit/ai/callbacks/learning_feedback_test.go`
```go
package callbacks_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "context"

    "github.com/jordigilh/kubernaut/pkg/ai/llm"
    "github.com/jordigilh/kubernaut/pkg/ai/callbacks"
    "github.com/jordigilh/kubernaut/pkg/events/completion"
    "github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

var _ = Describe("BR-PA-009: AI Learning Feedback", func() {
    var (
        llmClient llm.Client // Use EXISTING AI interface
        handler   callbacks.LearningHandler
        ctx       context.Context
    )

    BeforeEach(func() {
        llmClient = mocks.NewMockLLMClient() // Use existing mock
        handler = callbacks.NewLearningHandler(llmClient) // Will fail - doesn't exist
        ctx = context.Background()
    })

    It("should update confidence scoring from workflow outcomes", func() {
        // BR-PA-009: Confidence scoring based on actual outcomes
        result := completion.WorkflowCompletionResult{
            WorkflowID: "test-workflow-456",
            Status:     completion.StatusSuccess,
            Confidence: 0.8,
        }

        err := handler.OnWorkflowComplete(ctx, result) // Will fail - method doesn't exist
        Expect(err).ToNot(HaveOccurred())

        // Verify AI client received learning feedback
        Expect(llmClient.GetConfidenceUpdate()).To(ContainSubstring("workflow-456"))
    })
})
```

#### **File**: `test/integration/completion_callbacks_integration_test.go`
```go
package integration_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/workflow/engine"
    "github.com/jordigilh/kubernaut/pkg/events/completion"
    "github.com/jordigilh/kubernaut/test/integration/shared"
)

var _ = Describe("BR-INS-001: End-to-End Completion Integration", func() {
    var (
        workflowEngine engine.Engine
        eventBus       completion.EventBus
        testEnv        *shared.TestEnvironment
    )

    BeforeEach(func() {
        testEnv = shared.SetupIntegrationTest()
        workflowEngine = testEnv.WorkflowEngine // Use REAL workflow engine
        eventBus = testEnv.EventBus              // Use REAL event bus
    })

    It("should trigger effectiveness assessment after workflow completion", func() {
        // BR-INS-001: Effectiveness assessment after remediation
        workflow := testEnv.CreateTestWorkflow()

        err := workflowEngine.Execute(ctx, workflow) // REAL execution
        Expect(err).ToNot(HaveOccurred())

        // Verify completion event triggered effectiveness assessment
        Eventually(func() bool {
            return testEnv.EffectivenessMonitor.HasAssessment(workflow.ID)
        }).Should(BeTrue())
    })
})
```

**RED Phase Validation**:
```bash
# Tests MUST fail initially
go test ./test/unit/events/... 2>&1 | grep "FAIL"
go test ./test/unit/ai/callbacks/... 2>&1 | grep "FAIL"

# If no failures, RED phase incomplete
echo "❌ Tests not failing - RED phase incomplete"
```

---

### **TDD GREEN Phase: Minimal Implementation + Integration**

**Business Interfaces to Create** (minimal implementation):

#### **File**: `pkg/events/completion/interfaces.go`
```go
package completion

import "context"

// EventBus defines completion event publishing and subscription
type EventBus interface {
    Publish(ctx context.Context, event Event) error
    RegisterHTTP(eventType string, config HTTPCallbackConfig) error
}

// Event represents a completion event
type Event interface {
    Type() string
    Timestamp() time.Time
}

// WorkflowCompleteEvent for workflow completion notifications
type WorkflowCompleteEvent struct {
    WorkflowID string
    Status     string
    Duration   time.Duration
}

func (e WorkflowCompleteEvent) Type() string { return "workflow.complete" }
func (e WorkflowCompleteEvent) Timestamp() time.Time { return time.Now() }

// HTTPCallbackConfig for cross-service HTTP callbacks
type HTTPCallbackConfig struct {
    ServiceName string
    Endpoint    string
    Method      string
    Timeout     time.Duration
}
```

#### **File**: `pkg/events/completion/event_bus.go`
```go
package completion

import (
    "context"
    "fmt"
    "sync"
)

type eventBus struct {
    httpCallbacks map[string][]HTTPCallbackConfig
    mu            sync.RWMutex
}

// NewEventBus creates a minimal event bus implementation
func NewEventBus(config *Config) EventBus {
    return &eventBus{
        httpCallbacks: make(map[string][]HTTPCallbackConfig),
    }
}

func (e *eventBus) Publish(ctx context.Context, event Event) error {
    // Minimal implementation - just log for now
    fmt.Printf("Publishing event: %s\n", event.Type())
    return nil
}

func (e *eventBus) RegisterHTTP(eventType string, config HTTPCallbackConfig) error {
    e.mu.Lock()
    defer e.mu.Unlock()

    e.httpCallbacks[eventType] = append(e.httpCallbacks[eventType], config)
    return nil
}
```

#### **File**: `pkg/ai/callbacks/learning_handler.go`
```go
package callbacks

import (
    "context"

    "github.com/jordigilh/kubernaut/pkg/ai/llm"
    "github.com/jordigilh/kubernaut/pkg/events/completion"
)

// LearningHandler handles AI learning from completion events
type LearningHandler struct {
    llmClient llm.Client // Use EXISTING AI interface
}

func NewLearningHandler(llmClient llm.Client) *LearningHandler {
    return &LearningHandler{llmClient: llmClient}
}

func (h *LearningHandler) OnWorkflowComplete(ctx context.Context, result completion.WorkflowCompletionResult) error {
    // Minimal implementation - enhance existing AI client
    confidenceUpdate := fmt.Sprintf("workflow-%s: %s", result.WorkflowID, result.Status)

    // Use EXISTING AI client method (will enhance in REFACTOR)
    return h.llmClient.UpdateConfidence(ctx, confidenceUpdate)
}
```

**MANDATORY Integration with Main Applications**:

#### **File**: `cmd/kubernaut/main.go` (enhance existing)
```go
// Add to existing main.go
func main() {
    // ... existing code ...

    // Initialize completion event bus
    eventBus := completion.NewEventBus(config.Events)

    // Integrate with workflow engine
    workflowEngine.SetEventBus(eventBus)

    // Integrate AI learning handler
    learningHandler := callbacks.NewLearningHandler(llmClient)
    eventBus.RegisterHandler("workflow.complete", learningHandler.OnWorkflowComplete)

    // ... rest of existing code ...
}
```

**GREEN Phase Validation**:
```bash
# Tests MUST pass with minimal implementation
go test ./test/unit/events/...
go test ./test/unit/ai/callbacks/...

# Main app integration MUST be present
grep -r "NewEventBus\|SetEventBus" cmd/ --include="*.go" | wc -l
# Must return > 0 or GREEN phase incomplete
```

---

### **TDD REFACTOR Phase: Enhance Existing Methods**

**REFACTOR Rules**:
- ✅ **ENHANCE**: Existing event bus methods with sophisticated logic
- ✅ **ENHANCE**: AI learning handler with advanced algorithms
- ❌ **FORBIDDEN**: New types, interfaces, or files
- ❌ **FORBIDDEN**: Structural changes to main app integration

#### **Enhanced Event Bus** (same file, enhanced methods):
```go
func (e *eventBus) Publish(ctx context.Context, event Event) error {
    // REFACTOR: Enhanced implementation with retry logic
    e.mu.RLock()
    callbacks := e.httpCallbacks[event.Type()]
    e.mu.RUnlock()

    for _, callback := range callbacks {
        if err := e.deliverCallback(ctx, event, callback); err != nil {
            // Enhanced retry logic with exponential backoff
            return e.retryWithBackoff(ctx, event, callback, 3)
        }
    }
    return nil
}

func (e *eventBus) retryWithBackoff(ctx context.Context, event Event, callback HTTPCallbackConfig, attempts int) error {
    // Enhanced retry implementation
    for i := 0; i < attempts; i++ {
        backoff := time.Duration(100*math.Pow(2, float64(i))) * time.Millisecond
        time.Sleep(backoff)

        if err := e.deliverCallback(ctx, event, callback); err == nil {
            return nil
        }
    }
    return fmt.Errorf("callback delivery failed after %d attempts", attempts)
}
```

#### **Enhanced AI Learning Handler** (same file, enhanced methods):
```go
func (h *LearningHandler) OnWorkflowComplete(ctx context.Context, result completion.WorkflowCompletionResult) error {
    // REFACTOR: Enhanced AI learning with sophisticated analysis

    // Extract learning patterns from workflow outcome
    learningData := h.extractLearningPatterns(result)

    // Update confidence scoring with detailed analysis
    confidenceUpdate := llm.ConfidenceUpdate{
        WorkflowID:         result.WorkflowID,
        OriginalConfidence: result.OriginalConfidence,
        ActualOutcome:      result.Status == "success",
        Duration:           result.Duration,
        ContextFactors:     learningData.ContextFactors,
    }

    // Enhanced AI client usage
    if err := h.llmClient.UpdateConfidenceScoring(ctx, confidenceUpdate); err != nil {
        return fmt.Errorf("confidence update failed: %w", err)
    }

    // Trigger advanced learning pipeline
    return h.triggerLearningPipeline(ctx, learningData)
}

func (h *LearningHandler) extractLearningPatterns(result completion.WorkflowCompletionResult) *LearningData {
    // Enhanced pattern extraction algorithm
    return &LearningData{
        WorkflowID:      result.WorkflowID,
        SuccessFactors:  h.analyzeSuccessFactors(result),
        FailurePatterns: h.analyzeFailurePatterns(result),
        ContextFactors:  h.extractContextFactors(result),
    }
}
```

**REFACTOR Phase Validation**:
```bash
# No new types allowed
git diff HEAD~1 | grep "^+type.*struct" && echo "❌ New types forbidden in REFACTOR"

# Integration preserved
grep -r "SetEventBus" cmd/ --include="*.go" | wc -l
# Must still return > 0
```

---

## 📊 **CONFIGURATION MANAGEMENT**

Following [06-documentation-standards.mdc](mdc:.cursor/rules/06-documentation-standards.mdc):

#### **File**: `config/completion-events.yaml`
```yaml
# Event-driven completion tracking configuration
completion_events:
  enabled: true
  migration_mode: true  # Feature flag for gradual rollout

  event_bus:
    in_process_timeout: 1s
    http_timeout: 5s
    retry:
      max_attempts: 3
      base_delay: 100ms
      max_delay: 30s
      backoff_factor: 2.0
      jitter_percent: 0.1

  feature_flags:
    workflow_completion_callbacks: true
    action_completion_callbacks: false    # Roll out incrementally
    effectiveness_trigger_callbacks: false
    ai_learning_callbacks: false

  # Service callback configurations
  services:
    ai-analysis-service:
      endpoints:
        - event_types: ["workflow.complete", "workflow.failed"]
          url: "http://ai-service:8082/api/v1/callbacks/workflow-complete"
          payload_type: "standard"
          timeout: 10s

    alert-processor-service:
      endpoints:
        - event_types: ["action.complete", "action.failed"]
          url: "http://alert-processor:8081/api/v1/callbacks/action-complete"
          payload_type: "minimal"
          timeout: 5s
```

---

## 🔧 **VALIDATION COMMANDS**

Following [00-core-development-methodology.mdc](mdc:.cursor/rules/00-core-development-methodology.mdc):

### **TDD Phase Validation Scripts**

#### **File**: `scripts/validate-completion-events-red.sh`
```bash
#!/bin/bash
echo "🔴 COMPLETION EVENTS RED VALIDATION"

# Check tests fail initially
FAILING_TESTS=$(go test ./test/unit/events/... ./test/unit/ai/callbacks/... 2>&1 | grep "FAIL" | wc -l)
if [ "$FAILING_TESTS" -eq 0 ]; then
    echo "❌ VIOLATION: Tests not failing in RED phase"
    exit 1
fi

# Check existing AI interface usage
EXISTING_AI_USAGE=$(grep -r "llm\.Client" test/unit/ai/callbacks/ --include="*.go" | wc -l)
if [ "$EXISTING_AI_USAGE" -eq 0 ]; then
    echo "❌ VIOLATION: Must use existing llm.Client interface"
    exit 1
fi

echo "✅ RED phase validation complete"
```

#### **File**: `scripts/validate-completion-events-green.sh`
```bash
#!/bin/bash
echo "🟢 COMPLETION EVENTS GREEN VALIDATION"

# Check tests pass
go test ./test/unit/events/... ./test/unit/ai/callbacks/...
if [ $? -ne 0 ]; then
    echo "❌ VIOLATION: Tests failing in GREEN phase"
    exit 1
fi

# Check main app integration
MAIN_INTEGRATION=$(grep -r "NewEventBus\|SetEventBus" cmd/ --include="*.go" | wc -l)
if [ "$MAIN_INTEGRATION" -eq 0 ]; then
    echo "❌ VIOLATION: Event bus not integrated in main application"
    exit 1
fi

echo "✅ GREEN phase validation complete"
```

#### **File**: `scripts/validate-completion-events-refactor.sh`
```bash
#!/bin/bash
echo "🔄 COMPLETION EVENTS REFACTOR VALIDATION"

# Check no new types
NEW_TYPES=$(git diff HEAD~1 | grep "^+type.*struct" | wc -l)
if [ "$NEW_TYPES" -gt 0 ]; then
    echo "❌ VIOLATION: New types forbidden in REFACTOR"
    exit 1
fi

# Verify integration preserved
INTEGRATION_PRESERVED=$(grep -r "SetEventBus" cmd/ --include="*.go" | wc -l)
if [ "$INTEGRATION_PRESERVED" -eq 0 ]; then
    echo "❌ VIOLATION: Main app integration lost during REFACTOR"
    exit 1
fi

echo "✅ REFACTOR phase validation complete"
```

---

## 🧪 **TESTING STRATEGY**

Following [03-testing-strategy.mdc](mdc:.cursor/rules/03-testing-strategy.mdc):

### **Unit Tests** (70%+ coverage - REAL business logic):
- ✅ Event bus publishing and subscription logic
- ✅ AI learning feedback algorithms
- ✅ HTTP callback retry mechanisms
- ✅ Payload configuration and filtering
- **Mock ONLY**: External HTTP endpoints, external AI APIs

### **Integration Tests** (20% coverage - Cross-component):
- ✅ Workflow engine → Event bus → AI service callbacks
- ✅ Event bus → Effectiveness monitor triggers
- ✅ End-to-end completion notification flow
- **Real Components**: Event bus, AI learning handlers, workflow engine

### **E2E Tests** (10% coverage - Complete workflows):
- ✅ Full alert → remediation → completion → assessment → notification
- ✅ Multi-service callback delivery validation
- ✅ Learning feedback loop verification

### **Mock Usage Decision Matrix**:
| Component | Unit Tests | Integration Tests | E2E Tests |
|-----------|------------|-------------------|-----------|
| **External HTTP Endpoints** | MOCK | MOCK (CI) / REAL (dev) | REAL |
| **External AI APIs** | MOCK | MOCK (CI) / REAL (dev) | REAL |
| **Event Bus Logic** | REAL | REAL | REAL |
| **AI Learning Handlers** | REAL | REAL | REAL |
| **Workflow Engine** | MOCK | REAL | REAL |

---

## 📈 **METRICS & MONITORING**

Following [06-documentation-standards.mdc](mdc:.cursor/rules/06-documentation-standards.mdc):

### **Prometheus Metrics**:
```yaml
# Callback system metrics
kubernaut_completion_events_published_total{event_type, source_service}
kubernaut_callbacks_delivered_total{event_type, target_service, status}
kubernaut_callbacks_duration_seconds{callback_type, target_service}
kubernaut_callbacks_retries_total{target_service, attempt}

# Learning system metrics
kubernaut_ai_confidence_updates_total{update_type, outcome}
kubernaut_ai_learning_events_total{event_type}
kubernaut_effectiveness_assessments_total{status, interval}
```

### **Alerting Rules**:
```yaml
# High callback failure rate
- alert: HighCallbackFailureRate
  expr: rate(kubernaut_callbacks_delivered_total{status="failed"}[5m]) > 0.1
  for: 2m

# Learning pipeline stuck
- alert: LearningPipelineStuck
  expr: increase(kubernaut_ai_learning_events_total[10m]) == 0
  for: 10m
```

---

## 🚀 **DEPLOYMENT CHECKLIST**

### **Phase 1: TDD GREEN Deployment**
- [ ] Unit tests passing with 70%+ coverage
- [ ] Integration tests validating cross-component behavior
- [ ] Main application integration confirmed
- [ ] Feature flags configured (all disabled initially)
- [ ] Monitoring and alerting deployed

### **Phase 2: TDD REFACTOR Deployment**
- [ ] Enhanced algorithms and retry logic deployed
- [ ] Performance validation completed
- [ ] Advanced AI learning algorithms tested
- [ ] Load testing with callback delivery

### **Phase 3: Gradual Feature Rollout**
- [ ] Enable `workflow_completion_callbacks` feature flag
- [ ] Monitor callback delivery success rate (target: >95%)
- [ ] Enable `ai_learning_callbacks` feature flag
- [ ] Validate learning feedback loop improvements

### **Phase 4: Full Production**
- [ ] Enable all feature flags
- [ ] Complete end-to-end validation
- [ ] Performance optimization based on metrics
- [ ] Documentation and training completed

---

## ✅ **SUCCESS CRITERIA**

### **Functional Requirements**:
- [ ] All completion events delivered to registered services (>95% success rate)
- [ ] AI confidence scoring improved by learning feedback
- [ ] Effectiveness assessment triggered automatically after remediation
- [ ] Complete audit trail of all completion events

### **Performance Requirements**:
- [ ] In-process callbacks: <1 second latency
- [ ] HTTP callbacks: <5 second latency
- [ ] System handles 1000+ completion events per minute
- [ ] Callback failure rate: <1%

### **TDD Compliance Requirements**:
- [ ] 70%+ unit test coverage achieved
- [ ] All business requirements mapped to tests (BR-XXX-XXX)
- [ ] Main application integration validated
- [ ] No orphaned business code (all code used in main apps)

**Business Outcome**: AI learning improves recommendation accuracy by 15%+ through completion feedback loops.

---

**Document Status**: ✅ **READY FOR TDD IMPLEMENTATION**
**Methodology Compliance**: ✅ **FULL TDD WORKFLOW REQUIRED**
**Integration Ready**: ✅ **MAIN APP INTEGRATION MANDATORY**

This implementation plan provides complete context and technical details for implementing event-driven completion tracking following the mandatory TDD methodology and kubernaut project rules.