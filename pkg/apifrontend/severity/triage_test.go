package severity_test

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
)

var _ = Describe("Triage Orchestrator", func() {

	var (
		defaultInput severity.TriageInput
		defaultCfg   severity.Config
	)

	BeforeEach(func() {
		defaultInput = severity.TriageInput{
			Namespace:   "prod",
			Kind:        "Deployment",
			Name:        "web-api",
			Description: "High error rate on web-api",
			Labels: map[string]string{
				"namespace": "prod",
				"kind":      "Deployment",
				"name":      "web-api",
			},
		}
		defaultCfg = severity.DefaultConfig()
	})

	Describe("Tier 1: Firing Alert Inheritance", func() {
		It("UT-AF-T-023: firing alert with severity=critical returns critical, source=firing_alert", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{Labels: map[string]string{"alertname": "HighCPU", "namespace": "prod", "kind": "Deployment", "name": "web-api", "severity": "critical"}, State: "firing"},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("critical"))
			Expect(result.Source).To(Equal(severity.SourceFiringAlert))
			Expect(result.AlertName).To(Equal("HighCPU"))
		})

		It("UT-AF-T-024: multiple firing alerts returns highest severity", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{Labels: map[string]string{"alertname": "LowDisk", "namespace": "prod", "kind": "Deployment", "name": "web-api", "severity": "low"}, State: "firing"},
					{Labels: map[string]string{"alertname": "HighCPU", "namespace": "prod", "kind": "Deployment", "name": "web-api", "severity": "critical"}, State: "firing"},
					{Labels: map[string]string{"alertname": "HighMem", "namespace": "prod", "kind": "Deployment", "name": "web-api", "severity": "high"}, State: "firing"},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("critical"))
		})

		It("UT-AF-T-025: no firing alerts falls through to Tier 1.5", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test",
						Rules: []prom.Rule{
							{Name: "PendingAlert", Query: `up{namespace="prod"}`, State: "pending", Labels: map[string]string{"severity": "high"}},
						},
					},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).To(Equal(severity.SourcePendingAlert))
		})
	})

	Describe("Tier 1: Namespace-scoped and cluster-scoped alert correlation (#1369)", func() {

		It("UT-AF-1369-001: resource-specific alert takes priority over namespace-scoped alert", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{Labels: map[string]string{"alertname": "IstioHighDenyRate", "namespace": "prod", "severity": "critical"}, State: "firing"},
					{Labels: map[string]string{"alertname": "HighCPU", "namespace": "prod", "kind": "Deployment", "name": "web-api", "severity": "warning"}, State: "firing"},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.AlertName).To(Equal("HighCPU"), "resource-specific match should win over namespace-scoped")
			Expect(result.Source).To(Equal(severity.SourceFiringAlert))
		})

		It("UT-AF-1369-002: namespace-scoped alert matches when no resource-specific match exists", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{Labels: map[string]string{"alertname": "IstioHighDenyRate", "namespace": "prod", "severity": "critical"}, State: "firing"},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("critical"))
			Expect(result.AlertName).To(Equal("IstioHighDenyRate"))
			Expect(result.Source).To(Equal(severity.SourceNSFiringAlert))
		})

		It("UT-AF-1369-003: cluster-scoped alert matches when no resource or namespace match exists", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{Labels: map[string]string{"alertname": "etcdHighCommitDurations", "severity": "warning"}, State: "firing"},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("warning"))
			Expect(result.AlertName).To(Equal("etcdHighCommitDurations"))
			Expect(result.Source).To(Equal(severity.SourceClusterFiringAlert))
		})

		It("UT-AF-1369-004: namespace-scoped alert returns ns_firing_alert source", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{Labels: map[string]string{"alertname": "IstioRequestsUnauthorized", "namespace": "prod", "severity": "high"}, State: "firing"},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).To(Equal(severity.SourceNSFiringAlert))
		})

		It("UT-AF-1369-005: cluster-scoped alert returns cluster_firing_alert source", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{Labels: map[string]string{"alertname": "KubeAPIErrorBudgetBurn", "severity": "critical"}, State: "firing"},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).To(Equal(severity.SourceClusterFiringAlert))
		})

		It("UT-AF-1369-006: multiple namespace-scoped alerts return highest severity", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{Labels: map[string]string{"alertname": "IstioLowRate", "namespace": "prod", "severity": "low"}, State: "firing"},
					{Labels: map[string]string{"alertname": "IstioHighDenyRate", "namespace": "prod", "severity": "critical"}, State: "firing"},
					{Labels: map[string]string{"alertname": "IstioMediumRate", "namespace": "prod", "severity": "medium"}, State: "firing"},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("critical"))
			Expect(result.AlertName).To(Equal("IstioHighDenyRate"))
		})

		It("UT-AF-1369-007: empty targetNamespace prevents namespace-scoped false matches", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{Labels: map[string]string{"alertname": "IstioHighDenyRate", "namespace": "prod", "severity": "critical"}, State: "firing"},
				},
			}
			emptyNSInput := severity.TriageInput{
				Namespace:   "",
				Kind:        "Deployment",
				Name:        "web-api",
				Description: "error",
				Labels:      map[string]string{"kind": "Deployment", "name": "web-api"},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			_, err := triager.Triage(context.Background(), emptyNSInput)
			Expect(err).To(MatchError(severity.ErrSeverityUndetermined),
				"should not match namespace-scoped when targetNamespace is empty, and #1839 fails closed rather than falling back to an ungrounded LLM guess")
		})

		It("UT-AF-1369-008: pending namespace-scoped alert returns ns_pending_alert source", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{Labels: map[string]string{"alertname": "IstioHighDenyRate", "namespace": "prod", "severity": "high"}, State: "pending"},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).To(Equal(severity.SourceNSPendingAlert))
		})

		It("UT-AF-1369-009: resource-specific match blocks lower-priority matches even if they have higher severity", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{Labels: map[string]string{"alertname": "HighCPU", "namespace": "prod", "kind": "Deployment", "name": "web-api", "severity": "info"}, State: "firing"},
					{Labels: map[string]string{"alertname": "IstioHighDenyRate", "namespace": "prod", "severity": "critical"}, State: "firing"},
					{Labels: map[string]string{"alertname": "etcdHighCommit", "severity": "critical"}, State: "firing"},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.AlertName).To(Equal("HighCPU"), "resource-specific match wins regardless of severity")
			Expect(result.Severity).To(Equal("info"))
			Expect(result.Source).To(Equal(severity.SourceFiringAlert))
		})
	})

	Describe("Tier 1.5: Pending Alert Check", func() {
		It("UT-AF-T-026: pending rule with severity=high returns high, source=pending_alert", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test",
						Rules: []prom.Rule{
							{Name: "HighMemPending", Query: `mem_usage{namespace="prod"} > 80`, State: "pending", Labels: map[string]string{"severity": "high"}},
						},
					},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("high"))
			Expect(result.Source).To(Equal(severity.SourcePendingAlert))
			Expect(result.RuleName).To(Equal("HighMemPending"))
		})

		It("UT-AF-T-027: no pending alerts falls through to Tier 2", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test",
						Rules: []prom.Rule{
							{Name: "InactiveRule", Query: `up{namespace="prod"}`, State: "inactive", Labels: map[string]string{"severity": "medium"}},
						},
					},
				},
				queryResult: &prom.QueryResult{
					Samples: []prom.Sample{{Value: 1, Metric: map[string]string{"namespace": "prod"}}},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).To(Equal(severity.SourceRuleEval))
		})
	})

	Describe("Tier 2: Rule Expression Evaluation", func() {
		It("UT-AF-T-028: matching rule expression with data returns severity, source=rule_evaluation", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test",
						Rules: []prom.Rule{
							{Name: "HighCPU", Query: `cpu_usage{namespace="prod"} > 0.9`, State: "inactive", Labels: map[string]string{"severity": "critical"}},
						},
					},
				},
				queryResult: &prom.QueryResult{
					Samples: []prom.Sample{{Value: 0.95, Metric: map[string]string{"namespace": "prod"}}},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("critical"))
			Expect(result.Source).To(Equal(severity.SourceRuleEval))
			Expect(result.RuleName).To(Equal("HighCPU"))
		})

		It("UT-AF-T-029: expression returns empty falls through to Tier 2.5", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test",
						Rules: []prom.Rule{
							{Name: "NoDataRule", Query: `rate(http_requests_total{namespace="prod"}[5m]) > 100`, State: "inactive", Labels: map[string]string{"severity": "high"}},
						},
					},
				},
				queryResult: &prom.QueryResult{Samples: []prom.Sample{}},
			}
			mockLLM := &mockLLM{
				ruleResult: severity.TriageResult{Severity: "high", Source: severity.SourceLLMRuleInform},
			}
			triager := severity.NewTriager(mockProm, mockLLM, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).To(Equal(severity.SourceLLMRuleInform))
		})

		It("UT-AF-T-036: max 10 instant queries per triage enforced", func() {
			rules := make([]prom.Rule, 15)
			for i := range rules {
				rules[i] = prom.Rule{
					Name:   "Rule" + string(rune('A'+i)),
					Query:  `up{namespace="prod"}`,
					State:  "inactive",
					Labels: map[string]string{"severity": "medium"},
				}
			}
			queryCount := 0
			mockProm := &mockPromClient{
				alerts:      []prom.Alert{},
				ruleGroups:  []prom.RuleGroup{{Name: "big", Rules: rules}},
				queryResult: &prom.QueryResult{Samples: []prom.Sample{}},
				queryHook: func() {
					queryCount++
				},
			}
			cfg := defaultCfg
			cfg.MaxQueriesPerCall = 10
			triager := severity.NewTriager(mockProm, &mockLLM{}, cfg, logr.Discard())
			_, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(queryCount).To(BeNumerically("<=", 10))
		})
	})

	Describe("Tier 2.5: LLM with Rule Context", func() {
		It("UT-AF-T-030: LLM receives rule context and returns severity", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test",
						Rules: []prom.Rule{
							{Name: "NoDataRule", Query: `rate(requests{namespace="prod"}[5m])`, State: "inactive", Labels: map[string]string{"severity": "high"}},
						},
					},
				},
				queryResult: &prom.QueryResult{Samples: []prom.Sample{}},
			}
			mockLLM := &mockLLM{
				ruleResult: severity.TriageResult{Severity: "high", Source: severity.SourceLLMRuleInform},
			}
			triager := severity.NewTriager(mockProm, mockLLM, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("high"))
			Expect(result.Source).To(Equal(severity.SourceLLMRuleInform))
			Expect(mockLLM.rulesCalled).To(BeTrue())
		})
	})

	Describe("No Grounded Signal — Fail Closed (#1839)", func() {
		It("UT-AF-1839-001: no matching rules skips 2.5, fails closed with ErrSeverityUndetermined (no LLM invoked)", func() {
			mockProm := &mockPromClient{
				alerts:     []prom.Alert{},
				ruleGroups: []prom.RuleGroup{},
			}
			mockLLM := &mockLLM{}
			triager := severity.NewTriager(mockProm, mockLLM, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).To(MatchError(severity.ErrSeverityUndetermined))
			Expect(result.Severity).To(BeEmpty())
			Expect(mockLLM.rulesCalled).To(BeFalse(),
				"#1839: removed Tier 3 must never invoke the LLM with zero grounding evidence")
		})
	})

	Describe("Full Pipeline Fallthrough", func() {
		It("UT-AF-T-033: T1 miss → T1.5 miss → T2 miss → T2.5 hit", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test",
						Rules: []prom.Rule{
							{Name: "InactiveNoData", Query: `rate(req{namespace="prod"}[5m])`, State: "inactive", Labels: map[string]string{"severity": "high"}},
						},
					},
				},
				queryResult: &prom.QueryResult{Samples: []prom.Sample{}},
			}
			mockLLM := &mockLLM{
				ruleResult: severity.TriageResult{Severity: "high", Source: severity.SourceLLMRuleInform},
			}
			triager := severity.NewTriager(mockProm, mockLLM, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).To(Equal(severity.SourceLLMRuleInform))
		})

		It("UT-AF-1839-002: all tiers miss → fails closed, no RemediationRequest severity is fabricated", func() {
			mockProm := &mockPromClient{
				alerts:     []prom.Alert{},
				ruleGroups: []prom.RuleGroup{},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).To(MatchError(severity.ErrSeverityUndetermined))
			Expect(result.Severity).To(BeEmpty())
		})
	})

	Describe("Severity Ordering", func() {
		It("UT-AF-T-035: critical > high > warning > info (ADR-066)", func() {
			Expect(severity.CompareSeverity("critical", "high")).To(BeNumerically(">", 0))
			Expect(severity.CompareSeverity("high", "warning")).To(BeNumerically(">", 0))
			Expect(severity.CompareSeverity("warning", "info")).To(BeNumerically(">", 0))
			Expect(severity.CompareSeverity("critical", "info")).To(BeNumerically(">", 0))
			Expect(severity.CompareSeverity("warning", "warning")).To(BeNumerically("==", 0))
			Expect(severity.CompareSeverity("info", "critical")).To(BeNumerically("<", 0))

			Expect(severity.HighestSeverity([]string{"info", "critical", "warning"})).To(Equal("critical"))
			Expect(severity.HighestSeverity([]string{"info"})).To(Equal("info"))
			Expect(severity.HighestSeverity([]string{})).To(BeEmpty())
		})

		It("UT-AF-1412-001: warning ranks correctly in severity model (ADR-066)", func() {
			Expect(severity.CompareSeverity("warning", "info")).To(BeNumerically(">", 0))
			Expect(severity.CompareSeverity("critical", "warning")).To(BeNumerically(">", 0))
			Expect(severity.CompareSeverity("high", "warning")).To(BeNumerically(">", 0))
			Expect(severity.HighestSeverity([]string{"warning", "info"})).To(Equal("warning"))
			Expect(severity.ValidateSeverity("warning")).To(BeTrue())
		})

		It("UT-AF-1417-001: NormalizeSeverity maps medium to warning (backward-compat)", func() {
			Expect(severity.NormalizeSeverity("medium")).To(Equal("warning"))
		})
	})

	Describe("Graceful Degradation", func() {
		It("UT-AF-T-037: Prometheus error at Tier 1 falls through to Tier 1.5", func() {
			mockProm := &mockPromClient{
				alertsErr: errors.New("connection refused"),
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test",
						Rules: []prom.Rule{
							{Name: "PendingRule", Query: `up{namespace="prod"}`, State: "pending", Labels: map[string]string{"severity": "medium"}},
						},
					},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).To(Equal(severity.SourcePendingAlert))
		})

		It("UT-AF-T-038: Prometheus error at all tiers fails closed (#1839, no ungrounded LLM fallback)", func() {
			mockProm := &mockPromClient{
				alertsErr: errors.New("connection refused"),
				rulesErr:  errors.New("connection refused"),
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			_, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).To(MatchError(severity.ErrSeverityUndetermined))
		})
	})

	Describe("Edge Cases", func() {
		It("UT-AF-T-043: empty resource labels skips Prometheus, fails closed (#1839)", func() {
			mockProm := &mockPromClient{}
			input := severity.TriageInput{
				Namespace:   "prod",
				Kind:        "Deployment",
				Name:        "web",
				Description: "issue",
				Labels:      map[string]string{},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			_, err := triager.Triage(context.Background(), input)
			Expect(err).To(MatchError(severity.ErrSeverityUndetermined))
		})

		It("UT-AF-T-044: > 100 rules bounded to MaxRulesEvaluated", func() {
			rules := make([]prom.Rule, 200)
			for i := range rules {
				rules[i] = prom.Rule{
					Name:   "Rule",
					Query:  `up{namespace="prod"}`,
					State:  "inactive",
					Labels: map[string]string{"severity": "medium"},
				}
			}
			queryCount := 0
			mockProm := &mockPromClient{
				alerts:      []prom.Alert{},
				ruleGroups:  []prom.RuleGroup{{Name: "huge", Rules: rules}},
				queryResult: &prom.QueryResult{Samples: []prom.Sample{}},
				queryHook:   func() { queryCount++ },
			}
			cfg := defaultCfg
			cfg.MaxRulesEvaluated = 100
			cfg.MaxQueriesPerCall = 100
			triager := severity.NewTriager(mockProm, &mockLLM{}, cfg, logr.Discard())
			_, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(queryCount).To(BeNumerically("<=", 100))
		})

		It("UT-AF-T-045: context cancellation propagates to all tiers", func() {
			mockProm := &mockPromClient{
				alertsHook: func() {
					time.Sleep(200 * time.Millisecond)
				},
			}
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()

			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			_, err := triager.Triage(ctx, defaultInput)
			Expect(err).To(HaveOccurred())
		})

		It("UT-AF-T-046: disabled triage returns empty result", func() {
			cfg := defaultCfg
			cfg.Enabled = false
			triager := severity.NewTriager(&mockPromClient{}, &mockLLM{}, cfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(BeEmpty())
			Expect(result.Source).To(BeEmpty())
		})

		It("UT-AF-T-047: NewTriager panics when LLM is nil", func() {
			Expect(func() {
				severity.NewTriager(&mockPromClient{}, nil, defaultCfg, logr.Discard())
			}).To(Panic())
		})

		It("UT-AF-T-048: Tier 2.5 LLM error falls through to fail-closed rather than defaulting (#1839)", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test",
						Rules: []prom.Rule{
							{Name: "NoDataRule", Query: `rate(requests{namespace="prod"}[5m])`, State: "inactive", Labels: map[string]string{"severity": "high"}},
						},
					},
				},
				queryResult: &prom.QueryResult{Samples: []prom.Sample{}},
			}
			llm := &mockLLM{
				ruleErr: errors.New("LLM unavailable"),
			}
			triager := severity.NewTriager(mockProm, llm, defaultCfg, logr.Discard())
			_, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).To(MatchError(severity.ErrSeverityUndetermined),
				"a Tier 2.5 LLM error is a tier miss (not fatal on its own), but with Tier 3 removed there is nothing left to fall back to")
		})
	})

	Describe("Confidence Threshold", func() {
		It("UT-AF-T-051: LLM confidence below threshold downgrades to warning (Tier 2.5)", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test",
						Rules: []prom.Rule{
							{Name: "InactiveRule", Query: `up{namespace="prod"}`, State: "inactive", Labels: map[string]string{"severity": "high"}},
						},
					},
				},
				queryResult: &prom.QueryResult{Samples: []prom.Sample{}},
			}
			mockLLM := &mockLLM{
				ruleResult: severity.TriageResult{Severity: "critical", Source: severity.SourceLLMRuleInform, Confidence: 0.4},
			}
			cfg := defaultCfg
			cfg.LLMConfidence = 0.7
			triager := severity.NewTriager(mockProm, mockLLM, cfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("warning"))
			Expect(result.Source).To(Equal(severity.SourceLLMRuleInform))
		})

		It("UT-AF-T-051b: LLM confidence above threshold keeps original severity (Tier 2.5)", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test",
						Rules: []prom.Rule{
							{Name: "InactiveRule", Query: `up{namespace="prod"}`, State: "inactive", Labels: map[string]string{"severity": "high"}},
						},
					},
				},
				queryResult: &prom.QueryResult{Samples: []prom.Sample{}},
			}
			mockLLM := &mockLLM{
				ruleResult: severity.TriageResult{Severity: "critical", Source: severity.SourceLLMRuleInform, Confidence: 0.9},
			}
			cfg := defaultCfg
			cfg.LLMConfidence = 0.7
			triager := severity.NewTriager(mockProm, mockLLM, cfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("critical"))
		})

		It("UT-AF-T-051c: LLM confidence below threshold downgrades to medium (Tier 2.5)", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test",
						Rules: []prom.Rule{
							{Name: "InactiveRule", Query: `up{namespace="prod"}`, State: "inactive", Labels: map[string]string{"severity": "high"}},
						},
					},
				},
				queryResult: &prom.QueryResult{Samples: []prom.Sample{}},
			}
			mockLLM := &mockLLM{
				ruleResult: severity.TriageResult{Severity: "high", Source: severity.SourceLLMRuleInform, Confidence: 0.3},
			}
			cfg := defaultCfg
			cfg.LLMConfidence = 0.7
			triager := severity.NewTriager(mockProm, mockLLM, cfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("warning"))
			Expect(result.Source).To(Equal(severity.SourceLLMRuleInform))
		})

		It("UT-AF-T-051d: zero confidence skips threshold check (backward compat, Tier 2.5)", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test",
						Rules: []prom.Rule{
							{Name: "InactiveRule", Query: `up{namespace="prod"}`, State: "inactive", Labels: map[string]string{"severity": "high"}},
						},
					},
				},
				queryResult: &prom.QueryResult{Samples: []prom.Sample{}},
			}
			mockLLM := &mockLLM{
				ruleResult: severity.TriageResult{Severity: "high", Source: severity.SourceLLMRuleInform, Confidence: 0},
			}
			cfg := defaultCfg
			cfg.LLMConfidence = 0.7
			triager := severity.NewTriager(mockProm, mockLLM, cfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("high"))
		})
	})

	Describe("Cluster-scoped pending alert correlation", func() {
		It("UT-AF-1369-010: cluster-scoped pending alert returns cluster_pending_alert source", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{Labels: map[string]string{"alertname": "ClusterWidePending", "severity": "warning"}, State: "pending"},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).To(Equal(severity.SourceClusterPendingAlert))
			Expect(result.AlertName).To(Equal("ClusterWidePending"))
		})
	})

	Describe("Resource-key overlap mismatch (labelsOverlap)", func() {
		It("UT-AF-1369-011: alert with an overlapping key but a different value is not a resource match", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					// Shares the "kind" key with defaultInput.Labels but the value
					// differs ("StatefulSet" vs "Deployment"), so labelsOverlap's
					// key-overlap path must reject this as a resource-level match.
					// It still correlates at the namespace level via "namespace".
					{Labels: map[string]string{"alertname": "WrongKind", "namespace": "prod", "kind": "StatefulSet", "name": "web-api", "severity": "critical"}, State: "firing"},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).To(Equal(severity.SourceNSFiringAlert), "mismatched-value overlap must not count as a resource-level match")
			Expect(result.AlertName).To(Equal("WrongKind"))
		})
	})

	Describe("Tier 1.5 rule parsing and matching edge cases", func() {
		It("UT-AF-T-026b: pending rule with unparseable PromQL is skipped, not fatal", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test",
						Rules: []prom.Rule{
							{Name: "Unparseable", Query: `up{{{`, State: "pending", Labels: map[string]string{"severity": "critical"}},
						},
					},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			_, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).To(MatchError(severity.ErrSeverityUndetermined), "unparseable rule must be skipped, not surfaced as a hard error")
		})

		It("UT-AF-T-026c: pending rule whose matchers don't match the resource is skipped", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test",
						Rules: []prom.Rule{
							{Name: "OtherNamespace", Query: `up{namespace="other"}`, State: "pending", Labels: map[string]string{"severity": "critical"}},
						},
					},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			_, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).To(MatchError(severity.ErrSeverityUndetermined), "a rule matching a different resource must not correlate")
		})
	})

	Describe("Tier 2 rule state and matching edge cases", func() {
		It("UT-AF-T-028b: a rule in an unexpected state (neither pending nor inactive) is skipped by Tier 2", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test",
						Rules: []prom.Rule{
							// "firing" rules are Alertmanager's concern, not this rule-state
							// classifier's -- Tier 2 only acts on "inactive" rules.
							{Name: "AlreadyFiring", Query: `up{namespace="prod"}`, State: "firing", Labels: map[string]string{"severity": "critical"}},
						},
					},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			_, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).To(MatchError(severity.ErrSeverityUndetermined))
		})

		It("UT-AF-T-028c: an inactive rule with unparseable PromQL is skipped, not fatal", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test",
						Rules: []prom.Rule{
							{Name: "Unparseable", Query: `up{{{`, State: "inactive", Labels: map[string]string{"severity": "critical"}},
						},
					},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			_, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).To(MatchError(severity.ErrSeverityUndetermined))
		})

		It("UT-AF-T-028d: an inactive rule whose matchers don't match the resource is skipped", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test",
						Rules: []prom.Rule{
							{Name: "OtherNamespace", Query: `up{namespace="other"}`, State: "inactive", Labels: map[string]string{"severity": "critical"}},
						},
					},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			_, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).To(MatchError(severity.ErrSeverityUndetermined))
		})

		It("UT-AF-T-028e: Tier 2 instant query failure is recorded as matched-but-not-found, falls through to Tier 2.5", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test",
						Rules: []prom.Rule{
							{Name: "QueryFails", Query: `up{namespace="prod"}`, State: "inactive", Labels: map[string]string{"severity": "critical"}},
						},
					},
				},
				queryErr: errors.New("prometheus query timeout"),
			}
			mockLLM := &mockLLM{ruleResult: severity.TriageResult{Severity: "high", Source: severity.SourceLLMRuleInform}}
			triager := severity.NewTriager(mockProm, mockLLM, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred(), "an instant-query failure must degrade to a tier miss, not a fatal pipeline error")
			Expect(result.Source).To(Equal(severity.SourceLLMRuleInform), "the failed rule is still recorded as matched, so Tier 2.5 runs with it as context")
			Expect(mockLLM.rulesCalled).To(BeTrue())
		})

		It("UT-AF-T-028f: Tier 2 stops evaluating further rules once MaxRulesEvaluated matched rules are recorded", func() {
			rules := []prom.Rule{
				{Name: "First", Query: `up{namespace="prod"}`, State: "inactive", Labels: map[string]string{"severity": "critical"}},
				{Name: "Second", Query: `up{namespace="prod"}`, State: "inactive", Labels: map[string]string{"severity": "critical"}},
			}
			queryCount := 0
			mockProm := &mockPromClient{
				alerts:      []prom.Alert{},
				ruleGroups:  []prom.RuleGroup{{Name: "test", Rules: rules}},
				queryResult: &prom.QueryResult{Samples: []prom.Sample{}},
				queryHook:   func() { queryCount++ },
			}
			cfg := defaultCfg
			cfg.MaxRulesEvaluated = 1
			triager := severity.NewTriager(mockProm, &mockLLM{}, cfg, logr.Discard())
			_, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(queryCount).To(Equal(1), "the second matching rule must never be queried once MaxRulesEvaluated=1 is reached")
		})
	})

	Describe("Rules cache reuse across calls", func() {
		It("UT-AF-T-039b: a second Triage call within the cache TTL does not re-fetch rules", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{},
				ruleGroups: []prom.RuleGroup{
					{
						Name: "test",
						Rules: []prom.Rule{
							{Name: "PendingRule", Query: `up{namespace="prod"}`, State: "pending", Labels: map[string]string{"severity": "high"}},
						},
					},
				},
			}
			cfg := defaultCfg
			cfg.CacheTTLSeconds = 30
			triager := severity.NewTriager(mockProm, &mockLLM{}, cfg, logr.Discard())

			_, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			_, err = triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())

			Expect(mockProm.rulesCallCount).To(Equal(1), "the second call must be served from the RulesCache, not re-fetched")
		})
	})

	Describe("Context cancellation at pipeline entry", func() {
		It("UT-AF-T-045b: a context canceled before the pipeline starts fails fast without querying Prometheus", func() {
			mockProm := &mockPromClient{}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			_, err := triager.Triage(ctx, defaultInput)
			Expect(err).To(MatchError(context.Canceled))
		})
	})

	Describe("Concurrency", func() {
		It("UT-AF-T-084: 10 goroutines calling Triage concurrently under -race", func() {
			mockProm := &mockPromClient{
				alerts: []prom.Alert{
					{Labels: map[string]string{"alertname": "Test", "namespace": "prod", "kind": "Deployment", "name": "web-api", "severity": "high"}, State: "firing"},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())

			var wg sync.WaitGroup
			errs := make(chan error, 10)
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					_, err := triager.Triage(context.Background(), defaultInput)
					if err != nil {
						errs <- err
					}
				}()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})

})

