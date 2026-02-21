package credentials

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/pkg/notification/credentials"
)

var _ = Describe("CredentialResolver (BR-NOT-104)", func() {
	var (
		tmpDir   string
		resolver *credentials.Resolver
	)

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "cred-resolver-test-*")
		Expect(err).NotTo(HaveOccurred())

		logger := zap.New(zap.UseDevMode(true))
		resolver, err = credentials.NewResolver(tmpDir, logger)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if resolver != nil {
			Expect(resolver.Close()).To(Succeed())
		}
		os.RemoveAll(tmpDir)
	})

	Describe("Credential Resolution (BR-NOT-104-001)", func() {
		It("UT-NOT-104-001: resolves existing credential file to its content", func() {
			// Given: a credential file exists
			writeCredentialFile(tmpDir, "slack-sre-critical", "https://hooks.slack.com/services/T123/B456/xxx")
			resolver, err := credentials.NewResolver(tmpDir, zap.New(zap.UseDevMode(true)))
			Expect(err).NotTo(HaveOccurred())

			// When: resolving by name
			value, err := resolver.Resolve("slack-sre-critical")

			// Then: returns exact file content
			Expect(err).NotTo(HaveOccurred())
			Expect(value).To(Equal("https://hooks.slack.com/services/T123/B456/xxx"))
		})

		It("UT-NOT-104-002: returns descriptive error for non-existent credential", func() {
			// Given: no credential file named "nonexistent"
			// When: resolving
			value, err := resolver.Resolve("nonexistent")

			// Then: error contains the credential name
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nonexistent"))
			Expect(value).To(BeEmpty())
		})

		It("UT-NOT-104-006: trims whitespace and newlines from credential file content", func() {
			// Given: a credential file with trailing whitespace and newlines
			writeCredentialFile(tmpDir, "slack-messy", "  https://hooks.slack.com/xxx  \n")
			resolver, err := credentials.NewResolver(tmpDir, zap.New(zap.UseDevMode(true)))
			Expect(err).NotTo(HaveOccurred())

			// When: resolving
			value, err := resolver.Resolve("slack-messy")

			// Then: value is trimmed
			Expect(err).NotTo(HaveOccurred())
			Expect(value).To(Equal("https://hooks.slack.com/xxx"))
		})
	})

	Describe("Credential Reload (BR-NOT-104-002)", func() {
		It("UT-NOT-104-003: reload picks up new credential files", func() {
			// Given: resolver initialized with empty directory
			// When: new file is added and Reload() called
			_, err := resolver.Resolve("slack-new")
			Expect(err).To(HaveOccurred(), "should fail before reload")

			writeCredentialFile(tmpDir, "slack-new", "https://hooks.slack.com/new")
			Expect(resolver.Reload()).To(Succeed())

			// Then: new credential resolves
			value, err := resolver.Resolve("slack-new")
			Expect(err).NotTo(HaveOccurred())
			Expect(value).To(Equal("https://hooks.slack.com/new"))
		})

		It("UT-NOT-104-004: reload picks up changed credential values", func() {
			// Given: resolver with existing credential
			writeCredentialFile(tmpDir, "slack-sre", "old-url")
			resolver, err := credentials.NewResolver(tmpDir, zap.New(zap.UseDevMode(true)))
			Expect(err).NotTo(HaveOccurred())

			value, err := resolver.Resolve("slack-sre")
			Expect(err).NotTo(HaveOccurred())
			Expect(value).To(Equal("old-url"))

			// When: file content is changed and Reload() called
			writeCredentialFile(tmpDir, "slack-sre", "new-url")
			Expect(resolver.Reload()).To(Succeed())

			// Then: returns updated value
			value, err = resolver.Resolve("slack-sre")
			Expect(err).NotTo(HaveOccurred())
			Expect(value).To(Equal("new-url"))
		})

		It("UT-NOT-104-005: reload removes deleted credential files", func() {
			// Given: resolver with existing credential
			writeCredentialFile(tmpDir, "slack-temp", "some-url")
			resolver, err := credentials.NewResolver(tmpDir, zap.New(zap.UseDevMode(true)))
			Expect(err).NotTo(HaveOccurred())

			value, err := resolver.Resolve("slack-temp")
			Expect(err).NotTo(HaveOccurred())
			Expect(value).To(Equal("some-url"))

			// When: file is deleted and Reload() called
			os.Remove(filepath.Join(tmpDir, "slack-temp"))
			Expect(resolver.Reload()).To(Succeed())

			// Then: credential is no longer resolvable
			_, err = resolver.Resolve("slack-temp")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Credential Reference Validation (BR-NOT-104-003)", func() {
		It("UT-NOT-104-007: ValidateRefs returns nil when all refs resolve", func() {
			// Given: credentials directory with files a, b, c
			writeCredentialFile(tmpDir, "a", "val-a")
			writeCredentialFile(tmpDir, "b", "val-b")
			writeCredentialFile(tmpDir, "c", "val-c")
			resolver, err := credentials.NewResolver(tmpDir, zap.New(zap.UseDevMode(true)))
			Expect(err).NotTo(HaveOccurred())

			// When: validating all refs
			err = resolver.ValidateRefs([]string{"a", "b", "c"})

			// Then: no error
			Expect(err).NotTo(HaveOccurred())
		})

		It("UT-NOT-104-008: ValidateRefs returns error listing all unresolvable refs", func() {
			// Given: credentials directory with only file "a"
			writeCredentialFile(tmpDir, "a", "val-a")
			resolver, err := credentials.NewResolver(tmpDir, zap.New(zap.UseDevMode(true)))
			Expect(err).NotTo(HaveOccurred())

			// When: validating refs including missing ones
			err = resolver.ValidateRefs([]string{"a", "missing1", "missing2"})

			// Then: error lists all missing refs
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing1"))
			Expect(err.Error()).To(ContainSubstring("missing2"))
			Expect(err.Error()).NotTo(ContainSubstring(`"a"`))
		})
	})
})

func writeCredentialFile(dir, name, content string) {
	err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
	Expect(err).NotTo(HaveOccurred())
}
