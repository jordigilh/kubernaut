/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
*/

package auditconfidence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	_ "github.com/jackc/pgx/v5/stdlib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pressly/goose/v3"
	corev1 "k8s.io/api/core/v1"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	kaaudit "github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/cert"
	dsconfig "github.com/jordigilh/kubernaut/pkg/datastorage/config"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedauth "github.com/jordigilh/kubernaut/pkg/shared/auth"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

func TestAuditConfidence(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AIAnalysis confidence audit persistence")
}

// IT-AA-2586-005: production AA audit emitters and buffered store write to
// a fresh local PostgreSQL instance migrated with the real schema. AU-3 and
// CC7.2: a correlation query recovers the policy and decision inputs.
var _ = Describe("BR-AUDIT-005 confidence audit", func() {
	It("preserves absent and explicit zero RCA separately from workflow confidence", func() {
		ctx := context.Background()
		dir := GinkgoT().TempDir()
		freePort := func() int {
			listener, listenErr := net.Listen("tcp", "127.0.0.1:0")
			Expect(listenErr).NotTo(HaveOccurred())
			port := listener.Addr().(*net.TCPAddr).Port
			Expect(listener.Close()).To(Succeed())
			return port
		}
		postgresPort, redisPort := freePort(), freePort()
		postgresContainer := fmt.Sprintf("audit-confidence-postgres-%d", os.Getpid())
		redisContainer := fmt.Sprintf("audit-confidence-redis-%d", os.Getpid())
		removeContainer := func(name string) {
			if cleanupErr := exec.Command("podman", "rm", "-f", name).Run(); cleanupErr != nil {
				GinkgoWriter.Printf("cleanup of %s returned %v\n", name, cleanupErr)
			}
		}
		removeContainer(postgresContainer)
		removeContainer(redisContainer)
		Expect(infrastructure.StartPostgreSQL(ctx, infrastructure.PostgreSQLConfig{
			ContainerName: postgresContainer,
			Port:          postgresPort,
			DBName:        "postgres",
			DBUser:        "postgres",
			DBPassword:    "test_password",
		}, GinkgoWriter)).To(Succeed())
		DeferCleanup(func() { removeContainer(postgresContainer) })
		Expect(infrastructure.WaitForPostgreSQLReady(ctx, postgresContainer, "postgres", "postgres", GinkgoWriter)).To(Succeed())
		Expect(infrastructure.StartRedis(ctx, infrastructure.RedisConfig{
			ContainerName: redisContainer,
			Port:          redisPort,
		}, GinkgoWriter)).To(Succeed())
		DeferCleanup(func() { removeContainer(redisContainer) })
		Expect(infrastructure.WaitForRedisReady(ctx, redisContainer, GinkgoWriter)).To(Succeed())
		db, err := sql.Open("pgx", fmt.Sprintf("postgres://postgres:test_password@127.0.0.1:%d/postgres?sslmode=disable", postgresPort))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(db.Close()).To(Succeed()) })
		Expect(goose.SetDialect("postgres")).To(Succeed())
		Expect(goose.Up(db, filepath.Join("..", "..", "..", "..", "migrations"))).To(Succeed())
		Eventually(func() error {
			conn, dialErr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", redisPort), time.Second)
			if dialErr == nil {
				_ = conn.Close()
			}
			return dialErr
		}, 5*time.Second, 50*time.Millisecond).Should(Succeed())
		pair, err := cert.GenerateSelfSigned(cert.CertificateOptions{CommonName: "data-storage-integration", Organization: "Kubernaut", DNSNames: []string{"localhost"}, ValidityDuration: 24 * time.Hour, KeySize: 2048})
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(filepath.Join(dir, "tls.crt"), pair.CertPEM, 0600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(dir, "tls.key"), pair.KeyPEM, 0600)).To(Succeed())
		identity := "system:serviceaccount:datastorage-test:confidence-test"
		ds, err := server.NewServer(server.ServerDeps{
			DBConnStr: fmt.Sprintf("postgres://postgres:test_password@127.0.0.1:%d/postgres?sslmode=disable", postgresPort),
			RedisAddr: fmt.Sprintf("127.0.0.1:%d", redisPort), Logger: logr.Discard(), DLQMaxLen: 1000,
			AppConfig:     &dsconfig.Config{Server: dsconfig.ServerConfig{SignerCertDir: dir}, Database: dsconfig.DatabaseConfig{MaxOpenConns: 25, MaxIdleConns: 5, ConnMaxLifetime: "5m", ConnMaxIdleTime: "10m"}},
			ServerConfig:  &server.Config{Port: 18090, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, TLS: sharedtls.TLSConfig{CertDir: dir}},
			Authenticator: &sharedauth.MockAuthenticator{ValidUsers: map[string]string{"test-token": identity}},
			Authorizer:    &sharedauth.MockAuthorizer{AllowedUsers: map[string]bool{identity: true}}, AuthNamespace: "datastorage-test",
		})
		Expect(err).NotTo(HaveOccurred())
		httpServer := httptest.NewServer(ds.Handler())
		DeferCleanup(func() {
			httpServer.Close()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			Expect(ds.Shutdown(shutdownCtx)).To(Succeed())
		})
		dsClient, err := audit.NewOpenAPIClientAdapterWithTransport(httpServer.URL, 5*time.Second, testauth.NewStaticTokenTransport("test-token"))
		Expect(err).NotTo(HaveOccurred())
		queryClient, err := ogenclient.NewClient(httpServer.URL, ogenclient.WithClient(&http.Client{Timeout: 5 * time.Second, Transport: testauth.NewStaticTokenTransport("test-token")}))
		Expect(err).NotTo(HaveOccurred())
		store, err := audit.NewBufferedStore(dsClient, audit.DefaultConfig(), "aianalysis", logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(store.Close()).To(Succeed()) })
		client := aiaudit.NewAuditClient(store, logr.Discard())
		evaluator := rego.NewEvaluator(rego.Config{}, logr.Discard())
		Expect(evaluator.LoadPolicy(`package aianalysis.approval
import rego.v1
require_approval := true
reason := "user policy"`)).To(Succeed())
		handler := handlers.NewAnalyzingHandler(evaluator, logr.Discard(), metrics.NewMetrics(), client)
		zero, known := 0.0, 0.65
		for _, tc := range []struct {
			id    string
			score *float64
		}{{"rr-2586-missing", nil}, {"rr-2586-zero", &zero}, {"rr-2586-known", &known}} {
			analysis := &aianalysisv1.AIAnalysis{
				Spec:   aianalysisv1.AIAnalysisSpec{RemediationRequestRef: corev1.ObjectReference{Name: tc.id}},
				Status: aianalysisv1.AIAnalysisStatus{RCAResult: &aianalysisv1.RCAResult{RCAConfidence: tc.score, SelectedWorkflow: &aianalysisv1.SelectedWorkflow{Confidence: .95}}},
			}
			_, err = handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())
			Expect(analysis.Status.GetApproval().ApprovalRequired).To(BeTrue())
			flushCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			Expect(store.Flush(flushCtx)).To(Succeed())
			cancel()
			rows, queryErr := db.QueryContext(ctx, `SELECT event_type, event_data FROM audit_events WHERE correlation_id = $1 AND event_type IN ('aianalysis.rego.evaluation', 'aianalysis.approval.decision') ORDER BY event_timestamp`, tc.id)
			Expect(queryErr).NotTo(HaveOccurred())
			count := 0
			for rows.Next() {
				var eventType string
				var raw []byte
				Expect(rows.Scan(&eventType, &raw)).To(Succeed())
				var payload map[string]interface{}
				Expect(json.Unmarshal(raw, &payload)).To(Succeed())
				Expect(eventType).To(Or(Equal("aianalysis.rego.evaluation"), Equal("aianalysis.approval.decision")))
				Expect(payload).To(HaveKeyWithValue("workflow_confidence", .95))
				if tc.score == nil {
					Expect(payload).NotTo(HaveKey("rca_confidence"))
				} else {
					Expect(payload).To(HaveKeyWithValue("rca_confidence", *tc.score))
				}
				count++
			}
			Expect(rows.Err()).NotTo(HaveOccurred())
			Expect(rows.Close()).To(Succeed())
			Expect(count).To(Equal(2))
			response, queryErr := queryClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{CorrelationID: ogenclient.NewOptString(tc.id), Limit: ogenclient.NewOptInt(100)})
			Expect(queryErr).NotTo(HaveOccurred())
			found := 0
			for _, event := range response.Data {
				if event.EventType != "aianalysis.rego.evaluation" && event.EventType != "aianalysis.approval.decision" {
					continue
				}
				raw, marshalErr := json.Marshal(event.EventData)
				Expect(marshalErr).NotTo(HaveOccurred())
				var payload map[string]interface{}
				Expect(json.Unmarshal(raw, &payload)).To(Succeed())
				Expect(payload).To(HaveKeyWithValue("workflow_confidence", .95))
				if tc.score == nil {
					Expect(payload).NotTo(HaveKey("rca_confidence"))
				} else {
					Expect(payload).To(HaveKeyWithValue("rca_confidence", *tc.score))
				}
				found++
			}
			Expect(found).To(Equal(2))
			// IT-AA-2586-010: both terminal emitters survive real DS HTTP storage
			// and correlation reads without confusing missing and zero RCA.
			analysis.Status.Phase = aianalysisv1.PhaseCompleted
			client.RecordAnalysisComplete(ctx, analysis)
			Expect(client.RecordAnalysisFailed(ctx, analysis, errors.New("test failure"))).To(Succeed())
			flushCtx, cancel = context.WithTimeout(ctx, 10*time.Second)
			Expect(store.Flush(flushCtx)).To(Succeed())
			cancel()
			terminal, queryErr := queryClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{CorrelationID: ogenclient.NewOptString(tc.id), Limit: ogenclient.NewOptInt(100)})
			Expect(queryErr).NotTo(HaveOccurred())
			seenTerminal := make(map[string]bool)
			for _, event := range terminal.Data {
				if event.EventType != "aianalysis.analysis.completed" && event.EventType != "aianalysis.analysis.failed" {
					continue
				}
				seenTerminal[event.EventType] = true
				raw, err := json.Marshal(event.EventData)
				Expect(err).NotTo(HaveOccurred())
				var payload map[string]interface{}
				Expect(json.Unmarshal(raw, &payload)).To(Succeed())
				if tc.score == nil {
					Expect(payload).NotTo(HaveKey("rca_confidence"))
				} else {
					Expect(payload).To(HaveKeyWithValue("rca_confidence", *tc.score))
				}
				Expect(payload["confidence"]).To(BeNumerically("~", .95, .00001))
			}
			Expect(seenTerminal).To(HaveKey("aianalysis.analysis.completed"))
			Expect(seenTerminal).To(HaveKey("aianalysis.analysis.failed"))
		}

		// IT-KA-2586-031: real KA typed dispatch, DS HTTP storage and
		// correlation-ID readback preserve RCA presence for both completion events.
		kaStore := kaaudit.NewDSAuditStore(queryClient)
		for _, tc := range []struct {
			id    string
			score *float64
		}{{"rr-ka-2586-missing", nil}, {"rr-ka-2586-zero", &zero}, {"rr-ka-2586-known", &known}} {
			workflow := .95
			for _, eventType := range []string{kaaudit.EventTypeRCAComplete, kaaudit.EventTypeResponseComplete} {
				result := &katypes.InvestigationResult{RCASummary: "RCA", Severity: "high", Confidence: workflow, RCAConfidence: tc.score, WorkflowID: "workflow", WorkflowName: "workflow", WorkflowSelectionConfidence: &workflow}
				if eventType == kaaudit.EventTypeRCAComplete {
					result.WorkflowID, result.WorkflowName, result.WorkflowSelectionConfidence = "", "", nil
					result.Confidence = 0
					if tc.score != nil {
						result.Confidence = *tc.score
					}
				}
				raw, marshalErr := json.Marshal(investigator.ResultToAuditJSON(result))
				Expect(marshalErr).NotTo(HaveOccurred())
				event := kaaudit.NewEvent(eventType, tc.id)
				event.EventAction, event.EventOutcome = kaaudit.ActionResponseSent, kaaudit.OutcomeSuccess
				event.Data["response_data"] = string(raw)
				Expect(kaStore.StoreAudit(ctx, event)).To(Succeed())
			}
			Eventually(func(g Gomega) {
				response, queryErr := queryClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{CorrelationID: ogenclient.NewOptString(tc.id), Limit: ogenclient.NewOptInt(100)})
				g.Expect(queryErr).NotTo(HaveOccurred())
				g.Expect(response.Data).To(HaveLen(2))
				seen := make(map[string]bool)
				for _, event := range response.Data {
					seen[event.EventType] = true
					encoded, err := json.Marshal(event.EventData)
					g.Expect(err).NotTo(HaveOccurred())
					var payload struct {
						Response map[string]json.RawMessage `json:"response_data"`
					}
					g.Expect(json.Unmarshal(encoded, &payload)).To(Succeed())
					if tc.score == nil {
						g.Expect(payload.Response).NotTo(HaveKey("rca_confidence"))
					} else {
						var actual float64
						g.Expect(json.Unmarshal(payload.Response["rca_confidence"], &actual)).To(Succeed())
						g.Expect(actual).To(Equal(*tc.score))
					}
					if event.EventType == kaaudit.EventTypeRCAComplete {
						g.Expect(payload.Response).NotTo(HaveKey("selectedWorkflow"))
					} else {
						var selection map[string]json.RawMessage
						g.Expect(json.Unmarshal(payload.Response["selectedWorkflow"], &selection)).To(Succeed())
						g.Expect(string(selection["workflow_selection_confidence"])).To(MatchJSON("0.95"))
					}
				}
				g.Expect(seen).To(HaveKey(kaaudit.EventTypeRCAComplete))
				g.Expect(seen).To(HaveKey(kaaudit.EventTypeResponseComplete))
			}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
		}
	})
})
