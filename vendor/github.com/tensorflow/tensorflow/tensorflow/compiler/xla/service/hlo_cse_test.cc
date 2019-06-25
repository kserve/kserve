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

#include "tensorflow/compiler/xla/service/hlo_cse.h"

#include <memory>
#include <string>
#include <utility>
#include <vector>

#include "absl/memory/memory.h"
#include "tensorflow/compiler/xla/layout_util.h"
#include "tensorflow/compiler/xla/literal.h"
#include "tensorflow/compiler/xla/service/hlo_computation.h"
#include "tensorflow/compiler/xla/service/hlo_instruction.h"
#include "tensorflow/compiler/xla/service/hlo_matchers.h"
#include "tensorflow/compiler/xla/service/hlo_module.h"
#include "tensorflow/compiler/xla/service/hlo_opcode.h"
#include "tensorflow/compiler/xla/shape_util.h"
#include "tensorflow/compiler/xla/tests/hlo_test_base.h"
#include "tensorflow/compiler/xla/tests/literal_test_util.h"
#include "tensorflow/compiler/xla/tests/test_utils.h"
#include "tensorflow/compiler/xla/util.h"
#include "tensorflow/compiler/xla/xla_data.pb.h"

#include "tensorflow/compiler/xla/service/hlo_parser.h"
#include "tensorflow/compiler/xla/types.h"
#include "tensorflow/core/platform/types.h"

namespace op = xla::testing::opcode_matchers;

namespace xla {
namespace {

class HloCseTest : public HloTestBase {
 protected:
  HloCseTest() {}
};

TEST_F(HloCseTest, CombineTwoConstants) {
  // Test that two identical constants are commoned.
  auto builder = HloComputation::Builder(TestName());
  auto constant1 = builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR0<float>(42.0f)));
  auto constant2 = builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR0<float>(42.0f)));
  builder.AddInstruction(HloInstruction::CreateBinary(
      constant1->shape(), HloOpcode::kAdd, constant1, constant2));

  auto module = CreateNewVerifiedModule();
  auto computation = module->AddEntryComputation(builder.Build());

  EXPECT_EQ(3, computation->instruction_count());

  HloCSE cse(/*is_layout_sensitive=*/false);
  EXPECT_TRUE(cse.Run(module.get()).ValueOrDie());

  EXPECT_EQ(2, computation->instruction_count());
  HloInstruction* constant = *computation->instructions().begin();
  EXPECT_EQ(42.0f, constant->literal().Get<float>({}));

  auto result = ExecuteAndTransfer(module->Clone(), {});
  auto expected = LiteralUtil::CreateR0<float>(84.0);
  EXPECT_TRUE(LiteralTestUtil::Near(expected, result, ErrorSpec(1e-4)));
}

TEST_F(HloCseTest, CombineTwoConstantsDifferentLayoutsAndInsensitive) {
  // Test that two identical constants with different layouts are commoned if
  // the pass is not layout sensitive.
  auto builder = HloComputation::Builder(TestName());
  auto constant1 = builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR2WithLayout<float>(
          {{1.0, 2.0}, {3.0, 4.0}}, LayoutUtil::MakeLayout({0, 1}))));
  auto constant2 = builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR2WithLayout<float>(
          {{1.0, 2.0}, {3.0, 4.0}}, LayoutUtil::MakeLayout({1, 0}))));
  auto add = builder.AddInstruction(HloInstruction::CreateBinary(
      constant1->shape(), HloOpcode::kAdd, constant1, constant2));

  auto module = CreateNewVerifiedModule();
  auto computation = module->AddEntryComputation(builder.Build());

  EXPECT_EQ(3, computation->instruction_count());
  EXPECT_THAT(add, op::Add(constant1, constant2));

  HloCSE cse(/*is_layout_sensitive=*/false);
  EXPECT_TRUE(cse.Run(module.get()).ValueOrDie());

  EXPECT_EQ(2, computation->instruction_count());
  auto first_operand = add->operand(0);
  EXPECT_THAT(first_operand, ::testing::AnyOf(constant1, constant2));
  EXPECT_THAT(add, op::Add(first_operand, first_operand));

  auto result = ExecuteAndTransfer(module->Clone(), {});
  auto expected = LiteralUtil::CreateR2<float>({{2.0, 4.0}, {6.0, 8.0}});
  EXPECT_TRUE(LiteralTestUtil::Near(expected, result, ErrorSpec(1e-4)));
}

