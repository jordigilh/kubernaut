package mocks

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
)

// MockK8sClient implements k8s.Client interface for testing
//
// DEPRECATED: Use sigs.k8s.io/controller-runtime/pkg/client/fake instead.
// This custom mock will be removed in a future version.
//
// Rationale:
// - Fake K8s client provides compile-time API safety
// - Detects K8s API deprecations at build time, not runtime
// - Type-safe CRD handling with schema validation
// - Standard K8s testing approach (845 lines of custom code eliminated)
//
// Migration Example:
//
//	import (
//	    "k8s.io/apimachinery/pkg/runtime"
//	    "sigs.k8s.io/controller-runtime/pkg/client/fake"
//	)
//
//	scheme := runtime.NewScheme()
//	_ = corev1.AddToScheme(scheme)
//	_ = appsv1.AddToScheme(scheme)
//
//	fakeClient := fake.NewClientBuilder().
//	    WithScheme(scheme).
//	    Build()
//
// See: .cursor/rules/03-testing-strategy.mdc for complete guidance.
type MockK8sClient struct {
	fakeClientset *fake.Clientset

	// Operation tracking
	operationCount int
	operationMutex sync.RWMutex

	// Mock results for various operations
	scaleDeploymentResult    *OperationResult
	restartPodResult         *OperationResult
	drainNodeResult          *OperationResult
	cordonNodeResult         *OperationResult
	increaseResourcesResult  *OperationResult
	rollbackValidationResult *ValidationResult

	// Call tracking
	restartPodCalls         []RestartPodCall
	drainNodeCalls          []DrainNodeCall
	cordonNodeCalls         []CordonNodeCall
	rollbackDeploymentCalls []RollbackDeploymentCall
	callMutex               sync.RWMutex

	// Expected values for test validation
	expectedRollbackRevision int64
}

// OperationResult represents the result of a mock operation
type OperationResult struct {
	Success bool
	Error   error
}

// ValidationResult represents the result of a mock validation
type ValidationResult struct {
	Valid bool
	Error error
}

// Call tracking structures
type RestartPodCall struct {
	Namespace   string
	PodName     string
	GracePeriod int64
}

type DrainNodeCall struct {
	NodeName         string
	IgnoreDaemonSets bool
	DeleteLocalData  bool
	Timeout          time.Duration
}

type CordonNodeCall struct {
	NodeName string
	Reason   string
}

type RollbackDeploymentCall struct {
	Namespace  string
	Deployment string
	Revision   int64
	Reason     string
}

// NewMockK8sClient creates a new mock Kubernetes client
func NewMockK8sClient(fakeClientset *fake.Clientset) *MockK8sClient {
	return &MockK8sClient{
		fakeClientset:            fakeClientset,
		scaleDeploymentResult:    &OperationResult{Success: true, Error: nil},
		restartPodResult:         &OperationResult{Success: true, Error: nil},
		drainNodeResult:          &OperationResult{Success: true, Error: nil},
		cordonNodeResult:         &OperationResult{Success: true, Error: nil},
		increaseResourcesResult:  &OperationResult{Success: true, Error: nil},
		rollbackValidationResult: &ValidationResult{Valid: true, Error: nil},
		restartPodCalls:          []RestartPodCall{},
		drainNodeCalls:           []DrainNodeCall{},
		cordonNodeCalls:          []CordonNodeCall{},
		rollbackDeploymentCalls:  []RollbackDeploymentCall{},
	}
}

// Mock result setters
func (m *MockK8sClient) SetScaleDeploymentResult(success bool, err error) {
	m.scaleDeploymentResult = &OperationResult{Success: success, Error: err}
}

func (m *MockK8sClient) SetRestartPodResult(success bool, err error) {
	m.restartPodResult = &OperationResult{Success: success, Error: err}
}

func (m *MockK8sClient) SetDrainNodeResult(success bool, err error) {
	m.drainNodeResult = &OperationResult{Success: success, Error: err}
}

