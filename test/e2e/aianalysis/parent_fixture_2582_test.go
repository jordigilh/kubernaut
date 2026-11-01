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

// parentSeedingClient keeps direct E2E AIAnalysis fixtures aligned with the
// production RR -> AIA handoff. The production AIA parent gate remains active;
// this wrapper only supplies the parent object that RO would create.
type parentSeedingClient struct {
	client.Client
	parentPhase remediationv1.RemediationPhase
}

func (c *parentSeedingClient) Create(ctx context.Context, object client.Object, opts ...client.CreateOption) error {
	if parent, ok := object.(*remediationv1.RemediationRequest); ok {
		return c.createAuthorizedParent(ctx, parent, opts...)
	}

	analysis, ok := object.(*aianalysisv1.AIAnalysis)
	if ok && analysis.Spec.RemediationRequestRef.Name != "" {
		if analysis.Spec.RemediationRequestRef.Namespace == "" {
			analysis.Spec.RemediationRequestRef.Namespace = analysis.Namespace
		}
		parent := parentForAIAnalysis(analysis)
		if err := c.createAuthorizedParent(ctx, parent); err != nil {
			return fmt.Errorf("seed AIA E2E parent RemediationRequest: %w", err)
		}
	}

	return c.Client.Create(ctx, object, opts...)
}

func (c *parentSeedingClient) createAuthorizedParent(ctx context.Context, parent *remediationv1.RemediationRequest, opts ...client.CreateOption) error {
	if err := c.Client.Create(ctx, parent, opts...); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		existing := &remediationv1.RemediationRequest{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(parent), existing); err != nil {
			return fmt.Errorf("read existing AIA E2E parent RemediationRequest: %w", err)
		}
		if existing.Status.OverallPhase == "" {
			existing.Status.OverallPhase = c.parentPhase
			if err := c.Status().Update(ctx, existing); err != nil {
				return fmt.Errorf("persist existing AIA E2E parent phase: %w", err)
			}
		}
		return nil
	}

	// Status is a subresource; assign the phase after Create so a create
	// response cannot clear the in-memory status before this write.
	parent.Status.OverallPhase = c.parentPhase
	if err := c.Status().Update(ctx, parent); err != nil {
		return fmt.Errorf("persist AIA E2E parent phase: %w", err)
	}
	return nil
}

func parentForAIAnalysis(analysis *aianalysisv1.AIAnalysis) *remediationv1.RemediationRequest {
	ref := analysis.Spec.RemediationRequestRef
	signal := analysis.Spec.AnalysisRequest.SignalContext
	targetNamespace := signal.TargetResource.Namespace
	if targetNamespace == "" {
		targetNamespace = ref.Namespace
	}
	targetKind := signal.TargetResource.Kind
	if targetKind == "" {
		targetKind = "Pod"
	}
	targetName := signal.TargetResource.Name
	if targetName == "" {
		targetName = "aianalysis-e2e-target"
	}
	severity := signal.Severity
	if severity == "" {
		severity = "critical"
	}
	signalName := signal.SignalName
	if signalName == "" {
		signalName = "aianalysis-e2e-parent"
	}

	return &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{Name: ref.Name, Namespace: ref.Namespace},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SignalName:        signalName,
			Severity:          severity,
			SignalType:        "alert",
			TargetType:        "kubernetes",
			TargetResource: remediationv1.ResourceIdentifier{
				Kind:      targetKind,
				Name:      targetName,
				Namespace: targetNamespace,
			},
			FiringTime:   metav1.Now(),
			ReceivedTime: metav1.Now(),
		},
		Status: remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseAnalyzing},
	}
}
