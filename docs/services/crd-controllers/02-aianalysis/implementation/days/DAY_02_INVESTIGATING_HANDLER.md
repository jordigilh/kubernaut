# Day 2: InvestigatingHandler - HolmesGPT-API Integration

**Part of**: AI Analysis Implementation Plan V1.0
**Parent Document**: [IMPLEMENTATION_PLAN_V1.0.md](../../IMPLEMENTATION_PLAN_V1.0.md)
**Duration**: 6-8 hours
**Target Confidence**: 68%
**Version**: v1.4

**Changelog**:
- **v1.4** (2025-12-06): DD-HAPI-002 v1.4 `validation_attempts_history` integration
  - ✅ **CRD Schema**: Added `ValidationAttempt` type and `ValidationAttemptsHistory` field
  - ✅ **Client Update**: `IncidentResponse` includes `ValidationAttemptsHistory []ValidationAttempt`
  - ✅ **Handler Update**: `handleWorkflowResolutionFailure()` parses and stores validation history
  - ✅ **Message Building**: Operator-friendly messages built from validation attempt errors
  - ✅ **Timestamp Parsing**: ISO timestamps converted to `metav1.Time`
  - ✅ **Unit Tests**: 4 new tests for validation history handling
  - 📏 **Reference**: Q18/Q19 resolved in [AIANALYSIS_TO_HOLMESGPT_API_TEAM.md](../../../../handoff/AIANALYSIS_TO_HOLMESGPT_API_TEAM.md)
- **v1.3** (2025-12-06): HAPI `human_review_reason` enum field integration
  - ✅ **New Field**: `HumanReviewReason` enum field added (per HAPI response)
  - ✅ **Direct Mapping**: Enum-to-enum mapping instead of warning parsing
  - ✅ **Backward Compatible**: Fallback to warning parsing if `human_review_reason` is null
  - 📏 **Reference**: [RESPONSE_HAPI_TO_AIANALYSIS_NEEDS_HUMAN_REVIEW.md](../../../../handoff/RESPONSE_HAPI_TO_AIANALYSIS_NEEDS_HUMAN_REVIEW.md)
- **v1.2** (2025-12-06): BR-HAPI-197 `needs_human_review` integration
  - ✅ **Field Added**: `NeedsHumanReview` field in `IncidentResponse`
  - ✅ **Failure Handling**: When `NeedsHumanReview=true`, handler fails with structured reason
  - ✅ **Failure Taxonomy**: `Reason=WorkflowResolutionFailed` + `SubReason` (granular cause)
  - ✅ **Tests Added**: Tests for `needs_human_review` handling and SubReason mapping
  - 📏 **Reference**: [BR-HAPI-197](../../../../requirements/BR-HAPI-197-needs-human-review-field.md)
- **v1.1** (2025-12-05): Architecture clarification alignment
  - ✅ **Response Structure**: Updated `IncidentResponse` to include RCA, SelectedWorkflow, AlternativeWorkflows
  - ✅ **Handler Update**: `processResponse` now captures all v1.5 response fields
  - ✅ **Config Simplified**: Removed `APIKey` (internal service auth)
  - ✅ **Retry Storage**: Changed from `Status.RetryCount` to annotations
  - ✅ **Test Updates**: Added tests for full response capture
  - 📏 **Reference**: [AIANALYSIS_TO_HOLMESGPT_API_TEAM.md](../../../../handoff/AIANALYSIS_TO_HOLMESGPT_API_TEAM.md) Q12-Q13
- **v1.0** (2025-12-04): Initial document

---

## 🔔 Architecture Clarification (Dec 5, 2025)

> **IMPORTANT**: Per HolmesGPT-API team response to Q12-Q13:
>
> The `/api/v1/incident/analyze` endpoint returns **ALL** analysis results in a single call:
> - `root_cause_analysis`: Structured RCA with summary, severity, contributing factors
> - `selected_workflow`: AI-selected workflow for execution (DD-CONTRACT-002)
> - `alternative_workflows`: Other workflows considered (INFORMATIONAL ONLY - NOT for execution)
> - `target_in_owner_chain`: Whether RCA target matches OwnerChain
> - `warnings`: Non-fatal warnings
>
> **Key Principle**: "Alternatives are for CONTEXT, not EXECUTION" - helps operators during approval.

---

## 🎯 Day 2 Objectives

