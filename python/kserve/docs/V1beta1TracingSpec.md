# V1beta1TracingSpec

TracingSpec defines the distributed tracing configuration. When present (even as an empty object `{}`), tracing is enabled with defaults. When omitted, tracing is disabled.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**exporter** | **str** | Exporter specifies which exporter is used for traces. Maps to the OTEL_TRACES_EXPORTER environment variable. Default: \&quot;otlp\&quot; | [optional] 
**exporter_endpoint** | **str** | ExporterEndpoint is the OTLP exporter endpoint. Maps to the OTEL_EXPORTER_OTLP_ENDPOINT environment variable. Default: \&quot;http://otel-collector:4317\&quot; | [optional] 
**sampler** | **str** | Sampler specifies the sampler to use for traces. Maps to the OTEL_TRACES_SAMPLER environment variable. Default: \&quot;parentbased_traceidratio\&quot; | [optional] 
**sampler_arg** | **str** | SamplerArg is an argument passed to the traces sampler, such as a sampling ratio. Maps to the OTEL_TRACES_SAMPLER_ARG environment variable. Default: \&quot;0.05\&quot; | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


