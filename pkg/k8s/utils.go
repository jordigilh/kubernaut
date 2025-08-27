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

// parseQuantity parses a resource quantity string
func parseQuantity(size string) (resource.Quantity, error) {
	return resource.ParseQuantity(size)
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
