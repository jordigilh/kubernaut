/*
Copyright 2025 Jordi Gil.

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

package gateway

// BR-GATEWAY-036: TokenReview Authentication Integration Tests
// BR-GATEWAY-037: SubjectAccessReview Authorization Integration Tests
// Authority: DD-AUTH-014 (Middleware-Based SAR Authentication)
//
// Integration tests use REAL K8s auth via envtest (no mocks per INTEGRATION_E2E_NO_MOCKS_POLICY).
// ServiceAccount tokens are created via SecurityTestTokens infrastructure.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gwpkg "github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

var _ = Describe("Gateway Authentication & Authorization (BR-GATEWAY-036, BR-GATEWAY-037)", Ordered, func() {

	var (
		testServer        *httptest.Server
		gatewayServer     *gwpkg.Server
		authTestNamespace string
	)

	BeforeAll(func() {
		ctx, cancel := contextWithTimeout(5 * time.Minute)
		defer cancel()

		// Setup security tokens (creates ServiceAccounts + tokens for auth tests)
		tokens := SetupSecurityTokens()

		testK8sClient, _ := createTestK8sClient(ctx)
		dataStorageURL := fmt.Sprintf("http://127.0.0.1:%d", infrastructure.GatewayIntegrationDataStoragePort)

		// ADR-053: Create a managed namespace so scope validation passes (BR-SCOPE-002).
		// 'default' namespace is unmanaged and scope filtering rejects signals for it.
		authTestNamespace = helpers.CreateTestNamespace(ctx, testK8sClient.Client, "gw-auth-int")

		// BR-GATEWAY-036/037: Wire real K8s auth using the same clientset as security tokens
		authenticator := auth.NewK8sAuthenticator(tokens.Clientset)
		authorizer := auth.NewK8sAuthorizer(tokens.Clientset)

		opts := DefaultTestServerOptions()
		opts.Authenticator = authenticator
		opts.Authorizer = authorizer

		var err error
		gatewayServer, err = StartTestGatewayWithOptions(ctx, testK8sClient, dataStorageURL, opts)
		Expect(err).ToNot(HaveOccurred(), "Gateway server must start successfully")

		testServer = httptest.NewServer(gatewayServer.Handler())
		DeferCleanup(func() {
			testServer.Close()
			helpers.DeleteTestNamespace(ctx, testK8sClient.Client, authTestNamespace)
		})
	})

	Context("BR-GATEWAY-036: TokenReview Authentication", func() {

		It("IT-GW-036-001: Unauthenticated Prometheus webhook request is rejected with 401", func() {
			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "AuthTestAlert036001",
				Namespace: authTestNamespace,
				Severity:  "warning",
				Resource:  ResourceIdentifier{Kind: "Pod", Name: "auth-test-pod"},
			})

			resp := SendWebhookWithAuth(
				testServer.URL+"/api/v1/signals/prometheus",
				payload,
				"", // No token — unauthenticated
			)

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
				"IT-GW-036-001: Unauthenticated webhook must return 401")

			var problem map[string]interface{}
			Expect(json.Unmarshal(resp.Body, &problem)).To(Succeed(),
				"IT-GW-036-001: Error response must be valid JSON (RFC 7807)")
			Expect(problem["title"]).To(Equal("Unauthorized"))
		})

		It("IT-GW-036-002: Authenticated and authorized Prometheus webhook creates RemediationRequest", func() {
			tokens := GetSecurityTokens()

			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "AuthTestAlert036002",
				Namespace: authTestNamespace,
				Severity:  "warning",
				Resource:  ResourceIdentifier{Kind: "Pod", Name: "auth-test-pod-ok"},
			})

			resp := SendWebhookWithAuth(
				testServer.URL+"/api/v1/signals/prometheus",
				payload,
				tokens.AuthorizedToken,
			)

			Expect(resp.StatusCode).To(Equal(http.StatusCreated),
				"IT-GW-036-002: Authorized webhook must return 201 Created")
		})

		It("IT-GW-036-003: Health and readiness endpoints bypass auth (dedicated server)", func() {
			// Issue #753: health/readiness now on a dedicated server without auth middleware.
			// Verify the exported handlers respond 200 without authentication.
			healthMux := http.NewServeMux()
			healthMux.HandleFunc("/healthz", gatewayServer.LivenessHandler())
			healthMux.HandleFunc("/readyz", gatewayServer.ReadinessHandler())
			healthTestServer := httptest.NewServer(healthMux)
			defer healthTestServer.Close()

			for _, endpoint := range []string{"/healthz", "/readyz"} {
				resp, err := http.Get(healthTestServer.URL + endpoint)
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("IT-GW-036-003: GET %s should not error", endpoint))
				body, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusOK),
					fmt.Sprintf("IT-GW-036-003: %s must return 200 without authentication (body: %s)", endpoint, string(body)))
			}
		})
	})

	Context("BR-GATEWAY-037: SubjectAccessReview Authorization", func() {

		It("IT-GW-037-001: Authenticated but unauthorized SA returns 403 Forbidden", func() {
			tokens := GetSecurityTokens()

			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "AuthTestAlert037001",
				Namespace: authTestNamespace,
				Severity:  "warning",
				Resource:  ResourceIdentifier{Kind: "Pod", Name: "auth-test-pod-unauth"},
			})

			resp := SendWebhookWithAuth(
				testServer.URL+"/api/v1/signals/prometheus",
				payload,
				tokens.UnauthorizedToken,
			)

			Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
				"IT-GW-037-001: Authenticated but unauthorized SA must get 403")

			var problem map[string]interface{}
			Expect(json.Unmarshal(resp.Body, &problem)).To(Succeed(),
				"IT-GW-037-001: Error response must be valid JSON (RFC 7807)")
			Expect(problem["title"]).To(Equal("Forbidden"))
		})

		It("IT-GW-037-002: K8s Event endpoint is equally protected by auth middleware", func() {
			resp := SendWebhookWithAuth(
				testServer.URL+"/api/v1/signals/kubernetes-event",
				[]byte(`{"metadata":{"name":"test-event"}}`),
				"", // No token — unauthenticated
			)

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
				"IT-GW-037-002: K8s Event endpoint must also require authentication")
		})
	})
})

func contextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}
