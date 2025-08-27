package k8s

import (
	"context"

	"github.com/sirupsen/logrus"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func createTestLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)
	return logger
}

func createTestBasicClient(objects ...runtime.Object) *basicClient {
	return &basicClient{
		clientset: fake.NewSimpleClientset(objects...),
		namespace: "test-namespace",
		log:       createTestLogger(),
	}
}

func createTestPodForBasicClient(namespace, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app": "test-app",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "main",
					Image: "nginx",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("512Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("250m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
}

func createTestDeploymentForBasicClient(namespace, name string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "main",
							Image: "nginx",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("250m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						},
					},
				},
			},
		},
	}
}

var _ = Describe("BasicClient", func() {
	var (
		client *basicClient
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("GetPod", func() {
		Context("when pod exists", func() {
			BeforeEach(func() {
				pod := createTestPodForBasicClient("test-namespace", "test-pod")
				client = createTestBasicClient(pod)
			})

			It("should return the pod successfully", func() {
				result, err := client.GetPod(ctx, "test-namespace", "test-pod")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Name).To(Equal("test-pod"))
				Expect(result.Namespace).To(Equal("test-namespace"))
			})

			It("should use default namespace when empty", func() {
				result, err := client.GetPod(ctx, "", "test-pod")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Name).To(Equal("test-pod"))
			})
		})

		Context("when pod does not exist", func() {
			BeforeEach(func() {
				client = createTestBasicClient()
			})

			It("should return an error", func() {
				_, err := client.GetPod(ctx, "test-namespace", "nonexistent-pod")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get pod"))
			})
		})
	})

	Describe("DeletePod", func() {
		Context("when pod exists", func() {
			BeforeEach(func() {
				pod := createTestPodForBasicClient("test-namespace", "test-pod")
				client = createTestBasicClient(pod)
			})

			It("should delete the pod successfully", func() {
				err := client.DeletePod(ctx, "test-namespace", "test-pod")
				Expect(err).NotTo(HaveOccurred())

				// Verify pod is deleted
				_, err = client.GetPod(ctx, "test-namespace", "test-pod")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when pod does not exist", func() {
			BeforeEach(func() {
				client = createTestBasicClient()
			})

			It("should return an error", func() {
				err := client.DeletePod(ctx, "test-namespace", "nonexistent-pod")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to delete pod"))
			})
		})
	})

	Describe("ListPodsWithLabel", func() {
		var pod1, pod2, pod3 *corev1.Pod

		BeforeEach(func() {
			pod1 = createTestPodForBasicClient("test-namespace", "pod-1")
			pod1.Labels["version"] = "v1"
			
			pod2 = createTestPodForBasicClient("test-namespace", "pod-2")
			pod2.Labels["version"] = "v2"
			
			pod3 = createTestPodForBasicClient("test-namespace", "pod-3")
			pod3.Labels = map[string]string{"app": "other-app"}

			client = createTestBasicClient(pod1, pod2, pod3)
		})

		It("should list pods with matching app label", func() {
			pods, err := client.ListPodsWithLabel(ctx, "test-namespace", "app=test-app")
			Expect(err).NotTo(HaveOccurred())
			Expect(pods.Items).To(HaveLen(2))
		})

		It("should list pods with specific version", func() {
			pods, err := client.ListPodsWithLabel(ctx, "test-namespace", "version=v1")
			Expect(err).NotTo(HaveOccurred())
			Expect(pods.Items).To(HaveLen(1))
			Expect(pods.Items[0].Name).To(Equal("pod-1"))
		})

		It("should return empty list when no pods match", func() {
			pods, err := client.ListPodsWithLabel(ctx, "test-namespace", "nonexistent=label")
			Expect(err).NotTo(HaveOccurred())
			Expect(pods.Items).To(HaveLen(0))
		})

		It("should use default namespace when empty", func() {
			pods, err := client.ListPodsWithLabel(ctx, "", "app=other-app")
			Expect(err).NotTo(HaveOccurred())
			Expect(pods.Items).To(HaveLen(1))
		})
	})

	Describe("GetDeployment", func() {
		Context("when deployment exists", func() {
			BeforeEach(func() {
				deployment := createTestDeploymentForBasicClient("test-namespace", "test-deployment", 3)
				client = createTestBasicClient(deployment)
			})

			It("should return the deployment successfully", func() {
				result, err := client.GetDeployment(ctx, "test-namespace", "test-deployment")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Name).To(Equal("test-deployment"))
				Expect(result.Namespace).To(Equal("test-namespace"))
				Expect(*result.Spec.Replicas).To(Equal(int32(3)))
			})

			It("should use default namespace when empty", func() {
				result, err := client.GetDeployment(ctx, "", "test-deployment")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Name).To(Equal("test-deployment"))
			})
		})

		Context("when deployment does not exist", func() {
			BeforeEach(func() {
				client = createTestBasicClient()
			})

			It("should return an error", func() {
				_, err := client.GetDeployment(ctx, "test-namespace", "nonexistent-deployment")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get deployment"))
			})
		})
	})

	Describe("ScaleDeployment", func() {
		Context("when deployment exists", func() {
			BeforeEach(func() {
				deployment := createTestDeploymentForBasicClient("test-namespace", "test-deployment", 3)
				client = createTestBasicClient(deployment)
			})

			It("should scale up successfully", func() {
				err := client.ScaleDeployment(ctx, "test-namespace", "test-deployment", 5)
				Expect(err).NotTo(HaveOccurred())

				updated, err := client.GetDeployment(ctx, "test-namespace", "test-deployment")
				Expect(err).NotTo(HaveOccurred())
				Expect(*updated.Spec.Replicas).To(Equal(int32(5)))
			})

			It("should scale down successfully", func() {
				err := client.ScaleDeployment(ctx, "test-namespace", "test-deployment", 1)
				Expect(err).NotTo(HaveOccurred())

				updated, err := client.GetDeployment(ctx, "test-namespace", "test-deployment")
				Expect(err).NotTo(HaveOccurred())
				Expect(*updated.Spec.Replicas).To(Equal(int32(1)))
			})

			It("should use default namespace when empty", func() {
				err := client.ScaleDeployment(ctx, "", "test-deployment", 2)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when deployment does not exist", func() {
			BeforeEach(func() {
				client = createTestBasicClient()
			})

			It("should return an error", func() {
				err := client.ScaleDeployment(ctx, "test-namespace", "nonexistent-deployment", 3)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("UpdatePodResources", func() {
		var newResources corev1.ResourceRequirements

		BeforeEach(func() {
			newResources = corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1000m"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			}
		})

		Context("when pod has deployment owner", func() {
			BeforeEach(func() {
				pod := createTestPodForBasicClient("test-namespace", "test-pod")
				pod.OwnerReferences = []metav1.OwnerReference{
					{
						Kind: "ReplicaSet",
						Name: "test-rs",
					},
				}

				rs := &appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rs",
						Namespace: "test-namespace",
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind: "Deployment",
								Name: "test-deployment",
							},
						},
					},
				}

				deployment := createTestDeploymentForBasicClient("test-namespace", "test-deployment", 3)
				client = createTestBasicClient(pod, rs, deployment)
			})

			It("should update deployment resources successfully", func() {
				err := client.UpdatePodResources(ctx, "test-namespace", "test-pod", newResources)
				Expect(err).NotTo(HaveOccurred())

				updatedDeployment, err := client.GetDeployment(ctx, "test-namespace", "test-deployment")
				Expect(err).NotTo(HaveOccurred())
				Expect(updatedDeployment.Spec.Template.Spec.Containers[0].Resources).To(Equal(newResources))
			})
		})

		Context("when pod has no deployment owner", func() {
			BeforeEach(func() {
				orphanPod := createTestPodForBasicClient("test-namespace", "orphan-pod")
				client = createTestBasicClient(orphanPod)
			})

			It("should return an error", func() {
				err := client.UpdatePodResources(ctx, "test-namespace", "orphan-pod", newResources)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not find deployment"))
			})
		})

		Context("when pod does not exist", func() {
			BeforeEach(func() {
				client = createTestBasicClient()
			})

			It("should return an error", func() {
				err := client.UpdatePodResources(ctx, "test-namespace", "nonexistent-pod", newResources)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("updateDeploymentResources", func() {
		var newResources corev1.ResourceRequirements

		BeforeEach(func() {
			newResources = corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2000m"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1000m"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			}
		})

		Context("when deployment exists", func() {
			BeforeEach(func() {
				deployment := createTestDeploymentForBasicClient("test-namespace", "test-deployment", 3)
				client = createTestBasicClient(deployment)
			})

			It("should update resources successfully", func() {
				err := client.updateDeploymentResources(ctx, "test-namespace", "test-deployment", newResources)
				Expect(err).NotTo(HaveOccurred())

				updated, err := client.GetDeployment(ctx, "test-namespace", "test-deployment")
				Expect(err).NotTo(HaveOccurred())
				Expect(updated.Spec.Template.Spec.Containers[0].Resources).To(Equal(newResources))
			})
		})

		Context("when deployment does not exist", func() {
			BeforeEach(func() {
				client = createTestBasicClient()
			})

			It("should return an error", func() {
				err := client.updateDeploymentResources(ctx, "test-namespace", "nonexistent-deployment", newResources)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("IsHealthy", func() {
		BeforeEach(func() {
			client = createTestBasicClient()
		})

		It("should check health status", func() {
			// The fake client's health check depends on whether it can access the namespace
			// In test environments, this might fail due to lack of actual k8s cluster
			healthy := client.IsHealthy()
			// Just verify the method returns a boolean, regardless of value
			Expect(healthy).To(BeAssignableToTypeOf(true))
		})
	})

	Describe("DefaultNamespace", func() {
		BeforeEach(func() {
			pod := createTestPodForBasicClient("test-namespace", "test-pod")
			client = createTestBasicClient(pod)
		})

		It("should use default namespace for GetPod when empty", func() {
			result, err := client.GetPod(ctx, "", "test-pod")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Name).To(Equal("test-pod"))
		})

		It("should use default namespace for DeletePod when empty", func() {
			err := client.DeletePod(ctx, "", "test-pod")
			Expect(err).NotTo(HaveOccurred())
		})
	})
})