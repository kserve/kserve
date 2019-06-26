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

#include "tensorflow/compiler/tf2xla/type_util.h"
#include "tensorflow/compiler/tf2xla/xla_helpers.h"
#include "tensorflow/compiler/tf2xla/xla_op_kernel.h"
#include "tensorflow/compiler/tf2xla/xla_op_registry.h"
#include "tensorflow/compiler/xla/array4d.h"
#include "tensorflow/compiler/xla/client/lib/constants.h"
#include "tensorflow/compiler/xla/client/xla_builder.h"
#include "tensorflow/core/framework/kernel_def_builder.h"
#include "tensorflow/core/framework/register_types.h"
#include "tensorflow/core/lib/math/math_util.h"

namespace tensorflow {
namespace {

// We implement bilinear interpolation by upsampling followed by convolution.
// The basic idea is as follows. To scale from NxN to RxR:
//
//    1. S := (N - 1) /  gcd(N-1, R-1)
//    2. k := (R - 1) /  gcd(N-1, R-1)
//    3. Convolution((2k-1)x(2k-1), stride=S, lhs_dilation=k, padding=k-1)
//
// For example, to Scale from 7x7 -> 15x15:
//
//    1. S := (7-1) / gcd(7-1, 15-1) = 6 / gcd(6, 14) = 6 / 2 = 3
//    2. k := (15 - 1) / gcd(7-1, 15-1) = 14 / gcd(6, 14) = 14 / 2 = 7
//    3. Convolution(15x15, stride=3, lhs_dilation=7, padding=2)
//
//
// The 7x7 -> 15x15 case is much too large to write out in full as an
// example. The smallest interesting example is 3x3 -> 4x4.
//
// S := 2
// k := 3
//
// 00 03 06    00 00 00 00 00 00 00 00 00 00 00      00 02 04 06
// 09 12 15 -> 00 00 00 00 00 00 00 00 00 00 00   -> 06 08 10 12
// 18 21 24    00 00 00 00 00 03 00 00 06 00 00      12 14 16 18
//             00 00 00 00 00 00 00 00 00 00 00      18 20 22 24
//             00 00 00 00 00 00 00 00 00 00 00
//             00 00 09 00 00 12 00 00 15 00 00
//             00 00 00 00 00 00 00 00 00 00 00
//             00 00 00 00 00 00 00 00 00 00 00
//             00 00 18 00 00 21 00 00 24 00 00
//             00 00 00 00 00 00 00 00 00 00 00
//             00 00 00 00 00 00 00 00 00 00 00
//
// with the following convolutional kernel, with stride [2, 2]:
//       1 2 3 2 1
//       2 4 6 4 2
// 1/9 * 3 6 9 6 3
//       2 4 6 4 2
//       1 2 3 2 1
// Note that the convolution kernel matrix is separable and thus we can instead
// use 2 consecutive 1D kernel of the dimension 2k-1, along each axis.

// Computes the size of the convolutional kernel and stride to use when resizing
// from in_size to out_size.
struct ResizeConvolutionDims {
  // Size of the kernel to use.
  std::vector<int64> kernel_size;

