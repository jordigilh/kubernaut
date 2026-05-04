package tools_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/ds"
	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

var _ = Describe("kubernaut_get_effectiveness", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-123-001: returns scores", func() {
		mock := &ds.MockClient{
			GetEffectivenessFn: func(ctx context.Context, opts ds.EffectivenessOpts) (*ds.EffectivenessReport, error) {
				return &ds.EffectivenessReport{WorkflowID: "wf-1", SuccessRate: 0.85, SampleSize: 100}, nil
			},
		}
		result, err := tools.HandleGetEffectiveness(ctx, mock, tools.GetEffectivenessArgs{WorkflowID: "wf-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.SuccessRate).To(BeNumerically("~", 0.85))
		Expect(result.SampleSize).To(Equal(100))
	})

	It("UT-AF-123-002: no data", func() {
		mock := &ds.MockClient{
			GetEffectivenessFn: func(ctx context.Context, opts ds.EffectivenessOpts) (*ds.EffectivenessReport, error) {
				return &ds.EffectivenessReport{}, nil
			},
		}
		result, err := tools.HandleGetEffectiveness(ctx, mock, tools.GetEffectivenessArgs{WorkflowID: "unknown"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.SampleSize).To(Equal(0))
	})

	It("UT-AF-123-003: DS unavailable", func() {
		mock := &ds.MockClient{
			GetEffectivenessFn: func(ctx context.Context, opts ds.EffectivenessOpts) (*ds.EffectivenessReport, error) {
				return nil, fmt.Errorf("connection refused")
			},
		}
		_, err := tools.HandleGetEffectiveness(ctx, mock, tools.GetEffectivenessArgs{WorkflowID: "wf-1"})
		Expect(err).To(HaveOccurred())
	})

	It("UT-AF-123-004: filter by namespace", func() {
		mock := &ds.MockClient{
			GetEffectivenessFn: func(ctx context.Context, opts ds.EffectivenessOpts) (*ds.EffectivenessReport, error) {
				Expect(opts.Namespace).To(Equal("payments"))
				return &ds.EffectivenessReport{SuccessRate: 0.9, SampleSize: 50}, nil
			},
		}
		result, err := tools.HandleGetEffectiveness(ctx, mock, tools.GetEffectivenessArgs{Namespace: "payments"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.SampleSize).To(Equal(50))
	})
})
