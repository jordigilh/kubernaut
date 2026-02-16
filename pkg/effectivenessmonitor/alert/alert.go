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

// Package alert provides the alert resolution scorer for the Effectiveness Monitor.
// It checks whether the original alert that triggered the remediation has resolved
// after the remediation was applied.
//
// Business Requirements:
// - BR-EM-002: Alert resolution check via AlertManager API
//
// Scoring Logic:
//   - 1.0: Original alert has resolved (no longer active in AlertManager)
//   - 0.0: Original alert is still active
//   - nil: AlertManager unavailable or disabled
package alert

import (
	"context"
	"fmt"

	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
)

// AlertContext contains the information needed to check alert resolution.
type AlertContext struct {
	// AlertName is the name of the original alert that triggered remediation.
	AlertName string
	// AlertLabels are the labels of the original alert for precise matching.
	AlertLabels map[string]string
	// Namespace is the namespace of the target resource.
	Namespace string
}

// Scorer evaluates whether the original alert has resolved after remediation.
type Scorer interface {
	// Score checks AlertManager for the original alert and returns a ComponentResult.
	// Returns 1.0 if resolved, 0.0 if still active, nil score if AM unavailable.
	Score(ctx context.Context, amClient client.AlertManagerClient, alertCtx AlertContext) types.ComponentResult
}

// scorer is the concrete implementation of Scorer.
type scorer struct{}

// NewScorer creates a new alert resolution scorer.
func NewScorer() Scorer {
	return &scorer{}
}

// Score checks AlertManager for the original alert.
//
// Scoring Logic (BR-EM-002):
//   - 1.0: Original alert has resolved (no longer active in AlertManager)
//   - 0.0: Original alert is still active
//   - nil: AlertManager unavailable or error
func (s *scorer) Score(ctx context.Context, amClient client.AlertManagerClient, alertCtx AlertContext) types.ComponentResult {
	result := types.ComponentResult{
		Component: types.ComponentAlert,
	}

	// Build alert filters from context
	filters := client.AlertFilters{
		Matchers: buildMatchers(alertCtx),
	}

	// Query AlertManager
	alerts, err := amClient.GetAlerts(ctx, filters)
	if err != nil {
		result.Assessed = false
		result.Score = nil
		result.Error = err
		result.Details = "AlertManager unavailable: " + err.Error()
		return result
	}

	result.Assessed = true

	// Check if any active alerts match
	activeCount := 0
	for _, a := range alerts {
		if a.State == "active" {
			activeCount++
		}
	}

	if activeCount > 0 {
		score := 0.0
		result.Score = &score
		result.Details = fmt.Sprintf("alert %q still active (%d active alerts)", alertCtx.AlertName, activeCount)
	} else {
		score := 1.0
		result.Score = &score
		result.Details = fmt.Sprintf("alert %q resolved", alertCtx.AlertName)
	}

	return result
}

// buildMatchers constructs AlertManager filter matchers from an AlertContext.
func buildMatchers(alertCtx AlertContext) []string {
	matchers := make([]string, 0, len(alertCtx.AlertLabels)+1)
	if alertCtx.AlertName != "" {
		matchers = append(matchers, fmt.Sprintf("alertname=%q", alertCtx.AlertName))
	}
	for k, v := range alertCtx.AlertLabels {
		matchers = append(matchers, fmt.Sprintf("%s=%q", k, v))
	}
	return matchers
}
