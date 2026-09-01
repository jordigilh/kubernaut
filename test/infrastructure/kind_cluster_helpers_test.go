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

// Issue #1542 follow-up: Kind CLI v0.30.0 was hard-pinned via exact-match
// string comparison ("kind v0.30."), which broke every time the CLI needed
// a bump (e.g. containerd config v4 support requires Kind CLI >= v0.32.0
// for kindest/node:v1.36.1+). checkKindVersionOutput replaces the exact
// match with a minimum-version check so future Kind releases don't require
// a code change here.
var _ = Describe("checkKindVersionOutput", func() {
	DescribeTable("version validation against the minimum supported Kind CLI version",
		func(versionOutput string, expectErr bool) {
			err := checkKindVersionOutput(versionOutput)
			if expectErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry("UT-INFRA-KIND-001: accepts the exact minimum version",
			"kind v0.32.0 go1.26.4 linux/amd64", false),
		Entry("UT-INFRA-KIND-002: accepts a newer minor version",
			"kind v0.33.1 go1.26.4 linux/amd64", false),
		Entry("UT-INFRA-KIND-003: accepts a newer major version",
			"kind v1.0.0 go1.26.4 linux/amd64", false),
		Entry("UT-INFRA-KIND-004: rejects a version below the minimum minor",
			"kind v0.30.0 go1.25.0 darwin/arm64", true),
		Entry("UT-INFRA-KIND-005: rejects a much older version",
			"kind v0.20.0 go1.24.0 linux/amd64", true),
		Entry("UT-INFRA-KIND-006: rejects unparsable output",
			"command not found: kind", true),
		Entry("UT-INFRA-KIND-007: rejects empty output",
			"", true),
	)
})

// Issue #2327/#2326 helios08 fleet E2E triage: an earlier version of
// clusterHasLiveNode checked only `err == nil && output != ""`, which is
// always true for `kind get nodes` -- it exits 0 and prints diagnostic
// banner lines regardless of whether any node exists, only signaling "no
// nodes" via the literal text below. That false-positive let the cluster
// "reuse" fast path in CreateKindClusterWithExtraMounts reuse a stale,
// nodeless cluster registration, so every subsequent `kind load
// image-archive` failed with "no nodes found for cluster" after an entire
// image-build cycle had already run.
var _ = Describe("hasLiveNodeInOutput", func() {
	DescribeTable("detecting a live node in `kind get nodes` combined output",
		func(output string, expectLive bool) {
			Expect(hasLiveNodeInOutput(output)).To(Equal(expectLive))
		},
		Entry("UT-INFRA-KIND-008: reports live when a real node name is present",
			"using podman due to KIND_EXPERIMENTAL_PROVIDER\nenabling experimental podman provider\nfleet-e2e-control-plane\n",
			true),
		Entry("UT-INFRA-KIND-009: reports not live when kind reports zero nodes, "+
			"despite non-empty banner output (the exact false-positive this guards against)",
			"using podman due to KIND_EXPERIMENTAL_PROVIDER\nenabling experimental podman provider\nNo kind nodes found for cluster \"fleet-e2e\".\n",
			false),
		Entry("UT-INFRA-KIND-010: reports not live for completely empty output",
			"", false),
	)
})