| Objective | Priority | BR Reference |
|-----------|----------|--------------|
| Implement HolmesGPT-API client wrapper | P0 | BR-AI-006 |
| Implement InvestigatingHandler | P0 | BR-AI-007 |
| Handle API responses (targetInOwnerChain, warnings) | P0 | BR-AI-008 |
| Implement retry logic with exponential backoff | P0 | BR-AI-009 |
| Handle permanent errors (401, 400) | P0 | BR-AI-010 |
| Handle `needs_human_review` response (BR-HAPI-197) | P0 | BR-HAPI-197 |

---

## 🔴 TDD RED Phase: Write Failing Tests

### 1. HolmesGPT Client Tests

```go
// test/unit/aianalysis/holmesgpt_client_test.go
package aianalysis

import (
    "context"
    "net/http"
    "net/http/httptest"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/aianalysis/client"
)

var _ = Describe("HolmesGPTClient", func() {
    var (
        mockServer *httptest.Server
        hgClient   *client.HolmesGPTClient
    )

    AfterEach(func() {
        if mockServer != nil {
            mockServer.Close()
        }
    })

    // BR-AI-006: API call construction
    Describe("Investigate", func() {
        Context("with successful response", func() {
            BeforeEach(func() {
                mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                    Expect(r.URL.Path).To(Equal("/api/v1/incident/analyze"))
                    Expect(r.Method).To(Equal(http.MethodPost))

                    w.Header().Set("Content-Type", "application/json")
                    w.WriteHeader(http.StatusOK)
                    w.Write([]byte(`{
                        "analysis": "Root cause: OOM",
                        "target_in_owner_chain": true,
                        "confidence": 0.85,
                        "warnings": []
                    }`))
                }))

                hgClient = client.NewHolmesGPTClient(client.Config{
                    BaseURL: mockServer.URL,
                })
            })

            It("should return valid response - BR-AI-006", func() {
                resp, err := hgClient.Investigate(ctx, &client.IncidentRequest{
                    Context: "Test incident",
                })

                Expect(err).NotTo(HaveOccurred())
                Expect(resp.Analysis).To(Equal("Root cause: OOM"))
                Expect(resp.TargetInOwnerChain).To(BeTrue())
                Expect(resp.Confidence).To(BeNumerically("~", 0.85, 0.01))
            })
        })

        // BR-AI-009: Transient error handling
        Context("with 503 Service Unavailable", func() {
            BeforeEach(func() {
                mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                    w.WriteHeader(http.StatusServiceUnavailable)
                }))
                hgClient = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
            })

            It("should return transient error", func() {
                _, err := hgClient.Investigate(ctx, &client.IncidentRequest{})

                Expect(err).To(HaveOccurred())
                var apiErr *client.APIError
                Expect(errors.As(err, &apiErr)).To(BeTrue())
                Expect(apiErr.IsTransient()).To(BeTrue())
            })
        })

        // BR-AI-010: Permanent error handling
        Context("with 401 Unauthorized", func() {
            BeforeEach(func() {
                mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                    w.WriteHeader(http.StatusUnauthorized)
                }))
                hgClient = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
            })

            It("should return permanent error", func() {
                _, err := hgClient.Investigate(ctx, &client.IncidentRequest{})

                Expect(err).To(HaveOccurred())
                var apiErr *client.APIError
                Expect(errors.As(err, &apiErr)).To(BeTrue())
                Expect(apiErr.IsTransient()).To(BeFalse())
            })
        })
    })
})
```

### 2. InvestigatingHandler Tests

