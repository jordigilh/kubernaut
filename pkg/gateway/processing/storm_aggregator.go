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
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/gateway/types"
	"github.com/redis/go-redis/v9"
)

// StormAggregator aggregates alerts during storm windows
//
// Business Requirement: BR-GATEWAY-016 - Storm aggregation
// Design Decision: DD-GATEWAY-008 - Buffered first-alert aggregation with sliding window
//
// When a storm is detected (BR-GATEWAY-015), instead of creating individual
// RemediationRequest CRDs for each alert, StormAggregator collects alerts
// for a configurable window and creates a single aggregated CRD.
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
// Algorithm (DD-GATEWAY-008):
// 1. First N alerts: Buffer in Redis (no window creation yet)
// 2. Nth alert (threshold): Create aggregation window with sliding timer
// 3. Subsequent alerts: Add to window + reset timer (sliding window)
// 4. After inactivity timeout OR max duration: Create single CRD
//
// Redis keys:
// - alert:buffer:<namespace>:<alertname> (list of buffered signals before threshold)
// - alert:storm:aggregate:<namespace>:<alertname> (stores aggregation window ID, BR-GATEWAY-011)
// - alert:storm:resources:<window-id> (sorted set of affected resources)
// - alert:storm:metadata:<window-id> (first signal metadata for CRD creation)
type StormAggregator struct {
	redisClient        *redis.Client
	windowDuration     time.Duration  // Default: 1 minute (inactivity timeout)
	bufferThreshold    int            // Alerts before creating window (default: 5)
	maxWindowDuration  time.Duration  // Max window duration (default: 5 minutes)
	defaultMaxSize     int            // Default namespace buffer size (default: 1000)
	globalMaxSize      int            // Global buffer limit (default: 5000)
	perNamespaceLimits map[string]int // Per-namespace buffer size overrides (BR-GATEWAY-011)
	samplingThreshold  float64        // Utilization to trigger sampling (default: 0.95)
	samplingRate       float64        // Sample rate when threshold reached (default: 0.5)
}

// NewStormAggregatorWithConfig creates a storm aggregator with full DD-GATEWAY-008 configuration
//
// Parameters:
// - redisClient: Redis client for aggregation tracking
// - bufferThreshold: Number of alerts to buffer before creating window (0 = use default 5)
// - inactivityTimeout: Sliding window timeout (0 = use default 60s)
// - maxWindowDuration: Maximum window duration (0 = use default 5m)
// - defaultMaxSize: Default namespace buffer size (0 = use default 1000)
// - globalMaxSize: Global buffer limit (0 = use default 5000)
// - perNamespaceLimits: Per-namespace buffer size overrides (nil = no overrides)
// - samplingThreshold: Utilization to trigger sampling (0 = use default 0.95)
// - samplingRate: Sample rate when threshold reached (0 = use default 0.5)
//
// Use cases:
// - Production: Load from config/gateway.yaml
// - Testing: Override specific values for test scenarios
//
// Example:
//
//	// Production (from config)
//	aggregator := NewStormAggregatorWithConfig(
//	    redisClient,
//	    cfg.Processing.Storm.BufferThreshold,
//	    cfg.Processing.Storm.InactivityTimeout,
//	    cfg.Processing.Storm.MaxWindowDuration,
//	    cfg.Processing.Storm.DefaultMaxSize,
//	    cfg.Processing.Storm.GlobalMaxSize,
//	    cfg.Processing.Storm.PerNamespaceLimits,
//	    cfg.Processing.Storm.SamplingThreshold,
//	    cfg.Processing.Storm.SamplingRate,
//	)
//
//	// Testing (fast window, no namespace overrides)
//	aggregator := NewStormAggregatorWithConfig(redisClient, 3, 5*time.Second, 30*time.Second, 100, 500, nil, 0.95, 0.5)
func NewStormAggregatorWithConfig(
	redisClient *redis.Client,
	bufferThreshold int,
	inactivityTimeout time.Duration,
	maxWindowDuration time.Duration,
	defaultMaxSize int,
	globalMaxSize int,
	perNamespaceLimits map[string]int,
	samplingThreshold float64,
	samplingRate float64,
) *StormAggregator {
	// Apply defaults for zero values
	if bufferThreshold == 0 {
		bufferThreshold = 5
	}
	if inactivityTimeout == 0 {
		inactivityTimeout = 60 * time.Second
	}
	if maxWindowDuration == 0 {
		maxWindowDuration = 5 * time.Minute
	}
	if defaultMaxSize == 0 {
		defaultMaxSize = 1000
	}
	if globalMaxSize == 0 {
		globalMaxSize = 5000
	}
	if samplingThreshold == 0 {
		samplingThreshold = 0.95
	}
	if samplingRate == 0 {
		samplingRate = 0.5
	}
	if perNamespaceLimits == nil {
		perNamespaceLimits = make(map[string]int)
	}

	return &StormAggregator{
		redisClient:        redisClient,
		windowDuration:     inactivityTimeout,
		bufferThreshold:    bufferThreshold,
		maxWindowDuration:  maxWindowDuration,
		defaultMaxSize:     defaultMaxSize,
		globalMaxSize:      globalMaxSize,
		perNamespaceLimits: perNamespaceLimits,
		samplingThreshold:  samplingThreshold,
		samplingRate:       samplingRate,
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
	// DD-GATEWAY-008 + BR-GATEWAY-011: Include namespace for multi-tenant isolation
	key := fmt.Sprintf("alert:storm:aggregate:%s:%s", signal.Namespace, signal.AlertName)

	windowID, err := a.redisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		// No aggregation window exists
		return false, "", nil
	} else if err != nil {
		return false, "", fmt.Errorf("failed to check aggregation window: %w", err)
	}

	return true, windowID, nil
}

