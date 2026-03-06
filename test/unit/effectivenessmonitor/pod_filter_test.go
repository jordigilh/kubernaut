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

package effectivenessmonitor

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	controller "github.com/jordigilh/kubernaut/internal/controller/effectivenessmonitor"
)

var _ = Describe("Pod Filtering and Health Stats (#246, BR-EM-001)", func() {

	Describe("FilterActivePods", func() {

		It("UT-EM-246-001: should exclude pods with DeletionTimestamp set", func() {
			now := metav1.Now()
			pods := []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "active"},
					Status:     corev1.PodStatus{Phase: corev1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "terminating", DeletionTimestamp: &now},
					Status:     corev1.PodStatus{Phase: corev1.PodRunning},
				},
			}
			active := controller.FilterActivePods(pods)
			Expect(active).To(HaveLen(1))
			Expect(active[0].Name).To(Equal("active"))
		})

		It("UT-EM-246-002: should exclude pods in Succeeded phase", func() {
			pods := []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "active"},
					Status:     corev1.PodStatus{Phase: corev1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "succeeded"},
					Status:     corev1.PodStatus{Phase: corev1.PodSucceeded},
				},
			}
			active := controller.FilterActivePods(pods)
			Expect(active).To(HaveLen(1))
			Expect(active[0].Name).To(Equal("active"))
		})

		It("UT-EM-246-003: should exclude pods in Failed phase", func() {
			pods := []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "active"},
					Status:     corev1.PodStatus{Phase: corev1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "failed"},
					Status:     corev1.PodStatus{Phase: corev1.PodFailed},
				},
			}
			active := controller.FilterActivePods(pods)
			Expect(active).To(HaveLen(1))
			Expect(active[0].Name).To(Equal("active"))
		})

		It("UT-EM-246-004: should return empty slice when all pods are inactive", func() {
			now := metav1.Now()
			pods := []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "terminating", DeletionTimestamp: &now},
					Status:     corev1.PodStatus{Phase: corev1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "succeeded"},
					Status:     corev1.PodStatus{Phase: corev1.PodSucceeded},
				},
			}
			active := controller.FilterActivePods(pods)
			Expect(active).To(BeEmpty())
		})

		It("UT-EM-246-005: should return all pods when none are terminating or terminal", func() {
			pods := []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "running"},
					Status:     corev1.PodStatus{Phase: corev1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pending"},
					Status:     corev1.PodStatus{Phase: corev1.PodPending},
				},
			}
			active := controller.FilterActivePods(pods)
			Expect(active).To(HaveLen(2))
		})
	})

	Describe("ComputePodHealthStats", func() {

		It("UT-EM-246-006: should not count restarts from pre-remediation pods", func() {
			remediationTime := metav1.NewTime(time.Now())
			preRemCreation := metav1.NewTime(time.Now().Add(-1 * time.Hour))

			pods := []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "pre-rem-pod",
						CreationTimestamp: preRemCreation,
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						ContainerStatuses: []corev1.ContainerStatus{
							{Name: "main", Ready: true, RestartCount: 5},
						},
					},
				},
			}
			status := controller.ComputePodHealthStats(pods, &remediationTime)
			Expect(status.TotalReplicas).To(Equal(int32(1)))
			Expect(status.ReadyReplicas).To(Equal(int32(1)))
			Expect(status.RestartsSinceRemediation).To(Equal(int32(0)),
				"pre-remediation pod's cumulative restarts should be excluded")
			Expect(status.TargetExists).To(BeTrue())
		})

		It("UT-EM-246-007: should count restarts from post-remediation pods", func() {
			remediationTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
			postRemCreation := metav1.NewTime(time.Now())

			pods := []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "post-rem-pod",
						CreationTimestamp: postRemCreation,
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						ContainerStatuses: []corev1.ContainerStatus{
							{Name: "main", Ready: true, RestartCount: 3},
						},
					},
				},
			}
			status := controller.ComputePodHealthStats(pods, &remediationTime)
			Expect(status.RestartsSinceRemediation).To(Equal(int32(3)),
				"post-remediation pod restarts should be counted")
		})

		It("UT-EM-246-008: should count all restarts when remediationStartedAt is nil", func() {
			pods := []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod"},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						ContainerStatuses: []corev1.ContainerStatus{
							{Name: "main", Ready: true, RestartCount: 4},
						},
					},
				},
			}
			status := controller.ComputePodHealthStats(pods, nil)
			Expect(status.RestartsSinceRemediation).To(Equal(int32(4)),
				"when remediationStartedAt is nil, all restarts should be counted (backward compat)")
		})

		It("UT-EM-246-009: should detect CrashLoopBackOff from container waiting state", func() {
			pods := []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "crash-pod"},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						ContainerStatuses: []corev1.ContainerStatus{
							{
								Name:  "main",
								Ready: false,
								State: corev1.ContainerState{
									Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
								},
								RestartCount: 10,
							},
						},
					},
				},
			}
			status := controller.ComputePodHealthStats(pods, nil)
			Expect(status.CrashLoops).To(BeTrue())
		})

		It("UT-EM-246-010: should count pending pods", func() {
			pods := []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "running-pod"},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						ContainerStatuses: []corev1.ContainerStatus{
							{Name: "main", Ready: true, RestartCount: 0},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pending-pod"},
					Status:     corev1.PodStatus{Phase: corev1.PodPending},
				},
			}
			status := controller.ComputePodHealthStats(pods, nil)
			Expect(status.TotalReplicas).To(Equal(int32(2)))
			Expect(status.ReadyReplicas).To(Equal(int32(1)))
			Expect(status.PendingCount).To(Equal(int32(1)))
		})
	})
})
