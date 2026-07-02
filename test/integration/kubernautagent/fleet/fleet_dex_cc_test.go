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

package fleet_test

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

var _ = Describe("IT-FLEET-DEXCC-001 [AC-3]: IdP issues service identity tokens with role-bearing claims that distinguish read-only from write-capable services (BR-INTEGRATION-054)", Label("container"), func() {
	BeforeEach(func() {
		if os.Getenv("FLEET_IT_CONTAINERS") != "true" {
			Skip("FLEET_IT_CONTAINERS=true required for container-based IT tests (DEX)")
		}
	})

	It("obtains client_credentials token with groups claim for fleet-read", func() {
		cfg := infrastructure.DefaultDexFleetReadConfig()
		token, err := infrastructure.GetDexClientCredentialsToken(cfg)
		Expect(err).ToNot(HaveOccurred(), "DEX should issue client_credentials token for fleet-read client")
		Expect(token).ToNot(BeEmpty())

		claims := decodeJWTPayload(token)
		Expect(claims).To(HaveKey("groups"),
			"JWT must contain groups claim for gateway CEL authorization")

		groups, ok := claims["groups"].([]interface{})
		Expect(ok).To(BeTrue(), "groups claim must be an array")
		Expect(groups).To(ContainElement("mcp-read"),
			"fleet-read client must have mcp-read group for read-only access")
	})

	It("ROPC for existing kubernaut-agent client still works (backward compatibility)", func() {
		cfg := infrastructure.DefaultDexE2EConfig()
		token, err := infrastructure.GetDexIDToken(cfg)
		Expect(err).ToNot(HaveOccurred(), "ROPC grant must still work after client_credentials upgrade")
		Expect(token).ToNot(BeEmpty())
	})
})

func decodeJWTPayload(token string) map[string]interface{} {
	parts := strings.Split(token, ".")
	Expect(len(parts)).To(BeNumerically(">=", 2), "JWT must have at least header.payload")

	payload := parts[1]
	if mod := len(payload) % 4; mod != 0 {
		payload += strings.Repeat("=", 4-mod)
	}

	decoded, err := base64.URLEncoding.DecodeString(payload)
	Expect(err).ToNot(HaveOccurred(), "JWT payload must be valid base64url")

	var claims map[string]interface{}
	Expect(json.Unmarshal(decoded, &claims)).To(Succeed())
	return claims
}
