package tools_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/ds"
	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

var _ = Describe("kubernaut_get_audit_trail", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-124-001: returns audit events", func() {
		mock := &ds.MockClient{
			GetAuditTrailFn: func(ctx context.Context, opts ds.AuditTrailOpts) ([]ds.AuditEvent, error) {
				return []ds.AuditEvent{
					{Timestamp: "2026-05-01T10:00:00Z", EventType: "created", Actor: "alice"},
					{Timestamp: "2026-05-01T10:05:00Z", EventType: "approved", Actor: "bob"},
				}, nil
			},
		}
		result, err := tools.HandleGetAuditTrail(ctx, mock, tools.GetAuditTrailArgs{RRID: "pay/rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(2))
	})

	It("UT-AF-124-002: no events", func() {
		mock := &ds.MockClient{
			GetAuditTrailFn: func(ctx context.Context, opts ds.AuditTrailOpts) ([]ds.AuditEvent, error) {
				return nil, nil
			},
		}
		result, err := tools.HandleGetAuditTrail(ctx, mock, tools.GetAuditTrailArgs{RRID: "pay/missing"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
	})

	It("UT-AF-124-003: DS unavailable", func() {
		mock := &ds.MockClient{
			GetAuditTrailFn: func(ctx context.Context, opts ds.AuditTrailOpts) ([]ds.AuditEvent, error) {
				return nil, fmt.Errorf("connection refused")
			},
		}
		_, err := tools.HandleGetAuditTrail(ctx, mock, tools.GetAuditTrailArgs{RRID: "pay/rr-1"})
		Expect(err).To(HaveOccurred())
	})

	It("UT-AF-124-004: filter by event type", func() {
		mock := &ds.MockClient{
			GetAuditTrailFn: func(ctx context.Context, opts ds.AuditTrailOpts) ([]ds.AuditEvent, error) {
				Expect(opts.EventType).To(Equal("approval"))
				return []ds.AuditEvent{{EventType: "approval", Actor: "bob"}}, nil
			},
		}
		result, err := tools.HandleGetAuditTrail(ctx, mock, tools.GetAuditTrailArgs{RRID: "pay/rr-1", EventType: "approval"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
	})
})
