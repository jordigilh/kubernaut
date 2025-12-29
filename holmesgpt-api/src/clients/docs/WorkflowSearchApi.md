# datastorage.WorkflowSearchApi

All URIs are relative to *http://data-storage.kubernaut-system.svc.cluster.local:8080*

Method | HTTP request | Description
------------- | ------------- | -------------
[**search_workflows**](WorkflowSearchApi.md#search_workflows) | **POST** /api/v1/workflows/search | Search workflows using semantic search


# **search_workflows**
> WorkflowSearchResponse search_workflows(workflow_search_request)

Search workflows using semantic search

Search for remediation workflows using semantic search with hybrid weighted scoring.

**Business Requirements**:
- BR-STORAGE-013: Semantic search with hybrid weighted scoring

**Design Decisions**:
- DD-WORKFLOW-004: V1.0 uses confidence = base_similarity (no boost/penalty)


### Example


```python
import datastorage
from datastorage.models.workflow_search_request import WorkflowSearchRequest
from datastorage.models.workflow_search_response import WorkflowSearchResponse
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
    api_instance = datastorage.WorkflowSearchApi(api_client)
    workflow_search_request = datastorage.WorkflowSearchRequest() # WorkflowSearchRequest | 

    try:
        # Search workflows using semantic search
        api_response = api_instance.search_workflows(workflow_search_request)
        print("The response of WorkflowSearchApi->search_workflows:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling WorkflowSearchApi->search_workflows: %s\n" % e)
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
**200** | Search results |  -  |
**400** | Bad request - Invalid search parameters |  -  |
**500** | Internal server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

