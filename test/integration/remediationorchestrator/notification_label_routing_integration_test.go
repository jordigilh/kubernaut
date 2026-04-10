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

// Integration tests for Issue #416: Label-based notification routing.
// These tests validate dual NR creation and owner label resolution
// against a live K8s API (envtest).
//
// Business Requirements:
// - BR-ORCH-416 (Label-based notification routing)
//
// Defense-in-Depth:
// - Unit tests: Pure logic (DetermineNotificationTarget, BuildNotificationExtensions)
// - Integration tests: Real K8s API interaction (this file)

package remediationorchestrator

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
)

var _ = Describe("Label-Based Notification Routing Integration (#416)", func() {
	var testNamespace string

	BeforeEach(func() {
		testNamespace = createTestNamespace("label-routing")
	})

	AfterEach(func() {
		deleteTestNamespace(testNamespace)
	})

	// IT-RO-416-001: Dual NR creation with real K8s resources via envtest.
	// When signal target differs from RCA target, two NRs must be created
	// with correct Extensions carrying routing keys.
	It("IT-RO-416-001: should create dual NRs with correct Extensions when targets differ", func() {
		signalTarget := remediationv1.ResourceIdentifier{
			Kind:      "Deployment",
			Name:      "web-frontend",
			Namespace: testNamespace,
		}
		rcaTarget := remediationv1.ResourceIdentifier{
			Kind:      "Pod",
			Name:      "web-frontend-abc123",
			Namespace: testNamespace,
		}

		// Create the Deployment with owner labels in envtest
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web-frontend",
				Namespace: testNamespace,
				Labels: map[string]string{
					creator.LabelTeam:  "frontend-team",
					creator.LabelOwner: "alice",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "web-frontend"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web-frontend"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx"}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, deploy)).To(Succeed())

		// Step 1: Resolve owner labels from live K8s
		ownerLabels, err := creator.ResolveOwnerLabels(ctx, k8sClient, signalTarget)
		Expect(err).ToNot(HaveOccurred())
		Expect(ownerLabels).To(HaveKeyWithValue(creator.LabelTeam, "frontend-team"))

		// Step 2: Determine notification targets
		notifTarget, isDual := creator.DetermineNotificationTarget(signalTarget, &rcaTarget)
		Expect(isDual).To(BeTrue())
		Expect(notifTarget).To(Equal("signal"))

		// Step 3: Build Extensions for signal NR
		signalExt := creator.BuildNotificationExtensions(signalTarget, "signal", ownerLabels)
		Expect(signalExt).To(HaveKeyWithValue("notification-target", "signal"))
		Expect(signalExt).To(HaveKeyWithValue("team", "frontend-team"))

		// Step 4: Build Extensions for RCA NR
		rcaExt := creator.BuildNotificationExtensions(rcaTarget, "rca", ownerLabels)
		Expect(rcaExt).To(HaveKeyWithValue("notification-target", "rca"))
		Expect(rcaExt).To(HaveKeyWithValue("target-kind", "Pod"))

		// Step 5: Create both NRs in envtest
		fingerprint := func() string {
			h := sha256.Sum256([]byte(uuid.New().String()))
			return hex.EncodeToString(h[:])
		}
		rrName := "rr-dual-nr-" + uuid.New().String()[:8]
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rrName,
				Namespace: ROControllerNamespace,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: fingerprint(),
				SignalName:        "HighMemoryAlert",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource:    signalTarget,
				FiringTime:        metav1.Now(),
				ReceivedTime:      metav1.Now(),
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())

		signalNR := &notificationv1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rrName + "-signal",
				Namespace: ROControllerNamespace,
			},
			Spec: notificationv1.NotificationRequestSpec{
				RemediationRequestRef: &corev1.ObjectReference{
					Name:      rr.Name,
					Namespace: rr.Namespace,
				},
				Type:       notificationv1.NotificationTypeEscalation,
				Priority:   notificationv1.NotificationPriorityCritical,
				Subject:    "Signal notification for " + signalTarget.Name,
				Body:       "Test signal NR",
				Extensions: signalExt,
			},
		}
		Expect(k8sClient.Create(ctx, signalNR)).To(Succeed())

		rcaNR := &notificationv1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rrName + "-rca",
				Namespace: ROControllerNamespace,
			},
			Spec: notificationv1.NotificationRequestSpec{
				RemediationRequestRef: &corev1.ObjectReference{
					Name:      rr.Name,
					Namespace: rr.Namespace,
				},
				Type:       notificationv1.NotificationTypeEscalation,
				Priority:   notificationv1.NotificationPriorityCritical,
				Subject:    "RCA notification for " + rcaTarget.Name,
				Body:       "Test RCA NR",
				Extensions: rcaExt,
			},
		}
		Expect(k8sClient.Create(ctx, rcaNR)).To(Succeed())

		// Verify both NRs exist with correct Extensions
		nrList := &notificationv1.NotificationRequestList{}
		Expect(k8sClient.List(ctx, nrList,
			client.InNamespace(ROControllerNamespace),
			client.MatchingLabels{},
		)).To(Succeed())

		var foundSignal, foundRCA bool
		for _, nr := range nrList.Items {
			if nr.Name == rrName+"-signal" {
				foundSignal = true
				Expect(nr.Spec.Extensions).To(HaveKeyWithValue("notification-target", "signal"))
				Expect(nr.Spec.Extensions).To(HaveKeyWithValue("team", "frontend-team"))
			}
			if nr.Name == rrName+"-rca" {
				foundRCA = true
				Expect(nr.Spec.Extensions).To(HaveKeyWithValue("notification-target", "rca"))
				Expect(nr.Spec.Extensions).To(HaveKeyWithValue("target-kind", "Pod"))
			}
		}
		Expect(foundSignal).To(BeTrue(), "signal NR should exist")
		Expect(foundRCA).To(BeTrue(), "RCA NR should exist")
	})

	// IT-RO-416-002: Owner label resolution from live K8s resources.
	// Validates the full label resolution path including namespace fallback
	// against a real kube-apiserver (envtest).
	It("IT-RO-416-002: should resolve owner labels from live K8s resource and namespace", func() {
		// Create Deployment WITH owner labels
		deployWithLabels := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "labeled-app",
				Namespace: testNamespace,
				Labels: map[string]string{
					creator.LabelTeam:  "backend-team",
					creator.LabelOwner: "bob",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "labeled-app"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "labeled-app"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx"}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, deployWithLabels)).To(Succeed())

		// Create Deployment WITHOUT owner labels (should fall back to namespace)
		deployNoLabels := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unlabeled-app",
				Namespace: testNamespace,
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "unlabeled-app"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "unlabeled-app"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx"}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, deployNoLabels)).To(Succeed())

		// Patch the test namespace with owner labels for fallback
		ns := &corev1.Namespace{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: testNamespace}, ns)).To(Succeed())
		if ns.Labels == nil {
			ns.Labels = make(map[string]string)
		}
		ns.Labels[creator.LabelTeam] = "platform-eng"
		Expect(k8sClient.Update(ctx, ns)).To(Succeed())

		// Case A: Resource has labels -> read from resource
		labelsA, err := creator.ResolveOwnerLabels(ctx, k8sClient, remediationv1.ResourceIdentifier{
			Kind:      "Deployment",
			Name:      "labeled-app",
			Namespace: testNamespace,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(labelsA).To(HaveKeyWithValue(creator.LabelTeam, "backend-team"))
		Expect(labelsA).To(HaveKeyWithValue(creator.LabelOwner, "bob"))

		// Case B: Resource has no labels -> fall back to namespace
		labelsB, err := creator.ResolveOwnerLabels(ctx, k8sClient, remediationv1.ResourceIdentifier{
			Kind:      "Deployment",
			Name:      "unlabeled-app",
			Namespace: testNamespace,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(labelsB).To(HaveKeyWithValue(creator.LabelTeam, "platform-eng"))
	})
})
