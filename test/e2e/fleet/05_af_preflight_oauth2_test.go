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
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// E2E-FLEET-006: AF performs preflight checks via MCP gateway with OAuth2
// Authority: Issue #54, ADR-068
// FedRAMP: IA-5 (authenticator management), SC-8 (transmission confidentiality)
var _ = Describe("E2E-FLEET-006 [IA-5, SC-8]: AF performs preflight checks via MCP gateway with OAuth2 client_credentials (BR-INTEGRATION-054)", Label("fleet"), func() {
	It("should obtain Keycloak client_credentials token and verify MCP gateway accepts authenticated requests", func() {
		// Keycloak replaces DEX in this suite (RFC 8693 Standard Token
		// Exchange, Spike S17/S20 -- DEX has no Standard Token Exchange).
		By("Obtaining OAuth2 client_credentials token from Keycloak (IA-5)")
		// 30557: this suite's Keycloak NodePort (DD-TEST-001, same dedicated
		// port as the FMC E2E lane -- see keycloakHostPortFleet in fleet_e2e.go).
		cfg := infrastructure.DefaultKeycloakFleetReadConfig(30557)
		cfg.Scopes = []string{"kube-mcp-server-audience"}
		token, err := infrastructure.GetKeycloakClientCredentialsToken(cfg)
		Expect(err).ToNot(HaveOccurred(), "Keycloak should issue client_credentials token")
		Expect(token).ToNot(BeEmpty(), "IA-5: token must be non-empty")

		By("Verifying MCP gateway is reachable via NodePort")
		mcpCtx := context.Background()
		authClient := &http.Client{Transport: testauth.NewStaticTokenTransport(token)}
		mcpClient, err := mcpclient.New(mcpCtx, mcpGatewayURL, mcpclient.WithHTTPClient(authClient))
		Expect(err).ToNot(HaveOccurred(), "MCP gateway must be reachable via NodePort")
		defer mcpClient.Close()

		tools, err := mcpClient.Session().ListTools(mcpCtx, nil)
		Expect(err).ToNot(HaveOccurred(), "MCP gateway must respond to tool listing")
		Expect(tools.Tools).ToNot(BeEmpty(), "MCP gateway must expose tools")
	})

	It("should execute a preflight read via MCP gateway to validate cluster accessibility", func() {
		mcpCtx := context.Background()
		mcpClient, err := newFleetMCPClient(mcpCtx, "remote-cluster")
		Expect(err).ToNot(HaveOccurred(), "should connect to MCP gateway for preflight")
		defer mcpClient.Close()

		By("Performing preflight namespace list via MCP gateway (SC-8: secure channel)")
		nsList := &unstructured.UnstructuredList{}
		nsList.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "NamespaceList"})
		err = mcpClient.List(mcpCtx, nsList)
		Expect(err).ToNot(HaveOccurred(),
			"SC-8: preflight namespace list via MCP gateway must succeed")
		Expect(nsList.Items).ToNot(BeEmpty(),
			"remote cluster must have namespaces accessible via MCP")

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
		mcpClient, err := newFleetMCPClient(mcpCtx, "remote-cluster")
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
