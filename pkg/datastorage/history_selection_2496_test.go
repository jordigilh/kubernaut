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
	"math"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DD-KA-2496 history selection metadata (BR-KA-016, AU-3, SI-10)", func() {
	DescribeTable("UT-DS-2496-001: both tiers preserve known selection confidence and omit unknown values",
		func(value interface{}, known bool) {
			row := repository.RawAuditRow{CorrelationID: "rr-selection-2496", EventTimestamp: time.Date(2026, 10, 8, 10, 0, 0, 0, time.UTC),
				EventData: map[string]interface{}{
					"workflow_id": "5be40179-af71-4139-a630-7fcebe1891ad", "workflow_name": "Restore memory",
					"workflow_selection_confidence": value, "confidence": 0.99, "confidence_str": "0.88",
					"parameters": map[string]string{"PASSWORD": "history-sensitive-sentinel-2496"},
				}}
			for _, projection := range []interface{}{
				server.CorrelateTier1Chain([]repository.RawAuditRow{row}, nil, "sha256:current"),
				server.BuildTier2Summaries([]repository.RawAuditRow{row}, nil, "sha256:current"),
			} {
				raw, err := json.Marshal(projection)
				Expect(err).NotTo(HaveOccurred())
				var entries []map[string]interface{}
				Expect(json.Unmarshal(raw, &entries)).To(Succeed())
				Expect(entries).To(HaveLen(1))
				Expect(entries[0]).To(HaveKeyWithValue("workflowId", row.EventData["workflow_id"]))
				Expect(entries[0]).To(HaveKeyWithValue("workflowName", "Restore memory"))
				if known {
					Expect(entries[0]).To(HaveKeyWithValue("workflowSelectionConfidence", value))
				} else {
					Expect(entries[0]).NotTo(HaveKey("workflowSelectionConfidence"))
				}
				Expect(entries[0]).To(HaveKeyWithValue("completedAt", row.EventTimestamp.Format(time.RFC3339)))
				Expect(string(raw)).NotTo(ContainSubstring("history-sensitive-sentinel-2496"))
			}
			if value == nil {
				Expect(row.EventData).To(HaveKey("workflow_selection_confidence"))
				Expect(row.EventData["workflow_selection_confidence"]).To(BeNil())
			} else {
				Expect(row.EventData["workflow_selection_confidence"]).To(Equal(value), "reading history must not rewrite source records")
			}
		},
		Entry("selected confidence is distinct from RCA and approval confidence", 0.94, true),
		Entry("known zero", 0.0, true),
		Entry("missing or null", nil, false),
		Entry("string is not a selected score", "0.94", false),
		Entry("negative", -0.1, false),
		Entry("above one", 1.01, false),
		Entry("non-finite", math.Inf(1), false),
	)

	It("UT-DS-2496-001: incomplete identity is not manufactured from action or version", func() {
		row := repository.RawAuditRow{EventData: map[string]interface{}{"workflow_id": 123, "workflow_name": "  ", "action_type": "ScaleMemory", "workflow_version": "1.2"}}
		for _, projection := range []interface{}{server.CorrelateTier1Chain([]repository.RawAuditRow{row}, nil, ""), server.BuildTier2Summaries([]repository.RawAuditRow{row}, nil, "")} {
			raw, err := json.Marshal(projection)
			Expect(err).NotTo(HaveOccurred())
			var entries []map[string]interface{}
			Expect(json.Unmarshal(raw, &entries)).To(Succeed())
			Expect(entries[0]).NotTo(HaveKey("workflowId"))
			Expect(entries[0]).NotTo(HaveKey("workflowName"))
			Expect(entries[0]).To(HaveKeyWithValue("actionType", "ScaleMemory"))
		}
	})

	It("UT-DS-2496-004: history reads report incomplete metadata once without payloads or backfill", func() {
		rows := []repository.RawAuditRow{{CorrelationID: "rr-unknown", EventData: map[string]interface{}{"parameters": "history-sensitive-sentinel-2496"}},
			{CorrelationID: "rr-zero", EventData: map[string]interface{}{"workflow_id": "wf-zero", "workflow_name": "Known zero", "workflow_selection_confidence": 0.0}}}
		query := &mockRemediationHistoryQuerier{queryROEventsBySpecHashFn: func(_ context.Context, _, _, _ string, _, _ time.Time) ([]repository.RawAuditRow, error) {
			return rows, nil
		}}
		sink := &bug674SpyLogSink{}
		h := server.NewHandler(server.WithRemediationHistoryQuerier(query), server.WithLogger(logr.New(sink)))
		recorder := httptest.NewRecorder()
		h.HandleGetRemediationHistoryContext(recorder, httptest.NewRequest(http.MethodGet,
			"/api/v1/remediation-history/context?targetKind=Deployment&targetName=api&targetNamespace=prod&clusterId=origin-a&currentSpecHash=sha256:current", nil))
		Expect(recorder.Code).To(Equal(http.StatusOK))
		var gaps []bug674LogEntry
		for _, entry := range sink.entries {
			if entry.msg == "Remediation history selection metadata incomplete" {
				gaps = append(gaps, entry)
			}
		}
		Expect(gaps).To(HaveLen(1), "one bounded summary per history lookup, not per record/tier")
		Expect(gaps[0].fields).To(HaveKeyWithValue("affected_record_count", 1))
		Expect(gaps[0].fields).To(HaveKeyWithValue("backfill_attempted", false))
		Expect(gaps[0].fields).To(HaveKeyWithValue("target_resource", "prod/Deployment/api"))
		Expect(gaps[0].fields).To(HaveKeyWithValue("cluster_id", "origin-a"))
		Expect(gaps[0].fields).To(HaveKeyWithValue("missing_fields", ConsistOf("workflow_id", "workflow_name", "workflow_selection_confidence")))
		Expect(recorder.Body.String()).NotTo(ContainSubstring("history-sensitive-sentinel-2496"))
		Expect(rows[0].EventData).To(HaveKeyWithValue("parameters", "history-sensitive-sentinel-2496"))
	})
})
