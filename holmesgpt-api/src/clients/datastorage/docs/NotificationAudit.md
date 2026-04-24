# NotificationAudit

Notification audit record for tracking delivery attempts. Maps to `notification_audit` PostgreSQL table. 

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

## Example

```python
from datastorage.models.notification_audit import NotificationAudit

# TODO update the JSON string below
json = "{}"
# create an instance of NotificationAudit from a JSON string
notification_audit_instance = NotificationAudit.from_json(json)
# print the JSON string representation of the object
print NotificationAudit.to_json()

# convert the object into a dict
notification_audit_dict = notification_audit_instance.to_dict()
# create an instance of NotificationAudit from a dict
notification_audit_form_dict = notification_audit.from_dict(notification_audit_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


