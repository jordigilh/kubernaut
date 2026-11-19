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

// This spec lives in package scope (not scope_test) because it pins the
// unexported classification inventory. Ginkgo registers it in the same suite
// binary as the scope_test specs, which own RunSpecs.
package scope

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Issue #2670 §2 D4: the recognized cluster-scoped inventory is a security
// surface (it decides which identities resolve at level 1). It must match the
// list published in ADR-053 §"Recognized kinds" exactly, so that adding or
// removing a kind is a deliberate change in code, test and documentation.
var _ = Describe("Recognized cluster-scoped kinds (BR-SCOPE-001 FR-SCOPE-004)", func() {

	It("UT-SCOPE-2670-012 [BR-SCOPE-001]: the classification inventory matches the documented set", func() {
		documented := map[string]bool{
			// Upstream Kubernetes, shipped by default (19).
			"Node":                           true,
			"PersistentVolume":               true,
			"Namespace":                      true,
			"ClusterRole":                    true,
			"ClusterRoleBinding":             true,
			"StorageClass":                   true,
			"CustomResourceDefinition":       true,
			"IngressClass":                   true,
			"PriorityClass":                  true,
			"ValidatingWebhookConfiguration": true,
			"MutatingWebhookConfiguration":   true,
			"CSIDriver":                      true,
			"CSINode":                        true,
			"VolumeAttachment":               true,
			"RuntimeClass":                   true,
			"APIService":                     true,
			"CertificateSigningRequest":      true,
			"FlowSchema":                     true,
			"PriorityLevelConfiguration":     true,
			// OpenShift-only (4).
			"ClusterOperator": true,
			"ClusterVersion":  true,
			"Infrastructure":  true,
			"ClusterNetwork":  true,
		}

		Expect(clusterScopedKinds).To(Equal(documented),
			"clusterScopedKinds and the ADR-053 inventory must change together (issue #2670)")
		Expect(clusterScopedKinds).To(HaveLen(23))
	})

	It("UT-SCOPE-2670-014 [BR-SCOPE-001, SI-10]: namespaced kinds are never classified as cluster-scoped", func() {
		for _, kind := range []string{"Pod", "Deployment", "Service", "ConfigMap", "Secret", "Job", "CronJob", "PersistentVolumeClaim"} {
			Expect(IsClusterScopedKind(kind)).To(BeFalse(), "%s is namespaced and must keep the named 2-level hierarchy", kind)
		}
		Expect(IsKnownNamespacedKind("PersistentVolumeClaim")).To(BeTrue(),
			"PVC must be a known namespaced kind so a namespace-less identity is terminal")
	})
})
