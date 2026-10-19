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
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("Issue #2533 AIAnalysis decision context repository bridge", func() {
	var (
		db      *sql.DB
		sqlMock sqlmock.Sqlmock
		repo    *repository.RemediationHistoryRepository
	)

	BeforeEach(func() {
		var err error
		db, sqlMock, err = sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		Expect(err).NotTo(HaveOccurred())
		repo = repository.NewRemediationHistoryRepository(db, logr.Discard())
	})

	AfterEach(func() {
		Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
		_ = db.Close()
	})

	It("UT-DS-2533-001: performs one bounded cluster-scoped batch association and omits malformed context", func() {
		encoded, err := json.Marshal(map[string]interface{}{
			"event_type":         "aianalysis.analysis.completed",
			"analysis_name":      "aa-2533",
			"namespace":          "prod",
			"phase":              "Completed",
			"approval_required":  false,
			"degraded_mode":      false,
			"warnings_count":     0,
			"workflow_rationale": "The current target evidence supports the catalog workflow.",
			"repeat_justification": map[string]string{
				"evidence_basis":          "Current evidence basis",
				"prior_outcome_relevance": "The prior outcome was ineffective",
				"expected_mechanism":      "The workflow addresses the observed mechanism",
			},
		})
		Expect(err).NotTo(HaveOccurred())

		sqlMock.ExpectQuery(`(?s)SELECT DISTINCT ON \(correlation_id\) correlation_id, event_data.*ORDER BY correlation_id, event_timestamp DESC, event_id DESC`).
			WithArgs(sqlmock.AnyArg(), "cluster-2533", 2).
			WillReturnRows(sqlmock.NewRows([]string{"correlation_id", "event_data"}).
				AddRow("rr-known-2533", encoded).
				AddRow("rr-malformed-2533", []byte(`{"workflow_rationale":`)))

		result, err := repo.QueryAIAnalysisDecisionContextBatch(context.Background(),
			[]string{"rr-known-2533", "rr-known-2533", "", "rr-malformed-2533"}, "cluster-2533")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HaveKey("rr-known-2533"))
		Expect(result["rr-known-2533"].WorkflowRationale).To(Equal("The current target evidence supports the catalog workflow."))
		Expect(result["rr-known-2533"].RepeatJustification).To(HaveValue(Equal(sharedtypes.RepeatJustification{
			EvidenceBasis:         "Current evidence basis",
			PriorOutcomeRelevance: "The prior outcome was ineffective",
			ExpectedMechanism:     "The workflow addresses the observed mechanism",
		})))
		Expect(result).NotTo(HaveKey("rr-malformed-2533"))
	})

	It("UT-DS-2533-002: rejects a correlation batch over the bounded history cardinality", func() {
		correlationIDs := make([]string, repository.MaxAIAnalysisDecisionContextResults+1)
		for i := range correlationIDs {
			correlationIDs[i] = fmt.Sprintf("rr-overflow-2533-%d", i)
		}
		result, err := repo.QueryAIAnalysisDecisionContextBatch(context.Background(), correlationIDs, "cluster-2533")
		Expect(errors.Is(err, repository.ErrRemediationHistoryResourceLimit)).To(BeTrue())
		Expect(result).To(BeNil())
	})
})
