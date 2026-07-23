package apifrontend_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ds"
)

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

func newAuthenticatedDSClient() (ds.Client, error) {
	return ds.NewOgenClient(ds.OgenClientConfig{
		BaseURL:   "http://127.0.0.1:18096",
		Timeout:   10 * time.Second,
		Transport: &bearerTransport{token: serviceAccountToken},
	})
}

var _ = Describe("DS Client Integration (ds/)", func() {

	Describe("AC-19: DS client queries real DataStorage", func() {
		// #1677 Phase 2g (DD-WORKFLOW-019): ListWorkflows was removed from
		// ds.Client -- DS's GET /api/v1/workflows was retired in favor of
		// KubernautAgent's own workflow catalog (ka.MCPClient.ListWorkflows).
		// GetAuditTrail replaces it here as the "real DS round trip" probe;
		// it succeeds (empty result) for a correlation ID with no events.
		It("IT-AF-1195-028: GetAuditTrail queries real DS", func() {
			client, err := newAuthenticatedDSClient()
			Expect(err).NotTo(HaveOccurred())

			events, err := client.GetAuditTrail(context.Background(), ds.AuditTrailOpts{RRID: "rr-nonexistent-1195-028"})
			Expect(err).NotTo(HaveOccurred())
			Expect(events).NotTo(BeNil())
		})

		It("IT-AF-1195-029: GetRemediationHistory returns structured error for missing required param", func() {
			client, err := newAuthenticatedDSClient()
			Expect(err).NotTo(HaveOccurred())

			_, err = client.GetRemediationHistory(context.Background(), ds.HistoryOpts{
				Namespace: defaultFixture,
				Kind:      "Deployment",
				Name:      "test-app",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("currentSpecHash"),
				"DS should return 400 with missing currentSpecHash (HistoryOpts does not expose this field yet)")
		})
	})

	Describe("AC-20: Error responses handled gracefully", func() {
		It("IT-AF-1195-030: error from DS is wrapped, no panic", func() {
			errorBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"internal server error"}`))
			}))
			defer errorBackend.Close()

			client, err := ds.NewOgenClient(ds.OgenClientConfig{
				BaseURL: errorBackend.URL,
				Timeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = client.GetAuditTrail(context.Background(), ds.AuditTrailOpts{RRID: "rr-error-1195-030"})
			Expect(err).To(HaveOccurred(), "DS errors should be returned, not panicked")
		})
	})
})
