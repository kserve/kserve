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

#ifndef TENSORFLOW_CORE_KERNELS_CAST_OP_IMPL_H_
#define TENSORFLOW_CORE_KERNELS_CAST_OP_IMPL_H_

#define EIGEN_USE_THREADS

#include "tensorflow/core/framework/op_kernel.h"
#include "tensorflow/core/kernels/cast_op.h"

namespace tensorflow {

namespace functor {

CAST_FUNCTORS(Eigen::ThreadPoolDevice);

#ifdef TENSORFLOW_USE_SYCL
CAST_FUNCTORS(Eigen::SyclDevice);
#endif  // TENSORFLOW_USE_SYCL

}  // namespace functor

#define CURRY_TYPES3_NO_HALF(FN, arg0, arg1) \
  FN(arg0, arg1, bool);                      \
  FN(arg0, arg1, uint8);                     \
  FN(arg0, arg1, uint16);                    \
  FN(arg0, arg1, uint32);                    \
  FN(arg0, arg1, uint64);                    \
  FN(arg0, arg1, int8);                      \
  FN(arg0, arg1, int16);                     \
  FN(arg0, arg1, int32);                     \
  FN(arg0, arg1, int64);                     \
  FN(arg0, arg1, float);                     \
  FN(arg0, arg1, double);                    \
  FN(arg0, arg1, std::complex<float>);       \
  FN(arg0, arg1, std::complex<double>)

#define CURRY_TYPES3_NO_BF16(FN, arg0, arg1) \
  CURRY_TYPES3_NO_HALF(FN, arg0, arg1)       \
  FN(arg0, arg1, Eigen::half);

#define CURRY_TYPES3(FN, arg0, arg1)   \
  CURRY_TYPES3_NO_BF16(FN, arg0, arg1) \
  FN(arg0, arg1, bfloat16);

#define CAST_CASE(DEVICE, IN, OUT)                                        \
  if (DataTypeToEnum<OUT>::value == dst_dtype) {                          \
    return [](OpKernelContext* ctx, const Tensor& inp, Tensor* out,       \
              bool truncate) {                                            \
      functor::CastFunctor<DEVICE, OUT, IN> func;                         \
      func(ctx->eigen_device<DEVICE>(), out->flat<OUT>(), inp.flat<IN>(), \
           truncate);                                                     \
    };                                                                    \
  }

// The functions below are implemented in the cast_op_impl_*.cc files.
CastFunctorType GetCpuCastFromBool(DataType dst_dtype);

CastFunctorType GetCpuCastFromUint8(DataType dst_dtype);

CastFunctorType GetCpuCastFromUint16(DataType dst_dtype);

CastFunctorType GetCpuCastFromInt8(DataType dst_dtype);

CastFunctorType GetCpuCastFromUint32(DataType dst_dtype);

CastFunctorType GetCpuCastFromUint64(DataType dst_dtype);

CastFunctorType GetCpuCastFromInt8(DataType dst_dtype);

CastFunctorType GetCpuCastFromInt16(DataType dst_dtype);

CastFunctorType GetCpuCastFromInt32(DataType dst_dtype);

CastFunctorType GetCpuCastFromInt64(DataType dst_dtype);

CastFunctorType GetCpuCastFromHalf(DataType dst_dtype);

CastFunctorType GetCpuCastFromFloat(DataType dst_dtype);

CastFunctorType GetCpuCastFromDouble(DataType dst_dtype);

CastFunctorType GetCpuCastFromComplex64(DataType dst_dtype);

CastFunctorType GetCpuCastFromComplex128(DataType dst_dtype);

CastFunctorType GetCpuCastFromBfloat(DataType dst_dtype);

#if GOOGLE_CUDA
// Same, for GPU.
CastFunctorType GetGpuCastFromBool(DataType dst_dtype);

CastFunctorType GetGpuCastFromUint8(DataType dst_dtype);

CastFunctorType GetGpuCastFromUint16(DataType dst_dtype);

CastFunctorType GetGpuCastFromInt8(DataType dst_dtype);

CastFunctorType GetGpuCastFromUint32(DataType dst_dtype);

CastFunctorType GetGpuCastFromUint64(DataType dst_dtype);

CastFunctorType GetGpuCastFromInt16(DataType dst_dtype);

CastFunctorType GetGpuCastFromInt32(DataType dst_dtype);

CastFunctorType GetGpuCastFromInt64(DataType dst_dtype);

CastFunctorType GetGpuCastFromHalf(DataType dst_dtype);

CastFunctorType GetGpuCastFromFloat(DataType dst_dtype);

CastFunctorType GetGpuCastFromDouble(DataType dst_dtype);

CastFunctorType GetGpuCastFromComplex64(DataType dst_dtype);

CastFunctorType GetGpuCastFromComplex128(DataType dst_dtype);

CastFunctorType GetGpuCastFromBfloat(DataType dst_dtype);

#endif  // GOOGLE_CUDA

#ifdef TENSORFLOW_USE_SYCL
CastFunctorType GetSyclCastFromBool(DataType dst_dtype);

CastFunctorType GetSyclCastFromUint8(DataType dst_dtype);

CastFunctorType GetSyclCastFromUint16(DataType dst_dtype);

CastFunctorType GetSyclCastFromUint32(DataType dst_dtype);

CastFunctorType GetSyclCastFromUint64(DataType dst_dtype);

CastFunctorType GetSyclCastFromInt16(DataType dst_dtype);

CastFunctorType GetSyclCastFromInt32(DataType dst_dtype);

CastFunctorType GetSyclCastFromInt64(DataType dst_dtype);

CastFunctorType GetSyclCastFromFloat(DataType dst_dtype);

CastFunctorType GetSyclCastFromDouble(DataType dst_dtype);
#endif  // TENSORFLOW_USE_SYCL

}  // namespace tensorflow

#endif  // TENSORFLOW_CORE_KERNELS_CAST_OP_IMPL_H_
