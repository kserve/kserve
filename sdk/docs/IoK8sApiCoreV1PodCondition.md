# IoK8sApiCoreV1PodCondition

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**last_probe_time** | [**IoK8sApimachineryPkgApisMetaV1Time**](IoK8sApimachineryPkgApisMetaV1Time.md) | Last time we probed the condition. | [optional] 
**last_transition_time** | [**IoK8sApimachineryPkgApisMetaV1Time**](IoK8sApimachineryPkgApisMetaV1Time.md) | Last time the condition transitioned from one status to another. | [optional] 
**message** | **str** | Human-readable message indicating details about last transition. | [optional] 
**reason** | **str** | Unique, one-word, CamelCase reason for the condition&#39;s last transition. | [optional] 
**status** | **str** | Status is the status of the condition. Can be True, False, Unknown. More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#pod-conditions | 
**type** | **str** | Type is the type of the condition. More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#pod-conditions | 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


