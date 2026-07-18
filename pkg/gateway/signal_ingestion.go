/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	// BR-GATEWAY-093: Circuit breaker detection

	// DD-AUDIT-003: Audit integration
	// Ogen generated audit types
	// BR-AUDIT-005 Gap #7: Standardized error details
	// BR-GATEWAY-036/037: Shared auth middleware
	// Issue #753: Dedicated health server
	// Issue #756: FileWatcher for cert rotation
	// Issue #493/#678: Conditional TLS

	// BR-GATEWAY-190: Lease resources for distributed locking

	// BR-GATEWAY-036/037: K8s clientset for TokenReview/SAR

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/middleware" // BR-109: Request ID middleware
	"github.com/jordigilh/kubernaut/pkg/gateway/types"      // BR-HTTP-015: Shared CORS library
	// DD-005: Shared sanitization library
	"github.com/jordigilh/kubernaut/pkg/shared/scope" // BR-SCOPE-002: Resource scope management
)

// RegisterAdapter registers a RoutableAdapter using chi router
//
// This method:
// 1. Validates adapter (checks for duplicate names/routes)
// 2. Registers adapter in registry
// 3. Creates HTTP handler (batch-aware for BatchParser adapters, single-signal otherwise)
// 4. Applies middleware and registers route with chi router
//
// Middleware applied:
// - Content-Type validation (BR-042)
// - Request ID (chi middleware - global)
// - Real IP extraction (chi middleware - global)
//
// Example:
//
//	prometheusAdapter := adapters.NewPrometheusAdapter(ownerResolver, registry)
//	server.RegisterAdapter(prometheusAdapter)
//	// Now POST /api/v1/signals/prometheus is active
func (s *Server) RegisterAdapter(adapter adapters.RoutableAdapter) error {
	// Register in registry
	if err := s.adapterRegistry.Register(adapter); err != nil {
		return fmt.Errorf("failed to register adapter: %w", err)
	}

	// Create adapter HTTP handler
	handler := s.createAdapterHandler(adapter)

	// BR-042: Apply Content-Type validation middleware
	// Rejects non-JSON payloads early, before processing
	wrappedHandler := middleware.ValidateContentType(handler)

	// BR-GATEWAY-074, BR-GATEWAY-075: Apply adapter-specific replay prevention middleware
	// Each adapter declares its own strategy via ReplayValidator():
	// - Header-based (e.g., Prometheus): middleware.TimestampValidator (X-Timestamp header)
	// - Body-based (e.g., K8s Events): middleware.EventFreshnessValidator (event timestamp)
	finalHandler := adapter.ReplayValidator(5 * time.Minute)(wrappedHandler)

	// BR-GATEWAY-036/037: Apply auth middleware (outermost layer)
	// Auth is checked before any content-type validation or replay prevention
	if s.authMiddleware != nil {
		finalHandler = s.authMiddleware.Handler(finalHandler)
	}

	// Register route using chi with full path
	// Chi automatically enforces POST method (returns 405 for other methods)
	// Note: chi.Router.Post() accepts http.HandlerFunc, so we use HandlerFunc wrapper
	s.router.Post(adapter.GetRoute(), finalHandler.ServeHTTP)

	s.logger.Info("Registered adapter route",
		"adapter", adapter.Name(),
		"route", adapter.GetRoute())

	return nil
}

