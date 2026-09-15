# V1beta1MetricsSpec

MetricsSpec specifies how to scale based on a single metric (only `type` and one other matching field should be set at once).
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**external** | [**V1beta1ExternalMetricSource**](V1beta1ExternalMetricSource.md) |  | [optional] 
**name** | **str** | Name is the name of the trigger. Must be unique within the ScaledObject. Used by KEDA to identify triggers. | [optional] 
**podmetric** | [**V1beta1PodMetricSource**](V1beta1PodMetricSource.md) |  | [optional] 
**resource** | [**V1beta1ResourceMetricSource**](V1beta1ResourceMetricSource.md) |  | [optional] 
**type** | **str** | type is the type of metric source.  It should be one of \&quot;Resource\&quot;, \&quot;External\&quot;, \&quot;PodMetric\&quot;. \&quot;Resource\&quot; or \&quot;External\&quot; each mapping to a matching field in the object. | [default to '']
**use_cached_metrics** | **bool** | UseCachedMetrics determines whether KEDA should use cached metrics. Not supported for cpu, memory, or cron scalers. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


