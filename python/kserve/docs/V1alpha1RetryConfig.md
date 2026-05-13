# V1alpha1RetryConfig

RetryConfig defines retry behavior for an inference step when it encounters transient failures. Retries use exponential backoff with jitter. Only 5xx status codes and connection errors are retried; 4xx errors are not retried.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**initial_delay_ms** | **int** | InitialDelayMilliseconds is the initial backoff delay in milliseconds before the first retry. Subsequent retries use exponential backoff with jitter. | [optional] 
**max_delay_ms** | **int** | MaxDelayMilliseconds is the maximum delay in milliseconds between retries. Defaults to 300000 (5 minutes). Set a lower value for latency-sensitive workloads. | [optional] 
**max_retries** | **int** | MaxRetries is the maximum number of retry attempts (not counting the initial request). | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


