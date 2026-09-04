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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Failure-diagnostics formatter for E2E specs that delete their test
// namespace in AfterEach: whatever is dumped here is the only pod-level
// evidence left in the CI log (must-gather only covers fixed namespaces).
var _ = Describe("SummarizePodsForDiagnostics", func() {
	It("UT-INFRA-DIAG-001: reports no-pods-found for an empty list (distinguishes never-created)", func() {
		out := SummarizePodsForDiagnostics("fp-e2e-crashloop-1", &corev1.PodList{})
		Expect(out).To(ContainSubstring("fp-e2e-crashloop-1"))
		Expect(out).To(ContainSubstring("no pods found"))
	})

	It("UT-INFRA-DIAG-002: surfaces CrashLoopBackOff waiting reason and restart count", func() {
		pods := &corev1.PodList{Items: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{Name: "crashloop-app-abc", Namespace: "fp-e2e-x"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name:         "crashloop-app",
					RestartCount: 7,
					State:        corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
				}},
			},
		}}}
		out := SummarizePodsForDiagnostics("fp-e2e-x", pods)
		Expect(out).To(ContainSubstring("crashloop-app-abc"))
		Expect(out).To(ContainSubstring("CrashLoopBackOff"))
		Expect(out).To(ContainSubstring("restarts=7"))
	})

	It("UT-INFRA-DIAG-003: surfaces pending pods with their unschedulable condition message", func() {
		pods := &corev1.PodList{Items: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{Name: "crashloop-app-def", Namespace: "fp-e2e-x"},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				Conditions: []corev1.PodCondition{{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/1 nodes are available: 1 node(s) had untolerated taint(s)",
				}},
			},
		}}}
		out := SummarizePodsForDiagnostics("fp-e2e-x", pods)
		Expect(out).To(ContainSubstring("Pending"))
		Expect(out).To(ContainSubstring("Unschedulable"))
		Expect(out).To(ContainSubstring("untolerated taint"))
	})

	It("UT-INFRA-DIAG-004: surfaces image-pull waiting reason and terminated exit codes", func() {
		pods := &corev1.PodList{Items: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{Name: "crashloop-app-ghi", Namespace: "fp-e2e-x"},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name:  "crashloop-app",
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}},
				}},
				InitContainerStatuses: []corev1.ContainerStatus{{
					Name:  "init",
					State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137}},
				}},
			},
		}}}
		out := SummarizePodsForDiagnostics("fp-e2e-x", pods)
		Expect(out).To(ContainSubstring("ImagePullBackOff"))
		Expect(out).To(ContainSubstring("OOMKilled"))
		Expect(out).To(ContainSubstring("exit=137"))
	})
})
