package executor

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/k8s"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/metrics"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	"github.com/sirupsen/logrus"
)

type Executor interface {
	Execute(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error
	IsHealthy() bool
	GetActionRegistry() *ActionRegistry
}

type executor struct {
	k8sClient k8s.Client
	config    config.ActionsConfig
	log       *logrus.Logger
	registry  *ActionRegistry

	// Cooldown tracking
	cooldownMu    sync.RWMutex
	lastExecution map[string]time.Time

	// Concurrency control
	semaphore chan struct{}
}

type ExecutionResult struct {
	Action    string    `json:"action"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	DryRun    bool      `json:"dry_run"`
}

func NewExecutor(k8sClient k8s.Client, cfg config.ActionsConfig, log *logrus.Logger) Executor {
	registry := NewActionRegistry()
	
	e := &executor{
		k8sClient:     k8sClient,
		config:        cfg,
		log:           log,
		registry:      registry,
		lastExecution: make(map[string]time.Time),
		semaphore:     make(chan struct{}, cfg.MaxConcurrent),
	}
	
	// Register all built-in actions
	e.registerBuiltinActions()
	
	return e
}

func (e *executor) Execute(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	// Check cooldown
	if err := e.checkCooldown(alert); err != nil {
		return err
	}

	// Acquire semaphore for concurrency control
	select {
	case e.semaphore <- struct{}{}:
		metrics.IncrementConcurrentActions()
		defer func() {
			<-e.semaphore
			metrics.DecrementConcurrentActions()
		}()
	case <-ctx.Done():
		return ctx.Err()
	}

	e.log.WithFields(logrus.Fields{
		"action":    action.Action,
		"alert":     alert.Name,
		"namespace": alert.Namespace,
		"dry_run":   e.config.DryRun,
	}).Info("Executing action")

	// Start timing the action execution
	timer := metrics.NewTimer()

	// Execute action using registry
	err := e.registry.Execute(ctx, action, alert)

	// Update cooldown tracker
	e.updateCooldown(alert)

	if err != nil {
		// Record error metrics
		metrics.RecordActionError(action.Action, "execution_failed")
		e.log.WithFields(logrus.Fields{
			"action": action.Action,
			"alert":  alert.Name,
			"error":  err,
		}).Error("Action execution failed")
		return err
	}

	// Record successful action execution with timing
	timer.RecordAction(action.Action)

	e.log.WithFields(logrus.Fields{
		"action": action.Action,
		"alert":  alert.Name,
	}).Info("Action executed successfully")

	return nil
}

func (e *executor) executeScaleDeployment(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	replicas, err := e.getReplicasFromParameters(action.Parameters)
	if err != nil {
		return fmt.Errorf("failed to get replicas from parameters: %w", err)
	}

	deploymentName := e.getDeploymentName(alert)
	if deploymentName == "" {
		return fmt.Errorf("cannot determine deployment name from alert")
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"deployment": deploymentName,
			"namespace":  alert.Namespace,
			"replicas":   replicas,
		}).Info("DRY RUN: Would scale deployment")
		return nil
	}

	return e.k8sClient.ScaleDeployment(ctx, alert.Namespace, deploymentName, int32(replicas))
}

func (e *executor) executeRestartPod(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	podName := e.getPodName(alert)
	if podName == "" {
		return fmt.Errorf("cannot determine pod name from alert")
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"pod":       podName,
			"namespace": alert.Namespace,
		}).Info("DRY RUN: Would restart pod")
		return nil
	}

	return e.k8sClient.DeletePod(ctx, alert.Namespace, podName)
}

func (e *executor) executeIncreaseResources(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	resources, err := e.getResourcesFromParameters(action.Parameters)
	if err != nil {
		return fmt.Errorf("failed to get resources from parameters: %w", err)
	}

	podName := e.getPodName(alert)
	if podName == "" {
		return fmt.Errorf("cannot determine pod name from alert")
	}

	k8sResources, err := resources.ToK8sResourceRequirements()
	if err != nil {
		return fmt.Errorf("failed to convert resource requirements: %w", err)
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"pod":       podName,
			"namespace": alert.Namespace,
			"resources": resources,
		}).Info("DRY RUN: Would update pod resources")
		return nil
	}

	return e.k8sClient.UpdatePodResources(ctx, alert.Namespace, podName, k8sResources)
}

func (e *executor) executeNotifyOnly(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	message := "Alert received - no automated action taken"
	if msg, ok := action.Parameters["message"].(string); ok {
		message = msg
	}

	e.log.WithFields(logrus.Fields{
		"alert":     alert.Name,
		"namespace": alert.Namespace,
		"message":   message,
		"reasoning": action.Reasoning,
	}).Warn("NOTIFICATION: Manual intervention may be required")

	return nil
}

func (e *executor) executeRollbackDeployment(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	deploymentName := e.getDeploymentName(alert)
	if deploymentName == "" {
		return fmt.Errorf("cannot determine deployment name from alert")
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"deployment": deploymentName,
			"namespace":  alert.Namespace,
		}).Info("DRY RUN: Would rollback deployment")
		return nil
	}

	return e.k8sClient.RollbackDeployment(ctx, alert.Namespace, deploymentName)
}

func (e *executor) executeExpandPVC(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	pvcName := alert.Resource
	if pvcName == "" {
		return fmt.Errorf("cannot determine PVC name from alert")
	}

	// Get new size from parameters
	newSize := "10Gi" // default
	if size, ok := action.Parameters["new_size"].(string); ok {
		newSize = size
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"pvc":       pvcName,
			"namespace": alert.Namespace,
			"new_size":  newSize,
		}).Info("DRY RUN: Would expand PVC")
		return nil
	}

	return e.k8sClient.ExpandPVC(ctx, alert.Namespace, pvcName, newSize)
}

func (e *executor) executeDrainNode(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	nodeName := alert.Resource
	if nodeName == "" {
		// Try to get node name from alert labels
		if node, ok := alert.Labels["node"]; ok {
			nodeName = node
		} else {
			return fmt.Errorf("cannot determine node name from alert")
		}
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"node": nodeName,
		}).Info("DRY RUN: Would drain node")
		return nil
	}

	return e.k8sClient.DrainNode(ctx, nodeName)
}

func (e *executor) executeQuarantinePod(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	podName := alert.Resource
	if podName == "" {
		return fmt.Errorf("cannot determine pod name from alert")
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"pod":       podName,
			"namespace": alert.Namespace,
		}).Info("DRY RUN: Would quarantine pod")
		return nil
	}

	return e.k8sClient.QuarantinePod(ctx, alert.Namespace, podName)
}

func (e *executor) executeCollectDiagnostics(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	resource := alert.Resource
	if resource == "" {
		return fmt.Errorf("cannot determine resource name from alert")
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"resource":  resource,
			"namespace": alert.Namespace,
		}).Info("DRY RUN: Would collect diagnostics")
		return nil
	}

	diagnostics, err := e.k8sClient.CollectDiagnostics(ctx, alert.Namespace, resource)
	if err != nil {
		return err
	}

	// Log diagnostic summary
	e.log.WithFields(logrus.Fields{
		"resource":    resource,
		"namespace":   alert.Namespace,
		"diagnostics": diagnostics,
	}).Info("Collected diagnostic information")

	return nil
}

func (e *executor) getReplicasFromParameters(params map[string]interface{}) (int, error) {
	replicasInterface, ok := params["replicas"]
	if !ok {
		return 0, fmt.Errorf("replicas parameter not found")
	}

	switch v := replicasInterface.(type) {
	case int:
		return v, nil
	case float64:
		return int(v), nil
	case string:
		return strconv.Atoi(v)
	default:
		return 0, fmt.Errorf("invalid replicas type: %T", v)
	}
}

func (e *executor) getResourcesFromParameters(params map[string]interface{}) (k8s.ResourceRequirements, error) {
	resources := k8s.ResourceRequirements{}

	if cpuLimit, ok := params["cpu_limit"].(string); ok {
		resources.CPULimit = cpuLimit
	}
	if memoryLimit, ok := params["memory_limit"].(string); ok {
		resources.MemoryLimit = memoryLimit
	}
	if cpuRequest, ok := params["cpu_request"].(string); ok {
		resources.CPURequest = cpuRequest
	}
	if memoryRequest, ok := params["memory_request"].(string); ok {
		resources.MemoryRequest = memoryRequest
	}

	// Set defaults if not specified
	if resources.CPULimit == "" && resources.CPURequest == "" {
		resources.CPULimit = "500m"
		resources.CPURequest = "250m"
	}
	if resources.MemoryLimit == "" && resources.MemoryRequest == "" {
		resources.MemoryLimit = "1Gi"
		resources.MemoryRequest = "512Mi"
	}

	return resources, nil
}

func (e *executor) getDeploymentName(alert types.Alert) string {
	// Try to extract deployment name from various sources
	if deployment, ok := alert.Labels["deployment"]; ok {
		return deployment
	}
	if deployment, ok := alert.Labels["app"]; ok {
		return deployment
	}
	if deployment, ok := alert.Annotations["deployment"]; ok {
		return deployment
	}

	// Try to extract from resource field
	if alert.Resource != "" {
		return alert.Resource
	}

	return ""
}

func (e *executor) getPodName(alert types.Alert) string {
	// Try to extract pod name from various sources
	if pod, ok := alert.Labels["pod"]; ok {
		return pod
	}
	if pod, ok := alert.Labels["pod_name"]; ok {
		return pod
	}
	if pod, ok := alert.Annotations["pod"]; ok {
		return pod
	}

	// Try to extract from resource field
	if alert.Resource != "" {
		return alert.Resource
	}

	return ""
}

func (e *executor) checkCooldown(alert types.Alert) error {
	e.cooldownMu.RLock()
	defer e.cooldownMu.RUnlock()

	key := fmt.Sprintf("%s/%s", alert.Namespace, alert.Name)
	if lastExec, exists := e.lastExecution[key]; exists {
		if time.Since(lastExec) < e.config.CooldownPeriod {
			return fmt.Errorf("action for alert %s is in cooldown period", key)
		}
	}

	return nil
}

func (e *executor) updateCooldown(alert types.Alert) {
	e.cooldownMu.Lock()
	defer e.cooldownMu.Unlock()

	key := fmt.Sprintf("%s/%s", alert.Namespace, alert.Name)
	e.lastExecution[key] = time.Now()
}

func (e *executor) IsHealthy() bool {
	return e.k8sClient != nil && e.k8sClient.IsHealthy()
}

// registerBuiltinActions registers all built-in action handlers with the registry
func (e *executor) registerBuiltinActions() {
	// Register basic actions
	e.registry.Register("scale_deployment", e.executeScaleDeployment)
	e.registry.Register("restart_pod", e.executeRestartPod)
	e.registry.Register("increase_resources", e.executeIncreaseResources)
	e.registry.Register("notify_only", e.executeNotifyOnly)
	
	// Register advanced actions
	e.registry.Register("rollback_deployment", e.executeRollbackDeployment)
	e.registry.Register("expand_pvc", e.executeExpandPVC)
	e.registry.Register("drain_node", e.executeDrainNode)
	e.registry.Register("quarantine_pod", e.executeQuarantinePod)
	e.registry.Register("collect_diagnostics", e.executeCollectDiagnostics)
}

// GetActionRegistry returns the action registry for external use (e.g., adding custom actions)
func (e *executor) GetActionRegistry() *ActionRegistry {
	return e.registry
}
