# V1beta1LocalModelConfig

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**default_job_image** | **str** |  | [optional] 
**disable_volume_management** | **bool** |  | [optional] 
**enabled** | **bool** |  | [default to False]
**fs_group** | **int** |  | [optional] 
**job_namespace** | **str** |  | [default to '']
**job_ttl_seconds_after_finished** | **int** |  | [optional] 
**reconcilation_frequency_in_secs** | **int** |  | [optional] 
**shared_pvc_import_fs_group** | **int** | SharedPVCImportFSGroup is applied to shared-PVC import Jobs, which run in the cache&#39;s own namespace. It is independent of FSGroup, which applies only to per-node download Jobs in JobNamespace. Leave it unset to let namespace admission assign the group. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


