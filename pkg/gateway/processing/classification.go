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
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnvironmentClassifier determines environment from namespace labels and ConfigMap fallback
//
// This classifier provides two-tier environment lookup:
// 1. Namespace labels (primary): Check for "environment" label on namespace
// 2. ConfigMap fallback (secondary): Look up in "kubernaut-environment-overrides" ConfigMap
// 3. Default fallback (tertiary): Return "unknown" if no explicit classification
//
// Environment classification affects:
// - Priority assignment (critical + prod → P0, critical + dev → P2)
// - GitOps remediation behavior (prod = read-only Argo approval, dev = automatic apply)
// - Notification routing (prod alerts → PagerDuty, dev alerts → Slack)
//
// Dynamic Configuration:
// - Accepts ANY non-empty string as a valid environment
// - Organizations define their own environment taxonomy (prod, staging, dev, qa, canary, etc.)
// - No hardcoded validation - labels provide dynamic configuration
//
// Caching:
// - In-memory TTL-based cache for namespace → environment mapping
// - Reduces Kubernetes API calls (~1ms after first lookup vs ~10-50ms cold lookup)
// - Cache expires after configurable TTL (default: 30 seconds)
// - Cache is automatically cleaned up by go-cache background goroutine
// - Cache is shared across all Gateway replicas (stored in Redis in future enhancement)
type EnvironmentClassifier struct {
	k8sClient client.Client
	logger    *logrus.Logger

	// cache stores namespace → environment mapping with TTL
	// Key: namespace name
	// Value: environment (any non-empty string, e.g., "prod", "staging", "dev", "canary", "qa-eu")
	// TTL: Configurable (default 30s), after which cache entry is automatically evicted
	cache    *cache.Cache
	cacheTTL time.Duration
	mu       sync.RWMutex // Still needed for thread-safe operations
}

// NewEnvironmentClassifier creates a new environment classifier with default cache TTL (30 seconds)
//
// Parameters:
// - k8sClient: Kubernetes client for namespace and ConfigMap lookups
// - logger: Structured logger for debugging classification decisions
//
// For custom TTL, use NewEnvironmentClassifierWithTTL.
func NewEnvironmentClassifier(k8sClient client.Client, logger *logrus.Logger) *EnvironmentClassifier {
	return NewEnvironmentClassifierWithTTL(k8sClient, logger, 30*time.Second)
}

// NewEnvironmentClassifierWithTTL creates a new environment classifier with custom cache TTL
//
// Parameters:
// - k8sClient: Kubernetes client for namespace and ConfigMap lookups
// - logger: Structured logger for debugging classification decisions
// - cacheTTL: Time-to-live for cache entries (e.g., 30*time.Second)
//   - Production default: 30 seconds (balances freshness vs. API calls)
//   - Testing: 5 seconds (faster cache expiry for integration tests)
//   - Set to 0 for no caching (every lookup hits Kubernetes API)
//
// Cache TTL Trade-offs:
// - Shorter TTL (5-10s): More responsive to environment changes, more API calls
// - Longer TTL (60-300s): Fewer API calls, slower to reflect changes
// - Default 30s: Good balance for most use cases
func NewEnvironmentClassifierWithTTL(k8sClient client.Client, logger *logrus.Logger, cacheTTL time.Duration) *EnvironmentClassifier {
	// Create TTL cache with cleanup interval = 2x TTL
	// This ensures expired entries are cleaned up efficiently
	cleanupInterval := 2 * cacheTTL
	if cacheTTL == 0 {
		// No caching - use very short cleanup to avoid memory leaks
		cleanupInterval = 1 * time.Minute
	}

	return &EnvironmentClassifier{
		k8sClient: k8sClient,
		logger:    logger,
		cache:     cache.New(cacheTTL, cleanupInterval),
		cacheTTL:  cacheTTL,
	}
}

