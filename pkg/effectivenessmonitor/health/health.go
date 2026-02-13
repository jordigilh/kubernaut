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

// Package health provides the health check scorer for the Effectiveness Monitor.
// It evaluates the Kubernetes readiness/liveness state of the target resource
// after remediation to determine if the resource is healthy.
//
// Business Requirements:
// - BR-EM-001: Health check via K8s readiness/liveness status
//
// Scoring Logic:
//   - 1.0: All pods ready, no restarts since remediation
//   - 0.75: All pods ready, but restarts detected since remediation
//   - 0.5: Partial readiness (some pods ready, some not)
//   - 0.0: No pods ready or target resource not found
package health

import (
	"context"
	"fmt"

	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
)

// TargetStatus represents the health state of a Kubernetes target resource.
type TargetStatus struct {
	// TotalReplicas is the total number of desired replicas.
	TotalReplicas int32
	// ReadyReplicas is the number of replicas that are ready.
	ReadyReplicas int32
	// RestartsSinceRemediation is the total restart count since remediation started.
	RestartsSinceRemediation int32
	// TargetExists indicates whether the target resource was found.
	TargetExists bool
}

// Scorer evaluates the health of a target resource and produces a score.
// Implementation uses K8s API to check pod readiness/liveness.
type Scorer interface {
	// Score evaluates the health of the target resource and returns a ComponentResult.
	// The score is based on pod readiness, restart counts, and target existence.
	Score(ctx context.Context, status TargetStatus) types.ComponentResult
}

// scorer is the concrete implementation of Scorer.
type scorer struct{}

// NewScorer creates a new health check scorer.
func NewScorer() Scorer {
	return &scorer{}
}

// Score evaluates the health of the target resource.
//
// Scoring Logic (BR-EM-001):
//   - 1.0: All pods ready, no restarts since remediation
//   - 0.75: All pods ready, but restarts detected since remediation
//   - 0.5: Partial readiness (some pods ready, some not)
//   - 0.0: No pods ready or target resource not found
func (s *scorer) Score(_ context.Context, status TargetStatus) types.ComponentResult {
	result := types.ComponentResult{
		Component: types.ComponentHealth,
		Assessed:  true,
	}

	// Target not found -> 0.0
	if !status.TargetExists {
		score := 0.0
		result.Score = &score
		result.Details = "target resource not found"
		return result
	}

	// Zero total replicas (scaled down) -> 0.0
	if status.TotalReplicas == 0 {
		score := 0.0
		result.Score = &score
		result.Details = "target has 0 desired replicas"
		return result
	}

	// No pods ready -> 0.0
	if status.ReadyReplicas == 0 {
		score := 0.0
		result.Score = &score
		result.Details = fmt.Sprintf("0/%d pods ready", status.TotalReplicas)
		return result
	}

	// Partial readiness -> 0.5
	if status.ReadyReplicas < status.TotalReplicas {
		score := 0.5
		result.Score = &score
		result.Details = fmt.Sprintf("%d/%d pods ready (partial)", status.ReadyReplicas, status.TotalReplicas)
		return result
	}

	// All pods ready
	if status.RestartsSinceRemediation > 0 {
		// All ready but restarts detected -> 0.75
		score := 0.75
		result.Score = &score
		result.Details = fmt.Sprintf("all %d pods ready, %d restarts since remediation",
			status.TotalReplicas, status.RestartsSinceRemediation)
		return result
	}

	// All pods ready, no restarts -> 1.0
	score := 1.0
	result.Score = &score
	result.Details = fmt.Sprintf("all %d pods ready, no restarts", status.TotalReplicas)
	return result
}
