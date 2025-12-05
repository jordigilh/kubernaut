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

// Package enricher provides K8s context enrichment for signal processing.
package enricher

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnrichmentResult holds the enriched K8s context data.
type EnrichmentResult struct {
	NamespaceLabels map[string]string
}

// K8sEnricher enriches signal processing with K8s context.
type K8sEnricher struct {
	client client.Client
	logger logr.Logger
}

// NewK8sEnricher creates a new K8sEnricher.
func NewK8sEnricher(c client.Client, logger logr.Logger) *K8sEnricher {
	return &K8sEnricher{
		client: c,
		logger: logger.WithName("k8s-enricher"),
	}
}

// Enrich retrieves K8s context for the given namespace and pod.
func (e *K8sEnricher) Enrich(ctx context.Context, namespace, podName string) (*EnrichmentResult, error) {
	// Get namespace
	ns := &corev1.Namespace{}
	if err := e.client.Get(ctx, types.NamespacedName{Name: namespace}, ns); err != nil {
		return nil, err
	}

	return &EnrichmentResult{
		NamespaceLabels: ns.Labels,
	}, nil
}

