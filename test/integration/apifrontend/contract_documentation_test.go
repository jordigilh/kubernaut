package apifrontend_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

func repoRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}

func readContractDocument(relativePath string) string {
	root := repoRoot()
	Expect(root).NotTo(BeEmpty())
	content, err := os.ReadFile(filepath.Join(root, relativePath))
	Expect(err).NotTo(HaveOccurred())
	return string(content)
}

var _ = Describe("API Frontend backend-incarnation documentation (#2614)", func() {
	It("IT-AF-2614-012: documents the exact OpenAPI wire and response matrix", func() {
		content := readContractDocument("api/apifrontend/openapi/apifrontend-v1.yaml")
		var document map[string]any
		Expect(yaml.Unmarshal([]byte(content), &document)).To(Succeed())

		paths, ok := document["paths"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(paths).To(HaveKey("/.well-known/kubernaut-console-contract.json"))
		Expect(paths).To(HaveKey("/a2a/status"))
		Expect(content).To(ContainSubstring("Cache-Control"))
		Expect(content).To(ContainSubstring("additionalProperties: false"))
		Expect(content).To(ContainSubstring("backend_incarnation"))
		Expect(content).To(ContainSubstring("X-Kubernaut-Backend-Incarnation"))
		Expect(content).To(ContainSubstring("pre-authentication IP limiter"))
		Expect(content).To(ContainSubstring("post-authentication rate limiting"))
		Expect(content).To(ContainSubstring("invalid_request"))
		Expect(content).NotTo(ContainSubstring("413"), "A2A/root must not advertise a generic 413 contract")
		for _, path := range []string{"/a2a/invoke", "/", "/mcp", "/a2a/status"} {
			Expect(content).To(ContainSubstring(path))
		}
	})

	It("IT-AF-2614-012: documents route boundaries and Console qualification prerequisites", func() {
		architecture := readContractDocument("docs/services/apifrontend/design/ARCHITECTURE.md")
		boundary := readContractDocument("docs/services/apifrontend/security/SERVICE_BOUNDARY.md")
		authentication := readContractDocument("docs/services/apifrontend/security/AUTHENTICATION_AND_RBAC.md")
		contract := readContractDocument("docs/services/apifrontend/api/BACKEND_INCARNATION_CONTRACT.md")

		for _, content := range []string{architecture, boundary, authentication, contract} {
			Expect(content).To(ContainSubstring("/.well-known/kubernaut-console-contract.json"))
			Expect(content).To(ContainSubstring("X-Kubernaut-Backend-Incarnation"))
			Expect(content).To(ContainSubstring("#2606"))
		}
		Expect(architecture).To(ContainSubstring("single AF replica"))
		Expect(architecture).To(ContainSubstring("OAuth2 Proxy"))
		Expect(boundary).To(ContainSubstring("POST /a2a/status"))
		Expect(authentication).To(ContainSubstring("public health, readiness, metrics, agent-card"))
		Expect(authentication).To(ContainSubstring("no service-wide 413 promise"))
		Expect(contract).To(ContainSubstring("## Wire contract"))
		Expect(contract).To(ContainSubstring("## Console #168 rollout and evidence"))
		Expect(contract).To(ContainSubstring("retryable and must not log the user out"))
		Expect(contract).To(ContainSubstring("never automatically replayed"))
		Expect(strings.Count(contract, "backend_incarnation")).To(BeNumerically(">", 0))
	})
})
