package validate_test

import (
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
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

var _ = Describe("AlertName (F1)", func() {
	DescribeTable("valid alert names",
		func(name string) {
			Expect(validate.AlertName(name)).To(Succeed())
		},
		Entry("simple", "KubePodCrashLooping"),
		Entry("with underscores", "high_memory_usage"),
		Entry("with colons", "namespace:container_cpu:rate5m"),
		Entry("starts with underscore", "_internal_alert"),
		Entry("starts with colon", ":aggregation_rule"),
		Entry("single char", "A"),
		Entry("all caps", "ALERT"),
		Entry("mixed", "kube_pod_container_status_waiting_reason"),
	)

	DescribeTable("invalid alert names",
		func(name string, substr string) {
			err := validate.AlertName(name)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(substr))
		},
		Entry("empty", "", "must not be empty"),
		Entry("starts with digit", "1alert", "invalid alert_name"),
		Entry("contains hyphen", "my-alert", "invalid alert_name"),
		Entry("contains dot", "my.alert", "invalid alert_name"),
		Entry("contains slash", "path/traversal", "invalid alert_name"),
		Entry("contains space", "my alert", "invalid alert_name"),
		Entry("CRLF injection", "alert\r\ninjection", "invalid alert_name"),
		Entry("too long", strings.Repeat("a", 254), "exceeds max length"),
	)
})

var _ = Describe("APIVersion (F2)", func() {
	DescribeTable("valid api versions",
		func(v string) {
			Expect(validate.APIVersion(v)).To(Succeed())
		},
		Entry("core v1", "v1"),
		Entry("apps/v1", "apps/v1"),
		Entry("batch/v1", "batch/v1"),
		Entry("autoscaling/v2", "autoscaling/v2"),
		Entry("policy/v1", "policy/v1"),
		Entry("route.openshift.io/v1", "route.openshift.io/v1"),
		Entry("serving.knative.dev/v1", "serving.knative.dev/v1"),
		Entry("v1beta1", "v1beta1"),
		Entry("apps/v1beta2", "apps/v1beta2"),
		Entry("apiextensions.k8s.io/v1", "apiextensions.k8s.io/v1"),
	)

	DescribeTable("invalid api versions",
		func(v string, substr string) {
			err := validate.APIVersion(v)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(substr))
		},
		Entry("empty", "", "must not be empty"),
		Entry("no version prefix", "apps", "invalid api_version"),
		Entry("uppercase group", "Apps/v1", "invalid api_version"),
		Entry("double slash", "apps//v1", "invalid api_version"),
		Entry("missing v prefix", "apps/1", "invalid api_version"),
		Entry("path traversal", "../../v1", "invalid api_version"),
		Entry("space", "apps /v1", "invalid api_version"),
		Entry("too long", strings.Repeat("a", 254), "exceeds max length"),
	)
})

var _ = Describe("IT-AF-1351: RRID validation wiring", func() {

	Describe("IT-AF-1351-VALID: valid namespace/name formats accepted", func() {
		DescribeTable("accepts valid rr_id values",
			func(rrid string) {
				Expect(validate.RRID(rrid)).To(Succeed())
			},
			Entry("simple name", "my-rr-001"),
			Entry("namespace/name", "prod/my-rr-001"),
			Entry("long valid name", "my-namespace/my-long-remediation-request-name-123"),
		)
	})

	Describe("IT-AF-1351-INVALID: invalid formats rejected", func() {
		DescribeTable("rejects invalid rr_id values",
			func(rrid string) {
				Expect(validate.RRID(rrid)).NotTo(Succeed())
			},
			Entry("path traversal", "../../etc/passwd"),
			Entry("SQL injection", "drop table; --"),
			Entry("empty", ""),
			Entry("spaces", "my namespace/my name"),
			Entry("too many slashes", "a/b/c"),
			Entry("uppercase not DNS", "MyNamespace/MyRR"),
		)
	})
})
