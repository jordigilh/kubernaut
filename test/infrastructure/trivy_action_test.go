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
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

var _ = Describe("Trivy CI action [BR-SECURITY-2493]", func() {
	DescribeTable("UT-SEC-2493-004/005/006: production script retains output/status and bounded classification under errexit",
		func(mode string, expectedStatus int, expectedAttempts int, expectedOutput string) {
			dir := GinkgoT().TempDir()
			GinkgoT().Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			GinkgoT().Setenv("TRIVY_FIXTURE_MODE", mode)
			GinkgoT().Setenv("TRIVY_FIXTURE_CALLS", filepath.Join(dir, "calls"))
			GinkgoT().Setenv("SLEEP_FIXTURE_CALLS", filepath.Join(dir, "sleeps"))
			Expect(os.WriteFile(filepath.Join(dir, "trivy"), []byte(`#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$TRIVY_FIXTURE_CALLS"
attempt=$(wc -l < "$TRIVY_FIXTURE_CALLS")
case "$TRIVY_FIXTURE_MODE" in
  clean) printf 'fixture clean report\n'; exit 0 ;;
  finding) printf 'fixture HIGH vulnerability report\n'; printf 'fixture scanner diagnostic\n' >&2; exit 1 ;;
  recover)
    if [ "$attempt" -eq 1 ]; then printf 'failed to download vulnerability DB: fixture transient error\n' >&2; exit 2; fi
    printf 'fixture recovered report\n'; exit 0 ;;
  exhausted) printf 'failed to download vulnerability DB: fixture exhausted error\n' >&2; exit 2 ;;
  config) printf 'fixture invalid scanner configuration\n' >&2; exit 7 ;;
  startup) printf 'fixture scanner command could not start\n' >&2; exit 127 ;;
  *) printf 'unknown fixture mode\n' >&2; exit 9 ;;
esac
`), 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(dir, "sleep"), []byte("#!/usr/bin/env bash\nprintf '%s\\n' \"$*\" >> \"$SLEEP_FIXTURE_CALLS\"\n"), 0755)).To(Succeed())
			script := readSecurityRunBlock(".github/actions/trivy-scan-with-retry/action.yml", "Scan image for CVEs (Trivy, with DB-download retry)")
			for name, value := range map[string]string{
				"image-ref": "test-owned-image", "format": "table", "exit-code": "1",
				"severity": "CRITICAL,HIGH", "ignore-unfixed": "true", "trivyignores": "", "max-attempts": "3",
			} {
				script = strings.ReplaceAll(script, "${{ inputs."+name+" }}", value)
			}
			output, err := runSecurityShell(script)
			if expectedStatus == 0 {
				Expect(err).NotTo(HaveOccurred(), "output: %s", output)
			} else {
				Expect(err).To(MatchError(ContainSubstring("exit status "+strconv.Itoa(expectedStatus))), "output: %s", output)
			}
			Expect(output).To(ContainSubstring(expectedOutput))
			calls, readErr := os.ReadFile(filepath.Join(dir, "calls"))
			Expect(readErr).NotTo(HaveOccurred())
			Expect(strings.Split(strings.TrimSpace(string(calls)), "\n")).To(HaveLen(expectedAttempts))
			Expect(string(calls)).To(ContainSubstring("--severity CRITICAL,HIGH"))
			Expect(string(calls)).To(ContainSubstring("--exit-code 1"))
			Expect(string(calls)).To(ContainSubstring("--ignore-unfixed"))
			Expect(string(calls)).NotTo(ContainSubstring("--ignorefile"), "application scan must not receive diagnostic-image exceptions")
			sleeps, sleepErr := os.ReadFile(filepath.Join(dir, "sleeps"))
			if expectedAttempts == 1 {
				Expect(os.IsNotExist(sleepErr)).To(BeTrue(), "non-transient errors must not trigger backoff")
			} else {
				Expect(sleepErr).NotTo(HaveOccurred())
				Expect(string(sleeps)).To(Equal(strings.Join([]string{"15", "30"}[:expectedAttempts-1], "\n") + "\n"))
			}
		},
		Entry("success", "clean", 0, 1, "fixture clean report"),
		Entry("genuine finding", "finding", 1, 1, "fixture HIGH vulnerability report\nfixture scanner diagnostic"),
		Entry("transient failure recovers", "recover", 0, 2, "fixture recovered report"),
		Entry("transient failure exhausts the bound", "exhausted", 2, 3, "fixture exhausted error"),
		Entry("non-transient failure", "config", 7, 1, "fixture invalid scanner configuration"),
		Entry("scanner startup failure", "startup", 127, 1, "fixture scanner command could not start"),
	)
})

func readSecurityRunBlock(relativePath, name string) string {
	var document yaml.Node
	ExpectWithOffset(1, yaml.Unmarshal([]byte(readSecurityFixtureSource(relativePath)), &document)).To(Succeed())
	script := findSecurityRunBlock(&document, name)
	ExpectWithOffset(1, script).NotTo(BeEmpty(), "production YAML step: %s", name)
	return script
}

func findSecurityRunBlock(node *yaml.Node, name string) string {
	if securityYAMLValue(node, "name") == name {
		return securityYAMLValue(node, "run")
	}
	for _, child := range node.Content {
		if script := findSecurityRunBlock(child, name); script != "" {
			return script
		}
	}
	return ""
}

func securityYAMLValue(node *yaml.Node, key string) string {
	if node.Kind != yaml.MappingNode {
		return ""
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			return node.Content[index+1].Value
		}
	}
	return ""
}

func runSecurityShell(script string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "bash", "-e", "-o", "pipefail", "-c", script).CombinedOutput() //nolint:gosec // G204: actual repository run block; only external commands are fixtures
	return string(output), err
}