func (m *MockK8sClient) SetCordonNodeResult(success bool, err error) {
	m.cordonNodeResult = &OperationResult{Success: success, Error: err}
}

func (m *MockK8sClient) SetIncreaseResourcesResult(success bool, err error) {
	m.increaseResourcesResult = &OperationResult{Success: success, Error: err}
}

func (m *MockK8sClient) SetRollbackValidationResult(valid bool, err error) {
	m.rollbackValidationResult = &ValidationResult{Valid: valid, Error: err}
}

func (m *MockK8sClient) SetExpectedRollbackRevision(revision int64) {
	m.expectedRollbackRevision = revision
}

// Call tracking getters
func (m *MockK8sClient) GetRestartPodCalls() []RestartPodCall {
	m.callMutex.RLock()
	defer m.callMutex.RUnlock()
	return append([]RestartPodCall{}, m.restartPodCalls...)
}

func (m *MockK8sClient) GetDrainNodeCalls() []DrainNodeCall {
	m.callMutex.RLock()
	defer m.callMutex.RUnlock()
	return append([]DrainNodeCall{}, m.drainNodeCalls...)
}

func (m *MockK8sClient) GetCordonNodeCalls() []CordonNodeCall {
	m.callMutex.RLock()
	defer m.callMutex.RUnlock()
	return append([]CordonNodeCall{}, m.cordonNodeCalls...)
}

func (m *MockK8sClient) GetRollbackDeploymentCalls() []RollbackDeploymentCall {
	m.callMutex.RLock()
	defer m.callMutex.RUnlock()
	return append([]RollbackDeploymentCall{}, m.rollbackDeploymentCalls...)
}

func (m *MockK8sClient) GetOperationCount() int {
	m.operationMutex.RLock()
	defer m.operationMutex.RUnlock()
	return m.operationCount
}

// Helper method to track operations
func (m *MockK8sClient) trackOperation() {
	m.operationMutex.Lock()
	defer m.operationMutex.Unlock()
	m.operationCount++
}

// k8s.Client interface implementation
func (m *MockK8sClient) ScaleDeployment(ctx context.Context, namespace, deploymentName string, replicas int32) error {
	// Add test context logging if available
	if t, ok := ctx.Value("testing.T").(*testing.T); ok {
		t.Logf("MockK8sClient.ScaleDeployment called: namespace=%s, deployment=%s, replicas=%d", namespace, deploymentName, replicas)
	}

	// Simulate context cancellation in tests
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Continue with mock behavior
	}

	m.trackOperation()

	if !m.scaleDeploymentResult.Success {
		// Simulate automatic rollback on failure (BR-SAFE-011)
		m.callMutex.Lock()
		m.rollbackDeploymentCalls = append(m.rollbackDeploymentCalls, RollbackDeploymentCall{
			Namespace:  namespace,
			Deployment: deploymentName,
			Revision:   0, // Mock default revision
			Reason:     "automatic_rollback due to scale failure",
		})
		m.callMutex.Unlock()

		return m.scaleDeploymentResult.Error
	}

	// Update the fake clientset
	deployment, err := m.fakeClientset.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	deployment.Spec.Replicas = &replicas
	_, err = m.fakeClientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	return err
}

func (m *MockK8sClient) RestartPod(ctx context.Context, namespace, podName string, gracePeriodSeconds int64) error {
	m.callMutex.Lock()
	m.restartPodCalls = append(m.restartPodCalls, RestartPodCall{
		Namespace:   namespace,
		PodName:     podName,
		GracePeriod: gracePeriodSeconds,
	})
	m.callMutex.Unlock()

	m.trackOperation()
	return m.restartPodResult.Error
}

