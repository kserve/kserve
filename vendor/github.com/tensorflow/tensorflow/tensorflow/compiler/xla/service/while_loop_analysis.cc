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

#include "tensorflow/compiler/xla/service/while_loop_analysis.h"
#include "tensorflow/compiler/xla/service/hlo_evaluator.h"
#include "tensorflow/compiler/xla/service/hlo_instruction.h"
#include "tensorflow/compiler/xla/service/hlo_module.h"
#include "tensorflow/compiler/xla/service/hlo_opcode.h"

namespace xla {

using absl::nullopt;
using absl::optional;

// Finds and returns the non-constant operand in instr.
//
// CHECK-fails if instr doesn't have exactly one unique non-constant operand.
static const HloInstruction* NonConstantOperand(const HloInstruction* instr) {
  const HloInstruction* result = nullptr;
  for (const HloInstruction* operand : instr->operands()) {
    if (!operand->IsConstant()) {
      if (result != nullptr) {
        CHECK_EQ(result, operand);
      }
      result = operand;
    }
  }
  CHECK_NE(result, nullptr);
  return result;
}

// If all of instr's operands are either constants or have the form
//   get-tuple-element(gte_operand, N)
// for the same value N, returns N.  Otherwise, returns nullopt.
static optional<int64> GetGTEOperandIndex(const HloInstruction* instr,
                                          const HloInstruction* gte_operand) {
  VLOG(2) << "GetGTEOperandIndex(" << instr->ToString() << ", "
          << gte_operand->ToString() << ")";
  optional<int64> tuple_idx;
  for (const HloInstruction* operand : instr->operands()) {
    if (operand->IsConstant()) {
      continue;
    }
    // Look through copies.
    // TODO(b/68830972): We wouldn't need this if for loop matching on the GPU
    // would run before copy insertion.
    if (operand->opcode() == HloOpcode::kCopy) {
      operand = operand->operand(0);
    }
    if (operand->opcode() != HloOpcode::kGetTupleElement) {
      VLOG(2) << "instr uses something other than gte(gte_operand): "
              << operand->ToString();
      return nullopt;
    }
    if (operand->operand(0) != gte_operand) {
      VLOG(2) << "instr has gte whose operand is not gte_operand: "
              << operand->ToString();
      return nullopt;
    }
    if (tuple_idx && tuple_idx != operand->tuple_index()) {
      VLOG(2) << "instr has operands with conflicting gte indices, "
              << *tuple_idx << " vs " << operand->tuple_index();
      return nullopt;
    }

    tuple_idx = operand->tuple_index();
  }
  return tuple_idx;
}

// Tries to get the tuple index of the induction variable of a while loop.
//
// Checks that the loop condition and root both plumb the induction variable
// through the same tuple index, and that they both apply exactly one op to the
// induction variable before  deciding whether to do another loop iteration (in
// the loop condition's case) or packing the induction variable into the result
// tuple (in the loop body's case).
//
// Specifically, checks that the loop condition has structure
//
//   root = op(constants, get-tuple-elem(param0, N), constants)
//
// and the loop body has the structure
//
//   inc = op(constants, get-tuple-elem(param0, N), constants)
//   root = tuple(..., inc, ...)  // inc is N'th operand of tuple().
//
// If so, returns N.  Otherwise, returns nullopt.
static optional<int64> GetLoopInductionVarTupleIdx(
    const HloInstruction* while_op) {
  CHECK_EQ(while_op->opcode(), HloOpcode::kWhile);
  VLOG(2) << "Finding induction variable for loop "
          << while_op->ToShortString();

  // The while_cond computation should have the form
  //
  //   while_cond_root =
  //       op(constants, get-tuple-elem(while_cond_param, N), constants).
  //
  // If it does, set indvar_tuple_idx to N.
  auto* while_cond = while_op->while_condition();
  auto* while_cond_root = while_cond->root_instruction();
  auto* while_cond_param = while_cond->parameter_instruction(0);
  optional<int64> indvar_tuple_idx =
      GetGTEOperandIndex(while_cond_root, while_cond_param);
  if (!indvar_tuple_idx) {
    VLOG(2) << "Induction variable not found in loop condition: "
            << while_cond->root_instruction()->ToString();
    return nullopt;
  }

  // The while_body computation should have the form
  //
  //   while_body_inc =
  //       op(constants, get-tuple-elem(while_body_param, N), constants)
  //   while_body_root = tuple(..., while_body_inc, ...)
  //
  // where while_body_inc is operand N of while_body_root.
  auto* while_body = while_op->while_body();
  auto* while_body_root = while_body->root_instruction();
  if (while_body_root->opcode() != HloOpcode::kTuple) {
    VLOG(2) << "While body's root is not a tuple instruction: "
            << while_body_root->ToString();
    return nullopt;
  }

  auto* while_body_inc = while_body_root->operand(*indvar_tuple_idx);
  auto* while_body_param = while_body->parameter_instruction(0);
  optional<int64> while_body_indvar_tuple_idx =
      GetGTEOperandIndex(while_body_inc, while_body_param);
  if (!while_body_indvar_tuple_idx) {
    VLOG(2)
        << "Induction variable not found in while body increment instruction: "
        << while_body_inc->ToString();
    return nullopt;
  }
  if (while_body_indvar_tuple_idx != indvar_tuple_idx) {
    VLOG(2) << "Tuple index of induction variable does not match between loop "
               "condition ("
            << *indvar_tuple_idx << ") and while body ("
            << *while_body_indvar_tuple_idx << ")";
    return nullopt;
  }

  // Finally, check that the while loop's initial value is a tuple with enough
  // elements.
  auto* while_init = while_op->operand(0);
  if (while_init->opcode() != HloOpcode::kTuple) {
    VLOG(2) << "While init expected to be a tuple: " << while_init->ToString();
    return nullopt;
  }

  VLOG(2) << "Induction variable's tuple index: " << *indvar_tuple_idx;
  return indvar_tuple_idx;
}

optional<int64> ComputeWhileLoopTripCount(HloInstruction* while_op,
                                          int64 max_value_returned) {
  VLOG(2) << "Getting trip count for loop " << while_op->ToString();

  // The loop's induction variable is found at
  //
  //   get-tuple-elem(comp->parameter_instruction(0), *indvar_tuple_idx),
  //
  // where comp is while_op->while_body() or while_op->while_condition().
  optional<int64> indvar_tuple_idx = GetLoopInductionVarTupleIdx(while_op);
  if (!indvar_tuple_idx) {
    return nullopt;
  }

  // Now that we know the index of the induction variable, we can we can try to
  // compute how many times the loop executes.  Start by computing the induction
  // variable's initial value.
  HloEvaluator evaluator(/*max_loop_iterations=*/0);
  auto* while_init = while_op->mutable_operand(0);
  auto* indvar_init = while_init->mutable_operand(*indvar_tuple_idx);
  StatusOr<Literal> indvar_init_result = evaluator.Evaluate(indvar_init);
  if (!indvar_init_result.ok()) {
    VLOG(2) << "Couldn't evaluate induction variable init: "
            << indvar_init_result.status();
    return nullopt;
  }

  auto* while_body = while_op->while_body();
  auto* while_body_indvar_update =
      while_body->root_instruction()->operand(*indvar_tuple_idx);
  auto* while_body_indvar = NonConstantOperand(while_body_indvar_update);

  // The initial value of the induction variable.
  Literal indvar_iter_val = std::move(indvar_init_result).ValueOrDie();
  for (int64 trip_count = 0; trip_count != max_value_returned + 1;
       ++trip_count) {
    auto* while_cond = while_op->while_condition();
    auto* while_cond_root = while_cond->root_instruction();
    auto* while_cond_indvar = NonConstantOperand(while_cond_root);
    StatusOr<Literal> result = evaluator.EvaluateWithSubstitutions(
        while_cond_root, {{while_cond_indvar, &indvar_iter_val}});
    if (!result.ok()) {
      VLOG(2) << "Couldn't evaluate while cond: " << result.status();
      return nullopt;
    }
    if (result.ValueOrDie().data<bool>() == absl::Span<const bool>{false}) {
      VLOG(2) << "Loop has static trip count of " << trip_count;
      return trip_count;
    }

    // Calculate the value of the induction variable after one iteration of the
    // loop, and check whether the while condition is true with this new value.
    StatusOr<Literal> indvar_next_result = evaluator.EvaluateWithSubstitutions(
        while_body_indvar_update, {{while_body_indvar, &indvar_iter_val}});
    if (!indvar_next_result.ok()) {
      VLOG(2) << "Couldn't evaluate induction variable update: "
              << indvar_next_result.status();
      return nullopt;
    }
    indvar_iter_val = std::move(indvar_next_result).ValueOrDie();
  }

  VLOG(2) << "Loop has unknown trip count.";
  return nullopt;
}

// If the only user of this instruction is a get-tuple-element, return that
// get-tuple-element, otherwise return null. If this runs before CSE/DCE, we may
// get a false negative if there are several copies of the same GTE, or there
// are unused GTEs, but we can live with this.
static HloInstruction* GetOnlyGTE(HloInstruction* inst) {
  if (inst->user_count() != 1) {
    return nullptr;
  }

  HloInstruction* user = inst->users().back();
  if (user->opcode() != HloOpcode::kGetTupleElement) {
    return nullptr;
  }
  return user;
}

optional<int64> ComputeWhileLoopTripCountUpperBound(HloInstruction* while_op) {
  // If we know the exact trip count, it's also the upper bound.
  auto exact_trip_count = ComputeWhileLoopTripCount(while_op);
  if (exact_trip_count) {
    VLOG(2) << "Loop has exact trip count.";
    return exact_trip_count;
  }

  // There is one more case we know how to handle. If the loop condition only
  // looks at one element of the tuple, and the loop body sets this element to a
  // constant, there are two options:
  // 1) Evaluating the condition on this constant returns true. In this case,
  // the loop either executes 0 times, or is an infinite loop, depending on the
  // init value.
  // 2) Evaluating the condition on this constant returns false. In this case,
  // the loop executes 0 or 1 times, depending on the init value. This means
  // that, regardless of the init value, the upper bound on the trip count is 1.

  // Check whether the condition depends on a single parameter, and find out
  // which.
  auto* while_cond = while_op->while_condition();
  auto* while_cond_param = while_cond->parameter_instruction(0);
  auto* cond_gte = GetOnlyGTE(while_cond_param);
  if (!cond_gte) {
    VLOG(2) << "Induction variable not found in loop condition: "
            << while_cond->root_instruction()->ToString();
    return nullopt;
  }

  // Now check whether this gets set to a constant by the while body.
  auto* while_body = while_op->while_body();
  auto* while_body_root = while_body->root_instruction();
  if (while_body_root->opcode() != HloOpcode::kTuple) {
    VLOG(3) << "While body's root is not a tuple instruction: "
            << while_body_root->ToString();
    return nullopt;
  }

  int64 indvar_index = cond_gte->tuple_index();
  auto* while_body_indvar = while_body_root->operand(indvar_index);
  if (while_body_indvar->opcode() != HloOpcode::kConstant) {
    VLOG(3) << "While body does not set the IV to a constant: "
            << while_body_indvar->ToString();
    return nullopt;
  }

  // We have a constant. Evaluate the condition on this constant.
  HloEvaluator evaluator(/*max_loop_iterations=*/0);
  Literal fake_input = Literal::CreateFromShape(while_cond_param->shape());
  TF_CHECK_OK(fake_input.CopyFrom(while_body_indvar->literal(),
                                  /*dest_shape_index=*/{indvar_index},
                                  /*src_shape_index=*/{}));
  StatusOr<Literal> eval_result =
      evaluator.Evaluate<Literal>(*while_cond, {std::move(fake_input)});

  if (!eval_result.ok()) {
    VLOG(2) << "Couldn't evaluate while loop condition.";
    return nullopt;
  }

  Literal cond_result_pred = std::move(eval_result.ValueOrDie());
  CHECK(ShapeUtil::Equal(cond_result_pred.shape(),
                         ShapeUtil::MakeShape(PRED, {})));

  // Per the explanation above, if the evaluated condition returns false, the
  // loop executes at most once.
  bool cond_returns_true = cond_result_pred.GetFirstElement<bool>();
  if (!cond_returns_true) {
    VLOG(2) << "Upper bound on the trip count is 1";
    return 1;
  }

  VLOG(2) << "Loop has no known upper bound on the trip count.";
  return nullopt;
}

}  // namespace xla
