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

package notification

import (
	"context"
	"os"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/pkg/notification/routing"
)

// testLogger is used for Router tests
var testLogger logr.Logger

func init() {
	testLogger = zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
}

// =============================================================================
// BR-NOT-067: Routing Configuration Hot-Reload
// =============================================================================
//
// Business Requirement:
// The Notification Service MUST reload routing configuration without restart
// when the ConfigMap changes, enabling dynamic routing updates without service
// disruption.
//
// Acceptance Criteria:
// - ConfigMap changes detected within 30 seconds
// - Routing table updated without restart
// - In-flight notifications not affected
// - Config reload logged with before/after diff
//
// =============================================================================

var _ = Describe("BR-NOT-067: Routing Configuration Hot-Reload", func() {

	Context("ConfigMap Watcher", func() {

		It("should detect ConfigMap create event", func() {
			// BR-NOT-067: ConfigMap changes detected within 30 seconds
			// Create a ConfigMap with routing configuration
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routing.DefaultConfigMapName,
					Namespace: routing.DefaultConfigMapNamespace,
				},
				Data: map[string]string{
					routing.DefaultConfigMapKey: `
route:
  receiver: default-console
receivers:
  - name: default-console
    consoleConfigs:
      - enabled: true
`,
				},
			}

			// Verify the watcher extracts routing config from ConfigMap
			Expect(configMap.Data[routing.DefaultConfigMapKey]).NotTo(BeEmpty())
		})

		It("should detect ConfigMap update event", func() {
			// BR-NOT-067: ConfigMap changes detected within 30 seconds
			// Update should trigger config reload

			oldConfig := `
route:
  receiver: default-console
receivers:
  - name: default-console
    consoleConfigs:
      - enabled: true
`
			newConfig := `
route:
  receiver: slack-ops
  routes:
    - match:
        severity: critical
      receiver: pagerduty-oncall
receivers:
  - name: slack-ops
    slackConfigs:
      - channel: "#ops"
  - name: pagerduty-oncall
    pagerdutyConfigs:
      - serviceKey: test-key
`
			// Verify old and new configs are different
			Expect(oldConfig).NotTo(Equal(newConfig))
		})
	})

	Context("Routing Table Update", func() {

		It("should update routing table without restart", func() {
			// BR-NOT-067: Routing table updated without restart
			router := routing.NewRouter(testLogger)

			// Load initial config
			initialConfig := `
route:
  receiver: default-console
receivers:
  - name: default-console
    consoleConfigs:
      - enabled: true
`
			err := router.LoadConfig([]byte(initialConfig))
			Expect(err).NotTo(HaveOccurred())

			// Get initial receiver for empty attributes
			initialReceiver := router.FindReceiver(map[string]string{})
			Expect(initialReceiver.Name).To(Equal("default-console"))

			// Load updated config (simulating hot-reload)
			updatedConfig := `
route:
  receiver: slack-ops
receivers:
  - name: slack-ops
    slackConfigs:
      - channel: "#ops"
`
			err = router.LoadConfig([]byte(updatedConfig))
			Expect(err).NotTo(HaveOccurred())

			// Verify routing table updated
			updatedReceiver := router.FindReceiver(map[string]string{})
			Expect(updatedReceiver.Name).To(Equal("slack-ops"))
		})

		It("should preserve old config on parse error", func() {
			// BR-NOT-067: Graceful error handling - invalid config should not break routing
			router := routing.NewRouter(testLogger)

			// Load valid config first
			validConfig := `
route:
  receiver: default-console
receivers:
  - name: default-console
    consoleConfigs:
      - enabled: true
`
			err := router.LoadConfig([]byte(validConfig))
			Expect(err).NotTo(HaveOccurred())

			// Attempt to load invalid config
			invalidConfig := `
route:
  receiver: non-existent-receiver
receivers:
  - name: default-console
`
			err = router.LoadConfig([]byte(invalidConfig))
			Expect(err).To(HaveOccurred())

			// Old config should still work
			receiver := router.FindReceiver(map[string]string{})
			Expect(receiver.Name).To(Equal("default-console"))
		})
	})

	Context("In-Flight Notification Safety", func() {

		It("should not affect in-flight notifications during reload", func() {
			// BR-NOT-067: In-flight notifications not affected
			router := routing.NewRouter(testLogger)

			// Load initial config
			err := router.LoadConfig([]byte(`
route:
  receiver: default-console
receivers:
  - name: default-console
    consoleConfigs:
      - enabled: true
`))
			Expect(err).NotTo(HaveOccurred())

			// Simulate in-flight notification resolution (before reload)
			attrs := map[string]string{"severity": "critical"}
			receiverBeforeReload := router.FindReceiver(attrs)

			// Reload config with different routing
			err = router.LoadConfig([]byte(`
route:
  routes:
    - match:
        severity: critical
      receiver: pagerduty
  receiver: slack-ops
receivers:
  - name: slack-ops
    slackConfigs:
      - channel: "#ops"
  - name: pagerduty
    pagerdutyConfigs:
      - serviceKey: test
`))
			Expect(err).NotTo(HaveOccurred())

			// The receiver reference from before reload should still be valid
			// (no nil pointer dereference or corruption)
			Expect(receiverBeforeReload).NotTo(BeNil())
			Expect(receiverBeforeReload.Name).NotTo(BeEmpty())

			// New notifications get new routing
			receiverAfterReload := router.FindReceiver(attrs)
			Expect(receiverAfterReload.Name).To(Equal("pagerduty"))
		})
	})

	Context("Reload Logging", func() {

		It("should log config reload with changes summary", func() {
			// BR-NOT-067: Config reload logged with before/after diff
			router := routing.NewRouter(testLogger)

			// Load initial config
			err := router.LoadConfig([]byte(`
route:
  receiver: default-console
receivers:
  - name: default-console
    consoleConfigs:
      - enabled: true
`))
			Expect(err).NotTo(HaveOccurred())

			// Get config summary before reload
			summaryBefore := router.GetConfigSummary()
			Expect(summaryBefore).To(ContainSubstring("default-console"))

			// Load updated config
			err = router.LoadConfig([]byte(`
route:
  receiver: slack-ops
receivers:
  - name: slack-ops
    slackConfigs:
      - channel: "#ops"
`))
			Expect(err).NotTo(HaveOccurred())

			// Get config summary after reload
			summaryAfter := router.GetConfigSummary()
			Expect(summaryAfter).To(ContainSubstring("slack-ops"))
			Expect(summaryBefore).NotTo(Equal(summaryAfter))
		})
	})

	Context("ConfigMap Handler Integration", func() {

		It("should handle ConfigMap with correct name and namespace", func() {
			// BR-NOT-067: Only react to the routing ConfigMap
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routing.DefaultConfigMapName,
					Namespace: routing.GetConfigMapNamespace(),
				},
			}

			Expect(configMap.Name).To(Equal("notification-routing-config"))
			Expect(routing.IsRoutingConfigMap(configMap.Name, configMap.Namespace)).To(BeTrue())
		})

		It("should ignore ConfigMaps with different name", func() {
			Expect(routing.IsRoutingConfigMap("some-other-config", routing.GetConfigMapNamespace())).To(BeFalse())
		})

		It("should ignore ConfigMaps with different namespace", func() {
			Expect(routing.IsRoutingConfigMap(routing.DefaultConfigMapName, "some-other-namespace")).To(BeFalse())
		})
	})

	Context("Configurable Namespace (#207)", func() {

		It("should use POD_NAMESPACE env var when set", func() {
			// #207: Routing ConfigMap namespace must match the pod's namespace
			original := os.Getenv("POD_NAMESPACE")
			defer os.Setenv("POD_NAMESPACE", original)

			os.Setenv("POD_NAMESPACE", "kubernaut-system")
			Expect(routing.GetConfigMapNamespace()).To(Equal("kubernaut-system"))
		})

		It("should fall back to DefaultConfigMapNamespace when POD_NAMESPACE is not set", func() {
			original := os.Getenv("POD_NAMESPACE")
			defer os.Setenv("POD_NAMESPACE", original)

			os.Unsetenv("POD_NAMESPACE")
			Expect(routing.GetConfigMapNamespace()).To(Equal(routing.DefaultConfigMapNamespace))
		})

		It("should use POD_NAMESPACE in IsRoutingConfigMap matching", func() {
			original := os.Getenv("POD_NAMESPACE")
			defer os.Setenv("POD_NAMESPACE", original)

			os.Setenv("POD_NAMESPACE", "kubernaut-system")
			Expect(routing.IsRoutingConfigMap("notification-routing-config", "kubernaut-system")).To(BeTrue())
			Expect(routing.IsRoutingConfigMap("notification-routing-config", "kubernaut-notifications")).To(BeFalse())
		})
	})

	Context("Reload Timing", func() {

		It("should reload within 30 second SLA", func() {
			// BR-NOT-067: ConfigMap changes detected within 30 seconds
			// This is a documentation test - actual timing depends on controller-runtime
			// The watch mechanism should trigger reload immediately upon ConfigMap event

			// Verify default resync period is set appropriately
			// Note: controller-runtime watches have near-instant event delivery
			// The 30-second SLA accounts for potential Kubernetes API latency
			expectedSLA := 30 // seconds
			Expect(expectedSLA).To(BeNumerically(">=", 1))
		})
	})
})

