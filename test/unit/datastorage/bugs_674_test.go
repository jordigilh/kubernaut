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
	"context"
	"strings"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/query"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
)

// bug674LogEntry captures a single structured log message for assertion.
type bug674LogEntry struct {
	msg    string
	fields map[string]interface{}
}

// bug674SpyLogSink captures log output for test assertions on functions
// that communicate results via logging (e.g. ValidateMemoryConfiguration).
type bug674SpyLogSink struct {
	entries []bug674LogEntry
}

func (s *bug674SpyLogSink) Init(_ logr.RuntimeInfo)  {}
func (s *bug674SpyLogSink) Enabled(_ int) bool        { return true }
func (s *bug674SpyLogSink) Error(_ error, _ string, _ ...interface{}) {}
func (s *bug674SpyLogSink) WithValues(_ ...interface{}) logr.LogSink  { return s }
func (s *bug674SpyLogSink) WithName(_ string) logr.LogSink           { return s }

func (s *bug674SpyLogSink) Info(_ int, msg string, keysAndValues ...interface{}) {
	entry := bug674LogEntry{msg: msg, fields: make(map[string]interface{})}
	for i := 0; i+1 < len(keysAndValues); i += 2 {
		if key, ok := keysAndValues[i].(string); ok {
			entry.fields[key] = keysAndValues[i+1]
		}
	}
	s.entries = append(s.entries, entry)
}

func (s *bug674SpyLogSink) hasMessage(substring string) bool {
	for _, e := range s.entries {
		if strings.Contains(e.msg, substring) {
			return true
		}
	}
	return false
}

