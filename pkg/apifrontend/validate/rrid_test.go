package validate_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
)

var _ = Describe("Action validation (G13)", func() {
	It("UT-AF-1234-097: invalid action string rejected", func() {
		err := validate.Action("destroy")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid action"))
	})

	It("UT-AF-1234-097b: valid action accepted", func() {
		for _, a := range []string{"investigate", "discover", "select", "takeover", "message", "complete", "cancel", "status", "reconnect"} {
			err := validate.Action(a)
			Expect(err).NotTo(HaveOccurred(), "action %s should be valid", a)
		}
	})
})

var _ = Describe("MessageLength validation (G13)", func() {
	It("UT-AF-1234-096: message over 10KB rejected", func() {
		longMsg := strings.Repeat("x", validate.MaxMessageLen+1)
		err := validate.MessageLength(longMsg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("exceeds maximum"))
	})

	It("UT-AF-1234-096b: message at max length accepted", func() {
		maxMsg := strings.Repeat("x", validate.MaxMessageLen)
		err := validate.MessageLength(maxMsg)
		Expect(err).NotTo(HaveOccurred())
	})
})
