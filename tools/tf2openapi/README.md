# Tf2OpenAPI
The tool enables generating OpenAPI 3.0 specifications for prediction HTTP requests from TensorFlow SavedModel files. These files contain signature definitions (SignatureDefs) for models. 

## Overview
### Use Cases for OpenAPI
This project is motivated by the following uses of an OpenAPI specification:
* <b> Validate HTTP requests </b> (i.e. prediction requests) so the user can be notified of malformed inputs and correct them before running comparatively expensive computations on the model 
* <b> Automatically generate user-friendly documentation </b> or UI with the API and display sample payloads for users so that they can format inputs properly
* <b> Generate payloads </b> for benchmarking purposes, such as recording the response times of queries to an ML model

### Project: TensorFlow-to-OpenAPI Transformer
The outcome of this project is a TensorFlow-to-OpenAPI transformer which takes SavedModel protobuf binary files and generates an OpenAPI specification for prediction requests.

## Caveats
* There is a dependency on protobufs defined by TensorFlow, e.g. [tensorflow/core/protobuf](https://github.com/tensorflow/tensorflow/tree/master/tensorflow/core/protobuf). When compiling proto source files from the TensorFlow repository, ensure that the field `option go_package` is defined; otherwise, there will be missing Go files, causing problems with Go imports.
