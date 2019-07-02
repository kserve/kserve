# IoK8sApiCoreV1Event

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**action** | **str** | What action was taken/failed regarding to the Regarding object. | [optional] 
**api_version** | **str** | APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#resources | [optional] 
**count** | **int** | The number of times this event has occurred. | [optional] 
**event_time** | [**IoK8sApimachineryPkgApisMetaV1MicroTime**](IoK8sApimachineryPkgApisMetaV1MicroTime.md) | Time when this Event was first observed. | [optional] 
**first_timestamp** | [**IoK8sApimachineryPkgApisMetaV1Time**](IoK8sApimachineryPkgApisMetaV1Time.md) | The time at which the event was first recorded. (Time of server receipt is in TypeMeta.) | [optional] 
**involved_object** | [**IoK8sApiCoreV1ObjectReference**](IoK8sApiCoreV1ObjectReference.md) | The object that this event is about. | 
**kind** | **str** | Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#types-kinds | [optional] 
**last_timestamp** | [**IoK8sApimachineryPkgApisMetaV1Time**](IoK8sApimachineryPkgApisMetaV1Time.md) | The time at which the most recent occurrence of this event was recorded. | [optional] 
**message** | **str** | A human-readable description of the status of this operation. | [optional] 
**metadata** | [**IoK8sApimachineryPkgApisMetaV1ObjectMeta**](IoK8sApimachineryPkgApisMetaV1ObjectMeta.md) | Standard object&#39;s metadata. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata | 
**reason** | **str** | This should be a short, machine understandable string that gives the reason for the transition into the object&#39;s current status. | [optional] 
**related** | [**IoK8sApiCoreV1ObjectReference**](IoK8sApiCoreV1ObjectReference.md) | Optional secondary object for more complex actions. | [optional] 
**reporting_component** | **str** | Name of the controller that emitted this Event, e.g. &#x60;kubernetes.io/kubelet&#x60;. | [optional] 
**reporting_instance** | **str** | ID of the controller instance, e.g. &#x60;kubelet-xyzf&#x60;. | [optional] 
**series** | [**IoK8sApiCoreV1EventSeries**](IoK8sApiCoreV1EventSeries.md) | Data about the Event series this event represents or nil if it&#39;s a singleton Event. | [optional] 
**source** | [**IoK8sApiCoreV1EventSource**](IoK8sApiCoreV1EventSource.md) | The component reporting this event. Should be a short machine understandable string. | [optional] 
**type** | **str** | Type of this event (Normal, Warning), new types could be added in the future | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


