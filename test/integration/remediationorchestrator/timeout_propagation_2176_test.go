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

package remediationorchestrator

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// DD-TIMEOUT-002 / Issue #2176: proves RO's authoritative
// Status.TimeoutConfig.Processing value flows verbatim into the created
// SignalProcessing's Spec.TimesOutAt through the real production dispatch
// path (envtest -> RemediationOrchestrator controller -> PendingHandler ->
// creator.SignalProcessingCreator), not by calling the creator directly.
var _ = Describe("DD-TIMEOUT-002: RO propagates Status.TimeoutConfig.Processing to SP Spec.TimesOutAt", Label("integration", "timeout"), func() {
	var (
		namespace string
		rrName    string
	)

	BeforeEach(func() {
		namespace = createTestNamespace(ctx, "ro-2176-timeout")
		rrName = fmt.Sprintf("rr-2176-%s", uuid.New().String()[:13])
	})

	AfterEach(func() {
		deleteTestNamespace(namespace)
	})

	It("IT-ORCH-2176-001: sets SP.Spec.TimesOutAt from RR.Status.TimeoutConfig.Processing via the real PendingHandler", func() {
		By("Creating a RemediationRequest")
		_ = createRemediationRequest(namespace, rrName)

		By("Waiting for RO to initialize Status.TimeoutConfig on first reconcile")
		rr := &remediationv1.RemediationRequest{}
		Eventually(func() bool {
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: ROControllerNamespace}, rr); err != nil {
				return false
			}
			return rr.Status.TimeoutConfig != nil && rr.Status.TimeoutConfig.Processing != nil
		}, timeout, interval).Should(BeTrue(), "RO must initialize Status.TimeoutConfig.Processing before creating SP")

		processingTimeout := rr.Status.TimeoutConfig.Processing.Duration
		Expect(processingTimeout).To(BeNumerically(">", 0))

		By("Waiting for SignalProcessing CRD to be created by the real PendingHandler")
		spName := fmt.Sprintf("sp-%s", rrName)
		sp := &signalprocessingv1.SignalProcessing{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
		}, timeout, interval).Should(Succeed())

		By("Verifying SP.Spec.TimesOutAt was populated verbatim from RR.Status.TimeoutConfig.Processing")
		Expect(sp.Spec.TimesOutAt).ToNot(BeNil(),
			"DD-TIMEOUT-002: SP.Spec.TimesOutAt must be set when RO has an authoritative Processing timeout")
		// Tolerance accounts for the (typically sub-second) reconcile latency
		// between RR's TimeoutConfig initialization and SP creation.
		expectedDeadline := sp.CreationTimestamp.Add(processingTimeout)
		Expect(sp.Spec.TimesOutAt.Time).To(BeTemporally("~", expectedDeadline, 10*time.Second),
			"SP.Spec.TimesOutAt should equal SP creation time + RR's authoritative Processing duration")

		GinkgoWriter.Printf("✅ IT-ORCH-2176-001: SP.Spec.TimesOutAt=%s sourced from RR.Status.TimeoutConfig.Processing=%s\n",
			sp.Spec.TimesOutAt.Time, processingTimeout)
	})
})
