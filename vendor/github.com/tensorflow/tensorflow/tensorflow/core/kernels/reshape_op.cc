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
#include "tensorflow/core/kernels/reshape_op.h"

namespace tensorflow {

REGISTER_KERNEL_BUILDER(Name("Reshape")
                            .Device(DEVICE_CPU)
                            .HostMemory("shape")
                            .TypeConstraint<int32>("Tshape"),
                        ReshapeOp);
REGISTER_KERNEL_BUILDER(Name("Reshape")
                            .Device(DEVICE_CPU)
                            .HostMemory("shape")
                            .TypeConstraint<int64>("Tshape"),
                        ReshapeOp);

#define REGISTER_GPU_KERNEL(type)                               \
  REGISTER_KERNEL_BUILDER(Name("Reshape")                       \
                              .Device(DEVICE_GPU)               \
                              .HostMemory("shape")              \
                              .TypeConstraint<type>("T")        \
                              .TypeConstraint<int32>("Tshape"), \
                          ReshapeOp);                           \
  REGISTER_KERNEL_BUILDER(Name("Reshape")                       \
                              .Device(DEVICE_GPU)               \
                              .HostMemory("shape")              \
                              .TypeConstraint<type>("T")        \
                              .TypeConstraint<int64>("Tshape"), \
                          ReshapeOp);
TF_CALL_NUMBER_TYPES_NO_INT32(REGISTER_GPU_KERNEL);
TF_CALL_bool(REGISTER_GPU_KERNEL);
#undef REGISTER_GPU_KERNEL

#ifdef TENSORFLOW_USE_SYCL
#define REGISTER_SYCL_KERNEL(type)                              \
  REGISTER_KERNEL_BUILDER(Name("Reshape")                       \
                              .Device(DEVICE_SYCL)              \
                              .HostMemory("shape")              \
                              .TypeConstraint<type>("T")        \
                              .TypeConstraint<int32>("Tshape"), \
                          ReshapeOp);                           \
  REGISTER_KERNEL_BUILDER(Name("Reshape")                       \
                              .Device(DEVICE_SYCL)              \
                              .HostMemory("shape")              \
                              .TypeConstraint<type>("T")        \
                              .TypeConstraint<int64>("Tshape"), \
                          ReshapeOp);
REGISTER_SYCL_KERNEL(float)
REGISTER_SYCL_KERNEL(double)
REGISTER_SYCL_KERNEL(uint8)
REGISTER_SYCL_KERNEL(int8)
REGISTER_SYCL_KERNEL(int64)
REGISTER_SYCL_KERNEL(uint16)

REGISTER_KERNEL_BUILDER(Name("Reshape")
                            .Device(DEVICE_SYCL)
                            .HostMemory("tensor")
                            .HostMemory("shape")
                            .HostMemory("output")
                            .TypeConstraint<int32>("T")
                            .TypeConstraint<int32>("Tshape"),
                        ReshapeOp);
REGISTER_KERNEL_BUILDER(Name("Reshape")
                            .Device(DEVICE_SYCL)
                            .HostMemory("tensor")
                            .HostMemory("shape")
                            .HostMemory("output")
                            .TypeConstraint<int32>("T")
                            .TypeConstraint<int64>("Tshape"),
                        ReshapeOp);
#undef REGISTER_SYCL_KERNEL
#endif  // TENSORFLOW_USE_SYCL

#if GOOGLE_CUDA
// A special GPU kernel for int32.
// TODO(b/25387198): Also enable int32 in device memory. This kernel
// registration requires all int32 inputs and outputs to be in host memory.
REGISTER_KERNEL_BUILDER(Name("Reshape")
                            .Device(DEVICE_GPU)
                            .HostMemory("tensor")
                            .HostMemory("shape")
                            .HostMemory("output")
                            .TypeConstraint<int32>("T")
                            .TypeConstraint<int32>("Tshape"),
                        ReshapeOp);
REGISTER_KERNEL_BUILDER(Name("Reshape")
                            .Device(DEVICE_GPU)
                            .HostMemory("tensor")
                            .HostMemory("shape")
                            .HostMemory("output")
                            .TypeConstraint<int32>("T")
                            .TypeConstraint<int64>("Tshape"),
                        ReshapeOp);
#endif

}  // namespace tensorflow
