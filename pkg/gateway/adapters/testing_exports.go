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

package adapters

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
)

// NewTestAPIResourceRegistry creates an APIResourceRegistry backed by fake
// discovery with standard Kubernetes API resources. Use this in tests instead
// of passing a nil registry to NewPrometheusAdapter.
//
// Registered resources: Deployment, StatefulSet, DaemonSet, ReplicaSet (apps/v1),
// Pod, Node, Service, PersistentVolumeClaim (v1), Job, CronJob (batch/v1),
// HorizontalPodAutoscaler (autoscaling/v2), PodDisruptionBudget (policy/v1).
func NewTestAPIResourceRegistry() *APIResourceRegistry {
	cs := fakeclientset.NewSimpleClientset()
	fd := cs.Discovery().(*fakediscovery.FakeDiscovery)
	fd.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Name: "deployments", SingularName: "deployment", Kind: "Deployment", Namespaced: true},
				{Name: "statefulsets", SingularName: "statefulset", Kind: "StatefulSet", Namespaced: true},
				{Name: "daemonsets", SingularName: "daemonset", Kind: "DaemonSet", Namespaced: true},
				{Name: "replicasets", SingularName: "replicaset", Kind: "ReplicaSet", Namespaced: true},
			},
		},
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", SingularName: "pod", Kind: "Pod", Namespaced: true},
				{Name: "nodes", SingularName: "node", Kind: "Node", Namespaced: false},
				{Name: "services", SingularName: "service", Kind: "Service", Namespaced: true},
				{Name: "persistentvolumeclaims", SingularName: "persistentvolumeclaim", Kind: "PersistentVolumeClaim", Namespaced: true},
			},
		},
		{
			GroupVersion: "batch/v1",
			APIResources: []metav1.APIResource{
				{Name: "jobs", SingularName: "job", Kind: "Job", Namespaced: true},
				{Name: "cronjobs", SingularName: "cronjob", Kind: "CronJob", Namespaced: true},
			},
		},
		{
			GroupVersion: "autoscaling/v2",
			APIResources: []metav1.APIResource{
				{Name: "horizontalpodautoscalers", SingularName: "horizontalpodautoscaler", Kind: "HorizontalPodAutoscaler", Namespaced: true},
			},
		},
		{
			GroupVersion: "policy/v1",
			APIResources: []metav1.APIResource{
				{Name: "poddisruptionbudgets", SingularName: "poddisruptionbudget", Kind: "PodDisruptionBudget", Namespaced: true},
			},
		},
	}
	registry, err := NewAPIResourceRegistry(fd)
	if err != nil {
		panic("NewTestAPIResourceRegistry: " + err.Error())
	}
	return registry
}
