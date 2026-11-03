# Predict Protocol - Version 2

The *Predict Protocol, version 2* is a set of HTTP/REST and GRPC APIs
for inference / prediction servers. By implementing this protocol both
inference clients and servers will increase their utility and
portability by being able to operate seamlessly on platforms that have
standardized around this protocol.

The protocol is composed of a required set of APIs that must be
implemented by a compliant server. This required set of APIs is
described in [required_api.md](./required_api.md). The [GRPC proto
specification](grpc_predict_v2.proto)
for the required APIs is available.

The protocol supports an extension mechanism as a required part of the
API, but no specific extensions are required to be implemented by a
compliant server. Inference servers that have implemented extensions
and the link to those extensions are:

- Triton Inference Server:
  https://github.com/triton-inference-server/server/tree/master/docs/protocol

The protocol is not yet finalized and so feedback is welcome. To
provide feedback open an issue and prepend the title with "[Predict
Protocol V2]".
