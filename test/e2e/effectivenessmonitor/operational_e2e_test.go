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

package effectivenessmonitor

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
)

// E2E Tests: Operational Scenarios (Validity Window, Fail-Fast, Graceful Shutdown)
//
// These tests validate K8s-only operational behavior that does not require
// Prometheus or AlertManager data injection.
//
// Scenarios:
//   - E2E-EM-VW-001: Delayed assessment -> EA marked expired
//   - E2E-EM-FF-001: EM started without Prometheus -> pod fails to start
//   - E2E-EM-GS-001: SIGTERM handled within shutdown timeout

var _ = Describe("EffectivenessMonitor Operational E2E Tests", Label("e2e"), func() {

	// ========================================================================
	// E2E-EM-VW-001: Validity Window Expiry
	// ========================================================================
	Describe("Validity Window (VW)", func() {
		var testNS string

		BeforeEach(func() {
			testNS = createTestNamespace("em-vw-e2e")
		})

		AfterEach(func() {
			deleteTestNamespace(testNS)
		})

		It("E2E-EM-VW-001: should mark EA as expired when validity deadline has passed", func() {
			By("Creating an EA with a validity deadline in the past")
			name := uniqueName("ea-vw-expired")
			correlationID := uniqueName("corr-vw")

			// createExpiredEA uses a two-step approach: Create + Status().Update()
			// because Kubernetes ignores status fields on Create (status is a subresource).
			createExpiredEA(testNS, name, correlationID)

			By("Waiting for EM to mark the EA as Completed with expired reason")
			ea := waitForEAPhase(testNS, name, eav1.PhaseCompleted)

			Expect(ea.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonExpired),
				"EA should have assessment reason 'expired'")

			By("Verifying no component assessments were performed")
			Expect(ea.Status.Components.HealthAssessed).To(BeFalse(),
				"Health should not be assessed for expired EA")
			Expect(ea.Status.Components.AlertAssessed).To(BeFalse(),
				"Alert should not be assessed for expired EA")
			Expect(ea.Status.Components.MetricsAssessed).To(BeFalse(),
				"Metrics should not be assessed for expired EA")
			Expect(ea.Status.Components.HashComputed).To(BeFalse(),
				"Hash should not be computed for expired EA")
		})
	})

	// ========================================================================
	// E2E-EM-FF-001: Fail-Fast Startup
	// ========================================================================
	Describe("Fail-Fast Startup (FF)", func() {
		It("E2E-EM-FF-001: should fail to start when Prometheus is unreachable", func() {
			By("Creating an EM deployment with Prometheus URL pointing to a non-existent service")
			// This test verifies that the EM controller exits with a FATAL error
			// when it cannot reach an enabled Prometheus at startup.
			//
			// Strategy: Deploy a second EM instance (em-ff-test) with a bad Prometheus URL.
			// Verify the pod enters CrashLoopBackOff or has restart count > 0.

			ffNamespace := createTestNamespace("em-ff-e2e")
			defer deleteTestNamespace(ffNamespace)

			// Get the actual EM image from the running deployment (built by SetupEMInfrastructure).
			// The image has a dynamically generated tag, so we can't hardcode it.
			imageCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
				"get", "deployment", "effectivenessmonitor-controller",
				"-n", "kubernaut-system",
				"-o", "jsonpath={.spec.template.spec.containers[0].image}")
			imageOut, err := imageCmd.Output()
			Expect(err).ToNot(HaveOccurred(), "Failed to get EM image from deployment")
			emImage := string(imageOut)
			Expect(emImage).ToNot(BeEmpty(), "EM image name should not be empty")
			GinkgoWriter.Printf("  Using EM image: %s\n", emImage)

			manifest := fmt.Sprintf(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: em-ff-test-config
  namespace: %[1]s
data:
  effectivenessmonitor.yaml: |
    assessment:
      stabilizationWindow: 30s
      validityWindow: 90s
      scoringThreshold: 0.5
    datastorage:
      url: http://data-storage-service.kubernaut-system:8080
      timeout: 5s
      buffer:
        bufferSize: 10
        batchSize: 10
        flushInterval: 5s
        maxRetries: 3
    controller:
      leaderElection: false
    external:
      prometheusUrl: http://prometheus-does-not-exist:9090
      prometheusEnabled: true
      alertManagerUrl: http://alertmanager-svc.kubernaut-system:9093
      alertManagerEnabled: true
      connectionTimeout: 3s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: em-ff-test
  namespace: %[1]s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: em-ff-test
  template:
    metadata:
      labels:
        app: em-ff-test
    spec:
      containers:
      - name: controller
        image: %[2]s
        imagePullPolicy: Never
        args:
        - "--config=/etc/effectivenessmonitor/effectivenessmonitor.yaml"
        - "--health-probe-bind-address=:8082"
        - "--metrics-bind-address=:9091"
        volumeMounts:
        - name: config
          mountPath: /etc/effectivenessmonitor
      volumes:
      - name: config
        configMap:
          name: em-ff-test-config
`, ffNamespace, emImage)

			cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(manifest)
			output, err := cmd.CombinedOutput()
			Expect(err).ToNot(HaveOccurred(), "Failed to deploy fail-fast test: %s", string(output))

			By("Waiting for the pod to enter CrashLoopBackOff or have restart count > 0")
			Eventually(func() bool {
				podListCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
					"get", "pods", "-n", ffNamespace, "-l", "app=em-ff-test",
					"-o", "jsonpath={.items[0].status.containerStatuses[0].restartCount}")
				out, err := podListCmd.Output()
				if err != nil {
					return false
				}
				// If restart count > 0, the pod has crashed at least once
				return string(out) != "" && string(out) != "0"
			}, 90*time.Second, 3*time.Second).Should(BeTrue(),
				"EM pod should crash when Prometheus is unreachable (fail-fast)")
		})
	})

	// ========================================================================
	// E2E-EM-GS-001: Graceful Shutdown
	// ========================================================================
	Describe("Graceful Shutdown (GS)", func() {
		It("E2E-EM-GS-001: should handle SIGTERM within shutdown timeout", func() {
			By("Verifying the EM controller is currently running")
			statusCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
				"get", "deployment", "effectivenessmonitor-controller",
				"-n", "kubernaut-system",
				"-o", "jsonpath={.status.readyReplicas}")
			out, err := statusCmd.Output()
			Expect(err).ToNot(HaveOccurred())
			Expect(string(out)).To(Equal("1"), "EM controller should be running")

			By("Creating an EA to ensure there is in-flight work")
			gsNS := createTestNamespace("em-gs-e2e")
			defer deleteTestNamespace(gsNS)

			name := uniqueName("ea-gs")
			correlationID := uniqueName("corr-gs")
			createTargetPod(gsNS, "target-pod")
			waitForPodReady(gsNS, "target-pod")
			createEA(gsNS, name, correlationID,
				withTargetPod("target-pod"),
			)

			By("Sending SIGTERM to the EM controller pod via kubectl delete pod")
			// Delete the pod (triggers SIGTERM); the Deployment will recreate it
			deleteCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
				"delete", "pod", "-n", "kubernaut-system",
				"-l", "app=effectivenessmonitor-controller",
				"--grace-period=30", "--wait=true")
			deleteOutput, err := deleteCmd.CombinedOutput()
			Expect(err).ToNot(HaveOccurred(),
				"Failed to delete EM pod for shutdown test: %s", string(deleteOutput))

			By("Verifying the EM controller recovers (new pod becomes ready)")
			Eventually(func() string {
				cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
					"get", "deployment", "effectivenessmonitor-controller",
					"-n", "kubernaut-system",
					"-o", "jsonpath={.status.readyReplicas}")
				out, err := cmd.Output()
				if err != nil {
					return ""
				}
				return string(out)
			}, 90*time.Second, 3*time.Second).Should(Equal("1"),
				"EM controller should recover after SIGTERM")

			By("Verifying the EA was eventually processed (may have been picked up by new pod)")
			ea := &eav1.EffectivenessAssessment{}
			Eventually(func() string {
				if err := apiReader.Get(ctx, client.ObjectKey{Namespace: gsNS, Name: name}, ea); err != nil {
					return ""
				}
				return ea.Status.Phase
			}, timeout, interval).Should(Or(
				Equal(eav1.PhaseCompleted),
				Equal(eav1.PhaseAssessing),
			), "EA should be processed after EM controller restart")
		})
	})
})

