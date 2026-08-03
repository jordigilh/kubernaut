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

package severity_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
)

// generateFakeServiceAccountJSON builds a GCP service account credential
// blob with a real RSA-2048 key so google.CredentialsFromJSONWithType/ADC
// parsing can accept it without any live network call or dependency on the
// host's real ambient ADC state. Mirrors the identically-named helper in
// pkg/apifrontend/launcher/model_1801_test.go and this same file's
// counterpart on release/v1.5 (kubernaut#1870).
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

// kubernaut#1861 (main-line port of kubernaut#1731/#1870's release/v1.5
// fix): severity.NewAnthropicVertexClient never read a resolved profile's
// own credential bytes for severityTriage's independent Vertex connection
// -- it fell back to whichever credentials happened to be ambient
// (GOOGLE_APPLICATION_CREDENTIALS or the environment's default ADC),
// rather than authenticating independently of AF's own agent.llm
// connection. Mirrors the technique already proven for Kubernaut Agent's
// Vertex path, AF's own agent.llm Gemini-on-Vertex path (kubernaut#1801's
// newVertexGeminiModel), and this exact fix on release/v1.5 (#1870).
var _ = Describe("NewAnthropicVertexClient — per-profile credentials (#1861)", func() {
	var origADC, origHome string
	var hadOrigADC, hadOrigHome bool

	BeforeEach(func() {
		// Both GOOGLE_APPLICATION_CREDENTIALS and the well-known
		// ~/.config/gcloud/application_default_credentials.json file (which
		// google.FindDefaultCredentials/credentials.DetectDefault fall back
		// to via $HOME) must be blocked, or a developer machine with real
		// gcloud ADC configured would make the "explicit bytes alone"
		// assertions below pass for the wrong reason.
		origADC, hadOrigADC = os.LookupEnv("GOOGLE_APPLICATION_CREDENTIALS")
		Expect(os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")).To(Succeed())
		origHome, hadOrigHome = os.LookupEnv("HOME")
		Expect(os.Setenv("HOME", GinkgoT().TempDir())).To(Succeed())
	})

	AfterEach(func() {
		if hadOrigADC {
			Expect(os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", origADC)).To(Succeed())
		} else {
			Expect(os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")).To(Succeed())
		}
		if hadOrigHome {
			Expect(os.Setenv("HOME", origHome)).To(Succeed())
		} else {
			Expect(os.Unsetenv("HOME")).To(Succeed())
		}
	})

	It("UT-AF-1861-001: constructs from explicit credentials JSON bytes alone, with no ambient ADC", func() {
		client, err := severity.NewAnthropicVertexClient(context.Background(),
			"test-project", "us-central1", string(generateFakeServiceAccountJSON()))

		Expect(err).NotTo(HaveOccurred())
		Expect(client).NotTo(BeNil())
	})

	It("UT-AF-1861-002: rejects malformed credentials JSON with a clear error instead of silently falling back to ADC", func() {
		_, err := severity.NewAnthropicVertexClient(context.Background(),
			"test-project", "us-central1", "{not valid json")

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("credentials"))
	})

	It("UT-AF-1861-003: rejects an unsupported credential type (e.g. external_account) before ever touching ADC", func() {
		_, err := severity.NewAnthropicVertexClient(context.Background(),
			"test-project", "us-central1", `{"type":"external_account"}`)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("external_account"))
	})

	It("UT-AF-1861-004: still requires project before touching any credential resolution, explicit bytes or ADC", func() {
		_, err := severity.NewAnthropicVertexClient(context.Background(),
			"", "us-central1", string(generateFakeServiceAccountJSON()))

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("vertexProject"))
	})

	It("UT-AF-1861-005: falls back to ambient ADC unchanged when credentialsJSON is empty (backward compatibility)", func() {
		// No ambient ADC and no explicit bytes: must fail exactly the way
		// it always has (recovered panic from vertex.WithGoogleAuth), not
		// with some new/different error introduced by this fix.
		_, err := severity.NewAnthropicVertexClient(context.Background(),
			"test-project", "us-central1", "")

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("GCP ADC unavailable"))
	})
})
