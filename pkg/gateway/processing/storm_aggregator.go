/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package processing

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// StormAggregator aggregates alerts during storm windows
//
// Business Requirement: BR-GATEWAY-016 - Storm aggregation
//
// When a storm is detected (BR-GATEWAY-015), instead of creating individual
// RemediationRequest CRDs for each alert, StormAggregator collects alerts
// for a fixed window (1 minute) and creates a single aggregated CRD.
//
// Benefits:
// - Reduces CRD count by 10-50x during storms
// - AI service receives single aggregated analysis request
// - Downstream remediation is coordinated, not parallel
// - Prevents overwhelming Kubernetes API and downstream services
//
// Example:
// - Without aggregation: 50 pod crashes → 50 CRDs → 50 remediation workflows
// - With aggregation: 50 pod crashes → 1 CRD → 1 coordinated remediation
//
// Algorithm:
// 1. First alert in storm: Create aggregation window in Redis (1-minute TTL)
// 2. Subsequent alerts: Add resource to window (no CRD creation yet)
// 3. After 1 minute: Create single RemediationRequest CRD with all resources
//
// Redis keys:
// - alert:storm:aggregate:<alertname> (stores aggregation window ID)
// - alert:storm:resources:<window-id> (sorted set of affected resources)
// - alert:storm:metadata:<window-id> (first signal metadata for CRD creation)
type StormAggregator struct {
	redisClient    *redis.Client
	windowDuration time.Duration // Default: 1 minute
}

// NewStormAggregator creates a new storm aggregator with default window duration
//
// Default window: 1 minute
// Rationale: Balances early detection (don't wait too long) with aggregation
// efficiency (collect enough alerts to make aggregation worthwhile)
func NewStormAggregator(redisClient *redis.Client) *StormAggregator {
	return NewStormAggregatorWithWindow(redisClient, 0) // Use default
}

// NewStormAggregatorWithWindow creates a storm aggregator with custom window duration
//
// Parameters:
// - redisClient: Redis client for aggregation tracking
// - windowDuration: Aggregation window duration (0 = use default 1 minute)
//
// Use cases:
// - Production: Use default (0) for 1-minute windows
// - Testing: Use 5*time.Second for fast integration tests
//
// Example:
//
//	// Production
//	aggregator := NewStormAggregator(redisClient)
//
//	// Testing (5-second window)
//	aggregator := NewStormAggregatorWithWindow(redisClient, 5*time.Second)
func NewStormAggregatorWithWindow(redisClient *redis.Client, windowDuration time.Duration) *StormAggregator {
	if windowDuration == 0 {
		windowDuration = 1 * time.Minute // Production default
	}
	return &StormAggregator{
		redisClient:    redisClient,
		windowDuration: windowDuration,
	}
}

// ShouldAggregate checks if an alert should be added to an existing aggregation window
//
// Call this after storm detection confirms a storm is occurring.
//
// Returns:
// - bool: true if aggregation window exists for this alertname
// - string: window ID (for adding resources)
// - error: Redis errors
//
// Usage:
//
//	if isStorm {
//	    shouldAggregate, windowID, err := aggregator.ShouldAggregate(ctx, signal)
//	    if shouldAggregate {
//	        aggregator.AddResource(ctx, windowID, signal)
//	        return // Don't create individual CRD
//	    }
//	}
func (a *StormAggregator) ShouldAggregate(ctx context.Context, signal *types.NormalizedSignal) (bool, string, error) {
	key := fmt.Sprintf("alert:storm:aggregate:%s", signal.AlertName)

	windowID, err := a.redisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		// No aggregation window exists
		return false, "", nil
	} else if err != nil {
		return false, "", fmt.Errorf("failed to check aggregation window: %w", err)
	}

	return true, windowID, nil
}

// StartAggregation creates a new aggregation window for the alert
//
// Call this when:
// - Storm detected
// - No existing aggregation window for this alertname
//
// Creates:
// - Aggregation window entry in Redis (1-minute TTL)
// - First resource entry in sorted set
// - Metadata entry for CRD creation
//
// Returns:
// - string: window ID (timestamp-based unique identifier)
// - error: Redis errors
//
// Example window ID: "PodCrashed-1696800000"
func (a *StormAggregator) StartAggregation(ctx context.Context, signal *types.NormalizedSignal, stormMetadata *StormMetadata) (string, error) {
	windowID := fmt.Sprintf("%s-%d", signal.AlertName, time.Now().Unix())
	windowKey := fmt.Sprintf("alert:storm:aggregate:%s", signal.AlertName)

	// Store window ID with TTL (1 minute)
	if err := a.redisClient.Set(ctx, windowKey, windowID, a.windowDuration).Err(); err != nil {
		return "", fmt.Errorf("failed to start aggregation window: %w", err)
	}

	// Store first signal metadata for later CRD creation
	if err := a.storeSignalMetadata(ctx, windowID, signal, stormMetadata); err != nil {
		return "", fmt.Errorf("failed to store signal metadata: %w", err)
	}

	// Add first resource
	if err := a.AddResource(ctx, windowID, signal); err != nil {
		return "", fmt.Errorf("failed to add first resource: %w", err)
	}

	return windowID, nil
}

