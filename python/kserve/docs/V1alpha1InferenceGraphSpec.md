# V1alpha1InferenceGraphSpec

InferenceGraphSpec defines the InferenceGraph spec
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**affinity** | [**V1Affinity**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1Affinity.md) |  | [optional] 
**max_replicas** | **int** | Maximum number of replicas for autoscaling. | [optional] 
**min_replicas** | **int** | Minimum number of replicas, defaults to 1 but can be set to 0 to enable scale-to-zero. | [optional] 
**node_name** | **str** | NodeName specifies the node name for the InferenceGraph. https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/ | [optional] 
**node_selector** | **dict(str, str)** | NodeSelector specifies the node selector for the InferenceGraph. https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/ | [optional] 
**nodes** | [**dict(str, V1alpha1InferenceRouter)**](V1alpha1InferenceRouter.md) | Map of InferenceGraph router nodes Each node defines the router which can be different routing types | 
**resources** | [**V1ResourceRequirements**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1ResourceRequirements.md) |  | [optional] 
**scale_metric** | **str** | ScaleMetric defines the scaling metric type watched by autoscaler possible values are concurrency, rps, cpu, memory. concurrency, rps are supported via Knative Pod Autoscaler(https://knative.dev/docs/serving/autoscaling/autoscaling-metrics). | [optional] 
**scale_target** | **int** | ScaleTarget specifies the integer target value of the metric type the Autoscaler watches for. concurrency and rps targets are supported by Knative Pod Autoscaler (https://knative.dev/docs/serving/autoscaling/autoscaling-targets/). | [optional] 
**service_account_name** | **str** | ServiceAccountName specifies the service account name for the InferenceGraph. https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/ | [optional] 
**timeout** | **int** | TimeoutSeconds specifies the number of seconds to wait before timing out a request to the component. | [optional] 
**tolerations** | [**list[V1Toleration]**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1Toleration.md) | Toleration specifies the toleration for the InferenceGraph. https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/ | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


