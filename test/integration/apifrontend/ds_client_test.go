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
		It("IT-AF-1195-028: ListWorkflows queries real DS", func() {
			client, err := newAuthenticatedDSClient()
			Expect(err).NotTo(HaveOccurred())

			workflows, err := client.ListWorkflows(context.Background(), ds.ListWorkflowsOpts{})
			Expect(err).NotTo(HaveOccurred())
			Expect(workflows).NotTo(BeNil())
		})

		It("IT-AF-1195-029: GetRemediationHistory queries real DS", func() {
			client, err := newAuthenticatedDSClient()
			Expect(err).NotTo(HaveOccurred())

			history, err := client.GetRemediationHistory(context.Background(), ds.HistoryOpts{
				Namespace: "default",
				Kind:      "Deployment",
				Name:      "test-app",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(history).NotTo(BeNil())
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

			_, err = client.ListWorkflows(context.Background(), ds.ListWorkflowsOpts{})
			Expect(err).To(HaveOccurred(), "DS errors should be returned, not panicked")
		})
	})
})
