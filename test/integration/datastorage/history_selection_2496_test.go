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

package datastorage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/go-logr/zapr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/agentsession"
	kaaudit "github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	aimetrics "github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	dsconfig "github.com/jordigilh/kubernaut/pkg/datastorage/config"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// BR-KA-016, BR-AUDIT-005; DD-KA-2496; AU-3/CC7.2/SI-10/AC-4.
const (
	dsITRejected2496       = "rejected"
	dsITRejectedAbsent2496 = "rejected-absent"
)

// Uses the canonical suite's migrated PostgreSQL/Redis, production server and
// generated client. Only the external TokenReview/SAR authority is replaced.
var _ = Describe("#2496 authenticated history selection metadata", Label("integration", "remediation-history"), func() {
	var (
		ts                                             *httptest.Server
		httpClient                                     *http.Client
		client                                         *api.Client
		logs                                           *observer.ObservedLogs
		params                                         api.GetRemediationHistoryContextParams
		target, cluster, recentID, historicalID, oldID string
		recorded                                       time.Time
	)

	BeforeEach(func() {
		id := uuid.NewString()
		cluster = "origin-" + id
		target = "prod/Deployment/api-" + id
		recentID, historicalID, oldID = "rr-recent-"+id, "rr-history-"+id, "rr-unknown-"+id
		// The generated audit wire uses RFC3339 second precision; compare the
		// recorded instant, not Go's location representation from PostgreSQL.
		recorded = time.Now().UTC().Add(-30 * time.Minute).Truncate(time.Second)
		params = api.GetRemediationHistoryContextParams{
			TargetKind: "Deployment", TargetName: "api-" + id, TargetNamespace: "prod",
			CurrentSpecHash: "sha256:" + id, ClusterId: api.NewOptString(cluster),
			Tier1Window: api.NewOptString("1h"), Tier2Window: api.NewOptString("2160h"),
		}
		core, observed := observer.New(zap.InfoLevel)
		logs = observed
		pgHost, pgPort := os.Getenv("POSTGRES_HOST"), os.Getenv("POSTGRES_PORT")
		if pgHost == "" {
			pgHost = localhost
		}
		if pgPort == "" {
			pgPort = "15433"
		}
		redisHost, redisPort := os.Getenv("REDIS_HOST"), os.Getenv("REDIS_PORT")
		if redisHost == "" {
			redisHost = localhost
		}
		if redisPort == "" {
			redisPort = "16379"
		}
		const token, user = "history-2496-token", "system:serviceaccount:prod:history-reader"
		srv, err := server.NewServer(server.ServerDeps{
			DBConnStr: fmt.Sprintf("host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable", pgHost, pgPort),
			RedisAddr: redisHost + ":" + redisPort, Logger: zapr.NewLogger(zap.New(core)),
			AppConfig: &dsconfig.Config{
				Server:   dsconfig.ServerConfig{SignerCertDir: datastorageIntegrationSigningCertDirOrDie()},
				Database: dsconfig.DatabaseConfig{MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: "1m", ConnMaxIdleTime: "1m"},
			},
			ServerConfig: &server.Config{Port: 0, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, TLS: datastorageIntegrationTLSConfig()},
			DLQMaxLen:    100, AuthNamespace: "prod",
			Authenticator: &auth.MockAuthenticator{ValidUsers: map[string]string{token: user}},
			Authorizer:    &auth.MockAuthorizer{AllowedUsers: map[string]bool{user: true}},
		})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer shutdownCancel()
			Expect(srv.Shutdown(shutdownCtx)).To(Succeed())
		})
		ts = httptest.NewServer(srv.Handler())
		DeferCleanup(ts.Close)
		httpClient = &http.Client{Transport: &bearerTransport{token: token}, Timeout: 15 * time.Second}
		client, err = api.NewClient(ts.URL, api.WithClient(httpClient))
		Expect(err).NotTo(HaveOccurred())
	})

	writeRO := func(id, resource, origin string, confidence float64, timestamp time.Time) {
		GinkgoHelper()
		event, err := roaudit.NewManager("remediationorchestrator").BuildRemediationWorkflowCreatedEvent(
			id, "prod", id, origin, roaudit.RemediationWorkflowCreatedData{
				PreRemediationSpecHash: params.CurrentSpecHash, TargetResource: resource,
				WorkflowID: "f125dbf3-a40f-4faa-a4e7-c3c480dd77cc", WorkflowName: "IncreaseMemoryLimits",
				WorkflowSelectionConfidence: &confidence, ActionType: "IncreaseMemory", SignalType: "OOMKilled",
			})
		Expect(err).NotTo(HaveOccurred())
		event.EventTimestamp = timestamp
		postHistoryAudit2496(httpClient, ts.URL, event)
	}

	// EM component/lifecycle data goes through the actual authenticated typed
	// ingestion route, not directly into a repository projection.
	writeAssessment := func(id string, reason string, timestamp time.Time) {
		GinkgoHelper()
		for _, component := range []string{"health", "alert", "completed"} {
			eventType := "effectiveness." + component + ".assessed"
			data := map[string]interface{}{"event_type": eventType, "correlation_id": id, "namespace": "prod", "component": component, "assessed": true, "score": 0.0}
			if component == "alert" {
				data["alert_resolution"] = map[string]interface{}{"alert_resolved": false}
			}
			if component == "completed" {
				eventType = "effectiveness.assessment.completed"
				data["event_type"], data["reason"] = eventType, reason
			}
			raw, err := json.Marshal(map[string]interface{}{
				"version": "1.0", "event_type": eventType, "event_timestamp": timestamp,
				"event_category": "effectiveness", "event_action": "assessed", "event_outcome": "success",
				"actor_type": "service", "actor_id": "effectivenessmonitor", "correlation_id": id, "event_data": data,
			})
			Expect(err).NotTo(HaveOccurred())
			var event api.AuditEventRequest
			Expect(json.Unmarshal(raw, &event)).To(Succeed())
			postHistoryAudit2496(httpClient, ts.URL, &event)
		}
	}

	DescribeTable("IT-KA-2496-005 / IT-AA-2496-002: typed selection audits survive authenticated ingestion and correlation queries (BR-KA-016, BR-AUDIT-005, AU-3, CC7.2)",
		func(confidence float64, mode string) {
			required, matched, rcaConfidence := true, true, .93
			result := &katypes.InvestigationResult{
				RCASummary: "Memory pressure", Severity: "high", Confidence: confidence, RCAConfidence: &rcaConfidence,
				WorkflowID: "catalog-2496", WorkflowName: "increase-memory", WorkflowSelectionConfidence: &confidence,
				WorkflowVersion: "1.0.0", ActionType: "IncreaseMemory", ExecutionBundle: "ghcr.io/kubernaut/increase-memory:1.0.0",
				RepeatJustification:         &sharedtypes.RepeatJustification{EvidenceBasis: "New limit observed", PriorOutcomeRelevance: "Prior Full zero", ExpectedMechanism: "Increase limit"},
				RepeatJustificationRequired: &required, PriorWorkflowMatch: &matched, RepeatValidationOutcome: "accepted",
				HandoffDigest: "sha256:" + strings.Repeat("b", 64), PriorRemediationUIDs: []string{"trusted-prior-2496"},
			}
			if mode == "unknown" {
				result.WorkflowSelectionConfidence = nil
			}
			if mode == "review" || mode == dsITRejected2496 || mode == dsITRejectedAbsent2496 {
				result.HumanReviewNeeded, result.HumanReviewReason = true, "rca_incomplete"
			}
			if mode == dsITRejected2496 || mode == dsITRejectedAbsent2496 {
				result.WorkflowID, result.ProposedWorkflowID, result.RepeatValidationOutcome = "", "catalog-2496", dsITRejected2496
				result.RepeatJustification.ExpectedMechanism = ""
				result.Parameters = map[string]interface{}{"PRIOR": "private-audit-2496"}
				result.ExecutionBundle = "private-bundle-2496"
				if mode == dsITRejectedAbsent2496 {
					result.RepeatJustification = nil
				}
			}
			if mode == "no-workflow" {
				result = &katypes.InvestigationResult{RCASummary: "No suitable workflow", HumanReviewNeeded: true, HumanReviewReason: "no_matching_workflows"}
			}
			raw, err := json.Marshal(investigator.ResultToAuditJSON(result))
			Expect(err).NotTo(HaveOccurred())
			event := kaaudit.NewEvent(kaaudit.EventTypeResponseComplete, recentID)
			event.EventAction, event.EventOutcome = kaaudit.ActionResponseSent, kaaudit.OutcomeSuccess
			event.Data["response_data"] = string(raw)
			Expect(kaaudit.NewDSAuditStore(client).StoreAudit(ctx, event)).To(Succeed())

			adapter, err := sharedaudit.NewOpenAPIClientAdapterWithTransport(ts.URL, 15*time.Second, httpClient.Transport)
			Expect(err).NotTo(HaveOccurred())
			store, err := sharedaudit.NewBufferedStore(adapter, sharedaudit.DefaultConfig(), "aianalysis-2496", logger)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { Expect(store.Close()).To(Succeed()) })
			analysis := &aianalysisv1.AIAnalysis{ObjectMeta: metav1.ObjectMeta{Name: "analysis-" + recentID, Namespace: "prod"},
				Spec: aianalysisv1.AIAnalysisSpec{RemediationID: recentID, RemediationRequestRef: corev1.ObjectReference{Name: recentID}, ClusterID: cluster}}
			auditClient := aiaudit.NewAuditClient(store, logger)
			_, err = handlers.NewResponseProcessor(logger, aimetrics.NewMetrics(), auditClient).ProcessAgentSessionResult(ctx, analysis,
				agentsession.MapInvestigationResultToAgentSessionResult(logger, result, recentID))
			Expect(err).NotTo(HaveOccurred())
			// Terminal response paths already call the real audit emitter. Exercise
			// the other public emitter once, without fabricating duplicate events.
			if analysis.Status.Phase != aianalysisv1.PhaseCompleted {
				auditClient.RecordAnalysisComplete(ctx, analysis)
			}
			if analysis.Status.Phase != aianalysisv1.PhaseFailed {
				Expect(auditClient.RecordAnalysisFailed(ctx, analysis, errors.New("investigation requires review"))).To(Succeed())
			}
			Expect(store.Flush(ctx)).To(Succeed())

			for _, eventType := range []string{kaaudit.EventTypeResponseComplete, aiaudit.EventTypeAnalysisCompleted, aiaudit.EventTypeAnalysisFailed} {
				queried, err := client.QueryAuditEvents(ctx, api.QueryAuditEventsParams{CorrelationID: api.NewOptString(recentID), EventType: api.NewOptString(eventType)})
				Expect(err).NotTo(HaveOccurred())
				Expect(queried.Data).To(HaveLen(1), eventType)
				persisted := queried.Data[0]
				Expect(persisted.CorrelationID).To(Equal(recentID))
				actor, hasActor := persisted.ActorID.Get()
				Expect(hasActor).To(BeTrue())
				Expect(actor).NotTo(BeEmpty())
				actorType, hasActorType := persisted.ActorType.Get()
				Expect(hasActorType).To(BeTrue())
				Expect(actorType).To(Equal("service"))
				eventID, hasEventID := persisted.EventID.Get()
				Expect(hasEventID).To(BeTrue())
				Expect(eventID).NotTo(Equal(uuid.Nil))
				Expect(persisted.EventAction).NotTo(BeEmpty())
				Expect(persisted.EventOutcome).NotTo(BeEmpty())
				raw, err = json.Marshal(persisted.EventData)
				Expect(err).NotTo(HaveOccurred())
				var payload map[string]json.RawMessage
				Expect(json.Unmarshal(raw, &payload)).To(Succeed())
				if eventType == kaaudit.EventTypeResponseComplete {
					response := payload["response_data"]
					payload = nil
					Expect(json.Unmarshal(response, &payload)).To(Succeed())
				}
				if mode == "no-workflow" {
					Expect(payload).NotTo(HaveKey("selectedWorkflow"))
					Expect(payload).NotTo(HaveKey("workflow_id"))
					Expect(payload).NotTo(HaveKey("selection_assessment"))
					continue
				}
				selection := payload
				if result.ProposedWorkflowID != "" {
					Expect(payload).NotTo(HaveKey("selectedWorkflow"))
					Expect(payload).NotTo(HaveKey("workflow_id"))
					Expect(payload).To(HaveKey("selection_assessment"))
					Expect(json.Unmarshal(payload["selection_assessment"], &selection)).To(Succeed())
					Expect(string(selection["proposed_workflow_id"])).To(Equal(`"catalog-2496"`))
					Expect(string(raw)).NotTo(ContainSubstring("private-"))
				} else if eventType == kaaudit.EventTypeResponseComplete {
					Expect(json.Unmarshal(payload["selectedWorkflow"], &selection)).To(Succeed())
					Expect(string(selection["workflowId"])).To(Equal(`"catalog-2496"`))
					Expect(string(selection["workflowName"])).To(Equal(`"increase-memory"`))
				} else {
					Expect(string(selection["workflow_id"])).To(Equal(`"catalog-2496"`))
					Expect(string(selection["workflow_name"])).To(Equal(`"increase-memory"`))
				}
				if mode == "unknown" {
					Expect(selection).NotTo(HaveKey("workflow_selection_confidence"))
				} else {
					var actual float64
					Expect(json.Unmarshal(selection["workflow_selection_confidence"], &actual)).To(Succeed())
					Expect(actual).To(BeNumerically("~", confidence, 0.000001))
				}
				if result.RepeatJustification == nil {
					Expect(selection).NotTo(HaveKey("repeat_justification"), "missing reasoning must not suppress the rejected terminal audit")
				} else {
					Expect(string(selection["repeat_justification"])).To(ContainSubstring("New limit observed"))
				}
				Expect(string(selection["repeat_justification_required"])).To(Equal("true"))
				Expect(string(selection["prior_workflow_match"])).To(Equal("true"))
				Expect(string(selection["repeat_validation_outcome"])).To(Equal(`"` + result.RepeatValidationOutcome + `"`))
				Expect(string(selection["handoff_digest"])).To(Equal(`"` + result.HandoffDigest + `"`))
				Expect(string(selection["prior_remediation_uids"])).To(MatchJSON(`["trusted-prior-2496"]`))
			}
		},
		Entry("selected success", .84, "success"), Entry("zero review", 0., "review"), Entry("partial review", .84, "review"),
		Entry("unknown confidence", .84, "unknown"), Entry("rejected incomplete", .84, dsITRejected2496), Entry("no workflow", 0., "no-workflow"),
		Entry("rejected without submitted reasoning", .84, dsITRejectedAbsent2496),
	)

	It("IT-DS-2496-001: both authenticated tiers preserve producer metadata, known zero and immutable incomplete rows", func() {
		writeRO(recentID, target, cluster, 0, recorded)
		writeAssessment(recentID, "Full", recorded.Add(time.Minute))
		writeRO(historicalID, target, cluster, .94, recorded.Add(-2*time.Hour))
		writeAssessment(historicalID, "SpecDrift", recorded.Add(-2*time.Hour+time.Minute))
		legacy := &repository.AuditEvent{
			EventID: uuid.New(), EventTimestamp: recorded.Add(-3 * time.Hour), EventType: "remediation.workflow_created", Version: "1.0",
			EventCategory: "orchestration", EventAction: "workflow_created", EventOutcome: "success", CorrelationID: oldID,
			ActorType: "service", ActorID: "remediationorchestrator", ClusterID: cluster,
			EventData: map[string]interface{}{"target_resource": target, "pre_remediation_spec_hash": params.CurrentSpecHash,
				"action_type": "IncreaseMemory", "parameters": map[string]string{"PRIOR_PASSWORD_2496": "history-sensitive-2496-value"}},
		}
		_, err := repository.NewAuditEventsRepository(db.DB, logger).Create(ctx, legacy)
		Expect(err).NotTo(HaveOccurred())
		before := snapshotHistoryAudit2496(oldID)
		Expect(before).To(ContainSubstring("history-sensitive-2496-value"))

		res, err := client.GetRemediationHistoryContext(ctx, params)
		Expect(err).NotTo(HaveOccurred())
		history, ok := res.(*api.RemediationHistoryContext)
		Expect(ok).To(BeTrue(), "authenticated generated-client response must be the complete success DTO")
		Expect(history.Tier1.Chain).To(HaveLen(1))
		Expect(history.Tier2.Chain).To(HaveLen(2))
		recent := history.Tier1.Chain[0]
		Expect(recent.WorkflowId).To(Equal(api.NewOptString("f125dbf3-a40f-4faa-a4e7-c3c480dd77cc")))
		Expect(recent.WorkflowName).To(Equal(api.NewOptString("IncreaseMemoryLimits")))
		Expect(recent.WorkflowSelectionConfidence.Set).To(BeTrue())
		Expect(recent.WorkflowSelectionConfidence.Value).To(BeZero())
		Expect(recent.EffectivenessScore.Set).To(BeTrue())
		Expect(recent.EffectivenessScore.Null).To(BeFalse())
		Expect(recent.EffectivenessScore.Value).To(BeZero())
		Expect(recent.SignalResolved.Set).To(BeTrue())
		Expect(recent.SignalResolved.Value).To(BeFalse())
		Expect(recent.CompletedAt).To(BeTemporally("==", recorded))
		Expect(history.Tier2.Chain[0].RemediationUID).To(Equal(oldID))
		Expect(history.Tier2.Chain[0].WorkflowId.Set).To(BeFalse())
		Expect(history.Tier2.Chain[0].WorkflowName.Set).To(BeFalse())
		Expect(history.Tier2.Chain[0].WorkflowSelectionConfidence.Set).To(BeFalse())
		Expect(history.Tier2.Chain[1].WorkflowSelectionConfidence.Value).To(Equal(.94))
		Expect(history.Tier2.Chain[1].AssessmentReason.Value).To(Equal(api.RemediationHistorySummaryAssessmentReasonSpecDrift))
		Expect(history.Tier2.Chain[1].EffectivenessScore.Value).To(BeZero(), "existing SpecDrift score representation is retained; its reason invalidates the evidence")
		Expect(snapshotHistoryAudit2496(oldID)).To(Equal(before), "payload and chain hashes remain immutable")
		gaps := logs.FilterMessage("Remediation history selection metadata incomplete").All()
		Expect(gaps).To(HaveLen(1))
		Expect(gaps[0].ContextMap()).To(HaveKeyWithValue("affected_record_count", int64(1)))
		Expect(gaps[0].ContextMap()).To(HaveKeyWithValue("backfill_attempted", false))
		Expect(gaps[0].ContextMap()).To(HaveKeyWithValue("cluster_id", cluster))
		Expect(gaps[0].ContextMap()).To(HaveKeyWithValue("request_id", Not(BeEmpty())))
		Expect(gaps[0].ContextMap()).To(HaveKeyWithValue("missing_fields", ConsistOf("workflow_id", "workflow_name", "workflow_selection_confidence")))
		raw, err := json.Marshal(history)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).NotTo(ContainSubstring("PRIOR_PASSWORD_2496"))
		Expect(string(raw)).NotTo(ContainSubstring("history-sensitive-2496-value"))
		Expect(fmt.Sprint(logs.All())).NotTo(ContainSubstring("history-sensitive-2496-value"))
	})

	It("IT-DS-2496-002: identical workflow/spec/name on other targets and origins cannot enter authenticated history", func() {
		writeRO(recentID, target, cluster, .84, recorded)
		writeRO("rr-sibling-"+recentID, target, "other-"+cluster, .99, recorded)
		writeRO("rr-target-"+recentID, target+"-other", cluster, .98, recorded)
		res, err := client.GetRemediationHistoryContext(ctx, params)
		Expect(err).NotTo(HaveOccurred())
		history, ok := res.(*api.RemediationHistoryContext)
		Expect(ok).To(BeTrue())
		Expect(history.Tier1.Chain).To(HaveLen(1))
		Expect(history.Tier1.Chain[0].RemediationUID).To(Equal(recentID))
		Expect(history.Tier1.Chain[0].WorkflowSelectionConfidence.Value).To(Equal(.84))
		Expect(history.Tier2.Chain).To(BeEmpty())
		Expect(logs.FilterMessage("Remediation history selection metadata incomplete").Len()).To(BeZero(), "pending effectiveness is not an identity-metadata gap")
		response, err := http.Get(ts.URL + "/api/v1/remediation-history/context")
		Expect(err).NotTo(HaveOccurred())
		Expect(response.Body.Close()).To(Succeed())
		Expect(response.StatusCode).To(Equal(http.StatusUnauthorized), "real middleware must protect history reads")
	})
})

func postHistoryAudit2496(client *http.Client, baseURL string, event *api.AuditEventRequest) {
	GinkgoHelper()
	raw, err := json.Marshal(event)
	Expect(err).NotTo(HaveOccurred())
	resp, err := client.Post(baseURL+"/api/v1/audit/events", "application/json", bytes.NewReader(raw))
	Expect(err).NotTo(HaveOccurred())
	body, readErr := io.ReadAll(resp.Body)
	Expect(resp.Body.Close()).To(Succeed())
	Expect(readErr).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated), "authenticated typed ingestion failed: %s", body)
}

func snapshotHistoryAudit2496(correlationID string) string {
	GinkgoHelper()
	var snapshot string
	err := db.GetContext(ctx, &snapshot, `SELECT event_data::text || '|' || event_hash || '|' || previous_event_hash FROM audit_events WHERE correlation_id=$1 AND event_type='remediation.workflow_created'`, correlationID)
	Expect(err).NotTo(HaveOccurred())
	return snapshot
}
