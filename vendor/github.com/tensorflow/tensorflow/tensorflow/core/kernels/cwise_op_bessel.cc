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

#include "tensorflow/core/kernels/cwise_ops_common.h"

namespace tensorflow {
REGISTER3(UnaryOp, CPU, "BesselI0e", functor::bessel_i0e, Eigen::half, float,
          double);
REGISTER3(UnaryOp, CPU, "BesselI1e", functor::bessel_i1e, Eigen::half, float,
          double);
#if GOOGLE_CUDA
REGISTER3(UnaryOp, GPU, "BesselI0e", functor::bessel_i0e, Eigen::half, float,
          double);
REGISTER3(UnaryOp, GPU, "BesselI1e", functor::bessel_i1e, Eigen::half, float,
          double);
#endif
}  // namespace tensorflow
