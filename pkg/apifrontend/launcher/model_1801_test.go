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

package launcher_test

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// generateFakeServiceAccountJSON builds a GCP service account credential
// blob with a real RSA-2048 key so credentials.DetectDefault/ADC discovery
// can parse and accept it without any live network call or dependency on
// the host's real ambient ADC state. Mirrors the identically-named test
// helpers in cmd/kubernautagent and pkg/kubernautagent/llm/geminifamily.
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

// InjectAmbientGoogleCredentials (#1801): the Claude-on-Vertex SDK path
// (adk-anthropic-go / anthropic-sdk-go's vertex.WithGoogleAuth) has no
// explicit-credentials-bytes option and can only discover credentials via
// ambient ADC (GOOGLE_APPLICATION_CREDENTIALS). Previously this env var was
// declared statically in the Helm Deployment manifest -- unlike every other
// credential in Kubernaut, which is either passed as explicit bytes (KA) or
// injected into the process environment at startup (HAPI's
// _inject_runtime_env()). This closes that gap for AF by moving the
// assignment in-process, right before each ADC-dependent construction call.
var _ = Describe("InjectAmbientGoogleCredentials (#1801)", func() {
	var origADC string
	var hadOrigADC bool

	BeforeEach(func() {
		origADC, hadOrigADC = os.LookupEnv("GOOGLE_APPLICATION_CREDENTIALS")
	})

	AfterEach(func() {
		if hadOrigADC {
			Expect(os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", origADC)).To(Succeed())
		} else {
			Expect(os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")).To(Succeed())
		}
	})

	It("UT-AF-1801-001: sets GOOGLE_APPLICATION_CREDENTIALS in-process to cfg.APIKeyFile when set", func() {
		path := filepath.Join(os.TempDir(), "fake-credentials.json")
		cfg := types.LLMConfig{APIKeyFile: path}

		Expect(launcher.InjectAmbientGoogleCredentials(cfg)).To(Succeed())
		Expect(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")).To(Equal(path))
	})

	It("UT-AF-1801-002: leaves any pre-existing ambient env untouched when cfg.APIKeyFile is empty", func() {
		Expect(os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/some/preexisting/path")).To(Succeed())
		cfg := types.LLMConfig{APIKeyFile: ""}

		Expect(launcher.InjectAmbientGoogleCredentials(cfg)).To(Succeed())
		Expect(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")).To(Equal("/some/preexisting/path"))
	})
})

// IT-AF-1801: end-to-end wiring proof through the real production dispatch
// (NewModelFromConfig -> newVertexAIModel -> newVertexAnthropicModel/
// newVertexGeminiModel, CHECKPOINT W) that both vertex_ai model families
// can construct successfully from an explicitly-supplied credentials file
// alone, with zero dependency on the test host's own ambient ADC state —
// proving the fix actually closes the gap rather than merely not
// regressing the ADC-present case (which IT-AF-1792-001/002 already cover
// via their soft-check pattern).
var _ = Describe("NewModelFromConfig — vertex_ai credential wiring without ambient ADC (#1801)", func() {
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
		Expect(os.WriteFile(credPath, generateFakeServiceAccountJSON(), 0o600)).To(Succeed())
	})

	AfterEach(func() {
		if hadOrigADC {
			Expect(os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", origADC)).To(Succeed())
		} else {
			Expect(os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")).To(Succeed())
		}
	})

	It("IT-AF-1801-001: vertex_ai + claude-* constructs via cfg.APIKeyFile alone, with no ambient ADC pre-set", func() {
		cfg := types.LLMConfig{
			Provider:       types.LLMProviderVertexAI,
			Model:          "claude-sonnet-4-20250514",
			VertexProject:  "test-project",
			VertexLocation: "us-central1",
			APIKeyFile:     credPath,
		}
		m, err := launcher.NewModelFromConfig(context.Background(), cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(m).NotTo(BeNil())

		// The construction call must have injected the env var as a
		// side effect (InjectAmbientGoogleCredentials), proving the
		// in-process runtime-injection path — not a static manifest
		// declaration — is what made this succeed.
		Expect(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")).To(Equal(credPath))
	})

	It("IT-AF-1801-002: vertex_ai + gemini-* constructs via cfg.APIKey explicit bytes alone, with no ambient ADC pre-set or env var touched", func() {
		cfg := types.LLMConfig{
			Provider:       types.LLMProviderVertexAI,
			Model:          "gemini-2.5-pro",
			VertexProject:  "test-project",
			VertexLocation: "us-central1",
			// Simulates AF's config loader resolving APIKey from
			// APIKeyFile's content (pkg/apifrontend/config's
			// resolveLLMKey) once the Helm chart renders apiKeyFile
			// for vertex_ai too (kubernaut#1801).
			APIKey: string(generateFakeServiceAccountJSON()),
		}
		m, err := launcher.NewModelFromConfig(context.Background(), cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(m).NotTo(BeNil())

		// Explicit-bytes auth must never touch the ambient env var —
		// this is the "zero env var" path, matching Kubernaut Agent.
		_, isSet := os.LookupEnv("GOOGLE_APPLICATION_CREDENTIALS")
		Expect(isSet).To(BeFalse())
	})
})
