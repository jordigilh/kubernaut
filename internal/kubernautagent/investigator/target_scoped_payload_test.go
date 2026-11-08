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

package investigator

import (
	"encoding/json"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("Target-scoped resources_list payload verification", func() {
	It("UT-KA-2609-016: accepts local metadata-list JSON and remote YAML for the requested Kind", func() {
		target := katypes.RemediationTarget{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		}

		Expect(hasExactResourceListEvidence(
			`{"apiVersion":"meta.k8s.io/v1","kind":"DeploymentList","items":[]}`,
			target,
		)).To(BeTrue(), "local PartialObjectMetadataList protocol version must not replace the requested GVK")
		Expect(hasExactResourceListEvidence(
			"apiVersion: apps/v1\nkind: DeploymentList\nitems: []\n",
			target,
		)).To(BeTrue(), "a full remote list envelope must be accepted as exact endpoint evidence")
		Expect(hasExactResourceListEvidence(
			"- apiVersion: apps/v1\n  kind: Deployment\n  metadata:\n    name: api-server\n",
			target,
		)).To(BeTrue(), "kube-mcp-server YAML item sequences must be accepted")
		Expect(hasExactResourceListEvidence("[]", target)).To(BeTrue(),
			"a successful empty remote list proves the requested endpoint accepted the exact GVK")
		Expect(hasExactResourceListEvidence("No resources found", target)).To(BeTrue(),
			"the kube-mcp-server zero-match text response is successful exact-list evidence")
	})

	It("UT-KA-2609-017: rejects unstructured, malformed, or wrong-GVK resources_list output", func() {
		target := katypes.RemediationTarget{Kind: "Deployment", APIVersion: "apps/v1"}
		Expect(hasExactResourceListEvidence(`{"items":[]}`, target)).To(BeFalse())
		Expect(hasExactResourceListEvidence(`{"kind":"PodList","items":[]}`, target)).To(BeFalse())
		Expect(hasExactResourceListEvidence(`{"kind":"DeploymentList","items":"not-a-list"}`, target)).To(BeFalse())
		Expect(hasExactResourceListEvidence(`{"kind":"DeploymentList","apiVersion":"v1","items":[]}`, target)).To(BeFalse())
		Expect(hasExactResourceListEvidence(`{"kind":"DeploymentList","items":"x"}`, target)).To(BeFalse())
		Expect(hasExactResourceListEvidence(`"x"`, target)).To(BeFalse())
		Expect(hasExactResourceListEvidence(`{"apiVersion":"apps/v1","kind":"DeploymentList","items":[null]}`, target)).To(BeFalse())
		Expect(hasExactResourceListEvidence(`{"apiVersion":"apps/v1","kind":"DeploymentList","items":[{}]}`, target)).To(BeFalse())
		Expect(hasExactResourceListEvidence(`{"apiVersion":"apps/v1","kind":"DeploymentList","items":[{"metadata":{"name":123}}]}`, target)).To(BeFalse())
		Expect(hasExactResourceListEvidence(`{"apiVersion":"apps/v1","kind":"DeploymentList","items":[{"metadata":{"labels":{"app":123}}}]}`, target)).To(BeFalse())
		Expect(hasExactResourceListEvidence(
			"- apiVersion: v1\n  kind: Pod\n  metadata:\n    name: api-server\n",
			target,
		)).To(BeFalse())
	})

	It("UT-KA-2609-018: projects target-scoped remote lists to metadata before handoff", func() {
		projected, err := projectResourcesListOutput(
			"- apiVersion: v1\n  kind: Secret\n  metadata:\n    name: credentials\n  data:\n    password: fixture-only-value\n",
			[]byte(`{"apiVersion":"v1","kind":"Secret"}`),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(projected).To(ContainSubstring(`"kind":"SecretList"`))
		Expect(projected).To(ContainSubstring(`"name":"credentials"`))
		Expect(projected).NotTo(ContainSubstring("fixture-only-value"))
		Expect(projected).NotTo(ContainSubstring("password"))
	})

	It("UT-KA-2609-019: preserves list completeness metadata during projection", func() {
		projected, err := projectResourcesListOutput(
			`{"apiVersion":"meta.k8s.io/v1","kind":"DeploymentList","items":[{"metadata":{"name":"api-server"}}],"item_count":1,"total_count":7,"truncated":true}`,
			[]byte(`{"apiVersion":"apps/v1","kind":"Deployment"}`),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(projected).To(ContainSubstring(`"item_count":1`))
		Expect(projected).To(ContainSubstring(`"total_count":7`))
		Expect(projected).To(ContainSubstring(`"truncated":true`))

		empty, err := projectResourcesListOutput("No resources found", []byte(`{"apiVersion":"apps/v1","kind":"Deployment"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(empty).To(ContainSubstring(`"item_count":0`))
		Expect(empty).To(ContainSubstring(`"total_count":0`))
		Expect(empty).To(ContainSubstring(`"truncated":false`))

		unknownTotal, err := projectResourcesListOutput(
			`{"apiVersion":"meta.k8s.io/v1","kind":"DeploymentList","items":[{"metadata":{"name":"api-server"}}],"truncated":true}`,
			[]byte(`{"apiVersion":"apps/v1","kind":"Deployment"}`),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(unknownTotal).To(ContainSubstring(`"truncated":true`))
		Expect(unknownTotal).NotTo(ContainSubstring(`"total_count"`))
	})

	It("UT-KA-2609-020: rejects malformed metadata and completeness scalars", func() {
		_, err := projectResourcesListOutput(
			`{"apiVersion":"meta.k8s.io/v1","kind":"DeploymentList","items":[{"metadata":{"name":123}}],"item_count":1,"total_count":1,"truncated":false}`,
			[]byte(`{"apiVersion":"apps/v1","kind":"Deployment"}`),
		)
		Expect(err).To(HaveOccurred())

		_, err = projectResourcesListOutput(
			`{"apiVersion":"meta.k8s.io/v1","kind":"DeploymentList","items":[{"metadata":{"name":"api-server"}}],"item_count":"1","total_count":1,"truncated":false}`,
			[]byte(`{"apiVersion":"apps/v1","kind":"Deployment"}`),
		)
		Expect(err).To(HaveOccurred())
	})

	It("UT-KA-2609-021: preserves remote continuation metadata without fabricating an unknown total", func() {
		result := "# continue: opaque-next-page\n- apiVersion: apps/v1\n  kind: Deployment\n  metadata:\n    name: api-server\n"
		projected, err := projectResourcesListOutput(result, []byte(`{"apiVersion":"apps/v1","kind":"Deployment"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(projected).To(ContainSubstring(`"truncated":true`))
		Expect(projected).NotTo(ContainSubstring(`"total_count"`))

		result = "# continue: opaque-next-page\n# remainingItemCount: 5\n- apiVersion: apps/v1\n  kind: Deployment\n  metadata:\n    name: api-server\n"
		projected, err = projectResourcesListOutput(result, []byte(`{"apiVersion":"apps/v1","kind":"Deployment"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(projected).To(ContainSubstring(`"truncated":true`))
		Expect(projected).To(ContainSubstring(`"total_count":6`))
	})

	It("UT-KA-2609-022: bounds projected remote labels", func() {
		labels := make([]string, 0, 33)
		for i := 0; i < 33; i++ {
			labels = append(labels, fmt.Sprintf("label-%02d: %q", i, strings.Repeat("x", 300)))
		}
		projected, err := projectResourcesListOutput(
			"- apiVersion: apps/v1\n  kind: Deployment\n  metadata:\n    name: api-server\n    labels:\n      "+strings.Join(labels, "\n      ")+"\n",
			[]byte(`{"apiVersion":"apps/v1","kind":"Deployment"}`),
		)
		Expect(err).NotTo(HaveOccurred())
		var payload struct {
			Items []struct {
				Metadata struct {
					Labels map[string]string `json:"labels"`
				} `json:"metadata"`
			} `json:"items"`
		}
		Expect(json.Unmarshal([]byte(projected), &payload)).To(Succeed())
		Expect(payload.Items).To(HaveLen(1))
		Expect(payload.Items[0].Metadata.Labels).To(HaveLen(32))
		Expect(payload.Items[0].Metadata.Labels["label-00"]).To(HaveLen(256))
		Expect(payload.Items[0].Metadata.Labels).NotTo(HaveKey("label-32"))
	})

	It("UT-KA-2609-023: rejects a list envelope whose GVK differs from the request", func() {
		_, err := projectResourcesListOutput(
			`{"apiVersion":"apps/v1","kind":"PodList","items":[]}`,
			[]byte(`{"apiVersion":"apps/v1","kind":"Deployment"}`),
		)
		Expect(err).To(HaveOccurred())

		_, err = projectResourcesListOutput(
			`{"apiVersion":"v1","kind":"DeploymentList","items":[]}`,
			[]byte(`{"apiVersion":"apps/v1","kind":"Deployment"}`),
		)
		Expect(err).To(HaveOccurred())
	})

	It("UT-KA-2609-024: does not use local metadata-list protocol evidence for remote targets", func() {
		metadataList := `{"apiVersion":"meta.k8s.io/v1","kind":"DeploymentList","items":[{"metadata":{"name":"api-server"}}]}`
		target := katypes.RemediationTarget{Kind: "Deployment", APIVersion: "apps/v1"}
		Expect(hasExactResourceListEvidence(metadataList, target)).To(BeTrue())
		Expect(hasExactResourceListEvidenceForSource(metadataList, target, false)).To(BeFalse())
		Expect(hasExactResourceListEvidenceForSource("[]", target, false)).To(BeTrue(),
			"a successful empty remote sequence remains exact endpoint evidence")
	})
})
