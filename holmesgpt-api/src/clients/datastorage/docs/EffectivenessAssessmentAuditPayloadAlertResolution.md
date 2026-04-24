# EffectivenessAssessmentAuditPayloadAlertResolution

Structured alert resolution check results from AlertManager. Only present for effectiveness.alert.assessed events. 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**alert_resolved** | **bool** | Whether the triggering alert is no longer active in AlertManager | [optional] 
**active_count** | **int** | Number of matching active alerts found in AlertManager | [optional] 
**resolution_time_seconds** | **float** | Seconds from remediation completion to alert resolution (null if not resolved) | [optional] 

## Example

```python
from datastorage.models.effectiveness_assessment_audit_payload_alert_resolution import EffectivenessAssessmentAuditPayloadAlertResolution

# TODO update the JSON string below
json = "{}"
# create an instance of EffectivenessAssessmentAuditPayloadAlertResolution from a JSON string
effectiveness_assessment_audit_payload_alert_resolution_instance = EffectivenessAssessmentAuditPayloadAlertResolution.from_json(json)
# print the JSON string representation of the object
print EffectivenessAssessmentAuditPayloadAlertResolution.to_json()

# convert the object into a dict
effectiveness_assessment_audit_payload_alert_resolution_dict = effectiveness_assessment_audit_payload_alert_resolution_instance.to_dict()
# create an instance of EffectivenessAssessmentAuditPayloadAlertResolution from a dict
effectiveness_assessment_audit_payload_alert_resolution_form_dict = effectiveness_assessment_audit_payload_alert_resolution.from_dict(effectiveness_assessment_audit_payload_alert_resolution_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


