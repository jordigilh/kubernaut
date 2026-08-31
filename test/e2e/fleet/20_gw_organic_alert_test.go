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

package fleet

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E-FLEET-021 [AC-4, AU-3, BR-INTEGRATION-054]: a real AlertManager webhook
// forward -- not a direct test-initiated POST -- organically creates a
// RemediationRequest.
//
// Every other Gateway-facing fleet test (E2E-FLEET-001/002/018/019/...) drives
// Gateway via postFleetAlertUntilAccepted, a synthetic-payload POST issued
// directly by the test harness. None of them exercise AlertManager's own
// notification pipeline (its real receiver/webhook_config, group_wait,
// routing) end to end. This test proves that path organically: it creates a
// real target Deployment, injects a synthetic metric that trips a dedicated
// Prometheus alerting rule (fleet-organic-gateway-alert.yml,
// test/infrastructure/prometheus_alertmanager_e2e.go), and asserts a
// RemediationRequest appears WITHOUT ever calling Gateway's HTTP API itself.
//
// Ordering matters here in a way it doesn't for the direct-POST tests: the
// target Deployment must exist BEFORE the alert first fires, since Gateway's
// owner-resolution does a live lookup and drops the signal if the target is
// missing (same constraint 01_signal_ingestion_test.go documents). The
// alerting rule's expr is gated on a synthetic metric (never vector(1) > 0)
// specifically so it stays inactive until this test injects it, after the
// fixture already exists.
//
// Authority: kubernaut#2309 (fleet E2E realism follow-up), Issue #54, ADR-068
// FedRAMP: AC-4 (information flow enforcement), AU-3 (audit record content)
var _ = Describe("E2E-FLEET-021 [AC-4, AU-3]: real AlertManager webhook forward organically creates a RemediationRequest", Label("fleet"), func() {
	It("should create a RemediationRequest from a Prometheus-rule-fired, AlertManager-forwarded alert with no test-initiated Gateway POST", func() {
		const (
			prometheusURL = "http://localhost:9190"
			targetName    = "fleet-organic-gw-target"
			ruleName      = "FleetOrganicGatewayAlert"
		)

		By("Creating the real target Deployment BEFORE the alert can fire (Gateway owner-resolution requires it to exist)")
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetName,
				Namespace: namespace,
				Labels:    map[string]string{"kubernaut.ai/managed": "true"},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](0),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": targetName}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": targetName}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "app", Image: "busybox:1.36"}},
					},
				},
			},
		}
		if createErr := k8sClient.Create(ctx, dep); createErr != nil && !apierrors.IsAlreadyExists(createErr) {
			Expect(createErr).NotTo(HaveOccurred(), "Failed to create %s fixture", targetName)
		}
		DeferCleanup(func() { _ = k8sClient.Delete(context.Background(), dep) })

		By("Injecting the synthetic metric that trips fleet-organic-gateway-alert.yml's rule")
		Expect(infrastructure.InjectMetrics(ctx, prometheusURL, []infrastructure.TestMetric{
			{
				Name: "fleet_organic_gw_signal",
				Labels: map[string]string{
					"namespace": namespace,
					"kind":      "Deployment",
					"name":      targetName,
				},
				Value:     1,
				Timestamp: time.Now(),
			},
		})).To(Succeed(), "failed to inject fleet_organic_gw_signal metric")

		By("Waiting for FleetOrganicGatewayAlert to transition to firing")
		Eventually(func() error {
			return infrastructure.WaitForPrometheusRuleState(ctx, prometheusURL, ruleName,
				infrastructure.RuleStateFiring, 5*time.Second)
		}, 60*time.Second, 2*time.Second).Should(Succeed(), "FleetOrganicGatewayAlert must be firing")

		By("Verifying a RemediationRequest appears from AlertManager's OWN webhook forward -- no test-initiated Gateway POST was made")
		Eventually(func(g Gomega) {
			var rrList remediationv1.RemediationRequestList
			g.Expect(k8sClient.List(ctx, &rrList, client.InNamespace(namespace))).To(Succeed())

			found := false
			for i := range rrList.Items {
				if rrList.Items[i].Spec.SignalName == ruleName {
					found = true
					break
				}
			}
			g.Expect(found).To(BeTrue(),
				"AC-4: a RemediationRequest with signalName=%q must exist, organically created by "+
					"AlertManager's real webhook forward to Gateway -- this is a hub-local signal (no "+
					"cluster label), so scope-check uses the local K8s informer cache, not FMC",
				ruleName)
			// fmcSyncTimeout-scale window even though this signal is hub-local: it
			// comfortably covers Prometheus's evaluation_interval (15s) + rule
			// interval (10s) + AlertManager's group_wait/group_interval (5s each)
			// stacked before the webhook fires at all.
		}, fmcSyncTimeout, 2*time.Second).Should(Succeed())
	})
})
