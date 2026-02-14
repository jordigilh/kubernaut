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

// E2E tests for GET /api/v1/remediation-history/context endpoint.
//
// Business Requirements:
//   - BR-HAPI-016: Remediation history context for LLM prompt enrichment
//
// Design Decisions:
//   - DD-HAPI-016 v1.1: Two-step query pattern
//   - DD-EM-002 v1.1: spec_drift assessment reason
//
// Test Plan: docs/testing/DD-HAPI-016/TEST_PLAN.md (E2E-DS-016-001 through E2E-DS-016-004)
//
// Infrastructure: Kind cluster with DS deployed, PostgreSQL at NodePort 25433.
// Pattern: Same as 12_audit_write_api_test.go — direct DB inserts + HTTP API queries.

package datastorage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BR-HAPI-016: Remediation History API E2E Tests (DD-HAPI-016 v1.1)", Label("e2e", "remediation-history"), Ordered, func() {
	var (
		testDB          *sql.DB
		testID          string
		serviceURL      string
		targetResource  string
		currentSpecHash string
	)

	BeforeEach(func() {
		serviceURL = dataStorageURL
		testID = generateTestID()
		targetResource = fmt.Sprintf("default/Deployment/e2e-nginx-%s", testID)
		currentSpecHash = "sha256:e2e_current_" + testID

		// Verify DS service is ready
		httpClient := &http.Client{Timeout: 2 * time.Second}
		Eventually(func() int {
			resp, err := httpClient.Get(serviceURL + "/health")
			if err != nil || resp == nil {
				return 0
			}
			defer func() { _ = resp.Body.Close() }()
			return resp.StatusCode
		}, "10s", "500ms").Should(Equal(200), "DataStorage service should be ready")

		// Connect to PostgreSQL for direct data insertion (same as 12_audit_write_api_test.go)
		connStr := "host=localhost port=25433 user=slm_user password=test_password dbname=action_history sslmode=disable"
		var err error
		testDB, err = sql.Open("pgx", connStr)
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() error { return testDB.Ping() }, "10s", "1s").Should(Succeed())
	})

	AfterEach(func() {
		if testDB != nil {
			// Clean up test data
			_, _ = testDB.Exec("DELETE FROM audit_events WHERE correlation_id LIKE $1",
				fmt.Sprintf("%%-%s%%", testID))
			_ = testDB.Close()
		}
	})

	// ============================================================================
	// Helpers: Insert test data directly into PostgreSQL
	// ============================================================================

	insertAuditEvent := func(
		eventType, eventCategory, correlationID string,
		eventData map[string]interface{},
		eventTimestamp time.Time,
	) {
		GinkgoHelper()
		eventDataJSON, err := json.Marshal(eventData)
		Expect(err).ToNot(HaveOccurred())

		_, err = testDB.Exec(
			`INSERT INTO audit_events (
				event_id, event_date, event_timestamp, event_type, version,
				event_category, event_action, event_outcome, correlation_id,
				resource_type, resource_id, actor_id, actor_type,
				retention_days, is_sensitive, event_data
			) VALUES (
				$1, $2, $3, $4, '1.0',
				$5, 'create', 'success', $6,
				'test', 'test', 'test', 'system',
				90, false, $7
			)`,
			uuid.New(), eventTimestamp.Format("2006-01-02"), eventTimestamp, eventType,
			eventCategory, correlationID, eventDataJSON,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to insert audit event: %s", eventType)
	}

	insertROEvent := func(correlationID, target, preHash, workflowType string, ts time.Time) {
		GinkgoHelper()
		insertAuditEvent("remediation.workflow_created", "remediation", correlationID,
			map[string]interface{}{
				"target_resource":           target,
				"pre_remediation_spec_hash": preHash,
				"workflow_type":             workflowType,
				"signal_type":              "HighCPULoad",
				"signal_fingerprint":        "fp-e2e-" + testID,
				"outcome":                  "success",
			}, ts)
	}

	insertEMEvents := func(correlationID, reason string, score float64, preHash, postHash string, ts time.Time) {
		GinkgoHelper()
		insertAuditEvent("effectiveness.health.assessed", "effectiveness", correlationID,
			map[string]interface{}{"assessed": true, "score": 0.85, "pod_running": true, "readiness_pass": true}, ts.Add(1*time.Minute))
		insertAuditEvent("effectiveness.alert.assessed", "effectiveness", correlationID,
			map[string]interface{}{"assessed": true, "score": 0.9, "signal_resolved": true}, ts.Add(2*time.Minute))
		insertAuditEvent("effectiveness.metrics.assessed", "effectiveness", correlationID,
			map[string]interface{}{"assessed": true, "score": 0.8, "cpu_before": 0.85, "cpu_after": 0.45}, ts.Add(3*time.Minute))
		insertAuditEvent("effectiveness.hash.computed", "effectiveness", correlationID,
			map[string]interface{}{"pre_remediation_spec_hash": preHash, "post_remediation_spec_hash": postHash}, ts.Add(4*time.Minute))
		insertAuditEvent("effectiveness.assessment.completed", "effectiveness", correlationID,
			map[string]interface{}{"reason": reason, "score": score}, ts.Add(5*time.Minute))
	}

	queryRemediationHistory := func(params map[string]string) (int, map[string]interface{}) {
		GinkgoHelper()
		req, err := http.NewRequest(http.MethodGet, serviceURL+"/api/v1/remediation-history/context", nil)
		Expect(err).ToNot(HaveOccurred())

		q := req.URL.Query()
		for k, v := range params {
			q.Set(k, v)
		}
		req.URL.RawQuery = q.Encode()

		resp, err := AuthHTTPClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		bodyBytes, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())

		var body map[string]interface{}
		if resp.StatusCode == http.StatusOK {
			err = json.Unmarshal(bodyBytes, &body)
			Expect(err).ToNot(HaveOccurred(), "Failed to parse JSON: %s", string(bodyBytes))
		}
		return resp.StatusCode, body
	}

	// ============================================================================
	// E2E Scenarios
	// ============================================================================

	It("E2E-DS-016-001: Full remediation context with assessed EM data flows through the real service", func() {
		// Arrange: insert RO + EM events (reason=full)
		now := time.Now().UTC()
		cid := fmt.Sprintf("corr-e2e-full-%s", testID)
		insertROEvent(cid, targetResource, currentSpecHash, "ScaleUp", now.Add(-2*time.Hour))
		insertEMEvents(cid, "full", 0.85, currentSpecHash, "sha256:e2e_post_"+testID, now.Add(-2*time.Hour))

		// Act: query via HTTP API
		status, body := queryRemediationHistory(map[string]string{
			"targetKind":      "Deployment",
			"targetName":      fmt.Sprintf("e2e-nginx-%s", testID),
			"targetNamespace": "default",
			"currentSpecHash": currentSpecHash,
		})

		// Assert
		Expect(status).To(Equal(http.StatusOK))
		Expect(body["targetResource"]).To(Equal(targetResource))
		Expect(body["currentSpecHash"]).To(Equal(currentSpecHash))

		tier1 := body["tier1"].(map[string]interface{})
		chain := tier1["chain"].([]interface{})
		Expect(chain).To(HaveLen(1))

		entry := chain[0].(map[string]interface{})
		Expect(entry).To(HaveKey("assessmentReason"))
		Expect(entry["assessmentReason"]).To(Equal("full"))
		Expect(entry).To(HaveKey("effectivenessScore"))
		score := entry["effectivenessScore"].(float64)
		Expect(score).To(BeNumerically(">", 0.0))
		Expect(entry).To(HaveKey("healthChecks"))
		Expect(entry).To(HaveKey("metricDeltas"))
	})

	It("E2E-DS-016-002: spec_drift assessment reason and score=0.0 survive full service stack", func() {
		// Arrange
		now := time.Now().UTC()
		cid := fmt.Sprintf("corr-e2e-drift-%s", testID)
		insertROEvent(cid, targetResource, currentSpecHash, "ScaleUp", now.Add(-2*time.Hour))
		insertEMEvents(cid, "spec_drift", 0.0, currentSpecHash, "sha256:e2e_drift_"+testID, now.Add(-2*time.Hour))

		// Act
		status, body := queryRemediationHistory(map[string]string{
			"targetKind":      "Deployment",
			"targetName":      fmt.Sprintf("e2e-nginx-%s", testID),
			"targetNamespace": "default",
			"currentSpecHash": currentSpecHash,
		})

		// Assert
		Expect(status).To(Equal(http.StatusOK))

		tier1 := body["tier1"].(map[string]interface{})
		chain := tier1["chain"].([]interface{})
		Expect(chain).To(HaveLen(1))

		entry := chain[0].(map[string]interface{})
		Expect(entry["assessmentReason"]).To(Equal("spec_drift"),
			"spec_drift reason must survive the full service pipeline")
		Expect(entry["effectivenessScore"]).To(BeNumerically("==", 0.0),
			"spec_drift score must be 0.0 (unreliable)")
	})

	It("E2E-DS-016-003: Non-existent target returns 200 OK with empty chains", func() {
		// Act: query for a target with zero events
		status, body := queryRemediationHistory(map[string]string{
			"targetKind":      "Deployment",
			"targetName":      "e2e-ghost-" + testID,
			"targetNamespace": "no-such-ns",
			"currentSpecHash": "sha256:no_such_hash",
		})

		// Assert
		Expect(status).To(Equal(http.StatusOK))

		tier1 := body["tier1"].(map[string]interface{})
		chain := tier1["chain"].([]interface{})
		Expect(chain).To(BeEmpty())

		tier2 := body["tier2"].(map[string]interface{})
		t2chain := tier2["chain"].([]interface{})
		Expect(t2chain).To(BeEmpty())

		Expect(body["regressionDetected"]).To(BeFalse())
	})

	It("E2E-DS-016-004: Multiple entries with mixed assessment reasons are all returned correctly", func() {
		// Arrange: insert 3 RO events with mixed EM reasons
		now := time.Now().UTC()
		cid1 := fmt.Sprintf("corr-e2e-mixed1-%s", testID)
		cid2 := fmt.Sprintf("corr-e2e-mixed2-%s", testID)
		cid3 := fmt.Sprintf("corr-e2e-mixed3-%s", testID)

		insertROEvent(cid1, targetResource, currentSpecHash, "ScaleUp", now.Add(-3*time.Hour))
		insertEMEvents(cid1, "full", 0.85, currentSpecHash, "sha256:p1_"+testID, now.Add(-3*time.Hour))

		insertROEvent(cid2, targetResource, currentSpecHash, "RestartPod", now.Add(-2*time.Hour))
		insertEMEvents(cid2, "spec_drift", 0.0, currentSpecHash, "sha256:p2_"+testID, now.Add(-2*time.Hour))

		insertROEvent(cid3, targetResource, currentSpecHash, "ScaleDown", now.Add(-1*time.Hour))
		insertEMEvents(cid3, "partial", 0.65, currentSpecHash, "sha256:p3_"+testID, now.Add(-1*time.Hour))

		// Act
		status, body := queryRemediationHistory(map[string]string{
			"targetKind":      "Deployment",
			"targetName":      fmt.Sprintf("e2e-nginx-%s", testID),
			"targetNamespace": "default",
			"currentSpecHash": currentSpecHash,
		})

		// Assert
		Expect(status).To(Equal(http.StatusOK))

		tier1 := body["tier1"].(map[string]interface{})
		chain := tier1["chain"].([]interface{})
		Expect(chain).To(HaveLen(3), "All 3 entries should be returned")

		// Verify each entry has the correct assessmentReason (ordered by timestamp ASC)
		reasons := make([]string, 3)
		for i, e := range chain {
			entry := e.(map[string]interface{})
			if r, ok := entry["assessmentReason"]; ok {
				reasons[i] = r.(string)
			}
		}
		Expect(reasons).To(Equal([]string{"full", "spec_drift", "partial"}),
			"Assessment reasons should match insertion order (ASC by timestamp)")
	})

	It("E2E-DS-016-005: Invalid tier1Window returns 400 Bad Request", func() {
		// Business outcome: Malformed parameters rejected cleanly by the deployed service.
		req, err := http.NewRequest(http.MethodGet, serviceURL+"/api/v1/remediation-history/context", nil)
		Expect(err).ToNot(HaveOccurred())

		q := req.URL.Query()
		q.Set("targetKind", "Deployment")
		q.Set("targetName", "e2e-nginx-invalid")
		q.Set("targetNamespace", "default")
		q.Set("currentSpecHash", "sha256:abc")
		q.Set("tier1Window", "not-a-duration")
		req.URL.RawQuery = q.Encode()

		resp, err := AuthHTTPClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
			"Invalid tier1Window should be rejected with 400")
	})
})
