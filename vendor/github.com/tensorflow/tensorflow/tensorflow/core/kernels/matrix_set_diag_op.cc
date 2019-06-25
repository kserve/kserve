/* Copyright 2015 The TensorFlow Authors. All Rights Reserved.

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

#if GOOGLE_CUDA
#define EIGEN_USE_GPU
#endif  // GOOGLE_CUDA

#include "tensorflow/core/kernels/matrix_set_diag_op.h"

#include "third_party/eigen3/unsupported/Eigen/CXX11/Tensor"
#include "tensorflow/core/framework/op_kernel.h"
#include "tensorflow/core/framework/register_types.h"
#include "tensorflow/core/framework/tensor.h"
#include "tensorflow/core/framework/tensor_shape.h"
#include "tensorflow/core/framework/tensor_types.h"
#include "tensorflow/core/framework/types.h"
#include "tensorflow/core/lib/core/threadpool.h"
#include "tensorflow/core/platform/logging.h"
#include "tensorflow/core/platform/macros.h"

namespace tensorflow {

typedef Eigen::ThreadPoolDevice CPUDevice;
typedef Eigen::GpuDevice GPUDevice;

template <typename Device, typename T>
class MatrixSetDiagOp : public OpKernel {
 public:
  explicit MatrixSetDiagOp(OpKernelConstruction* context) : OpKernel(context) {}

  void Compute(OpKernelContext* context) override {
    const Tensor& input = context->input(0);
    const Tensor& diag = context->input(1);

    const TensorShape& input_shape = input.shape();
    const TensorShape& diag_shape = diag.shape();
    const int rank = input_shape.dims();

    // Preliminary validation of sizes.
    OP_REQUIRES(context, TensorShapeUtils::IsMatrixOrHigher(input_shape),
                errors::InvalidArgument(
                    "input must be at least 2-dim, received shape: ",
                    input.shape().DebugString()));

    // Check to make sure the last dimension of diag is equal to the smaller of
    // the last two dimensions of input.
    const int64 min_dim = std::min(input_shape.dim_size(rank - 1),
                                   input_shape.dim_size(rank - 2));
    TensorShape expected_diag_shape = input_shape;
    expected_diag_shape.RemoveLastDims(2);
    expected_diag_shape.AddDim(min_dim);
    OP_REQUIRES(context, expected_diag_shape == diag_shape,
                errors::InvalidArgument(
                    "must have diagonal.shape == input.shape[:-2] + "
                    "min(input.shape[-2:]), but received input shape: ",
                    input_shape.DebugString(),
                    " and diagonal shape: ", diag_shape.DebugString()));

    if (input.NumElements() == 0) {
      // This is a no-op.
      context->set_output(0, input);
      return;
    }

    auto input_reshaped = input.flat_inner_dims<T, 3>();
    auto diag_reshaped = diag.flat_inner_dims<T, 2>();
    Tensor* output = nullptr;
    OP_REQUIRES_OK(context, context->forward_input_or_allocate_output(
                                {0}, 0, input_shape, &output));
    auto output_reshaped = output->flat_inner_dims<T, 3>();
    functor::MatrixSetDiag<Device, T>::Compute(
        context, context->eigen_device<Device>(), input_reshaped, diag_reshaped,
        output_reshaped);
  }

 private:
  TF_DISALLOW_COPY_AND_ASSIGN(MatrixSetDiagOp);
};

#define REGISTER_MATRIX_SET_DIAG(type)                                    \
  REGISTER_KERNEL_BUILDER(                                                \
      Name("MatrixSetDiag").Device(DEVICE_CPU).TypeConstraint<type>("T"), \
      MatrixSetDiagOp<CPUDevice, type>);
TF_CALL_POD_TYPES(REGISTER_MATRIX_SET_DIAG);
#undef REGISTER_MATRIX_SET_DIAG

// Registration of the deprecated kernel.
// Delete after 10mar2017.
#define REGISTER_BATCH_MATRIX_SET_DIAG(type)                                   \
  REGISTER_KERNEL_BUILDER(                                                     \
      Name("BatchMatrixSetDiag").Device(DEVICE_CPU).TypeConstraint<type>("T"), \
      MatrixSetDiagOp<CPUDevice, type>);
TF_CALL_POD_TYPES(REGISTER_BATCH_MATRIX_SET_DIAG);
#undef REGISTER_BATCH_MATRIX_SET_DIAG

namespace functor {

// Implementation of the functor specialization for CPU.
template <typename T>
struct MatrixSetDiag<CPUDevice, T> {
  static void Compute(OpKernelContext* context, const CPUDevice& device,
                      typename TTypes<T, 3>::ConstTensor input,
                      typename TTypes<T, 2>::ConstTensor diag,
                      typename TTypes<T, 3>::Tensor output) {
    if (input.data() != output.data()) {
      output.device(device) = input;
    }
    auto compute_shard = [&output, &diag](int64 begin, int64 end) {
      for (int64 batch = begin; batch < end; ++batch) {
        for (int64 col = 0; col < diag.dimension(1); ++col) {
          output(batch, col, col) = diag(batch, col);
        }
      }
    };
    auto thread_pool =
        context->device()->tensorflow_cpu_worker_threads()->workers;
    int64 cost_per_batch = 10 * output.dimension(1);  // Heuristic.
    thread_pool->ParallelFor(output.dimension(0), cost_per_batch,
                             std::move(compute_shard));
  }
};

}  // namespace functor

#if GOOGLE_CUDA

// Forward declarations of the functor specializations for GPU.
namespace functor {
#define DECLARE_GPU_SPEC(T)                         \
  template <>                                       \
  void MatrixSetDiag<GPUDevice, T>::Compute(        \
      OpKernelContext* context, const GPUDevice& d, \
      typename TTypes<T, 3>::ConstTensor input,     \
      typename TTypes<T, 2>::ConstTensor diag,      \
      typename TTypes<T, 3>::Tensor output);        \
  extern template struct MatrixSetDiag<GPUDevice, T>;

TF_CALL_GPU_NUMBER_TYPES(DECLARE_GPU_SPEC);
TF_CALL_bool(DECLARE_GPU_SPEC);
TF_CALL_complex64(DECLARE_GPU_SPEC);
TF_CALL_complex128(DECLARE_GPU_SPEC);

}  // namespace functor

// Registration of the GPU implementations.
#define REGISTER_MATRIX_SET_DIAG_GPU(type)                                \
  REGISTER_KERNEL_BUILDER(                                                \
      Name("MatrixSetDiag").Device(DEVICE_GPU).TypeConstraint<type>("T"), \
      MatrixSetDiagOp<GPUDevice, type>);
TF_CALL_GPU_NUMBER_TYPES(REGISTER_MATRIX_SET_DIAG_GPU);
TF_CALL_bool(REGISTER_MATRIX_SET_DIAG_GPU);
TF_CALL_complex64(REGISTER_MATRIX_SET_DIAG_GPU);
TF_CALL_complex128(REGISTER_MATRIX_SET_DIAG_GPU);
#undef REGISTER_MATRIX_SET_DIAG_GPU

// Registration of the deprecated kernel.
// Delete after 10mar2017.
#define REGISTER_BATCH_MATRIX_SET_DIAG_GPU(type)                               \
  REGISTER_KERNEL_BUILDER(                                                     \
      Name("BatchMatrixSetDiag").Device(DEVICE_GPU).TypeConstraint<type>("T"), \
      MatrixSetDiagOp<GPUDevice, type>);
TF_CALL_GPU_NUMBER_TYPES(REGISTER_BATCH_MATRIX_SET_DIAG_GPU);
#undef REGISTER_BATCH_MATRIX_SET_DIAG_GPU

#endif  // GOOGLE_CUDA

}  // namespace tensorflow
