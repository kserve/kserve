# V1beta1ComponentExtensionSpec

ComponentExtensionSpec defines the deployment configuration for a given InferenceService component

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**annotations** | **Dict[str, str]** | Annotations that will be add to the component pod. More info: http://kubernetes.io/docs/user-guide/annotations | [optional] 
**batcher** | [**V1beta1Batcher**](V1beta1Batcher.md) |  | [optional] 
**canary_traffic_percent** | **int** | CanaryTrafficPercent defines the traffic split percentage between the candidate revision and the last ready revision | [optional] 
**container_concurrency** | **int** | ContainerConcurrency specifies how many requests can be processed concurrently, this sets the hard limit of the container concurrency(https://knative.dev/docs/serving/autoscaling/concurrency). | [optional] 
**labels** | **Dict[str, str]** | Labels that will be add to the component pod. More info: http://kubernetes.io/docs/user-guide/labels | [optional] 
**logger** | [**V1beta1LoggerSpec**](V1beta1LoggerSpec.md) |  | [optional] 
**max_replicas** | **int** | Maximum number of replicas for autoscaling. | [optional] 
**min_replicas** | **int** | Minimum number of replicas, defaults to 1 but can be set to 0 to enable scale-to-zero. | [optional] 
**scale_metric** | **str** | ScaleMetric defines the scaling metric type watched by autoscaler possible values are concurrency, rps, cpu, memory. concurrency, rps are supported via Knative Pod Autoscaler(https://knative.dev/docs/serving/autoscaling/autoscaling-metrics). | [optional] 
**scale_target** | **int** | ScaleTarget specifies the integer target value of the metric type the Autoscaler watches for. concurrency and rps targets are supported by Knative Pod Autoscaler (https://knative.dev/docs/serving/autoscaling/autoscaling-targets/). | [optional] 
**timeout** | **int** | TimeoutSeconds specifies the number of seconds to wait before timing out a request to the component. | [optional] 

## Example

```python
from kserve.models.v1beta1_component_extension_spec import V1beta1ComponentExtensionSpec

# TODO update the JSON string below
json = "{}"
# create an instance of V1beta1ComponentExtensionSpec from a JSON string
v1beta1_component_extension_spec_instance = V1beta1ComponentExtensionSpec.from_json(json)
# print the JSON string representation of the object
print V1beta1ComponentExtensionSpec.to_json()

# convert the object into a dict
v1beta1_component_extension_spec_dict = v1beta1_component_extension_spec_instance.to_dict()
# create an instance of V1beta1ComponentExtensionSpec from a dict
v1beta1_component_extension_spec_form_dict = v1beta1_component_extension_spec.from_dict(v1beta1_component_extension_spec_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


