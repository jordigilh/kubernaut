# AI Analysis Service - Days 2-4: Phase Handlers

> **Note (ADR-056/ADR-055):** References to `EnrichmentResults.DetectedLabels` and `EnrichmentResults.OwnerChain` in this document are historical. These fields were removed per ADR-056 and ADR-055.

**Parent Document**: [IMPLEMENTATION_PLAN_V1.0.md](../IMPLEMENTATION_PLAN_V1.0.md)
**Duration**: 24 hours (3 days × 8h)
**Phase**: Core Implementation
**Methodology**: APDC-TDD

---

## 📋 **Overview**

| Day | Focus | Key Deliverables |
|-----|-------|------------------|
| **Day 2** | Validating Handler | Input validation, FailedDetections |
| **Day 3** | Investigating Handler | HolmesGPT-API integration |
| **Day 4** | Analyzing + Recommending | Rego policy engine |

---

## 📅 **Day 2: Validating Handler (8h)**

### **Objectives**
- Complete ValidatingHandler implementation
- Input validation with `go-playground/validator`
- FailedDetections enum validation (DD-WORKFLOW-001 v2.1)

### **Hour-by-Hour Breakdown**

#### **Hours 1-3: Enhanced ValidatingHandler (3h)**

**File**: `pkg/aianalysis/phases/validating.go`

```go
package phases

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-playground/validator/v10"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ValidDetectedLabelFields defines allowed values for FailedDetections
// DD-WORKFLOW-001 v2.2: 7 fields (podSecurityLevel removed)
var ValidDetectedLabelFields = map[string]bool{
	"gitOpsManaged":   true,
	"pdbProtected":    true,
	"hpaEnabled":      true,
	"stateful":        true,
	"helmManaged":     true,
	"networkIsolated": true,
	"serviceMesh":     true,
}

// ValidatingHandler handles the Validating phase
type ValidatingHandler struct {
	Client   client.Client
	Log      logr.Logger
	Validate *validator.Validate
}

// NewValidatingHandler creates a new ValidatingHandler
func NewValidatingHandler(c client.Client, log logr.Logger) *ValidatingHandler {
	v := validator.New()
	// Register custom validation for FailedDetections
	v.RegisterValidation("faileddetections", validateFailedDetections)

	return &ValidatingHandler{
		Client:   c,
		Log:      log.WithName("validating-handler"),
		Validate: v,
	}
}

// Handle validates the SignalContext input
func (h *ValidatingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	startTime := time.Now()
	h.Log.Info("Starting validation",
		"name", analysis.Name,
		"fingerprint", analysis.Spec.AnalysisRequest.SignalContext.Fingerprint,
	)

	// Step 1: Validate required fields
	result := h.validateRequiredFields(analysis)
	if !result.Valid {
		return h.handleValidationFailure(ctx, analysis, result.Errors[0])
	}

	// Step 2: Validate FailedDetections enum values
	if err := h.validateFailedDetectionsEnum(analysis); err != nil {
		return h.handleValidationFailure(ctx, analysis, err.Error())
	}

	// Step 3: Validate EnrichmentResults structure
	if err := h.validateEnrichmentResults(analysis); err != nil {
		return h.handleValidationFailure(ctx, analysis, err.Error())
	}

	// Validation successful - transition to Investigating
	h.Log.Info("Validation successful",
		"name", analysis.Name,
		"duration", time.Since(startTime),
	)

	analysis.Status.Phase = aianalysisv1.PhaseInvestigating
	analysis.Status.Message = "Validation complete, starting investigation"
	analysis.Status.LastTransitionTime = &metav1.Time{Time: time.Now()}

	if err := h.Client.Status().Update(ctx, analysis); err != nil {
		h.Log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

// validateRequiredFields checks all required SignalContext fields
func (h *ValidatingHandler) validateRequiredFields(analysis *aianalysisv1.AIAnalysis) ValidationResult {
	result := ValidationResult{Valid: true, Errors: []string{}}
	ctx := analysis.Spec.AnalysisRequest.SignalContext

	// BR-AI-020: Required fields
	checks := []struct {
		field string
		value string
	}{
		{"fingerprint", ctx.Fingerprint},
		{"signalType", ctx.SignalType},
		{"environment", ctx.Environment},
		{"businessPriority", ctx.BusinessPriority},
		{"targetResource.kind", ctx.TargetResource.Kind},
		{"targetResource.name", ctx.TargetResource.Name},
	}

	for _, check := range checks {
		if check.value == "" {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("%s is required", check.field))
		}
	}

	return result
}

// validateFailedDetectionsEnum validates FailedDetections array values
// DD-WORKFLOW-001 v2.1: Only known field names allowed
func (h *ValidatingHandler) validateFailedDetectionsEnum(analysis *aianalysisv1.AIAnalysis) error {
	detectedLabels := analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults.DetectedLabels
	if detectedLabels == nil {
		return nil
	}

	for _, field := range detectedLabels.FailedDetections {
		if !ValidDetectedLabelFields[field] {
			return fmt.Errorf("invalid field in FailedDetections: %s (valid: gitOpsManaged, pdbProtected, hpaEnabled, stateful, helmManaged, networkIsolated, serviceMesh)", field)
		}
	}

	return nil
}

// validateEnrichmentResults validates the EnrichmentResults structure
func (h *ValidatingHandler) validateEnrichmentResults(analysis *aianalysisv1.AIAnalysis) error {
	enrichment := analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults

	// OwnerChain validation
	for i, entry := range enrichment.OwnerChain {
		if entry.Kind == "" || entry.Name == "" {
			return fmt.Errorf("ownerChain[%d]: kind and name are required", i)
		}
	}

	// CustomLabels validation (keys must be non-empty)
	for key := range enrichment.CustomLabels {
		if key == "" {
			return fmt.Errorf("customLabels: empty key not allowed")
		}
	}

	return nil
}

// handleValidationFailure updates status for validation failure
func (h *ValidatingHandler) handleValidationFailure(ctx context.Context, analysis *aianalysisv1.AIAnalysis, message string) (ctrl.Result, error) {
	h.Log.Error(nil, "Validation failed", "error", message)

	analysis.Status.Phase = aianalysisv1.PhaseFailed
	analysis.Status.Message = fmt.Sprintf("Validation failed: %s", message)
	analysis.Status.LastTransitionTime = &metav1.Time{Time: time.Now()}

	if err := h.Client.Status().Update(ctx, analysis); err != nil {
		h.Log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Don't requeue - validation failure is permanent
	return ctrl.Result{}, nil
}

// Name returns the handler name
func (h *ValidatingHandler) Name() string {
	return "validating"
}
```

