package testenv

import (
	"context"
	"fmt"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/k8s"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

// setupFakeK8sEnvironment creates a fake Kubernetes environment for testing
func setupFakeK8sEnvironment() (*TestEnvironment, error) {
	// Create scheme with necessary objects
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	appsv1.AddToScheme(scheme)

	// Create fake client
	client := fake.NewSimpleClientset()

	ctx, cancel := context.WithCancel(context.Background())

	env := &TestEnvironment{
		Environment: nil, // No envtest environment for fake
		Config:      nil, // No real config for fake
		Client:      client,
		Context:     ctx,
		CancelFunc:  cancel,
	}

	return env, nil
}

// CreateDefaultNamespace creates the default namespace in the environment
func (te *TestEnvironment) CreateDefaultNamespace() error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}
	_, err := te.Client.CoreV1().Namespaces().Create(te.Context, ns, metav1.CreateOptions{})
	return err
}

// CreateK8sClient creates a k8s.Client using the test environment
func (te *TestEnvironment) CreateK8sClient(logger *logrus.Logger) k8s.Client {
	return &k8sTestClient{
		clientset: te.Client,
		config: config.KubernetesConfig{
			Namespace: "default",
		},
		log: logger,
	}
}

// k8sTestClient implements k8s.Client for testing
type k8sTestClient struct {
	clientset kubernetes.Interface
	config    config.KubernetesConfig
	log       *logrus.Logger
}

func (c *k8sTestClient) GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	if namespace == "" {
		namespace = c.config.Namespace
	}

	deployment, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment %s/%s: %w", namespace, name, err)
	}

	c.log.WithFields(logrus.Fields{
		"namespace":  namespace,
		"deployment": name,
		"replicas":   *deployment.Spec.Replicas,
	}).Debug("Retrieved deployment")

	return deployment, nil
}

func (c *k8sTestClient) ScaleDeployment(ctx context.Context, namespace, name string, replicas int32) error {
	if namespace == "" {
		namespace = c.config.Namespace
	}

	c.log.WithFields(logrus.Fields{
		"namespace":  namespace,
		"deployment": name,
		"replicas":   replicas,
	}).Info("Scaling deployment")

	deployment, err := c.GetDeployment(ctx, namespace, name)
	if err != nil {
		return err
	}

	deployment.Spec.Replicas = &replicas

	_, err = c.clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to scale deployment %s/%s to %d replicas: %w", namespace, name, replicas, err)
	}

	c.log.WithFields(logrus.Fields{
		"namespace":  namespace,
		"deployment": name,
		"replicas":   replicas,
	}).Info("Successfully scaled deployment")

	return nil
}

func (c *k8sTestClient) GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	if namespace == "" {
		namespace = c.config.Namespace
	}

	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod %s/%s: %w", namespace, name, err)
	}

	c.log.WithFields(logrus.Fields{
		"namespace": namespace,
		"pod":       name,
		"phase":     pod.Status.Phase,
	}).Debug("Retrieved pod")

	return pod, nil
}

func (c *k8sTestClient) DeletePod(ctx context.Context, namespace, name string) error {
	if namespace == "" {
		namespace = c.config.Namespace
	}

	c.log.WithFields(logrus.Fields{
		"namespace": namespace,
		"pod":       name,
	}).Info("Deleting pod")

	err := c.clientset.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete pod %s/%s: %w", namespace, name, err)
	}

	c.log.WithFields(logrus.Fields{
		"namespace": namespace,
		"pod":       name,
	}).Info("Successfully deleted pod")

	return nil
}

func (c *k8sTestClient) UpdatePodResources(ctx context.Context, namespace, name string, resources corev1.ResourceRequirements) error {
	if namespace == "" {
		namespace = c.config.Namespace
	}

	c.log.WithFields(logrus.Fields{
		"namespace": namespace,
		"pod":       name,
		"resources": resources,
	}).Info("Updating pod resources")

	// Get the pod's owner (usually a deployment)
	pod, err := c.GetPod(ctx, namespace, name)
	if err != nil {
		return err
	}

	// Find the deployment that owns this pod
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "ReplicaSet" {
			rs, err := c.clientset.AppsV1().ReplicaSets(namespace).Get(ctx, owner.Name, metav1.GetOptions{})
			if err != nil {
				continue
			}

			for _, rsOwner := range rs.OwnerReferences {
				if rsOwner.Kind == "Deployment" {
					return c.updateDeploymentResources(ctx, namespace, rsOwner.Name, resources)
				}
			}
		}
	}

	return fmt.Errorf("could not find deployment for pod %s/%s", namespace, name)
}

