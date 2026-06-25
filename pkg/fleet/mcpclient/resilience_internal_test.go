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

package mcpclient

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("UT-FLEET-RES-006 [AC-3]: Resilience layer correctly classifies retryable error patterns (BR-INTEGRATION-065)", func() {
	var rc *ResilientClient

	BeforeEach(func() {
		rc = &ResilientClient{
			logger: logr.Discard(),
		}
	})

	It("returns false for nil error", func() {
		Expect(rc.isRetryableError(nil)).To(BeFalse())
	})

	DescribeTable("isRetryableError classification",
		func(errMsg string, expected bool) {
			Expect(rc.isRetryableError(fmt.Errorf("%s", errMsg))).To(Equal(expected))
		},
		Entry("401 unauthorized is retryable", "HTTP 401 Unauthorized", true),
		Entry("session not found is retryable", "session not found for id abc123", true),
		Entry("connection refused is retryable", "dial tcp 127.0.0.1:1975: connection refused", true),
		Entry("EOF is retryable", "unexpected EOF", true),
		Entry("connection reset is retryable", "read: connection reset by peer", true),
		Entry("generic error is not retryable", "object not found: Pod/nginx", false),
		Entry("permission denied is not retryable", "forbidden: user lacks permission", false),
		Entry("validation error is not retryable", "admission webhook denied the request", false),
	)
})

// UT-FLEET-RES-OPT: Option and config unit tests
// Authority: BR-FLEET-054 (Fleet OAuth2 Authentication)
// FedRAMP: IA-5 (Authenticator Management) -- option wiring for token transport
var _ = Describe("UT-FLEET-RES-OPT: Options and ResilienceConfig", func() {
	Describe("DefaultResilienceConfig", func() {
		It("UT-FLEET-RES-OPT-001: should return production defaults", func() {
			cfg := DefaultResilienceConfig()
			Expect(cfg.InitialInterval).To(Equal(1 * time.Second))
			Expect(cfg.MaxInterval).To(Equal(30 * time.Second))
			Expect(cfg.MaxElapsedTime).To(Equal(5 * time.Minute))
			Expect(cfg.TokenRefreshTimeout).To(Equal(10 * time.Second))
		})
	})

	Describe("WithClusterID", func() {
		It("UT-FLEET-RES-OPT-002: should set clusterID on config", func() {
			cfg := &clientConfig{}
			WithClusterID("prod-east")(cfg)
			Expect(cfg.clusterID).To(Equal("prod-east"))
		})
	})

	Describe("WithHTTPClient", func() {
		It("UT-FLEET-RES-OPT-003: should set custom HTTP client", func() {
			cfg := &clientConfig{}
			custom := &http.Client{Timeout: 42 * time.Second}
			WithHTTPClient(custom)(cfg)
			Expect(cfg.httpClient).To(Equal(custom))
		})
	})

	Describe("WithTimeout", func() {
		It("UT-FLEET-RES-OPT-004: should create HTTP client if none set", func() {
			cfg := &clientConfig{}
			WithTimeout(15)(cfg)
			Expect(cfg.httpClient).ToNot(BeNil())
			Expect(cfg.httpClient.Timeout).To(Equal(15 * time.Second))
		})

		It("UT-FLEET-RES-OPT-005: should update existing HTTP client timeout", func() {
			existing := &http.Client{Timeout: 5 * time.Second}
			cfg := &clientConfig{httpClient: existing}
			WithTimeout(30)(cfg)
			Expect(cfg.httpClient.Timeout).To(Equal(30 * time.Second))
		})
	})

	Describe("WithTokenRefreshTimeout", func() {
		It("UT-FLEET-RES-OPT-006: should set timeout on new HTTP client when none exists", func() {
			cfg := &clientConfig{}
			WithTokenRefreshTimeout(7 * time.Second)(cfg)
			Expect(cfg.httpClient).ToNot(BeNil())
			Expect(cfg.httpClient.Timeout).To(Equal(7 * time.Second))
		})

		It("UT-FLEET-RES-OPT-007: should update existing HTTP client timeout", func() {
			existing := &http.Client{Timeout: 5 * time.Second}
			cfg := &clientConfig{httpClient: existing}
			WithTokenRefreshTimeout(12 * time.Second)(cfg)
			Expect(cfg.httpClient.Timeout).To(Equal(12 * time.Second))
		})
	})

	Describe("findReloadableTransport", func() {
		It("UT-FLEET-RES-OPT-008: should return nil when no OAuth2 transport is configured", func() {
			rc := &ResilientClient{
				opts: []Option{WithClusterID("test")},
			}
			Expect(rc.findReloadableTransport()).To(BeNil())
		})

		It("UT-FLEET-RES-OPT-009: should return nil when HTTP client has non-OAuth2 transport", func() {
			rc := &ResilientClient{
				opts: []Option{
					WithHTTPClient(&http.Client{Transport: http.DefaultTransport}),
				},
			}
			Expect(rc.findReloadableTransport()).To(BeNil())
		})
	})
})
