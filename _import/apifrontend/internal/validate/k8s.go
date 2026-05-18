// Package validate provides reusable input validation helpers for Kubernetes
// resource names and namespaces, wrapping k8s.io/apimachinery/pkg/util/validation
// to present user-friendly error messages.
package validate

import (
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

var kindRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

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

// Kind validates that k is a valid Kubernetes resource kind (PascalCase identifier, ASCII alphanumeric only).
func Kind(k string) error {
	if k == "" {
		return fmt.Errorf("kind must not be empty")
	}
	if len(k) > 63 {
		return fmt.Errorf("kind %q exceeds max length 63", k)
	}
	if !kindRE.MatchString(k) {
		return fmt.Errorf("invalid kind %q: must start with letter and contain only ASCII alphanumeric characters", k)
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
