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
	"github.com/jordigilh/kubernaut/pkg/shared/backoff" // ADR-052 Addendum 001: Exponential backoff with jitter
	// Issue #753: Dedicated health server
	// Issue #756: FileWatcher for cert rotation
	// Issue #493/#678: Conditional TLS

	// BR-GATEWAY-190: Lease resources for distributed locking

	// BR-GATEWAY-036/037: K8s clientset for TokenReview/SAR

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1" // ADR-068: Federated scope checking factory
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

		start := time.Now()

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
		s.sendSuccessResponse(w, r, response, adapter, start)
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
	start := time.Now()

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
		s.processSingleSignal(ctx, w, r, adapter, signals[0], logger, start)
		return
	}

	// Multi-alert batch: process each signal independently, return 207.
	s.processMultiSignalBatch(ctx, w, r, adapter, signals, logger)
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
	start time.Time,
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

	s.sendSuccessResponse(w, r, response, adapter, start)
}

// processMultiSignalBatch processes multiple signals independently and returns
// HTTP 207 Multi-Status with per-alert results (#1036).
func (s *Server) processMultiSignalBatch(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
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
		if err := ctx.Err(); err != nil {
			results = append(results, ProcessingResult{
				Status:      "failed",
				Fingerprint: signal.Fingerprint,
				Error:       "request timeout exceeded",
			})
			summary.Failed++
			continue
		}

		if valErr := adapter.Validate(signal); valErr != nil {
			logger.Info("Signal validation failed in batch",
				"fingerprint", signal.Fingerprint,
				"error", valErr)
			results = append(results, ProcessingResult{
				Status:      "rejected",
				Fingerprint: signal.Fingerprint,
				Error:       fmt.Sprintf("Signal validation failed: %s", valErr.Error()),
			})
			summary.Rejected++
			continue
		}

		response, procErr := s.ProcessSignal(ctx, signal)
		if procErr != nil {
			logger.Error(procErr, "Signal processing failed in batch",
				"fingerprint", signal.Fingerprint)
			results = append(results, ProcessingResult{
				Status:      "failed",
				Fingerprint: signal.Fingerprint,
				Error:       "Processing failed",
			})
			summary.Failed++
			continue
		}

		result := ProcessingResult{
			Status:      response.Status,
			Fingerprint: response.Fingerprint,
			Message:     response.Message,
		}
		results = append(results, result)

		switch response.Status {
		case StatusCreated:
			summary.Created++
		case StatusDeduplicated:
			summary.Deduplicated++
		case StatusRejected:
			summary.Rejected++
		default:
			summary.Created++
		}
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

// sendSuccessResponse sends the success HTTP response with metrics recording
func (s *Server) sendSuccessResponse(
	w http.ResponseWriter,
	r *http.Request,
	response *ProcessingResponse,
	adapter adapters.SignalAdapter,
	start time.Time,
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

// ProcessSignal implements adapters.SignalProcessor interface.
//
// Main signal processing pipeline orchestrator, called by adapter handlers.
// TDD REFACTOR: Simplified by extracting helper methods.
//
// Pipeline stages:
//  1. Scope validation → validateScope() rejects unmanaged resources
//  2. Optional distributed lock (DD-GATEWAY-013) for multi-replica safety
//  3. Deduplication check → K8s status lookup (DD-GATEWAY-011); if duplicate,
//     update status.deduplication on the existing RemediationRequest and return HTTP 202
//  4. CRD creation → createRemediationRequestCRD() for new signals; return HTTP 201
//
// Note: Environment classification and Priority assignment removed (2025-12-06).
// These are now owned by Signal Processing service per DD-CATEGORIZATION-001.
//
// Performance (order-of-magnitude; varies by cluster and API load):
// - New signal: p95 often ~50-80ms — K8s dedup check, CRD creation (Kubernetes API).
// - Duplicate: p95 often lower — K8s dedup check and status patch; no new CRD.
func (s *Server) ProcessSignal(ctx context.Context, signal *types.NormalizedSignal) (*ProcessingResponse, error) {
	start := time.Now()
	logger := middleware.GetLogger(ctx)

	// Record ingestion metric (environment label removed - SP owns classification)
	s.metricsInstance.AlertsReceivedTotal.WithLabelValues(signal.Source, signal.Severity).Inc()

	// BR-SCOPE-002: Validate resource is within Kubernaut's management scope
	if rejection, err := s.validateScope(ctx, signal); err != nil {
		return nil, err
	} else if rejection != nil {
		return rejection, nil
	}

	// BR-GATEWAY-190: Acquire distributed lock for multi-replica safety
	// DD-GATEWAY-013: K8s Lease-based distributed locking pattern
	// ADR-052 Addendum 001 (Jan 2026): Exponential backoff with jitter (anti-thundering herd)
	if s.lockManager != nil {
		const maxRetries = 10 // 10 retries = ~2.5s total wait with exponential backoff

		// Configure shared backoff with jitter (pkg/shared/backoff)
		// ADR-052 Addendum 001: Use production-proven backoff from Notification v3.1
		backoffConfig := backoff.Config{
			BasePeriod:    100 * time.Millisecond, // Start at 100ms (proven in production)
			MaxPeriod:     1 * time.Second,        // Cap at 1s (faster than 30s lease expiry)
			Multiplier:    2.0,                    // Standard exponential (100ms → 200ms → 400ms → 800ms)
			JitterPercent: 10,                     // ±10% jitter (prevents thundering herd)
		}

		// Iterative retry loop with exponential backoff (replaces unbounded recursion)
		// ADR-052 Addendum 001: Prevents stack overflow risk from recursive retry
		for attempt := int32(1); attempt <= maxRetries; attempt++ {
			acquired, err := s.lockManager.AcquireLock(ctx, signal.Fingerprint)
			if err != nil {
				return nil, fmt.Errorf("distributed lock acquisition failed: %w", err)
			}

			if acquired {
				// Lock acquired - exit retry loop and proceed with normal flow
				break
			}

			// Lock held by another Gateway pod
			logger.V(1).Info("Lock contention, retrying with exponential backoff",
				"attempt", attempt,
				"maxRetries", maxRetries,
				"fingerprint", signal.Fingerprint)

			// Check if we've exhausted all retries (early return for failure case)
			if attempt >= maxRetries {
				// Max retries exceeded - fail immediately
				return nil, fmt.Errorf("lock acquisition timeout after %d attempts (fingerprint: %s)",
					maxRetries, signal.Fingerprint)
			}

			// Exponential backoff with jitter (shared implementation)
			backoffDuration := backoffConfig.Calculate(attempt)
			logger.V(2).Info("Backing off before retry",
				"backoff", backoffDuration,
				"attempt", attempt,
				"fingerprint", signal.Fingerprint)

			time.Sleep(backoffDuration)

			// Retry deduplication check (other pod may have created RR by now)
			// Issue #195: Use controllerNamespace — RRs live in controller NS per ADR-057
			shouldDeduplicate, existingRR, err := s.phaseChecker.ShouldDeduplicate(ctx, s.controllerNamespace, signal.Fingerprint)
			if err != nil {
				return nil, fmt.Errorf("deduplication check failed after lock contention: %w", err)
			}

			if shouldDeduplicate && existingRR != nil {
				// BR-GATEWAY-190: Another pod created RR during lock contention
				// Handle deduplication and return early (no need to continue retry loop)
				return s.handleDuplicateSignal(ctx, signal, existingRR)
			}

			// Still no RR - continue to next retry attempt
		}

		// Lock acquired successfully - ensure it's released after operation
		defer func() {
			if err := s.lockManager.ReleaseLock(ctx, signal.Fingerprint); err != nil {
				logger.Error(err, "Failed to release distributed lock", "fingerprint", signal.Fingerprint)
			}
		}()
	}

	// 1. Deduplication check (DD-GATEWAY-011: K8s status-based, NOT Redis)
	// BR-GATEWAY-185: Redis deprecation - use PhaseBasedDeduplicationChecker
	// Issue #195: Use controllerNamespace — RRs live in controller NS per ADR-057
	shouldDeduplicate, existingRR, err := s.phaseChecker.ShouldDeduplicate(ctx, s.controllerNamespace, signal.Fingerprint)
	if err != nil {
		logger.Error(err, "Deduplication check failed",
			"fingerprint", signal.Fingerprint)
		return nil, fmt.Errorf("deduplication check failed: %w", err)
	}

	if shouldDeduplicate && existingRR != nil {
		// Update status.deduplication (DD-GATEWAY-011)
		// Must be synchronous - HTTP response includes occurrence count
		if err := s.statusUpdater.UpdateDeduplicationStatus(ctx, existingRR); err != nil {
			logger.Info("Failed to update deduplication status (DD-GATEWAY-011)",
				"error", err,
				"fingerprint", signal.Fingerprint,
				"rr", existingRR.Name)
		}

		// Get occurrence count for metrics and logging
		occurrenceCount := int32(1)
		if existingRR.Status.Deduplication != nil {
			occurrenceCount = existingRR.Status.Deduplication.OccurrenceCount
		}

		// Record metrics
		s.metricsInstance.AlertsDeduplicatedTotal.WithLabelValues(signal.SignalName).Inc()

		logger.V(1).Info("Duplicate signal detected (K8s status-based)",
			"fingerprint", signal.Fingerprint,
			"existingRR", existingRR.Name,
			"phase", existingRR.Status.OverallPhase,
			"occurrenceCount", occurrenceCount)

		// DD-AUDIT-003: Emit audit event (BR-GATEWAY-191)
		// Fire-and-forget: audit failures don't affect business logic
		s.emitSignalDeduplicatedAudit(ctx, signal, existingRR.Name, existingRR.Namespace, occurrenceCount)

		// Return duplicate response with data from existing RR
		return NewDuplicateResponseFromRR(signal.Fingerprint, existingRR), nil
	}

	// 2. CRD creation pipeline
	return s.createRemediationRequestCRD(ctx, signal, start)
}

// handleDuplicateSignal handles the case where another pod created a RemediationRequest during lock contention
// TDD REFACTOR: Extracted from ProcessSignal lock retry loop for clarity and testability
//
// BR-GATEWAY-190: Multi-replica deduplication safety
// ADR-052 Addendum 001: This helper is called when exponential backoff retry discovers
// that another Gateway pod successfully acquired the lock and created the RR.
//
// Business Outcome:
//   - Updates occurrence count for deduplication tracking
//   - Records metrics for alert deduplication monitoring
//   - Emits audit event for compliance and debugging
//   - Returns early from retry loop (no need to continue retrying)
//
// Returns:
//   - *ProcessingResponse: Duplicate response with existing RR reference
//   - error: Non-nil if status update or audit emission fails critically
func (s *Server) handleDuplicateSignal(ctx context.Context, signal *types.NormalizedSignal, existingRR *remediationv1alpha1.RemediationRequest) (*ProcessingResponse, error) {
	logger := middleware.GetLogger(ctx)

	// Update occurrence count for deduplication tracking
	if err := s.statusUpdater.UpdateDeduplicationStatus(ctx, existingRR); err != nil {
		// Non-critical: Log and continue (deduplication still succeeded)
		logger.Info("Failed to update deduplication status after lock contention",
			"error", err,
			"fingerprint", signal.Fingerprint,
			"rr", existingRR.Name)
	}

	// Get updated occurrence count for metrics and audit
	occurrenceCount := int32(1)
	if existingRR.Status.Deduplication != nil {
		occurrenceCount = existingRR.Status.Deduplication.OccurrenceCount
	}

	// Record metrics for monitoring dashboard
	s.metricsInstance.AlertsDeduplicatedTotal.WithLabelValues(signal.SignalName).Inc()

	// Emit audit event for compliance (DD-AUDIT-003)
	s.emitSignalDeduplicatedAudit(ctx, signal, existingRR.Name, existingRR.Namespace, occurrenceCount)

	// Return duplicate response (early exit from retry loop)
	return NewDuplicateResponseFromRR(signal.Fingerprint, existingRR), nil
}

// createRemediationRequestCRD handles the CRD creation pipeline
// TDD REFACTOR: Extracted from ProcessSignal for clarity
// Business Outcome: Consistent CRD creation (BR-004)
//
// Note: Environment, Priority, and RemediationPath removed from Gateway (2025-12-06)
// Signal Processing service now owns classification and path decision
// per DD-CATEGORIZATION-001 and DD-WORKFLOW-001 (risk_tolerance in CustomLabels)
func (s *Server) createRemediationRequestCRD(ctx context.Context, signal *types.NormalizedSignal, start time.Time) (*ProcessingResponse, error) {
	logger := middleware.GetLogger(ctx)

	// Create RemediationRequest CRD (classification and path moved to SP)
	rr, err := s.crdCreator.CreateRemediationRequest(ctx, signal)
	if err != nil {
		logger.Error(err, "Failed to create RemediationRequest CRD",
			"fingerprint", signal.Fingerprint)

		// DD-AUDIT-003: Emit crd.creation_failed audit event (DD-AUDIT-003)
		// Fire-and-forget: audit failures don't affect business logic
		s.emitCRDCreationFailedAudit(ctx, signal, err)

		return nil, fmt.Errorf("failed to create RemediationRequest CRD: %w", err)
	}

	// DD-GATEWAY-011: Initialize status.deduplication for NEW CRD
	// Gateway owns status.deduplication per DD-GATEWAY-011
	// Must initialize immediately after creation (OccurrenceCount=1, FirstSeenAt=now)
	if err := s.statusUpdater.UpdateDeduplicationStatus(ctx, rr); err != nil {
		logger.Info("Failed to initialize deduplication status (DD-GATEWAY-011)",
			"error", err,
			"fingerprint", signal.Fingerprint,
			"rr", rr.Name)
		// Non-fatal: CRD exists, status update can be retried by RO or next duplicate
	}

	// DD-GATEWAY-011: Redis deduplication storage DEPRECATED
	// Deduplication now uses K8s status-based lookup (phaseChecker.ShouldDeduplicate)
	// and status updates (statusUpdater.UpdateDeduplicationStatus)
	// Redis is no longer used for deduplication state

	// DD-AUDIT-003: Emit signal.received audit event (BR-GATEWAY-190)
	// Fire-and-forget: audit failures don't affect business logic
	s.emitSignalReceivedAudit(ctx, signal, rr.Name, rr.Namespace)

	// DD-AUDIT-003: Emit crd.created audit event (DD-AUDIT-003)
	// Fire-and-forget: audit failures don't affect business logic
	s.emitCRDCreatedAudit(ctx, signal, rr.Name, rr.Namespace)

	// Record processing duration
	duration := time.Since(start)
	logger.Info("Signal processed successfully",
		"fingerprint", signal.Fingerprint,
		"crdName", rr.Name,
		"duration_ms", duration.Milliseconds())

	return NewCRDCreatedResponse(signal.Fingerprint, rr.Name, rr.Namespace), nil
}
