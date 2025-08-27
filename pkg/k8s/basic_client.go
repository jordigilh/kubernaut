package k8s

import (
	"context"
	"fmt"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/pkg/metrics"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// BasicClient provides core Kubernetes operations
type BasicClient interface {
	// Core pod operations
	GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error)
	DeletePod(ctx context.Context, namespace, name string) error
	ListPodsWithLabel(ctx context.Context, namespace, labelSelector string) (*corev1.PodList, error)

	// Core deployment operations
	GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error)
	ScaleDeployment(ctx context.Context, namespace, name string, replicas int32) error

	// Resource management
	UpdatePodResources(ctx context.Context, namespace, name string, resources corev1.ResourceRequirements) error

	// Health check
	IsHealthy() bool
}

// basicClient implements core Kubernetes operations
type basicClient struct {
	clientset kubernetes.Interface
	namespace string
	log       *logrus.Logger
}

func (c *basicClient) GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	if namespace == "" {
		namespace = c.namespace
	}

	metrics.RecordK8sAPICall("get")
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

func (c *basicClient) DeletePod(ctx context.Context, namespace, name string) error {
	if namespace == "" {
		namespace = c.namespace
	}

	c.log.WithFields(logrus.Fields{
		"namespace": namespace,
		"pod":       name,
	}).Info("Deleting pod")

	metrics.RecordK8sAPICall("delete")
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

func (c *basicClient) ListPodsWithLabel(ctx context.Context, namespace, labelSelector string) (*corev1.PodList, error) {
	if namespace == "" {
		namespace = c.namespace
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

func (c *basicClient) GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	if namespace == "" {
		namespace = c.namespace
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

func (c *basicClient) ScaleDeployment(ctx context.Context, namespace, name string, replicas int32) error {
	if namespace == "" {
		namespace = c.namespace
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

	metrics.RecordK8sAPICall("update")
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

func (c *basicClient) UpdatePodResources(ctx context.Context, namespace, name string, resources corev1.ResourceRequirements) error {
	if namespace == "" {
		namespace = c.namespace
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

func (c *basicClient) updateDeploymentResources(ctx context.Context, namespace, deploymentName string, resources corev1.ResourceRequirements) error {
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

func (c *basicClient) IsHealthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := c.clientset.CoreV1().Namespaces().Get(ctx, c.namespace, metav1.GetOptions{})
	return err == nil
}
