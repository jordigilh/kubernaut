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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Must-gather curl fixture [BR-PLATFORM-001.6a]", func() {
	It("UT-MG-2493-001: serves distinct endpoint bodies through curl's output/write-out contract", func() {
		GinkgoT().Setenv("BATS_TEST_TMPDIR", GinkgoT().TempDir())
		GinkgoT().Setenv("SECURITY_FIXTURE_HELPERS", filepath.Join(getProjectRoot(), "cmd", "must-gather", "test", "helpers.bash"))
		output, err := runSecurityShell(`source "$SECURITY_FIXTURE_HELPERS"
setup_test_environment
trap teardown_test_environment EXIT
create_mock_datastorage_workflows
create_mock_datastorage_audit
mock_curl "$TEST_TEMP_DIR/workflows.json" "$TEST_TEMP_DIR/audit-events.json"
status=$(curl --silent --show-error --output "$TEST_TEMP_DIR/workflow-body.json" --write-out '%{http_code}' 'https://data-storage-service.fixture.svc:8080/api/v1/workflows?limit=50')
test "$status" = 200
jq -e '.workflows[0].name == "workflow-1"' "$TEST_TEMP_DIR/workflow-body.json"
status=$(curl --silent --show-error --output "$TEST_TEMP_DIR/audit-body.json" --write-out '%{http_code}' 'https://data-storage-service.fixture.svc:8080/api/v1/audit/events?limit=1000&offset=0&since=24h')
test "$status" = 200
jq -e '.data[0].event_type == "remediation.created"' "$TEST_TEMP_DIR/audit-body.json"
grep -F 'data-storage-service.fixture.svc:8080/api/v1/audit/events?' "$TEST_TEMP_DIR/curl-calls.log"
`)
		Expect(err).NotTo(HaveOccurred(), "fixture output: %s", output)
		Expect(output).To(ContainSubstring("data-storage-service.fixture.svc:8080/api/v1/audit/events?"))
	})
})

var _ = Describe("Must-gather image inventory [BR-PLATFORM-001]", func() {
	DescribeTable("UT-MG-2493-003: actual workflow verifies required executable collector files, not a stale count",
		func(mode string, succeeds bool) {
			dir := GinkgoT().TempDir()
			imageRoot := filepath.Join(dir, "image")
			collectors := filepath.Join(imageRoot, "collectors")
			Expect(os.MkdirAll(collectors, 0755)).To(Succeed())
			Expect(os.MkdirAll(filepath.Join(imageRoot, "sanitizers"), 0755)).To(Succeed())
			files, err := filepath.Glob(filepath.Join(getProjectRoot(), "cmd", "must-gather", "collectors", "*.sh"))
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(ContainElement(filepath.Join(getProjectRoot(), "cmd", "must-gather", "collectors", "fleet-service-path.sh")))
			for _, file := range files {
				name := filepath.Base(file)
				if mode == "missing" && name == "fleet-service-path.sh" {
					continue
				}
				permissions := os.FileMode(0755)
				if mode == "not-executable" && name == "fleet-service-path.sh" {
					permissions = 0644
				}
				Expect(os.WriteFile(filepath.Join(collectors, name), []byte("#!/bin/sh\n"), permissions)).To(Succeed())
			}
			for _, name := range []string{"gather", "sanitizers/sanitize-all.sh", "kubectl"} {
				Expect(os.WriteFile(filepath.Join(imageRoot, name), []byte("#!/bin/sh\n"), 0755)).To(Succeed())
			}
			Expect(os.WriteFile(filepath.Join(dir, "podman"), []byte(`#!/usr/bin/env bash
set -euo pipefail
command="${@: -1}"
command="${command//\/usr\/share\/must-gather/$CONTAINER_FIXTURE_ROOT}"
command="${command//\/usr\/bin\/gather/$CONTAINER_FIXTURE_ROOT/gather}"
bash -e -o pipefail -c "$command"
`), 0755)).To(Succeed())
			GinkgoT().Setenv("PATH", dir+string(os.PathListSeparator)+imageRoot+string(os.PathListSeparator)+os.Getenv("PATH"))
			GinkgoT().Setenv("CONTAINER_FIXTURE_ROOT", imageRoot)
			script := readSecurityRunBlock(".github/workflows/must-gather-tests.yml", "Verify container contents")
			GinkgoT().Setenv("SECURITY_FIXTURE_WORKTREE", getProjectRoot())
			output, runErr := runSecurityShell("cd \"$SECURITY_FIXTURE_WORKTREE\"\n" + script)
			if succeeds {
				Expect(runErr).NotTo(HaveOccurred(), "output: %s", output)
				return
			}
			Expect(runErr).To(HaveOccurred(), "missing/non-executable collector must fail: %s", output)
			Expect(output).To(ContainSubstring("fleet-service-path.sh"), "identify the required diagnostic file")
		},
		Entry("complete image", "complete", true),
		Entry("missing Fleet collector", "missing", false),
		Entry("non-executable Fleet collector", "not-executable", false),
	)
})
