/*
Copyright 2026 Jordi Gil.

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

// HTTP handler for GET /api/v1/remediation-history/context.
//
// BR-HAPI-016: Remediation history context for LLM prompt enrichment.
// DD-HAPI-016 v1.4: Both tiers query by spec hash for causal chain integrity.
//
// This handler orchestrates the full remediation history query flow:
//  1. Parse and validate required query parameters
//  2. Query RO events by spec hash (Tier 1)
//  3. Batch query EM component events by correlation_id
//  4. Correlate into detailed Tier 1 entries
//  5. Detect regression (preRemediation hash match)
//  6. If regression: query Tier 2 by spec hash
//  7. Return JSON response
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/response"
)

// Default lookback windows per DD-HAPI-016 v1.1.
const (
	defaultTier1Window = 24 * time.Hour   // 24 hours
	defaultTier2Window = 2160 * time.Hour // 90 days
)

// remediationHistoryRequest holds the parsed and validated request
// parameters for HandleGetRemediationHistoryContext.
type remediationHistoryRequest struct {
	targetResource  string
	currentSpecHash string
	tier1Window     time.Duration
	tier2Window     time.Duration
}

// HandleGetRemediationHistoryContext handles GET /api/v1/remediation-history/context.
//
// Query Parameters (required):
//   - targetKind: Kubernetes resource kind (e.g., Deployment)
//   - targetName: Kubernetes resource name
//   - targetNamespace: Kubernetes resource namespace
//   - currentSpecHash: SHA-256 hash of the current target resource spec
//
// Query Parameters (optional):
//   - tier1Window: Tier 1 lookback duration (default "24h")
//   - tier2Window: Tier 2 lookback duration (default "2160h")
func (h *Handler) HandleGetRemediationHistoryContext(w http.ResponseWriter, r *http.Request) {
	req, ok := h.parseRemediationHistoryRequest(w, r)
	if !ok {
		return
	}

	now := time.Now()
	ctx := r.Context()

	h.logger.V(1).Info("Querying remediation history context",
		"target_resource", req.targetResource,
		"current_spec_hash", req.currentSpecHash,
		"tier1_window", req.tier1Window.String(),
		"tier2_window", req.tier2Window.String())

	tier1Entries, tier1Since, ok := h.queryTier1History(w, ctx, req, now)
	if !ok {
		return
	}

	// DD-HAPI-016 v1.1: preRemediation hash match detects regression from Tier 1.
	regressionDetected := DetectRegression(tier1Entries)

	// GAP-DS-1: Tier 2 runs regardless of Tier 1 results — regression can be
	// detected from Tier 2 alone when Tier 1 is empty but historical events exist.
	tier2Summaries := h.queryTier2History(ctx, req, now, tier1Since)
	if !regressionDetected && len(tier2Summaries) > 0 {
		regressionDetected = DetectRegressionFromTier2(tier2Summaries)
	}

	// Ensure non-nil slices for JSON serialization ([] not null)
	if tier1Entries == nil {
		tier1Entries = []api.RemediationHistoryEntry{}
	}
	if tier2Summaries == nil {
		tier2Summaries = []api.RemediationHistorySummary{}
	}

	h.logger.Info("Remediation history context served",
		"target_resource", req.targetResource,
		"tier1_count", len(tier1Entries),
		"tier2_count", len(tier2Summaries),
		"regression_detected", regressionDetected)

	h.writeRemediationHistoryResponse(w, req, tier1Entries, tier2Summaries, regressionDetected)
}

// parseRemediationHistoryRequest parses and validates the required/optional
// query parameters for HandleGetRemediationHistoryContext, writing an
// RFC 7807 error response and returning ok=false on any validation failure.
// Extracted from HandleGetRemediationHistoryContext
// (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3) — pure code motion, no behavior
// change.
func (h *Handler) parseRemediationHistoryRequest(w http.ResponseWriter, r *http.Request) (remediationHistoryRequest, bool) {
	q := r.URL.Query()

	targetKind := q.Get("targetKind")
	targetName := q.Get("targetName")
	targetNamespace := q.Get("targetNamespace")
	currentSpecHash := q.Get("currentSpecHash")

	if targetKind == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest,
			"missing-parameter", "Missing Required Parameter",
			"targetKind query parameter is required", h.logger)
		return remediationHistoryRequest{}, false
	}
	if targetName == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest,
			"missing-parameter", "Missing Required Parameter",
			"targetName query parameter is required", h.logger)
		return remediationHistoryRequest{}, false
	}
	// targetNamespace may be empty for cluster-scoped resources (#762)
	if currentSpecHash == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest,
			"missing-parameter", "Missing Required Parameter",
			"currentSpecHash query parameter is required", h.logger)
		return remediationHistoryRequest{}, false
	}

	tier1Window, ok := h.parseHistoryWindow(w, q, "tier1Window", defaultTier1Window)
	if !ok {
		return remediationHistoryRequest{}, false
	}
	tier2Window, ok := h.parseHistoryWindow(w, q, "tier2Window", defaultTier2Window)
	if !ok {
		return remediationHistoryRequest{}, false
	}

	if detail, ok := validateWindows(tier1Window, tier2Window); !ok {
		response.WriteRFC7807Error(w, http.StatusBadRequest,
			"invalid-parameter", "Invalid Parameter", detail, h.logger)
		return remediationHistoryRequest{}, false
	}

	// Build target resource string: "{namespace}/{kind}/{name}" or "{kind}/{name}" for cluster-scoped (#762)
	var targetResource string
	if targetNamespace != "" {
		targetResource = fmt.Sprintf("%s/%s/%s", targetNamespace, targetKind, targetName)
	} else {
		targetResource = fmt.Sprintf("%s/%s", targetKind, targetName)
	}

	return remediationHistoryRequest{
		targetResource:  targetResource,
		currentSpecHash: currentSpecHash,
		tier1Window:     tier1Window,
		tier2Window:     tier2Window,
	}, true
}

// parseHistoryWindow parses an optional duration query parameter, falling
// back to defaultValue when absent, and writing an RFC 7807 error (returning
// ok=false) when present but not a valid duration string.
func (h *Handler) parseHistoryWindow(w http.ResponseWriter, q url.Values, param string, defaultValue time.Duration) (time.Duration, bool) {
	v := q.Get(param)
	if v == "" {
		return defaultValue, true
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		response.WriteRFC7807Error(w, http.StatusBadRequest,
			"invalid-parameter", "Invalid Parameter",
			fmt.Sprintf("%s must be a valid duration string (e.g. '24h', '7d')", param), h.logger)
		return 0, false
	}
	return parsed, true
}

// queryTier1History executes DD-HAPI-016 v1.4 Tier 1 query steps: query RO
// events by spec hash (Issue #586), batch query EM events by correlation_id,
// and correlate into detailed Tier 1 entries. Returns ok=false (after writing
// the RFC 7807 error response) on any query failure. Extracted from
// HandleGetRemediationHistoryContext (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3)
// — pure code motion, no behavior change.
func (h *Handler) queryTier1History(w http.ResponseWriter, ctx context.Context, req remediationHistoryRequest, now time.Time) ([]api.RemediationHistoryEntry, time.Time, bool) {
	tier1Since := now.Add(-req.tier1Window)
	roEvents, err := h.remediationHistoryRepo.QueryROEventsBySpecHash(ctx, req.currentSpecHash, tier1Since, now)
	if err != nil {
		h.logger.Error(err, "Failed to query Tier 1 RO events",
			"spec_hash", req.currentSpecHash, "since", tier1Since, "until", now)
		response.WriteRFC7807Error(w, http.StatusInternalServerError,
			"query-error", "Internal Server Error",
			"Failed to query remediation history", h.logger)
		return nil, tier1Since, false
	}

	var emEvents map[string][]*EffectivenessEvent
	if len(roEvents) > 0 {
		correlationIDs := make([]string, 0, len(roEvents))
		for _, ro := range roEvents {
			correlationIDs = append(correlationIDs, ro.CorrelationID)
		}

		emEvents, err = h.remediationHistoryRepo.QueryEffectivenessEventsBatch(ctx, correlationIDs)
		if err != nil {
			h.logger.Error(err, "Failed to query EM events",
				"correlation_id_count", len(correlationIDs))
			response.WriteRFC7807Error(w, http.StatusInternalServerError,
				"query-error", "Internal Server Error",
				"Failed to query effectiveness events", h.logger)
			return nil, tier1Since, false
		}
	}

	return CorrelateTier1Chain(roEvents, emEvents, req.currentSpecHash), tier1Since, true
}

// queryTier2History executes the GAP-DS-1 Tier 2 query: always query by spec
// hash regardless of Tier 1 results, batch query EM events, and build
// summaries. Errors are non-fatal (logged, continuing with an empty Tier 2)
// since Tier 2 is supplementary context. Extracted from
// HandleGetRemediationHistoryContext (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3)
// — pure code motion, no behavior change.
func (h *Handler) queryTier2History(ctx context.Context, req remediationHistoryRequest, now, tier1Since time.Time) []api.RemediationHistorySummary {
	tier2Since := now.Add(-req.tier2Window)
	tier2RO, err := h.remediationHistoryRepo.QueryROEventsBySpecHash(ctx, req.currentSpecHash, tier2Since, tier1Since)
	if err != nil {
		h.logger.Error(err, "Failed to query Tier 2 events (non-fatal, continuing with empty Tier 2)",
			"spec_hash", req.currentSpecHash, "since", tier2Since, "until", tier1Since)
		return nil
	}
	if len(tier2RO) == 0 {
		return nil
	}

	t2CorrelationIDs := make([]string, 0, len(tier2RO))
	for _, ro := range tier2RO {
		t2CorrelationIDs = append(t2CorrelationIDs, ro.CorrelationID)
	}
	t2EMEvents, err := h.remediationHistoryRepo.QueryEffectivenessEventsBatch(ctx, t2CorrelationIDs)
	if err != nil {
		h.logger.Error(err, "Failed to query Tier 2 EM events (non-fatal)",
			"correlation_id_count", len(t2CorrelationIDs))
	}
	return BuildTier2Summaries(tier2RO, t2EMEvents, req.currentSpecHash)
}

// writeRemediationHistoryResponse encodes and writes the JSON response body
// for HandleGetRemediationHistoryContext. Extracted from
// HandleGetRemediationHistoryContext (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3)
// — pure code motion, no behavior change.
func (h *Handler) writeRemediationHistoryResponse(w http.ResponseWriter, req remediationHistoryRequest, tier1Entries []api.RemediationHistoryEntry, tier2Summaries []api.RemediationHistorySummary, regressionDetected bool) {
	resp := map[string]interface{}{
		"targetResource":     req.targetResource,
		"currentSpecHash":    req.currentSpecHash,
		"regressionDetected": regressionDetected,
		"tier1": map[string]interface{}{
			"window": req.tier1Window.String(),
			"chain":  tier1Entries,
		},
		"tier2": map[string]interface{}{
			"window": req.tier2Window.String(),
			"chain":  tier2Summaries,
		},
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(resp); err != nil {
		h.logger.Error(err, "Failed to encode remediation history response",
			"target_resource", req.targetResource)
		response.WriteRFC7807Error(w, http.StatusInternalServerError,
			"encoding-error", "Internal Server Error",
			"Failed to serialize remediation history response", h.logger)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// validateWindows checks that tier1Window and tier2Window are positive and that
// tier2Window is strictly greater than tier1Window. Returns the error detail
// string and false if validation fails.
func validateWindows(tier1, tier2 time.Duration) (string, bool) {
	if tier1 <= 0 {
		return "tier1Window must be a positive duration", false
	}
	if tier2 <= 0 {
		return "tier2Window must be a positive duration", false
	}
	if tier2 <= tier1 {
		return "tier2Window must be greater than tier1Window", false
	}
	return "", true
}
