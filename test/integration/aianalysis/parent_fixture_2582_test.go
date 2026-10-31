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

package aianalysis

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// parentSeedingClient gives existing AIA integration fixtures a valid persisted
// parent while keeping the production reconciler's fresh-read gate enabled.
// Tests that create a parent first retain its explicit status and are not
// overwritten.
type parentSeedingClient struct {
	client.Client
	parentPhase remediationv1.RemediationPhase
}

func (c *parentSeedingClient) Create(ctx context.Context, object client.Object, opts ...client.CreateOption) error {
	analysis, ok := object.(*aianalysisv1.AIAnalysis)
	if ok && analysis.Spec.RemediationRequestRef.Name != "" && analysis.Spec.RemediationRequestRef.Namespace != "" {
		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      analysis.Spec.RemediationRequestRef.Name,
				Namespace: analysis.Spec.RemediationRequestRef.Namespace,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				SignalName:        "parent-fixture",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind: "Pod", Name: "parent-fixture-target", Namespace: analysis.Spec.RemediationRequestRef.Namespace,
				},
				FiringTime:   metav1.Now(),
				ReceivedTime: metav1.Now(),
			},
			Status: remediationv1.RemediationRequestStatus{OverallPhase: c.parentPhase},
		}
		if err := c.Client.Create(ctx, parent); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return fmt.Errorf("seed AIA parent RemediationRequest: %w", err)
			}
		} else if err := c.Client.Status().Update(ctx, parent); err != nil {
			return fmt.Errorf("persist AIA parent RemediationRequest phase: %w", err)
		}
	}
	return c.Client.Create(ctx, object, opts...)
}