#### **Hours 4-6: ValidatingHandler Tests (3h)**

**File**: `test/unit/aianalysis/validating_test.go`

```go
package aianalysis

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/phases"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("BR-AI-020, BR-AI-021: Validating Phase Handler", func() {
	var (
		ctx     context.Context
		handler *phases.ValidatingHandler
		client  client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		client = newFakeClient()
		handler = phases.NewValidatingHandler(client, zap.New(zap.UseDevMode(true)))
	})

	// Table-driven tests for FailedDetections validation
	DescribeTable("FailedDetections field validation",
		func(failedDetections []string, expectSuccess bool, expectedError string) {
			analysis := newTestAIAnalysis("test-validation")
			analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults.DetectedLabels = &sharedtypes.DetectedLabels{
				FailedDetections: failedDetections,
			}
			Expect(client.Create(ctx, analysis)).To(Succeed())

			_, err := handler.Handle(ctx, analysis)

			var updated aianalysisv1.AIAnalysis
			Expect(client.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)).To(Succeed())

			if expectSuccess {
				Expect(updated.Status.Phase).To(Equal(aianalysisv1.PhaseInvestigating))
			} else {
				Expect(updated.Status.Phase).To(Equal(aianalysisv1.PhaseFailed))
				Expect(updated.Status.Message).To(ContainSubstring(expectedError))
			}
		},
		Entry("valid field: gitOpsManaged", []string{"gitOpsManaged"}, true, ""),
		Entry("valid field: pdbProtected", []string{"pdbProtected"}, true, ""),
		Entry("valid field: hpaEnabled", []string{"hpaEnabled"}, true, ""),
		Entry("valid field: stateful", []string{"stateful"}, true, ""),
		Entry("valid field: helmManaged", []string{"helmManaged"}, true, ""),
		Entry("valid field: networkIsolated", []string{"networkIsolated"}, true, ""),
		Entry("valid field: serviceMesh", []string{"serviceMesh"}, true, ""),
		Entry("invalid field: podSecurityLevel (removed v2.2)", []string{"podSecurityLevel"}, false, "invalid field"),
		Entry("invalid field: unknownField", []string{"unknownField"}, false, "invalid field"),
		Entry("empty slice: valid", []string{}, true, ""),
		Entry("nil slice: valid", nil, true, ""),
		Entry("multiple valid fields", []string{"gitOpsManaged", "pdbProtected"}, true, ""),
		Entry("mixed valid/invalid", []string{"gitOpsManaged", "invalidField"}, false, "invalid field"),
	)

	Context("Required field validation", func() {
		It("should fail when fingerprint is missing", func() {
			analysis := newTestAIAnalysis("test-no-fingerprint")
			analysis.Spec.AnalysisRequest.SignalContext.Fingerprint = ""
			Expect(client.Create(ctx, analysis)).To(Succeed())

			_, _ = handler.Handle(ctx, analysis)

			var updated aianalysisv1.AIAnalysis
			Expect(client.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(aianalysisv1.PhaseFailed))
			Expect(updated.Status.Message).To(ContainSubstring("fingerprint is required"))
		})

		It("should fail when targetResource is incomplete", func() {
			analysis := newTestAIAnalysis("test-no-target")
			analysis.Spec.AnalysisRequest.SignalContext.TargetResource.Kind = ""
			Expect(client.Create(ctx, analysis)).To(Succeed())

			_, _ = handler.Handle(ctx, analysis)

			var updated aianalysisv1.AIAnalysis
			Expect(client.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(aianalysisv1.PhaseFailed))
		})

		It("should succeed with complete valid input", func() {
			analysis := newTestAIAnalysis("test-valid")
			Expect(client.Create(ctx, analysis)).To(Succeed())

			_, err := handler.Handle(ctx, analysis)
			Expect(err).ToNot(HaveOccurred())

			var updated aianalysisv1.AIAnalysis
			Expect(client.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(aianalysisv1.PhaseInvestigating))
			Expect(updated.Status.Message).To(ContainSubstring("Validation complete"))
		})
	})
})
```