// Issue #674: 11 latent bugs found during TP-668 coverage audit.
// TDD RED phase — all tests targeting buggy behavior MUST FAIL before fixes.
var _ = Describe("Issue #674: Latent bug fixes (BR-STORAGE-010, BR-STORAGE-020)", func() {

	// =========================================================================
	// Bug 1: convertToStandardPlaceholders corrupts SQL with >10 params
	// =========================================================================
	Describe("Bug 1: SQL placeholder format (UT-DS-674-001..002)", func() {

		It("UT-DS-674-001: Build with all filters emits native PostgreSQL $N placeholders", func() {
			b := query.NewBuilder().
				WithNamespace("ns").
				WithSignalName("sig").
				WithSeverity("critical").
				WithCluster("prod").
				WithEnvironment("production").
				WithActionType("restart")

			sql, args, err := b.Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(args).To(HaveLen(8)) // 6 filters + LIMIT + OFFSET
			Expect(sql).To(ContainSubstring("$1"), "Build should return native $N placeholders")
			Expect(sql).ToNot(ContainSubstring("?"), "Build should not convert to ? placeholders")
		})

		It("UT-DS-674-002: BuildCount with all filters emits native PostgreSQL $N placeholders", func() {
			b := query.NewBuilder().
				WithNamespace("ns").
				WithSignalName("sig").
				WithSeverity("critical").
				WithCluster("prod").
				WithEnvironment("production").
				WithActionType("restart")

			sql, args, err := b.BuildCount()
			Expect(err).ToNot(HaveOccurred())
			Expect(args).To(HaveLen(6)) // 6 filters, no LIMIT/OFFSET
			Expect(sql).To(ContainSubstring("$1"), "BuildCount should return native $N placeholders")
			Expect(sql).ToNot(ContainSubstring("?"), "BuildCount should not convert to ? placeholders")
		})
	})

	// =========================================================================
	// Bug 2: NewValidatorWithRules silently discards custom rules
	// =========================================================================
	Describe("Bug 2: Validator respects custom rules (UT-DS-674-005..006)", func() {

		It("UT-DS-674-005: custom MaxNameLength=10 rejects names longer than 10 chars", func() {
			rules := &validation.ValidationRules{
				MaxNameLength:       10,
				MaxNamespaceLength:  255,
				MaxActionTypeLength: 100,
				ValidPhases:         []string{"pending", "processing", "completed", "failed"},
				ValidStatuses:       []string{"success", "failure", "pending", "running"},
			}
			v := validation.NewValidatorWithRules(logr.Discard(), rules)

			auditRecord := &models.RemediationAudit{
				Name:       "a-very-long-name-exceeding-ten",
				Namespace:  "default",
				Phase:      "pending",
				ActionType: "restart",
				Status:     "success",
			}
			err := v.ValidateRemediationAudit(auditRecord)
			Expect(err).To(HaveOccurred(), "custom MaxNameLength=10 should reject 30-char name")
			Expect(err.Error()).To(ContainSubstring("name"))
		})

		It("UT-DS-674-006: custom ValidPhases rejects phase not in custom set", func() {
			rules := &validation.ValidationRules{
				MaxNameLength:       255,
				MaxNamespaceLength:  255,
				MaxActionTypeLength: 100,
				ValidPhases:         []string{"alpha", "beta"},
				ValidStatuses:       []string{"success"},
			}
			v := validation.NewValidatorWithRules(logr.Discard(), rules)

			auditRecord := &models.RemediationAudit{
				Name:       "test",
				Namespace:  "default",
				Phase:      "pending", // In hardcoded set but NOT in custom {"alpha","beta"}
				ActionType: "restart",
				Status:     "success",
			}
			err := v.ValidateRemediationAudit(auditRecord)
			Expect(err).To(HaveOccurred(), "custom ValidPhases should reject 'pending' when only alpha/beta allowed")
		})
	})

	// =========================================================================
	// Bug 3: ActionTrace.Validate() is a no-op
	// =========================================================================
	Describe("Bug 3: ActionTrace.Validate enforces struct tags (UT-DS-674-008..010)", func() {

		It("UT-DS-674-008: empty ActionID fails validation", func() {
			trace := &models.ActionTrace{
				ActionID:        "",
				ActionType:      "restart",
				ActionTimestamp: time.Now(),
				Status:          "completed",
			}
			err := trace.Validate()
			Expect(err).To(HaveOccurred(), "empty ActionID should fail required validation")
		})

		It("UT-DS-674-009: empty ActionType fails validation", func() {
			trace := &models.ActionTrace{
				ActionID:        "act-001",
				ActionType:      "",
				ActionTimestamp: time.Now(),
				Status:          "completed",
			}
			err := trace.Validate()
			Expect(err).To(HaveOccurred(), "empty ActionType should fail required validation")
		})

		It("UT-DS-674-010: valid ActionTrace passes validation", func() {
			trace := &models.ActionTrace{
				ActionID:        "act-001",
				ActionType:      "restart",
				ActionTimestamp: time.Now(),
				Status:          "completed",
			}
			err := trace.Validate()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	// =========================================================================
	// Bug 5: ParseTimeParam produces future timestamps for negative day inputs
	// =========================================================================
	Describe("Bug 5: ParseTimeParam negative day handling (UT-DS-674-011..014)", func() {

		It("UT-DS-674-011: -1d returns error for negative day input", func() {
			_, err := query.ParseTimeParam("-1d")
			Expect(err).To(HaveOccurred(), "negative day input should be rejected")
		})

		It("UT-DS-674-012: -7d returns error for negative day input", func() {
			_, err := query.ParseTimeParam("-7d")
			Expect(err).To(HaveOccurred(), "negative day input should be rejected")
		})

		It("UT-DS-674-013: 7d returns timestamp approximately 7 days in the past", func() {
			result, err := query.ParseTimeParam("7d")
			Expect(err).ToNot(HaveOccurred())
			expected := time.Now().Add(-7 * 24 * time.Hour)
			Expect(result).To(BeTemporally("~", expected, 5*time.Second))
		})

		It("UT-DS-674-014: 1d returns timestamp approximately 24 hours in the past", func() {
			result, err := query.ParseTimeParam("1d")
			Expect(err).ToNot(HaveOccurred())
			expected := time.Now().Add(-24 * time.Hour)
			Expect(result).To(BeTemporally("~", expected, 5*time.Second))
		})
	})

	// =========================================================================
	// Bug 6: Workflow status filtering is case-sensitive
	// =========================================================================
	Describe("Bug 6: Case-insensitive workflow status predicates (UT-DS-674-015..017)", func() {

		It("UT-DS-674-015: IsActive returns true for lowercase 'active'", func() {
			wf := &models.RemediationWorkflow{Status: "active"}
			Expect(wf.IsActive()).To(BeTrue(), "lowercase 'active' should match")
		})

		It("UT-DS-674-016: IsActive returns true for canonical 'Active'", func() {
			wf := &models.RemediationWorkflow{Status: "Active"}
			Expect(wf.IsActive()).To(BeTrue(), "canonical 'Active' should match")
		})

		It("UT-DS-674-017: IsActive returns true for uppercase 'ACTIVE'", func() {
			wf := &models.RemediationWorkflow{Status: "ACTIVE"}
			Expect(wf.IsActive()).To(BeTrue(), "uppercase 'ACTIVE' should match")
		})
	})

	// =========================================================================
	// Bug 7: NewWorkflowCreatedAuditEvent(nil) panics
	// =========================================================================
	Describe("Bug 7: Nil workflow audit event safety (UT-DS-674-018..019)", func() {

		It("UT-DS-674-018: nil workflow returns error without panic", func() {
			Expect(func() {
				evt, err := audit.NewWorkflowCreatedAuditEvent(nil)
				Expect(err).To(HaveOccurred(), "nil workflow should return error")
				Expect(evt).To(BeNil())
			}).ToNot(Panic(), "nil workflow must not cause panic")
		})

		It("UT-DS-674-019: valid workflow returns audit event", func() {
			wf := &models.RemediationWorkflow{
				WorkflowID:      "12345678-1234-1234-1234-123456789012",
				WorkflowName:    "test-workflow",
				Version:         "v1.0.0",
				Status:          "Active",
				Name:            "Test Workflow",
				ExecutionEngine: models.ExecutionEngineTekton,
			}
			evt, err := audit.NewWorkflowCreatedAuditEvent(wf)
			Expect(err).ToNot(HaveOccurred())
			Expect(evt.EventType).To(Equal(audit.EventTypeWorkflowCreated))
		})
	})

	// =========================================================================
	// Bug 8: parsePostgreSQLSize misinterprets bare integers as MB
	// =========================================================================
	Describe("Bug 8: PostgreSQL bare integer size parsing (UT-DS-674-020..022)", func() {

		It("UT-DS-674-020: bare integer '16384' interpreted as 8kB blocks (128 MB), not 16384 MB", func() {
			db, mock, err := sqlmock.New()
			Expect(err).ToNot(HaveOccurred())
			defer db.Close()

			mock.ExpectQuery("SELECT current_setting").WillReturnRows(
				sqlmock.NewRows([]string{"current_setting"}).AddRow("16384"),
			)

			spy := &bug674SpyLogSink{}
			logger := logr.New(spy)
			v := schema.NewVersionValidator(db, logger)

			err = v.ValidateMemoryConfiguration(context.Background())
			Expect(err).ToNot(HaveOccurred())

			// 16384 as 8kB blocks = 128 MB < 1 GB recommended → should warn
			// Bug: current code interprets as 16384 MB (16 GB) → logs "optimal"
			Expect(spy.hasMessage("below recommended")).To(BeTrue(),
				"bare integer 16384 should be 128 MB (8kB blocks), triggering below-recommended warning")
		})

		It("UT-DS-674-021: '128MB' shared_buffers triggers below-recommended warning", func() {
			db, mock, err := sqlmock.New()
			Expect(err).ToNot(HaveOccurred())
			defer db.Close()

			mock.ExpectQuery("SELECT current_setting").WillReturnRows(
				sqlmock.NewRows([]string{"current_setting"}).AddRow("128MB"),
			)

			spy := &bug674SpyLogSink{}
			logger := logr.New(spy)
			v := schema.NewVersionValidator(db, logger)

			err = v.ValidateMemoryConfiguration(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(spy.hasMessage("below recommended")).To(BeTrue(),
				"128MB < 1GB should trigger below-recommended warning")
		})

		It("UT-DS-674-022: '1GB' shared_buffers logs optimal", func() {
			db, mock, err := sqlmock.New()
			Expect(err).ToNot(HaveOccurred())
			defer db.Close()

			mock.ExpectQuery("SELECT current_setting").WillReturnRows(
				sqlmock.NewRows([]string{"current_setting"}).AddRow("1GB"),
			)

			spy := &bug674SpyLogSink{}
			logger := logr.New(spy)
			v := schema.NewVersionValidator(db, logger)

			err = v.ValidateMemoryConfiguration(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(spy.hasMessage("optimal")).To(BeTrue(),
				"1GB >= 1GB should log optimal")
		})
	})

	// =========================================================================
	// Bug 10: ValidateRemediationAudit never validates Status field
	// =========================================================================
	Describe("Bug 10: Status field validation (UT-DS-674-025..027)", func() {

		It("UT-DS-674-025: invalid Status is rejected", func() {
			v := validation.NewValidator(logr.Discard())
			auditRecord := &models.RemediationAudit{
				Name:       "test",
				Namespace:  "default",
				Phase:      "pending",
				ActionType: "restart",
				Status:     "BOGUS_STATUS",
			}
			err := v.ValidateRemediationAudit(auditRecord)
			Expect(err).To(HaveOccurred(), "invalid Status should be rejected")
			Expect(err.Error()).To(ContainSubstring("status"))
		})

		It("UT-DS-674-026: valid Status is accepted", func() {
			v := validation.NewValidator(logr.Discard())
			auditRecord := &models.RemediationAudit{
				Name:       "test",
				Namespace:  "default",
				Phase:      "pending",
				ActionType: "restart",
				Status:     "success",
			}
			err := v.ValidateRemediationAudit(auditRecord)
			Expect(err).ToNot(HaveOccurred())
		})

		It("UT-DS-674-027: DefaultRules contains all expected valid statuses", func() {
			rules := validation.DefaultRules()
			Expect(rules.ValidStatuses).To(ContainElements("success", "failure", "pending", "running"))
		})
	})
})
