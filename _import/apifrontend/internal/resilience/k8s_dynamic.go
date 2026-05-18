//nolint:revive,gocritic // Interface implementation methods satisfy dynamic.Interface contract; signatures cannot be changed
package resilience

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

// ResilientDynamicClient wraps a dynamic.Interface and protects non-watch
// operations with a K8sCircuitBreaker. Watch operations bypass the CB
// by design (they are long-lived streaming operations).
type ResilientDynamicClient struct {
	inner dynamic.Interface
	cb    *K8sCircuitBreaker
}

// NewResilientDynamicClient creates a dynamic.Interface wrapper that routes
// Get, List, Create, Update, Patch, Delete through the circuit breaker.
func NewResilientDynamicClient(inner dynamic.Interface, cb *K8sCircuitBreaker) *ResilientDynamicClient {
	return &ResilientDynamicClient{inner: inner, cb: cb}
}

func (r *ResilientDynamicClient) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return &resilientNamespaceableResource{
		inner: r.inner.Resource(resource),
		cb:    r.cb,
	}
}

type resilientNamespaceableResource struct {
	inner dynamic.NamespaceableResourceInterface
	cb    *K8sCircuitBreaker
}

func (r *resilientNamespaceableResource) Namespace(ns string) dynamic.ResourceInterface {
	return &resilientResourceInterface{
		inner: r.inner.Namespace(ns),
		cb:    r.cb,
	}
}

func (r *resilientNamespaceableResource) Get(ctx context.Context, name string, opts metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
	var result *unstructured.Unstructured
	err := r.cb.Execute(ctx, func(ctx context.Context) error {
		var e error
		result, e = r.inner.Get(ctx, name, opts, subresources...)
		return e
	})
	return result, err
}

func (r *resilientNamespaceableResource) List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	var result *unstructured.UnstructuredList
	err := r.cb.Execute(ctx, func(ctx context.Context) error {
		var e error
		result, e = r.inner.List(ctx, opts)
		return e
	})
	return result, err
}

func (r *resilientNamespaceableResource) Create(ctx context.Context, obj *unstructured.Unstructured, opts metav1.CreateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	var result *unstructured.Unstructured
	err := r.cb.Execute(ctx, func(ctx context.Context) error {
		var e error
		result, e = r.inner.Create(ctx, obj, opts, subresources...)
		return e
	})
	return result, err
}

func (r *resilientNamespaceableResource) Update(ctx context.Context, obj *unstructured.Unstructured, opts metav1.UpdateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	var result *unstructured.Unstructured
	err := r.cb.Execute(ctx, func(ctx context.Context) error {
		var e error
		result, e = r.inner.Update(ctx, obj, opts, subresources...)
		return e
	})
	return result, err
}

func (r *resilientNamespaceableResource) UpdateStatus(ctx context.Context, obj *unstructured.Unstructured, opts metav1.UpdateOptions) (*unstructured.Unstructured, error) {
	var result *unstructured.Unstructured
	err := r.cb.Execute(ctx, func(ctx context.Context) error {
		var e error
		result, e = r.inner.UpdateStatus(ctx, obj, opts)
		return e
	})
	return result, err
}

func (r *resilientNamespaceableResource) Delete(ctx context.Context, name string, opts metav1.DeleteOptions, subresources ...string) error {
	return r.cb.Execute(ctx, func(ctx context.Context) error {
		return r.inner.Delete(ctx, name, opts, subresources...)
	})
}

func (r *resilientNamespaceableResource) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return r.cb.Execute(ctx, func(ctx context.Context) error {
		return r.inner.DeleteCollection(ctx, opts, listOpts)
	})
}

func (r *resilientNamespaceableResource) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*unstructured.Unstructured, error) {
	var result *unstructured.Unstructured
	err := r.cb.Execute(ctx, func(ctx context.Context) error {
		var e error
		result, e = r.inner.Patch(ctx, name, pt, data, opts, subresources...)
		return e
	})
	return result, err
}

// Watch bypasses the circuit breaker — long-lived streaming operations should
// not be subject to fail-fast.
//
//nolint:gocritic // hugeParam: signature matches dynamic.NamespaceableResourceInterface
func (r *resilientNamespaceableResource) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return r.inner.Watch(ctx, opts)
}

func (r *resilientNamespaceableResource) Apply(ctx context.Context, name string, obj *unstructured.Unstructured, opts metav1.ApplyOptions, subresources ...string) (*unstructured.Unstructured, error) {
	var result *unstructured.Unstructured
	err := r.cb.Execute(ctx, func(ctx context.Context) error {
		var e error
		result, e = r.inner.Apply(ctx, name, obj, opts, subresources...)
		return e
	})
	return result, err
}

