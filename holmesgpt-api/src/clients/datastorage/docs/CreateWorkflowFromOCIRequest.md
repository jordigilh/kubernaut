# CreateWorkflowFromOCIRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**container_image** | **str** | OCI image pullspec. Data Storage pulls this image, extracts /workflow-schema.yaml (ADR-043), validates it, and populates all catalog fields from the extracted schema.  | 

## Example

```python
from datastorage.models.create_workflow_from_oci_request import CreateWorkflowFromOCIRequest

# TODO update the JSON string below
json = "{}"
# create an instance of CreateWorkflowFromOCIRequest from a JSON string
create_workflow_from_oci_request_instance = CreateWorkflowFromOCIRequest.from_json(json)
# print the JSON string representation of the object
print CreateWorkflowFromOCIRequest.to_json()

# convert the object into a dict
create_workflow_from_oci_request_dict = create_workflow_from_oci_request_instance.to_dict()
# create an instance of CreateWorkflowFromOCIRequest from a dict
create_workflow_from_oci_request_form_dict = create_workflow_from_oci_request.from_dict(create_workflow_from_oci_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


