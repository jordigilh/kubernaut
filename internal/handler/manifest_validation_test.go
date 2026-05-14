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
	return filepath.Join(filepath.Dir(f), "..", "..")
}

// ---------------------------------------------------------------------------
// TC-A-02: ServiceMonitor must produce job=apifrontend
// ---------------------------------------------------------------------------

var _ = Describe("ServiceMonitor manifest", func() {
	var smData map[string]interface{}

	BeforeEach(func() {
		path := filepath.Join(repoRoot(), "deploy", "kustomize", "base", "08-servicemonitor.yaml")
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
		path := filepath.Join(repoRoot(), "deploy", "kustomize", "base", "05-prometheusrule.yaml")
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
// TC-A-RBAC-01: Tool names in rbac_roles.yaml must match bridge registration
// ---------------------------------------------------------------------------

var _ = Describe("RBAC tool name alignment", func() {
	// Registered MCP tool names from mcp_bridge.go (canonical list)
	registeredTools := map[string]bool{
		"kubernaut_list_remediations":       true,
		"kubernaut_get_remediation":         true,
		"kubernaut_submit_signal":           true,
		"kubernaut_approve":                 true,
		"kubernaut_cancel_remediation":      true,
		"kubernaut_watch":                   true,
		"kubernaut_start_investigation":     true,
		"kubernaut_poll_investigation":      true,
		"kubernaut_select_workflow":         true,
		"kubernaut_present_decision":        true,
		"kubernaut_list_workflows":          true,
		"kubernaut_get_remediation_history": true,
		"kubernaut_get_effectiveness":       true,
		"kubernaut_get_audit_trail":         true,
		"af_list_events":                    true,
		"af_get_pods":                       true,
		"af_get_workloads":                  true,
		"af_resolve_owner":                  true,
		"af_check_existing_rr":              true,
		"af_create_rr":                      true,
	}

	It("TC-A-RBAC-01a: every tool in deploy rbac_roles.yaml must exist in bridge registration", func() {
		path := filepath.Join(repoRoot(), "deploy", "kustomize", "base", "rbac_roles.yaml")
		raw, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())

		var rbac struct {
			Roles map[string][]string `yaml:"roles"`
		}
		Expect(yaml.Unmarshal(raw, &rbac)).To(Succeed())

		for role, tools := range rbac.Roles {
			for _, tool := range tools {
				Expect(registeredTools).To(HaveKey(tool),
					"role %q references tool %q which is not registered in mcp_bridge.go", role, tool)
			}
		}
	})

	It("TC-A-RBAC-01b: every registered MCP tool must appear in at least one RBAC role", func() {
		path := filepath.Join(repoRoot(), "deploy", "kustomize", "base", "rbac_roles.yaml")
		raw, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())

		var rbac struct {
			Roles map[string][]string `yaml:"roles"`
		}
		Expect(yaml.Unmarshal(raw, &rbac)).To(Succeed())

		allRBACTools := map[string]bool{}
		for _, tools := range rbac.Roles {
			for _, t := range tools {
				allRBACTools[t] = true
			}
		}

		for tool := range registeredTools {
			Expect(allRBACTools).To(HaveKey(tool),
				"registered tool %q has no RBAC role assignment", tool)
		}
	})

	It("TC-A-RBAC-01c: deploy rbac_roles.yaml must use kubernaut_present_decision (MCP bridge name)", func() {
		path := filepath.Join(repoRoot(), "deploy", "kustomize", "base", "rbac_roles.yaml")
		raw, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())

		Expect(string(raw)).NotTo(MatchRegexp(`(?m)^\s*-\s+present_decision\s*$`),
			"deploy rbac_roles.yaml should use 'kubernaut_present_decision' (MCP bridge registration name), not bare 'present_decision'")
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
		path := filepath.Join(repoRoot(), "deploy", "kustomize", "base", "06-networkpolicy.yaml")
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
		configPath := filepath.Join(repoRoot(), "deploy", "kustomize", "base", "config.yaml")
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
