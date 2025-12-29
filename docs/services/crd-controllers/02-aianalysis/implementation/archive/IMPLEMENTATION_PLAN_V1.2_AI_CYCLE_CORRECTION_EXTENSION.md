# AI Analysis Controller - Implementation Plan v1.2 Extension: AI-Driven Cycle Correction

**Version**: 1.2 - AI-DRIVEN CYCLE CORRECTION (75% Confidence) ⏳ **DEFERRED TO V1.1**
**Date**: 2025-10-17
**Timeline**: +2-3 days (16-24 hours) on top of v1.1
**Status**: ⏳ **DEFERRED - Will implement after V1.0 tested and validated** (75% Confidence)
**Based On**: AIAnalysis v1.1 + ADR-021-AI Enhancement
**Prerequisites**: V1.0 complete, AIAnalysis v1.1 validated, HolmesGPT API correction mode confirmed

---

## ⚠️ **DEFERRAL NOTICE**

**This feature has been postponed to V1.1 release.**

**Deferral Rationale**:
- 🔴 **HolmesGPT API support unknown** - Requires `AnalyzeWithCorrection` endpoint (external dependency)
- 🟡 **Success rate hypothesis untested** - 60-70% correction rate needs empirical validation
- ✅ **V1.0 foundation priority** - Focus on proven architectural risk mitigations first
- ✅ **Q4 2025 timeline** - Avoid scope creep, deliver V1.0 on schedule

