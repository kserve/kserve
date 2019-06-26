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

#include "tensorflow/compiler/xla/service/hlo_matchers.h"

#include "absl/strings/str_join.h"
#include "tensorflow/compiler/xla/service/hlo_instruction.h"
#include "tensorflow/compiler/xla/test.h"

namespace xla {
namespace testing {

bool HloMatcher::MatchAndExplain(
    const HloInstruction* instruction,
    ::testing::MatchResultListener* listener) const {
  // These cases are self-explanatory from the printed value.
  if (!instruction || instruction->opcode() != opcode_) {
    return false;
  }
  // Special case: no operand matchers means don't verify.
  if (operands_.empty()) {
    return true;
  }
  const auto& operands = instruction->operands();
  if (operands.size() != operands_.size()) {
    *listener << "has too "
              << (operands.size() > operands_.size() ? "many" : "few")
              << " operands (got " << operands.size() << ", want "
              << operands_.size() << ")";
    return false;
  }
  for (int index = 0; index < operands.size(); index++) {
    ::testing::StringMatchResultListener inner_listener;
    if (!operands_[index].MatchAndExplain(operands[index], &inner_listener)) {
      if (listener->IsInterested()) {
        *listener << "\noperand " << index << ":\n\t"
                  << operands[index]->ToString()
                  << "\ndoesn't match expected:\n\t";
        operands_[index].DescribeTo(listener->stream());
        string explanation = inner_listener.str();
        if (!explanation.empty()) {
          *listener << ", " << explanation;
        }
      }
      return false;
    }
  }
  return true;
}

void HloMatcher::DescribeTo(::std::ostream* os) const {
  *os << opcode_;
  if (!operands_.empty()) {
    *os << "(";
    for (int i = 0; i < operands_.size(); i++) {
      if (i > 0) {
        *os << ", ";
      }
      operands_[i].DescribeTo(os);
    }
    *os << ")";
  }
}

bool HloParameterMatcher::MatchAndExplain(
    const HloInstruction* instruction,
    ::testing::MatchResultListener* listener) const {
  if (!HloMatcher::MatchAndExplain(instruction, listener)) {
    return false;
  }
  if (instruction->parameter_number() != parameter_number_) {
    *listener << "has wrong parameter number (got "
              << instruction->parameter_number() << ", want "
              << parameter_number_ << ")";
    return false;
  }
  return true;
}

bool HloGetTupleElementMatcher::MatchAndExplain(
    const HloInstruction* instruction,
    ::testing::MatchResultListener* listener) const {
  if (!HloMatcher::MatchAndExplain(instruction, listener)) {
    return false;
  }
  if (instruction->tuple_index() != tuple_index_) {
    *listener << "has wrong tuple index (got " << instruction->tuple_index()
              << ", want " << tuple_index_ << ")";
    return false;
  }
  return true;
}

void HloCustomCallMatcher::DescribeTo(std::ostream* os) const {
  HloMatcher::DescribeTo(os);
  *os << " with call target that ";
  call_target_matcher_.DescribeTo(os);
}

bool HloCustomCallMatcher::MatchAndExplain(
    const HloInstruction* instruction,
    ::testing::MatchResultListener* listener) const {
  if (!HloMatcher::MatchAndExplain(instruction, listener)) {
    return false;
  }
  ::testing::StringMatchResultListener sub_listener;
  bool result = ExplainMatchResult(
      call_target_matcher_, instruction->custom_call_target(), &sub_listener);
  if (sub_listener.str().empty()) {
    sub_listener << " that ";

    std::stringstream desc_stream;
    if (result) {
      call_target_matcher_.DescribeTo(&desc_stream);
    } else {
      call_target_matcher_.DescribeNegationTo(&desc_stream);
    }
    sub_listener << desc_stream.str();
  }
  *listener << "custom-call with call target" << sub_listener.str();
  return result;
}

bool HloShapeMatcher::MatchAndExplain(
    const HloInstruction* instruction,
    ::testing::MatchResultListener* listener) const {
  if (ShapeUtil::Compatible(instruction->shape(), shape_)) {
    return true;
  }
  *listener << instruction->ToString() << " has incorrect shape (expected: "
            << ShapeUtil::HumanString(shape_) << ")";
  return false;
}

void HloShapeMatcher::DescribeTo(std::ostream* os) const {
  *os << ShapeUtil::HumanString(shape_);
}

bool HloShapeAndLayoutMatcher::MatchAndExplain(
    const HloInstruction* instruction,
    ::testing::MatchResultListener* listener) const {
  if (ShapeUtil::Equal(instruction->shape(), shape_)) {
    return true;
  }
  *listener << instruction->ToString() << " has incorrect shape (expected: "
            << ShapeUtil::HumanStringWithLayout(shape_) << ")";
  return false;
}

void HloShapeAndLayoutMatcher::DescribeTo(std::ostream* os) const {
  *os << ShapeUtil::HumanStringWithLayout(shape_);
}

bool HloShardingMatcher::MatchAndExplain(
    const HloInstruction* instruction,
    ::testing::MatchResultListener* listener) const {
  if (!sharding_.has_value()) {
    if (!instruction->has_sharding()) {
      return true;
    }
    *listener << instruction->ToString() << " expected to have no sharding.";
    return false;
  }
  if (instruction->has_sharding()) {
    if (instruction->sharding() == sharding_.value()) {
      return true;
    }
    *listener << instruction->ToString()
              << " has incorrect sharding (expected: " << sharding_->ToString()
              << ")";
    return false;
  } else {
    *listener << instruction->ToString()
              << " has no sharding (expected: " << sharding_->ToString() << ")";
    return false;
  }
}

void HloShardingMatcher::DescribeTo(std::ostream* os) const {
  if (sharding_.has_value()) {
    *os << sharding_->ToString();
  } else {
    *os << "<no-sharding>";
  }
}

bool HloDotWithContractingDimsMatcher::MatchAndExplain(
    const HloInstruction* instruction,
    ::testing::MatchResultListener* listener) const {
  if (!HloMatcher::MatchAndExplain(instruction, listener)) {
    return false;
  }

  const DotDimensionNumbers& dim_nums = instruction->dot_dimension_numbers();
  if (dim_nums.lhs_contracting_dimensions_size() != 1 ||
      dim_nums.lhs_contracting_dimensions(0) != lhs_contracting_dim_) {
    *listener << instruction->ToString()
              << " has wrong lhs_contracting_dimensions (got {"
              << absl::StrJoin(dim_nums.lhs_contracting_dimensions(), ",")
              << "} want {" << lhs_contracting_dim_ << "})";
    return false;
  }

  if (dim_nums.rhs_contracting_dimensions_size() != 1 ||
      dim_nums.rhs_contracting_dimensions(0) != rhs_contracting_dim_) {
    *listener << instruction->ToString()
              << " has wrong rhs_contracting_dimensions (got {"
              << absl::StrJoin(dim_nums.rhs_contracting_dimensions(), ",")
              << "} want {" << rhs_contracting_dim_ << "})";
    return false;
  }

  return true;
}

void HloDotWithContractingDimsMatcher::DescribeTo(std::ostream* os) const {
  HloMatcher::DescribeTo(os);
  *os << " with lhs_contracting_dims={" << lhs_contracting_dim_
      << "} and rhs_contracting_dims={" << rhs_contracting_dim_ << "}";
}

}  // namespace testing

void PrintTo(const HloInstruction* inst, ::std::ostream* os) {
  *os << (inst ? inst->ToString() : "nullptr");
}

}  // namespace xla
