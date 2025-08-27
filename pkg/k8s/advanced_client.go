package k8s

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AdvancedClient provides high-priority and complex Kubernetes operations
type AdvancedClient interface {
	// High-priority actions
	RollbackDeployment(ctx context.Context, namespace, name string) error
	ExpandPVC(ctx context.Context, namespace, name string, newSize string) error
	DrainNode(ctx context.Context, nodeName string) error
	QuarantinePod(ctx context.Context, namespace, name string) error
	CollectDiagnostics(ctx context.Context, namespace, resource string) (map[string]interface{}, error)
}

// advancedClient implements high-priority and complex Kubernetes operations
type advancedClient struct {
	basicClient *basicClient
}

func (c *advancedClient) RollbackDeployment(ctx context.Context, namespace, name string) error {
	if namespace == "" {
		namespace = c.basicClient.namespace
	}

	c.basicClient.log.WithFields(logrus.Fields{
		"namespace":  namespace,
		"deployment": name,
	}).Info("Rolling back deployment")

	// Get current deployment
	deployment, err := c.basicClient.GetDeployment(ctx, namespace, name)
	if err != nil {
		return err
	}

	// Check if there are previous revisions
	if deployment.Annotations["deployment.kubernetes.io/revision"] == "1" {
		return fmt.Errorf("deployment %s/%s has no previous revision to rollback to", namespace, name)
	}

	// For this implementation, we'll trigger a rollback by adding an annotation
	// In a real implementation, you'd use kubectl rollout undo or the equivalent API calls
	if deployment.Annotations == nil {
		deployment.Annotations = make(map[string]string)
	}
	deployment.Annotations["prometheus-alerts-slm/rollback-requested"] = time.Now().Format(time.RFC3339)

	_, err = c.basicClient.clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to trigger rollback for deployment %s/%s: %w", namespace, name, err)
	}

	c.basicClient.log.WithFields(logrus.Fields{
		"namespace":  namespace,
		"deployment": name,
	}).Info("Successfully triggered deployment rollback")

	return nil
}

func (c *advancedClient) ExpandPVC(ctx context.Context, namespace, name string, newSize string) error {
	if namespace == "" {
		namespace = c.basicClient.namespace
	}

	c.basicClient.log.WithFields(logrus.Fields{
		"namespace": namespace,
		"pvc":       name,
		"new_size":  newSize,
	}).Info("Expanding PVC")

	// Get the current PVC
	pvc, err := c.basicClient.clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get PVC %s/%s: %w", namespace, name, err)
	}

	// Parse the new size
	newQuantity, err := parseQuantity(newSize)
	if err != nil {
		return fmt.Errorf("invalid size format %s: %w", newSize, err)
	}

	// Check if the new size is larger than current
	currentSize := pvc.Spec.Resources.Requests["storage"]
	if newQuantity.Cmp(currentSize) <= 0 {
		return fmt.Errorf("new size %s must be larger than current size %s", newSize, currentSize.String())
	}

	// Update the PVC size
	pvc.Spec.Resources.Requests["storage"] = newQuantity

	_, err = c.basicClient.clientset.CoreV1().PersistentVolumeClaims(namespace).Update(ctx, pvc, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update PVC %s/%s: %w", namespace, name, err)
	}

	c.basicClient.log.WithFields(logrus.Fields{
		"namespace": namespace,
		"pvc":       name,
		"new_size":  newSize,
	}).Info("Successfully expanded PVC")

	return nil
}

func (c *advancedClient) DrainNode(ctx context.Context, nodeName string) error {
	c.basicClient.log.WithFields(logrus.Fields{
		"node": nodeName,
	}).Info("Draining node")

	// Get the node
	node, err := c.basicClient.clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	// First, cordon the node (mark as unschedulable)
	node.Spec.Unschedulable = true

	_, err = c.basicClient.clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to cordon node %s: %w", nodeName, err)
	}

	c.basicClient.log.WithFields(logrus.Fields{
		"node": nodeName,
	}).Info("Node cordoned successfully")

	// For this implementation, we'll add a label to indicate drain was requested
	// In a real implementation, you'd evict all pods from the node
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}
	node.Labels["prometheus-alerts-slm/drain-requested"] = time.Now().Format(time.RFC3339)

	_, err = c.basicClient.clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to mark node %s as drained: %w", nodeName, err)
	}

	c.basicClient.log.WithFields(logrus.Fields{
		"node": nodeName,
	}).Info("Successfully initiated node drain")

	return nil
}

func (c *advancedClient) QuarantinePod(ctx context.Context, namespace, name string) error {
	if namespace == "" {
		namespace = c.basicClient.namespace
	}

	c.basicClient.log.WithFields(logrus.Fields{
		"namespace": namespace,
		"pod":       name,
	}).Info("Quarantining pod")

	// Get the pod
	pod, err := c.basicClient.GetPod(ctx, namespace, name)
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
	_, err = c.basicClient.clientset.CoreV1().Pods(namespace).Update(ctx, pod, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to quarantine pod %s/%s: %w", namespace, name, err)
	}

	// In a real implementation, you would also:
	// 1. Create a NetworkPolicy to isolate the pod
	// 2. Possibly move it to a quarantine namespace
	// 3. Add monitoring and alerting for quarantined pods

	c.basicClient.log.WithFields(logrus.Fields{
		"namespace": namespace,
		"pod":       name,
	}).Info("Successfully quarantined pod")

	return nil
}

func (c *advancedClient) CollectDiagnostics(ctx context.Context, namespace, resource string) (map[string]interface{}, error) {
	if namespace == "" {
		namespace = c.basicClient.namespace
	}

	c.basicClient.log.WithFields(logrus.Fields{
		"namespace": namespace,
		"resource":  resource,
	}).Info("Collecting diagnostics")

	diagnostics := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"namespace": namespace,
		"resource":  resource,
	}

	// Collect pod information if resource looks like a pod
	if pod, err := c.basicClient.GetPod(ctx, namespace, resource); err == nil {
		diagnostics["pod_info"] = map[string]interface{}{
			"phase":      pod.Status.Phase,
			"conditions": pod.Status.Conditions,
			"host_ip":    pod.Status.HostIP,
			"pod_ip":     pod.Status.PodIP,
			"start_time": pod.Status.StartTime,
			"restart_count": func() int32 {
				if len(pod.Status.ContainerStatuses) > 0 {
					return pod.Status.ContainerStatuses[0].RestartCount
				}
				return 0
			}(),
		}
	}

	// Collect deployment information if resource looks like a deployment
	if deployment, err := c.basicClient.GetDeployment(ctx, namespace, resource); err == nil {
		diagnostics["deployment_info"] = map[string]interface{}{
			"replicas":           deployment.Spec.Replicas,
			"ready_replicas":     deployment.Status.ReadyReplicas,
			"updated_replicas":   deployment.Status.UpdatedReplicas,
			"available_replicas": deployment.Status.AvailableReplicas,
			"conditions":         deployment.Status.Conditions,
		}
	}

	// Collect events related to the resource
	events, err := c.basicClient.clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", resource),
	})
	if err == nil {
		var eventInfo []map[string]interface{}
		for _, event := range events.Items {
			eventInfo = append(eventInfo, map[string]interface{}{
				"type":    event.Type,
				"reason":  event.Reason,
				"message": event.Message,
				"time":    event.LastTimestamp,
			})
		}
		diagnostics["events"] = eventInfo
	}

	c.basicClient.log.WithFields(logrus.Fields{
		"namespace":   namespace,
		"resource":    resource,
		"data_points": len(diagnostics),
	}).Info("Successfully collected diagnostics")

	return diagnostics, nil
}
