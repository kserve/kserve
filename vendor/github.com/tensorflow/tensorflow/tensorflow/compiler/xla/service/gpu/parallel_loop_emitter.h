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

#ifndef TENSORFLOW_COMPILER_XLA_SERVICE_GPU_PARALLEL_LOOP_EMITTER_H_
#define TENSORFLOW_COMPILER_XLA_SERVICE_GPU_PARALLEL_LOOP_EMITTER_H_

#include "llvm/IR/IRBuilder.h"
#include "tensorflow/compiler/xla/service/gpu/partition_assignment.h"
#include "tensorflow/compiler/xla/service/llvm_ir/ir_array.h"
#include "tensorflow/compiler/xla/service/llvm_ir/loop_emitter.h"

namespace xla {
namespace gpu {

// Emits a parallel loop for every element in the given array shape. This loop
// emitted will be executed by multiple threads in parallel. Therefore, each
// thread instance of the loop iterates over part of the array, and they
// collectively iterates over the entire array.
class ParallelLoopEmitter : public llvm_ir::LoopEmitter {
 public:
  // `thread_count` is the number of threads to parallelize the loop on.
  // The meanings of other parameters are the same as LoopEmitter.
  ParallelLoopEmitter(BodyEmitter body_emitter, const Shape& shape,
                      const LaunchDimensions& launch_dimensions,
                      llvm::IRBuilder<>* b, int unroll_factor = 1);
  // Constructs a ParallelLoopEmitter from an element generator that generates
  // each element of the given target array.
  ParallelLoopEmitter(const llvm_ir::ElementGenerator& target_element_generator,
                      const llvm_ir::IrArray& target_array,
                      const LaunchDimensions& launch_dimensions,
                      llvm::IRBuilder<>* b, int unroll_factor = 1);

  // Constructs a loop emitter for a loop that generates on element of each of N
  // arrays on each iteration.
  //
  // This is used in multi-output fusion.  target_element_generator should
  // produce a struct with N elements, one for each of target_arrays.
  ParallelLoopEmitter(const llvm_ir::ElementGenerator& target_element_generator,
                      absl::Span<const llvm_ir::IrArray> target_arrays,
                      const LaunchDimensions& launch_dimensions,
                      llvm::IRBuilder<>* b, int unroll_factor = 1);

  ParallelLoopEmitter(const ParallelLoopEmitter&) = delete;
  ParallelLoopEmitter& operator=(const ParallelLoopEmitter&) = delete;
  ~ParallelLoopEmitter() override = default;

  std::vector<llvm_ir::IrArray::Index> EmitIndexAndSetExitBasicBlock(
      absl::string_view loop_name, llvm::Type* index_type) override;

 private:
  // The thread and block dimension to parallelize the loop on.
  const LaunchDimensions launch_dimensions_;
  const int unroll_factor_;
};

}  // namespace gpu
}  // namespace xla

#endif  // TENSORFLOW_COMPILER_XLA_SERVICE_GPU_PARALLEL_LOOP_EMITTER_H_
