<a id="__pageTop"></a>
# datastorage.apis.tags.metrics_api.MetricsApi

All URIs are relative to *http://localhost:8080*

Method | HTTP request | Description
------------- | ------------- | -------------
[**get_metrics**](#get_metrics) | **get** /metrics | Prometheus metrics

# **get_metrics**
<a id="get_metrics"></a>
> str get_metrics()

Prometheus metrics

Exposes Prometheus metrics in text format.  **Metrics Exposed** (BR-STORAGE-019, GAP-10): - `datastorage_audit_traces_total{service,status}` - Audit write operations - `datastorage_audit_lag_seconds{service}` - Time between event and audit write - `datastorage_write_duration_seconds{table}` - Database write latency - `datastorage_validation_failures_total{field,reason}` - Validation errors 

### Example

```python
import datastorage
from datastorage.apis.tags import metrics_api
from pprint import pprint
# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)

# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = metrics_api.MetricsApi(api_client)

    # example, this endpoint has no required or optional parameters
    try:
        # Prometheus metrics
        api_response = api_instance.get_metrics()
        pprint(api_response)
    except datastorage.ApiException as e:
        print("Exception when calling MetricsApi->get_metrics: %s\n" % e)
```
### Parameters
This endpoint does not need any parameter.

### Return Types, Responses

Code | Class | Description
------------- | ------------- | -------------
n/a | api_client.ApiResponseWithoutDeserialization | When skip_deserialization is True this response is returned
200 | [ApiResponseFor200](#get_metrics.ApiResponseFor200) | Prometheus metrics

#### get_metrics.ApiResponseFor200
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor200ResponseBodyTextPlain, ] |  |
headers | Unset | headers were not defined |

# SchemaFor200ResponseBodyTextPlain

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
str,  | str,  |  | 

### Authorization

No authorization required

[[Back to top]](#__pageTop) [[Back to API list]](../../../README.md#documentation-for-api-endpoints) [[Back to Model list]](../../../README.md#documentation-for-models) [[Back to README]](../../../README.md)

