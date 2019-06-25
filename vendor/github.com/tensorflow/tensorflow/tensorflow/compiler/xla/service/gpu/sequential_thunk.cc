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

#include "tensorflow/compiler/xla/service/gpu/sequential_thunk.h"

#include "tensorflow/compiler/xla/service/gpu/hlo_execution_profiler.h"
#include "tensorflow/core/lib/core/errors.h"

namespace xla {
namespace gpu {

SequentialThunk::SequentialThunk(std::vector<std::unique_ptr<Thunk>> thunks,
                                 const HloInstruction* hlo)
    : Thunk(Kind::kSequential, hlo), thunks_(std::move(thunks)) {}

Status SequentialThunk::Initialize(const GpuExecutable& executable,
                                   se::StreamExecutor* executor) {
  for (auto& thunk : thunks_) {
    TF_RETURN_IF_ERROR(thunk->Initialize(executable, executor));
  }
  return Status::OK();
}

Status SequentialThunk::ExecuteOnStream(
    const BufferAllocations& buffer_allocations, se::Stream* stream,
    HloExecutionProfiler* profiler) {
  auto op_profiler = profiler->MakeScopedInstructionProfiler(hlo_instruction());
  for (const auto& thunk : thunks_) {
    TF_RETURN_IF_ERROR(
        thunk->ExecuteOnStream(buffer_allocations, stream, profiler));
  }
  return Status::OK();
}

}  // namespace gpu
}  // namespace xla
