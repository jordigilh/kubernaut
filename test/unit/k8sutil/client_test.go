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
	originalKubeconfig := os.Getenv("KUBECONFIG")
	defer func() {
		if originalKubeconfig != "" {
			_ = os.Setenv("KUBECONFIG", originalKubeconfig)
		} else {
			_ = os.Unsetenv("KUBECONFIG")
		}
	}()

	t.Run("returns error when no config available", func(t *testing.T) {
		_ = os.Unsetenv("KUBECONFIG")
		_, err := k8sutil.GetConfig()
		if err == nil {
			t.Error("expected error when no config available")
		}
	})
}
