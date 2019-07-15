# Tf2OpenAPI
This tool takes TensorFlow SavedModel files as inputs and generates OpenAPI 3.0 specifications for HTTP prediction requests. The SavedModel files must contain signature definitions (SignatureDefs) for models.

## Usage
```
Usage:
  tf2openapi [flags]

Required Flags:
  -m, --model_base_path string           Absolute path of SavedModel file

Flags:
  -h, --help                     help for tf2openapi
  -t, --metagraph_tags strings   All tags identifying desired MetaGraph
  -m, --model_base_path string   Absolute path of SavedModel file
  -n, --name string              Name of model (default "model")
  -o, --output_file string       Absolute path of file to write OpenAPI spec to
  -s, --signature_def string     Serving Signature Def Key
  -v, --version string           Model version (default "1")

```

## Overview
### Use Cases for OpenAPI
This project is motivated by the following uses of an OpenAPI specification:
* <b> Validate HTTP requests </b> (i.e. prediction requests) so the user can be notified of malformed inputs and correct them before running comparatively expensive computations on the model 
* <b> Automatically generate user-friendly documentation </b> or UI with the API and display sample payloads for users so that they can format inputs properly
* <b> Generate payloads </b> for benchmarking purposes, such as recording the response times of queries to an ML model

### Project: TensorFlow-to-OpenAPI Transformer
The outcome of this project is a TensorFlow-to-OpenAPI transformer which takes SavedModel protobuf binary files and generates an OpenAPI specification for prediction requests.

#### Design Decisions
There are numerous ways to format valid model input payloads to TFServing. Here are the formats this tool has chosen:
* For batchable inputs with -1 in all 0-dimensions, uses the [row format](https://www.tensorflow.org/tfx/serving/api_rest#specifying_input_tensors_in_row_format)
  * Single named input tensor: uses `[val1, val2, etc.]` instead of equally valid `[{tensor: val1}, {tensor: val2}, ..]`
  * Multiple named input tensors: uses `[{tensor1: val1, tensor2: val3, ..}, {tensor1: val2, tensor2: val4, ..}..]`
* For non-batchable inputs (e.g. scalars) and batchable inputs without -1 in the 0th dimensions of all tensors to indicate that they are batchable, uses the [column format](https://www.tensorflow.org/tfx/serving/api_rest#specifying_input_tensors_in_column_format)
  * Single named input tensor: uses `val` instead of equally valid `{tensor: val}`
  * Multiple named input tensors: uses `{tensor1: [val1, val2, ..], tensor2: [val3, val4, ..] ..}`

Tf2OpenAPI generates a row format payload whenever possible because it's more intuitive for a user to construct.

## Caveats
* There is a dependency on protobufs defined by TensorFlow, e.g. [tensorflow/core/protobuf](https://github.com/tensorflow/tensorflow/tree/master/tensorflow/core/protobuf). Specific protos must be compiled into Go using [protoc](https://github.com/golang/protobuf/tree/master/protoc-gen-go) in the order: tensorflow/core/lib/core/\*.proto, tensorflow/core/framework/\*.proto, tensorflow/core/protobuf/saver.proto, tensorflow/core/protobuf/meta_graph.proto, tensorflow/core/protobuf/saved_model.proto. See Makefile which will automate this.  

## TensorFlow Compatibility
* This tool is compatible with TensorFlow versions 1.xx up to and including 1.13.1. To make it compatible with future TensorFlow versions, you will need to compile the TensorFlow protos and convert them to the internal models. See [DEVELOPER_GUIDE](DEVELOPER_GUIDE.md) for potential issues and solutions.

