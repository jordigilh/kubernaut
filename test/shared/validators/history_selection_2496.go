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
package validators

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WaitForHistoryAnalysis reads admitted terminal AA state through the real API.
func WaitForHistoryAnalysis(ctx context.Context, reader client.Reader, rr client.ObjectKey) *aianalysisv1.AIAnalysis {
	GinkgoHelper()
	var completed *aianalysisv1.AIAnalysis
	Eventually(func(g Gomega) {
		list := &aianalysisv1.AIAnalysisList{}
		g.Expect(reader.List(ctx, list, client.InNamespace(rr.Namespace))).To(Succeed())
		for _, aa := range list.Items {
			if aa.Spec.RemediationRequestRef.Name == rr.Name {
				g.Expect(aa.Status.Phase).To(BeElementOf(aianalysisv1.PhaseCompleted, aianalysisv1.PhaseFailed))
				completed = aa.DeepCopy()
				return
			}
		}
		g.Expect(completed).NotTo(BeNil(), "AA must be created for %s", rr.Name)
	}, 4*time.Minute, 2*time.Second).Should(Succeed())
	return completed
}

// WaitForHistoryAgentResult uses admitted AgentSession data as independent
// expected evidence for the durable and live projection assertions.
func WaitForHistoryAgentResult(ctx context.Context, reader client.Reader, aa client.ObjectKey) *agentsessionv1.AgentSessionResult {
	GinkgoHelper()
	var result *agentsessionv1.AgentSessionResult
	Eventually(func(g Gomega) {
		list := &agentsessionv1.AgentSessionList{}
		g.Expect(reader.List(ctx, list, client.InNamespace(aa.Namespace))).To(Succeed())
		for _, as := range list.Items {
			for _, owner := range as.OwnerReferences {
				if owner.Kind == "AIAnalysis" && owner.Name == aa.Name {
					g.Expect(as.Status.Result).NotTo(BeNil())
					result = as.Status.Result.DeepCopy()
					return
				}
			}
		}
		g.Expect(result).NotTo(BeNil())
	}, time.Minute, time.Second).Should(Succeed())
	return result
}

// ValidateHistorySelectionAudit proves the RO producer boundary for BR-KA-016,
// AU-3 and CC7.2. Expected values come from admitted AA state, not an audit mapper.
// Shared by the canonical fullpipeline and supplemental fleet journeys.
func ValidateHistorySelectionAudit(ctx context.Context, ds *ogenclient.Client, rrName string, selected *aianalysisv1.SelectedWorkflow) {
	GinkgoHelper()
	Expect(selected).NotTo(BeNil())
	Expect(selected.WorkflowID).NotTo(BeEmpty())
	Expect(selected.WorkflowName).NotTo(BeEmpty())
	Expect(selected.WorkflowSelectionConfidence).To(HaveValue(Equal(selected.Confidence)))
	Expect(selected.HandoffDigest).To(MatchRegexp(`^sha256:[a-f0-9]{64}$`))
	Eventually(func(g Gomega) {
		events := queryHistoryAuditEvents(ctx, ds, rrName, g)
		var created []map[string]any
		for _, event := range events {
			if event.EventType != "remediation.workflow_created" {
				continue
			}
			g.Expect(event.CorrelationID).To(Equal(rrName))
			g.Expect(event.EventID.IsSet()).To(BeTrue())
			g.Expect(event.ActorID.Value).NotTo(BeEmpty())
			g.Expect(event.EventAction).NotTo(BeEmpty())
			raw, marshalErr := json.Marshal(event.EventData)
			g.Expect(marshalErr).NotTo(HaveOccurred())
			var fields map[string]any
			g.Expect(json.Unmarshal(raw, &fields)).To(Succeed())
			created = append(created, fields)
		}
		g.Expect(created).To(HaveLen(1))
		g.Expect(created[0]).To(HaveKeyWithValue("workflow_id", selected.WorkflowID))
		g.Expect(created[0]).To(HaveKeyWithValue("workflow_name", selected.WorkflowName))
		g.Expect(created[0]).To(HaveKeyWithValue("workflow_selection_confidence", selected.Confidence))
	}, time.Minute, time.Second).Should(Succeed())
}

