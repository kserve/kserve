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

#ifndef TENSORFLOW_COMPILER_XLA_SERVICE_GPU_INSTRUCTION_FUSION_H_
#define TENSORFLOW_COMPILER_XLA_SERVICE_GPU_INSTRUCTION_FUSION_H_

#include "tensorflow/compiler/xla/service/hlo_instruction.h"
#include "tensorflow/compiler/xla/service/instruction_fusion.h"

namespace xla {
namespace gpu {

class GpuInstructionFusion : public InstructionFusion {
 public:
  explicit GpuInstructionFusion(bool may_duplicate)
      : InstructionFusion(GpuInstructionFusion::IsExpensive, may_duplicate) {}

  // Maximum number of operands plus outputs allowed on a single fusion node.
  // Exposed publicly mainly for tests.
  static constexpr int64 kMaxOperandsAndOutputsPerFusion = 64;

  // Determines whether the combination of `a` and `b` into a (possibly
  // multi-output) fusion would be "too large" -- i.e., have more operands and
  // outputs than is allowed.
  //
  // `ShouldFuse` and `ShouldFuseIntoMultiOutput` call this; it's public so that
  // other fusion passes (e.g. GPU multi-output fusion) can also call this.
  static bool FusionWouldBeTooLarge(const HloInstruction* a,
                                    const HloInstruction* b);

  static bool IsExpensive(const HloInstruction& instruction);

  bool ShouldFuse(HloInstruction* consumer, int64 operand_index) override;

  bool ShouldFuseIntoMultiOutput(HloInstruction* consumer,
                                 int64 operand_index) override;

  HloInstruction::FusionKind ChooseKind(
      const HloInstruction* producer, const HloInstruction* consumer) override;
};

}  // namespace gpu
}  // namespace xla

#endif  // TENSORFLOW_COMPILER_XLA_SERVICE_GPU_INSTRUCTION_FUSION_H_