// StartAggregation creates a new aggregation window for the alert (DD-GATEWAY-008 enhanced)
//
// Call this when:
// - Storm detected
// - No existing aggregation window for this alertname
//
// DD-GATEWAY-008 Behavior:
// - BEFORE threshold: Buffer alert in Redis (no window creation)
// - AT threshold: Create aggregation window + move buffered alerts to window
//
// Creates (when threshold reached):
// - Aggregation window entry in Redis (sliding window with inactivity timeout)
// - Resource entries in sorted set (from buffer + current alert)
// - Metadata entry for CRD creation
//
// Returns:
// - string: window ID (empty if buffering, timestamp-based if window created)
// - error: Redis errors
//
// Example window ID: "PodCrashed-1696800000"
//
// Business Requirements:
// - BR-GATEWAY-016: Buffer first N alerts before creating window
// - BR-GATEWAY-008: Sliding window with inactivity timeout
func (a *StormAggregator) StartAggregation(ctx context.Context, signal *types.NormalizedSignal, stormMetadata *StormMetadata) (string, error) {
	// DD-GATEWAY-008: Buffer first N alerts before creating window
	_, shouldAggregate, err := a.BufferFirstAlert(ctx, signal)
	if err != nil {
		return "", fmt.Errorf("failed to buffer alert: %w", err)
	}

	// If threshold not reached, just buffer (no window creation)
	if !shouldAggregate {
		// Return empty windowID to indicate buffering (not aggregating yet)
		return "", nil
	}

	// Threshold reached: Create aggregation window
	currentTime := time.Now()
	// DD-GATEWAY-008 + BR-GATEWAY-011: Include namespace for multi-tenant isolation
	windowID := fmt.Sprintf("%s:%s-%d", signal.Namespace, signal.AlertName, currentTime.Unix())
	windowKey := fmt.Sprintf("alert:storm:aggregate:%s:%s", signal.Namespace, signal.AlertName)

	// Store window ID with TTL (BR-GATEWAY-008: sliding window with inactivity timeout)
	// Use windowDuration (inactivityTimeout) for consistent sliding window behavior
	// Safety: AddResource checks maxWindowDuration via IsWindowExpired (defense-in-depth)
	if err := a.redisClient.Set(ctx, windowKey, windowID, a.windowDuration).Err(); err != nil {
		return "", fmt.Errorf("failed to start aggregation window: %w", err)
	}

	// DD-GATEWAY-008: Populate sliding window tracking fields
	stormMetadata.AbsoluteStartTime = currentTime
	stormMetadata.LastActivity = currentTime

	// Store first signal metadata for later CRD creation
	if err := a.storeSignalMetadata(ctx, windowID, signal, stormMetadata); err != nil {
		return "", fmt.Errorf("failed to store signal metadata: %w", err)
	}

	// Move all buffered alerts to aggregation window
	bufferKey := fmt.Sprintf("alert:buffer:%s:%s", signal.Namespace, signal.AlertName)
	bufferedAlerts, err := a.redisClient.LRange(ctx, bufferKey, 0, -1).Result()
	if err != nil {
		return "", fmt.Errorf("failed to retrieve buffered alerts: %w", err)
	}

	// Add all buffered resources to window
	for _, resourceID := range bufferedAlerts {
		key := fmt.Sprintf("alert:storm:resources:%s", windowID)
		if err := a.redisClient.ZAdd(ctx, key, redis.Z{
			Score:  float64(time.Now().Unix()),
			Member: resourceID,
		}).Err(); err != nil {
			return "", fmt.Errorf("failed to add buffered resource to window: %w", err)
		}
	}

	// Clear buffer (alerts now in window)
	a.redisClient.Del(ctx, bufferKey)

	// Set TTL on resources (2x window duration for retrieval after close)
	resourceKey := fmt.Sprintf("alert:storm:resources:%s", windowID)
	a.redisClient.Expire(ctx, resourceKey, 2*a.windowDuration)

	return windowID, nil
}

