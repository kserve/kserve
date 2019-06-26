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

// XLA-specific sequence and range Ops.

#include "tensorflow/compiler/tf2xla/xla_helpers.h"
#include "tensorflow/compiler/tf2xla/xla_op_kernel.h"
#include "tensorflow/compiler/tf2xla/xla_op_registry.h"
#include "tensorflow/compiler/xla/client/xla_builder.h"
#include "tensorflow/compiler/xla/literal.h"
#include "tensorflow/compiler/xla/primitive_util.h"
#include "tensorflow/core/framework/op_kernel.h"
#include "tensorflow/core/framework/register_types.h"
#include "tensorflow/core/framework/tensor.h"
#include "tensorflow/core/framework/tensor_shape.h"
#include "tensorflow/core/framework/types.h"

namespace tensorflow {
namespace {

// The type-specific part of the implementation of Range.
template <typename T>
xla::StatusOr<xla::XlaOp> CreateRangeTensor(
    const xla::LiteralSlice& start_literal,
    const xla::LiteralSlice& limit_literal,
    const xla::LiteralSlice& delta_literal, xla::XlaBuilder* builder) {
  T start = start_literal.Get<T>({});
  T limit = limit_literal.Get<T>({});
  T delta = delta_literal.Get<T>({});

  if (delta == 0) {
    return errors::InvalidArgument("Requires delta != 0: ", delta);
  }
  if (delta > 0) {
    if (start > limit) {
      return errors::InvalidArgument(
          "Requires start <= limit when delta > 0: ", start, "/", limit);
    }
  } else {
    if (start < limit) {
      return errors::InvalidArgument(
          "Requires start >= limit when delta < 0: ", start, "/", limit);
    }
  }
  int64 size =
      (std::is_integral<T>::value
           ? ((std::abs(limit - start) + std::abs(delta) - 1) / std::abs(delta))
           : std::ceil(std::abs((limit - start) / delta)));

  return xla::ConstantR0(builder, start) +
         xla::ConstantR0(builder, delta) *
             xla::Iota(builder, xla::primitive_util::NativeToPrimitiveType<T>(),
                       size);
}

class RangeOp : public XlaOpKernel {
 public:
  explicit RangeOp(OpKernelConstruction* ctx) : XlaOpKernel(ctx) {}

  void Compile(XlaOpKernelContext* ctx) override {
    const TensorShape start_in_shape = ctx->InputShape(0);
    const TensorShape limit_in_shape = ctx->InputShape(1);
    const TensorShape delta_in_shape = ctx->InputShape(2);
    OP_REQUIRES(ctx, TensorShapeUtils::IsScalar(start_in_shape),
                errors::InvalidArgument("start must be a scalar, not shape ",
                                        start_in_shape.DebugString()));
    OP_REQUIRES(ctx, TensorShapeUtils::IsScalar(limit_in_shape),
                errors::InvalidArgument("limit must be a scalar, not shape ",
                                        limit_in_shape.DebugString()));
    OP_REQUIRES(ctx, TensorShapeUtils::IsScalar(delta_in_shape),
                errors::InvalidArgument("delta must be a scalar, not shape ",
                                        delta_in_shape.DebugString()));
    xla::Literal start, limit, delta;
    OP_REQUIRES_OK(ctx, ctx->ConstantInput(0, &start));
    OP_REQUIRES_OK(ctx, ctx->ConstantInput(1, &limit));
    OP_REQUIRES_OK(ctx, ctx->ConstantInput(2, &delta));

    DataType type = input_type(0);
    xla::StatusOr<xla::XlaOp> output;
    switch (type) {
      case DT_INT32:
        output = CreateRangeTensor<int32>(start, limit, delta, ctx->builder());
        break;
      case DT_INT64:
        output = CreateRangeTensor<int64>(start, limit, delta, ctx->builder());
        break;
      case DT_FLOAT:
        output = CreateRangeTensor<float>(start, limit, delta, ctx->builder());
        break;
      case DT_DOUBLE:
        output = CreateRangeTensor<double>(start, limit, delta, ctx->builder());
        break;
      default:
        output = errors::InvalidArgument("Invalid type for Range ",
                                         DataTypeString(type));
    }
    OP_REQUIRES_OK(ctx, output.status());
    ctx->SetOutput(0, output.ValueOrDie());
  }
};

REGISTER_XLA_OP(Name("Range")
                    .CompileTimeConstantInput("start")
                    .CompileTimeConstantInput("limit")
                    .CompileTimeConstantInput("delta"),
                RangeOp);

class LinSpaceOp : public XlaOpKernel {
 public:
  explicit LinSpaceOp(OpKernelConstruction* ctx) : XlaOpKernel(ctx) {}

  void Compile(XlaOpKernelContext* ctx) override {
    const TensorShape start_in_shape = ctx->InputShape("start");
    const TensorShape stop_in_shape = ctx->InputShape("stop");
    const TensorShape num_in_shape = ctx->InputShape("num");
    OP_REQUIRES(ctx, TensorShapeUtils::IsScalar(start_in_shape),
                errors::InvalidArgument("start must be a scalar, not shape ",
                                        start_in_shape.DebugString()));
    OP_REQUIRES(ctx, TensorShapeUtils::IsScalar(stop_in_shape),
                errors::InvalidArgument("stop must be a scalar, not shape ",
                                        stop_in_shape.DebugString()));
    OP_REQUIRES(ctx, TensorShapeUtils::IsScalar(num_in_shape),
                errors::InvalidArgument("num must be a scalar, not shape ",
                                        num_in_shape.DebugString()));

    DataType type = ctx->input_type(0);

    int64 num;
    OP_REQUIRES_OK(ctx, ctx->ConstantInputAsIntScalar("num", &num));
    OP_REQUIRES(ctx, num > 0,
                errors::InvalidArgument("Requires num > 0: ", num));
    Tensor out_constant(type, TensorShape({num}));

    xla::Literal start_literal;
    OP_REQUIRES_OK(ctx, ctx->ConstantInput("start", &start_literal));
    xla::Literal stop_literal;
    OP_REQUIRES_OK(ctx, ctx->ConstantInput("stop", &stop_literal));

    switch (type) {
      case DT_FLOAT: {
        float start = start_literal.GetFirstElement<float>();
        float stop = stop_literal.GetFirstElement<float>();
        auto flat = out_constant.flat<float>();
        if (num == 1) {
          flat(0) = start;
        } else {
          const float step = (stop - start) / (num - 1);
          for (int64 i = 0; i < num; ++i) {
            flat(i) = start + step * i;
          }
        }
        break;
      }
      case DT_DOUBLE: {
        double start = start_literal.GetFirstElement<double>();
        double stop = stop_literal.GetFirstElement<double>();
        auto flat = out_constant.flat<double>();
        if (num == 1) {
          flat(0) = start;
        } else {
          const double step = (stop - start) / (num - 1);
          for (int64 i = 0; i < num; ++i) {
            flat(i) = start + step * i;
          }
        }
        break;
      }

      default:
        ctx->SetStatus(errors::InvalidArgument("Invalid argument type ",
                                               DataTypeString(type)));
        return;
    }
    ctx->SetConstantOutput(0, out_constant);
  }
};

REGISTER_XLA_OP(Name("LinSpace")
                    .CompileTimeConstantInput("start")
                    .CompileTimeConstantInput("stop")
                    .CompileTimeConstantInput("num"),
                LinSpaceOp);

}  // namespace
}  // namespace tensorflow
