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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// UT-SCOPE-2658-001 [BR-SEVERITY-001, BR-ORCH-042, SI-10]: the group
// normalizer and core-alias canonicalization that every #2658 consumer
// (severity triage confirmation, DS targetApiVersion, fingerprints, WFE
// identity strings) shares. One canonical vocabulary; reserved aliases are a
// closed set so no spelling can smuggle a bogus group that matches no GVK
// and no persisted row.
var _ = Describe("Group normalization and core alias [BR-SEVERITY-001, BR-ORCH-042, SI-10]", func() {
	DescribeTable("UT-SCOPE-2658-001a normalizes the canonical vocabulary",
		func(in string, wantGroup string, wantState scope.NormalizeState) {
			group, state := scope.NormalizeAPIGroup(in)
			Expect(group).To(Equal(wantGroup))
			Expect(state).To(Equal(wantState))
		},
		Entry("core alias", "core", "", scope.NormalizeStateResolvedGroup),
		Entry("core version alias", "v1", "", scope.NormalizeStateResolvedGroup),
		Entry("core slash alias", "core/v1", "", scope.NormalizeStateResolvedGroup),
		Entry("grouped version", "apps/v1", "apps", scope.NormalizeStateResolvedGroup),
		Entry("bare group", "apps", "apps", scope.NormalizeStateResolvedGroup),
		Entry("unresolved sentinel", "unresolved", "", scope.NormalizeStateScopeUnknown),
		Entry("empty is invalid", "", "", scope.NormalizeStateInvalid),
		Entry("bare non-reserved core version is invalid", "v2", "", scope.NormalizeStateInvalid),
		Entry("bare prerelease version is invalid", "v1beta1", "", scope.NormalizeStateInvalid),
		Entry("bare alpha version is invalid", "v1alpha2", "", scope.NormalizeStateInvalid),
		Entry("core slash with non-v1 version is invalid", "core/v2", "", scope.NormalizeStateInvalid),
		Entry("core slash with prerelease version is invalid", "core/v1beta1", "", scope.NormalizeStateInvalid),
		Entry("whitespace-padded group", " apps ", "apps", scope.NormalizeStateResolvedGroup),
	)

	DescribeTable("UT-SCOPE-2658-001b ParseAPIVersion canonicalizes the core/v1 alias only",
		func(in string, wantGroup string, wantVersion string, wantErr bool) {
			group, version, err := scope.ParseAPIVersion(in)
			if wantErr {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(group).To(Equal(wantGroup))
			Expect(version).To(Equal(wantVersion))
		},
		Entry("core/v1 canonicalizes to the core group", "core/v1", "", "v1", false),
		Entry("core/v2 is rejected (closed alias set)", "core/v2", "", "", true),
		Entry("core/v1beta1 is rejected (closed alias set)", "core/v1beta1", "", "", true),
		Entry("grouped form is unchanged", "apps/v1", "apps", "v1", false),
		Entry("core form is unchanged", "v1", "", "v1", false),
		// R2-1: "unresolved" is a reserved sentinel, not a group or version —
		// accepting it here made BuildTargetIdentity emit hybrid identities
		// that ParseTargetIdentity rejects.
		Entry("unresolved as a group is rejected", "unresolved/v1", "", "", true),
		Entry("unresolved as a grouped version is rejected", "apps/unresolved", "", "", true),
		Entry("unresolved as a core version is rejected", "unresolved", "", "", true),
	)
})

// UT-RO-2658-005 (scope portion) [BR-ORCH-042, DD-WE-001]: the shared
// identity-string builders. Three forms, defined once: resolved(group),
// core, and unknown (unresolved in both slots) — an unknown target must
// never share a key with a genuine core resource, or the busy/cooldown/lock
// gates collide them (#2658 Finding A).
var _ = Describe("Target identity encoding [BR-ORCH-042, DD-WE-001, DD-RO-002]", func() {
	DescribeTable("UT-RO-2658-005a BuildTargetIdentity renders namespace/group/version/Kind/name",
		func(apiVersion, namespace, kind, name, want string) {
			identity := scope.BuildTargetIdentity(apiVersion, namespace, kind, name)
			Expect(identity).To(Equal(want))
			Expect(identity).NotTo(ContainSubstring("@"),
				"the identity must be a plain path: no annotation character")
			Expect(strings.Count(identity, "/")).To(BeElementOf(3, 4),
				"4 parts cluster-scoped, 5 parts namespaced")
		},
		Entry("namespaced grouped", "apps/v1", "prod", "Deployment", "web", "prod/apps/v1/Deployment/web"),
		Entry("namespaced core renders the core group", "v1", "prod", "Pod", "api", "prod/core/v1/Pod/api"),
		Entry("core/v1 alias canonicalizes", "core/v1", "prod", "Pod", "api", "prod/core/v1/Pod/api"),
		Entry("cluster-scoped grouped", "metrics.k8s.io/v1", "", "Node", "worker-1", "metrics.k8s.io/v1/Node/worker-1"),
		Entry("cluster-scoped core", "v1", "", "Node", "worker-1", "core/v1/Node/worker-1"),
		Entry("unknown apiVersion renders the sentinel in both slots", "", "prod", "Route", "web", "prod/unresolved/unresolved/Route/web"),
		Entry("unparseable apiVersion renders the sentinel", "not a version", "", "Node", "worker-1", "unresolved/unresolved/Node/worker-1"),
		// R2-1: the reserved word in either slot is not a group/version, so
		// the builder degrades to the unknown form instead of emitting a
		// hybrid identity the parser would reject.
		Entry("reserved sentinel as group degrades to unknown", "unresolved/v1", "prod", "Pod", "api", "prod/unresolved/unresolved/Pod/api"),
		Entry("reserved sentinel as version degrades to unknown", "apps/unresolved", "prod", "Deployment", "web", "prod/unresolved/unresolved/Deployment/web"),
	)

	DescribeTable("UT-RO-2658-005b ParseTargetIdentity round-trips the coordinates",
		func(apiVersion, namespace, kind, name string) {
			identity := scope.BuildTargetIdentity(apiVersion, namespace, kind, name)
			gotAPIVersion, gotNamespace, gotKind, gotName, err := scope.ParseTargetIdentity(identity)
			Expect(err).NotTo(HaveOccurred())
			Expect(gotNamespace).To(Equal(namespace))
			Expect(gotKind).To(Equal(kind))
			Expect(gotName).To(Equal(name))
			if apiVersion == "" || apiVersion == "not a version" {
				Expect(gotAPIVersion).To(BeEmpty())
			} else {
				Expect(scope.WireAPIVersion(gotAPIVersion)).To(Equal(scope.WireAPIVersion(apiVersion)))
			}
		},
		Entry("namespaced grouped", "apps/v1", "prod", "Deployment", "web"),
		Entry("namespaced core", "v1", "prod", "Pod", "api"),
		Entry("cluster-scoped grouped", "metrics.k8s.io/v1", "", "Node", "worker-1"),
		Entry("CRD version that is not v<N>", "example.com/stable", "prod", "Gateway", "edge"),
		Entry("hyphenated group", "cert-manager.io/v1", "prod", "Certificate", "le"),
		Entry("unknown", "", "prod", "Route", "web"),
	)

	It("UT-RO-2658-005c distinguishes unknown from core without a sentinel collision", func() {
		core := scope.BuildTargetIdentity("v1", "prod", "Pod", "api")
		unknown := scope.BuildTargetIdentity("", "prod", "Pod", "api")
		Expect(core).NotTo(Equal(unknown))
		Expect(scope.APIGroup(core)).To(Equal(""))
		Expect(scope.APIGroup(unknown)).To(Equal("unresolved"),
			"the unknown identity must report the reserved sentinel, not read as core")
		kind := func(identity string) string {
			_, _, k, _, err := scope.ParseTargetIdentity(identity)
			Expect(err).NotTo(HaveOccurred())
			return k
		}
		Expect(kind(core)).To(Equal("Pod"))
		Expect(kind(unknown)).To(Equal("Pod"))
		grouped := scope.BuildTargetIdentity("apps/v1", "prod", "Deployment", "web")
		Expect(scope.APIGroup(grouped)).To(Equal("apps"),
			"R2-2: a grouped identity must report its group, not the empty string")
	})

	DescribeTable("UT-RO-2658-005d WireAPIVersion is canonical and never empty",
		func(apiVersion, want string) {
			Expect(scope.WireAPIVersion(apiVersion)).To(Equal(want))
		},
		Entry("grouped", "apps/v1", "apps/v1"),
		Entry("core", "v1", "v1"),
		Entry("core/v1 alias", "core/v1", "v1"),
		Entry("absent", "", "unresolved"),
		Entry("unparseable", "garbage", "unresolved"),
	)

	It("UT-RO-2658-005e rejects malformed identities", func() {
		for _, bad := range []string{
			"prod/Ingress/web",
			"prod/apps/v1/Ingress",
			"prod/apps/v1//web",
			"",
			// Half-known hybrids: no builder produces them, and accepting them
			// would present an unresolved coordinate as resolvable.
			"prod/unresolved/v1/Pod/api",
			"prod/apps/unresolved/Deployment/web",
			// Leading-empty namespace slot: malformed, not cluster-scoped.
			"/apps/v1/Deployment/web",
		} {
			_, _, _, _, err := scope.ParseTargetIdentity(bad)
			Expect(err).To(HaveOccurred(), "identity %q must be rejected", bad)
		}
	})
})
