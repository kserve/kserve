# KServe 2023 Roadmap

## Objective: "Graduate core inference capability to stable/GA"
- Promote `InferenceService` and `ClusterServingRuntime`/`ServingRuntime` CRD from v1beta1 to v1 
  * Improve gRPC yaml interface.
  * Unifying model storage spec and implementation between KServe and ModelMesh
  * Add Status to `ServingRuntime` for both ModelMesh and KServe, surface `ServingRuntime` validation errors and deployment status.
  * Deprecate `TrainedModel` CRD.
  * Collocate transformer and predictor in the pod.
  * Stablize `RawDeployment` mode.

- All model formats to support v2 inference protocol including custom serving runtime
  * TorchServe to support v2 gRPC inference protocol.
  * Support batching for v2 inference protocol.
  * Transformer and Explainer v2 inference protocol interoperability.
  * Improve codec for v2 inference protocol

Reference: [Control plane issues](https://github.com/kserve/kserve/issues?q=is%3Aissue+is%3Aopen+label%3Akserve%2Fcontrol-plane), [Data plane issues](https://github.com/kserve/kserve/issues?q=is%3Aissue+is%3Aopen+label%3Akfserving%2Fdata-plane)，[Serving Runtime issues](https://github.com/kserve/kserve/issues?q=is%3Aissue+is%3Aopen+label%3Akserve%2Fservingruntime).

## Objective: "Graduate KServe Python SDK to 1.0“

- Improve python sdk dependency management
- Stablize major cloud storage support
- Standarized model packaging API
- Improve KServe model server observability with metrics and distruted tracing
- Support batch inference

Reference：[Python SDK issues](https://github.com/kserve/kserve/issues?q=is%3Aissue+is%3Aopen+label%3Akserve%2Fsdk), [Storage issues](https://github.com/kserve/kserve/issues?q=is%3Aissue+is%3Aopen+label%3Akfserving%2Fstorage)

## Objective: "Graduate ModelMesh to beta"
- Support TorchServe ServingRuntime
- Add PVC support and unify storage implementation with KServe
- Add optional ingress for ModelMesh deployments
- Etcd secret security for multi-namespace mode
- Add estimated model size

Reference: [ModelMesh issues](https://github.com/kserve/modelmesh-serving/issues?page=1&q=is%3Aissue+is%3Aopen)

## Objective: "Graduate InferenceGraph to beta"
- Improve `InferenceGraph` spec for replica and concurrency control.
- Allow setting resource limits per `InferenceGraph`.
- Support distributed tracing.
- Support gRPC for `InferenceGraph`.
- Standalone `Transformer` support for `InferenceGraph`.
- Support traffic mirroring node.
- Support `RawDeployment` mode for `InferenceGraph`.

Reference: [InferenceGraph issues](https://github.com/kserve/kserve/issues?q=is%3Aissue+is%3Aopen+label%3Akserve%2Finference_graph)

## Objective: "Secure InferenceService"
- Document KServe ServiceMesh setup with mTLS
- Support programmatic authentication token
- Per service level auth
- SPIFFE/SPIRE identity integration with `InferenceService`

Reference: [Auth related issues](https://github.com/kserve/kserve/issues?q=is%3Aissue+is%3Aopen+label%3Akserve%2Fauth)
