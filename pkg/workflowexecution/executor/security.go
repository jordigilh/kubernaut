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

package executor

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

// restrictedPodSecurityContext returns the pod-level SecurityContext applied to
// every spawned execution pod (Job and Tekton). Mirrors the controller's own
// restricted profile (charts/kubernaut/templates/_helpers.tpl: kubernaut.podSecurityContext).
//
// Authority: BR-WE-018 (Execution Pod Security Hardening), FedRAMP AC-6/CM-7.
// This profile is intentionally non-configurable -- see BR-WE-018 for rationale.
func restrictedPodSecurityContext() *corev1.PodSecurityContext {
	return &corev1.PodSecurityContext{
		RunAsNonRoot: ptr.To(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

// restrictedContainerSecurityContext returns the container-level SecurityContext
// applied to the "workflow" container in spawned Job pods. Mirrors the
// controller's own restricted profile (kubernaut.containerSecurityContext).
//
// Not applicable to Tekton: PipelineRun's TaskRunTemplate.PodTemplate exposes
// only a pod-level SecurityContext; container-level settings belong to the
// Task spec resolved from the OCI bundle, outside WE controller's authoring
// control (see BR-WE-018 for the asymmetry rationale).
//
// Authority: BR-WE-018 (Execution Pod Security Hardening), FedRAMP AC-6/CM-7.
func restrictedContainerSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: ptr.To(false),
		ReadOnlyRootFilesystem:   ptr.To(true),
		RunAsNonRoot:             ptr.To(true),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
	}
}

const scratchSpaceMountPath = "/tmp"

// scratchSpaceVolume returns the emptyDir volume providing writable scratch
// space for the workflow container under readOnlyRootFilesystem: true.
//
// Authority: BR-WE-018 (Execution Pod Security Hardening).
func scratchSpaceVolume() corev1.Volume {
	return corev1.Volume{
		Name: "tmp",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// scratchSpaceVolumeMount mounts the scratch space volume at /tmp.
func scratchSpaceVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "tmp",
		MountPath: scratchSpaceMountPath,
	}
}

// scratchSpaceEnvVars points HOME and TMPDIR at the writable scratch space so
// tools that default to writing under $HOME (e.g. kubectl's discovery cache)
// don't fail under readOnlyRootFilesystem: true.
func scratchSpaceEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: "HOME", Value: scratchSpaceMountPath},
		{Name: "TMPDIR", Value: scratchSpaceMountPath},
	}
}