TEST_F(HloCseTest, CombineTwoConstantsDifferentLayoutsAndSensitive) {
  // Test that two identical constants with different layouts are *not* commoned
  // if the pass is layout sensitive.
  auto builder = HloComputation::Builder(TestName());
  auto constant1 = builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR2WithLayout<float>(
          {{1.0, 2.0}, {3.0, 4.0}}, LayoutUtil::MakeLayout({0, 1}))));
  auto constant2 = builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR2WithLayout<float>(
          {{1.0, 2.0}, {3.0, 4.0}}, LayoutUtil::MakeLayout({1, 0}))));
  auto add = builder.AddInstruction(HloInstruction::CreateBinary(
      constant1->shape(), HloOpcode::kAdd, constant1, constant2));

  auto module = CreateNewVerifiedModule();
  auto computation = module->AddEntryComputation(builder.Build());

  EXPECT_EQ(3, computation->instruction_count());
  EXPECT_THAT(add, op::Add(constant1, constant2));

  HloCSE cse(/*is_layout_sensitive=*/true);
  EXPECT_FALSE(cse.Run(module.get()).ValueOrDie());

  EXPECT_EQ(3, computation->instruction_count());
  EXPECT_THAT(add, op::Add(constant1, constant2));

  auto result = ExecuteAndTransfer(module->Clone(), {});
  auto expected = LiteralUtil::CreateR2<float>({{2.0, 4.0}, {6.0, 8.0}});
  EXPECT_TRUE(LiteralTestUtil::Near(expected, result, ErrorSpec(1e-4)));
}

TEST_F(HloCseTest, ConstantsSameValueDifferentType) {
  // Test that constants with the same value but different type are *not*
  // commoned.
  auto builder = HloComputation::Builder(TestName());
  std::vector<HloInstruction*> constants;
  constants.push_back(builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR0<uint32>(42))));
  constants.push_back(builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR0<int32>(42))));
  constants.push_back(builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR0<uint64>(42.0))));
  constants.push_back(builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR0<int64>(42.0))));
  constants.push_back(builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR0<double>(42.0))));
  constants.push_back(builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR0<float>(42.0f))));
  // Duplicate the float constant to verify something happens.
  constants.push_back(builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR0<float>(42.0f))));

  const Shape shape_r0 = ShapeUtil::MakeShape(F32, {});
  for (int64 i = 0; i < constants.size(); ++i) {
    constants[i] = builder.AddInstruction(
        HloInstruction::CreateConvert(shape_r0, constants[i]));
  }
  HloInstruction* root = builder.AddInstruction(HloInstruction::CreateBinary(
      shape_r0, HloOpcode::kAdd, constants[0], constants[1]));
  for (int64 i = 2; i < constants.size(); ++i) {
    root = builder.AddInstruction(HloInstruction::CreateBinary(
        shape_r0, HloOpcode::kAdd, root, constants[i]));
  }

  auto module = CreateNewVerifiedModule();
  auto computation = module->AddEntryComputation(builder.Build());

  EXPECT_EQ(20, computation->instruction_count());

  HloCSE cse(/*is_layout_sensitive=*/false);
  EXPECT_TRUE(cse.Run(module.get()).ValueOrDie());

  // CSE will remove both the second float(42.0f) and the corresponding
  // convert/cast.
  EXPECT_EQ(18, computation->instruction_count());
}

TEST_F(HloCseTest, NonscalarConstants) {
  // Test that identical nonscalar constants are merged.
  auto builder = HloComputation::Builder(TestName());
  auto common_constant1 = builder.AddInstruction(HloInstruction::CreateConstant(
      LiteralUtil::CreateR2<float>({{1.0, 2.0}, {3.0, 4.0}})));
  auto common_constant2 = builder.AddInstruction(HloInstruction::CreateConstant(
      LiteralUtil::CreateR2<float>({{1.0, 2.0}, {3.0, 4.0}})));
  // Create a constant which has the same shape but a different value.
  auto uncommon_constant =
      builder.AddInstruction(HloInstruction::CreateConstant(
          LiteralUtil::CreateR2<float>({{2.0, 4.0}, {6.0, 8.0}})));

  // Tie the constants together with a tuple. This makes it easier to refer to
  // the constant instructions via their use.
  auto tuple = builder.AddInstruction(HloInstruction::CreateTuple(
      {common_constant1, common_constant2, uncommon_constant}));

  auto module = CreateNewVerifiedModule();
  auto computation = module->AddEntryComputation(builder.Build());

  EXPECT_EQ(4, computation->instruction_count());
  EXPECT_THAT(tuple,
              op::Tuple(common_constant1, common_constant2, uncommon_constant));

  HloCSE cse(/*is_layout_sensitive=*/false);
  EXPECT_TRUE(cse.Run(module.get()).ValueOrDie());

  EXPECT_EQ(3, computation->instruction_count());
  auto first_operand = tuple->operand(0);
  EXPECT_THAT(first_operand,
              ::testing::AnyOf(common_constant1, common_constant2));
  EXPECT_THAT(tuple,
              op::Tuple(first_operand, first_operand, uncommon_constant));
}

