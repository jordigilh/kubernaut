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

package infrastructure

import (
	"context"
	"io"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"
)

var _ = Describe("AIAnalysis controller configuration manifest", func() {
	It("UT-INFRA-AA-2575-001 [BR-AI-088, SI-10]: keeps confidenceThreshold in the ConfigMap config.yaml string", func() {
		manifest := captureKubectlManifest(func() error {
			return deployAIAnalysisControllerManifestOnly(
				context.Background(),
				"test-kubeconfig",
				"localhost/aianalysis:test",
				io.Discard,
			)
		})

		foundConfigMap := false
		for _, document := range strings.Split(manifest, "\n---\n") {
			var raw map[string]interface{}
			Expect(yaml.Unmarshal([]byte(document), &raw)).To(Succeed())
			if raw["kind"] != "ConfigMap" {
				continue
			}

			metadata, ok := raw["metadata"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "ConfigMap metadata must be a YAML mapping")
			if metadata["name"] != "aianalysis-config" {
				continue
			}

			foundConfigMap = true
			data, ok := raw["data"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "ConfigMap data must be a YAML mapping")
			Expect(data).To(HaveKey("config.yaml"))
			Expect(data).NotTo(HaveKey("confidenceThreshold"),
				"ConfigMap.data values must remain string-valued keys")

			configYAML, ok := data["config.yaml"].(string)
			Expect(ok).To(BeTrue(), "ConfigMap config.yaml must be a string value")

			var config struct {
				Rego struct {
					ConfidenceThreshold float64 `json:"confidenceThreshold"`
				} `json:"rego"`
			}
			Expect(yaml.Unmarshal([]byte(configYAML), &config)).To(Succeed())
			Expect(config.Rego.ConfidenceThreshold).To(Equal(0.9))
			break
		}

		Expect(foundConfigMap).To(BeTrue(), "AIAnalysis controller ConfigMap must be emitted")
	})
})
