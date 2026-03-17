# V1beta1ConfidentialSpec

ConfidentialSpec enables confidential model serving with encrypted model artifacts. When enabled, the storage initializer will decrypt model files using keys obtained from a Key Broker Service (KBS) via TEE attestation.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**enabled** | **bool** | Enabled controls whether confidential model serving is active. When true, the confidential storage initializer image is used and encrypted model artifacts are decrypted after download. | [default to False]
**resource_id** | **str** | ResourceId is the KBS resource identifier for the decryption key, in the format kbs:///&lt;repo&gt;/&lt;type&gt;/&lt;tag&gt;. If omitted, the storage initializer will attempt auto-discovery. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


