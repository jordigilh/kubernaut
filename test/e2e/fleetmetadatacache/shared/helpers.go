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

package shared

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"strings"

	. "github.com/onsi/gomega" //nolint:staticcheck // Ginkgo/Gomega DSL dot-import convention

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
)

// ScopeCheck queries FMC's real /api/v1/scope/check endpoint and returns the
// decoded "managed" boolean. No Valkey keys are seeded by the test -- this
// exercises FMC's actual HTTP API end to end. Gateway-agnostic: FMC's own
// REST contract never varies by which gateway fronts kube-mcp-server.
func ScopeCheck(g Gomega, h *Harness, clusterID, group, version, kind, namespace, name string) bool {
	q := url.Values{}
	q.Set("cluster", clusterID)
	q.Set("group", group)
	q.Set("version", version)
	q.Set("kind", kind)
	q.Set("namespace", namespace)
	q.Set("name", name)

	req, err := http.NewRequestWithContext(h.Ctx, http.MethodGet, h.FMCAPIBaseURL+fmc.ScopeCheckPath+"?"+q.Encode(), http.NoBody)
	g.Expect(err).ToNot(HaveOccurred(), "failed to build scope check request")
	resp, err := h.FMCHTTPClient.Do(req)
	g.Expect(err).ToNot(HaveOccurred(), "scope check request failed")
	defer func() { _ = resp.Body.Close() }()
	g.Expect(resp.StatusCode).To(Equal(http.StatusOK), "scope check should return 200")

	var body fmc.ScopeCheckResponse
	g.Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
	return body.Managed
}

// ListClusters queries FMC's real /api/v1/clusters endpoint.
func ListClusters(g Gomega, h *Harness) []fmc.ClusterInfoResponse {
	req, err := http.NewRequestWithContext(h.Ctx, http.MethodGet, h.FMCAPIBaseURL+fmc.ClustersPath, http.NoBody)
	g.Expect(err).ToNot(HaveOccurred(), "failed to build cluster list request")
	resp, err := h.FMCHTTPClient.Do(req)
	g.Expect(err).ToNot(HaveOccurred(), "cluster list request failed")
	defer func() { _ = resp.Body.Close() }()
	g.Expect(resp.StatusCode).To(Equal(http.StatusOK), "cluster list should return 200")

	var body fmc.ClusterListResponse
	g.Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
	return body.Clusters
}

// ClusterIDs extracts just the .ID field from ListClusters for
// ContainElement/Not-ContainElement assertions.
func ClusterIDs(g Gomega, h *Harness) []string {
	clusters := ListClusters(g, h)
	ids := make([]string, 0, len(clusters))
	for _, c := range clusters {
		ids = append(ids, c.ID)
	}
	return ids
}

// ReadyzStatus queries FMC's real /readyz endpoint (wired in
// cmd/fleetmetadatacache/main.go via fmc.ReadyzHandler backed by
// scopecache.ValkeyCacheReader.Ping) and returns the HTTP status code. No
// path constant is exported for /readyz (unlike ScopeCheckPath/ClustersPath)
// since it is a Kubernetes probe endpoint, not a public API contract.
//
// Issue #1683: /readyz lives exclusively on FMC's dedicated health port
// (FMCHealthBaseURL) since the 3-port split -- it is no longer served on
// the TLS-protected API port (FMCAPIBaseURL).
func ReadyzStatus(g Gomega, h *Harness) int {
	req, err := http.NewRequestWithContext(h.Ctx, http.MethodGet, h.FMCHealthBaseURL+"/readyz", http.NoBody)
	g.Expect(err).ToNot(HaveOccurred(), "failed to build /readyz request")
	resp, err := h.FMCHTTPClient.Do(req)
	g.Expect(err).ToNot(HaveOccurred(), "/readyz request failed")
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

// CanI runs `kubectl auth can-i <verb> <resource>` impersonating the given
// ServiceAccount and returns true if allowed. Uses --as (SubjectAccessReview
// via impersonation) rather than fetching a real token, since the assertion
// is about RBAC policy, not authentication.
func CanI(h *Harness, verb, resource, asServiceAccount string) bool {
	cmd := exec.CommandContext(h.Ctx, "kubectl", "--kubeconfig", h.KubeconfigPath,
		"auth", "can-i", verb, resource,
		"--as", fmt.Sprintf("system:serviceaccount:%s:%s", h.Namespace, asServiceAccount))
	out, _ := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)) == "yes"
}

// RawAPIServerRequest issues a GET to the real Kubernetes API server with an
// explicit Bearer token, bypassing client-go's normal credential plugin so
// the test can present exactly the token under evaluation (an exchanged
// token, or a deliberately un-exchanged one). TLS trust (CA only) is derived
// from the suite's own kubeconfig (same CA the API server's cert chains to).
//
// Critically, the suite's kubeconfig is Kind's cluster-admin config, which
// carries an mTLS client certificate for "kubernetes-admin". The API
// server's authenticator chain is a union: if a valid client cert is
// presented during the TLS handshake, x509 authentication succeeds
// regardless of the Bearer token's validity, silently defeating this test's
// entire purpose (a wrong-audience token would still get 200 as
// kubernetes-admin, not the 401 the OIDC path should produce). This function
// therefore strips CertData/CertFile/KeyData/KeyFile before building the TLS
// config so the ONLY credential in play is the Bearer header.
func RawAPIServerRequest(g Gomega, path, bearerToken string) *http.Response {
	restCfg, err := config.GetConfig()
	g.Expect(err).ToNot(HaveOccurred())

	caOnlyCfg := *restCfg
	caOnlyCfg.CertData = nil
	caOnlyCfg.CertFile = ""
	caOnlyCfg.KeyData = nil
	caOnlyCfg.KeyFile = ""

	tlsCfg, err := rest.TLSConfigFor(&caOnlyCfg)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(tlsCfg.Certificates).To(BeEmpty(),
		"test bug: TLS config must not carry a client certificate, or x509 auth would mask the Bearer-token check under test")

	httpClient := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, restCfg.Host+path, nil)
	g.Expect(err).ToNot(HaveOccurred())
	req.Header.Set("Authorization", "Bearer "+bearerToken)

	resp, err := httpClient.Do(req)
	g.Expect(err).ToNot(HaveOccurred())
	return resp
}
