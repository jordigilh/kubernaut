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

	"github.com/go-logr/logr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

type labelDetectorGetTool struct{}

func (labelDetectorGetTool) Name() string                { return "resources_get" }
func (labelDetectorGetTool) Description() string         { return "remote label detector get" }
func (labelDetectorGetTool) Parameters() json.RawMessage { return json.RawMessage(`{}`) }
func (labelDetectorGetTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var request map[string]string
	Expect(json.Unmarshal(args, &request)).To(Succeed())
	if request["kind"] == "Namespace" {
		return `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"remote-ns"}}`, nil
	}
	return `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"remote-target","namespace":"remote-ns","labels":{"app":"remote-target"}}}`, nil
}

type labelDetectorListTool struct{}

func (labelDetectorListTool) Name() string                { return "resources_list" }
func (labelDetectorListTool) Description() string         { return "remote label detector list" }
func (labelDetectorListTool) Parameters() json.RawMessage { return json.RawMessage(`{}`) }
func (labelDetectorListTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var request map[string]string
	Expect(json.Unmarshal(args, &request)).To(Succeed())
	switch request["kind"] {
	case "HorizontalPodAutoscaler":
		return `{"apiVersion":"autoscaling/v2","kind":"HorizontalPodAutoscalerList","items":[{"spec":{"scaleTargetRef":{"kind":"Deployment","name":"remote-target"}}}]}`, nil
	case "PodDisruptionBudget":
		return `{"apiVersion":"policy/v1","kind":"PodDisruptionBudgetList","items":[{"spec":{"selector":{"matchLabels":{"app":"remote-target"}}}}]}`, nil
	case "NetworkPolicy":
		return `{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicyList","items":[]}`, nil
	case "ResourceQuota":
		return `{"apiVersion":"v1","kind":"ResourceQuotaList","items":[]}`, nil
	case "PersistentVolumeClaim":
		return `{"apiVersion":"v1","kind":"PersistentVolumeClaimList","items":[]}`, nil
	default:
		return `{"items":[]}`, nil
	}
}

