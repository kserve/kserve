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

// XLA-specific Shape Ops.

#include "tensorflow/compiler/tf2xla/kernels/shape_util.h"
#include "tensorflow/compiler/tf2xla/type_util.h"
#include "tensorflow/compiler/tf2xla/xla_helpers.h"
#include "tensorflow/compiler/tf2xla/xla_op_kernel.h"
#include "tensorflow/compiler/tf2xla/xla_op_registry.h"
#include "tensorflow/compiler/xla/client/xla_builder.h"
#include "tensorflow/core/framework/kernel_def_builder.h"
#include "tensorflow/core/framework/tensor_shape.h"
#include "tensorflow/core/kernels/bounds_check.h"

namespace tensorflow {
namespace {

class ShapeOp : public XlaOpKernel {
 public:
  explicit ShapeOp(OpKernelConstruction* ctx) : XlaOpKernel(ctx) {
    OP_REQUIRES_OK(ctx, ctx->GetAttr("out_type", &out_dtype_));
  }

  void Compile(XlaOpKernelContext* ctx) override {
    const TensorShape input_shape = ctx->InputShape(0);
    Tensor shape_constant(out_dtype_, TensorShape({input_shape.dims()}));
    OP_REQUIRES_OK(ctx, TensorShapeToConstant(input_shape, &shape_constant));
    ctx->SetConstantOutput(0, shape_constant);
  }

 private:
  DataType out_dtype_;
};

REGISTER_XLA_OP(Name("Shape").CompilationOnly().IsMetadataOp(), ShapeOp);

class ShapeNOp : public XlaOpKernel {
 public:
  explicit ShapeNOp(OpKernelConstruction* ctx) : XlaOpKernel(ctx) {
    OP_REQUIRES_OK(ctx, ctx->GetAttr("out_type", &out_dtype_));
  }

  void Compile(XlaOpKernelContext* ctx) override {
    for (int i = 0; i < ctx->num_inputs(); ++i) {
      const TensorShape input_shape = ctx->InputShape(i);
      Tensor shape_constant(out_dtype_, TensorShape({input_shape.dims()}));
      OP_REQUIRES_OK(ctx, TensorShapeToConstant(input_shape, &shape_constant));
      ctx->SetConstantOutput(i, shape_constant);
    }
  }

  bool IsExpensive() override { return false; }

 private:
  DataType out_dtype_;
};
REGISTER_XLA_OP(Name("ShapeN").CompilationOnly().IsMetadataOp(), ShapeNOp);

class RankOp : public XlaOpKernel {
 public:
  explicit RankOp(OpKernelConstruction* ctx) : XlaOpKernel(ctx) {}

  void Compile(XlaOpKernelContext* ctx) override {
    const TensorShape input_shape = ctx->InputShape(0);
    const int rank = input_shape.dims();
    Tensor rank_constant(DT_INT32, TensorShape({}));
    rank_constant.scalar<int32>()() = rank;

    ctx->SetConstantOutput(0, rank_constant);
  }
};

REGISTER_XLA_OP(Name("Rank").CompilationOnly().IsMetadataOp(), RankOp);

class SizeOp : public XlaOpKernel {
 public:
  explicit SizeOp(OpKernelConstruction* ctx) : XlaOpKernel(ctx) {}

  void Compile(XlaOpKernelContext* ctx) override {
    const TensorShape input_shape = ctx->InputShape(0);
    const int64 size = input_shape.num_elements();
    OP_REQUIRES(ctx, FastBoundsCheck(size, std::numeric_limits<int32>::max()),
                errors::InvalidArgument("Size does not work for tensors > "
                                        "int32 max."));
    Tensor size_constant(DT_INT32, TensorShape({}));
    size_constant.scalar<int32>()() = static_cast<int32>(size);

    ctx->SetConstantOutput(0, size_constant);
  }
};

REGISTER_XLA_OP(Name("Size").CompilationOnly().IsMetadataOp(), SizeOp);

class ExpandDimsOp : public XlaOpKernel {
 public:
  explicit ExpandDimsOp(OpKernelConstruction* ctx) : XlaOpKernel(ctx) {}