// AddResource adds a resource to an existing aggregation window (DD-GATEWAY-008 enhanced)
//
// Call this for subsequent alerts during a storm (after the aggregation
// window has been created by StartAggregation).
//
// DD-GATEWAY-008 Enhancements:
// - Extends window timer on each alert (sliding window behavior)
// - Checks max window duration safety limit
// - Forces window closure if max duration exceeded
//
// Resources are stored in a Redis sorted set with timestamp as score.
// This allows:
// - Tracking when each resource was added
// - Retrieving resources in chronological order
// - Automatically expiring old entries
//
// Resource identifier format: "namespace:Kind:name"
// Example: "prod-api:Pod:payment-api-789"
//
// Business Requirements:
// - BR-GATEWAY-008: Sliding window with inactivity timeout
// - BR-GATEWAY-008: Maximum window duration safety limit
func (a *StormAggregator) AddResource(ctx context.Context, windowID string, signal *types.NormalizedSignal) error {
	key := fmt.Sprintf("alert:storm:resources:%s", windowID)
	resourceID := signal.Resource.String()

	// DD-GATEWAY-008: Check if window has exceeded max duration
	// Extract window start time from windowID (format: "AlertName-UnixTimestamp")
	// Example: "PodCrashLooping-1696800000" → start time = 1696800000

	// Find the last hyphen to split alertname from timestamp
	// This handles alert names with hyphens (e.g., "Pod-Crash-Looping-1696800000")
	lastHyphen := strings.LastIndex(windowID, "-")
	if lastHyphen == -1 {
		// No hyphen found - invalid windowID format
		return fmt.Errorf("invalid windowID format '%s': expected 'AlertName-UnixTimestamp'", windowID)
	}

	timestampStr := windowID[lastHyphen+1:]
	startUnix, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		// Failed to parse timestamp
		return fmt.Errorf("failed to parse timestamp from windowID '%s': %w", windowID, err)
	}

	windowStartTime := time.Unix(startUnix, 0)
	currentTime := time.Now()
	if a.IsWindowExpired(windowStartTime, currentTime, a.maxWindowDuration) {
		// Max duration exceeded: reject new alert, force window closure
		return fmt.Errorf("window exceeded max duration (%v), rejecting new alert", a.maxWindowDuration)
	}

	// Add to sorted set (score = timestamp)
	if err := a.redisClient.ZAdd(ctx, key, redis.Z{
		Score:  float64(time.Now().Unix()),
		Member: resourceID,
	}).Err(); err != nil {
		return fmt.Errorf("failed to add resource to aggregation: %w", err)
	}

	// DD-GATEWAY-008: Extend window timer (sliding window behavior)
	// Reset TTL to full window duration on EVERY alert
	now := time.Now()
	_, extendErr := a.ExtendWindow(ctx, windowID, now)
	if extendErr != nil {
		// Non-fatal: log warning but don't fail the operation
		// Window will still expire based on original TTL
		// TODO: Add logging when logger is available
	}

	// Set TTL on resources (2x window duration to allow retrieval after window closes)
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
		// DD-GATEWAY-008: Sliding window tracking (BR-GATEWAY-008)
		"absolute_start_time": stormMetadata.AbsoluteStartTime.Format(time.RFC3339Nano),
		"last_activity":       stormMetadata.LastActivity.Format(time.RFC3339Nano),
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
	// DD-GATEWAY-008 Day 4: Check capacity BEFORE buffering (BR-GATEWAY-011)
	isOver, currentSize, limit, err := a.IsOverCapacity(ctx, signal.Namespace)
	if err != nil {
		return 0, false, fmt.Errorf("failed to check capacity: %w", err)
	}

	if isOver {
		// Namespace is over capacity: reject alert
		return currentSize, false, fmt.Errorf("namespace %s over capacity (%d/%d alerts)", signal.Namespace, currentSize, limit)
	}

	// DD-GATEWAY-008 Day 5: Check if sampling should be enabled (BR-GATEWAY-011)
	shouldSample, _, err := a.ShouldEnableSampling(ctx, signal.Namespace)
	if err != nil {
		// Non-fatal: log warning but continue (don't block on sampling check)
		// TODO: Add logging when logger is available (include utilization in log)
	}

	if shouldSample {
		// Sampling enabled: randomly accept/reject based on sampling rate
		// Generate random number [0.0, 1.0)
		randomValue := rand.Float64()

		if randomValue > a.samplingRate {
			// Reject alert (sampled out)
			// Return current size but no error (not a failure, just sampled)
			return currentSize, false, nil
		}
		// Accept alert (passed sampling)
	}

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

	// Use configured threshold from struct (default: 5)
	threshold := a.bufferThreshold
	shouldAggregate := int(bufferSize) >= threshold

	return int(bufferSize), shouldAggregate, nil
}