// ValidateHistoryDecisionAudit independently reconstructs the durable KA/AA
// selection or non-executable assessment for BR-KA-016 / BR-AUDIT-005.
func ValidateHistoryDecisionAudit(ctx context.Context, ds *ogenclient.Client, rrName, priorWorkflowID string, result *agentsessionv1.AgentSessionResult) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		events := queryHistoryAuditEvents(ctx, ds, rrName, g)
		var catalog, response, analysis bool
		for _, event := range events {
			g.Expect(event.EventID.IsSet()).To(BeTrue())
			g.Expect(event.CorrelationID).To(Equal(rrName))
			g.Expect(event.EventAction).NotTo(BeEmpty())
			g.Expect(event.EventOutcome).NotTo(BeEmpty())
			g.Expect(event.ActorID.Value).NotTo(BeEmpty())
			g.Expect(event.ActorType.IsSet()).To(BeTrue())
			g.Expect(event.ActorType.Value).NotTo(BeEmpty())
			raw, marshalErr := json.Marshal(event.EventData)
			g.Expect(marshalErr).NotTo(HaveOccurred())
			g.Expect(string(raw)).NotTo(ContainSubstring("private-history-collision-2496"))
			var fields map[string]any
			g.Expect(json.Unmarshal(raw, &fields)).To(Succeed())
			switch event.EventType {
			case "workflow.catalog.workflows_listed":
				g.Expect(event.ActorType.Value).To(Equal("service"))
				g.Expect(event.ActorID.Value).To(Equal("kubernaut-agent"))
				g.Expect(string(raw)).To(ContainSubstring(priorWorkflowID), "the prior workflow must remain available, not banned")
				catalog = true
			case "aiagent.response.complete":
				g.Expect(event.ActorType.Value).To(Equal("service"))
				g.Expect(event.ActorID.Value).To(Equal("kubernaut-agent"))
				data, ok := fields["response_data"].(map[string]any)
				g.Expect(ok).To(BeTrue())
				validateHistoryReconstruction(g, data, result, true)
				response = true
			case "aianalysis.analysis.completed", "aianalysis.analysis.failed":
				g.Expect(event.ActorType.Value).To(Equal("service"))
				g.Expect(event.ActorID.Value).To(Equal("aianalysis-controller"))
				payload, valid := event.EventData.GetAIAnalysisAuditPayload()
				g.Expect(valid).To(BeTrue())
				g.Expect(string(payload.EventType)).To(Equal(event.EventType), "failed and completed discriminators must match")
				validateHistoryReconstruction(g, fields, result, false)
				analysis = true
			case "webhook.remediationrequest.timeout_modified":
				// Admission records the authenticated Kubernetes principal as
				// a user, even when that principal is a controller SA. Do not
				// confuse it with the service's own decision/audit emission.
				g.Expect(event.ActorType.Value).To(Equal("user"))
				g.Expect(event.ActorID.Value).To(HavePrefix("system:serviceaccount:"))
			}
		}
		g.Expect(catalog && response && analysis).To(BeTrue(), "catalog, terminal KA and AA records must reconstruct the decision")
	}, time.Minute, time.Second).Should(Succeed())
}

// A catalog/selection record can precede the newest 50 events for the same
// correlation. AU-3/CC7.2 reconstruction must follow every page, not mistake the
// default first page for the complete lifecycle population (E2E-FP-2496-002).
func queryHistoryAuditEvents(ctx context.Context, ds *ogenclient.Client, rrName string, g Gomega) []ogenclient.AuditEvent {
	GinkgoHelper()
	const pageSize = 50
	events := make([]ogenclient.AuditEvent, 0, pageSize)
	for offset := 0; ; {
		resp, err := ds.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
			CorrelationID: ogenclient.NewOptString(rrName), Limit: ogenclient.NewOptInt(pageSize), Offset: ogenclient.NewOptInt(offset),
		})
		g.Expect(err).NotTo(HaveOccurred())
		events = append(events, resp.Data...)
		pagination, present := resp.Pagination.Get()
		g.Expect(present).To(BeTrue(), "correlation queries must return pagination metadata")
		more, known := pagination.HasMore.Get()
		g.Expect(known).To(BeTrue(), "the final page must be explicit, not assumed")
		if !more {
			return events
		}
		g.Expect(resp.Data).NotTo(BeEmpty(), "a continuing page must advance the correlation query")
		offset += len(resp.Data)
	}
}

func validateHistoryReconstruction(g Gomega, fields map[string]any, result *agentsessionv1.AgentSessionResult, kaResponse bool) {
	GinkgoHelper()
	if result.SelectedWorkflow == nil {
		g.Expect(fields).NotTo(HaveKey("selectedWorkflow"))
		g.Expect(fields).NotTo(HaveKey("workflow_id"))
	} else {
		var expected map[string]any
		g.Expect(json.Unmarshal(result.SelectedWorkflow.Raw, &expected)).To(Succeed())
		selected := fields
		idKey, nameKey := "workflow_id", "workflow_name"
		if kaResponse {
			var ok bool
			selected, ok = fields["selectedWorkflow"].(map[string]any)
			g.Expect(ok).To(BeTrue())
			idKey, nameKey = "workflowId", "workflowName"
		}
		g.Expect(selected).To(HaveKeyWithValue(idKey, expected["workflow_id"]))
		g.Expect(selected).To(HaveKeyWithValue(nameKey, expected["workflow_name"]))
		for _, field := range []string{"workflow_selection_confidence", "repeat_justification", "repeat_justification_required",
			"prior_workflow_match", "repeat_validation_outcome", "handoff_digest", "prior_remediation_uids"} {
			if value, present := expected[field]; present {
				g.Expect(selected).To(HaveKeyWithValue(field, value), "durable selection field %s must survive", field)
			}
		}
	}
	if result.SelectionAssessment == nil {
		g.Expect(fields).NotTo(HaveKey("selection_assessment"))
		return
	}
	var expected map[string]any
	g.Expect(json.Unmarshal(result.SelectionAssessment.Raw, &expected)).To(Succeed())
	g.Expect(fields).To(HaveKeyWithValue("selection_assessment", expected), "rejected reasoning must remain non-executable and reconstructable")
}