// AddResource adds a resource to an existing aggregation window
//
// Call this for subsequent alerts during a storm (after the aggregation
// window has been created by StartAggregation).
//
// Resources are stored in a Redis sorted set with timestamp as score.
// This allows:
// - Tracking when each resource was added
// - Retrieving resources in chronological order
// - Automatically expiring old entries
//
// Resource identifier format: "namespace:Kind:name"
// Example: "prod-api:Pod:payment-api-789"
func (a *StormAggregator) AddResource(ctx context.Context, windowID string, signal *types.NormalizedSignal) error {
	key := fmt.Sprintf("alert:storm:resources:%s", windowID)
	resourceID := signal.Resource.String()

	// Add to sorted set (score = timestamp)
	if err := a.redisClient.ZAdd(ctx, key, &redis.Z{
		Score:  float64(time.Now().Unix()),
		Member: resourceID,
	}).Err(); err != nil {
		return fmt.Errorf("failed to add resource to aggregation: %w", err)
	}

	// Set TTL (2x window duration to allow retrieval after window closes)
	a.redisClient.Expire(ctx, key, 2*a.windowDuration)

	return nil
}

// GetAggregatedResources retrieves all resources in the aggregation window
//
// Call this after the aggregation window expires (1 minute) to retrieve
// all resources that should be included in the aggregated CRD.
//
// Returns resources in chronological order (sorted by timestamp).
//
// Returns:
// - []string: List of resource identifiers (format: "namespace:Kind:name")
// - error: Redis errors
func (a *StormAggregator) GetAggregatedResources(ctx context.Context, windowID string) ([]string, error) {
	key := fmt.Sprintf("alert:storm:resources:%s", windowID)

	resources, err := a.redisClient.ZRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve aggregated resources: %w", err)
	}

	return resources, nil
}

// GetSignalMetadata retrieves the stored signal metadata for CRD creation
//
// Call this after the aggregation window expires to retrieve the original
// signal metadata needed to create the aggregated RemediationRequest CRD.
//
// The metadata stored includes:
// - AlertName, Severity, Namespace
// - Labels, Annotations
// - Storm metadata (type, window, initial count)
//
// Returns:
// - *types.NormalizedSignal: Reconstructed signal with original metadata
// - *StormMetadata: Storm detection metadata
// - error: Redis errors or JSON parsing errors
func (a *StormAggregator) GetSignalMetadata(ctx context.Context, windowID string) (*types.NormalizedSignal, *StormMetadata, error) {
	metadataKey := fmt.Sprintf("alert:storm:metadata:%s", windowID)

	// Retrieve stored metadata
	data, err := a.redisClient.HGetAll(ctx, metadataKey).Result()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve signal metadata: %w", err)
	}

	if len(data) == 0 {
		return nil, nil, fmt.Errorf("no metadata found for window %s", windowID)
	}

	// Reconstruct signal from stored data
	signal := &types.NormalizedSignal{
		AlertName:   data["alertname"],
		Severity:    data["severity"],
		Namespace:   data["namespace"],
		Fingerprint: data["fingerprint"],
		SourceType:  data["source_type"],
		Source:      data["source"],
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
	}

	// Parse timestamps (required for CRD creation)
	if firingTimeStr, ok := data["firing_time"]; ok && firingTimeStr != "" {
		if t, err := time.Parse(time.RFC3339Nano, firingTimeStr); err == nil {
			signal.FiringTime = t
		}
	}
	if receivedTimeStr, ok := data["received_time"]; ok && receivedTimeStr != "" {
		if t, err := time.Parse(time.RFC3339Nano, receivedTimeStr); err == nil {
			signal.ReceivedTime = t
		}
	}

	// Reconstruct resource
	if resourceNamespace, ok := data["resource_namespace"]; ok {
		signal.Resource = types.ResourceIdentifier{
			Namespace: resourceNamespace,
			Kind:      data["resource_kind"],
			Name:      data["resource_name"],
		}
	}

	// Reconstruct storm metadata
	stormMetadata := &StormMetadata{
		StormType:  data["storm_type"],
		Window:     data["storm_window"],
		AlertCount: 1, // Will be updated with actual count
	}

	return signal, stormMetadata, nil
}