  // Stride of the convolution to use.
  std::vector<int64> stride;
};
ResizeConvolutionDims ComputeResizeConvolutionParameters(
    absl::Span<const int64> in_size, absl::Span<const int64> out_size,
    bool align_corners) {
  CHECK_EQ(in_size.size(), out_size.size());
  int num_spatial_dims = in_size.size();
  ResizeConvolutionDims dims;
  dims.kernel_size.resize(num_spatial_dims);
  dims.stride.resize(num_spatial_dims);
  for (int i = 0; i < num_spatial_dims; ++i) {
    if (in_size[i] == 1) {
      // We must handle input size 1 specially because XLA convolution does
      // not allow stride 0.
      dims.stride[i] = dims.kernel_size[i] = 1;
    } else if (out_size[i] == 1) {
      // If in_size[i] > 1 but out_size[i] == 1, then we slice out the first
      // entry before resizing.
      dims.stride[i] = dims.kernel_size[i] = 1;
    } else {
      // The scaling factor changes depending on the alignment of corners.
      const int64 in_size_factor = align_corners ? in_size[i] - 1 : in_size[i];
      const int64 out_size_factor =
          align_corners ? out_size[i] - 1 : out_size[i];

      int64 gcd = MathUtil::GCD(static_cast<uint64>(in_size_factor),
                                static_cast<uint64>(out_size_factor));
      dims.stride[i] = in_size_factor / gcd;
      dims.kernel_size[i] = out_size_factor / gcd;
    }
  }
  return dims;
}

// The upper padding of the input needed by ConvGeneralDilated calls is
// determined by solving two related relationships (assuming rhs_dilation == 0):
// 1. dilated_input_dim = lower_padding + upper_padding
//                        + lhs_dilation * (in_size - 1) + 1
// 2. dilated_input_dim = (2 * dims.kernel-size - 1)
//                        + dims.stride * (out_size - 1)
int64 CalculateUpperPadding(int64 in_size, int64 out_size, int64 kernel_size,
                            int64 stride) {
  return (2 * kernel_size - 1) + (out_size - 1) * stride - (kernel_size - 1) -
         1 - (kernel_size * (in_size - 1));
}

// Form a 2D convolution kernel like:
//       1 2 3 2 1
//       2 4 6 4 2
// 1/9 * 3 6 9 6 3
//       2 4 6 4 2
//       1 2 3 2 1
// by multiplying two 1D kernels of the form:
// 1/3 * [1 2 3 2 1]
// If the 2D kernel would be very large, the 1D kernel can be applied once in
// each dimension due to the symmetry of the kernel along all axis to reduce the
// computational intensity.
xla::XlaOp Make1DKernel(xla::XlaBuilder* builder, int64 n) {
  std::vector<float> kernel(n * 2 - 1);
  for (int64 i = 0; i < n; ++i) {
    float v = (i + 1.0f) / n;
    kernel[i] = v;
    kernel[n * 2 - 2 - i] = v;
  }
  return xla::ConstantR1<float>(builder, kernel);
}

// Kernels with more than 16 spatial elements are considered intense and the
// kernel should applied to each dimension independently.
const int64 kMax2DKernelSize = 16;

xla::XlaOp MakeBilinearResizeKernel(xla::XlaBuilder* builder,
                                    absl::Span<const int64> kernel_size,
                                    int64 channels) {
  auto depthwise_kernel = xla::Broadcast(
      xla::Zero(builder, xla::F32),
      {(2 * kernel_size[0] - 1), (2 * kernel_size[1] - 1), channels, 1});

  return xla::Mul(
      xla::Add(depthwise_kernel, Make1DKernel(builder, kernel_size[1]),
               /*broadcast_dimensions=*/{1}),
      Make1DKernel(builder, kernel_size[0]),
      /*broadcast_dimensions=*/{0});
}

xla::XlaOp MakeBilinearResizeKernelInDim(xla::XlaBuilder* builder,
                                         absl::Span<const int64> kernel_size,
                                         int64 channels, int64 dim) {
  auto depthwise_kernel =
      xla::Broadcast(xla::Zero(builder, xla::F32),
                     {dim == 0 ? (2 * kernel_size[0] - 1) : 1,
                      dim == 1 ? (2 * kernel_size[1] - 1) : 1, channels, 1});
  return xla::Add(depthwise_kernel, Make1DKernel(builder, kernel_size[dim]),
                  /*broadcast_dimensions=*/{dim});
}

xla::XlaOp ResizeUsingDilationAndConvolution(xla::XlaBuilder* builder,
                                             const xla::XlaOp& input,
                                             const int num_spatial_dims,
                                             std::vector<int64> in_size,
                                             std::vector<int64> out_size,
                                             const int64 channels,
                                             const bool align_corners) {
  // Picture for a 1x3 to 1x4 resize:
  // stride = 2, kernel size = 3
  // Input:
  // 3 6 9
  // Input with dilation and padding:
  // 0 0 3 0 0 6 0 0 9 0 0
  // Convolution kernel:
  // 1/3 * [1 2 3 2 1]
  // Output:
  // 3 5 7 9
  xla::ConvolutionDimensionNumbers dimension_numbers;
  dimension_numbers.set_input_batch_dimension(0);
  dimension_numbers.set_output_batch_dimension(0);
  dimension_numbers.set_input_feature_dimension(num_spatial_dims + 1);
  dimension_numbers.set_output_feature_dimension(num_spatial_dims + 1);
  for (int i = 0; i < num_spatial_dims; ++i) {
    dimension_numbers.add_input_spatial_dimensions(1 + i);
    dimension_numbers.add_output_spatial_dimensions(1 + i);
    dimension_numbers.add_kernel_spatial_dimensions(i);
  }
  dimension_numbers.set_kernel_input_feature_dimension(num_spatial_dims + 1);
  dimension_numbers.set_kernel_output_feature_dimension(num_spatial_dims);

  ResizeConvolutionDims dims =
      ComputeResizeConvolutionParameters(in_size, out_size, align_corners);
  xla::XlaOp output;

  // Concatenation and padding below currently assumes num_spatial_dims is 2 to
  // prevent needless code complexity.
  CHECK_EQ(num_spatial_dims, 2)
      << "ResizeUsingDilationAndConvolution pads only 2 dimensions currently.";
  std::vector<int64> upper_padding(num_spatial_dims);
  for (int i = 0; i < num_spatial_dims; ++i) {
    upper_padding[i] = dims.kernel_size[i] - 1;
  }
  xla::XlaOp input_data = input;

  if (!align_corners) {
    // When Tensorflow does not align_corners, the resize indexing can access
    // beyond the upper bound and is instead clamped to prevent out of bounds
    // reads. This is conceptually the same as extending the edges of the input.
    // We emulate this by copying the last row/column of the input.
    // Calculate what padding would be needed then determine how far to extend
    // the border before lhs dilation.
    std::vector<int64> num_extended(num_spatial_dims);
    upper_padding[0] = CalculateUpperPadding(
        in_size[0], out_size[0], dims.kernel_size[0], dims.stride[0]);
    upper_padding[1] = CalculateUpperPadding(
        in_size[1], out_size[1], dims.kernel_size[1], dims.stride[1]);
    num_extended[0] = upper_padding[0] / (dims.kernel_size[0]);
    num_extended[1] = upper_padding[1] / (dims.kernel_size[1]);

    const int64 batch_dim_size =
        builder->GetShape(input).ValueOrDie().dimensions(0);
    if (num_extended[0] > 0) {
      auto slice = xla::Slice(
          input_data, {0, in_size[0] - 1, 0, 0},
          {batch_dim_size, in_size[0], in_size[1], channels}, {1, 1, 1, 1});
      for (int i = 0; i < num_extended[0]; i++) {
        input_data = xla::ConcatInDim(builder, {input_data, slice}, 1);
      }
    }

    if (num_extended[1] > 0) {
      auto slice = xla::Slice(
          input_data, {0, 0, in_size[1] - 1, 0},
          {batch_dim_size, in_size[0] + num_extended[0], in_size[1], channels},
          {1, 1, 1, 1});
      for (int i = 0; i < num_extended[1]; i++) {
        input_data = xla::ConcatInDim(builder, {input_data, slice}, 2);
      }
    }

    // Setting in_size to (in_size + num_extended) due to the above Slice and
    // ConcatInDim. Recalculate needed padding after the above Slice/Concat.
    upper_padding[0] =
        CalculateUpperPadding(in_size[0] + num_extended[0], out_size[0],
                              dims.kernel_size[0], dims.stride[0]);
    upper_padding[1] =
        CalculateUpperPadding(in_size[1] + num_extended[1], out_size[1],
                              dims.kernel_size[1], dims.stride[1]);
  }

  // Split convolutions into independent dimensions if they would be a very
  // large kernel.
  if (dims.kernel_size[0] * dims.kernel_size[1] < kMax2DKernelSize) {
    xla::XlaOp kernel =
        MakeBilinearResizeKernel(builder, dims.kernel_size, channels);
    output =
        xla::ConvGeneralDilated(input_data, kernel, dims.stride,
                                /*padding=*/
                                {{dims.kernel_size[0] - 1, upper_padding[0]},
                                 {dims.kernel_size[1] - 1, upper_padding[1]}},
                                /*lhs_dilation=*/dims.kernel_size,
                                /*rhs_dilation=*/{1, 1}, dimension_numbers,
                                /*feature_group_count=*/channels);
  } else {
    xla::XlaOp kernel0 =
        MakeBilinearResizeKernelInDim(builder, dims.kernel_size, channels, 0);
    output = xla::ConvGeneralDilated(
        input_data, kernel0, {dims.stride[0], 1},
        /*padding=*/
        {{dims.kernel_size[0] - 1, upper_padding[0]}, {0, 0}},
        /*lhs_dilation=*/{dims.kernel_size[0], 1},
        /*rhs_dilation=*/{1, 1}, dimension_numbers,
        /*feature_group_count=*/channels);
    xla::XlaOp kernel1 =
        MakeBilinearResizeKernelInDim(builder, dims.kernel_size, channels, 1);
    output = xla::ConvGeneralDilated(
        output, kernel1, {1, dims.stride[1]},
        /*padding=*/
        {{0, 0}, {dims.kernel_size[1] - 1, upper_padding[1]}},
        /*lhs_dilation=*/{1, dims.kernel_size[1]},
        /*rhs_dilation=*/{1, 1}, dimension_numbers,
        /*feature_group_count=*/channels);
  }

  // Add broadcasts to handle expanding from a size == 1 dimension to a
  // size > 1 dimension.
  for (int i = 0; i < num_spatial_dims; ++i) {
    if (in_size[i] == 1 && out_size[i] > 1) {
      output = xla::Add(output, xla::ConstantR1<float>(builder, out_size[i], 0),
                        /*broadcast_dimensions=*/{1 + i});
    }
  }
  return output;
}

xla::XlaOp ResizeUsingDilationAndConvolutionGradOp(xla::XlaBuilder* builder,
                                                   const xla::XlaOp& grad,
                                                   const int num_spatial_dims,
                                                   std::vector<int64> in_size,
                                                   std::vector<int64> grad_size,
                                                   const int64 channels,
                                                   const bool align_corners) {
  ResizeConvolutionDims dims =
      ComputeResizeConvolutionParameters(in_size, grad_size, align_corners);

  // To form the backward convolution, we keep the kernel unchanged (it is
  // already symmetric) and swap the roles of strides and LHS dilation.
  xla::ConvolutionDimensionNumbers dimension_numbers;
  dimension_numbers.set_input_batch_dimension(0);
  dimension_numbers.set_output_batch_dimension(0);
  dimension_numbers.set_input_feature_dimension(num_spatial_dims + 1);
  dimension_numbers.set_output_feature_dimension(num_spatial_dims + 1);
  for (int i = 0; i < num_spatial_dims; ++i) {
    dimension_numbers.add_input_spatial_dimensions(i + 1);
    dimension_numbers.add_output_spatial_dimensions(i + 1);
    dimension_numbers.add_kernel_spatial_dimensions(i);
  }
  dimension_numbers.set_kernel_input_feature_dimension(num_spatial_dims + 1);
  dimension_numbers.set_kernel_output_feature_dimension(num_spatial_dims);
  xla::XlaOp output;
  if (dims.kernel_size[0] * dims.kernel_size[1] < kMax2DKernelSize) {
    xla::XlaOp kernel =
        MakeBilinearResizeKernel(builder, dims.kernel_size, channels);

    // Broadcast the input kernel where the forward op expanded from a size == 1
    // dimension to a size > 1 dimension. This has the effect of summing the
    // gradient contributions in that dimension.
    for (int i = 0; i < num_spatial_dims; ++i) {
      if (in_size[i] == 1 && grad_size[i] > 1) {
        kernel =
            xla::Add(kernel, xla::ConstantR1<float>(builder, grad_size[i], 0),
                     /*broadcast_dimensions=*/{i});
      }
    }

    output = xla::ConvGeneralDilated(
        grad, kernel, /*window_strides=*/dims.kernel_size,
        /*padding=*/
        {{dims.kernel_size[0] - 1, dims.kernel_size[0] - 1},
         {dims.kernel_size[1] - 1, dims.kernel_size[1] - 1}},
        /*lhs_dilation=*/dims.stride,
        /*rhs_dilation=*/{1, 1}, dimension_numbers,
        /*feature_group_count=*/channels);
  } else {
    xla::XlaOp kernel0 =
        MakeBilinearResizeKernelInDim(builder, dims.kernel_size, channels, 0);
    xla::XlaOp kernel1 =
        MakeBilinearResizeKernelInDim(builder, dims.kernel_size, channels, 1);

    // Broadcast the input kernel where the forward op expanded from a size == 1
    // dimension to a size > 1 dimension. This has the effect of summing the
    // gradient contributions in that dimension.
    if (in_size[0] == 1 && grad_size[0] > 1) {
      kernel0 =
          xla::Add(kernel0, xla::ConstantR1<float>(builder, grad_size[0], 0),
                   /*broadcast_dimensions=*/{0});
    }
    if (in_size[1] == 1 && grad_size[1] > 1) {
      kernel1 =
          xla::Add(kernel0, xla::ConstantR1<float>(builder, grad_size[1], 0),
                   /*broadcast_dimensions=*/{1});
    }

    output = xla::ConvGeneralDilated(
        grad, kernel0, /*window_strides=*/{dims.kernel_size[0], 1},
        /*padding=*/
        {{dims.kernel_size[0] - 1, dims.kernel_size[0] - 1}, {0, 0}},
        /*lhs_dilation=*/{dims.stride[0], 1},
        /*rhs_dilation=*/{1, 1}, dimension_numbers,
        /*feature_group_count=*/channels);

    output = xla::ConvGeneralDilated(
        output, kernel1, /*window_strides=*/{1, dims.kernel_size[1]},
        /*padding=*/
        {{0, 0}, {dims.kernel_size[1] - 1, dims.kernel_size[1] - 1}},
        /*lhs_dilation=*/{1, dims.stride[1]},
        /*rhs_dilation=*/{1, 1}, dimension_numbers,
        /*feature_group_count=*/channels);
  }

  // If in_size[i] > 1 and grad_size[i] == 1, pad the output in dimension i.
  // Opposite of the slice performed by the forward op.
  xla::PaddingConfig padding = xla::MakeNoPaddingConfig(4);
  bool pad_output = false;
  for (int i = 0; i < num_spatial_dims; ++i) {
    if (in_size[i] > 1 && grad_size[i] == 1) {
      pad_output = true;
      padding.mutable_dimensions(1 + i)->set_edge_padding_high(in_size[i] - 1);
    }
  }
  if (pad_output) {
    output = xla::Pad(output, xla::ConstantR0<float>(builder, 0.0f), padding);
  }
  return output;
}

class ResizeBilinearOp : public XlaOpKernel {
 public:
  explicit ResizeBilinearOp(OpKernelConstruction* ctx) : XlaOpKernel(ctx) {
    OP_REQUIRES_OK(ctx, ctx->GetAttr("align_corners", &align_corners_));
  }