TEST_F(HloCseTest, IdenticalInstructions) {
  // Test that three identical instructions are commoned.
  auto builder = HloComputation::Builder(TestName());
  auto constant = builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR0<float>(42.0)));
  auto exp1 = builder.AddInstruction(HloInstruction::CreateUnary(
      constant->shape(), HloOpcode::kExp, constant));
  auto exp2 = builder.AddInstruction(HloInstruction::CreateUnary(
      constant->shape(), HloOpcode::kExp, constant));
  auto exp3 = builder.AddInstruction(HloInstruction::CreateUnary(
      constant->shape(), HloOpcode::kExp, constant));
  auto tuple =
      builder.AddInstruction(HloInstruction::CreateTuple({exp1, exp2, exp3}));

  auto module = CreateNewVerifiedModule();
  auto computation = module->AddEntryComputation(builder.Build());

  EXPECT_EQ(5, computation->instruction_count());
  EXPECT_THAT(tuple, op::Tuple(exp1, exp2, exp3));

  HloCSE cse(/*is_layout_sensitive=*/true);
  EXPECT_TRUE(cse.Run(module.get()).ValueOrDie());

  EXPECT_EQ(3, computation->instruction_count());
  auto first_operand = tuple->operand(0);
  EXPECT_THAT(first_operand, ::testing::AnyOf(exp1, exp2, exp3));
  EXPECT_THAT(tuple, op::Tuple(first_operand, first_operand, first_operand));
}

// Test two identical while loops with same inputs
TEST_F(HloCseTest, WhileLoopsIdenticalConditionsAndBodiesSameInput) {
  const char* const hlo_string = R"(
    HloModule WhileLoopsIdenticalConditionsAndBodiesSameInput

    %body (param: (f32[], f32[])) -> (f32[], f32[]) {
      %param = (f32[], f32[]) parameter(0)
      %get-tuple-element = f32[] get-tuple-element((f32[], f32[]) %param),
index=0 %get-tuple-element.1 = f32[] get-tuple-element((f32[], f32[]) %param),
index=1 %add = f32[] add(f32[] %get-tuple-element, f32[] %get-tuple-element.1)
      ROOT %tuple = (f32[], f32[]) tuple(f32[] %get-tuple-element, f32[] %add)
    }

    %condition (param.1: (f32[], f32[])) -> pred[] {
      %param.1 = (f32[], f32[]) parameter(0)
      ROOT %constant = pred[] constant(false)
    }

    %condition.1 (param.2: (f32[], f32[])) -> pred[] {
      %param.2 = (f32[], f32[]) parameter(0)
      ROOT %constant.1 = pred[] constant(false)
    }

    ENTRY %WhileLoopsIdenticalConditionsAndBodiesSameInput () -> (f32[], f32[])
{ %constant.2 = f32[] constant(1) %constant.3 = f32[] constant(2) %tuple.1 =
(f32[], f32[]) tuple(f32[] %constant.2, f32[] %constant.3) %while = (f32[],
f32[]) while((f32[], f32[]) %tuple.1), condition=%condition, body=%body ROOT
%while.1 = (f32[], f32[]) while((f32[], f32[]) %tuple.1),
condition=%condition.1, body=%body
    })";

  TF_ASSERT_OK_AND_ASSIGN(auto m, ParseAndReturnVerifiedModule(hlo_string));
  auto computation = m->entry_computation();

  EXPECT_EQ(5, computation->instruction_count());
  HloCSE cse(true);
  EXPECT_TRUE(cse.Run(m.get()).ValueOrDie());
  EXPECT_EQ(4, computation->instruction_count());
}

