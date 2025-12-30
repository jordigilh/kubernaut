# datastorage.model.notification_audit.NotificationAudit

Notification audit record for tracking delivery attempts. Maps to `notification_audit` PostgreSQL table. 

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
dict, frozendict.frozendict,  | frozendict.frozendict,  | Notification audit record for tracking delivery attempts. Maps to &#x60;notification_audit&#x60; PostgreSQL table.  | 

### Dictionary Keys
Key | Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | ------------- | -------------
**sent_at** | str, datetime,  | str,  | Timestamp when notification was sent (RFC 3339 format). Used to calculate audit lag (time between event and audit write).  | value must conform to RFC-3339 date-time
**remediation_id** | str,  | str,  | Kubernetes RemediationRequest CRD name. Links notification to originating remediation workflow.  | 
**channel** | str,  | str,  | Notification delivery channel. Determines recipient format and delivery mechanism.  | must be one of ["email", "slack", "pagerduty", "webhook", ] 
**recipient** | str,  | str,  | Notification recipient (email address, Slack channel, PagerDuty key, etc.). Format depends on channel type.  | 
**notification_id** | str,  | str,  | Unique notification identifier from Notification Controller. Used as unique constraint to prevent duplicate audit records.  | 
**message_summary** | str,  | str,  | Human-readable summary of notification content. Truncated to 1000 characters for audit purposes.  | 
**status** | str,  | str,  | Notification delivery status from Notification Controller.  | must be one of ["sent", "failed", "pending", ] 
**delivery_status** | None, str,  | NoneClass, str,  | HTTP status code or delivery confirmation from notification channel. Optional - only present for sent notifications.  | [optional] 
**error_message** | None, str,  | NoneClass, str,  | Error message from notification channel if delivery failed. Optional - only present for failed notifications.  | [optional] 
**escalation_level** | decimal.Decimal, int,  | decimal.Decimal,  | Escalation level for notification (0 &#x3D; first attempt, 1+ &#x3D; escalated). Tracks how many times this incident has been escalated.  | [optional] 
**any_string_name** | dict, frozendict.frozendict, str, date, datetime, int, float, bool, decimal.Decimal, None, list, tuple, bytes, io.FileIO, io.BufferedReader | frozendict.frozendict, str, BoolClass, decimal.Decimal, NoneClass, tuple, bytes, FileIO | any string name can be used but the value must be the correct type | [optional]

[[Back to Model list]](../../README.md#documentation-for-models) [[Back to API list]](../../README.md#documentation-for-api-endpoints) [[Back to README]](../../README.md)