#### **Hours 7-8: Integration & EOD (2h)**

- Wire ValidatingHandler to reconciler
- Run full test suite
- Document Day 2 completion

**Day 2 EOD Checklist:**
- [ ] ValidatingHandler with go-playground/validator
- [ ] FailedDetections enum validation (DD-WORKFLOW-001 v2.1)
- [ ] 15+ unit tests for validation scenarios
- [ ] All tests passing
- [ ] No lint errors

---

## 📅 **Day 3: Investigating Handler (8h)**

### **Objectives**
- HolmesGPT-API client integration
- InvestigatingHandler implementation
- Error handling with retry/backoff

### **Hour-by-Hour Breakdown**

#### **Hours 1-3: HolmesGPT Client Wrapper (3h)**

**File**: `pkg/aianalysis/holmesgpt/client.go`

```go
package holmesgpt

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/clients/holmesgpt"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Config holds HolmesGPT-API client configuration
type Config struct {
	BaseURL        string
	Timeout        time.Duration
	MaxRetries     int
	RetryBaseDelay time.Duration
}

// DefaultConfig returns default configuration
func DefaultConfig() Config {
	return Config{
		BaseURL:        "http://holmesgpt-api:8080",
		Timeout:        60 * time.Second,
		MaxRetries:     3,
		RetryBaseDelay: 1 * time.Second,
	}
}

// Client wraps the generated HolmesGPT-API client
type Client struct {
	client *holmesgpt.Client
	config Config
	log    logr.Logger
}

// NewClient creates a new HolmesGPT client wrapper
func NewClient(cfg Config, log logr.Logger) (*Client, error) {
	httpClient := &http.Client{
		Timeout: cfg.Timeout,
	}

	client, err := holmesgpt.NewClient(cfg.BaseURL, holmesgpt.WithClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create HolmesGPT client: %w", err)
	}

	return &Client{
		client: client,
		config: cfg,
		log:    log.WithName("holmesgpt-client"),
	}, nil
}

// AnalyzeIncident calls the /api/v1/incident/analyze endpoint
func (c *Client) AnalyzeIncident(ctx context.Context, req *IncidentRequest) (*IncidentResponse, error) {
	c.log.Info("Calling HolmesGPT-API",
		"endpoint", "/api/v1/incident/analyze",
		"fingerprint", req.Fingerprint,
	)

	startTime := time.Now()

	// Convert to generated types
	apiReq := c.toAPIRequest(req)

	// Call with retry
	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := c.config.RetryBaseDelay * time.Duration(1<<(attempt-1))
			c.log.Info("Retrying HolmesGPT-API call",
				"attempt", attempt+1,
				"delay", delay,
			)
			time.Sleep(delay)
		}

		resp, err := c.client.PostApiV1IncidentAnalyze(ctx, apiReq)
		if err == nil {
			c.log.Info("HolmesGPT-API call succeeded",
				"duration", time.Since(startTime),
				"attempt", attempt+1,
			)
			return c.fromAPIResponse(resp), nil
		}

		lastErr = err
		c.log.Error(err, "HolmesGPT-API call failed",
			"attempt", attempt+1,
		)

		// Don't retry on non-retriable errors
		if !c.isRetriable(err) {
			break
		}
	}

	return nil, fmt.Errorf("HolmesGPT-API call failed after %d attempts: %w",
		c.config.MaxRetries+1, lastErr)
}

// IncidentRequest is the request for incident analysis
type IncidentRequest struct {
	Fingerprint       string
	SignalType        string
	Environment       string
	BusinessPriority  string
	TargetResource    TargetResource
	EnrichmentResults *sharedtypes.EnrichmentResults
	IsRecoveryAttempt bool
	PreviousExecution *PreviousExecution
}

// TargetResource identifies the affected resource
type TargetResource struct {
	Kind      string
	Name      string
	Namespace string
}

// PreviousExecution contains recovery context
type PreviousExecution struct {
	WorkflowID    string
	FailureReason string
	AttemptNumber int
}

// IncidentResponse is the response from incident analysis
type IncidentResponse struct {
	RootCause          string
	Confidence         float64
	SelectedWorkflow   *SelectedWorkflow
	Warnings           []string
	TargetInOwnerChain bool
}

// SelectedWorkflow contains the recommended workflow
type SelectedWorkflow struct {
	WorkflowID     string
	ContainerImage string
	Parameters     map[string]string
	Rationale      string
}

// toAPIRequest converts to generated API types
func (c *Client) toAPIRequest(req *IncidentRequest) *holmesgpt.IncidentAnalyzeRequest {
	// Implementation depends on generated types
	return &holmesgpt.IncidentAnalyzeRequest{
		// Map fields...
	}
}

// fromAPIResponse converts from generated API types
func (c *Client) fromAPIResponse(resp *holmesgpt.IncidentResponse) *IncidentResponse {
	return &IncidentResponse{
		RootCause:          resp.RootCause,
		Confidence:         resp.Confidence,
		Warnings:           resp.Warnings,
		TargetInOwnerChain: resp.TargetInOwnerChain,
		SelectedWorkflow: &SelectedWorkflow{
			WorkflowID:     resp.SelectedWorkflow.WorkflowId,
			ContainerImage: resp.SelectedWorkflow.ContainerImage,
			Parameters:     resp.SelectedWorkflow.Parameters,
			Rationale:      resp.SelectedWorkflow.Rationale,
		},
	}
}

// isRetriable determines if an error is retriable
func (c *Client) isRetriable(err error) bool {
	// Implement based on error types (timeout, 5xx, etc.)
	return true // Simplified
}
```

