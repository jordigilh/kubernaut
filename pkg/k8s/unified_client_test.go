package k8s

import (
	"context"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// Helper function to create int32 pointer
func int32Ptr(i int32) *int32 {
	return &i
}

var _ = Describe("UnifiedClient", func() {
	var (
		fakeClientset *fake.Clientset
		logger        *logrus.Logger
		client        Client
		ctx           context.Context
	)

	BeforeEach(func() {
		fakeClientset = fake.NewSimpleClientset()
		logger = logrus.New()
		logger.SetLevel(logrus.FatalLevel) // Suppress logs during testing

		client = NewUnifiedClient(fakeClientset, config.KubernetesConfig{
			Namespace: "test-namespace",
		}, logger)

		ctx = context.Background()
	})

	It("should implement required interfaces", func() {
		var basicClient BasicClient = client
		var advancedClient AdvancedClient = client
		var fullClient Client = client
		
		Expect(basicClient).ToNot(BeNil())
		Expect(advancedClient).ToNot(BeNil())
		Expect(fullClient).ToNot(BeNil())
	})

	Describe("Health checks", func() {
		It("should report fake client as healthy", func() {
			Expect(client.IsHealthy()).To(BeTrue())
		})
	})

	Describe("Pod operations", func() {
		It("should get a pod successfully", func() {
			// Create a test pod
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-namespace",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "test-container", Image: "test-image"},
					},
				},
			}

			_, err := fakeClientset.CoreV1().Pods("test-namespace").Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Get the pod via unified client
			retrievedPod, err := client.GetPod(ctx, "test-namespace", "test-pod")
			Expect(err).ToNot(HaveOccurred())
			Expect(retrievedPod.Name).To(Equal("test-pod"))
			Expect(retrievedPod.Namespace).To(Equal("test-namespace"))
		})

		It("should quarantine a pod successfully", func() {
			// Create a test pod
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "quarantine-test-pod",
					Namespace: "test-namespace",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "test-container", Image: "test-image"},
					},
				},
			}

			_, err := fakeClientset.CoreV1().Pods("test-namespace").Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Quarantine the pod
			err = client.QuarantinePod(ctx, "test-namespace", "quarantine-test-pod")
			Expect(err).ToNot(HaveOccurred())

			// Verify quarantine labels were added
			quarantinedPod, err := client.GetPod(ctx, "test-namespace", "quarantine-test-pod")
			Expect(err).ToNot(HaveOccurred())
			Expect(quarantinedPod.Labels["prometheus-alerts-slm/quarantined"]).To(Equal("true"))
			Expect(quarantinedPod.Labels["prometheus-alerts-slm/quarantine-time"]).ToNot(BeEmpty())
		})
	})

	Describe("Deployment operations", func() {
		It("should scale a deployment successfully", func() {
			// Create a test deployment
			replicas := int32(1)
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-namespace",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "test-container", Image: "test-image"},
							},
						},
					},
				},
			}

			_, err := fakeClientset.AppsV1().Deployments("test-namespace").Create(ctx, deployment, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Scale the deployment
			err = client.ScaleDeployment(ctx, "test-namespace", "test-deployment", 3)
			Expect(err).ToNot(HaveOccurred())

			// Verify the scaling
			scaledDeployment, err := client.GetDeployment(ctx, "test-namespace", "test-deployment")
			Expect(err).ToNot(HaveOccurred())
			Expect(*scaledDeployment.Spec.Replicas).To(Equal(int32(3)))
		})
	})

	Describe("Diagnostics operations", func() {
		It("should collect diagnostics successfully", func() {
			diagnostics, err := client.CollectDiagnostics(ctx, "test-namespace", "test-resource")
			Expect(err).ToNot(HaveOccurred())
			
			Expect(diagnostics["namespace"]).To(Equal("test-namespace"))
			Expect(diagnostics["resource"]).To(Equal("test-resource"))
			Expect(diagnostics["timestamp"]).ToNot(BeEmpty())
		})
	})

	Describe("Advanced client actions", func() {
		BeforeEach(func() {
			// Create all required resources before each test
			
			// Create test pod
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-namespace",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "test-container", Image: "test-image"},
					},
				},
			}
			_, err := fakeClientset.CoreV1().Pods("test-namespace").Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Create test node
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Spec: corev1.NodeSpec{},
			}
			_, err = fakeClientset.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Create test HPA
			hpa := &autoscalingv1.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "test-namespace",
				},
				Spec: autoscalingv1.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "test-deployment",
						APIVersion: "apps/v1",
					},
					MinReplicas:                    int32Ptr(1),
					MaxReplicas:                    5,
					TargetCPUUtilizationPercentage: int32Ptr(80),
				},
			}
			_, err = fakeClientset.AutoscalingV1().HorizontalPodAutoscalers("test-namespace").Create(ctx, hpa, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Create test DaemonSet
			daemonSet := &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-daemonset",
					Namespace: "test-namespace",
				},
				Spec: appsv1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test-daemon"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test-daemon"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "daemon-container", Image: "daemon-image"},
							},
						},
					},
				},
			}
			_, err = fakeClientset.AppsV1().DaemonSets("test-namespace").Create(ctx, daemonSet, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Create test Secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
				},
				Data: map[string][]byte{
					"key": []byte("value"),
				},
			}
			_, err = fakeClientset.CoreV1().Secrets("test-namespace").Create(ctx, secret, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Create test StatefulSet
			statefulSet := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-statefulset",
					Namespace: "test-namespace",
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test-stateful"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test-stateful"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "stateful-container", Image: "stateful-image"},
							},
						},
					},
					ServiceName: "test-stateful-service",
				},
			}
			_, err = fakeClientset.AppsV1().StatefulSets("test-namespace").Create(ctx, statefulSet, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Create test PVC
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-namespace",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			}
			_, err = fakeClientset.CoreV1().PersistentVolumeClaims("test-namespace").Create(ctx, pvc, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Create test Deployment (needed for some operations)
			replicas := int32(1)
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-namespace",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "test-container", Image: "test-image"},
							},
						},
					},
				},
			}
			_, err = fakeClientset.AppsV1().Deployments("test-namespace").Create(ctx, deployment, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
		})

		DescribeTable("should execute all unified client actions successfully",
			func(actionName string, action func() error) {
				err := action()
				Expect(err).ToNot(HaveOccurred(), "Action %s should not error with fake client", actionName)
			},
			Entry("CleanupStorage", "CleanupStorage", func() error {
				return client.CleanupStorage(ctx, "test-namespace", "test-pod", "/var/log")
			}),
			Entry("BackupData", "BackupData", func() error {
				return client.BackupData(ctx, "test-namespace", "test-pod", "backup-20231201")
			}),
			Entry("CompactStorage", "CompactStorage", func() error {
				return client.CompactStorage(ctx, "test-namespace", "test-pod")
			}),
			Entry("CordonNode", "CordonNode", func() error {
				return client.CordonNode(ctx, "test-node")
			}),
			Entry("UpdateHPA", "UpdateHPA", func() error {
				return client.UpdateHPA(ctx, "test-namespace", "test-hpa", 1, 10)
			}),
			Entry("RestartDaemonSet", "RestartDaemonSet", func() error {
				return client.RestartDaemonSet(ctx, "test-namespace", "test-daemonset")
			}),
			Entry("RotateSecrets", "RotateSecrets", func() error {
				return client.RotateSecrets(ctx, "test-namespace", "test-secret")
			}),
			Entry("AuditLogs", "AuditLogs", func() error {
				return client.AuditLogs(ctx, "test-namespace", "test-pod", "security")
			}),
			Entry("UpdateNetworkPolicy", "UpdateNetworkPolicy", func() error {
				return client.UpdateNetworkPolicy(ctx, "test-namespace", "test-policy", "allow")
			}),
			Entry("RestartNetwork", "RestartNetwork", func() error {
				return client.RestartNetwork(ctx, "coredns")
			}),
			Entry("ResetServiceMesh", "ResetServiceMesh", func() error {
				return client.ResetServiceMesh(ctx, "istio")
			}),
			Entry("FailoverDatabase", "FailoverDatabase", func() error {
				return client.FailoverDatabase(ctx, "test-namespace", "primary-db", "replica-db")
			}),
			Entry("RepairDatabase", "RepairDatabase", func() error {
				return client.RepairDatabase(ctx, "test-namespace", "test-db", "fsck")
			}),
			Entry("ScaleStatefulSet", "ScaleStatefulSet", func() error {
				return client.ScaleStatefulSet(ctx, "test-namespace", "test-statefulset", 3)
			}),
			Entry("EnableDebugMode", "EnableDebugMode", func() error {
				return client.EnableDebugMode(ctx, "test-namespace", "test-resource", "debug", "1h")
			}),
			Entry("CreateHeapDump", "CreateHeapDump", func() error {
				return client.CreateHeapDump(ctx, "test-namespace", "test-pod", "/tmp/heap.dump")
			}),
			Entry("OptimizeResources", "OptimizeResources", func() error {
				return client.OptimizeResources(ctx, "test-namespace", "test-resource", "cpu")
			}),
			Entry("MigrateWorkload", "MigrateWorkload", func() error {
				return client.MigrateWorkload(ctx, "test-namespace", "test-workload", "target-node")
			}),
		)
	})
})

var _ = Describe("NewClient", func() {
	var logger *logrus.Logger

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.FatalLevel)
	})

	Context("when creating a unified client", func() {
		It("should create a client that implements all required interfaces", func() {
			// This test requires a valid kubeconfig or in-cluster config
			// In CI/testing environments, this might fail - that's expected
			cfg := config.KubernetesConfig{
				Namespace: "default",
			}

			client, err := NewClient(cfg, logger)
			if err != nil {
				Skip("Kubernetes config not available in test environment: " + err.Error())
			}

			var basicClient BasicClient = client
			var advancedClient AdvancedClient = client
			var fullClient Client = client
			
			Expect(basicClient).ToNot(BeNil())
			Expect(advancedClient).ToNot(BeNil())
			Expect(fullClient).ToNot(BeNil())
		})
	})
})