// Classify determines the environment for a given namespace
//
// Lookup order:
// 1. Check cache (fast path, ~1ms)
// 2. Check namespace labels (primary, ~10-50ms)
//   - Label key: "environment"
//   - Valid values: ANY non-empty string
//   - Examples:
//     kubectl label namespace prod-api environment=prod
//     kubectl label namespace canary environment=canary
//     kubectl label namespace qa-eu environment=qa-eu
//
// 3. Check ConfigMap override (fallback, ~10-50ms)
//   - ConfigMap: kubernaut-system/kubernaut-environment-overrides
//   - Data format: namespace-name: environment
//   - Example:
//     apiVersion: v1
//     kind: ConfigMap
//     metadata:
//     name: kubernaut-environment-overrides
//     namespace: kubernaut-system
//     data:
//     prod-api: prod
//     staging-api: staging
//     dev-api: dev
//     canary-api: canary
//
// 4. Default fallback (last resort)
//   - Returns "unknown" if no explicit classification found
//   - Logs warning for visibility
//
// Returns:
// - string: Any environment string from labels/ConfigMap, or "unknown" if not found
func (c *EnvironmentClassifier) Classify(ctx context.Context, namespace string) string {
	// 1. Check cache first (fast path)
	if env := c.getFromCache(namespace); env != "" {
		c.logger.WithFields(logrus.Fields{
			"namespace":   namespace,
			"environment": env,
			"source":      "cache",
		}).Debug("Environment classification from cache")
		return env
	}

	// 2. Check namespace labels (primary)
	ns := &corev1.Namespace{}
	if err := c.k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, ns); err == nil {
		if env, ok := ns.Labels["environment"]; ok && env != "" {
			// Accept any non-empty environment string for dynamic configuration
			// Organizations define their own environment taxonomy
			c.setCache(namespace, env)
			c.logger.WithFields(logrus.Fields{
				"namespace":   namespace,
				"environment": env,
				"source":      "namespace_label",
			}).Debug("Environment classification from namespace label")
			return env
		}
	} else {
		// Log error but continue to fallback (namespace might not exist yet)
		c.logger.WithFields(logrus.Fields{
			"namespace": namespace,
			"error":     err,
		}).Debug("Failed to get namespace for environment classification")
	}

	// 3. Check ConfigMap override (fallback)
	cm := &corev1.ConfigMap{}
	if err := c.k8sClient.Get(ctx, types.NamespacedName{
		Name:      "kubernaut-environment-overrides",
		Namespace: "kubernaut-system",
	}, cm); err == nil {
		if env, ok := cm.Data[namespace]; ok && env != "" {
			// Accept any non-empty environment string for dynamic configuration
			c.setCache(namespace, env)
			c.logger.WithFields(logrus.Fields{
				"namespace":   namespace,
				"environment": env,
				"source":      "configmap_override",
			}).Debug("Environment classification from ConfigMap override")
			return env
		}
	} else {
		// Log error but continue to default fallback
		c.logger.WithFields(logrus.Fields{
			"namespace": namespace,
			"error":     err,
		}).Debug("Failed to get ConfigMap for environment classification")
	}

	// 4. Default fallback (last resort)
	c.logger.WithFields(logrus.Fields{
		"namespace": namespace,
	}).Warn("No environment label or ConfigMap override found, defaulting to 'unknown'")

	defaultEnv := "unknown"
	c.setCache(namespace, defaultEnv)
	return defaultEnv
}

// getFromCache retrieves environment from cache
//
// Thread-safe read using go-cache (supports concurrent lookups).
// Cache entries automatically expire after TTL.
//
// Returns:
// - string: Environment if found in cache and not expired, empty string otherwise
func (c *EnvironmentClassifier) getFromCache(namespace string) string {
	if val, found := c.cache.Get(namespace); found {
		if env, ok := val.(string); ok {
			return env
		}
	}
	return ""
}

// setCache stores environment in cache with TTL
//
// Thread-safe write using go-cache.
//
// Cache invalidation:
// - Automatic TTL expiry (default: 30 seconds)
// - Background cleanup goroutine removes expired entries
// - Manual invalidation: Use ClearCache() or restart Gateway service
// - Future enhancement: Watch namespace labels and ConfigMap for real-time invalidation
func (c *EnvironmentClassifier) setCache(namespace, environment string) {
	// Use cache.DefaultExpiration to inherit TTL from cache constructor
	c.cache.Set(namespace, environment, cache.DefaultExpiration)

	c.logger.WithFields(logrus.Fields{
		"namespace":   namespace,
		"environment": environment,
		"ttl":         c.cacheTTL,
	}).Debug("Cached environment classification with TTL")
}

// ClearCache clears the entire cache
//
// Use cases:
// - Testing (reset cache between test cases)
// - Manual cache invalidation (via admin API in future)
// - Memory management (if cache grows too large)
func (c *EnvironmentClassifier) ClearCache() {
	c.cache.Flush()

	c.logger.Info("Cleared environment classification cache")
}

// GetCacheSize returns the number of cached entries
//
// Use cases:
// - Monitoring (expose as Prometheus gauge)
// - Debugging
// - Testing
//
// Note: This includes both non-expired and expired (not yet cleaned up) entries.
// go-cache cleanup goroutine runs at 2x TTL interval.
func (c *EnvironmentClassifier) GetCacheSize() int {
	return c.cache.ItemCount()
}
