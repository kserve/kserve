# KnativeCondition

Conditions defines a readiness condition for a Knative resource. See: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**last_transition_time** | [**KnativeVolatileTime**](KnativeVolatileTime.md) | LastTransitionTime is the last time the condition transitioned from one status to another. We use VolatileTime in place of metav1.Time to exclude this from creating equality.Semantic differences (all other things held constant). | [optional] 
**message** | **str** | A human readable message indicating details about the transition. | [optional] 
**reason** | **str** | The reason for the condition&#39;s last transition. | [optional] 
**severity** | **str** | Severity with which to treat failures of this type of condition. When this is not specified, it defaults to Error. | [optional] 
**status** | **str** | Status of the condition, one of True, False, Unknown. | 
**type** | **str** | Type of condition. | 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


