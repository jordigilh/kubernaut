package tools_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("IT-AF-1351: Error redaction wiring", func() {

	It("IT-AF-1351-REDACT: FormatEventForUser redacts IPs from error events end-to-end", func() {
		evt := ka.InvestigationEvent{
			Type: ka.EventTypeError,
			Data: []byte(`{"error": "dial tcp 192.168.1.100:6443: connect: connection refused"}`),
		}
		text := tools.FormatEventForUser(evt)
		Expect(text).NotTo(ContainSubstring("192.168.1.100"))
		Expect(text).To(HavePrefix("Error: "))
	})

	It("IT-AF-1351-REDACT-URL: FormatEventForUser redacts URLs from error events", func() {
		evt := ka.InvestigationEvent{
			Type: ka.EventTypeError,
			Data: []byte(`{"error": "POST https://internal-ka.svc:8443/api/investigate failed: timeout"}`),
		}
		text := tools.FormatEventForUser(evt)
		Expect(text).NotTo(ContainSubstring("https://"))
		Expect(text).NotTo(ContainSubstring("internal-ka.svc"))
	})
})
