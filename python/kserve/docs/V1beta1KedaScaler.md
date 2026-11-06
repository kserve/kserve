# V1beta1KedaScaler

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**idle_replica_count** | **int** | Number of idle replicas, Default: ignored, must be less than minReplicaCount | [optional] 
**max_replica_count** | **int** | Maximum number of replicas for autoscaling. | [optional] 
**min_replica_count** | **int** | Minimum number of replicas, default: 0 | [optional] 
**triggers** | [**list[GithubComKedacoreKedaV2ApisKedaV1alpha1ScaleTriggers]**](https://pkg.go.dev/github.com/kedacore/keda/v2/apis/keda/v1alpha1#ScaleTriggers) |  | [optional]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


