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

package errors

// PromptBudgetError identifies a rendered prompt that exceeded its configured
// byte budget before provider dispatch.
type PromptBudgetError struct {
	PromptBytes int
	BudgetBytes int
}

// Error implements error while preserving the stable failure classification
// for callers that use errors.As.
func (e *PromptBudgetError) Error() string {
	return "workflow selection prompt exceeds configured byte budget"
}
