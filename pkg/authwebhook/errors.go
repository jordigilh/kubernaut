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

import "errors"

// ErrActionTypeCRDNotFound indicates no ActionType CRD in the namespace has a
// spec.name matching the queried action type. Issue #1674: typed sentinel
// replacing the previous ambiguous (nil, nil) return from findActionTypeKey.
var ErrActionTypeCRDNotFound = errors.New("ActionType CRD not found")

// PermanentError indicates a non-retryable error from DataStorage.
// HTTP 400 (validation), 403 (forbidden), 404 (not found) are permanent —
// retrying will not produce a different result.
type PermanentError struct {
	msg string
}

func (e *PermanentError) Error() string {
	return e.msg
}

// NewPermanentError creates a PermanentError with the given message.
func NewPermanentError(msg string) error {
	return &PermanentError{msg: msg}
}

// IsPermanentError returns true if the error (or any wrapped error) is a PermanentError.
func IsPermanentError(err error) bool {
	var pe *PermanentError
	return errors.As(err, &pe)
}
