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

// Config-Disabled Permutation Integration Tests
//
// These tests validate the reconciler's behavior when Prometheus and/or
// AlertManager are disabled via ReconcilerConfig. They catch nil-pointer
// dereferences when a disabled service's client is never initialized but
// the reconciler still runs.
//
// Isolation Strategy:
// The suite-level controller has both PrometheusEnabled=true and
// AlertManagerEnabled=true. To test disabled-config behavior without the
// suite controller competing for EAs, these tests use an ISOLATED envtest
// (separate kube-apiserver + etcd) with direct Reconcile() invocation.
//
// Test Scenarios:
//   - IT-EM-CF-002: Both Prom + AM disabled → health+hash only
//   - IT-EM-AR-005: AM disabled → no alert assessment
//   - IT-EM-AE-006: Prom disabled → no metrics assessment
//   - IT-EM-AE-007: AM disabled → exactly 3 components (health, hash, metrics)
//   - IT-EM-FF-005: Prom disabled + nil client → startup succeeds, no panic
package effectivenessmonitor

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/effectivenessmonitor"
	emclient "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

var _ = Describe("Config-Disabled Permutations (BR-EM-006, BR-EM-007, BR-EM-008)", Ordered, func() {

	// Isolated envtest: no suite-level controller registers watches here,
	// so direct Reconcile() calls are the ONLY thing processing EAs.
	var (
		isolatedEnv    *envtest.Environment
		isolatedClient client.Client
		isolatedCtx    context.Context
		isolatedCancel context.CancelFunc

		// Per-container mocks (independent from suite-level mocks)
		localMockProm *infrastructure.MockPrometheus
		localMockAM   *infrastructure.MockAlertManager
	)

	BeforeAll(func() {
		isolatedCtx, isolatedCancel = context.WithCancel(context.Background())

		By("Starting isolated envtest for config-disabled tests")
		isolatedEnv = &envtest.Environment{
			CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
			ErrorIfCRDPathMissing: true,
		}
		isolatedCfg, err := isolatedEnv.Start()
		Expect(err).NotTo(HaveOccurred())
		Expect(isolatedCfg).NotTo(BeNil())

		// Scheme is already registered by suite-level SynchronizedBeforeSuite Phase 2
		isolatedClient, err = client.New(isolatedCfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())

		By("Creating default namespace in isolated envtest")
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
		_ = isolatedClient.Create(isolatedCtx, ns) // Ignore AlreadyExists

		By("Starting isolated Prometheus and AlertManager httptest mocks")
		now := float64(time.Now().Unix())
		preTime := now - 60
		localMockProm = infrastructure.NewMockPrometheus(infrastructure.MockPrometheusConfig{
			Ready:   true,
			Healthy: true,
			QueryResponse: infrastructure.NewPromVectorResponse(
				map[string]string{"__name__": "container_cpu_usage_seconds_total", "namespace": "default"},
				0.25, now,
			),
			QueryRangeResponse: infrastructure.NewPromMatrixResponse(
				map[string]string{"__name__": "container_cpu_usage_seconds_total", "namespace": "default"},
				[][]interface{}{
					{preTime, "0.500000"}, // pre-remediation: 50% CPU
					{now, "0.250000"},     // post-remediation: 25% CPU (improvement)
				},
			),
		})
		localMockAM = infrastructure.NewMockAlertManager(infrastructure.MockAlertManagerConfig{
			Ready:   true,
			Healthy: true,
		})
		GinkgoWriter.Printf("✅ Isolated envtest + mocks ready (Prom=%s, AM=%s)\n",
			localMockProm.URL(), localMockAM.URL())
	})

	AfterAll(func() {
		if localMockProm != nil {
			localMockProm.Close()
		}
		if localMockAM != nil {
			localMockAM.Close()
		}
		if isolatedCancel != nil {
			isolatedCancel()
		}
		if isolatedEnv != nil {
			err := isolatedEnv.Stop()
			if err != nil {
				GinkgoWriter.Printf("⚠️  Failed to stop isolated envtest: %v\n", err)
			}
		}
	})

	// ========================================================================
	// HELPERS (scoped to this Ordered container)
	// ========================================================================

	// makeReconciler creates a Reconciler with the specified config toggles.
	// Uses a fresh Prometheus registry to avoid duplicate metric registration panics.
	makeReconciler := func(promEnabled, amEnabled bool) *controller.Reconciler {
		cfg := controller.DefaultReconcilerConfig()
		cfg.PrometheusEnabled = promEnabled
		cfg.AlertManagerEnabled = amEnabled

		var pc emclient.PrometheusQuerier
		var ac emclient.AlertManagerClient
		if promEnabled {
			pc = emclient.NewPrometheusHTTPClient(localMockProm.URL(), 5*time.Second)
		}
		if amEnabled {
			ac = emclient.NewAlertManagerHTTPClient(localMockAM.URL(), 5*time.Second)
		}

		return controller.NewReconciler(
			isolatedClient,
			scheme.Scheme,
			record.NewFakeRecorder(100),
			emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			pc, ac,
			nil, nil, // AuditManager, DSQuerier (not wired in INT)
			cfg,
		)
	}

	// createIsolatedEA creates an EA in the isolated envtest.
	createIsolatedEA := func(ns, name, corrID string) {
		// Ensure namespace exists
		nsObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}
		_ = isolatedClient.Create(isolatedCtx, nsObj) // Ignore AlreadyExists

		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name: name, Namespace: ns,
				Labels: map[string]string{"kubernaut.ai/correlation-id": corrID},
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID: corrID,
				TargetResource: eav1.TargetResource{
					Kind: "Deployment", Name: "test-app", Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
				},
			},
		}
		Expect(isolatedClient.Create(isolatedCtx, ea)).To(Succeed())
	}

	// reconcileUntilDone calls Reconcile() repeatedly until the EA reaches Completed.
	reconcileUntilDone := func(r *controller.Reconciler, ns, name string) *eav1.EffectivenessAssessment {
		var ea *eav1.EffectivenessAssessment
		Eventually(func(g Gomega) {
			_, _ = r.Reconcile(isolatedCtx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
			})
			ea = &eav1.EffectivenessAssessment{}
			g.Expect(isolatedClient.Get(isolatedCtx,
				types.NamespacedName{Name: name, Namespace: ns}, ea)).To(Succeed())
			g.Expect(ea.Status.Phase).To(Equal(eav1.PhaseCompleted),
				fmt.Sprintf("EA %s/%s should reach Completed (current: %s)", ns, name, ea.Status.Phase))
		}, 60*time.Second, 500*time.Millisecond).Should(Succeed())
		return ea
	}

	// ========================================================================
	// IT-EM-CF-002: Both Prom + AM disabled → reconciler runs health+hash only
	// Risk: nil-pointer dereference if reconciler calls disabled client
	// ========================================================================
	It("IT-EM-CF-002: should complete with health+hash only when both Prom and AM disabled", func() {
		ns := "cd-cf-002"
		createIsolatedEA(ns, "ea-cf-002", "rr-cf-002")

		By("Creating reconciler with both Prom and AM disabled (nil clients)")
		r := makeReconciler(false, false)

		By("Reconciling until EA reaches Completed")
		ea := reconcileUntilDone(r, ns, "ea-cf-002")

		By("Verifying always-on components are assessed")
		Expect(ea.Status.Components.HealthAssessed).To(BeTrue(),
			"health must be assessed (always-on)")
		Expect(ea.Status.Components.HashComputed).To(BeTrue(),
			"hash must be computed (always-on)")

		By("Verifying disabled components are NOT assessed")
		Expect(ea.Status.Components.AlertAssessed).To(BeFalse(),
			"alert must NOT be assessed (AM disabled)")
		Expect(ea.Status.Components.MetricsAssessed).To(BeFalse(),
			"metrics must NOT be assessed (Prom disabled)")

		By("Verifying EA completes with 'full' reason (disabled = not required)")
		Expect(ea.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonFull),
			"disabled components should not prevent 'full' completion")
		Expect(ea.Status.CompletedAt).NotTo(BeNil())
	})

	// ========================================================================
	// IT-EM-AR-005: AM disabled → no alert assessment, no alert event emitted
	// Risk: nil AM client invoked despite config disable
	// ========================================================================
	It("IT-EM-AR-005: should skip alert assessment when AM disabled in config", func() {
		ns := "cd-ar-005"
		createIsolatedEA(ns, "ea-ar-005", "rr-ar-005")

		By("Creating reconciler with AM disabled, Prom enabled")
		r := makeReconciler(true, false)

		By("Reconciling until EA reaches Completed")
		ea := reconcileUntilDone(r, ns, "ea-ar-005")

		By("Verifying alert is NOT assessed (AM disabled)")
		Expect(ea.Status.Components.AlertAssessed).To(BeFalse(),
			"alert must NOT be assessed (AM disabled)")
		Expect(ea.Status.Components.AlertScore).To(BeNil(),
			"alert score must be nil when AM disabled")

		By("Verifying other components assessed normally")
		Expect(ea.Status.Components.HealthAssessed).To(BeTrue())
		Expect(ea.Status.Components.HashComputed).To(BeTrue())
		Expect(ea.Status.Components.MetricsAssessed).To(BeTrue(),
			"metrics must be assessed (Prom enabled)")

		Expect(ea.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonFull))
	})

	// ========================================================================
	// IT-EM-AE-006: Prom disabled → 3 components only (health, hash, alert)
	// Validates that only 4 audit events would be emitted (3 components + completed)
	// ========================================================================
	It("IT-EM-AE-006: should assess only health, hash, alert when Prom disabled", func() {
		ns := "cd-ae-006"
		createIsolatedEA(ns, "ea-ae-006", "rr-ae-006")

		By("Creating reconciler with Prom disabled, AM enabled")
		r := makeReconciler(false, true)

		By("Reconciling until EA reaches Completed")
		ea := reconcileUntilDone(r, ns, "ea-ae-006")

		By("Verifying metrics is NOT assessed (Prom disabled)")
		Expect(ea.Status.Components.MetricsAssessed).To(BeFalse(),
			"metrics must NOT be assessed (Prom disabled)")
		Expect(ea.Status.Components.MetricsScore).To(BeNil(),
			"metrics score must be nil when Prom disabled")

		By("Verifying health, hash, alert are assessed")
		Expect(ea.Status.Components.HealthAssessed).To(BeTrue())
		Expect(ea.Status.Components.HashComputed).To(BeTrue())
		Expect(ea.Status.Components.AlertAssessed).To(BeTrue(),
			"alert must be assessed (AM enabled)")

		By("Counting assessed components (proxy for audit event count)")
		assessedCount := countAssessedComponents(ea)
		Expect(assessedCount).To(Equal(3),
			"exactly 3 components should be assessed when Prom disabled (health, hash, alert)")

		Expect(ea.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonFull))
	})

	// ========================================================================
	// IT-EM-AE-007: AM disabled → 3 components only (health, hash, metrics)
	// Validates that only 4 audit events would be emitted (3 components + completed)
	// ========================================================================
	It("IT-EM-AE-007: should assess only health, hash, metrics when AM disabled", func() {
		ns := "cd-ae-007"
		createIsolatedEA(ns, "ea-ae-007", "rr-ae-007")

		By("Creating reconciler with AM disabled, Prom enabled")
		r := makeReconciler(true, false)

		By("Reconciling until EA reaches Completed")
		ea := reconcileUntilDone(r, ns, "ea-ae-007")

		By("Verifying alert is NOT assessed (AM disabled)")
		Expect(ea.Status.Components.AlertAssessed).To(BeFalse(),
			"alert must NOT be assessed (AM disabled)")

		By("Verifying health, hash, metrics are assessed")
		Expect(ea.Status.Components.HealthAssessed).To(BeTrue())
		Expect(ea.Status.Components.HashComputed).To(BeTrue())
		Expect(ea.Status.Components.MetricsAssessed).To(BeTrue(),
			"metrics must be assessed (Prom enabled)")

		By("Counting assessed components (proxy for audit event count)")
		assessedCount := countAssessedComponents(ea)
		Expect(assessedCount).To(Equal(3),
			"exactly 3 components should be assessed when AM disabled (health, hash, metrics)")

		Expect(ea.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonFull))
	})

	// ========================================================================
	// IT-EM-FF-005: Prom disabled + nil client → startup succeeds, no panic
	// Risk: EM pod fails to start when optional dependency not deployed
	// ========================================================================
	It("IT-EM-FF-005: should start and reconcile successfully when Prom disabled and client nil", func() {
		ns := "cd-ff-005"
		createIsolatedEA(ns, "ea-ff-005", "rr-ff-005")

		By("Creating reconciler with Prom disabled and nil Prom client")
		cfg := controller.DefaultReconcilerConfig()
		cfg.PrometheusEnabled = false
		cfg.AlertManagerEnabled = true

		amClient := emclient.NewAlertManagerHTTPClient(localMockAM.URL(), 5*time.Second)

		r := controller.NewReconciler(
			isolatedClient,
			scheme.Scheme,
			record.NewFakeRecorder(100),
			emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			nil,      // Prom client: nil (disabled, not deployed)
			amClient, // AM client: real (enabled)
			nil, nil, // AuditManager, DSQuerier
			cfg,
		)

		By("Reconciling — should not panic with nil Prom client")
		ea := reconcileUntilDone(r, ns, "ea-ff-005")

		By("Verifying EA completes without panic")
		Expect(ea.Status.Phase).To(Equal(eav1.PhaseCompleted))
		Expect(ea.Status.Components.MetricsAssessed).To(BeFalse(),
			"metrics must NOT be assessed (Prom disabled + nil client)")
		Expect(ea.Status.Components.AlertAssessed).To(BeTrue(),
			"alert must be assessed (AM enabled + real client)")
		Expect(ea.Status.CompletedAt).NotTo(BeNil())
	})
})

// countAssessedComponents counts how many of the 4 assessment components
// are marked as assessed in the EA status. Used as a proxy for audit event
// count validation in integration tests (AuditManager is nil at INT tier).
func countAssessedComponents(ea *eav1.EffectivenessAssessment) int {
	count := 0
	if ea.Status.Components.HealthAssessed {
		count++
	}
	if ea.Status.Components.HashComputed {
		count++
	}
	if ea.Status.Components.AlertAssessed {
		count++
	}
	if ea.Status.Components.MetricsAssessed {
		count++
	}
	return count
}
