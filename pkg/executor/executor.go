package executor

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/actionhistory"
	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/k8s"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/metrics"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	"github.com/sirupsen/logrus"
)

type Executor interface {
	Execute(ctx context.Context, action *types.ActionRecommendation, alert types.Alert, actionTrace *actionhistory.ResourceActionTrace) error
	IsHealthy() bool
	GetActionRegistry() *ActionRegistry
}

type executor struct {
	k8sClient         k8s.Client
	config            config.ActionsConfig
	actionHistoryRepo actionhistory.Repository
	log               *logrus.Logger
	registry          *ActionRegistry

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

func NewExecutor(k8sClient k8s.Client, cfg config.ActionsConfig, actionHistoryRepo actionhistory.Repository, log *logrus.Logger) Executor {
	registry := NewActionRegistry()

	e := &executor{
		k8sClient:         k8sClient,
		config:            cfg,
		actionHistoryRepo: actionHistoryRepo,
		log:               log,
		registry:          registry,
		lastExecution:     make(map[string]time.Time),
		semaphore:         make(chan struct{}, cfg.MaxConcurrent),
	}

	// Register all built-in actions
	e.registerBuiltinActions()

	return e
}

func (e *executor) Execute(ctx context.Context, action *types.ActionRecommendation, alert types.Alert, actionTrace *actionhistory.ResourceActionTrace) error {
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
	executionStart := time.Now()

	// Update action trace with execution start (if available)
	if actionTrace != nil && e.actionHistoryRepo != nil {
		actionTrace.ExecutionStartTime = &executionStart
		actionTrace.ExecutionStatus = "running"
	}

	// Execute action using registry
	err := e.registry.Execute(ctx, action, alert)

	executionEnd := time.Now()
	executionDuration := executionEnd.Sub(executionStart)

	// Update cooldown tracker
	e.updateCooldown(alert)

	// Update action trace with execution results (if available)
	if actionTrace != nil && e.actionHistoryRepo != nil {
		e.updateActionTrace(ctx, actionTrace, err, executionEnd, executionDuration)
	}

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

// updateActionTrace updates the action trace with execution results
func (e *executor) updateActionTrace(ctx context.Context, actionTrace *actionhistory.ResourceActionTrace, execErr error, executionEnd time.Time, duration time.Duration) {
	actionTrace.ExecutionEndTime = &executionEnd
	durationMs := int(duration.Milliseconds())
	actionTrace.ExecutionDurationMs = &durationMs

	if execErr != nil {
		actionTrace.ExecutionStatus = "failed"
		errorMsg := execErr.Error()
		actionTrace.ExecutionError = &errorMsg
		// Failed actions get immediate low effectiveness score
		immediateEffectiveness := 0.1
		actionTrace.EffectivenessScore = &immediateEffectiveness
		assessedAt := time.Now()
		actionTrace.EffectivenessAssessedAt = &assessedAt
		method := "immediate_failure"
		actionTrace.EffectivenessAssessmentMethod = &method
		notes := "Action execution failed, immediate low effectiveness assigned"
		actionTrace.EffectivenessNotes = &notes
	} else {
		actionTrace.ExecutionStatus = "completed"
		// For successful actions, defer effectiveness assessment
		actionTrace.EffectivenessScore = nil
		assessmentDue := time.Now().Add(10 * time.Minute)
		actionTrace.EffectivenessAssessmentDue = &assessmentDue

		e.log.WithFields(logrus.Fields{
			"action_id":      actionTrace.ActionID,
			"assessment_due": assessmentDue,
		}).Debug("Scheduled effectiveness assessment")
	}

	// Update the trace in the database
	if err := e.actionHistoryRepo.UpdateActionTrace(ctx, actionTrace); err != nil {
		e.log.WithError(err).Warn("Failed to update action trace")
	} else {
		e.log.WithFields(logrus.Fields{
			"action_id": actionTrace.ActionID,
			"status":    actionTrace.ExecutionStatus,
			"duration":  duration,
		}).Debug("Updated action trace")
	}
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

	// Register storage & persistence actions
	e.registry.Register("cleanup_storage", e.executeCleanupStorage)
	e.registry.Register("backup_data", e.executeBackupData)
	e.registry.Register("compact_storage", e.executeCompactStorage)

	// Register application lifecycle actions
	e.registry.Register("cordon_node", e.executeCordonNode)
	e.registry.Register("update_hpa", e.executeUpdateHPA)
	e.registry.Register("restart_daemonset", e.executeRestartDaemonSet)

	// Register security & compliance actions
	e.registry.Register("rotate_secrets", e.executeRotateSecrets)
	e.registry.Register("audit_logs", e.executeAuditLogs)

	// Register network & connectivity actions
	e.registry.Register("update_network_policy", e.executeUpdateNetworkPolicy)
	e.registry.Register("restart_network", e.executeRestartNetwork)
	e.registry.Register("reset_service_mesh", e.executeResetServiceMesh)

	// Register database & stateful actions
	e.registry.Register("failover_database", e.executeFailoverDatabase)
	e.registry.Register("repair_database", e.executeRepairDatabase)
	e.registry.Register("scale_statefulset", e.executeScaleStatefulSet)

	// Register monitoring & observability actions
	e.registry.Register("enable_debug_mode", e.executeEnableDebugMode)
	e.registry.Register("create_heap_dump", e.executeCreateHeapDump)

	// Register resource management actions
	e.registry.Register("optimize_resources", e.executeOptimizeResources)
	e.registry.Register("migrate_workload", e.executeMigrateWorkload)
}

// GetActionRegistry returns the action registry for external use (e.g., adding custom actions)
func (e *executor) GetActionRegistry() *ActionRegistry {
	return e.registry
}

// Storage & Persistence Actions

func (e *executor) executeCleanupStorage(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	path := "/var/log"
	if pathParam, ok := action.Parameters["path"].(string); ok {
		path = pathParam
	}

	podName := e.getPodName(alert)
	if podName == "" {
		return fmt.Errorf("cannot determine pod name from alert")
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"pod":       podName,
			"namespace": alert.Namespace,
			"path":      path,
		}).Info("DRY RUN: Would cleanup storage")
		return nil
	}

	return e.k8sClient.CleanupStorage(ctx, alert.Namespace, podName, path)
}

