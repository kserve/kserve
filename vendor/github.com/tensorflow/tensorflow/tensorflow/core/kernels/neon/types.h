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
#ifndef TENSORFLOW_CORE_KERNELS_NEON_TYPES_H_
#define TENSORFLOW_CORE_KERNELS_NEON_TYPES_H_

#include "tensorflow/core/platform/logging.h"

namespace tensorflow {
namespace neon {

enum class FusedActivationFunctionType { kNone, kRelu6, kRelu1, kRelu };

template <int N>
struct Dims {
  int sizes[N];
  int strides[N];
};

inline int Offset(const Dims<4>& dims, int i0, int i1, int i2, int i3) {
  DCHECK(i0 >= 0 && i0 < dims.sizes[0]);
  DCHECK(i1 >= 0 && i1 < dims.sizes[1]);
  DCHECK(i2 >= 0 && i2 < dims.sizes[2]);
  DCHECK(i3 >= 0 && i3 < dims.sizes[3]);
  return i0 * dims.strides[0] + i1 * dims.strides[1] + i2 * dims.strides[2] +
         i3 * dims.strides[3];
}

// Get array size, DCHECKing that the dim index is in range.
template <int N>
int ArraySize(const Dims<N>& array, int index) {
  DCHECK(index >= 0 && index < N);
  return array.sizes[index];
}

// Get common array size, DCHECKing that they all agree.
template <typename ArrayType1, typename ArrayType2>
int MatchingArraySize(const ArrayType1& array1, int index1,
                      const ArrayType2& array2, int index2) {
  DCHECK_EQ(ArraySize(array1, index1), ArraySize(array2, index2));
  return ArraySize(array1, index1);
}

template <typename ArrayType1, typename ArrayType2, typename... Args>
int MatchingArraySize(const ArrayType1& array1, int index1,
                      const ArrayType2& array2, int index2, Args... args) {
  DCHECK_EQ(ArraySize(array1, index1), ArraySize(array2, index2));
  return MatchingArraySize(array1, index1, args...);
}

inline int RequiredBufferSizeForDims(const Dims<4>& dims) {
  int max_offset = 0;
  for (int i = 0; i < 4; i++) {
    max_offset += (dims.sizes[i] - 1) * dims.strides[i];
  }
  return max_offset + 1;
}

}  // end namespace neon
}  // end namespace tensorflow

#endif  // TENSORFLOW_CORE_KERNELS_NEON_TYPES_H_
