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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

var _ = Describe("isolated Kubernaut Agent E2E fixture", func() {
	It("UT-INFRA-AA-2527-001 [BR-AA-KA-065.3, AC-6, ASVS V4.1.3]: renders a namespaced ClusterIP fixture without shared NodePorts", func() {
		manifest := isolatedKubernautAgentManifest(IsolatedKubernautAgentConfig{
			Namespace:            "ka-restart-e2e-abc123",
			SharedNamespace:      "kubernaut-agent-e2e",
			DataStorageNamespace: "kubernaut-agent-e2e",
			TLSSourceNamespace:   "kubernaut-agent-e2e",
			CAPath:               "/tmp/inter-service-ca.pem",
			DeploymentName:       "ka-restart-agent-abc123",
			ServiceName:          "kubernaut-agent",
			ServiceAccountName:   "ka-restart-agent-sa-abc123",
			Image:                "localhost/kubernautagent:e2e-abc123",
			DataStorageURL:       "https://data-storage-service.kubernaut-agent-e2e.svc.cluster.local:8080",
			DataStorageHealthURL: "https://data-storage-service.kubernaut-agent-e2e.svc.cluster.local:8080/readyz",
			LLMEndpoint:          "http://mock-llm.kubernaut-agent-e2e.svc.cluster.local:8080",
			ShadowLLMEndpoint:    "http://mock-llm.kubernaut-agent-e2e.svc.cluster.local:8080",
			PrometheusURL:        "http://prometheus-svc.kubernaut-agent-e2e.svc.cluster.local:9090",
			AlertManagerURL:      "http://alertmanager-svc.kubernaut-agent-e2e.svc.cluster.local:9093",
		})

		Expect(manifest).To(ContainSubstring("name: ka-restart-agent-abc123"))
		Expect(manifest).To(ContainSubstring("namespace: ka-restart-e2e-abc123"))
		Expect(manifest).To(ContainSubstring("serviceAccountName: ka-restart-agent-sa-abc123"))
		Expect(manifest).To(ContainSubstring("type: ClusterIP"))
		Expect(manifest).To(ContainSubstring("data-storage-service.kubernaut-agent-e2e.svc.cluster.local"))
		Expect(manifest).To(ContainSubstring("mock-llm.kubernaut-agent-e2e.svc.cluster.local"))
		Expect(manifest).To(ContainSubstring("tlsCaFile: /etc/tls-ca/ca.crt"))
		Expect(manifest).To(ContainSubstring("name: kubernaut-agent\n  namespace: ka-restart-e2e-abc123"))
		Expect(manifest).NotTo(ContainSubstring("%!"), "the rendered Kubernetes manifest must not contain fmt formatting errors")
		Expect(manifest).NotTo(ContainSubstring("nodePort:"), "parallel fixtures must not claim shared host NodePorts")

		var configMap struct {
			Data map[string]string `yaml:"data"`
		}
		Expect(yaml.Unmarshal([]byte(manifest), &configMap)).To(Succeed(), "the first Kubernetes document must be valid YAML")
		Expect(configMap.Data["config.yaml"]).NotTo(BeEmpty())
		var renderedConfig map[string]interface{}
		Expect(yaml.Unmarshal([]byte(configMap.Data["config.yaml"]), &renderedConfig)).To(Succeed(),
			"the embedded KA configuration must remain valid YAML")
	})

	It("UT-INFRA-AA-2527-002 [BR-AA-KA-065.3, AC-6]: gives each fixture unique cluster-scoped bindings", func() {
		first := isolatedKubernautAgentClusterRoleBindingNames("ka-restart-e2e-a")
		second := isolatedKubernautAgentClusterRoleBindingNames("ka-restart-e2e-b")

		Expect(first).To(HaveLen(2))
		Expect(second).To(HaveLen(2))
		Expect(first).NotTo(Equal(second), "parallel fixtures must not share ClusterRoleBinding names")
		Expect(isolatedKubernautAgentDataStorageBindingName("ka-restart-e2e-a")).NotTo(
			(Equal(isolatedKubernautAgentDataStorageBindingName("ka-restart-e2e-b"))),
			"parallel fixtures must not share the cross-namespace DataStorage RoleBinding")
		Expect(isolatedKubernautAgentRBACManifest(IsolatedKubernautAgentConfig{
			Namespace:          "ka-restart-e2e-a",
			ServiceAccountName: "ka-restart-agent-sa-a",
		})).NotTo(ContainSubstring("kubernaut-agent-investigator-binding"),
			"the fixture must not overwrite the shared KA ClusterRoleBinding")
	})
})