// Test two while loops with same conditions, same inputs, but different
// bodies
TEST_F(HloCseTest, WhileLoopsIdenticalConditionsSameInputAndDifferentBodies) {
  const char* const hlo_string = R"(
    HloModule WhileLoopsIdenticalConditionsSameInputAndDifferentBodies

    %body (param: (f32[], f32[])) -> (f32[], f32[]) {
      %param = (f32[], f32[]) parameter(0)
      %get-tuple-element = f32[] get-tuple-element((f32[], f32[]) %param),
index=0 %get-tuple-element.1 = f32[] get-tuple-element((f32[], f32[]) %param),
index=1 %add = f32[] add(f32[] %get-tuple-element, f32[] %get-tuple-element.1)
      ROOT %tuple = (f32[], f32[]) tuple(f32[] %get-tuple-element, f32[] %add)
    }

    %body2 (param.1: (f32[], f32[])) -> (f32[], f32[]) {
      %param.1 = (f32[], f32[]) parameter(0)
      %get-tuple-element.2 = f32[] get-tuple-element((f32[], f32[]) %param.1),
index=0 %get-tuple-element.3 = f32[] get-tuple-element((f32[], f32[]) %param.1),
index=1 %sub = f32[] subtract(f32[] %get-tuple-element.2, f32[]
%get-tuple-element.3) ROOT %tuple.2 = (f32[], f32[]) tuple(f32[]
%get-tuple-element.2, f32[] %sub)
    }

    %condition (param.2: (f32[], f32[])) -> pred[] {
      %param.2 = (f32[], f32[]) parameter(0)
      ROOT %constant = pred[] constant(false)
    }

    %condition.1 (param.3: (f32[], f32[])) -> pred[] {
      %param.3 = (f32[], f32[]) parameter(0)
      ROOT %constant.1 = pred[] constant(false)
    }

    ENTRY %WhileLoopsIdenticalConditionsSameInputAndDifferentBodies () ->
(f32[], f32[]) { %constant.2 = f32[] constant(1) %constant.3 = f32[] constant(2)
      %tuple.1 = (f32[], f32[]) tuple(f32[] %constant.2, f32[] %constant.3)
      %while = (f32[], f32[]) while((f32[], f32[]) %tuple.1),
condition=%condition, body=%body ROOT %while.1 = (f32[], f32[]) while((f32[],
f32[]) %tuple.1), condition=%condition.1, body=%body2
    })";

  TF_ASSERT_OK_AND_ASSIGN(auto m, ParseAndReturnVerifiedModule(hlo_string));
  auto computation = m->entry_computation();

  EXPECT_EQ(5, computation->instruction_count());
  HloCSE cse(true);
  EXPECT_FALSE(cse.Run(m.get()).ValueOrDie());
  EXPECT_EQ(5, computation->instruction_count());
}

// Test two identical while loops with different inputs
TEST_F(HloCseTest, WhileLoopsIdenticalConditionsAndBodiesDifferentInput) {
  const char* const hlo_string = R"(
    HloModule WhileLoopsIdenticalConditionsAndBodiesDifferentInput

    %body (param: (f32[], f32[])) -> (f32[], f32[]) {
      %param = (f32[], f32[]) parameter(0)
      %get-tuple-element = f32[] get-tuple-element((f32[], f32[]) %param),
index=0 %get-tuple-element.1 = f32[] get-tuple-element((f32[], f32[]) %param),
index=1 %add = f32[] add(f32[] %get-tuple-element, f32[] %get-tuple-element.1)
      ROOT %tuple = (f32[], f32[]) tuple(f32[] %get-tuple-element, f32[] %add)
    }

    %condition (param.1: (f32[], f32[])) -> pred[] {
      %param.1 = (f32[], f32[]) parameter(0)
      ROOT %constant = pred[] constant(false)
    }

    %condition.1 (param.2: (f32[], f32[])) -> pred[] {
      %param.2 = (f32[], f32[]) parameter(0)
      ROOT %constant.1 = pred[] constant(false)
    }

    ENTRY %WhileLoopsIdenticalConditionsAndBodiesDifferentInput () -> (f32[],
f32[]) { %constant.2 = f32[] constant(1) %constant.3 = f32[] constant(2)
      %tuple.1 = (f32[], f32[]) tuple(f32[] %constant.2, f32[] %constant.3)
      %while = (f32[], f32[]) while((f32[], f32[]) %tuple.1),
condition=%condition, body=%body %constant.4 = f32[] constant(1) %constant.5 =
f32[] constant(2) %tuple.2 = (f32[], f32[]) tuple(f32[] %constant.4, f32[]
%constant.5) ROOT %while.1 = (f32[], f32[]) while((f32[], f32[]) %tuple.2),
condition=%condition.1, body=%body
    })";

  TF_ASSERT_OK_AND_ASSIGN(auto m, ParseAndReturnVerifiedModule(hlo_string));
  auto computation = m->entry_computation();

  EXPECT_EQ(8, computation->instruction_count());
  HloCSE cse(true);
  EXPECT_FALSE(cse.Run(m.get()).ValueOrDie());
  EXPECT_EQ(8, computation->instruction_count());
}

