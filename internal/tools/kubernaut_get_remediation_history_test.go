package tools_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/ds"
	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

var _ = Describe("kubernaut_get_remediation_history", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-122-001: returns history", func() {
		mock := &ds.MockClient{
			GetRemediationHistoryFn: func(ctx context.Context, opts ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
				return []ds.HistoricalRemediation{
					{ID: "rr-hist-1", Namespace: "payments", Phase: "Completed"},
				}, nil
			},
		}
		result, err := tools.HandleGetRemediationHistory(ctx, mock, tools.GetRemediationHistoryArgs{Namespace: "payments"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
	})

	It("UT-AF-122-002: no history", func() {
		mock := &ds.MockClient{
			GetRemediationHistoryFn: func(ctx context.Context, opts ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
				return nil, nil
			},
		}
		result, err := tools.HandleGetRemediationHistory(ctx, mock, tools.GetRemediationHistoryArgs{Namespace: "empty"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
	})

	It("UT-AF-122-003: DS unavailable", func() {
		mock := &ds.MockClient{
			GetRemediationHistoryFn: func(ctx context.Context, opts ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
				return nil, fmt.Errorf("connection refused")
			},
		}
		_, err := tools.HandleGetRemediationHistory(ctx, mock, tools.GetRemediationHistoryArgs{Namespace: "pay"})
		Expect(err).To(HaveOccurred())
	})

	It("UT-AF-122-004: filter by date range", func() {
		mock := &ds.MockClient{
			GetRemediationHistoryFn: func(ctx context.Context, opts ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
				Expect(opts.Since).To(Equal("2026-01-01"))
				return []ds.HistoricalRemediation{{ID: "rr-1"}}, nil
			},
		}
		result, err := tools.HandleGetRemediationHistory(ctx, mock, tools.GetRemediationHistoryArgs{Since: "2026-01-01"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
	})
})