func (e *executor) executeBackupData(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	resource := alert.Resource
	if resource == "" {
		return fmt.Errorf("cannot determine resource name from alert")
	}

	backupName := fmt.Sprintf("%s-emergency-backup-%d", resource, time.Now().Unix())
	if name, ok := action.Parameters["backup_name"].(string); ok {
		backupName = name
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"resource":    resource,
			"namespace":   alert.Namespace,
			"backup_name": backupName,
		}).Info("DRY RUN: Would backup data")
		return nil
	}

	return e.k8sClient.BackupData(ctx, alert.Namespace, resource, backupName)
}

func (e *executor) executeCompactStorage(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	resource := alert.Resource
	if resource == "" {
		return fmt.Errorf("cannot determine resource name from alert")
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"resource":  resource,
			"namespace": alert.Namespace,
		}).Info("DRY RUN: Would compact storage")
		return nil
	}

	return e.k8sClient.CompactStorage(ctx, alert.Namespace, resource)
}

// Application Lifecycle Actions

func (e *executor) executeCordonNode(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	nodeName := alert.Resource
	if nodeName == "" {
		if node, ok := alert.Labels["node"]; ok {
			nodeName = node
		} else {
			return fmt.Errorf("cannot determine node name from alert")
		}
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"node": nodeName,
		}).Info("DRY RUN: Would cordon node")
		return nil
	}

	return e.k8sClient.CordonNode(ctx, nodeName)
}

func (e *executor) executeUpdateHPA(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	hpaName := alert.Resource
	if hpaName == "" {
		if hpa, ok := alert.Labels["hpa"]; ok {
			hpaName = hpa
		} else {
			return fmt.Errorf("cannot determine HPA name from alert")
		}
	}

	minReplicas := int32(1)
	maxReplicas := int32(10)

	if min, ok := action.Parameters["min_replicas"].(float64); ok {
		minReplicas = int32(min)
	}
	if max, ok := action.Parameters["max_replicas"].(float64); ok {
		maxReplicas = int32(max)
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"hpa":          hpaName,
			"namespace":    alert.Namespace,
			"min_replicas": minReplicas,
			"max_replicas": maxReplicas,
		}).Info("DRY RUN: Would update HPA")
		return nil
	}

	return e.k8sClient.UpdateHPA(ctx, alert.Namespace, hpaName, minReplicas, maxReplicas)
}

