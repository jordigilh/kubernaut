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

package llm_test

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo"
)

var _ = Describe("Vertex AI + Claude Adapter — #684", func() {

	Describe("Bug 2: Provider alias recognition", func() {

		It("UT-KA-684-101: vertex_ai is accepted as a valid provider", func() {
			adapter, err := langchaingo.New("vertex_ai", "http://localhost:9999", "claude-sonnet-4-6", "",
				langchaingo.WithVertexProject("my-project"),
				langchaingo.WithVertexLocation("us-central1"),
				langchaingo.WithHTTPClient(&http.Client{}),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(adapter).NotTo(BeNil())
			var _ llm.Client = adapter
		})

		It("UT-KA-684-102: existing vertex (Gemini) provider still works with credentials JSON", func() {
			_, thisFile, _, _ := runtime.Caller(0)
			fixturesDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "fixtures")
			credPath := filepath.Join(fixturesDir, "gcp-mock-credentials.json")
			Expect(credPath).To(BeAnExistingFile())

			credJSON, err := os.ReadFile(credPath)
			Expect(err).NotTo(HaveOccurred())

			adapter, adapterErr := langchaingo.New("vertex", "", "gemini-1.5-pro", string(credJSON),
				langchaingo.WithVertexProject("my-project"),
				langchaingo.WithVertexLocation("us-central1"),
			)
			Expect(adapterErr).NotTo(HaveOccurred())
			Expect(adapter).NotTo(BeNil())
		})

		It("UT-KA-684-105: vertex_ai without project returns descriptive error", func() {
			adapter, err := langchaingo.New("vertex_ai", "", "claude-sonnet-4-6", "",
				langchaingo.WithHTTPClient(&http.Client{}),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("project"))
			Expect(adapter).To(BeNil())
		})
	})
})