#### **Hours 4-6: InvestigatingHandler (3h)**

**File**: `pkg/aianalysis/phases/investigating.go`

```go
package phases

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/holmesgpt"
)

// InvestigatingHandler handles the Investigating phase
type InvestigatingHandler struct {
	Client        client.Client
	Log           logr.Logger
	HolmesGPT     *holmesgpt.Client
	InvestTimeout time.Duration
}

// NewInvestigatingHandler creates a new InvestigatingHandler
func NewInvestigatingHandler(c client.Client, log logr.Logger, hgpt *holmesgpt.Client) *InvestigatingHandler {
	return &InvestigatingHandler{
		Client:        c,
		Log:           log.WithName("investigating-handler"),
		HolmesGPT:     hgpt,
		InvestTimeout: 60 * time.Second,
	}
}

// Handle performs investigation via HolmesGPT-API
func (h *InvestigatingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	startTime := time.Now()
	h.Log.Info("Starting investigation",
		"name", analysis.Name,
		"fingerprint", analysis.Spec.AnalysisRequest.SignalContext.Fingerprint,
		"isRecovery", analysis.Spec.IsRecoveryAttempt,
	)

	// Create investigation context with timeout
	investigateCtx, cancel := context.WithTimeout(ctx, h.InvestTimeout)
	defer cancel()

	// Build request from SignalContext
	req := h.buildRequest(analysis)

	// Call HolmesGPT-API
	resp, err := h.HolmesGPT.AnalyzeIncident(investigateCtx, req)
	if err != nil {
		return h.handleInvestigationError(ctx, analysis, err)
	}

	// Update status with results
	h.updateStatusFromResponse(analysis, resp)

	h.Log.Info("Investigation complete",
		"name", analysis.Name,
		"duration", time.Since(startTime),
		"confidence", resp.Confidence,
		"workflowId", resp.SelectedWorkflow.WorkflowID,
	)

	// Transition to Analyzing phase
	analysis.Status.Phase = aianalysisv1.PhaseAnalyzing
	analysis.Status.Message = "Investigation complete, evaluating approval policy"
	analysis.Status.LastTransitionTime = &metav1.Time{Time: time.Now()}

	if err := h.Client.Status().Update(ctx, analysis); err != nil {
		h.Log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

// buildRequest constructs the HolmesGPT-API request
func (h *InvestigatingHandler) buildRequest(analysis *aianalysisv1.AIAnalysis) *holmesgpt.IncidentRequest {
	ctx := analysis.Spec.AnalysisRequest.SignalContext

	req := &holmesgpt.IncidentRequest{
		Fingerprint:       ctx.Fingerprint,
		SignalType:        ctx.SignalType,
		Environment:       ctx.Environment,
		BusinessPriority:  ctx.BusinessPriority,
		TargetResource: holmesgpt.TargetResource{
			Kind:      ctx.TargetResource.Kind,
			Name:      ctx.TargetResource.Name,
			Namespace: ctx.TargetResource.Namespace,
		},
		EnrichmentResults: &ctx.EnrichmentResults,
		IsRecoveryAttempt: analysis.Spec.IsRecoveryAttempt,
	}

	// Add recovery context if this is a retry
	if analysis.Spec.IsRecoveryAttempt && len(analysis.Spec.PreviousExecutions) > 0 {
		prev := analysis.Spec.PreviousExecutions[len(analysis.Spec.PreviousExecutions)-1]
		req.PreviousExecution = &holmesgpt.PreviousExecution{
			WorkflowID:    prev.WorkflowID,
			FailureReason: prev.FailureReason,
			AttemptNumber: analysis.Spec.RecoveryAttemptNumber,
		}
	}

	return req
}

// updateStatusFromResponse updates status fields from API response
func (h *InvestigatingHandler) updateStatusFromResponse(analysis *aianalysisv1.AIAnalysis, resp *holmesgpt.IncidentResponse) {
	analysis.Status.RootCause = resp.RootCause
	analysis.Status.Confidence = resp.Confidence
	analysis.Status.Warnings = resp.Warnings
	analysis.Status.TargetInOwnerChain = &resp.TargetInOwnerChain

	if resp.SelectedWorkflow != nil {
		analysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowID:     resp.SelectedWorkflow.WorkflowID,
			ContainerImage: resp.SelectedWorkflow.ContainerImage,
			Parameters:     resp.SelectedWorkflow.Parameters,
			Rationale:      resp.SelectedWorkflow.Rationale,
		}
	}
}

// handleInvestigationError handles HolmesGPT-API errors
func (h *InvestigatingHandler) handleInvestigationError(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) (ctrl.Result, error) {
	h.Log.Error(err, "Investigation failed",
		"name", analysis.Name,
	)

	// Check if retriable
	if isTransientError(err) {
		// Requeue with backoff
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Permanent failure
	analysis.Status.Phase = aianalysisv1.PhaseFailed
	analysis.Status.Message = fmt.Sprintf("Investigation failed: %v", err)
	analysis.Status.LastTransitionTime = &metav1.Time{Time: time.Now()}

	if updateErr := h.Client.Status().Update(ctx, analysis); updateErr != nil {
		h.Log.Error(updateErr, "Failed to update status")
		return ctrl.Result{}, updateErr
	}

	return ctrl.Result{}, nil
}

// Name returns the handler name
func (h *InvestigatingHandler) Name() string {
	return "investigating"
}
```

