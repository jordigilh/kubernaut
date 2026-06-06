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
			result, err := triager.Triage(context.Background(), emptyNSInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).NotTo(Equal(severity.SourceNSFiringAlert), "should not match namespace-scoped when targetNamespace is empty")
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
					{Labels: map[string]string{"alertname": "HighCPU", "namespace": "prod", "kind": "Deployment", "name": "web-api", "severity": "low"}, State: "firing"},
					{Labels: map[string]string{"alertname": "IstioHighDenyRate", "namespace": "prod", "severity": "critical"}, State: "firing"},
					{Labels: map[string]string{"alertname": "etcdHighCommit", "severity": "critical"}, State: "firing"},
				},
			}
			triager := severity.NewTriager(mockProm, &mockLLM{}, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.AlertName).To(Equal("HighCPU"), "resource-specific match wins regardless of severity")
			Expect(result.Severity).To(Equal("low"))
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
			triager := severity.NewTriager(mockProm, &mockLLM{pureResult: severity.TriageResult{Severity: "medium", Source: severity.SourceLLMTriage}}, cfg, logr.Discard())
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

	Describe("Tier 3: Pure LLM Fallback", func() {
		It("UT-AF-T-031: no matching rules skips 2.5, falls through to Tier 3", func() {
			mockProm := &mockPromClient{
				alerts:     []prom.Alert{},
				ruleGroups: []prom.RuleGroup{},
			}
			mockLLM := &mockLLM{
				pureResult: severity.TriageResult{Severity: "medium", Source: severity.SourceLLMTriage},
			}
			triager := severity.NewTriager(mockProm, mockLLM, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).To(Equal(severity.SourceLLMTriage))
			Expect(mockLLM.pureCalled).To(BeTrue())
			Expect(mockLLM.rulesCalled).To(BeFalse())
		})

		It("UT-AF-T-032: pure LLM returns severity and source=llm_triage", func() {
			mockProm := &mockPromClient{
				alerts:     []prom.Alert{},
				ruleGroups: []prom.RuleGroup{},
			}
			mockLLM := &mockLLM{
				pureResult: severity.TriageResult{Severity: "low", Source: severity.SourceLLMTriage},
			}
			triager := severity.NewTriager(mockProm, mockLLM, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("low"))
			Expect(result.Source).To(Equal(severity.SourceLLMTriage))
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

		It("UT-AF-T-034: all tiers miss → Tier 3", func() {
			mockProm := &mockPromClient{
				alerts:     []prom.Alert{},
				ruleGroups: []prom.RuleGroup{},
			}
			mockLLM := &mockLLM{
				pureResult: severity.TriageResult{Severity: "medium", Source: severity.SourceLLMTriage},
			}
			triager := severity.NewTriager(mockProm, mockLLM, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).To(Equal(severity.SourceLLMTriage))
		})
	})

	Describe("Severity Ordering", func() {
		It("UT-AF-T-035: critical > high > medium > low > info", func() {
			Expect(severity.CompareSeverity("critical", "high")).To(BeNumerically(">", 0))
			Expect(severity.CompareSeverity("high", "medium")).To(BeNumerically(">", 0))
			Expect(severity.CompareSeverity("medium", "low")).To(BeNumerically(">", 0))
			Expect(severity.CompareSeverity("low", "info")).To(BeNumerically(">", 0))
			Expect(severity.CompareSeverity("critical", "info")).To(BeNumerically(">", 0))
			Expect(severity.CompareSeverity("medium", "medium")).To(BeNumerically("==", 0))
			Expect(severity.CompareSeverity("low", "critical")).To(BeNumerically("<", 0))

			Expect(severity.HighestSeverity([]string{"low", "critical", "medium"})).To(Equal("critical"))
			Expect(severity.HighestSeverity([]string{"info"})).To(Equal("info"))
			Expect(severity.HighestSeverity([]string{})).To(BeEmpty())
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

		It("UT-AF-T-038: Prometheus error at all tiers falls to Tier 3 LLM", func() {
			mockProm := &mockPromClient{
				alertsErr: errors.New("connection refused"),
				rulesErr:  errors.New("connection refused"),
			}
			mockLLM := &mockLLM{
				pureResult: severity.TriageResult{Severity: "medium", Source: severity.SourceLLMTriage},
			}
			triager := severity.NewTriager(mockProm, mockLLM, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).To(Equal(severity.SourceLLMTriage))
		})
	})

	Describe("Edge Cases", func() {
		It("UT-AF-T-043: empty resource labels skips Prometheus, goes to Tier 3", func() {
			mockProm := &mockPromClient{}
			mockLLM := &mockLLM{
				pureResult: severity.TriageResult{Severity: "medium", Source: severity.SourceLLMTriage},
			}
			input := severity.TriageInput{
				Namespace:   "prod",
				Kind:        "Deployment",
				Name:        "web",
				Description: "issue",
				Labels:      map[string]string{},
			}
			triager := severity.NewTriager(mockProm, mockLLM, defaultCfg, logr.Discard())
			result, err := triager.Triage(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).To(Equal(severity.SourceLLMTriage))
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
			triager := severity.NewTriager(mockProm, &mockLLM{pureResult: severity.TriageResult{Severity: "medium", Source: severity.SourceLLMTriage}}, cfg, logr.Discard())
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

		It("UT-AF-T-048: Tier 3 LLM error propagates instead of defaulting", func() {
			mockProm := &mockPromClient{
				alerts:     []prom.Alert{},
				ruleGroups: []prom.RuleGroup{},
			}
			llm := &mockLLM{
				pureErr: errors.New("LLM unavailable"),
			}
			triager := severity.NewTriager(mockProm, llm, defaultCfg, logr.Discard())
			_, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("tier 3 LLM triage failed"))
		})
	})

	Describe("Confidence Threshold", func() {
		It("UT-AF-T-051: LLM confidence below threshold downgrades to medium (Tier 3)", func() {
			mockProm := &mockPromClient{
				alerts:     []prom.Alert{},
				ruleGroups: []prom.RuleGroup{},
			}
			mockLLM := &mockLLM{
				pureResult: severity.TriageResult{Severity: "critical", Source: severity.SourceLLMTriage, Confidence: 0.4},
			}
			cfg := defaultCfg
			cfg.LLMConfidence = 0.7
			triager := severity.NewTriager(mockProm, mockLLM, cfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("medium"))
			Expect(result.Source).To(Equal(severity.SourceLLMTriage))
		})

		It("UT-AF-T-051b: LLM confidence above threshold keeps original severity", func() {
			mockProm := &mockPromClient{
				alerts:     []prom.Alert{},
				ruleGroups: []prom.RuleGroup{},
			}
			mockLLM := &mockLLM{
				pureResult: severity.TriageResult{Severity: "critical", Source: severity.SourceLLMTriage, Confidence: 0.9},
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
			Expect(result.Severity).To(Equal("medium"))
			Expect(result.Source).To(Equal(severity.SourceLLMRuleInform))
		})

		It("UT-AF-T-051d: zero confidence skips threshold check (backward compat)", func() {
			mockProm := &mockPromClient{
				alerts:     []prom.Alert{},
				ruleGroups: []prom.RuleGroup{},
			}
			mockLLM := &mockLLM{
				pureResult: severity.TriageResult{Severity: "high", Source: severity.SourceLLMTriage, Confidence: 0},
			}
			cfg := defaultCfg
			cfg.LLMConfidence = 0.7
			triager := severity.NewTriager(mockProm, mockLLM, cfg, logr.Discard())
			result, err := triager.Triage(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("high"))
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
	alerts      []prom.Alert
	alertsErr   error
	alertsHook  func()
	ruleGroups  []prom.RuleGroup
	rulesErr    error
	queryResult *prom.QueryResult
	queryErr    error
	queryHook   func()
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
	pureResult  severity.TriageResult
	pureErr     error
	pureCalled  bool
}

func (m *mockLLM) TriageWithRules(_ context.Context, _ []prom.Rule, _ severity.TriageInput) (severity.TriageResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rulesCalled = true
	return m.ruleResult, m.ruleErr
}

func (m *mockLLM) TriagePure(_ context.Context, _ severity.TriageInput) (severity.TriageResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pureCalled = true
	return m.pureResult, m.pureErr
}
