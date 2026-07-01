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

package fleetmetadatacache

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
)

// fmcSyncTimeout bounds the retry window for a real FMC sync cycle to pick up
// a newly labeled resource. FMC's sync.interval=10s / keyTtl=30s (see
// test/infrastructure/fleet_e2e.go DeployFleetCoreInfra), and syncAll()
// iterates 3 registered clusters (loopback-cluster, prod-east, prod-west) x 6
// resource kinds sequentially each cycle, so worst-case staleness exceeds the
// nominal 10s interval. 60s gives ample margin (mirrors test/e2e/fleet's
// fmcSyncTimeout rationale).
const fmcSyncTimeout = 60 * time.Second

// scopeCheck queries FMC's real /api/v1/scope/check endpoint and returns the
// decoded "managed" boolean. No Valkey keys are seeded by the test -- this
// exercises FMC's actual HTTP API end to end.
func scopeCheck(g Gomega, clusterID, group, version, kind, namespace, name string) bool {
	q := url.Values{}
	q.Set("cluster", clusterID)
	q.Set("group", group)
	q.Set("version", version)
	q.Set("kind", kind)
	q.Set("namespace", namespace)
	q.Set("name", name)

	resp, err := fmcHTTPClient.Get(fmcAPIBaseURL + fmc.ScopeCheckPath + "?" + q.Encode())
	g.Expect(err).ToNot(HaveOccurred(), "scope check request failed")
	defer func() { _ = resp.Body.Close() }()
	g.Expect(resp.StatusCode).To(Equal(http.StatusOK), "scope check should return 200")

	var body fmc.ScopeCheckResponse
	g.Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
	return body.Managed
}

// listClusters queries FMC's real /api/v1/clusters endpoint.
func listClusters(g Gomega) []fmc.ClusterInfoResponse {
	resp, err := fmcHTTPClient.Get(fmcAPIBaseURL + fmc.ClustersPath)
	g.Expect(err).ToNot(HaveOccurred(), "cluster list request failed")
	defer func() { _ = resp.Body.Close() }()
	g.Expect(resp.StatusCode).To(Equal(http.StatusOK), "cluster list should return 200")

	var body fmc.ClusterListResponse
	g.Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
	return body.Clusters
}

// E2E-FMC-054-010: Proves FMC's real sync journey end to end -- the one
// journey no unit or integration test can prove, since IT-FLEET-VALKEY-004
// manually seeds Valkey keys instead of exercising the real DEX OAuth2 ->
// Kuadrant MCP Gateway -> kube-mcp-server -> Valkey pipeline.
//
// Authority: Issue #54, ADR-068 (SC-7 boundary protection, AC-3 access
// enforcement), BR-INTEGRATION-065.
var _ = Describe("E2E-FMC-054-010: FMC discovers managed resources via the real DEX+Kuadrant+kube-mcp-server pipeline", Ordered, func() {
	var testNS *corev1.Namespace

	BeforeAll(func() {
		testNS = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("fmc-e2e-managed-%d", time.Now().UnixNano()),
			},
		}
		Expect(k8sClient.Create(ctx, testNS)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, testNS)
		})
	})

	It("lists loopback-cluster, prod-east, and prod-west via real MCPServerRegistration discovery", func() {
		Eventually(func(g Gomega) {
			clusters := listClusters(g)
			ids := make([]string, 0, len(clusters))
			for _, c := range clusters {
				ids = append(ids, c.ID)
			}
			g.Expect(ids).To(ContainElements("loopback-cluster", "prod-east", "prod-west"))
		}, timeout, interval).Should(Succeed())
	})

	It("marks a kubernaut.ai/managed=true Service as managed after a real sync cycle (SC-7, AC-3)", func() {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fmc-e2e-managed-svc",
				Namespace: testNS.Name,
				Labels:    map[string]string{"kubernaut.ai/managed": "true"},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Port: 80}},
			},
		}
		Expect(k8sClient.Create(ctx, svc)).To(Succeed())

		Eventually(func(g Gomega) {
			managed := scopeCheck(g, "loopback-cluster", "", "v1", "Service", testNS.Name, "fmc-e2e-managed-svc")
			g.Expect(managed).To(BeTrue(),
				"resource labeled kubernaut.ai/managed=true should be discovered by FMC's real sync pipeline")
		}, fmcSyncTimeout, interval).Should(Succeed())
	})
})
