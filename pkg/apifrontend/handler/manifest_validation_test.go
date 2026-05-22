package handler_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

// repoRoot returns the project root by walking up from the test file location.
func repoRoot() string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(f), "..", "..", "..")
}

// ---------------------------------------------------------------------------
// TC-A-02: ServiceMonitor must produce job=apifrontend
// ---------------------------------------------------------------------------

var _ = Describe("ServiceMonitor manifest", func() {
	var smData map[string]interface{}

	BeforeEach(func() {
		path := filepath.Join(repoRoot(), "deploy", "apifrontend", "base", "08-servicemonitor.yaml")
		raw, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred(), "failed to read ServiceMonitor YAML")
		Expect(yaml.Unmarshal(raw, &smData)).To(Succeed())
	})

	It("TC-A-02a: must set spec.jobLabel or include relabeling that produces job=apifrontend", func() {
		spec, ok := smData["spec"].(map[string]interface{})
		Expect(ok).To(BeTrue(), "spec missing from ServiceMonitor")

		// Option 1: spec.jobLabel is set
		jobLabel, hasJobLabel := spec["jobLabel"]

		// Option 2: endpoints[].relabelings set job label
		hasRelabeling := false
		if endpoints, ok := spec["endpoints"].([]interface{}); ok {
			for _, ep := range endpoints {
				epMap, ok := ep.(map[string]interface{})
				if !ok {
					continue
				}
				if relabelings, ok := epMap["relabelings"].([]interface{}); ok {
					for _, r := range relabelings {
						rMap, ok := r.(map[string]interface{})
						if !ok {
							continue
						}
						if rMap["targetLabel"] == "job" {
							hasRelabeling = true
						}
					}
				}
			}
		}

		Expect(hasJobLabel || hasRelabeling).To(BeTrue(),
			"ServiceMonitor must set spec.jobLabel or relabeling for job=apifrontend; "+
				"current jobLabel=%v, hasRelabeling=%v", jobLabel, hasRelabeling)
	})

	It("TC-A-02c: selector.matchLabels must be consistent with metadata", func() {
		spec, ok := smData["spec"].(map[string]interface{})
		Expect(ok).To(BeTrue(), "spec must be a map")
		selector, ok := spec["selector"].(map[string]interface{})
		Expect(ok).To(BeTrue(), "spec.selector must be a map")
		matchLabels, ok := selector["matchLabels"].(map[string]interface{})
		Expect(ok).To(BeTrue(), "spec.selector.matchLabels must be a map")
		Expect(matchLabels).To(HaveKey("app.kubernetes.io/name"))
	})
})

// ---------------------------------------------------------------------------
// TC-A-11: PromQL dependency latency must include dependency in by() clause
// ---------------------------------------------------------------------------

var _ = Describe("PrometheusRule manifest", func() {
	type alertRule struct {
		Alert       string            `yaml:"alert"`
		Expr        string            `yaml:"expr"`
		Annotations map[string]string `yaml:"annotations"`
	}
	type ruleGroup struct {
		Name  string      `yaml:"name"`
		Rules []alertRule `yaml:"rules"`
	}
	type prometheusRule struct {
		Spec struct {
			Groups []ruleGroup `yaml:"groups"`
		} `yaml:"spec"`
	}

	var pr prometheusRule

	BeforeEach(func() {
		path := filepath.Join(repoRoot(), "deploy", "apifrontend", "base", "05-prometheusrule.yaml")
		raw, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(yaml.Unmarshal(raw, &pr)).To(Succeed())
	})

	It("TC-A-11b: ApifrontendDependencyLatencyHigh must use by(le, dependency)", func() {
		for _, g := range pr.Spec.Groups {
			for _, r := range g.Rules {
				if r.Alert == "ApifrontendDependencyLatencyHigh" {
					Expect(r.Expr).To(ContainSubstring("by (le, dependency)"),
						"ApifrontendDependencyLatencyHigh expr must aggregate by (le, dependency), got: %s", r.Expr)
					return
				}
			}
		}
		Fail("ApifrontendDependencyLatencyHigh alert not found in PrometheusRule")
	})

	It("TC-A-11c: all rules referencing labels.dependency must include dependency in by()", func() {
		for _, g := range pr.Spec.Groups {
			for _, r := range g.Rules {
				if strings.Contains(r.Annotations["description"], "{{ $labels.dependency }}") {
					Expect(r.Expr).To(ContainSubstring("dependency"),
						"alert %s references $labels.dependency in annotation but PromQL has no dependency in aggregation: %s",
						r.Alert, r.Expr)
				}
			}
		}
	})

	It("TC-A-02b: all expr fields must filter on job=apifrontend", func() {
		for _, g := range pr.Spec.Groups {
			for _, r := range g.Rules {
				if r.Expr == "" {
					continue
				}
				Expect(r.Expr).To(ContainSubstring(`job="apifrontend"`),
					"alert %s must filter on job=\"apifrontend\", expr: %s", r.Alert, r.Expr)
			}
		}
	})
})

