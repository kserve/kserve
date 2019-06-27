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

#ifndef TENSORFLOW_LITE_EXPERIMENTAL_MICRO_SIMPLE_TENSOR_ALLOCATOR_H_
#define TENSORFLOW_LITE_EXPERIMENTAL_MICRO_SIMPLE_TENSOR_ALLOCATOR_H_

#include "tensorflow/lite/c/c_api_internal.h"
#include "tensorflow/lite/core/api/error_reporter.h"
#include "tensorflow/lite/schema/schema_generated.h"

namespace tflite {

// TODO(petewarden): This allocator never frees up or reuses  any memory, even
// though we have enough information about lifetimes of the tensors to do so.
// This makes it pretty wasteful, so we should use a more intelligent method.
class SimpleTensorAllocator {
 public:
  SimpleTensorAllocator(uint8_t* buffer, int buffer_size)
      : data_size_(0), data_size_max_(buffer_size), data_(buffer) {}

  TfLiteStatus AllocateTensor(
      const tflite::Tensor& flatbuffer_tensor, int create_before,
      int destroy_after,
      const flatbuffers::Vector<flatbuffers::Offset<Buffer>>* buffers,
      ErrorReporter* error_reporter, TfLiteTensor* result);

  uint8_t* AllocateMemory(size_t size, size_t alignment);

  int GetDataSize() const { return data_size_; }

 private:
  int data_size_;
  int data_size_max_;
  uint8_t* data_;
};

}  // namespace tflite

#endif  // TENSORFLOW_LITE_EXPERIMENTAL_MICRO_SIMPLE_TENSOR_ALLOCATOR_H_
