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
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// #1777 (BR-AUDIT-005 / FedRAMP AU-3): sameKindValidationGate and
// apiVersionValidationGate both send a real corrective retry prompt to the
// LLM (a full re-transmission of history + a correction message), but the
// audit event they emit BEFORE building that retry hardcodes
// prompt_length=0/prompt_preview="" regardless of what is actually sent.
// This breaks SOC2 CC8.1 audit-trace reconstruction: the trail claims an
// empty prompt was sent when a real, non-trivial corrective prompt was.
var _ = Describe("#1777: validation-gate audit events must reflect the real retry prompt", func() {

	var (
		logger     logr.Logger
		auditStore *gateRecordingAuditStore
		mockClient *gateMockLLMClient
		builder    *prompt.Builder
		rp         *parser.ResultParser
		enricher   *enrichment.Enricher
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		logger = logr.Discard()
		auditStore = &gateRecordingAuditStore{}
		mockClient = &gateMockLLMClient{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		enricher = enrichment.NewEnricher(&gateK8sClient{}, &gateDSClient{}, auditStore, logger)
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	Describe("UT-KA-1777-001: apiVersionValidationGate audit event carries the real retry prompt", func() {
		It("should populate prompt_length/prompt_preview from the actual retryMessages, not 0/\"\" (BR-AUDIT-005, FedRAMP AU-3)", func() {
			mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
				{Group: "operators.coreos.com", Version: "v1alpha1"},
				{Group: "messaging.knative.dev", Version: "v1"},
			})
			mapper.Add(schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "Subscription"}, meta.RESTScopeNamespace)
			mapper.Add(schema.GroupVersionKind{Group: "messaging.knative.dev", Version: "v1", Kind: "Subscription"}, meta.RESTScopeNamespace)

			signal := katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "etcd-operator-xyz",
				Name:         "etcd-operator-xyz",
				Namespace:    "demo-operator",
				Severity:     "critical",
				Message:      "RBAC denial on wrong API group",
			}

			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				// Gate retry: LLM provides api_version
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"operators.coreos.com/v1alpha1"}
				}`}},
				gateWfToolResp(`{"workflow_id":"restart-sub","confidence":0.9}`),
			}

			resolver := investigator.NewMapperScopeResolver(mapper)
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			gateEvents := auditStore.eventsByAction(audit.ActionAPIVersionGate)
			Expect(gateEvents).NotTo(BeEmpty(), "UT-KA-1777-001: gate must emit an audit event")
			ev := gateEvents[0]

			Expect(ev.Data["prompt_length"]).To(BeNumerically(">", 0),
				"UT-KA-1777-001: gate sends a real corrective prompt to the LLM — the audit trail "+
					"must reflect that, not a hardcoded 0 (SOC2 CC8.1 reconstruction)")
			preview, ok := ev.Data["prompt_preview"].(string)
			Expect(ok).To(BeTrue())
			Expect(preview).NotTo(BeEmpty(),
				"UT-KA-1777-001: prompt_preview must carry the actual correction message, not \"\"")
			Expect(preview).To(ContainSubstring("api_version"),
				"UT-KA-1777-001: prompt_preview must reflect the real correction message content")
		})
	})

	Describe("UT-KA-1777-002: sameKindValidationGate audit event carries the real retry prompt", func() {
		It("should populate prompt_length/prompt_preview from the actual retryMessages, not 0/\"\" (BR-AUDIT-005, FedRAMP AU-3)", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "api-server-xyz",
				Name:         "api-server-xyz",
				Namespace:    "production",
				Severity:     "critical",
				Message:      "Pod OOMKilled repeatedly",
			}

			mockClient.responses = []llm.ChatResponse{
				// Same-kind gate fires: RemediationTarget.Kind ("Pod") == signal.ResourceKind ("Pod")
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Pod OOMKilled",
					"confidence":0.85,
					"remediation_target":{"kind":"Pod","name":"api-server-xyz","namespace":"production"}
				}`}},
				// Gate retry: LLM re-targets the parent Deployment
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Deployment memory limit too low",
					"confidence":0.85,
					"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"production","api_version":"apps/v1"}
				}`}},
				gateWfToolResp(`{"workflow_id":"increase-memory-limit","confidence":0.9}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			gateEvents := auditStore.eventsByAction(audit.ActionSameKindGate)
			Expect(gateEvents).NotTo(BeEmpty(), "UT-KA-1777-002: gate must emit an audit event")
			ev := gateEvents[0]

			Expect(ev.Data["prompt_length"]).To(BeNumerically(">", 0),
				"UT-KA-1777-002: gate sends a real corrective prompt to the LLM — the audit trail "+
					"must reflect that, not a hardcoded 0 (SOC2 CC8.1 reconstruction)")
			preview, ok := ev.Data["prompt_preview"].(string)
			Expect(ok).To(BeTrue())
			Expect(preview).NotTo(BeEmpty(),
				"UT-KA-1777-002: prompt_preview must carry the actual correction message, not \"\"")
			Expect(preview).To(ContainSubstring("same resource kind"),
				"UT-KA-1777-002: prompt_preview must reflect the real correction message content")
		})
	})
})
