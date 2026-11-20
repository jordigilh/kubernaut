package severity_test

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
)

// stubSiblingChecker is a configurable CrossGroupSiblingChecker for the
// #2658 group-scope gate tests.
type stubSiblingChecker struct {
	siblings []string
	err      error
	calls    int
}

func (s *stubSiblingChecker) CheckCrossGroupSiblings(_ context.Context, _, _, _, _ string) ([]string, error) {
	s.calls++
	return s.siblings, s.err
}

var _ = Describe("Group-scope gate (#2658) [BR-SEVERITY-001 AC-8, SI-10]", func() {
	var (
		input severity.TriageInput
		cfg   severity.Config
	)

	BeforeEach(func() {
		input = severity.TriageInput{
			Namespace: "prod",
			Kind:      "Route",
			Name:      "web",
			APIGroup:  "route.openshift.io",
			ClusterID: hubClusterID,
			Labels: map[string]string{
				"namespace": "prod",
				"kind":      "Route",
				"name":      "web",
			},
		}
		cfg = severity.DefaultConfig()
	})

	firingMatchProm := func() *mockPromClient {
		return &mockPromClient{
			alerts: []prom.Alert{
				{Labels: map[string]string{"alertname": "RouteDegraded", "namespace": "prod", "kind": "Route", "name": "web", "severity": "critical"}, State: "firing"},
			},
		}
	}

	Describe("UT-AF-2658-001: cross-group sibling exists → fail closed", func() {
		It("returns an ambiguous group_unscopable result with the sibling groups", func() {
			triager := severity.NewTriager(firingMatchProm(), &mockLLM{}, cfg, logr.Discard(),
				severity.WithCrossGroupSiblingChecker(&stubSiblingChecker{siblings: []string{"serving.knative.dev"}}))
			result, err := triager.Triage(context.Background(), input)
			var amb *severity.AmbiguousSeverityError
			Expect(errors.As(err, &amb)).To(BeTrue(), "a sibling-group ambiguity must fail closed, not inherit severity")
			Expect(amb.Candidate.AmbiguityReason).To(Equal(severity.AmbiguityReasonGroupUnscopable))
			Expect(amb.Candidate.CandidateAPIGroups).To(Equal([]string{"serving.knative.dev"}))
			Expect(result.Ambiguous).To(BeTrue())
		})
	})

	Describe("UT-AF-2658-002: no sibling (unique instance) → adjudicate", func() {
		It("accepts the correlated severity as today", func() {
			triager := severity.NewTriager(firingMatchProm(), &mockLLM{}, cfg, logr.Discard(),
				severity.WithCrossGroupSiblingChecker(&stubSiblingChecker{}))
			result, err := triager.Triage(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("critical"))
			Expect(result.Source).To(Equal(severity.SourceFiringAlert))
		})

		It("adjudicates when the sibling lives in another namespace (name mismatch across groups)", func() {
			// The stub models the checker's contract: it only reports
			// siblings in the SAME namespace/clusterspace. An empty result
			// means no same-ns/kind/name sibling exists in any other group.
			triager := severity.NewTriager(firingMatchProm(), &mockLLM{}, cfg, logr.Discard(),
				severity.WithCrossGroupSiblingChecker(&stubSiblingChecker{siblings: nil}))
			result, err := triager.Triage(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("critical"))
		})
	})

	Describe("UT-AF-2658-003: single-group Kind fast path / legacy callers without APIGroup", func() {
		It("does not engage the gate when the target carries no APIGroup (legacy input)", func() {
			noGroup := input
			noGroup.APIGroup = ""
			triager := severity.NewTriager(firingMatchProm(), &mockLLM{}, cfg, logr.Discard())
			result, err := triager.Triage(context.Background(), noGroup)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("critical"))
		})
	})

	Describe("UT-AF-2658-004: rule-tier candidates carry a non-empty identifier", func() {
		It("surfaces the rule name as CandidateIdentifier with AlertName empty", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{Name: "g", Rules: []prom.Rule{
						{Name: "RoutePendingRule", State: "pending", Query: `route_status{kind="Route",name="web"}`, Labels: map[string]string{"severity": "high"}},
					}},
				},
			}
			// Make the rule match: the target labels must satisfy every
			// matcher in the query. kind/name matchers overlap the input
			// labels, so the rule correlates via Tier 1.5.
			input.Labels["route_status"] = ""
			triager := severity.NewTriager(mockProm, &mockLLM{}, cfg, logr.Discard(),
				severity.WithCrossGroupSiblingChecker(&stubSiblingChecker{siblings: []string{"serving.knative.dev"}}))
			_, err := triager.Triage(context.Background(), input)
			var amb *severity.AmbiguousSeverityError
			Expect(errors.As(err, &amb)).To(BeTrue())
			Expect(amb.Candidate.AmbiguityReason).To(Equal(severity.AmbiguityReasonGroupUnscopable))
			Expect(amb.Candidate.CandidateIdentifier).NotTo(BeEmpty(), "rule-tier ambiguity must carry a confirmable candidate identifier")
			Expect(amb.Candidate.AlertName).To(BeEmpty(), "an alert name must never bypass the group gate (R3-3)")
		})
	})

	Describe("UT-AF-2658-005: confirmed_api_group resolution (2×2 reason × confirmation matrix)", func() {
		It("bypasses the gate when the confirmed group equals the target's own group", func() {
			confirmed := input
			confirmed.ConfirmedAPIGroup = "route.openshift.io"
			triager := severity.NewTriager(firingMatchProm(), &mockLLM{}, cfg, logr.Discard(),
				severity.WithCrossGroupSiblingChecker(&stubSiblingChecker{siblings: []string{"serving.knative.dev"}}))
			result, err := triager.Triage(context.Background(), confirmed)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("critical"))
		})

		It("normalizes group/version and alias spellings before comparing", func() {
			confirmed := input
			confirmed.ConfirmedAPIGroup = "route.openshift.io/v1"
			triager := severity.NewTriager(firingMatchProm(), &mockLLM{}, cfg, logr.Discard(),
				severity.WithCrossGroupSiblingChecker(&stubSiblingChecker{siblings: []string{"serving.knative.dev"}}))
			result, err := triager.Triage(context.Background(), confirmed)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("critical"))
		})

		It("returns alert_belongs_to_sibling when the confirmed group is a sibling's", func() {
			confirmed := input
			confirmed.ConfirmedAPIGroup = "serving.knative.dev"
			triager := severity.NewTriager(firingMatchProm(), &mockLLM{}, cfg, logr.Discard(),
				severity.WithCrossGroupSiblingChecker(&stubSiblingChecker{siblings: []string{"serving.knative.dev"}}))
			_, err := triager.Triage(context.Background(), confirmed)
			var amb *severity.AmbiguousSeverityError
			Expect(errors.As(err, &amb)).To(BeTrue())
			Expect(amb.Candidate.AmbiguityReason).To(Equal(severity.AmbiguityReasonAlertBelongsToSibling))
		})

		It("a signal-name confirmation never bypasses the group gate", func() {
			confirmed := input
			confirmed.ConfirmedSignalName = "RouteDegraded"
			triager := severity.NewTriager(firingMatchProm(), &mockLLM{}, cfg, logr.Discard(),
				severity.WithCrossGroupSiblingChecker(&stubSiblingChecker{siblings: []string{"serving.knative.dev"}}))
			_, err := triager.Triage(context.Background(), confirmed)
			var amb *severity.AmbiguousSeverityError
			Expect(errors.As(err, &amb)).To(BeTrue())
			Expect(amb.Candidate.AmbiguityReason).To(Equal(severity.AmbiguityReasonGroupUnscopable))
		})

		It("a group confirmation never bypasses the cluster-scoped gate", func() {
			clusterOnly := input
			clusterOnly.ConfirmedAPIGroup = "route.openshift.io"
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{Labels: map[string]string{"alertname": "ClusterWideIssue", "severity": "high"}, State: "firing"},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, cfg, logr.Discard(),
				severity.WithCrossGroupSiblingChecker(&stubSiblingChecker{}))
			_, err := triager.Triage(context.Background(), clusterOnly)
			var amb *severity.AmbiguousSeverityError
			Expect(errors.As(err, &amb)).To(BeTrue())
			Expect(amb.Candidate.AmbiguityReason).To(Equal(severity.AmbiguityReasonClusterScopedOnly))
		})
	})

	Describe("UT-AF-2658-006: DD-AF-012 contract intact for cluster_scoped_only", func() {
		It("keeps the confirmed_signal_name round trip for cluster-scoped-only matches", func() {
			clusterOnly := input
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{Labels: map[string]string{"alertname": "ClusterWideIssue", "severity": "high"}, State: "firing"},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, cfg, logr.Discard())
			_, err := triager.Triage(context.Background(), clusterOnly)
			var amb *severity.AmbiguousSeverityError
			Expect(errors.As(err, &amb)).To(BeTrue())
			Expect(amb.Candidate.AmbiguityReason).To(Equal(severity.AmbiguityReasonClusterScopedOnly))

			confirmed := clusterOnly
			confirmed.ConfirmedSignalName = "ClusterWideIssue"
			result, err := triager.Triage(context.Background(), confirmed)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("high"))
		})
	})

	Describe("UT-AF-2658-007: checker error and fleet mode", func() {
		It("fails closed when the checker errors (hub-local)", func() {
			triager := severity.NewTriager(firingMatchProm(), &mockLLM{}, cfg, logr.Discard(),
				severity.WithCrossGroupSiblingChecker(&stubSiblingChecker{err: errors.New("discovery unavailable")}))
			_, err := triager.Triage(context.Background(), input)
			var amb *severity.AmbiguousSeverityError
			Expect(errors.As(err, &amb)).To(BeTrue())
			Expect(amb.Candidate.AmbiguityReason).To(Equal(severity.AmbiguityReasonGroupUnscopable))
			Expect(amb.Candidate.CandidateAPIGroups).To(BeEmpty())
		})

		It("fails closed when no checker is wired but the target carries a group", func() {
			triager := severity.NewTriager(firingMatchProm(), &mockLLM{}, cfg, logr.Discard())
			_, err := triager.Triage(context.Background(), input)
			var amb *severity.AmbiguousSeverityError
			Expect(errors.As(err, &amb)).To(BeTrue())
			Expect(amb.Candidate.AmbiguityReason).To(Equal(severity.AmbiguityReasonGroupUnscopable))
		})

		It("adjudicates in fleet mode (no per-cluster reader; documented deviation)", func() {
			fleet := input
			fleet.FleetMode = true
			checker := &stubSiblingChecker{siblings: []string{"serving.knative.dev"}}
			triager := severity.NewTriager(firingMatchProm(), &mockLLM{}, cfg, logr.Discard(),
				severity.WithCrossGroupSiblingChecker(checker))
			result, err := triager.Triage(context.Background(), fleet)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("critical"))
			Expect(checker.calls).To(Equal(0), "fleet mode must not probe the hub-local discovery for a remote cluster")
		})
	})
})

