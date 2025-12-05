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

// Package classifier provides environment and business classification for signal processing.
//
// This package implements environment classification per BR-SP-051, BR-SP-052, BR-SP-053:
//   - Primary: Namespace label `kubernaut.ai/environment` (confidence: 0.95)
//   - Fallback: ConfigMap override `kubernaut-environment-overrides` (confidence: 0.75)
//   - Default: "unknown" (confidence: 0.0)
//
// Ported from Gateway service to centralize classification in Signal Processing.
// Per IMPLEMENTATION_PLAN_V1.21.md Day 4 specification.
package classifier

import (
	"context"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/patrickmn/go-cache"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

const (
	// EnvironmentLabelKey is the ONLY label key used for environment detection.
	// Per BR-SP-051: Only kubernaut.ai/ prefixed labels to prevent accidental capture.
	EnvironmentLabelKey = "kubernaut.ai/environment"

	// ConfigMapName is the name of the environment overrides ConfigMap.
	ConfigMapName = "kubernaut-environment-overrides"

	// ConfigMapNamespace is the namespace where the ConfigMap lives.
	ConfigMapNamespace = "kubernaut-system"

	// NamespaceLabelConfidence is the confidence when environment is from namespace label.
	NamespaceLabelConfidence = 0.95

	// ConfigMapConfidence is the confidence when environment is from ConfigMap.
	ConfigMapConfidence = 0.75

	// DefaultConfidence is the confidence when using default environment.
	DefaultConfidence = 0.0

	// DefaultEnvironment is returned when no detection succeeds.
	DefaultEnvironment = "unknown"
)

// EnvironmentClassifier determines environment from namespace labels and ConfigMap fallback.
//
// Classification priority (BR-SP-051, BR-SP-052, BR-SP-053):
// 1. Namespace label: kubernaut.ai/environment (confidence: 0.95)
// 2. ConfigMap fallback (confidence: 0.75)
// 3. Default: "unknown" (confidence: 0.0)
//
// Caching:
// - In-memory TTL-based cache for namespace → environment mapping
// - Reduces Kubernetes API calls
// - Cache expires after configurable TTL (default: 30 seconds)
type EnvironmentClassifier struct {
	k8sClient client.Client
	logger    logr.Logger
	cache     *cache.Cache
	cacheTTL  time.Duration
}

// NewEnvironmentClassifier creates a new environment classifier with default cache TTL (30 seconds).
func NewEnvironmentClassifier(k8sClient client.Client, logger logr.Logger) *EnvironmentClassifier {
	return NewEnvironmentClassifierWithTTL(k8sClient, logger, 30*time.Second)
}

// NewEnvironmentClassifierWithTTL creates a new environment classifier with custom cache TTL.
func NewEnvironmentClassifierWithTTL(k8sClient client.Client, logger logr.Logger, cacheTTL time.Duration) *EnvironmentClassifier {
	cleanupInterval := 2 * cacheTTL
	if cacheTTL == 0 {
		cleanupInterval = 1 * time.Minute
	}

	return &EnvironmentClassifier{
		k8sClient: k8sClient,
		logger:    logger.WithName("environment-classifier"),
		cache:     cache.New(cacheTTL, cleanupInterval),
		cacheTTL:  cacheTTL,
	}
}

// Classify determines the environment for a given Kubernetes context.
//
// BR-SP-051: Primary detection from kubernaut.ai/environment namespace label
// BR-SP-052: ConfigMap fallback when label is absent
// BR-SP-053: Default to "unknown" when all methods fail
//
// Never fails - always returns a valid EnvironmentClassification.
func (c *EnvironmentClassifier) Classify(ctx context.Context, k8sCtx *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.EnvironmentClassification, error) {
	// Handle nil context
	if k8sCtx == nil || k8sCtx.Namespace == nil {
		c.logger.V(1).Info("No namespace context provided, returning default")
		return c.defaultResult(), nil
	}

	namespaceName := k8sCtx.Namespace.Name

	// 1. Check cache first (fast path)
	if result := c.getFromCache(namespaceName); result != nil {
		c.logger.V(1).Info("Environment classification from cache",
			"namespace", namespaceName,
			"environment", result.Environment)
		return result, nil
	}

	// 2. Check namespace labels for kubernaut.ai/environment (primary - BR-SP-051)
	if env := c.getFromNamespaceLabels(k8sCtx.Namespace); env != "" {
		result := &signalprocessingv1alpha1.EnvironmentClassification{
			Environment: strings.ToLower(env),
			Confidence:  NamespaceLabelConfidence,
			Source:      "namespace-label",
		}
		c.setCache(namespaceName, result)
		c.logger.V(1).Info("Environment classification from namespace label",
			"namespace", namespaceName,
			"environment", result.Environment)
		return result, nil
	}

	// 3. Check ConfigMap override (fallback - BR-SP-052)
	if env := c.getFromConfigMap(ctx, namespaceName); env != "" {
		result := &signalprocessingv1alpha1.EnvironmentClassification{
			Environment: strings.ToLower(env),
			Confidence:  ConfigMapConfidence,
			Source:      "configmap",
		}
		c.setCache(namespaceName, result)
		c.logger.V(1).Info("Environment classification from ConfigMap",
			"namespace", namespaceName,
			"environment", result.Environment)
		return result, nil
	}

	// 4. Default fallback (last resort - BR-SP-053)
	c.logger.Info("No environment label or ConfigMap override found, defaulting to 'unknown'",
		"namespace", namespaceName)

	result := c.defaultResult()
	c.setCache(namespaceName, result)
	return result, nil
}

// getFromNamespaceLabels extracts environment from namespace labels.
// Only checks kubernaut.ai/environment - ignores all other labels.
func (c *EnvironmentClassifier) getFromNamespaceLabels(ns *signalprocessingv1alpha1.NamespaceContext) string {
	if ns == nil || ns.Labels == nil {
		return ""
	}

	env, ok := ns.Labels[EnvironmentLabelKey]
	if !ok || env == "" {
		return ""
	}

	return env
}

// getFromConfigMap looks up environment from the overrides ConfigMap.
func (c *EnvironmentClassifier) getFromConfigMap(ctx context.Context, namespace string) string {
	if c.k8sClient == nil {
		return ""
	}

	cm := &corev1.ConfigMap{}
	err := c.k8sClient.Get(ctx, types.NamespacedName{
		Name:      ConfigMapName,
		Namespace: ConfigMapNamespace,
	}, cm)

	if err != nil {
		c.logger.V(1).Info("Failed to get ConfigMap for environment classification",
			"namespace", namespace,
			"error", err)
		return ""
	}

	if env, ok := cm.Data[namespace]; ok && env != "" {
		return env
	}

	return ""
}

// defaultResult returns the default environment classification.
func (c *EnvironmentClassifier) defaultResult() *signalprocessingv1alpha1.EnvironmentClassification {
	return &signalprocessingv1alpha1.EnvironmentClassification{
		Environment: DefaultEnvironment,
		Confidence:  DefaultConfidence,
		Source:      "default",
	}
}

// getFromCache retrieves environment from cache.
func (c *EnvironmentClassifier) getFromCache(namespace string) *signalprocessingv1alpha1.EnvironmentClassification {
	if val, found := c.cache.Get(namespace); found {
		if result, ok := val.(*signalprocessingv1alpha1.EnvironmentClassification); ok {
			return result
		}
	}
	return nil
}

// setCache stores environment in cache with TTL.
func (c *EnvironmentClassifier) setCache(namespace string, result *signalprocessingv1alpha1.EnvironmentClassification) {
	c.cache.Set(namespace, result, cache.DefaultExpiration)
}

// ClearCache clears the entire cache.
func (c *EnvironmentClassifier) ClearCache() {
	c.cache.Flush()
	c.logger.Info("Cleared environment classification cache")
}

// GetCacheSize returns the number of cached entries.
func (c *EnvironmentClassifier) GetCacheSize() int {
	return c.cache.ItemCount()
}

