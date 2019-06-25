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

#include "tensorflow/compiler/xla/service/bfloat16_normalization.h"

#include "absl/types/span.h"
#include "tensorflow/compiler/xla/service/hlo_computation.h"
#include "tensorflow/compiler/xla/service/hlo_instruction.h"
#include "tensorflow/compiler/xla/service/hlo_opcode.h"
#include "tensorflow/compiler/xla/shape_util.h"
#include "tensorflow/compiler/xla/status_macros.h"
#include "tensorflow/compiler/xla/xla_data.pb.h"
#include "tensorflow/core/platform/logging.h"
#include "tensorflow/core/platform/types.h"

namespace xla {

class BFloat16NormalizationVisitor : public DfsHloVisitorWithDefault {
 public:
  explicit BFloat16NormalizationVisitor(HloComputation* computation,
                                        const BFloat16Support* bfloat16_support)
      : computation_(computation), bfloat16_support_(bfloat16_support) {}

  Status DefaultAction(HloInstruction* hlo) override;

  static bool Run(HloComputation* computation,
                  const BFloat16Support* bfloat16_support) {
    BFloat16NormalizationVisitor visitor(computation, bfloat16_support);
    TF_CHECK_OK(computation->Accept(&visitor));
    return visitor.changed_;
  }

 private:
  // Checks if the HLO uses BF16 in an unsupported way, and if so, inserts
  // conversions between F32 and BF16 to make it supported.
  Status HandleInstruction(HloInstruction* hlo);

  // Handle instructions with tuple outputs by examining each output
  // independently.
  Status HandleMultipleOutputs(HloInstruction* hlo);

  // Inserts a conversion HLO that changes the given HLO's output type.
  Status InsertConvertAfterOutput(HloInstruction* hlo, PrimitiveType to,
                                  HloComputation* computation);

  // Changes the output type to the specified type, then inserts a conversion
  // to the original type.
  Status ChangeOutputTypeThenInsertConvertBack(HloInstruction* hlo,
                                               PrimitiveType to,
                                               HloComputation* computation);

  // Inserts a conversion HLO that changes the given HLO's operand type.
  Status InsertConvertBeforeOperand(HloInstruction* hlo, int64 operand_idx,
                                    PrimitiveType to,
                                    HloComputation* computation);

  // Inserts conversion HLOs to replace the called computations' BF16
  // operands/outputs to F32.
  Status ConvertCalledComputations(
      HloInstruction* hlo, absl::Span<HloComputation* const> bf16_called_comps);

