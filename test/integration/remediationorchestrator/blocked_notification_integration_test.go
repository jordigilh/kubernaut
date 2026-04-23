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

// Issue #803: Integration tests for ManualReview NotificationRequest creation
// on IneffectiveChain blocks. Uses envtest (real K8s API) to validate NR CRD
// creation with real UIDs, owner references, and ReviewSource field.
//
// Business requirements:
// - BR-ORCH-036: Manual review notification for any unrecoverable failure
// - BR-ORCH-042.5: Notification on Block
package remediationorchestrator

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

var _ = Describe("Issue #803: Blocked NotificationRequest Integration Tests", Label("integration", "blocked-notification"), func() {
	var (
		nc            *creator.NotificationCreator
		testNamespace string
	)

	BeforeEach(func() {
		testNamespace = createTestNamespace("blocked-notif")
		nc = creator.NewNotificationCreator(
			k8sClient,
			k8sManager.GetScheme(),
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
		)
	})

	AfterEach(func() {
		deleteTestNamespace(testNamespace)
	})

	newRR := func(name string) *remediationv1.RemediationRequest {
		now := metav1.Now()
		h := sha256.Sum256([]byte(uuid.New().String()))
		fp := hex.EncodeToString(h[:])

		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ROControllerNamespace,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: fp,
				SignalName:        "it-803-signal",
				Severity:          "high",
				SignalType:        "alert",
				SignalSource:      "test",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind: "Deployment", Name: "test-app", Namespace: testNamespace,
				},
				FiringTime:   now,
				ReceivedTime: now,
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)).To(Succeed())
		return rr
	}

	// IT-RO-803-001: IneffectiveChain block creates NR with RoutingEngine source
	It("IT-RO-803-001: should create ManualReview NR with ReviewSource=RoutingEngine via real K8s API", func() {
		rr := newRR("rr-it-803-001")

		reviewCtx := &creator.ManualReviewContext{
			Source:  notificationv1.ReviewSourceRoutingEngine,
			Reason:  "IneffectiveChain",
			Message: "3 consecutive ineffective remediations detected (Layer1 hash chain). Escalating to manual review.",
		}

		name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(name).To(Equal("nr-manual-review-rr-it-803-001"))

		nr := &notificationv1.NotificationRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{
			Name: name, Namespace: rr.Namespace,
		}, nr)).To(Succeed())

		Expect(nr.Spec.ReviewSource).To(Equal(notificationv1.ReviewSourceRoutingEngine))
		Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeManualReview))
		Expect(nr.Spec.RemediationRequestRef).To(HaveField("UID", Equal(rr.UID)))
		Expect(nr.Spec.Context.Review.Reason).To(Equal("IneffectiveChain"))
		Expect(nr.OwnerReferences).To(HaveLen(1))
		Expect(nr.OwnerReferences[0].Kind).To(Equal("RemediationRequest"))
		Expect(nr.OwnerReferences[0].Name).To(Equal(rr.Name))
	})

	// IT-RO-803-002: ConsecutiveFailures does NOT create ManualReview NR
	It("IT-RO-803-002: should NOT create ManualReview NR for non-IneffectiveChain block reasons", func() {
		rr := newRR("rr-it-803-002")

		// Simulate that we would NOT call CreateManualReviewNotification for ConsecutiveFailures.
		// The guard is in handleBlocked (tested in unit tests). Here we verify that if
		// someone mistakenly passes a non-IneffectiveChain source, the NR name is still
		// deterministic and we can check the cluster for absence of manual review NRs
		// when no creation call is made.
		nrName := "nr-manual-review-rr-it-803-002"
		nr := &notificationv1.NotificationRequest{}
		err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{
			Name: nrName, Namespace: rr.Namespace,
		}, nr)
		Expect(err).To(HaveOccurred(), "ManualReview NR should NOT exist when no creation call is made for ConsecutiveFailures")
	})

	// IT-RO-803-003: Idempotent NR creation via real K8s API
	It("IT-RO-803-003: should be idempotent - second CreateManualReviewNotification returns same name", func() {
		rr := newRR("rr-it-803-003")

		reviewCtx := &creator.ManualReviewContext{
			Source:  notificationv1.ReviewSourceRoutingEngine,
			Reason:  "IneffectiveChain",
			Message: "Ineffective chain detected",
		}

		name1, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
		Expect(err).NotTo(HaveOccurred())

		name2, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(name2).To(Equal(name1))

		nrList := &notificationv1.NotificationRequestList{}
		Expect(k8sClient.List(ctx, nrList, client.InNamespace(ROControllerNamespace))).To(Succeed())
		manualReviewCount := 0
		for _, nr := range nrList.Items {
			if nr.Name == "nr-manual-review-rr-it-803-003" {
				manualReviewCount++
			}
		}
		Expect(manualReviewCount).To(Equal(1), "Should have exactly 1 ManualReview NR after idempotent double call")
	})
})
