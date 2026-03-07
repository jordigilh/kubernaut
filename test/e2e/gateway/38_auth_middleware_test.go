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

// BR-GATEWAY-036: TokenReview Authentication E2E Tests
// BR-GATEWAY-037: SubjectAccessReview Authorization E2E Tests
// Authority: DD-AUTH-014 (Middleware-Based SAR Authentication)
//
// E2E tests use REAL K8s auth in Kind cluster (no mocks per INTEGRATION_E2E_NO_MOCKS_POLICY).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Gateway Auth Middleware E2E (BR-GATEWAY-036, BR-GATEWAY-037)", Ordered, func() {

	var (
		authorizedToken   string
		unauthorizedToken string
		authTestNamespace string
	)

	BeforeAll(func() {
		Expect(gatewayURL).ToNot(BeEmpty(), "gatewayURL must be set by suite setup")
		Expect(kubeconfigPath).ToNot(BeEmpty(), "kubeconfigPath must be set by suite setup")

		// ADR-053: Use managed namespace instead of 'default' (which is unmanaged by default).
		// Scope filtering rejects signals for resources in unmanaged namespaces (HTTP 200).
		authTestNamespace = helpers.CreateTestNamespaceAndWait(k8sClient, "gw-auth")

		By("Creating authorized ServiceAccount with RBAC for gateway-service/create")
		err := infrastructure.CreateE2EServiceAccountWithGatewayAccess(
			ctx,
			gatewayNamespace,
			kubeconfigPath,
			"e2e-gateway-authorized",
			GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "Should create authorized ServiceAccount")

		authorizedToken, err = infrastructure.GetServiceAccountToken(
			ctx,
			gatewayNamespace,
			"e2e-gateway-authorized",
			kubeconfigPath,
		)
		Expect(err).ToNot(HaveOccurred(), "Should get authorized token")
		Expect(authorizedToken).ToNot(BeEmpty(), "Authorized token must not be empty")

		By("Creating unauthorized ServiceAccount (no RBAC binding)")
		err = infrastructure.CreateServiceAccount(
			ctx,
			gatewayNamespace,
			kubeconfigPath,
			"e2e-gateway-unauthorized",
			GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "Should create unauthorized ServiceAccount")

		unauthorizedToken, err = infrastructure.GetServiceAccountToken(
			ctx,
			gatewayNamespace,
			"e2e-gateway-unauthorized",
			kubeconfigPath,
		)
		Expect(err).ToNot(HaveOccurred(), "Should get unauthorized token")
		Expect(unauthorizedToken).ToNot(BeEmpty(), "Unauthorized token must not be empty")

		helpers.EnsureTestPods(ctx, k8sClient, authTestNamespace,
			"e2e-auth-pod-001", "e2e-auth-pod-002", "e2e-auth-pod-003", "e2e-auth-pod-004")
	})

	Context("BR-GATEWAY-036: TokenReview Authentication", func() {

		It("E2E-GW-036-001: Gateway rejects unauthenticated Prometheus webhook request", func() {
			payload := buildAuthTestPayload("E2EAuthTest036001", authTestNamespace, "Pod", "e2e-auth-pod-001")

			req, err := http.NewRequest("POST",
				fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
				bytes.NewBuffer(payload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
				"E2E-GW-036-001: Unauthenticated request must return 401")

			body, _ := io.ReadAll(resp.Body)
			var problem map[string]interface{}
			Expect(json.Unmarshal(body, &problem)).To(Succeed(),
				"E2E-GW-036-001: Error response must be RFC 7807 JSON")
			Expect(problem["title"]).To(Equal("Unauthorized"))
		})

		It("E2E-GW-036-002: Gateway accepts authenticated and authorized Prometheus webhook", func() {
			payload := buildAuthTestPayload("E2EAuthTest036002", authTestNamespace, "Pod", "e2e-auth-pod-002")

			req, err := http.NewRequest("POST",
				fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
				bytes.NewBuffer(payload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authorizedToken))

			Eventually(func() int {
				r, _ := http.NewRequest("POST",
					fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
					bytes.NewBuffer(payload))
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
				r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authorizedToken))
				resp, err := http.DefaultClient.Do(r)
				if err != nil {
					return 0
				}
				defer func() { _ = resp.Body.Close() }()
				_, _ = io.ReadAll(resp.Body)
				return resp.StatusCode
			}, 30*time.Second, 2*time.Second).Should(Equal(http.StatusCreated),
				"E2E-GW-036-002: Authorized request must return 201 Created")

			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(ctx, rrList, &client.ListOptions{})
				if err != nil {
					return 0
				}
				count := 0
				for _, rr := range rrList.Items {
					if rr.Spec.SignalName == "E2EAuthTest036002" {
						count++
					}
				}
				return count
			}, 30*time.Second, 2*time.Second).Should(BeNumerically(">=", 1),
				"E2E-GW-036-002: RemediationRequest CRD must be created")
		})
	})

	Context("BR-GATEWAY-037: SubjectAccessReview Authorization", func() {

		It("E2E-GW-037-001: Gateway rejects authenticated but unauthorized K8s event webhook", func() {
			payload := []byte(fmt.Sprintf(`{
				"metadata": {"name": "e2e-auth-event-001", "namespace": "%s"},
				"involvedObject": {"kind": "Pod", "name": "e2e-auth-pod-003", "namespace": "%s"},
				"reason": "MemoryPressure",
				"type": "Warning"
			}`, authTestNamespace, authTestNamespace))

			req, err := http.NewRequest("POST",
				fmt.Sprintf("%s/api/v1/signals/kubernetes-event", gatewayURL),
				bytes.NewBuffer(payload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", unauthorizedToken))

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
				"E2E-GW-037-001: Authenticated but unauthorized SA must get 403")

			body, _ := io.ReadAll(resp.Body)
			var problem map[string]interface{}
			Expect(json.Unmarshal(body, &problem)).To(Succeed())
			Expect(problem["title"]).To(Equal("Forbidden"))
		})

		It("E2E-GW-037-002: Authorized ServiceAccount signal triggers full RR creation pipeline", func() {
			payload := buildAuthTestPayload("E2EAuthTest037002", authTestNamespace, "Pod", "e2e-auth-pod-004")

			req, err := http.NewRequest("POST",
				fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
				bytes.NewBuffer(payload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authorizedToken))

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			_, _ = io.ReadAll(resp.Body)

			Expect(resp.StatusCode).To(Equal(http.StatusCreated),
				"E2E-GW-037-002: Authorized request should create RR")

			Eventually(func() string {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(ctx, rrList, &client.ListOptions{})
				if err != nil {
					return ""
				}
				for _, rr := range rrList.Items {
					if rr.Spec.SignalName == "E2EAuthTest037002" {
						return string(rr.Status.OverallPhase)
					}
				}
				return ""
			}, 60*time.Second, 2*time.Second).ShouldNot(BeEmpty(),
				"E2E-GW-037-002: RR must enter the remediation pipeline")
		})
	})
})

// buildAuthTestPayload creates a minimal Prometheus AlertManager webhook payload for auth E2E tests
func buildAuthTestPayload(alertName, namespace, resourceKind, resourceName string) []byte {
	kindLabel := "pod"
	if resourceKind != "" {
		kindLabel = strings.ToLower(resourceKind)
	}

	payload := map[string]interface{}{
		"version":  "4",
		"groupKey": fmt.Sprintf("{}:{alertname=\"%s\"}", alertName),
		"status":   "firing",
		"alerts": []map[string]interface{}{
			{
				"status": "firing",
				"labels": map[string]string{
					"alertname": alertName,
					"namespace": namespace,
					"severity":  "warning",
					kindLabel:   resourceName,
				},
				"annotations": map[string]string{
					"summary": fmt.Sprintf("Auth E2E test alert: %s", alertName),
				},
				"startsAt": time.Now().Format(time.RFC3339),
			},
		},
	}

	data, _ := json.Marshal(payload)
	return data
}
