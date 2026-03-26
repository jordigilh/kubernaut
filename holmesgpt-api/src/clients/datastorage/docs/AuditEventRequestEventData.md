# AuditEventRequestEventData

Service-specific event data as structured type. V2.0: Typed schemas documented below for API validation. Go client uses interface{} for clean code ergonomics. See DD-AUDIT-004 for structured type requirements. 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** |  | 
**original_payload** | **Dict[str, object]** | Full signal payload for RR.Spec.OriginalPayload reconstruction | [optional] 
**signal_labels** | **Dict[str, str]** | Signal labels for RR.Spec.SignalLabels reconstruction | [optional] 
**signal_annotations** | **Dict[str, str]** | Signal annotations for RR.Spec.SignalAnnotations reconstruction | [optional] 
**signal_type** | **str** | Signal type identifier. All adapters normalize to \&quot;alert\&quot;. Adapter identity is preserved in audit actor ID (signal.Source). | 
**signal_name** | **str** | Name of the original signal that triggered the remediation pipeline. Extracted from EA spec target resource context. Only present for assessment.completed events.  | 
**namespace** | **str** | Kubernetes namespace of the EffectivenessAssessment | 
**fingerprint** | **str** | Unique identifier for the signal (deduplication) | 
**severity** | **str** | Normalized severity level (DD-SEVERITY-001 v1.1) | [optional] 
**resource_kind** | **str** | Kubernetes resource kind | [optional] 
**resource_name** | **str** | Name of the affected Kubernetes resource | [optional] 
**remediation_request** | **str** | Created RemediationRequest reference (namespace/name) | [optional] 
**deduplication_status** | **str** | Whether this is a new or duplicate signal | [optional] 
**occurrence_count** | **int** | Number of times this signal has been seen | [optional] 
**error_details** | [**ErrorDetails**](ErrorDetails.md) |  | [optional] 
**rr_name** | **str** | Name of the RemediationRequest | 
**outcome** | **str** | Evaluation outcome | 
**duration_ms** | **int** | Evaluation duration in milliseconds | 
**failure_phase** | **str** | Phase where the failure occurred | [optional] 
**failure_reason** | **str** | Categorized failure reason | [optional] 
**from_phase** | **str** | Phase being transitioned from | [optional] 
**to_phase** | **str** | Phase being transitioned to | [optional] 
**transition_reason** | **str** | Reason for the transition | [optional] 
**rar_name** | **str** | Name of the RemediationApprovalRequest | [optional] 
**required_by** | **datetime** | Approval deadline (RFC3339) | [optional] 
**workflow_id** | **str** | DataStorage catalog UUID (set after successful registration) | 
**confidence_str** | **str** | Workflow selection confidence as string | [optional] 
**decision** | **str** | Decision made | 
**approved_by** | **str** | User who approved the request | [optional] 
**rejected_by** | **str** | User who rejected the request | [optional] 
**rejection_reason** | **str** | Reason for rejection | [optional] 
**message** | **str** | Additional message or context for the event | [optional] 
**reason** | **str** | Assessment completion reason (only for assessment.completed events) | 
**sub_reason** | **str** | Detailed failure sub-reason | [optional] 
**notification_name** | **str** | Alias for notification_id | [optional] 
**timeout_config** | [**TimeoutConfig**](TimeoutConfig.md) |  | [optional] 
**pre_remediation_spec_hash** | **str** | Canonical SHA-256 hash of the target resource&#39;s .spec BEFORE remediation. Retrieved from DataStorage audit trail (remediation.workflow_created event). Format: \&quot;sha256:&lt;hex&gt;\&quot;. Only present for effectiveness.hash.computed events.  | [optional] 
**target_resource** | **str** | Target resource being remediated | 
**workflow_version** | **str** | Workflow version | 
**workflow_type** | **str** | Action type from DD-WORKFLOW-016 taxonomy (e.g., ScaleReplicas, RestartPod). Propagated from AIAnalysis.SelectedWorkflow.ActionType via HAPI three-step discovery. Used by DS remediation history to populate workflowType on entries and summaries.  | [optional] 
**ea_name** | **str** | Name of the EffectivenessAssessment CRD | [optional] 
**hash_compute_delay** | **str** | Duration-based hash compute delay from EA spec config (Issue #277). The EM computes hash_compute_after &#x3D; EA.creationTimestamp + hash_compute_delay. Present only for async-managed targets. Format: Go duration string. Reference: DD-EM-004, BR-EM-010, BR-RO-103, Issue #277  | [optional] 
**alert_check_delay** | **str** | Duration-based alert check delay from EA spec config (Issue #277). Set by the RO for proactive signals where the triggering alert needs extra time to resolve after remediation. Format: Go duration string. Reference: BR-EM-009, BR-RO-103, Issue #277  | [optional] 
**gitops_sync_delay** | **str** | DEPRECATED (Issue #277): Use orchestrator.ea.created event instead. Was: GitOps sync delay propagated via EA spec. Now emitted by the RO in the orchestrator.ea.created audit event.  | [optional] 
**operator_reconcile_delay** | **str** | DEPRECATED (Issue #277): Use orchestrator.ea.created event instead. Was: Operator reconcile delay propagated via EA spec. Now emitted by the RO in the orchestrator.ea.created audit event.  | [optional] 
**is_gitops_managed** | **bool** | Whether the remediation target was detected as GitOps-managed. Only present for orchestrator.ea.created events.  | [optional] 
**is_crd** | **bool** | Whether the remediation target is a CRD (non-built-in group). Only present for orchestrator.ea.created events.  | [optional] 
**phase** | **str** | Phase in which the failure occurred | 
**signal** | **str** | Name of the signal being processed | 
**external_severity** | **str** | Original severity from external monitoring system (e.g., Sev1, P0, critical) | [optional] 
**normalized_severity** | **str** | Normalized severity determined by Rego policy (DD-SEVERITY-001 v1.1) | [optional] 
**determination_source** | **str** | Source of severity determination for audit trail | [optional] 
**policy_hash** | **str** | SHA256 hash of Rego policy used for severity determination (for audit trail and policy version tracking) | [optional] 
**environment** | **str** | Environment context | 
**environment_source** | **str** | Source of the environment classification | [optional] 
**priority** | **str** |  | 
**priority_source** | **str** | Source of the priority assignment | [optional] 
**criticality** | **str** | Business criticality classification | [optional] 
**sla_requirement** | **str** | SLA requirement for remediation | [optional] 
**has_owner_chain** | **bool** | Whether the resource has an owner chain | [optional] 
**owner_chain_length** | **int** | Number of resources in the K8s owner chain (1 &#x3D; no parent) | 
**degraded_mode** | **bool** | Whether operating in degraded mode | 
**has_namespace** | **bool** | Whether namespace context was enriched | [optional] 
**has_pod** | **bool** | Whether pod context was enriched | [optional] 
**has_deployment** | **bool** | Whether deployment context was enriched | [optional] 
**business_unit** | **str** | Owning business unit | [optional] 
**signal_mode** | **str** | Whether this signal is reactive (incident occurred) or proactive (incident anticipated). BR-SP-106 Proactive Signal Mode Classification. | [optional] 
**source_signal_name** | **str** | Original signal name before normalization. Only populated for proactive signals (e.g., PredictedOOMKill). SOC2 CC7.4 audit trail preservation. | [optional] 
**error** | **str** |  | [optional] 
**analysis_name** | **str** | Name of the AIAnalysis CRD | 
**approval_required** | **bool** | Whether manual approval is required | 
**approval_reason** | **str** | Reason for approval requirement | 
**warnings_count** | **int** | Number of warnings encountered | 
**confidence** | **float** | Workflow confidence level (optional) | 
**provider_response_summary** | [**ProviderResponseSummary**](ProviderResponseSummary.md) |  | [optional] 
**container_image** | **str** | Tekton PipelineRun container image | 
**execution_name** | **str** | Name of the WorkflowExecution CRD | 
**started_at** | **datetime** | When the PipelineRun started execution | [optional] 
**completed_at** | **datetime** | Timestamp when the assessment completed (EA status.completedAt). Only present for assessment.completed events.  | [optional] 
**duration** | **str** | Human-readable execution duration | [optional] 
**failure_message** | **str** | Detailed failure message from Tekton | [optional] 
**failed_task_name** | **str** | Name of the failed TaskRun (if identified) | [optional] 
**pipelinerun_name** | **str** | Name of the associated Tekton PipelineRun | [optional] 
**parameters** | **Dict[str, str]** | Post-normalization workflow parameters applied to PipelineRun (map[string]string). SOC2 CC7.1-CC7.3 chain of custody. | [optional] 
**notification_id** | **str** |  | 
**type** | **str** | Notification type | 
**notification_type** | **str** | Alias for type (matches CRD NotificationType enum) | [optional] 
**final_status** | **str** | Final status of the notification (matches api/notification/v1alpha1/notificationrequest_types.go:60-65) | [optional] 
**delivery_channels** | **List[str]** | Channels the notification was delivered to, extracted from status.deliveryAttempts (e.g. \&quot;slack\&quot;, \&quot;console\&quot;, \&quot;file\&quot;) | [optional] 
**cancelled_by** | **str** | Username who cancelled the notification | [optional] 
**user_uid** | **str** | UID of the user who performed the action | [optional] 
**user_groups** | **List[str]** | Groups of the user who performed the action | [optional] 
**action** | **str** |  | 
**workflow_name** | **str** | Name of the RemediationWorkflow CRD (metadata.name) | 
**clear_reason** | **str** | Reason for clearing the block | 
**cleared_at** | **datetime** | When the block was cleared | 
**previous_state** | **str** |  | 
**new_state** | **str** | State after unblocking (always \&quot;Running\&quot;) | 
**request_name** | **str** | Name of the RemediationApprovalRequest | 
**decided_at** | **datetime** | When decision was made | 
**decision_message** | **str** | Optional rationale from operator | 
**ai_analysis_ref** | **str** | Name of the referenced AIAnalysis | 
**remediation_request_name** | **str** | Parent RemediationRequest name (correlation ID) | 
**ai_analysis_name** | **str** | AIAnalysis CRD that required approval | 
**decided_by** | **str** | Authenticated username from webhook (SOC 2 CC8.1) | 
**timeout_deadline** | **datetime** | Approval deadline | [optional] 
**decision_duration_seconds** | **int** | Time to decision (seconds) | [optional] 
**timeout_reason** | **str** | Reason for timeout (for timeout event) | [optional] 
**timeout_duration_seconds** | **int** | Timeout duration in seconds (for timeout event) | [optional] 
**query** | [**QueryMetadata**](QueryMetadata.md) |  | 
**results** | [**ResultsMetadata**](ResultsMetadata.md) |  | 
**search_metadata** | [**SearchExecutionMetadata**](SearchExecutionMetadata.md) |  | 
**version** | **str** | Workflow version | 
**schema_version** | **str** | Schema format version (e.g., 1.0, 1.1). #255 | [optional] 
**status** | **str** | Workflow status | 
**is_latest_version** | **bool** | Whether this is the latest version | 
**execution_engine** | **str** | Workflow execution engine | 
**name** | **str** | Display name | 
**description** | [**ActionTypeDescriptionPayload**](ActionTypeDescriptionPayload.md) |  | 
**labels** | **Dict[str, object]** | Workflow labels | [optional] 
**updated_fields** | **List[str]** |  | 
**old_phase** | **str** | Previous phase | 
**new_phase** | **str** | New phase | 
**endpoint** | **str** | API endpoint called | 
**http_status_code** | **int** | HTTP status code | 
**auto_approved** | **bool** | Whether auto-approved | 
**degraded** | **bool** | Whether evaluation ran in degraded mode | 
**error_message** | **str** | Error message from the failed investigation | 
**channel** | **str** |  | 
**subject** | **str** |  | 
**body** | **str** |  | 
**metadata** | **Dict[str, str]** |  | [optional] 
**error_type** | **str** |  | 
**event_id** | **str** | Unique event identifier | 
**incident_id** | **str** | Incident correlation ID (remediation_id) | 
**response_data** | [**IncidentResponseData**](IncidentResponseData.md) |  | 
**duration_seconds** | **float** | Duration of the investigation before failure (seconds) | [optional] 
**root_owner_kind** | **str** | Resolved root owner resource kind (e.g., Deployment, StatefulSet) | 
**root_owner_name** | **str** | Resolved root owner resource name | 
**root_owner_namespace** | **str** | Resolved root owner namespace (empty for cluster-scoped resources) | [optional] 
**detected_labels_summary** | **object** | Infrastructure labels detected by LabelDetector (null when detector unavailable) | [optional] 
**failed_detections** | **List[str]** | Labels that could not be detected (null when all succeeded or detector unavailable) | [optional] 
**remediation_history_fetched** | **bool** | Whether remediation history was successfully fetched from DataStorage | 
**detail** | **str** | Detailed failure context including retry information | 
**affected_resource_kind** | **str** | Kind of the resource that was being enriched when failure occurred | 
**affected_resource_name** | **str** | Name of the resource that was being enriched when failure occurred | 
**affected_resource_namespace** | **str** | Namespace of the resource (empty for cluster-scoped) | [optional] 
**model** | **str** | LLM model identifier | 
**prompt_length** | **int** | Length of prompt sent to LLM | 
**prompt_preview** | **str** | First 500 characters of prompt for audit | 
**max_tokens** | **int** | Maximum tokens requested | [optional] 
**toolsets_enabled** | **List[str]** | List of enabled toolsets | [optional] 
**mcp_servers** | **List[str]** | List of MCP servers | [optional] 
**has_analysis** | **bool** | Whether LLM provided analysis | 
**analysis_length** | **int** | Length of LLM response | 
**analysis_preview** | **str** | First 500 characters of response for audit | 
**tokens_used** | **int** | Tokens consumed by LLM | [optional] 
**tool_call_count** | **int** | Number of tool calls made by LLM | [optional] [default to 0]
**tool_call_index** | **int** | Sequential index of tool call in conversation | 
**tool_name** | **str** | Name of tool invoked | 
**tool_arguments** | **Dict[str, object]** | Arguments passed to tool (flexible for different tools) | [optional] 
**tool_result** | **object** | Full result returned by tool | 
**tool_result_preview** | **str** | First 500 characters of tool result | [optional] 
**attempt** | **int** | Current validation attempt number | 
**max_attempts** | **int** | Maximum validation attempts allowed | 
**is_valid** | **bool** | Whether validation succeeded | 
**errors** | **List[str]** | List of validation error messages | [optional] 
**validation_errors** | **str** | Combined validation error messages (for backward compatibility) | [optional] 
**human_review_reason** | **str** | Reason code if needs_human_review (final attempt) | [optional] 
**is_final_attempt** | **bool** | Whether this is the final validation attempt | [optional] [default to False]
**modified_by** | **str** | User who modified the timeout configuration | 
**modified_at** | **datetime** | When the modification occurred | 
**old_timeout_config** | [**TimeoutConfig**](TimeoutConfig.md) |  | [optional] 
**new_timeout_config** | [**TimeoutConfig**](TimeoutConfig.md) |  | [optional] 
**catalog_status** | **str** |  | [optional] 
**denial_reason** | **str** |  | [optional] 
**correlation_id** | **str** | Correlation ID (EA spec.correlationID, matches parent RR name) | 
**component** | **str** | Assessment component that produced this event | 
**assessed** | **bool** | Whether the component was successfully assessed | [optional] 
**score** | **float** | Component score (0.0-1.0), null if not assessed | [optional] 
**details** | **str** | Human-readable details about the assessment result | [optional] 
**components_assessed** | **List[str]** | List of component names that were assessed (e.g. [\&quot;health\&quot;,\&quot;hash\&quot;,\&quot;alert\&quot;,\&quot;metrics\&quot;]). Only present for assessment.completed events.  | [optional] 
**assessment_duration_seconds** | **float** | Seconds from RemediationRequest creation to assessment completion. Computed as (completedAt - remediationCreatedAt). Null if remediationCreatedAt is not set. Only present for assessment.completed events. Distinct from alert_resolution.resolution_time_seconds which measures alert-level resolution.  | [optional] 
**validity_deadline** | **datetime** | Computed validity deadline (only for assessment.scheduled events). EA.creationTimestamp + validityWindow from EM config.  | [optional] 
**prometheus_check_after** | **datetime** | Computed earliest time for Prometheus check (only for assessment.scheduled events). EA.creationTimestamp + stabilizationWindow.  | [optional] 
**alertmanager_check_after** | **datetime** | Computed earliest time for AlertManager check (only for assessment.scheduled events). EA.creationTimestamp + stabilizationWindow.  | [optional] 
**hash_compute_after** | **datetime** | Earliest time for hash computation (only for assessment.scheduled events). Set by the RO for async-managed targets (GitOps, operator CRDs) where spec changes propagate after the WorkflowExecution completes. Nil/absent means hash is computed immediately (sync targets). Reference: DD-EM-004, BR-EM-010, BR-RO-103  | [optional] 
**total_propagation_delay** | **str** | DEPRECATED (Issue #277): Use hash_compute_delay and alert_check_delay instead. Was: Sum of gitops_sync_delay + operator_reconcile_delay.  | [optional] 
**validity_window** | **str** | Validity window duration from EM config (only for assessment.scheduled events). Included for operational observability.  | [optional] 
**stabilization_window** | **str** | Stabilization window duration from EA spec (only for assessment.scheduled events). Included for operational observability.  | [optional] 
**post_remediation_spec_hash** | **str** | Canonical SHA-256 hash of the target resource&#39;s .spec AFTER remediation. Computed by EM during assessment using DD-EM-002 canonical hash algorithm. Format: \&quot;sha256:&lt;hex&gt;\&quot;. Only present for effectiveness.hash.computed events.  | [optional] 
**hash_match** | **bool** | Whether the pre and post remediation spec hashes match. true &#x3D; no change detected (remediation may have been reverted or had no effect). false &#x3D; spec changed (expected for successful remediations). Only present for effectiveness.hash.computed events.  | [optional] 
**health_checks** | [**EffectivenessAssessmentAuditPayloadHealthChecks**](EffectivenessAssessmentAuditPayloadHealthChecks.md) |  | [optional] 
**metric_deltas** | [**EffectivenessAssessmentAuditPayloadMetricDeltas**](EffectivenessAssessmentAuditPayloadMetricDeltas.md) |  | [optional] 
**alert_resolution** | [**EffectivenessAssessmentAuditPayloadAlertResolution**](EffectivenessAssessmentAuditPayloadAlertResolution.md) |  | [optional] 
**action_type** | **str** |  | 
**registered_by** | **str** |  | 
**was_reenabled** | **bool** |  | 
**old_description** | [**ActionTypeDescriptionPayload**](ActionTypeDescriptionPayload.md) |  | 
**new_description** | [**ActionTypeDescriptionPayload**](ActionTypeDescriptionPayload.md) |  | 
**updated_by** | **str** |  | 
**disabled_by** | **str** |  | 
**disabled_at** | **datetime** |  | 
**reenabled_by** | **str** |  | 
**denied_reason** | **str** |  | 
**dependent_workflow_count** | **int** |  | 
**dependent_workflows** | **List[str]** |  | 
**requested_by** | **str** |  | 
**action_type_name** | **str** | PascalCase name from spec.name | 
**crd_name** | **str** | K8s metadata.name | 
**crd_namespace** | **str** | K8s namespace | 
**previously_existed** | **bool** |  | [optional] 
**denial_operation** | **str** |  | [optional] 

## Example

```python
from datastorage.models.audit_event_request_event_data import AuditEventRequestEventData

# TODO update the JSON string below
json = "{}"
# create an instance of AuditEventRequestEventData from a JSON string
audit_event_request_event_data_instance = AuditEventRequestEventData.from_json(json)
# print the JSON string representation of the object
print AuditEventRequestEventData.to_json()

# convert the object into a dict
audit_event_request_event_data_dict = audit_event_request_event_data_instance.to_dict()
# create an instance of AuditEventRequestEventData from a dict
audit_event_request_event_data_form_dict = audit_event_request_event_data.from_dict(audit_event_request_event_data_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


