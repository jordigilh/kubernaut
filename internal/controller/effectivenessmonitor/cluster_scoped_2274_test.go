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

package controller

import (
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL convention
	. "github.com/onsi/gomega"    //nolint:staticcheck // Gomega DSL convention

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
)

// Issue #2274: Fleet metric/alert queries built by EM must be scoped to the
// remediation's cluster (ea.Spec.ClusterID, BR-FLEET-054), which is exactly
// the Thanos "cluster" external label. Without a cluster= matcher, a
// remote-cluster remediation's PromQL/AlertManager query can silently match
// a same-named resource on a different fleet cluster (DD-EM-005 v1.3
// addendum). Empty ClusterID (hub/local remediations) MUST produce
// byte-identical queries to pre-#2274 behavior.
var _ = Describe("cluster-scoped PromQL query builders (BR-EM-003, DD-EM-005 v1.3, Issue #2274)", func() {

	// UT-EM-2274-001
	Describe("buildMetricQuerySpecs (namespace-scoped)", func() {
		It("appends a cluster= matcher to every selector when clusterID is set", func() {
			specs := buildMetricQuerySpecs("prod", "remote-cluster")
			Expect(specs).To(HaveLen(5))
			for _, s := range specs {
				Expect(s.Query).To(ContainSubstring(`cluster="remote-cluster"`),
					"query %q must be scoped to the fleet cluster", s.Name)
			}
		})

		It("scopes BOTH sides of the http_error_rate ratio (two independent sum() selectors)", func() {
			specs := buildMetricQuerySpecs("prod", "remote-cluster")
			var errRate *metricQuerySpec
			for i := range specs {
				if specs[i].Name == "http_error_rate" {
					errRate = &specs[i]
				}
			}
			Expect(errRate).ToNot(BeNil())
			Expect(strings.Count(errRate.Query, `cluster="remote-cluster"`)).To(Equal(2),
				"both the numerator and denominator sum() must be independently cluster-scoped, "+
					"otherwise the ratio silently blends cluster error rates (Issue #2274 spike finding)")
		})

		It("UT-EM-2274-BC-001: produces byte-identical queries to pre-#2274 behavior when clusterID is empty", func() {
			specs := buildMetricQuerySpecs("prod", "")
			for _, s := range specs {
				Expect(s.Query).ToNot(ContainSubstring("cluster="),
					"empty clusterID (hub/local remediation) must not introduce a cluster= matcher")
			}
		})
	})

	// UT-EM-2274-002
	Describe("buildNodeMetricQuerySpecs", func() {
		It("appends a cluster= matcher to all 3 Node condition queries", func() {
			specs := buildNodeMetricQuerySpecs("worker-1", "remote-cluster")
			Expect(specs).To(HaveLen(3))
			for _, s := range specs {
				Expect(s.Query).To(ContainSubstring(`cluster="remote-cluster"`))
				Expect(s.Query).To(ContainSubstring(`node="worker-1"`))
			}
		})

		It("UT-EM-2274-BC-002: produces byte-identical queries to pre-#2274 behavior when clusterID is empty", func() {
			specs := buildNodeMetricQuerySpecs("worker-1", "")
			for _, s := range specs {
				Expect(s.Query).ToNot(ContainSubstring("cluster="))
			}
		})
	})

	// UT-EM-2274-003
	Describe("buildPVMetricQuerySpecs", func() {
		// usageRatioSpecName dedups the goconst-flagged literal repeated across
		// the three usage-ratio assertions below.
		const usageRatioSpecName = "kubelet_volume_stats_used_bytes_ratio"

		It("appends a cluster= matcher to the Failed/Pending phase queries", func() {
			specs := buildPVMetricQuerySpecs("pvc-abc123", "remote-cluster")
			byName := make(map[string]metricQuerySpec, len(specs))
			for _, s := range specs {
				byName[s.Name] = s
			}
			Expect(byName["kube_persistentvolume_status_phase_failed"].Query).To(ContainSubstring(`cluster="remote-cluster"`))
			Expect(byName["kube_persistentvolume_status_phase_pending"].Query).To(ContainSubstring(`cluster="remote-cluster"`))
		})

		It("scopes ALL THREE joined metrics in the usage-ratio query (spike finding: on()/group_left() ignore unlisted labels, so each raw selector must be pre-filtered)", func() {
			specs := buildPVMetricQuerySpecs("pvc-abc123", "remote-cluster")
			var usage *metricQuerySpec
			for i := range specs {
				if specs[i].Name == usageRatioSpecName {
					usage = &specs[i]
				}
			}
			Expect(usage).ToNot(BeNil())
			Expect(usage.Query).To(ContainSubstring(`kubelet_volume_stats_used_bytes{cluster="remote-cluster"}`),
				"the raw kubelet_volume_stats_used_bytes selector (left side of the join) must be cluster-scoped")
			Expect(strings.Count(usage.Query, `cluster="remote-cluster"`)).To(Equal(3),
				"all 3 joined metrics (kubelet_volume_stats_used_bytes, kube_persistentvolume_claim_ref, "+
					"kube_persistentvolume_capacity_bytes) must each carry the cluster matcher — "+
					"on(namespace, persistentvolumeclaim)/on(persistentvolume) do not restrict matching by cluster")
		})

		It("UT-EM-2274-BC-003: produces byte-identical queries to pre-#2274 behavior when clusterID is empty", func() {
			specs := buildPVMetricQuerySpecs("pvc-abc123", "")
			for _, s := range specs {
				Expect(s.Query).ToNot(ContainSubstring("cluster="))
			}
			var usage *metricQuerySpec
			for i := range specs {
				if specs[i].Name == usageRatioSpecName {
					usage = &specs[i]
				}
			}
			Expect(usage.Query).To(ContainSubstring("(kubelet_volume_stats_used_bytes "),
				"bare metric name (no braces) must be preserved when clusterID is empty")
		})
	})

	// UT-EM-2274-004
	Describe("buildKSMFlagQuerySpec", func() {
		It("appends cluster= after resource and extra matchers when clusterID is set", func() {
			spec := buildKSMFlagQuerySpec("test_flag", "some_metric", "node", "worker-1", "remote-cluster", `condition="Ready"`)
			Expect(spec.Query).To(Equal(`some_metric{node="worker-1",condition="Ready",cluster="remote-cluster"}`))
		})

		It("omits the cluster matcher entirely when clusterID is empty", func() {
			spec := buildKSMFlagQuerySpec("test_flag", "some_metric", "node", "worker-1", "")
			Expect(spec.Query).To(Equal(`some_metric{node="worker-1"}`))
		})
	})

	// UT-EM-2274-005
	Describe("buildMetricQuerySpecsForTarget", func() {
		It("threads clusterID through to the namespace-scoped builder", func() {
			target := eav1.TargetResource{Namespace: "prod", Name: "my-deploy"}
			specs := buildMetricQuerySpecsForTarget(target, "remote-cluster")
			Expect(specs).ToNot(BeEmpty())
			for _, s := range specs {
				Expect(s.Query).To(ContainSubstring(`cluster="remote-cluster"`))
			}
		})

		It("threads clusterID through to the Node cluster-scoped builder", func() {
			target := eav1.TargetResource{Kind: "Node", Name: "worker-1"}
			specs := buildMetricQuerySpecsForTarget(target, "remote-cluster")
			Expect(specs).ToNot(BeEmpty())
			for _, s := range specs {
				Expect(s.Query).To(ContainSubstring(`cluster="remote-cluster"`))
			}
		})

		It("threads clusterID through to the PersistentVolume cluster-scoped builder", func() {
			target := eav1.TargetResource{Kind: "PersistentVolume", Name: "pvc-abc123"}
			specs := buildMetricQuerySpecsForTarget(target, "remote-cluster")
			Expect(specs).ToNot(BeEmpty())
			for _, s := range specs {
				Expect(s.Query).To(ContainSubstring(`cluster="remote-cluster"`))
			}
		})
	})
})
