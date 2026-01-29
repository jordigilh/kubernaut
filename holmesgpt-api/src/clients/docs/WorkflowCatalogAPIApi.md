# datastorage.WorkflowCatalogAPIApi

All URIs are relative to *http://localhost:8080*

Method | HTTP request | Description
------------- | ------------- | -------------
[**create_workflow**](WorkflowCatalogAPIApi.md#create_workflow) | **POST** /api/v1/workflows | Create workflow
[**disable_workflow**](WorkflowCatalogAPIApi.md#disable_workflow) | **PATCH** /api/v1/workflows/{workflow_id}/disable | Disable workflow
[**get_workflow_by_id**](WorkflowCatalogAPIApi.md#get_workflow_by_id) | **GET** /api/v1/workflows/{workflow_id} | Get workflow by UUID
[**list_workflows**](WorkflowCatalogAPIApi.md#list_workflows) | **GET** /api/v1/workflows | List workflows
[**search_workflows**](WorkflowCatalogAPIApi.md#search_workflows) | **POST** /api/v1/workflows/search | Label-based workflow search
[**update_workflow**](WorkflowCatalogAPIApi.md#update_workflow) | **PATCH** /api/v1/workflows/{workflow_id} | Update workflow mutable fields


# **create_workflow**
> RemediationWorkflow create_workflow(remediation_workflow)

Create workflow

Create a new workflow in the catalog.

**Business Requirement**: BR-STORAGE-014 (Workflow Catalog Management)
**Design Decision**: DD-WORKFLOW-005 v1.0 (Direct REST API workflow registration)


### Example


```python
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
    api_instance = datastorage.WorkflowCatalogAPIApi(api_client)
    remediation_workflow = datastorage.RemediationWorkflow() # RemediationWorkflow | 

    try:
        # Create workflow
        api_response = api_instance.create_workflow(remediation_workflow)
        print("The response of WorkflowCatalogAPIApi->create_workflow:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling WorkflowCatalogAPIApi->create_workflow: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **remediation_workflow** | [**RemediationWorkflow**](RemediationWorkflow.md)|  | 

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
**201** | Workflow created successfully |  -  |
**400** | Validation error |  -  |
**401** | Authentication failed - Invalid or missing Bearer token.  **Authority**: DD-AUTH-014 (Middleware-based authentication)  |  -  |
**403** | Authorization failed - Kubernetes SubjectAccessReview (SAR) denied access.  **Authority**: DD-AUTH-014 (Middleware-based authorization)  |  -  |
**500** | Internal server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **disable_workflow**
> RemediationWorkflow disable_workflow(workflow_id, workflow_disable_request=workflow_disable_request)

Disable workflow

Convenience endpoint to disable a workflow (soft delete).
Sets status to 'disabled' with timestamp and reason.

**Design Decision**: DD-WORKFLOW-012 (Convenience endpoint for soft-delete)


### Example


```python
import datastorage
from datastorage.models.remediation_workflow import RemediationWorkflow
from datastorage.models.workflow_disable_request import WorkflowDisableRequest
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
    api_instance = datastorage.WorkflowCatalogAPIApi(api_client)
    workflow_id = UUID('38400000-8cf0-11bd-b23e-10b96e4ef00d') # UUID | 
    workflow_disable_request = datastorage.WorkflowDisableRequest() # WorkflowDisableRequest |  (optional)

    try:
        # Disable workflow
        api_response = api_instance.disable_workflow(workflow_id, workflow_disable_request=workflow_disable_request)
        print("The response of WorkflowCatalogAPIApi->disable_workflow:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling WorkflowCatalogAPIApi->disable_workflow: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **workflow_id** | **UUID**|  | 
 **workflow_disable_request** | [**WorkflowDisableRequest**](WorkflowDisableRequest.md)|  | [optional] 

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
**200** | Workflow disabled |  -  |
**404** | Workflow not found |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **get_workflow_by_id**
> RemediationWorkflow get_workflow_by_id(workflow_id)

Get workflow by UUID

Retrieve a specific workflow by its UUID.

**Design Decision**: DD-WORKFLOW-002 v3.0 (UUID primary key)


### Example


```python
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
    api_instance = datastorage.WorkflowCatalogAPIApi(api_client)
    workflow_id = UUID('38400000-8cf0-11bd-b23e-10b96e4ef00d') # UUID | 

    try:
        # Get workflow by UUID
        api_response = api_instance.get_workflow_by_id(workflow_id)
        print("The response of WorkflowCatalogAPIApi->get_workflow_by_id:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling WorkflowCatalogAPIApi->get_workflow_by_id: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **workflow_id** | **UUID**|  | 

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

# **list_workflows**
> WorkflowListResponse list_workflows(status=status, environment=environment, priority=priority, component=component, workflow_name=workflow_name, limit=limit, offset=offset)

List workflows

List workflows with optional filters and pagination.

**Business Requirement**: BR-STORAGE-014 (Workflow Catalog Management)


### Example


```python
import datastorage
from datastorage.models.workflow_list_response import WorkflowListResponse
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
    api_instance = datastorage.WorkflowCatalogAPIApi(api_client)
    status = 'status_example' # str |  (optional)
    environment = 'environment_example' # str |  (optional)
    priority = 'priority_example' # str |  (optional)
    component = 'component_example' # str |  (optional)
    workflow_name = 'workflow_name_example' # str | Filter by workflow name (exact match for test idempotency) (optional)
    limit = 100 # int |  (optional) (default to 100)
    offset = 0 # int |  (optional) (default to 0)

    try:
        # List workflows
        api_response = api_instance.list_workflows(status=status, environment=environment, priority=priority, component=component, workflow_name=workflow_name, limit=limit, offset=offset)
        print("The response of WorkflowCatalogAPIApi->list_workflows:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling WorkflowCatalogAPIApi->list_workflows: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **status** | **str**|  | [optional] 
 **environment** | **str**|  | [optional] 
 **priority** | **str**|  | [optional] 
 **component** | **str**|  | [optional] 
 **workflow_name** | **str**| Filter by workflow name (exact match for test idempotency) | [optional] 
 **limit** | **int**|  | [optional] [default to 100]
 **offset** | **int**|  | [optional] [default to 0]

### Return type

[**WorkflowListResponse**](WorkflowListResponse.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Workflow list |  -  |
**500** | Internal server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **search_workflows**
> WorkflowSearchResponse search_workflows(workflow_search_request)

Label-based workflow search

Search workflows using label-based matching with wildcard support and weighted scoring.

**V1.0 Implementation**: Pure SQL label matching (no embeddings/semantic search)

**Business Requirement**: BR-STORAGE-013 (Label-Based Workflow Search)
**Design Decision**: DD-WORKFLOW-004 v1.5 (Label-Only Scoring with Wildcard Weighting)

**Behavior**:
- Mandatory filters: signal_type, severity, component, environment, priority
- Optional filters: custom_labels, detected_labels
- Wildcard support: "*" matches any non-null value
- Weighted scoring: Exact matches > Wildcard matches
- Returns top_k results sorted by confidence score (0.0-1.0)


### Example


```python
import datastorage
from datastorage.models.workflow_search_request import WorkflowSearchRequest
from datastorage.models.workflow_search_response import WorkflowSearchResponse
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
    api_instance = datastorage.WorkflowCatalogAPIApi(api_client)
    workflow_search_request = datastorage.WorkflowSearchRequest() # WorkflowSearchRequest | 

    try:
        # Label-based workflow search
        api_response = api_instance.search_workflows(workflow_search_request)
        print("The response of WorkflowCatalogAPIApi->search_workflows:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling WorkflowCatalogAPIApi->search_workflows: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **workflow_search_request** | [**WorkflowSearchRequest**](WorkflowSearchRequest.md)|  | 

### Return type

[**WorkflowSearchResponse**](WorkflowSearchResponse.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Workflow search results |  -  |
**400** | Validation error |  -  |
**500** | Internal server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **update_workflow**
> RemediationWorkflow update_workflow(workflow_id, workflow_update_request)

Update workflow mutable fields

Update mutable workflow fields (status, metrics).
Immutable fields (description, content, labels) require creating a new version.

**Design Decision**: DD-WORKFLOW-012 (Mutable vs Immutable Fields)


### Example


```python
import datastorage
from datastorage.models.remediation_workflow import RemediationWorkflow
from datastorage.models.workflow_update_request import WorkflowUpdateRequest
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
    api_instance = datastorage.WorkflowCatalogAPIApi(api_client)
    workflow_id = UUID('38400000-8cf0-11bd-b23e-10b96e4ef00d') # UUID | 
    workflow_update_request = datastorage.WorkflowUpdateRequest() # WorkflowUpdateRequest | 

    try:
        # Update workflow mutable fields
        api_response = api_instance.update_workflow(workflow_id, workflow_update_request)
        print("The response of WorkflowCatalogAPIApi->update_workflow:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling WorkflowCatalogAPIApi->update_workflow: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **workflow_id** | **UUID**|  | 
 **workflow_update_request** | [**WorkflowUpdateRequest**](WorkflowUpdateRequest.md)|  | 

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
**200** | Workflow updated |  -  |
**400** | Validation error (attempted to update immutable field) |  -  |
**404** | Workflow not found |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

