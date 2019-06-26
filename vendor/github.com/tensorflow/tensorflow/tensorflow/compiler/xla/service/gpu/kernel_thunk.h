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

#ifndef TENSORFLOW_COMPILER_XLA_SERVICE_GPU_KERNEL_THUNK_H_
#define TENSORFLOW_COMPILER_XLA_SERVICE_GPU_KERNEL_THUNK_H_

#include <memory>
#include <string>
#include <vector>

#include "absl/types/span.h"
#include "tensorflow/compiler/xla/service/buffer_assignment.h"
#include "tensorflow/compiler/xla/service/gpu/buffer_allocations.h"
#include "tensorflow/compiler/xla/service/gpu/hlo_execution_profiler.h"
#include "tensorflow/compiler/xla/service/gpu/partition_assignment.h"
#include "tensorflow/compiler/xla/service/gpu/thunk.h"
#include "tensorflow/compiler/xla/service/hlo_instruction.h"
#include "tensorflow/compiler/xla/types.h"
#include "tensorflow/core/platform/mutex.h"
#include "tensorflow/core/platform/stream_executor_no_cuda.h"
#include "tensorflow/core/platform/thread_annotations.h"

namespace xla {
namespace gpu {

class GpuExecutable;

// This class stores everything that StreamExecutor needs for launching a
// kernel. It implements the ExecuteOnStream interface for GpuExecutable to
// invoke the corresponding kernel.
//
// This is thread-compatible.
class KernelThunk : public Thunk {
 public:
  // Constructs a thunk for the given kernel.
  //
  // `hlo_instruction` is as in Thunk. Other arguments are as the class members.
  KernelThunk(absl::Span<const BufferAllocation* const> args,
              const string& kernel_name, const HloInstruction* hlo_instruction,
              int unroll_factor);
  KernelThunk(const KernelThunk&) = delete;
  KernelThunk& operator=(const KernelThunk&) = delete;
  ~KernelThunk() override = default;

  const string& kernel_name() const { return kernel_name_; }
  int unroll_factor() const { return unroll_factor_; }
  void SetLaunchDimensions(const LaunchDimensions& launch_dims);

  Status Initialize(const GpuExecutable& executable,
                    se::StreamExecutor* executor) override;

  // Executes the kernel for the thunk on "stream", which must be non-null.
  Status ExecuteOnStream(const BufferAllocations& buffer_allocations,
                         se::Stream* stream,
                         HloExecutionProfiler* profiler) override;

 private:
  // Buffers passed to the kernel as arguments.
  const std::vector<const BufferAllocation*> args_;

  // Entry kernel name for the computation.
  const string kernel_name_;

  // The number of times this kernel should be unrolled. This works as a
  // multiplier on the number of elements produced by a GPU thread.
  const int unroll_factor_;

  // The thread and block dimension used to launch the kernel.
  // Will be set by IrEmitterUnnested.
  LaunchDimensions launch_dimensions_;

  // Describes how to load this kernel. ExecuteOnStream reuses this loader
  // specification for all executions.
  mutable tensorflow::mutex mutex_;
  std::unique_ptr<se::MultiKernelLoaderSpec> loader_spec_ GUARDED_BY(mutex_);

  // Loaded kernels for each `StreamExecutor`.  Requires pointer stability of
  // values.
  std::unordered_map<se::StreamExecutor*, se::KernelBase> kernel_cache_
      GUARDED_BY(mutex_);
};

}  // namespace gpu
}  // namespace xla

#endif  // TENSORFLOW_COMPILER_XLA_SERVICE_GPU_KERNEL_THUNK_H_
