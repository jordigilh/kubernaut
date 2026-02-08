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

// Package k8serrors provides helpers for classifying Kubernetes controller errors
// that lack typed error types in the K8s API or controller-runtime.
//
// SOC2 Round 2 M-6: Centralizes string-based error checks that were duplicated
// across multiple controllers, providing a single point of maintenance.
package k8serrors

import "strings"

// IsIndexerConflict checks if the error is an "indexer conflict" error from
// controller-runtime's field indexer. This occurs when two controllers register
// the same field index (expected behavior in multi-controller setups).
//
// Note: controller-runtime does not expose a typed error for this case.
func IsIndexerConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), "indexer conflict")
}

// IsNamespaceTerminating checks if the error indicates that the target namespace
// is being terminated. This is a benign race condition during async cleanup.
//
// Note: The K8s API server returns this as an unstructured error message.
func IsNamespaceTerminating(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "namespace") &&
		strings.Contains(errMsg, "being terminated")
}