// createAdapterHandler creates an HTTP handler for an adapter
//
// This handler:
// For single-signal adapters:
// 1. Reads request body
// 2. Calls adapter.Parse() to convert to NormalizedSignal
// 3. Validates signal using adapter.Validate()
// 4. Calls ProcessSignal() to run full pipeline
// 5. Returns HTTP response (201/202/400/500)
//
// For BatchParser adapters (e.g., Prometheus):
// Delegates to handleBatchRequest which processes each signal independently
// and returns HTTP 207 Multi-Status with per-alert results.
//
// REFACTORED: Reduced cyclomatic complexity by extracting helper methods
func (s *Server) createAdapterHandler(adapter adapters.SignalAdapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only accept POST requests
		if r.Method != http.MethodPost {
			s.writeJSONError(w, r, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ctx := r.Context()
		logger := middleware.GetLogger(ctx)

		// Check if adapter supports batch parsing (e.g., Prometheus AlertManager)
		if batchAdapter, ok := adapter.(adapters.BatchParser); ok {
			s.handleBatchRequest(ctx, w, r, adapter, batchAdapter, logger)
			return
		}

		// Read, parse, and validate signal
		signal, err := s.readParseValidateSignal(ctx, w, r, adapter, logger)
		if err != nil {
			return // Error response already sent
		}

		// BR-GATEWAY-102: Enforce per-handler timeout on K8s API operations.
		// Must be < WriteTimeout to allow writing 504 JSON before the server kills the connection.
		k8sTimeout := s.config.Server.K8sRequestTimeout
		if k8sTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, k8sTimeout)
			defer cancel()
		}

		// Process signal through pipeline
		response, err := s.ProcessSignal(ctx, signal)
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				logger.Error(err, "K8s request timeout exceeded",
					"adapter", adapter.Name(),
					"timeout", k8sTimeout)
				s.writeJSONError(w, r, "Request processing timed out", http.StatusGatewayTimeout)
				return
			}
			s.handleProcessingError(w, r, err, adapter.Name(), logger)
			return
		}

		// Send success response
		s.sendSuccessResponse(w, response)
	}
}

// handleBatchRequest processes a batch payload where each signal is handled independently.
//
// Routing strategy:
//   - Single-alert batches (len == 1): delegate to the standard single-signal pipeline
//     so that existing HTTP status contracts (201, 202, 500, 504) are preserved.
//     AlertManager retries on 5xx, so returning 207 for a single failed alert
//     would silently swallow the failure.
//   - Multi-alert batches (len > 1): return HTTP 207 Multi-Status with per-alert
//     results and an aggregate summary (#1036).
func (s *Server) handleBatchRequest(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	adapter adapters.SignalAdapter,
	batchAdapter adapters.BatchParser,
	logger logr.Logger,
) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			logger.Info("Request body too large", "limit", maxRequestBodySize)
			s.writeJSONError(w, r, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		logger.Error(err, "Failed to read request body")
		s.writeJSONError(w, r, "Failed to read request body", http.StatusBadRequest)
		return
	}

	signals, err := batchAdapter.ParseBatch(ctx, body)
	if err != nil {
		logger.Info("Batch parse failed", "adapter", adapter.Name(), "error", err)
		s.writeJSONError(w, r, "Failed to parse batch payload", http.StatusBadRequest)
		return
	}

	if len(signals) == 0 {
		resp := BatchProcessingResponse{
			Results: []ProcessingResult{},
			Summary: BatchSummary{Total: 0},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMultiStatus)
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	// Single-alert batch: use standard single-signal pipeline to preserve
	// HTTP status contracts (201/202/500/504) for backward compatibility.
	if len(signals) == 1 {
		s.processSingleSignal(ctx, w, r, adapter, signals[0], logger)
		return
	}

	// Multi-alert batch: process each signal independently, return 207.
	s.processMultiSignalBatch(ctx, w, adapter, signals, logger)
}

// processSingleSignal handles a single signal through the standard pipeline,
// preserving existing HTTP status code contracts.
func (s *Server) processSingleSignal(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	adapter adapters.SignalAdapter,
	signal *types.NormalizedSignal,
	logger logr.Logger,
) {
	if valErr := adapter.Validate(signal); valErr != nil {
		logger.Info("Signal validation failed",
			"adapter", adapter.Name(),
			"error", valErr)
		s.writeValidationError(w, r, fmt.Sprintf("Signal validation failed: %v", valErr))
		return
	}

	k8sTimeout := s.config.Server.K8sRequestTimeout
	if k8sTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, k8sTimeout)
		defer cancel()
	}

	response, procErr := s.ProcessSignal(ctx, signal)
	if procErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			logger.Error(procErr, "K8s request timeout exceeded",
				"adapter", adapter.Name(),
				"timeout", k8sTimeout)
			s.writeJSONError(w, r, "Request processing timed out", http.StatusGatewayTimeout)
			return
		}
		s.handleProcessingError(w, r, procErr, adapter.Name(), logger)
		return
	}

	s.sendSuccessResponse(w, response)
}

