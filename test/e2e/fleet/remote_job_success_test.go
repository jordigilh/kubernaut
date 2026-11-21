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

package fleet

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// expectRemoteWorkflowJobSucceeds asserts that the Job a WFE dispatched to
// the spoke cluster (DD-TEST-013) completes successfully -- the assertion
// Issue #2688 showed was missing: two spoke remediation Jobs failed with
// BackoffLimitExceeded ~3s after start while their owning specs (routing and
// cluster-override field assertions only) kept passing.
//
// The moment the Job reports a failed pod, pod logs / pod describe / job
// spec / events are captured via
// infrastructure.CaptureWorkflowJobDiagnostics -- at failure time, BEFORE the
// WFE controller's Cleanup deletes the Job and its pods, which is what made
// #2688 undiagnosable (must-gather's jobs/all-jobs.yaml was empty).
//
// Authority: Issue #2688, BR-INTEGRATION-054 (real spoke remediation),
// FedRAMP: AU-2/AU-3 (failure evidence with actor attribution via the Job's
// provenance labels).
func expectRemoteWorkflowJobSucceeds(wfeName string) {
	GinkgoHelper()
	var diagnosticsCaptured bool
	Eventually(func(g Gomega) {
		jobs := &batchv1.JobList{}
		g.Expect(remoteK8sClient.List(ctx, jobs,
			client.InNamespace(infrastructure.ExecutionNamespace),
			client.MatchingLabels{"kubernaut.ai/workflow-execution": wfeName})).To(Succeed(),
			"failed to list spoke Jobs in %s", infrastructure.ExecutionNamespace)
		g.Expect(jobs.Items).NotTo(BeEmpty(),
			"no Job found on the spoke cluster for WFE %s (issue #2688: silent dispatch failure)", wfeName)

		job := jobs.Items[0]
		if job.Status.Failed > 0 && !diagnosticsCaptured {
			// Failure time: the WFE controller will delete this Job (and its
			// pods) as soon as it observes the terminal state, so capture the
			// pod logs NOW, inside the poll, not in any post-hoc hook.
			diagnosticsCaptured = true
			infrastructure.CaptureWorkflowJobDiagnostics(ctx, remoteKubeconfigPath, wfeName,
				job.Name, infrastructure.ExecutionNamespace, GinkgoWriter)
		}
		g.Expect(job.Status.Failed).To(BeZero(),
			"spoke Job %s has %d failed pod(s) -- real remediate.sh execution failed (pod logs above / issue #2688)",
			job.Name, job.Status.Failed)
		g.Expect(job.Status.Succeeded).To(BeNumerically(">", 0),
			"spoke Job %s has not succeeded yet (active=%d)", job.Name, job.Status.Active)
	}, timeout, interval).Should(Succeed(),
		"spoke remediation Job for WFE %s must succeed (issue #2688: previously unasserted)", wfeName)
}
