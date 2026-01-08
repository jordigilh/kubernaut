# datastorage.MetricsApi

All URIs are relative to *http://localhost:8080*

Method | HTTP request | Description
------------- | ------------- | -------------
[**get_metrics**](MetricsApi.md#get_metrics) | **GET** /metrics | Prometheus metrics


# **get_metrics**
> str get_metrics()

Prometheus metrics

Exposes Prometheus metrics in text format.  **Metrics Exposed** (BR-STORAGE-019, GAP-10): - `datastorage_audit_traces_total{service,status}` - Audit write operations - `datastorage_audit_lag_seconds{service}` - Time between event and audit write - `datastorage_write_duration_seconds{table}` - Database write latency - `datastorage_validation_failures_total{field,reason}` - Validation errors 

### Example


```python
import time
import os
import datastorage
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
    api_instance = datastorage.MetricsApi(api_client)

    try:
        # Prometheus metrics
        api_response = api_instance.get_metrics()
        print("The response of MetricsApi->get_metrics:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling MetricsApi->get_metrics: %s\n" % e)
```



### Parameters

This endpoint does not need any parameter.

### Return type

**str**

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: text/plain

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Prometheus metrics |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

