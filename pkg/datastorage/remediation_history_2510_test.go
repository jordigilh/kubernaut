/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
*/

package datastorage_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
)

// BR-KA-016 / BR-AUDIT-005; DD-KA-2511; FedRAMP AU-3/SI-10; SOC 2 CC7.2.
var _ = Describe("#2510 applied-state history evidence", func() {
	It("UT-DS-2510-001: confirms a remediation when post hash matches current target", func() {
		at := time.Now().UTC().Add(-time.Hour)
		calls := 0
		mock := &mockRemediationHistoryQuerier{
			queryROEventsBySpecHashFn: func(_ context.Context, _, _, _ string, _, _ time.Time) ([]repository.RawAuditRow, error) {
				calls++
				if calls == 1 {
					return []repository.RawAuditRow{{EventType: "remediation.workflow_created", CorrelationID: "rr-2510-confirmed", EventTimestamp: at, EventData: map[string]interface{}{
						"target_resource": "default/Deployment/api", "pre_remediation_spec_hash": "sha256:before",
						"workflow_id": "workflow-a", "workflow_name": "generic-remediation", "workflow_selection_confidence": 0.9,
					}}}, nil
				}
				return nil, nil
			},
			queryEffectivenessEventsFn: func(_ context.Context, _ []string) (map[string][]*server.EffectivenessEvent, error) {
				return map[string][]*server.EffectivenessEvent{"rr-2510-confirmed": {
					{EventData: map[string]interface{}{"event_type": "effectiveness.hash.computed", "pre_remediation_spec_hash": "sha256:before", "post_remediation_spec_hash": "sha256:current"}},
					{EventData: map[string]interface{}{"event_type": "effectiveness.assessment.completed", "reason": "Full"}},
				}}, nil
			},
		}
		h := server.NewHandler(server.WithRemediationHistoryQuerier(mock))
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/v1/remediation-history/context?targetKind=Deployment&targetName=api&targetNamespace=default&currentSpecHash=sha256:current", nil)
		h.HandleGetRemediationHistoryContext(rec, req)
		Expect(rec.Code).To(Equal(200))
		var response map[string]interface{}
		Expect(json.Unmarshal(rec.Body.Bytes(), &response)).To(Succeed())
		entry := response["tier1"].(map[string]interface{})["chain"].([]interface{})[0].(map[string]interface{})
		Expect(entry).To(HaveKeyWithValue("appliedState", "confirmed"))
		Expect(entry).To(HaveKeyWithValue("specChangeStatus", "post_hash_matches_current"))
		Expect(entry).To(HaveKeyWithValue("preRemediationSpecHash", "sha256:before"))
		Expect(entry).To(HaveKeyWithValue("postRemediationSpecHash", "sha256:current"))
	})

	It("UT-DS-2510-002: distinguishes explicit NoExecution from unknown evidence", func() {
		at := time.Now().UTC().Add(-time.Hour)
		mock := &mockRemediationHistoryQuerier{
			queryROEventsBySpecHashFn: func(_ context.Context, _, _, _ string, _, _ time.Time) ([]repository.RawAuditRow, error) {
				return []repository.RawAuditRow{{EventType: "remediation.workflow_created", CorrelationID: "rr-2510-no-execution", EventTimestamp: at, EventData: map[string]interface{}{
					"target_resource": "default/Deployment/api", "pre_remediation_spec_hash": "sha256:before",
				}}}, nil
			},
			queryEffectivenessEventsFn: func(_ context.Context, _ []string) (map[string][]*server.EffectivenessEvent, error) {
				return map[string][]*server.EffectivenessEvent{"rr-2510-no-execution": {{EventData: map[string]interface{}{"event_type": "effectiveness.assessment.completed", "reason": "NoExecution"}}}}, nil
			},
		}
		h := server.NewHandler(server.WithRemediationHistoryQuerier(mock))
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/v1/remediation-history/context?targetKind=Deployment&targetName=api&targetNamespace=default&currentSpecHash=sha256:current", nil)
		h.HandleGetRemediationHistoryContext(rec, req)
		Expect(rec.Code).To(Equal(200))
		var response map[string]interface{}
		Expect(json.Unmarshal(rec.Body.Bytes(), &response)).To(Succeed())
		entry := response["tier1"].(map[string]interface{})["chain"].([]interface{})[0].(map[string]interface{})
		Expect(entry).To(HaveKeyWithValue("appliedState", "not_confirmed"))
		Expect(entry).To(HaveKeyWithValue("specChangeStatus", "no_execution"))
	})
})