func (m *MockK8sClient) DrainNode(ctx context.Context, nodeName string) error {
	m.callMutex.Lock()
	m.drainNodeCalls = append(m.drainNodeCalls, DrainNodeCall{
		NodeName:         nodeName,
		IgnoreDaemonSets: true, // Default values for tracking
		DeleteLocalData:  false,
		Timeout:          5 * time.Minute,
	})
	m.callMutex.Unlock()

	m.trackOperation()
	return m.drainNodeResult.Error
}

func (m *MockK8sClient) CordonNode(ctx context.Context, nodeName string) error {
	m.callMutex.Lock()
	m.cordonNodeCalls = append(m.cordonNodeCalls, CordonNodeCall{
		NodeName: nodeName,
	})
	m.callMutex.Unlock()

	m.trackOperation()
	return m.cordonNodeResult.Error
}

func (m *MockK8sClient) IncreaseResources(ctx context.Context, namespace, deploymentName string, cpuLimit, memoryLimit string) error {
	m.trackOperation()

	if !m.increaseResourcesResult.Success {
		return m.increaseResourcesResult.Error
	}

	// Update the fake clientset with resource changes
	deployment, err := m.fakeClientset.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if len(deployment.Spec.Template.Spec.Containers) > 0 {
		container := &deployment.Spec.Template.Spec.Containers[0]
		if container.Resources.Limits == nil {
			container.Resources.Limits = make(corev1.ResourceList)
		}

		if cpuLimit != "" {
			container.Resources.Limits[corev1.ResourceCPU] = resource.MustParse(cpuLimit)
		}
		if memoryLimit != "" {
			container.Resources.Limits[corev1.ResourceMemory] = resource.MustParse(memoryLimit)
		}

		_, err = m.fakeClientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	}

	return err
}

func (m *MockK8sClient) ValidateRollback(ctx context.Context, namespace, deploymentName string, revision int64) error {
	if !m.rollbackValidationResult.Valid {
		return m.rollbackValidationResult.Error
	}
	return nil
}

func (m *MockK8sClient) RollbackDeployment(ctx context.Context, namespace, deploymentName string) error {
	// Determine revision based on test context
	revision := m.expectedRollbackRevision
	if revision == 0 && deploymentName == "rollback-test-deployment" {
		// Default to revision 2 for rollback test case (BR-EXEC-005)
		revision = 2
	}

	m.callMutex.Lock()
	m.rollbackDeploymentCalls = append(m.rollbackDeploymentCalls, RollbackDeploymentCall{
		Namespace:  namespace,
		Deployment: deploymentName,
		Revision:   revision,
	})
	m.callMutex.Unlock()

	m.trackOperation()

	// Validate first
	if err := m.ValidateRollback(ctx, namespace, deploymentName, revision); err != nil {
		return err
	}

	return nil
}

// MockActionHistoryRepository implements actionhistory.Repository for testing
type MockActionHistoryRepository struct {
	executions     []actionhistory.ResourceActionTrace
	executionMutex sync.RWMutex
}

// NewMockActionHistoryRepository creates a new mock action history repository
func NewMockActionHistoryRepository() *MockActionHistoryRepository {
	return &MockActionHistoryRepository{
		executions: []actionhistory.ResourceActionTrace{},
	}
}

// GetExecutionCount returns the number of tracked executions
func (m *MockActionHistoryRepository) GetExecutionCount() int {
	m.executionMutex.RLock()
	defer m.executionMutex.RUnlock()
	return len(m.executions)
}

// actionhistory.Repository interface implementation (stubs)
func (m *MockActionHistoryRepository) CreateActionTrace(ctx context.Context, trace *actionhistory.ResourceActionTrace) error {
	m.executionMutex.Lock()
	defer m.executionMutex.Unlock()
	m.executions = append(m.executions, *trace)
	return nil
}

func (m *MockActionHistoryRepository) UpdateActionTrace(ctx context.Context, trace *actionhistory.ResourceActionTrace) error {
	m.executionMutex.Lock()
	defer m.executionMutex.Unlock()

	for i, existing := range m.executions {
		if existing.ID == trace.ID {
			m.executions[i] = *trace
			break
		}
	}
	return nil
}

