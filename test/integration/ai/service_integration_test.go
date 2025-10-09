/*
Copyright 2025 Jordi Gil.

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

//go:build integration
// +build integration

package ai

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/jordigilh/kubernaut/test/integration/shared/testenv"
)

var _ = Describe("ServiceIntegration - Real K8s Integration Testing", func() {
	var (
		serviceIntegration *holmesgpt.ServiceIntegration
		testEnv            *testenv.TestEnvironment
		log                *logrus.Logger
		ctx                context.Context
	)

	BeforeEach(func() {
		// Setup real Kubernetes test environment
		// Per Project Guidelines Line 4: Reuse existing infrastructure patterns
		var err error
		testEnv, err = testenv.SetupEnvironment()
		Expect(err).ToNot(HaveOccurred())

		log = logrus.New()
		log.SetLevel(logrus.ErrorLevel) // Reduce test noise
		ctx = testEnv.Context

		// Ensure monitoring namespace exists for service discovery
		monitoringNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "monitoring",
			},
		}
		_, err = testEnv.Client.CoreV1().Namespaces().Create(ctx, monitoringNS, metav1.CreateOptions{})
		if err != nil {
			// Namespace might already exist - log but continue
			log.WithError(err).Debug("Monitoring namespace creation failed (might already exist)")
		}

		// Create test configuration with real K8s integration
		// Business Requirement: BR-HOLMES-020 - Real-time toolset updates
		config := &k8s.ServiceDiscoveryConfig{
			DiscoveryInterval:   100 * time.Millisecond,
			CacheTTL:            1 * time.Minute,
			HealthCheckInterval: 10 * time.Second,
			Enabled:             true,
			Namespaces:          []string{"monitoring"},
			ServicePatterns:     k8s.GetDefaultServicePatterns(), // BR-HOLMES-017: Well-known service detection
		}

		// Create service integration with REAL Kubernetes client
		serviceIntegration, err = holmesgpt.NewServiceIntegration(testEnv.Client, config, log)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		// Per Project Guidelines Line 4: Always log errors, never ignore them
		if serviceIntegration != nil {
			serviceIntegration.Stop()
		}

		// Cleanup test environment
		if testEnv != nil {
			err := testEnv.Cleanup()
			if err != nil {
				log.WithError(err).Error("Failed to cleanup test environment")
			}
		}
	})

	// Business Requirement: BR-HOLMES-020 - Real-time toolset updates
	// These tests were moved from unit tests due to fake K8s client limitations
	Describe("Real K8s Service Discovery Integration", func() {
		Context("Dynamic Updates with Real K8s", func() {
			BeforeEach(func() {
				err := serviceIntegration.Start(ctx)
				Expect(err).ToNot(HaveOccurred())
				time.Sleep(200 * time.Millisecond) // Wait for initialization
			})

			It("should handle service discovery updates with real K8s", func() {
				// Business Requirement: BR-HOLMES-020 - Real-time service discovery updates
				// Add a Prometheus service to trigger discovery
				prometheusService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "prometheus-integration-test",
						Namespace: "monitoring",
						Labels: map[string]string{
							"app.kubernetes.io/name": "prometheus",
						},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "web", Port: 9090},
						},
					},
				}

				// Create service in REAL K8s cluster
				_, err := testEnv.Client.CoreV1().Services("monitoring").Create(ctx, prometheusService, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				// Wait for service discovery to process the change
				// With real K8s, this should work unlike with fake client
				Eventually(func() bool {
					stats := serviceIntegration.GetServiceDiscoveryStats()
					return stats.TotalServices > 0
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				// Eventually should have prometheus toolsets
				Eventually(func() bool {
					prometheusToolsets := serviceIntegration.GetToolsetByServiceType("prometheus")
					return len(prometheusToolsets) > 0
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				// Cleanup: Remove the test service
				err = testEnv.Client.CoreV1().Services("monitoring").Delete(ctx, "prometheus-integration-test", metav1.DeleteOptions{})
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("Event Handling with Real K8s", func() {
			var testHandler *TestToolsetUpdateHandler

			BeforeEach(func() {
				testHandler = NewTestToolsetUpdateHandler()
				serviceIntegration.AddToolsetUpdateHandler(testHandler)

				err := serviceIntegration.Start(ctx)
				Expect(err).ToNot(HaveOccurred())
				time.Sleep(300 * time.Millisecond) // Wait for baseline toolsets to be added
			})

			It("should handle multiple event handlers with real K8s", func() {
				// Business Requirement: BR-HOLMES-025 - Runtime toolset management API
				secondHandler := NewTestToolsetUpdateHandler()
				serviceIntegration.AddToolsetUpdateHandler(secondHandler)

				// Force a toolset update - this should work with real K8s
				err := serviceIntegration.RefreshToolsets(ctx)
				Expect(err).ToNot(HaveOccurred())

				// Allow more time for real K8s operations
				time.Sleep(1 * time.Second)

				// Both handlers should have received updates
				// Per Project Guidelines: Always log errors for debugging
				log.WithFields(logrus.Fields{
					"first_handler_updates":  len(testHandler.UpdatedToolsets),
					"second_handler_updates": len(secondHandler.UpdatedToolsets),
				}).Info("Event handler update counts")

				Expect(len(testHandler.UpdatedToolsets)).To(BeNumerically(">=", 1))
				Expect(len(secondHandler.UpdatedToolsets)).To(BeNumerically(">=", 1))
			})

			It("should handle handler errors gracefully with real K8s", func() {
				// Add a handler that always fails
				failingHandler := &FailingToolsetUpdateHandler{}
				serviceIntegration.AddToolsetUpdateHandler(failingHandler)

				// Force a toolset update - should not crash despite failing handler
				err := serviceIntegration.RefreshToolsets(ctx)
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(500 * time.Millisecond)

				// Original handler should still receive updates despite failing handler
				Expect(len(testHandler.UpdatedToolsets)).To(BeNumerically(">=", 1))
			})
		})
	})
})

// Test helper structs for event handling tests
// Business Requirement: BR-HOLMES-025 - Runtime toolset management API
type TestToolsetUpdateHandler struct {
	UpdatedToolsets []([]*holmesgpt.ToolsetConfig)
}

func NewTestToolsetUpdateHandler() *TestToolsetUpdateHandler {
	return &TestToolsetUpdateHandler{
		UpdatedToolsets: make([]([]*holmesgpt.ToolsetConfig), 0),
	}
}

func (h *TestToolsetUpdateHandler) OnToolsetsUpdated(toolsets []*holmesgpt.ToolsetConfig) error {
	h.UpdatedToolsets = append(h.UpdatedToolsets, toolsets)
	return nil
}

type FailingToolsetUpdateHandler struct{}

func (h *FailingToolsetUpdateHandler) OnToolsetsUpdated(toolsets []*holmesgpt.ToolsetConfig) error {
	return fmt.Errorf("simulated handler failure")
}
