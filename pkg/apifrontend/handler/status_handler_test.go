package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
)

var _ = Describe("StatusHandler JSON-RPC parsing", func() {
	var h *handler.StatusHandler

	BeforeEach(func() {
		h = handler.NewStatusHandler(nil, "kubernaut-system", logr.Discard())
	})

	It("UT-AF-1460-001: valid status/subscribe request parses correctly", func() {
		body := map[string]any{
			"jsonrpc": "2.0",
			"id":      "status-1",
			"method":  "status/subscribe",
			"params":  map[string]any{"rr_id": "rr-0fd7ce7b49a7"},
		}
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/a2a/status", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusServiceUnavailable),
			"valid request with no client returns 503 (parsing succeeded)")
		Expect(rec.Code).NotTo(Equal(http.StatusBadRequest),
			"valid request must not return 400")
	})

	It("UT-AF-1460-002: missing rr_id returns JSON-RPC error -32602", func() {
		body := map[string]any{
			"jsonrpc": "2.0",
			"id":      "status-2",
			"method":  "status/subscribe",
			"params":  map[string]any{},
		}
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/a2a/status", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		var resp map[string]any
		Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())
		errObj, ok := resp["error"].(map[string]any)
		Expect(ok).To(BeTrue(), "response must contain error object")
		Expect(errObj["code"]).To(BeNumerically("==", -32602))
	})

	It("UT-AF-1460-003: wrong method returns JSON-RPC error -32601", func() {
		body := map[string]any{
			"jsonrpc": "2.0",
			"id":      "status-3",
			"method":  "status/unsubscribe",
			"params":  map[string]any{"rr_id": "rr-xxx"},
		}
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/a2a/status", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		var resp map[string]any
		Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())
		errObj, ok := resp["error"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(errObj["code"]).To(BeNumerically("==", -32601))
	})

	It("UT-AF-1460-004: malformed JSON returns JSON-RPC error -32600", func() {
		req := httptest.NewRequest(http.MethodPost, "/a2a/status", bytes.NewReader([]byte("{invalid")))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		var resp map[string]any
		Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())
		errObj, ok := resp["error"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(errObj["code"]).To(BeNumerically("==", -32600))
	})
})
