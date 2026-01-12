# holmesgpt_api_client.RecoveryAnalysisApi

All URIs are relative to *http://localhost*

Method | HTTP request | Description
------------- | ------------- | -------------
[**recovery_analyze_endpoint_api_v1_recovery_analyze_post**](RecoveryAnalysisApi.md#recovery_analyze_endpoint_api_v1_recovery_analyze_post) | **POST** /api/v1/recovery/analyze | Recovery Analyze Endpoint


# **recovery_analyze_endpoint_api_v1_recovery_analyze_post**
> RecoveryResponse recovery_analyze_endpoint_api_v1_recovery_analyze_post(recovery_request)

Recovery Analyze Endpoint

Analyze failed action and provide recovery strategies

Business Requirement: BR-HAPI-001 (Recovery analysis endpoint)
Design Decision: DD-WORKFLOW-002 v2.4 - WorkflowCatalogToolset via SDK

Called by: AIAnalysis Controller (for recovery attempts after workflow failure)

### Example


```python
import holmesgpt_api_client
from holmesgpt_api_client.models.recovery_request import RecoveryRequest
from holmesgpt_api_client.models.recovery_response import RecoveryResponse
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
    api_instance = holmesgpt_api_client.RecoveryAnalysisApi(api_client)
    recovery_request = holmesgpt_api_client.RecoveryRequest() # RecoveryRequest | 

    try:
        # Recovery Analyze Endpoint
        api_response = api_instance.recovery_analyze_endpoint_api_v1_recovery_analyze_post(recovery_request)
        print("The response of RecoveryAnalysisApi->recovery_analyze_endpoint_api_v1_recovery_analyze_post:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling RecoveryAnalysisApi->recovery_analyze_endpoint_api_v1_recovery_analyze_post: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **recovery_request** | [**RecoveryRequest**](RecoveryRequest.md)|  | 

### Return type

[**RecoveryResponse**](RecoveryResponse.md)

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

