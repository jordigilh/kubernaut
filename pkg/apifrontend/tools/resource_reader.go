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

package tools

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceReader abstracts GVR-based Kubernetes read access for kubectl tools.
// BR-FLEET-054: Enables fleet integration by allowing both dynamic.Interface
// (local cluster) and client.Reader (fleet/MCP gateway) to be used interchangeably.
//
// Both GVR and GVK are provided: DynamicResourceReader uses GVR for its native
// API, while ClientResourceReader uses GVK to set the correct type on the
// unstructured object for controller-runtime's client.Reader dispatch.
type ResourceReader interface {
	GetResource(ctx context.Context, gvr schema.GroupVersionResource, gvk schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, error)
	ListResources(ctx context.Context, gvr schema.GroupVersionResource, gvk schema.GroupVersionKind, namespace string, opts metav1.ListOptions) (*unstructured.UnstructuredList, error)
}

// DynamicResourceReader wraps dynamic.Interface to implement ResourceReader.
// Used for local-cluster access (the existing code path).
type DynamicResourceReader struct {
	Client dynamic.Interface
}

func (d *DynamicResourceReader) GetResource(ctx context.Context, gvr schema.GroupVersionResource, _ schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, error) {
	var resClient dynamic.ResourceInterface
	if namespace != "" {
		resClient = d.Client.Resource(gvr).Namespace(namespace)
	} else {
		resClient = d.Client.Resource(gvr)
	}
	return resClient.Get(ctx, name, metav1.GetOptions{})
}

func (d *DynamicResourceReader) ListResources(ctx context.Context, gvr schema.GroupVersionResource, _ schema.GroupVersionKind, namespace string, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	var resClient dynamic.ResourceInterface
	if namespace != "" {
		resClient = d.Client.Resource(gvr).Namespace(namespace)
	} else {
		resClient = d.Client.Resource(gvr)
	}
	return resClient.List(ctx, opts)
}

// ClientResourceReader wraps client.Reader (controller-runtime) to implement ResourceReader.
// BR-FLEET-054: Used for fleet/MCP gateway reads where the remote cluster is
// accessed via a client.Reader obtained from fleet.ReaderFactory.
type ClientResourceReader struct {
	Reader client.Reader
}

func (c *ClientResourceReader) GetResource(ctx context.Context, _ schema.GroupVersionResource, gvk schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	key := client.ObjectKey{Name: name, Namespace: namespace}
	if err := c.Reader.Get(ctx, key, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func (c *ClientResourceReader) ListResources(ctx context.Context, _ schema.GroupVersionResource, gvk schema.GroupVersionKind, namespace string, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gvk.Group,
		Version: gvk.Version,
		Kind:    gvk.Kind + "List",
	})
	var listOpts []client.ListOption
	if namespace != "" {
		listOpts = append(listOpts, client.InNamespace(namespace))
	}
	if opts.LabelSelector != "" {
		selector, err := metav1.ParseToLabelSelector(opts.LabelSelector)
		if err != nil {
			return nil, fmt.Errorf("invalid label selector %q: %w", opts.LabelSelector, err)
		}
		ls, err := metav1.LabelSelectorAsSelector(selector)
		if err != nil {
			return nil, fmt.Errorf("convert label selector: %w", err)
		}
		listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: ls})
	}
	if err := c.Reader.List(ctx, list, listOpts...); err != nil {
		return nil, err
	}
	return list, nil
}

// ResourceReaderFactory creates ResourceReader instances for a given cluster.
// BR-FLEET-054: Empty clusterID returns the local-cluster reader.
type ResourceReaderFactory func(ctx context.Context, clusterID string) (ResourceReader, error)

var _ ResourceReader = (*DynamicResourceReader)(nil)
var _ ResourceReader = (*ClientResourceReader)(nil)
