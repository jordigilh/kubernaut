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

package alert

import (
	"testing"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL convention
	. "github.com/onsi/gomega"    //nolint:staticcheck // Gomega DSL convention
)

func TestAlert(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EM Alert Scorer Suite")
}

// Issue #2274: AlertManager alert-resolution queries built by EM must be
// scoped to the remediation's cluster (ea.Spec.ClusterID, BR-FLEET-054) so a
// remote-cluster remediation cannot match an alert firing for a same-named
// resource on a different fleet cluster.
var _ = Describe("buildMatchers cluster-scoping (BR-EM-002, DD-EM-005 v1.3, Issue #2274)", func() {

	// UT-EM-2274-006
	It("appends a cluster= matcher when AlertContext.ClusterID is set", func() {
		matchers := buildMatchers(AlertContext{
			AlertName: "HighCPU",
			Namespace: "prod",
			ClusterID: "remote-cluster",
		})
		Expect(matchers).To(ContainElement(`cluster="remote-cluster"`))
	})

	// UT-EM-2274-007
	It("UT-EM-2274-BC-004: omits the cluster matcher entirely when ClusterID is empty (backward compat)", func() {
		matchers := buildMatchers(AlertContext{
			AlertName: "HighCPU",
			Namespace: "prod",
		})
		for _, m := range matchers {
			Expect(m).ToNot(HavePrefix("cluster="))
		}
	})

	It("combines cluster= with AlertLabels for cluster-scoped Node/PV targets", func() {
		matchers := buildMatchers(AlertContext{
			AlertName:   "KubeNodeNotReady",
			ClusterID:   "remote-cluster",
			AlertLabels: map[string]string{"node": "worker-1"},
		})
		Expect(matchers).To(ContainElement(`cluster="remote-cluster"`))
		Expect(matchers).To(ContainElement(`node="worker-1"`))
	})
})
