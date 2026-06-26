package tools_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ds"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
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
		result, err := tools.HandleGetRemediationHistory(ctx, mock, tools.GetRemediationHistoryArgs{
			Kind: "Deployment", Name: "api", SpecHash: "sha256:test", Namespace: "payments",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
	})

	It("UT-AF-122-002: no history", func() {
		mock := &ds.MockClient{
			GetRemediationHistoryFn: func(ctx context.Context, opts ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
				return nil, nil
			},
		}
		result, err := tools.HandleGetRemediationHistory(ctx, mock, tools.GetRemediationHistoryArgs{
			Kind: "Deployment", Name: "api", SpecHash: "sha256:test", Namespace: "empty",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
	})

	It("UT-AF-122-003: DS unavailable", func() {
		mock := &ds.MockClient{
			GetRemediationHistoryFn: func(ctx context.Context, opts ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
				return nil, fmt.Errorf("connection refused")
			},
		}
		_, err := tools.HandleGetRemediationHistory(ctx, mock, tools.GetRemediationHistoryArgs{
			Kind: "Deployment", Name: "api", SpecHash: "sha256:test", Namespace: "pay",
		})
		Expect(err).To(HaveOccurred())
	})

	It("UT-AF-122-004: filter by date range", func() {
		mock := &ds.MockClient{
			GetRemediationHistoryFn: func(ctx context.Context, opts ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
				Expect(opts.Since).To(Equal("2026-01-01"))
				return []ds.HistoricalRemediation{{ID: "rr-1"}}, nil
			},
		}
		result, err := tools.HandleGetRemediationHistory(ctx, mock, tools.GetRemediationHistoryArgs{
			Kind: "Deployment", Name: "api", SpecHash: "sha256:test", Since: "2026-01-01",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
	})

	It("UT-AF-1462-001: empty kind returns validation error", func() {
		mock := &ds.MockClient{
			GetRemediationHistoryFn: func(ctx context.Context, opts ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
				Fail("DS client should not be called when kind is empty")
				return nil, nil
			},
		}
		_, err := tools.HandleGetRemediationHistory(ctx, mock, tools.GetRemediationHistoryArgs{
			Kind: "", Name: "api", SpecHash: "sha256:abc123",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("kind is required"))
	})

	It("UT-AF-1462-002: empty name returns validation error", func() {
		mock := &ds.MockClient{
			GetRemediationHistoryFn: func(ctx context.Context, opts ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
				Fail("DS client should not be called when name is empty")
				return nil, nil
			},
		}
		_, err := tools.HandleGetRemediationHistory(ctx, mock, tools.GetRemediationHistoryArgs{
			Kind: "Deployment", Name: "", SpecHash: "sha256:abc123",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("name is required"))
	})

	It("UT-AF-1462-003: empty spec_hash returns validation error", func() {
		mock := &ds.MockClient{
			GetRemediationHistoryFn: func(ctx context.Context, opts ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
				Fail("DS client should not be called when spec_hash is empty")
				return nil, nil
			},
		}
		_, err := tools.HandleGetRemediationHistory(ctx, mock, tools.GetRemediationHistoryArgs{
			Kind: "Deployment", Name: "api", SpecHash: "",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("spec_hash is required"))
	})

	It("UT-AF-1462-004: all params valid passes spec_hash to DS client", func() {
		var capturedOpts ds.HistoryOpts
		mock := &ds.MockClient{
			GetRemediationHistoryFn: func(ctx context.Context, opts ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
				capturedOpts = opts
				return []ds.HistoricalRemediation{{ID: "rr-1", Namespace: "prod", Phase: "Completed"}}, nil
			},
		}
		result, err := tools.HandleGetRemediationHistory(ctx, mock, tools.GetRemediationHistoryArgs{
			Namespace: "prod", Kind: "Deployment", Name: "api", SpecHash: "sha256:abc123",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		Expect(capturedOpts.SpecHash).To(Equal("sha256:abc123"))
	})

	It("UT-AF-1462-005: empty namespace is allowed for cluster-scoped resources", func() {
		var capturedOpts ds.HistoryOpts
		mock := &ds.MockClient{
			GetRemediationHistoryFn: func(ctx context.Context, opts ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
				capturedOpts = opts
				return []ds.HistoricalRemediation{{ID: "rr-2"}}, nil
			},
		}
		result, err := tools.HandleGetRemediationHistory(ctx, mock, tools.GetRemediationHistoryArgs{
			Kind: "ClusterRole", Name: "admin", SpecHash: "sha256:def456",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		Expect(capturedOpts.Namespace).To(BeEmpty())
	})

	It("UT-AF-1462-E01: spec_hash without sha256: prefix returns format error", func() {
		mock := &ds.MockClient{
			GetRemediationHistoryFn: func(ctx context.Context, opts ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
				Fail("DS client should not be called with malformed spec_hash")
				return nil, nil
			},
		}
		_, err := tools.HandleGetRemediationHistory(ctx, mock, tools.GetRemediationHistoryArgs{
			Kind: "Deployment", Name: "api", SpecHash: "md5:badprefix",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("sha256: prefix"))
	})

	It("UT-AF-1462-E02: spec_hash with valid sha256: prefix passes validation", func() {
		mock := &ds.MockClient{
			GetRemediationHistoryFn: func(ctx context.Context, opts ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
				return []ds.HistoricalRemediation{{ID: "rr-1"}}, nil
			},
		}
		result, err := tools.HandleGetRemediationHistory(ctx, mock, tools.GetRemediationHistoryArgs{
			Kind: "Deployment", Name: "api", SpecHash: "sha256:e3b0c44298fc1c149afbf4c8996fb924",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
	})

	It("UT-AF-1462-006: since parameter passes through correctly", func() {
		var capturedOpts ds.HistoryOpts
		mock := &ds.MockClient{
			GetRemediationHistoryFn: func(ctx context.Context, opts ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
				capturedOpts = opts
				return []ds.HistoricalRemediation{{ID: "rr-3"}}, nil
			},
		}
		_, err := tools.HandleGetRemediationHistory(ctx, mock, tools.GetRemediationHistoryArgs{
			Kind: "Deployment", Name: "api", SpecHash: "sha256:abc123", Since: "24h",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(capturedOpts.Since).To(Equal("24h"))
	})
})