// ---------------------------------------------------------------------------
// TC-A-RBAC-01: Tool names in Helm values.yaml personas must match bridge registration
// ---------------------------------------------------------------------------

var _ = Describe("RBAC tool name alignment", func() {
	registeredTools := map[string]bool{
		"kubernaut_list_remediations":        true,
		"kubernaut_get_remediation":          true,
		"kubernaut_approve":                  true,
		"kubernaut_cancel_remediation":       true,
		"kubernaut_watch":                    true,
		"kubernaut_start_investigation":      true,
		"kubernaut_poll_investigation":       true,
		"kubernaut_select_workflow":          true,
		"kubernaut_discover_workflows":       true,
		"kubernaut_present_decision":         true,
		"kubernaut_list_workflows":           true,
		"kubernaut_get_remediation_history":  true,
		"kubernaut_get_effectiveness":        true,
		"kubernaut_get_audit_trail":          true,
		"kubernaut_takeover":                true,
		"kubernaut_message":                 true,
		"kubernaut_complete":                true,
		"kubernaut_cancel":                  true,
		"kubernaut_status":                  true,
		"kubernaut_reconnect":               true,
		"kubernaut_stream_investigation":    true,
	}

	// A2A-only internal tools registered in pkg/apifrontend/agent/root.go.
	// These are exposed via A2A (ADK) and authorized through the same persona
	// definitions but are NOT registered in the MCP bridge.
	a2aInternalTools := map[string]bool{
		"kubectl_get":                       true,
		"kubectl_list":                      true,
		"kubectl_list_events":               true,
		"check_existing_rr":                 true,
		"create_rr":                         true,
	}

	allKnownTools := map[string]bool{}
	for k, v := range registeredTools {
		allKnownTools[k] = v
	}
	for k, v := range a2aInternalTools {
		allKnownTools[k] = v
	}

	type helmValues struct {
		Apifrontend struct {
			Config struct {
				RBAC struct {
					Personas map[string][]string `yaml:"personas"`
				} `yaml:"rbac"`
			} `yaml:"config"`
		} `yaml:"apifrontend"`
	}

	loadPersonas := func() map[string][]string {
		path := filepath.Join(repoRoot(), "charts", "kubernaut", "values.yaml")
		raw, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		var v helmValues
		Expect(yaml.Unmarshal(raw, &v)).To(Succeed())
		return v.Apifrontend.Config.RBAC.Personas
	}

	It("TC-A-RBAC-01a: every tool in Helm persona definitions must exist in bridge or A2A registration", func() {
		personas := loadPersonas()
		for persona, tools := range personas {
			for _, tool := range tools {
				Expect(allKnownTools).To(HaveKey(tool),
					"persona %q references tool %q which is not registered in mcp_bridge.go or agent/root.go", persona, tool)
			}
		}
	})

	It("TC-A-RBAC-01b: every registered MCP tool must appear in at least one persona", func() {
		personas := loadPersonas()
		allPersonaTools := map[string]bool{}
		for _, tools := range personas {
			for _, t := range tools {
				allPersonaTools[t] = true
			}
		}
		for tool := range registeredTools {
			Expect(allPersonaTools).To(HaveKey(tool),
				"registered tool %q has no persona assignment in Helm values", tool)
		}
	})

	It("TC-A-RBAC-01c: Helm personas must use kubernaut_present_decision (MCP bridge name)", func() {
		path := filepath.Join(repoRoot(), "charts", "kubernaut", "values.yaml")
		raw, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).NotTo(MatchRegexp(`(?m)^\s*-\s+present_decision\s*$`),
			"Helm values should use 'kubernaut_present_decision' (MCP bridge registration name), not bare 'present_decision'")
	})
})

// ---------------------------------------------------------------------------
// TC-P1-09: NetworkPolicy Prometheus egress targets monitoring namespace
// ---------------------------------------------------------------------------

