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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
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

// sameNameReplacementClient replaces an RR between a failed status write and
// the retry refetch. The retry must reject the replacement by UID instead of
// applying the stale failure transition to the new object.
type sameNameReplacementClient struct {
	client.Client
	replaced bool
}

func (c *sameNameReplacementClient) Status() client.SubResourceWriter {
	return &sameNameReplacementStatusWriter{
		SubResourceWriter: c.Client.Status(),
		client:            c,
	}
}

type sameNameReplacementStatusWriter struct {
	client.SubResourceWriter
	client *sameNameReplacementClient
}

func (w *sameNameReplacementStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if !w.client.replaced {
		if _, ok := obj.(*remediationv1.RemediationRequest); ok {
			w.client.replaced = true
			current := &remediationv1.RemediationRequest{}
			if err := w.client.Get(ctx, client.ObjectKeyFromObject(obj), current); err != nil {
				return err
			}
			if err := w.client.Delete(ctx, current); err != nil {
				return err
			}
			replacement := current.DeepCopy()
			replacement.UID = types.UID("rr-replacement")
			replacement.ResourceVersion = ""
			replacement.Status.OverallPhase = remediationv1.PhaseAwaitingApproval
			if err := w.client.Create(ctx, replacement); err != nil {
				return err
			}
			return apierrors.NewConflict(
				schema.GroupResource{Group: remediationv1.GroupVersion.Group, Resource: "remediationrequests"},
				obj.GetName(),
				errors.New("same-name replacement won concurrent failure transition"),
			)
		}
	}
	return w.SubResourceWriter.Update(ctx, obj, opts...)
}

func newTerminalSafetyReconciler(c client.Client, apiReader client.Reader, scheme *runtime.Scheme) *Reconciler {
	return NewReconciler(ReconcilerDeps{
		Client:    c,
		APIReader: apiReader,
		Scheme:    scheme,
		Metrics:   rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
		Recorder:  record.NewFakeRecorder(10),
		Timeouts:  TimeoutConfig{},
	})
}

func terminalSafetyRemediationRequest(name string, uid types.UID) *remediationv1.RemediationRequest {
	return &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			UID:               uid,
			CreationTimestamp: metav1.Now(),
		},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: "terminal-winner-fingerprint",
		},
		Status: remediationv1.RemediationRequestStatus{
			OverallPhase: remediationv1.PhaseAwaitingApproval,
		},
	}
}

func terminalSafetyRAR(parent *remediationv1.RemediationRequest) *remediationv1.RemediationApprovalRequest {
	controller := true
	blockOwnerDeletion := true
	return &remediationv1.RemediationApprovalRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rar-" + parent.Name,
			Namespace: parent.Namespace,
			Finalizers: []string{
				creator.ApprovalRequestFinalizerName,
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         remediationv1.GroupVersion.String(),
				Kind:               "RemediationRequest",
				Name:               parent.Name,
				UID:                parent.UID,
				Controller:         &controller,
				BlockOwnerDeletion: &blockOwnerDeletion,
			}},
		},
		Spec: remediationv1.RemediationApprovalRequestSpec{
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       parent.Name,
				Namespace:  parent.Namespace,
				UID:        parent.UID,
			},
		},
	}
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

		reconciler := newTerminalSafetyReconciler(conflictClient, baseClient, scheme)

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

	It("UT-RO-2564-022: retains the RAR finalizer when the authoritative parent UID was replaced", func() {
		ctx := context.Background()
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())

		original := terminalSafetyRemediationRequest("rr-parent-replaced", types.UID("rr-original"))
		rar := terminalSafetyRAR(original)
		baseClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			WithObjects(original, rar).
			Build()
		staleParent := original.DeepCopy()

		Expect(baseClient.Delete(ctx, original)).To(Succeed())
		replacement := original.DeepCopy()
		replacement.UID = types.UID("rr-replacement")
		replacement.ResourceVersion = ""
		Expect(baseClient.Create(ctx, replacement)).To(Succeed())
		Expect(baseClient.Delete(ctx, rar)).To(Succeed())

		reconciler := newTerminalSafetyReconciler(baseClient, baseClient, scheme)
		_, handled, err := reconciler.reconcileApprovalRequestDeletion(ctx, staleParent, logr.Discard())
		Expect(handled).To(BeTrue())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("uid changed"))

		currentRAR := &remediationv1.RemediationApprovalRequest{}
		Expect(baseClient.Get(ctx, client.ObjectKeyFromObject(rar), currentRAR)).To(Succeed())
		Expect(currentRAR.DeletionTimestamp).NotTo(BeNil())
		Expect(controllerutil.ContainsFinalizer(currentRAR, creator.ApprovalRequestFinalizerName)).To(BeTrue())

		currentReplacement := &remediationv1.RemediationRequest{}
		Expect(baseClient.Get(ctx, client.ObjectKeyFromObject(replacement), currentReplacement)).To(Succeed())
		Expect(currentReplacement.UID).To(Equal(types.UID("rr-replacement")))
		Expect(currentReplacement.Status.OverallPhase).To(Equal(remediationv1.PhaseAwaitingApproval))
		Expect(currentReplacement.Status.EnsureCompletionStatus().FailurePhase).To(BeNil())
	})

	It("UT-RO-2564-023: rejects a same-name replacement during failure status retry", func() {
		ctx := context.Background()
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())

		original := terminalSafetyRemediationRequest("rr-status-replaced", types.UID("rr-original"))
		baseClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			WithObjects(original).
			Build()
		replacementClient := &sameNameReplacementClient{Client: baseClient}
		reconciler := newTerminalSafetyReconciler(replacementClient, baseClient, scheme)

		current := &remediationv1.RemediationRequest{}
		Expect(baseClient.Get(ctx, client.ObjectKeyFromObject(original), current)).To(Succeed())
		_, err := reconciler.transitionToFailed(ctx, current, remediationv1.FailurePhaseApproval, errors.New("approval collision"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("uid changed"))

		currentReplacement := &remediationv1.RemediationRequest{}
		Expect(baseClient.Get(ctx, client.ObjectKeyFromObject(original), currentReplacement)).To(Succeed())
		Expect(currentReplacement.UID).To(Equal(types.UID("rr-replacement")))
		Expect(currentReplacement.Status.OverallPhase).To(Equal(remediationv1.PhaseAwaitingApproval))
		Expect(currentReplacement.Status.EnsureCompletionStatus().FailurePhase).To(BeNil())

		notifications := &notificationv1.NotificationRequestList{}
		Expect(baseClient.List(ctx, notifications)).To(Succeed())
		Expect(notifications.Items).To(BeEmpty(), "a replacement identity failure must not emit an escalation")
	})
})
