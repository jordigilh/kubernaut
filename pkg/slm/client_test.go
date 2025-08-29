package slm

import (
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

var _ = Describe("SLM Client", func() {
	var logger *logrus.Logger

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests
	})

	Describe("NewClient", func() {
		DescribeTable("creating new client",
			func(cfg config.SLMConfig, expectErr bool, errString string) {
				client, err := NewClient(cfg, logger)

				if expectErr {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(errString))
					Expect(client).To(BeNil())
				} else {
					Expect(err).ToNot(HaveOccurred())
					Expect(client).ToNot(BeNil())
					var clientInterface Client = client
					Expect(clientInterface).ToNot(BeNil())
				}
			},
			Entry("valid localai config",
				config.SLMConfig{
					Provider: "localai",
					Endpoint: "http://localhost:8080",
					Model:    "test-model",
					Timeout:  30 * time.Second,
				},
				false,
				"",
			),
			Entry("invalid provider",
				config.SLMConfig{
					Provider: "invalid",
					Endpoint: "http://localhost:8080",
					Model:    "test-model",
				},
				true,
				"only LocalAI provider supported, got: invalid",
			),
		)
	})
})