```go
// test/unit/aianalysis/investigating_handler_test.go
package aianalysis

import (
    "context"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    ctrl "sigs.k8s.io/controller-runtime"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/internal/controller/aianalysis"
    "github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
    "github.com/jordigilh/kubernaut/pkg/testutil"
)

// BR-AI-007: InvestigatingHandler tests
var _ = Describe("InvestigatingHandler", func() {
    var (
        handler    *handlers.InvestigatingHandler
        mockClient *testutil.MockHolmesGPTClient
        ctx        context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        mockClient = testutil.NewMockHolmesGPTClient()
        handler = handlers.NewInvestigatingHandler(mockClient, ctrl.Log.WithName("test"))
    })

    // Helper to create valid AIAnalysis
    createTestAnalysis := func() *aianalysisv1.AIAnalysis {
        return &aianalysisv1.AIAnalysis{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "test-analysis",
                Namespace: "default",
            },
            Spec: aianalysisv1.AIAnalysisSpec{
                RemediationRequestRef: corev1.ObjectReference{
                    Kind:      "RemediationRequest",
                    Name:      "test-rr",
                    Namespace: "default",
                },
                RemediationID: "test-remediation-001",
                AnalysisRequest: aianalysisv1.AnalysisRequest{
                    SignalContext: aianalysisv1.SignalContextInput{
                        Fingerprint:      "test-fingerprint",
                        Severity:         "warning",
                        SignalType:       "OOMKilled",
                        Environment:      "production",
                        BusinessPriority: "P0",
                        TargetResource: aianalysisv1.TargetResource{
                            Kind:      "Pod",
                            Name:      "test-pod",
                            Namespace: "default",
                        },
                    },
                    AnalysisTypes: []string{"investigation"},
                },
            },
            Status: aianalysisv1.AIAnalysisStatus{
                Phase: aianalysis.PhaseInvestigating,
            },
        }
    }

    Describe("Handle", func() {
        // BR-AI-007: Process HolmesGPT response
        Context("with successful API response", func() {
            BeforeEach(func() {
                mockClient.WithSuccessResponse(
                    "Root cause identified: OOM",
                    0.9,
                    true,
                    []string{},
                )
            })

            It("should transition to Analyzing phase", func() {
                analysis := createTestAnalysis()

                result, err := handler.Handle(ctx, analysis)

                Expect(err).NotTo(HaveOccurred())
                Expect(result.Requeue).To(BeTrue())
                Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseAnalyzing))
            })

            It("should capture targetInOwnerChain in status", func() {
                analysis := createTestAnalysis()

                _, err := handler.Handle(ctx, analysis)

                Expect(err).NotTo(HaveOccurred())
                Expect(analysis.Status.TargetInOwnerChain).NotTo(BeNil())
                Expect(*analysis.Status.TargetInOwnerChain).To(BeTrue())
            })
        })

        // BR-AI-008: Handle warnings
        Context("with warnings in response", func() {
            BeforeEach(func() {
                mockClient.WithSuccessResponse(
                    "Analysis with warnings",
                    0.7,
                    false,
                    []string{"High memory pressure", "Node scheduling delayed"},
                )
            })

            It("should capture warnings in status", func() {
                analysis := createTestAnalysis()

                _, err := handler.Handle(ctx, analysis)

                Expect(err).NotTo(HaveOccurred())
                Expect(analysis.Status.Warnings).To(HaveLen(2))
                Expect(analysis.Status.Warnings).To(ContainElement("High memory pressure"))
            })
        })

        // BR-HAPI-197: Handle needs_human_review response
        Context("when response has needs_human_review=true", func() {
            // Preferred: Use human_review_reason enum (Dec 6, 2025)
            DescribeTable("should map human_review_reason enum to SubReason",
                func(humanReviewReason string, expectedSubReason string) {
                    mockClient.WithHumanReviewReasonEnum(humanReviewReason, []string{"some warning"})
                    analysis := createTestAnalysis()

                    _, err := handler.Handle(ctx, analysis)

                    Expect(err).NotTo(HaveOccurred())
                    Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))
                    Expect(analysis.Status.Reason).To(Equal("WorkflowResolutionFailed"))
                    Expect(analysis.Status.SubReason).To(Equal(expectedSubReason))
                },
                Entry("workflow_not_found", "workflow_not_found", "WorkflowNotFound"),
                Entry("image_mismatch", "image_mismatch", "ImageMismatch"),
                Entry("parameter_validation_failed", "parameter_validation_failed", "ParameterValidationFailed"),
                Entry("no_matching_workflows", "no_matching_workflows", "NoMatchingWorkflows"),
                Entry("low_confidence", "low_confidence", "LowConfidence"),
                Entry("llm_parsing_error", "llm_parsing_error", "LLMParsingError"),
            )

            // Backward compatibility: Fallback to warning parsing
            DescribeTable("should fallback to warning parsing when enum is nil",
                func(warnings []string, expectedSubReason string) {
                    mockClient.WithHumanReviewRequired(warnings)  // No enum, just warnings
                    analysis := createTestAnalysis()

                    _, err := handler.Handle(ctx, analysis)

                    Expect(err).NotTo(HaveOccurred())
                    Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))
                    Expect(analysis.Status.Reason).To(Equal("WorkflowResolutionFailed"))
                    Expect(analysis.Status.SubReason).To(Equal(expectedSubReason))
                },
                Entry("workflow not found",
                    []string{"Workflow validation failed: workflow 'restart-pod-v1' not found in catalog"},
                    "WorkflowNotFound"),
                Entry("no matching workflows",
                    []string{"No workflows matched the incident criteria"},
                    "NoMatchingWorkflows"),
                Entry("low confidence",
                    []string{"Confidence (0.55) below threshold (0.70)"},
                    "LowConfidence"),
            )

            It("should preserve partial response for operator context", func() {
                reason := "parameter_validation_failed"
                mockClient.WithHumanReviewRequiredWithPartialResponse(
                    &reason,
                    []string{"Workflow validation failed"},
                    &client.SelectedWorkflow{
                        WorkflowID: "invalid-workflow",
                        Confidence: 0.85,
                    },
                )
                analysis := createTestAnalysis()

                _, err := handler.Handle(ctx, analysis)

                Expect(err).NotTo(HaveOccurred())
                Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))
                // Partial response preserved for operator context
                Expect(analysis.Status.SelectedWorkflow).NotTo(BeNil())
                Expect(analysis.Status.SelectedWorkflow.WorkflowID).To(Equal("invalid-workflow"))
            })
        })

        // BR-AI-009/010: Error handling using DescribeTable
        DescribeTable("error handling based on HTTP status code",
            func(statusCode int, shouldRetry bool, expectedPhase string) {
                mockClient.WithAPIError(statusCode, "API Error")
                analysis := createTestAnalysis()

                result, err := handler.Handle(ctx, analysis)

                if shouldRetry {
                    // BR-AI-009: Transient errors should retry
                    Expect(err).To(HaveOccurred())
                    Expect(result.RequeueAfter).To(BeNumerically(">", 0))
                } else {
                    // BR-AI-010: Permanent errors should fail immediately
                    Expect(err).NotTo(HaveOccurred())
                    Expect(analysis.Status.Phase).To(Equal(expectedPhase))
                }
            },
            Entry("503 Service Unavailable - retry", 503, true, aianalysis.PhaseInvestigating),
            Entry("429 Too Many Requests - retry", 429, true, aianalysis.PhaseInvestigating),
            Entry("502 Bad Gateway - retry", 502, true, aianalysis.PhaseInvestigating),
            Entry("504 Gateway Timeout - retry", 504, true, aianalysis.PhaseInvestigating),
            Entry("401 Unauthorized - fail", 401, false, aianalysis.PhaseFailed),
            Entry("400 Bad Request - fail", 400, false, aianalysis.PhaseFailed),
        )

        // BR-AI-009: Max retries exceeded
        Context("when max retries exceeded", func() {
            BeforeEach(func() {
                mockClient.WithAPIError(503, "Service Unavailable")
            })

            It("should mark as Failed after max retries", func() {
                analysis := createTestAnalysis()
                // Set retry count via annotations
                analysis.Annotations = map[string]string{
                    handlers.RetryCountAnnotation: "5",
                }

                result, err := handler.Handle(ctx, analysis)

                Expect(err).NotTo(HaveOccurred())
                Expect(result.Requeue).To(BeFalse())
                Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))
                Expect(analysis.Status.Message).To(ContainSubstring("max retries"))
            })
        })
    })
})
```

