/*
Copyright 2026 Jordi Gil.

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

package transport

import (
	"fmt"
	"os"
	"strings"
)

// ResolveValue returns the literal value unchanged. Used for non-sensitive
// headers like tenant IDs or correlation headers.
func ResolveValue(val string) string {
	return val
}

// ResolveSecretKeyRef reads a header value from an environment variable.
// In Kubernetes, secretKeyRef values are injected as env vars or volume mounts;
// this function handles the env var path.
func ResolveSecretKeyRef(envVar string) (string, error) {
	val := os.Getenv(envVar)
	if val == "" {
		return "", fmt.Errorf("secretKeyRef %q: environment variable is empty or unset", envVar)
	}
	return val, nil
}

// ResolveFilePath reads a header value from a file, trimming whitespace.
// The file is re-read on every call to support sidecar-rotated tokens
// without requiring a Kubernaut Agent restart.
func ResolveFilePath(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("filePath %q: %w", path, err)
	}
	val := strings.TrimSpace(string(data))
	if val == "" {
		return "", fmt.Errorf("filePath %q: file is empty", path)
	}
	return val, nil
}