  HloComputation* computation_;
  const BFloat16Support* bfloat16_support_;
  bool changed_ = false;
};

Status BFloat16NormalizationVisitor::InsertConvertAfterOutput(
    HloInstruction* hlo, PrimitiveType to, HloComputation* computation) {
  bool is_root = computation->root_instruction() == hlo;
  std::vector<HloInstruction*> materialized_users = hlo->users();
  // Use inst's shape temporarily, in order to pass checks in ReplaceUseWith.
  auto convert = computation->AddInstruction(
      HloInstruction::CreateConvert(hlo->shape(), hlo));
  for (auto* user : materialized_users) {
    TF_RETURN_IF_ERROR(hlo->ReplaceUseWith(user, convert));
  }
  if (is_root) {
    computation->set_root_instruction(convert);
  }
  convert->mutable_shape()->set_element_type(to);
  changed_ = true;
  return Status::OK();
}

Status BFloat16NormalizationVisitor::ChangeOutputTypeThenInsertConvertBack(
    HloInstruction* hlo, PrimitiveType to, HloComputation* computation) {
  auto original_type = hlo->shape().element_type();
  hlo->mutable_shape()->set_element_type(to);
  return InsertConvertAfterOutput(hlo, original_type, computation);
}

Status BFloat16NormalizationVisitor::InsertConvertBeforeOperand(
    HloInstruction* hlo, int64 operand_idx, PrimitiveType to,
    HloComputation* computation) {
  auto operand = hlo->mutable_operand(operand_idx);
  auto convert = computation->AddInstruction(HloInstruction::CreateConvert(
      ShapeUtil::ChangeElementType(operand->shape(), to), operand));
  TF_RETURN_IF_ERROR(hlo->ReplaceOperandWith(operand_idx, convert));
  changed_ = true;
  return Status::OK();
}

Status BFloat16NormalizationVisitor::ConvertCalledComputations(
    HloInstruction* hlo, absl::Span<HloComputation* const> bf16_called_comps) {
  std::map<HloComputation*, HloComputation*> cloned_computations;
  for (auto& comp : bf16_called_comps) {
    auto cloned = comp->parent()->AddEmbeddedComputation(comp->Clone());
    cloned_computations[comp] = cloned;
    changed_ = true;
  }
  hlo->ReplaceCalledComputations([&](HloComputation* comp) {
    auto it = cloned_computations.find(comp);
    if (it != cloned_computations.end()) {
      return it->second;
    }
    return comp;
  });
  for (auto& comp_pair : cloned_computations) {
    auto comp = comp_pair.second;
    if (comp->root_instruction()->shape().element_type() == BF16) {
      TF_RETURN_IF_ERROR(
          InsertConvertAfterOutput(comp->root_instruction(), F32, comp));
    }
    for (auto* param : comp->parameter_instructions()) {
      if (param->shape().element_type() == BF16) {
        // This changes the parameter to F32 then inserts a convert after it.
        TF_RETURN_IF_ERROR(
            ChangeOutputTypeThenInsertConvertBack(param, F32, comp));
      }
    }
  }
  return Status::OK();
}

Status BFloat16NormalizationVisitor::HandleMultipleOutputs(
    HloInstruction* hlo) {
  std::vector<PrimitiveType> operand_types(hlo->operand_count());
  std::vector<PrimitiveType> output_types(hlo->operand_count());
  int64 f32_count = 0;
  int64 bf16_count = 0;
  bool has_unsupported_bf16_operand = false;
  bool has_unsupported_bf16_output = false;
  for (int64 i = 0; i < hlo->operand_count(); ++i) {
    operand_types[i] = hlo->operand(i)->shape().element_type();
    output_types[i] = ShapeUtil::GetSubshape(hlo->shape(), {i}).element_type();
    if (operand_types[i] == F32) {
      f32_count += 1;
    } else if (operand_types[i] == BF16) {
      bf16_count += 1;
      if (!bfloat16_support_->SupportsBF16Operand(*hlo, i)) {
        has_unsupported_bf16_operand = true;
      }
    }
    if (output_types[i] == F32) {
      f32_count += 1;
    } else if (output_types[i] == BF16) {
      bf16_count += 1;
      if (!bfloat16_support_->SupportsBF16Output(*hlo)) {
        has_unsupported_bf16_output = true;
      }
    }
  }

  if (bf16_count == 0) {
    return Status::OK();
  }

  auto should_convert_operand = [&](int64 i) {
    if (operand_types[i] != BF16) {
      return false;
    }
    if (!bfloat16_support_->SupportsBF16Operand(*hlo, i)) {
      return true;
    }
    if (bfloat16_support_->SupportsMixedPrecisions(*hlo)) {
      return false;
    }
    return has_unsupported_bf16_operand || has_unsupported_bf16_output ||
           f32_count > 0;
  };

  for (int64 i = 0; i < hlo->operand_count(); ++i) {
    if (should_convert_operand(i)) {
      TF_RETURN_IF_ERROR(InsertConvertBeforeOperand(hlo, i, F32, computation_));
      f32_count += 1;
      bf16_count -= 1;
    }
  }

  if (!has_unsupported_bf16_output &&
      (bfloat16_support_->SupportsMixedPrecisions(*hlo) || f32_count == 0 ||
       bf16_count == 0)) {
    return Status::OK();
  }

  std::vector<HloInstruction*> materialized_users = hlo->users();
  std::vector<HloInstruction*> output_elements(hlo->operand_count());
  auto original_shape = hlo->shape();
  for (int64 i = 0; i < hlo->operand_count(); ++i) {
    auto subshape = ShapeUtil::GetMutableSubshape(hlo->mutable_shape(), {i});
    if (output_types[i] != BF16) {
      output_elements[i] = computation_->AddInstruction(
          HloInstruction::CreateGetTupleElement(*subshape, hlo, i));
      continue;
    }
    subshape->set_element_type(F32);
    auto gte = computation_->AddInstruction(
        HloInstruction::CreateGetTupleElement(*subshape, hlo, i));
    output_elements[i] =
        computation_->AddInstruction(HloInstruction::CreateConvert(
            ShapeUtil::ChangeElementType(*subshape, BF16), gte));
  }
  auto tuple = computation_->AddInstruction(
      HloInstruction::CreateTuple(output_elements));

  // Use the hlo' shape temporarily, in order to pass checks in
  // ReplaceUseWith.
  *tuple->mutable_shape() = hlo->shape();
  for (auto* user : materialized_users) {
    TF_RETURN_IF_ERROR(hlo->ReplaceUseWith(user, tuple));
  }
  bool is_root = computation_->root_instruction() == hlo;
  if (is_root) {
    computation_->set_root_instruction(tuple);
  }
  *tuple->mutable_shape() = original_shape;
  return Status::OK();
}

Status BFloat16NormalizationVisitor::HandleInstruction(HloInstruction* hlo) {
  int f32_count = 0;
  int bf16_count = 1;

  for (int64 i = 0; i < hlo->operand_count(); ++i) {
    if (hlo->operand(i)->shape().element_type() == F32) {
      f32_count += 1;
    } else if (hlo->operand(i)->shape().element_type() == BF16) {
      bf16_count += 1;
    }
  }

  if (hlo->shape().element_type() == F32) {
    f32_count += 1;
  } else if (hlo->shape().element_type() == BF16) {
    bf16_count += 1;
  }

  std::vector<HloComputation*> bf16_called_comps;
  for (auto* comp : hlo->called_computations()) {
    bool comp_has_bf16 = false;
    if (comp->root_instruction()->shape().element_type() == F32) {
      f32_count += 1;
    } else if (comp->root_instruction()->shape().element_type() == BF16) {
      bf16_count += 1;
      comp_has_bf16 = true;
    }
    for (auto* param : comp->parameter_instructions()) {
      if (param->shape().element_type() == F32) {
        f32_count += 1;
      } else if (param->shape().element_type() == BF16) {
        bf16_count += 1;
        comp_has_bf16 = true;
      }
    }
    if (comp_has_bf16) {
      bf16_called_comps.push_back(comp);
    }
  }

  // Resolve unsupported BF16 operands.
  for (int i = 0; i < hlo->operand_count(); ++i) {
    if (hlo->operand(i)->shape().element_type() == BF16 &&
        !bfloat16_support_->SupportsBF16Operand(*hlo, i)) {
      TF_RETURN_IF_ERROR(InsertConvertBeforeOperand(hlo, i, F32, computation_));
      bf16_count -= 1;
      f32_count += 1;
    }
  }

  // Resolve unsupported BF16 output.
  if (hlo->shape().element_type() == BF16 &&
      !bfloat16_support_->SupportsBF16Output(*hlo)) {
    TF_RETURN_IF_ERROR(
        ChangeOutputTypeThenInsertConvertBack(hlo, F32, computation_));
    bf16_count -= 1;
    f32_count += 1;
  }

  // Resolve unsupported mixed precision after resolving unsupported BF16
  // operands and output, because the numbers of BF16 operands/output and F32
  // operands/output may have changed.
  if (bfloat16_support_->SupportsMixedPrecisions(*hlo) || bf16_count == 0 ||
      f32_count == 0) {
    return Status::OK();
  }
  // See if we can change everything to BF16.
  if (hlo->called_computations().empty() &&
      hlo->shape().element_type() == BF16) {
    bool can_use_bf16 = true;
    for (int i = 0; i < hlo->operand_count(); ++i) {
      if (hlo->operand(i)->shape().element_type() == BF16) {
        continue;
      }
      if ((bfloat16_support_->EffectiveOperandPrecisionIsBF16(*hlo, i) ||
           bfloat16_support_->EffectiveOperandPrecisionIsOutputPrecision(*hlo,
                                                                         i)) &&
          bfloat16_support_->SupportsBF16Operand(*hlo, i)) {
        continue;
      }
      can_use_bf16 = false;
      break;
    }
    if (can_use_bf16) {
      for (int i = 0; i < hlo->operand_count(); ++i) {
        if (hlo->operand(i)->shape().element_type() == F32) {
          TF_RETURN_IF_ERROR(
              InsertConvertBeforeOperand(hlo, i, BF16, computation_));
        }
      }
      return Status::OK();
    }
  }
  if (hlo->shape().element_type() == BF16) {
    TF_RETURN_IF_ERROR(
        ChangeOutputTypeThenInsertConvertBack(hlo, F32, computation_));
  }
  for (int i = 0; i < hlo->operand_count(); ++i) {
    if (hlo->operand(i)->shape().element_type() == BF16) {
      TF_RETURN_IF_ERROR(InsertConvertBeforeOperand(hlo, i, F32, computation_));
    }
  }
  return ConvertCalledComputations(hlo, bf16_called_comps);
}

Status BFloat16NormalizationVisitor::DefaultAction(HloInstruction* hlo) {
  // Do not change instructions related to entry and exit of a computation,
  // tuples, fusion, convert, side-effecting instructions, and control flow.
  if (hlo->opcode() == HloOpcode::kTuple ||            //
      hlo->opcode() == HloOpcode::kGetTupleElement ||  //
      hlo->opcode() == HloOpcode::kConstant ||         //
      hlo->opcode() == HloOpcode::kParameter ||        //
      hlo->opcode() == HloOpcode::kFusion ||           //
      hlo->opcode() == HloOpcode::kConvert ||          //
      hlo->opcode() == HloOpcode::kCall ||             //
      hlo->opcode() == HloOpcode::kCustomCall ||       //
      hlo->opcode() == HloOpcode::kWhile ||            //
      hlo->opcode() == HloOpcode::kConditional ||      //
      hlo->HasSideEffectNoRecurse()) {
    return Status::OK();
  }
  // TODO(b/112040122): Correctly normalize variadic reduce.
  if ((hlo->opcode() == HloOpcode::kSort ||
       hlo->opcode() == HloOpcode::kCrossReplicaSum) &&
      ShapeUtil::IsTuple(hlo->shape())) {
    return HandleMultipleOutputs(hlo);
  }
  return HandleInstruction(hlo);
}

StatusOr<bool> BFloat16Normalization::Run(HloModule* module) {
  XLA_VLOG_LINES(
      2, "BFloat16Normalization::Run(), before:\n" + module->ToString());
  bool changed = false;
  for (auto* comp : module->MakeComputationPostOrder()) {
    if (BFloat16NormalizationVisitor::Run(comp, bfloat16_support_)) {
      changed = true;
    }
  }
  XLA_VLOG_LINES(2,
                 "BFloat16Normalization::Run(), after:\n" + module->ToString());
  return changed;
}

}  // namespace xla
