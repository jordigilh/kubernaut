package naming_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// repoRoot resolves the workspace root by walking up from this test file.
func repoRoot() string {
	wd, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred())
	for dir := wd; dir != "/"; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
	}
	Fail("Could not find repository root (go.mod)")
	return ""
}

// scanForLegacyName searches files under dir for the forbidden string.
// Returns a list of "file:line" entries where the string was found.
func scanForLegacyName(root, dir, forbidden string) []string {
	var violations []string
	base := filepath.Join(root, dir)

	err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		allowed := map[string]bool{
			".yaml": true, ".yml": true, ".go": true, ".json": true,
			".sh": true, ".tmpl": true, ".md": true, ".mdc": true,
		}
		if !allowed[ext] && ext != "" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if strings.Contains(line, forbidden) {
				rel, _ := filepath.Rel(root, path)
				violations = append(violations, rel+":"+strings.TrimSpace(line)+" (line "+string(rune('0'+i+1))+")")
			}
		}
		return nil
	})
	Expect(err).ToNot(HaveOccurred())
	return violations
}

var _ = Describe("Legacy Naming Convention Guard (Issue #691)", func() {
	var root string

	BeforeEach(func() {
		root = repoRoot()
	})

	It("UT-RENAME-691-001: no holmesgpt-api in config/ directory", func() {
		violations := scanForLegacyName(root, "config", "holmesgpt-api")
		Expect(violations).To(BeEmpty(),
			"config/ must not contain legacy 'holmesgpt-api' references:\n"+strings.Join(violations, "\n"))
	})

	It("UT-RENAME-691-002: no holmesgpt-api in deploy/ directory", func() {
		violations := scanForLegacyName(root, "deploy", "holmesgpt-api")
		Expect(violations).To(BeEmpty(),
			"deploy/ must not contain legacy 'holmesgpt-api' references:\n"+strings.Join(violations, "\n"))
	})

	It("UT-RENAME-691-003: no holmesgpt-api in internal/ Go/JSON code", func() {
		violations := scanForLegacyName(root, "internal", "holmesgpt-api")
		Expect(violations).To(BeEmpty(),
			"internal/ must not contain legacy 'holmesgpt-api' references:\n"+strings.Join(violations, "\n"))
	})

	It("UT-RENAME-691-004: no holmesgpt-api in docker/", func() {
		violations := scanForLegacyName(root, "docker", "holmesgpt-api")
		Expect(violations).To(BeEmpty(),
			"docker/ must not contain legacy 'holmesgpt-api' references:\n"+strings.Join(violations, "\n"))
	})

	It("UT-RENAME-691-005: dead function deployHolmesGPTAPIManifestOnly removed", func() {
		path := filepath.Join(root, "test/infrastructure/aianalysis_e2e.go")
		data, err := os.ReadFile(path)
		if err != nil {
			Skip("File not found (already deleted)")
		}
		Expect(string(data)).ToNot(ContainSubstring("deployHolmesGPTAPIManifestOnly"),
			"Dead function deployHolmesGPTAPIManifestOnly must be removed")
	})
})
