# AuditEventRequestEventData

Service-specific event data as structured type. V2.0: Typed schemas documented below for API validation. Go client uses interface{} for clean code ergonomics. See DD-AUDIT-004 for structured type requirements. 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**original_payload** | **Dict[str, object]** | Full signal payload for RR.Spec.OriginalPayload reconstruction | [optional] 
**signal_labels** | **Dict[str, str]** | Signal labels for RR.Spec.SignalLabels reconstruction | [optional] 
**signal_annotations** | **Dict[str, str]** | Signal annotations for RR.Spec.SignalAnnotations reconstruction | [optional] 
**signal_type** | **str** | Signal type identifier for classification and metrics (prometheus-alert&#x3D;Prometheus AlertManager, kubernetes-event&#x3D;Kubernetes events) | 
**alert_name** | **str** | Name of the alert | 
**namespace** | **str** | Kubernetes namespace of the AIAnalysis | 
**fingerprint** | **str** | Unique identifier for the signal (deduplication) | 
**severity** | **str** | Severity level of the signal | [optional] 
**resource_kind** | **str** | Kubernetes resource kind | [optional] 
**resource_name** | **str** | Name of the affected Kubernetes resource | [optional] 
**remediation_request** | **str** | Created RemediationRequest reference (namespace/name) | [optional] 
**deduplication_status** | **str** | Whether this is a new or duplicate signal | [optional] 
**occurrence_count** | **int** | Number of times this signal has been seen | [optional] 
**error_details** | [**ErrorDetails**](ErrorDetails.md) |  | [optional] 
**rr_name** | **str** | Name of the RemediationRequest being orchestrated | 
**outcome** | **str** | Evaluation outcome | 
**duration_ms** | **int** | Evaluation duration in milliseconds | 
**failure_phase** | **str** | Phase where the failure occurred | [optional] 
**failure_reason** | **str** | Categorized failure reason | [optional] 
**from_phase** | **str** | Phase being transitioned from | [optional] 
**to_phase** | **str** | Phase being transitioned to | [optional] 
**transition_reason** | **str** | Reason for the transition | [optional] 
**rar_name** | **str** | Name of the RemediationApprovalRequest | [optional] 
**required_by** | **datetime** | Approval deadline (RFC3339) | [optional] 
**workflow_id** | **str** | Workflow ID being validated | 
**confidence_str** | **str** | Workflow selection confidence as string | [optional] 
**decision** | **str** | Decision made | 
**approved_by** | **str** | User who approved the request | [optional] 
**rejected_by** | **str** | User who rejected the request | [optional] 
**rejection_reason** | **str** | Reason for rejection | [optional] 
**message** | **str** | Additional message or context for the event | [optional] 
**reason** | **str** |  | 
**sub_reason** | **str** | Detailed failure sub-reason | [optional] 
**notification_name** | **str** | Alias for notification_id | [optional] 
**phase** | **str** | Phase in which error occurred | 
**signal** | **str** | Name of the signal being processed | 
**environment** | **str** | Environment context | 
**environment_source** | **str** | Source of the environment classification | [optional] 
**priority** | **str** |  | 
**priority_source** | **str** | Source of the priority assignment | [optional] 
**criticality** | **str** | Business criticality classification | [optional] 
**sla_requirement** | **str** | SLA requirement for remediation | [optional] 
**has_owner_chain** | **bool** | Whether the resource has an owner chain | [optional] 
**owner_chain_length** | **int** | Length of the owner chain | [optional] 
**degraded_mode** | **bool** | Whether operating in degraded mode | 
**has_pdb** | **bool** | Whether the resource has a PodDisruptionBudget | [optional] 
**has_hpa** | **bool** | Whether the resource has a HorizontalPodAutoscaler | [optional] 
**has_namespace** | **bool** | Whether namespace context was enriched | [optional] 
**has_pod** | **bool** | Whether pod context was enriched | [optional] 
**has_deployment** | **bool** | Whether deployment context was enriched | [optional] 
**business_unit** | **str** | Owning business unit | [optional] 
**error** | **str** |  | [optional] 
**analysis_name** | **str** | Name of the AIAnalysis CRD | 
**approval_required** | **bool** | Whether manual approval is required | 
**approval_reason** | **str** | Reason for approval requirement | 
**warnings_count** | **int** | Number of warnings encountered | 
**confidence** | **float** | Workflow confidence level (optional) | [optional] 
**target_in_owner_chain** | **bool** | Whether target is in owner chain | [optional] 
**provider_response_summary** | [**ProviderResponseSummary**](ProviderResponseSummary.md) |  | [optional] 
**workflow_version** | **str** | Version of the workflow being executed | 
**target_resource** | **str** | Kubernetes resource being acted upon (format depends on scope) | 
**container_image** | **str** | Tekton PipelineRun container image | 
**execution_name** | **str** | Name of the WorkflowExecution CRD | 
**started_at** | **datetime** | When the PipelineRun started execution | [optional] 
**completed_at** | **datetime** | When the PipelineRun finished (success or failure) | [optional] 
**duration** | **str** | Human-readable execution duration | [optional] 
**failure_message** | **str** | Detailed failure message from Tekton | [optional] 
**failed_task_name** | **str** | Name of the failed TaskRun (if identified) | [optional] 
**pipelinerun_name** | **str** | Name of the associated Tekton PipelineRun | [optional] 
**notification_id** | **str** |  | 
**type** | **str** | Notification type | 
**notification_type** | **str** | Alias for type (matches CRD NotificationType enum) | [optional] 
**final_status** | **str** | Final status of the notification (matches api/notification/v1alpha1/notificationrequest_types.go:60-65) | [optional] 
**recipients** | [**List[NotificationAuditPayloadRecipientsInner]**](NotificationAuditPayloadRecipientsInner.md) | Array of notification recipients from CRD (BR-NOTIFICATION-001, matches api/notification/v1alpha1/notificationrequest_types.go:80-102) | [optional] 
**cancelled_by** | **str** | Username who cancelled the notification | [optional] 
**user_uid** | **str** | UID of the user who performed the action | [optional] 
**user_groups** | **List[str]** | Groups of the user who performed the action | [optional] 
**action** | **str** | Webhook action performed | [optional] 
**workflow_name** | **str** | Name of workflow being validated | 
**clear_reason** | **str** | Reason for clearing the block | 
**cleared_at** | **datetime** | When the block was cleared | 
**previous_state** | **str** | State before unblocking (always \&quot;Blocked\&quot;) | 
**new_state** | **str** | State after unblocking (always \&quot;Running\&quot;) | 
**request_name** | **str** | Name of the RemediationApprovalRequest | 
**decided_at** | **datetime** | When the decision was made | 
**decision_message** | **str** | Reason for the decision | 
**ai_analysis_ref** | **str** | Name of the referenced AIAnalysis | 
**query** | [**QueryMetadata**](QueryMetadata.md) |  | 
**results** | [**ResultsMetadata**](ResultsMetadata.md) |  | 
**search_metadata** | [**SearchExecutionMetadata**](SearchExecutionMetadata.md) |  | 
**version** | **str** | Workflow version | 
**status** | **str** | Workflow status | 
**is_latest_version** | **bool** | Whether this is the latest version | 
**execution_engine** | **str** | Workflow execution engine | 
**name** | **str** | Display name | 
**description** | **str** | Workflow description | [optional] 
**labels** | **Dict[str, object]** | Workflow labels | [optional] 
**updated_fields** | [**WorkflowCatalogUpdatedFields**](WorkflowCatalogUpdatedFields.md) |  | 
**old_phase** | **str** | Previous phase | 
**new_phase** | **str** | New phase | 
**endpoint** | **str** | API endpoint called | 
**http_status_code** | **int** | HTTP status code | 
**auto_approved** | **bool** | Whether auto-approved | 
**degraded** | **bool** | Whether evaluation ran in degraded mode | 
**error_message** | **str** | Error message | 
**channel** | **str** |  | 
**subject** | **str** |  | 
**body** | **str** |  | 
**metadata** | **Dict[str, str]** |  | [optional] 
**error_type** | **str** |  | 
**event_id** | **str** | Unique event identifier | 
**incident_id** | **str** | Incident correlation ID (remediation_id) | 
**response_data** | [**IncidentResponseData**](IncidentResponseData.md) |  | 
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


