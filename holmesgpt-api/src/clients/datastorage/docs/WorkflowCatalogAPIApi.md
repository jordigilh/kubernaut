# datastorage.WorkflowCatalogAPIApi

All URIs are relative to *http://localhost:8080*

Method | HTTP request | Description
------------- | ------------- | -------------
[**create_workflow**](WorkflowCatalogAPIApi.md#create_workflow) | **POST** /api/v1/workflows | Register workflow from OCI image
[**deprecate_workflow**](WorkflowCatalogAPIApi.md#deprecate_workflow) | **PATCH** /api/v1/workflows/{workflow_id}/deprecate | Deprecate workflow
[**disable_workflow**](WorkflowCatalogAPIApi.md#disable_workflow) | **PATCH** /api/v1/workflows/{workflow_id}/disable | Disable workflow
[**enable_workflow**](WorkflowCatalogAPIApi.md#enable_workflow) | **PATCH** /api/v1/workflows/{workflow_id}/enable | Enable workflow
[**get_workflow_by_id**](WorkflowCatalogAPIApi.md#get_workflow_by_id) | **GET** /api/v1/workflows/{workflow_id} | Get workflow by UUID (with optional security gate)
[**list_workflows**](WorkflowCatalogAPIApi.md#list_workflows) | **GET** /api/v1/workflows | List workflows
[**update_workflow**](WorkflowCatalogAPIApi.md#update_workflow) | **PATCH** /api/v1/workflows/{workflow_id} | Update workflow mutable fields


# **create_workflow**
> RemediationWorkflow create_workflow(create_workflow_from_oci_request)

Register workflow from OCI image

Register a new workflow by providing an OCI image pullspec. Data Storage pulls the image, extracts /workflow-schema.yaml (ADR-043), validates the schema, and populates all catalog fields from it.  **Business Requirement**: BR-WORKFLOW-017-001 (OCI-based workflow registration) **Design Decision**: DD-WORKFLOW-017 (Workflow Lifecycle Component Interactions) 

### Example


```python
import time
import os
import datastorage
from datastorage.models.create_workflow_from_oci_request import CreateWorkflowFromOCIRequest
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
    create_workflow_from_oci_request = datastorage.CreateWorkflowFromOCIRequest() # CreateWorkflowFromOCIRequest | 

    try:
        # Register workflow from OCI image
        api_response = api_instance.create_workflow(create_workflow_from_oci_request)
        print("The response of WorkflowCatalogAPIApi->create_workflow:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling WorkflowCatalogAPIApi->create_workflow: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **create_workflow_from_oci_request** | [**CreateWorkflowFromOCIRequest**](CreateWorkflowFromOCIRequest.md)|  | 

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
**400** | Validation error - schema invalid, unknown action_type, missing mandatory labels, or missing required fields.  |  -  |
**401** | Authentication failed - Invalid or missing Bearer token.  **Authority**: DD-AUTH-014 (Middleware-based authentication)  |  -  |
**403** | Authorization failed - Kubernetes SubjectAccessReview (SAR) denied access.  **Authority**: DD-AUTH-014 (Middleware-based authorization)  |  -  |
**409** | Conflict - Duplicate workflow. Workflow with same workflow_name and version already exists.  **Authority**: DS-BUG-001 fix (RFC 9110 Section 15.5.10)  |  -  |
**422** | Unprocessable Entity - /workflow-schema.yaml not found in OCI image.  |  -  |
**500** | Internal server error |  -  |
**502** | Bad Gateway - OCI image could not be pulled from registry.  |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **deprecate_workflow**
> RemediationWorkflow deprecate_workflow(workflow_id, workflow_lifecycle_request)

Deprecate workflow

Mark a workflow as deprecated. Deprecated workflows are excluded from discovery results but remain in the catalog for audit history.  **Design Decision**: DD-WORKFLOW-017 Phase 4.4 (Lifecycle PATCH endpoints) 

### Example


```python
import time
import os
import datastorage
from datastorage.models.remediation_workflow import RemediationWorkflow
from datastorage.models.workflow_lifecycle_request import WorkflowLifecycleRequest
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
    workflow_id = 'workflow_id_example' # str | 
    workflow_lifecycle_request = datastorage.WorkflowLifecycleRequest() # WorkflowLifecycleRequest | 

    try:
        # Deprecate workflow
        api_response = api_instance.deprecate_workflow(workflow_id, workflow_lifecycle_request)
        print("The response of WorkflowCatalogAPIApi->deprecate_workflow:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling WorkflowCatalogAPIApi->deprecate_workflow: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **workflow_id** | **str**|  | 
 **workflow_lifecycle_request** | [**WorkflowLifecycleRequest**](WorkflowLifecycleRequest.md)|  | 

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
**200** | Workflow deprecated |  -  |
**400** | Missing required field (reason) |  -  |
**404** | Workflow not found |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **disable_workflow**
> RemediationWorkflow disable_workflow(workflow_id, workflow_lifecycle_request)

Disable workflow

Convenience endpoint to disable a workflow (soft delete). Sets status to 'disabled' with timestamp and reason.  **Design Decision**: DD-WORKFLOW-012, DD-WORKFLOW-017 Phase 4.4 

### Example


```python
import time
import os
import datastorage
from datastorage.models.remediation_workflow import RemediationWorkflow
from datastorage.models.workflow_lifecycle_request import WorkflowLifecycleRequest
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
    workflow_id = 'workflow_id_example' # str | 
    workflow_lifecycle_request = datastorage.WorkflowLifecycleRequest() # WorkflowLifecycleRequest | 

    try:
        # Disable workflow
        api_response = api_instance.disable_workflow(workflow_id, workflow_lifecycle_request)
        print("The response of WorkflowCatalogAPIApi->disable_workflow:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling WorkflowCatalogAPIApi->disable_workflow: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **workflow_id** | **str**|  | 
 **workflow_lifecycle_request** | [**WorkflowLifecycleRequest**](WorkflowLifecycleRequest.md)|  | 

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
**400** | Missing required field (reason) |  -  |
**404** | Workflow not found |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **enable_workflow**
> RemediationWorkflow enable_workflow(workflow_id, workflow_lifecycle_request)

Enable workflow

Re-enable a previously disabled or deprecated workflow. Sets status to 'active' with timestamp and reason.  **Design Decision**: DD-WORKFLOW-017 Phase 4.4 (Lifecycle PATCH endpoints) 

### Example


```python
import time
import os
import datastorage
from datastorage.models.remediation_workflow import RemediationWorkflow
from datastorage.models.workflow_lifecycle_request import WorkflowLifecycleRequest
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
    workflow_id = 'workflow_id_example' # str | 
    workflow_lifecycle_request = datastorage.WorkflowLifecycleRequest() # WorkflowLifecycleRequest | 

    try:
        # Enable workflow
        api_response = api_instance.enable_workflow(workflow_id, workflow_lifecycle_request)
        print("The response of WorkflowCatalogAPIApi->enable_workflow:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling WorkflowCatalogAPIApi->enable_workflow: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **workflow_id** | **str**|  | 
 **workflow_lifecycle_request** | [**WorkflowLifecycleRequest**](WorkflowLifecycleRequest.md)|  | 

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
**200** | Workflow enabled |  -  |
**400** | Missing required field (reason) |  -  |
**404** | Workflow not found |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

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
    api_instance = datastorage.WorkflowCatalogAPIApi(api_client)
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
        print("The response of WorkflowCatalogAPIApi->get_workflow_by_id:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling WorkflowCatalogAPIApi->get_workflow_by_id: %s\n" % e)
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

# **list_workflows**
> WorkflowListResponse list_workflows(status=status, environment=environment, priority=priority, component=component, workflow_name=workflow_name, limit=limit, offset=offset)

List workflows

List workflows with optional filters and pagination.  **Business Requirement**: BR-STORAGE-014 (Workflow Catalog Management) 

### Example


```python
import time
import os
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

# **update_workflow**
> RemediationWorkflow update_workflow(workflow_id, workflow_update_request)

Update workflow mutable fields

Update mutable workflow fields (status, metrics). Immutable fields (description, content, labels) require creating a new version.  **Design Decision**: DD-WORKFLOW-012 (Mutable vs Immutable Fields) 

### Example


```python
import time
import os
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
    workflow_id = 'workflow_id_example' # str | 
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
 **workflow_id** | **str**|  | 
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

