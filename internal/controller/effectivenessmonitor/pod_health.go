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

package controller

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/health"
)

// FilterActivePods returns only pods that are active workload members:
// pods that are not terminating (DeletionTimestamp == nil) and not in a
// terminal phase (Succeeded/Failed). Exported for unit testing (#246).
func FilterActivePods(pods []corev1.Pod) []*corev1.Pod {
	active := make([]*corev1.Pod, 0, len(pods))
	for i := range pods {
		pod := &pods[i]
		if pod.DeletionTimestamp != nil {
			continue
		}
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		active = append(active, pod)
	}
	return active
}

// ComputePodHealthStats aggregates health indicators from a set of active pods.
// remediationStartedAt controls restart counting: pods created before that time
// have their cumulative RestartCount excluded (they predate the remediation).
// Exported for unit testing (#246).
func ComputePodHealthStats(pods []*corev1.Pod, remediationStartedAt *metav1.Time) health.TargetStatus {
	totalReplicas := int32(len(pods))
	readyReplicas := int32(0)
	totalRestarts := int32(0)
	crashLoops := false
	oomKilled := false
	pendingCount := int32(0)

	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodPending {
			pendingCount++
		}

		preRemediationPod := remediationStartedAt != nil && !pod.CreationTimestamp.Time.After(remediationStartedAt.Time)

		for _, cs := range pod.Status.ContainerStatuses {
			if !preRemediationPod {
				totalRestarts += cs.RestartCount
			}
			if cs.Ready {
				readyReplicas++
				break
			}
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				crashLoops = true
			}
			if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
				oomKilled = true
			}
		}
	}

	return health.TargetStatus{
		TotalReplicas:            totalReplicas,
		ReadyReplicas:            readyReplicas,
		RestartsSinceRemediation: totalRestarts,
		TargetExists:            true,
		CrashLoops:              crashLoops,
		OOMKilled:               oomKilled,
		PendingCount:            pendingCount,
	}
}
