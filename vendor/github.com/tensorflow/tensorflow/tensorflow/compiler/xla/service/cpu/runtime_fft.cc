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

#include "tensorflow/compiler/xla/service/cpu/runtime_fft.h"

#define EIGEN_USE_THREADS

#include "tensorflow/compiler/xla/executable_run_options.h"
#include "tensorflow/compiler/xla/service/cpu/runtime_fft_impl.h"
#include "tensorflow/core/platform/dynamic_annotations.h"
#include "tensorflow/core/platform/types.h"

using tensorflow::int32;
using tensorflow::int64;

TF_ATTRIBUTE_NO_SANITIZE_MEMORY void __xla_cpu_runtime_EigenFft(
    const void* run_options_ptr, void* out, void* operand, int32 fft_type,
    int32 fft_rank, int64 input_batch, int64 fft_length0, int64 fft_length1,
    int64 fft_length2) {
  const xla::ExecutableRunOptions* run_options =
      static_cast<const xla::ExecutableRunOptions*>(run_options_ptr);
  tensorflow::xla::EigenFftImpl(*run_options->intra_op_thread_pool(), out,
                                operand, fft_type, fft_rank, input_batch,
                                fft_length0, fft_length1, fft_length2);
}
