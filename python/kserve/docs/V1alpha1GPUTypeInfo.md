# V1alpha1GPUTypeInfo

GPUTypeInfo describes GPU hardware on a node
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**cuda_version** | **str** | CUDA or ROCm version | [optional] 
**driver_version** | **str** | Driver version for this GPU type | [optional] 
**gpu_type** | **str** | GPU type identifier (from MCV detection) Examples: \&quot;Aldebaran/MI200\&quot;, \&quot;nvidia-a100-80gb\&quot; | [default to '']
**ids** | **list[int]** | GPU device IDs for this type (0-indexed) Example: [0, 1, 2, 3] means GPUs 0-3 are this type Supports heterogeneous nodes with mixed GPU types | 
**rocm_version** | **str** |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


