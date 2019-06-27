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

// IWYU pragma: private, include "third_party/tensorflow/stream_executor/stream_executor.h"

#ifndef TENSORFLOW_STREAM_EXECUTOR_LIB_STATUS_H_
#define TENSORFLOW_STREAM_EXECUTOR_LIB_STATUS_H_

#include "absl/strings/string_view.h"
#include "tensorflow/core/lib/core/status.h"
#include "tensorflow/stream_executor/lib/error.h"  // IWYU pragma: export
#include "tensorflow/stream_executor/platform/logging.h"

namespace stream_executor {
namespace port {

using Status = tensorflow::Status;

#define SE_CHECK_OK(val) TF_CHECK_OK(val)
#define SE_ASSERT_OK(val) \
  ASSERT_EQ(::stream_executor::port::Status::OK(), (val))

// Define some canonical error helpers.
inline Status UnimplementedError(absl::string_view message) {
  return Status(error::UNIMPLEMENTED, message);
}
inline Status InternalError(absl::string_view message) {
  return Status(error::INTERNAL, message);
}
inline Status FailedPreconditionError(absl::string_view message) {
  return Status(error::FAILED_PRECONDITION, message);
}

}  // namespace port
}  // namespace stream_executor

namespace perftools {
namespace gputools {

// Temporarily pull stream_executor into perftools::gputools while we migrate
// code to the new namespace.  TODO(b/77980417): Remove this once we've
// completed the migration.
using namespace stream_executor;  // NOLINT[build/namespaces]

}  // namespace gputools
}  // namespace perftools

#endif  // TENSORFLOW_STREAM_EXECUTOR_LIB_STATUS_H_