// UT-AF-2658-005 (core group): the core API group is a legitimate sibling
// group for a Kind registered in more than one group (e.g. Event in core and
// events.k8s.io), so the gate must engage for a target whose group is the
// core group -- and the core spellings ("v1", "core") must be usable as
// confirmations. BR-SEVERITY-001 AC-8, SI-10.
var _ = Describe("Group-scope gate: core-group sibling (#2658) [BR-SEVERITY-001 AC-8]", func() {
	var (
		cfg      severity.Config
		coreEvt  severity.TriageInput
		promCore *mockPromClient
	)

	BeforeEach(func() {
		cfg = severity.DefaultConfig()
		// Core-group target: Event in the core ("") API group.
		coreEvt = severity.TriageInput{
			Namespace:     "prod",
			Kind:          "Event",
			Name:          "web-abc",
			APIGroup:      "",
			APIGroupKnown: true,
			ClusterID:     hubClusterID,
			Labels: map[string]string{
				"namespace": "prod",
				"kind":      "Event",
				"name":      "web-abc",
			},
		}
		promCore = &mockPromClient{
			alerts: []prom.Alert{
				{Labels: map[string]string{"alertname": "EventStorm", "namespace": "prod", "kind": "Event", "name": "web-abc", "severity": "high"}, State: "firing"},
			},
		}
	})

	It("bypasses the gate when confirmed_api_group is the core spelling v1", func() {
		confirmed := coreEvt
		confirmed.ConfirmedAPIGroup = "v1"
		triager := severity.NewTriager(promCore, &mockLLM{}, cfg, logr.Discard(),
			severity.WithCrossGroupSiblingChecker(&stubSiblingChecker{siblings: []string{"events.k8s.io"}}))
		result, err := triager.Triage(context.Background(), confirmed)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Severity).To(Equal("high"))
	})

	It("fails closed for a core-group target when the core sibling exists and the user confirms the sibling's group", func() {
		eventsTarget := coreEvt
		eventsTarget.APIGroup = "events.k8s.io"
		eventsTarget.ConfirmedAPIGroup = "core"
		triager := severity.NewTriager(promCore, &mockLLM{}, cfg, logr.Discard(),
			severity.WithCrossGroupSiblingChecker(&stubSiblingChecker{siblings: []string{""}}))
		_, err := triager.Triage(context.Background(), eventsTarget)
		var amb *severity.AmbiguousSeverityError
		Expect(errors.As(err, &amb)).To(BeTrue())
		Expect(amb.Candidate.AmbiguityReason).To(Equal(severity.AmbiguityReasonAlertBelongsToSibling))
		Expect(amb.Candidate.CandidateAPIGroups).To(Equal([]string{""}))
	})

	It("does not engage the gate for a legacy input that carries no group knowledge", func() {
		legacy := coreEvt
		legacy.APIGroupKnown = false
		legacy.APIGroup = ""
		triager := severity.NewTriager(promCore, &mockLLM{}, cfg, logr.Discard())
		result, err := triager.Triage(context.Background(), legacy)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Severity).To(Equal("high"))
	})
})

