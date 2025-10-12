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

package kind

import (
	"fmt"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WaitForPodReady waits for a pod to be in Ready state.
// Uses Gomega's Eventually() for asynchronous assertions.
//
// Example:
//
//	pod := suite.WaitForPodReady("monitoring", "prometheus-0", 60*time.Second)
func (s *IntegrationSuite) WaitForPodReady(namespace, name string, timeout time.Duration) *corev1.Pod {
	var pod *corev1.Pod
	Eventually(func() bool {
		var err error
		pod, err = s.Client.CoreV1().Pods(namespace).Get(
			s.Context, name, metav1.GetOptions{})
		if err != nil {
			return false
		}

		// Check if pod is ready
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				return true
			}
		}
		return false
	}, timeout, 1*time.Second).Should(BeTrue(),
		fmt.Sprintf("Pod %s/%s should be ready within %v", namespace, name, timeout))

	return pod
}

// WaitForPodsReady waits for all pods matching a label selector to be ready.
//
// Example:
//
//	pods := suite.WaitForPodsReady("monitoring", "app=prometheus", 60*time.Second)
func (s *IntegrationSuite) WaitForPodsReady(namespace, labelSelector string, timeout time.Duration) *corev1.PodList {
	var pods *corev1.PodList
	Eventually(func() bool {
		var err error
		pods, err = s.Client.CoreV1().Pods(namespace).List(
			s.Context, metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil || len(pods.Items) == 0 {
			return false
		}

		// Check if all pods are ready
		for _, pod := range pods.Items {
			ready := false
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					ready = true
					break
				}
			}
			if !ready {
				return false
			}
		}
		return true
	}, timeout, 1*time.Second).Should(BeTrue(),
		fmt.Sprintf("Pods matching %s in namespace %s should be ready within %v", labelSelector, namespace, timeout))

	return pods
}

// WaitForDeploymentReady waits for a deployment to have all replicas ready.
//
// Example:
//
//	deployment := suite.WaitForDeploymentReady("default", "my-app", 60*time.Second)
func (s *IntegrationSuite) WaitForDeploymentReady(namespace, name string, timeout time.Duration) {
	Eventually(func() bool {
		deployment, err := s.Client.AppsV1().Deployments(namespace).Get(
			s.Context, name, metav1.GetOptions{})
		if err != nil {
			return false
		}

		return deployment.Status.ReadyReplicas == *deployment.Spec.Replicas &&
			deployment.Status.UpdatedReplicas == *deployment.Spec.Replicas &&
			deployment.Status.AvailableReplicas == *deployment.Spec.Replicas
	}, timeout, 2*time.Second).Should(BeTrue(),
		fmt.Sprintf("Deployment %s/%s should be ready within %v", namespace, name, timeout))
}

// WaitForNamespaceCreated waits for a namespace to be created.
//
// Example:
//
//	ns := suite.WaitForNamespaceCreated("my-namespace", 30*time.Second)
func (s *IntegrationSuite) WaitForNamespaceCreated(name string, timeout time.Duration) *corev1.Namespace {
	var ns *corev1.Namespace
	Eventually(func() error {
		var err error
		ns, err = s.Client.CoreV1().Namespaces().Get(
			s.Context, name, metav1.GetOptions{})
		return err
	}, timeout, 500*time.Millisecond).Should(Succeed(),
		fmt.Sprintf("Namespace %s should be created within %v", name, timeout))

	return ns
}

// WaitForNamespaceDeleted waits for a namespace to be deleted.
//
// Example:
//
//	suite.WaitForNamespaceDeleted("my-namespace", 60*time.Second)
func (s *IntegrationSuite) WaitForNamespaceDeleted(name string, timeout time.Duration) {
	Eventually(func() bool {
		_, err := s.Client.CoreV1().Namespaces().Get(
			s.Context, name, metav1.GetOptions{})
		return err != nil // Namespace should not exist
	}, timeout, 1*time.Second).Should(BeTrue(),
		fmt.Sprintf("Namespace %s should be deleted within %v", name, timeout))
}
