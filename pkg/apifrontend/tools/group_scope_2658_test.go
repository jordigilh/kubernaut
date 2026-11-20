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

package tools_test

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	"github.com/google/jsonschema-go/jsonschema"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// siblingCheckerStub is a CrossGroupSiblingChecker double for the #2658 group
// gate at the tool layer.
type siblingCheckerStub struct {
	siblings []string
}

func (s siblingCheckerStub) CheckCrossGroupSiblings(_ context.Context, _, _, _, _ string) ([]string, error) {
	return s.siblings, nil
}

// withNoCrossGroupSiblings supplies the #2658 group-scope checker to fixtures
// that model a unique instance: no same-named resource exists in another API
// group, so the gate adjudicates exactly as before #2658.
func withNoCrossGroupSiblings() severity.TriagerOption {
	return severity.WithCrossGroupSiblingChecker(siblingCheckerStub{})
}

// crossGroupTestTriager returns a Triager that positively correlates a firing
// alert to the given target (like defaultTestTriager) and reports the supplied
// cross-group sibling groups for it (#2658).
func crossGroupTestTriager(kind, name string, siblings ...string) *severity.Triager {
	mockProm := &alertOverridePromClient{
		alerts: []prom.Alert{
			{State: "firing", Labels: map[string]string{
				"alertname": "TestDefaultAlert", "namespace": "prod", "kind": kind, "name": name,
				"severity": "warning", "cluster": hubClusterID,
			}},
		},
	}
	return severity.NewTriager(mockProm, severity.NewNoopLLMTriager(logr.Discard()), severity.DefaultConfig(), logr.Discard(),
		severity.WithCrossGroupSiblingChecker(siblingCheckerStub{siblings: siblings}))
}

// noSiblingTestTriager is defaultTestTriager plus a checker that reports no
// cross-group sibling (the unique-instance case), so legacy fixtures exercise
// the same confident path as before #2658.
func noSiblingTestTriager(kind, name string, clusterID ...string) *severity.Triager {
	alertClusterID := hubClusterID
	if len(clusterID) > 0 {
		alertClusterID = clusterID[0]
	}
	mockProm := &alertOverridePromClient{
		alerts: []prom.Alert{
			{State: "firing", Labels: map[string]string{
				"alertname": "TestDefaultAlert", "namespace": "prod", "kind": kind, "name": name,
				"severity": "warning", "cluster": alertClusterID,
			}},
		},
	}
	return severity.NewTriager(mockProm, severity.NewNoopLLMTriager(logr.Discard()), severity.DefaultConfig(), logr.Discard(),
		severity.WithCrossGroupSiblingChecker(siblingCheckerStub{}))
}

func countRRs(tc crclient.Client) int {
	var list remediationv1.RemediationRequestList
	Expect(tc.List(context.Background(), &list)).To(Succeed())
	return len(list.Items)
}

