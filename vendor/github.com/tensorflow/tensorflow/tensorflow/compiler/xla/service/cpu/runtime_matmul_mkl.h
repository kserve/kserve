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

#ifndef TENSORFLOW_COMPILER_XLA_SERVICE_CPU_RUNTIME_MATMUL_MKL_H_
#define TENSORFLOW_COMPILER_XLA_SERVICE_CPU_RUNTIME_MATMUL_MKL_H_

#include <iostream>
#include "tensorflow/core/platform/types.h"
#ifdef INTEL_MKL
#include "third_party/intel_mkl_ml/include/mkl_cblas.h"

extern void __xla_cpu_runtime_MKLMatMulF32(
    const void* /* xla::ExecutableRunOptions* */ run_options_ptr, float* out,
    float* lhs, float* rhs, tensorflow::int64 m, tensorflow::int64 n,
    tensorflow::int64 k, tensorflow::int32 transpose_lhs,
    tensorflow::int32 transpose_rhs);
extern void __xla_cpu_runtime_MKLMatMulF64(
    const void* /* xla::ExecutableRunOptions* */ run_options_ptr, double* out,
    double* lhs, double* rhs, tensorflow::int64 m, tensorflow::int64 n,
    tensorflow::int64 k, tensorflow::int32 transpose_lhs,
    tensorflow::int32 transpose_rhs);
extern void __xla_cpu_runtime_MKLSingleThreadedMatMulF32(
    const void* /* xla::ExecutableRunOptions* */ run_options_ptr, float* out,
    float* lhs, float* rhs, tensorflow::int64 m, tensorflow::int64 n,
    tensorflow::int64 k, tensorflow::int32 transpose_lhs,
    tensorflow::int32 transpose_rhs);
extern void __xla_cpu_runtime_MKLSingleThreadedMatMulF64(
    const void* /* xla::ExecutableRunOptions* */ run_options_ptr, double* out,
    double* lhs, double* rhs, tensorflow::int64 m, tensorflow::int64 n,
    tensorflow::int64 k, tensorflow::int32 transpose_lhs,
    tensorflow::int32 transpose_rhs);

#else
extern void __xla_cpu_runtime_MKLMatMulF32(
    const void* /* xla::ExecutableRunOptions* */ run_options_ptr, float* out,
    float* lhs, float* rhs, tensorflow::int64 m, tensorflow::int64 n,
    tensorflow::int64 k, tensorflow::int32 transpose_lhs,
    tensorflow::int32 transpose_rhs) {
  std::cerr << "Attempt to call MKL MatMul runtime library without defining "
               "INTEL_MKL. Add --config=mkl to build with MKL.";
  exit(1);
}
extern void __xla_cpu_runtime_MKLMatMulF64(
    const void* /* xla::ExecutableRunOptions* */ run_options_ptr, double* out,
    double* lhs, double* rhs, tensorflow::int64 m, tensorflow::int64 n,
    tensorflow::int64 k, tensorflow::int32 transpose_lhs,
    tensorflow::int32 transpose_rhs) {
  std::cerr << "Attempt to call MKL MatMul runtime library without defining "
               "INTEL_MKL. Add --config=mkl to build with MKL.";
  exit(1);
}
extern void __xla_cpu_runtime_MKLSingleThreadedMatMulF32(
    const void* /* xla::ExecutableRunOptions* */ run_options_ptr, float* out,
    float* lhs, float* rhs, tensorflow::int64 m, tensorflow::int64 n,
    tensorflow::int64 k, tensorflow::int32 transpose_lhs,
    tensorflow::int32 transpose_rhs) {
  std::cerr << "Attempt to call MKL MatMul runtime library without defining "
               "INTEL_MKL. Add --config=mkl to build with MKL.";
  exit(1);
}
extern void __xla_cpu_runtime_MKLSingleThreadedMatMulF64(
    const void* /* xla::ExecutableRunOptions* */ run_options_ptr, double* out,
    double* lhs, double* rhs, tensorflow::int64 m, tensorflow::int64 n,
    tensorflow::int64 k, tensorflow::int32 transpose_lhs,
    tensorflow::int32 transpose_rhs) {
  std::cerr << "Attempt to call MKL MatMul runtime library without defining "
               "INTEL_MKL. Add --config=mkl to build with MKL.";
  exit(1);
}

#endif  // INTEL_MKL
#endif  // TENSORFLOW_COMPILER_XLA_SERVICE_CPU_RUNTIME_MATMUL_MKL_H_