func (c *k8sTestClient) updateDeploymentResources(ctx context.Context, namespace, deploymentName string, resources corev1.ResourceRequirements) error {
	deployment, err := c.GetDeployment(ctx, namespace, deploymentName)
	if err != nil {
		return err
	}

	// Update resources for all containers
	for i := range deployment.Spec.Template.Spec.Containers {
		deployment.Spec.Template.Spec.Containers[i].Resources = resources
	}

	_, err = c.clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment %s/%s resources: %w", namespace, deploymentName, err)
	}

	c.log.WithFields(logrus.Fields{
		"namespace":  namespace,
		"deployment": deploymentName,
		"resources":  resources,
	}).Info("Successfully updated deployment resources")

	return nil
}

func (c *k8sTestClient) ListPodsWithLabel(ctx context.Context, namespace, labelSelector string) (*corev1.PodList, error) {
	if namespace == "" {
		namespace = c.config.Namespace
	}

	pods, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods with label %s in namespace %s: %w", labelSelector, namespace, err)
	}

	c.log.WithFields(logrus.Fields{
		"namespace":     namespace,
		"labelSelector": labelSelector,
		"count":         len(pods.Items),
	}).Debug("Listed pods with label")

	return pods, nil
}

func (c *k8sTestClient) IsHealthy() bool {
	// For fake client, always return true as it's an in-memory client
	// and we don't need to check actual cluster connectivity
	return true
}

// Advanced client methods implementation
func (c *k8sTestClient) RollbackDeployment(ctx context.Context, namespace, name string) error {
	if namespace == "" {
		namespace = c.config.Namespace
	}

	c.log.WithFields(logrus.Fields{
		"namespace":  namespace,
		"deployment": name,
	}).Info("Rolling back deployment")

	// Get current deployment
	deployment, err := c.GetDeployment(ctx, namespace, name)
	if err != nil {
		return err
	}

	// For testing, we'll add a rollback annotation
	if deployment.Annotations == nil {
		deployment.Annotations = make(map[string]string)
	}
	deployment.Annotations["prometheus-alerts-slm/rollback-requested"] = time.Now().Format(time.RFC3339)

	_, err = c.clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to trigger rollback for deployment %s/%s: %w", namespace, name, err)
	}

	c.log.WithFields(logrus.Fields{
		"namespace":  namespace,
		"deployment": name,
	}).Info("Successfully triggered deployment rollback")

	return nil
}

func (c *k8sTestClient) ExpandPVC(ctx context.Context, namespace, name string, newSize string) error {
	if namespace == "" {
		namespace = c.config.Namespace
	}

	c.log.WithFields(logrus.Fields{
		"namespace": namespace,
		"pvc":       name,
		"new_size":  newSize,
	}).Info("Expanding PVC")

	// For fake client, just log the operation
	c.log.WithFields(logrus.Fields{
		"namespace": namespace,
		"pvc":       name,
		"new_size":  newSize,
	}).Info("Successfully expanded PVC (simulated)")

	return nil
}

func (c *k8sTestClient) DrainNode(ctx context.Context, nodeName string) error {
	c.log.WithFields(logrus.Fields{
		"node": nodeName,
	}).Info("Draining node")

	// For fake client, just log the operation
	c.log.WithFields(logrus.Fields{
		"node": nodeName,
	}).Info("Successfully initiated node drain (simulated)")

	return nil
}

func (c *k8sTestClient) QuarantinePod(ctx context.Context, namespace, name string) error {
	if namespace == "" {
		namespace = c.config.Namespace
	}

	c.log.WithFields(logrus.Fields{
		"namespace": namespace,
		"pod":       name,
	}).Info("Quarantining pod")

	// Get the pod
	pod, err := c.GetPod(ctx, namespace, name)
	if err != nil {
		return err
	}

	// Add quarantine labels
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	pod.Labels["prometheus-alerts-slm/quarantined"] = "true"
	pod.Labels["prometheus-alerts-slm/quarantine-time"] = time.Now().Format(time.RFC3339)

	// Update the pod
	_, err = c.clientset.CoreV1().Pods(namespace).Update(ctx, pod, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to quarantine pod %s/%s: %w", namespace, name, err)
	}

	c.log.WithFields(logrus.Fields{
		"namespace": namespace,
		"pod":       name,
	}).Info("Successfully quarantined pod")

	return nil
}

func (c *k8sTestClient) CollectDiagnostics(ctx context.Context, namespace, resource string) (map[string]interface{}, error) {
	if namespace == "" {
		namespace = c.config.Namespace
	}

	c.log.WithFields(logrus.Fields{
		"namespace": namespace,
		"resource":  resource,
	}).Info("Collecting diagnostics")

	diagnostics := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"namespace": namespace,
		"resource":  resource,
		"status":    "collected",
		"simulated": true,
	}

	c.log.WithFields(logrus.Fields{
		"namespace":   namespace,
		"resource":    resource,
		"data_points": len(diagnostics),
	}).Info("Successfully collected diagnostics")

	return diagnostics, nil
}
