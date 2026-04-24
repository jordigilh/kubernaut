# datastorage.RemediationHistoryAPIApi

All URIs are relative to *http://localhost:8080*

Method | HTTP request | Description
------------- | ------------- | -------------
[**get_remediation_history_context**](RemediationHistoryAPIApi.md#get_remediation_history_context) | **GET** /api/v1/remediation-history/context | Get remediation history context for a target resource


# **get_remediation_history_context**
> RemediationHistoryContext get_remediation_history_context(target_kind, target_name, target_namespace, current_spec_hash, tier1_window=tier1_window, tier2_window=tier2_window)

Get remediation history context for a target resource

Returns structured remediation history context for LLM prompt enrichment.  **Business Requirements**: BR-HAPI-016 (Remediation history context)  **Design Document**: DD-HAPI-016  **Behavior**: Aggregates `remediation.workflow_created` (RO) and `effectiveness.assessment.completed` (EM) audit events into structured remediation chains for a target resource.  **Two-Tier Query Design**: - **Tier 1** (default 24h): Detailed remediation chain with health checks, metric deltas,   and full effectiveness data for the target resource. - **Tier 2** (default 90d): Summary chain activated when `currentSpecHash` matches a   historical `preRemediationSpecHash` beyond the Tier 1 window, indicating configuration   regression.  **Hash Comparison**: For each entry, performs three-way comparison of `currentSpecHash` against `preRemediationSpecHash` and `postRemediationSpecHash`.  **Regression Detection**: Sets `regressionDetected: true` if any entry's `preRemediationSpecHash` matches `currentSpecHash`.  **Authentication**: Protected by OAuth-proxy in production/E2E. Integration tests use mock X-Auth-Request-User header. 

### Example


```python
import time
import os
import datastorage
from datastorage.models.remediation_history_context import RemediationHistoryContext
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
    api_instance = datastorage.RemediationHistoryAPIApi(api_client)
    target_kind = 'Deployment' # str | Kubernetes resource kind (e.g. Deployment, StatefulSet)
    target_name = 'my-app' # str | Kubernetes resource name
    target_namespace = 'prod' # str | Kubernetes resource namespace
    current_spec_hash = 'sha256:aabb112233dd' # str | SHA-256 hash of the current target resource spec (canonical JSON)
    tier1_window = '24h' # str | Tier 1 lookback window (default 24h). Accepts Go duration strings. (optional) (default to '24h')
    tier2_window = '2160h' # str | Tier 2 lookback window (default 2160h / 90d). Accepts Go duration strings. (optional) (default to '2160h')

    try:
        # Get remediation history context for a target resource
        api_response = api_instance.get_remediation_history_context(target_kind, target_name, target_namespace, current_spec_hash, tier1_window=tier1_window, tier2_window=tier2_window)
        print("The response of RemediationHistoryAPIApi->get_remediation_history_context:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling RemediationHistoryAPIApi->get_remediation_history_context: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **target_kind** | **str**| Kubernetes resource kind (e.g. Deployment, StatefulSet) | 
 **target_name** | **str**| Kubernetes resource name | 
 **target_namespace** | **str**| Kubernetes resource namespace | 
 **current_spec_hash** | **str**| SHA-256 hash of the current target resource spec (canonical JSON) | 
 **tier1_window** | **str**| Tier 1 lookback window (default 24h). Accepts Go duration strings. | [optional] [default to &#39;24h&#39;]
 **tier2_window** | **str**| Tier 2 lookback window (default 2160h / 90d). Accepts Go duration strings. | [optional] [default to &#39;2160h&#39;]

### Return type

[**RemediationHistoryContext**](RemediationHistoryContext.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Remediation history context retrieved successfully. May contain Tier 1 (recent detailed), Tier 2 (historical summary), or both. Empty chains indicate no remediation history for the target.  |  -  |
**400** | Bad Request - missing required query parameters.  |  -  |
**500** | Internal Server Error - database query failure.  |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

