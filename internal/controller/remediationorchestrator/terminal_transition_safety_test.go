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

package controller

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// terminalWinnerClient simulates a resourceVersion conflict where another
// reconcile commits a terminal outcome before the failure transition retries.
// The production safety contract is that the winning terminal phase remains
// authoritative and no failure side effect is emitted.
type terminalWinnerClient struct {
	client.Client
	injected bool
}

func (c *terminalWinnerClient) Status() client.SubResourceWriter {
	return &terminalWinnerStatusWriter{
		SubResourceWriter: c.Client.Status(),
		client:            c,
	}
}

type terminalWinnerStatusWriter struct {
	client.SubResourceWriter
	client *terminalWinnerClient
}

func (w *terminalWinnerStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if !w.client.injected {
		if _, ok := obj.(*remediationv1.RemediationRequest); ok {
			w.client.injected = true
			current := &remediationv1.RemediationRequest{}
			if err := w.client.Get(ctx, client.ObjectKeyFromObject(obj), current); err != nil {
				return err
			}
			current.Status.OverallPhase = remediationv1.PhaseCompleted
			if err := w.client.Client.Status().Update(ctx, current); err != nil {
				return err
			}
			return apierrors.NewConflict(
				schema.GroupResource{Group: remediationv1.GroupVersion.Group, Resource: "remediationrequests"},
				obj.GetName(),
				errors.New("concurrent terminal transition won"),
			)
		}
	}
	return w.SubResourceWriter.Update(ctx, obj, opts...)
}

var _ = Describe("RAR approval safety terminal transition races", func() {
	It("UT-RO-2564-021: preserves a winning terminal phase after failure status conflict", func() {
		ctx := context.Background()
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())

		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "rr-terminal-winner",
				Namespace:         "default",
				CreationTimestamp: metav1.Now(),
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "terminal-winner-fingerprint",
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseAwaitingApproval,
			},
		}
		baseClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			WithObjects(rr).
			Build()
		conflictClient := &terminalWinnerClient{Client: baseClient}

		reconciler := NewReconciler(ReconcilerDeps{
			Client:     conflictClient,
			APIReader:  baseClient,
			Scheme:     scheme,
			Metrics:    rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			Recorder:   record.NewFakeRecorder(10),
			Timeouts:   TimeoutConfig{},
			AuditStore: nil,
		})

		current := &remediationv1.RemediationRequest{}
		Expect(baseClient.Get(ctx, client.ObjectKeyFromObject(rr), current)).To(Succeed())
		_, err := reconciler.transitionToFailed(ctx, current, remediationv1.FailurePhaseApproval, errors.New("approval collision"))
		Expect(err).NotTo(HaveOccurred())

		finalRR := &remediationv1.RemediationRequest{}
		Expect(baseClient.Get(ctx, client.ObjectKeyFromObject(rr), finalRR)).To(Succeed())
		Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhaseCompleted))
		Expect(finalRR.Status.EnsureCompletionStatus().FailurePhase).To(BeNil())

		notifications := &notificationv1.NotificationRequestList{}
		Expect(baseClient.List(ctx, notifications)).To(Succeed())
		Expect(notifications.Items).To(BeEmpty(), "a losing failure transition must not emit an escalation side effect")
	})
})