var _ = Describe("Group-unscopable severity at the tool layer (#2658) [BR-SEVERITY-001 AC-8]", func() {

	Describe("UT-AF-2658-005: confirmed_api_group round trip through HandleCreateRR", func() {
		It("surfaces group_unscopable with candidate_api_groups and creates no RR", func() {
			tc := newTypedFakeClient()

			result, err := tools.HandleCreateRR(context.Background(),
				&tools.ToolDeps{Client: tc, ControllerNS: "prod", Triager: crossGroupTestTriager("Ingress", "web", "extensions"), ScopeChecker: testAlwaysManagedScopeChecker()},
				&tools.CreateRRArgs{ClusterID: hubClusterID, Namespace: "prod", Kind: "Ingress", Name: "web", Description: "sibling exists", APIVersion: "networking.k8s.io/v1"},
				"alice")
			Expect(err).NotTo(HaveOccurred(), "a group ambiguity is a typed result, not a Go error")
			Expect(result.Ambiguous).To(BeTrue())
			Expect(result.AmbiguityReason).To(Equal(severity.AmbiguityReasonGroupUnscopable))
			Expect(result.CandidateAPIGroups).To(Equal([]string{"extensions"}))
			Expect(result.RRID).To(BeEmpty())
			Expect(result.Message).To(ContainSubstring("API group"),
				"the message must explain the group ambiguity in user terms")
			Expect(result.Message).To(ContainSubstring("confirmed_api_group"),
				"the message must name the field that resolves the ambiguity")
			Expect(countRRs(tc)).To(BeZero())
		})

		It("bypasses the gate when confirmed_api_group names the target's own group", func() {
			tc := newTypedFakeClient()

			result, err := tools.HandleCreateRR(context.Background(),
				&tools.ToolDeps{Client: tc, ControllerNS: "prod", Triager: crossGroupTestTriager("Ingress", "web", "extensions"), ScopeChecker: testAlwaysManagedScopeChecker()},
				&tools.CreateRRArgs{ClusterID: hubClusterID, Namespace: "prod", Kind: "Ingress", Name: "web", Description: "confirmed",
					APIVersion: "networking.k8s.io/v1", ConfirmedAmbiguousAPIGroup: "networking.k8s.io/v1"},
				"alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Ambiguous).To(BeFalse())
			Expect(result.RRID).NotTo(BeEmpty())
			Expect(result.Severity).To(Equal("warning"))
		})

		It("returns an instructive non-error alert_belongs_to_sibling result", func() {
			tc := newTypedFakeClient()

			result, err := tools.HandleCreateRR(context.Background(),
				&tools.ToolDeps{Client: tc, ControllerNS: "prod", Triager: crossGroupTestTriager("Ingress", "web", "extensions"), ScopeChecker: testAlwaysManagedScopeChecker()},
				&tools.CreateRRArgs{ClusterID: hubClusterID, Namespace: "prod", Kind: "Ingress", Name: "web", Description: "belongs to sibling",
					APIVersion: "networking.k8s.io/v1", ConfirmedAmbiguousAPIGroup: "extensions"},
				"alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Ambiguous).To(BeTrue())
			Expect(result.AmbiguityReason).To(Equal(severity.AmbiguityReasonAlertBelongsToSibling))
			Expect(result.CandidateAPIGroups).To(Equal([]string{"extensions"}))
			Expect(result.Message).To(ContainSubstring("belongs to"),
				"the agent needs to know the alert refers to the sibling's resource, not this one")
			Expect(result.RRID).To(BeEmpty())
			Expect(countRRs(tc)).To(BeZero())
		})

		It("rejects a malformed confirmed_api_group as invalid input (SI-10)", func() {
			tc := newTypedFakeClient()

			_, err := tools.HandleCreateRR(context.Background(),
				&tools.ToolDeps{Client: tc, ControllerNS: "prod", Triager: crossGroupTestTriager("Ingress", "web", "extensions"), ScopeChecker: testAlwaysManagedScopeChecker()},
				&tools.CreateRRArgs{ClusterID: hubClusterID, Namespace: "prod", Kind: "Ingress", Name: "web", Description: "bad group",
					APIVersion: "networking.k8s.io/v1", ConfirmedAmbiguousAPIGroup: "bad_group!"},
				"alice")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, tools.ErrInvalidInput)).To(BeTrue())
		})

		It("rejects the reserved unresolved sentinel as a confirmed group (SI-10)", func() {
			tc := newTypedFakeClient()

			_, err := tools.HandleCreateRR(context.Background(),
				&tools.ToolDeps{Client: tc, ControllerNS: "prod", Triager: crossGroupTestTriager("Ingress", "web", "extensions"), ScopeChecker: testAlwaysManagedScopeChecker()},
				&tools.CreateRRArgs{ClusterID: hubClusterID, Namespace: "prod", Kind: "Ingress", Name: "web", Description: "sentinel",
					APIVersion: "networking.k8s.io/v1", ConfirmedAmbiguousAPIGroup: "unresolved"},
				"alice")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, tools.ErrInvalidInput)).To(BeTrue())
		})

		It("adjudicates a unique instance (no sibling) exactly as before", func() {
			tc := newTypedFakeClient()

			result, err := tools.HandleCreateRR(context.Background(),
				&tools.ToolDeps{Client: tc, ControllerNS: "prod", Triager: crossGroupTestTriager("Ingress", "web"), ScopeChecker: testAlwaysManagedScopeChecker()},
				&tools.CreateRRArgs{ClusterID: hubClusterID, Namespace: "prod", Kind: "Ingress", Name: "web", Description: "unique", APIVersion: "networking.k8s.io/v1"},
				"alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Ambiguous).To(BeFalse())
			Expect(result.RRID).NotTo(BeEmpty())
		})

		It("gates a core-group target and accepts the v1 alias as its confirmation", func() {
			tc := newTypedFakeClient()

			gated, err := tools.HandleCreateRR(context.Background(),
				&tools.ToolDeps{Client: tc, ControllerNS: "prod", Triager: crossGroupTestTriager("Event", "web-abc", "events.k8s.io"), ScopeChecker: testAlwaysManagedScopeChecker()},
				&tools.CreateRRArgs{ClusterID: hubClusterID, Namespace: "prod", Kind: "Event", Name: "web-abc", Description: "core target", APIVersion: "v1"},
				"alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(gated.Ambiguous).To(BeTrue(), "a core-group target with an events.k8s.io sibling must be gated")
			Expect(gated.AmbiguityReason).To(Equal(severity.AmbiguityReasonGroupUnscopable))

			confirmed, err := tools.HandleCreateRR(context.Background(),
				&tools.ToolDeps{Client: tc, ControllerNS: "prod", Triager: crossGroupTestTriager("Event", "web-abc", "events.k8s.io"), ScopeChecker: testAlwaysManagedScopeChecker()},
				&tools.CreateRRArgs{ClusterID: hubClusterID, Namespace: "prod", Kind: "Event", Name: "web-abc", Description: "core confirmed",
					APIVersion: "v1", ConfirmedAmbiguousAPIGroup: "core/v1"},
				"alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(confirmed.Ambiguous).To(BeFalse())
			Expect(confirmed.RRID).NotTo(BeEmpty())
		})

		It("returns alert_belongs_to_sibling when core is confirmed for a core sibling of a non-core target", func() {
			tc := newTypedFakeClient()

			result, err := tools.HandleCreateRR(context.Background(),
				&tools.ToolDeps{Client: tc, ControllerNS: "prod", Triager: crossGroupTestTriager("Event", "web-abc", ""), ScopeChecker: testAlwaysManagedScopeChecker()},
				&tools.CreateRRArgs{ClusterID: hubClusterID, Namespace: "prod", Kind: "Event", Name: "web-abc", Description: "core sibling",
					APIVersion: "events.k8s.io/v1", ConfirmedAmbiguousAPIGroup: "core"},
				"alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Ambiguous).To(BeTrue())
			Expect(result.AmbiguityReason).To(Equal(severity.AmbiguityReasonAlertBelongsToSibling))
			Expect(result.RRID).To(BeEmpty())
		})
	})

	Describe("IT-AF-2658-104: confirmed_api_group is LLM-visible on all three tools", func() {
		It("exposes confirmed_api_group in each tool's argument schema", func() {
			type schemaCase struct {
				name   string
				schema *jsonschema.Schema
			}
			cases := []schemaCase{}
			remediate, err := jsonschema.For[tools.RemediateArgs](nil)
			Expect(err).NotTo(HaveOccurred())
			cases = append(cases, schemaCase{"kubernaut_remediate", remediate})
			investigate, err := jsonschema.For[tools.InvestigateMCPArgs](nil)
			Expect(err).NotTo(HaveOccurred())
			cases = append(cases, schemaCase{"kubernaut_investigate", investigate})
			alert, err := jsonschema.For[tools.InvestigateAlertArgs](nil)
			Expect(err).NotTo(HaveOccurred())
			cases = append(cases, schemaCase{"kubernaut_investigate_alert", alert})

			for _, c := range cases {
				Expect(c.schema.Properties).To(HaveKey("confirmed_api_group"),
					"%s must let the agent confirm the API group", c.name)
			}
		})

		It("propagates the confirmed group and the ambiguity reason through kubernaut_remediate", func() {
			tc := newTypedFakeClient()
			deps := &tools.ToolDeps{Client: tc, ControllerNS: "prod", Triager: crossGroupTestTriager("Ingress", "web", "extensions"), ScopeChecker: testAlwaysManagedScopeChecker()}

			gated, err := tools.HandleRemediate(context.Background(), deps,
				&tools.RemediateArgs{ClusterID: hubClusterID, Namespace: "prod", Kind: "Ingress", Name: "web", APIVersion: "networking.k8s.io/v1"}, "alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(gated.Ambiguous).To(BeTrue())
			Expect(gated.AmbiguityReason).To(Equal(severity.AmbiguityReasonGroupUnscopable))
			Expect(gated.CandidateAPIGroups).To(Equal([]string{"extensions"}))

			confirmed, err := tools.HandleRemediate(context.Background(), deps,
				&tools.RemediateArgs{ClusterID: hubClusterID, Namespace: "prod", Kind: "Ingress", Name: "web", APIVersion: "networking.k8s.io/v1", ConfirmedAPIGroup: "networking.k8s.io"}, "alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(confirmed.Ambiguous).To(BeFalse())
			Expect(confirmed.RRID).NotTo(BeEmpty())
		})

		It("propagates the confirmed group through kubernaut_investigate", func() {
			tc := newTypedFakeClient()
			cfg := &tools.InvestigateConfig{
				Client: tc, Namespace: "prod", ScopeChecker: testAlwaysManagedScopeChecker(),
				Triager: crossGroupTestTriager("Ingress", "web", "extensions"),
			}

			_, gated, _, err := tools.CreateRRForInvestigation(context.Background(), cfg,
				&tools.InvestigateMCPArgs{Namespace: "prod", Kind: "Ingress", Name: "web", APIVersion: "networking.k8s.io/v1"}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(gated).NotTo(BeNil())
			Expect(gated.AmbiguityReason).To(Equal(severity.AmbiguityReasonGroupUnscopable))

			rrID, ambiguous, _, err := tools.CreateRRForInvestigation(context.Background(), cfg,
				&tools.InvestigateMCPArgs{Namespace: "prod", Kind: "Ingress", Name: "web", APIVersion: "networking.k8s.io/v1", ConfirmedAPIGroup: "networking.k8s.io"}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(ambiguous).To(BeNil())
			Expect(rrID).NotTo(BeEmpty())
		})

		It("propagates the confirmed group through kubernaut_investigate_alert", func() {
			tc := newTypedFakeClient()
			cfg := tools.InvestigateAlertConfig{
				Client: tc, ControllerNS: "prod", ScopeChecker: testAlwaysManagedScopeChecker(),
				Triager: crossGroupTestTriager("Ingress", "web", "extensions"),
			}

			gated, err := tools.HandleInvestigateAlert(context.Background(), cfg,
				&tools.InvestigateAlertArgs{AlertName: "TestDefaultAlert", APIVersion: "networking.k8s.io/v1", Kind: "Ingress", Name: "web", Namespace: "prod"}, "alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(gated.Ambiguous).To(BeTrue())
			Expect(gated.AmbiguityReason).To(Equal(severity.AmbiguityReasonGroupUnscopable))

			confirmed, err := tools.HandleInvestigateAlert(context.Background(), cfg,
				&tools.InvestigateAlertArgs{AlertName: "TestDefaultAlert", APIVersion: "networking.k8s.io/v1", Kind: "Ingress", Name: "web", Namespace: "prod", ConfirmedAPIGroup: "networking.k8s.io"}, "alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(confirmed.Ambiguous).To(BeFalse())
			Expect(confirmed.RRID).NotTo(BeEmpty())
		})
	})
})

// guard against an accidental field-order drift between CreateRRResult and
// RemediateResult, whose Go struct conversion (RemediateResult(result))
// requires identical field sequences.
var _ = Describe("CreateRRResult/RemediateResult conversion contract (#2658)", func() {
	It("keeps the trailing ambiguity fields in the same order", func() {
		src := tools.CreateRRResult{AmbiguityReason: "group_unscopable", CandidateAPIGroups: []string{"extensions"}}
		dst := tools.RemediateResult(src)
		Expect(dst.AmbiguityReason).To(Equal("group_unscopable"))
		Expect(dst.CandidateAPIGroups).To(Equal([]string{"extensions"}))
	})
})

// recordingSiblingChecker records the target group the production triage path
// derived from apiVersion (IT-AF-2658-101 wiring proof).
type recordingSiblingChecker struct {
	targetGroup string
	namespace   string
	kind        string
	name        string
	siblings    []string
}

func (r *recordingSiblingChecker) CheckCrossGroupSiblings(_ context.Context, namespace, kind, name, targetGroup string) ([]string, error) {
	r.targetGroup = targetGroup
	r.namespace = namespace
	r.kind = kind
	r.name = name
	return r.siblings, nil
}

var _ = Describe("IT-AF-2658-101: TriageInput.APIGroup is populated from the mandatory apiVersion", func() {
	DescribeTable("derives the canonical group from apiVersion at the single HandleCreateRR call site",
		func(apiVersion, wantGroup string) {
			tc := newTypedFakeClient()
			checker := &recordingSiblingChecker{}
			mockProm := &alertOverridePromClient{alerts: []prom.Alert{
				{State: "firing", Labels: map[string]string{
					"alertname": "TestDefaultAlert", "namespace": "prod", "kind": "Route", "name": "web",
					"severity": "warning", "cluster": hubClusterID,
				}},
			}}
			triager := severity.NewTriager(mockProm, severity.NewNoopLLMTriager(logr.Discard()), severity.DefaultConfig(), logr.Discard(),
				severity.WithCrossGroupSiblingChecker(checker))

			_, err := tools.HandleCreateRR(context.Background(),
				&tools.ToolDeps{Client: tc, ControllerNS: "prod", Triager: triager, ScopeChecker: testAlwaysManagedScopeChecker()},
				&tools.CreateRRArgs{ClusterID: hubClusterID, Namespace: "prod", Kind: "Route", Name: "web", Description: "wiring", APIVersion: apiVersion},
				"alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(checker.targetGroup).To(Equal(wantGroup))
			Expect(checker.namespace).To(Equal("prod"))
			Expect(checker.kind).To(Equal("Route"))
			Expect(checker.name).To(Equal("web"))
		},
		Entry("grouped apiVersion", "route.openshift.io/v1", "route.openshift.io"),
		Entry("core apiVersion yields the empty (core) group", "v1", ""),
	)

	It("rejects the core/v1 spelling at the tool gate instead of persisting it (SI-10, #2658)", func() {
		tc := newTypedFakeClient()
		_, err := tools.HandleCreateRR(context.Background(),
			&tools.ToolDeps{Client: tc, ControllerNS: "prod", Triager: noSiblingTestTriager("Pod", "api"), ScopeChecker: testAlwaysManagedScopeChecker()},
			&tools.CreateRRArgs{ClusterID: hubClusterID, Namespace: "prod", Kind: "Pod", Name: "api", Description: "core alias", APIVersion: "core/v1"},
			"alice")
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, tools.ErrInvalidInput)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring(`use "v1" for core resources`))
		Expect(countRRs(tc)).To(BeZero(), "a rejected apiVersion must never be persisted on an RR")
	})
})
