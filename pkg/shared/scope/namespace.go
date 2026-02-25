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

package scope

import (
	"fmt"
	"os"
	"strings"
)

// NamespaceFilePath is the path to the Kubernetes service account namespace file.
// Exported to allow tests to override it with a temp file.
//
// ADR-057: CRD Namespace Consolidation — controller namespace discovery.
var NamespaceFilePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

const controllerNamespaceEnvVar = "KUBERNAUT_CONTROLLER_NAMESPACE"

// GetControllerNamespace returns the namespace where kubernaut controllers are deployed.
//
// Discovery order:
//  1. KUBERNAUT_CONTROLLER_NAMESPACE environment variable (for local dev / testing)
//  2. Kubernetes service account namespace file (standard in-cluster discovery)
//
// ADR-057: All kubernaut CRDs are consolidated into the controller namespace.
// Controllers call this at startup to determine the namespace for CRD creation
// and watch restriction via Cache.ByObject.
func GetControllerNamespace() (string, error) {
	if val, ok := os.LookupEnv(controllerNamespaceEnvVar); ok {
		ns := strings.TrimSpace(val)
		if ns == "" {
			return "", fmt.Errorf("controller namespace: %s is set but empty", controllerNamespaceEnvVar)
		}
		return ns, nil
	}

	data, err := os.ReadFile(NamespaceFilePath)
	if err != nil {
		return "", fmt.Errorf("controller namespace: cannot read service account namespace file %s: %w", NamespaceFilePath, err)
	}

	ns := strings.TrimSpace(string(data))
	if ns == "" {
		return "", fmt.Errorf("controller namespace: service account namespace file %s is empty", NamespaceFilePath)
	}
	return ns, nil
}