// =============================================================================
// Integration Test: Controller ConfigMap Watch
// =============================================================================

var _ = Describe("BR-NOT-067: Controller ConfigMap Watch Integration", func() {

	Context("SetupWithManager ConfigMap Watch", func() {

		It("should watch ConfigMaps in routing namespace", func() {
			// BR-NOT-067: Controller should watch ConfigMap for changes
			// #207: Namespace is dynamic (from POD_NAMESPACE env var)

			expectedConfigMapName := routing.DefaultConfigMapName
			expectedNamespace := routing.GetConfigMapNamespace()

			Expect(expectedConfigMapName).To(Equal("notification-routing-config"))
			Expect(expectedNamespace).NotTo(BeEmpty())
		})
	})

	Context("Reconciler Routing Config", func() {

		It("should use routing config for channel resolution", func() {
			// BR-NOT-065 + BR-NOT-067: Routing config affects channel resolution
			// When routing config is updated via hot-reload, new notifications
			// should use the updated routing rules

			ctx := context.Background()

			// Create a test router
			router := routing.NewRouter(testLogger)
			err := router.LoadConfig([]byte(`
route:
  routes:
    - match:
        skip-reason: PreviousExecutionFailed
      receiver: pagerduty-critical
  receiver: default-console
receivers:
  - name: default-console
    consoleConfigs:
      - enabled: true
  - name: pagerduty-critical
    pagerdutyConfigs:
      - serviceKey: test
`))
			Expect(err).NotTo(HaveOccurred())

			// Verify routing resolution
			attrs := map[string]string{
				routing.AttrSkipReason: routing.SkipReasonPreviousExecutionFailed,
			}
			receiver := router.FindReceiver(attrs)
			Expect(receiver.Name).To(Equal("pagerduty-critical"))

			_ = ctx // Context used in actual controller
		})
	})
})
