# datastorage.WorkflowsApi

All URIs are relative to *http://data-storage.kubernaut-system.svc.cluster.local:8080*

Method | HTTP request | Description
------------- | ------------- | -------------
[**create_workflow**](WorkflowsApi.md#create_workflow) | **POST** /api/v1/workflows | Create a new workflow
[**disable_workflow**](WorkflowsApi.md#disable_workflow) | **PATCH** /api/v1/workflows/{workflow_id}/disable | Disable a workflow
[**get_workflow**](WorkflowsApi.md#get_workflow) | **GET** /api/v1/workflows/{workflow_id} | Get workflow by UUID
[**list_workflow_versions**](WorkflowsApi.md#list_workflow_versions) | **GET** /api/v1/workflows/by-name/{workflow_name}/versions | List all versions of a workflow by name


# **create_workflow**
> RemediationWorkflow create_workflow(create_workflow_request)

Create a new workflow

Create a new workflow in the catalog with automatic embedding generation.

**Business Requirements**:
- BR-STORAGE-012: Workflow catalog persistence
- BR-WORKFLOW-001: Workflow version management

**Design Decisions**:
- DD-STORAGE-011: Synchronous embedding generation
- DD-WORKFLOW-012: Workflow immutability (unique workflow_id + version)
- ADR-043: Content must be valid workflow-schema.yaml


### Example


```python
import datastorage
from datastorage.models.create_workflow_request import CreateWorkflowRequest
from datastorage.models.remediation_workflow import RemediationWorkflow
from datastorage.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://data-storage.kubernaut-system.svc.cluster.local:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://data-storage.kubernaut-system.svc.cluster.local:8080"
)


# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = datastorage.WorkflowsApi(api_client)
    create_workflow_request = datastorage.CreateWorkflowRequest() # CreateWorkflowRequest | 

    try:
        # Create a new workflow
        api_response = api_instance.create_workflow(create_workflow_request)
        print("The response of WorkflowsApi->create_workflow:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling WorkflowsApi->create_workflow: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **create_workflow_request** | [**CreateWorkflowRequest**](CreateWorkflowRequest.md)|  | 

### Return type

[**RemediationWorkflow**](RemediationWorkflow.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**201** | Workflow created successfully |  * Location - URL to the created workflow <br>  * X-Request-ID - Request ID for tracing <br>  |
**400** | Bad request - Invalid or missing required fields |  -  |
**409** | Conflict - Workflow already exists with this workflow_id + version |  -  |
**500** | Internal server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **disable_workflow**
> RemediationWorkflow disable_workflow(workflow_id, disable_workflow_request)

Disable a workflow

Disable a specific workflow (soft delete).

**DD-WORKFLOW-002 v3.0**: workflow_id is UUID (single identifier)

**Design Decisions**:
- DD-WORKFLOW-012 v2.0: Workflows are never hard deleted, only disabled
- Disabled workflows are excluded from search results


### Example


```python
import datastorage
from datastorage.models.disable_workflow_request import DisableWorkflowRequest
from datastorage.models.remediation_workflow import RemediationWorkflow
from datastorage.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://data-storage.kubernaut-system.svc.cluster.local:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://data-storage.kubernaut-system.svc.cluster.local:8080"
)


# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = datastorage.WorkflowsApi(api_client)
    workflow_id = UUID('38400000-8cf0-11bd-b23e-10b96e4ef00d') # UUID | DD-WORKFLOW-002 v3.0 - UUID primary key
    disable_workflow_request = datastorage.DisableWorkflowRequest() # DisableWorkflowRequest | 

    try:
        # Disable a workflow
        api_response = api_instance.disable_workflow(workflow_id, disable_workflow_request)
        print("The response of WorkflowsApi->disable_workflow:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling WorkflowsApi->disable_workflow: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **workflow_id** | **UUID**| DD-WORKFLOW-002 v3.0 - UUID primary key | 
 **disable_workflow_request** | [**DisableWorkflowRequest**](DisableWorkflowRequest.md)|  | 

### Return type

[**RemediationWorkflow**](RemediationWorkflow.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Workflow disabled successfully |  -  |
**400** | Bad request - Missing reason |  -  |
**404** | Workflow not found |  -  |
**500** | Internal server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **get_workflow**
> RemediationWorkflow get_workflow(workflow_id)

Get workflow by UUID

Retrieve a specific workflow by its UUID.

**DD-WORKFLOW-002 v3.0**: workflow_id is UUID (single identifier)

**Business Requirements**:
- BR-WORKFLOW-001: Workflow version management

**Design Decisions**:
- DD-WORKFLOW-012 v2.0: Workflows are immutable, UUID is primary key


### Example


```python
import datastorage
from datastorage.models.remediation_workflow import RemediationWorkflow
from datastorage.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://data-storage.kubernaut-system.svc.cluster.local:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://data-storage.kubernaut-system.svc.cluster.local:8080"
)


# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = datastorage.WorkflowsApi(api_client)
    workflow_id = UUID('38400000-8cf0-11bd-b23e-10b96e4ef00d') # UUID | DD-WORKFLOW-002 v3.0 - UUID primary key

    try:
        # Get workflow by UUID
        api_response = api_instance.get_workflow(workflow_id)
        print("The response of WorkflowsApi->get_workflow:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling WorkflowsApi->get_workflow: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **workflow_id** | **UUID**| DD-WORKFLOW-002 v3.0 - UUID primary key | 

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

# **list_workflow_versions**
> List[WorkflowVersionSummary] list_workflow_versions(workflow_name)

List all versions of a workflow by name

Retrieve all versions of a specific workflow by its human-readable name,
ordered by creation date (newest first).

**DD-WORKFLOW-002 v3.0**: Use workflow_name (human-readable) to list versions.
Each version has its own UUID (workflow_id).

**Business Requirements**:
- BR-WORKFLOW-001: Workflow version management


### Example


```python
import datastorage
from datastorage.models.workflow_version_summary import WorkflowVersionSummary
from datastorage.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://data-storage.kubernaut-system.svc.cluster.local:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://data-storage.kubernaut-system.svc.cluster.local:8080"
)


# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = datastorage.WorkflowsApi(api_client)
    workflow_name = 'workflow_name_example' # str | DD-WORKFLOW-002 v3.0 - Human-readable workflow identifier

    try:
        # List all versions of a workflow by name
        api_response = api_instance.list_workflow_versions(workflow_name)
        print("The response of WorkflowsApi->list_workflow_versions:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling WorkflowsApi->list_workflow_versions: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **workflow_name** | **str**| DD-WORKFLOW-002 v3.0 - Human-readable workflow identifier | 

### Return type

[**List[WorkflowVersionSummary]**](WorkflowVersionSummary.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | List of workflow versions (empty array if none found) |  -  |
**500** | Internal server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

