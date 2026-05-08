package logging_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/jordigilh/kubernaut-apifrontend/internal/logging"
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

	Describe("NewSlogLogger", func() {
		It("UT-AF-LOG-005: creates a working *slog.Logger", func() {
			level := zap.NewAtomicLevelAt(zap.InfoLevel)
			sl := logging.NewSlogLogger(level)
			Expect(sl).NotTo(BeNil())
			Expect(sl.Enabled(context.Background(), slog.LevelInfo)).To(BeTrue())
			Expect(sl.Enabled(context.Background(), slog.LevelDebug)).To(BeFalse())
		})

		It("UT-AF-LOG-006: respects dynamic level changes", func() {
			level := zap.NewAtomicLevelAt(zap.InfoLevel)
			sl := logging.NewSlogLogger(level)

			Expect(sl.Enabled(context.Background(), slog.LevelDebug)).To(BeFalse())
			level.SetLevel(zapcore.DebugLevel)
			Expect(sl.Enabled(context.Background(), slog.LevelDebug)).To(BeTrue())
		})

		It("UT-AF-LOG-007: maps zap levels to slog levels correctly", func() {
			cases := []struct {
				zapLevel  zapcore.Level
				slogLevel slog.Level
				enabled   bool
			}{
				{zapcore.DebugLevel, slog.LevelDebug, true},
				{zapcore.InfoLevel, slog.LevelDebug, false},
				{zapcore.WarnLevel, slog.LevelInfo, false},
				{zapcore.ErrorLevel, slog.LevelWarn, false},
			}
			for _, tc := range cases {
				level := zap.NewAtomicLevelAt(tc.zapLevel)
				sl := logging.NewSlogLogger(level)
				Expect(sl.Enabled(context.Background(), tc.slogLevel)).To(Equal(tc.enabled),
					"zap=%v slog=%v", tc.zapLevel, tc.slogLevel)
			}
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
