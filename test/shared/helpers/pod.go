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

package helpers

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureTestPod creates a minimal Pod in the given namespace if it doesn't already exist.
// The Pod uses the pause image (pre-cached in Kind clusters) so it starts instantly.
//
// This is required for E2E tests that send alerts referencing pod names -- the Gateway's
// OwnerResolver (#282, #284) queries the K8s API to resolve the owner chain, and drops
// signals for pods that don't exist.
//
// The created Pod has no ownerReferences, so the OwnerResolver will produce a pod-level
// fingerprint. For tests that need deployment-level fingerprints, use EnsureTestDeployment.
func EnsureTestPod(ctx context.Context, k8sClient client.Client, namespace, name string) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "pause",
				Image: "registry.k8s.io/pause:3.10",
			}},
		},
	}

	err := k8sClient.Create(ctx, pod)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to create test pod %s/%s for owner resolution", namespace, name))
	}
}

// EnsureTestPods creates multiple minimal Pods in the given namespace.
// Convenience wrapper around EnsureTestPod for tests that reference multiple pods.
func EnsureTestPods(ctx context.Context, k8sClient client.Client, namespace string, names ...string) {
	for _, name := range names {
		EnsureTestPod(ctx, k8sClient, namespace, name)
	}
	GinkgoWriter.Printf("Created %d test pods in namespace %s for owner resolution\n", len(names), namespace)
}