var _ = Describe("NetworkPolicy manifest", func() {
	type networkPolicy struct {
		Spec struct {
			Egress []struct {
				To []struct {
					NamespaceSelector struct {
						MatchLabels map[string]string `yaml:"matchLabels"`
					} `yaml:"namespaceSelector"`
				} `yaml:"to"`
				Ports []struct {
					Port int `yaml:"port"`
				} `yaml:"ports"`
			} `yaml:"egress"`
		} `yaml:"spec"`
	}

	var np networkPolicy

	BeforeEach(func() {
		path := filepath.Join(repoRoot(), "deploy", "apifrontend", "base", "06-networkpolicy.yaml")
		raw, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(yaml.Unmarshal(raw, &np)).To(Succeed())
	})

	It("TC-P1-09a: Prometheus egress rule targets monitoring namespace", func() {
		found := false
		for _, rule := range np.Spec.Egress {
			isPrometheusPort := false
			for _, p := range rule.Ports {
				if p.Port == 9090 {
					isPrometheusPort = true
					break
				}
			}
			if !isPrometheusPort {
				continue
			}
			for _, to := range rule.To {
				ns := to.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"]
				Expect(ns).To(Equal("monitoring"),
					"Prometheus egress (port 9090) must target 'monitoring' namespace, got %q", ns)
				found = true
			}
		}
		Expect(found).To(BeTrue(), "no egress rule found for port 9090")
	})

	It("TC-P1-09b: Prometheus namespace matches config reference", func() {
		configPath := filepath.Join(repoRoot(), "deploy", "apifrontend", "base", "config.yaml")
		configRaw, err := os.ReadFile(configPath)
		Expect(err).NotTo(HaveOccurred())
		configText := string(configRaw)

		for _, rule := range np.Spec.Egress {
			isPrometheusPort := false
			for _, p := range rule.Ports {
				if p.Port == 9090 {
					isPrometheusPort = true
					break
				}
			}
			if !isPrometheusPort {
				continue
			}
			for _, to := range rule.To {
				ns := to.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"]
				Expect(configText).To(ContainSubstring(ns),
					"NetworkPolicy Prometheus namespace %q not found in config.yaml", ns)
			}
		}
	})
})

// ---------------------------------------------------------------------------
// UT-AF-1226-030/031: Impersonation ClusterRole must not include serviceaccounts
// ---------------------------------------------------------------------------

var _ = Describe("Impersonation ClusterRole manifests", func() {
	readYAMLRules := func(path string) []map[string]interface{} {
		raw, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred(), "failed to read %s", path)
		var doc map[string]interface{}
		Expect(yaml.Unmarshal(raw, &doc)).To(Succeed())
		rulesRaw, ok := doc["rules"].([]interface{})
		Expect(ok).To(BeTrue(), "rules missing from %s", path)
		var rules []map[string]interface{}
		for _, r := range rulesRaw {
			rm, ok := r.(map[string]interface{})
			Expect(ok).To(BeTrue())
			rules = append(rules, rm)
		}
		return rules
	}

	hasServiceAccounts := func(rules []map[string]interface{}) bool {
		for _, rule := range rules {
			verbs, ok := rule["verbs"].([]interface{})
			if !ok {
				continue
			}
			isImpersonate := false
			for _, v := range verbs {
				if v == "impersonate" {
					isImpersonate = true
					break
				}
			}
			if !isImpersonate {
				continue
			}
			resources, ok := rule["resources"].([]interface{})
			if !ok {
				continue
			}
			for _, r := range resources {
				if r == "serviceaccounts" {
					return true
				}
			}
		}
		return false
	}

	It("UT-AF-1226-030: Helm ClusterRole must not include serviceaccounts in impersonation", func() {
		path := filepath.Join(repoRoot(), "charts", "kubernaut", "templates", "apifrontend", "apifrontend.yaml")
		raw, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		content := string(raw)
		Expect(content).NotTo(ContainSubstring("serviceaccounts"),
			"Helm ClusterRole template must not reference serviceaccounts in impersonation rules")
	})

	It("UT-AF-1226-031: Deploy base ClusterRole must not include serviceaccounts in impersonation", func() {
		path := filepath.Join(repoRoot(), "deploy", "apifrontend", "base", "02-rbac.yaml")
		rules := readYAMLRules(path)
		Expect(hasServiceAccounts(rules)).To(BeFalse(),
			"deploy/apifrontend/base/02-rbac.yaml must not include serviceaccounts in impersonation rules")
	})
})

// ---------------------------------------------------------------------------
// TC-P3-07: Dockerfile FIPS boringcrypto (MED-10, BAC-14)
// ---------------------------------------------------------------------------

var _ = Describe("Dockerfile FIPS Compliance", func() {
	It("TC-P3-07a: Dockerfile sets GOEXPERIMENT=boringcrypto", func() {
		dockerfilePath := filepath.Join(repoRoot(), "docker", "apifrontend.Dockerfile")
		content, err := os.ReadFile(dockerfilePath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(ContainSubstring("GOEXPERIMENT=boringcrypto"),
			"TC-P3-07a: Dockerfile must set GOEXPERIMENT=boringcrypto for FIPS compliance")
	})
})