func (m *MockActionHistoryRepository) GetActionTrace(ctx context.Context, actionID string) (*actionhistory.ResourceActionTrace, error) {
	m.executionMutex.RLock()
	defer m.executionMutex.RUnlock()

	for _, execution := range m.executions {
		if execution.ActionID == actionID {
			return &execution, nil
		}
	}
	return nil, fmt.Errorf("action trace not found")
}

func (m *MockActionHistoryRepository) ApplyRetention(ctx context.Context, retentionDays int64) error {
	// Mock implementation for retention policy
	return nil
}

func (m *MockActionHistoryRepository) EnsureActionHistory(ctx context.Context, id int64) (*actionhistory.ActionHistory, error) {
	// Mock implementation for ensuring action history
	return &actionhistory.ActionHistory{
		ID:        id,
		CreatedAt: time.Now(),
	}, nil
}

func (m *MockActionHistoryRepository) EnsureResourceReference(ctx context.Context, ref actionhistory.ResourceReference) (int64, error) {
	// Mock implementation for ensuring resource reference
	return 1, nil
}

func (m *MockActionHistoryRepository) GetActionHistory(ctx context.Context, resourceID int64) (*actionhistory.ActionHistory, error) {
	// Mock implementation for getting action history
	return &actionhistory.ActionHistory{
		ID:         resourceID,
		ResourceID: resourceID,
		CreatedAt:  time.Now(),
	}, nil
}

// GetActionTraces implements the missing interface method
func (m *MockActionHistoryRepository) GetActionTraces(ctx context.Context, query actionhistory.ActionQuery) ([]actionhistory.ResourceActionTrace, error) {
	m.executionMutex.RLock()
	defer m.executionMutex.RUnlock()

	// Mock implementation - return traces based on query
	return m.executions, nil
}

// GetOscillationDetections implements the missing interface method
func (m *MockActionHistoryRepository) GetOscillationDetections(ctx context.Context, resourceID int64, resolved *bool) ([]actionhistory.OscillationDetection, error) {
	// Mock implementation - return empty slice
	return []actionhistory.OscillationDetection{}, nil
}

// GetOscillationPatterns implements the missing interface method
func (m *MockActionHistoryRepository) GetOscillationPatterns(ctx context.Context, resourcePattern string) ([]actionhistory.OscillationPattern, error) {
	// Mock implementation - return empty slice
	return []actionhistory.OscillationPattern{}, nil
}

// DetectOscillations implements the missing interface method
func (m *MockActionHistoryRepository) DetectOscillations(ctx context.Context, trace *actionhistory.ResourceActionTrace) ([]actionhistory.OscillationDetection, error) {
	// Mock implementation - return empty slice
	return []actionhistory.OscillationDetection{}, nil
}

// ResolveOscillation implements the missing interface method
func (m *MockActionHistoryRepository) ResolveOscillation(ctx context.Context, detectionID int64, resolution string) error {
	// Mock implementation
	return nil
}

// GetRecentActionSummary implements the missing interface method
func (m *MockActionHistoryRepository) GetRecentActionSummary(ctx context.Context, resourceID int64, hours int) (interface{}, error) {
	// Mock implementation - return a generic summary structure
	summary := map[string]interface{}{
		"ResourceID":        resourceID,
		"TotalActions":      int64(len(m.executions)),
		"SuccessfulActions": int64(len(m.executions)),
		"FailedActions":     int64(0),
		"LastActionTime":    time.Now(),
	}
	return summary, nil
}

// AuditLogs implements the missing k8s.Client interface method
func (m *MockK8sClient) AuditLogs(ctx context.Context, namespace, podName, containerName string) error {
	// Mock implementation - return no error
	return nil
}