// processMultiSignalBatch processes multiple signals independently and returns
// HTTP 207 Multi-Status with per-alert results (#1036). Per-signal failures
// are captured in the response body (via processOneSignalInBatch), not
// written as an HTTP error response, so this needs no *http.Request.
func (s *Server) processMultiSignalBatch(
	ctx context.Context,
	w http.ResponseWriter,
	adapter adapters.SignalAdapter,
	signals []*types.NormalizedSignal,
	logger logr.Logger,
) {
	k8sTimeout := s.config.Server.K8sRequestTimeout
	if k8sTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, k8sTimeout)
		defer cancel()
	}

	results := make([]ProcessingResult, 0, len(signals))
	var summary BatchSummary
	summary.Total = len(signals)

	for _, signal := range signals {
		result, outcome := s.processOneSignalInBatch(ctx, adapter, signal, logger)
		results = append(results, result)
		summary.record(outcome)
	}

	resp := BatchProcessingResponse{
		Results: results,
		Summary: summary,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusMultiStatus)
	if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
		logger.Error(encErr, "Failed to encode batch response")
	}
}

// processOneSignalInBatch validates and processes a single signal within a
// multi-signal batch (#1036), classifying the outcome so the caller can
// aggregate BatchSummary counts. Extracted from processMultiSignalBatch
// (funlen).
func (s *Server) processOneSignalInBatch(
	ctx context.Context,
	adapter adapters.SignalAdapter,
	signal *types.NormalizedSignal,
	logger logr.Logger,
) (ProcessingResult, BatchSignalOutcome) {
	if err := ctx.Err(); err != nil {
		return ProcessingResult{
			Status:      "failed",
			Fingerprint: signal.Fingerprint,
			Error:       "request timeout exceeded",
		}, BatchOutcomeFailed
	}

	if valErr := adapter.Validate(signal); valErr != nil {
		logger.Info("Signal validation failed in batch",
			"fingerprint", signal.Fingerprint,
			"error", valErr)
		return ProcessingResult{
			Status:      "rejected",
			Fingerprint: signal.Fingerprint,
			Error:       fmt.Sprintf("Signal validation failed: %s", valErr.Error()),
		}, BatchOutcomeRejected
	}

	response, procErr := s.ProcessSignal(ctx, signal)
	if procErr != nil {
		logger.Error(procErr, "Signal processing failed in batch",
			"fingerprint", signal.Fingerprint)
		return ProcessingResult{
			Status:      "failed",
			Fingerprint: signal.Fingerprint,
			Error:       "Processing failed",
		}, BatchOutcomeFailed
	}

	result := ProcessingResult{
		Status:      response.Status,
		Fingerprint: response.Fingerprint,
		Message:     response.Message,
	}

	switch response.Status {
	case StatusDeduplicated:
		return result, BatchOutcomeDeduplicated
	case StatusRejected:
		return result, BatchOutcomeRejected
	case StatusCreated:
		return result, BatchOutcomeCreated
	default:
		return result, BatchOutcomeCreated
	}
}

// maxRequestBodySize references the shared constant from middleware.
// Issue #673 C-1 + C-ADV-1: Single source of truth for the body cap.
const maxRequestBodySize = middleware.MaxRequestBodySize

// readParseValidateSignal reads, parses, and validates the signal from the request
// Returns nil signal and writes error response if any step fails
func (s *Server) readParseValidateSignal(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	adapter adapters.SignalAdapter,
	logger logr.Logger,
) (*types.NormalizedSignal, error) {
	// Issue #673 C-1: Limit request body size before reading into memory
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			logger.Info("Request body too large", "limit", maxRequestBodySize)
			s.writeJSONError(w, r, "Request body too large", http.StatusRequestEntityTooLarge)
			return nil, err
		}
		logger.Error(err, "Failed to read request body")
		s.writeValidationError(w, r, "Failed to read request body")
		return nil, err
	}

	// Parse signal using adapter
	signal, err := adapter.Parse(ctx, body)
	if err != nil {
		logger.Info("Failed to parse signal",
			"adapter", adapter.Name(),
			"error", err)
		s.writeValidationError(w, r, "Failed to parse signal")
		return nil, err
	}

	// Validate signal
	if err := adapter.Validate(signal); err != nil {
		logger.Info("Signal validation failed",
			"adapter", adapter.Name(),
			"error", err)
		s.writeValidationError(w, r, "Signal validation failed")
		return nil, err
	}

	return signal, nil
}

