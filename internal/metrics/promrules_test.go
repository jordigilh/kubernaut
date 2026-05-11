package metrics_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	promparser "github.com/prometheus/prometheus/promql/parser"
	"gopkg.in/yaml.v3"
)

// prometheusRuleFile mirrors the relevant portion of the PrometheusRule CRD spec.
type prometheusRuleFile struct {
	Spec struct {
		Groups []ruleGroup `yaml:"groups"`
	} `yaml:"spec"`
}

type ruleGroup struct {
	Name  string `yaml:"name"`
	Rules []rule `yaml:"rules"`
}

type rule struct {
	Alert       string            `yaml:"alert"`
	Expr        string            `yaml:"expr"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
}

func loadPrometheusRules() prometheusRuleFile {
	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(root, "deploy", "kustomize", "base", "05-prometheusrule.yaml")

	data, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred(), "failed to read prometheus-rules.yaml")

	var prf prometheusRuleFile
	Expect(yaml.Unmarshal(data, &prf)).To(Succeed())
	return prf
}

var _ = Describe("SLO Conformance — Prometheus Rules", func() {
	var (
		prf       prometheusRuleFile
		pqlParser promparser.Parser
	)

	BeforeEach(func() {
		prf = loadPrometheusRules()
		pqlParser = promparser.NewParser(promparser.Options{})
	})

	It("UT-AF-SLO-001: all PromQL expressions are syntactically valid", func() {
		for _, group := range prf.Spec.Groups {
			for _, r := range group.Rules {
				if r.Expr == "" {
					continue
				}
				expr := strings.TrimSpace(r.Expr)
				_, err := pqlParser.ParseExpr(expr)
				Expect(err).NotTo(HaveOccurred(),
					"invalid PromQL in alert %s (group %s): %s", r.Alert, group.Name, expr)
			}
		}
	})

	It("UT-AF-SLO-002: all alerts have a runbook_url annotation", func() {
		for _, group := range prf.Spec.Groups {
			for _, r := range group.Rules {
				if r.Alert == "" {
					continue
				}
				Expect(r.Annotations).To(HaveKey("runbook_url"),
					"alert %s in group %s is missing runbook_url annotation", r.Alert, group.Name)
				Expect(r.Annotations["runbook_url"]).NotTo(BeEmpty(),
					"alert %s has empty runbook_url", r.Alert)
			}
		}
	})

	It("UT-AF-SLO-003: all alerts have a severity label", func() {
		validSeverities := map[string]bool{"info": true, "warning": true, "critical": true}
		for _, group := range prf.Spec.Groups {
			for _, r := range group.Rules {
				if r.Alert == "" {
					continue
				}
				Expect(r.Labels).To(HaveKey("severity"),
					"alert %s in group %s is missing severity label", r.Alert, group.Name)
				Expect(validSeverities).To(HaveKey(r.Labels["severity"]),
					"alert %s has invalid severity: %s", r.Alert, r.Labels["severity"])
			}
		}
	})

	It("UT-AF-SLO-004: metric names in rules match registered AF metrics", func() {
		// SYNC: update this map when adding/removing metrics in internal/metrics/metrics.go.
		// Include _bucket suffixes for histograms referenced by prometheus-rules.yaml.
		knownMetrics := map[string]bool{
			"af_http_request_duration_seconds":              true,
			"af_http_request_duration_seconds_bucket":       true,
			"af_http_requests_total":                        true,
			"af_tool_call_duration_seconds":                 true,
			"af_tool_call_duration_seconds_bucket":          true,
			"af_tool_calls_total":                           true,
			"af_circuit_breaker_state":                      true,
			"af_downstream_request_duration_seconds":        true,
			"af_downstream_request_duration_seconds_bucket": true,
			"af_rate_limit_rejections_total":                true,
			"af_mcp_rbac_denied_total":                      true,
			"af_sse_active_connections":                     true,
			"af_audit_buffer_overflow_total":                true,
			"af_severity_triage_total":                      true,
			"af_severity_triage_duration_seconds":           true,
			"af_severity_triage_duration_seconds_bucket":    true,
			"af_severity_triage_errors_total":               true,
			"up":                                            true,
		}

		for _, group := range prf.Spec.Groups {
			for _, r := range group.Rules {
				if r.Expr == "" {
					continue
				}
				expr, err := pqlParser.ParseExpr(strings.TrimSpace(r.Expr))
				if err != nil {
					continue
				}
				promparser.Inspect(expr, func(node promparser.Node, _ []promparser.Node) error {
					if vs, ok := node.(*promparser.VectorSelector); ok {
						metricName := vs.Name
						if metricName != "" {
							Expect(knownMetrics).To(HaveKey(metricName),
								"alert %s references unknown metric %s", r.Alert, metricName)
						}
					}
					return nil
				})
			}
		}
	})

	It("UT-AF-SLO-005: DefBuckets align with SLO thresholds", func() {
		defBuckets := []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}
		sloThresholds := []float64{0.05, 0.2, 0.5, 1.0, 2.0}

		for _, threshold := range sloThresholds {
			found := false
			for _, bucket := range defBuckets {
				ratio := bucket / threshold
				if ratio >= 0.75 && ratio <= 1.5 {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(),
				"SLO threshold %f has no aligned DefBucket within 50%%", threshold)
		}
	})
})
