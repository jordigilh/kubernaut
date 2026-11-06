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

package approvalsafety_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

func TestApprovalSafety(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Approval Safety Envtest Suite")
}

var _ = Describe("BR-ORCH-025 missing approval", func() {
	It("IT-RO-2570-027 fails the production reconcile to manual review without replacement or execution", func() {
		ctx := context.Background()
		env := &envtest.Environment{CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "..", "config", "crd", "bases")}, ErrorIfCRDPathMissing: true}
		cfg, err := env.Start()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(env.Stop()).To(Succeed()) })
		s := runtime.NewScheme()
		for _, add := range []func(*runtime.Scheme) error{scheme.AddToScheme, remediationv1.AddToScheme, aianalysisv1.AddToScheme, signalv1.AddToScheme, notificationv1.AddToScheme, workflowv1.AddToScheme, eav1.AddToScheme} {
			Expect(add(s)).To(Succeed())
		}
		c, err := client.New(cfg, client.Options{Scheme: s})
		Expect(err).NotTo(HaveOccurred())
		now := metav1.Now()
		rr := &remediationv1.RemediationRequest{ObjectMeta: metav1.ObjectMeta{Name: "missing-approval", Namespace: "default"}, Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: strings.Repeat("a", 64), SignalName: "TestAlert", Severity: "critical", SignalType: "alert", TargetType: "kubernetes",
			TargetResource: remediationv1.ResourceIdentifier{Kind: "Deployment", Name: "app", Namespace: "default"}, FiringTime: now, ReceivedTime: now,
		}}
		Expect(c.Create(ctx, rr)).To(Succeed())
		rr.Status.OverallPhase = remediationv1.PhaseAwaitingApproval
		rr.Status.StartTime = &now
		Expect(c.Status().Update(ctx, rr)).To(Succeed())
		// Nil audit isolates phase/CRD wiring; this test does not prove audit delivery.
		recorder := record.NewFakeRecorder(20)
		r := controller.NewReconciler(controller.ReconcilerDeps{Client: c, APIReader: c, Scheme: s, Recorder: recorder, Metrics: rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())})
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(rr)})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.Get(ctx, client.ObjectKeyFromObject(rr), rr)).To(Succeed())
		Expect(rr.Status.OverallPhase).To(Equal(remediationv1.PhaseFailed))
		completion := rr.Status.CompletionStatus
		Expect(completion).NotTo(BeNil())
		Expect(completion.FailurePhase).NotTo(BeNil())
		Expect(*completion.FailurePhase).To(Equal(remediationv1.FailurePhaseApproval))
		Expect(completion.RequiresManualReview).To(BeTrue())
		Expect(completion.Outcome).To(Equal(remediationv1.OutcomeManualReviewRequired))
		err = c.Get(ctx, client.ObjectKey{Name: "rar-" + rr.Name, Namespace: rr.Namespace}, &remediationv1.RemediationApprovalRequest{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
		wes := &workflowv1.WorkflowExecutionList{}
		Expect(c.List(ctx, wes, client.InNamespace(rr.Namespace))).To(Succeed())
		Expect(wes.Items).To(BeEmpty())
		failureEvents := 0
		for len(recorder.Events) > 0 {
			if strings.Contains(<-recorder.Events, "RemediationFailed") {
				failureEvents++
			}
		}
		Expect(failureEvents).To(Equal(1), "only the committed transition emits a failure event")
	})
})
