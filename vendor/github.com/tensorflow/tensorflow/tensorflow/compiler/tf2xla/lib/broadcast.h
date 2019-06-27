/* Copyright 2018 The TensorFlow Authors. All Rights Reserved.

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

#ifndef TENSORFLOW_COMPILER_TF2XLA_LIB_BROADCAST_H_
#define TENSORFLOW_COMPILER_TF2XLA_LIB_BROADCAST_H_

#include "absl/types/span.h"
#include "tensorflow/compiler/xla/client/xla_builder.h"
#include "tensorflow/compiler/xla/statusor.h"

namespace tensorflow {

// Broadcasts 'input' up to shape 'output_dims', using TensorFlow broadcasting
// rules. Supports broadcasting a dimension of size x to size x*y, i.e., tiling.
xla::StatusOr<xla::XlaOp> BroadcastTo(xla::XlaOp input,
                                      absl::Span<int64 const> output_dims);

}  // namespace tensorflow

#endif  // TENSORFLOW_COMPILER_TF2XLA_LIB_BROADCAST_H_