// UT-AF-2658-005 / AU-3: the typed ambiguity error's text must describe the
// actual reason -- it is wrapped into the tool error and logged, so a
// hardcoded cluster-scoped story would be factually wrong for a group
// ambiguity.
var _ = Describe("AmbiguousSeverityError text derives from the ambiguity reason (#2658)", func() {
	It("names the group ambiguity and the candidate groups for group_unscopable", func() {
		err := &severity.AmbiguousSeverityError{Candidate: severity.TriageResult{
			AmbiguityReason:     severity.AmbiguityReasonGroupUnscopable,
			CandidateAPIGroups:  []string{"extensions", "networking.k8s.io"},
			CandidateIdentifier: "IngressHighLatency",
		}}
		text := err.Error()
		Expect(text).To(ContainSubstring("API group"))
		Expect(text).To(ContainSubstring("extensions"))
		Expect(text).NotTo(ContainSubstring("cluster-scoped"),
			"a group ambiguity must not be described with the cluster-scoped story (AU-3)")
	})

	It("states that the alert belongs to a sibling group's resource", func() {
		err := &severity.AmbiguousSeverityError{Candidate: severity.TriageResult{
			AmbiguityReason:    severity.AmbiguityReasonAlertBelongsToSibling,
			CandidateAPIGroups: []string{"extensions"},
		}}
		Expect(err.Error()).To(ContainSubstring("sibling"))
		Expect(err.Error()).NotTo(ContainSubstring("cluster-scoped"))
	})

	It("keeps the DD-AF-012 cluster-scoped wording for cluster_scoped_only", func() {
		err := &severity.AmbiguousSeverityError{Candidate: severity.TriageResult{
			AmbiguityReason: severity.AmbiguityReasonClusterScopedOnly,
			AlertName:       "ClusterWideIssue",
		}}
		Expect(err.Error()).To(ContainSubstring("cluster-scoped alert"))
		Expect(err.Error()).To(ContainSubstring("ClusterWideIssue"))
	})
})
