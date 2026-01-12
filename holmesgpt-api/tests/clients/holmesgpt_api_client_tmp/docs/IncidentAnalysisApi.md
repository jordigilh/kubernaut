# holmesgpt_api_client.IncidentAnalysisApi

All URIs are relative to *http://localhost*

Method | HTTP request | Description
------------- | ------------- | -------------
[**incident_analyze_endpoint_api_v1_incident_analyze_post**](IncidentAnalysisApi.md#incident_analyze_endpoint_api_v1_incident_analyze_post) | **POST** /api/v1/incident/analyze | Incident Analyze Endpoint


# **incident_analyze_endpoint_api_v1_incident_analyze_post**
> IncidentResponse incident_analyze_endpoint_api_v1_incident_analyze_post(incident_request)

Incident Analyze Endpoint

Analyze initial incident and provide RCA + workflow selection

Business Requirement: BR-HAPI-002 (Incident analysis endpoint)
Business Requirement: BR-WORKFLOW-001 (MCP Workflow Integration)

Called by: AIAnalysis Controller (for initial incident RCA and workflow selection)

### Example


```python
import holmesgpt_api_client
from holmesgpt_api_client.models.incident_request import IncidentRequest
from holmesgpt_api_client.models.incident_response import IncidentResponse
from holmesgpt_api_client.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost
# See configuration.py for a list of all supported configuration parameters.
configuration = holmesgpt_api_client.Configuration(
    host = "http://localhost"
)


# Enter a context with an instance of the API client
with holmesgpt_api_client.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = holmesgpt_api_client.IncidentAnalysisApi(api_client)
    incident_request = holmesgpt_api_client.IncidentRequest() # IncidentRequest | 

    try:
        # Incident Analyze Endpoint
        api_response = api_instance.incident_analyze_endpoint_api_v1_incident_analyze_post(incident_request)
        print("The response of IncidentAnalysisApi->incident_analyze_endpoint_api_v1_incident_analyze_post:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling IncidentAnalysisApi->incident_analyze_endpoint_api_v1_incident_analyze_post: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **incident_request** | [**IncidentRequest**](IncidentRequest.md)|  | 

### Return type

[**IncidentResponse**](IncidentResponse.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successful Response |  -  |
**422** | Validation Error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

