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
	"crypto/sha256"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// signalProcessingParentFixtureClient supplies the persisted Processing RR
// phase for direct SignalProcessing E2E fixtures. The production SP parent
// gate remains enabled; this wrapper models the RO handoff before enrichment.
type signalProcessingParentFixtureClient struct {
	client.Client
}

func (c *signalProcessingParentFixtureClient) Create(ctx context.Context, object client.Object, opts ...client.CreateOption) error {
	sp, ok := object.(*signalprocessingv1.SignalProcessing)
	if !ok || sp.Spec.RemediationRequestRef.Name == "" {
		return c.Client.Create(ctx, object, opts...)
	}
	if err := ensureSPParent(ctx, c.Client, sp); err != nil {
		return fmt.Errorf("seed direct SignalProcessing parent: %w", err)
	}
	return c.Client.Create(ctx, object, opts...)
}

func ensureSPParent(ctx context.Context, k8sClient client.Client, sp *signalprocessingv1.SignalProcessing) error {
	ref := sp.Spec.RemediationRequestRef
	if ref.Namespace == "" {
		ref.Namespace = sp.Namespace
		sp.Spec.RemediationRequestRef.Namespace = ref.Namespace
	}
	parent := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{Name: ref.Name, Namespace: ref.Namespace},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: fmt.Sprintf("%x", sha256.Sum256([]byte(ref.Namespace+"/"+ref.Name))),
			SignalName:        "signalprocessing-e2e-parent",
			Severity:          "critical",
			SignalType:        "alert",
			TargetType:        "kubernetes",
			TargetResource: remediationv1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "signalprocessing-e2e-target",
				Namespace: "default",
			},
			FiringTime:   metav1.Now(),
			ReceivedTime: metav1.Now(),
		},
	}
	if err := k8sClient.Create(ctx, parent); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(parent), parent); err != nil {
			return fmt.Errorf("read existing SignalProcessing parent: %w", err)
		}
		if parent.Status.OverallPhase != "" {
			return nil
		}
	}
	parent.Status.OverallPhase = remediationv1.PhaseProcessing
	if err := k8sClient.Status().Update(ctx, parent); err != nil {
		return fmt.Errorf("persist SignalProcessing parent phase: %w", err)
	}
	return nil
}
