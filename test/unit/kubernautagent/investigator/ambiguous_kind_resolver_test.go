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

package investigator_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
)

var _ = Describe("TP-1044: IsAmbiguousKind resolver for multi-group kind detection", func() {

	newAmbiguousMapper := func() *meta.DefaultRESTMapper {
		mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
			{Group: "operators.coreos.com", Version: "v1alpha1"},
			{Group: "messaging.knative.dev", Version: "v1"},
		})
		mapper.Add(schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "Subscription"}, meta.RESTScopeNamespace)
		mapper.Add(schema.GroupVersionKind{Group: "messaging.knative.dev", Version: "v1", Kind: "Subscription"}, meta.RESTScopeNamespace)
		return mapper
	}

	Describe("UT-KA-1044-010: IsAmbiguousKind detects multi-group kind", func() {
		It("should return true and both GVRs for Subscription in two API groups (BR-AI-1044 AC1)", func() {
			resolver := investigator.NewMapperScopeResolver(newAmbiguousMapper())
			ambiguous, gvrs, err := resolver.IsAmbiguousKind("Subscription")
			Expect(err).NotTo(HaveOccurred())
			Expect(ambiguous).To(BeTrue(),
				"UT-KA-1044-010: Subscription exists in operators.coreos.com and messaging.knative.dev — must be ambiguous")
			Expect(gvrs).To(HaveLen(2),
				"UT-KA-1044-010: must return GVRs for both API groups")

			groups := make([]string, len(gvrs))
			for i, gvr := range gvrs {
				groups[i] = gvr.Group
			}
			Expect(groups).To(ContainElements("operators.coreos.com", "messaging.knative.dev"),
				"UT-KA-1044-010: GVRs must include both conflicting groups")
		})
	})

	Describe("UT-KA-1044-011: IsAmbiguousKind returns false for single-group kind", func() {
		It("should return false for Deployment registered only in apps/v1 (BR-AI-1044 AC4)", func() {
			mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
				{Group: "apps", Version: "v1"},
			})
			mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)

			resolver := investigator.NewMapperScopeResolver(mapper)
			ambiguous, gvrs, err := resolver.IsAmbiguousKind("Deployment")
			Expect(err).NotTo(HaveOccurred())
			Expect(ambiguous).To(BeFalse(),
				"UT-KA-1044-011: Deployment in a single group must not be ambiguous")
			Expect(gvrs).To(HaveLen(1),
				"UT-KA-1044-011: single-group kind should return 1 GVR")
		})
	})

	Describe("UT-KA-1044-012: IsAmbiguousKind returns false for unknown kind", func() {
		It("should return false with no error for a kind not in the mapper (BR-AI-1044)", func() {
			mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
				{Group: "apps", Version: "v1"},
			})
			mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)

			resolver := investigator.NewMapperScopeResolver(mapper)
			ambiguous, gvrs, err := resolver.IsAmbiguousKind("NonExistentKind")
			Expect(err).NotTo(HaveOccurred(),
				"UT-KA-1044-012: unknown kind should not return an error (NoResourceMatchError handled)")
			Expect(ambiguous).To(BeFalse(),
				"UT-KA-1044-012: unknown kind must not be ambiguous")
			Expect(gvrs).To(BeEmpty(),
				"UT-KA-1044-012: unknown kind should return no GVRs")
		})
	})

	Describe("UT-KA-1044-013: IsAmbiguousKind with empty string", func() {
		It("should return false without panic (BR-AI-1044 nil/zero)", func() {
			resolver := investigator.NewMapperScopeResolver(newAmbiguousMapper())
			ambiguous, gvrs, err := resolver.IsAmbiguousKind("")
			Expect(err).NotTo(HaveOccurred(),
				"UT-KA-1044-013: empty kind must not cause an error")
			Expect(ambiguous).To(BeFalse(),
				"UT-KA-1044-013: empty kind must not be ambiguous")
			Expect(gvrs).To(BeEmpty())
		})
	})

	Describe("UT-KA-1044-014: IsAmbiguousKind with path traversal input", func() {
		It("should return false without panic for adversarial input (BR-AI-1044 adversarial)", func() {
			resolver := investigator.NewMapperScopeResolver(newAmbiguousMapper())
			ambiguous, gvrs, err := resolver.IsAmbiguousKind("../../etc/passwd")
			Expect(err).NotTo(HaveOccurred(),
				"UT-KA-1044-014: path traversal input must not cause an error")
			Expect(ambiguous).To(BeFalse(),
				"UT-KA-1044-014: adversarial input must not be ambiguous")
			Expect(gvrs).To(BeEmpty())
		})
	})

	Describe("UT-KA-1044-015: IsAmbiguousKind with Unicode input", func() {
		It("should return false without panic for Unicode kind (BR-AI-1044 adversarial)", func() {
			resolver := investigator.NewMapperScopeResolver(newAmbiguousMapper())
			ambiguous, gvrs, err := resolver.IsAmbiguousKind("\u00dcn\u00efc\u00f6d\u00e9")
			Expect(err).NotTo(HaveOccurred(),
				"UT-KA-1044-015: Unicode kind must not cause an error")
			Expect(ambiguous).To(BeFalse(),
				"UT-KA-1044-015: Unicode kind must not be ambiguous")
			Expect(gvrs).To(BeEmpty())
		})
	})

	Describe("UT-KA-1044-016: IsAmbiguousKind with max-length+1 input", func() {
		It("should return false without panic for 256-char kind (BR-AI-1044 adversarial)", func() {
			resolver := investigator.NewMapperScopeResolver(newAmbiguousMapper())
			longKind := strings.Repeat("A", 256)
			ambiguous, gvrs, err := resolver.IsAmbiguousKind(longKind)
			Expect(err).NotTo(HaveOccurred(),
				"UT-KA-1044-016: max-length+1 kind must not cause an error")
			Expect(ambiguous).To(BeFalse(),
				"UT-KA-1044-016: max-length+1 kind must not be ambiguous")
			Expect(gvrs).To(BeEmpty())
		})
	})

	Describe("UT-KA-1044-017: IsAmbiguousKind for kind in 3+ API groups", func() {
		It("should return true with all GVRs for a kind in three groups (BR-AI-1044 edge case)", func() {
			mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
				{Group: "alpha.example.com", Version: "v1"},
				{Group: "beta.example.com", Version: "v1"},
				{Group: "gamma.example.com", Version: "v1"},
			})
			mapper.Add(schema.GroupVersionKind{Group: "alpha.example.com", Version: "v1", Kind: "Widget"}, meta.RESTScopeNamespace)
			mapper.Add(schema.GroupVersionKind{Group: "beta.example.com", Version: "v1", Kind: "Widget"}, meta.RESTScopeNamespace)
			mapper.Add(schema.GroupVersionKind{Group: "gamma.example.com", Version: "v1", Kind: "Widget"}, meta.RESTScopeNamespace)

			resolver := investigator.NewMapperScopeResolver(mapper)
			ambiguous, gvrs, err := resolver.IsAmbiguousKind("Widget")
			Expect(err).NotTo(HaveOccurred())
			Expect(ambiguous).To(BeTrue(),
				"UT-KA-1044-017: Widget in 3 groups must be ambiguous")
			Expect(len(gvrs)).To(BeNumerically(">=", 3),
				"UT-KA-1044-017: must return GVRs for all three groups")

			groups := make(map[string]bool)
			for _, gvr := range gvrs {
				groups[gvr.Group] = true
			}
			Expect(groups).To(HaveKey("alpha.example.com"))
			Expect(groups).To(HaveKey("beta.example.com"))
			Expect(groups).To(HaveKey("gamma.example.com"))
		})
	})
})
