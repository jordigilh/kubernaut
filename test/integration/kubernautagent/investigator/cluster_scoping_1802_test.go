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

package investigator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// E2E-KA-1802-001 (BR-ORCH-042.5, Issue #1802): fleet cluster_id scoping
// isolates remediation history in the LLM prompt across the FULL journey --
// Investigator.Investigate() -> Enricher.Enrich() -> DSAdapter -> real
// DataStorage/Postgres -> prompt.Builder -> LLM request -- not just the
// enrichment unit (IT-KA-1802-001, test/integration/kubernautagent/
// enrichment/enrichment_real_ds_test.go) in isolation.
//
// This lives alongside fleet_prescoping_test.go / fleet_e2e_journey_test.go
// (same package, same real DS + real Postgres suite infrastructure) rather
// than the Kind-cluster test/e2e/kubernautagent suite: the synchronous
// /v1/incident/analyze REST contract (agentclient.IncidentRequest) has no
// cluster_id field today -- ClusterID only reaches SignalContext via the
// RemediationRequest-driven MCP/interactive path (mcp/adapters/
// signal_resolver.go). A real Investigate() journey through real DS/Postgres
// is the correct, and already-established, "E2E" tier for this repo's fleet
// scenarios (see fleet_e2e_journey_test.go's E2E-KA-FLEET-001/002).
var _ = Describe("E2E-KA-1802-001: fleet cluster_id scoping isolates remediation history end to end (BR-ORCH-042.5, Issue #1802, main only)", Label("e2e", "issue-1802"), func() {

	It("must not leak cluster-a's remediation history into cluster-b's workflow-selection prompt for an identically-named/-spec'd target", func() {
		testID := uuid.New().String()[:8]
		invLogger := logr.Discard()
		auditStore := newCapturingAuditStore(suiteAuditStore)

		ns := "production"
		kind := "Deployment"
		name := "app-1802-" + testID
		target := fmt.Sprintf("%s/%s/%s", ns, kind, name)
		// Must match k8sFixtureClient.GetSpecHash's fixed return value, since
		// this journey lets Enrich() auto-compute the spec hash via the
		// fixture K8s client rather than passing one explicitly.
		specHash := sha256FixtureHash
		clusterA := "cluster-a-" + testID
		clusterB := "cluster-b-" + testID
		corrA := "corr-1802e2e-a-" + testID

		By("Seeding cluster-a's remediation history for the shared target + spec hash directly in Postgres")
		eventData, err := json.Marshal(map[string]interface{}{
			"target_resource":           target,
			"pre_remediation_spec_hash": specHash,
			"action_type":               "IncreaseMemory",
			"signal_type":               "OOMKilled",
			"signal_fingerprint":        "fp-1802e2e-" + testID,
			"outcome":                   "success",
		})
		Expect(err).ToNot(HaveOccurred())
		ts := time.Now().Add(-1 * time.Hour)
		_, err = seedDB.ExecContext(context.Background(),
			`INSERT INTO audit_events (
				event_id, event_date, event_timestamp, event_type, event_version,
				event_category, event_action, event_outcome, correlation_id,
				resource_type, resource_id, actor_id, actor_type,
				retention_days, is_sensitive, event_data, cluster_id
			) VALUES (
				$1, $2, $3, 'remediation.workflow_created', '1.0',
				'remediation', 'create', 'success', $4,
				'test', 'test', 'test', 'system',
				90, false, $5, $6
			)`,
			uuid.New(), ts.Format("2006-01-02"), ts, corrA, eventData, clusterA,
		)
		Expect(err).ToNot(HaveOccurred(), "seeding cluster-a's remediation history must succeed")

		newInvestigator := func() (*investigator.Investigator, *mockLLMClient) {
			mockClient := &mockLLMClient{responses: []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled","confidence":0.9}`}},
				{
					Message:   llm.Message{Role: "assistant", Content: ""},
					ToolCalls: []llm.ToolCall{{ID: "tc_wf1", Name: "list_available_actions", Arguments: `{}`}},
				},
				{
					Message: llm.Message{Role: "assistant", Content: ""},
					ToolCalls: []llm.ToolCall{
						{ID: "tc_submit", Name: "submit_result_no_workflow", Arguments: `{"root_cause_analysis":{"summary":"OOMKilled"},"reasoning":"none"}`},
					},
				},
			}}
			enricher := enrichment.NewEnricher(&k8sFixtureClient{}, suiteDSAdapter, auditStore, invLogger)
			builder, berr := prompt.NewBuilder()
			Expect(berr).ToNot(HaveOccurred())
			rp := parser.NewResultParser()

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: registry.New(),
			})
			return inv, mockClient
		}

		By("Investigating cluster-b's identically-named/-namespaced/-spec'd resource")
		invB, mockClientB := newInvestigator()
		_, err = invB.Investigate(context.Background(), katypes.SignalContext{
			Name: "OOMKilled", Namespace: ns, ResourceKind: kind, ResourceName: name,
			ClusterID: clusterB, RemediationID: "rem-1802e2e-b-" + testID,
		})
		Expect(err).ToNot(HaveOccurred())
		// calls[0] = Phase 1 (RCA) initial prompt; calls[1] = Phase 3 (workflow
		// discovery/selection) initial prompt -- BuildRemediationHistorySection
		// is only rendered into the Phase 3 prompt (prompt/builder.go
		// RenderWorkflowSelection), not the RCA investigation prompt.
		Expect(len(mockClientB.calls)).To(BeNumerically(">=", 2))
		promptB := allMessageContent(mockClientB.calls[1].Messages)
		Expect(promptB).NotTo(ContainSubstring(corrA),
			"Issue #1802: cluster-b's workflow-selection prompt must NOT contain cluster-a's remediation "+
				"history for an identically-named/-spec'd resource -- cluster_id must scope the DS query end "+
				"to end, from Investigate() through the real DataStorage/Postgres query to the LLM prompt")

		By("Sanity: cluster-a's own investigation DOES see its own history in the workflow-selection prompt")
		invA, mockClientA := newInvestigator()
		_, err = invA.Investigate(context.Background(), katypes.SignalContext{
			Name: "OOMKilled", Namespace: ns, ResourceKind: kind, ResourceName: name,
			ClusterID: clusterA, RemediationID: "rem-1802e2e-a-" + testID,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(len(mockClientA.calls)).To(BeNumerically(">=", 2))
		promptA := allMessageContent(mockClientA.calls[1].Messages)
		Expect(promptA).To(ContainSubstring(corrA),
			"cluster-a's own matching ClusterID history must still reach its workflow-selection prompt "+
				"(zero regression)")
	})
})
