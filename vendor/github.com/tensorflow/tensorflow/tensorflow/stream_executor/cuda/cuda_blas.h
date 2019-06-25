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

// CUDA-specific support for BLAS functionality -- this wraps the cuBLAS library
// capabilities, and is only included into CUDA implementation code -- it will
// not introduce cuda headers into other code.

#ifndef TENSORFLOW_STREAM_EXECUTOR_CUDA_CUDA_BLAS_H_
#define TENSORFLOW_STREAM_EXECUTOR_CUDA_CUDA_BLAS_H_

#include "tensorflow/stream_executor/blas.h"
#include "tensorflow/stream_executor/host_or_device_scalar.h"
#include "tensorflow/stream_executor/platform/mutex.h"
#include "tensorflow/stream_executor/platform/port.h"
#include "tensorflow/stream_executor/platform/thread_annotations.h"
#include "tensorflow/stream_executor/plugin_registry.h"

typedef struct cublasContext *cublasHandle_t;

namespace stream_executor {

class Stream;

namespace cuda {

// Opaque and unique identifier for the cuBLAS plugin.
extern const PluginId kCuBlasPlugin;

class CUDAExecutor;

// BLAS plugin for CUDA platform via cuBLAS library.
//
// This satisfies the platform-agnostic BlasSupport interface.
//
// Note that the cuBLAS handle that this encapsulates is implicitly tied to the
// context (and, as a result, the device) that the parent CUDAExecutor is tied
// to. This simply happens as an artifact of creating the cuBLAS handle when a
// CUDA context is active.
//
// Thread-safe post-initialization.
class CUDABlas : public blas::BlasSupport {
 public:
  explicit CUDABlas(CUDAExecutor *parent);

  // Allocates a cuBLAS handle.
  bool Init();

  // Releases the cuBLAS handle, if present.
  ~CUDABlas() override;

  TENSORFLOW_STREAM_EXECUTOR_GPU_BLAS_SUPPORT_OVERRIDES

 private:
  // Tells cuBLAS to enqueue the BLAS operation onto a particular Stream.
  //
  // cuBLAS is stateful, and only be associated with one stream (in order to
  // enqueue dispatch) at a given time. As a result, this generally must be
  // invoked before calling into cuBLAS.
  bool SetStream(Stream *stream) EXCLUSIVE_LOCKS_REQUIRED(mu_);

  // A helper function that calls the real cuBLAS function together with error
  // handling.
  //
  // cublas_func:        cuBLAS function pointer.
  // cublas_name:        cuBLAS function name.
  // stream:             Stream to enqueue the BLAS operation onto.
  // pointer_mode_host:  Indicate if the pointer to a scalar value is from host
  //                     (true) or device (false).
  // err_on_failure:     Whether to print an error if the cublas function fails.
  // args:               Arguments of cuBLAS function.
  template <typename FuncT, typename... Args>
  bool DoBlasInternalImpl(FuncT cublas_func, Stream *stream,
                          bool pointer_mode_host, bool err_on_failure,
                          bool use_tensor_op_math, Args... args);

  // Convenience functions that call DoBlasInternalImpl with different values
  // for err_on_failure.
  template <typename FuncT, typename... Args>
  bool DoBlasInternal(FuncT cublas_func, Stream *stream, bool pointer_mode_host,
                      Args... args) {
    return DoBlasInternalImpl(cublas_func, stream, pointer_mode_host,
                              /*err_on_failure=*/true, /*use_tensor_ops=*/false,
                              args...);
  }
  template <typename FuncT, typename... Args>
  bool DoBlasInternalFailureOK(FuncT cublas_func, Stream *stream,
                               bool pointer_mode_host, Args... args) {
    // Tensor ops are hard-coded off in this path, but can still be enabled with
    // a specific algorithm choice as in DoBlasGemmWithAlgorithmImpl().
    return DoBlasInternalImpl(cublas_func, stream, pointer_mode_host,
                              /*err_on_failure=*/false,
                              /*use_tensor_ops=*/false, args...);
  }

  // A helper function to implement DoBlasGemmBatched interfaces for generic
  // types.
  template <typename T, typename Scalar, typename FuncT>
  port::Status DoBlasGemmBatchedInternal(
      FuncT cublas_func, Stream *stream, blas::Transpose transa,
      blas::Transpose transb, uint64 m, uint64 n, uint64 k, Scalar alpha,
      const port::ArraySlice<DeviceMemory<T> *> &a_array, int lda,
      const port::ArraySlice<DeviceMemory<T> *> &b_array, int ldb, Scalar beta,
      const port::ArraySlice<DeviceMemory<T> *> &c_array, int ldc,
      int batch_count, ScratchAllocator *scratch_allocator);

  // Helper function for implementing DoBlasGemmWithAlgorithm.
  template <typename InT, typename OutT, typename CompT>
  bool DoBlasGemmWithAlgorithmImpl(
      Stream *stream, blas::Transpose transa, blas::Transpose transb, uint64 m,
      uint64 n, uint64 k, const HostOrDeviceScalar<CompT> &alpha,
      const DeviceMemory<InT> &a, int lda, const DeviceMemory<InT> &b, int ldb,
      const HostOrDeviceScalar<CompT> &beta, DeviceMemory<OutT> *c, int ldc,
      blas::ComputationType computation_type, blas::AlgorithmType algorithm,
      blas::ProfileResult *output_profile_result);

  // Helper function for implementing DoBlasGemmWithProfiling.
  template <typename T, typename ParamType>
  bool DoBlasGemmWithProfilingImpl(
      Stream *stream, blas::Transpose transa, blas::Transpose transb, uint64 m,
      uint64 n, uint64 k, const ParamType &alpha, const DeviceMemory<T> &a,
      int lda, const DeviceMemory<T> &b, int ldb, const ParamType &beta,
      DeviceMemory<T> *c, int ldc, blas::ProfileResult *output_profile_result);

  // Helper function for implementing DoBlasGemvWithProfiling.
  template <typename T>
  bool DoBlasGemvWithProfilingImpl(Stream *stream, blas::Transpose trans,
                                   uint64 m, uint64 n, const T &alpha,
                                   const DeviceMemory<T> &a, int lda,
                                   const DeviceMemory<T> &x, int incx,
                                   const T &beta, DeviceMemory<T> *y, int incy,
                                   blas::ProfileResult *output_profile_result);

  // mutex that guards the cuBLAS handle for this device.
  mutex mu_;

  // CUDAExecutor which instantiated this CUDABlas.
  // Immutable post-initialization.
  CUDAExecutor *parent_;

  // cuBLAS library handle on the device.
  cublasHandle_t blas_ GUARDED_BY(mu_);

  SE_DISALLOW_COPY_AND_ASSIGN(CUDABlas);
};

}  // namespace cuda
}  // namespace stream_executor

#endif  // TENSORFLOW_STREAM_EXECUTOR_CUDA_CUDA_BLAS_H_
