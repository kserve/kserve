# KServe 2024-2025 Roadmap
## Objective: "Support GenAI inference"
- LLM Serving Runtimes
   * Support Speculative Decoding with vLLM runtime [https://github.com/kserve/kserve/issues/3800].
   * Support LoRA adapters [https://github.com/kserve/kserve/issues/3750].
   * Support LLM Serving runtimes for TensorRT-LLM, TGI and provide benchmarking comparisons [https://github.com/kserve/kserve/issues/3868].
   * Support multi-host, multi-GPU inference runtime [https://github.com/kserve/kserve/issues/2145].

- LLM Autoscaling
   * Support Model Caching with automatic PV/PVC provisioning [https://github.com/kserve/kserve/issues/3869].
   * Support Autoscaling settings for serving runtimes.
   * Support Autoscaling based on custom metrics [https://github.com/kserve/kserve/issues/3561].

- LLM RAG/Agent Pipeline Orchestration
   * Support declarative RAG/Agent workflow using KServe Inference Graph [https://github.com/kserve/kserve/issues/3829].

-  Open Inference Protocol extension to GenAI Task APIs
   * Community-maintained Open Inference Protocol repo for OpenAI schema [https://docs.google.com/document/d/1odTMdIFdm01CbRQ6CpLzUIGVppHSoUvJV_zwcX6GuaU].
   * Support vertical GenAI Task APIs such as embedding, Text-to-Image, Text-To-Code, Doc-To-Text [https://github.com/kserve/kserve/issues/3572].

- LLM Gateway
   * Support multiple LLM providers.
   * Support token based rate limiting.
   * Support LLM router with traffic shaping, fallback, load balancing.
   * LLM Gateway observability for metrics and cost reporting

## Objective: "Graduate core inference capability to stable/GA"
- Promote `InferenceService` and `ClusterServingRuntime`/`ServingRuntime` CRD to v1
  * Improve `InferenceService` CRD for REST/gRPC protocol interface
  * Improve model storage interface 
  * Deprecate `TrainedModel` CRD and add multiple model support for co-hosting, draft model, LoRA adapters to InferenceService.
  * Improve YAML UX for predictor and transformer container collocation.
  * Close the feature gap between `RawDeployment` and `Serverless` mode.

- Open Inference Protocol 
  * Support batching for v2 inference protocol
  * Transformer and Explainer v2 inference protocol interoperability
  * Improve codec for v2 inference protocol

Reference: [Control plane issues](https://github.com/kserve/kserve/issues?q=is%3Aissue+is%3Aopen+label%3Akserve%2Fcontrol-plane), [Data plane issues](https://github.com/kserve/kserve/issues?q=is%3Aissue+is%3Aopen+label%3Akfserving%2Fdata-plane)，[Serving Runtime issues](https://github.com/kserve/kserve/issues?q=is%3Aissue+is%3Aopen+label%3Akserve%2Fservingruntime).

## Objective: "Graduate KServe Python SDK to 1.0“

- Create standardized model packaging API
- Improve KServe model server observability with metrics and distributed tracing
- Support batch inference

Reference：[Python SDK issues](https://github.com/kserve/kserve/issues?q=is%3Aissue+is%3Aopen+label%3Akserve%2Fsdk), [Storage issues](https://github.com/kserve/kserve/issues?q=is%3Aissue+is%3Aopen+label%3Akfserving%2Fstorage)

## Objective: "Graduate InferenceGraph"
- Improve `InferenceGraph` spec for replica and concurrency control
- Support distributed tracing
- Support gRPC for `InferenceGraph`
- Standalone `Transformer` support for `InferenceGraph`
- Support traffic mirroring node
- Improve `RawDeployment` mode for `InferenceGraph`

Reference: [InferenceGraph issues](https://github.com/kserve/kserve/issues?q=is%3Aissue+is%3Aopen+label%3Akserve%2Finference_graph)

## Objective: "Secure InferenceService"
- Document KServe ServiceMesh setup with mTLS
- Support programmatic authentication token
- Implement per service level auth
- Add support for SPIFFE/SPIRE identity integration with `InferenceService`

Reference: [Auth related issues](https://github.com/kserve/kserve/issues?q=is%3Aissue+is%3Aopen+label%3Akserve%2Fauth)

## Objective: "KServe 1.0 documentation"
- Add ModelMesh docs and explain the use cases for classic KServe and ModelMesh
- Unify the data plane v1 and v2 page formats
- Improve v2 data plane docs to tell the story why and what changed
- Clean up the examples in kserve repo and unify them with the website's by creating one source of truth for documentation
- Update any out-of-date documentation and make sure the website as a whole is consistent and cohesive