// --- Test mocks ---

type mockPromClient struct {
	alerts         []prom.Alert
	alertsErr      error
	alertsHook     func()
	ruleGroups     []prom.RuleGroup
	rulesErr       error
	rulesCallCount int
	queryResult    *prom.QueryResult
	queryErr       error
	queryHook      func()
}

func (m *mockPromClient) GetAlerts(ctx context.Context) ([]prom.Alert, error) {
	if m.alertsHook != nil {
		m.alertsHook()
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return m.alerts, m.alertsErr
}

func (m *mockPromClient) GetRules(_ context.Context) ([]prom.RuleGroup, error) {
	m.rulesCallCount++
	return m.ruleGroups, m.rulesErr
}

func (m *mockPromClient) InstantQuery(_ context.Context, _ string) (*prom.QueryResult, error) {
	if m.queryHook != nil {
		m.queryHook()
	}
	if m.queryResult != nil {
		return m.queryResult, m.queryErr
	}
	return &prom.QueryResult{}, m.queryErr
}

type mockLLM struct {
	mu          sync.Mutex
	ruleResult  severity.TriageResult
	ruleErr     error
	rulesCalled bool
}

func (m *mockLLM) TriageWithRules(_ context.Context, _ []prom.Rule, _ severity.TriageInput) (severity.TriageResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rulesCalled = true
	return m.ruleResult, m.ruleErr
}
