# NotificationAuditResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**remediation_id** | **str** | Kubernetes RemediationRequest CRD name. Links notification to originating remediation workflow.  | 
**notification_id** | **str** | Unique notification identifier from Notification Controller. Used as unique constraint to prevent duplicate audit records.  | 
**recipient** | **str** | Notification recipient (email address, Slack channel, PagerDuty key, etc.). Format depends on channel type.  | 
**channel** | **str** | Notification delivery channel. Determines recipient format and delivery mechanism.  | 
**message_summary** | **str** | Human-readable summary of notification content. Truncated to 1000 characters for audit purposes.  | 
**status** | **str** | Notification delivery status from Notification Controller.  | 
**sent_at** | **datetime** | Timestamp when notification was sent (RFC 3339 format). Used to calculate audit lag (time between event and audit write).  | 
**delivery_status** | **str** | HTTP status code or delivery confirmation from notification channel. Optional - only present for sent notifications.  | [optional] 
**error_message** | **str** | Error message from notification channel if delivery failed. Optional - only present for failed notifications.  | [optional] 
**escalation_level** | **int** | Escalation level for notification (0 &#x3D; first attempt, 1+ &#x3D; escalated). Tracks how many times this incident has been escalated.  | [optional] 
**id** | **int** | Auto-generated primary key from PostgreSQL.  | 
**created_at** | **datetime** | Timestamp when record was created in PostgreSQL.  | 
**updated_at** | **datetime** | Timestamp when record was last updated in PostgreSQL.  | 

## Example

```python
from datastorage.models.notification_audit_response import NotificationAuditResponse

# TODO update the JSON string below
json = "{}"
# create an instance of NotificationAuditResponse from a JSON string
notification_audit_response_instance = NotificationAuditResponse.from_json(json)
# print the JSON string representation of the object
print NotificationAuditResponse.to_json()

# convert the object into a dict
notification_audit_response_dict = notification_audit_response_instance.to_dict()
# create an instance of NotificationAuditResponse from a dict
notification_audit_response_form_dict = notification_audit_response.from_dict(notification_audit_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


