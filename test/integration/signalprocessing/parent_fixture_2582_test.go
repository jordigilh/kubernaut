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

package signalprocessing

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// parentSeedingClient gives existing SP integration fixtures a valid parent
// while keeping the production controller's direct parent gate enabled.
type parentSeedingClient struct {
	client.Client
	parentPhase remediationv1.RemediationPhase
}

func (c *parentSeedingClient) Create(ctx context.Context, object client.Object, opts ...client.CreateOption) error {
	if parent, ok := object.(*remediationv1.RemediationRequest); ok {
		if err := c.Client.Create(ctx, parent, opts...); err != nil {
			return err
		}
		if parent.Status.OverallPhase == "" {
			parent.Status.OverallPhase = c.parentPhase
		}
		if err := c.Status().Update(ctx, parent); err != nil {
			return fmt.Errorf("persist direct SP parent RemediationRequest phase: %w", err)
		}
		return nil
	}
	sp, ok := object.(*signalprocessingv1.SignalProcessing)
	if ok && sp.Spec.RemediationRequestRef.Name != "" && sp.Spec.RemediationRequestRef.Namespace != "" {
		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: sp.Spec.RemediationRequestRef.Name, Namespace: sp.Spec.RemediationRequestRef.Namespace},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				SignalName:        "parent-fixture",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource:    remediationv1.ResourceIdentifier{Kind: "Pod", Name: "parent-fixture-target", Namespace: sp.Spec.RemediationRequestRef.Namespace},
				FiringTime:        metav1.Now(),
				ReceivedTime:      metav1.Now(),
			},
			Status: remediationv1.RemediationRequestStatus{OverallPhase: c.parentPhase},
		}
		if err := c.Client.Create(ctx, parent); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return fmt.Errorf("seed SP parent RemediationRequest: %w", err)
			}
			existing := &remediationv1.RemediationRequest{}
			if err := c.Get(ctx, client.ObjectKeyFromObject(parent), existing); err != nil {
				return fmt.Errorf("read existing SP parent RemediationRequest: %w", err)
			}
			if existing.Status.OverallPhase == "" {
				existing.Status.OverallPhase = c.parentPhase
				if err := c.Status().Update(ctx, existing); err != nil {
					return fmt.Errorf("persist existing SP parent RemediationRequest phase: %w", err)
				}
			}
		} else {
			parent.Status.OverallPhase = c.parentPhase
			if err := c.Client.Status().Update(ctx, parent); err != nil {
				return fmt.Errorf("persist SP parent RemediationRequest phase: %w", err)
			}
		}
	}
	return c.Client.Create(ctx, object, opts...)
}
