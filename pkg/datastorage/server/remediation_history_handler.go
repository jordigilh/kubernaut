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
// DD-HAPI-016 v1.1: Two-step query pattern with EM scoring infrastructure.
//
// This handler orchestrates the full remediation history query flow:
//  1. Parse and validate required query parameters
//  2. Query RO events by target resource (Tier 1)
//  3. Batch query EM component events by correlation_id
//  4. Correlate into detailed Tier 1 entries
//  5. Detect regression (preRemediation hash match)
//  6. If regression: query Tier 2 by spec hash
//  7. Return JSON response
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/response"
)

// Default lookback windows per DD-HAPI-016 v1.1.
const (
	defaultTier1Window = 24 * time.Hour        // 24 hours
	defaultTier2Window = 2160 * time.Hour      // 90 days
)

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
	q := r.URL.Query()

	// Step 1: Parse and validate required parameters
	targetKind := q.Get("targetKind")
	targetName := q.Get("targetName")
	targetNamespace := q.Get("targetNamespace")
	currentSpecHash := q.Get("currentSpecHash")

	if targetKind == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest,
			"missing-parameter", "Missing Required Parameter",
			"targetKind query parameter is required", h.logger)
		return
	}
	if targetName == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest,
			"missing-parameter", "Missing Required Parameter",
			"targetName query parameter is required", h.logger)
		return
	}
	if targetNamespace == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest,
			"missing-parameter", "Missing Required Parameter",
			"targetNamespace query parameter is required", h.logger)
		return
	}
	if currentSpecHash == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest,
			"missing-parameter", "Missing Required Parameter",
			"currentSpecHash query parameter is required", h.logger)
		return
	}

	// Step 2: Parse optional window durations
	tier1Window := defaultTier1Window
	if v := q.Get("tier1Window"); v != "" {
		parsed, err := time.ParseDuration(v)
		if err != nil {
			response.WriteRFC7807Error(w, http.StatusBadRequest,
				"invalid-parameter", "Invalid Parameter",
				fmt.Sprintf("tier1Window must be a valid Go duration string: %s", err), h.logger)
			return
		}
		tier1Window = parsed
	}

	tier2Window := defaultTier2Window
	if v := q.Get("tier2Window"); v != "" {
		parsed, err := time.ParseDuration(v)
		if err != nil {
			response.WriteRFC7807Error(w, http.StatusBadRequest,
				"invalid-parameter", "Invalid Parameter",
				fmt.Sprintf("tier2Window must be a valid Go duration string: %s", err), h.logger)
			return
		}
		tier2Window = parsed
	}

	// Build target resource string: "{namespace}/{kind}/{name}"
	targetResource := fmt.Sprintf("%s/%s/%s", targetNamespace, targetKind, targetName)
	now := time.Now()
	ctx := r.Context()

	h.logger.V(1).Info("Querying remediation history context",
		"target_resource", targetResource,
		"current_spec_hash", currentSpecHash,
		"tier1_window", tier1Window.String(),
		"tier2_window", tier2Window.String())

	// Step 3: Query Tier 1 RO events by target resource (DD-HAPI-016 v1.1 Step 1)
	tier1Since := now.Add(-tier1Window)
	roEvents, err := h.remediationHistoryRepo.QueryROEventsByTarget(ctx, targetResource, tier1Since)
	if err != nil {
		h.logger.Error(err, "Failed to query RO events",
			"target_resource", targetResource, "since", tier1Since)
		response.WriteRFC7807Error(w, http.StatusInternalServerError,
			"query-error", "Internal Server Error",
			"Failed to query remediation history", h.logger)
		return
	}

	// Step 4: Batch query EM events by correlation_id (DD-HAPI-016 v1.1 Step 2)
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
			return
		}
	}

	// Step 5: Correlate into Tier 1 entries (DD-HAPI-016 v1.1 Step 3)
	tier1Entries := CorrelateTier1Chain(roEvents, emEvents, currentSpecHash)

	// Step 6: Detect regression (DD-HAPI-016 v1.1: preRemediation hash match)
	regressionDetected := DetectRegression(tier1Entries)

	// Step 7: If regression, query Tier 2 by spec hash (DD-HAPI-016 v1.1 Step 4)
	var tier2Summaries []api.RemediationHistorySummary
	if regressionDetected {
		h.logger.Info("Regression detected, querying Tier 2 history",
			"target_resource", targetResource,
			"current_spec_hash", currentSpecHash)

		tier2Since := now.Add(-tier2Window)
		tier2RO, err := h.remediationHistoryRepo.QueryROEventsBySpecHash(ctx, currentSpecHash, tier2Since, tier1Since)
		if err != nil {
			// Non-fatal: Tier 2 is supplementary context. Log and continue with empty Tier 2.
			h.logger.Error(err, "Failed to query Tier 2 events (non-fatal, continuing with empty Tier 2)",
				"spec_hash", currentSpecHash, "since", tier2Since, "until", tier1Since)
		} else if len(tier2RO) > 0 {
			// Batch query EM events for Tier 2
			t2CorrelationIDs := make([]string, 0, len(tier2RO))
			for _, ro := range tier2RO {
				t2CorrelationIDs = append(t2CorrelationIDs, ro.CorrelationID)
			}
			t2EMEvents, err := h.remediationHistoryRepo.QueryEffectivenessEventsBatch(ctx, t2CorrelationIDs)
			if err != nil {
				h.logger.Error(err, "Failed to query Tier 2 EM events (non-fatal)",
					"correlation_id_count", len(t2CorrelationIDs))
			}
			tier2Summaries = BuildTier2Summaries(tier2RO, t2EMEvents, currentSpecHash)
		}
	}

	// Build response
	// Ensure non-nil slices for JSON serialization ([] not null)
	if tier1Entries == nil {
		tier1Entries = []api.RemediationHistoryEntry{}
	}
	if tier2Summaries == nil {
		tier2Summaries = []api.RemediationHistorySummary{}
	}

	h.logger.Info("Remediation history context served",
		"target_resource", targetResource,
		"tier1_count", len(tier1Entries),
		"tier2_count", len(tier2Summaries),
		"regression_detected", regressionDetected)

	resp := map[string]interface{}{
		"targetResource":     targetResource,
		"currentSpecHash":    currentSpecHash,
		"regressionDetected": regressionDetected,
		"tier1": map[string]interface{}{
			"window": tier1Window.String(),
			"chain":  tier1Entries,
		},
		"tier2": map[string]interface{}{
			"window": tier2Window.String(),
			"chain":  tier2Summaries,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error(err, "Failed to encode remediation history response",
			"target_resource", targetResource)
	}
}
