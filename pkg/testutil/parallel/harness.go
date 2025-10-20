// Package parallel provides test utilities for testing parallel execution and concurrency control
package parallel

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ExecutionHarness provides controlled parallel execution testing with concurrency limits,
// task tracking, and timing measurement. Use this to test workflow parallel execution,
// semaphore-based concurrency control, and coordinated task execution.
//
// Example:
//
//	harness := parallel.NewExecutionHarness(3) // Max 3 concurrent tasks
//	for i := 0; i < 10; i++ {
//	    harness.ExecuteTask(ctx, fmt.Sprintf("task-%d", i), 100*time.Millisecond)
//	}
//	Expect(harness.WaitForAllTasks(ctx, 10)).To(Succeed())
//	Expect(harness.GetMaxConcurrency()).To(BeNumerically("<=", 3))
type ExecutionHarness struct {
	maxConcurrency int
	activeTasks    chan struct{}
	completedTasks sync.Map // map[string]time.Duration
	taskTimings    sync.Map // map[string]time.Time (start time)
	startTime      time.Time
	mu             sync.RWMutex
	activeCurrent  int
	activeMax      int
}

// NewExecutionHarness creates a harness with specified maximum concurrency
func NewExecutionHarness(maxConcurrency int) *ExecutionHarness {
	return &ExecutionHarness{
		maxConcurrency: maxConcurrency,
		activeTasks:    make(chan struct{}, maxConcurrency),
		startTime:      time.Now(),
	}
}

// ExecuteTask simulates task execution with concurrency control.
// Blocks if maxConcurrency is reached, tracking timing and concurrency metrics.
func (h *ExecutionHarness) ExecuteTask(
	ctx context.Context,
	taskID string,
	duration time.Duration,
) error {
	// Record start time
	startTime := time.Since(h.startTime)
	h.taskTimings.Store(taskID, time.Now())

	// Acquire semaphore slot (blocks if at max concurrency)
	select {
	case h.activeTasks <- struct{}{}:
		h.updateActiveConcurrency(1)
		defer func() {
			<-h.activeTasks
			h.updateActiveConcurrency(-1)
		}()
	case <-ctx.Done():
		return fmt.Errorf("task %s: context cancelled while waiting for slot: %w", taskID, ctx.Err())
	}

	// Simulate task execution
	select {
	case <-time.After(duration):
		elapsed := time.Since(h.startTime) - startTime
		h.completedTasks.Store(taskID, elapsed)
		return nil
	case <-ctx.Done():
		return fmt.Errorf("task %s: context cancelled during execution: %w", taskID, ctx.Err())
	}
}

// updateActiveConcurrency updates the current and maximum active task counts
func (h *ExecutionHarness) updateActiveConcurrency(delta int) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.activeCurrent += delta
	if h.activeCurrent > h.activeMax {
		h.activeMax = h.activeCurrent
	}
}

// WaitForAllTasks blocks until expectedCount tasks complete or timeout occurs
func (h *ExecutionHarness) WaitForAllTasks(
	ctx context.Context,
	expectedCount int,
) error {
	deadline := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			count := h.GetCompletedCount()
			return fmt.Errorf("timeout waiting for tasks: %d/%d completed", count, expectedCount)
		case <-ticker.C:
			if h.GetCompletedCount() >= expectedCount {
				return nil
			}
		case <-ctx.Done():
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		}
	}
}

// GetCompletedCount returns the number of completed tasks
func (h *ExecutionHarness) GetCompletedCount() int {
	count := 0
	h.completedTasks.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// GetMaxConcurrency returns the maximum concurrent tasks observed
func (h *ExecutionHarness) GetMaxConcurrency() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.activeMax
}

// GetCurrentConcurrency returns the current number of active tasks
func (h *ExecutionHarness) GetCurrentConcurrency() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.activeCurrent
}

// GetTaskTiming returns the elapsed time for a completed task
func (h *ExecutionHarness) GetTaskTiming(taskID string) (time.Duration, bool) {
	val, ok := h.completedTasks.Load(taskID)
	if !ok {
		return 0, false
	}
	return val.(time.Duration), true
}

// VerifyConcurrencyLimit checks that max concurrency was never exceeded
func (h *ExecutionHarness) VerifyConcurrencyLimit() bool {
	return h.GetMaxConcurrency() <= h.maxConcurrency
}

// GetTaskTimings returns all task timings as a map
func (h *ExecutionHarness) GetTaskTimings() map[string]time.Duration {
	timings := make(map[string]time.Duration)
	h.completedTasks.Range(func(key, value interface{}) bool {
		timings[key.(string)] = value.(time.Duration)
		return true
	})
	return timings
}

// DependencyGraph represents a task dependency graph for testing dependency resolution
type DependencyGraph struct {
	nodes map[string]*GraphNode
	mu    sync.RWMutex
}

