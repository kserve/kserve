# V1beta1LoggerSpec

LoggerSpec specifies optional payload logging available for all components
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**batch_interval** | **str** | Max duration to wait before flushing a partial batch (e.g. \&quot;5s\&quot;, \&quot;100ms\&quot;). Only used when BatchSize &gt; 1. Defaults to \&quot;0\&quot; (no time-based flushing). | [optional] 
**batch_size** | **int** | Number of log records per batch for blob storage. Defaults to 1 (immediate). | [optional] 
**marshaller_url** | **str** | URL of the log marshaller service that transforms log records before storage. Defaults to the embedded JSON marshaller at http://localhost:9083/marshal. | [optional] 
**metadata_annotations** | **list[str]** | Matched inference service annotations for propagating to inference logger cloud events. | [optional] 
**metadata_headers** | **list[str]** | Matched metadata HTTP headers for propagating to inference logger cloud events. | [optional] 
**mode** | **str** | Specifies the scope of the loggers. &lt;br /&gt; Valid values are: &lt;br /&gt; - \&quot;all\&quot; (default): log both request and response; &lt;br /&gt; - \&quot;request\&quot;: log only request; &lt;br /&gt; - \&quot;response\&quot;: log only response &lt;br /&gt; | [optional] 
**storage** | [**V1beta1LoggerStorageSpec**](V1beta1LoggerStorageSpec.md) |  | [optional] 
**url** | **str** | URL to send logging events | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


