/*
Copyright 2025 Kubernaut.

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

// Package k8sutil provides utilities for Kubernetes client initialization.
//
// This package implements DD-013: Kubernetes Client Initialization Standard
// See: docs/architecture/decisions/DD-013-kubernetes-client-initialization-standard.md
package k8sutil

import (
	"fmt"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// GetConfig returns a Kubernetes REST config, trying KUBECONFIG first, then in-cluster.
//
// This is the standard pattern for HTTP services that need Kubernetes access.
// For CRD controllers, use ctrl.GetConfigOrDie() from controller-runtime instead.
//
// Configuration Resolution Order:
//  1. KUBECONFIG environment variable (development mode)
//  2. In-cluster config from service account (production mode)
//
// Usage:
//
//	config, err := k8sutil.GetConfig()
//	if err != nil {
//	    return fmt.Errorf("failed to get Kubernetes config: %w", err)
//	}
//	clientset, err := kubernetes.NewForConfig(config)
//
// DD-013: Standard Kubernetes client initialization for HTTP services
func GetConfig() (*rest.Config, error) {
	// Try KUBECONFIG environment variable first (development)
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build config from KUBECONFIG: %w", err)
		}
		return config, nil
	}

	// Fallback to in-cluster config (production)
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	return config, nil
}

// NewClientset creates a Kubernetes clientset using the standard config resolution.
//
// This is a convenience wrapper around GetConfig() + kubernetes.NewForConfig().
// For CRD controllers, use manager.GetClient() from controller-runtime instead.
//
// Configuration Resolution Order:
//  1. KUBECONFIG environment variable (development mode)
//  2. In-cluster config from service account (production mode)
//
// Usage:
//
//	clientset, err := k8sutil.NewClientset()
//	if err != nil {
//	    return fmt.Errorf("failed to create Kubernetes client: %w", err)
//	}
//	// Use clientset to interact with Kubernetes API
//	serverVersion, err := clientset.Discovery().ServerVersion()
//
// DD-013: Standard Kubernetes client initialization for HTTP services
func NewClientset() (kubernetes.Interface, error) {
	config, err := GetConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return clientset, nil
}
