# IoK8sApiCoreV1PodExecOptions

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**api_version** | **str** | APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#resources | [optional] 
**command** | **list[str]** | Command is the remote command to execute. argv array. Not executed within a shell. | 
**container** | **str** | Container in which to execute the command. Defaults to only container if there is only one container in the pod. | [optional] 
**kind** | **str** | Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#types-kinds | [optional] 
**stderr** | **bool** | Redirect the standard error stream of the pod for this call. Defaults to true. | [optional] 
**stdin** | **bool** | Redirect the standard input stream of the pod for this call. Defaults to false. | [optional] 
**stdout** | **bool** | Redirect the standard output stream of the pod for this call. Defaults to true. | [optional] 
**tty** | **bool** | TTY if true indicates that a tty will be allocated for the exec call. Defaults to false. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


