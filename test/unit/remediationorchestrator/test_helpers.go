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

package remediationorchestrator

import (
	"context"
	"time"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
)

// MockRoutingEngine is a mock implementation for unit tests
// Per DD-RO-002: Routing engine is MANDATORY, so tests must provide mock implementation
type MockRoutingEngine struct{}

func (m *MockRoutingEngine) CheckPreAnalysisConditions(ctx context.Context, rr *remediationv1.RemediationRequest) (*routing.BlockingCondition, error) {
	return nil, nil // Always return not blocked for unit tests
}

func (m *MockRoutingEngine) CheckPostAnalysisConditions(ctx context.Context, rr *remediationv1.RemediationRequest, workflowID string, targetResource string, preRemediationSpecHash string) (*routing.BlockingCondition, error) {
	return nil, nil // Always return not blocked for unit tests
}

func (m *MockRoutingEngine) CheckUnmanagedResource(ctx context.Context, rr *remediationv1.RemediationRequest) *routing.BlockingCondition {
	return nil // Always return managed for unit tests
}

func (m *MockRoutingEngine) Config() routing.Config {
	return routing.Config{
		ConsecutiveFailureThreshold: 3,
		ConsecutiveFailureCooldown:  3600,
		RecentlyRemediatedCooldown:  300,
	}
}

func (m *MockRoutingEngine) CalculateExponentialBackoff(consecutiveFailures int32) time.Duration {
	return time.Duration(consecutiveFailures) * time.Minute
}
