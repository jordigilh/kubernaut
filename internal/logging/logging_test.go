package logging_test

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

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
	})
})
