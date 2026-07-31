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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// IT-AF-1792-003/004: newLLMTriagerFromConfig's provider: vertex_ai branch
// (#1778/#1792's shared model-family detector, third of its three call
// sites alongside KA's builder and AF's launcher) was refactored to call
// the promoted types.IsAnthropicModel instead of the former AF-only
// severity.IsAnthropicModel -- a pure call-site swap, zero intended
// behavior change. Prior to this test, that branch had no dedicated IT
// proving the dispatch itself (only indirect coverage of config
// resolution via TestTriageConfigResolution_*); this closes that gap by
// exercising the real production factory (CHECKPOINT W), following the
// same ADC-dependent soft-check pattern already established for AF's
// launcher (pkg/apifrontend/launcher/model_test.go's IT-AF-1792-001/002)
// since both newAnthropicTriagerForVertex/newGenAITriagerForVertex
// resolve live GCP credentials at construction time.
var _ = Describe("newLLMTriagerFromConfig — vertex_ai model-family dispatch (#1778/#1792)", func() {
	It("IT-AF-1792-003: routes vertex_ai + a gemini-* model to *severity.GenAITriager, not AnthropicTriager", func() {
		cfg := types.LLMConfig{
			Provider:       types.LLMProviderVertexAI,
			Model:          "gemini-2.5-pro",
			VertexProject:  "test-project",
			VertexLocation: "us-central1",
		}

		triager, err := newLLMTriagerFromConfig(context.Background(), cfg, logr.Discard())
		if err != nil {
			Expect(err.Error()).To(Or(ContainSubstring("credentials"), ContainSubstring("ADC")))
			return
		}
		Expect(triager).NotTo(BeNil())
		Expect(triager).To(BeAssignableToTypeOf(&severity.GenAITriager{}),
			"vertex_ai + a gemini-* model must route to GenAITriager, not AnthropicTriager")
	})

	It("IT-AF-1792-004: still routes vertex_ai + a claude-* model to *severity.AnthropicTriager (no regression)", func() {
		cfg := types.LLMConfig{
			Provider:       types.LLMProviderVertexAI,
			Model:          "claude-sonnet-4-6",
			VertexProject:  "test-project",
			VertexLocation: "us-central1",
		}

		triager, err := newLLMTriagerFromConfig(context.Background(), cfg, logr.Discard())
		if err != nil {
			Expect(err.Error()).To(Or(ContainSubstring("credentials"), ContainSubstring("ADC")))
			return
		}
		Expect(triager).NotTo(BeNil())
		Expect(triager).To(BeAssignableToTypeOf(&severity.AnthropicTriager{}),
			"vertex_ai + a claude-* model must remain routed to AnthropicTriager")
	})
})
