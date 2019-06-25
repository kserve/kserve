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

#ifndef TENSORFLOW_STREAM_EXECUTOR_LIB_ENV_H_
#define TENSORFLOW_STREAM_EXECUTOR_LIB_ENV_H_

#include "absl/strings/string_view.h"
#include "tensorflow/core/platform/env.h"
#include "tensorflow/stream_executor/lib/status.h"
#include "tensorflow/stream_executor/platform/port.h"

namespace stream_executor {
namespace port {

using tensorflow::Env;
using tensorflow::Thread;

inline Status FileExists(const string& filename) {
  return Env::Default()->FileExists(filename);
}

inline Status FileExists(const absl::string_view& filename) {
  return Env::Default()->FileExists(string(filename));
}

}  // namespace port
}  // namespace stream_executor

#endif  // TENSORFLOW_STREAM_EXECUTOR_LIB_ENV_H_
