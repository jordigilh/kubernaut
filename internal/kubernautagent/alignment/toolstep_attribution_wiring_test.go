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

package alignment_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// BR-INTEGRATION-1489, DD-FLEET-004: proves the production entry point,
// SubmitToolStep, actually attributes cluster ID via audit.ClusterIDFromContext
// end-to-end through a real Observer/Evaluator — not just the extracted
// attributionClusterID helper in isolation (that's UT-KA-FLEET-019).
var _ = Describe("SubmitToolStep cluster attribution wiring (BR-INTEGRATION-1489, DD-FLEET-004)", Label("fleet", "integration"), func() {

	Describe("IT-KA-FLEET-016 [AU-3/CC8.1]: SubmitToolStep attributes ClusterID from context", func() {
		It("attributes the context's ClusterID to a generically-named tool call, not just prefixed ones", func() {
			client := &mockLLMClient{responses: []llm.ChatResponse{cleanResponse()}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 10 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "")
			observer, err := alignment.NewObserver(evaluator)
			Expect(err).NotTo(HaveOccurred())

			ctx := alignment.WithObserver(context.Background(), observer)
			ctx = audit.WithClusterID(ctx, "remote-east")

			alignment.SubmitToolStep(ctx, "kubectl_get_by_name", `{"pod":"api-server"}`)

			wr := observer.WaitForCompletion(5 * time.Second)
			Expect(wr.Observations).To(HaveLen(1))
			Expect(wr.Observations[0].Step.ClusterID).To(Equal("remote-east"),
				"IT-KA-FLEET-016: a generically-named tool call must still be attributed to the "+
					"investigation's target cluster via context — DD-FLEET-004 removes the "+
					"'{clusterID}__tool' name prefix this used to rely on")
		})

		It("falls back to the legacy name-prefix convention when the context carries no ClusterID", func() {
			client := &mockLLMClient{responses: []llm.ChatResponse{cleanResponse()}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 10 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "")
			observer, err := alignment.NewObserver(evaluator)
			Expect(err).NotTo(HaveOccurred())

			ctx := alignment.WithObserver(context.Background(), observer)

			alignment.SubmitToolStep(ctx, "prefix-cluster__resources_get", `{"pod":"api-server"}`)

			wr := observer.WaitForCompletion(5 * time.Second)
			Expect(wr.Observations).To(HaveLen(1))
			Expect(wr.Observations[0].Step.ClusterID).To(Equal("prefix-cluster"),
				"IT-KA-FLEET-016: pre-DD-FLEET-004 callers that never set audit.WithClusterID must be "+
					"unaffected — the legacy name-parsing fallback still works")
		})
	})
})
