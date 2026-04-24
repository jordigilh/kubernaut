# datastorage.EffectivenessAssessmentAPIApi

All URIs are relative to *http://localhost:8080*

Method | HTTP request | Description
------------- | ------------- | -------------
[**get_effectiveness_score**](EffectivenessAssessmentAPIApi.md#get_effectiveness_score) | **GET** /api/v1/effectiveness/{correlation_id} | Compute weighted effectiveness score on demand


# **get_effectiveness_score**
> EffectivenessScoreResponse get_effectiveness_score(correlation_id)

Compute weighted effectiveness score on demand

Computes the weighted effectiveness score for a given remediation lifecycle from component audit events in the audit trail.  **Architecture**: Per ADR-EM-001 Principle 5, DataStorage computes the overall score. The Effectiveness Monitor emits raw component assessment events; this endpoint aggregates them and applies the DD-017 v2.1 scoring formula:    score = (health_score * 0.40 + alert_score * 0.35 + metrics_score * 0.25) / total_weight  **Business Requirements**: BR-EM-001 to BR-EM-004  **Response includes**: - Weighted overall score (0.0 to 1.0) - Individual component scores (health, alert, metrics) - Hash comparison data (pre/post remediation spec hash per DD-EM-002) - Assessment status (no_data, in_progress, EffectivenessAssessed)  **Authentication**: Protected by OAuth-proxy in production/E2E. 

### Example


```python
import time
import os
import datastorage
from datastorage.models.effectiveness_score_response import EffectivenessScoreResponse
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
    api_instance = datastorage.EffectivenessAssessmentAPIApi(api_client)
    correlation_id = 'rr-prometheus-alert-highcpu-abc123' # str | The correlation ID (typically RemediationRequest name) that links all audit events in a remediation lifecycle. 

    try:
        # Compute weighted effectiveness score on demand
        api_response = api_instance.get_effectiveness_score(correlation_id)
        print("The response of EffectivenessAssessmentAPIApi->get_effectiveness_score:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling EffectivenessAssessmentAPIApi->get_effectiveness_score: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **correlation_id** | **str**| The correlation ID (typically RemediationRequest name) that links all audit events in a remediation lifecycle.  | 

### Return type

[**EffectivenessScoreResponse**](EffectivenessScoreResponse.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Effectiveness score computed successfully. |  -  |
**404** | Not Found - no effectiveness assessment events found for the given correlation_id.  |  -  |
**500** | Internal Server Error - database query or score computation failure.  |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

