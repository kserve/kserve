# Tf2OpenAPI
This tool takes TensorFlow SavedModel files as inputs and generates OpenAPI 3.0 specifications for HTTP prediction requests. The SavedModel files must contain signature definitions (SignatureDefs) for models.

## Overview
### Use Cases for OpenAPI
This project is motivated by the following uses of an OpenAPI specification:
* <b> Validate HTTP requests </b> (i.e. prediction requests) so the user can be notified of malformed inputs and correct them before running comparatively expensive computations on the model 
* <b> Automatically generate user-friendly documentation </b> or UI with the API and display sample payloads for users so that they can format inputs properly
* <b> Generate payloads </b> for benchmarking purposes, such as recording the response times of queries to an ML model

### Project: TensorFlow-to-OpenAPI Transformer
The outcome of this project is a TensorFlow-to-OpenAPI transformer which takes SavedModel protobuf binary files and generates an OpenAPI specification for prediction requests.

## Caveats
* There is a dependency on protobufs defined by TensorFlow, e.g. [tensorflow/core/protobuf](https://github.com/tensorflow/tensorflow/tree/master/tensorflow/core/protobuf). Specific protos must be compiled into Go using [protoc](https://github.com/golang/protobuf/tree/master/protoc-gen-go) in the order: tensorflow/core/lib/core/\*.proto, tensorflow/core/framework/\*.proto, tensorflow/core/protobuf/saver.proto, tensorflow/core/protobuf/meta_graph.proto, tensorflow/core/protobuf/saved_model.proto. See Makefile which will automate this.  

  ## TensorFlow Compatibility
* This tool is compatible with TensorFlow versions 1.xx up to and including 1.13.1. To make it compatible with future TensorFlow versions, you will need to compile the TensorFlow protos and convert them to the internal models. See [DEVELOPER_GUIDE](DEVELOPER_GUIDE.md) for potential issues and solutions.
