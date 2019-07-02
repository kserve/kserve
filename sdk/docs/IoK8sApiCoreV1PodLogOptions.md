# IoK8sApiCoreV1PodLogOptions

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**api_version** | **str** | APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#resources | [optional] 
**container** | **str** | The container for which to stream logs. Defaults to only container if there is one container in the pod. | [optional] 
**follow** | **bool** | Follow the log stream of the pod. Defaults to false. | [optional] 
**kind** | **str** | Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#types-kinds | [optional] 
**limit_bytes** | **int** | If set, the number of bytes to read from the server before terminating the log output. This may not display a complete final line of logging, and may return slightly more or slightly less than the specified limit. | [optional] 
**previous** | **bool** | Return previous terminated container logs. Defaults to false. | [optional] 
**since_seconds** | **int** | A relative time in seconds before the current time from which to show logs. If this value precedes the time a pod was started, only logs since the pod start will be returned. If this value is in the future, no logs will be returned. Only one of sinceSeconds or sinceTime may be specified. | [optional] 
**since_time** | [**IoK8sApimachineryPkgApisMetaV1Time**](IoK8sApimachineryPkgApisMetaV1Time.md) | An RFC3339 timestamp from which to show logs. If this value precedes the time a pod was started, only logs since the pod start will be returned. If this value is in the future, no logs will be returned. Only one of sinceSeconds or sinceTime may be specified. | [optional] 
**tail_lines** | **int** | If set, the number of lines from the end of the logs to show. If not specified, logs are shown from the creation of the container or sinceSeconds or sinceTime | [optional] 
**timestamps** | **bool** | If true, add an RFC3339 or RFC3339Nano timestamp at the beginning of every line of log output. Defaults to false. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


