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

// Package detection provides auto-detection of cluster characteristics.
// BR-SP-101: DetectedLabels Auto-Detection
// BR-SP-103: FailedDetections Tracking
// DD-WORKFLOW-001 v2.3: Detection methods documented
package detection

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// LabelDetector auto-detects 8 cluster characteristics from K8s resources.
// TDD RED PHASE: Stub implementation - all methods return nil/empty
type LabelDetector struct {
	client client.Client
	logger logr.Logger
}

// NewLabelDetector creates a new LabelDetector.
// TDD RED PHASE: Stub - returns empty struct
func NewLabelDetector(c client.Client, logger logr.Logger) *LabelDetector {
	return &LabelDetector{
		client: c,
		logger: logger,
	}
}

// DetectLabels detects 8 label types from K8s context.
// TDD RED PHASE: Stub - returns nil (tests will fail)
func (d *LabelDetector) DetectLabels(ctx context.Context, k8sCtx *sharedtypes.KubernetesContext, ownerChain []sharedtypes.OwnerChainEntry) *sharedtypes.DetectedLabels {
	// TDD RED PHASE: Return nil to make tests fail
	return nil
}

