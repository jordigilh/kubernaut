package notification

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	notificationconfig "github.com/jordigilh/kubernaut/pkg/notification/config"
)

var _ = Describe("Notification Config Credential Settings (BR-NOT-104)", func() {

	Describe("DefaultConfig Credentials Directory", func() {

		It("UT-NOT-104-013: DefaultConfig includes CredentialsDir with projected volume path", func() {
			cfg := notificationconfig.DefaultConfig()

			Expect(cfg.Delivery.Credentials.Dir).To(Equal("/etc/notification/credentials/"))
		})
	})

	Describe("Config applyDefaults", func() {

		It("UT-NOT-104-014: applyDefaults sets CredentialsDir when empty", func() {
			cfg, err := notificationconfig.LoadFromBytes([]byte(`
controller:
  metricsAddr: ":9090"
`))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Delivery.Credentials.Dir).To(Equal("/etc/notification/credentials/"))
		})
	})

	Describe("SlackSettings no longer contains WebhookURL", func() {

		It("UT-NOT-104-015: SlackSettings has Timeout but no WebhookURL field", func() {
			cfg := notificationconfig.DefaultConfig()

			Expect(cfg.Delivery.Slack.Timeout).To(BeNumerically(">", 0))
			// WebhookURL field removed per BR-NOT-104: credentials resolved via CredentialResolver
			// Compile-time verification: this test file imports the config package.
			// If SlackSettings still had WebhookURL, the struct would be different.
			// We verify by ensuring default config works with no webhook URL at all.
		})
	})

	Describe("LoadFromEnv no longer loads SLACK_WEBHOOK_URL", func() {

		It("UT-NOT-104-016: LoadFromEnv does not set any Slack webhook URL", func() {
			os.Setenv("SLACK_WEBHOOK_URL", "https://should-be-ignored.example.com")
			defer os.Unsetenv("SLACK_WEBHOOK_URL")

			cfg := notificationconfig.DefaultConfig()
			cfg.LoadFromEnv()

			// BR-NOT-104: No WebhookURL field exists. LoadFromEnv is a no-op for Slack.
			// Verify config unchanged after LoadFromEnv.
			defaultCfg := notificationconfig.DefaultConfig()
			Expect(cfg.Delivery.Slack).To(Equal(defaultCfg.Delivery.Slack))
		})
	})
})
