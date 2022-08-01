# V1beta1ComponentExtensionSpec

ComponentExtensionSpec defines the deployment configuration for a given InferenceService component
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**batcher** | [**V1beta1Batcher**](V1beta1Batcher.md) |  | [optional] 
**canary_traffic_percent** | **int** | CanaryTrafficPercent defines the traffic split percentage between the candidate revision and the last ready revision | [optional] 
**container_concurrency** | **int** | ContainerConcurrency specifies how many requests can be processed concurrently, this sets the hard limit of the container concurrency(https://knative.dev/docs/serving/autoscaling/concurrency). | [optional] 
**logger** | [**V1beta1LoggerSpec**](V1beta1LoggerSpec.md) |  | [optional] 
**max_replicas** | **int** | Maximum number of replicas for autoscaling. | [optional] 
**min_replicas** | **int** | Minimum number of replicas, defaults to 1 but can be set to 0 to enable scale-to-zero. | [optional] 
**scale_metric** | **str** | ScaleMetric defines the scaling metric type watched by autoscaler possible values are concurrency, rps, cpu, memory. concurrency, rps are supported via Knative Pod Autoscaler(https://knative.dev/docs/serving/autoscaling/autoscaling-metrics). | [optional] 
**scale_target** | **int** | ScaleTarget specifies the integer target value of the metric type the Autoscaler watches for. concurrency and rps targets are supported by Knative Pod Autoscaler (https://knative.dev/docs/serving/autoscaling/autoscaling-targets/). | [optional] 
**timeout** | **int** | TimeoutSeconds specifies the number of seconds to wait before timing out a request to the component. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


