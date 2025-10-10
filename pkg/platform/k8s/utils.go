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

package k8s

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ResourceRequirements represents custom resource requirements
type ResourceRequirements struct {
	CPULimit      string
	MemoryLimit   string
	CPURequest    string
	MemoryRequest string
}

// ToK8sResourceRequirements converts custom ResourceRequirements to corev1.ResourceRequirements
func (r ResourceRequirements) ToK8sResourceRequirements() (corev1.ResourceRequirements, error) {
	result := corev1.ResourceRequirements{
		Limits:   make(corev1.ResourceList),
		Requests: make(corev1.ResourceList),
	}

	if r.CPULimit != "" {
		if qty, err := resource.ParseQuantity(r.CPULimit); err != nil {
			return result, err
		} else {
			result.Limits[corev1.ResourceCPU] = qty
		}
	}

	if r.MemoryLimit != "" {
		if qty, err := resource.ParseQuantity(r.MemoryLimit); err != nil {
			return result, err
		} else {
			result.Limits[corev1.ResourceMemory] = qty
		}
	}

	if r.CPURequest != "" {
		if qty, err := resource.ParseQuantity(r.CPURequest); err != nil {
			return result, err
		} else {
			result.Requests[corev1.ResourceCPU] = qty
		}
	}

	if r.MemoryRequest != "" {
		if qty, err := resource.ParseQuantity(r.MemoryRequest); err != nil {
			return result, err
		} else {
			result.Requests[corev1.ResourceMemory] = qty
		}
	}

	return result, nil
}
