package creator

import "github.com/jordigilh/kubernaut/pkg/shared/parentlifecycle"

// ChildCollisionError is retained as a package-level alias for RO callers.
type ChildCollisionError = parentlifecycle.ChildCollisionError

// IsChildCollision reports whether err is a DD-2582 child provenance
// collision, including errors wrapped by a controller or phase handler.
func IsChildCollision(err error) bool {
	return parentlifecycle.IsChildCollision(err)
}

func newChildCollision(kind, namespace, name string, cause error) error {
	return parentlifecycle.NewChildCollisionError(kind, namespace, name, cause)
}