// GraphNode represents a task node in the dependency graph
type GraphNode struct {
	ID            string
	Dependencies  []string
	Status        string // "pending", "running", "completed", "failed"
	StartTime     time.Time
	CompletedTime time.Time
	mu            sync.RWMutex
}

// NewDependencyGraph creates a new dependency graph for testing
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		nodes: make(map[string]*GraphNode),
	}
}

// AddNode adds a node to the dependency graph
func (g *DependencyGraph) AddNode(id string, dependencies []string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.nodes[id] = &GraphNode{
		ID:           id,
		Dependencies: dependencies,
		Status:       "pending",
	}
}

// CanExecute checks if a node's dependencies are all completed
func (g *DependencyGraph) CanExecute(nodeID string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	node, exists := g.nodes[nodeID]
	if !exists {
		return false
	}

	node.mu.RLock()
	defer node.mu.RUnlock()

	if node.Status != "pending" {
		return false
	}

	// Check all dependencies are completed
	for _, depID := range node.Dependencies {
		depNode, exists := g.nodes[depID]
		if !exists {
			return false
		}

		depNode.mu.RLock()
		depStatus := depNode.Status
		depNode.mu.RUnlock()

		if depStatus != "completed" {
			return false
		}
	}

	return true
}

// MarkRunning marks a node as running
func (g *DependencyGraph) MarkRunning(nodeID string) error {
	g.mu.RLock()
	node, exists := g.nodes[nodeID]
	g.mu.RUnlock()

	if !exists {
		return fmt.Errorf("node %s not found", nodeID)
	}

	node.mu.Lock()
	defer node.mu.Unlock()

	if node.Status != "pending" {
		return fmt.Errorf("node %s already %s", nodeID, node.Status)
	}

	node.Status = "running"
	node.StartTime = time.Now()
	return nil
}

// MarkCompleted marks a node as completed
func (g *DependencyGraph) MarkCompleted(nodeID string) error {
	g.mu.RLock()
	node, exists := g.nodes[nodeID]
	g.mu.RUnlock()

	if !exists {
		return fmt.Errorf("node %s not found", nodeID)
	}

	node.mu.Lock()
	defer node.mu.Unlock()

	if node.Status != "running" {
		return fmt.Errorf("node %s not running (status: %s)", nodeID, node.Status)
	}

	node.Status = "completed"
	node.CompletedTime = time.Now()
	return nil
}

// MarkFailed marks a node as failed
func (g *DependencyGraph) MarkFailed(nodeID string) error {
	g.mu.RLock()
	node, exists := g.nodes[nodeID]
	g.mu.RUnlock()

	if !exists {
		return fmt.Errorf("node %s not found", nodeID)
	}

	node.mu.Lock()
	defer node.mu.Unlock()

	node.Status = "failed"
	return nil
}

// GetStatus returns the status of a node
func (g *DependencyGraph) GetStatus(nodeID string) (string, error) {
	g.mu.RLock()
	node, exists := g.nodes[nodeID]
	g.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("node %s not found", nodeID)
	}

	node.mu.RLock()
	defer node.mu.RUnlock()

	return node.Status, nil
}

// GetReadyNodes returns all nodes that can be executed
func (g *DependencyGraph) GetReadyNodes() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var ready []string
	for id := range g.nodes {
		if g.CanExecute(id) {
			ready = append(ready, id)
		}
	}
	return ready
}

// DetectCycle detects if there's a cycle in the dependency graph
func (g *DependencyGraph) DetectCycle() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	for id := range g.nodes {
		if g.detectCycleUtil(id, visited, recStack) {
			return true
		}
	}
	return false
}

func (g *DependencyGraph) detectCycleUtil(nodeID string, visited, recStack map[string]bool) bool {
	if recStack[nodeID] {
		return true
	}

	if visited[nodeID] {
		return false
	}

	visited[nodeID] = true
	recStack[nodeID] = true

	node := g.nodes[nodeID]
	for _, depID := range node.Dependencies {
		if g.detectCycleUtil(depID, visited, recStack) {
			return true
		}
	}

	recStack[nodeID] = false
	return false
}

// TopologicalSort returns nodes in topologically sorted order
func (g *DependencyGraph) TopologicalSort() ([]string, error) {
	if g.DetectCycle() {
		return nil, fmt.Errorf("cycle detected in dependency graph")
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	visited := make(map[string]bool)
	var result []string

	var visit func(string)
	visit = func(nodeID string) {
		if visited[nodeID] {
			return
		}
		visited[nodeID] = true

		node := g.nodes[nodeID]
		for _, depID := range node.Dependencies {
			visit(depID)
		}

		result = append(result, nodeID)
	}

	for id := range g.nodes {
		visit(id)
	}

	return result, nil
}