// ExtendWindow extends window expiration time (sliding window behavior)
//
// Business Requirement: BR-GATEWAY-008 - Sliding window with inactivity timeout
//
// Returns:
// - time.Time: new expiration time
// - error: Redis errors
func (a *StormAggregator) ExtendWindow(ctx context.Context, windowID string, currentTime time.Time) (time.Time, error) {
	// TDD REFACTOR: Improved efficiency by using targeted search instead of full SCAN
	//
	// Strategy: Try common key patterns first (O(1)), fall back to SCAN only if needed
	// This optimizes for the common case (production windowIDs) while maintaining
	// backward compatibility with test windowIDs

	// Production windowID format: "Namespace:AlertName-UnixTimestamp" (DD-GATEWAY-008 + BR-GATEWAY-011)
	// Example: "prod-api:PodCrashLooping-1696800000" → key: "alert:storm:aggregate:prod-api:PodCrashLooping"

	// Extract namespace and alert name from windowID
	// Format: "namespace:alertname-timestamp"
	var namespace, alertName string
	if colonIdx := strings.Index(windowID, ":"); colonIdx > 0 {
		namespace = windowID[:colonIdx]
		rest := windowID[colonIdx+1:]
		// Extract alert name (everything before last hyphen in rest)
		alertName = rest
		for i := len(rest) - 1; i >= 0; i-- {
			if rest[i] == '-' {
				alertName = rest[:i]
				break
			}
		}
	} else {
		// Legacy format (no namespace): "alertname-timestamp"
		alertName = windowID
		for i := len(windowID) - 1; i >= 0; i-- {
			if windowID[i] == '-' {
				alertName = windowID[:i]
				break
			}
		}
	}

	// Try direct key lookup first (O(1) - fast path for production)
	var windowKey string
	if namespace != "" {
		windowKey = fmt.Sprintf("alert:storm:aggregate:%s:%s", namespace, alertName)
	} else {
		// Legacy format fallback
		windowKey = fmt.Sprintf("alert:storm:aggregate:%s", alertName)
	}
	storedWindowID, err := a.redisClient.Get(ctx, windowKey).Result()

	if err == nil && storedWindowID == windowID {
		// Fast path: Found matching window
		if err := a.redisClient.Expire(ctx, windowKey, a.windowDuration).Err(); err != nil {
			return time.Time{}, fmt.Errorf("failed to extend window: %w", err)
		}
		return currentTime.Add(a.windowDuration), nil
	}

	// Slow path: Fall back to SCAN for non-standard windowIDs (tests, edge cases)
	// This maintains backward compatibility while optimizing the common case
	// Pattern: alert:storm:aggregate:*:* (namespace:alertname format)
	iter := a.redisClient.Scan(ctx, 0, "alert:storm:aggregate:*", 100).Iterator()
	windowKey = ""

	for iter.Next(ctx) {
		key := iter.Val()
		val, err := a.redisClient.Get(ctx, key).Result()
		if err == nil && val == windowID {
			windowKey = key
			break
		}
	}

	if windowKey == "" {
		return time.Time{}, fmt.Errorf("window not found for windowID: %s", windowID)
	}

	// Reset TTL to window duration (sliding window behavior)
	if err := a.redisClient.Expire(ctx, windowKey, a.windowDuration).Err(); err != nil {
		return time.Time{}, fmt.Errorf("failed to extend window: %w", err)
	}

	// Calculate new expiration time
	newExpiration := currentTime.Add(a.windowDuration)
	return newExpiration, nil
}