  void Compile(XlaOpKernelContext* ctx) override {
    xla::XlaBuilder* b = ctx->builder();

    TensorShape input_shape = ctx->InputShape(0);
    OP_REQUIRES(ctx, input_shape.dims() == 4,
                errors::InvalidArgument("input must be 4-dimensional",
                                        input_shape.DebugString()));
    const int64 batch = input_shape.dim_size(0);
    std::vector<int64> in_size = {input_shape.dim_size(1),
                                  input_shape.dim_size(2)};
    const int64 channels = input_shape.dim_size(3);
    OP_REQUIRES(ctx, in_size[0] > 0 && in_size[1] > 0,
                errors::InvalidArgument("input size must be positive, got [",
                                        in_size[0], ",", in_size[1], "]"));

    std::vector<int64> out_size;
    OP_REQUIRES_OK(ctx, ctx->ConstantInputAsIntVector(1, &out_size));
    OP_REQUIRES(ctx, out_size.size() == 2,
                errors::InvalidArgument("output size must be length 2, got ",
                                        out_size.size()));
    OP_REQUIRES(ctx, out_size[0] > 0 && out_size[1] > 0,
                errors::InvalidArgument("output size must be positive, got [",
                                        out_size[0], ",", out_size[1], "]"));

    const int num_spatial_dims = 2;

    xla::XlaOp input = ctx->Input(0);

    // If in_size[i] > 1 and out_size[i] == 1, slice out the first input in
    // dimension i.
    bool slice_input = false;
    for (int i = 0; i < num_spatial_dims; ++i) {
      if (in_size[i] > 1 && out_size[i] == 1) {
        // If in_size[i] > 1 but out_size[i] == 1, then we slice out the first
        // entry before resizing.
        slice_input = true;
        in_size[i] = 1;
      }
    }
    if (slice_input) {
      input =
          xla::Slice(input, {0, 0, 0, 0},
                     {batch, in_size[0], in_size[1], channels}, {1, 1, 1, 1});
    }

    // Output is always type float.
    input = xla::ConvertElementType(input, xla::F32);

    // Special Case:
    // Instead of doing a ResizeUsingDilationAndConvolution directly,
    // while (out_size[0]-1) = c * 2^x * (in_size[0]-1) for x>1 c>1, resize the
    // image to 2*(in_size[0]-1)+1 x-times and then resize by scale c(int here).
    // Instead of resizing directly we resize it iteratively.
    //
    // Since bilinear resize can be broken down as 2 sequential linear
    // operations along different dimensions.
    // Given sufficient numerical stability and a<e<c and b<f<d, bilinear resize
    // from image of size axb -> cxd is same as resizing axb -> exf -> cxd.
    // This does not work in the case of align_corners_=false because of special
    // padding requirements that cause multiple resizes to be very different
    // from a single resize.
    //
    // This makes the convolutions kernels smaller and the operation faster.
    xla::XlaOp output = input;
    while (in_size != out_size) {
      if (in_size[0] != 1 && in_size[1] != 1) {
        std::vector<float> k = {
            (static_cast<float>(out_size[0]) - 1) / ((in_size[0] - 1) * 2),
            (static_cast<float>(out_size[1]) - 1) / ((in_size[1] - 1) * 2)};
        if ((k[0] == std::floor(k[0])) && (k[1] == std::floor(k[1])) &&
            k[0] > 1 && k[1] > 1 && align_corners_) {
          std::vector<int64> next_out_size = {(in_size[0] - 1) * 2 + 1,
                                              (in_size[1] - 1) * 2 + 1};
          output = ResizeUsingDilationAndConvolution(b, input, num_spatial_dims,
                                                     in_size, next_out_size,
                                                     channels, align_corners_);
          input = output;
          in_size = next_out_size;
        } else {
          output = ResizeUsingDilationAndConvolution(b, input, num_spatial_dims,
                                                     in_size, out_size,
                                                     channels, align_corners_);
          in_size = out_size;
        }
      } else {
        output = ResizeUsingDilationAndConvolution(b, input, num_spatial_dims,
                                                   in_size, out_size, channels,
                                                   align_corners_);
        in_size = out_size;
      }
    }

    ctx->SetOutput(0, output);
  }

