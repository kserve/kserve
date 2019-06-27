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
#ifndef TENSORFLOW_LITE_DELEGATES_FLEX_DELEGATE_DATA_H_
#define TENSORFLOW_LITE_DELEGATES_FLEX_DELEGATE_DATA_H_

#include "tensorflow/lite/delegates/flex/buffer_map.h"
#include "tensorflow/core/common_runtime/eager/context.h"

namespace tflite {
namespace flex {

// Data kept by the Flex delegate for the lifetime of an Interpreter.
class DelegateData {
 public:
  // Create a new DelegateData, initialized with a newly-created EagerContext.
  static tensorflow::Status Create(std::unique_ptr<DelegateData>* data);

  ~DelegateData();

  // The EagerContext that is required for execution of Flex Ops.
  tensorflow::EagerContext* GetEagerContext() { return eager_context_.get(); }

  // Map from TF Lite tensor index to TensorFlow tensor for a given context.
  BufferMap* GetBufferMap(const TfLiteContext* context) {
    return &buffer_map_[context];
  }

 private:
  explicit DelegateData(tensorflow::EagerContext* eager_context);

  std::unique_ptr<tensorflow::EagerContext> eager_context_;
  // TODO(b/112439500): Clean up stale BufferMap instances after adding the
  // necessary cleanup hook from a TfLiteContext to a TfLiteDelegate.
  std::unordered_map<const TfLiteContext*, BufferMap> buffer_map_;
};

}  // namespace flex
}  // namespace tflite

#endif  // TENSORFLOW_LITE_DELEGATES_FLEX_DELEGATE_DATA_H_
