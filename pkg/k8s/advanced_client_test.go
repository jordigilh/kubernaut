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

func createTestAdvancedClient(objects ...runtime.Object) *advancedClient {
	basicClient := &basicClient{
		clientset: fake.NewSimpleClientset(objects...),
		namespace: "test-namespace",
		log: func() *logrus.Logger {
			logger := logrus.New()
			logger.SetLevel(logrus.FatalLevel)
			return logger
		}(),
	}
	
	return &advancedClient{
		basicClient: basicClient,
	}
}

func createTestDeploymentForAdvanced(namespace, name string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"deployment.kubernetes.io/revision": "2",
			},
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

func createTestPVCForAdvanced(namespace, name string, size string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					"storage": resource.MustParse(size),
				},
			},
		},
	}
}

func createTestNode(name string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1.NodeSpec{
			Unschedulable: false,
		},
	}
}

func createTestPodForAdvanced(namespace, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "main",
					Image: "nginx",
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
}

var _ = Describe("AdvancedClient", func() {
	var (
		client *advancedClient
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("RollbackDeployment", func() {
		Context("when deployment exists with valid revision", func() {
			BeforeEach(func() {
				deployment := createTestDeploymentForAdvanced("test-namespace", "test-deployment", 3)
				client = createTestAdvancedClient(deployment)
			})

			It("should add rollback annotation successfully", func() {
				err := client.RollbackDeployment(ctx, "test-namespace", "test-deployment")
				Expect(err).NotTo(HaveOccurred())

				// Verify rollback annotation was added
				updated, err := client.basicClient.GetDeployment(ctx, "test-namespace", "test-deployment")
				Expect(err).NotTo(HaveOccurred())
				Expect(updated.Annotations).To(HaveKey("prometheus-alerts-slm/rollback-requested"))
			})

			It("should use default namespace when empty", func() {
				err := client.RollbackDeployment(ctx, "", "test-deployment")
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when deployment has no previous revision", func() {
			BeforeEach(func() {
				deployment := createTestDeploymentForAdvanced("test-namespace", "test-deployment", 3)
				deployment.Annotations["deployment.kubernetes.io/revision"] = "1"
				client = createTestAdvancedClient(deployment)
			})

			It("should return an error", func() {
				err := client.RollbackDeployment(ctx, "test-namespace", "test-deployment")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("has no previous revision to rollback to"))
			})
		})

		Context("when deployment does not exist", func() {
			BeforeEach(func() {
				client = createTestAdvancedClient()
			})

			It("should return an error", func() {
				err := client.RollbackDeployment(ctx, "test-namespace", "nonexistent-deployment")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("ExpandPVC", func() {
		Context("when PVC exists", func() {
			BeforeEach(func() {
				pvc := createTestPVCForAdvanced("test-namespace", "test-pvc", "1Gi")
				client = createTestAdvancedClient(pvc)
			})

			It("should expand PVC size successfully", func() {
				err := client.ExpandPVC(ctx, "test-namespace", "test-pvc", "2Gi")
				Expect(err).NotTo(HaveOccurred())

				// Verify PVC size was updated
				pvc, err := client.basicClient.clientset.CoreV1().PersistentVolumeClaims("test-namespace").Get(ctx, "test-pvc", metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(pvc.Spec.Resources.Requests["storage"]).To(Equal(resource.MustParse("2Gi")))
			})

			It("should use default namespace when empty", func() {
				err := client.ExpandPVC(ctx, "", "test-pvc", "2Gi")
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when trying to shrink PVC", func() {
			BeforeEach(func() {
				pvc := createTestPVCForAdvanced("test-namespace", "test-pvc", "2Gi")
				client = createTestAdvancedClient(pvc)
			})

			It("should return an error", func() {
				err := client.ExpandPVC(ctx, "test-namespace", "test-pvc", "1Gi")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be larger than current size"))
			})
		})

		Context("when PVC does not exist", func() {
			BeforeEach(func() {
				client = createTestAdvancedClient()
			})

			It("should return an error", func() {
				err := client.ExpandPVC(ctx, "test-namespace", "nonexistent-pvc", "2Gi")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when size format is invalid", func() {
			BeforeEach(func() {
				pvc := createTestPVCForAdvanced("test-namespace", "test-pvc", "1Gi")
				client = createTestAdvancedClient(pvc)
			})

			It("should return an error", func() {
				err := client.ExpandPVC(ctx, "test-namespace", "test-pvc", "invalid-size")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid size format"))
			})
		})
	})

	Describe("DrainNode", func() {
		Context("when node exists", func() {
			BeforeEach(func() {
				node := createTestNode("test-node")
				client = createTestAdvancedClient(node)
			})

			It("should cordon and mark node for drain", func() {
				err := client.DrainNode(ctx, "test-node")
				Expect(err).NotTo(HaveOccurred())

				// Verify node was cordoned
				node, err := client.basicClient.clientset.CoreV1().Nodes().Get(ctx, "test-node", metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(node.Spec.Unschedulable).To(BeTrue())
				Expect(node.Labels).To(HaveKey("prometheus-alerts-slm/drain-requested"))
			})
		})

		Context("when node does not exist", func() {
			BeforeEach(func() {
				client = createTestAdvancedClient()
			})

			It("should return an error", func() {
				err := client.DrainNode(ctx, "nonexistent-node")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("QuarantinePod", func() {
		Context("when pod exists", func() {
			BeforeEach(func() {
				pod := createTestPodForAdvanced("test-namespace", "test-pod")
				client = createTestAdvancedClient(pod)
			})

			It("should add quarantine labels to pod", func() {
				err := client.QuarantinePod(ctx, "test-namespace", "test-pod")
				Expect(err).NotTo(HaveOccurred())

				// Verify quarantine labels were added
				pod, err := client.basicClient.GetPod(ctx, "test-namespace", "test-pod")
				Expect(err).NotTo(HaveOccurred())
				Expect(pod.Labels).To(HaveKey("prometheus-alerts-slm/quarantined"))
				Expect(pod.Labels["prometheus-alerts-slm/quarantined"]).To(Equal("true"))
				Expect(pod.Labels).To(HaveKey("prometheus-alerts-slm/quarantine-time"))
			})

			It("should use default namespace when empty", func() {
				err := client.QuarantinePod(ctx, "", "test-pod")
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when pod does not exist", func() {
			BeforeEach(func() {
				client = createTestAdvancedClient()
			})

			It("should return an error", func() {
				err := client.QuarantinePod(ctx, "test-namespace", "nonexistent-pod")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("CollectDiagnostics", func() {
		Context("when collecting diagnostics for existing resources", func() {
			BeforeEach(func() {
				pod := createTestPodForAdvanced("test-namespace", "test-resource")
				deployment := createTestDeploymentForAdvanced("test-namespace", "test-resource", 3)
				client = createTestAdvancedClient(pod, deployment)
			})

			It("should collect comprehensive diagnostics", func() {
				diagnostics, err := client.CollectDiagnostics(ctx, "test-namespace", "test-resource")
				Expect(err).NotTo(HaveOccurred())
				Expect(diagnostics).To(HaveKey("timestamp"))
				Expect(diagnostics).To(HaveKey("namespace"))
				Expect(diagnostics).To(HaveKey("resource"))
				Expect(diagnostics).To(HaveKey("pod_info"))
				Expect(diagnostics).To(HaveKey("deployment_info"))
				Expect(diagnostics).To(HaveKey("events"))
			})

			It("should use default namespace when empty", func() {
				diagnostics, err := client.CollectDiagnostics(ctx, "", "test-resource")
				Expect(err).NotTo(HaveOccurred())
				Expect(diagnostics).NotTo(BeEmpty())
			})
		})

		Context("when resource does not exist", func() {
			BeforeEach(func() {
				client = createTestAdvancedClient()
			})

			It("should still return basic diagnostics", func() {
				diagnostics, err := client.CollectDiagnostics(ctx, "test-namespace", "nonexistent-resource")
				Expect(err).NotTo(HaveOccurred())
				Expect(diagnostics).To(HaveKey("timestamp"))
				Expect(diagnostics).To(HaveKey("namespace"))
				Expect(diagnostics).To(HaveKey("resource"))
				Expect(diagnostics).To(HaveKey("events"))
			})
		})
	})
})