**V1.0 Includes Instead**:
- ✅ Dependency cycle **detection** (Kahn's algorithm) - BR-AI-066 to BR-AI-070
- ✅ Manual approval fallback for detected cycles (proven, safe)

**V1.1 Prerequisites** (before implementing this feature):
1. ✅ V1.0 shipped and validated in production
2. ✅ HolmesGPT API extended with correction mode
3. ✅ 100 synthetic cycles tested (success rate >60% validated)
4. ✅ Latency measured (<60s per correction validated)

---

**Parent Plan**: [IMPLEMENTATION_PLAN_V1.0.md](./IMPLEMENTATION_PLAN_V1.0.md)
**Previous Extension**: [IMPLEMENTATION_PLAN_V1.1_HOLMESGPT_RETRY_EXTENSION.md](./IMPLEMENTATION_PLAN_V1.1_HOLMESGPT_RETRY_EXTENSION.md)

---

## 🎯 **Extension Overview**

**Purpose**: Add AI-driven cycle correction capability - query HolmesGPT with feedback when cycle detected

**What's Being Added**:
1. **AI Cycle Correction Loop**: Retry workflow generation with feedback (max 3 attempts)
2. **Feedback Generation**: Structured feedback about cycle for HolmesGPT
3. **Correction Request API**: Extended HolmesGPT client with correction mode

**New Business Requirements**:
- **BR-AI-071**: AIAnalysis SHOULD retry workflow generation with HolmesGPT when dependency cycle detected (max 3 retries)
- **BR-AI-072**: AIAnalysis MUST provide clear feedback to HolmesGPT about dependency cycle nodes
- **BR-AI-073**: AIAnalysis MUST fall back to manual approval after 3 failed correction attempts
- **BR-AI-074**: AIAnalysis MUST track cycle correction attempts in status

**Architectural Decision**:
- [ADR-021-AI: AI-Driven Dependency Cycle Correction (V1.1)](../../../../architecture/decisions/ADR-021-AI-DRIVEN-CYCLE-CORRECTION-ASSESSMENT.md)

**Key Dependency**: **HolmesGPT API must support correction mode** (`AnalyzeWithCorrection` endpoint)

---

## 📋 **What's NOT Changing**

**v1.1 Features (Unchanged)**:
- ✅ HolmesGPT retry with exponential backoff (BR-AI-061 to BR-AI-065)
- ✅ Dependency cycle detection with Kahn's algorithm (BR-AI-066 to BR-AI-070)
- ✅ All existing AIAnalysis capabilities

**v1.1 Fail-Fast Behavior (Enhanced)**:
- Current: Detect cycle → Fail → Manual approval
- Enhanced: Detect cycle → Query HolmesGPT with feedback → Retry (3x) → Manual approval

---

## 🆕 **What's Being Added**

### **New Files** (v1.2):
1. `pkg/aianalysis/correction/feedback_generator.go` - Cycle feedback generation
2. `pkg/aianalysis/correction/correction_coordinator.go` - Correction retry loop
3. `test/unit/aianalysis/correction_test.go` - AI correction tests
4. `test/integration/aianalysis/ai_correction_test.go` - AI correction integration tests

### **Enhanced Files** (v1.2):
1. `pkg/aianalysis/holmesgpt/client.go` - Add `AnalyzeWithCorrection` method
2. `internal/controller/aianalysis/aianalysis_controller.go` - Integrate correction loop
3. `api/aianalysis/v1alpha1/aianalysis_types.go` - Add correction status fields

---

## 📅 2-3 Day Implementation Timeline

| Day | Focus | Hours | Key Deliverables |
|-----|-------|-------|------------------|
| **Day 19** | Feedback Generation + Correction Loop (RED+GREEN) | 8h | Tests + feedback generator + correction coordinator |
| **Day 20** | HolmesGPT Client Enhancement (GREEN+REFACTOR) | 8h | `AnalyzeWithCorrection` endpoint + retry logic |
| **Day 21** | Integration Testing + BR Coverage | 8h | AI correction scenarios, success rate measurement, BR mapping |

**Total**: 24 hours (3 days @ 8h/day)

---

## 🚀 Day 19: Feedback Generation + Correction Loop (8h)

### ANALYSIS Phase (1h)

**Business Context**:
- **BR-AI-071**: Retry workflow generation with HolmesGPT when cycle detected (max 3 retries)
- **BR-AI-072**: Provide clear feedback to HolmesGPT about cycle nodes
- **BR-AI-073**: Fall back to manual approval after 3 failed attempts
- **BR-AI-074**: Track correction attempts in status

**Architectural Context**:
- ADR-021-AI specifies AI-driven correction as V1.1 enhancement
- Hypothesis: 60-70% of cycles can be auto-corrected by LLM
- Latency impact: +30-60s per correction attempt (acceptable for 52+ min MTTR improvement)

**Search existing correction patterns**:
```bash
# Find LLM correction patterns
codebase_search "LLM feedback correction self-healing patterns"
grep -r "correction\|feedback.*model\|retry.*LLM" pkg/ --include="*.go"
```

**Map business requirements to test scenarios**:
1. **BR-AI-071**: Cycle → Feedback → Corrected workflow → Success
2. **BR-AI-072**: Feedback quality (contains cycle nodes, constraint explanation)
3. **BR-AI-073**: 3 failed corrections → Manual approval
4. **BR-AI-074**: Status tracking throughout correction attempts

---

### PLAN Phase (1h)

**TDD Strategy**:
- **Unit tests** (70%+ coverage target):
  - Feedback generation (cycle description, constraint formatting)
  - Correction loop logic (max 3 retries, exhaustion detection)
  - Status update formatting

- **Integration tests** (>50% coverage target):
  - Real HolmesGPT correction requests
  - Cycle → Correction → Success flow
  - 3 failed corrections → Manual approval flow
  - Correction attempt tracking

**Success criteria**:
- Feedback clearly describes cycle and constraints
- HolmesGPT successfully corrects 60-70% of cycles (hypothesis)
- Manual fallback after 3 failed attempts
- Status tracking shows correction progress

---

### DO-RED (3h)

**1. Create correction package structure:**
```bash
mkdir -p pkg/aianalysis/correction
mkdir -p test/unit/aianalysis/correction
```

**2. Write failing unit tests for feedback generation:**

**File**: `test/unit/aianalysis/correction/feedback_generator_test.go`
```go
package correction

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/validation"
)

func TestCycleCorrection(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cycle Correction Suite")
}

var _ = Describe("BR-AI-072: Cycle Feedback Generation", func() {
	var generator *FeedbackGenerator

	BeforeEach(func() {
		generator = NewFeedbackGenerator()
	})

	Context("Feedback Content Quality", func() {
		It("should generate structured feedback with cycle description", func() {
			// BR-AI-072: Clear feedback about cycle
			steps := []validation.Step{
				{ID: "rec-001", Action: "scale_deployment", Dependencies: []string{"rec-002"}},
				{ID: "rec-002", Action: "restart_pods", Dependencies: []string{"rec-001"}},
			}

			cycleError := "dependency cycle detected: steps involved in cycle: [rec-001, rec-002]"
			feedback := generator.GenerateFeedback(steps, cycleError)

			// Verify feedback contains key elements
			Expect(feedback).To(ContainSubstring("dependency cycle detected"))
			Expect(feedback).To(ContainSubstring("rec-001"))
			Expect(feedback).To(ContainSubstring("rec-002"))
			Expect(feedback).To(ContainSubstring("Directed Acyclic Graph (DAG)"))
			Expect(feedback).To(ContainSubstring("Please regenerate"))
		})

		It("should include current dependency list", func() {
			steps := []validation.Step{
				{ID: "rec-001", Dependencies: []string{"rec-003"}},
				{ID: "rec-002", Dependencies: []string{"rec-001"}},
				{ID: "rec-003", Dependencies: []string{"rec-002"}},
			}

			cycleError := "dependency cycle detected: steps involved in cycle: [rec-001, rec-002, rec-003]"
			feedback := generator.GenerateFeedback(steps, cycleError)

			// Verify dependency list included
			Expect(feedback).To(ContainSubstring("Current workflow dependencies"))
			Expect(feedback).To(ContainSubstring("rec-001: depends on [rec-003]"))
			Expect(feedback).To(ContainSubstring("rec-002: depends on [rec-001]"))
			Expect(feedback).To(ContainSubstring("rec-003: depends on [rec-002]"))
		})

		It("should provide example valid patterns", func() {
			steps := []validation.Step{
				{ID: "rec-001", Dependencies: []string{"rec-002"}},
				{ID: "rec-002", Dependencies: []string{"rec-001"}},
			}

			cycleError := "dependency cycle detected"
			feedback := generator.GenerateFeedback(steps, cycleError)

			// Verify examples provided
			Expect(feedback).To(ContainSubstring("Example valid dependency patterns"))
			Expect(feedback).To(ContainSubstring("Linear"))
			Expect(feedback).To(ContainSubstring("Parallel then merge"))
			Expect(feedback).To(ContainSubstring("Fork then parallel"))
		})
	})

	Context("Feedback Constraints", func() {
		It("should specify DAG constraint clearly", func() {
			steps := []validation.Step{
				{ID: "rec-001", Dependencies: []string{"rec-002"}},
				{ID: "rec-002", Dependencies: []string{"rec-001"}},
			}

			feedback := generator.GenerateFeedback(steps, "cycle detected")

			Expect(feedback).To(ContainSubstring("No circular dependencies"))
			Expect(feedback).To(ContainSubstring("step A cannot depend on step B if step B depends on step A"))
		})

		It("should maintain remediation goals", func() {
			steps := []validation.Step{
				{ID: "rec-001", Action: "scale_deployment", Dependencies: []string{"rec-002"}},
				{ID: "rec-002", Action: "restart_pods", Dependencies: []string{"rec-001"}},
			}

			feedback := generator.GenerateFeedback(steps, "cycle detected")

			Expect(feedback).To(ContainSubstring("Maintain the same remediation goals"))
			Expect(feedback).To(ContainSubstring("restructure the dependencies"))
		})
	})
})

var _ = Describe("BR-AI-071 + BR-AI-073: Correction Loop Logic", func() {
	var coordinator *CorrectionCoordinator

	BeforeEach(func() {
		coordinator = NewCorrectionCoordinator(3) // max 3 retries
	})

	Context("Correction Attempt Tracking", func() {
		It("should track attempt count", func() {
			coordinator.RecordAttempt(1, "cycle detected: [rec-001, rec-002]")
			coordinator.RecordAttempt(2, "cycle detected: [rec-003, rec-004]")

			Expect(coordinator.AttemptCount()).To(Equal(2))
		})

		It("should detect exhaustion after 3 attempts", func() {
			coordinator.RecordAttempt(1, "cycle 1")
			coordinator.RecordAttempt(2, "cycle 2")
			coordinator.RecordAttempt(3, "cycle 3")

			Expect(coordinator.IsExhausted()).To(BeTrue())
		})

		It("should NOT be exhausted before 3 attempts", func() {
			coordinator.RecordAttempt(1, "cycle 1")
			coordinator.RecordAttempt(2, "cycle 2")

			Expect(coordinator.IsExhausted()).To(BeFalse())
		})
	})

	Context("BR-AI-074: Status Formatting", func() {
		It("should format status for CRD updates", func() {
			coordinator.RecordAttempt(2, "cycle detected: [rec-001, rec-002]")

			status := coordinator.FormatStatus()
			Expect(status).To(ContainSubstring("attempt 2/3"))
			Expect(status).To(ContainSubstring("cycle detected"))
		})

		It("should indicate exhaustion status", func() {
			coordinator.RecordAttempt(1, "cycle 1")
			coordinator.RecordAttempt(2, "cycle 2")
			coordinator.RecordAttempt(3, "cycle 3")

			status := coordinator.FormatStatus()
			Expect(status).To(ContainSubstring("exhausted"))
			Expect(status).To(ContainSubstring("manual approval required"))
		})
	})
})
```

**3. Write failing integration tests:**

**File**: `test/integration/aianalysis/ai_correction_test.go`
```go
package aianalysis

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/testutil"
)

var _ = Describe("BR-AI-071 to BR-AI-074: AI-Driven Cycle Correction Integration", func() {
	var (
		ctx       context.Context
		namespace string
		aianalysis *aianalysisv1alpha1.AIAnalysis
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = testutil.GenerateNamespace("ai-correction")

		// Create namespace
		ns := testutil.NewNamespace(namespace)
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	})

	AfterEach(func() {
		testutil.CleanupNamespace(ctx, k8sClient, namespace)
	})

	Context("BR-AI-071: Successful Cycle Correction", func() {
		It("should correct cycle on first attempt", func() {
			// Configure mock HolmesGPT to return cycle, then corrected workflow
			testutil.MockHolmesGPT().
				ReturnRecommendations([]testutil.Recommendation{
					{ID: "rec-001", Dependencies: []string{"rec-002"}},
					{ID: "rec-002", Dependencies: []string{"rec-001"}}, // Cycle
				}).
				ThenCorrect([]testutil.Recommendation{
					{ID: "rec-001", Dependencies: []string{}},
					{ID: "rec-002", Dependencies: []string{"rec-001"}}, // Fixed
				})

			aianalysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-correction",
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					AlertName: "HighPodCrashRate",
				},
			}
			Expect(k8sClient.Create(ctx, aianalysis)).To(Succeed())

			// Wait for AIAnalysis to complete with corrected workflow
			Eventually(func() string {
				var ai aianalysisv1alpha1.AIAnalysis
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(aianalysis), &ai)
				return ai.Status.Phase
			}, 2*time.Minute, 5*time.Second).Should(Equal("CreatingWorkflow"))

			// Verify correction attempt tracked (BR-AI-074)
			var ai aianalysisv1alpha1.AIAnalysis
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(aianalysis), &ai)).To(Succeed())
			Expect(ai.Status.CycleCorrectionAttempts).To(Equal(1))
			Expect(ai.Status.DependencyValidationStatus).To(Equal("valid"))
		})

		It("should correct cycle after 2 attempts", func() {
			// Configure mock to fail twice, succeed on third
			testutil.MockHolmesGPT().
				ReturnCycle().
				ThenReturnCycle(). // Attempt 1 - still cycle
				ThenCorrect([]testutil.Recommendation{ // Attempt 2 - fixed
					{ID: "rec-001", Dependencies: []string{}},
					{ID: "rec-002", Dependencies: []string{"rec-001"}},
				})

			aianalysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-correction-retry",
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					AlertName: "DatabaseConnectionPoolExhaustion",
				},
			}
			Expect(k8sClient.Create(ctx, aianalysis)).To(Succeed())

			// Wait for correction
			Eventually(func() string {
				var ai aianalysisv1alpha1.AIAnalysis
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(aianalysis), &ai)
				return ai.Status.Phase
			}, 3*time.Minute, 5*time.Second).Should(Equal("CreatingWorkflow"))

			// Verify 2 correction attempts
			var ai aianalysisv1alpha1.AIAnalysis
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(aianalysis), &ai)).To(Succeed())
			Expect(ai.Status.CycleCorrectionAttempts).To(Equal(2))
		})
	})

	Context("BR-AI-073: Correction Exhaustion → Manual Approval", func() {
		It("should create AIApprovalRequest after 3 failed corrections", func() {
			// Configure mock to always return cycle
			testutil.MockHolmesGPT().AlwaysReturnCycle()

			aianalysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-correction-exhausted",
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					AlertName: "CascadingFailure",
				},
			}
			Expect(k8sClient.Create(ctx, aianalysis)).To(Succeed())

			// Wait for exhaustion → manual approval
			Eventually(func() string {
				var ai aianalysisv1alpha1.AIAnalysis
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(aianalysis), &ai)
				return ai.Status.Phase
			}, 3*time.Minute, 5*time.Second).Should(Equal("Approving"))

			// Verify AIApprovalRequest created
			var ai aianalysisv1alpha1.AIAnalysis
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(aianalysis), &ai)).To(Succeed())
			Expect(ai.Status.ApprovalRequestName).ToNot(BeEmpty())
			Expect(ai.Status.ApprovalContext.Reason).To(ContainSubstring("3 correction attempts failed"))
			Expect(ai.Status.CycleCorrectionAttempts).To(Equal(3))
		})
	})

	Context("BR-AI-072 + BR-AI-074: Feedback Quality and Status Tracking", func() {
		It("should provide feedback with cycle details to HolmesGPT", func() {
			// Configure mock to capture feedback
			feedbackCapture := testutil.MockHolmesGPT().
				ReturnCycle().
				CaptureFeedback()

			aianalysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-feedback",
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					AlertName: "HighPodCrashRate",
				},
			}
			Expect(k8sClient.Create(ctx, aianalysis)).To(Succeed())

			// Wait for correction attempt
			time.Sleep(30 * time.Second)

			// Verify feedback sent to HolmesGPT
			feedback := feedbackCapture.GetLastFeedback()
			Expect(feedback).ToNot(BeEmpty())
			Expect(feedback).To(ContainSubstring("dependency cycle detected"))
			Expect(feedback).To(ContainSubstring("Current workflow dependencies"))
			Expect(feedback).To(ContainSubstring("Directed Acyclic Graph"))
		})

		It("should update status during correction attempts", func() {
			// Configure mock to fail once, succeed on second
			testutil.MockHolmesGPT().
				ReturnCycle().
				ThenCorrect([]testutil.Recommendation{
					{ID: "rec-001", Dependencies: []string{}},
					{ID: "rec-002", Dependencies: []string{"rec-001"}},
				})

			aianalysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-status-tracking",
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					AlertName: "HighPodCrashRate",
				},
			}
			Expect(k8sClient.Create(ctx, aianalysis)).To(Succeed())

			// Wait for first correction attempt
			time.Sleep(15 * time.Second)

			// Check status after first attempt
			var ai aianalysisv1alpha1.AIAnalysis
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(aianalysis), &ai)).To(Succeed())
			Expect(ai.Status.CycleCorrectionAttempts).To(Equal(1))
			Expect(ai.Status.Message).To(ContainSubstring("Dependency cycle detected"))
			Expect(ai.Status.Message).To(ContainSubstring("attempt 1/3"))

			// Wait for completion
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(aianalysis), &ai)
				return ai.Status.Phase
			}, 2*time.Minute, 5*time.Second).Should(Equal("CreatingWorkflow"))
		})
	})
})
```

**4. Run tests (expect failures):**
```bash
# Unit tests should fail (correction logic not implemented)
go test ./test/unit/aianalysis/correction/... -v

# Integration tests should fail
go test ./test/integration/aianalysis/ai_correction_test.go -v
```

---

### DO-GREEN (4h)

**1. Implement feedback generator:**

**File**: `pkg/aianalysis/correction/feedback_generator.go`
```go
package correction

import (
	"fmt"
	"strings"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/validation"
)

// FeedbackGenerator generates structured feedback for HolmesGPT cycle correction
type FeedbackGenerator struct{}

// NewFeedbackGenerator creates a new feedback generator
func NewFeedbackGenerator() *FeedbackGenerator {
	return &FeedbackGenerator{}
}

// GenerateFeedback creates structured feedback for HolmesGPT (BR-AI-072)
func (f *FeedbackGenerator) GenerateFeedback(steps []validation.Step, validationError string) string {
	// Extract cycle nodes from error
	// Build current dependency list
	depList := f.formatDependencyList(steps)

	feedback := fmt.Sprintf(`
The workflow you generated has a dependency cycle and cannot be executed.

Error: %s

Current workflow dependencies:
%s

Please regenerate the workflow with the following constraints:
1. No circular dependencies (step A cannot depend on step B if step B depends on step A, directly or indirectly)
2. All dependencies must form a Directed Acyclic Graph (DAG)
3. Maintain the same remediation goals but restructure the dependencies to be valid
4. If parallel execution is intended, ensure steps have no mutual dependencies

Example valid dependency patterns:
- Linear: step-1 → step-2 → step-3
- Parallel then merge: step-1, step-2 → step-3 (steps 1&2 parallel, then step 3)
- Fork then parallel: step-1 → step-2, step-3 (step 1, then steps 2&3 parallel)

Please provide a corrected workflow that maintains the same remediation goals but with valid dependencies.
`,
		validationError,
		depList,
	)

	return feedback
}

// formatDependencyList creates human-readable dependency list
func (f *FeedbackGenerator) formatDependencyList(steps []validation.Step) string {
	var lines []string
	for _, step := range steps {
		deps := "none"
		if len(step.Dependencies) > 0 {
			deps = strings.Join(step.Dependencies, ", ")
		}
		lines = append(lines, fmt.Sprintf("  - %s: depends on [%s]", step.ID, deps))
	}
	return strings.Join(lines, "\n")
}

// CorrectionCoordinator manages cycle correction retry loop
type CorrectionCoordinator struct {
	maxRetries     int
	attempts       []CorrectionAttempt
	feedbackGen    *FeedbackGenerator
}

// CorrectionAttempt tracks a single correction attempt
type CorrectionAttempt struct {
	AttemptNumber int
	Error         string
	Timestamp     time.Time
}

// NewCorrectionCoordinator creates a new coordinator
func NewCorrectionCoordinator(maxRetries int) *CorrectionCoordinator {
	return &CorrectionCoordinator{
		maxRetries:  maxRetries,
		attempts:    []CorrectionAttempt{},
		feedbackGen: NewFeedbackGenerator(),
	}
}

// RecordAttempt records a correction attempt (BR-AI-074)
func (c *CorrectionCoordinator) RecordAttempt(attemptNumber int, error string) {
	c.attempts = append(c.attempts, CorrectionAttempt{
		AttemptNumber: attemptNumber,
		Error:         error,
		Timestamp:     time.Now(),
	})
}

// AttemptCount returns total correction attempts
func (c *CorrectionCoordinator) AttemptCount() int {
	return len(c.attempts)
}

// IsExhausted checks if correction retries exhausted (BR-AI-073)
func (c *CorrectionCoordinator) IsExhausted() bool {
	return c.AttemptCount() >= c.maxRetries
}

// FormatStatus formats status for CRD updates (BR-AI-074)
func (c *CorrectionCoordinator) FormatStatus() string {
	if c.AttemptCount() == 0 {
		return "No cycle correction attempts"
	}

	if c.IsExhausted() {
		return fmt.Sprintf("Cycle correction exhausted after %d attempts - manual approval required",
			c.AttemptCount())
	}

	lastAttempt := c.attempts[len(c.attempts)-1]
	return fmt.Sprintf("Cycle correction in progress (attempt %d/%d): %s",
		c.AttemptCount(),
		c.maxRetries,
		lastAttempt.Error)
}
```

**2. Enhance HolmesGPT client with correction mode:**

**File**: `pkg/aianalysis/holmesgpt/client.go` (add new method)
```go
package holmesgpt

import (
	// ... existing imports ...
)

// CorrectionRequest represents a correction request to HolmesGPT
type CorrectionRequest struct {
	OriginalRequest  *InvestigationRequest       `json:"original_request"`
	PreviousAnalysis *InvestigationResult        `json:"previous_analysis"`
	Feedback         string                      `json:"feedback"`
	CorrectionMode   bool                        `json:"correction_mode"`
}

// AnalyzeWithCorrection queries HolmesGPT with feedback for correction (BR-AI-071)
func (c *Client) AnalyzeWithCorrection(ctx context.Context, correctionReq *CorrectionRequest) (*InvestigationResult, error) {
	c.logger.Info("Requesting workflow correction from HolmesGPT",
		zap.String("alert", correctionReq.OriginalRequest.AlertName),
		zap.Int("feedbackLength", len(correctionReq.Feedback)))

	// Build request payload
	payload, err := json.Marshal(correctionReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal correction request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(
		ctx,
		"POST",
		fmt.Sprintf("%s/api/v1/investigate/correct", c.baseURL),
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create correction request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Correction-Mode", "true")

	// Execute request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute correction request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HolmesGPT correction API returned %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result InvestigationResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode correction response: %w", err)
	}

	c.logger.Info("HolmesGPT correction complete",
		zap.Float64("confidence", result.Confidence),
		zap.Int("recommendations", len(result.Recommendations)))

	return &result, nil
}
```

**3. Integrate correction loop into AIAnalysis controller:**

**File**: `internal/controller/aianalysis/aianalysis_controller.go` (modify dependency validation)
```go
// ValidateAndCorrectDependencies validates workflow and queries HolmesGPT for corrections if needed (BR-AI-071)
func (r *AIAnalysisReconciler) ValidateAndCorrectDependencies(
	ctx context.Context,
	aiAnalysis *aianalysisv1alpha1.AIAnalysis,
	recommendations []HolmesGPTRecommendation,
) ([]HolmesGPTRecommendation, error) {
	log := log.FromContext(ctx)

	// Create correction coordinator
	coordinator := correction.NewCorrectionCoordinator(3) // max 3 retries

	for attempt := 1; attempt <= 3; attempt++ {
		// Validate dependency graph
		steps := convertToSteps(recommendations)
		validator := validation.NewDependencyValidator()

		if err := validator.ValidateDependencyGraph(steps); err != nil {
			log.Info("Dependency validation failed, requesting correction from HolmesGPT",
				zap.Int("attempt", attempt),
				zap.String("error", err.Error()))

			// Record attempt (BR-AI-074)
			coordinator.RecordAttempt(attempt, err.Error())

			// Check if this is last attempt (BR-AI-073)
			if coordinator.IsExhausted() {
				log.Error(err, "Dependency correction failed after max retries",
					zap.Int("attempts", 3))

				// Update status for manual approval
				aiAnalysis.Status.CycleCorrectionAttempts = coordinator.AttemptCount()
				aiAnalysis.Status.Phase = "Approving"
				aiAnalysis.Status.ApprovalRequired = true
				aiAnalysis.Status.ApprovalContext = &aianalysisv1alpha1.ApprovalContext{
					Reason: fmt.Sprintf("Dependency cycle correction failed after %d attempts", 3),
					ConfidenceScore: aiAnalysis.Status.ConfidenceScore,
					ConfidenceLevel: aiAnalysis.Status.ConfidenceLevel,
					InvestigationSummary: aiAnalysis.Status.InvestigationResult.RootCause,
					EvidenceCollected: []string{
						fmt.Sprintf("Cycle detection: %s", err.Error()),
						fmt.Sprintf("Correction attempts: %d", 3),
						"HolmesGPT unable to generate valid dependency graph",
					},
					RecommendedActions: convertToRecommendedActions(recommendations),
					WhyApprovalRequired: "Dependency cycle correction exhausted - manual workflow design required",
				}

				if updateErr := r.Status().Update(ctx, aiAnalysis); updateErr != nil {
					return nil, updateErr
				}

				return nil, fmt.Errorf("dependency correction failed after %d attempts: %w", 3, err)
			}

			// Generate feedback (BR-AI-072)
			feedbackGen := correction.NewFeedbackGenerator()
			feedback := feedbackGen.GenerateFeedback(steps, err.Error())

			// Update status to reflect correction attempt (BR-AI-074)
			aiAnalysis.Status.CycleCorrectionAttempts = coordinator.AttemptCount()
			aiAnalysis.Status.Message = coordinator.FormatStatus()
			if updateErr := r.Status().Update(ctx, aiAnalysis); updateErr != nil {
				return nil, updateErr
			}

			// Query HolmesGPT for corrected workflow
			correctionReq := &holmesgpt.CorrectionRequest{
				OriginalRequest: &holmesgpt.InvestigationRequest{
					AlertName:    aiAnalysis.Spec.AlertName,
					AlertSummary: aiAnalysis.Spec.AlertSummary,
					Context:      aiAnalysis.Status.ContextData,
					Namespace:    aiAnalysis.Spec.TargetNamespace,
				},
				PreviousAnalysis: aiAnalysis.Status.InvestigationResult,
				Feedback:         feedback,
				CorrectionMode:   true,
			}

			correctedResult, err := r.HolmesGPTClient.AnalyzeWithCorrection(ctx, correctionReq)
			if err != nil {
				log.Error(err, "Failed to request corrected workflow from HolmesGPT",
					zap.Int("attempt", attempt))
				return nil, fmt.Errorf("HolmesGPT correction request failed: %w", err)
			}

			// Update recommendations for next validation attempt
			recommendations = correctedResult.Recommendations
			continue
		}

		// Validation passed
		log.Info("Dependency validation passed",
			zap.Int("totalSteps", len(recommendations)),
			zap.Int("correctionAttempts", attempt-1))

		// Update final status (BR-AI-074)
		aiAnalysis.Status.CycleCorrectionAttempts = coordinator.AttemptCount()
		aiAnalysis.Status.DependencyValidationStatus = "valid"

		return recommendations, nil
	}

	// Should not reach here
	return nil, fmt.Errorf("unexpected: exceeded max retries without returning")
}

// convertToSteps converts HolmesGPT recommendations to validation steps
func convertToSteps(recommendations []HolmesGPTRecommendation) []validation.Step {
	steps := make([]validation.Step, len(recommendations))
	for i, rec := range recommendations {
		steps[i] = validation.Step{
			ID:           rec.ID,
			Action:       rec.Action,
			Dependencies: rec.Dependencies,
		}
	}
	return steps
}
```

**4. Update AIAnalysis CRD with correction fields:**

**File**: `api/aianalysis/v1alpha1/aianalysis_types.go` (add to status)
```go
type AIAnalysisStatus struct {
	// ... existing fields ...

	// Cycle correction fields (BR-AI-074)
	CycleCorrectionAttempts int    `json:"cycleCorrectionAttempts,omitempty"` // Number of correction attempts
	CycleCorrectionSuccess  bool   `json:"cycleCorrectionSuccess,omitempty"`  // Whether correction succeeded

	// ... existing fields ...
}
```

**5. Run tests (expect pass):**
```bash
# Unit tests should pass
go test ./test/unit/aianalysis/correction/... -v

# Integration tests should pass (requires HolmesGPT mock)
go test ./test/integration/aianalysis/ai_correction_test.go -v
```

---

## 🚀 Day 20-21: Implementation Completion

**(Abbreviated for space - follows same pattern as Day 19)**

**Key Deliverables**:
- HolmesGPT client enhancement with correction mode
- Integration testing with real correction scenarios
- BR coverage matrix (BR-AI-071 to BR-AI-074)
- Success rate measurement (hypothesis: 60-70% correction rate)

---

## 📊 Implementation Summary

### What Was Added (v1.2):

**New Packages**:
1. `pkg/aianalysis/correction/` - Feedback generation + correction loop (BR-AI-071 to BR-AI-074)

**Enhanced Files**:
1. `pkg/aianalysis/holmesgpt/client.go` - Add `AnalyzeWithCorrection` method
2. `internal/controller/aianalysis/aianalysis_controller.go` - Integrate correction loop
3. `api/aianalysis/v1alpha1/aianalysis_types.go` - Add correction status fields

**New Tests**:
1. Unit tests: `test/unit/aianalysis/correction/`
2. Integration tests: `test/integration/aianalysis/ai_correction_test.go`

**Timeline Impact**:
- v1.1: 18-19 days (144-152 hours)
- v1.2 extension: +3 days (24 hours)
- **Total: 21-22 days (168-176 hours)**

**Confidence Assessment**: **75%** ⏳ **V1.1 Implementation** (requires HolmesGPT API validation)

**Why 75% (not higher)**:
- ✅ **Implementation straightforward**: 60% confidence (retry loop is simple)
- ⚠️ **HolmesGPT API support unknown**: 40% confidence (needs `AnalyzeWithCorrection` endpoint)
- ✅ **Latency acceptable**: 65% confidence (<60s per retry, worth 52+ min MTTR improvement)
- ⚠️ **Success rate unvalidated**: 50% confidence (hypothesis: 60-70% cycles auto-corrected)

**Prerequisites for V1.2**:
1. ⏳ Validate HolmesGPT API can be extended with correction mode
2. ⏳ Test correction success rate on synthetic cycles (target >60%)
3. ⏳ Measure latency (<60s per retry)

**Value**: Saves 52+ minutes per cycle (manual intervention avoidance)

---

## 🔗 References

**Architecture Decisions**:
- [ADR-021-AI: AI-Driven Dependency Cycle Correction Assessment](../../../../architecture/decisions/ADR-021-AI-DRIVEN-CYCLE-CORRECTION-ASSESSMENT.md)
- [ADR-021: Workflow Dependency Cycle Detection & Validation](../../../../architecture/decisions/ADR-021-workflow-dependency-cycle-detection.md)

**Business Requirements**:
- BR-AI-071 to BR-AI-074: AI-driven cycle correction

**Parent Plan**: [IMPLEMENTATION_PLAN_V1.0.md](./IMPLEMENTATION_PLAN_V1.0.md)
**Previous Extension**: [IMPLEMENTATION_PLAN_V1.1_HOLMESGPT_RETRY_EXTENSION.md](./IMPLEMENTATION_PLAN_V1.1_HOLMESGPT_RETRY_EXTENSION.md)

---

**Document Owner**: AI Analysis Team
**Last Updated**: 2025-10-17
**Status**: ⏳ Assessment Complete - V1.1 Implementation (75% Confidence)
**Note**: This is a **planned extension** for V1.1, not part of V1.0 scope

