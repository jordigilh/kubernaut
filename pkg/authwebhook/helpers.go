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

package authwebhook

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RetryGetCRD fetches a CRD object with exponential backoff. After an admission
// response, the CRD may not yet be committed to etcd by the API server; this
// helper retries up to maxRetries times (500ms, 1s, 2s, 4s...) to handle that race.
func RetryGetCRD(ctx context.Context, k8sClient client.Client, key types.NamespacedName, obj client.Object, maxRetries int) error {
	backoff := 500 * time.Millisecond
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("context expired waiting for CRD %s/%s after %d attempts: %w",
					key.Namespace, key.Name, attempt, ctx.Err())
			case <-time.After(backoff):
				backoff *= 2
			}
		}
		lastErr = k8sClient.Get(ctx, key, obj)
		if lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("failed to fetch CRD %s/%s after %d retries: %w",
		key.Namespace, key.Name, maxRetries, lastErr)
}