---

## 🟢 TDD GREEN Phase: Minimal Implementation

### 1. HolmesGPT Client Wrapper

```go
// pkg/aianalysis/client/holmesgpt.go
package client

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

// Config for HolmesGPT-API client (no APIKey - internal service auth)
type Config struct {
    BaseURL string
    Timeout time.Duration
}

// HolmesGPTClient wraps HolmesGPT-API calls
type HolmesGPTClient struct {
    baseURL    string
    httpClient *http.Client
}

// NewHolmesGPTClient creates a new client
func NewHolmesGPTClient(cfg Config) *HolmesGPTClient {
    timeout := cfg.Timeout
    if timeout == 0 {
        timeout = 60 * time.Second
    }

    return &HolmesGPTClient{
        baseURL: cfg.BaseURL,
        httpClient: &http.Client{
            Timeout: timeout,
        },
    }
}

// IncidentRequest represents request to /api/v1/incident/analyze
type IncidentRequest struct {
    Context        string                 `json:"context"`
    DetectedLabels map[string]interface{} `json:"detected_labels,omitempty"`
    CustomLabels   map[string][]string    `json:"custom_labels,omitempty"`
    OwnerChain     []OwnerChainEntry      `json:"owner_chain,omitempty"`
}

// OwnerChainEntry represents a resource in the owner chain
type OwnerChainEntry struct {
    Namespace string `json:"namespace"`
    Kind      string `json:"kind"`
    Name      string `json:"name"`
}

// IncidentResponse represents response from HolmesGPT-API /api/v1/incident/analyze
// Per HolmesGPT-API team (Dec 5, 2025): Returns ALL analysis results in one call
// BR-HAPI-197: Added NeedsHumanReview + HumanReviewReason fields (Dec 6, 2025)
type IncidentResponse struct {
    IncidentID           string                `json:"incident_id"`
    Analysis             string                `json:"analysis"`
    RootCauseAnalysis    *RootCauseAnalysis    `json:"root_cause_analysis,omitempty"`
    SelectedWorkflow     *SelectedWorkflow     `json:"selected_workflow,omitempty"`
    AlternativeWorkflows []AlternativeWorkflow `json:"alternative_workflows,omitempty"`
    Confidence           float64               `json:"confidence"`
    Timestamp            string                `json:"timestamp"`
    TargetInOwnerChain   bool                  `json:"target_in_owner_chain"`
    Warnings             []string              `json:"warnings,omitempty"`
    // BR-HAPI-197: True when AI cannot produce reliable result
    NeedsHumanReview     bool                  `json:"needs_human_review"`
    // HumanReviewReason: Structured enum for reliable SubReason mapping (Dec 6, 2025)
    // Enum: workflow_not_found, image_mismatch, parameter_validation_failed,
    //       no_matching_workflows, low_confidence, llm_parsing_error
    HumanReviewReason    *string               `json:"human_review_reason,omitempty"`
}

// RootCauseAnalysis contains structured RCA results
type RootCauseAnalysis struct {
    Summary             string   `json:"summary"`
    Severity            string   `json:"severity"`
    ContributingFactors []string `json:"contributing_factors,omitempty"`
}

// SelectedWorkflow contains the AI-selected workflow for execution
type SelectedWorkflow struct {
    WorkflowID      string            `json:"workflow_id"`
    Version         string            `json:"version,omitempty"`
    ContainerImage  string            `json:"containerImage"`
    ContainerDigest string            `json:"containerDigest,omitempty"`
    Confidence      float64           `json:"confidence"`
    Parameters      map[string]string `json:"parameters,omitempty"`
    Rationale       string            `json:"rationale"`
}

// AlternativeWorkflow - INFORMATIONAL ONLY, NOT for automatic execution
type AlternativeWorkflow struct {
    WorkflowID     string  `json:"workflow_id"`
    ContainerImage string  `json:"containerImage,omitempty"`
    Confidence     float64 `json:"confidence"`
    Rationale      string  `json:"rationale"`
}

// Investigate calls the HolmesGPT-API incident analyze endpoint
// BR-AI-006: API call construction
func (c *HolmesGPTClient) Investigate(ctx context.Context, req *IncidentRequest) (*IncidentResponse, error) {
    body, err := json.Marshal(req)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal request: %w", err)
    }

    httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
        c.baseURL+"/api/v1/incident/analyze", bytes.NewReader(body))
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    httpReq.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, &APIError{
            StatusCode: resp.StatusCode,
            Message:    fmt.Sprintf("API returned status %d", resp.StatusCode),
        }
    }

    var result IncidentResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("failed to decode response: %w", err)
    }

    return &result, nil
}

// APIError represents an API error
// BR-AI-009/010: Error classification for retry logic
type APIError struct {
    StatusCode int
    Message    string
}

func (e *APIError) Error() string {
    return fmt.Sprintf("API error (status %d): %s", e.StatusCode, e.Message)
}

// IsTransient returns true if the error is retry-able
// BR-AI-009: 429, 502, 503, 504 are transient
// BR-AI-010: 400, 401, 403, 404 are permanent
func (e *APIError) IsTransient() bool {
    switch e.StatusCode {
    case http.StatusTooManyRequests,   // 429
        http.StatusBadGateway,         // 502
        http.StatusServiceUnavailable, // 503
        http.StatusGatewayTimeout:     // 504
        return true
    default:
        return false
    }
}
```

