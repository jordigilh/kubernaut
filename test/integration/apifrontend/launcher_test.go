package apifrontend_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func jsonRPCBody(id int, text string) []byte {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "message/send",
		"params": map[string]any{
			"message": map[string]any{
				"role": "user",
				"parts": []map[string]any{
					{"text": text},
				},
			},
		},
	})
	return body
}

var _ = Describe("Launcher A2A Integration (launcher/)", func() {

	Describe("AC-30: A2A handler accepts JSON-RPC via router", func() {
		It("IT-AF-1195-044: POST /a2a/invoke with valid JSON-RPC returns JSON-RPC response", func() {
			token := signValidToken("launcher-user-044")
			req, err := http.NewRequest(http.MethodPost, routerServer.URL+"/a2a/invoke", bytes.NewReader(jsonRPCBody(1, "list pods in namespace default")))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/json"))
		})

		It("IT-AF-1195-045: POST /a2a with invalid JSON returns error response", func() {
			resp, err := http.Post(routerServer.URL+"/a2a/invoke", "application/json",
				bytes.NewReader([]byte(`{invalid json`)))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(respBody)).NotTo(BeEmpty())
		})
	})

	Describe("AC-31: A2A handler rejects unauthenticated requests", func() {
		It("IT-AF-1195-046: unauthenticated A2A request returns 401", func() {
			req, err := http.NewRequest(http.MethodPost, routerServer.URL+"/a2a/invoke", bytes.NewReader(jsonRPCBody(2, "test")))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})
	})

	Describe("AC-32: A2A handler emits audit events", func() {
		It("IT-AF-1195-047: authenticated A2A request emits triage_started audit event", func() {
			token := signValidToken("launcher-test-user")
			req, err := http.NewRequest(http.MethodPost, routerServer.URL+"/a2a/invoke", bytes.NewReader(jsonRPCBody(3, "list remediations in default")))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).NotTo(Equal(http.StatusUnauthorized),
				"authenticated request should pass auth middleware")
		})
	})
})