// IsWindowExpired checks if window exceeded max duration (pure function)
//
// Business Requirement: BR-GATEWAY-008 - Maximum window duration safety limit
//
// Returns:
// - bool: true if window has exceeded max duration, false otherwise
func (a *StormAggregator) IsWindowExpired(windowStartTime, currentTime time.Time, maxDuration time.Duration) bool {
	elapsed := currentTime.Sub(windowStartTime)
	return elapsed > maxDuration
}

// GetNamespaceUtilization returns buffer utilization percentage (0.0-1.0)
//
// Business Requirement: BR-GATEWAY-011 - Multi-tenant isolation and capacity management
//
// Returns:
// - float64: utilization percentage (0.0 = empty, 1.0 = full)
// - error: Redis errors
func (a *StormAggregator) GetNamespaceUtilization(ctx context.Context, namespace string) (float64, error) {
	// Count all buffered alerts for this namespace
	// Buffer keys pattern: alert:buffer:<namespace>:*
	pattern := fmt.Sprintf("alert:buffer:%s:*", namespace)

	var totalBuffered int64
	iter := a.redisClient.Scan(ctx, 0, pattern, 100).Iterator()

	for iter.Next(ctx) {
		key := iter.Val()
		count, err := a.redisClient.LLen(ctx, key).Result()
		if err != nil {
			continue // Skip errors, count what we can
		}
		totalBuffered += count
	}

	if err := iter.Err(); err != nil {
		return 0.0, fmt.Errorf("failed to scan namespace buffers: %w", err)
	}

	// Get namespace-specific limit (Day 4: multi-tenant isolation)
	maxSize := float64(a.GetNamespaceLimit(namespace))
	utilization := float64(totalBuffered) / maxSize

	return utilization, nil
}

// ShouldSample determines if sampling should be enabled (pure function)
//
// Business Requirement: BR-GATEWAY-011 - Overflow protection through sampling
//
// Returns:
// - bool: true if sampling should be enabled, false otherwise
func (a *StormAggregator) ShouldSample(currentUtilization, samplingThreshold float64) bool {
	return currentUtilization > samplingThreshold
}

// ShouldEnableSampling checks if sampling should be enabled for a namespace
//
// Business Requirement: BR-GATEWAY-011 - Overflow protection through sampling
//
// Combines GetNamespaceUtilization() and ShouldSample() for convenience.
//
// Parameters:
// - ctx: Context for Redis operations
// - namespace: The namespace to check
//
// Returns:
// - bool: true if sampling should be enabled
// - float64: current utilization (0.0-1.0)
// - error: Redis errors
//
// Example:
// - prod-api with 1900/2000 alerts, threshold 0.95 → (false, 0.95, nil)
// - prod-api with 1950/2000 alerts, threshold 0.95 → (true, 0.975, nil)
func (a *StormAggregator) ShouldEnableSampling(ctx context.Context, namespace string) (bool, float64, error) {
	utilization, err := a.GetNamespaceUtilization(ctx, namespace)
	if err != nil {
		return false, 0.0, fmt.Errorf("failed to get utilization: %w", err)
	}

	shouldSample := a.ShouldSample(utilization, a.samplingThreshold)

	return shouldSample, utilization, nil
}

