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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrl "sigs.k8s.io/controller-runtime"

	notification "github.com/jordigilh/kubernaut/internal/controller/notification"
	"github.com/jordigilh/kubernaut/pkg/notification/credentials"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	notificationmetrics "github.com/jordigilh/kubernaut/pkg/notification/metrics"
	"github.com/jordigilh/kubernaut/pkg/notification/routing"

	"github.com/prometheus/client_golang/prometheus"
)

// =============================================================================
// Issue #244: FileWatcher Migration — ReloadRoutingFromContent Unit Tests
// =============================================================================
//
// BR-NOT-067: Routing Configuration Hot-Reload (via FileWatcher)
// BR-NOT-104: Per-receiver credential resolution for Slack delivery
//
// These tests validate the new file-content-based routing reload path that
// replaces the ConfigMap informer approach.
// =============================================================================

// newTestReconciler creates a minimal NotificationRequestReconciler for unit testing
// ReloadRoutingFromContent. Uses real Router and Orchestrator with optional credential resolver.
func newTestReconciler(credResolver *credentials.Resolver) (*notification.NotificationRequestReconciler, *delivery.Orchestrator) {
	logger := ctrl.Log.WithName("test-reload")
	router := routing.NewRouter(logger)
	metrics := notificationmetrics.NewMetricsWithRegistry(prometheus.NewRegistry())
	orchestrator := delivery.NewOrchestrator(nil, metrics, nil, logger)

	return &notification.NotificationRequestReconciler{
		Router:               router,
		DeliveryOrchestrator: orchestrator,
		CredentialResolver:   credResolver,
		SlackTimeout:         5 * time.Second,
	}, orchestrator
}

// writeCredentialFile writes a credential value to a temp directory for testing.
func writeCredentialFile(dir, name, value string) {
	err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0644)
	Expect(err).NotTo(HaveOccurred())
}