#### **Hours 7-8: Tests & EOD (2h)**

**Day 3 EOD Checklist:**
- [ ] HolmesGPT client wrapper with retry/backoff
- [ ] InvestigatingHandler with error handling
- [ ] Recovery flow support (DD-RECOVERY-002)
- [ ] 10+ unit tests for HolmesGPT integration
- [ ] MockLLMServer integration tested locally

---

## 📅 **Day 4: Rego Policy Engine (8h)**

### **Objectives**
- Rego policy engine for approval decisions
- AnalyzingHandler implementation
- ConfigMap policy loading

### **Hour-by-Hour Breakdown**

#### **Hours 1-4: Rego Engine (4h)**

**File**: `pkg/aianalysis/rego/engine.go`

```go
package rego

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/open-policy-agent/opa/rego"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

// Engine evaluates Rego policies for approval decisions
type Engine struct {
	log           logr.Logger
	policy        string
	policyVersion string
	mu            sync.RWMutex
	prepared      *rego.PreparedEvalQuery
}

// NewEngine creates a new Rego engine
func NewEngine(log logr.Logger) *Engine {
	return &Engine{
		log: log.WithName("rego-engine"),
	}
}

// LoadPolicy loads a Rego policy from string
func (e *Engine) LoadPolicy(policy string, version string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.log.Info("Loading Rego policy",
		"version", version,
		"policyLength", len(policy),
	)

	// Prepare the query
	query, err := rego.New(
		rego.Query("data.approval.decision"),
		rego.Module("approval.rego", policy),
	).PrepareForEval(context.Background())

	if err != nil {
		return fmt.Errorf("failed to prepare Rego policy: %w", err)
	}

	e.policy = policy
	e.policyVersion = version
	e.prepared = &query

	e.log.Info("Rego policy loaded successfully", "version", version)
	return nil
}

// Evaluate evaluates the approval policy
func (e *Engine) Evaluate(ctx context.Context, input *PolicyInput) (*PolicyDecision, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.prepared == nil {
		return nil, fmt.Errorf("no policy loaded")
	}

	startTime := time.Now()
	e.log.V(1).Info("Evaluating Rego policy",
		"environment", input.Environment,
		"confidence", input.Confidence,
	)

	// Create input map
	inputMap := e.toInputMap(input)

	// Evaluate
	results, err := e.prepared.Eval(ctx, rego.EvalInput(inputMap))
	if err != nil {
		return nil, fmt.Errorf("policy evaluation failed: %w", err)
	}

	if len(results) == 0 || len(results[0].Expressions) == 0 {
		// Default to manual approval if no decision
		e.log.Info("No policy decision, defaulting to manual approval")
		return &PolicyDecision{
			Outcome:       "manual_approval",
			Reason:        "No matching policy rule",
			PolicyVersion: e.policyVersion,
		}, nil
	}

	// Parse decision
	decision, err := e.parseDecision(results[0].Expressions[0].Value)
	if err != nil {
		return nil, err
	}

	decision.PolicyVersion = e.policyVersion
	decision.EvaluationTime = time.Since(startTime)

	e.log.Info("Policy evaluation complete",
		"outcome", decision.Outcome,
		"duration", decision.EvaluationTime,
	)

	return decision, nil
}

// PolicyInput is the input for policy evaluation
type PolicyInput struct {
	Environment        string
	BusinessPriority   string
	Confidence         float64
	SignalType         string
	TargetResource     TargetResourceInput
	DetectedLabels     *DetectedLabelsInput
	CustomLabels       map[string][]string
	SelectedWorkflow   *SelectedWorkflowInput
	TargetInOwnerChain bool
	Warnings           []string
	IsRecoveryAttempt  bool
	RecoveryAttempt    int
}

// DetectedLabelsInput is the DetectedLabels in Rego input format
type DetectedLabelsInput struct {
	FailedDetections []string
	GitOpsManaged    bool
	PDBProtected     bool
	HPAEnabled       bool
	Stateful         bool
	HelmManaged      bool
	NetworkIsolated  bool
	ServiceMesh      string
}

// PolicyDecision is the output of policy evaluation
type PolicyDecision struct {
	Outcome        string        // "auto_approve", "manual_approval", "reject"
	Reason         string
	PolicyVersion  string
	EvaluationTime time.Duration
}

// toInputMap converts PolicyInput to map for Rego
func (e *Engine) toInputMap(input *PolicyInput) map[string]interface{} {
	m := map[string]interface{}{
		"environment":           input.Environment,
		"business_priority":     input.BusinessPriority,
		"confidence":            input.Confidence,
		"signal_type":           input.SignalType,
		"target_in_owner_chain": input.TargetInOwnerChain,
		"warnings":              input.Warnings,
		"is_recovery_attempt":   input.IsRecoveryAttempt,
		"recovery_attempt":      input.RecoveryAttempt,
		"target_resource": map[string]interface{}{
			"kind":      input.TargetResource.Kind,
			"name":      input.TargetResource.Name,
			"namespace": input.TargetResource.Namespace,
		},
	}

	if input.DetectedLabels != nil {
		m["detected_labels"] = map[string]interface{}{
			"failed_detections": input.DetectedLabels.FailedDetections,
			"gitops_managed":    input.DetectedLabels.GitOpsManaged,
			"pdb_protected":     input.DetectedLabels.PDBProtected,
			"hpa_enabled":       input.DetectedLabels.HPAEnabled,
			"stateful":          input.DetectedLabels.Stateful,
			"helm_managed":      input.DetectedLabels.HelmManaged,
			"network_isolated":  input.DetectedLabels.NetworkIsolated,
			"service_mesh":      input.DetectedLabels.ServiceMesh,
		}
	}

	if input.CustomLabels != nil {
		m["custom_labels"] = input.CustomLabels
	}

	if input.SelectedWorkflow != nil {
		m["selected_workflow"] = map[string]interface{}{
			"workflow_id":     input.SelectedWorkflow.WorkflowID,
			"container_image": input.SelectedWorkflow.ContainerImage,
		}
	}

	return m
}

// parseDecision parses the Rego output into PolicyDecision
func (e *Engine) parseDecision(value interface{}) (*PolicyDecision, error) {
	decisionMap, ok := value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected decision format: %T", value)
	}

	decision := &PolicyDecision{}

	if outcome, ok := decisionMap["outcome"].(string); ok {
		decision.Outcome = outcome
	}
	if reason, ok := decisionMap["reason"].(string); ok {
		decision.Reason = reason
	}

	return decision, nil
}

// GetPolicyVersion returns the current policy version
func (e *Engine) GetPolicyVersion() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.policyVersion
}
```

