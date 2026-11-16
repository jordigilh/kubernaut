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

package scope_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

var _ = Describe("ResourceIdentity GVK validation [BR-SCOPE-001, SI-10]", func() {
	It("UT-SCOPE-2592-GVK-007 [AC-4, AC-6]: rejects Kind-only local identity", func() {
		_, err := scope.GVKForIdentity(scope.ResourceIdentity{Kind: "Deployment"})
		Expect(err).To(HaveOccurred())
	})
	It("UT-SCOPE-2592-GVK-001 parses a grouped apiVersion", func() {
		group, version, err := scope.ParseAPIVersion("apps/v1")

		Expect(err).NotTo(HaveOccurred())
		Expect(group).To(Equal("apps"))
		Expect(version).To(Equal("v1"))
	})

	It("UT-SCOPE-2592-GVK-002 parses a core apiVersion", func() {
		group, version, err := scope.ParseAPIVersion("v1")

		Expect(err).NotTo(HaveOccurred())
		Expect(group).To(BeEmpty())
		Expect(version).To(Equal("v1"))
	})

	DescribeTable("UT-SCOPE-2592-GVK-003 rejects incomplete apiVersions",
		func(apiVersion string) {
			_, _, err := scope.ParseAPIVersion(apiVersion)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, scope.ErrInvalidIdentity)).To(BeTrue(),
				"identity validation failures must be distinguishable from infrastructure errors (SI-10)")
		},
		Entry("empty", ""),
		Entry("group without version", "apps/"),
		Entry("empty group with separator", "/v1"),
		Entry("multiple separators", "apps/v1/extra"),
		Entry("group without separator is not a core version", "apps"),
		Entry("malformed version", "apps/v1x"),
	)

	It("UT-SCOPE-2592-GVK-008 rejects an empty apiVersion in ApplyAPIVersion", func() {
		_, err := scope.ApplyAPIVersion(scope.ResourceIdentity{Kind: "Deployment", Namespace: "default", Name: "nginx"}, "")
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, scope.ErrInvalidIdentity)).To(BeTrue())
	})

	It("UT-SCOPE-2592-GVK-004 rejects a missing remote GVK instead of inferring from Kind", func() {
		_, err := scope.GVKForIdentity(scope.ResourceIdentity{
			ClusterID: "prod-east",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "nginx",
		})

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("explicit group/version"))
	})

	It("UT-SCOPE-2592-GVK-005 accepts an explicit core remote GVK", func() {
		gvk, err := scope.GVKForIdentity(scope.ResourceIdentity{
			ClusterID: "prod-east",
			Version:   "v1",
			Kind:      "Pod",
			Namespace: "default",
			Name:      "nginx",
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(gvk.Group).To(BeEmpty())
		Expect(gvk.Version).To(Equal("v1"))
		Expect(gvk.Kind).To(Equal("Pod"))
	})

	It("UT-SCOPE-2592-GVK-006 applies apiVersion without allowing partial identity", func() {
		identity, err := scope.ApplyAPIVersion(scope.ResourceIdentity{
			ClusterID: "prod-east",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "nginx",
		}, "apps/v1")

		Expect(err).NotTo(HaveOccurred())
		Expect(identity.Group).To(Equal("apps"))
		Expect(identity.Version).To(Equal("v1"))
	})
})
