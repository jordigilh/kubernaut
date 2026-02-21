package notification

import (
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Routing Config credential_ref (BR-NOT-104-004)", func() {

	Describe("SlackConfig credential_ref parsing", func() {

		It("UT-NOT-104-009: SlackConfig with credential_ref parses from YAML", func() {
			configYAML := `
route:
  receiver: sre-critical
receivers:
  - name: sre-critical
    slackConfigs:
      - channel: '#sre-critical'
        credentialRef: slack-sre-critical
`
			config, err := routing.ParseConfig([]byte(configYAML))
			Expect(err).NotTo(HaveOccurred())
			Expect(config.Receivers).To(HaveLen(1))
			Expect(config.Receivers[0].SlackConfigs).To(HaveLen(1))
			Expect(config.Receivers[0].SlackConfigs[0].CredentialRef).To(Equal("slack-sre-critical"))
		})

		It("UT-NOT-104-010: ValidateCredentialRefs fails when SlackConfig has no credential_ref", func() {
			configYAML := `
route:
  receiver: sre-critical
receivers:
  - name: sre-critical
    slackConfigs:
      - channel: '#sre-critical'
`
			config, err := routing.ParseConfig([]byte(configYAML))
			Expect(err).NotTo(HaveOccurred(), "structural validation should pass")

			err = config.ValidateCredentialRefs()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("credentialRef"))
			Expect(err.Error()).To(ContainSubstring("sre-critical"))
		})
	})
})
