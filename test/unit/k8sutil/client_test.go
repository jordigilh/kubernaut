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

package k8sutil

import (
	"os"
	"testing"

	k8sutil "github.com/jordigilh/kubernaut/pkg/k8sutil"
)

func TestGetConfig(t *testing.T) {
	// Save and restore original KUBECONFIG
	originalKubeconfig := os.Getenv("KUBECONFIG")
	defer func() {
		if originalKubeconfig != "" {
			os.Setenv("KUBECONFIG", originalKubeconfig)
		} else {
			os.Unsetenv("KUBECONFIG")
		}
	}()

	t.Run("returns error when no config available", func(t *testing.T) {
		os.Unsetenv("KUBECONFIG")
		// In test environment, in-cluster config will fail
		_, err := k8sutil.GetConfig()
		if err == nil {
			t.Error("expected error when no config available")
		}
	})

	t.Run("uses KUBECONFIG when set", func(t *testing.T) {
		// This test requires a valid kubeconfig file
		// Skip if KUBECONFIG not set in test environment
		if originalKubeconfig == "" {
			t.Skip("KUBECONFIG not set, skipping")
		}

		os.Setenv("KUBECONFIG", originalKubeconfig)
		config, err := k8sutil.GetConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if config == nil {
			t.Error("expected non-nil config")
		}
	})
}

func TestNewClientset(t *testing.T) {
	// Skip if no Kubernetes access available
	if os.Getenv("KUBECONFIG") == "" {
		t.Skip("KUBECONFIG not set, skipping")
	}

	clientset, err := k8sutil.NewClientset()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clientset == nil {
		t.Error("expected non-nil clientset")
	}

	// Verify clientset works by checking server version
	_, err = clientset.Discovery().ServerVersion()
	if err != nil {
		t.Errorf("failed to get server version: %v", err)
	}
}