#### **Hours 5-7: AnalyzingHandler + RecommendingHandler (3h)**

**File**: `pkg/aianalysis/phases/analyzing.go`

```go
package phases

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
)

// AnalyzingHandler handles the Analyzing phase (Rego policy evaluation)
type AnalyzingHandler struct {
	Client     client.Client
	Log        logr.Logger
	RegoEngine *rego.Engine
}

// NewAnalyzingHandler creates a new AnalyzingHandler
func NewAnalyzingHandler(c client.Client, log logr.Logger, engine *rego.Engine) *AnalyzingHandler {
	return &AnalyzingHandler{
		Client:     c,
		Log:        log.WithName("analyzing-handler"),
		RegoEngine: engine,
	}
}

// Handle evaluates Rego approval policy
func (h *AnalyzingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	h.Log.Info("Starting policy evaluation",
		"name", analysis.Name,
		"confidence", analysis.Status.Confidence,
	)

	// Build policy input
	input := h.buildPolicyInput(analysis)

	// Evaluate policy
	decision, err := h.RegoEngine.Evaluate(ctx, input)
	if err != nil {
		h.Log.Error(err, "Policy evaluation failed, defaulting to manual approval")
		// Graceful degradation: default to manual approval
		decision = &rego.PolicyDecision{
			Outcome: "manual_approval",
			Reason:  "Policy evaluation error: " + err.Error(),
		}
	}

	// Update status with decision
	analysis.Status.ApprovalDecision = decision.Outcome
	analysis.Status.ApprovalReason = decision.Reason
	analysis.Status.PolicyVersion = decision.PolicyVersion

	// Determine next phase based on decision
	switch decision.Outcome {
	case "auto_approve":
		analysis.Status.Phase = aianalysisv1.PhaseRecommending
		analysis.Status.ApprovalRequired = false
	case "manual_approval":
		analysis.Status.Phase = aianalysisv1.PhaseRecommending
		analysis.Status.ApprovalRequired = true
	case "reject":
		analysis.Status.Phase = aianalysisv1.PhaseFailed
		analysis.Status.Message = "Recommendation rejected by policy: " + decision.Reason
	}

	analysis.Status.LastTransitionTime = &metav1.Time{Time: time.Now()}

	if err := h.Client.Status().Update(ctx, analysis); err != nil {
		h.Log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	h.Log.Info("Policy evaluation complete",
		"name", analysis.Name,
		"outcome", decision.Outcome,
		"approvalRequired", analysis.Status.ApprovalRequired,
	)

	return ctrl.Result{Requeue: true}, nil
}

// buildPolicyInput constructs Rego input from AIAnalysis
func (h *AnalyzingHandler) buildPolicyInput(analysis *aianalysisv1.AIAnalysis) *rego.PolicyInput {
	ctx := analysis.Spec.AnalysisRequest.SignalContext

	input := &rego.PolicyInput{
		Environment:        ctx.Environment,
		BusinessPriority:   ctx.BusinessPriority,
		Confidence:         analysis.Status.Confidence,
		SignalType:         ctx.SignalType,
		IsRecoveryAttempt:  analysis.Spec.IsRecoveryAttempt,
		RecoveryAttempt:    analysis.Spec.RecoveryAttemptNumber,
		Warnings:           analysis.Status.Warnings,
		TargetInOwnerChain: analysis.Status.TargetInOwnerChain != nil && *analysis.Status.TargetInOwnerChain,
		TargetResource: rego.TargetResourceInput{
			Kind:      ctx.TargetResource.Kind,
			Name:      ctx.TargetResource.Name,
			Namespace: ctx.TargetResource.Namespace,
		},
		CustomLabels: ctx.EnrichmentResults.CustomLabels,
	}

	// Map DetectedLabels
	if dl := ctx.EnrichmentResults.DetectedLabels; dl != nil {
		input.DetectedLabels = &rego.DetectedLabelsInput{
			FailedDetections: dl.FailedDetections,
			GitOpsManaged:    dl.GitOpsManaged,
			PDBProtected:     dl.PDBProtected,
			HPAEnabled:       dl.HPAEnabled,
			Stateful:         dl.Stateful,
			HelmManaged:      dl.HelmManaged,
			NetworkIsolated:  dl.NetworkIsolated,
			ServiceMesh:      dl.ServiceMesh,
		}
	}

	// Map SelectedWorkflow
	if sw := analysis.Status.SelectedWorkflow; sw != nil {
		input.SelectedWorkflow = &rego.SelectedWorkflowInput{
			WorkflowID:     sw.WorkflowID,
			ContainerImage: sw.ContainerImage,
		}
	}

	return input
}

// Name returns the handler name
func (h *AnalyzingHandler) Name() string {
	return "analyzing"
}
```

