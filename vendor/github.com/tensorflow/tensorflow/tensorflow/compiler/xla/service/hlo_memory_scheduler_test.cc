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

#include "tensorflow/compiler/xla/service/hlo_memory_scheduler.h"

#include <memory>
#include <string>

#include "absl/algorithm/container.h"
#include "absl/container/flat_hash_map.h"
#include "tensorflow/compiler/xla/service/heap_simulator.h"
#include "tensorflow/compiler/xla/service/hlo_computation.h"
#include "tensorflow/compiler/xla/service/hlo_dce.h"
#include "tensorflow/compiler/xla/service/hlo_instruction.h"
#include "tensorflow/compiler/xla/service/hlo_opcode.h"
#include "tensorflow/compiler/xla/service/hlo_ordering.h"
#include "tensorflow/compiler/xla/service/hlo_parser.h"
#include "tensorflow/compiler/xla/shape_util.h"
#include "tensorflow/compiler/xla/tests/hlo_test_base.h"
#include "tensorflow/compiler/xla/types.h"
#include "tensorflow/compiler/xla/xla_data.pb.h"
#include "tensorflow/core/lib/core/status_test_util.h"

namespace xla {
namespace {

class HloSchedulingTest : public HloTestBase {};

TEST_F(HloSchedulingTest, LastUseScheduledFirst) {
  // Tests scheduling of the following HLO code:
  //
  //   %ab = abs(%param)
  //   %exp = exp(%param)
  //   %add = add(%ab, %exp)
  //   %negate = negate(%exp)
  //   %sub = subtract(%add, %negate)
  //
  // %add should be scheduled before %negate because %add is the last (and only)
  // use of %ab. Scheduling %add first then frees up %ab's buffer.
  const Shape vec = ShapeUtil::MakeShape(xla::F32, {42});
  auto builder = HloComputation::Builder(TestName());
  auto param =
      builder.AddInstruction(HloInstruction::CreateParameter(0, vec, "param"));
  auto ab = builder.AddInstruction(
      HloInstruction::CreateUnary(vec, HloOpcode::kAbs, param));
  auto exp = builder.AddInstruction(
      HloInstruction::CreateUnary(vec, HloOpcode::kExp, param));

  auto add = builder.AddInstruction(
      HloInstruction::CreateBinary(vec, HloOpcode::kAdd, ab, exp));
  auto negate = builder.AddInstruction(
      HloInstruction::CreateUnary(vec, HloOpcode::kNegate, exp));
  auto sub = builder.AddInstruction(
      HloInstruction::CreateBinary(vec, HloOpcode::kSubtract, add, negate));

  auto module = CreateNewVerifiedModule();
  module->AddEntryComputation(builder.Build());

  HloMemoryScheduler scheduler([](const BufferValue& buffer) {
    return ShapeUtil::ByteSizeOf(buffer.shape());
  });
  ASSERT_FALSE(module->has_schedule());
  TF_ASSERT_OK_AND_ASSIGN(bool changed, scheduler.Run(module.get()));
  EXPECT_TRUE(changed);
  ASSERT_TRUE(module->has_schedule());
  TF_ASSERT_OK(module->schedule().Verify());

  // Verify that all instructions are in the sequence.
  const std::vector<HloInstruction*>& sequence =
      module->schedule().sequence(module->entry_computation()).instructions();
  EXPECT_EQ(module->entry_computation()->instruction_count(), sequence.size());

  // The first instruction should be the parameter and the last the root "sub".
  EXPECT_EQ(param, sequence.front());
  EXPECT_EQ(sub, sequence.back());

  SequentialHloOrdering ordering(module->schedule());
  EXPECT_TRUE(ordering.ExecutesBefore(add, negate));

  // Clear the schedule using the descheduling pass.
  HloDescheduler descheduler;
  EXPECT_TRUE(module->has_schedule());
  TF_ASSERT_OK_AND_ASSIGN(bool descheduler_changed,
                          descheduler.Run(module.get()));
  EXPECT_TRUE(descheduler_changed);
  EXPECT_FALSE(module->has_schedule());
}

TEST_F(HloSchedulingTest, ListSchedulerHandlesAliasing) {
  const char* module_str = R"(
HloModule test_aliasing_module

ENTRY root {
  param = s32[1000] parameter(0)
  p0 = s32[1000] copy(param)
  p1 = s32[1000] copy(param)
  t = (s32[1000], s32[1000]) tuple(p0, p1)
  a = s32[1000] get-tuple-element(t), index=0
  b = s32[1000] get-tuple-element(t), index=1
  c = s32[1000] add(a, b)
  d = s32[1000] add(c, b)
  e = s32[1000] add(c, c)
  f = s32[1000] add(e, e)
  ROOT result = (s32[1000], s32[1000], s32[1000]) tuple(d, e, f)
})";

