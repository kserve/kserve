# V1alpha1KernelCacheCaptureSession

KernelCacheCaptureSession identifies one capture attempt.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**id** | **str** | ID uniquely identifies the capture session for a producer Pod. The sidecar includes this value in its status reports so the controller can reject stale reports from another capture session. | [default to '']
**node_name** | **str** | NodeName is the node assigned to the producer Pod. | [optional] 
**pod_name** | **str** |  | [default to '']
**requested_node_group** | **str** | RequestedNodeGroup is the node group requested by the producer Pod. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