#### **Hour 8: EOD & Integration (1h)**

**Day 4 EOD Checklist:**
- [ ] Rego engine with policy loading
- [ ] AnalyzingHandler with policy evaluation
- [ ] RecommendingHandler (status finalization)
- [ ] ConfigMap watcher for hot-reload
- [ ] 15+ Rego policy tests (DescribeTable)
- [ ] Graceful degradation on policy failure

---

## ✅ **Days 2-4 Summary**

| Day | Component | Tests | Coverage |
|-----|-----------|-------|----------|
| Day 2 | ValidatingHandler | 15 | Validation, FailedDetections |
| Day 3 | InvestigatingHandler | 10 | HolmesGPT, retry, recovery |
| Day 4 | Rego Engine + Analyzing | 15 | Policy evaluation, graceful degradation |

**Total**: ~40 unit tests, 4 phase handlers complete

---

## 📚 **References**

| Document | Purpose |
|----------|---------|
| [DD-WORKFLOW-001 v2.2](../../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) | FailedDetections schema |
| [DD-RECOVERY-002](../../../../architecture/decisions/DD-RECOVERY-002-direct-aianalysis-recovery-flow.md) | Recovery flow |
| [REGO_POLICY_EXAMPLES.md](../../REGO_POLICY_EXAMPLES.md) | Policy input schema |
| [AIANALYSIS_TO_HOLMESGPT_API_TEAM.md](../../../../handoff/AIANALYSIS_TO_HOLMESGPT_API_TEAM.md) | API contract |

