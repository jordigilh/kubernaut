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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

func newITAmbiguousMapper() *meta.DefaultRESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "operators.coreos.com", Version: "v1alpha1"},
		{Group: "messaging.knative.dev", Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "", Version: "v1"},
	})
	mapper.Add(schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "Subscription"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "messaging.knative.dev", Version: "v1", Kind: "Subscription"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, meta.RESTScopeNamespace)
	return mapper
}

var _ = Describe("TP-1044: apiVersionValidationGate Integration — Full Investigate() Pipeline", func() {

	var (
		invLogger  logr.Logger
		auditStore *capturingAuditStore
		mockClient *mockLLMClient
		builder    *prompt.Builder
		rp         *parser.ResultParser
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		invLogger = logr.Discard()
		auditStore = newCapturingAuditStore(suiteAuditStore)
		mockClient = &mockLLMClient{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	signal := katypes.SignalContext{
		ResourceKind: "Pod",
		ResourceName: "etcd-operator-xyz",
		Name:         "etcd-operator-xyz",
		Namespace:    "demo-operator",
		Severity:     "critical",
		Environment:  "Development",
		Priority:     "P1",
		Message:      "RBAC denial on wrong API group",
	}

	Describe("IT-KA-1044-001: Full pipeline — ambiguous kind, retry succeeds", func() {
		It("should complete pipeline with correct api_version after gate retry (BR-AI-1044 AC2)", func() {
			k8s := &k8sFixtureClient{ownerChain: nil, err: nil}
			localEnricher := enrichment.NewEnricher(k8s, suiteDSAdapter, auditStore, invLogger)
			resolver := investigator.NewMapperScopeResolver(newITAmbiguousMapper())

			mockClient.responses = []llm.ChatResponse{
				// Phase 1 RCA: ambiguous kind, no api_version → gate fires
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd managed by OLM needs operator restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				// Gate retry: LLM provides api_version
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd managed by OLM needs operator restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"operators.coreos.com/v1alpha1"}
				}`}},
				// Workflow selection
				wfToolResp(`{"workflow_id":"restart-operator","confidence":0.9,"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"operators.coreos.com/v1alpha1"}}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: localEnricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
				Pipeline: investigator.Pipeline{CatalogFetcher: &staticCatalogFetcher{validator: parser.NewValidator([]string{"restart-operator"})}},
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.APIVersion).To(Equal("operators.coreos.com/v1alpha1"),
				"IT-KA-1044-001: pipeline must use api_version from gate retry")
			Expect(result.WorkflowID).NotTo(BeEmpty(),
				"IT-KA-1044-001: workflow selection must succeed after gate retry")
			Expect(result.HumanReviewNeeded).To(BeFalse(),
				"IT-KA-1044-001: successful retry must not trigger human review")
			Expect(mockClient.calls).To(HaveLen(3),
				"IT-KA-1044-001: exactly 3 LLM calls (RCA, gate retry, workflow)")
		})
	})

	Describe("IT-KA-1044-002: Full pipeline — ambiguous kind, exhaustion → human review", func() {
		It("should set HumanReviewNeeded=true and stop before workflow selection (BR-AI-1044 AC3 Security)", func() {
			k8s := &k8sFixtureClient{ownerChain: nil, err: nil}
			localEnricher := enrichment.NewEnricher(k8s, suiteDSAdapter, auditStore, invLogger)
			resolver := investigator.NewMapperScopeResolver(newITAmbiguousMapper())

			mockClient.responses = []llm.ChatResponse{
				// Phase 1 RCA: ambiguous kind, no api_version
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				// Gate retry: still no api_version
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: localEnricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"IT-KA-1044-002: gate exhaustion must trigger human review (security fail-safe)")
			Expect(result.HumanReviewReason).To(Equal("rca_incomplete"),
				"IT-KA-1044-002: reason must be rca_incomplete")
			Expect(mockClient.calls).To(HaveLen(2),
				"IT-KA-1044-002: only 2 LLM calls (RCA + gate retry, no workflow)")
		})
	})

	Describe("IT-KA-1044-003: Full pipeline — unambiguous kind, gate skips", func() {
		It("should proceed normally for Deployment without gate intervention (BR-AI-1044 AC4)", func() {
			k8s := &k8sFixtureClient{
				ownerChain: []enrichment.OwnerChainEntry{
					{Kind: "Deployment", Name: "api-server", Namespace: "production"},
				},
			}
			localEnricher := enrichment.NewEnricher(k8s, suiteDSAdapter, auditStore, invLogger)
			resolver := investigator.NewMapperScopeResolver(newITAmbiguousMapper())

			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Deployment api-server needs more replicas",
					"confidence":0.85,
					"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"production"}
				}`}},
				wfToolResp(`{"workflow_id":"scale-up","confidence":0.9,"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"production"}}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: localEnricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
				Pipeline: investigator.Pipeline{CatalogFetcher: &staticCatalogFetcher{validator: parser.NewValidator([]string{"scale-up"})}},
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "api-server-pod",
				Name:         "api-server-pod",
				Namespace:    "production",
				Severity:     "critical",
				Environment:  "Production",
				Priority:     "P0",
				Message:      "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.WorkflowID).To(Equal("scale-up"),
				"IT-KA-1044-003: unambiguous kind must proceed to workflow")
			Expect(result.HumanReviewNeeded).To(BeFalse(),
				"IT-KA-1044-003: unambiguous kind must not trigger human review")
			Expect(mockClient.calls).To(HaveLen(2),
				"IT-KA-1044-003: only 2 LLM calls (no gate retry)")
		})
	})

	Describe("IT-KA-1044-004: Chained gates — same-kind fires, then api_version gate on retry", func() {
		It("should apply both gates sequentially when signal kind matches RCA target (BR-AI-1044 AC9)", func() {
			k8s := &k8sFixtureClient{ownerChain: nil, err: nil}
			localEnricher := enrichment.NewEnricher(k8s, suiteDSAdapter, auditStore, invLogger)
			resolver := investigator.NewMapperScopeResolver(newITAmbiguousMapper())

			mockClient.responses = []llm.ChatResponse{
				// Phase 1 RCA: signal=Subscription, target=Subscription → same-kind gate fires
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				// Same-kind gate retry: different kind, but ambiguous and no api_version → api_version gate fires
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				// api_version gate retry: provides api_version
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"operators.coreos.com/v1alpha1"}
				}`}},
				wfToolResp(`{"workflow_id":"restart-sub","confidence":0.9}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: localEnricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
				Pipeline: investigator.Pipeline{CatalogFetcher: &staticCatalogFetcher{validator: parser.NewValidator([]string{"restart-sub"})}},
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				ResourceKind: "Subscription",
				ResourceName: "etcd",
				Name:         "etcd",
				Namespace:    "demo-operator",
				Severity:     "critical",
				Environment:  "Development",
				Priority:     "P1",
				Message:      "Subscription unhealthy",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.APIVersion).To(Equal("operators.coreos.com/v1alpha1"),
				"IT-KA-1044-004: chained gates must produce correct api_version")
			Expect(result.HumanReviewNeeded).To(BeFalse(),
				"IT-KA-1044-004: successful chain must not trigger human review")
		})
	})

	Describe("IT-KA-1044-005: Audit event for api_version_validation_gate persisted", func() {
		It("should emit audit event with gate action and ambiguity details (BR-AI-1044 AC6)", func() {
			k8s := &k8sFixtureClient{ownerChain: nil, err: nil}
			localEnricher := enrichment.NewEnricher(k8s, suiteDSAdapter, auditStore, invLogger)
			resolver := investigator.NewMapperScopeResolver(newITAmbiguousMapper())

			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"operators.coreos.com/v1alpha1"}
				}`}},
				wfToolResp(`{"workflow_id":"restart-sub","confidence":0.9}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: localEnricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			gateEvents := filterEvents(auditStore.events, audit.EventTypeLLMRequest)
			var found bool
			for _, ev := range gateEvents {
				if ev.EventAction == audit.ActionAPIVersionGate {
					found = true
					Expect(ev.Data).To(HaveKey("ambiguous_kind"),
						"IT-KA-1044-005: audit event must include ambiguous_kind")
					Expect(ev.Data).To(HaveKey("conflicting_groups"),
						"IT-KA-1044-005: audit event must include conflicting_groups")
					Expect(ev.Data).To(HaveKey("retry_outcome"),
						"IT-KA-1044-005: audit event must include retry_outcome")
					break
				}
			}
			Expect(found).To(BeTrue(),
				"IT-KA-1044-005: api_version_validation_gate audit event must be persisted")
		})
	})

	Describe("IT-KA-2141-001: retry_outcome/ambiguous_kind/conflicting_groups actually reach Data Storage (BR-AI-1044, BR-AI-2120, FedRAMP AU-3)", func() {
		It("must be queryable from DS by correlation_id after a real gate-exhaustion Investigate() call", func() {
			correlationID := fmt.Sprintf("test-2141-gate-retry-%d", time.Now().UnixNano())

			k8s := &k8sFixtureClient{ownerChain: nil, err: nil}
			localEnricher := enrichment.NewEnricher(k8s, suiteDSAdapter, auditStore, invLogger)
			resolver := investigator.NewMapperScopeResolver(newITAmbiguousMapper())

			// Same shape as IT-KA-1044-002: gate exhausts because the LLM
			// never supplies api_version, guaranteeing ambiguous_kind/
			// conflicting_groups/retry_outcome=exhausted are set on the
			// audit event -- this is the exact scenario that motivated
			// #2141 (these fields previously never reached DS regardless
			// of the in-process ordering fix from #2119/#2121, since
			// LLMRequestPayload had no carrier field for them).
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: localEnricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			gateSignal := signal
			gateSignal.RemediationID = correlationID

			_, err := inv.Investigate(context.Background(), gateSignal)
			Expect(err).NotTo(HaveOccurred())

			By("Querying DS for the api_version_validation_gate audit event")
			var events []ogenclient.AuditEvent
			Eventually(func() bool {
				params := ogenclient.QueryAuditEventsParams{}
				params.CorrelationID.SetTo(correlationID)
				params.EventType.SetTo(audit.EventTypeLLMRequest)
				params.Limit.SetTo(100)

				resp, qErr := ogenClient.QueryAuditEvents(context.Background(), params)
				if qErr != nil {
					GinkgoWriter.Printf("  ⚠️  Audit query failed: %v\n", qErr)
					return false
				}
				events = resp.Data
				return len(events) > 0
			}, 15*time.Second, 1*time.Second).Should(BeTrue(),
				fmt.Sprintf("DS should contain %s event(s) for correlation_id=%s", audit.EventTypeLLMRequest, correlationID))

			var gateEvent *ogenclient.AuditEvent
			for i := range events {
				if events[i].EventAction == audit.ActionAPIVersionGate {
					gateEvent = &events[i]
					break
				}
			}
			Expect(gateEvent).NotTo(BeNil(),
				"IT-KA-2141-001: api_version_validation_gate event must be among the persisted events")

			payload, ok := gateEvent.EventData.GetLLMRequestPayload()
			Expect(ok).To(BeTrue(), "IT-KA-2141-001: persisted event_data must decode as LLMRequestPayload")

			ambiguousKind, hasAmbiguousKind := payload.AmbiguousKind.Get()
			Expect(hasAmbiguousKind).To(BeTrue(),
				"IT-KA-2141-001: ambiguous_kind must be reconstructable from DS, not just from an in-memory test double")
			Expect(ambiguousKind).To(Equal("Subscription"))

			conflictingGroups, hasConflictingGroups := payload.ConflictingGroups.Get()
			Expect(hasConflictingGroups).To(BeTrue(),
				"IT-KA-2141-001: conflicting_groups must be reconstructable from DS")
			Expect(conflictingGroups).NotTo(BeEmpty())

			retryOutcome, hasRetryOutcome := payload.RetryOutcome.Get()
			Expect(hasRetryOutcome).To(BeTrue(),
				"IT-KA-2141-001: retry_outcome must be reconstructable from DS -- this is the field #2119/#2121's "+
					"deferred-StoreAudit ordering fix was written to protect, now proven end-to-end rather than "+
					"just via a snapshotting test double")
			Expect(retryOutcome).To(Equal("exhausted"))
		})
	})
})
