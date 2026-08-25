# V1beta1PodMetrics

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**backend** | **str** | Backend defines the scaling metric type watched by the autoscaler. Possible value: opentelemetry. | [optional] [default to '']
**metric_names** | **list[str]** | MetricNames is the list of metric names in the backend. | [optional] 
**operation_over_time** | **str** | OperationOverTime specifies the operation to aggregate the metrics over time. Possible values are last_one, avg, max, min, rate, count. Default is &#39;last_one&#39;. | [optional] 
**query** | **str** | Query specifies the query to run to get metrics from the PodsMetricsBackend. | [optional] 
**server_address** | **str** | ServerAddress specifies the address of the PodsMetricsBackend server. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