// Test two while loops with identical bodies and same inputs, but different
// conditions
TEST_F(HloCseTest, WhileLoopsIdenticalBodiesAndInputDifferntConditions) {
  const char* const hlo_string = R"(
    HloModule WhileLoopsIdenticalBodiesAndInputDifferntConditions

    %body (param: (f32[], f32[])) -> (f32[], f32[]) {
      %param = (f32[], f32[]) parameter(0)
      %get-tuple-element = f32[] get-tuple-element((f32[], f32[]) %param),
index=0 %get-tuple-element.1 = f32[] get-tuple-element((f32[], f32[]) %param),
index=1 %add = f32[] add(f32[] %get-tuple-element, f32[] %get-tuple-element.1)
      ROOT %tuple = (f32[], f32[]) tuple(f32[] %get-tuple-element, f32[] %add)
    }

    %condition (param.1: (f32[], f32[])) -> pred[] {
      %param.1 = (f32[], f32[]) parameter(0)
      ROOT %constant = pred[] constant(false)
    }

    %condition.1 (param.2: (f32[], f32[])) -> pred[] {
      %param.2 = (f32[], f32[]) parameter(0)
      ROOT %constant.1 = pred[] constant(true)
    }

    ENTRY %WhileLoopsIdenticalBodiesAndInputDifferntConditions () -> (f32[],
f32[]) { %constant.2 = f32[] constant(1) %constant.3 = f32[] constant(2)
      %tuple.1 = (f32[], f32[]) tuple(f32[] %constant.2, f32[] %constant.3)
      %while = (f32[], f32[]) while((f32[], f32[]) %tuple.1),
condition=%condition, body=%body ROOT %while.1 = (f32[], f32[]) while((f32[],
f32[]) %tuple.1), condition=%condition.1, body=%body
    })";

  TF_ASSERT_OK_AND_ASSIGN(auto m, ParseAndReturnVerifiedModule(hlo_string));
  auto computation = m->entry_computation();

  EXPECT_EQ(5, computation->instruction_count());
  HloCSE cse(true);
  EXPECT_FALSE(cse.Run(m.get()).ValueOrDie());
  EXPECT_EQ(5, computation->instruction_count());
}

TEST_F(HloCseTest, IdenticalInstructionsDifferentLayoutsSensitive) {
  // Test that two identical instructions with different layouts are *not*
  // commoned if the pass is layout sensitive.
  auto builder = HloComputation::Builder(TestName());
  auto constant = builder.AddInstruction(HloInstruction::CreateConstant(
      LiteralUtil::CreateR2<float>({{1.0, 2.0}, {3.0, 4.0}})));

  auto exp1 = builder.AddInstruction(HloInstruction::CreateUnary(
      constant->shape(), HloOpcode::kExp, constant));
  *exp1->mutable_shape()->mutable_layout() = LayoutUtil::MakeLayout({0, 1});

  auto exp2 = builder.AddInstruction(HloInstruction::CreateUnary(
      constant->shape(), HloOpcode::kExp, constant));
  *exp2->mutable_shape()->mutable_layout() = LayoutUtil::MakeLayout({1, 0});

  auto tuple =
      builder.AddInstruction(HloInstruction::CreateTuple({exp1, exp2}));

  auto module = CreateNewVerifiedModule();
  auto computation = module->AddEntryComputation(builder.Build());

  EXPECT_EQ(4, computation->instruction_count());
  EXPECT_THAT(tuple, op::Tuple(exp1, exp2));

  HloCSE cse(/*is_layout_sensitive=*/true);
  EXPECT_FALSE(cse.Run(module.get()).ValueOrDie());

  EXPECT_EQ(4, computation->instruction_count());
  EXPECT_THAT(tuple, op::Tuple(exp1, exp2));
}

