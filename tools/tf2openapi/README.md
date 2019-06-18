# Tf2OpenAPI
The tool enables generating OpenAPI 3.0 specifications for prediction HTTP requests from TensorFlow SavedModel files containing signature definitions (SignatureDefs) for models. One use case for the OpenAPI specification involves generating sample payloads in a UI.

## Overview
### Use Cases for OpenAPI
This project is motivated by the following uses of an OpenAPI specification:
* <b> Validate HTTP requests </b> (i.e. prediction requests) so the user can be notified of malformed inputs and correct them before running comparatively expensive computations on the model 
* <b> Automatically generate user-friendly documentation </b> or UI with the API and display sample payloads for users so that they can format inputs properly
* <b> Generate payloads </b> for benchmarking purposes, such as recording the response times of queries to an ML model

### Project: TensorFlow-to-OpenAPI Transformer
The outcome of this project is a TensorFlow-to-OpenAPI transformer which takes SavedModel protobuf binary files and generates an OpenAPI specification for prediction requests.

## Caveats
* There is a dependency on protobufs defined by TensorFlow, e.g. [tensorflow/core/protobuf](https://github.com/tensorflow/tensorflow/tree/master/tensorflow/core/protobuf). Specific protos must be compiled into Go using [protoc](https://github.com/golang/protobuf/tree/master/protoc-gen-go) in the order: tensorflow/core/lib/core/\*.proto, tensorflow/core/framework/\*.proto, tensorflow/core/protobuf/meta_graph.proto, tensorflow/core/protobuf/saved_model.proto, tensorflow/core/protobuf/saver.proto  
