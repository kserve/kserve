# V1beta1ScalerSpec

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**max_replicas** | **int** | Maximum number of replicas for autoscaling. | [optional] 
**metric_backend** | **str** | MetricsBackend defines the scaling metric type watched by autoscaler possible values are prometheus, graphite. | [optional] 
**metric_query** | **str** | Query to run to get metrics from MetricsBackend | [optional] 
**min_replicas** | **int** | Minimum number of replicas, defaults to 1 but can be set to 0 to enable scale-to-zero. | [optional] 
**query_parameters** | **str** | A comma-separated list of query Parameters to include while querying the MetricsBackend endpoint. | [optional] 
**query_time** | **str** | queryTime is relative time range to execute query against. specialized for graphite (https://graphite-api.readthedocs.io/en/latest/api.html#from-until) | [optional] 
**scale_metric** | **str** | ScaleMetric defines the scaling metric type watched by autoscaler possible values are concurrency, rps, cpu, memory. concurrency, rps are supported via Knative Pod Autoscaler(https://knative.dev/docs/serving/autoscaling/autoscaling-metrics). | [optional] 
**scale_metric_type** | **str** | Type of metric to use. Options are Utilization, or AverageValue. | [optional] 
**scale_target** | **int** | ScaleTarget specifies the integer target value of the metric type the Autoscaler watches for. concurrency and rps targets are supported by Knative Pod Autoscaler (https://knative.dev/docs/serving/autoscaling/autoscaling-targets/). | [optional] 
**server_address** | **str** | Address of MetricsBackend server. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


