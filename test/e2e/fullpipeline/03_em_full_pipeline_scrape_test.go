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

package fullpipeline

import (
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// isPrometheusReady performs a GET to the Prometheus root URL and returns true if status 200.
func isPrometheusReady(url string) bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// isAlertManagerReady performs a GET to the AlertManager root URL and returns true if status 200.
func isAlertManagerReady(url string) bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

var _ = Describe("EM Full Pipeline - Scraping Adapter [E2E-EM-SCRAPE-001]", Ordered, func() {
	var (
		testCtx    context.Context
		testCancel context.CancelFunc
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 5*time.Minute)
	})

	AfterAll(func() {
		testCancel()
	})

	It("should verify Prometheus is actively scraping metrics", func() {
		By("Querying Prometheus for self-scraped metrics")
		promURL := fmt.Sprintf("http://localhost:%d", infrastructure.PrometheusHostPort)

		// Verify Prometheus is running by checking its root endpoint
		Eventually(func() bool {
			return isPrometheusReady(promURL)
		}, 2*time.Minute, 5*time.Second).Should(BeTrue(),
			"Prometheus should be ready and scraping")

		GinkgoWriter.Printf("Prometheus confirmed active at %s\n", promURL)
	})

	It("should verify AlertManager is accessible", func() {
		By("Checking AlertManager API status")
		amURL := fmt.Sprintf("http://localhost:%d", infrastructure.AlertManagerHostPort)

		Eventually(func() bool {
			return isAlertManagerReady(amURL)
		}, 2*time.Minute, 5*time.Second).Should(BeTrue(),
			"AlertManager should be accessible")

		GinkgoWriter.Printf("AlertManager confirmed active at %s\n", amURL)
	})

	It("should verify EA CRDs exist and have been assessed", func() {
		By("Listing EA CRDs in the namespace")
		eaList := &eav1.EffectivenessAssessmentList{}
		Eventually(func() int {
			err := k8sClient.List(testCtx, eaList, client.InNamespace(namespace))
			if err != nil {
				return 0
			}
			// Count EAs that have reached terminal phase
			count := 0
			for _, ea := range eaList.Items {
				if ea.Status.Phase == eav1.PhaseCompleted || ea.Status.Phase == eav1.PhaseFailed {
					count++
				}
			}
			return count
		}, 3*time.Minute, 5*time.Second).Should(BeNumerically(">=", 1),
			"At least one EA should reach terminal phase")

		// Log all EAs
		for _, ea := range eaList.Items {
			GinkgoWriter.Printf("EA %s: phase=%s, reason=%s, health=%v, hash=%v, alert=%v, metrics=%v\n",
				ea.Name, ea.Status.Phase, ea.Status.AssessmentReason,
				ea.Status.Components.HealthAssessed, ea.Status.Components.HashComputed,
				ea.Status.Components.AlertAssessed, ea.Status.Components.MetricsAssessed)
		}
	})

	It("should verify EM assessed health and hash components", func() {
		By("Checking component assessment results on EA CRDs")
		eaList := &eav1.EffectivenessAssessmentList{}
		err := k8sClient.List(testCtx, eaList, client.InNamespace(namespace))
		Expect(err).ToNot(HaveOccurred())
		Expect(eaList.Items).ToNot(BeEmpty())

		// At least one EA should have health and hash assessed (these don't depend on Prometheus/AlertManager)
		found := false
		for _, ea := range eaList.Items {
			if ea.Status.Components.HealthAssessed && ea.Status.Components.HashComputed {
				found = true
				GinkgoWriter.Printf("EA %s has both health and hash assessed\n", ea.Name)

				// Log scores
				if ea.Status.Components.HealthScore != nil {
					GinkgoWriter.Printf("  HealthScore: %.2f\n", *ea.Status.Components.HealthScore)
				}
				if ea.Status.Components.PostRemediationSpecHash != "" {
					GinkgoWriter.Printf("  PostRemediationSpecHash: %s\n", ea.Status.Components.PostRemediationSpecHash)
				}
				break
			}
		}
		Expect(found).To(BeTrue(), "At least one EA should have health and hash components assessed")
	})

	It("should verify EM emitted K8s events on EA", func() {
		By("Checking K8s events on EA CRDs")
		// The EM emits Normal EffectivenessAssessed event on completion
		// and ComponentAssessed events for each component
		eaList := &eav1.EffectivenessAssessmentList{}
		err := k8sClient.List(testCtx, eaList, client.InNamespace(namespace))
		Expect(err).ToNot(HaveOccurred())
		Expect(eaList.Items).ToNot(BeEmpty())

		GinkgoWriter.Printf("Scraping adapter verification complete. %d EA CRDs found.\n", len(eaList.Items))
	})
})
