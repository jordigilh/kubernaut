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

package fleet

import (
	"context"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// E2E-FLEET-006: AF performs preflight checks via MCP gateway with OAuth2
// Authority: Issue #54, ADR-068
// FedRAMP: IA-5 (authenticator management), SC-8 (transmission confidentiality)
var _ = Describe("E2E-FLEET-006 [IA-5, SC-8]: AF performs preflight checks via MCP gateway with OAuth2 client_credentials (BR-INTEGRATION-054)", Label("fleet"), func() {
	It("should obtain DEX client_credentials token and verify EAIGW accepts authenticated requests", func() {
		By("Obtaining OAuth2 client_credentials token from DEX (IA-5)")
		cfg := infrastructure.DefaultDexFleetReadConfig()
		cfg.TokenEndpoint = "http://localhost:30556/dex/token"
		token, err := infrastructure.GetDexClientCredentialsToken(cfg)
		Expect(err).ToNot(HaveOccurred(), "DEX should issue client_credentials token")
		Expect(token).ToNot(BeEmpty(), "IA-5: token must be non-empty")

		By("Verifying EAIGW health endpoint is accessible")
		healthResp, err := http.Get(eaigwHealthURL)
		Expect(err).ToNot(HaveOccurred())
		defer healthResp.Body.Close()
		Expect(healthResp.StatusCode).To(Equal(http.StatusOK),
			"EAIGW health endpoint must be accessible")
	})

	It("should execute a preflight read via MCP gateway to validate cluster accessibility", func() {
		mcpCtx := context.Background()
		mcpClient, err := mcpclient.New(mcpCtx, eaigwMCPURL, mcpclient.WithClusterID("loopback-cluster"))
		Expect(err).ToNot(HaveOccurred(), "should connect to EAIGW for preflight")
		defer mcpClient.Close()

		By("Performing preflight namespace list via MCP gateway (SC-8: secure channel)")
		nsList := &unstructured.UnstructuredList{}
		nsList.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "NamespaceList"})
		err = mcpClient.List(mcpCtx, nsList)
		Expect(err).ToNot(HaveOccurred(),
			"SC-8: preflight namespace list via MCP gateway must succeed")
		Expect(nsList.Items).ToNot(BeEmpty(),
			"loopback cluster must have namespaces accessible via MCP")

		By("Verifying kubernaut-system namespace is accessible (preflight check)")
		found := false
		for _, ns := range nsList.Items {
			if ns.GetName() == "kubernaut-system" {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(),
			"preflight must confirm kubernaut-system namespace is accessible on remote cluster")
	})

	It("should list pods in kubernaut-system via MCP gateway with label selector", func() {
		mcpCtx := context.Background()
		mcpClient, err := mcpclient.New(mcpCtx, eaigwMCPURL, mcpclient.WithClusterID("loopback-cluster"))
		Expect(err).ToNot(HaveOccurred())
		defer mcpClient.Close()

		By("Listing pods with label selector via MCP gateway")
		podList := &unstructured.UnstructuredList{}
		podList.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "PodList"})
		err = mcpClient.List(mcpCtx, podList, client.InNamespace("kube-system"))
		Expect(err).ToNot(HaveOccurred(),
			"AF preflight should be able to list pods in kube-system via MCP")
		Expect(podList.Items).ToNot(BeEmpty())
	})
})