func (r *resilientNamespaceableResource) ApplyStatus(ctx context.Context, name string, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
	var result *unstructured.Unstructured
	err := r.cb.Execute(ctx, func(ctx context.Context) error {
		var e error
		result, e = r.inner.ApplyStatus(ctx, name, obj, opts)
		return e
	})
	return result, err
}

// resilientResourceInterface wraps a namespaced dynamic.ResourceInterface with CB.
type resilientResourceInterface struct {
	inner dynamic.ResourceInterface
	cb    *K8sCircuitBreaker
}

func (r *resilientResourceInterface) Get(ctx context.Context, name string, opts metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
	var result *unstructured.Unstructured
	err := r.cb.Execute(ctx, func(ctx context.Context) error {
		var e error
		result, e = r.inner.Get(ctx, name, opts, subresources...)
		return e
	})
	return result, err
}

func (r *resilientResourceInterface) List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	var result *unstructured.UnstructuredList
	err := r.cb.Execute(ctx, func(ctx context.Context) error {
		var e error
		result, e = r.inner.List(ctx, opts)
		return e
	})
	return result, err
}

func (r *resilientResourceInterface) Create(ctx context.Context, obj *unstructured.Unstructured, opts metav1.CreateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	var result *unstructured.Unstructured
	err := r.cb.Execute(ctx, func(ctx context.Context) error {
		var e error
		result, e = r.inner.Create(ctx, obj, opts, subresources...)
		return e
	})
	return result, err
}

func (r *resilientResourceInterface) Update(ctx context.Context, obj *unstructured.Unstructured, opts metav1.UpdateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	var result *unstructured.Unstructured
	err := r.cb.Execute(ctx, func(ctx context.Context) error {
		var e error
		result, e = r.inner.Update(ctx, obj, opts, subresources...)
		return e
	})
	return result, err
}

func (r *resilientResourceInterface) UpdateStatus(ctx context.Context, obj *unstructured.Unstructured, opts metav1.UpdateOptions) (*unstructured.Unstructured, error) {
	var result *unstructured.Unstructured
	err := r.cb.Execute(ctx, func(ctx context.Context) error {
		var e error
		result, e = r.inner.UpdateStatus(ctx, obj, opts)
		return e
	})
	return result, err
}

func (r *resilientResourceInterface) Delete(ctx context.Context, name string, opts metav1.DeleteOptions, subresources ...string) error {
	return r.cb.Execute(ctx, func(ctx context.Context) error {
		return r.inner.Delete(ctx, name, opts, subresources...)
	})
}

func (r *resilientResourceInterface) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return r.cb.Execute(ctx, func(ctx context.Context) error {
		return r.inner.DeleteCollection(ctx, opts, listOpts)
	})
}

func (r *resilientResourceInterface) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*unstructured.Unstructured, error) {
	var result *unstructured.Unstructured
	err := r.cb.Execute(ctx, func(ctx context.Context) error {
		var e error
		result, e = r.inner.Patch(ctx, name, pt, data, opts, subresources...)
		return e
	})
	return result, err
}

// Watch bypasses the circuit breaker.
//
//nolint:gocritic // hugeParam: signature matches dynamic.ResourceInterface
func (r *resilientResourceInterface) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return r.inner.Watch(ctx, opts)
}

func (r *resilientResourceInterface) Apply(ctx context.Context, name string, obj *unstructured.Unstructured, opts metav1.ApplyOptions, subresources ...string) (*unstructured.Unstructured, error) {
	var result *unstructured.Unstructured
	err := r.cb.Execute(ctx, func(ctx context.Context) error {
		var e error
		result, e = r.inner.Apply(ctx, name, obj, opts, subresources...)
		return e
	})
	return result, err
}

func (r *resilientResourceInterface) ApplyStatus(ctx context.Context, name string, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
	var result *unstructured.Unstructured
	err := r.cb.Execute(ctx, func(ctx context.Context) error {
		var e error
		result, e = r.inner.ApplyStatus(ctx, name, obj, opts)
		return e
	})
	return result, err
}

// Compile-time interface checks.
var _ dynamic.Interface = (*ResilientDynamicClient)(nil)
var _ dynamic.NamespaceableResourceInterface = (*resilientNamespaceableResource)(nil)
var _ dynamic.ResourceInterface = (*resilientResourceInterface)(nil)
