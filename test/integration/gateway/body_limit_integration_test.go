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

// Issue #673 C-1 + C-ADV-1: Request body size limit integration tests
// BR-GATEWAY-182: Defense-in-depth hardening
//
// These tests validate that the gateway rejects oversized request bodies
// with 413 Request Entity Too Large at the earliest body-reading middleware layer.
// Uses StartTestGatewayWithOptions with nil auth (body limit is orthogonal to auth).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

var _ = Describe("Issue #673: Request Body Size Limit (BR-GATEWAY-182)", Ordered, func() {

	var (
		testServer *httptest.Server
	)

	BeforeAll(func() {
		testCtx, cancel := contextWithTimeout(2 * time.Minute)
		defer cancel()

		testK8sClient, _ := createTestK8sClient(testCtx)
		dataStorageURL := fmt.Sprintf("http://127.0.0.1:%d", infrastructure.GatewayIntegrationDataStoragePort)

		// Nil auth: body size limit is enforced before auth middleware
		opts := &TestServerOptions{
			ReadTimeout:   5 * time.Second,
			WriteTimeout:  10 * time.Second,
			IdleTimeout:   120 * time.Second,
			Authenticator: nil,
			Authorizer:    nil,
		}

		gatewayServer, err := StartTestGatewayWithOptions(testCtx, testK8sClient, dataStorageURL, opts)
		Expect(err).ToNot(HaveOccurred(), "Gateway server must start successfully")

		testServer = httptest.NewServer(gatewayServer.Handler())
		DeferCleanup(func() {
			testServer.Close()
		})
	})

	It("IT-GW-673-001: Oversized body (>256KB) returns 413 Request Entity Too Large", func() {
		// 300KB payload exceeds the 256KB maxRequestBodySize
		oversizedBody := bytes.Repeat([]byte("A"), 300*1024)

		resp, err := http.Post(
			testServer.URL+"/api/v1/signals/prometheus",
			"application/json",
			bytes.NewReader(oversizedBody),
		)
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusRequestEntityTooLarge),
			"IT-GW-673-001: Oversized body must return 413")
		Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"),
			"IT-GW-673-001: Error response must be RFC 7807 format")

		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())
		var problem map[string]interface{}
		Expect(json.Unmarshal(body, &problem)).To(Succeed(),
			"IT-GW-673-001: Response body must be valid JSON")
		Expect(problem["status"]).To(BeEquivalentTo(413))
	})

	It("IT-GW-673-002: Normal body (<256KB) proceeds to adapter parsing", func() {
		// 100KB payload is within the 256KB limit
		normalBody := bytes.Repeat([]byte("B"), 100*1024)

		resp, err := http.Post(
			testServer.URL+"/api/v1/signals/prometheus",
			"application/json",
			bytes.NewReader(normalBody),
		)
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
			"IT-GW-673-002: Normal-sized body must reach adapter parsing and fail as malformed JSON (400)")

	})

	It("IT-GW-673-003: 413 response carries correct RFC 7807 type and title", func() {
		oversizedBody := bytes.Repeat([]byte("C"), 300*1024)

		resp, err := http.Post(
			testServer.URL+"/api/v1/signals/prometheus",
			"application/json",
			bytes.NewReader(oversizedBody),
		)
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusRequestEntityTooLarge))

		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())

		var problem map[string]interface{}
		Expect(json.Unmarshal(body, &problem)).To(Succeed())

		Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/payload-too-large"),
			"IT-GW-673-003: 413 response must use ErrorTypePayloadTooLarge URI")
		Expect(problem["title"]).To(Equal("Request Entity Too Large"),
			"IT-GW-673-003: 413 response must use TitlePayloadTooLarge")
		Expect(problem["status"]).To(BeEquivalentTo(413))
	})

	It("IT-GW-673-004: Boundary -- exactly 256KB passes, 256KB+1 is rejected", func() {
		const maxBody = 256 * 1024

		By("Sending exactly maxRequestBodySize bytes (should NOT get 413)")
		exactBody := bytes.Repeat([]byte("D"), maxBody)
		resp, err := http.Post(
			testServer.URL+"/api/v1/signals/prometheus",
			"application/json",
			bytes.NewReader(exactBody),
		)
		Expect(err).ToNot(HaveOccurred())
		resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
			"IT-GW-673-004: Exactly 256KB must reach adapter parsing and fail as malformed JSON (400)")

		By("Sending maxRequestBodySize + 1 bytes (MUST get 413)")
		overByOne := bytes.Repeat([]byte("E"), maxBody+1)
		resp2, err := http.Post(
			testServer.URL+"/api/v1/signals/prometheus",
			"application/json",
			bytes.NewReader(overByOne),
		)
		Expect(err).ToNot(HaveOccurred())
		resp2.Body.Close()
		Expect(resp2.StatusCode).To(Equal(http.StatusRequestEntityTooLarge),
			"IT-GW-673-004: 256KB+1 must be rejected with 413")
	})

	// C-ADV-1: Verify freshness middleware caps body reads
	// Without X-Timestamp, AlertManagerFreshnessValidator uses the body-fallback path.
	// The middleware must reject oversized bodies with 413 BEFORE full buffering.
	It("IT-GW-673-005: Oversized body via Prometheus freshness body-fallback returns 413", func() {
		oversizedBody := bytes.Repeat([]byte("F"), 300*1024)

		req, err := http.NewRequest(http.MethodPost,
			testServer.URL+"/api/v1/signals/prometheus",
			bytes.NewReader(oversizedBody))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		// No X-Timestamp header: forces body-fallback path in AlertManagerFreshnessValidator

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusRequestEntityTooLarge),
			"IT-GW-673-005: Freshness middleware must reject oversized body with 413")
		Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())
		var problem map[string]interface{}
		Expect(json.Unmarshal(body, &problem)).To(Succeed())
		Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/payload-too-large"),
			"IT-GW-673-005: Freshness 413 must use ErrorTypePayloadTooLarge")
		Expect(problem["title"]).To(Equal("Request Entity Too Large"),
			"IT-GW-673-005: Freshness 413 must use TitlePayloadTooLarge")
	})

	// C-ADV-1 + M-ADV-1: Verify kubernetes-event adapter also enforces body limit
	It("IT-GW-673-006: Oversized body via kubernetes-event endpoint returns 413", func() {
		oversizedBody := bytes.Repeat([]byte("G"), 300*1024)

		req, err := http.NewRequest(http.MethodPost,
			testServer.URL+"/api/v1/signals/kubernetes-event",
			bytes.NewReader(oversizedBody))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusRequestEntityTooLarge),
			"IT-GW-673-006: K8s event endpoint must reject oversized body with 413")
		Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())
		var problem map[string]interface{}
		Expect(json.Unmarshal(body, &problem)).To(Succeed())
		Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/payload-too-large"),
			"IT-GW-673-006: K8s event 413 must use ErrorTypePayloadTooLarge")
		Expect(problem["title"]).To(Equal("Request Entity Too Large"),
			"IT-GW-673-006: K8s event 413 must use TitlePayloadTooLarge")
	})
})