func (e *executor) executeRestartDaemonSet(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	daemonSetName := alert.Resource
	if daemonSetName == "" {
		if ds, ok := alert.Labels["daemonset"]; ok {
			daemonSetName = ds
		} else {
			return fmt.Errorf("cannot determine DaemonSet name from alert")
		}
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"daemonset": daemonSetName,
			"namespace": alert.Namespace,
		}).Info("DRY RUN: Would restart DaemonSet")
		return nil
	}

	return e.k8sClient.RestartDaemonSet(ctx, alert.Namespace, daemonSetName)
}

// Security & Compliance Actions

func (e *executor) executeRotateSecrets(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	secretName := alert.Resource
	if secretName == "" {
		if secret, ok := alert.Labels["secret"]; ok {
			secretName = secret
		} else {
			return fmt.Errorf("cannot determine secret name from alert")
		}
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"secret":    secretName,
			"namespace": alert.Namespace,
		}).Info("DRY RUN: Would rotate secrets")
		return nil
	}

	return e.k8sClient.RotateSecrets(ctx, alert.Namespace, secretName)
}

func (e *executor) executeAuditLogs(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	resource := alert.Resource
	if resource == "" {
		return fmt.Errorf("cannot determine resource name from alert")
	}

	auditScope := "security"
	if scope, ok := action.Parameters["scope"].(string); ok {
		auditScope = scope
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"resource":    resource,
			"namespace":   alert.Namespace,
			"audit_scope": auditScope,
		}).Info("DRY RUN: Would audit logs")
		return nil
	}

	return e.k8sClient.AuditLogs(ctx, alert.Namespace, resource, auditScope)
}

// Network & Connectivity Actions

func (e *executor) executeUpdateNetworkPolicy(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	policyName := alert.Resource
	if policyName == "" {
		if policy, ok := alert.Labels["network_policy"]; ok {
			policyName = policy
		} else {
			return fmt.Errorf("cannot determine network policy name from alert")
		}
	}

	action_type := "allow_egress"
	if actionType, ok := action.Parameters["action_type"].(string); ok {
		action_type = actionType
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"policy":      policyName,
			"namespace":   alert.Namespace,
			"action_type": action_type,
		}).Info("DRY RUN: Would update network policy")
		return nil
	}

	return e.k8sClient.UpdateNetworkPolicy(ctx, alert.Namespace, policyName, action_type)
}

func (e *executor) executeRestartNetwork(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	component := "cni"
	if comp, ok := action.Parameters["component"].(string); ok {
		component = comp
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"component": component,
			"namespace": alert.Namespace,
		}).Info("DRY RUN: Would restart network component")
		return nil
	}

	return e.k8sClient.RestartNetwork(ctx, component)
}

func (e *executor) executeResetServiceMesh(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	meshType := "istio"
	if mesh, ok := action.Parameters["mesh_type"].(string); ok {
		meshType = mesh
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"mesh_type": meshType,
			"namespace": alert.Namespace,
		}).Info("DRY RUN: Would reset service mesh")
		return nil
	}

	return e.k8sClient.ResetServiceMesh(ctx, meshType)
}

// Database & Stateful Actions

func (e *executor) executeFailoverDatabase(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	databaseName := alert.Resource
	if databaseName == "" {
		if db, ok := alert.Labels["database"]; ok {
			databaseName = db
		} else {
			return fmt.Errorf("cannot determine database name from alert")
		}
	}

	replicaName := databaseName + "-replica"
	if replica, ok := action.Parameters["replica_name"].(string); ok {
		replicaName = replica
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"database":  databaseName,
			"replica":   replicaName,
			"namespace": alert.Namespace,
		}).Info("DRY RUN: Would failover database")
		return nil
	}

	return e.k8sClient.FailoverDatabase(ctx, alert.Namespace, databaseName, replicaName)
}

