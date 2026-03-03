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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
)

var _ = Describe("IsBuiltInGroup — sync vs async target classification (DD-EM-004, BR-RO-103.1)", func() {

	Context("Built-in K8s groups: target is sync-managed, hash computed immediately", func() {
		DescribeTable("UT-RO-251-001..003: should identify built-in groups as sync targets",
			func(group string) {
				Expect(creator.IsBuiltInGroup(group)).To(BeTrue(),
					"BR-RO-103.1: built-in group %q means resource is not operator-managed, no hash deferral needed", group)
			},
			Entry("UT-RO-251-001: core group (Pod, Service, ConfigMap)", ""),
			Entry("UT-RO-251-002: apps group (Deployment, StatefulSet)", "apps"),
			Entry("UT-RO-251-003: batch group (Job, CronJob)", "batch"),
			Entry("autoscaling group (HPA)", "autoscaling"),
			Entry("networking.k8s.io group (Ingress, NetworkPolicy)", "networking.k8s.io"),
			Entry("policy group (PodDisruptionBudget)", "policy"),
			Entry("rbac.authorization.k8s.io group", "rbac.authorization.k8s.io"),
			Entry("storage.k8s.io group (StorageClass, PVC)", "storage.k8s.io"),
			Entry("coordination.k8s.io group (Lease)", "coordination.k8s.io"),
			Entry("node.k8s.io group (RuntimeClass)", "node.k8s.io"),
			Entry("scheduling.k8s.io group (PriorityClass)", "scheduling.k8s.io"),
			Entry("discovery.k8s.io group (EndpointSlice)", "discovery.k8s.io"),
			Entry("admissionregistration.k8s.io group (Webhooks)", "admissionregistration.k8s.io"),
		)
	})

	Context("CRD groups: target is operator-managed, hash computation must be deferred", func() {
		DescribeTable("UT-RO-251-004..006: should identify CRD groups as async targets",
			func(group string) {
				Expect(creator.IsBuiltInGroup(group)).To(BeFalse(),
					"BR-RO-103.1: CRD group %q means resource is operator-managed, RO must set hashComputeAfter", group)
			},
			Entry("UT-RO-251-004: cert-manager.io (Certificate CRDs)", "cert-manager.io"),
			Entry("UT-RO-251-005: acid.zalan.do (Postgres operator CRDs)", "acid.zalan.do"),
			Entry("UT-RO-251-006: argoproj.io (ArgoCD CRDs)", "argoproj.io"),
			Entry("strimzi.io (Kafka operator CRDs)", "strimzi.io"),
			Entry("kubernaut.ai (our own CRDs)", "kubernaut.ai"),
		)
	})

	Context("Edge cases", func() {
		It("UT-RO-251-007: unknown group should be classified as CRD (safe default)", func() {
			Expect(creator.IsBuiltInGroup("unknown.example.com")).To(BeFalse(),
				"BR-RO-103.1: unknown groups default to async-managed — safe to defer hash rather than miss a change")
		})

		It("should not match partial built-in group names", func() {
			Expect(creator.IsBuiltInGroup("apps.k8s.io")).To(BeFalse(),
				"apps.k8s.io is not the same as apps — must be exact match")
			Expect(creator.IsBuiltInGroup("networking")).To(BeFalse(),
				"networking is not the same as networking.k8s.io — must be exact match")
		})
	})
})
