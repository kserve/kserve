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

#include "tensorflow/compiler/xla/service/gpu/for_thunk.h"

#include "absl/memory/memory.h"
#include "tensorflow/compiler/xla/service/gpu/hlo_execution_profiler.h"
#include "tensorflow/compiler/xla/util.h"
#include "tensorflow/core/lib/core/errors.h"

namespace xla {
namespace gpu {

ForThunk::ForThunk(const int64 loop_limit,
                   std::unique_ptr<ThunkSequence> body_thunk_sequence,
                   const HloInstruction* hlo)
    : Thunk(Kind::kWhile, hlo),
      loop_limit_(loop_limit),
      body_thunk_sequence_(absl::make_unique<SequentialThunk>(
          // Pass nullptr as the HloInstruction* to the body_thunk_sequence_
          // constructor because this SequentialThunk is logically "part of"
          // this ForThunk, and shouldn't be profiled separately from it.
          std::move(*body_thunk_sequence), nullptr)) {}

Status ForThunk::Initialize(const GpuExecutable& executable,
                            se::StreamExecutor* executor) {
  TF_RETURN_IF_ERROR(body_thunk_sequence_->Initialize(executable, executor));
  return Status::OK();
}

Status ForThunk::ExecuteOnStream(const BufferAllocations& buffer_allocations,
                                 se::Stream* stream,
                                 HloExecutionProfiler* profiler) {
  VLOG(2) << "Executing ForThunk with " << loop_limit_ << " iters for "
          << (hlo_instruction() ? hlo_instruction()->ToString() : "<null>");
  auto op_profiler = profiler->MakeScopedInstructionProfiler(hlo_instruction());
  for (int64 i = 0; i < loop_limit_; ++i) {
    profiler->StartHloComputation();
    // Invoke loop body thunk sequence.
    TF_RETURN_IF_ERROR(body_thunk_sequence_->ExecuteOnStream(buffer_allocations,
                                                             stream, profiler));
    profiler->FinishHloComputation(hlo_instruction()->while_body());
  }
  return Status::OK();
}

}  // namespace gpu
}  // namespace xla
