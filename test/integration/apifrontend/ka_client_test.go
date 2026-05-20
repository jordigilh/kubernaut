package apifrontend_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
)

var _ = Describe("KA Client Integration (ka/)", func() {

	Describe("AC-16: REST client round-trip with circuit breaker", func() {
		It("IT-AF-1195-024: REST client reaches real KA health endpoint", func() {
			ctx := context.Background()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:18131/healthz", nil)
			Expect(err).NotTo(HaveOccurred())

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			client := ka.NewClient(ka.Config{
				BaseURL:            "http://127.0.0.1:18130",
				Timeout:            10 * time.Second,
				CBFailureThreshold: 5,
				CBTimeout:          5 * time.Second,
			})
			Expect(client).NotTo(BeNil(), "client should construct against real KA endpoint")
		})

		It("IT-AF-1195-025: circuit breaker protects KA calls on failure", func() {
			failCount := 0
			failingBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				failCount++
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer failingBackend.Close()

			client := ka.NewClient(ka.Config{
				BaseURL:            failingBackend.URL,
				Timeout:            2 * time.Second,
				CBFailureThreshold: 3,
				CBTimeout:          100 * time.Millisecond,
				CBMaxRequests:      1,
			})

			ctx := context.Background()
			// Send requests until CB opens
			for i := 0; i < 5; i++ {
				_, _ = client.Analyze(ctx, ka.AnalyzeRequest{
					Namespace: "test", Kind: "Deployment", Name: "test-app",
				})
			}

			// After CB opens, subsequent requests should fail fast
			_, err := client.Analyze(ctx, ka.AnalyzeRequest{
				Namespace: "test", Kind: "Deployment", Name: "test-app",
			})
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("AC-18: JWT delegation passes token to KA", func() {
		It("IT-AF-1195-027: JWT delegation transport injects user token into KA request", func() {
			var receivedAuth string
			captureBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedAuth = r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"session_id":"test-123","status":"analyzing"}`))
			}))
			defer captureBackend.Close()

			token := signValidToken("ka-delegation-user")
			identity := &auth.UserIdentity{
				Username:  "ka-delegation-user",
				Issuer:    jwksServer.URL,
				RawToken:  token,
				ExpiresAt: time.Now().Add(1 * time.Hour),
			}

			client := ka.NewClient(ka.Config{
				BaseURL:            captureBackend.URL,
				Timeout:            5 * time.Second,
				CBFailureThreshold: 5,
			})

			ctx := auth.WithUserIdentity(context.Background(), identity)
			_, _ = client.Analyze(ctx, ka.AnalyzeRequest{
				Namespace: "test", Kind: "Deployment", Name: "test-app",
			})

			Expect(receivedAuth).To(Equal("Bearer " + token),
				"JWT delegation should forward user's raw token to KA")
		})
	})
})
