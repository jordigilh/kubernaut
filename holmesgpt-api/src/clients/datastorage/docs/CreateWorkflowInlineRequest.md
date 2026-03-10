# CreateWorkflowInlineRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**content** | **str** | Raw YAML content of the RemediationWorkflow CRD. Data Storage parses the CRD envelope (apiVersion/kind/metadata/spec), validates the spec, and populates all catalog fields from it.  | 
**source** | **str** | Registration source. Set to \&quot;crd\&quot; when the request originates from the Auth Webhook on CRD creation, or \&quot;api\&quot; for direct API calls.  | [optional] 
**registered_by** | **str** | Identity of the user or service account that triggered the registration. Populated from AdmissionReview.request.userInfo.username when source is \&quot;crd\&quot;.  | [optional] 

## Example

```python
from datastorage.models.create_workflow_inline_request import CreateWorkflowInlineRequest

# TODO update the JSON string below
json = "{}"
# create an instance of CreateWorkflowInlineRequest from a JSON string
create_workflow_inline_request_instance = CreateWorkflowInlineRequest.from_json(json)
# print the JSON string representation of the object
print CreateWorkflowInlineRequest.to_json()

# convert the object into a dict
create_workflow_inline_request_dict = create_workflow_inline_request_instance.to_dict()
# create an instance of CreateWorkflowInlineRequest from a dict
create_workflow_inline_request_form_dict = create_workflow_inline_request.from_dict(create_workflow_inline_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