 private:
  bool align_corners_;
};

REGISTER_XLA_OP(Name("ResizeBilinear").CompileTimeConstantInput("size"),
                ResizeBilinearOp);

class ResizeBilinearGradOp : public XlaOpKernel {
 public:
  explicit ResizeBilinearGradOp(OpKernelConstruction* ctx) : XlaOpKernel(ctx) {
    OP_REQUIRES_OK(ctx, ctx->GetAttr("align_corners", &align_corners_));
    OP_REQUIRES(
        ctx, align_corners_ == true,
        errors::Unimplemented("ResizeBilinearGrad with align_corners=False is "
                              "not yet implemented"));

    DataType output_dtype;
    OP_REQUIRES_OK(ctx, ctx->GetAttr("T", &output_dtype));
    OP_REQUIRES_OK(ctx, DataTypeToPrimitiveType(output_dtype, &output_type_));
  }

  void Compile(XlaOpKernelContext* ctx) override {
    xla::XlaBuilder* b = ctx->builder();

    TensorShape input_shape = ctx->InputShape(1);
    OP_REQUIRES(ctx, input_shape.dims() == 4,
                errors::InvalidArgument("input must be 4-dimensional",
                                        input_shape.DebugString()));
    const int64 batch = input_shape.dim_size(0);
    std::vector<int64> in_size = {input_shape.dim_size(1),
                                  input_shape.dim_size(2)};
    const int64 channels = input_shape.dim_size(3);
    OP_REQUIRES(ctx, in_size[0] > 0 && in_size[1] > 0,
                errors::InvalidArgument("input size must be positive, got [",
                                        in_size[0], ",", in_size[1], "]"));

    TensorShape grad_shape = ctx->InputShape(0);
    OP_REQUIRES(ctx, grad_shape.dims() == 4,
                errors::InvalidArgument("gradient must be 4-dimensional",
                                        grad_shape.DebugString()));
    const int64 grad_batch = grad_shape.dim_size(0);
    const std::vector<int64> grad_size = {grad_shape.dim_size(1),
                                          grad_shape.dim_size(2)};
    const int64 grad_channels = grad_shape.dim_size(3);
    OP_REQUIRES(ctx, batch == grad_batch,
                errors::InvalidArgument(
                    "activations and gradients must have the same batch size (",
                    batch, " vs. ", grad_batch, ")"));
    OP_REQUIRES(ctx, grad_size[0] > 0 && grad_size[1] > 0,
                errors::InvalidArgument("gradient size must be positive, got [",
                                        grad_size[0], ",", grad_size[1], "]"));
    OP_REQUIRES(
        ctx, channels == grad_channels,
        errors::InvalidArgument(
            "activations and gradients must have the same number of channels (",
            channels, " vs. ", grad_channels, ")"));

    const int num_spatial_dims = 2;

    xla::XlaOp grad = ctx->Input(0);

    xla::XlaOp output = grad;
    while (in_size != grad_size) {
      if (in_size[0] != 1 && in_size[1] != 1) {
        std::vector<float> k = {
            (static_cast<float>(grad_size[0]) - 1) / ((in_size[0] - 1) * 2),
            (static_cast<float>(grad_size[1]) - 1) / ((in_size[1] - 1) * 2)};
        if ((k[0] == std::floor(k[0])) && (k[1] == std::floor(k[1])) &&
            k[0] > 1 && k[1] > 1) {
          std::vector<int64> next_grad_size = {(in_size[0] - 1) * 2 + 1,
                                               (in_size[1] - 1) * 2 + 1};
          output = ResizeUsingDilationAndConvolutionGradOp(
              b, grad, num_spatial_dims, in_size, next_grad_size, channels,
              align_corners_);
          grad = output;
          in_size = next_grad_size;
        } else {
          output = ResizeUsingDilationAndConvolutionGradOp(
              b, grad, num_spatial_dims, in_size, grad_size, channels,
              align_corners_);
          in_size = grad_size;
        }
      } else {
        output = ResizeUsingDilationAndConvolutionGradOp(
            b, grad, num_spatial_dims, in_size, grad_size, channels,
            align_corners_);
        in_size = grad_size;
      }
    }

    output = xla::ConvertElementType(output, output_type_);
    ctx->SetOutput(0, output);
  }

 private:
  bool align_corners_;
  xla::PrimitiveType output_type_;
};

REGISTER_XLA_OP(Name("ResizeBilinearGrad"), ResizeBilinearGradOp);

}  // namespace
}  // namespace tensorflow
