package tools_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/ka"
	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

var _ = Describe("kubernaut_start_investigation", func() {
	var (
		ctx    context.Context
		server *httptest.Server
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	It("UT-AF-111-001: calls KA POST /analyze and returns session_id", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "sess-abc"})
		}))
		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleStartInvestigation(ctx, kaClient, tools.StartInvestigationArgs{
			Namespace: "payments", Name: "rr-1",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.SessionID).To(Equal("sess-abc"))
	})

	It("UT-AF-111-002: populates user-friendly response", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "sess-abc"})
		}))
		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleStartInvestigation(ctx, kaClient, tools.StartInvestigationArgs{
			Namespace: "payments", Name: "rr-1",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Message).NotTo(BeEmpty())
		Expect(result.Status).To(Equal("started"))
	})

	It("UT-AF-111-003: returns error when KA returns 403", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		_, err := tools.HandleStartInvestigation(ctx, kaClient, tools.StartInvestigationArgs{
			Namespace: "forbidden", Name: "rr-1",
		})
		Expect(err).To(HaveOccurred())
	})

	It("UT-AF-111-004: returns error when KA is unavailable", func() {
		kaClient := ka.NewClient(ka.Config{BaseURL: "http://127.0.0.1:1"})
		_, err := tools.HandleStartInvestigation(ctx, kaClient, tools.StartInvestigationArgs{
			Namespace: "payments", Name: "rr-1",
		})
		Expect(err).To(HaveOccurred())
	})
})
