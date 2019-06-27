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

#ifndef TENSORFLOW_COMPILER_XLA_CLIENT_EXECUTABLE_BUILD_OPTIONS_H_
#define TENSORFLOW_COMPILER_XLA_CLIENT_EXECUTABLE_BUILD_OPTIONS_H_

#include "absl/strings/string_view.h"
#include "absl/types/optional.h"
#include "tensorflow/compiler/xla/service/device_memory_allocator.h"
#include "tensorflow/compiler/xla/shape.h"
#include "tensorflow/compiler/xla/util.h"
#include "tensorflow/compiler/xla/xla.pb.h"
#include "tensorflow/compiler/xla/xla_data.pb.h"

namespace xla {

// Class containing options for building an LocalExecutable with
// LocalClient::Compile.
class ExecutableBuildOptions {
 public:
  // If set, this is the device to build the computation for. Valid
  // device_ordinal values are: 0 to # of devices - 1. These values are
  // identical to the device ordinal values used by StreamExecutor. The built
  // executable will be executable on any device equivalent to the specified
  // device as determined by Backend::devices_equivalent(). A value of -1
  // indicates this option has not been set.
  ExecutableBuildOptions& set_device_ordinal(int device_ordinal);
  int device_ordinal() const;

  // If set, this specifies the layout of the result of the computation. If not
  // set, the service will chose the layout of the result. A Shape is used to
  // store the layout to accommodate tuple result shapes. A value of nullptr
  // indicates the option has not been set.
  ExecutableBuildOptions& set_result_layout(const Shape& shape_with_layout);
  const Shape* result_layout() const;

  // Expose access to the XLA debug options which will be passed to the
  // compilation process.
  bool has_debug_options() const { return debug_options_.has_value(); }
  const DebugOptions& debug_options() const { return *debug_options_; }
  DebugOptions* mutable_debug_options();

  // If set, this specifies an allocator that can be used to allocate temporary
  // space on the device during compilation.  For example, the compiler might
  // want to run various algorithms on the device and pick the fastest one -- it
  // might allocate buffers for use by these algorithms using this allocator.
  //
  // This does not need to be the same as the DeviceMemoryAllocator passed when
  // running the executable.
  ExecutableBuildOptions& set_device_allocator(
      DeviceMemoryAllocator* allocator);
  DeviceMemoryAllocator* device_allocator() const;

  // Returns a string representation of the build options, suitable for
  // debugging.
  string ToString() const;

 private:
  int device_ordinal_ = -1;
  Shape result_layout_;
  bool result_layout_set_ = false;
  absl::optional<DebugOptions> debug_options_;
  DeviceMemoryAllocator* device_allocator_ = nullptr;
};

}  // namespace xla

#endif  // TENSORFLOW_COMPILER_XLA_CLIENT_EXECUTABLE_BUILD_OPTIONS_H_
