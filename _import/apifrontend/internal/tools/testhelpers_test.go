package tools_test

import (
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func newForbiddenError(resource string) *errors.StatusError {
	return errors.NewForbidden(
		schema.GroupResource{Group: "kubernaut.ai", Resource: resource},
		"",
		nil,
	)
}