TEST_F(HloCseTest, IdenticalInstructionsDifferentLayoutsInsensitive) {
  // Test that two identical instructions with different layouts are commoned if
  // the pass is layout insensitive.
  auto builder = HloComputation::Builder(TestName());
  auto constant = builder.AddInstruction(HloInstruction::CreateConstant(
      LiteralUtil::CreateR2<float>({{1.0, 2.0}, {3.0, 4.0}})));

  auto exp1 = builder.AddInstruction(HloInstruction::CreateUnary(
      constant->shape(), HloOpcode::kExp, constant));
  *exp1->mutable_shape()->mutable_layout() = LayoutUtil::MakeLayout({0, 1});

  auto exp2 = builder.AddInstruction(HloInstruction::CreateUnary(
      constant->shape(), HloOpcode::kExp, constant));
  *exp2->mutable_shape()->mutable_layout() = LayoutUtil::MakeLayout({1, 0});

  auto tuple =
      builder.AddInstruction(HloInstruction::CreateTuple({exp1, exp2}));

  auto module = CreateNewVerifiedModule();
  auto computation = module->AddEntryComputation(builder.Build());

  EXPECT_EQ(4, computation->instruction_count());
  EXPECT_THAT(tuple, op::Tuple(exp1, exp2));

  HloCSE cse(/*is_layout_sensitive=*/false);
  EXPECT_TRUE(cse.Run(module.get()).ValueOrDie());

  EXPECT_EQ(3, computation->instruction_count());
  auto first_operand = tuple->operand(0);
  EXPECT_THAT(first_operand, ::testing::AnyOf(exp1, exp2));
  EXPECT_THAT(tuple, op::Tuple(first_operand, first_operand));
}

TEST_F(HloCseTest, FusionInternalCSE) {
  // Test that we can CSE expressions that live within a fusion node
  // computation.
  auto module = CreateNewVerifiedModule();
  auto builder = HloComputation::Builder(TestName());

  const Shape shape_r0 = ShapeUtil::MakeShape(F32, {});
  auto param0 = builder.AddInstruction(
      HloInstruction::CreateParameter(0, shape_r0, "p0"));
  auto param1 = builder.AddInstruction(
      HloInstruction::CreateParameter(1, shape_r0, "p1"));
  auto add1 = builder.AddInstruction(
      HloInstruction::CreateBinary(shape_r0, HloOpcode::kAdd, param0, param1));
  auto add2 = builder.AddInstruction(
      HloInstruction::CreateBinary(shape_r0, HloOpcode::kAdd, param0, param1));
  auto mul = builder.AddInstruction(
      HloInstruction::CreateBinary(shape_r0, HloOpcode::kMultiply, add1, add2));

  auto computation = module->AddEntryComputation(builder.Build());
  auto fused_computation =
      computation
          ->CreateFusionInstruction({mul, add1, add2},
                                    HloInstruction::FusionKind::kLoop)
          ->fused_instructions_computation();

  EXPECT_EQ(5, fused_computation->instruction_count());
  HloCSE cse(/*is_layout_sensitive=*/false);
  EXPECT_TRUE(cse.Run(module.get()).ValueOrDie());
  EXPECT_EQ(4, fused_computation->instruction_count());

  auto root = fused_computation->root_instruction();
  EXPECT_THAT(root, op::Multiply(root->operand(0), root->operand(0)));
}

TEST_F(HloCseTest, IdenticalExpressions) {
  // Test that two identical expressions are commoned. Build the following
  // computation:
  //
  //   constant = 42.0
  //   negate1 = neg(constant)
  //   exp1 = exp(constant)
  //   add1 = add(negate1, exp1)
  //   negate2 = neg(constant)
  //   exp2 = exp(constant)
  //   add2 = add(negate2, exp2)
  //   tuple = tuple(add1, add2)
  //
  // The *1 instructions should be merged with the *2 instructions.
  auto builder = HloComputation::Builder(TestName());
  auto constant = builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR0<float>(42.0)));

  auto negate1 = builder.AddInstruction(HloInstruction::CreateUnary(
      constant->shape(), HloOpcode::kNegate, constant));
  auto exp1 = builder.AddInstruction(HloInstruction::CreateUnary(
      constant->shape(), HloOpcode::kExp, constant));
  auto add1 = builder.AddInstruction(HloInstruction::CreateBinary(
      constant->shape(), HloOpcode::kAdd, negate1, exp1));

  auto negate2 = builder.AddInstruction(HloInstruction::CreateUnary(
      constant->shape(), HloOpcode::kNegate, constant));
  auto exp2 = builder.AddInstruction(HloInstruction::CreateUnary(
      constant->shape(), HloOpcode::kExp, constant));
  auto add2 = builder.AddInstruction(HloInstruction::CreateBinary(
      constant->shape(), HloOpcode::kAdd, negate2, exp2));

  auto tuple =
      builder.AddInstruction(HloInstruction::CreateTuple({add1, add2}));

  auto module = CreateNewVerifiedModule();
  auto computation = module->AddEntryComputation(builder.Build());

  EXPECT_EQ(8, computation->instruction_count());
  EXPECT_THAT(tuple, op::Tuple(op::Add(negate1, exp1), op::Add(negate2, exp2)));

  HloCSE cse(/*is_layout_sensitive=*/false);
  EXPECT_TRUE(cse.Run(module.get()).ValueOrDie());

  EXPECT_EQ(5, computation->instruction_count());
  auto operand = tuple->operand(0);
  EXPECT_THAT(tuple, op::Tuple(operand, operand));
  EXPECT_THAT(operand, op::Add(op::Negate(), op::Exp()));
}

