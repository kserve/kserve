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

#if GOOGLE_CUDA

#define EIGEN_USE_GPU

#include "third_party/eigen3/unsupported/Eigen/CXX11/Tensor"
#include "tensorflow/core/kernels/ops_util.h"
#include "tensorflow/core/kernels/transpose_functor.h"
#include "tensorflow/core/util/cuda_kernel_helper.h"

// TODO(yangzihao): Remove the dependency of conv_2d.h once we move all
// GPU util functions and transpose kernels into separate files.
#include "tensorflow/core/kernels/conv_2d.h"

typedef Eigen::GpuDevice GPUDevice;

namespace tensorflow {
namespace internal {

template <typename T, bool conjugate>
__global__ void TransposeKernel(int nthreads, const T* src, const int32* buf,
                                const int32 ndims, T* dst) {
  const int32* in_strides = buf;
  const int32* out_strides = buf + ndims;
  const int32* perm = buf + ndims * 2;
  CUDA_1D_KERNEL_LOOP(o_idx, nthreads) {
    int32 i_idx = 0;
    int32 t = o_idx;
    for (int32 i = 0; i < ndims; ++i) {
      const int32 ratio = t / out_strides[i];
      t -= ratio * out_strides[i];
      i_idx += ratio * in_strides[perm[i]];
    }
    if (conjugate) {
      dst[o_idx] = Eigen::numext::conj(ldg(src + i_idx));
    } else {
      dst[o_idx] = ldg(src + i_idx);
    }
  }
}

template <typename T, bool conjugate>
void TransposeSimple(const GPUDevice& d, const Tensor& in,
                     const gtl::ArraySlice<int32> perm, Tensor* out) {
  // Ensures we can use 32-bit index.
  const int64 nelem = in.NumElements();
  CHECK_LT(nelem, kint32max) << "Tensor too large to transpose on GPU";
  // Pack strides and permutation into one buffer.
  const int32 ndims = in.dims();
  gtl::InlinedVector<int32, 24> host_buf(ndims * 3);
  gtl::InlinedVector<int32, 8> in_strides = ComputeStride<int32>(in.shape());
  gtl::InlinedVector<int32, 8> out_strides = ComputeStride<int32>(out->shape());
  // Dimension permutation.
  for (int i = 0; i < ndims; ++i) {
    host_buf[i] = in_strides[i];
    host_buf[ndims + i] = out_strides[i];
    host_buf[ndims * 2 + i] = perm[i];
  }
  // Copies the input strides, output strides and permutation to the device.
  auto num_bytes = sizeof(int64) * host_buf.size();
  auto dev_buf = d.allocate(num_bytes);
  // NOTE: host_buf is not allocated by CudaHostAllocator, and
  // therefore we are doing a sync copy effectively.
  d.memcpyHostToDevice(dev_buf, host_buf.data(), num_bytes);
  // Launch kernel to q[...] = p[...].
  const T* p = reinterpret_cast<const T*>(in.tensor_data().data());
  T* q = reinterpret_cast<T*>(const_cast<char*>((out->tensor_data().data())));
  CudaLaunchConfig cfg = GetCudaLaunchConfig(nelem, d);
  TransposeKernel<T, conjugate>
      <<<cfg.block_count, cfg.thread_per_block, 0, d.stream()>>>(
          cfg.virtual_thread_count, p, reinterpret_cast<const int32*>(dev_buf),
          ndims, q);
  // Safe to deallocate immediately after the kernel launch.
  d.deallocate(dev_buf);
}

// TransposeUsingTile tries to reduce the dimension of the input tensor to 3 and
// then call special kernels to swap either dimension 1 and dimension 2 or
// dimension 0 and dimension 2. It returns true if the operation is success,
// false otherwise.
template <typename T, bool conjugate = false>
struct TransposeUsingTile {
  static bool run(const Eigen::GpuDevice& d, const Tensor& in,
                  const gtl::ArraySlice<int32> perm, Tensor* out) {
    // First try to reduce the dimensions of the input tensor.
    TransposePermsVec new_perm;
    TransposeDimsVec new_dims;
    ReduceTransposeDimensions(in.shape(), perm, &new_perm, &new_dims);

    // Only use special GPU kernel when dimension is 2 or 3.
    int dims = new_dims.size();
    if (dims < 2 || dims > 3) return false;
    auto in_data = reinterpret_cast<const T*>(in.tensor_data().data());
    auto out_data =
        reinterpret_cast<T*>(const_cast<char*>(out->tensor_data().data()));
    switch (dims) {
      case 2:
        if (new_perm[0] == 1 && new_perm[1] == 0) {
          // Add the first dimension size as 1.
          new_dims.insert(new_dims.begin(), 1);
          tensorflow::functor::SwapDimension1And2InTensor3<GPUDevice, T,
                                                           conjugate>()(
              d, in_data, new_dims, out_data);
          return true;
        }
        break;
      case 3:
        if (new_perm == TransposePermsVec({0, 2, 1})) {
          tensorflow::functor::SwapDimension1And2InTensor3<GPUDevice, T,
                                                           conjugate>()(
              d, in_data, new_dims, out_data);
          return true;
        } else if (new_perm == TransposePermsVec({2, 1, 0})) {
          tensorflow::functor::SwapDimension0And2InTensor3<GPUDevice, T,
                                                           conjugate>()(
              d, in_data, new_dims, out_data);
          return true;
        } else {
          // do not handle other 3D permutations
          return false;
        }
        break;
      default:
        return false;
    }
    return false;
  }
};

template <bool conjugate>
struct TransposeUsingTile<complex64, conjugate> {
  static bool run(const Eigen::GpuDevice& d, const Tensor& in,
                  const gtl::ArraySlice<int32> perm, Tensor* out) {
    if (!conjugate) {
      return TransposeUsingTile<uint64>::run(d, in, perm, out);
    } else {
      return TransposeUsingTile<float2, true>::run(d, in, perm, out);
    }
  }
};

template <bool conjugate>
struct TransposeUsingTile<complex128, conjugate> {
  static bool run(const Eigen::GpuDevice& d, const Tensor& in,
                  const gtl::ArraySlice<int32> perm, Tensor* out) {
    if (!conjugate) {
      return TransposeUsingTile<float4>::run(d, in, perm, out);
    } else {
      return TransposeUsingTile<double2, true>::run(d, in, perm, out);
    }
  }
};

}  // namespace internal

// Transpose kernel specialized for GPU Device.
template <typename T, bool conjugate>
struct Transpose<GPUDevice, T, conjugate> {
  static void run(const GPUDevice& d, const Tensor& in,
                  const gtl::ArraySlice<int32> perm, Tensor* out) {
    switch (in.dims()) {
      case 2:
        if (!internal::TransposeUsingTile<T, conjugate>::run(d, in, perm,
                                                             out)) {
          internal::TransposeUsingEigen<GPUDevice, T, 2>(d, in, perm, conjugate,
                                                         out);
        }
        break;
      case 3:
        if (!internal::TransposeUsingTile<T, conjugate>::run(d, in, perm,
                                                             out)) {
          internal::TransposeUsingEigen<GPUDevice, T, 3>(d, in, perm, conjugate,
                                                         out);
        }
        break;
      case 4:
        if (!internal::TransposeUsingTile<T, conjugate>::run(d, in, perm,
                                                             out)) {
          internal::TransposeUsingEigen<GPUDevice, T, 4>(d, in, perm, conjugate,
                                                         out);
        }
        break;
      case 5:
        if (!internal::TransposeUsingTile<T, conjugate>::run(d, in, perm,
                                                             out)) {
          internal::TransposeUsingEigen<GPUDevice, T, 5>(d, in, perm, conjugate,
                                                         out);
        }
        break;
      case 6:
        if (!internal::TransposeUsingTile<T, conjugate>::run(d, in, perm,
                                                             out)) {
          internal::TransposeUsingEigen<GPUDevice, T, 6>(d, in, perm, conjugate,
                                                         out);
        }
        break;
      case 7:
        if (!internal::TransposeUsingTile<T, conjugate>::run(d, in, perm,
                                                             out)) {
          internal::TransposeUsingEigen<GPUDevice, T, 7>(d, in, perm, conjugate,
                                                         out);
        }
        break;
      case 8:
        if (!internal::TransposeUsingTile<T, conjugate>::run(d, in, perm,
                                                             out)) {
          internal::TransposeUsingEigen<GPUDevice, T, 8>(d, in, perm, conjugate,
                                                         out);
        }
        break;
      default:
        internal::TransposeSimple<T, conjugate>(d, in, perm, out);
        break;
    }
  }
};

template <bool conjugate>
struct Transpose<GPUDevice, string, conjugate> {
  static void run(const GPUDevice& d, const Tensor& in,
                  const gtl::ArraySlice<int32> perm, Tensor* out) {
    LOG(FATAL) << "Transpose of DT_STRING tensor not supported on GPU.";
  }
};

// Explicit instantiation.
template struct Transpose<GPUDevice, string, false>;

template <>
Status DoTranspose(const GPUDevice& device, const Tensor& in,
                   const gtl::ArraySlice<int32> perm, Tensor* out) {
  return internal::DoTransposeImpl(device, in, perm, /*conjugate=*/false, out);
}
template <>
Status DoConjugateTranspose(const GPUDevice& device, const Tensor& in,
                            const gtl::ArraySlice<int32> perm, Tensor* out) {
  return internal::DoTransposeImpl(device, in, perm, /*conjugate=*/true, out);
}
template <>
Status DoMatrixTranspose(const GPUDevice& device, const Tensor& in,
                         Tensor* out) {
  return internal::DoMatrixTransposeImpl(device, in, /*conjugate=*/false, out);
}
template <>
Status DoConjugateMatrixTranspose(const GPUDevice& device, const Tensor& in,
                                  Tensor* out) {
  return internal::DoMatrixTransposeImpl(device, in, /*conjugate=*/true, out);
}

}  // namespace tensorflow
#endif  // GOOGLE_CUDA
