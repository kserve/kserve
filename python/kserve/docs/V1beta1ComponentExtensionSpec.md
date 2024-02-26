# V1beta1ComponentExtensionSpec

ComponentExtensionSpec defines the deployment configuration for a given InferenceService component
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**annotations** | **dict(str, str)** | Annotations that will be add to the component pod. More info: http://kubernetes.io/docs/user-guide/annotations | [optional] 
**batcher** | [**V1beta1Batcher**](V1beta1Batcher.md) |  | [optional] 
**canary_traffic_percent** | **int** | CanaryTrafficPercent defines the traffic split percentage between the candidate revision and the last ready revision | [optional] 
**container_concurrency** | **int** | ContainerConcurrency specifies how many requests can be processed concurrently, this sets the hard limit of the container concurrency(https://knative.dev/docs/serving/autoscaling/concurrency). | [optional] 
**labels** | **dict(str, str)** | Labels that will be add to the component pod. More info: http://kubernetes.io/docs/user-guide/labels | [optional] 
**logger** | [**V1beta1LoggerSpec**](V1beta1LoggerSpec.md) |  | [optional] 
**max_replicas** | **int** | Maximum number of replicas for autoscaling. | [optional] 
**min_ready_seconds** | **int** | Minimum number of seconds for which a newly created pod should be ready without any of its container crashing, for it to be considered available. Defaults to 0 (pod will be considered available as soon as it is ready) | [optional] 
**min_replicas** | **int** | Minimum number of replicas, defaults to 1 but can be set to 0 to enable scale-to-zero. | [optional] 
**paused** | **bool** | Indicates that the deployment is paused. | [optional] 
**progress_deadline_seconds** | **int** | The maximum time in seconds for a deployment to make progress before it is considered to be failed. The deployment controller will continue to process failed deployments and a condition with a ProgressDeadlineExceeded reason will be surfaced in the deployment status. Note that progress will not be estimated during the time a deployment is paused. Defaults to 600s. | [optional] 
**replicas** | **int** | Number of desired pods. This is a pointer to distinguish between explicit zero and not specified. Defaults to 1. | [optional] 
**revision_history_limit** | **int** | The number of old ReplicaSets to retain to allow rollback. This is a pointer to distinguish between explicit zero and not specified. Defaults to 10. | [optional] 
**scale_metric** | **str** | ScaleMetric defines the scaling metric type watched by autoscaler possible values are concurrency, rps, cpu, memory. concurrency, rps are supported via Knative Pod Autoscaler(https://knative.dev/docs/serving/autoscaling/autoscaling-metrics). | [optional] 
**scale_target** | **int** | ScaleTarget specifies the integer target value of the metric type the Autoscaler watches for. concurrency and rps targets are supported by Knative Pod Autoscaler (https://knative.dev/docs/serving/autoscaling/autoscaling-targets/). | [optional] 
**selector** | [**V1LabelSelector**](V1LabelSelector.md) |  | 
**strategy** | [**K8sIoApiAppsV1DeploymentStrategy**](K8sIoApiAppsV1DeploymentStrategy.md) |  | [optional] 
**template** | [**V1PodTemplateSpec**](V1PodTemplateSpec.md) |  | 
**timeout** | **int** | TimeoutSeconds specifies the number of seconds to wait before timing out a request to the component. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