// Issue #2343: Investigate()'s automatic pre-fetch enrichment step
// (resolveEnrichmentCached -> Enricher.Enrich) always resolved owner-chain/
// spec-hash against the hub K8sClient, even for a fleet-target investigation
// -- unlike get_namespaced_resource_context/get_cluster_resource_context
// (issue #2306, proven by fleet_resource_context_it_test.go), which already
// resolve per-call from ctx via custom.ResolveK8sClient. Every existing fleet
// IT in this package built its Enricher with a bare k8sFixtureClient and
// never installed a K8sResolver, so the exact call path that was broken
// (Enricher.k8s inside the pre-fetch, not the LLM-callable tools) was never
// exercised with a fixture that could reveal "wrong cluster" -- these two
// specs close that coverage gap by wiring the Enricher exactly as
// cmd/kubernautagent/datastorage.go's buildEnricher does in production.
var _ = Describe("Fleet-aware enrichment pre-fetch (BR-INTEGRATION-1489, Issue #2343)", Label("fleet", "integration"), func() {

	var (
		invLogger  logr.Logger
		auditStore *capturingAuditStore
	)

	BeforeEach(func() {
		invLogger = logr.Discard()
		auditStore = newCapturingAuditStore(suiteAuditStore)
	})

	// minimalInvestigationResponses is the shortest response sequence that
	// completes Investigate() without exercising any tool calls that would
	// themselves route through the fleet overlay -- isolating this spec to
	// the automatic pre-fetch enrichment step alone (mirrors
	// fleet_prescoping_test.go's IT-KA-FLEET-013 minimal sequence).
	minimalInvestigationResponses := func() []llm.ChatResponse {
		return []llm.ChatResponse{
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
		}
	}

	Describe("IT-KA-FLEET-036 [AC-6]: pre-fetch enrichment resolves against the fleet overlay, never the hub K8sClient", func() {
		It("emits an enrichment.completed audit event whose root owner is the overlay's resource, not the hub's", func() {
			// Hub-bound K8sClient: fixed, unrelated owner chain. If the
			// pre-fetch enrichment step ever used this for a fleet-target
			// investigation, its sentinel name would leak into the audit
			// event asserted on below.
			hubK8s := &k8sFixtureClient{ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "hub-should-not-appear", Namespace: "production"},
			}}

			overlayGetTool := &fakeTool{name: "resources_get", result: `{"apiVersion":"apps/v1","kind":"Deployment",` +
				`"metadata":{"name":"remote-target","namespace":"remote-ns"}}`}
			spy := &fleetOverlayResolverSpy{overlay: map[string]tools.Tool{"resources_get": overlayGetTool}}

			mockClient := &mockLLMClient{responses: minimalInvestigationResponses()}

			// WithK8sResolver installed exactly as buildEnricher wires it in
			// production (cmd/kubernautagent/datastorage.go): per-call
			// routing through custom.ResolveK8sClient, not a fixed client.
			enricher := enrichment.NewEnricher(hubK8s, suiteDSAdapter, auditStore, invLogger).
				WithK8sResolver(func(ctx context.Context) enrichment.K8sClient {
					return custom.ResolveK8sClient(ctx, hubK8s, invLogger)
				})
			builder, berr := prompt.NewBuilder()
			Expect(berr).ToNot(HaveOccurred())
			rp := parser.NewResultParser()

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: registry.New(),
				FleetOverlayResolver: spy,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "remote-target", Namespace: "remote-ns", ResourceKind: "Deployment", ResourceName: "remote-target",
				ClusterID: "remote-east", RemediationID: "rem-2343-036",
			})
			Expect(err).NotTo(HaveOccurred())

			completedEvents := filterEvents(auditStore.events, audit.EventTypeEnrichmentCompleted)
			Expect(completedEvents).NotTo(BeEmpty(), "IT-KA-FLEET-036: an enrichment.completed audit event must be emitted")
			ev := completedEvents[0]
			Expect(ev.Data["root_owner_name"]).To(Equal("remote-target"),
				"IT-KA-FLEET-036: pre-fetch enrichment must resolve the owner chain against the target "+
					"cluster's own resources_get tool, not fall through to the hub")
			Expect(ev.Data["root_owner_name"]).NotTo(Equal("hub-should-not-appear"),
				"IT-KA-FLEET-036: the hub-bound K8sClient must never be consulted for a fleet-target "+
					"investigation's pre-fetch enrichment (AC-6)")
		})

		It("still resolves against the hub K8sClient for a hub-local investigation (zero regression)", func() {
			hubK8s := &k8sFixtureClient{ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "hub-local-target", Namespace: "production"},
			}}
			mockClient := &mockLLMClient{responses: minimalInvestigationResponses()}

			enricher := enrichment.NewEnricher(hubK8s, suiteDSAdapter, auditStore, invLogger).
				WithK8sResolver(func(ctx context.Context) enrichment.K8sClient {
					return custom.ResolveK8sClient(ctx, hubK8s, invLogger)
				})
			builder, berr := prompt.NewBuilder()
			Expect(berr).ToNot(HaveOccurred())
			rp := parser.NewResultParser()

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: registry.New(),
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "hub-local-target", Namespace: "production", ResourceKind: "Deployment", ResourceName: "hub-local-target",
				RemediationID: "rem-2343-036b",
			})
			Expect(err).NotTo(HaveOccurred())

			completedEvents := filterEvents(auditStore.events, audit.EventTypeEnrichmentCompleted)
			Expect(completedEvents).NotTo(BeEmpty())
			ev := completedEvents[0]
			Expect(ev.Data["root_owner_name"]).To(Equal("hub-local-target"),
				"a hub-local investigation (no ClusterID, no overlay) must still resolve via the hub K8sClient")
		})
	})

	Describe("IT-KA-FLEET-2345 [AC-4, AC-6, SI-10]: detected labels use the fleet overlay", func() {
		It("detects remote HPA and PDB labels through production-style enricher routing", func() {
			hubK8s := &k8sFixtureClient{ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "hub-should-not-appear", Namespace: "production"},
			}}
			hubDetector := enrichment.NewLabelDetector(nil, newItTestMapper(), invLogger)
			spy := &fleetOverlayResolverSpy{overlay: map[string]tools.Tool{
				"resources_get":  labelDetectorGetTool{},
				"resources_list": labelDetectorListTool{},
			}}
			mockClient := &mockLLMClient{responses: minimalInvestigationResponses()}

			enricher := enrichment.NewEnricher(hubK8s, suiteDSAdapter, auditStore, invLogger).
				WithK8sResolver(func(ctx context.Context) enrichment.K8sClient {
					return custom.ResolveK8sClient(ctx, hubK8s, invLogger)
				}).
				WithLabelDetectorResolver(func(ctx context.Context) *enrichment.LabelDetector {
					return custom.ResolveLabelDetector(ctx, hubDetector, newItTestMapper(), invLogger)
				})

			builder, berr := prompt.NewBuilder()
			Expect(berr).NotTo(HaveOccurred())
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: parser.NewResultParser(), Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: registry.New(),
				FleetOverlayResolver: spy,
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "remote-target", Namespace: "remote-ns", ResourceKind: "Deployment", ResourceName: "remote-target",
				ClusterID: "remote-east", RemediationID: "rem-2345-001",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.DetectedLabels).NotTo(BeNil())
			Expect(result.DetectedLabels["hpaEnabled"]).To(BeTrue(), "AC-4: HPA evidence must come from the target cluster")
			Expect(result.DetectedLabels["pdbProtected"]).To(BeTrue(), "AC-4: PDB evidence must come from the target cluster")
			failed, ok := result.DetectedLabels["failedDetections"].([]string)
			if ok {
				Expect(failed).NotTo(ContainElement("hpaEnabled"), "SI-10: valid remote list responses must not be treated as detection failures")
				Expect(failed).NotTo(ContainElement("pdbProtected"), "SI-10: valid remote list responses must not be treated as detection failures")
			}
		})
	})
})
