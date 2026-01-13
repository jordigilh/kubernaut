# datastorage.AuditReconstructionAPIApi

All URIs are relative to *http://localhost:8080*

Method | HTTP request | Description
------------- | ------------- | -------------
[**reconstruct_remediation_request**](AuditReconstructionAPIApi.md#reconstruct_remediation_request) | **POST** /api/v1/audit/remediation-requests/{correlation_id}/reconstruct | Reconstruct RemediationRequest from audit trail


# **reconstruct_remediation_request**
> ReconstructionResponse reconstruct_remediation_request(correlation_id)

Reconstruct RemediationRequest from audit trail

Reconstructs a complete RemediationRequest CRD from audit trail events.  **Business Requirement**: BR-AUDIT-006 (SOC2 compliance)  **Workflow**: 1. Query audit events for given correlation_id 2. Parse gateway and orchestrator events 3. Map audit data to RR Spec/Status fields 4. Build complete Kubernetes-compliant CRD 5. Validate completeness and quality  **Use Cases**: - Disaster recovery (recreate lost RRs from audit trail) - Compliance audits (prove RR state at any point in time) - Debugging (understand RR evolution from audit events)  **Returns**: - Reconstructed RR in YAML format - Validation result (completeness percentage, warnings, errors)  **Authentication**: Protected by OAuth-proxy in production/E2E. Integration tests use mock X-Auth-Request-User header. 

### Example


```python
import time
import os
import datastorage
from datastorage.models.reconstruction_response import ReconstructionResponse
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
    api_instance = datastorage.AuditReconstructionAPIApi(api_client)
    correlation_id = 'rr-prometheus-alert-highcpu-abc123' # str | Unique correlation ID for the remediation lifecycle

    try:
        # Reconstruct RemediationRequest from audit trail
        api_response = api_instance.reconstruct_remediation_request(correlation_id)
        print("The response of AuditReconstructionAPIApi->reconstruct_remediation_request:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling AuditReconstructionAPIApi->reconstruct_remediation_request: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **correlation_id** | **str**| Unique correlation ID for the remediation lifecycle | 

### Return type

[**ReconstructionResponse**](ReconstructionResponse.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | RemediationRequest successfully reconstructed. Returns RR YAML and validation results.  |  -  |
**400** | Bad Request - reconstruction failed validation. Missing required audit events or invalid data.  |  -  |
**404** | Not Found - no audit events found for correlation_id.  |  -  |
**500** | Internal Server Error - database or reconstruction logic failure.  |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

