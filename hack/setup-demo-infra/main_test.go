package main

import (
	"flag"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("registerDemoFlags", func() {
	It("UT-INFRA-FLEETDEMO-067 [BR-PLATFORM-014]: exposes an operator image tag override", func() {
		flags := flag.NewFlagSet("setup-demo-infra-test", flag.ContinueOnError)
		values := registerDemoFlags(flags)

		Expect(flags.Parse([]string{"-operator-image-tag", "operator-3838"})).To(Succeed())
		Expect(*values.operatorImageTag).To(Equal("operator-3838"))
	})
})

var _ = Describe("parseReasoningEnabled", func() {
	It("UT-INFRA-FLEETDEMO-062 [SI-10]: leaves an omitted override unset", func() {
		enabled, set, err := parseReasoningEnabled("")
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeFalse())
		Expect(set).To(BeFalse())
	})

	It("UT-INFRA-FLEETDEMO-063 [SI-10]: parses true and false overrides", func() {
		trueValue, set, err := parseReasoningEnabled("true")
		Expect(err).NotTo(HaveOccurred())
		Expect(set).To(BeTrue())
		Expect(trueValue).To(BeTrue())

		falseValue, set, err := parseReasoningEnabled("false")
		Expect(err).NotTo(HaveOccurred())
		Expect(set).To(BeTrue())
		Expect(falseValue).To(BeFalse())
	})

	It("UT-INFRA-FLEETDEMO-064 [SI-10]: rejects a non-boolean override", func() {
		_, _, err := parseReasoningEnabled("yes")
		Expect(err).To(MatchError(`invalid -llm-reasoning-enabled "yes"; use true or false`))
	})
})

var _ = Describe("defaultClusterName", func() {
	It("UT-INFRA-FLEETDEMO-065 [BR-PLATFORM-014]: preserves an explicit cluster name", func() {
		Expect(defaultClusterName("custom", true)).To(Equal("custom"))
	})

	It("UT-INFRA-FLEETDEMO-066 [BR-PLATFORM-014]: selects topology-specific defaults", func() {
		Expect(defaultClusterName("", true)).To(Equal("kubernaut-hub"))
		Expect(defaultClusterName("", false)).To(Equal("kubernaut-demo"))
	})
})