func (e *executor) executeRepairDatabase(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	databaseName := alert.Resource
	if databaseName == "" {
		if db, ok := alert.Labels["database"]; ok {
			databaseName = db
		} else {
			return fmt.Errorf("cannot determine database name from alert")
		}
	}

	repairType := "consistency_check"
	if repair, ok := action.Parameters["repair_type"].(string); ok {
		repairType = repair
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"database":    databaseName,
			"namespace":   alert.Namespace,
			"repair_type": repairType,
		}).Info("DRY RUN: Would repair database")
		return nil
	}

	return e.k8sClient.RepairDatabase(ctx, alert.Namespace, databaseName, repairType)
}

func (e *executor) executeScaleStatefulSet(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	statefulSetName := alert.Resource
	if statefulSetName == "" {
		if sts, ok := alert.Labels["statefulset"]; ok {
			statefulSetName = sts
		} else {
			return fmt.Errorf("cannot determine StatefulSet name from alert")
		}
	}

	replicas, err := e.getReplicasFromParameters(action.Parameters)
	if err != nil {
		return fmt.Errorf("failed to get replicas from parameters: %w", err)
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"statefulset": statefulSetName,
			"namespace":   alert.Namespace,
			"replicas":    replicas,
		}).Info("DRY RUN: Would scale StatefulSet")
		return nil
	}

	return e.k8sClient.ScaleStatefulSet(ctx, alert.Namespace, statefulSetName, int32(replicas))
}

// Monitoring & Observability Actions

func (e *executor) executeEnableDebugMode(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	resource := alert.Resource
	if resource == "" {
		return fmt.Errorf("cannot determine resource name from alert")
	}

	logLevel := "debug"
	if level, ok := action.Parameters["log_level"].(string); ok {
		logLevel = level
	}

	duration := "30m"
	if dur, ok := action.Parameters["duration"].(string); ok {
		duration = dur
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"resource":  resource,
			"namespace": alert.Namespace,
			"log_level": logLevel,
			"duration":  duration,
		}).Info("DRY RUN: Would enable debug mode")
		return nil
	}

	return e.k8sClient.EnableDebugMode(ctx, alert.Namespace, resource, logLevel, duration)
}

func (e *executor) executeCreateHeapDump(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	podName := e.getPodName(alert)
	if podName == "" {
		return fmt.Errorf("cannot determine pod name from alert")
	}

	dumpPath := "/tmp/heap-dump.hprof"
	if path, ok := action.Parameters["dump_path"].(string); ok {
		dumpPath = path
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"pod":       podName,
			"namespace": alert.Namespace,
			"dump_path": dumpPath,
		}).Info("DRY RUN: Would create heap dump")
		return nil
	}

	return e.k8sClient.CreateHeapDump(ctx, alert.Namespace, podName, dumpPath)
}

// Resource Management Actions

func (e *executor) executeOptimizeResources(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	resource := alert.Resource
	if resource == "" {
		return fmt.Errorf("cannot determine resource name from alert")
	}

	optimizationType := "cpu_memory"
	if opt, ok := action.Parameters["optimization_type"].(string); ok {
		optimizationType = opt
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"resource":          resource,
			"namespace":         alert.Namespace,
			"optimization_type": optimizationType,
		}).Info("DRY RUN: Would optimize resources")
		return nil
	}

	return e.k8sClient.OptimizeResources(ctx, alert.Namespace, resource, optimizationType)
}

func (e *executor) executeMigrateWorkload(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	workloadName := alert.Resource
	if workloadName == "" {
		return fmt.Errorf("cannot determine workload name from alert")
	}

	targetNode := ""
	if node, ok := action.Parameters["target_node"].(string); ok {
		targetNode = node
	}

	if e.config.DryRun {
		e.log.WithFields(logrus.Fields{
			"workload":    workloadName,
			"namespace":   alert.Namespace,
			"target_node": targetNode,
		}).Info("DRY RUN: Would migrate workload")
		return nil
	}

	return e.k8sClient.MigrateWorkload(ctx, alert.Namespace, workloadName, targetNode)
}