// ValidateHistorySelectionLive checks the authenticated AF SSE response against
// admitted AgentSession data. It never queries DS as live-result transport.
func ValidateHistorySelectionLive(resp *http.Response, result *agentsessionv1.AgentSessionResult) {
	GinkgoHelper()
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/event-stream"))
	data, terminal := readHistorySummary(resp)
	Expect(terminal).To(BeElementOf("completed", "input-required"))
	Expect(data).NotTo(BeNil(), "KA completion must reach the live Console-facing investigation_summary DataPart")
	ValidateLiveSelectionProjection(data)
	raw, marshalErr := json.Marshal(data)
	Expect(marshalErr).NotTo(HaveOccurred())
	Expect(string(raw)).NotTo(ContainSubstring("private-history-collision-2496"))
	if result.SelectedWorkflow != nil {
		var selected map[string]any
		Expect(json.Unmarshal(result.SelectedWorkflow.Raw, &selected)).To(Succeed())
		live, ok := data["selected_workflow"].(map[string]any)
		Expect(ok).To(BeTrue())
		for _, field := range []string{"workflow_id", "workflow_name", "workflow_selection_confidence", "repeat_justification",
			"repeat_justification_required", "prior_workflow_match", "repeat_validation_outcome", "handoff_digest"} {
			if expected, present := selected[field]; present {
				Expect(live).To(HaveKeyWithValue(field, expected), "live trusted field %s must match KA, not an audit lookup", field)
			}
		}
	} else {
		Expect(data).NotTo(HaveKey("selected_workflow"))
	}
	if result.SelectionAssessment != nil {
		var assessment map[string]any
		Expect(json.Unmarshal(result.SelectionAssessment.Raw, &assessment)).To(Succeed())
		live, ok := data["selection_assessment"].(map[string]any)
		Expect(ok).To(BeTrue())
		for _, field := range []string{"proposed_workflow_id", "workflow_name", "workflow_selection_confidence", "repeat_justification",
			"repeat_justification_required", "prior_workflow_match", "repeat_validation_outcome", "handoff_digest"} {
			if expected, present := assessment[field]; present {
				Expect(live).To(HaveKeyWithValue(field, expected), "live trusted assessment field %s must match KA", field)
			}
		}
	} else {
		Expect(data).NotTo(HaveKey("selection_assessment"))
	}
}

type historySelectionStreamEvent struct {
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

type historySelectionStreamUpdate struct {
	Kind     string `json:"kind"`
	Artifact struct {
		Metadata map[string]any `json:"metadata"`
		Parts    []struct {
			Kind string         `json:"kind"`
			Data map[string]any `json:"data"`
		} `json:"parts"`
	} `json:"artifact"`
	Status struct {
		State string `json:"state"`
	} `json:"status"`
}

func readHistorySummary(resp *http.Response) (map[string]any, string) {
	GinkgoHelper()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var summary map[string]any
	var terminal string
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if !strings.HasPrefix(payload, "{") {
			continue
		}
		var event historySelectionStreamEvent
		Expect(json.Unmarshal([]byte(payload), &event)).To(Succeed())
		Expect(string(event.Error)).To(BeElementOf("", "null"), "AF must not return an RPC error instead of the current result")
		if len(event.Result) == 0 {
			continue
		}
		var update historySelectionStreamUpdate
		Expect(json.Unmarshal(event.Result, &update)).To(Succeed())
		if update.Kind == "status-update" {
			switch update.Status.State {
			case "completed", "failed", "input-required":
				terminal = update.Status.State
			}
			continue
		}
		if update.Kind != "artifact-update" || update.Artifact.Metadata["schema"] != "investigation_summary" {
			continue
		}
		summary = nil // The latest summary must itself carry the asserted DataPart.
		for _, part := range update.Artifact.Parts {
			if part.Kind == "data" && part.Data != nil {
				summary = part.Data
				break
			}
		}
	}
	Expect(scanner.Err()).NotTo(HaveOccurred(), "the live result stream must be read completely")
	return summary, terminal
}
