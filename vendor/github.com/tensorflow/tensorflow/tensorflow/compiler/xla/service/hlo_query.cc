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

#include "tensorflow/compiler/xla/service/hlo_query.h"

#include "tensorflow/compiler/xla/literal.h"
#include "tensorflow/compiler/xla/service/hlo_opcode.h"
#include "tensorflow/compiler/xla/shape_util.h"

namespace xla {
namespace hlo_query {

bool IsConstantR0F32(HloInstruction* instruction, float* out) {
  if (instruction->opcode() == HloOpcode::kConstant &&
      ShapeUtil::IsScalarWithElementType(instruction->shape(), F32)) {
    *out = instruction->literal().Get<float>({});
    return true;
  }

  return false;
}

bool AllOperandsAreParametersOrConstants(const HloInstruction& instruction) {
  for (const auto& operand : instruction.operands()) {
    if (operand->opcode() != HloOpcode::kParameter &&
        operand->opcode() != HloOpcode::kConstant) {
      return false;
    }
  }
  return true;
}

bool AllOperandsAreParameters(const HloInstruction& instruction) {
  for (const auto& operand : instruction.operands()) {
    if (operand->opcode() != HloOpcode::kParameter) {
      return false;
    }
  }
  return true;
}

bool AllOperandsAreConstants(const HloInstruction& instruction) {
  for (const auto& operand : instruction.operands()) {
    if (operand->opcode() != HloOpcode::kConstant) {
      return false;
    }
  }
  return true;
}

HloInstruction* GetMatchingOperand(
    const std::function<bool(const HloInstruction*)>& matcher,
    HloInstruction* instruction) {
  for (HloInstruction* op : instruction->operands()) {
    if (matcher(op)) {
      return op;
    }
  }
  return nullptr;
}

bool MatchBinaryInstructionOperand(
    const std::function<bool(const HloInstruction*)>& matcher,
    HloInstruction* instruction, HloInstruction** matching_operand,
    HloInstruction** other_operand) {
  CHECK_EQ(instruction->operand_count(), 2);
  if (matcher(instruction->operand(0))) {
    *matching_operand = instruction->mutable_operand(0);
    *other_operand = instruction->mutable_operand(1);
    return true;
  }
  if (matcher(instruction->operand(1))) {
    *matching_operand = instruction->mutable_operand(1);
    *other_operand = instruction->mutable_operand(0);
    return true;
  }
  return false;
}

bool MatchBinaryInstructionOperandOpcode(HloOpcode opcode,
                                         HloInstruction* instruction,
                                         HloInstruction** matching_operand,
                                         HloInstruction** other_operand) {
  return MatchBinaryInstructionOperand(
      [opcode](const HloInstruction* instruction) {
        return instruction->opcode() == opcode;
      },
      instruction, matching_operand, other_operand);
}

bool IsScalarConstant(const HloInstruction* instruction) {
  return instruction->IsConstant() && ShapeUtil::IsScalar(instruction->shape());
}

bool ContainsInstrWithOpcode(const HloComputation* comp,
                             const absl::flat_hash_set<HloOpcode>& opcodes) {
  for (const auto* instr : comp->instructions()) {
    if (opcodes.count(instr->opcode())) {
      return true;
    }
    for (const HloComputation* subcomp : instr->called_computations()) {
      if (ContainsInstrWithOpcode(subcomp, opcodes)) {
        return true;
      }
    }
  }
  return false;
}

}  // namespace hlo_query
}  // namespace xla
