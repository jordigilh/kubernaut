/*
Copyright 2025 Jordi Gil.

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
	"context"
	"encoding/json"
	"strings"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/metrics"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/query"
	"github.com/jordigilh/kubernaut/pkg/datastorage/reconstruction"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Issue #668: raise UT-tier coverage for non-excluded pkg/datastorage paths
// (models, audit, config, metrics, schema, validation, query, reconstruction).

var _ = Describe("DataStorage issue #668 UT coverage", func() {

	Describe("ActionTrace model helpers (BR-STORAGE-031)", func() {
		It("BR-STORAGE-031-01 BR-STORAGE-031-02 exposes table name, validation, success flags, and incident context", func() {
			step := 3
			at := &models.ActionTrace{
				ActionID:           "a1",
				ActionType:         "restart_pod",
				ActionTimestamp:    time.Now().UTC(),
				Status:             "completed",
				IncidentType:       "cpu-spike",
				AlertName:          "",
				WorkflowID:         "wf-1",
				WorkflowVersion:    "v1",
				WorkflowStepNumber: &step,
				AISelectedWorkflow: true,
			}
			Expect(at.TableName()).To(Equal("resource_action_traces"))
			Expect(at.Validate()).To(Succeed())
			Expect(at.IsSuccessful()).To(BeTrue())
			Expect(at.IsFailed()).To(BeFalse())
			Expect(at.HasIncidentContext()).To(BeTrue())
			Expect(at.HasWorkflowContext()).To(BeTrue())
			Expect(at.GetAIExecutionMode()).To(Equal("catalog_selection"))
			Expect(at.GetEffectiveIncidentType()).To(Equal("cpu-spike"))

			at2 := &models.ActionTrace{Status: "failed", IncidentType: "", AlertName: "OOM"}
			Expect(at2.IsFailed()).To(BeTrue())
			Expect(at2.HasIncidentContext()).To(BeTrue())
			Expect(at2.GetEffectiveIncidentType()).To(Equal("OOM"))
			Expect(at2.HasWorkflowContext()).To(BeFalse())
		})

		It("BR-STORAGE-031-04 classifies AI execution modes including chained, manual, and unknown", func() {
			Expect((&models.ActionTrace{AIManualEscalation: true}).GetAIExecutionMode()).To(Equal("manual_escalation"))
			Expect((&models.ActionTrace{AIChainedWorkflows: true}).GetAIExecutionMode()).To(Equal("chained_workflows"))
			Expect((&models.ActionTrace{}).GetAIExecutionMode()).To(Equal("unknown"))
		})
	})

	Describe("RemediationWorkflow lifecycle predicates (BR-STORAGE-012)", func() {
		It("BR-STORAGE-012 maps catalog status strings to IsActive, IsDisabled, IsDeprecated, and IsArchived", func() {
			cases := []struct {
				status     string
				active     bool
				disabled   bool
				deprecated bool
				archived   bool
			}{
				{"Active", true, false, false, false},
				{"Disabled", false, true, false, false},
				{"Deprecated", false, false, true, false},
				{"Archived", false, false, false, true},
			}
			for _, c := range cases {
				w := &models.RemediationWorkflow{Status: c.status}
				Expect(w.IsActive()).To(Equal(c.active), c.status)
				Expect(w.IsDisabled()).To(Equal(c.disabled), c.status)
				Expect(w.IsDeprecated()).To(Equal(c.deprecated), c.status)
				Expect(w.IsArchived()).To(Equal(c.archived), c.status)
			}
		})
	})

	Describe("Workflow labels and description adapters (BR-WORKFLOW-004)", func() {
		It("BR-WORKFLOW-004 CustomLabels Value, Scan, IsEmpty, and constructors round-trip JSON", func() {
			cl := models.NewCustomLabels()
			Expect(cl.IsEmpty()).To(BeTrue())
			v, err := cl.Value()
			Expect(err).ToNot(HaveOccurred())
			Expect(v).To(Equal([]byte("{}")))

			cl["team"] = []string{"payments"}
			Expect(cl.IsEmpty()).To(BeFalse())
			raw, err := json.Marshal(map[string][]string{"team": {"payments"}})
			Expect(err).ToNot(HaveOccurred())
			var scanned models.CustomLabels
			Expect(scanned.Scan(raw)).To(Succeed())
			Expect(scanned["team"]).To(Equal([]string{"payments"}))
			Expect(scanned.Scan(nil)).To(Succeed())
			Expect(scanned).To(BeEmpty())

			Expect(models.NewMandatoryLabels([]string{"high"}, []string{"pod"}, []string{"prod"}, "P1").Component).To(Equal([]string{"pod"}))
			dl := models.NewDetectedLabels()
			Expect(dl.FailedDetections).To(BeEmpty())
			Expect(dl.IsEmpty()).To(BeTrue())
			dl.GitOpsManaged = true
			Expect(dl.IsEmpty()).To(BeFalse())
		})

		It("BR-WORKFLOW-004 StructuredDescription String, ToShared, FromSharedDescription, and Scan", func() {
			sd := models.StructuredDescription{What: "restart", WhenToUse: "OOM", WhenNotToUse: "none", Preconditions: "p"}
			Expect(sd.String()).To(Equal("restart"))
			shared := sd.ToShared()
			Expect(shared.What).To(Equal("restart"))
			round := models.FromSharedDescription(sharedtypes.StructuredDescription{
				What: "x", WhenToUse: "y", WhenNotToUse: "z", Preconditions: "p",
			})
			Expect(round.What).To(Equal("x"))

			b, err := json.Marshal(sd)
			Expect(err).ToNot(HaveOccurred())
			var scanned models.StructuredDescription
			Expect(scanned.Scan(b)).To(Succeed())
			Expect(scanned.WhenToUse).To(Equal("OOM"))
			Expect(scanned.Scan(nil)).To(Succeed())
		})

		It("BR-WORKFLOW-004 CustomLabels Scan rejects non-byte values", func() {
			var cl models.CustomLabels
			Expect(cl.Scan(123)).To(HaveOccurred())
		})

		It("BR-WORKFLOW-004 DetectedLabels IsEmpty ignores resourceQuotaConstrained only (current contract)", func() {
			d := &models.DetectedLabels{ResourceQuotaConstrained: true}
			Expect(d.IsEmpty()).To(BeTrue())
		})

		It("BR-WORKFLOW-004 MandatoryLabels and DetectedLabels Value and Scan round-trip JSONB", func() {
			ml := models.MandatoryLabels{
				Severity: []string{"high"}, Component: []string{"pod"},
				Environment: []string{"prod"}, Priority: "P1",
			}
			raw, err := ml.Value()
			Expect(err).ToNot(HaveOccurred())
			var ml2 models.MandatoryLabels
			Expect(ml2.Scan(raw)).To(Succeed())
			Expect(ml2.Component).To(Equal([]string{"pod"}))

			dl := &models.DetectedLabels{GitOpsManaged: true, GitOpsTool: "argocd"}
			dv, err := dl.Value()
			Expect(err).ToNot(HaveOccurred())
			var dl2 models.DetectedLabels
			Expect(dl2.Scan(dv)).To(Succeed())
			Expect(dl2.GitOpsManaged).To(BeTrue())
			Expect(dl2.GitOpsTool).To(Equal("argocd"))
		})

		It("BR-WORKFLOW-004 StructuredDescription Value marshals JSON for JSONB columns", func() {
			sd := models.StructuredDescription{What: "w", WhenToUse: "use", WhenNotToUse: "never", Preconditions: "p"}
			v, err := sd.Value()
			Expect(err).ToNot(HaveOccurred())
			b, ok := v.([]byte)
			Expect(ok).To(BeTrue())
			Expect(string(b)).To(ContainSubstring("use"))
		})
	})

	Describe("Workflow catalog audit event builders (BR-STORAGE-183)", func() {
		It("BR-STORAGE-183 builds workflow.created audit with labels and UUID workflow id", func() {
			wid := uuid.MustParse("11111111-1111-1111-1111-111111111111").String()
			wf := &models.RemediationWorkflow{
				WorkflowID:      wid,
				WorkflowName:    "oom-fix",
				Version:         "v1.0.0",
				SchemaVersion:   "1.0",
				Name:            "OOM fix",
				Description:     models.StructuredDescription{What: "mem", WhenToUse: "OOM"},
				Content:         "{}",
				ContentHash:     "a" + strings.Repeat("b", 63), // len 64
				ActionType:      "scale",
				Status:          "Active",
				ExecutionEngine: models.ExecutionEngineJob,
				Labels: *models.NewMandatoryLabels(
					[]string{"high"}, []string{"pod"}, []string{"prod"}, "P1",
				),
			}
			ev, err := audit.NewWorkflowCreatedAuditEvent(wf)
			Expect(err).ToNot(HaveOccurred())
			Expect(ev.EventType).To(Equal(audit.EventTypeWorkflowCreated))
		})

		It("BR-STORAGE-183 builds workflow.updated audit for mutable field changes", func() {
			fields := ogenclient.WorkflowCatalogUpdatedFields{}
			fields.SetStatus(ogenclient.NewOptString("Disabled"))
			ev, err := audit.NewWorkflowUpdatedAuditEvent("22222222-2222-2222-2222-222222222222", fields)
			Expect(err).ToNot(HaveOccurred())
			Expect(ev.EventType).To(Equal(audit.EventTypeWorkflowUpdated))
		})
	})

	Describe("ServerConfig timeouts (BR-STORAGE-028)", func() {
		It("BR-STORAGE-028 GetReadTimeout and GetWriteTimeout parse YAML durations with safe defaults", func() {
			sc := config.ServerConfig{ReadTimeout: "5s", WriteTimeout: "2m"}
			Expect(sc.GetReadTimeout()).To(Equal(5 * time.Second))
			Expect(sc.GetWriteTimeout()).To(Equal(2 * time.Minute))

			empty := config.ServerConfig{}
			Expect(empty.GetReadTimeout()).To(Equal(30 * time.Second))
			Expect(empty.GetWriteTimeout()).To(Equal(30 * time.Second))

			bad := config.ServerConfig{ReadTimeout: "not-a-duration", WriteTimeout: "also-bad"}
			Expect(bad.GetReadTimeout()).To(Equal(30 * time.Second))
			Expect(bad.GetWriteTimeout()).To(Equal(30 * time.Second))
		})
	})

	Describe("Metrics factory (BR-STORAGE-019)", func() {
		It("BR-STORAGE-019 NewMetrics returns histograms bound to the default registerer", func() {
			m := metrics.NewMetrics("kubernaut", "datastorage")
			m.WriteDuration.WithLabelValues("test").Observe(0.001)
			m.AuditLagSeconds.WithLabelValues("test").Observe(0.002)
		})

		It("BR-STORAGE-019 NewMetricsWithRegistry registers isolated histograms on a fresh registry", func() {
			reg := prometheus.NewRegistry()
			m := metrics.NewMetricsWithRegistry("kubernaut", "datastorage", reg)
			m.WriteDuration.WithLabelValues("audit_events").Observe(0.01)
			m.AuditLagSeconds.WithLabelValues("gateway").Observe(0.02)
			fam, err := reg.Gather()
			Expect(err).ToNot(HaveOccurred())
			Expect(len(fam)).To(BeNumerically(">=", 2))
		})
	})

	Describe("PostgreSQL version validator (BR-STORAGE-010)", func() {
		It("BR-STORAGE-010 ValidatePostgreSQLVersion rejects majors below DD-011 minimum", func() {
			db, mock, err := sqlmock.New()
			Expect(err).ToNot(HaveOccurred())
			defer db.Close()

			mock.ExpectQuery("SELECT version\\(\\)").
				WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow("PostgreSQL 15.3 on x86_64"))

			v := schema.NewVersionValidator(db, logr.Discard())
			err = v.ValidatePostgreSQLVersion(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("PostgreSQL version 15"))
			Expect(mock.ExpectationsWereMet()).To(Succeed())
		})

		It("BR-STORAGE-010 ValidatePostgreSQLVersion accepts supported PostgreSQL 16+", func() {
			db, mock, err := sqlmock.New()
			Expect(err).ToNot(HaveOccurred())
			defer db.Close()

			mock.ExpectQuery("SELECT version\\(\\)").
				WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow("PostgreSQL 16.4 on aarch64"))

			val := schema.NewVersionValidator(db, logr.Discard())
			Expect(val.ValidatePostgreSQLVersion(context.Background())).To(Succeed())
			Expect(mock.ExpectationsWereMet()).To(Succeed())
		})

		It("BR-STORAGE-010 ValidateMemoryConfiguration parses shared_buffers and tolerates parse failures", func() {
			db, mock, err := sqlmock.New()
			Expect(err).ToNot(HaveOccurred())
			defer db.Close()

			mock.ExpectQuery("SELECT current_setting\\('shared_buffers'\\)").
				WillReturnRows(sqlmock.NewRows([]string{"shared_buffers"}).AddRow("128MB"))

			v := schema.NewVersionValidator(db, logr.Discard())
			Expect(v.ValidateMemoryConfiguration(context.Background())).To(Succeed())
			Expect(mock.ExpectationsWereMet()).To(Succeed())

			db2, mock2, err := sqlmock.New()
			Expect(err).ToNot(HaveOccurred())
			defer db2.Close()
			mock2.ExpectQuery("SELECT current_setting\\('shared_buffers'\\)").
				WillReturnRows(sqlmock.NewRows([]string{"shared_buffers"}).AddRow("not-a-size"))
			v2 := schema.NewVersionValidator(db2, logr.Discard())
			Expect(v2.ValidateMemoryConfiguration(context.Background())).To(Succeed())
			Expect(mock2.ExpectationsWereMet()).To(Succeed())
		})
	})

	Describe("Validation rules wiring (BR-STORAGE-010)", func() {
		It("BR-STORAGE-010 DefaultRules returns expected limits and phase/status enums", func() {
			r := validation.DefaultRules()
			Expect(r.MaxNameLength).To(Equal(255))
			Expect(r.MaxNamespaceLength).To(Equal(255))
			Expect(r.MaxActionTypeLength).To(Equal(100))
			Expect(r.ValidPhases).To(ContainElements("pending", "completed"))
			Expect(r.ValidStatuses).To(ContainElement("success"))
		})

		It("BR-STORAGE-010 NewValidatorWithRules constructs a working validator instance", func() {
			rules := validation.DefaultRules()
			rules.MaxNameLength = 10
			v := validation.NewValidatorWithRules(logr.Discard(), rules)
			err := v.ValidateRemediationAudit(&models.RemediationAudit{
				Name: "ok", Namespace: "ns", Phase: "pending", ActionType: "restart",
			})
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("AuditEventsQueryBuilder (BR-STORAGE-022)", func() {
		It("BR-STORAGE-022 BR-STORAGE-023 BR-STORAGE-025 builds filtered SELECT and COUNT with bound parameters", func() {
			since := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
			until := since.Add(24 * time.Hour)
			b := query.NewAuditEventsQueryBuilder(query.WithAuditEventsLogger(logr.Discard())).
				WithCorrelationID("c1").
				WithEventType("gateway.signal.received").
				WithService("gateway").
				WithOutcome("success").
				WithSeverity("high").
				WithSince(since).
				WithUntil(until).
				WithLimit(50).
				WithOffset(10)
			sqlStr, args, err := b.Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(sqlStr).To(ContainSubstring("FROM audit_events"))
			Expect(sqlStr).To(ContainSubstring("LIMIT"))
			Expect(args).To(HaveLen(9))

			csql, cargs, err := b.BuildCount()
			Expect(err).ToNot(HaveOccurred())
			Expect(csql).To(ContainSubstring("COUNT(*)"))
			Expect(cargs).To(HaveLen(7))
		})

		It("BR-STORAGE-023 rejects out-of-range limit and negative offset on audit event queries", func() {
			_, _, err := query.NewAuditEventsQueryBuilder().WithLimit(0).Build()
			Expect(err).To(HaveOccurred())
			_, _, err = query.NewAuditEventsQueryBuilder().WithLimit(1001).Build()
			Expect(err).To(HaveOccurred())
			_, _, err = query.NewAuditEventsQueryBuilder().WithLimit(1).WithOffset(-1).Build()
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Remediation trace query Builder (BR-STORAGE-021)", func() {
		It("BR-STORAGE-021 BR-STORAGE-022 WithLogger and dimension filters feed BuildCount SQL", func() {
			logger := logr.Discard()
			b := query.NewBuilder(query.WithLogger(logger)).
				WithSignalName("HighCPU").
				WithSeverity("critical").
				WithCluster("c1").
				WithEnvironment("prod").
				WithActionType("restart_pod").
				WithNamespace("kube-system").
				WithLimit(10).
				WithOffset(0)
			sqlStr, args, err := b.BuildCount()
			Expect(err).ToNot(HaveOccurred())
			Expect(sqlStr).To(ContainSubstring("COUNT(*)"))
			Expect(sqlStr).To(ContainSubstring("signal_name"))
			Expect(args).To(HaveLen(6))
		})
	})

	Describe("ParseTimeParam (BR-STORAGE-022)", func() {
		It("BR-STORAGE-022 parses durations, day suffixes, RFC3339, datetime without TZ, and date-only inputs", func() {
			_, err := query.ParseTimeParam("")
			Expect(err).To(HaveOccurred())

			tDay, err := query.ParseTimeParam("2d")
			Expect(err).ToNot(HaveOccurred())
			Expect(time.Since(tDay)).To(BeNumerically("<", 49*time.Hour))

			tHour, err := query.ParseTimeParam("1h")
			Expect(err).ToNot(HaveOccurred())
			Expect(time.Since(tHour)).To(BeNumerically("<", 2*time.Hour))

			abs, err := query.ParseTimeParam("2026-04-01T12:00:00Z")
			Expect(err).ToNot(HaveOccurred())
			Expect(abs.UTC().Format(time.RFC3339)).To(Equal("2026-04-01T12:00:00Z"))

			local, err := query.ParseTimeParam("2026-04-01T10:00:00")
			Expect(err).ToNot(HaveOccurred())
			Expect(local.UTC().Hour()).To(Equal(10))

			dayOnly, err := query.ParseTimeParam("2026-04-02")
			Expect(err).ToNot(HaveOccurred())
			Expect(dayOnly.UTC().Format("2006-01-02")).To(Equal("2026-04-02"))

			_, err = query.ParseTimeParam("not-a-time")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("RemediationAuditResult mapping (BR-STORAGE-006)", func() {
		It("BR-STORAGE-006 ToRemediationAudit copies all scan fields into the API model", func() {
			end := time.Now().UTC().Add(time.Hour)
			dur := int64(42)
			errMsg := "boom"
			r := &query.RemediationAuditResult{
				ID: 7, Name: "n", Namespace: "ns", Phase: "completed", ActionType: "x",
				Status: "success", StartTime: time.Now().UTC(), EndTime: &end,
				Duration: &dur, RemediationRequestID: "rr1", SignalFingerprint: "fp",
				Severity: "high", Environment: "prod", ClusterName: "c1",
				TargetResource: "Pod/p1", ErrorMessage: &errMsg, Metadata: "{}",
				CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
			}
			m := r.ToRemediationAudit()
			Expect(m.ID).To(Equal(r.ID))
			Expect(m.ErrorMessage).To(Equal(&errMsg))
			Expect(m.Duration).To(Equal(&dur))
		})
	})

	Describe("Reconstruction ParseAuditEvent Gap #4 (BR-AUDIT-006)", func() {
		It("BR-AUDIT-006 aianalysis.analysis.completed maps provider summary into ProviderData JSON", func() {
			payload := ogenclient.AIAnalysisAuditPayload{
				EventType:        ogenclient.AIAnalysisAuditPayloadEventTypeAianalysisAnalysisCompleted,
				AnalysisName:     "aa-1",
				Namespace:        "default",
				Phase:            ogenclient.AIAnalysisAuditPayloadPhaseCompleted,
				ApprovalRequired: false,
				DegradedMode:     false,
				WarningsCount:    0,
				ProviderResponseSummary: ogenclient.NewOptProviderResponseSummary(ogenclient.ProviderResponseSummary{
					IncidentID:       "inc-1",
					AnalysisPreview:  "preview",
					NeedsHumanReview: false,
					WarningsCount:    0,
				}),
			}
			event := ogenclient.AuditEvent{
				EventType:     "aianalysis.analysis.completed",
				CorrelationID: "corr-aa",
				EventData:     ogenclient.NewAuditEventEventDataAianalysisAnalysisCompletedAuditEventEventData(payload),
			}
			parsed, err := reconstruction.ParseAuditEvent(event)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsed.ProviderData).To(ContainSubstring("inc-1"))
			Expect(parsed.ProviderData).To(ContainSubstring("preview"))
		})

		It("BR-AUDIT-006 aianalysis.analysis.completed tolerates missing provider summary", func() {
			payload := ogenclient.AIAnalysisAuditPayload{
				EventType:        ogenclient.AIAnalysisAuditPayloadEventTypeAianalysisAnalysisCompleted,
				AnalysisName:     "aa-2",
				Namespace:        "default",
				Phase:            ogenclient.AIAnalysisAuditPayloadPhaseCompleted,
				ApprovalRequired: false,
				DegradedMode:     false,
				WarningsCount:    0,
			}
			event := ogenclient.AuditEvent{
				EventType:     "aianalysis.analysis.completed",
				CorrelationID: "corr-aa-2",
				EventData:     ogenclient.NewAuditEventEventDataAianalysisAnalysisCompletedAuditEventEventData(payload),
			}
			parsed, err := reconstruction.ParseAuditEvent(event)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsed.ProviderData).To(Equal(""))
		})
	})
})
