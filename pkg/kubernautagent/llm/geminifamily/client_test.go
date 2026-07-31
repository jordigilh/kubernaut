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

package geminifamily_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"google.golang.org/genai"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/geminifamily"
)

// generateFakeServiceAccountJSON builds a GCP service account credential
// blob with a real RSA-2048 key so credentials.DetectDefault can parse and
// accept it without any live network call. Mirrors
// anthropicfamily's identically-named test helper exactly, so both clients'
// "ambient ADC" constructor paths are exercised the same deterministic way.
func generateFakeServiceAccountJSON() []byte {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("generate RSA key for test credentials: %v", err))
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	creds := map[string]string{
		"type":           "service_account",
		"project_id":     "test-project",
		"private_key_id": "key123",
		"private_key":    string(keyPEM), // notsecret — generated at runtime via rsa.GenerateKey
		"client_email":   "test@test-project.iam.gserviceaccount.com",
		"client_id":      "123456789",
		"auth_uri":       "https://accounts.google.com/o/oauth2/auth",
		"token_uri":      "https://oauth2.googleapis.com/token",
	}
	b, _ := json.Marshal(creds)
	return b
}

var _ = Describe("geminifamily.New (Vertex AI) constructor validation — #1778 BR-AI-087", func() {
	// UT-GM-1778-101/102 pass nil credentialsJSON to exercise the "ambient
	// ADC" path (credentials.DetectDefault falling back to
	// GOOGLE_APPLICATION_CREDENTIALS/well-known locations). Without
	// pinning that env var to a fake-but-well-formed credential file, these
	// specs would pass or fail based on whatever GCP ADC state happens to
	// exist on the machine running the tests (developer laptops with
	// `gcloud auth application-default login` vs. a clean CI runner) — a
	// non-deterministic dependency on host state, found via CI failure.
	var origADC string

	BeforeEach(func() {
		origADC = os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
		adcPath := filepath.Join(GinkgoT().TempDir(), "adc.json")
		Expect(os.WriteFile(adcPath, generateFakeServiceAccountJSON(), 0600)).To(Succeed())
		Expect(os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", adcPath)).To(Succeed())
	})

	AfterEach(func() {
		if origADC != "" {
			Expect(os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", origADC)).To(Succeed())
		} else {
			Expect(os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")).To(Succeed())
		}
	})

	It("UT-GM-1778-100: returns error when project is empty", func() {
		client, err := geminifamily.New(context.Background(), "gemini-2.5-pro", nil, "", "us-central1")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("project"))
		Expect(client).To(BeNil())
	})

	It("UT-GM-1778-101: defaults location to us-central1 when empty", func() {
		client, err := geminifamily.New(context.Background(), "gemini-2.5-pro", nil, "my-project", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(client).NotTo(BeNil())
	})

	It("UT-GM-1778-102: implements llm.Client interface", func() {
		client, err := geminifamily.New(context.Background(), "gemini-2.5-pro", nil, "my-project", "us-central1")
		Expect(err).NotTo(HaveOccurred())
		var _ llm.Client = client
	})
})

var _ = Describe("geminifamily.NewWithAPIKey constructor validation — #1778 BR-AI-087", func() {
	It("UT-GM-1778-110: returns error when apiKey is empty", func() {
		client, err := geminifamily.NewWithAPIKey("", "gemini-2.5-pro")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("apiKey"))
		Expect(client).To(BeNil())
	})

	It("UT-GM-1778-111: implements llm.Client interface", func() {
		client, err := geminifamily.NewWithAPIKey("fake-api-key", "gemini-2.5-pro")
		Expect(err).NotTo(HaveOccurred())
		var _ llm.Client = client
	})
})

var _ = Describe("geminifamily.Client Chat/StreamChat — #1778 BR-AI-087", func() {
	var (
		server     *httptest.Server
		client     *geminifamily.Client
		makeClient func(http.HandlerFunc)
	)

	// makeClient spins up an httptest server standing in for the native
	// Gemini API and points the client at it via WithHTTPOptions (test-only
	// escape hatch — see WithHTTPOptions doc comment).
	makeClient = func(handler http.HandlerFunc) {
		server = httptest.NewServer(handler)
		var err error
		client, err = geminifamily.NewWithAPIKey("fake-api-key", "gemini-2.5-pro",
			geminifamily.WithHTTPOptions(genai.HTTPOptions{BaseURL: server.URL}),
		)
		Expect(err).NotTo(HaveOccurred())
	}

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	It("UT-GM-1778-201: maps a simple text response", func() {
		makeClient(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"candidates": [{
					"content": {"role": "model", "parts": [{"text": "The pod is OOMKilled."}]},
					"finishReason": "STOP"
				}],
				"usageMetadata": {"promptTokenCount": 50, "candidatesTokenCount": 10, "totalTokenCount": 60}
			}`))
		})

		resp, err := client.Chat(context.Background(), llm.ChatRequest{
			Messages: []llm.Message{
				{Role: "system", Content: "You are a Kubernetes investigator."},
				{Role: "user", Content: "Why is the pod crashing?"},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Message.Role).To(Equal("assistant"))
		Expect(resp.Message.Content).To(Equal("The pod is OOMKilled."))
		Expect(resp.Usage.PromptTokens).To(Equal(50))
		Expect(resp.Usage.CompletionTokens).To(Equal(10))
		Expect(resp.Usage.TotalTokens).To(Equal(60))
		Expect(resp.FinishReason).To(Equal(llm.FinishReasonStop))
	})

	It("UT-GM-1778-202: maps a tool-call response", func() {
		makeClient(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"candidates": [{
					"content": {"role": "model", "parts": [{"functionCall": {"name": "kubectl_describe", "args": {"kind": "Pod"}}}]},
					"finishReason": "STOP"
				}],
				"usageMetadata": {"promptTokenCount": 100, "candidatesTokenCount": 30, "totalTokenCount": 130}
			}`))
		})

		resp, err := client.Chat(context.Background(), llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "Describe the crashing pod"}},
			Tools: []llm.ToolDefinition{
				{Name: "kubectl_describe", Description: "Describe a Kubernetes resource", Parameters: json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"}}}`)},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.ToolCalls).To(HaveLen(1))
		Expect(resp.ToolCalls[0].Name).To(Equal("kubectl_describe"))
		Expect(resp.ToolCalls[0].Arguments).To(ContainSubstring("Pod"))
		Expect(resp.Message.ToolCalls).To(HaveLen(1))
		Expect(resp.FinishReason).To(Equal(llm.FinishReasonToolCalls))
	})

	It("UT-GM-1778-203: classifies a 400 error as non-retryable", func() {
		makeClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": {"code": 400, "message": "invalid argument", "status": "INVALID_ARGUMENT"}}`))
		})

		_, err := client.Chat(context.Background(), llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "hello"}},
		})
		Expect(err).To(HaveOccurred())
		Expect(llm.IsRetryable(err)).To(BeFalse(), "a 400 Gemini API error must be classified non-retryable, mirroring anthropicfamily's #1585 classification")
	})

	It("UT-GM-1778-204: a 503 error remains retryable", func() {
		makeClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error": {"code": 503, "message": "overloaded", "status": "UNAVAILABLE"}}`))
		})

		_, err := client.Chat(context.Background(), llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "hello"}},
		})
		Expect(err).To(HaveOccurred())
		Expect(llm.IsRetryable(err)).To(BeTrue())
	})

	It("UT-GM-1778-205: StreamChat delivers incremental text deltas and a final aggregated response", func() {
		makeClient(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			chunks := []string{
				`{"candidates":[{"content":{"role":"model","parts":[{"text":"The pod "}]}}]}`,
				`{"candidates":[{"content":{"role":"model","parts":[{"text":"is OOMKilled."}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`,
			}
			for _, c := range chunks {
				_, _ = fmt.Fprintf(w, "data: %s\n\n", c)
				if flusher != nil {
					flusher.Flush()
				}
			}
		})

		var deltas []string
		resp, err := client.StreamChat(context.Background(), llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "Why is the pod crashing?"}},
		}, func(ev llm.ChatStreamEvent) error {
			if ev.Delta != "" {
				deltas = append(deltas, ev.Delta)
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(deltas).To(ContainElement("The pod "))
		Expect(resp.Message.Content).To(Equal("The pod is OOMKilled."))
		Expect(resp.Usage.TotalTokens).To(Equal(15))
	})

	It("UT-GM-1778-206: Close is a no-op that never errors", func() {
		makeClient(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{}`)) //nolint:errcheck // test stub server, unused in this case
		})
		Expect(client.Close()).To(Succeed())
	})
})