TEST_F(HloCseTest, DoNotCombineRng) {
  // Test that two RNG ops are not commoned.
  auto builder = HloComputation::Builder(TestName());
  auto constant1 = builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR0<float>(0.0f)));
  auto constant2 = builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR0<float>(1.0f)));
  auto rng1 = builder.AddInstruction(HloInstruction::CreateRng(
      ShapeUtil::MakeShape(F32, {}), RandomDistribution::RNG_UNIFORM,
      {constant1, constant2}));
  auto rng2 = builder.AddInstruction(HloInstruction::CreateRng(
      ShapeUtil::MakeShape(F32, {}), RandomDistribution::RNG_UNIFORM,
      {constant1, constant2}));

  builder.AddInstruction(HloInstruction::CreateBinary(
      constant1->shape(), HloOpcode::kAdd, rng1, rng2));

  auto module = CreateNewVerifiedModule();
  auto computation = module->AddEntryComputation(builder.Build());

  HloInstruction* root = computation->root_instruction();
  EXPECT_THAT(root, op::Add(rng1, rng2));

  uint32 count_before = computation->instruction_count();

  HloCSE cse(/*is_layout_sensitive=*/false);
  EXPECT_FALSE(cse.Run(module.get()).ValueOrDie());

  uint32 count_after = computation->instruction_count();
  EXPECT_EQ(count_before, count_after);
  root = computation->root_instruction();
  EXPECT_THAT(root, op::Add(rng1, rng2));
}

TEST_F(HloCseTest, DoNotCombineCallsToImpureFunctions) {
  // Test that two calls to an impure function are not commoned. RNG
  // is the source of the impurity.

  auto module = CreateNewVerifiedModule();

  // rng_function is an impure function because it does RNG.
  HloComputation* rng_function = nullptr;
  {
    Shape scalar_shape = ShapeUtil::MakeShape(F32, {});
    auto builder = HloComputation::Builder(TestName() + "_rng_fun");
    auto constant1 = builder.AddInstruction(
        HloInstruction::CreateConstant(LiteralUtil::CreateR0<float>(0.0f)));
    auto constant2 = builder.AddInstruction(
        HloInstruction::CreateConstant(LiteralUtil::CreateR0<float>(1.0f)));
    auto rng = builder.AddInstruction(HloInstruction::CreateRng(
        scalar_shape, RandomDistribution::RNG_UNIFORM, {constant1, constant2}));
    auto param = builder.AddInstruction(HloInstruction::CreateParameter(
        0, ShapeUtil::MakeShape(F32, {}), "param"));
    builder.AddInstruction(HloInstruction::CreateBinary(
        scalar_shape, HloOpcode::kAdd, rng, param));
    rng_function = module->AddEmbeddedComputation(builder.Build());
  }

  // Computation calls rng_function twice with the same parameter.
  HloComputation* computation = nullptr;
  {
    auto builder = HloComputation::Builder(TestName());
    auto constant = builder.AddInstruction(
        HloInstruction::CreateConstant(LiteralUtil::CreateR1<float>({5.0f})));
    auto rng1 = builder.AddInstruction(
        HloInstruction::CreateMap(constant->shape(), {constant}, rng_function));
    auto rng2 = builder.AddInstruction(
        HloInstruction::CreateMap(constant->shape(), {constant}, rng_function));
    builder.AddInstruction(HloInstruction::CreateBinary(
        constant->shape(), HloOpcode::kAdd, rng1, rng2));
    computation = module->AddEntryComputation(builder.Build());
  }

  EXPECT_EQ(4, computation->instruction_count());
  HloInstruction* root = computation->root_instruction();
  EXPECT_THAT(root, op::Add(op::Map(), op::Map()));

  VLOG(3) << "before: " << module->ToString();

  HloCSE cse(/*is_layout_sensitive=*/false);
  EXPECT_FALSE(cse.Run(module.get()).ValueOrDie());

  VLOG(3) << "after: " << module->ToString();

  EXPECT_EQ(4, computation->instruction_count());
  root = computation->root_instruction();
  EXPECT_THAT(root, op::Add(op::Map(op::Constant()), op::Map(op::Constant())));
}

