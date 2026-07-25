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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Phase/alignment-override apiKeyFile resolution — #1726, BR-SECURITY-1726.
//
// Root cause: LLMRuntimeConfig.EffectivePhaseConfig and
// AlignmentCheckConfig.EffectiveLLM both copy the base profile's
// already-resolved APIKey into the override's output struct and only ever
// overwrite APIKeyFile, so a phase/alignment override's own apiKeyFile is
// never actually read — every phase/shadow client silently authenticates
// with the base profile's credentials. These tests prove (and, once fixed,
// prove the fix for) the 3 production call sites that build a phase/shadow
// LLM client: buildLLMClients, buildAlignmentStack (bootstrap.go), and
// reloadSinglePhaseClient (llm_builder.go, hot-reload path).
//
// writeTempKeyFile writes content to a fresh temp file under t.TempDir()
// and returns its path — a minimal fixture for apiKeyFile-bearing configs.
// Accepts testing.TB (not *testing.T) so both plain `testing.T`-based tests
// and Ginkgo specs (via GinkgoTB()) can share it, matching the
// generateTestCACert convention in testutil_ca_test.go.
func writeTempKeyFile(t testing.TB, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "apikey")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

var _ = Describe("Phase/alignment-override apiKeyFile resolution — wire-level wiring proofs (#1726)", func() {

	Describe("IT-KA-1726-001 (IA-5): buildLLMClients resolves a phase override's own apiKeyFile", func() {
		var (
			server       *httptest.Server
			receivedAuth string
		)

		BeforeEach(func() {
			receivedAuth = ""
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedAuth = r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
			}))
		})

		AfterEach(func() {
			server.Close()
		})

		It("authenticates the rca phase client with its own key, distinct from the base's", func() {
			baseKeyFile := writeTempKeyFile(GinkgoTB(), "base-secret-key")
			phaseKeyFile := writeTempKeyFile(GinkgoTB(), "phase-secret-key")

			cfg := kaconfig.DefaultConfig()
			cfg.AI.LLM.Provider = types.LLMProviderOpenAICompatible

			llmRuntime := &kaconfig.LLMRuntimeConfig{
				Model:      "gpt-4",
				Endpoint:   server.URL,
				APIKeyFile: baseKeyFile,
				APIKey:     "base-secret-key", // simulates resolveLLMCredentials having already run at boot
				PhaseModels: map[string]*kaconfig.LLMOverrideConfig{
					"rca": {APIKeyFile: phaseKeyFile},
				},
			}

			_, phaseSwappables := buildLLMClients(cfg, llmRuntime, testReloadLogger())
			rcaClient := phaseSwappables[katypes.PhaseRCA]
			Expect(rcaClient).NotTo(BeNil())

			_, err := rcaClient.Chat(context.Background(), helloChatRequest())
			Expect(err).NotTo(HaveOccurred())

			Expect(receivedAuth).To(Equal("Bearer phase-secret-key"),
				"phaseModels.rca.apiKeyFile must be resolved into its own APIKey, not inherit the base profile's")
		})

		// IT-KA-1726-004 (CM-6, regression): a phase override that does not
		// set its own apiKeyFile must keep inheriting the base profile's
		// resolved key, unchanged — the only configuration shape supported
		// in production today, and the one every existing deployment relies
		// on.
		It("still authenticates an unoverridden phase client with the base's key (no regression)", func() {
			baseKeyFile := writeTempKeyFile(GinkgoTB(), "base-secret-key")

			cfg := kaconfig.DefaultConfig()
			cfg.AI.LLM.Provider = types.LLMProviderOpenAICompatible

			llmRuntime := &kaconfig.LLMRuntimeConfig{
				Model:      "gpt-4",
				Endpoint:   server.URL,
				APIKeyFile: baseKeyFile,
				APIKey:     "base-secret-key",
				PhaseModels: map[string]*kaconfig.LLMOverrideConfig{
					// Tunes only the endpoint; no apiKeyFile override.
					"rca": {Endpoint: server.URL},
				},
			}

			_, phaseSwappables := buildLLMClients(cfg, llmRuntime, testReloadLogger())
			rcaClient := phaseSwappables[katypes.PhaseRCA]
			Expect(rcaClient).NotTo(BeNil())

			_, err := rcaClient.Chat(context.Background(), helloChatRequest())
			Expect(err).NotTo(HaveOccurred())

			Expect(receivedAuth).To(Equal("Bearer base-secret-key"),
				"a phase override with no apiKeyFile must keep inheriting the base's resolved key")
		})
	})

	Describe("IT-KA-1726-002 (IA-5): reloadSinglePhaseClient (hot-reload) resolves a phase override's own apiKeyFile", func() {
		var (
			server       *httptest.Server
			receivedAuth string
		)

		BeforeEach(func() {
			receivedAuth = ""
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedAuth = r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
			}))
		})

		AfterEach(func() {
			server.Close()
		})

		It("authenticates the reloaded rca phase client with its own key, distinct from the base's", func() {
			baseKeyFile := writeTempKeyFile(GinkgoTB(), "base-secret-key")
			phaseKeyFile := writeTempKeyFile(GinkgoTB(), "phase-secret-key")

			sc, resolver := setupPhaseResolver(GinkgoTB())
			bootRuntime := bootRuntimeFor()
			cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), resolver, bootRuntime)

			err := cb(`model: gpt-4
endpoint: ` + server.URL + `
apiKeyFile: ` + baseKeyFile + `
phaseModels:
  rca:
    apiKeyFile: ` + phaseKeyFile + `
`)
			Expect(err).NotTo(HaveOccurred())

			rcaClient, _, _ := resolver.ResolvePhase(katypes.PhaseRCA)
			Expect(rcaClient).NotTo(BeNil())

			_, chatErr := rcaClient.Chat(context.Background(), helloChatRequest())
			Expect(chatErr).NotTo(HaveOccurred())

			Expect(receivedAuth).To(Equal("Bearer phase-secret-key"),
				"a hot-reloaded phaseModels.rca.apiKeyFile must be resolved into its own APIKey, not inherit the base's")
		})

		// IT-KA-1726-005 (SI-10): a phase's own apiKeyFile that is unreadable
		// must reject that phase's client build (loudly logged), leaving no
		// phase client registered — never falling back to the base's key or
		// the credentials-dir scan.
		It("rejects registering a new phase client when its own apiKeyFile is unreadable", func() {
			baseKeyFile := writeTempKeyFile(GinkgoTB(), "base-secret-key")
			unreadableKeyFile := filepath.Join(GinkgoTB().TempDir(), "does-not-exist")

			sc, resolver := setupPhaseResolver(GinkgoTB())
			bootRuntime := bootRuntimeFor()
			cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), resolver, bootRuntime)

			err := cb(`model: gpt-4
endpoint: ` + server.URL + `
apiKeyFile: ` + baseKeyFile + `
phaseModels:
  rca:
    apiKeyFile: ` + unreadableKeyFile + `
`)
			// The overall reload succeeds (base identity/model unchanged);
			// per-phase build failures are logged, not propagated — matching
			// the existing reloadPhaseClients contract for any other
			// per-phase build failure (e.g. a bad model).
			Expect(err).NotTo(HaveOccurred())

			Expect(resolver.PhaseSwappable(katypes.PhaseRCA)).To(BeNil(),
				"a phase whose own apiKeyFile is unreadable must not be registered")
			fallbackClient, fallbackModel, _ := resolver.ResolvePhase(katypes.PhaseRCA)
			Expect(fallbackModel).To(Equal("gpt-4"), "unregistered phase falls back to the base model")
			Expect(fallbackClient).NotTo(BeNil())
		})
	})

	Describe("IT-KA-1726-003 (IA-5): buildAlignmentStack resolves the shadow client's own apiKeyFile", func() {
		var (
			server       *httptest.Server
			receivedAuth string
		)

		BeforeEach(func() {
			receivedAuth = ""
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var parsed map[string]interface{}
				_ = json.Unmarshal(body, &parsed)
				receivedAuth = r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"{\"suspicious\":false}"},"finish_reason":"stop"}]}`))
			}))
		})

		AfterEach(func() {
			server.Close()
		})

		It("sends the shadow override's own apiKeyFile-resolved key on the wire, distinct from the base's", func() {
			baseKeyFile := writeTempKeyFile(GinkgoTB(), "base-secret-key")
			shadowKeyFile := writeTempKeyFile(GinkgoTB(), "shadow-secret-key")

			cfg := kaconfig.DefaultConfig()
			cfg.AI.LLM.Provider = types.LLMProviderOpenAICompatible
			cfg.AI.AlignmentCheck = kaconfig.AlignmentCheckConfig{
				Enabled:        true,
				Mode:           kaconfig.AlignmentModeEnforce,
				Timeout:        5 * time.Second,
				MaxStepTokens:  500,
				MaxRetries:     1,
				VerdictTimeout: 5 * time.Second,
				LLM: &kaconfig.LLMOverrideConfig{
					APIKeyFile: shadowKeyFile,
				},
			}

			llmRuntime := &kaconfig.LLMRuntimeConfig{
				Model:      "gpt-4",
				Endpoint:   server.URL,
				APIKeyFile: baseKeyFile,
				APIKey:     "base-secret-key",
			}

			_, _, alignEvaluator, alignCfg := buildAlignmentStack(
				cfg, llmRuntime, &stubLLMClient{}, registry.New(), audit.NopAuditStore{}, testReloadLogger())
			Expect(alignCfg.Enabled).To(BeTrue())
			Expect(alignEvaluator).NotTo(BeNil())

			_ = alignEvaluator.EvaluateStep(context.Background(), alignment.Step{
				Index:   0,
				Kind:    alignment.StepKindToolResult,
				Content: "hello",
			})

			Expect(receivedAuth).To(Equal("Bearer shadow-secret-key"),
				"ai.alignmentCheck.llm.apiKeyFile must be resolved into its own APIKey, not inherit the base profile's")
		})
	})
})