// GetNamespaceLimit returns the buffer size limit for a given namespace
//
// Business Requirement: BR-GATEWAY-011 - Multi-tenant isolation
//
// Returns the namespace-specific limit if configured, otherwise returns the default limit.
//
// Parameters:
// - namespace: The namespace to get the limit for
//
// Returns:
// - int: Buffer size limit for the namespace
//
// Example:
// - prod-api with override of 2000 → returns 2000
// - dev-test with no override → returns defaultMaxSize (1000)
func (a *StormAggregator) GetNamespaceLimit(namespace string) int {
	// Check if namespace has a specific limit override
	if limit, exists := a.perNamespaceLimits[namespace]; exists {
		return limit
	}
	// Return default limit
	return a.defaultMaxSize
}

// IsOverCapacity checks if a namespace has exceeded its buffer capacity
//
// Business Requirement: BR-GATEWAY-011 - Multi-tenant isolation and capacity enforcement
//
// Checks both namespace-specific limit and global limit.
//
// Parameters:
// - ctx: Context for Redis operations
// - namespace: The namespace to check
//
// Returns:
// - bool: true if over capacity (should reject new alerts)
// - int: current buffer size in namespace
// - int: namespace limit
// - error: Redis errors
//
// Example:
// - prod-api with 1500 alerts, limit 2000 → (false, 1500, 2000, nil)
// - dev-test with 600 alerts, limit 500 → (true, 600, 500, nil)
func (a *StormAggregator) IsOverCapacity(ctx context.Context, namespace string) (bool, int, int, error) {
	// Get namespace-specific limit
	namespaceLimit := a.GetNamespaceLimit(namespace)

	// Count all buffered alerts in this namespace
	// Buffer keys pattern: alert:buffer:<namespace>:*
	pattern := fmt.Sprintf("alert:buffer:%s:*", namespace)

	var totalBuffered int64
	iter := a.redisClient.Scan(ctx, 0, pattern, 100).Iterator()

	for iter.Next(ctx) {
		key := iter.Val()
		count, err := a.redisClient.LLen(ctx, key).Result()
		if err != nil {
			continue // Skip errors, count what we can
		}
		totalBuffered += count
	}

	if err := iter.Err(); err != nil {
		return false, 0, namespaceLimit, fmt.Errorf("failed to scan namespace buffers: %w", err)
	}

	// Check if over capacity
	isOver := int(totalBuffered) >= namespaceLimit

	return isOver, int(totalBuffered), namespaceLimit, nil
}

// GetBufferedAlerts retrieves all buffered alerts for a given namespace and alert name
//
// DD-GATEWAY-008: Required for creating aggregated CRD with ALL buffered alerts when threshold reached
//
// Business Requirement: BR-GATEWAY-016 - Storm aggregation must reduce AI analysis costs by 90%+
//
// Returns:
// - []*types.NormalizedSignal: All buffered alerts
// - error: Redis errors
func (a *StormAggregator) GetBufferedAlerts(ctx context.Context, namespace, alertName string) ([]*types.NormalizedSignal, error) {
	bufferKey := fmt.Sprintf("alert:buffer:%s:%s", namespace, alertName)

	// Get all resource IDs from buffer
	resourceIDs, err := a.redisClient.LRange(ctx, bufferKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get buffered alerts: %w", err)
	}

	// TDD GREEN: Parse resource IDs and create signals
	signals := make([]*types.NormalizedSignal, 0, len(resourceIDs))
	for _, resourceID := range resourceIDs {
		// Parse resource ID format: "namespace:Kind:name"
		// Example: "production:Pod:app-pod-1"
		parts := strings.Split(resourceID, ":")
		if len(parts) != 3 {
			// Invalid format, skip
			continue
		}

		signal := &types.NormalizedSignal{
			AlertName: alertName,
			Namespace: namespace,
			Resource: types.ResourceIdentifier{
				Namespace: parts[0],
				Kind:      parts[1],
				Name:      parts[2],
			},
			Fingerprint: fmt.Sprintf("%s-%s", alertName, resourceID),
		}
		signals = append(signals, signal)
	}

	return signals, nil
}
