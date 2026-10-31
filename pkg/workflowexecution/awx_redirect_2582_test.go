package workflowexecution_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

var _ = Describe("AWX redirect credential boundary (DD-2582)", func() {
	It("UT-WE-2582-011: rejects cross-origin and scheme-changing redirects before forwarding the bearer token", func() {
		cases := []func(baseURL, evilURL string) string{
			func(_, evilURL string) string { return evilURL + "/api/v2/jobs/?page=2" },
			func(baseURL, _ string) string {
				return strings.Replace(baseURL, "http://", "https://", 1) + "/api/v2/jobs/?page=2"
			},
		}

		for _, nextURL := range cases {
			var evilHits int
			evil := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				evilHits++
			}))
			mux := http.NewServeMux()
			var baseURL string
			mux.HandleFunc("/api/v2/jobs/", func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, nextURL(baseURL, evil.URL), http.StatusFound)
			})
			base := httptest.NewServer(mux)
			baseURL = base.URL

			client, err := executor.NewAWXHTTPClient(base.URL, "test-token")
			Expect(err).NotTo(HaveOccurred())
			_, err = client.FindJobByExecutionIdentity(context.Background(), "we-partial", "default", "wfe-uid")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reject AWX redirect outside configured origin"))
			Expect(evilHits).To(Equal(0))

			base.Close()
			evil.Close()
		}
	})
})
