# ExecutionFailure

Structured failure information using Kubernetes reason codes.  CRITICAL: The 'reason' field uses canonical Kubernetes reason codes as the API contract. This is NOT natural language - it's a structured enum-like value.  Valid reason codes include: - Resource: OOMKilled, InsufficientCPU, InsufficientMemory, Evicted - Scheduling: FailedScheduling, Unschedulable - Image: ImagePullBackOff, ErrImagePull, InvalidImageName - Execution: DeadlineExceeded, BackoffLimitExceeded, Error - Permission: Unauthorized, Forbidden - Volume: FailedMount, FailedAttachVolume - Node: NodeNotReady, NodeUnreachable - Network: NetworkNotReady

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**failed_step_index** | **int** | 0-indexed step that failed | 
**failed_step_name** | **str** | Name of the failed step | 
**reason** | **str** | Kubernetes reason code (e.g., &#39;OOMKilled&#39;, &#39;DeadlineExceeded&#39;). NOT natural language. | 
**message** | **str** | Human-readable error message (for logging/debugging) | 
**exit_code** | **int** |  | [optional] 
**failed_at** | **str** | ISO timestamp of failure | 
**execution_time** | **str** | Duration before failure (e.g., &#39;2m34s&#39;) | 

## Example

```python
from holmesgpt_api_client.models.execution_failure import ExecutionFailure

# TODO update the JSON string below
json = "{}"
# create an instance of ExecutionFailure from a JSON string
execution_failure_instance = ExecutionFailure.from_json(json)
# print the JSON string representation of the object
print(ExecutionFailure.to_json())

# convert the object into a dict
execution_failure_dict = execution_failure_instance.to_dict()
# create an instance of ExecutionFailure from a dict
execution_failure_from_dict = ExecutionFailure.from_dict(execution_failure_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


