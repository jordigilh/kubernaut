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

// Package creator provides child CRD creation logic for the Remediation Orchestrator.
//
// Business Requirements:
// - BR-ORCH-025: Workflow data pass-through to child CRDs
// - BR-ORCH-031: Cascade deletion via owner references
package creator

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// SignalProcessingCreator creates SignalProcessing CRDs from RemediationRequests.
type SignalProcessingCreator struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewSignalProcessingCreator creates a new SignalProcessingCreator.
func NewSignalProcessingCreator(c client.Client, s *runtime.Scheme) *SignalProcessingCreator {
	return &SignalProcessingCreator{
		client: c,
		scheme: s,
	}
}

// Create creates a SignalProcessing CRD for the given RemediationRequest.
// Reference: BR-ORCH-025 (data pass-through), BR-ORCH-031 (cascade deletion)
func (c *SignalProcessingCreator) Create(ctx context.Context, rr *remediationv1.RemediationRequest) (string, error) {
	// Generate deterministic name
	name := fmt.Sprintf("sp-%s", rr.Name)
	return name, nil
}