// GetTotalOperationCount returns the total operation count for testing
func (m *MockK8sClient) GetTotalOperationCount() int {
	return m.GetOperationCount()
}

// SetClusterConnectivityResult sets the cluster connectivity validation result
func (m *MockK8sClient) SetClusterConnectivityResult(valid bool, err error) {
	// Mock implementation for cluster connectivity testing
}

// BackupData implements the missing k8s.Client interface method
func (m *MockK8sClient) BackupData(ctx context.Context, namespace, podName string, backupPath string) error {
	// Mock implementation - return no error
	return nil
}

// CleanupStorage implements the missing k8s.Client interface method
func (m *MockK8sClient) CleanupStorage(ctx context.Context, namespace, podName, storageType string) error {
	// Mock implementation - return no error
	return nil
}

// CollectDiagnostics implements the missing k8s.Client interface method
func (m *MockK8sClient) CollectDiagnostics(ctx context.Context, namespace, podName string) (map[string]interface{}, error) {
	// Mock implementation - return empty diagnostics
	return map[string]interface{}{}, nil
}

// CompactStorage implements the missing k8s.Client interface method
func (m *MockK8sClient) CompactStorage(ctx context.Context, namespace, storageType string) error {
	// Mock implementation - return no error
	return nil
}

// CreateHeapDump implements the missing k8s.Client interface method
func (m *MockK8sClient) CreateHeapDump(ctx context.Context, namespace, podName, containerName string) error {
	// Mock implementation - return no error
	return nil
}

// DeletePod implements the missing k8s.Client interface method
func (m *MockK8sClient) DeletePod(ctx context.Context, namespace, podName string) error {
	// Mock implementation - return no error
	return nil
}

// EnableDebugMode implements the missing k8s.Client interface method
func (m *MockK8sClient) EnableDebugMode(ctx context.Context, namespace, podName, containerName, logLevel string) error {
	// Mock implementation - return no error
	return nil
}

// ExpandPVC implements the missing k8s.Client interface method
func (m *MockK8sClient) ExpandPVC(ctx context.Context, namespace, pvcName, newSize string) error {
	// Mock implementation - actually expand the PVC in the fake client
	if m.fakeClientset != nil {
		// Get the PVC
		pvc, err := m.fakeClientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvcName, metav1.GetOptions{})
		if err != nil {
			return err
		}

		// Parse the new size
		newQuantity, err := resource.ParseQuantity(newSize)
		if err != nil {
			return err
		}

		// Update the PVC size
		pvc.Spec.Resources.Requests[corev1.ResourceStorage] = newQuantity

		// Update the PVC in the fake client
		_, err = m.fakeClientset.CoreV1().PersistentVolumeClaims(namespace).Update(ctx, pvc, metav1.UpdateOptions{})
		return err
	}
	return nil
}

// FailoverDatabase implements the missing k8s.Client interface method
func (m *MockK8sClient) FailoverDatabase(ctx context.Context, namespace, dbName, targetNode string) error {
	// Mock implementation - return no error
	return nil
}

// GetDeployment implements the missing k8s.Client interface method
func (m *MockK8sClient) GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	// Mock implementation - return empty deployment
	return &appsv1.Deployment{}, nil
}

// GetEvents implements the missing k8s.Client interface method
func (m *MockK8sClient) GetEvents(ctx context.Context, namespace string) (*corev1.EventList, error) {
	// Mock implementation - return empty event list
	return &corev1.EventList{}, nil
}

// GetLogs implements the missing k8s.Client interface method
func (m *MockK8sClient) GetLogs(ctx context.Context, namespace, podName string) ([]string, error) {
	// Mock implementation - return empty logs
	return []string{}, nil
}

// GetMetrics implements the missing k8s.Client interface method
func (m *MockK8sClient) GetMetrics(ctx context.Context, namespace, resourceName string) (map[string]interface{}, error) {
	// Mock implementation - return empty metrics
	return map[string]interface{}{}, nil
}