### 2. InvestigatingHandler

```go
// pkg/aianalysis/handlers/investigating.go
package handlers

import (
    "context"
    "errors"
    "fmt"
    "math"
    "math/rand"
    "strconv"
    "strings"
    "time"

    "github.com/go-logr/logr"
    ctrl "sigs.k8s.io/controller-runtime"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/internal/controller/aianalysis"
    "github.com/jordigilh/kubernaut/pkg/aianalysis/client"
)

const (
    MaxRetries           = 5
    BaseDelay            = 30 * time.Second
    MaxDelay             = 480 * time.Second
    RetryCountAnnotation = "aianalysis.kubernaut.ai/retry-count"
)

// HolmesGPTClientInterface for dependency injection
type HolmesGPTClientInterface interface {
    Investigate(ctx context.Context, req *client.IncidentRequest) (*client.IncidentResponse, error)
}

// InvestigatingHandler handles the Investigating phase
// BR-AI-007: Call HolmesGPT-API and process response
type InvestigatingHandler struct {
    log      logr.Logger
    hgClient HolmesGPTClientInterface
}

// NewInvestigatingHandler creates a new InvestigatingHandler
func NewInvestigatingHandler(hgClient HolmesGPTClientInterface, log logr.Logger) *InvestigatingHandler {
    return &InvestigatingHandler{
        hgClient: hgClient,
        log:      log.WithName("investigating-handler"),
    }
}

// Handle processes the Investigating phase
// BR-AI-007: Call HolmesGPT-API and update status
func (h *InvestigatingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    h.log.Info("Processing Investigating phase", "name", analysis.Name)

    req := h.buildRequest(analysis)

    resp, err := h.hgClient.Investigate(ctx, req)
    if err != nil {
        return h.handleError(ctx, analysis, err)
    }

    return h.processResponse(ctx, analysis, resp)
}

func (h *InvestigatingHandler) buildRequest(analysis *aianalysisv1.AIAnalysis) *client.IncidentRequest {
    spec := analysis.Spec.AnalysisRequest.SignalContext

    return &client.IncidentRequest{
        Context: fmt.Sprintf("Incident in %s environment, target: %s/%s/%s, signal: %s",
            spec.Environment,
            spec.TargetResource.Namespace,
            spec.TargetResource.Kind,
            spec.TargetResource.Name,
            spec.SignalType,
        ),
    }
}

// handleError - BR-AI-009/010: Retry transient, fail on permanent
func (h *InvestigatingHandler) handleError(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) (ctrl.Result, error) {
    var apiErr *client.APIError
    if errors.As(err, &apiErr) && apiErr.IsTransient() {
        retryCount := h.getRetryCount(analysis)

        if retryCount >= MaxRetries {
            h.log.Info("Max retries exceeded", "retryCount", retryCount)
            analysis.Status.Phase = aianalysis.PhaseFailed
            analysis.Status.Message = fmt.Sprintf("HolmesGPT-API max retries exceeded: %v", err)
            analysis.Status.Reason = "MaxRetriesExceeded"
            return ctrl.Result{}, nil
        }

        delay := calculateBackoff(retryCount)
        h.setRetryCount(analysis, retryCount+1)

        h.log.Info("Transient error, scheduling retry",
            "retryCount", retryCount+1,
            "delay", delay.String(),
        )
        return ctrl.Result{RequeueAfter: delay}, err
    }

    // Permanent error
    h.log.Info("Permanent error", "error", err)
    analysis.Status.Phase = aianalysis.PhaseFailed
    analysis.Status.Message = fmt.Sprintf("HolmesGPT-API error: %v", err)
    analysis.Status.Reason = "APIError"
    return ctrl.Result{}, nil
}

// processResponse - BR-AI-008: Capture ALL v1.5 response fields
// Per HolmesGPT-API team (Dec 5, 2025): /incident/analyze returns ALL results
// BR-HAPI-197: Handle needs_human_review response (Dec 6, 2025)
func (h *InvestigatingHandler) processResponse(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.IncidentResponse) (ctrl.Result, error) {
    h.log.Info("Processing successful response",
        "confidence", resp.Confidence,
        "targetInOwnerChain", resp.TargetInOwnerChain,
        "hasSelectedWorkflow", resp.SelectedWorkflow != nil,
        "alternativeWorkflowsCount", len(resp.AlternativeWorkflows),
        "needsHumanReview", resp.NeedsHumanReview,
    )

    // BR-HAPI-197: Check if workflow resolution failed
    if resp.NeedsHumanReview {
        return h.handleWorkflowResolutionFailure(ctx, analysis, resp)
    }

    // Store HAPI response metadata
    targetInOwnerChain := resp.TargetInOwnerChain
    analysis.Status.TargetInOwnerChain = &targetInOwnerChain
    analysis.Status.Warnings = resp.Warnings

    // Store RootCauseAnalysis (if present)
    if resp.RootCauseAnalysis != nil {
        analysis.Status.RootCause = resp.RootCauseAnalysis.Summary
        analysis.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
            Summary:             resp.RootCauseAnalysis.Summary,
            Severity:            resp.RootCauseAnalysis.Severity,
            ContributingFactors: resp.RootCauseAnalysis.ContributingFactors,
        }
    }

    // Store SelectedWorkflow (DD-CONTRACT-002)
    if resp.SelectedWorkflow != nil {
        analysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
            WorkflowID:      resp.SelectedWorkflow.WorkflowID,
            Version:         resp.SelectedWorkflow.Version,
            ContainerImage:  resp.SelectedWorkflow.ContainerImage,
            ContainerDigest: resp.SelectedWorkflow.ContainerDigest,
            Confidence:      resp.SelectedWorkflow.Confidence,
            Parameters:      resp.SelectedWorkflow.Parameters,
            Rationale:       resp.SelectedWorkflow.Rationale,
        }
    }

    // Store AlternativeWorkflows (INFORMATIONAL ONLY - NOT for execution)
    if len(resp.AlternativeWorkflows) > 0 {
        alternatives := make([]aianalysisv1.AlternativeWorkflow, 0, len(resp.AlternativeWorkflows))
        for _, alt := range resp.AlternativeWorkflows {
            alternatives = append(alternatives, aianalysisv1.AlternativeWorkflow{
                WorkflowID:     alt.WorkflowID,
                ContainerImage: alt.ContainerImage,
                Confidence:     alt.Confidence,
                Rationale:      alt.Rationale,
            })
        }
        analysis.Status.AlternativeWorkflows = alternatives
    }

    // Reset retry count on success
    h.setRetryCount(analysis, 0)

    // Transition to Analyzing phase
    analysis.Status.Phase = aianalysis.PhaseAnalyzing
    analysis.Status.Message = "Investigation complete, starting analysis"

    return ctrl.Result{Requeue: true}, nil
}

// Retry count stored in annotations (not Status) to survive status updates
func (h *InvestigatingHandler) getRetryCount(analysis *aianalysisv1.AIAnalysis) int {
    if analysis.Annotations == nil {
        return 0
    }
    countStr, ok := analysis.Annotations[RetryCountAnnotation]
    if !ok {
        return 0
    }
    count, _ := strconv.Atoi(countStr)
    return count
}

func (h *InvestigatingHandler) setRetryCount(analysis *aianalysisv1.AIAnalysis, count int) {
    if analysis.Annotations == nil {
        analysis.Annotations = make(map[string]string)
    }
    analysis.Annotations[RetryCountAnnotation] = strconv.Itoa(count)
}

func calculateBackoff(attemptCount int) time.Duration {
    delay := time.Duration(float64(BaseDelay) * math.Pow(2, float64(attemptCount)))
    if delay > MaxDelay {
        delay = MaxDelay
    }
    // Add jitter (±10%)
    jitter := time.Duration(float64(delay) * (0.9 + 0.2*rand.Float64()))
    return jitter
}

// BR-HAPI-197: Handle workflow resolution failure when needs_human_review=true
// Updated Dec 6, 2025: Use HumanReviewReason enum for reliable SubReason mapping
func (h *InvestigatingHandler) handleWorkflowResolutionFailure(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.IncidentResponse) (ctrl.Result, error) {
    h.log.Info("Workflow resolution failed, requires human review",
        "warnings", resp.Warnings,
        "humanReviewReason", resp.HumanReviewReason,
        "hasPartialWorkflow", resp.SelectedWorkflow != nil,
    )

    // Set structured failure
    analysis.Status.Phase = aianalysis.PhaseFailed
    analysis.Status.Reason = "WorkflowResolutionFailed"

    // Use HumanReviewReason enum if available (preferred), else fallback to warning parsing
    if resp.HumanReviewReason != nil {
        analysis.Status.SubReason = h.mapEnumToSubReason(*resp.HumanReviewReason)
    } else {
        // Backward compatibility: parse warnings if enum not available
        analysis.Status.SubReason = mapWarningsToSubReason(resp.Warnings)
    }

    analysis.Status.Message = strings.Join(resp.Warnings, "; ")
    analysis.Status.Warnings = resp.Warnings

    // Preserve partial response for operator context (BR-HAPI-197.4)
    if resp.SelectedWorkflow != nil {
        analysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
            WorkflowID:      resp.SelectedWorkflow.WorkflowID,
            ContainerImage:  resp.SelectedWorkflow.ContainerImage,
            Confidence:      resp.SelectedWorkflow.Confidence,
            Rationale:       resp.SelectedWorkflow.Rationale,
        }
    }

    // Preserve RCA for operator context
    if resp.RootCauseAnalysis != nil {
        analysis.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
            Summary:             resp.RootCauseAnalysis.Summary,
            Severity:            resp.RootCauseAnalysis.Severity,
            ContributingFactors: resp.RootCauseAnalysis.ContributingFactors,
        }
    }

    // Emit metric with sub-reason label (Day 5)
    // metrics.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", analysis.Status.SubReason).Inc()

    return ctrl.Result{}, nil  // Terminal - no requeue
}

// mapEnumToSubReason maps HAPI HumanReviewReason enum to CRD SubReason
// This is the preferred method - direct enum-to-enum mapping (Dec 6, 2025)
func (h *InvestigatingHandler) mapEnumToSubReason(reason string) string {
    mapping := map[string]string{
        "workflow_not_found":           "WorkflowNotFound",
        "image_mismatch":               "ImageMismatch",
        "parameter_validation_failed":  "ParameterValidationFailed",
        "no_matching_workflows":        "NoMatchingWorkflows",
        "low_confidence":               "LowConfidence",
        "llm_parsing_error":            "LLMParsingError",
    }
    if subReason, ok := mapping[reason]; ok {
        return subReason
    }
    h.log.Info("Unknown human_review_reason, defaulting to WorkflowNotFound", "reason", reason)
    return "WorkflowNotFound"
}

// mapWarningsToSubReason extracts SubReason from HAPI warnings
// DEPRECATED: Use mapEnumToSubReason when HumanReviewReason is available
// Kept for backward compatibility with older HAPI versions
func mapWarningsToSubReason(warnings []string) string {
    warningsStr := strings.ToLower(strings.Join(warnings, " "))

    switch {
    case strings.Contains(warningsStr, "not found") || strings.Contains(warningsStr, "does not exist"):
        return "WorkflowNotFound"
    case strings.Contains(warningsStr, "no workflows matched") || strings.Contains(warningsStr, "no matching"):
        return "NoMatchingWorkflows"
    case strings.Contains(warningsStr, "confidence") && strings.Contains(warningsStr, "below"):
        return "LowConfidence"
    case strings.Contains(warningsStr, "parameter validation") || strings.Contains(warningsStr, "missing required"):
        return "ParameterValidationFailed"
    case strings.Contains(warningsStr, "image mismatch") || strings.Contains(warningsStr, "container image"):
        return "ImageMismatch"
    case strings.Contains(warningsStr, "parse") || strings.Contains(warningsStr, "invalid json"):
        return "LLMParsingError"
    default:
        return "WorkflowNotFound"  // Default to most common case
    }
}
```

