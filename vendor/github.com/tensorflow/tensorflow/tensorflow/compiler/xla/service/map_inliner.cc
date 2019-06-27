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

#include "tensorflow/compiler/xla/service/map_inliner.h"

#include <memory>
#include <string>

#include "absl/types/span.h"
#include "tensorflow/compiler/xla/service/dfs_hlo_visitor_with_default.h"
#include "tensorflow/compiler/xla/service/hlo_computation.h"
#include "tensorflow/compiler/xla/service/hlo_instruction.h"
#include "tensorflow/compiler/xla/service/hlo_opcode.h"
#include "tensorflow/compiler/xla/service/hlo_query.h"
#include "tensorflow/compiler/xla/status_macros.h"
#include "tensorflow/compiler/xla/types.h"
#include "tensorflow/core/lib/core/errors.h"
#include "tensorflow/core/lib/core/status.h"
#include "tensorflow/core/platform/logging.h"

namespace xla {

// MapInlinerVisitor traverses the HLO computation and inlines maps.
class MapInlinerVisitor : public DfsHloVisitorWithDefault {
 public:
  explicit MapInlinerVisitor(HloComputation* computation)
      : computation_(computation) {}

  // Default visitor action is to do nothing and return OK.
  Status DefaultAction(HloInstruction* /*hlo_instruction*/) override {
    return Status::OK();
  }

  Status HandleMap(HloInstruction* map) override;

  // Runs the visitor on a computation.
  StatusOr<bool> Run(HloComputation* computation);

 private:
  // Current HloComputation instance the MapInlinerVisitor is traversing.
  HloComputation* computation_;

  // Whether algebraic simplification has occurred.
  bool changed_ = false;
};

StatusOr<bool> MapInlinerVisitor::Run(HloComputation* computation) {
  changed_ = false;
  computation_ = computation;
  TF_RETURN_IF_ERROR(computation->root_instruction()->Accept(this));
  return changed_;
}

Status MapInlinerVisitor::HandleMap(HloInstruction* map) {
  HloComputation* function = map->to_apply();
  HloInstruction& root = *function->root_instruction();
  // Only inlining functions that are simply a single operation until a better
  // profitability model for inlining is defined.
  if (hlo_query::AllOperandsAreParameters(root)) {
    if (root.opcode() == HloOpcode::kFusion ||
        root.opcode() == HloOpcode::kTrace) {
      // Cloning not supported for these instructions.
      return Status::OK();
    }
    VLOG(10) << "inlining map({X ... Y}, op) => : op(X ... Y) with function "
             << root.ToShortString();
    if (root.opcode() == HloOpcode::kParameter) {
      // If the root is a parameter, then use the corresponding operand as the
      // result of the computation.
      TF_RETURN_IF_ERROR(
          map->ReplaceAllUsesWith(map->operands()[root.parameter_number()]));
      TF_RETURN_IF_ERROR(computation_->RemoveInstruction(map));
    } else if (root.opcode() == HloOpcode::kConstant) {
      // If the input is a constant then the shape of the constant could be
      // different than the map shape. Hence, a broadcast is needed, else the
      // cloned operand with new shape and operands work.
      //
      // The constant is in an embedded computation and needs to be recreated
      // as part of the computation that the broadcast is inserted into.
      HloInstruction* constant = computation_->AddInstruction(root.Clone());
      HloInstruction* placed_instruction = computation_->AddInstruction(
          HloInstruction::CreateBroadcast(map->shape(), constant, {}));
      TF_RETURN_IF_ERROR(
          computation_->ReplaceInstruction(map, placed_instruction));
    } else {
      std::vector<HloInstruction*> params;
      for (int64 o = 0; o < root.operands().size(); o++) {
        params.push_back(map->operands()[root.operand(o)->parameter_number()]);
      }
      HloInstruction* placed_instruction = computation_->AddInstruction(
          root.CloneWithNewOperands(map->shape(), params));
      TF_RETURN_IF_ERROR(
          computation_->ReplaceInstruction(map, placed_instruction));
    }
    changed_ = true;
    return Status::OK();
  }

  return Status::OK();
}

StatusOr<bool> MapInliner::Run(HloModule* module) {
  MapInlinerVisitor visitor(/*computation=*/nullptr);
  bool changed = false;
  for (HloComputation* computation : module->computations()) {
    TF_ASSIGN_OR_RETURN(bool computation_changed, visitor.Run(computation));
    changed |= computation_changed;
  }
  return changed;
}

}  // namespace xla
