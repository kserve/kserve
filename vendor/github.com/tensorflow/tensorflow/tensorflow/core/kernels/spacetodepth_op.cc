/* Copyright 2016 The TensorFlow Authors. All Rights Reserved.

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

// See docs in ../ops/array_ops.cc.

#define EIGEN_USE_THREADS

#include <memory>
#include <string>
#include <utility>

#include "tensorflow/core/kernels/spacetodepth_op.h"

#include "third_party/eigen3/unsupported/Eigen/CXX11/Tensor"
#include "tensorflow/core/framework/op.h"
#include "tensorflow/core/framework/op_kernel.h"
#include "tensorflow/core/framework/register_types.h"
#include "tensorflow/core/framework/tensor.h"
#include "tensorflow/core/framework/tensor_shape.h"
#include "tensorflow/core/framework/tensor_types.h"
#include "tensorflow/core/framework/types.h"
#include "tensorflow/core/platform/logging.h"
#include "tensorflow/core/platform/types.h"
#include "tensorflow/core/util/tensor_format.h"

namespace tensorflow {

typedef Eigen::ThreadPoolDevice CPUDevice;
typedef Eigen::GpuDevice GPUDevice;

template <typename Device, typename T>
class SpaceToDepthOp : public OpKernel {
 public:
  explicit SpaceToDepthOp(OpKernelConstruction* context) : OpKernel(context) {
    string data_format_str;
    OP_REQUIRES_OK(context, context->GetAttr("data_format", &data_format_str));
    OP_REQUIRES(context, FormatFromString(data_format_str, &data_format_),
                errors::InvalidArgument("Invalid data format"));

    OP_REQUIRES_OK(context, context->GetAttr("block_size", &block_size_));
    OP_REQUIRES(context, block_size_ > 1,
                errors::InvalidArgument("Block size should be > 1, but was: ",
                                        block_size_));

    if (std::is_same<Device, CPUDevice>::value) {
      OP_REQUIRES(
          context, data_format_ == FORMAT_NHWC,
          errors::InvalidArgument(
              "Only NHWC data_format supported on CPU. Got ", data_format_str));
    }
  }

  void Compute(OpKernelContext* context) override {
    const Tensor& input = context->input(0);
    const int dims = input.dims();

    // Assuming qint8 <--> NCHW_VECT_C, OIHW_VECT_I (int8x4) here.
    constexpr bool is_int8x4 = std::is_same<T, qint8>::value;
    OP_REQUIRES(context, (is_int8x4 == (data_format_ == FORMAT_NCHW_VECT_C)),
                errors::InvalidArgument(
                    "qint8 should be used with data_format NCHW_VECT_C."));

    constexpr int kVect = is_int8x4 ? 4 : 1;
    constexpr int kDims = is_int8x4 ? 5 : 4;
    OP_REQUIRES(context, kDims == dims,
                errors::InvalidArgument("Input rank should be: ", kDims,
                                        " instead of: ", dims));

    constexpr int kNumSpatialDims = 2;
    const int batch_size =
        input.dim_size(GetTensorDimIndex<kNumSpatialDims>(data_format_, 'N'));
    const int height =
        input.dim_size(GetTensorDimIndex<kNumSpatialDims>(data_format_, 'H'));
    const int width =
        input.dim_size(GetTensorDimIndex<kNumSpatialDims>(data_format_, 'W'));
    const int input_depth =
        input.dim_size(GetTensorDimIndex<kNumSpatialDims>(data_format_, 'C')) *
        kVect;

    // Both width and height must be divisible by block_size.
    OP_REQUIRES(context,
                (width % block_size_) == 0 && (height % block_size_) == 0,
                errors::InvalidArgument(
                    "Image width ", width, " and height ", height,
                    " should be divisible by block_size: ", block_size_));

    // The 'spatial' block of size block_size_ X block_size_ will be moved
    // to depth.
    const int output_depth = input_depth * block_size_ * block_size_;
    const int output_width = width / block_size_;
    const int output_height = height / block_size_;

    // Allocate output tensor.
    Tensor* outputs_tensor = nullptr;
    OP_REQUIRES_OK(context,
                   context->allocate_output(
                       0,
                       ShapeFromFormat(data_format_, batch_size, output_height,
                                       output_width, output_depth),
                       &outputs_tensor));

    auto Tinput = input.tensor<T, kDims>();
    auto Toutput = outputs_tensor->tensor<T, kDims>();

    if (std::is_same<Device, GPUDevice>::value) {
      if (is_int8x4) {
        // NCHW_VECT_C with 4 x qint8 can be treated as NCHW int32.
        auto Tinput_v = input.template reinterpret_last_dimension<int32, 4>();
        auto Toutput_v = outputs_tensor->reinterpret_last_dimension<int32, 4>();
        functor::SpaceToDepthOpFunctor<GPUDevice, int32, FORMAT_NCHW> functor;
        functor(context->eigen_device<GPUDevice>(), Tinput_v, block_size_,
                Toutput_v);
        return;
      } else if (data_format_ == FORMAT_NCHW) {
        functor::SpaceToDepthOpFunctor<GPUDevice, T, FORMAT_NCHW> functor;
        functor(context->eigen_device<GPUDevice>(), Tinput, block_size_,
                Toutput);
        return;
      }
    }

    // NOTE: Assumes data_format_ == FORMAT_NHWC here, since we have rejected
    // (CPU && data_format_ != FORMAT_NHWC) in the constructor.

    if (!is_int8x4) {
      functor::SpaceToDepthOpFunctor<Device, T, FORMAT_NHWC> functor;
      functor(context->eigen_device<Device>(), Tinput, block_size_, Toutput);
    }
  };

 private:
  int block_size_;
  TensorFormat data_format_;
};

// Partial specialization of SpaceToDepthOpFunctor for a CPUDevice.
namespace functor {
template <typename T>
struct SpaceToDepthOpFunctor<CPUDevice, T, FORMAT_NHWC> {
  void operator()(const CPUDevice& d, typename TTypes<T, 4>::ConstTensor input,
                  int block_size, typename TTypes<T, 4>::Tensor output) {
    const int batch_size = output.dimension(0);
    const int input_height = input.dimension(1);
    const int input_width = input.dimension(2);
    const int input_depth = input.dimension(3);

    for (int b = 0; b < batch_size; ++b) {
      for (int h = 0; h < input_height; ++h) {
        const int out_h = h / block_size;
        const int offset_h = (h % block_size);
        for (int w = 0; w < input_width; ++w) {
          const int out_w = w / block_size;
          const int offset_w = (w % block_size);
          const int offset_d = (offset_h * block_size + offset_w) * input_depth;
          for (int d = 0; d < input_depth; ++d) {
            const int out_d = d + offset_d;
            output(b, out_h, out_w, out_d) = input(b, h, w, d);
          }
        }
      }
    }
  }
};
}  // namespace functor

#define REGISTER(type)                                                   \
  REGISTER_KERNEL_BUILDER(                                               \
      Name("SpaceToDepth").Device(DEVICE_CPU).TypeConstraint<type>("T"), \
      SpaceToDepthOp<CPUDevice, type>);

TF_CALL_ALL_TYPES(REGISTER);
#undef REGISTER

#if GOOGLE_CUDA
REGISTER_KERNEL_BUILDER(
    Name("SpaceToDepth").Device(DEVICE_GPU).TypeConstraint<float>("T"),
    SpaceToDepthOp<GPUDevice, float>);
REGISTER_KERNEL_BUILDER(
    Name("SpaceToDepth").Device(DEVICE_GPU).TypeConstraint<Eigen::half>("T"),
    SpaceToDepthOp<GPUDevice, Eigen::half>);
REGISTER_KERNEL_BUILDER(
    Name("SpaceToDepth").Device(DEVICE_GPU).TypeConstraint<qint8>("T"),
    SpaceToDepthOp<GPUDevice, qint8>);
REGISTER_KERNEL_BUILDER(
    Name("SpaceToDepth").Device(DEVICE_GPU).TypeConstraint<uint8>("T"),
    SpaceToDepthOp<GPUDevice, uint8>);
#endif  // GOOGLE_CUDA

}  // end namespace tensorflow
