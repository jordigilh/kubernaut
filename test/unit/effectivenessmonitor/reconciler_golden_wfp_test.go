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

// Golden Snapshot Test: WaitingForPropagation (WFP) Path
//
// Issue #254: Captures the monolith's WFP behavior as a frozen baseline.
// The WFP path is entered when an async-managed target (GitOps, operator CRD)
// has a HashComputeDelay that hasn't elapsed yet. The reconciler enters
// WaitingForPropagation phase, persists derived timing, and requeues until
// the hash deadline elapses.
//
// This test MUST pass against the monolith (RED phase captures baseline)
// and continue to pass after decomposition (GREEN/REFACTOR phases).
package effectivenessmonitor

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/effectivenessmonitor"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
)

var _ = Describe("Golden Snapshot — WFP Path (UT-EM-254-001, #254)", func() {

	const hashComputeDelay = 10 * time.Minute

	buildScheme := func() *runtime.Scheme {
		s := runtime.NewScheme()
		Expect(eav1.AddToScheme(s)).To(Succeed())
		Expect(corev1.AddToScheme(s)).To(Succeed())
		return s
	}

	makeReconciler := func(s *runtime.Scheme, recorder *record.FakeRecorder, objs ...client.Object) (*controller.Reconciler, client.Client) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(objs...).
			WithStatusSubresource(&eav1.EffectivenessAssessment{}).
			Build()

		cfg := controller.DefaultReconcilerConfig()
		cfg.PrometheusEnabled = false
		cfg.AlertManagerEnabled = false

		r := controller.NewReconciler(
			fakeClient, fakeClient,
			s, recorder,
			emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			nil, nil,
			nil, nil,
			cfg,
		)
		return r, fakeClient
	}

	// seedAsyncEA creates an EA with HashComputeDelay set, simulating an
	// async-managed target where the hash deadline hasn't elapsed yet.
	seedAsyncEA := func(ns, name string) *eav1.EffectivenessAssessment {
		hcd := metav1.Duration{Duration: hashComputeDelay}
		return &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         ns,
				CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Minute)),
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "corr-wfp-" + name,
				RemediationRequestPhase: "Completed",
				SignalTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "async-app", Namespace: ns,
				},
				RemediationTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "async-app", Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 0},
					HashComputeDelay:    &hcd,
				},
			},
		}
	}

	It("UT-EM-254-001: WFP path produces correct phase, timing, and requeue", func() {
		s := buildScheme()
		recorder := record.NewFakeRecorder(100)
		ea := seedAsyncEA("default", "ea-wfp-001")
		r, fc := makeReconciler(s, recorder, ea)

		ctx := context.Background()
		req := ctrl.Request{NamespacedName: types.NamespacedName{
			Name: "ea-wfp-001", Namespace: "default",
		}}

		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		// --- Snapshot: ctrl.Result ---
		// WFP returns with RequeueAfter = time until hash deadline.
		// Hash deadline = creation + HCD = now-1m + 10m = ~9m from now.
		Expect(result.RequeueAfter).To(BeNumerically(">", 8*time.Minute),
			"RequeueAfter must reflect remaining time until hash deadline")
		Expect(result.RequeueAfter).To(BeNumerically("<=", hashComputeDelay),
			"RequeueAfter must not exceed the full HashComputeDelay")

		// --- Snapshot: EA Status ---
		fetched := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(ctx, req.NamespacedName, fetched)).To(Succeed())

		Expect(fetched.Status.Phase).To(Equal(eav1.PhaseWaitingForPropagation),
			"Phase must transition to WaitingForPropagation")
		Expect(fetched.Status.ValidityDeadline).NotTo(BeNil(),
			"ValidityDeadline must be persisted during WFP")
		Expect(fetched.Status.PrometheusCheckAfter).NotTo(BeNil(),
			"PrometheusCheckAfter must be persisted during WFP")
		Expect(fetched.Status.AlertManagerCheckAfter).NotTo(BeNil(),
			"AlertManagerCheckAfter must be persisted during WFP")

		// Derived timing validation: ValidityDeadline > hash deadline
		hashDeadline := ea.CreationTimestamp.Add(hashComputeDelay)
		Expect(fetched.Status.ValidityDeadline.Time).To(BeTemporally(">", hashDeadline),
			"ValidityDeadline must be after the hash deadline")

		// --- Snapshot: Components ---
		// No component checks run during WFP — all flags must be false/zero.
		Expect(fetched.Status.Components.HashComputed).To(BeFalse(),
			"Hash must NOT be computed during WFP")
		Expect(fetched.Status.Components.HealthAssessed).To(BeFalse(),
			"Health must NOT be assessed during WFP")
		Expect(fetched.Status.Components.AlertAssessed).To(BeFalse(),
			"Alert must NOT be assessed during WFP")
		Expect(fetched.Status.Components.MetricsAssessed).To(BeFalse(),
			"Metrics must NOT be assessed during WFP")

		// --- Snapshot: Events ---
		// WFP path emits a "Scheduled" event on first entry.
		Eventually(recorder.Events).Should(Receive(ContainSubstring("Scheduled")),
			"WFP must emit a Scheduled event on first entry")
	})

	It("UT-EM-254-001b: Subsequent WFP reconcile requeues without re-persisting", func() {
		s := buildScheme()
		recorder := record.NewFakeRecorder(100)
		ea := seedAsyncEA("default", "ea-wfp-001b")
		r, fc := makeReconciler(s, recorder, ea)

		ctx := context.Background()
		req := ctrl.Request{NamespacedName: types.NamespacedName{
			Name: "ea-wfp-001b", Namespace: "default",
		}}

		// First reconcile: enters WFP
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		// Drain the recorder
		for len(recorder.Events) > 0 {
			<-recorder.Events
		}

		// Fetch the persisted state
		fetched := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(ctx, req.NamespacedName, fetched)).To(Succeed())
		rvAfterFirst := fetched.ResourceVersion

		// Second reconcile: should requeue without Status().Update
		result2, err2 := r.Reconcile(ctx, req)
		Expect(err2).NotTo(HaveOccurred())
		Expect(result2.RequeueAfter).To(BeNumerically(">", 0),
			"Second WFP reconcile must still requeue")

		// Verify no new Status().Update by checking resourceVersion is unchanged
		Expect(fc.Get(ctx, req.NamespacedName, fetched)).To(Succeed())
		Expect(fetched.ResourceVersion).To(Equal(rvAfterFirst),
			"Second WFP reconcile must NOT persist status (already in WFP)")
	})
})