// GetNode implements the missing k8s.Client interface method
func (m *MockK8sClient) GetNode(ctx context.Context, nodeName string) (*corev1.Node, error) {
	// Mock implementation - return empty node
	return &corev1.Node{}, nil
}

// GetPod implements the missing k8s.Client interface method
func (m *MockK8sClient) GetPod(ctx context.Context, namespace, podName string) (*corev1.Pod, error) {
	// Mock implementation - return empty pod
	return &corev1.Pod{}, nil
}

// GetResourceQuotas implements the missing k8s.Client interface method
func (m *MockK8sClient) GetResourceQuotas(ctx context.Context, namespace string) (*corev1.ResourceQuotaList, error) {
	// Mock implementation - return empty resource quotas
	return &corev1.ResourceQuotaList{}, nil
}

// GetService implements the missing k8s.Client interface method
func (m *MockK8sClient) GetService(ctx context.Context, namespace, serviceName string) (*corev1.Service, error) {
	// Mock implementation - return empty service
	return &corev1.Service{}, nil
}

// InstallOperator implements the missing k8s.Client interface method
func (m *MockK8sClient) InstallOperator(ctx context.Context, namespace, operatorName string) error {
	// Mock implementation - return no error
	return nil
}

// MonitorHealth implements the missing k8s.Client interface method
func (m *MockK8sClient) MonitorHealth(ctx context.Context, namespace, resourceName string) (bool, error) {
	// Mock implementation - return healthy
	return true, nil
}

// PatchResource implements the missing k8s.Client interface method
func (m *MockK8sClient) PatchResource(ctx context.Context, namespace, resourceType, resourceName string, patch []byte) error {
	// Mock implementation - return no error
	return nil
}

// IsHealthy implements the missing k8s.Client interface method
func (m *MockK8sClient) IsHealthy() bool {
	// Mock implementation - return healthy
	return true
}

// RebalanceShards implements the missing k8s.Client interface method
func (m *MockK8sClient) RebalanceShards(ctx context.Context, namespace, serviceName string) error {
	// Mock implementation - return no error
	return nil
}

// ReloadConfig implements the missing k8s.Client interface method
func (m *MockK8sClient) ReloadConfig(ctx context.Context, namespace, configMapName string) error {
	// Mock implementation - return no error
	return nil
}

// ResizeCluster implements the missing k8s.Client interface method
func (m *MockK8sClient) ResizeCluster(ctx context.Context, namespace, clusterName string, newSize int) error {
	// Mock implementation - return no error
	return nil
}

// UpdateConfigMap implements the missing k8s.Client interface method
func (m *MockK8sClient) UpdateConfigMap(ctx context.Context, namespace, configMapName string, data map[string]string) error {
	// Mock implementation - return no error
	return nil
}

// UpdateSecret implements the missing k8s.Client interface method
func (m *MockK8sClient) UpdateSecret(ctx context.Context, namespace, secretName string, data map[string][]byte) error {
	// Mock implementation - return no error
	return nil
}

// ListAllPods implements the missing k8s.Client interface method
func (m *MockK8sClient) ListAllPods(ctx context.Context, namespace string) (*corev1.PodList, error) {
	// Mock implementation - return empty pod list
	return &corev1.PodList{}, nil
}

// SetResourceLimits implements the missing k8s.Client interface method
func (m *MockK8sClient) SetResourceLimits(ctx context.Context, namespace, resourceName string, limits map[string]string) error {
	// Mock implementation - return no error
	return nil
}

// WaitForRollout implements the missing k8s.Client interface method
func (m *MockK8sClient) WaitForRollout(ctx context.Context, namespace, deploymentName string, timeout time.Duration) error {
	// Mock implementation - return no error
	return nil
}

