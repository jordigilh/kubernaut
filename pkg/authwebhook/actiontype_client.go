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

// ActionTypeCatalogClient is now an empty marker interface. #1661 Change 8d
// removed all four of its methods (CreateActionType, UpdateActionType,
// DisableActionType, ForceDisableActionType) -- ActionTypeHandler computes
// and patches everything locally with zero DS round-trips, mirroring
// WorkflowCatalogClient's Change 8c precedent (remediationworkflow_handler.go).
// The dsClient field/constructor param is kept (rather than removed
// outright) to avoid an unrelated, high-blast-radius signature change across
// every test call site and cmd/authwebhook/main.go in this REFACTOR pass;
// full removal is deferred to Phase 55 alongside the DS-side mutation
// handler deletion, once ActionTypeCatalogClient has zero remaining
// implementers to migrate.
type ActionTypeCatalogClient interface{}
