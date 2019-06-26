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
#if GOOGLE_CUDA

// See docs in ../ops/array_ops.cc.
#include "tensorflow/core/kernels/snapshot_op.h"

#include "tensorflow/core/framework/op_kernel.h"
#include "tensorflow/core/framework/register_types.h"
#include "tensorflow/core/framework/types.h"

namespace tensorflow {
typedef Eigen::GpuDevice GPUDevice;

// Definition of the GPU implementations declared in softsign_op.cc.
#define DEFINE_GPU_KERNELS(T) template struct functor::Snapshot<GPUDevice, T>;

TF_CALL_POD_TYPES(DEFINE_GPU_KERNELS);

}  // namespace tensorflow

#endif  // GOOGLE_CUDA
