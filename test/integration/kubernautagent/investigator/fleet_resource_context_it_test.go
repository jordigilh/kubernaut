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
	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/shared/hash"
)

// Issue #2306, work-stream 2: resourceContextTools (get_namespaced_resource_context /
// get_cluster_resource_context) are fleet-agnostic by name -- unlike kubectl_*, they
// are never suppressed from the LLM's schema -- but their Execute() must still resolve
// owner-chain/spec-hash/remediation-history against the investigation's *target*
// cluster, not the hub KA runs on. These two ITs prove that internal routing through
// the real Investigate() entry point, mirroring cluster_scoping_1802_test.go's
// established "real DS/Postgres through the full journey" pattern for this package.
// fleetOverlayResolverSpy (fleet_prescoping_test.go, same package) supplies the
// FleetOverlayResolver double.
var _ = Describe("Fleet-aware resourceContextTools (BR-INTEGRATION-1489, Issue #2306)", Label("fleet", "integration"), func() {

	var (
		invLogger logr.Logger
	)

	BeforeEach(func() {
		invLogger = logr.Discard()
	})

	Describe("IT-KA-FLEET-032 [AC-4/AC-6]: get_namespaced_resource_context resolves against the target cluster, never the hub", func() {
		It("returns the fleet overlay's owner-chain result and never the hub-bound K8sClient's", func() {
			auditStore := newCapturingAuditStore(suiteAuditStore)

			// Hub-bound K8sClient: fixed, unrelated owner chain. If Execute() ever
			// fell through to this client for a fleet-target investigation, its
			// name would leak into the tool result asserted on below.
			hubK8s := &k8sFixtureClient{ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "hub-should-not-appear", Namespace: "production"},
			}}

			reg := registry.New()
			custom.RegisterAll(reg, nil, auditStore, suiteDSAdapter, hubK8s, invLogger)

			overlayGetTool := &fakeTool{name: "resources_get", result: `{"apiVersion":"apps/v1","kind":"Deployment",` +
				`"metadata":{"name":"remote-deployment","namespace":"remote-ns"}}`}
			spy := &fleetOverlayResolverSpy{overlay: map[string]tools.Tool{"resources_get": overlayGetTool}}

			mockClient := &mockLLMClient{responses: []llm.ChatResponse{
				{
					Message: llm.Message{Role: "assistant", Content: ""},
					ToolCalls: []llm.ToolCall{{ID: "tc_ctx", Name: "get_namespaced_resource_context",
						Arguments: `{"kind":"Deployment","name":"remote-deployment","namespace":"remote-ns"}`}},
				},
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
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: reg,
				FleetOverlayResolver: spy,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "remote-deployment", Namespace: "remote-ns", ResourceKind: "Deployment", ResourceName: "remote-deployment",
				ClusterID: "remote-east", RemediationID: "rem-2306-032",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(len(mockClient.calls)).To(BeNumerically(">=", 2))

			toolResultContent := allMessageContent(mockClient.calls[1].Messages)
			Expect(toolResultContent).To(ContainSubstring("remote-deployment"),
				"IT-KA-FLEET-032: get_namespaced_resource_context must resolve the owner chain against the "+
					"target cluster's own resources_get tool, not fall through to the hub")
			Expect(toolResultContent).NotTo(ContainSubstring("hub-should-not-appear"),
				"IT-KA-FLEET-032: the hub-bound K8sClient must never be consulted for a fleet-target "+
					"investigation (AC-6: LLM must only see data for the cluster it was asked about)")
		})
	})

	Describe("IT-KA-FLEET-033 [AU-3]: get_namespaced_resource_context scopes remediation history by the target clusterID", func() {
		It("must not leak cluster-a's remediation history into cluster-b's tool result for an identically-named/-spec'd target", func() {
			testID := uuid.New().String()[:8]
			ns := "remote-ns"
			kind := "Deployment"
			name := "app-2306-" + testID
			target := fmt.Sprintf("%s/%s/%s", ns, kind, name)
			clusterA := "cluster-a-" + testID
			clusterB := "cluster-b-" + testID
			corrA := "corr-2306-a-" + testID

			overlayJSON := fmt.Sprintf(`{"apiVersion":"apps/v1","kind":%q,"metadata":{"name":%q,"namespace":%q}}`, kind, name, ns)

			// The spec hash the fleet overlay path will compute for overlayJSON
			// (via overlayK8sClient.GetSpecHash -> hash.CanonicalResourceFingerprint),
			// so the seeded row's pre_remediation_spec_hash lines up with what
			// fetchRemediationHistory actually queries with -- mirroring
			// cluster_scoping_1802_test.go's use of the fixture K8sClient's fixed
			// sha256FixtureHash for the hub-local case.
			var overlayObj map[string]interface{}
			Expect(json.Unmarshal([]byte(overlayJSON), &overlayObj)).To(Succeed())
			specHash, hErr := hash.CanonicalResourceFingerprint(overlayObj)
			Expect(hErr).ToNot(HaveOccurred())

			By("Seeding cluster-a's remediation history for the shared target + spec hash directly in Postgres")
			eventData, err := json.Marshal(map[string]interface{}{
				"target_resource":           target,
				"pre_remediation_spec_hash": specHash,
				"action_type":               "IncreaseMemory",
				"signal_type":               "OOMKilled",
				"signal_fingerprint":        "fp-2306-" + testID,
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

			newInvestigation := func(clusterID string) *mockLLMClient {
				auditStore := newCapturingAuditStore(suiteAuditStore)
				hubK8s := &k8sFixtureClient{}
				reg := registry.New()
				custom.RegisterAll(reg, nil, auditStore, suiteDSAdapter, hubK8s, invLogger)

				overlayGetTool := &fakeTool{name: "resources_get", result: overlayJSON}
				spy := &fleetOverlayResolverSpy{overlay: map[string]tools.Tool{"resources_get": overlayGetTool}}

				mockClient := &mockLLMClient{responses: []llm.ChatResponse{
					{
						Message: llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_ctx", Name: "get_namespaced_resource_context",
							Arguments: fmt.Sprintf(`{"kind":%q,"name":%q,"namespace":%q}`, kind, name, ns)}},
					},
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
					PhaseTools: investigator.DefaultPhaseToolMap(), Registry: reg,
					FleetOverlayResolver: spy,
				})

				_, ierr := inv.Investigate(context.Background(), katypes.SignalContext{
					Name: name, Namespace: ns, ResourceKind: kind, ResourceName: name,
					ClusterID: clusterID, RemediationID: "rem-2306-033-" + clusterID,
				})
				Expect(ierr).NotTo(HaveOccurred())
				return mockClient
			}

			By("Investigating cluster-b's identically-named/-namespaced/-spec'd resource via get_namespaced_resource_context")
			mockClientB := newInvestigation(clusterB)
			Expect(len(mockClientB.calls)).To(BeNumerically(">=", 2))
			toolResultB := allMessageContent(mockClientB.calls[1].Messages)
			Expect(toolResultB).NotTo(ContainSubstring(corrA),
				"IT-KA-FLEET-033: cluster-b's get_namespaced_resource_context result must NOT contain "+
					"cluster-a's remediation history for an identically-named/-spec'd resource -- clusterID "+
					"must scope the DS query end to end, from the tool's Execute() through the real "+
					"DataStorage/Postgres query")

			By("Sanity: cluster-a's own investigation DOES see its own history in the tool result")
			mockClientA := newInvestigation(clusterA)
			Expect(len(mockClientA.calls)).To(BeNumerically(">=", 2))
			toolResultA := allMessageContent(mockClientA.calls[1].Messages)
			Expect(toolResultA).To(ContainSubstring(corrA),
				"cluster-a's own matching ClusterID history must still reach its tool result (zero regression)")
		})
	})
})
