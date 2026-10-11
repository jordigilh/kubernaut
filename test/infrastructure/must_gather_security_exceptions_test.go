/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package infrastructure

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pelletier/go-toml/v2"
)

var _ = Describe("Must-gather kubectl security exceptions [BR-SECURITY-2493]", func() {
	It("UT-SEC-2493-007: accepts only the observed kubectl CVEs for six calendar months", func() {
		body, err := os.ReadFile(filepath.Join(getProjectRoot(), "cmd/must-gather/.trivyignore")) //nolint:gosec // G304: known project path
		Expect(err).NotTo(HaveOccurred())
		acceptedAt, err := time.Parse(time.DateOnly, "2026-10-11")
		Expect(err).NotTo(HaveOccurred())
		expiresAt := acceptedAt.AddDate(0, 6, 0).Format(time.DateOnly)
		ids := make([]string, 0, 16)
		for _, line := range strings.Split(string(body), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			fields := strings.Fields(line)
			Expect(fields).To(HaveLen(2), "every exception must have an explicit expiration: %s", line)
			Expect(fields[1]).To(Equal("exp:"+expiresAt), "exception: %s", fields[0])
			ids = append(ids, fields[0])
		}
		Expect(ids).To(ConsistOf(
			"CVE-2026-25681", "CVE-2026-27136", "CVE-2026-33814", "CVE-2026-33818",
			"CVE-2026-39821", "CVE-2026-39822", "CVE-2026-46600", "CVE-2026-56852",
			"CVE-2026-56853", "CVE-2026-56858", "CVE-2026-56859", "CVE-2026-56860",
			"CVE-2026-56862", "CVE-2026-78667", "CVE-2026-78669", "CVE-2026-97031",
		), "do not silently broaden the accepted CVE set or duplicate exceptions")
	})

	It("UT-SEC-2493-008: records explicit risk acceptance without claiming the CVEs are fixed", func() {
		body, err := os.ReadFile(filepath.Join(getProjectRoot(), "cmd/must-gather/.trivyignore")) //nolint:gosec // G304: known project path
		Expect(err).NotTo(HaveOccurred())
		for _, evidence := range []string{"risk acceptance", "not vulnerability remediation", "2026-10-11", "2027-04-11", "Issue #1662", "DD-PLATFORM-003", "v1.36.5", "v1.37.1"} {
			Expect(string(body)).To(ContainSubstring(evidence))
		}
		Expect(string(body)).NotTo(ContainSubstring("no fix available"))
	})

	DescribeTable("UT-SEC-2493-009: CI and release route exceptions only to must-gather",
		func(workflow string, matrixKey string, expectedScans int) {
			body, err := os.ReadFile(filepath.Join(getProjectRoot(), ".github/workflows", workflow)) //nolint:gosec // G304: known project path
			Expect(err).NotTo(HaveOccurred())
			ignoreInputs := make([]string, 0, expectedScans)
			for _, line := range strings.Split(string(body), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "trivyignores:") {
					ignoreInputs = append(ignoreInputs, line)
				}
			}
			Expect(ignoreInputs).To(HaveLen(expectedScans))
			for _, input := range ignoreInputs {
				Expect(input).To(Equal("trivyignores: ${{ matrix." + matrixKey + " == 'must-gather' && 'cmd/must-gather/.trivyignore' || '' }}"))
			}
		},
		Entry("CI infrastructure scan", "ci-pipeline.yml", "image_name", 1),
		Entry("both release architectures", "release.yml", "service", 2),
	)
})

type gitleaksChecksumConfig struct {
	Extend struct {
		UseDefault    bool     `toml:"useDefault"`
		DisabledRules []string `toml:"disabledRules"`
	} `toml:"extend"`
	Allowlists []gitleaksChecksumAllowance `toml:"allowlists"`
}

type gitleaksChecksumAllowance struct {
	Description string   `toml:"description"`
	Condition   string   `toml:"condition"`
	TargetRules []string `toml:"targetRules"`
	RegexTarget string   `toml:"regexTarget"`
	Paths       []string `toml:"paths"`
	Regexes     []string `toml:"regexes"`
	Commits     []string `toml:"commits"`
	Stopwords   []string `toml:"stopwords"`
}

var _ = Describe("Verified archive checksum exception [BR-SECURITY-2493]", func() {
	const archiveDigest = "03ad405812c6f0574da6f89c7e13c063250192db43c517f877b84f3565913a65"
	var allowance gitleaksChecksumAllowance

	BeforeEach(func() {
		body, err := os.ReadFile(filepath.Join(getProjectRoot(), ".gitleaks.toml")) //nolint:gosec // G304: known project path
		Expect(err).NotTo(HaveOccurred())
		var config gitleaksChecksumConfig
		Expect(toml.Unmarshal(body, &config)).To(Succeed())
		Expect(config.Extend.UseDefault).To(BeTrue(), "keep all built-in secret detection rules")
		Expect(config.Extend.DisabledRules).NotTo(ContainElement("generic-api-key"))
		var matches []gitleaksChecksumAllowance
		for _, candidate := range config.Allowlists {
			if strings.Contains(candidate.Description, "Verified must-gather archive SHA-256") {
				matches = append(matches, candidate)
			}
		}
		Expect(matches).To(HaveLen(1), "one exact false-positive exception, not a blanket documentation allowance")
		allowance = matches[0]
	})

	It("UT-SEC-2493-010: narrows the exception to the rule, file and extracted checksum", func() {
		Expect(allowance.Condition).To(Equal("AND"))
		Expect(allowance.TargetRules).To(ConsistOf("generic-api-key"))
		Expect(allowance.RegexTarget).To(Equal("secret"))
		Expect(allowance.Paths).To(ConsistOf(`^docs/tests/2502/TEST_PLAN\.md$`))
		Expect(allowance.Regexes).To(ConsistOf("^" + archiveDigest + "$"))
		Expect(allowance.Commits).To(BeEmpty(), "do not exempt an entire commit's contents")
		Expect(allowance.Stopwords).To(BeEmpty())
	})

	DescribeTable("UT-SEC-2493-011: preserves detection outside the exact false positive",
		func(path string, digest string, expected bool) {
			Expect(allowance.Condition).To(Equal("AND"))
			Expect(allowance.Paths).To(HaveLen(1))
			Expect(allowance.Regexes).To(HaveLen(1))
			pathPattern, err := regexp.Compile(allowance.Paths[0])
			Expect(err).NotTo(HaveOccurred())
			digestPattern, err := regexp.Compile(allowance.Regexes[0])
			Expect(err).NotTo(HaveOccurred())
			Expect(pathPattern.MatchString(path) && digestPattern.MatchString(digest)).To(Equal(expected))
		},
		Entry("verified archive digest at its documented path", "docs/tests/2502/TEST_PLAN.md", archiveDigest, true),
		Entry("different value in the same document", "docs/tests/2502/TEST_PLAN.md", "1"+archiveDigest[1:], false),
		Entry("same value in another document", "docs/tests/2502/OTHER.md", archiveDigest, false),
	)
})
