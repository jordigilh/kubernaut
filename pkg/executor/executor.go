package executor

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/mcp"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/slm"
	"github.com/sirupsen/logrus"
)

type Executor interface {
	Execute(ctx context.Context, action *slm.ActionRecommendation, alert slm.Alert) error
	IsHealthy() bool
}

type executor struct {
	mcpClient     mcp.Client
	config        config.ActionsConfig
	log           *logrus.Logger
	
	// Cooldown tracking
	cooldownMu    sync.RWMutex
	lastExecution map[string]time.Time
	
	// Concurrency control
	semaphore     chan struct{}
}

type ExecutionResult struct {
	Action    string    `json:"action"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	DryRun    bool      `json:"dry_run"`
}

func NewExecutor(mcpClient mcp.Client, cfg config.ActionsConfig, log *logrus.Logger) Executor {
	return &executor{
		mcpClient:     mcpClient,
		config:        cfg,
		log:           log,
		lastExecution: make(map[string]time.Time),
		semaphore:     make(chan struct{}, cfg.MaxConcurrent),
	}
}

func (e *executor) Execute(ctx context.Context, action *slm.ActionRecommendation, alert slm.Alert) error {
	// Check cooldown
	if err := e.checkCooldown(alert); err != nil {
		return err
	}

	// Acquire semaphore for concurrency control
	select {
	case e.semaphore <- struct{}{}:
		defer func() { <-e.semaphore }()
	case <-ctx.Done():
		return ctx.Err()
	}

	e.log.WithFields(logrus.Fields{
		"action":    action.Action,
		"alert":     alert.Name,
		"namespace": alert.Namespace,
		"dry_run":   e.config.DryRun,
	}).Info("Executing action")

	var err error
	switch action.Action {
	case "scale_deployment":
		err = e.executeScaleDeployment(ctx, action, alert)
	case "restart_pod":
		err = e.executeRestartPod(ctx, action, alert)
	case "increase_resources":
		err = e.executeIncreaseResources(ctx, action, alert)
	case "notify_only":
		err = e.executeNotifyOnly(ctx, action, alert)
	default:
		err = fmt.Errorf("unknown action: %s", action.Action)
	}

	// Update cooldown tracker
	e.updateCooldown(alert)

	if err != nil {
		e.log.WithFields(logrus.Fields{
			"action": action.Action,
			"alert":  alert.Name,
			"error":  err,
		}).Error("Action execution failed")
		return err
	}

	e.log.WithFields(logrus.Fields{
		"action": action.Action,
		"alert":  alert.Name,
	}).Info("Action executed successfully")

	return nil
}

func (e *executor) executeScaleDeployment(ctx context.Context, action *slm.ActionRecommendation, alert slm.Alert) error {
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

	return e.mcpClient.ScaleDeployment(ctx, alert.Namespace, deploymentName, int32(replicas))
}

func (e *executor) executeRestartPod(ctx context.Context, action *slm.ActionRecommendation, alert slm.Alert) error {
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

	return e.mcpClient.DeletePod(ctx, alert.Namespace, podName)
}

func (e *executor) executeIncreaseResources(ctx context.Context, action *slm.ActionRecommendation, alert slm.Alert) error {
	resources, err := e.getResourcesFromParameters(action.Parameters)
	if err != nil {
		return fmt.Errorf("failed to get resources from parameters: %w", err)
	}

	podName := e.getPodName(alert)
	if podName == "" {
		return fmt.Errorf("cannot determine pod name from alert")
	}

	k8sResources := resources.ToK8sResourceRequirements()

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"pod":       podName,
			"namespace": alert.Namespace,
			"resources": resources,
		}).Info("DRY RUN: Would update pod resources")
		return nil
	}

	return e.mcpClient.UpdatePodResources(ctx, alert.Namespace, podName, k8sResources)
}

func (e *executor) executeNotifyOnly(ctx context.Context, action *slm.ActionRecommendation, alert slm.Alert) error {
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

func (e *executor) getResourcesFromParameters(params map[string]interface{}) (mcp.ResourceRequirements, error) {
	resources := mcp.ResourceRequirements{}

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

func (e *executor) getDeploymentName(alert slm.Alert) string {
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

func (e *executor) getPodName(alert slm.Alert) string {
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

func (e *executor) checkCooldown(alert slm.Alert) error {
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

func (e *executor) updateCooldown(alert slm.Alert) {
	e.cooldownMu.Lock()
	defer e.cooldownMu.Unlock()

	key := fmt.Sprintf("%s/%s", alert.Namespace, alert.Name)
	e.lastExecution[key] = time.Now()
}

func (e *executor) IsHealthy() bool {
	return e.mcpClient != nil && e.mcpClient.IsHealthy()
}