package validate_test

import (
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/validate"
)

func TestValidateSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Validate Suite")
}

var _ = Describe("Namespace", func() {
	DescribeTable("valid namespaces",
		func(ns string) {
			Expect(validate.Namespace(ns)).To(Succeed())
		},
		Entry("simple", "default"),
		Entry("with hyphens", "kube-system"),
		Entry("with numbers", "ns-123"),
		Entry("single char", "a"),
		Entry("max length (63 chars)", strings.Repeat("a", 63)),
	)

	DescribeTable("invalid namespaces",
		func(ns string, substr string) {
			err := validate.Namespace(ns)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(substr))
		},
		Entry("empty", "", "must not be empty"),
		Entry("too long (64 chars)", strings.Repeat("a", 64), "invalid namespace"),
		Entry("uppercase", "MyNamespace", "invalid namespace"),
		Entry("leading hyphen", "-invalid", "invalid namespace"),
		Entry("trailing hyphen", "invalid-", "invalid namespace"),
		Entry("dot", "my.namespace", "invalid namespace"),
		Entry("slash", "../../etc", "invalid namespace"),
		Entry("underscore", "my_namespace", "invalid namespace"),
		Entry("space", "my namespace", "invalid namespace"),
		Entry("unicode", "名前空間", "invalid namespace"),
	)
})

var _ = Describe("ResourceName", func() {
	DescribeTable("valid resource names",
		func(name string) {
			Expect(validate.ResourceName(name)).To(Succeed())
		},
		Entry("simple", "my-pod"),
		Entry("with dots", "my.pod.v1"),
		Entry("with hyphens and numbers", "pod-123-abc"),
		Entry("max length (253 chars)", strings.Repeat("a", 253)),
	)

	DescribeTable("invalid resource names",
		func(name string, substr string) {
			err := validate.ResourceName(name)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(substr))
		},
		Entry("empty", "", "must not be empty"),
		Entry("too long (254 chars)", strings.Repeat("a", 254), "invalid resource name"),
		Entry("uppercase", "MyPod", "invalid resource name"),
		Entry("leading hyphen", "-pod", "invalid resource name"),
		Entry("trailing hyphen", "pod-", "invalid resource name"),
		Entry("slash", "ns/pod", "invalid resource name"),
		Entry("space", "my pod", "invalid resource name"),
	)
})

var _ = Describe("LabelValue", func() {
	DescribeTable("valid label values",
		func(v string) {
			Expect(validate.LabelValue(v)).To(Succeed())
		},
		Entry("empty (optional labels)", ""),
		Entry("simple", "Deployment"),
		Entry("with hyphens", "my-value"),
		Entry("with dots", "v1.2.3"),
		Entry("with underscores", "my_value"),
		Entry("max length (63 chars)", strings.Repeat("a", 63)),
		Entry("alphanumeric start/end", "a123b"),
	)

	DescribeTable("invalid label values",
		func(v string, substr string) {
			err := validate.LabelValue(v)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(substr))
		},
		Entry("too long (64 chars)", strings.Repeat("a", 64), "invalid label value"),
		Entry("leading hyphen", "-value", "invalid label value"),
		Entry("trailing hyphen", "value-", "invalid label value"),
		Entry("slash", "ns/name", "invalid label value"),
		Entry("space", "my value", "invalid label value"),
		Entry("unicode", "日本語", "invalid label value"),
	)
})