// handleProcessingError handles errors from signal processing and sends appropriate HTTP response
func (s *Server) handleProcessingError(
	w http.ResponseWriter,
	r *http.Request,
	err error,
	adapterName string,
	logger logr.Logger,
) {
	logger.Error(err, "Signal processing failed",
		"adapter", adapterName)

	// Issue #673 C-ADV-2: All processing errors return a generic message.
	// Internal details (K8s API addresses, CRD names, namespace names) are
	// already logged at line 958 via logger.Error -- no observability lost.
	s.writeInternalError(w, r, "Internal server error")
}

// sendSuccessResponse sends the success HTTP response. Metrics recording is
// handled by middleware.HTTPMetrics, not here (Phase 3a of FedRAMP
// remediation removed the duplicate observation that used to need
// adapter/start), so it takes only what it writes into the response.
func (s *Server) sendSuccessResponse(
	w http.ResponseWriter,
	response *ProcessingResponse,
) {
	// Determine HTTP status code based on response status
	statusCode := http.StatusCreated
	if response.Status == StatusRejected {
		statusCode = http.StatusOK // HTTP 200 for scope rejection (not an error, just informational)
	} else if response.Status == StatusDeduplicated || response.Duplicate {
		statusCode = http.StatusAccepted // HTTP 202 for deduplication
	}

	// HTTPRequestDuration is already observed by middleware.HTTPMetrics (http_metrics.go).
	// Duplicate observation removed — see Phase 3a of FedRAMP remediation.

	// Send response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error(err, "Failed to encode JSON response")
	}
}

// validateScope checks whether the signal's target resource is within Kubernaut's
// management scope. Returns (nil, nil) if managed, (*ProcessingResponse, nil) for
// a clean rejection, or (nil, error) on scope infrastructure failure.
//
// BR-SCOPE-002: Label-based resource opt-in with 2-level hierarchy.
// BR-SCOPE-013: Deny-by-default when scope checker is not initialized.
func (s *Server) validateScope(ctx context.Context, signal *types.NormalizedSignal) (*ProcessingResponse, error) {
	logger := middleware.GetLogger(ctx)

	if s.scopeChecker == nil {
		logger.Error(nil, "Scope checker not initialized — rejecting signal (deny-by-default)",
			"namespace", signal.Namespace,
			"kind", signal.Resource.Kind,
			"name", signal.Resource.Name,
			"fingerprint", signal.Fingerprint)
		s.metricsInstance.SignalsRejectedTotal.WithLabelValues(RejectionReasonScopeCheckerNotInitialized).Inc()
		return NewRejectedResponse(signal.Namespace, signal.Resource.Kind, signal.Resource.Name), nil
	}

	managed, err := s.scopeChecker.IsManagedResource(ctx, scope.ResourceIdentity{
		ClusterID: signal.ClusterID,
		Kind:      signal.Resource.Kind,
		Namespace: signal.Namespace,
		Name:      signal.Resource.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("scope validation failed: %w", err)
	}

	if managed {
		logger.V(1).Info("Scope check passed: resource is managed",
			"namespace", signal.Namespace,
			"kind", signal.Resource.Kind,
			"name", signal.Resource.Name)
		// nolint:nilnil // intentional "scope check passed, continue
		// processing" sentinel, not an error — sole caller
		// (signal_ingestion_process.go) already guards with
		// `else if rejection != nil` before using the response (Issue #1546
		// Tier 2).
		return nil, nil
	}

	s.metricsInstance.SignalsRejectedTotal.WithLabelValues(RejectionReasonUnmanagedResource).Inc()
	logger.Info("Signal rejected: resource not managed by Kubernaut",
		"namespace", signal.Namespace,
		"kind", signal.Resource.Kind,
		"name", signal.Resource.Name,
		"reason", RejectionReasonUnmanagedResource,
		"fingerprint", signal.Fingerprint)

	return NewRejectedResponse(signal.Namespace, signal.Resource.Kind, signal.Resource.Name), nil
}

// Note: ProcessSignal and its per-signal pipeline helpers
// (acquireDistributedLockWithRetry, handleDuplicateSignal,
// createRemediationRequestCRD) live in signal_ingestion_process.go
// (Wave 6 GREEN: file-size remediation).