// storeSignalMetadata stores signal metadata for later CRD creation
//
// Internal method called by StartAggregation.
// Stores signal data in Redis hash for later retrieval.
func (a *StormAggregator) storeSignalMetadata(ctx context.Context, windowID string, signal *types.NormalizedSignal, stormMetadata *StormMetadata) error {
	metadataKey := fmt.Sprintf("alert:storm:metadata:%s", windowID)

	// Store signal fields as hash
	data := map[string]interface{}{
		"alertname":    signal.AlertName,
		"severity":     signal.Severity,
		"namespace":    signal.Namespace,
		"fingerprint":  signal.Fingerprint,
		"source_type":  signal.SourceType,
		"source":       signal.Source,
		"storm_type":   stormMetadata.StormType,
		"storm_window": stormMetadata.Window,
		// Timestamp fields (required for CRD creation)
		"firing_time":   signal.FiringTime.Format(time.RFC3339Nano),
		"received_time": signal.ReceivedTime.Format(time.RFC3339Nano),
	}

	// Store resource information
	if signal.Resource.Namespace != "" {
		data["resource_namespace"] = signal.Resource.Namespace
		data["resource_kind"] = signal.Resource.Kind
		data["resource_name"] = signal.Resource.Name
	}

	if err := a.redisClient.HSet(ctx, metadataKey, data).Err(); err != nil {
		return fmt.Errorf("failed to store metadata: %w", err)
	}

	// Set TTL (2x window duration to allow retrieval after window closes)
	a.redisClient.Expire(ctx, metadataKey, 2*a.windowDuration)

	return nil
}

// GetWindowDuration returns the configured aggregation window duration
//
// This allows the server to wait the appropriate amount of time before
// creating the aggregated CRD.
func (a *StormAggregator) GetWindowDuration() time.Duration {
	return a.windowDuration
}

// GetResourceCount returns the current count of resources in the aggregation window
//
// Useful for metrics and logging during aggregation.
//
// Returns:
// - int: Number of resources currently aggregated
// - error: Redis errors
func (a *StormAggregator) GetResourceCount(ctx context.Context, windowID string) (int, error) {
	key := fmt.Sprintf("alert:storm:resources:%s", windowID)

	count, err := a.redisClient.ZCard(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get resource count: %w", err)
	}

	return int(count), nil
}

// AggregateOrCreate determines if a signal should be aggregated or if a new storm CRD should be created
//
// Returns:
// - bool: true if signal was aggregated into existing storm, false if new storm CRD should be created
// - string: storm window ID (for aggregation) or empty (for new CRD creation)
// - error: Redis errors or aggregation failures
func (a *StormAggregator) AggregateOrCreate(ctx context.Context, signal *types.NormalizedSignal) (bool, string, error) {
	// Check if there's an active storm window for this signal
	shouldAggregate, windowID, err := a.ShouldAggregate(ctx, signal)
	if err != nil {
		return false, "", fmt.Errorf("failed to check aggregation status: %w", err)
	}

	if shouldAggregate {
		// Add resource to existing storm window
		err = a.AddResource(ctx, windowID, signal)
		if err != nil {
			return false, "", fmt.Errorf("failed to add resource to storm window: %w", err)
		}
		return true, windowID, nil
	}

	// No active storm window - signal new CRD should be created
	return false, "", nil
}

// BufferFirstAlert buffers alert and returns whether aggregation should start (DD-GATEWAY-008)
//
// Business Requirement: BR-GATEWAY-016 - Buffer first N alerts before creating aggregation window
//
// Returns:
// - int: current buffer size
// - bool: shouldAggregate (true if threshold reached)
// - error: Redis errors
func (a *StormAggregator) BufferFirstAlert(ctx context.Context, signal *types.NormalizedSignal) (int, bool, error) {
	// Redis key for buffering: alert:buffer:<namespace>:<alertname>
	bufferKey := fmt.Sprintf("alert:buffer:%s:%s", signal.Namespace, signal.AlertName)

	// Add signal to buffer (Redis list)
	resourceID := signal.Resource.String()
	if err := a.redisClient.RPush(ctx, bufferKey, resourceID).Err(); err != nil {
		return 0, false, fmt.Errorf("failed to buffer alert: %w", err)
	}

	// Set TTL on buffer (2x window duration)
	a.redisClient.Expire(ctx, bufferKey, 2*a.windowDuration)

	// Get current buffer size
	bufferSize, err := a.redisClient.LLen(ctx, bufferKey).Result()
	if err != nil {
		return 0, false, fmt.Errorf("failed to get buffer size: %w", err)
	}

	// Default threshold: 5 alerts
	threshold := 5
	shouldAggregate := int(bufferSize) >= threshold

	return int(bufferSize), shouldAggregate, nil
}
