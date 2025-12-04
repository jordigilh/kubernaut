# Day 2: InvestigatingHandler - HolmesGPT-API Integration

**Part of**: AI Analysis Implementation Plan V1.0
**Parent Document**: [IMPLEMENTATION_PLAN_V1.0.md](../../IMPLEMENTATION_PLAN_V1.0.md)
**Duration**: 6-8 hours
**Target Confidence**: 68%

---

## 🎯 Day 2 Objectives

| Objective | Priority | BR Reference |
|-----------|----------|--------------|
| Implement HolmesGPT-API client wrapper | P0 | BR-AI-006 |
| Implement InvestigatingHandler | P0 | BR-AI-007 |
| Handle API responses (targetInOwnerChain, warnings) | P0 | BR-AI-008 |
| Implement retry logic with exponential backoff | P0 | BR-AI-009 |
| Handle permanent errors (401, 400) | P0 | BR-AI-010 |

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
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
)

var _ = Describe("InvestigatingHandler", func() {
    var (
        handler    *handlers.InvestigatingHandler
        mockClient *MockHolmesGPTClient
    )

    BeforeEach(func() {
        mockClient = NewMockHolmesGPTClient()
        handler = handlers.NewInvestigatingHandler(
            handlers.WithHolmesGPTClient(mockClient),
            handlers.WithLogger(testLogger),
        )
    })

    // BR-AI-007: Process HolmesGPT response
    Describe("Handle", func() {
        Context("with successful API response", func() {
            BeforeEach(func() {
                mockClient.SetResponse(&client.IncidentResponse{
                    Analysis:           "Root cause identified",
                    TargetInOwnerChain: true,
                    Confidence:         0.9,
                    Warnings:           []string{},
                }, nil)
            })

            It("should transition to Analyzing phase", func() {
                analysis := validAIAnalysis()
                analysis.Status.Phase = aianalysisv1.PhaseInvestigating

                result, err := handler.Handle(ctx, analysis)

                Expect(err).NotTo(HaveOccurred())
                Expect(analysis.Status.Phase).To(Equal(aianalysisv1.PhaseAnalyzing))
                Expect(analysis.Status.Investigation).NotTo(BeNil())
                Expect(analysis.Status.TargetInOwnerChain).To(BeTrue())
            })
        })

        // BR-AI-008: Handle warnings
        Context("with warnings in response", func() {
            BeforeEach(func() {
                mockClient.SetResponse(&client.IncidentResponse{
                    Analysis:           "Analysis with warnings",
                    TargetInOwnerChain: false,
                    Warnings:           []string{"High memory pressure", "Node scheduling delayed"},
                }, nil)
            })

            It("should capture warnings in status", func() {
                analysis := validAIAnalysis()
                analysis.Status.Phase = aianalysisv1.PhaseInvestigating

                _, err := handler.Handle(ctx, analysis)

                Expect(err).NotTo(HaveOccurred())
                Expect(analysis.Status.Warnings).To(HaveLen(2))
                Expect(analysis.Status.TargetInOwnerChain).To(BeFalse())
            })
        })

        // BR-AI-009: Retry on transient errors
        DescribeTable("retries on transient errors",
            func(statusCode int, shouldRetry bool) {
                mockClient.SetResponse(nil, &client.APIError{StatusCode: statusCode})
                
                analysis := validAIAnalysis()
                analysis.Status.Phase = aianalysisv1.PhaseInvestigating
                analysis.Status.RetryCount = 0

                result, err := handler.Handle(ctx, analysis)

                if shouldRetry {
                    Expect(err).To(HaveOccurred())
                    Expect(result.RequeueAfter).To(BeNumerically(">", 0))
                    Expect(analysis.Status.RetryCount).To(Equal(1))
                } else {
                    Expect(analysis.Status.Phase).To(Equal(aianalysisv1.PhaseFailed))
                }
            },
            Entry("503 Service Unavailable - retry", 503, true),
            Entry("429 Too Many Requests - retry", 429, true),
            Entry("502 Bad Gateway - retry", 502, true),
            Entry("401 Unauthorized - no retry", 401, false),
            Entry("400 Bad Request - no retry", 400, false),
        )

        // BR-AI-009: Max retries
        Context("when max retries exceeded", func() {
            BeforeEach(func() {
                mockClient.SetResponse(nil, &client.APIError{StatusCode: 503})
            })

            It("should mark as Failed after 5 retries", func() {
                analysis := validAIAnalysis()
                analysis.Status.Phase = aianalysisv1.PhaseInvestigating
                analysis.Status.RetryCount = 5 // Already at max

                _, _ = handler.Handle(ctx, analysis)

                Expect(analysis.Status.Phase).To(Equal(aianalysisv1.PhaseFailed))
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
    "io"
    "net/http"
    "time"
)

// Config for HolmesGPT-API client
type Config struct {
    BaseURL string
    Timeout time.Duration
    APIKey  string
}

// HolmesGPTClient wraps HolmesGPT-API calls
type HolmesGPTClient struct {
    baseURL    string
    httpClient *http.Client
    apiKey     string
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
        apiKey: cfg.APIKey,
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

// IncidentResponse represents response from HolmesGPT-API
type IncidentResponse struct {
    Analysis           string   `json:"analysis"`
    TargetInOwnerChain bool     `json:"target_in_owner_chain"`
    Confidence         float64  `json:"confidence"`
    Warnings           []string `json:"warnings,omitempty"`
}

// Investigate calls the HolmesGPT-API incident analyze endpoint
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
    if c.apiKey != "" {
        httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
    }

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
type APIError struct {
    StatusCode int
    Message    string
}

func (e *APIError) Error() string {
    return fmt.Sprintf("API error (status %d): %s", e.StatusCode, e.Message)
}

// IsTransient returns true if the error is retry-able
func (e *APIError) IsTransient() bool {
    switch e.StatusCode {
    case 429, 502, 503, 504:
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
    "math"
    "math/rand"
    "time"

    "github.com/go-logr/logr"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/aianalysis/client"
    ctrl "sigs.k8s.io/controller-runtime"
)

const (
    MaxRetries = 5
    BaseDelay  = 30 * time.Second
    MaxDelay   = 480 * time.Second
)

// InvestigatingHandler handles the Investigating phase
type InvestigatingHandler struct {
    log         logr.Logger
    hgClient    client.HolmesGPTClientInterface
}

// NewInvestigatingHandler creates a new InvestigatingHandler
func NewInvestigatingHandler(opts ...Option) *InvestigatingHandler {
    h := &InvestigatingHandler{}
    for _, opt := range opts {
        opt(h)
    }
    return h
}

// Phase returns the phase this handler processes
func (h *InvestigatingHandler) Phase() aianalysisv1.Phase {
    return aianalysisv1.PhaseInvestigating
}

// Handle calls HolmesGPT-API and processes the response
func (h *InvestigatingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    // Build request from spec
    req := h.buildRequest(analysis)

    // Call HolmesGPT-API
    resp, err := h.hgClient.Investigate(ctx, req)
    if err != nil {
        return h.handleError(ctx, analysis, err)
    }

    // Process successful response
    return h.processResponse(ctx, analysis, resp)
}

func (h *InvestigatingHandler) buildRequest(analysis *aianalysisv1.AIAnalysis) *client.IncidentRequest {
    spec := &analysis.Spec.SignalContext
    
    req := &client.IncidentRequest{
        Context: fmt.Sprintf("Incident in %s environment, target: %s/%s/%s",
            spec.Environment,
            spec.TargetResource.Namespace,
            spec.TargetResource.Kind,
            spec.TargetResource.Name,
        ),
    }

    // Add detected labels if present
    if spec.EnrichmentResults != nil && spec.EnrichmentResults.DetectedLabels != nil {
        req.DetectedLabels = h.convertDetectedLabels(spec.EnrichmentResults.DetectedLabels)
    }

    // Add custom labels
    if spec.EnrichmentResults != nil {
        req.CustomLabels = spec.EnrichmentResults.CustomLabels
    }

    // Add owner chain
    if spec.EnrichmentResults != nil {
        for _, entry := range spec.EnrichmentResults.OwnerChain {
            req.OwnerChain = append(req.OwnerChain, client.OwnerChainEntry{
                Namespace: entry.Namespace,
                Kind:      entry.Kind,
                Name:      entry.Name,
            })
        }
    }

    return req
}

func (h *InvestigatingHandler) handleError(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) (ctrl.Result, error) {
    var apiErr *client.APIError
    if errors.As(err, &apiErr) && apiErr.IsTransient() {
        // Transient error - retry with backoff
        if analysis.Status.RetryCount >= MaxRetries {
            // Max retries exceeded
            analysis.Status.Phase = aianalysisv1.PhaseFailed
            analysis.Status.Message = fmt.Sprintf("HolmesGPT-API max retries exceeded: %v", err)
            return ctrl.Result{}, nil
        }

        delay := calculateBackoff(analysis.Status.RetryCount)
        analysis.Status.RetryCount++
        h.log.Info("Transient error, scheduling retry",
            "retryCount", analysis.Status.RetryCount,
            "delay", delay.String(),
        )
        return ctrl.Result{RequeueAfter: delay}, err
    }

    // Permanent error
    analysis.Status.Phase = aianalysisv1.PhaseFailed
    analysis.Status.Message = fmt.Sprintf("HolmesGPT-API error: %v", err)
    return ctrl.Result{}, nil
}

func (h *InvestigatingHandler) processResponse(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.IncidentResponse) (ctrl.Result, error) {
    // Store investigation results
    analysis.Status.Investigation = &aianalysisv1.InvestigationResult{
        Analysis:   resp.Analysis,
        Confidence: resp.Confidence,
    }
    analysis.Status.TargetInOwnerChain = &resp.TargetInOwnerChain
    analysis.Status.Warnings = resp.Warnings

    // Reset retry count on success
    analysis.Status.RetryCount = 0

    // Transition to next phase
    analysis.Status.Phase = aianalysisv1.PhaseAnalyzing
    return ctrl.Result{Requeue: true}, nil
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
```

---

## ✅ Day 2 Completion Checklist

### Code Deliverables

- [ ] `pkg/aianalysis/client/holmesgpt.go` - HolmesGPT-API client
- [ ] `pkg/aianalysis/handlers/investigating.go` - InvestigatingHandler
- [ ] `test/unit/aianalysis/holmesgpt_client_test.go` - Client tests
- [ ] `test/unit/aianalysis/investigating_handler_test.go` - Handler tests

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