---

## ✅ Day 2 Completion Checklist

### Code Deliverables

- [ ] `pkg/aianalysis/client/holmesgpt.go` - HolmesGPT-API client (includes `NeedsHumanReview` field)
- [ ] `pkg/aianalysis/handlers/investigating.go` - InvestigatingHandler (includes BR-HAPI-197 handling)
- [ ] `test/unit/aianalysis/holmesgpt_client_test.go` - Client tests
- [ ] `test/unit/aianalysis/investigating_handler_test.go` - Handler tests (includes `needs_human_review` tests)

### BR-HAPI-197 Compliance Checklist

- [ ] `NeedsHumanReview` field in `IncidentResponse`
- [ ] `handleWorkflowResolutionFailure()` method in InvestigatingHandler
- [ ] `mapWarningsToSubReason()` helper function
- [ ] Tests for all 6 SubReason scenarios (DescribeTable)
- [ ] Test for partial response preservation

### Verification Commands

```bash
# Run all unit tests
go test -v ./test/unit/aianalysis/...

# Coverage check (target: 65%+)
go test -coverprofile=coverage.out ./pkg/aianalysis/...
go tool cover -func=coverage.out | grep total

# Build verification
go build ./pkg/aianalysis/...
```

### EOD Documentation

Create: `docs/services/crd-controllers/02-aianalysis/implementation/phase0/02-day2-complete.md`

---

## 📚 Related Documents

- [DAY_01_FOUNDATION.md](./DAY_01_FOUNDATION.md) - Previous day
- [DAY_03_ANALYZING_HANDLER.md](./DAY_03_ANALYZING_HANDLER.md) - Next day
- [APPENDIX_B_ERROR_HANDLING_PHILOSOPHY.md](../appendices/APPENDIX_B_ERROR_HANDLING_PHILOSOPHY.md) - Error patterns

