# datastorage.WorkflowDiscoveryAPIApi

All URIs are relative to *http://localhost:8080*

Method | HTTP request | Description
------------- | ------------- | -------------
[**get_workflow_by_id**](WorkflowDiscoveryAPIApi.md#get_workflow_by_id) | **GET** /api/v1/workflows/{workflow_id} | Get workflow by UUID (with optional security gate)
[**list_available_actions**](WorkflowDiscoveryAPIApi.md#list_available_actions) | **GET** /api/v1/workflows/actions | List available action types
[**list_workflows_by_action_type**](WorkflowDiscoveryAPIApi.md#list_workflows_by_action_type) | **GET** /api/v1/workflows/actions/{action_type} | List workflows for action type


# **get_workflow_by_id**
> RemediationWorkflow get_workflow_by_id(workflow_id, severity=severity, component=component, environment=environment, priority=priority, custom_labels=custom_labels, detected_labels=detected_labels, remediation_id=remediation_id)

Get workflow by UUID (with optional security gate)

Retrieve a specific workflow by its UUID. Step 3 of the three-step workflow discovery protocol when context filters are provided.  **Design Decision**: DD-WORKFLOW-002 v3.0 (UUID primary key) **Security Gate**: DD-WORKFLOW-016, DD-HAPI-017  **Without context filters**: Returns workflow by ID (existing behavior). **With context filters**: Returns workflow only if it matches the signal context. Returns 404 if the workflow exists but does not match the context filters (security gate - prevents info leakage by not distinguishing \"not found\" from \"filtered out\").  Emits `workflow.catalog.workflow_retrieved` audit event when context filters are present. 

### Example


```python
import time
import os
import datastorage
from datastorage.models.remediation_workflow import RemediationWorkflow
from datastorage.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)


# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = datastorage.WorkflowDiscoveryAPIApi(api_client)
    workflow_id = 'workflow_id_example' # str | 
    severity = 'severity_example' # str | Security gate: signal severity level (optional)
    component = 'component_example' # str | Security gate: Kubernetes resource type (optional)
    environment = 'environment_example' # str | Security gate: target environment (optional)
    priority = 'priority_example' # str | Security gate: business priority level (optional)
    custom_labels = 'custom_labels_example' # str | Security gate: JSON-encoded custom labels (optional)
    detected_labels = 'detected_labels_example' # str | Security gate: JSON-encoded detected labels (optional)
    remediation_id = 'remediation_id_example' # str | Remediation request ID for audit correlation (BR-AUDIT-021) (optional)

    try:
        # Get workflow by UUID (with optional security gate)
        api_response = api_instance.get_workflow_by_id(workflow_id, severity=severity, component=component, environment=environment, priority=priority, custom_labels=custom_labels, detected_labels=detected_labels, remediation_id=remediation_id)
        print("The response of WorkflowDiscoveryAPIApi->get_workflow_by_id:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling WorkflowDiscoveryAPIApi->get_workflow_by_id: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **workflow_id** | **str**|  | 
 **severity** | **str**| Security gate: signal severity level | [optional] 
 **component** | **str**| Security gate: Kubernetes resource type | [optional] 
 **environment** | **str**| Security gate: target environment | [optional] 
 **priority** | **str**| Security gate: business priority level | [optional] 
 **custom_labels** | **str**| Security gate: JSON-encoded custom labels | [optional] 
 **detected_labels** | **str**| Security gate: JSON-encoded detected labels | [optional] 
 **remediation_id** | **str**| Remediation request ID for audit correlation (BR-AUDIT-021) | [optional] 

### Return type

[**RemediationWorkflow**](RemediationWorkflow.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Workflow found |  -  |
**404** | Workflow not found |  -  |
**500** | Internal server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **list_available_actions**
> ActionTypeListResponse list_available_actions(severity, component, environment, priority, custom_labels=custom_labels, detected_labels=detected_labels, remediation_id=remediation_id, offset=offset, limit=limit)

List available action types

Step 1 of the three-step workflow discovery protocol. Returns action types from the taxonomy that have active workflows matching the provided signal context filters.  **Authority**: DD-WORKFLOW-016 (Action-Type Workflow Catalog Indexing) **Business Requirement**: BR-HAPI-017-001 (Three-Step Tool Implementation)  **Behavior**: - Queries action_type_taxonomy joined with remediation_workflow_catalog - Filters by active workflows matching signal context (severity, component, environment, priority) - Returns action types with descriptions and workflow counts - Paginated (default 10 per page) - Emits `workflow.catalog.actions_listed` audit event (DD-WORKFLOW-014 v3.0) 

### Example


```python
import time
import os
import datastorage
from datastorage.models.action_type_list_response import ActionTypeListResponse
from datastorage.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)


# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = datastorage.WorkflowDiscoveryAPIApi(api_client)
    severity = 'severity_example' # str | Signal severity level
    component = 'component_example' # str | Kubernetes resource type (pod, deployment, node, etc.)
    environment = 'environment_example' # str | Target environment (production, staging, etc.)
    priority = 'priority_example' # str | Business priority level
    custom_labels = 'custom_labels_example' # str | JSON-encoded custom labels (e.g., {\"constraint\":[\"cost-constrained\"]}) (optional)
    detected_labels = 'detected_labels_example' # str | JSON-encoded detected labels (e.g., {\"gitOpsManaged\":true}) (optional)
    remediation_id = 'remediation_id_example' # str | Remediation request ID for audit correlation (BR-AUDIT-021) (optional)
    offset = 0 # int | Pagination offset (optional) (default to 0)
    limit = 10 # int | Pagination limit (max 100) (optional) (default to 10)

    try:
        # List available action types
        api_response = api_instance.list_available_actions(severity, component, environment, priority, custom_labels=custom_labels, detected_labels=detected_labels, remediation_id=remediation_id, offset=offset, limit=limit)
        print("The response of WorkflowDiscoveryAPIApi->list_available_actions:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling WorkflowDiscoveryAPIApi->list_available_actions: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **severity** | **str**| Signal severity level | 
 **component** | **str**| Kubernetes resource type (pod, deployment, node, etc.) | 
 **environment** | **str**| Target environment (production, staging, etc.) | 
 **priority** | **str**| Business priority level | 
 **custom_labels** | **str**| JSON-encoded custom labels (e.g., {\&quot;constraint\&quot;:[\&quot;cost-constrained\&quot;]}) | [optional] 
 **detected_labels** | **str**| JSON-encoded detected labels (e.g., {\&quot;gitOpsManaged\&quot;:true}) | [optional] 
 **remediation_id** | **str**| Remediation request ID for audit correlation (BR-AUDIT-021) | [optional] 
 **offset** | **int**| Pagination offset | [optional] [default to 0]
 **limit** | **int**| Pagination limit (max 100) | [optional] [default to 10]

### Return type

[**ActionTypeListResponse**](ActionTypeListResponse.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Action types with matching workflow counts |  -  |
**400** | Validation error (missing required filters) |  -  |
**500** | Internal server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **list_workflows_by_action_type**
> WorkflowDiscoveryResponse list_workflows_by_action_type(action_type, severity, component, environment, priority, custom_labels=custom_labels, detected_labels=detected_labels, remediation_id=remediation_id, offset=offset, limit=limit)

List workflows for action type

Step 2 of the three-step workflow discovery protocol. Returns all active workflows matching the specified action type and signal context filters.  **Authority**: DD-WORKFLOW-016 (Action-Type Workflow Catalog Indexing) **Business Requirement**: BR-HAPI-017-001 (Three-Step Tool Implementation)  **LLM Instruction**: The LLM MUST review ALL workflows (across all pages) before selecting one. Do not select from an incomplete list.  **Behavior**: - Filters by action_type + signal context (severity, component, environment, priority) - Excludes disabled and deprecated workflows - Returns workflow metadata including effectiveness data - Paginated (default 10 per page) - Emits `workflow.catalog.workflows_listed` audit event (DD-WORKFLOW-014 v3.0) 

### Example


```python
import time
import os
import datastorage
from datastorage.models.workflow_discovery_response import WorkflowDiscoveryResponse
from datastorage.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)


# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = datastorage.WorkflowDiscoveryAPIApi(api_client)
    action_type = 'action_type_example' # str | Action type from taxonomy (e.g., ScaleReplicas, RestartPod)
    severity = 'severity_example' # str | Signal severity level
    component = 'component_example' # str | Kubernetes resource type
    environment = 'environment_example' # str | Target environment
    priority = 'priority_example' # str | Business priority level
    custom_labels = 'custom_labels_example' # str | JSON-encoded custom labels (optional)
    detected_labels = 'detected_labels_example' # str | JSON-encoded detected labels (optional)
    remediation_id = 'remediation_id_example' # str | Remediation request ID for audit correlation (BR-AUDIT-021) (optional)
    offset = 0 # int | Pagination offset (optional) (default to 0)
    limit = 10 # int | Pagination limit (max 100) (optional) (default to 10)

    try:
        # List workflows for action type
        api_response = api_instance.list_workflows_by_action_type(action_type, severity, component, environment, priority, custom_labels=custom_labels, detected_labels=detected_labels, remediation_id=remediation_id, offset=offset, limit=limit)
        print("The response of WorkflowDiscoveryAPIApi->list_workflows_by_action_type:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling WorkflowDiscoveryAPIApi->list_workflows_by_action_type: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **action_type** | **str**| Action type from taxonomy (e.g., ScaleReplicas, RestartPod) | 
 **severity** | **str**| Signal severity level | 
 **component** | **str**| Kubernetes resource type | 
 **environment** | **str**| Target environment | 
 **priority** | **str**| Business priority level | 
 **custom_labels** | **str**| JSON-encoded custom labels | [optional] 
 **detected_labels** | **str**| JSON-encoded detected labels | [optional] 
 **remediation_id** | **str**| Remediation request ID for audit correlation (BR-AUDIT-021) | [optional] 
 **offset** | **int**| Pagination offset | [optional] [default to 0]
 **limit** | **int**| Pagination limit (max 100) | [optional] [default to 10]

### Return type

[**WorkflowDiscoveryResponse**](WorkflowDiscoveryResponse.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Workflows for the specified action type |  -  |
**400** | Validation error |  -  |
**500** | Internal server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

