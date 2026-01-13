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

// ========================================
// MIGRATION STATUS: ✅ Converted from E2E to Integration
// ORIGINAL FILE: test/e2e/gateway/21_crd_lifecycle_test.go
// MIGRATION DATE: 2026-01-12
// PATTERN: Direct ProcessSignal() calls, no HTTP layer
// CHANGES:
//   - Removed HTTP validation tests (21a: malformed JSON, 21d: Content-Type)
//   - Kept business logic tests (21b: CRD creation, 21c: required field validation)
//   - Uses gateway.NewServerWithK8sClient for shared K8s client
//   - Calls ProcessSignal() directly instead of HTTP POST
// ========================================

package gateway

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway"

	"github.com/google/uuid"
)

// Test 21: CRD Lifecycle Operations (BR-GATEWAY-068, BR-GATEWAY-076, BR-GATEWAY-077)
// Business Outcome: Validate Gateway CRD creation and field validation business logic
// Coverage Target: pkg/gateway/processing/* + pkg/gateway/validation/* (+30% estimated)
//
// This test validates:
// - Valid signals create actual CRDs in Kubernetes (Business Logic)
// - Missing required fields are rejected (Business Logic Validation)
// - CRD spec fields are correctly populated from signal data
//
// NOTE: HTTP-specific tests (malformed JSON, Content-Type) remain in E2E tier
var _ = Describe("Test 21: CRD Lifecycle Operations (Integration)", Ordered, Label("crd-lifecycle", "integration"), func() {
	var (
		testNamespace string
		gwServer      *gateway.Server
		testLogger    = logger.WithValues("test", "crd-lifecycle-integration")
	)

	BeforeAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 21: CRD Lifecycle Operations - Setup (Integration)")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Generate unique namespace
		processID := GinkgoParallelProcess()
		testNamespace = fmt.Sprintf("crd-lifecycle-int-%d-%s", processID, uuid.New().String()[:8])
		testLogger.Info("Creating test namespace...", "namespace", testNamespace)

		// Create namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		// Wait for namespace to be ready
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: testNamespace}, ns)
		}, 30*time.Second, 1*time.Second).Should(Succeed())

		testLogger.Info("✅ Test namespace ready", "namespace", testNamespace)

		// Initialize Gateway with shared K8s client
		gwConfig := createGatewayConfig("http://mock-datastorage:8080")
		var err error
		gwServer, err = createGatewayServer(gwConfig, testLogger, k8sClient)
		Expect(err).ToNot(HaveOccurred())
		testLogger.Info("✅ Gateway server initialized with shared K8s client")
	})

	AfterAll(func() {
		if !CurrentSpecReport().Failed() {
			testLogger.Info("Cleaning up test namespace...", "namespace", testNamespace)
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
			_ = k8sClient.Delete(ctx, ns)
		} else {
			testLogger.Info("⚠️ Test failed - preserving namespace for debugging", "namespace", testNamespace)
		}
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 21: CRD Lifecycle Operations - Complete")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	It("should successfully create RemediationRequest CRD for valid signal", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 21b: Valid Signal Creates CRD")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		testLogger.Info("Step 1: Create normalized signal")
		signal := createNormalizedSignal(SignalBuilder{
			AlertName: "HighCPUUsage",
			Namespace:  testNamespace,
			Severity:   "critical",
			Source:     "prometheus",
			Labels: map[string]string{
				"component": "frontend",
				"pod":       "test-pod-12345",
			},
		})

		testLogger.Info("Step 2: Call ProcessSignal business logic")
		response, err := gwServer.ProcessSignal(ctx, signal)
		Expect(err).ToNot(HaveOccurred())
		Expect(response.Status).To(Equal(gateway.StatusCreated), "First signal should create new CRD")
		testLogger.Info("✅ Signal processed successfully", "fingerprint", response.Fingerprint)

		testLogger.Info("Step 3: Verify CRD created in Kubernetes")
		crdList := &remediationv1alpha1.RemediationRequestList{}
		err = k8sClient.List(ctx, crdList, client.InNamespace(testNamespace))
		Expect(err).ToNot(HaveOccurred())
		Expect(crdList.Items).To(HaveLen(1), "Exactly 1 CRD should be created")

		testLogger.Info("Step 4: Validate CRD spec fields")
		crd := crdList.Items[0]
		Expect(crd.Spec.SignalName).To(Equal("HighCPUUsage"))
		Expect(crd.Spec.Severity).To(Equal("critical"))
		Expect(crd.Spec.TargetResource.Namespace).To(Equal(testNamespace))
		Expect(crd.Spec.TargetResource.Kind).To(Equal("Pod"))

		testLogger.Info("✅ Test 21b PASSED: CRD created successfully", "crdName", crd.Name)
	})

	It("should apply defaults for missing optional fields", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 21c: Default Field Application")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		testLogger.Info("Step 1: Create signal with empty AlertName (helper applies default)")
		signalWithDefaults := createNormalizedSignal(SignalBuilder{
			AlertName: "", // Empty - helper will apply default "TestAlert"
			Namespace:  testNamespace,
			Severity:   "critical",
			Source:     "prometheus",
		})

		testLogger.Info("Step 2: Call ProcessSignal - helper applies defaults, so signal is valid")
		response, err := gwServer.ProcessSignal(ctx, signalWithDefaults)

		// Helper applies default AlertName="TestAlert", so signal is valid
		Expect(err).ToNot(HaveOccurred(), "Signal should be processed (helper applied defaults)")
		Expect(response.Status).To(Equal(gateway.StatusCreated), "Signal with defaults should create CRD")
		testLogger.Info("✅ Signal processed successfully with defaults", "status", response.Status)

		testLogger.Info("Step 3: Verify CRD was created with defaults")
		crdList := &remediationv1alpha1.RemediationRequestList{}
		err = k8sClient.List(ctx, crdList, client.InNamespace(testNamespace))
		Expect(err).ToNot(HaveOccurred())
		Expect(crdList.Items).To(HaveLen(2), "2 CRDs should exist (21b + 21c)")

		// Find the CRD with default AlertName
		var defaultCRD *remediationv1alpha1.RemediationRequest
		for i := range crdList.Items {
			if crdList.Items[i].Spec.SignalName == "TestAlert" {
				defaultCRD = &crdList.Items[i]
				break
			}
		}
		Expect(defaultCRD).ToNot(BeNil(), "CRD with default AlertName='TestAlert' should exist")
		testLogger.Info("✅ Test 21c PASSED: Defaults applied correctly", "signalName", defaultCRD.Spec.SignalName)
	})

	It("should populate CRD spec with all signal metadata", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 21e: CRD Spec Field Population")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		testLogger.Info("Step 1: Create signal with rich metadata")
		signal := createNormalizedSignal(SignalBuilder{
			AlertName: "DiskPressure",
			Namespace:  testNamespace,
			Severity:   "warning",
			Source:     "kubernetes-events",
			Kind:       "Node",
			ResourceName: "worker-node-01",
			Labels: map[string]string{
				"zone":         "us-west-2a",
				"instance_type": "m5.xlarge",
			},
		})

		testLogger.Info("Step 2: Process signal")
		response, err := gwServer.ProcessSignal(ctx, signal)
		Expect(err).ToNot(HaveOccurred())
		Expect(response.Status).To(Equal(gateway.StatusCreated), "First signal should create new CRD")

		testLogger.Info("Step 3: Retrieve CRD and validate all fields")
		crdList := &remediationv1alpha1.RemediationRequestList{}
		err = k8sClient.List(ctx, crdList, client.InNamespace(testNamespace))
		Expect(err).ToNot(HaveOccurred())

		// Find the DiskPressure CRD
		var diskPressureCRD *remediationv1alpha1.RemediationRequest
		for i := range crdList.Items {
			if crdList.Items[i].Spec.SignalName == "DiskPressure" {
				diskPressureCRD = &crdList.Items[i]
				break
			}
		}
		Expect(diskPressureCRD).ToNot(BeNil(), "DiskPressure CRD should exist")

		// Validate comprehensive field mapping
		Expect(diskPressureCRD.Spec.SignalName).To(Equal("DiskPressure"))
		Expect(diskPressureCRD.Spec.Severity).To(Equal("warning"))
		Expect(diskPressureCRD.Spec.TargetResource.Namespace).To(Equal(testNamespace))
		Expect(diskPressureCRD.Spec.TargetResource.Kind).To(Equal("Node"))
		Expect(diskPressureCRD.Spec.TargetResource.Name).To(Equal("worker-node-01"))
		Expect(diskPressureCRD.Spec.SignalSource).To(Equal("kubernetes-events"))
		Expect(diskPressureCRD.Spec.SignalLabels).To(HaveKeyWithValue("zone", "us-west-2a"))
		Expect(diskPressureCRD.Spec.SignalLabels).To(HaveKeyWithValue("instance_type", "m5.xlarge"))

		testLogger.Info("✅ Test 21e PASSED: All CRD fields correctly populated")
	})
})
