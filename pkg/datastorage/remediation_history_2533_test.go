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

package datastorage_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

type issue2533DecisionQuerier struct {
	*mockRemediationHistoryQuerier
	aiCalls        int
	correlationIDs []string
	clusterID      string
	aiErr          error
}

func (m *issue2533DecisionQuerier) QueryAIAnalysisDecisionContextBatch(
	_ context.Context,
	correlationIDs []string,
	clusterID string,
) (map[string]repository.AIAnalysisDecisionContext, error) {
	m.aiCalls++
	m.correlationIDs = append([]string(nil), correlationIDs...)
	m.clusterID = clusterID
	if m.aiErr != nil {
		return nil, m.aiErr
	}
	return map[string]repository.AIAnalysisDecisionContext{
		"rr-2533-known": {
			WorkflowRationale: "The current target evidence supports the catalog workflow.",
			RepeatJustification: &sharedtypes.RepeatJustification{
				EvidenceBasis:         "Current evidence basis",
				PriorOutcomeRelevance: "The prior outcome was ineffective",
				ExpectedMechanism:     "The workflow addresses the observed mechanism",
			},
		},
		"rr-2533-unrelated": {WorkflowRationale: "must not enter the requested target history"},
	}, nil
}

var _ = Describe("Issue #2533 target-scoped historical decision association", func() {
	It("IT-DS-2533-001: batches AI context for existing history correlations and leaves missing records unknown", func() {
		queryCount := 0
		base := &mockRemediationHistoryQuerier{}
		base.queryROEventsBySpecHashFn = func(_ context.Context, _, _, _ string, _, _ time.Time) ([]repository.RawAuditRow, error) {
			queryCount++
			if queryCount == 1 {
				return []repository.RawAuditRow{
					{CorrelationID: "rr-2533-known", EventTimestamp: time.Now().UTC(), EventData: map[string]interface{}{
						"target_resource":           "prod/Deployment/api",
						"pre_remediation_spec_hash": "sha256:prior",
					}},
					{CorrelationID: "rr-2533-missing", EventTimestamp: time.Now().UTC(), EventData: map[string]interface{}{
						"target_resource":           "prod/Deployment/api",
						"pre_remediation_spec_hash": "sha256:prior-2",
					}},
				}, nil
			}
			return nil, nil
		}
		base.queryEffectivenessEventsFn = func(_ context.Context, _ []string) (map[string][]*server.EffectivenessEvent, error) {
			return nil, nil
		}
		mock := &issue2533DecisionQuerier{mockRemediationHistoryQuerier: base}
		handler := server.NewHandler(server.WithRemediationHistoryQuerier(mock))
		recorder := httptest.NewRecorder()
		handler.HandleGetRemediationHistoryContext(recorder, httptest.NewRequest(http.MethodGet,
			"/api/v1/remediation-history/context?targetKind=Deployment&targetName=api&targetNamespace=prod&clusterId=cluster-2533&currentSpecHash=sha256:current", nil))

		Expect(recorder.Code).To(Equal(http.StatusOK))
		Expect(mock.aiCalls).To(Equal(1))
		Expect(mock.correlationIDs).To(ConsistOf("rr-2533-known", "rr-2533-missing"))
		Expect(mock.clusterID).To(Equal("cluster-2533"))

		var response map[string]interface{}
		Expect(json.Unmarshal(recorder.Body.Bytes(), &response)).To(Succeed())
		Expect(recorder.Body.String()).NotTo(ContainSubstring("must not enter the requested target history"))
		chain := response["tier1"].(map[string]interface{})["chain"].([]interface{})
		Expect(chain).To(HaveLen(2))
		for _, raw := range chain {
			entry := raw.(map[string]interface{})
			if entry["remediationUID"] == "rr-2533-known" {
				Expect(entry).To(HaveKeyWithValue("workflowRationale", "The current target evidence supports the catalog workflow."))
				Expect(entry).To(HaveKey("repeatJustification"))
			} else {
				Expect(entry).NotTo(HaveKey("workflowRationale"))
				Expect(entry).NotTo(HaveKey("repeatJustification"))
			}
		}
	})

	It("IT-DS-2533-002: supplemental AI decision-source failure remains fail-closed", func() {
		base := &mockRemediationHistoryQuerier{}
		base.queryROEventsBySpecHashFn = func(_ context.Context, _, _, _ string, _, _ time.Time) ([]repository.RawAuditRow, error) {
			return []repository.RawAuditRow{{
				CorrelationID: "rr-2533-context-unavailable", EventTimestamp: time.Now().UTC(), EventData: map[string]interface{}{
					"target_resource": "prod/Deployment/api", "pre_remediation_spec_hash": "sha256:prior",
				}}}, nil
		}
		base.queryEffectivenessEventsFn = func(_ context.Context, _ []string) (map[string][]*server.EffectivenessEvent, error) {
			return nil, nil
		}
		mock := &issue2533DecisionQuerier{
			mockRemediationHistoryQuerier: base,
			aiErr:                         errors.New("ai audit store temporarily unavailable"),
		}
		handler := server.NewHandler(server.WithRemediationHistoryQuerier(mock))
		recorder := httptest.NewRecorder()
		handler.HandleGetRemediationHistoryContext(recorder, httptest.NewRequest(http.MethodGet,
			"/api/v1/remediation-history/context?targetKind=Deployment&targetName=api&targetNamespace=prod&clusterId=cluster-2533&currentSpecHash=sha256:current", nil))

		Expect(mock.aiCalls).To(Equal(1))
		Expect(recorder.Code).To(Equal(http.StatusInternalServerError))
		Expect(recorder.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))
	})
})
