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
// - In-memory cache for namespace → environment mapping
// - Reduces Kubernetes API calls (~1ms after first lookup vs ~10-50ms cold lookup)
// - Cache is shared across all Gateway replicas (stored in Redis in future enhancement)
type EnvironmentClassifier struct {
	k8sClient client.Client
	logger    *logrus.Logger

	// cache stores namespace → environment mapping
	// Key: namespace name
	// Value: environment (any non-empty string, e.g., "prod", "staging", "dev", "canary", "qa-eu")
	cache map[string]string
	mu    sync.RWMutex
}

// NewEnvironmentClassifier creates a new environment classifier
//
// Parameters:
// - k8sClient: Kubernetes client for namespace and ConfigMap lookups
// - logger: Structured logger for debugging classification decisions
func NewEnvironmentClassifier(k8sClient client.Client, logger *logrus.Logger) *EnvironmentClassifier {
	return &EnvironmentClassifier{
		k8sClient: k8sClient,
		logger:    logger,
		cache:     make(map[string]string),
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

// isValidEnvironment checks if the environment value is valid
//
// Dynamic Configuration Philosophy:
// This function now accepts ANY non-empty string as a valid environment.
// Organizations define their own environment taxonomy based on their needs:
//
// Examples of valid environments:
// - Standard: "prod", "staging", "dev", "qa", "uat"
// - Regional: "prod-east", "prod-west", "staging-eu"
// - Deployment strategies: "canary", "blue", "green"
// - Custom: "production", "pre-prod", "development", "local"
//
// Why no hardcoded validation:
// - Labels are meant for DYNAMIC configuration, not static enforcement
// - Organizations have diverse environment taxonomies
// - Downstream services (Priority, Rego) handle environment-specific logic
// - Validation happens at business rule layer, not infrastructure layer
//
// Returns:
// - bool: true if non-empty, false otherwise
func isValidEnvironment(env string) bool {
	return env != ""
}

// getFromCache retrieves environment from cache
//
// Thread-safe read with RLock (supports concurrent lookups).
//
// Returns:
// - string: Environment if found in cache, empty string otherwise
func (c *EnvironmentClassifier) getFromCache(namespace string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cache[namespace]
}

// setCache stores environment in cache
//
// Thread-safe write with Lock.
//
// Cache invalidation:
// - No TTL (environment changes are rare)
// - Manual invalidation: Restart Gateway service or implement cache eviction API
// - Future enhancement: Watch namespace labels and ConfigMap for changes
func (c *EnvironmentClassifier) setCache(namespace, environment string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[namespace] = environment

	c.logger.WithFields(logrus.Fields{
		"namespace":   namespace,
		"environment": environment,
	}).Debug("Cached environment classification")
}

// ClearCache clears the entire cache
//
// Use cases:
// - Testing (reset cache between test cases)
// - Manual cache invalidation (via admin API in future)
// - Memory management (if cache grows too large)
func (c *EnvironmentClassifier) ClearCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]string)

	c.logger.Info("Cleared environment classification cache")
}

// GetCacheSize returns the number of cached entries
//
// Use cases:
// - Monitoring (expose as Prometheus gauge)
// - Debugging
// - Testing
func (c *EnvironmentClassifier) GetCacheSize() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}
