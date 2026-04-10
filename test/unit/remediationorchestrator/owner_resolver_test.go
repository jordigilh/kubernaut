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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
)

// Issue #416: Owner label resolution for label-based notification routing
var _ = Describe("Owner Label Resolution (#416)", func() {

	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
		_ = appsv1.AddToScheme(scheme)
	})

	// UT-RO-416-001: ResolveOwnerLabels reads kubernaut.ai/team from resource labels
	It("UT-RO-416-001: should read kubernaut.ai/team from resource labels", func() {
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-app",
				Namespace: "production",
				Labels: map[string]string{
					creator.LabelTeam:  "sre-platform",
					creator.LabelOwner: "jdoe",
				},
			},
		}
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deploy).Build()

		target := remediationv1.ResourceIdentifier{
			Kind:      "Deployment",
			Name:      "my-app",
			Namespace: "production",
		}

		labels, err := creator.ResolveOwnerLabels(ctx, k8sClient, target)
		Expect(err).ToNot(HaveOccurred())
		Expect(labels[creator.LabelTeam]).To(Equal("sre-platform"))
		Expect(labels[creator.LabelOwner]).To(Equal("jdoe"))
	})

	// UT-RO-416-002: ResolveOwnerLabels falls back to namespace labels
	It("UT-RO-416-002: should fall back to namespace labels when resource has no labels", func() {
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-app",
				Namespace: "production",
			},
		}
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "production",
				Labels: map[string]string{
					creator.LabelTeam: "platform-eng",
				},
			},
		}
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deploy, ns).Build()

		target := remediationv1.ResourceIdentifier{
			Kind:      "Deployment",
			Name:      "my-app",
			Namespace: "production",
		}

		labels, err := creator.ResolveOwnerLabels(ctx, k8sClient, target)
		Expect(err).ToNot(HaveOccurred())
		Expect(labels[creator.LabelTeam]).To(Equal("platform-eng"))
	})

	// UT-RO-416-003: ResolveOwnerLabels returns empty map when resource is deleted
	It("UT-RO-416-003: should return empty map when resource is deleted", func() {
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		target := remediationv1.ResourceIdentifier{
			Kind:      "Deployment",
			Name:      "deleted-app",
			Namespace: "production",
		}

		labels, err := creator.ResolveOwnerLabels(ctx, k8sClient, target)
		Expect(err).ToNot(HaveOccurred())
		Expect(labels).To(BeEmpty())
	})

	// UT-RO-416-004: ResolveOwnerLabels handles cluster-scoped resources
	It("UT-RO-416-004: should skip namespace fallback for cluster-scoped resources", func() {
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-1",
				Labels: map[string]string{
					creator.LabelTeam: "infra",
				},
			},
		}
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()

		target := remediationv1.ResourceIdentifier{
			Kind: "Node",
			Name: "worker-1",
		}

		labels, err := creator.ResolveOwnerLabels(ctx, k8sClient, target)
		Expect(err).ToNot(HaveOccurred())
		Expect(labels[creator.LabelTeam]).To(Equal("infra"))
	})
})

// Issue #416: Dual NR creation helpers for label-based notification routing
var _ = Describe("Dual NR Creation Helpers (#416)", func() {

	// UT-RO-416-005: Dual NR when signal target != RCA target (2 NRs created)
	It("UT-RO-416-005: should return dual=true when signal and RCA targets differ", func() {
		signalTarget := remediationv1.ResourceIdentifier{
			Kind:      "Deployment",
			Name:      "my-app",
			Namespace: "production",
		}
		rcaTarget := remediationv1.ResourceIdentifier{
			Kind:      "Pod",
			Name:      "my-app-xyz",
			Namespace: "production",
		}

		notifTarget, isDual := creator.DetermineNotificationTarget(signalTarget, &rcaTarget)
		Expect(notifTarget).To(Equal("signal"))
		Expect(isDual).To(BeTrue(), "different targets require dual NR creation")
	})

	// UT-RO-416-006: Single NR with notification-target=both when signal == RCA target
	It("UT-RO-416-006: should return both when signal and RCA targets are identical", func() {
		signalTarget := remediationv1.ResourceIdentifier{
			Kind:      "Deployment",
			Name:      "my-app",
			Namespace: "production",
		}
		rcaTarget := remediationv1.ResourceIdentifier{
			Kind:      "Deployment",
			Name:      "my-app",
			Namespace: "production",
		}

		notifTarget, isDual := creator.DetermineNotificationTarget(signalTarget, &rcaTarget)
		Expect(notifTarget).To(Equal("both"))
		Expect(isDual).To(BeFalse(), "identical targets need only one NR")
	})

	// UT-RO-416-007: Signal-only NR when RemediationTarget is nil
	It("UT-RO-416-007: should return signal when RCA target is nil", func() {
		signalTarget := remediationv1.ResourceIdentifier{
			Kind:      "Deployment",
			Name:      "my-app",
			Namespace: "production",
		}

		notifTarget, isDual := creator.DetermineNotificationTarget(signalTarget, nil)
		Expect(notifTarget).To(Equal("signal"))
		Expect(isDual).To(BeFalse(), "nil RCA means signal-only NR")
	})

	// UT-RO-416-008: NR Extensions carry notification-target, team, owner, target-kind, namespace
	It("UT-RO-416-008: should build Extensions with all routing keys", func() {
		target := remediationv1.ResourceIdentifier{
			Kind:      "Deployment",
			Name:      "my-app",
			Namespace: "production",
		}
		ownerLabels := map[string]string{
			creator.LabelTeam:  "sre-platform",
			creator.LabelOwner: "jdoe",
		}

		ext := creator.BuildNotificationExtensions(target, "signal", ownerLabels)
		Expect(ext).To(HaveKeyWithValue("notification-target", "signal"))
		Expect(ext).To(HaveKeyWithValue("target-kind", "Deployment"))
		Expect(ext).To(HaveKeyWithValue("namespace", "production"))
		Expect(ext).To(HaveKeyWithValue("team", "sre-platform"))
		Expect(ext).To(HaveKeyWithValue("owner", "jdoe"))
	})

	// UT-RO-416-008 addendum: cluster-scoped target omits namespace
	It("UT-RO-416-008: should omit namespace for cluster-scoped targets", func() {
		target := remediationv1.ResourceIdentifier{
			Kind: "Node",
			Name: "worker-1",
		}

		ext := creator.BuildNotificationExtensions(target, "signal", map[string]string{})
		Expect(ext).To(HaveKeyWithValue("notification-target", "signal"))
		Expect(ext).To(HaveKeyWithValue("target-kind", "Node"))
		Expect(ext).ToNot(HaveKey("namespace"))
	})

	// UT-RO-416-009: Status aggregation worst-status-wins
	It("UT-RO-416-009: should return worst status from multiple NR statuses", func() {
		Expect(creator.AggregateNotificationStatus([]string{"Sent", "Failed"})).To(Equal("Failed"))
		Expect(creator.AggregateNotificationStatus([]string{"Sent", "InProgress"})).To(Equal("InProgress"))
		Expect(creator.AggregateNotificationStatus([]string{"Pending", "Sent"})).To(Equal("Pending"))
		Expect(creator.AggregateNotificationStatus([]string{"Sent"})).To(Equal("Sent"))
		Expect(creator.AggregateNotificationStatus([]string{})).To(BeEmpty())
	})
})