  TF_ASSERT_OK_AND_ASSIGN(std::unique_ptr<HloModule> module,
                          ParseHloString(module_str));

  auto size_fn = [](const BufferValue& buffer) {
    return ShapeUtil::ByteSizeOf(buffer.shape(), /*pointer_size=*/8);
  };
  TF_ASSERT_OK_AND_ASSIGN(
      HloSchedule schedule,
      ScheduleModule(module.get(), size_fn, ListMemoryScheduler));
  // Verify that all instructions are in the sequence.
  const std::vector<HloInstruction*>& sequence =
      schedule.sequence(module->entry_computation()).instructions();
  EXPECT_EQ(module->entry_computation()->instruction_count(), sequence.size());

  std::unordered_map<string, const HloInstruction*> instructions_by_name;
  for (const HloInstruction* instruction : sequence) {
    instructions_by_name[instruction->name()] = instruction;
  }

  // The first instruction should be the parameter and the last the root.
  EXPECT_EQ(instructions_by_name.at("param"), sequence.front());
  EXPECT_EQ(instructions_by_name.at("result"), sequence.back());

  // Instructions "d" and "e" will both be schedulable at the same time, but
  // instruction "d" allows us to free the buffer of "p1", so the list scheduler
  // should prefer it.
  SequentialHloOrdering ordering(schedule);
  EXPECT_TRUE(ordering.ExecutesBefore(instructions_by_name.at("d"),
                                      instructions_by_name.at("e")));
}

TEST_F(HloSchedulingTest, TuplesAreAccountedCorrectly) {
  auto builder = HloComputation::Builder(TestName());
  const auto TUPLE_SIZE = 1;
  const Shape r1f32 = ShapeUtil::MakeShape(xla::F32, {6});

  // Wrap lit in abs because constants are considered free by
  // IgnoreInstruction, and it skews the accounting.
  auto lit = builder.AddInstruction(HloInstruction::CreateConstant(
      LiteralUtil::CreateR1<float>({1, 1, 1, 1, 1, 1})));
  auto abs_const = builder.AddInstruction(
      HloInstruction::CreateUnary(r1f32, HloOpcode::kAbs, lit));

  auto abs_abs1 = builder.AddInstruction(
      HloInstruction::CreateUnary(r1f32, HloOpcode::kAbs, abs_const));
  auto tuple = builder.AddInstruction(HloInstruction::CreateTuple(
      absl::Span<HloInstruction* const>({abs_abs1})));
  auto tuple_elm = builder.AddInstruction(
      HloInstruction::CreateGetTupleElement(r1f32, tuple, 0));

  auto abs_abs2 = builder.AddInstruction(
      HloInstruction::CreateUnary(r1f32, HloOpcode::kAbs, abs_const));

  builder.AddInstruction(HloInstruction::CreateBinary(r1f32, HloOpcode::kAdd,
                                                      tuple_elm, abs_abs2));

  auto module = CreateNewVerifiedModule();
  module->AddEntryComputation(builder.Build());
  TF_ASSERT_OK_AND_ASSIGN(HloSchedule schedule,
                          ScheduleModule(
                              module.get(),
                              [](const BufferValue& buffer) {
                                return ShapeUtil::ByteSizeOf(buffer.shape(),
                                                             TUPLE_SIZE);
                              },
                              ListMemoryScheduler));

  // Verify that all instructions are in the sequence.
  EXPECT_EQ(module->entry_computation()->instruction_count(),
            schedule.sequence(module->entry_computation()).size());
  SequentialHloOrdering ordering(schedule);
  // tuple allocates the tuple buffer and doesn't free anything.
  // abs_abs2 uses the same buffer for input/output, so its bytes-freed is 0.
  // abs_abs2 should be scheduled before tuple by List.
  EXPECT_TRUE(ordering.ExecutesBefore(abs_abs2, tuple));
}

TEST_F(HloSchedulingTest, MultiOutputFusionAccountedCorrectly) {
  const Shape r1f32 = ShapeUtil::MakeShape(xla::F32, {5});
  HloComputation::Builder builder(TestName());

  auto c1 = builder.AddInstruction(HloInstruction::CreateConstant(
      LiteralUtil::CreateR1<float>({1, 1, 1, 1, 1})));
  auto c2 = builder.AddInstruction(HloInstruction::CreateConstant(
      LiteralUtil::CreateR1<float>({1, 2, 3, 4, 5})));
  auto c3 = builder.AddInstruction(HloInstruction::CreateConstant(
      LiteralUtil::CreateR1<float>({0, 2, 4, 6, 8})));

  auto add = builder.AddInstruction(
      HloInstruction::CreateBinary(r1f32, HloOpcode::kAdd, c1, c2));
  auto mul = builder.AddInstruction(
      HloInstruction::CreateBinary(r1f32, HloOpcode::kMultiply, add, c3));
  auto tuple = builder.AddInstruction(HloInstruction::CreateTuple({add, mul}));

  auto tuple_elm = builder.AddInstruction(
      HloInstruction::CreateGetTupleElement(r1f32, tuple, 0));

  auto exp = builder.AddInstruction(
      HloInstruction::CreateUnary(r1f32, HloOpcode::kExp, c3));

  builder.AddInstruction(
      HloInstruction::CreateBinary(r1f32, HloOpcode::kAdd, tuple_elm, exp));

  auto module = CreateNewVerifiedModule();
  auto* computation = module->AddEntryComputation(builder.Build());

  auto fusion = computation->CreateFusionInstruction(
      {tuple, mul, add}, HloInstruction::FusionKind::kLoop);

  TF_ASSERT_OK_AND_ASSIGN(HloSchedule schedule,
                          ScheduleModule(
                              module.get(),
                              [](const BufferValue& buffer) {
                                return ShapeUtil::ByteSizeOf(buffer.shape(), 2);
                              },
                              ListMemoryScheduler));

  // Verify that all instructions are in the sequence.
  EXPECT_EQ(module->entry_computation()->instruction_count(),
            schedule.sequence(module->entry_computation()).size());
  SequentialHloOrdering ordering(schedule);
  // fusion allocates memory for the tuple elements and doesn't free anything,
  // so it's more expensive than exp.
  EXPECT_TRUE(ordering.ExecutesBefore(exp, fusion));
}

TEST_F(HloSchedulingTest, HeapSimulatorAccountsForSubcomputations) {
  auto module = CreateNewUnverifiedModule();
  const Shape r1f32 = ShapeUtil::MakeShape(F32, {4});

  // param != 0
  // Needs 17 bytes
  auto cond_builder = HloComputation::Builder("WhileCond");
  HloInstruction* cond_param = cond_builder.AddInstruction(
      HloInstruction::CreateParameter(0, r1f32, "cond_param"));
  HloInstruction* zero_vector =
      cond_builder.AddInstruction(HloInstruction::CreateConstant(
          LiteralUtil::CreateR1<float>({0, 0, 0, 0})));
  cond_builder.AddInstruction(HloInstruction::CreateBinary(
      ShapeUtil::MakeShape(PRED, {}), HloOpcode::kNe, cond_param, zero_vector));
  auto cond_computation = module->AddEmbeddedComputation(cond_builder.Build());

  // param - 1
  // Needs 16 bytes
  auto body_builder = HloComputation::Builder("WhileBody");
  HloInstruction* body_param = body_builder.AddInstruction(
      HloInstruction::CreateParameter(0, r1f32, "body_param"));
  HloInstruction* one_vector =
      body_builder.AddInstruction(HloInstruction::CreateConstant(
          LiteralUtil::CreateR2<float>({{1, 1, 1, 1}})));
  body_builder.AddInstruction(HloInstruction::CreateBinary(
      r1f32, HloOpcode::kSubtract, body_param, one_vector));
  auto body_computation = module->AddEmbeddedComputation(body_builder.Build());

  auto builder = HloComputation::Builder(TestName());
  HloInstruction* while_init =
      builder.AddInstruction(HloInstruction::CreateConstant(
          LiteralUtil::CreateR2<float>({{1, 1, 1, 1}})));
  // Creates 16 bytes, ignoring subcomputations
  builder.AddInstruction(HloInstruction::CreateWhile(
      r1f32, cond_computation, body_computation, while_init));

  module->AddEntryComputation(builder.Build());

  auto size_fn = [](const BufferValue& buffer) {
    return ShapeUtil::ByteSizeOf(buffer.shape());
  };
  TF_ASSERT_OK_AND_ASSIGN(
      HloSchedule schedule,
      ScheduleModule(module.get(), size_fn, ListMemoryScheduler));
  // Verify that all instructions are in the sequence.
  auto entry_computation = module->entry_computation();
  EXPECT_EQ(module->entry_computation()->instruction_count(),
            schedule.sequence(module->entry_computation()).size());

  absl::flat_hash_map<const HloComputation*, int64> memory_by_computation;
  memory_by_computation[cond_computation] = 17;
  memory_by_computation[body_computation] = 16;
  std::unique_ptr<TuplePointsToAnalysis> points_to_analysis =
      TuplePointsToAnalysis::Run(module.get()).ValueOrDie();

  // HeapSimulator doesn't account for subcomputations
  EXPECT_EQ(16, HeapSimulator::MinimumMemoryForComputation(
                    *entry_computation, schedule.sequence(entry_computation),
                    *points_to_analysis, size_fn)
                    .ValueOrDie());
  // HeapSimulator accounts for subcomputations. Cond is the largest one.
  // The output buffer of the while is aliased.
  EXPECT_EQ(17, HeapSimulator::MinimumMemoryForComputation(
                    *entry_computation, schedule.sequence(entry_computation),
                    *points_to_analysis, size_fn, &memory_by_computation)
                    .ValueOrDie());
}

TEST_F(HloSchedulingTest, TrivialScheduler) {
  const char* const hlo_string = R"(
HloModule ModuleWithWhile

body {
  param.b = (s32[], s32[]) parameter(0)
  gte.0 = s32[] get-tuple-element(param.b), index=0
  gte.1 = s32[] get-tuple-element(param.b), index=1
  add = s32[] add(gte.0, gte.1)
  ROOT tuple = (s32[], s32[]) tuple(gte.0, add)
}

cond {
  param.c = (s32[], s32[]) parameter(0)
  ROOT constant = pred[] constant(true)
}

ENTRY main {
  init = (s32[], s32[]) parameter(0)
  ROOT while = (s32[], s32[]) while(init), condition=cond, body=body
}
)";
  TF_ASSERT_OK_AND_ASSIGN(std::unique_ptr<HloModule> module,
                          ParseHloString(hlo_string));
  EXPECT_FALSE(module->has_schedule());
  TF_ASSERT_OK(HloTrivialScheduler().Run(module.get()).status());
  ASSERT_TRUE(module->has_schedule());
  TF_ASSERT_OK(module->schedule().Verify());

  // Verify that a clone of the module also has a schedule.
  std::unique_ptr<HloModule> clone = module->Clone();
  ASSERT_TRUE(clone->has_schedule());
  TF_ASSERT_OK(clone->schedule().Verify());
}

}  // namespace
}  // namespace xla
