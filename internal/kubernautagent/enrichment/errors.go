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

package enrichment

import (
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IsNotFoundError returns true when err (or any error in its chain) is a
// Kubernetes NotFound status error. Uses errors.As to handle wrapping by
// K8sAdapter (fmt.Errorf("k8s adapter: get %s/%s in %s: %w", ...)).
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	var statusErr *apierrors.StatusError
	if errors.As(err, &statusErr) {
		return statusErr.Status().Reason == metav1.StatusReasonNotFound
	}
	return false
}
