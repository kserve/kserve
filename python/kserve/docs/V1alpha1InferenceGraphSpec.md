# V1alpha1InferenceGraphSpec

InferenceGraphSpec defines the InferenceGraph spec
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**affinity** | [**V1Affinity**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1Affinity.md) |  | [optional] 
**client_service_timeout_seconds** | **int** | ClientServiceTimeoutSeconds specifies a time limit for requests made to the graph components. | [optional] 
**max_replicas** | **int** | Maximum number of replicas for autoscaling. | [optional] 
**min_replicas** | **int** | Minimum number of replicas, defaults to 1 but can be set to 0 to enable scale-to-zero. | [optional] 
**nodes** | [**dict(str, V1alpha1InferenceRouter)**](V1alpha1InferenceRouter.md) | Map of InferenceGraph router nodes Each node defines the router which can be different routing types | 
**resources** | [**V1ResourceRequirements**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1ResourceRequirements.md) |  | [optional] 
**scale_metric** | **str** | ScaleMetric defines the scaling metric type watched by autoscaler possible values are concurrency, rps, cpu, memory. concurrency, rps are supported via Knative Pod Autoscaler(https://knative.dev/docs/serving/autoscaling/autoscaling-metrics). | [optional] 
**scale_target** | **int** | ScaleTarget specifies the integer target value of the metric type the Autoscaler watches for. concurrency and rps targets are supported by Knative Pod Autoscaler (https://knative.dev/docs/serving/autoscaling/autoscaling-targets/). | [optional] 
**server_idle_timeout_seconds** | **int** | ServerIdleTimeoutSeconds specifies the maximum amount of time to wait for the next request when keep-alives are enabled. | [optional] 
**server_read_timeout_seconds** | **int** | ServerReadTimeoutSeconds specifies the number of seconds to wait before timing out a request read by the server. | [optional] 
**server_write_timeout_seconds** | **int** | ServerWriteTimeoutSeconds specifies the maximum duration before timing out writes of the response. | [optional] 
**timeout** | **int** | TimeoutSeconds specifies the number of seconds to wait before timing out a request to the component. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


