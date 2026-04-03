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

package cluster

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Identity holds the auto-discovered cluster name and UUID.
// Issue #615: Used by the Remediation Orchestrator to inject cluster context into notifications.
type Identity struct {
	Name string
	UUID string
}

var infrastructureGVK = schema.GroupVersionKind{
	Group:   "config.openshift.io",
	Version: "v1",
	Kind:    "Infrastructure",
}

const kindNodeLabel = "io.x-k8s.kind.cluster"

// DiscoverIdentity resolves cluster UUID (from kube-system namespace UID) and
// cluster name (OCP infrastructure > Kind node label > empty) at boot time.
func DiscoverIdentity(ctx context.Context, reader client.Reader) (*Identity, error) {
	id := &Identity{}

	var ns corev1.Namespace
	if err := reader.Get(ctx, types.NamespacedName{Name: "kube-system"}, &ns); err != nil {
		return id, fmt.Errorf("failed to read kube-system namespace: %w", err)
	}
	id.UUID = string(ns.UID)

	if name := discoverOCPName(ctx, reader); name != "" {
		id.Name = name
		return id, nil
	}

	if name := discoverKindName(ctx, reader); name != "" {
		id.Name = name
		return id, nil
	}

	return id, nil
}

func discoverOCPName(ctx context.Context, reader client.Reader) string {
	infra := &unstructured.Unstructured{}
	infra.SetGroupVersionKind(infrastructureGVK)
	if err := reader.Get(ctx, types.NamespacedName{Name: "cluster"}, infra); err != nil {
		return ""
	}
	name, _, _ := unstructured.NestedString(infra.Object, "status", "infrastructureName")
	return name
}

func discoverKindName(ctx context.Context, reader client.Reader) string {
	var nodes corev1.NodeList
	if err := reader.List(ctx, &nodes); err != nil || len(nodes.Items) == 0 {
		return ""
	}
	return nodes.Items[0].Labels[kindNodeLabel]
}