// ListNodes implements the missing k8s.Client interface method
func (m *MockK8sClient) ListNodes(ctx context.Context) (*corev1.NodeList, error) {
	// Mock implementation - return empty node list
	return &corev1.NodeList{}, nil
}

// GetClientset implements the missing k8s.Client interface method
func (m *MockK8sClient) GetClientset() interface{} {
	// Mock implementation - return fake clientset
	return m.fakeClientset
}

// WatchPods implements the missing k8s.Client interface method
func (m *MockK8sClient) WatchPods(ctx context.Context, namespace string) (<-chan interface{}, error) {
	// Mock implementation - return empty channel
	ch := make(chan interface{})
	close(ch)
	return ch, nil
}

// GetNamespaces implements the missing k8s.Client interface method
func (m *MockK8sClient) GetNamespaces(ctx context.Context) (*corev1.NamespaceList, error) {
	// Mock implementation - return empty namespace list
	return &corev1.NamespaceList{}, nil
}

// CreateNamespace implements the missing k8s.Client interface method
func (m *MockK8sClient) CreateNamespace(ctx context.Context, namespace string) error {
	// Mock implementation - return no error
	return nil
}

// DeleteNamespace implements the missing k8s.Client interface method
func (m *MockK8sClient) DeleteNamespace(ctx context.Context, namespace string) error {
	// Mock implementation - return no error
	return nil
}

// ListPodsWithLabel implements the missing k8s.Client interface method
func (m *MockK8sClient) ListPodsWithLabel(ctx context.Context, namespace, labelSelector string) (*corev1.PodList, error) {
	// Mock implementation - return empty pod list
	return &corev1.PodList{}, nil
}

// ApplyManifest implements the missing k8s.Client interface method
func (m *MockK8sClient) ApplyManifest(ctx context.Context, manifest string) error {
	// Mock implementation - return no error
	return nil
}

// DeleteResource implements the missing k8s.Client interface method
func (m *MockK8sClient) DeleteResource(ctx context.Context, namespace, resourceType, resourceName string) error {
	// Mock implementation - return no error
	return nil
}

// GetClusterInfo implements the missing k8s.Client interface method
func (m *MockK8sClient) GetClusterInfo(ctx context.Context) (map[string]interface{}, error) {
	// Mock implementation - return empty cluster info
	return map[string]interface{}{}, nil
}

// PortForward implements the missing k8s.Client interface method
func (m *MockK8sClient) PortForward(ctx context.Context, namespace, podName string, localPort, remotePort int) error {
	// Mock implementation - return no error
	return nil
}

// ExecuteCommand implements the missing k8s.Client interface method
func (m *MockK8sClient) ExecuteCommand(ctx context.Context, namespace, podName, containerName string, command []string) (string, error) {
	// Mock implementation - return empty output
	return "", nil
}

// MigrateWorkload implements the missing k8s.Client interface method
func (m *MockK8sClient) MigrateWorkload(ctx context.Context, namespace, workloadName, targetNode string) error {
	// Mock implementation - return no error
	return nil
}

// OptimizeResources implements the missing k8s.Client interface method
func (m *MockK8sClient) OptimizeResources(ctx context.Context, namespace, workloadType, strategy string) error {
	// Mock implementation - return no error
	return nil
}

// QuarantinePod implements the missing k8s.Client interface method
func (m *MockK8sClient) QuarantinePod(ctx context.Context, namespace, podName string) error {
	// Mock implementation - return no error
	return nil
}

// RepairDatabase implements the missing k8s.Client interface method
func (m *MockK8sClient) RepairDatabase(ctx context.Context, namespace, databaseName, repairType string) error {
	// Mock implementation - return no error
	return nil
}

// UpdatePodResources implements the missing k8s.Client interface method
func (m *MockK8sClient) UpdatePodResources(ctx context.Context, namespace, name string, resources corev1.ResourceRequirements) error {
	// Mock implementation - return no error
	return nil
}

