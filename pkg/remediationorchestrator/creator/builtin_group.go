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

package creator

// builtInGroups contains the set of Kubernetes built-in API groups.
// Resources in these groups are core K8s objects, not operator-managed CRDs.
var builtInGroups = map[string]bool{
	"":                                 true, // core (Pod, Service, ConfigMap, Secret, etc.)
	"apps":                             true, // Deployment, StatefulSet, DaemonSet, ReplicaSet
	"batch":                            true, // Job, CronJob
	"autoscaling":                      true, // HPA
	"networking.k8s.io":                true, // Ingress, NetworkPolicy
	"policy":                           true, // PodDisruptionBudget
	"rbac.authorization.k8s.io":        true, // Role, ClusterRole, etc.
	"storage.k8s.io":                   true, // StorageClass, PV, PVC
	"coordination.k8s.io":              true, // Lease
	"node.k8s.io":                      true, // RuntimeClass
	"scheduling.k8s.io":                true, // PriorityClass
	"discovery.k8s.io":                 true, // EndpointSlice
	"admissionregistration.k8s.io":     true, // Webhook configs
}

// IsBuiltInGroup returns true if the given API group is a built-in Kubernetes
// group (core, apps, batch, etc.) and false for CRD groups. Used to detect
// operator-managed targets for async hash deferral (DD-EM-004, BR-RO-103.1).
//
// Non-built-in groups indicate a Custom Resource managed by an operator, where
// spec changes may propagate asynchronously after the WorkflowExecution completes.
func IsBuiltInGroup(group string) bool {
	return builtInGroups[group]
}
