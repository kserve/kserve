/* Copyright 2017 The TensorFlow Authors. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
==============================================================================*/

#include "tensorflow/core/framework/common_shape_fns.h"
#include "tensorflow/core/framework/op.h"
#include "tensorflow/core/framework/shape_inference.h"

namespace tensorflow {

REGISTER_OP("_XlaSendFromHost")
    .Input("inputs: Tinputs")
    .Input("dynamic_key: string")
    .Attr("Tinputs: list(type) >= 0")
    .Attr("key: string")
    .Attr("device_ordinal: int")
    .SetIsStateful()
    .SetShapeFn(::tensorflow::shape_inference::NoOutputs)
    .Doc(R"doc(
A placeholder op for multiple values that will be sent from TensorFlow to a
running XLA computation.

inputs: A list of tensors that will be sent to the XLA computation.
dynamic_key: The key sent at runtime by the compile node to identify which
execution the transfer corresponds to.
Tinputs: The element types of each element in `inputs`.
key: A key that is unique in the computation and associates the send with the consumer in
the XLA computation.
device_ordinal: The device to use.
)doc");

REGISTER_OP("_XlaRecvAtHost")
    .Input("dynamic_key: string")
    .Output("outputs: Toutputs")
    .Attr("Toutputs: list(type) >= 0")
    .Attr("key: string")
    .Attr("device_ordinal: int")
    .SetIsStateful()
    .SetShapeFn(::tensorflow::shape_inference::UnknownShape)
    .Doc(R"doc(
A placeholder op for multiple values that will be sent to TensorFlow from a
running XLA computation.

dynamic_key: The key sent at runtime by the compile node to identify which
execution the transfer corresponds to.
outputs: A list of tensors that will be received from the XLA computation.
Toutputs: The element types of each element in `outputs`.
key: A key that is unique in the computation and associates the send with the consumer in
the XLA computation.
device_ordinal: The device to use.
)doc");

}  // namespace tensorflow