  void Compile(XlaOpKernelContext* ctx) override {
    const TensorShape input_shape = ctx->InputShape("input");
    const TensorShape dim_shape = ctx->InputShape("dim");

    std::vector<int64> dims;
    OP_REQUIRES_OK(ctx, ctx->ConstantInputReshapedToIntVector("dim", &dims));
    OP_REQUIRES(ctx, dims.size() == 1,
                errors::InvalidArgument(absl::StrCat(
                    "dim input to ExpandDims must be a scalar; got ",
                    dim_shape.DebugString())));
    int dim = dims[0];

    OP_REQUIRES(ctx,
                (dim >= -1 - input_shape.dims() && dim <= input_shape.dims()),
                errors::InvalidArgument("Tried to expand dim index ", dim,
                                        " for tensor with ", input_shape.dims(),
                                        " dimensions."));

    auto existing_dims = input_shape.dim_sizes();
    // Safe - # elements in tensor dims bounded.
    const int existing_dims_size = static_cast<int>(existing_dims.size());
    std::vector<int64> new_shape(existing_dims_size);
    for (size_t i = 0; i < new_shape.size(); ++i) {
      new_shape[i] = existing_dims[i];
    }

    // We emulate numpy's interpretation of the dim axis when
    // -input.dims() >= dim <= input.dims().
    if (dim < 0) {
      dim += existing_dims.size() + 1;
    }

    // Clamp to the end if needed.
    dim = std::min<int32>(dim, existing_dims_size);
    new_shape.emplace(new_shape.begin() + dim, 1);

    ctx->SetOutput(0, xla::Reshape(ctx->Input("input"), new_shape));
  }
};
REGISTER_XLA_OP(Name("ExpandDims").CompileTimeConstantInput("dim"),
                ExpandDimsOp);

class SqueezeOp : public XlaOpKernel {
 public:
  explicit SqueezeOp(OpKernelConstruction* ctx) : XlaOpKernel(ctx) {
    std::vector<int32> squeeze_dims;
    OP_REQUIRES_OK(ctx, ctx->GetAttr("squeeze_dims", &squeeze_dims));
    squeeze_dims_.insert(squeeze_dims.begin(), squeeze_dims.end());
  }

  void Compile(XlaOpKernelContext* ctx) override {
    const TensorShape input_shape = ctx->InputShape(0);
    auto existing_dims = input_shape.dim_sizes();
    int existing_dims_size = input_shape.dims();
    std::vector<int64> new_shape;

    std::unordered_set<int32> wrapped_squeeze_dims;
    wrapped_squeeze_dims.reserve(squeeze_dims_.size());
    // Validate squeeze dims against the input.
    for (int32 dim : squeeze_dims_) {
      OP_REQUIRES(ctx, (dim >= -input_shape.dims() && dim < input_shape.dims()),
                  errors::InvalidArgument("Tried to squeeze dim index ", dim,
                                          " for tensor with ",
                                          input_shape.dims(), " dimensions."));
      // If dim is < 0, we wrap around (-1 means the last element).
      if (dim < 0) {
        dim = existing_dims_size + dim;
      }

      wrapped_squeeze_dims.insert(dim);
    }

    for (int i = 0; i < existing_dims_size; ++i) {
      auto existing_dim = existing_dims[i];

      // If squeeze_set is non-empty, only squeeze those dimensions.
      if (!wrapped_squeeze_dims.empty()) {
        if (wrapped_squeeze_dims.count(i) > 0) {
          OP_REQUIRES(ctx, existing_dim == 1,
                      errors::InvalidArgument(
                          "Tried to explicitly squeeze dimension ", i,
                          " but dimension was not 1: ", existing_dim));
        } else {
          // This dimension is not being squeezed.
          new_shape.push_back(existing_dim);
        }
      } else {
        // Copy over all non-1-length dimensions.
        if (existing_dim != 1) {
          new_shape.push_back(existing_dim);
        }
      }
    }

    ctx->SetOutput(0, xla::Reshape(ctx->Input(0), new_shape));
  }

 private:
  std::unordered_set<int32> squeeze_dims_;
};

REGISTER_XLA_OP(Name("Squeeze"), SqueezeOp);

class ZerosLikeOp : public XlaOpKernel {
 public:
  explicit ZerosLikeOp(OpKernelConstruction* ctx) : XlaOpKernel(ctx) {}

  void Compile(XlaOpKernelContext* ctx) override {
    const TensorShape input_shape = ctx->InputShape(0);

    auto zero = XlaHelpers::Zero(ctx->builder(), input_type(0));
    ctx->SetOutput(0, xla::Broadcast(zero, input_shape.dim_sizes()));
  }
};

REGISTER_XLA_OP(Name("ZerosLike"), ZerosLikeOp);

class OnesLikeOp : public XlaOpKernel {
 public:
  explicit OnesLikeOp(OpKernelConstruction* ctx) : XlaOpKernel(ctx) {}

  void Compile(XlaOpKernelContext* ctx) override {
    const TensorShape input_shape = ctx->InputShape(0);

    auto one = XlaHelpers::One(ctx->builder(), input_type(0));
    ctx->SetOutput(0, xla::Broadcast(one, input_shape.dim_sizes()));
  }
};

REGISTER_XLA_OP(Name("OnesLike"), OnesLikeOp);

}  // namespace
}  // namespace tensorflow
