<a id="__pageTop"></a>
# datastorage.apis.tags.workflow_catalog_api_api.WorkflowCatalogAPIApi

All URIs are relative to *http://localhost:8080*

Method | HTTP request | Description
------------- | ------------- | -------------
[**create_workflow**](#create_workflow) | **post** /api/v1/workflows | Create workflow
[**disable_workflow**](#disable_workflow) | **patch** /api/v1/workflows/{workflow_id}/disable | Disable workflow
[**get_workflow_by_id**](#get_workflow_by_id) | **get** /api/v1/workflows/{workflow_id} | Get workflow by UUID
[**list_workflows**](#list_workflows) | **get** /api/v1/workflows | List workflows
[**search_workflows**](#search_workflows) | **post** /api/v1/workflows/search | Label-based workflow search
[**update_workflow**](#update_workflow) | **patch** /api/v1/workflows/{workflow_id} | Update workflow mutable fields

# **create_workflow**
<a id="create_workflow"></a>
> RemediationWorkflow create_workflow(remediation_workflow)

Create workflow

Create a new workflow in the catalog.  **Business Requirement**: BR-STORAGE-014 (Workflow Catalog Management) **Design Decision**: DD-WORKFLOW-005 v1.0 (Direct REST API workflow registration) 

### Example

```python
import datastorage
from datastorage.apis.tags import workflow_catalog_api_api
from datastorage.model.remediation_workflow import RemediationWorkflow
from datastorage.model.rfc7807_problem import RFC7807Problem
from pprint import pprint
# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)

# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = workflow_catalog_api_api.WorkflowCatalogAPIApi(api_client)

    # example passing only required values which don't have defaults set
    body = RemediationWorkflow(
        workflow_id="workflow_id_example",
        workflow_name="workflow_name_example",
        version="version_example",
        name="name_example",
        description="description_example",
        owner="owner_example",
        maintainer="maintainer_example",
        content="content_example",
        content_hash="content_hash_example",
        parameters=dict(),
        execution_engine="execution_engine_example",
        container_image="ghcr.io/kubernaut/workflows/oomkill:v1.0.0@sha256:abc123...",
        container_digest="sha256:abc123...",
        labels=MandatoryLabels(
            signal_type="OOMKilled",
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        ),
        custom_labels=CustomLabels(
            key=[
                "key_example"
            ],
        ),
        detected_labels=DetectedLabels(
            failed_detections=["pdbProtected","networkIsolated"],
            git_ops_managed=True,
            git_ops_tool="argocd",
            pdb_protected=True,
            hpa_enabled=True,
            stateful=True,
            helm_managed=True,
            network_isolated=True,
            service_mesh="istio",
        ),
        status="active",
        disabled_at="1970-01-01T00:00:00.00Z",
        disabled_by="disabled_by_example",
        disabled_reason="disabled_reason_example",
        is_latest_version=True,
        previous_version="previous_version_example",
        deprecation_notice="deprecation_notice_example",
        version_notes="version_notes_example",
        change_summary="change_summary_example",
        approved_by="approved_by_example",
        approved_at="1970-01-01T00:00:00.00Z",
        expected_success_rate=0.0,
        expected_duration_seconds=0,
        actual_success_rate=0.0,
        total_executions=0,
        successful_executions=0,
        created_at="1970-01-01T00:00:00.00Z",
        updated_at="1970-01-01T00:00:00.00Z",
        created_by="created_by_example",
        updated_by="updated_by_example",
    )
    try:
        # Create workflow
        api_response = api_instance.create_workflow(
            body=body,
        )
        pprint(api_response)
    except datastorage.ApiException as e:
        print("Exception when calling WorkflowCatalogAPIApi->create_workflow: %s\n" % e)
```
### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
body | typing.Union[SchemaForRequestBodyApplicationJson] | required |
content_type | str | optional, default is 'application/json' | Selects the schema and serialization of the request body
accept_content_types | typing.Tuple[str] | default is ('application/json', 'application/problem+json', ) | Tells the server the content type(s) that are accepted by the client
stream | bool | default is False | if True then the response.content will be streamed and loaded from a file like object. When downloading a file, set this to True to force the code to deserialize the content to a FileSchema file
timeout | typing.Optional[typing.Union[int, typing.Tuple]] | default is None | the timeout used by the rest client
skip_deserialization | bool | default is False | when True, headers and body will be unset and an instance of api_client.ApiResponseWithoutDeserialization will be returned

### body

# SchemaForRequestBodyApplicationJson
Type | Description  | Notes
------------- | ------------- | -------------
[**RemediationWorkflow**](../../models/RemediationWorkflow.md) |  | 


### Return Types, Responses

Code | Class | Description
------------- | ------------- | -------------
n/a | api_client.ApiResponseWithoutDeserialization | When skip_deserialization is True this response is returned
201 | [ApiResponseFor201](#create_workflow.ApiResponseFor201) | Workflow created successfully
400 | [ApiResponseFor400](#create_workflow.ApiResponseFor400) | Validation error
500 | [ApiResponseFor500](#create_workflow.ApiResponseFor500) | Internal server error

#### create_workflow.ApiResponseFor201
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor201ResponseBodyApplicationJson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor201ResponseBodyApplicationJson
Type | Description  | Notes
------------- | ------------- | -------------
[**RemediationWorkflow**](../../models/RemediationWorkflow.md) |  | 


#### create_workflow.ApiResponseFor400
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor400ResponseBodyApplicationProblemjson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor400ResponseBodyApplicationProblemjson
Type | Description  | Notes
------------- | ------------- | -------------
[**RFC7807Problem**](../../models/RFC7807Problem.md) |  | 


#### create_workflow.ApiResponseFor500
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor500ResponseBodyApplicationProblemjson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor500ResponseBodyApplicationProblemjson
Type | Description  | Notes
------------- | ------------- | -------------
[**RFC7807Problem**](../../models/RFC7807Problem.md) |  | 


### Authorization

No authorization required

[[Back to top]](#__pageTop) [[Back to API list]](../../../README.md#documentation-for-api-endpoints) [[Back to Model list]](../../../README.md#documentation-for-models) [[Back to README]](../../../README.md)

# **disable_workflow**
<a id="disable_workflow"></a>
> RemediationWorkflow disable_workflow(workflow_id)

Disable workflow

Convenience endpoint to disable a workflow (soft delete). Sets status to 'disabled' with timestamp and reason.  **Design Decision**: DD-WORKFLOW-012 (Convenience endpoint for soft-delete) 

### Example

```python
import datastorage
from datastorage.apis.tags import workflow_catalog_api_api
from datastorage.model.workflow_disable_request import WorkflowDisableRequest
from datastorage.model.remediation_workflow import RemediationWorkflow
from datastorage.model.rfc7807_problem import RFC7807Problem
from pprint import pprint
# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)

# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = workflow_catalog_api_api.WorkflowCatalogAPIApi(api_client)

    # example passing only required values which don't have defaults set
    path_params = {
        'workflow_id': "workflow_id_example",
    }
    try:
        # Disable workflow
        api_response = api_instance.disable_workflow(
            path_params=path_params,
        )
        pprint(api_response)
    except datastorage.ApiException as e:
        print("Exception when calling WorkflowCatalogAPIApi->disable_workflow: %s\n" % e)

    # example passing only optional values
    path_params = {
        'workflow_id': "workflow_id_example",
    }
    body = WorkflowDisableRequest(
        reason="reason_example",
        updated_by="updated_by_example",
    )
    try:
        # Disable workflow
        api_response = api_instance.disable_workflow(
            path_params=path_params,
            body=body,
        )
        pprint(api_response)
    except datastorage.ApiException as e:
        print("Exception when calling WorkflowCatalogAPIApi->disable_workflow: %s\n" % e)
```
### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
body | typing.Union[SchemaForRequestBodyApplicationJson, Unset] | optional, default is unset |
path_params | RequestPathParams | |
content_type | str | optional, default is 'application/json' | Selects the schema and serialization of the request body
accept_content_types | typing.Tuple[str] | default is ('application/json', 'application/problem+json', ) | Tells the server the content type(s) that are accepted by the client
stream | bool | default is False | if True then the response.content will be streamed and loaded from a file like object. When downloading a file, set this to True to force the code to deserialize the content to a FileSchema file
timeout | typing.Optional[typing.Union[int, typing.Tuple]] | default is None | the timeout used by the rest client
skip_deserialization | bool | default is False | when True, headers and body will be unset and an instance of api_client.ApiResponseWithoutDeserialization will be returned

### body

# SchemaForRequestBodyApplicationJson
Type | Description  | Notes
------------- | ------------- | -------------
[**WorkflowDisableRequest**](../../models/WorkflowDisableRequest.md) |  | 


### path_params
#### RequestPathParams

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
workflow_id | WorkflowIdSchema | | 

# WorkflowIdSchema

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
str, uuid.UUID,  | str,  |  | value must be a uuid

### Return Types, Responses

Code | Class | Description
------------- | ------------- | -------------
n/a | api_client.ApiResponseWithoutDeserialization | When skip_deserialization is True this response is returned
200 | [ApiResponseFor200](#disable_workflow.ApiResponseFor200) | Workflow disabled
404 | [ApiResponseFor404](#disable_workflow.ApiResponseFor404) | Workflow not found

#### disable_workflow.ApiResponseFor200
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor200ResponseBodyApplicationJson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor200ResponseBodyApplicationJson
Type | Description  | Notes
------------- | ------------- | -------------
[**RemediationWorkflow**](../../models/RemediationWorkflow.md) |  | 


#### disable_workflow.ApiResponseFor404
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor404ResponseBodyApplicationProblemjson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor404ResponseBodyApplicationProblemjson
Type | Description  | Notes
------------- | ------------- | -------------
[**RFC7807Problem**](../../models/RFC7807Problem.md) |  | 


### Authorization

No authorization required

[[Back to top]](#__pageTop) [[Back to API list]](../../../README.md#documentation-for-api-endpoints) [[Back to Model list]](../../../README.md#documentation-for-models) [[Back to README]](../../../README.md)

# **get_workflow_by_id**
<a id="get_workflow_by_id"></a>
> RemediationWorkflow get_workflow_by_id(workflow_id)

Get workflow by UUID

Retrieve a specific workflow by its UUID.  **Design Decision**: DD-WORKFLOW-002 v3.0 (UUID primary key) 

### Example

```python
import datastorage
from datastorage.apis.tags import workflow_catalog_api_api
from datastorage.model.remediation_workflow import RemediationWorkflow
from datastorage.model.rfc7807_problem import RFC7807Problem
from pprint import pprint
# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)

# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = workflow_catalog_api_api.WorkflowCatalogAPIApi(api_client)

    # example passing only required values which don't have defaults set
    path_params = {
        'workflow_id': "workflow_id_example",
    }
    try:
        # Get workflow by UUID
        api_response = api_instance.get_workflow_by_id(
            path_params=path_params,
        )
        pprint(api_response)
    except datastorage.ApiException as e:
        print("Exception when calling WorkflowCatalogAPIApi->get_workflow_by_id: %s\n" % e)
```
### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
path_params | RequestPathParams | |
accept_content_types | typing.Tuple[str] | default is ('application/json', 'application/problem+json', ) | Tells the server the content type(s) that are accepted by the client
stream | bool | default is False | if True then the response.content will be streamed and loaded from a file like object. When downloading a file, set this to True to force the code to deserialize the content to a FileSchema file
timeout | typing.Optional[typing.Union[int, typing.Tuple]] | default is None | the timeout used by the rest client
skip_deserialization | bool | default is False | when True, headers and body will be unset and an instance of api_client.ApiResponseWithoutDeserialization will be returned

### path_params
#### RequestPathParams

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
workflow_id | WorkflowIdSchema | | 

# WorkflowIdSchema

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
str, uuid.UUID,  | str,  |  | value must be a uuid

### Return Types, Responses

Code | Class | Description
------------- | ------------- | -------------
n/a | api_client.ApiResponseWithoutDeserialization | When skip_deserialization is True this response is returned
200 | [ApiResponseFor200](#get_workflow_by_id.ApiResponseFor200) | Workflow found
404 | [ApiResponseFor404](#get_workflow_by_id.ApiResponseFor404) | Workflow not found
500 | [ApiResponseFor500](#get_workflow_by_id.ApiResponseFor500) | Internal server error

#### get_workflow_by_id.ApiResponseFor200
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor200ResponseBodyApplicationJson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor200ResponseBodyApplicationJson
Type | Description  | Notes
------------- | ------------- | -------------
[**RemediationWorkflow**](../../models/RemediationWorkflow.md) |  | 


#### get_workflow_by_id.ApiResponseFor404
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor404ResponseBodyApplicationProblemjson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor404ResponseBodyApplicationProblemjson
Type | Description  | Notes
------------- | ------------- | -------------
[**RFC7807Problem**](../../models/RFC7807Problem.md) |  | 


#### get_workflow_by_id.ApiResponseFor500
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor500ResponseBodyApplicationProblemjson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor500ResponseBodyApplicationProblemjson
Type | Description  | Notes
------------- | ------------- | -------------
[**RFC7807Problem**](../../models/RFC7807Problem.md) |  | 


### Authorization

No authorization required

[[Back to top]](#__pageTop) [[Back to API list]](../../../README.md#documentation-for-api-endpoints) [[Back to Model list]](../../../README.md#documentation-for-models) [[Back to README]](../../../README.md)

# **list_workflows**
<a id="list_workflows"></a>
> WorkflowListResponse list_workflows()

List workflows

List workflows with optional filters and pagination.  **Business Requirement**: BR-STORAGE-014 (Workflow Catalog Management) 

### Example

```python
import datastorage
from datastorage.apis.tags import workflow_catalog_api_api
from datastorage.model.workflow_list_response import WorkflowListResponse
from datastorage.model.rfc7807_problem import RFC7807Problem
from pprint import pprint
# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)

# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = workflow_catalog_api_api.WorkflowCatalogAPIApi(api_client)

    # example passing only optional values
    query_params = {
        'status': "active",
        'environment': "environment_example",
        'priority': "priority_example",
        'component': "component_example",
        'limit': 100,
        'offset': 0,
    }
    try:
        # List workflows
        api_response = api_instance.list_workflows(
            query_params=query_params,
        )
        pprint(api_response)
    except datastorage.ApiException as e:
        print("Exception when calling WorkflowCatalogAPIApi->list_workflows: %s\n" % e)
```
### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
query_params | RequestQueryParams | |
accept_content_types | typing.Tuple[str] | default is ('application/json', 'application/problem+json', ) | Tells the server the content type(s) that are accepted by the client
stream | bool | default is False | if True then the response.content will be streamed and loaded from a file like object. When downloading a file, set this to True to force the code to deserialize the content to a FileSchema file
timeout | typing.Optional[typing.Union[int, typing.Tuple]] | default is None | the timeout used by the rest client
skip_deserialization | bool | default is False | when True, headers and body will be unset and an instance of api_client.ApiResponseWithoutDeserialization will be returned

### query_params
#### RequestQueryParams

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
status | StatusSchema | | optional
environment | EnvironmentSchema | | optional
priority | PrioritySchema | | optional
component | ComponentSchema | | optional
limit | LimitSchema | | optional
offset | OffsetSchema | | optional


# StatusSchema

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
str,  | str,  |  | must be one of ["active", "disabled", "deprecated", "archived", ] 

# EnvironmentSchema

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
str,  | str,  |  | 

# PrioritySchema

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
str,  | str,  |  | 

# ComponentSchema

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
str,  | str,  |  | 

# LimitSchema

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
decimal.Decimal, int,  | decimal.Decimal,  |  | if omitted the server will use the default value of 100

# OffsetSchema

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
decimal.Decimal, int,  | decimal.Decimal,  |  | if omitted the server will use the default value of 0

### Return Types, Responses

Code | Class | Description
------------- | ------------- | -------------
n/a | api_client.ApiResponseWithoutDeserialization | When skip_deserialization is True this response is returned
200 | [ApiResponseFor200](#list_workflows.ApiResponseFor200) | Workflow list
500 | [ApiResponseFor500](#list_workflows.ApiResponseFor500) | Internal server error

#### list_workflows.ApiResponseFor200
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor200ResponseBodyApplicationJson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor200ResponseBodyApplicationJson
Type | Description  | Notes
------------- | ------------- | -------------
[**WorkflowListResponse**](../../models/WorkflowListResponse.md) |  | 


#### list_workflows.ApiResponseFor500
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor500ResponseBodyApplicationProblemjson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor500ResponseBodyApplicationProblemjson
Type | Description  | Notes
------------- | ------------- | -------------
[**RFC7807Problem**](../../models/RFC7807Problem.md) |  | 


### Authorization

No authorization required

[[Back to top]](#__pageTop) [[Back to API list]](../../../README.md#documentation-for-api-endpoints) [[Back to Model list]](../../../README.md#documentation-for-models) [[Back to README]](../../../README.md)

# **search_workflows**
<a id="search_workflows"></a>
> WorkflowSearchResponse search_workflows(workflow_search_request)

Label-based workflow search

Search workflows using label-based matching with wildcard support and weighted scoring.  **V1.0 Implementation**: Pure SQL label matching (no embeddings/semantic search)  **Business Requirement**: BR-STORAGE-013 (Label-Based Workflow Search) **Design Decision**: DD-WORKFLOW-004 v1.5 (Label-Only Scoring with Wildcard Weighting)  **Behavior**: - Mandatory filters: signal_type, severity, component, environment, priority - Optional filters: custom_labels, detected_labels - Wildcard support: \"*\" matches any non-null value - Weighted scoring: Exact matches > Wildcard matches - Returns top_k results sorted by confidence score (0.0-1.0) 

### Example

```python
import datastorage
from datastorage.apis.tags import workflow_catalog_api_api
from datastorage.model.workflow_search_request import WorkflowSearchRequest
from datastorage.model.workflow_search_response import WorkflowSearchResponse
from datastorage.model.rfc7807_problem import RFC7807Problem
from pprint import pprint
# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)

# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = workflow_catalog_api_api.WorkflowCatalogAPIApi(api_client)

    # example passing only required values which don't have defaults set
    body = WorkflowSearchRequest(
        remediation_id="remediation_id_example",
        filters=WorkflowSearchFilters(
            signal_type="OOMKilled",
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
            custom_labels=CustomLabels(
                key=[
                    "key_example"
                ],
            ),
            detected_labels=DetectedLabels(
                failed_detections=["pdbProtected","networkIsolated"],
                git_ops_managed=True,
                git_ops_tool="argocd",
                pdb_protected=True,
                hpa_enabled=True,
                stateful=True,
                helm_managed=True,
                network_isolated=True,
                service_mesh="istio",
            ),
            status=[
                "active"
            ],
        ),
        top_k=10,
        min_score=0.0,
        include_disabled=False,
    )
    try:
        # Label-based workflow search
        api_response = api_instance.search_workflows(
            body=body,
        )
        pprint(api_response)
    except datastorage.ApiException as e:
        print("Exception when calling WorkflowCatalogAPIApi->search_workflows: %s\n" % e)
```
### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
body | typing.Union[SchemaForRequestBodyApplicationJson] | required |
content_type | str | optional, default is 'application/json' | Selects the schema and serialization of the request body
accept_content_types | typing.Tuple[str] | default is ('application/json', 'application/problem+json', ) | Tells the server the content type(s) that are accepted by the client
stream | bool | default is False | if True then the response.content will be streamed and loaded from a file like object. When downloading a file, set this to True to force the code to deserialize the content to a FileSchema file
timeout | typing.Optional[typing.Union[int, typing.Tuple]] | default is None | the timeout used by the rest client
skip_deserialization | bool | default is False | when True, headers and body will be unset and an instance of api_client.ApiResponseWithoutDeserialization will be returned

### body

# SchemaForRequestBodyApplicationJson
Type | Description  | Notes
------------- | ------------- | -------------
[**WorkflowSearchRequest**](../../models/WorkflowSearchRequest.md) |  | 


### Return Types, Responses

Code | Class | Description
------------- | ------------- | -------------
n/a | api_client.ApiResponseWithoutDeserialization | When skip_deserialization is True this response is returned
200 | [ApiResponseFor200](#search_workflows.ApiResponseFor200) | Workflow search results
400 | [ApiResponseFor400](#search_workflows.ApiResponseFor400) | Validation error
500 | [ApiResponseFor500](#search_workflows.ApiResponseFor500) | Internal server error

#### search_workflows.ApiResponseFor200
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor200ResponseBodyApplicationJson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor200ResponseBodyApplicationJson
Type | Description  | Notes
------------- | ------------- | -------------
[**WorkflowSearchResponse**](../../models/WorkflowSearchResponse.md) |  | 


#### search_workflows.ApiResponseFor400
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor400ResponseBodyApplicationProblemjson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor400ResponseBodyApplicationProblemjson
Type | Description  | Notes
------------- | ------------- | -------------
[**RFC7807Problem**](../../models/RFC7807Problem.md) |  | 


#### search_workflows.ApiResponseFor500
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor500ResponseBodyApplicationProblemjson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor500ResponseBodyApplicationProblemjson
Type | Description  | Notes
------------- | ------------- | -------------
[**RFC7807Problem**](../../models/RFC7807Problem.md) |  | 


### Authorization

No authorization required

[[Back to top]](#__pageTop) [[Back to API list]](../../../README.md#documentation-for-api-endpoints) [[Back to Model list]](../../../README.md#documentation-for-models) [[Back to README]](../../../README.md)

# **update_workflow**
<a id="update_workflow"></a>
> RemediationWorkflow update_workflow(workflow_idworkflow_update_request)

Update workflow mutable fields

Update mutable workflow fields (status, metrics). Immutable fields (description, content, labels) require creating a new version.  **Design Decision**: DD-WORKFLOW-012 (Mutable vs Immutable Fields) 

### Example

```python
import datastorage
from datastorage.apis.tags import workflow_catalog_api_api
from datastorage.model.workflow_update_request import WorkflowUpdateRequest
from datastorage.model.remediation_workflow import RemediationWorkflow
from datastorage.model.rfc7807_problem import RFC7807Problem
from pprint import pprint
# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)

# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = workflow_catalog_api_api.WorkflowCatalogAPIApi(api_client)

    # example passing only required values which don't have defaults set
    path_params = {
        'workflow_id': "workflow_id_example",
    }
    body = WorkflowUpdateRequest(
        status="active",
        disabled_by="disabled_by_example",
        disabled_reason="disabled_reason_example",
    )
    try:
        # Update workflow mutable fields
        api_response = api_instance.update_workflow(
            path_params=path_params,
            body=body,
        )
        pprint(api_response)
    except datastorage.ApiException as e:
        print("Exception when calling WorkflowCatalogAPIApi->update_workflow: %s\n" % e)
```
### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
body | typing.Union[SchemaForRequestBodyApplicationJson] | required |
path_params | RequestPathParams | |
content_type | str | optional, default is 'application/json' | Selects the schema and serialization of the request body
accept_content_types | typing.Tuple[str] | default is ('application/json', 'application/problem+json', ) | Tells the server the content type(s) that are accepted by the client
stream | bool | default is False | if True then the response.content will be streamed and loaded from a file like object. When downloading a file, set this to True to force the code to deserialize the content to a FileSchema file
timeout | typing.Optional[typing.Union[int, typing.Tuple]] | default is None | the timeout used by the rest client
skip_deserialization | bool | default is False | when True, headers and body will be unset and an instance of api_client.ApiResponseWithoutDeserialization will be returned

### body

# SchemaForRequestBodyApplicationJson
Type | Description  | Notes
------------- | ------------- | -------------
[**WorkflowUpdateRequest**](../../models/WorkflowUpdateRequest.md) |  | 


### path_params
#### RequestPathParams

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
workflow_id | WorkflowIdSchema | | 

# WorkflowIdSchema

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
str, uuid.UUID,  | str,  |  | value must be a uuid

### Return Types, Responses

Code | Class | Description
------------- | ------------- | -------------
n/a | api_client.ApiResponseWithoutDeserialization | When skip_deserialization is True this response is returned
200 | [ApiResponseFor200](#update_workflow.ApiResponseFor200) | Workflow updated
400 | [ApiResponseFor400](#update_workflow.ApiResponseFor400) | Validation error (attempted to update immutable field)
404 | [ApiResponseFor404](#update_workflow.ApiResponseFor404) | Workflow not found

#### update_workflow.ApiResponseFor200
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor200ResponseBodyApplicationJson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor200ResponseBodyApplicationJson
Type | Description  | Notes
------------- | ------------- | -------------
[**RemediationWorkflow**](../../models/RemediationWorkflow.md) |  | 


#### update_workflow.ApiResponseFor400
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor400ResponseBodyApplicationProblemjson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor400ResponseBodyApplicationProblemjson
Type | Description  | Notes
------------- | ------------- | -------------
[**RFC7807Problem**](../../models/RFC7807Problem.md) |  | 


#### update_workflow.ApiResponseFor404
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor404ResponseBodyApplicationProblemjson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor404ResponseBodyApplicationProblemjson
Type | Description  | Notes
------------- | ------------- | -------------
[**RFC7807Problem**](../../models/RFC7807Problem.md) |  | 


### Authorization

No authorization required

[[Back to top]](#__pageTop) [[Back to API list]](../../../README.md#documentation-for-api-endpoints) [[Back to Model list]](../../../README.md#documentation-for-models) [[Back to README]](../../../README.md)

