# V1alpha1ServingRuntimePodSpec


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**affinity** | [**V1Affinity**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1Affinity.md) |  | [optional] 
**annotations** | **Dict[str, str]** | Annotations that will be add to the pod. More info: http://kubernetes.io/docs/user-guide/annotations | [optional] 
**containers** | [**List[V1Container]**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1Container.md) | List of containers belonging to the pod. Containers cannot currently be added or removed. There must be at least one container in a Pod. Cannot be updated. | 
**image_pull_secrets** | [**List[V1LocalObjectReference]**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1LocalObjectReference.md) | ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the images used by this PodSpec. If specified, these secrets will be passed to individual puller implementations for them to use. For example, in the case of docker, only DockerConfig type secrets are honored. More info: https://kubernetes.io/docs/concepts/containers/images#specifying-imagepullsecrets-on-a-pod | [optional] 
**labels** | **Dict[str, str]** | Labels that will be add to the pod. More info: http://kubernetes.io/docs/user-guide/labels | [optional] 
**node_selector** | **Dict[str, str]** | NodeSelector is a selector which must be true for the pod to fit on a node. Selector which must match a node&#39;s labels for the pod to be scheduled on that node. More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/ | [optional] 
**tolerations** | [**List[V1Toleration]**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1Toleration.md) | If specified, the pod&#39;s tolerations. | [optional] 
**volumes** | [**List[V1Volume]**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1Volume.md) | List of volumes that can be mounted by containers belonging to the pod. More info: https://kubernetes.io/docs/concepts/storage/volumes | [optional] 

## Example

```python
from kserve.models.v1alpha1_serving_runtime_pod_spec import V1alpha1ServingRuntimePodSpec

# TODO update the JSON string below
json = "{}"
# create an instance of V1alpha1ServingRuntimePodSpec from a JSON string
v1alpha1_serving_runtime_pod_spec_instance = V1alpha1ServingRuntimePodSpec.from_json(json)
# print the JSON string representation of the object
print V1alpha1ServingRuntimePodSpec.to_json()

# convert the object into a dict
v1alpha1_serving_runtime_pod_spec_dict = v1alpha1_serving_runtime_pod_spec_instance.to_dict()
# create an instance of V1alpha1ServingRuntimePodSpec from a dict
v1alpha1_serving_runtime_pod_spec_form_dict = v1alpha1_serving_runtime_pod_spec.from_dict(v1alpha1_serving_runtime_pod_spec_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


