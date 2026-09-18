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

package registry

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

func probeClusters(
	ctx context.Context,
	client dynamic.Interface,
	namespace string,
	gvr schema.GroupVersionResource,
	started bool,
	stopped bool,
	trackable func(*unstructured.Unstructured) (ClusterInfo, bool),
) (map[string]ClusterInfo, error) {
	if stopped {
		return nil, fmt.Errorf("fleet cluster registry stopped")
	}
	if !started {
		return nil, fmt.Errorf("fleet cluster registry has not completed initial sync")
	}

	list, err := client.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: ManagedLabel + "=true"})
	if err != nil {
		return nil, fmt.Errorf("refresh fleet cluster registry: %w", err)
	}
	refreshed := make(map[string]ClusterInfo, len(list.Items))
	for i := range list.Items {
		if info, ok := trackable(&list.Items[i]); ok {
			refreshed[info.ID] = info
		}
	}
	return refreshed, nil
}
