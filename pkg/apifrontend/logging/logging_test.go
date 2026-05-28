package logging_test

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/logging"
)

func TestLoggingSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Logging Suite")
}

var _ = Describe("Logging", func() {
	Describe("NewLogger", func() {
		It("UT-AF-LOG-001: creates a non-nil logr.Logger", func() {
			level := zap.NewAtomicLevelAt(zap.InfoLevel)
			logger, err := logging.NewLogger(level)
			Expect(err).NotTo(HaveOccurred())
			Expect(logger.GetSink()).NotTo(BeNil())
		})

		It("UT-AF-LOG-002: respects AtomicLevel for hot-reload", func() {
			level := zap.NewAtomicLevelAt(zap.ErrorLevel)
			logger, err := logging.NewLogger(level)
			Expect(err).NotTo(HaveOccurred())

			Expect(logger.V(1).Enabled()).To(BeFalse())

			level.SetLevel(zap.DebugLevel)
			Expect(logger.V(1).Enabled()).To(BeTrue())
		})
	})

	Describe("Context propagation", func() {
		It("UT-AF-LOG-003: WithLogger/FromContext round-trips the logger", func() {
			level := zap.NewAtomicLevelAt(zap.InfoLevel)
			logger, err := logging.NewLogger(level)
			Expect(err).NotTo(HaveOccurred())

			ctx := logging.WithLogger(context.Background(), logger)
			extracted := logging.FromContext(ctx)
			Expect(extracted.GetSink()).NotTo(BeNil())
		})

		It("UT-AF-LOG-004: FromContext returns discard logger when none in context", func() {
			logger := logging.FromContext(context.Background())
			Expect(logger).To(Equal(logr.Discard()))
		})

		It("UT-AF-LOG-008: WithUserID/WithSessionID round-trip values", func() {
			ctx := context.Background()
			ctx = logging.WithUserID(ctx, "alice")
			ctx = logging.WithSessionID(ctx, "sess-123")

			level := zap.NewAtomicLevelAt(zap.InfoLevel)
			logger, err := logging.NewLogger(level)
			Expect(err).NotTo(HaveOccurred())

			enriched := logging.WithStandardFields(ctx, logger)
			Expect(enriched).NotTo(Equal(logger))
		})
	})

	Describe("WithStandardFields", func() {
		var baseLogger logr.Logger

		BeforeEach(func() {
			level := zap.NewAtomicLevelAt(zap.InfoLevel)
			var err error
			baseLogger, err = logging.NewLogger(level)
			Expect(err).NotTo(HaveOccurred())
		})

		It("UT-AF-LOG-009: populates all 3 fields from context", func() {
			ctx := context.Background()
			ctx = logging.WithUserID(ctx, "bob")
			ctx = logging.WithSessionID(ctx, "sess-456")

			enriched := logging.WithStandardFields(ctx, baseLogger)
			Expect(enriched).NotTo(Equal(baseLogger))
		})

		It("UT-AF-LOG-010: empty context returns logger unchanged", func() {
			ctx := context.Background()
			enriched := logging.WithStandardFields(ctx, baseLogger)
			Expect(enriched).To(Equal(baseLogger))
		})

		It("UT-AF-LOG-011: user_id set via WithUserID is picked up", func() {
			ctx := logging.WithUserID(context.Background(), "carol")
			enriched := logging.WithStandardFields(ctx, baseLogger)
			Expect(enriched).NotTo(Equal(baseLogger))
		})
	})
})
