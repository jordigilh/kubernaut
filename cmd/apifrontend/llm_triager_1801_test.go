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

package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// generateFakeServiceAccountJSON1801 builds a GCP service account
// credential blob with a real RSA-2048 key so ADC discovery can parse and
// accept it without any live network call or dependency on the test host's
// real ambient ADC state. Suffixed to avoid colliding with any
// package-local helper of the same conceptual purpose in other _test.go
// files in this package.
func generateFakeServiceAccountJSON1801() []byte {
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

// IT-AF-1801: proves newLLMTriagerFromConfig's vertex_ai branches
// (newAnthropicTriagerForVertex, newGenAITriagerForVertex) construct
// successfully from an explicitly-supplied credentials file alone, with
// zero dependency on the test host's own ambient ADC state — the
// severityTriage-side counterpart of pkg/apifrontend/launcher's
// IT-AF-1801-001/002 (main agent LLM side). Proves
// launcher.InjectAmbientGoogleCredentials is actually wired into both
// triager constructors (CHECKPOINT W), not just present as dead code.
var _ = Describe("newLLMTriagerFromConfig — vertex_ai credential wiring without ambient ADC (#1801)", func() {
	var (
		credPath   string
		origADC    string
		hadOrigADC bool
	)

	BeforeEach(func() {
		origADC, hadOrigADC = os.LookupEnv("GOOGLE_APPLICATION_CREDENTIALS")
		Expect(os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")).To(Succeed())

		dir := GinkgoT().TempDir()
		credPath = filepath.Join(dir, "credentials.json")
		Expect(os.WriteFile(credPath, generateFakeServiceAccountJSON1801(), 0o600)).To(Succeed())
	})

	AfterEach(func() {
		if hadOrigADC {
			Expect(os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", origADC)).To(Succeed())
		} else {
			Expect(os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")).To(Succeed())
		}
	})

	It("IT-AF-1801-003: vertex_ai + claude-* triager constructs via cfg.APIKeyFile alone, with no ambient ADC pre-set", func() {
		cfg := types.LLMConfig{
			Provider:       types.LLMProviderVertexAI,
			Model:          "claude-sonnet-4-20250514",
			VertexProject:  "test-project",
			VertexLocation: "us-central1",
			APIKeyFile:     credPath,
		}
		triager, err := newLLMTriagerFromConfig(context.Background(), cfg, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(triager).To(BeAssignableToTypeOf(&severity.AnthropicTriager{}))
		Expect(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")).To(Equal(credPath))
	})

	It("IT-AF-1801-004: vertex_ai + gemini-* triager constructs via cfg.APIKeyFile alone, with no ambient ADC pre-set", func() {
		cfg := types.LLMConfig{
			Provider:       types.LLMProviderVertexAI,
			Model:          "gemini-2.5-pro",
			VertexProject:  "test-project",
			VertexLocation: "us-central1",
			APIKeyFile:     credPath,
		}
		triager, err := newLLMTriagerFromConfig(context.Background(), cfg, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(triager).To(BeAssignableToTypeOf(&severity.GenAITriager{}))
		Expect(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")).To(Equal(credPath))
	})
})