var _ = Describe("Issue #244: ReloadRoutingFromContent", func() {

	Context("Valid Configuration Reload", func() {

		It("UT-NOT-244-001: valid routing YAML produces correct Router state and registers Slack delivery service", func() {
			credDir := GinkgoT().TempDir()
			writeCredentialFile(credDir, "slack-webhook", "http://localhost:9999/webhook")

			credResolver, err := credentials.NewResolver(credDir, ctrl.Log.WithName("test-creds"))
			Expect(err).NotTo(HaveOccurred())

			reconciler, orchestrator := newTestReconciler(credResolver)

			yamlContent := `
route:
  receiver: slack-console
receivers:
  - name: slack-console
    slackConfigs:
      - channel: "#ops"
        credentialRef: slack-webhook
    consoleConfigs:
      - enabled: true
`
			err = reconciler.ReloadRoutingFromContent(yamlContent)
			Expect(err).NotTo(HaveOccurred())

			receiver := reconciler.Router.FindReceiver(map[string]string{})
			Expect(receiver.Name).To(Equal("slack-console"))

			Expect(orchestrator.HasChannel("slack:slack-console")).To(BeTrue(),
				"Per-receiver Slack delivery service should be registered under key slack:<receiver>")
		})

		It("UT-NOT-244-002: per-receiver Slack delivery services rebuilt with correct channel keys", func() {
			credDir := GinkgoT().TempDir()
			writeCredentialFile(credDir, "webhook-r1", "http://localhost:9999/r1")
			writeCredentialFile(credDir, "webhook-r2", "http://localhost:9999/r2")

			credResolver, err := credentials.NewResolver(credDir, ctrl.Log.WithName("test-creds"))
			Expect(err).NotTo(HaveOccurred())

			reconciler, orchestrator := newTestReconciler(credResolver)

			yamlContent := `
route:
  receiver: r1
  routes:
    - match:
        severity: critical
      receiver: r2
receivers:
  - name: r1
    slackConfigs:
      - channel: "#general"
        credentialRef: webhook-r1
  - name: r2
    slackConfigs:
      - channel: "#critical"
        credentialRef: webhook-r2
`
			err = reconciler.ReloadRoutingFromContent(yamlContent)
			Expect(err).NotTo(HaveOccurred())

			Expect(orchestrator.HasChannel("slack:r1")).To(BeTrue(),
				"Receiver r1 should produce channel key slack:r1")
			Expect(orchestrator.HasChannel("slack:r2")).To(BeTrue(),
				"Receiver r2 should produce channel key slack:r2")
		})
	})

	Context("Graceful Degradation", func() {

		It("UT-NOT-244-003: malformed YAML is rejected with error; previous routing preserved", func() {
			reconciler, _ := newTestReconciler(nil)

			validConfig := `
route:
  receiver: default-console
receivers:
  - name: default-console
    consoleConfigs:
      - enabled: true
`
			err := reconciler.ReloadRoutingFromContent(validConfig)
			Expect(err).NotTo(HaveOccurred())

			receiver := reconciler.Router.FindReceiver(map[string]string{})
			Expect(receiver.Name).To(Equal("default-console"))

			err = reconciler.ReloadRoutingFromContent("{{invalid yaml: [broken")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse"),
				"Error should indicate a parse failure")

			receiverAfter := reconciler.Router.FindReceiver(map[string]string{})
			Expect(receiverAfter.Name).To(Equal("default-console"),
				"Previous valid config must be preserved after malformed reload")
		})

		It("UT-NOT-244-004: config with empty credentialRef is rejected; previous delivery services preserved", func() {
			credDir := GinkgoT().TempDir()
			writeCredentialFile(credDir, "good-ref", "http://localhost:9999/webhook")

			credResolver, err := credentials.NewResolver(credDir, ctrl.Log.WithName("test-creds"))
			Expect(err).NotTo(HaveOccurred())

			reconciler, orchestrator := newTestReconciler(credResolver)

			validConfig := `
route:
  receiver: good
receivers:
  - name: good
    slackConfigs:
      - channel: "#test"
        credentialRef: good-ref
`
			err = reconciler.ReloadRoutingFromContent(validConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(orchestrator.HasChannel("slack:good")).To(BeTrue())

			badConfig := `
route:
  receiver: bad
receivers:
  - name: bad
    slackConfigs:
      - channel: "#test"
        credentialRef: ""
`
			err = reconciler.ReloadRoutingFromContent(badConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("credential"),
				"Error should indicate credential validation failure")

			Expect(orchestrator.HasChannel("slack:good")).To(BeTrue(),
				"Previous Slack delivery service must remain after rejected config")
		})

		It("UT-NOT-244-009: empty file content preserves default config; no error", func() {
			reconciler, _ := newTestReconciler(nil)

			validConfig := `
route:
  receiver: initial
receivers:
  - name: initial
    consoleConfigs:
      - enabled: true
`
			err := reconciler.ReloadRoutingFromContent(validConfig)
			Expect(err).NotTo(HaveOccurred())

			err = reconciler.ReloadRoutingFromContent("")
			Expect(err).NotTo(HaveOccurred(),
				"Empty content should not produce an error")

			receiver := reconciler.Router.FindReceiver(map[string]string{})
			Expect(receiver.Name).To(Equal("initial"),
				"Router state should be unchanged after empty content reload")
		})
	})

	Context("Credential Validation", func() {

		It("UT-NOT-244-005: missing credential file produces actionable error with ref name", func() {
			credDir := GinkgoT().TempDir()
			// Do NOT write any credential file — directory is empty

			credResolver, err := credentials.NewResolver(credDir, ctrl.Log.WithName("test-creds"))
			Expect(err).NotTo(HaveOccurred())

			reconciler, _ := newTestReconciler(credResolver)

			yamlContent := `
route:
  receiver: needs-cred
receivers:
  - name: needs-cred
    slackConfigs:
      - channel: "#test"
        credentialRef: nonexistent-webhook
`
			err = reconciler.ReloadRoutingFromContent(yamlContent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nonexistent-webhook"),
				"Error should include the unresolvable credential reference name")
		})
	})

	Context("Multi-Receiver and Channel Lifecycle", func() {

		It("UT-NOT-244-006: multiple receivers with Slack configs produce distinct delivery services", func() {
			credDir := GinkgoT().TempDir()
			writeCredentialFile(credDir, "wh-alpha", "http://localhost:9999/alpha")
			writeCredentialFile(credDir, "wh-beta", "http://localhost:9999/beta")
			writeCredentialFile(credDir, "wh-gamma", "http://localhost:9999/gamma")

			credResolver, err := credentials.NewResolver(credDir, ctrl.Log.WithName("test-creds"))
			Expect(err).NotTo(HaveOccurred())

			reconciler, orchestrator := newTestReconciler(credResolver)

			yamlContent := `
route:
  receiver: alpha
  routes:
    - match:
        tier: beta
      receiver: beta
    - match:
        tier: gamma
      receiver: gamma
receivers:
  - name: alpha
    slackConfigs:
      - channel: "#alpha"
        credentialRef: wh-alpha
  - name: beta
    slackConfigs:
      - channel: "#beta"
        credentialRef: wh-beta
  - name: gamma
    slackConfigs:
      - channel: "#gamma"
        credentialRef: wh-gamma
`
			err = reconciler.ReloadRoutingFromContent(yamlContent)
			Expect(err).NotTo(HaveOccurred())

			Expect(orchestrator.HasChannel("slack:alpha")).To(BeTrue())
			Expect(orchestrator.HasChannel("slack:beta")).To(BeTrue())
			Expect(orchestrator.HasChannel("slack:gamma")).To(BeTrue())
		})

		It("UT-NOT-244-007: config change from 3 receivers to 1 unregisters stale channels", func() {
			credDir := GinkgoT().TempDir()
			writeCredentialFile(credDir, "wh-r1", "http://localhost:9999/r1")
			writeCredentialFile(credDir, "wh-r2", "http://localhost:9999/r2")
			writeCredentialFile(credDir, "wh-r3", "http://localhost:9999/r3")

			credResolver, err := credentials.NewResolver(credDir, ctrl.Log.WithName("test-creds"))
			Expect(err).NotTo(HaveOccurred())

			reconciler, orchestrator := newTestReconciler(credResolver)

			threeReceiverConfig := `
route:
  receiver: r1
  routes:
    - match:
        team: r2
      receiver: r2
    - match:
        team: r3
      receiver: r3
receivers:
  - name: r1
    slackConfigs:
      - channel: "#r1"
        credentialRef: wh-r1
  - name: r2
    slackConfigs:
      - channel: "#r2"
        credentialRef: wh-r2
  - name: r3
    slackConfigs:
      - channel: "#r3"
        credentialRef: wh-r3
`
			err = reconciler.ReloadRoutingFromContent(threeReceiverConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(orchestrator.HasChannel("slack:r1")).To(BeTrue())
			Expect(orchestrator.HasChannel("slack:r2")).To(BeTrue())
			Expect(orchestrator.HasChannel("slack:r3")).To(BeTrue())

			oneReceiverConfig := `
route:
  receiver: r1
receivers:
  - name: r1
    slackConfigs:
      - channel: "#r1"
        credentialRef: wh-r1
`
			err = reconciler.ReloadRoutingFromContent(oneReceiverConfig)
			Expect(err).NotTo(HaveOccurred())

			Expect(orchestrator.HasChannel("slack:r1")).To(BeTrue(),
				"Active receiver's channel must remain")
			Expect(orchestrator.HasChannel("slack:r2")).To(BeFalse(),
				"Stale channel slack:r2 must be unregistered")
			Expect(orchestrator.HasChannel("slack:r3")).To(BeFalse(),
				"Stale channel slack:r3 must be unregistered")
		})
	})

	Context("Thread Safety", func() {

		It("UT-NOT-244-008: concurrent routing reloads do not corrupt state or race", func() {
			credDir := GinkgoT().TempDir()
			writeCredentialFile(credDir, "wh-a", "http://localhost:9999/a")
			writeCredentialFile(credDir, "wh-b", "http://localhost:9999/b")

			credResolver, err := credentials.NewResolver(credDir, ctrl.Log.WithName("test-creds"))
			Expect(err).NotTo(HaveOccurred())

			reconciler, orchestrator := newTestReconciler(credResolver)

			configA := `
route:
  receiver: recv-a
receivers:
  - name: recv-a
    slackConfigs:
      - channel: "#a"
        credentialRef: wh-a
`
			configB := `
route:
  receiver: recv-b
receivers:
  - name: recv-b
    slackConfigs:
      - channel: "#b"
        credentialRef: wh-b
`
			var wg sync.WaitGroup
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					defer GinkgoRecover()
					config := configA
					if idx%2 == 0 {
						config = configB
					}
					_ = reconciler.ReloadRoutingFromContent(config)
				}(i)
			}
			wg.Wait()

			receiver := reconciler.Router.FindReceiver(map[string]string{})
			Expect(receiver).NotTo(BeNil(), "Router must have a valid receiver after concurrent reloads")
			Expect(receiver.Name).To(Or(Equal("recv-a"), Equal("recv-b")),
				"Final receiver should be one of the two valid configs")

			hasA := orchestrator.HasChannel("slack:recv-a")
			hasB := orchestrator.HasChannel("slack:recv-b")
			Expect(hasA || hasB).To(BeTrue(),
				"At least one Slack channel must be registered after concurrent reloads")
		})
	})

	Context("Legacy Code Removal Verification", func() {

		It("UT-NOT-244-010: no SLACK_WEBHOOK_URL references in production code", func() {
			projectRoot := os.Getenv("PROJECT_ROOT")
			if projectRoot == "" {
				cwd, err := os.Getwd()
				Expect(err).NotTo(HaveOccurred())
				projectRoot = filepath.Join(cwd, "..", "..", "..")
			}

			searchPaths := []string{
				filepath.Join(projectRoot, "cmd", "notification"),
				filepath.Join(projectRoot, "internal", "controller", "notification"),
			}

			for _, searchPath := range searchPaths {
				if _, err := os.Stat(searchPath); os.IsNotExist(err) {
					Fail(fmt.Sprintf("Search path does not exist: %s", searchPath))
				}
			}

			cmd := exec.Command("grep", "-r", "SLACK_WEBHOOK_URL",
				searchPaths[0], searchPaths[1])
			output, err := cmd.CombinedOutput()

			if err == nil {
				matches := strings.TrimSpace(string(output))
				if matches != "" {
					Fail(fmt.Sprintf("SLACK_WEBHOOK_URL still referenced in production code:\n%s", matches))
				}
			}
			// grep exit code 1 = no matches found (expected)
		})
	})
})
