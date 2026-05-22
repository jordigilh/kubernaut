package validate_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
)

var _ = Describe("ParseRRID (G13)", func() {
	It("UT-AF-1234-090: valid rr_id returns namespace and name", func() {
		ns, name, err := validate.ParseRRID("prod/rr-web-api-oom")
		Expect(err).NotTo(HaveOccurred())
		Expect(ns).To(Equal("prod"))
		Expect(name).To(Equal("rr-web-api-oom"))
	})

	It("UT-AF-1234-091: empty rr_id rejected", func() {
		_, _, err := validate.ParseRRID("")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("empty"))
	})

	It("UT-AF-1234-092: malformed rr_id (no slash) rejected", func() {
		_, _, err := validate.ParseRRID("prodrr-web-api-oom")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("namespace/name"))
	})

	It("UT-AF-1234-093: path traversal rr_id rejected", func() {
		_, _, err := validate.ParseRRID("../etc/passwd")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("path traversal"))
	})

	It("UT-AF-1234-094: invalid namespace in rr_id rejected", func() {
		_, _, err := validate.ParseRRID("INVALID_NS/rr-001")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("namespace"))
	})

	It("UT-AF-1234-095: invalid name in rr_id rejected", func() {
		_, _, err := validate.ParseRRID("prod/INVALID NAME!!!")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("name"))
	})
})

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
