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

#ifndef TENSORFLOW_CONTRIB_TENSORRT_CUSTOM_PLUGIN_EXAMPLES_INC_OP_KERNEL_H_
#define TENSORFLOW_CONTRIB_TENSORRT_CUSTOM_PLUGIN_EXAMPLES_INC_OP_KERNEL_H_

#if GOOGLE_CUDA
#if GOOGLE_TENSORRT
#include "cuda/include/cuda_runtime_api.h"

namespace tensorflow {
namespace tensorrt {

void IncrementKernel(const float* d_input, float inc, float* d_output,
                     int count, cudaStream_t stream);

}  // namespace tensorrt
}  // namespace tensorflow

#endif  // GOOGLE_TENSORRT
#endif  // GOOGLE_CUDA

#endif  // TENSORFLOW_CONTRIB_TENSORRT_CUSTOM_PLUGIN_EXAMPLES_INC_OP_KERNEL_H_
