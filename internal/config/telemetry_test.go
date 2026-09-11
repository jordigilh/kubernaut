package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/config"
)

var _ = Describe("TelemetryConfig", func() {
	It("accepts telemetry with no network endpoint", func() {
		Expect(config.TelemetryConfig{LogSink: true}.Validate()).To(Succeed())
	})

	It("accepts a complete client certificate pair", func() {
		cfg := config.TelemetryConfig{
			Endpoint: "collector.example.com:4318",
			TLS: config.TelemetryTLSConfig{
				CertFile: "/etc/telemetry/tls.crt",
				KeyFile:  "/etc/telemetry/tls.key",
			},
		}
		Expect(cfg.Validate()).To(Succeed())
	})

	It("rejects an incomplete client certificate pair", func() {
		cfg := config.TelemetryConfig{
			Endpoint: "collector.example.com:4318",
			TLS: config.TelemetryTLSConfig{
				CertFile: "/etc/telemetry/tls.crt",
			},
		}
		Expect(cfg.Validate()).To(MatchError("telemetry TLS certFile and keyFile must be set together"))
	})
})
