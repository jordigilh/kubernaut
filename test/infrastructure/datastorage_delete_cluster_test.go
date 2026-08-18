/*
Copyright 2025 Jordi Gil.

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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Issue #2185: a Kind cluster deleted by Go before the GitHub Actions
// "Collect must-gather logs" step (if: failure() || cancelled()) can run
// leaves that step nothing to inspect. This happened whenever a job was
// cancelled after Ginkgo already observed testsFailed=false: DeleteCluster
// fell through to an unconditional `kind delete cluster`, racing the
// workflow's own diagnostic-collection step.
//
// shouldDeleteClusterNow is the extracted decision point that fixes this:
// in CI/CD, Go must never delete the cluster itself, regardless of
// testsFailed -- the GitHub Actions "Cleanup Kind cluster" step (if:
// always()) is the sole owner of teardown there, and it already correctly
// runs after diagnostic collection in every outcome. Locally, the cluster
// is preserved by default (to aid troubleshooting) unless the caller opts
// into the old auto-delete behavior.
var _ = Describe("shouldDeleteClusterNow", func() {
	DescribeTable("deciding whether DeleteCluster should invoke `kind delete cluster` itself",
		func(inCICD, cleanupOptIn, expected bool) {
			Expect(shouldDeleteClusterNow(inCICD, cleanupOptIn)).To(Equal(expected))
		},
		Entry("UT-INFRA-DELCLUSTER-001: CI/CD never deletes, even with cleanup opted in",
			true, true, false),
		Entry("UT-INFRA-DELCLUSTER-002: CI/CD never deletes, opted out",
			true, false, false),
		Entry("UT-INFRA-DELCLUSTER-003: local preserves by default (no opt-in)",
			false, false, false),
		Entry("UT-INFRA-DELCLUSTER-004: local deletes when explicitly opted in",
			false, true, true),
	)
})
