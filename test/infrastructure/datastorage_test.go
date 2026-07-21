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

// Issue #1690 RCA follow-up: multi-cluster E2E suites (fleet,
// fleetmetadatacache, fleetmetadatacache/eaigw) call MustGatherPodLogs once
// per cluster with the same serviceName+namespace pair (e.g. both the
// primary and remote cluster have a "kubernaut-system" namespace) -- only
// clusterName differs. mustGatherDirPath must produce a distinct directory
// per cluster or the second call's must-gather pass silently overwrites the
// first cluster's namespace-level diagnostic files.
var _ = Describe("mustGatherDirPath", func() {
	DescribeTable("directory path construction",
		func(serviceName, clusterName, namespace, expected string) {
			Expect(mustGatherDirPath(serviceName, clusterName, namespace)).To(Equal(expected))
		},
		Entry("UT-INFRA-MG-001: primary cluster",
			"fleet", "fleet-e2e", "kubernaut-system",
			"/tmp/kubernaut-must-gather/fleet/fleet-e2e/kubernaut-system"),
		Entry("UT-INFRA-MG-002: remote cluster, same service+namespace as primary",
			"fleet", "fleet-e2e-remote", "kubernaut-system",
			"/tmp/kubernaut-must-gather/fleet/fleet-e2e-remote/kubernaut-system"),
	)

	It("UT-INFRA-MG-003: produces distinct paths for two clusters sharing serviceName and namespace", func() {
		primary := mustGatherDirPath("fleet", "fleet-e2e", "kubernaut-system")
		remote := mustGatherDirPath("fleet", "fleet-e2e-remote", "kubernaut-system")
		Expect(primary).ToNot(Equal(remote),
			"distinct clusters must never collide onto the same must-gather directory, or the second cluster's collection silently destroys the first cluster's diagnostic files (events.txt, jobs.txt, pod_status.*)")
	})
})
