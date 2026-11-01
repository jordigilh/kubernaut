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

package workflowexecution

import (
	"context"
	"crypto/sha256"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// wfeParentFixtureClient supplies the persisted Executing parent phase for
// direct WorkflowExecution E2E fixtures. The WFE controller's production gate
// remains unchanged; the wrapper models the RO handoff that normally creates
// and authorizes the child.
type wfeParentFixtureClient struct {
	client.Client
}

func (c *wfeParentFixtureClient) Create(ctx context.Context, object client.Object, opts ...client.CreateOption) error {
	wfe, ok := object.(*workflowexecutionv1alpha1.WorkflowExecution)
	if !ok || wfe.Spec.RemediationRequestRef.Name == "" {
		return c.Client.Create(ctx, object, opts...)
	}
	if err := ensureWFEParent(ctx, c.Client, wfe); err != nil {
		return fmt.Errorf("seed direct WorkflowExecution parent: %w", err)
	}
	return c.Client.Create(ctx, object, opts...)
}

func ensureWFEParent(ctx context.Context, k8sClient client.Client, wfe *workflowexecutionv1alpha1.WorkflowExecution) error {
	ref := wfe.Spec.RemediationRequestRef
	if ref.Namespace == "" {
		ref.Namespace = wfe.Namespace
		wfe.Spec.RemediationRequestRef.Namespace = ref.Namespace
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(ref.Namespace+"/"+ref.Name)))
	parent := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{Name: ref.Name, Namespace: ref.Namespace},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: fingerprint,
			SignalName:        "workflowexecution-e2e-parent",
			Severity:          "critical",
			SignalType:        "alert",
			TargetType:        "kubernetes",
			TargetResource: remediationv1.ResourceIdentifier{
				Kind:      "Deployment",
				Name:      "workflowexecution-e2e-target",
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
			return fmt.Errorf("read existing WorkflowExecution parent: %w", err)
		}
		if parent.Status.OverallPhase != "" {
			return nil
		}
	}
	parent.Status.OverallPhase = remediationv1.PhaseExecuting
	if err := k8sClient.Status().Update(ctx, parent); err != nil {
		return fmt.Errorf("persist WorkflowExecution parent phase: %w", err)
	}
	return nil
}
