// Package validate provides reusable input validation helpers for Kubernetes
// resource names and namespaces, wrapping k8s.io/apimachinery/pkg/util/validation
// to present user-friendly error messages.
package validate

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// Namespace validates that ns is a valid Kubernetes namespace (RFC 1123 DNS label).
func Namespace(ns string) error {
	if ns == "" {
		return fmt.Errorf("namespace must not be empty")
	}
	if errs := validation.IsDNS1123Label(ns); len(errs) > 0 {
		return fmt.Errorf("invalid namespace %q: %s", ns, strings.Join(errs, "; "))
	}
	return nil
}

// ResourceName validates that name is a valid Kubernetes resource name (RFC 1123 DNS subdomain).
func ResourceName(name string) error {
	if name == "" {
		return fmt.Errorf("resource name must not be empty")
	}
	if errs := validation.IsDNS1123Subdomain(name); len(errs) > 0 {
		return fmt.Errorf("invalid resource name %q: %s", name, strings.Join(errs, "; "))
	}
	return nil
}

// LabelValue validates that v is a valid Kubernetes label value.
func LabelValue(v string) error {
	if errs := validation.IsValidLabelValue(v); len(errs) > 0 {
		return fmt.Errorf("invalid label value %q: %s", v, strings.Join(errs, "; "))
	}
	return nil
}
