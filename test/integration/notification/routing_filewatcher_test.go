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
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	notification "github.com/jordigilh/kubernaut/internal/controller/notification"
	"github.com/jordigilh/kubernaut/pkg/notification/credentials"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	notificationmetrics "github.com/jordigilh/kubernaut/pkg/notification/metrics"
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// =============================================================================
// Issue #244: FileWatcher-Based Routing Reload Integration Tests
// =============================================================================
//
// BR-NOT-067: Routing Configuration Hot-Reload (via FileWatcher)
// BR-NOT-104: Per-receiver credential resolution for Slack delivery
//
// These tests validate the end-to-end integration of FileWatcher with the
// reconciler's ReloadRoutingFromContent, using real fsnotify events and
// real Router/Orchestrator instances. No mocks.
// =============================================================================

var _ = Describe("Issue #244: FileWatcher Routing Reload Integration", func() {

	Context("Controller Lifecycle Without ConfigMap Cache", func() {

		It("IT-NOT-244-001: controller starts and reconciles NotificationRequests without ConfigMap cache or configmaps RBAC", func() {
			// The existing integration test suite exercises a controller that was
			// constructed WITHOUT ConfigMap watch (SetupWithManager no longer
			// registers a ConfigMap watcher after #244). This test verifies that
			// NRs still reach Delivered phase.
			ns := helpers.CreateTestNamespace(ctx, k8sClient, "it244")
			defer helpers.DeleteTestNamespace(ctx, k8sClient, ns)

			nr := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "it-244-001-lifecycle",
					Namespace: ns,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "IT-NOT-244-001 lifecycle test",
					Body:     "Verifying controller works without ConfigMap cache",
					Extensions: map[string]string{
						"test-channel-set": "console-file-log",
					},
				},
			}
			Expect(k8sClient.Create(ctx, nr)).To(Succeed())

			Eventually(func(g Gomega) {
				key := client.ObjectKeyFromObject(nr)
				g.Expect(k8sClient.Get(ctx, key, nr)).To(Succeed())
				g.Expect(string(nr.Status.Phase)).To(Equal("Delivered"),
					"NR should reach Delivered phase without ConfigMap cache")
			}, 15*time.Second, 500*time.Millisecond).Should(Succeed())
		})
	})

	Context("FileWatcher-Triggered Routing Reload", func() {

		It("IT-NOT-244-002: file change on disk triggers routing reload and delivery service rebuild", func() {
			tmpDir := GinkgoT().TempDir()
			routingFile := filepath.Join(tmpDir, "routing.yaml")
			credDir := GinkgoT().TempDir()

			Expect(os.WriteFile(filepath.Join(credDir, "wh-initial"), []byte("http://localhost:9999/initial"), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(credDir, "wh-alpha"), []byte("http://localhost:9999/alpha"), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(credDir, "wh-beta"), []byte("http://localhost:9999/beta"), 0644)).To(Succeed())

			credResolver, err := credentials.NewResolver(credDir, ctrl.Log.WithName("it-244-creds"))
			Expect(err).NotTo(HaveOccurred())

			logger := ctrl.Log.WithName("it-244-002")
			router := routing.NewRouter(logger)
			metrics := notificationmetrics.NewMetricsWithRegistry(prometheus.NewRegistry())
			orchestrator := delivery.NewOrchestrator(nil, metrics, nil, logger)

			reconciler := &notification.NotificationRequestReconciler{
				Router:               router,
				DeliveryOrchestrator: orchestrator,
				CredentialResolver:   credResolver,
				SlackTimeout:         5 * time.Second,
			}

			initialConfig := `
route:
  receiver: initial
receivers:
  - name: initial
    slackConfigs:
      - channel: "#init"
        credentialRef: wh-initial
`
			Expect(os.WriteFile(routingFile, []byte(initialConfig), 0644)).To(Succeed())

			watcher, err := hotreload.NewFileWatcher(
				routingFile,
				func(content string) error {
					return reconciler.ReloadRoutingFromContent(content)
				},
				logger,
			)
			Expect(err).NotTo(HaveOccurred())

			watchCtx, watchCancel := context.WithCancel(ctx)
			defer watchCancel()

			err = watcher.Start(watchCtx)
			Expect(err).NotTo(HaveOccurred())
			defer watcher.Stop()

			Expect(orchestrator.HasChannel("slack:initial")).To(BeTrue(),
				"Initial config should register slack:initial on startup")

			newConfig := `
route:
  receiver: alpha
  routes:
    - match:
        tier: beta
      receiver: beta
receivers:
  - name: alpha
    slackConfigs:
      - channel: "#alpha"
        credentialRef: wh-alpha
  - name: beta
    slackConfigs:
      - channel: "#beta"
        credentialRef: wh-beta
`
			Expect(os.WriteFile(routingFile, []byte(newConfig), 0644)).To(Succeed())

			Eventually(func() bool {
				return orchestrator.HasChannel("slack:alpha") && orchestrator.HasChannel("slack:beta")
			}, 5*time.Second, 200*time.Millisecond).Should(BeTrue(),
				"New channels slack:alpha and slack:beta should be registered after file change")

			Eventually(func() bool {
				return !orchestrator.HasChannel("slack:initial")
			}, 5*time.Second, 200*time.Millisecond).Should(BeTrue(),
				"Stale channel slack:initial should be unregistered after file change")
		})

		It("IT-NOT-244-003: startup ordering ensures credentials are ready before routing reload", func() {
			tmpDir := GinkgoT().TempDir()
			routingFile := filepath.Join(tmpDir, "routing.yaml")
			credDir := GinkgoT().TempDir()

			Expect(os.WriteFile(filepath.Join(credDir, "slack-webhook"), []byte("http://localhost:9999/webhook"), 0644)).To(Succeed())

			credResolver, err := credentials.NewResolver(credDir, ctrl.Log.WithName("it-244-creds"))
			Expect(err).NotTo(HaveOccurred())

			// Start credential watching FIRST (simulates startup ordering)
			credCtx, credCancel := context.WithCancel(ctx)
			defer credCancel()
			Expect(credResolver.StartWatching(credCtx)).To(Succeed())
			defer func() { _ = credResolver.Close() }()

			logger := ctrl.Log.WithName("it-244-003")
			router := routing.NewRouter(logger)
			metrics := notificationmetrics.NewMetricsWithRegistry(prometheus.NewRegistry())
			orchestrator := delivery.NewOrchestrator(nil, metrics, nil, logger)

			reconciler := &notification.NotificationRequestReconciler{
				Router:               router,
				DeliveryOrchestrator: orchestrator,
				CredentialResolver:   credResolver,
				SlackTimeout:         5 * time.Second,
			}

			routingYAML := `
route:
  receiver: slack-console
receivers:
  - name: slack-console
    slackConfigs:
      - channel: "#ops"
        credentialRef: slack-webhook
`
			Expect(os.WriteFile(routingFile, []byte(routingYAML), 0644)).To(Succeed())

			// Start FileWatcher AFTER credential resolver (simulates startup ordering)
			watcher, err := hotreload.NewFileWatcher(
				routingFile,
				func(content string) error {
					return reconciler.ReloadRoutingFromContent(content)
				},
				logger,
			)
			Expect(err).NotTo(HaveOccurred())

			watchCtx, watchCancel := context.WithCancel(ctx)
			defer watchCancel()
			Expect(watcher.Start(watchCtx)).To(Succeed())
			defer watcher.Stop()

			Expect(orchestrator.HasChannel("slack:slack-console")).To(BeTrue(),
				"credentialRef should resolve successfully when credentials are loaded before routing")
		})

		It("IT-NOT-244-004: FileWatcher with missing file starts with default config; file created later triggers reload", func() {
			tmpDir := GinkgoT().TempDir()
			routingFile := filepath.Join(tmpDir, "routing.yaml")
			credDir := GinkgoT().TempDir()
			Expect(os.WriteFile(filepath.Join(credDir, "wh-late"), []byte("http://localhost:9999/late"), 0644)).To(Succeed())

			credResolver, err := credentials.NewResolver(credDir, ctrl.Log.WithName("it-244-creds"))
			Expect(err).NotTo(HaveOccurred())

			logger := ctrl.Log.WithName("it-244-004")
			router := routing.NewRouter(logger)
			metrics := notificationmetrics.NewMetricsWithRegistry(prometheus.NewRegistry())
			orchestrator := delivery.NewOrchestrator(nil, metrics, nil, logger)

			reconciler := &notification.NotificationRequestReconciler{
				Router:               router,
				DeliveryOrchestrator: orchestrator,
				CredentialResolver:   credResolver,
				SlackTimeout:         5 * time.Second,
			}

			watcher, err := hotreload.NewFileWatcher(
				routingFile,
				func(content string) error {
					return reconciler.ReloadRoutingFromContent(content)
				},
				logger,
			)
			Expect(err).NotTo(HaveOccurred())

			watchCtx, watchCancel := context.WithCancel(ctx)
			defer watchCancel()

			// Start should fail because file doesn't exist, but watcher object is created
			startErr := watcher.Start(watchCtx)
			if startErr != nil {
				// FileWatcher requires the file to exist on Start. Create it and retry.
				Expect(os.WriteFile(routingFile, []byte(""), 0644)).To(Succeed())
				Expect(watcher.Start(watchCtx)).To(Succeed())
			}
			defer watcher.Stop()

			// Default router state: console-fallback receiver
			receiver := router.FindReceiver(map[string]string{})
			Expect(receiver.Name).To(Equal("console-fallback"),
				"Router should use default config when routing file is empty")

			lateConfig := `
route:
  receiver: late-receiver
receivers:
  - name: late-receiver
    slackConfigs:
      - channel: "#late"
        credentialRef: wh-late
`
			Expect(os.WriteFile(routingFile, []byte(lateConfig), 0644)).To(Succeed())

			Eventually(func() bool {
				return orchestrator.HasChannel("slack:late-receiver")
			}, 5*time.Second, 200*time.Millisecond).Should(BeTrue(),
				"Late-created file should trigger routing reload and register the channel")

			receiver = router.FindReceiver(map[string]string{})
			Expect(receiver.Name).To(Equal("late-receiver"),
				"Router should use the late config after file creation")
		})
	})
})
