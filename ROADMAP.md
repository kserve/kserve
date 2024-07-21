# KServe 2024-2025 Roadmap
## Objective: "Support GenAI inference"
- LLM Serving Runtimes
   * Support Speculative Decoding with vLLM runtime.
   * Support LoRA adapters.
   * Support LLM Serving runtimes for TensorRT-LLM, TGI and provide benchmarking comparisions.
   * Support multi-host, multi-GPU inference runtime.

- LLM Autoscaling
   * Support Model Caching with automatic PV/PVC provisioning.
   * Support Autoscaling settings for serving runtimes.
   * Support Autoscaling based on custom metrics.

- LLM RAG Pipeline Orchestration
   * Support declarative RAG with KServe Inference Graph.

- GenAI Task APIs
   * Community maintained Open Inference Protocol repo for OpenAI schema.
   * Support verical GenAI Task APIs such as embedding, Text-to-Image, Text-To-Code, Doc-To-Text.

- LLM Gateway
   * Support multiple LLM providers.
   * Support token based rating limiting.
   * Support LLM router with traffic shaping, fallback, load balancing.

## Objective: "Graduate core inference capability to stable/GA"
- Promote `InferenceService` and `ClusterServingRuntime`/`ServingRuntime` CRD to v1
  * Improve `InferenceService` CRD for REST/gRPC protocol interface
  * Unify model storage spec and implementation between KServe and ModelMesh
  * Deprecate `TrainedModel` CRD and `InferenceService` to support multiple models for model collocations, LoRA adapters.
  * Improve YAML UX for predictor and transformer container collocation.
  * Stablize `RawDeployment` mode with comprehensive testing for supported features
 
- Open Inference Protocol 
  * Support batching for v2 inference protocol
  * Transformer and Explainer v2 inference protocol interoperability
  * Improve codec for v2 inference protocol

Reference: [Control plane issues](https://github.com/kserve/kserve/issues?q=is%3Aissue+is%3Aopen+label%3Akserve%2Fcontrol-plane), [Data plane issues](https://github.com/kserve/kserve/issues?q=is%3Aissue+is%3Aopen+label%3Akfserving%2Fdata-plane)，[Serving Runtime issues](https://github.com/kserve/kserve/issues?q=is%3Aissue+is%3Aopen+label%3Akserve%2Fservingruntime).

## Objective: "Graduate KServe Python SDK to 1.0“

- Create standarized model packaging API
- Improve KServe model server observability with metrics and distruted tracing
- Support batch inference

Reference：[Python SDK issues](https://github.com/kserve/kserve/issues?q=is%3Aissue+is%3Aopen+label%3Akserve%2Fsdk), [Storage issues](https://github.com/kserve/kserve/issues?q=is%3Aissue+is%3Aopen+label%3Akfserving%2Fstorage)

## Objective: "Graduate ModelMesh to beta"
- Add optional ingress for ModelMesh deployments
- Etcd secret security for multi-namespace mode
- Add estimated model size field

Reference: [ModelMesh issues](https://github.com/kserve/modelmesh-serving/issues?page=1&q=is%3Aissue+is%3Aopen)

## Objective: "Graduate InferenceGraph to beta"
- Improve `InferenceGraph` spec for replica and concurrency control
- Support distributed tracing
- Support gRPC for `InferenceGraph`
- Standalone `Transformer` support for `InferenceGraph`
- Support traffic mirroring node
- Support `RawDeployment` mode for `InferenceGraph`

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
- Clean up the examples in kserve repo and unify them with the website's by creating one source of truth for example documentation
- Update any out-of-date documentation and make sure the website as a whole is consistent and cohesive
