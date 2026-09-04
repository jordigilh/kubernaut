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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// SummarizePodsForDiagnostics renders a compact, human-readable summary of
// every pod in list for failure triage. E2E specs that delete their test
// namespace in AfterEach call this (via GinkgoWriter) when a wait times out:
// the dump is the only pod-level evidence left in the CI log, since
// must-gather only covers fixed namespaces and the test namespace is gone
// before collection runs.
//
// Pure function (no API calls) so it is unit-testable without a cluster;
// callers own listing the pods. An empty list renders explicitly as
// "no pods found" to distinguish "never created/scheduled" from "ran but
// never reached the awaited state".
func SummarizePodsForDiagnostics(namespace string, pods *corev1.PodList) string {
	var b strings.Builder
	if pods == nil || len(pods.Items) == 0 {
		fmt.Fprintf(&b, "diagnostics namespace=%s: no pods found\n", namespace)
		return b.String()
	}
	fmt.Fprintf(&b, "diagnostics namespace=%s pods=%d\n", namespace, len(pods.Items))
	for _, pod := range pods.Items {
		fmt.Fprintf(&b, "- pod=%s phase=%s node=%s\n",
			pod.Name, pod.Status.Phase, pod.Spec.NodeName)
		for _, cs := range pod.Status.InitContainerStatuses {
			fmt.Fprintf(&b, "    init=%s %s\n", cs.Name, summarizeContainerState(cs))
		}
		for _, cs := range pod.Status.ContainerStatuses {
			fmt.Fprintf(&b, "    container=%s restarts=%d %s\n",
				cs.Name, cs.RestartCount, summarizeContainerState(cs))
		}
		for _, cond := range pod.Status.Conditions {
			if cond.Status != corev1.ConditionTrue {
				fmt.Fprintf(&b, "    condition=%s status=%s reason=%s message=%s\n",
					cond.Type, cond.Status, cond.Reason, cond.Message)
			}
		}
	}
	return b.String()
}

func summarizeContainerState(cs corev1.ContainerStatus) string {
	switch {
	case cs.State.Waiting != nil:
		return "waiting=" + cs.State.Waiting.Reason
	case cs.State.Terminated != nil:
		return fmt.Sprintf("terminated=%s exit=%d",
			cs.State.Terminated.Reason, cs.State.Terminated.ExitCode)
	default:
		// Running or a zero value (e.g. a status entry with no state set
		// yet) -- either way the container is not waiting or terminated.
		return "running"
	}
}
