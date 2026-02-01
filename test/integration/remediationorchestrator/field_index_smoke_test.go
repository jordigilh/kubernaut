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

package remediationorchestrator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// Field Index Smoke Test
// This test verifies that the field index on spec.signalFingerprint works
// It runs before other tests to catch setup issues early
var _ = Describe("Field Index Smoke Test", func() {
	var testNamespace string

	BeforeEach(func() {
		testNamespace = createTestNamespace("field-index-smoke")
	})

	AfterEach(func() {
		deleteTestNamespace(testNamespace)
	})

	It("should successfully query by spec.signalFingerprint using field index", func() {
		By("Creating a test RemediationRequest")
		testFingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "smoke-test-rr",
				Namespace: testNamespace,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: testFingerprint,
				SignalName:        "smoke-test-signal",
				Severity:          "critical",
				SignalType:        "test",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: testNamespace,
				},
				FiringTime:   metav1.Now(),
				ReceivedTime: metav1.Now(),
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())
		GinkgoWriter.Printf("✅ Created RR: %s with fingerprint: %s\n", rr.Name, testFingerprint)

		By("Verifying RR exists via direct query")
		allRRs := &remediationv1.RemediationRequestList{}
		err := k8sManager.GetAPIReader().List(ctx, allRRs, client.InNamespace(testNamespace))
		Expect(err).ToNot(HaveOccurred())
		GinkgoWriter.Printf("📊 Direct query found %d RRs in namespace\n", len(allRRs.Items))
		Expect(len(allRRs.Items)).To(Equal(1), "Should find 1 RR via direct query")

		By("Querying by field selector (spec.signalFingerprint) - server-side with CRD selectableFields")
		indexedRRs := &remediationv1.RemediationRequestList{}
		err = k8sManager.GetAPIReader().List(ctx, indexedRRs,
			client.InNamespace(testNamespace),
			client.MatchingFields{"spec.signalFingerprint": testFingerprint},
		)

		if err != nil {
			GinkgoWriter.Printf("❌ Field index query error: %v (type: %T)\n", err, err)
			Fail("Field index query failed")
		}

		GinkgoWriter.Printf("📊 Field index query found %d RRs\n", len(indexedRRs.Items))

		if len(indexedRRs.Items) == 0 {
			GinkgoWriter.Println("❌ SMOKE TEST FAILED: Field index returned 0 results")
			GinkgoWriter.Println("   This indicates field index is not working in envtest")
			GinkgoWriter.Println("   Expected: 1 RR matching fingerprint")
			GinkgoWriter.Println("   Actual: 0 RRs")

			// Additional debugging
			for _, rr := range allRRs.Items {
				GinkgoWriter.Printf("   Found RR: %s, fingerprint=%s (len=%d)\n",
					rr.Name, rr.Spec.SignalFingerprint, len(rr.Spec.SignalFingerprint))
			}
		}

		Expect(len(indexedRRs.Items)).To(Equal(1), "Field index should return 1 RR")
		Expect(indexedRRs.Items[0].Name).To(Equal("smoke-test-rr"))
		Expect(indexedRRs.Items[0].Spec.SignalFingerprint).To(Equal(testFingerprint))

		GinkgoWriter.Println("✅ SMOKE TEST PASSED: Field index working correctly")
	})
})
