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

package agent

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// IT #2650 (SI-10): the new required api_version input of
// kubernaut_check_existing_remediation is validated through the real,
// schema-validating functiontool.New-constructed tool closure
// (af_check_existing_rr.go NewCheckExistingRemediationTool) — the exact tool
// wired into the agent's tool list in production — using the same
// runnableTool structural-typing technique as IT-AF-2092-001.
var _ = Describe("IT #2650 — kubernaut_check_existing_remediation api_version validation", func() {
	newCheckToolClient := func() client.Client {
		scheme := k8sruntime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		return fake.NewClientBuilder().WithScheme(scheme).Build()
	}

	It("IT-AF-2650-001 (SI-10): a tool call missing api_version is rejected with an invalid-input error before any lookup", func() {
		checkTool, err := tools.NewCheckExistingRemediationTool(newCheckToolClient(), "kubernaut-system")
		Expect(err).NotTo(HaveOccurred())
		runnable, ok := checkTool.(runnableTool)
		Expect(ok).To(BeTrue(), "kubernaut_check_existing_remediation must implement the runnable Run(ctx, args) contract")

		ctx := fakeToolContext{Context: context.Background()}

		// LLM-shaped args: namespace/kind/name present, api_version omitted.
		result, runErr := runnable.Run(ctx, map[string]any{
			"namespace": "prod",
			"kind":      "Route",
			"name":      "shared-route",
		})

		Expect(runErr).To(HaveOccurred(),
			"SI-10: api_version is a required input; its absence must fail the tool call")
		Expect(runErr.Error()).To(ContainSubstring("api_version"),
			"the rejection must name the missing input")
		Expect(result).To(BeNil())
	})
})