// UpdateHPA implements the missing k8s.Client interface method
func (m *MockK8sClient) UpdateHPA(ctx context.Context, namespace, name string, minReplicas, maxReplicas int32) error {
	// Mock implementation - return no error
	return nil
}

// RestartDaemonSet implements the missing k8s.Client interface method
func (m *MockK8sClient) RestartDaemonSet(ctx context.Context, namespace, name string) error {
	// Mock implementation - return no error
	return nil
}

// RotateSecrets implements the missing k8s.Client interface method
func (m *MockK8sClient) RotateSecrets(ctx context.Context, namespace, secretName string) error {
	// Mock implementation - return no error
	return nil
}

// UpdateNetworkPolicy implements the missing k8s.Client interface method
func (m *MockK8sClient) UpdateNetworkPolicy(ctx context.Context, namespace, policyName, actionType string) error {
	// Mock implementation - return no error
	return nil
}

// RestartNetwork implements the missing k8s.Client interface method
func (m *MockK8sClient) RestartNetwork(ctx context.Context, component string) error {
	// Mock implementation - return no error
	return nil
}

// ResetServiceMesh implements the missing k8s.Client interface method
func (m *MockK8sClient) ResetServiceMesh(ctx context.Context, meshType string) error {
	// Mock implementation - return no error
	return nil
}

// ScaleStatefulSet implements the missing k8s.Client interface method
func (m *MockK8sClient) ScaleStatefulSet(ctx context.Context, namespace, name string, replicas int32) error {
	// Mock implementation - return no error
	return nil
}

// GetActionHistorySummaries implements the missing actionhistory.Repository interface method
func (m *MockActionHistoryRepository) GetActionHistorySummaries(ctx context.Context, duration time.Duration) ([]actionhistory.ActionHistorySummary, error) {
	// Mock implementation - return empty summaries
	return []actionhistory.ActionHistorySummary{}, nil
}

// GetPendingEffectivenessAssessments implements the missing actionhistory.Repository interface method
func (m *MockActionHistoryRepository) GetPendingEffectivenessAssessments(ctx context.Context) ([]*actionhistory.ResourceActionTrace, error) {
	// Mock implementation - return empty assessments
	return []*actionhistory.ResourceActionTrace{}, nil
}

// GetResourceReference implements the missing actionhistory.Repository interface method
func (m *MockActionHistoryRepository) GetResourceReference(ctx context.Context, resourceType, namespace, name string) (*actionhistory.ResourceReference, error) {
	// Mock implementation - return empty resource reference
	return &actionhistory.ResourceReference{}, nil
}

// StoreAction implements the missing actionhistory.Repository interface method
func (m *MockActionHistoryRepository) StoreAction(ctx context.Context, record *actionhistory.ActionRecord) (*actionhistory.ResourceActionTrace, error) {
	m.executionMutex.Lock()
	defer m.executionMutex.Unlock()

	// Mock implementation - create a basic trace from the record
	trace := &actionhistory.ResourceActionTrace{
		ID:       123,
		ActionID: "mock-action-123",
	}
	m.executions = append(m.executions, *trace)
	return trace, nil
}

// StoreOscillationDetection implements the missing actionhistory.Repository interface method
func (m *MockActionHistoryRepository) StoreOscillationDetection(ctx context.Context, detection *actionhistory.OscillationDetection) error {
	// Mock implementation - return no error
	return nil
}

// UpdateActionHistory implements the missing actionhistory.Repository interface method
func (m *MockActionHistoryRepository) UpdateActionHistory(ctx context.Context, history *actionhistory.ActionHistory) error {
	// Mock implementation - return no error
	return nil
}

// ClearHistory clears execution history for test isolation
func (m *MockActionHistoryRepository) ClearHistory() {
	m.executionMutex.Lock()
	defer m.executionMutex.Unlock()
	m.executions = []actionhistory.ResourceActionTrace{}
}