TEST_F(HloCseTest, CompareComputations) {
  const char* const hlo_string = R"(
    HloModule m

    add_computation {
      add_lhs = f32[] parameter(0)
      add_rhs = f32[] parameter(1)
      ROOT add_root = f32[] add(add_lhs, add_rhs)
    }

    add_computation2 {
      add_lhs2 = f32[] parameter(0)
      add_rhs2 = f32[] parameter(1)
      ROOT add_root2 = f32[] add(add_lhs2, add_rhs2)
    }

    ENTRY entry {
      p = f32[10]{0} parameter(0)
      c = f32[] constant(0)
      r1 = f32[] reduce(p, c), dimensions={0}, to_apply=add_computation
      r2 = f32[] reduce(p, c), dimensions={0}, to_apply=add_computation2
      ROOT f2 = (f32[],f32[]) tuple(r1, r2)
    })";

  TF_ASSERT_OK_AND_ASSIGN(auto m, ParseAndReturnVerifiedModule(hlo_string));
  HloCSE cse(/*is_layout_sensitive=*/false);
  EXPECT_TRUE(cse.Run(m.get()).ValueOrDie());
  HloInstruction* root = m->entry_computation()->root_instruction();
  EXPECT_EQ(root->operand(0), root->operand(1));
}

TEST_F(HloCseTest, ConstantsSameValueInDifferentDomains) {
  // Test that constants with the same value but in different domains (disjoint
  // in this case) are not collapsed.
  auto builder = HloComputation::Builder(TestName());
  builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR0<uint32>(42)));
  builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR0<uint32>(42)));

  auto module = CreateNewVerifiedModule();
  auto computation = module->AddEntryComputation(builder.Build());

  EXPECT_EQ(2, computation->instruction_count());

  HloCSE cse(/*is_layout_sensitive=*/false);
  EXPECT_FALSE(cse.Run(module.get()).ValueOrDie());

  EXPECT_EQ(2, computation->instruction_count());
}

TEST_F(HloCseTest, Domain) {
  const char* const hlo_string = R"(
HloModule module
ENTRY %entry {
  %param = f32[] parameter(0), sharding={maximal device=0}
  %domain.0 = f32[] domain(%param),
    domain={kind="sharding", entry={maximal device=0}, exit={maximal device=1}}
  %domain.1 = f32[] domain(%param),
    domain={kind="sharding", entry={maximal device=0}, exit={maximal device=1}}
  %domain.2 = f32[] domain(%param),
    domain={kind="sharding", entry={maximal device=0}, exit={maximal device=2}}
  %negate.0 = f32[] negate(%domain.0)
  %negate.1 = f32[] negate(%domain.1)
  %negate.2 = f32[] negate(%domain.2)
  %domain.3 = f32[] domain(%negate.0),
    domain={kind="sharding", entry={maximal device=1}, exit={maximal device=0}}
  %domain.4 = f32[] domain(%negate.1),
    domain={kind="sharding", entry={maximal device=1}, exit={maximal device=0}}
  %domain.5 = f32[] domain(%negate.2),
    domain={kind="sharding", entry={maximal device=2}, exit={maximal device=0}}
  %add = f32[] add(%domain.3, %domain.4)
  ROOT %sub = f32[] subtract(%add, %domain.5)
})";

  TF_ASSERT_OK_AND_ASSIGN(auto m, ParseAndReturnVerifiedModule(hlo_string));
  HloCSE cse(/*is_layout_sensitive=*/false);
  EXPECT_TRUE(cse.Run(m.get()).ValueOrDie());
  const HloInstruction* sub = m->entry_computation()->root_instruction();
  const HloInstruction* add = sub->operand(0);
  EXPECT_EQ(add->operand(0), add->operand(1));
  EXPECT_NE(add->operand(0), sub->operand(1));
  EXPECT_NE(add->operand(1), sub->operand(1));
}

}  // namespace
}  // namespace xla
