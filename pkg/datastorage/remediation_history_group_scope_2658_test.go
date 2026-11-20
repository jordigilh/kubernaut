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
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

// UT-DS-2658-001 (repository half) / IT-DS-2658-301: the history predicate is a
// single exact-identity equality (#2658). Because group and version are ordinary
// parts of the identity, no cross-group or cross-version row can enter another
// target's chain, and no prefix/LIKE scan is needed.
var _ = Describe("QueryROEventsBySpecHash identity predicate (#2658) [BR-ORCH-042, SI-10]", func() {
	var (
		sqlMock sqlmock.Sqlmock
		repo    *repository.RemediationHistoryRepository
	)

	BeforeEach(func() {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		Expect(err).NotTo(HaveOccurred())
		sqlMock = mock
		repo = repository.NewRemediationHistoryRepository(db, kubelog.NewLogger(kubelog.DevelopmentOptions()))
	})

	AfterEach(func() {
		Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
	})

	emptyRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"event_type", "event_data", "event_timestamp", "correlation_id", "chain_depth"})
	}

	DescribeTable("matches one canonical identity by equality",
		func(identity string) {
			sqlMock.ExpectQuery(`ro\.event_data->>'target_resource' = \$1`).
				WithArgs(identity, "", "sha256:abc", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
				WillReturnRows(emptyRows())

			_, err := repo.QueryROEventsBySpecHash(context.Background(), repository.RemediationHistoryQuery{
				TargetResource: identity,
				SpecHash:       "sha256:abc",
				Since:          time.Now().Add(-time.Hour),
				Until:          time.Now(),
			})
			Expect(err).NotTo(HaveOccurred())
		},
		Entry("namespaced grouped", "prod/apps/v1/Ingress/web"),
		Entry("namespaced core", "prod/core/v1/Pod/api"),
		Entry("cluster-scoped", "metrics.k8s.io/v1/Node/worker-1"),
		Entry("unresolved", "prod/unresolved/unresolved/Route/web"),
	)

	It("applies the predicate to both the anchor and the recursive term", func() {
		// A multi-hop chain must not pick up another identity's hop (#2658 R6-4).
		sqlMock.ExpectQuery(`(?s)target_resource' = \$1.*UNION.*target_resource' = \$1`).
			WithArgs("prod/extensions/v1beta1/Ingress/web", "", "sha256:abc", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnRows(emptyRows())

		_, err := repo.QueryROEventsBySpecHash(context.Background(), repository.RemediationHistoryQuery{
			TargetResource: "prod/extensions/v1beta1/Ingress/web",
			SpecHash:       "sha256:abc",
			Since:          time.Now().Add(-time.Hour),
			Until:          time.Now(),
		})
		Expect(err).NotTo(HaveOccurred())
	})
})
