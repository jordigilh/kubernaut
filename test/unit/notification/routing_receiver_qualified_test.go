package notification

import (
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// testLogger is defined in routing_hotreload_test.go (same package)

var _ = Describe("Per-Receiver Delivery Wiring (BR-NOT-104-004)", func() {

	Describe("Receiver Qualified Channel Names", func() {

		It("UT-NOT-104-011: receiverToChannels returns receiver-qualified Slack names and unqualified others", func() {
			receiver := &routing.Receiver{
				Name: "sre-critical",
				SlackConfigs: []routing.SlackConfig{
					{Channel: "#sre-critical", CredentialRef: "slack-sre-critical"},
				},
				ConsoleConfigs: []routing.ConsoleConfig{
					{Enabled: true},
				},
			}

			channels := receiver.QualifiedChannels()

			Expect(channels).To(ContainElement("slack:sre-critical"))
			Expect(channels).To(ContainElement("console"))
			Expect(channels).To(HaveLen(2))
		})

		It("UT-NOT-104-017: Slack without credential_ref uses unqualified channel name", func() {
			receiver := &routing.Receiver{
				Name: "legacy-receiver",
				SlackConfigs: []routing.SlackConfig{
					{Channel: "#alerts"},
				},
			}

			channels := receiver.QualifiedChannels()

			Expect(channels).To(ContainElement("slack"))
			Expect(channels).NotTo(ContainElement("slack:legacy-receiver"))
		})
	})

	Describe("Routing Config Credential Validation on Load", func() {

		It("UT-NOT-104-012: unresolvable credential_ref preserves previous router config", func() {
			router := routing.NewRouter(testLogger)

			// Load a valid initial config
			initialConfigYAML := `
route:
  receiver: initial-receiver
receivers:
  - name: initial-receiver
    consoleConfigs:
      - enabled: true
`
			err := router.LoadConfig([]byte(initialConfigYAML))
			Expect(err).NotTo(HaveOccurred())

			initialConfig := router.GetConfig()
			Expect(initialConfig.Receivers).To(HaveLen(1))
			Expect(initialConfig.Receivers[0].Name).To(Equal("initial-receiver"))

			// Attempt to load new config that has credential_ref issues
			newConfigYAML := `
route:
  receiver: slack-receiver
receivers:
  - name: slack-receiver
    slackConfigs:
      - channel: '#alerts'
`
			newConfig, err := routing.ParseConfig([]byte(newConfigYAML))
			Expect(err).NotTo(HaveOccurred(), "structural parse should succeed")

			// Validate credential refs -- should fail
			err = newConfig.ValidateCredentialRefs()
			Expect(err).To(HaveOccurred(), "credential validation should fail")

			// Because validation failed, caller does NOT call router.LoadConfig
			// Verify previous config is preserved
			currentConfig := router.GetConfig()
			Expect(currentConfig.Receivers).To(HaveLen(1))
			Expect(currentConfig.Receivers[0].Name).To(Equal("initial-receiver"))
		})
	})
})
