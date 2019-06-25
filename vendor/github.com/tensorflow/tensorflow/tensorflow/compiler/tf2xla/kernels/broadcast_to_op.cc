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

#include "tensorflow/compiler/tf2xla/lib/broadcast.h"
#include "tensorflow/compiler/tf2xla/xla_op_kernel.h"
#include "tensorflow/compiler/tf2xla/xla_op_registry.h"
#include "tensorflow/core/platform/macros.h"
#include "tensorflow/core/platform/types.h"

namespace tensorflow {
namespace {

class BroadcastToOp : public XlaOpKernel {
 public:
  explicit BroadcastToOp(OpKernelConstruction* context)
      : XlaOpKernel(context) {}

  void Compile(XlaOpKernelContext* context) override {
    const TensorShape input_shape = context->InputShape(0);
    TensorShape output_shape;
    OP_REQUIRES_OK(context, context->ConstantInputAsShape(1, &output_shape));

    auto output = BroadcastTo(context->Input(0), output_shape.dim_sizes());
    OP_REQUIRES_OK(context, output.status());
    context->SetOutput(0, output.ValueOrDie());
  }
};

REGISTER_XLA_OP(Name("BroadcastTo").CompileTimeConstantInput("shape"),
                BroadcastToOp);

}  // namespace
}  // namespace tensorflow
