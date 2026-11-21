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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Issue #2688: spoke remediation Jobs failed BackoffLimitExceeded ~3s after
// start, but the WFE controller's Cleanup deletes the Job (and its pods with
// it) as soon as the WFE reaches a terminal phase -- long before the suite's
// must-gather runs -- so jobs/all-jobs.yaml was empty and no pod log existed.
// CaptureWorkflowJobDiagnostics is the failure-time replacement: it must run
// kubectl against the right kubeconfig/namespace and collect pod LOGS (the
// missing evidence), not just object dumps.
var _ = Describe("BuildWorkflowJobDiagnosticsCommands", func() {
	It("UT-INFRA-JOBDIAG-2688-001: targets the given kubeconfig and execution namespace on every command", func() {
		cmds := BuildWorkflowJobDiagnosticsCommands("/tmp/fleet-e2e-remote-config", "wfe-abc-1234", "kubernaut-workflows")

		Expect(cmds).NotTo(BeEmpty())
		for _, cmd := range cmds {
			Expect(cmd.Args).To(ContainElement("--kubeconfig"))
			Expect(cmd.Args).To(ContainElement("/tmp/fleet-e2e-remote-config"))
			Expect(cmd.Args).To(ContainElement("-n"))
			Expect(cmd.Args).To(ContainElement("kubernaut-workflows"))
		}
	})

	It("UT-INFRA-JOBDIAG-2688-002: captures pod logs across every Job-owned pod, not just the latest", func() {
		cmds := BuildWorkflowJobDiagnosticsCommands("/tmp/kc", "wfe-abc-1234", "kubernaut-workflows")

		var logs *JobDiagnosticsCommand
		for i := range cmds {
			if cmds[i].Label == "pod-logs" {
				logs = &cmds[i]
				break
			}
		}
		Expect(logs).NotTo(BeNil(), "a pod-logs command is required (issue #2688: no pod logs were captured)")
		Expect(logs.Args).To(ContainElement("logs"))
		// The job-name label selector covers every Pod the Job controller
		// created (BackoffLimit retries), unlike `kubectl logs job/<name>`.
		Expect(logs.Args).To(ContainElement("-l"))
		Expect(logs.Args).To(ContainElement("job-name=wfe-abc-1234"))
		Expect(logs.Args).To(ContainElement("--all-containers"))
		Expect(logs.Args).To(ContainElement("--timestamps"))
	})

	It("UT-INFRA-JOBDIAG-2688-003: captures the Job spec, pod descriptions and namespace events", func() {
		cmds := BuildWorkflowJobDiagnosticsCommands("/tmp/kc", "wfe-abc-1234", "kubernaut-workflows")

		labels := make([]string, 0, len(cmds))
		for _, cmd := range cmds {
			labels = append(labels, cmd.Label)
		}
		Expect(labels).To(ContainElement("job-spec"))
		Expect(labels).To(ContainElement("pod-describe"))
		Expect(labels).To(ContainElement("events"))

		for _, cmd := range cmds {
			if cmd.Label == "job-spec" {
				Expect(cmd.Args).To(ContainElement("wfe-abc-1234"))
			}
		}
	})
})
