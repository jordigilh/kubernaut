package mcp

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type Client interface {
	GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error)
	ScaleDeployment(ctx context.Context, namespace, name string, replicas int32) error
	GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error)
	DeletePod(ctx context.Context, namespace, name string) error
	UpdatePodResources(ctx context.Context, namespace, name string, resources corev1.ResourceRequirements) error
	ListPodsWithLabel(ctx context.Context, namespace, labelSelector string) (*corev1.PodList, error)
	IsHealthy() bool
}

type client struct {
	clientset *kubernetes.Clientset
	config    config.OpenShiftConfig
	log       *logrus.Logger
}

func NewClient(cfg config.OpenShiftConfig, log *logrus.Logger) (Client, error) {
	var k8sConfig *rest.Config
	var err error

	// Try to load config from the specified context or default locations
	if cfg.Context != "" {
		// Load from kubeconfig with specific context
		k8sConfig, err = loadConfigWithContext(cfg.Context)
	} else {
		// Try in-cluster config first, then kubeconfig
		k8sConfig, err = rest.InClusterConfig()
		if err != nil {
			k8sConfig, err = loadKubeConfig()
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load Kubernetes config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	c := &client{
		clientset: clientset,
		config:    cfg,
		log:       log,
	}

	// Test connection
	if err := c.testConnection(); err != nil {
		return nil, fmt.Errorf("failed to connect to Kubernetes cluster: %w", err)
	}

	log.WithFields(logrus.Fields{
		"context":   cfg.Context,
		"namespace": cfg.Namespace,
	}).Info("MCP client initialized successfully")

	return c, nil
}

func loadConfigWithContext(context string) (*rest.Config, error) {
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	config, err := clientcmd.LoadFromFile(kubeconfig)
	if err != nil {
		return nil, err
	}

	return clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{
		CurrentContext: context,
	}).ClientConfig()
}

func loadKubeConfig() (*rest.Config, error) {
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

func (c *client) testConnection() error {
	ctx := context.Background()
	_, err := c.clientset.CoreV1().Namespaces().Get(ctx, c.config.Namespace, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to access namespace %s: %w", c.config.Namespace, err)
	}
	return nil
}

func (c *client) GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
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

func (c *client) ScaleDeployment(ctx context.Context, namespace, name string, replicas int32) error {
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

func (c *client) GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
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

func (c *client) DeletePod(ctx context.Context, namespace, name string) error {
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

func (c *client) UpdatePodResources(ctx context.Context, namespace, name string, resources corev1.ResourceRequirements) error {
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

func (c *client) updateDeploymentResources(ctx context.Context, namespace, deploymentName string, resources corev1.ResourceRequirements) error {
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

func (c *client) ListPodsWithLabel(ctx context.Context, namespace, labelSelector string) (*corev1.PodList, error) {
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

func (c *client) IsHealthy() bool {
	ctx := context.Background()
	_, err := c.clientset.CoreV1().Namespaces().Get(ctx, c.config.Namespace, metav1.GetOptions{})
	return err == nil
}

// ResourceRequirements is a helper type for resource specifications
type ResourceRequirements struct {
	CPURequest    string
	MemoryRequest string
	CPULimit      string
	MemoryLimit   string
}

// ToK8sResourceRequirements converts ResourceRequirements to Kubernetes ResourceRequirements
func (r ResourceRequirements) ToK8sResourceRequirements() corev1.ResourceRequirements {
	req := corev1.ResourceRequirements{
		Requests: make(corev1.ResourceList),
		Limits:   make(corev1.ResourceList),
	}

	if r.CPURequest != "" {
		req.Requests[corev1.ResourceCPU] = resource.MustParse(r.CPURequest)
	}
	if r.MemoryRequest != "" {
		req.Requests[corev1.ResourceMemory] = resource.MustParse(r.MemoryRequest)
	}
	if r.CPULimit != "" {
		req.Limits[corev1.ResourceCPU] = resource.MustParse(r.CPULimit)
	}
	if r.MemoryLimit != "" {
		req.Limits[corev1.ResourceMemory] = resource.MustParse(r.MemoryLimit)
	}

	return req
}