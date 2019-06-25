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

#include <memory>
#include <utility>

#include "absl/memory/memory.h"
#include "tensorflow/compiler/xla/client/xla_builder.h"
#include "tensorflow/compiler/xla/literal_util.h"
#include "tensorflow/compiler/xla/service/cpu/custom_call_target_registry.h"
#include "tensorflow/compiler/xla/service/hlo_computation.h"
#include "tensorflow/compiler/xla/service/hlo_instruction.h"
#include "tensorflow/compiler/xla/service/hlo_module.h"
#include "tensorflow/compiler/xla/service/hlo_opcode.h"
#include "tensorflow/compiler/xla/shape_util.h"
#include "tensorflow/compiler/xla/tests/client_library_test_base.h"
#include "tensorflow/compiler/xla/tests/hlo_test_base.h"
#include "tensorflow/compiler/xla/tests/literal_test_util.h"
#include "tensorflow/compiler/xla/tests/test_macros.h"
#include "tensorflow/compiler/xla/xla_data.pb.h"
#include "tensorflow/core/platform/dynamic_annotations.h"
#include "tensorflow/core/platform/macros.h"
#include "tensorflow/core/platform/test.h"

namespace {
void R0F32Add2(float* out, float** in) {
  TF_ANNOTATE_MEMORY_IS_INITIALIZED(in, sizeof(float*));
  *out = **in + 2.0f;
}

void R2F32ReduceSum(float* out, float** in) {
  TF_ANNOTATE_MEMORY_IS_INITIALIZED(in, sizeof(float) * 4);
  float* array = in[0];
  *out = array[0] + array[1] + array[2] + array[3];
}

void Add1ToValues(float* out, float** in) {
  TF_ANNOTATE_MEMORY_IS_INITIALIZED(in, sizeof(float) * 4);
  float* array = in[0];
  out[0] = array[0] + 1;
  out[1] = array[1] + 1;
  out[2] = array[2] + 1;
  out[3] = array[3] + 1;
}

void F32TupleSwap(float** out, float** in) {
  TF_ANNOTATE_MEMORY_IS_INITIALIZED(in[0], sizeof(float));
  TF_ANNOTATE_MEMORY_IS_INITIALIZED(in[1], sizeof(float));
  *out[0] = *in[1];
  *out[1] = *in[0];
}

}  // namespace

REGISTER_CUSTOM_CALL_TARGET(R0F32Add2);
REGISTER_CUSTOM_CALL_TARGET(R2F32ReduceSum);
REGISTER_CUSTOM_CALL_TARGET(Add1ToValues);
REGISTER_CUSTOM_CALL_TARGET(F32TupleSwap);

namespace xla {
namespace {

class CustomCallTest : public HloTestBase {
 protected:
  Shape r0f32_ = ShapeUtil::MakeShape(F32, {});
  Shape r2f32_ = ShapeUtil::MakeShape(F32, {2, 2});
};

XLA_TEST_F(CustomCallTest, CustomCallR0F32Add2) {
  auto module = CreateNewUnverifiedModule();
  auto builder = HloComputation::Builder(TestName());

  auto constant = builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR0<float>(42.0f)));
  builder.AddInstruction(
      HloInstruction::CreateCustomCall(r0f32_, {constant}, "R0F32Add2"));

  module->AddEntryComputation(builder.Build());

  Literal result = ExecuteAndTransfer(std::move(module), {});
  LiteralTestUtil::ExpectR0Near<float>(44.0f, result, error_spec_);
}

XLA_TEST_F(CustomCallTest, CustomCallR2F32Reduce) {
  auto module = CreateNewUnverifiedModule();
  auto builder = HloComputation::Builder(TestName());

  Array2D<float> array(2, 2);
  array(0, 0) = 1.0f;
  array(0, 1) = 2.0f;
  array(1, 0) = 3.0f;
  array(1, 1) = 4.0f;

  auto constant = builder.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR2FromArray2D(array)));
  builder.AddInstruction(
      HloInstruction::CreateCustomCall(r0f32_, {constant}, "R2F32ReduceSum"));

  module->AddEntryComputation(builder.Build());

  Literal result = ExecuteAndTransfer(std::move(module), {});
  LiteralTestUtil::ExpectR0Near<float>(10.0f, result, error_spec_);
}

XLA_TEST_F(CustomCallTest, UsedInOtherComputations) {
  auto module = CreateNewUnverifiedModule();
  auto b = HloComputation::Builder(TestName());

  auto input = b.AddInstruction(
      HloInstruction::CreateConstant(LiteralUtil::CreateR2FromArray2D(
          Array2D<float>{{1.0f, 2.0f}, {3.0f, 4.0f}})));
  auto incremented = b.AddInstruction(HloInstruction::CreateCustomCall(
      ShapeUtil::MakeShape(F32, {1, 2, 2}), {input}, "Add1ToValues"));
  auto incremented_again = b.AddInstruction(HloInstruction::CreateCustomCall(
      ShapeUtil::MakeShape(F32, {1, 2, 2}), {incremented}, "Add1ToValues"));

  // Concatenate the values along first dim.
  b.AddInstruction(
      HloInstruction::CreateConcatenate(ShapeUtil::MakeShape(F32, {2, 2, 2}),
                                        {incremented, incremented_again}, 0));

  module->AddEntryComputation(b.Build());

  Literal result = ExecuteAndTransfer(std::move(module), {});
  LiteralTestUtil::ExpectR3EqualArray3D<float>(
      Array3D<float>{{{2, 3}, {4, 5}}, {{3, 4}, {5, 6}}}, result);
}

XLA_TEST_F(CustomCallTest, InputAndOutputLayoutDiffer) {
  auto module = CreateNewUnverifiedModule();
  auto b = HloComputation::Builder(TestName());

  auto input =
      b.AddInstruction(HloInstruction::CreateParameter(0, r2f32_, "p"));
  b.AddInstruction(
      HloInstruction::CreateCustomCall(r2f32_, {input}, "Add1ToValues"));

  module->AddEntryComputation(b.Build());
  ForceParameterLayout(module.get(), 0, LayoutUtil::MakeLayout({1, 0}));
  ForceResultLayout(module.get(), LayoutUtil::MakeLayout({0, 1}));

  Literal argument = LiteralUtil::CreateR2<float>({{1.f, 2.f}, {3.f, 4.f}});

  // Note, the expected result is transposed! This is because the input and
  // output layouts of the custom call differ and the called function just
  // blindly adds one to each element.
  Literal result = ExecuteAndTransfer(std::move(module), {&argument});
  LiteralTestUtil::ExpectR2Equal<float>({{2.f, 4.f}, {3.f, 5.f}}, result);
}

XLA_TEST_F(CustomCallTest, LayoutConstrained) {
  // The argument and result of the computation are set to different layouts,
  // but the custom call is layout constrained to a fixed operand and result
  // layout, so the correct result should be produced.
  auto module = CreateNewUnverifiedModule();
  auto b = HloComputation::Builder(TestName());

  auto input =
      b.AddInstruction(HloInstruction::CreateParameter(0, r2f32_, "p"));

  const Shape& r2f32_dim0_major =
      ShapeUtil::MakeShapeWithLayout(F32, {2, 2}, {1, 0});
  b.AddInstruction(HloInstruction::CreateCustomCall(
      r2f32_dim0_major, {input}, "Add1ToValues", {r2f32_dim0_major}));

  module->AddEntryComputation(b.Build());
  ForceParameterLayout(module.get(), 0, LayoutUtil::MakeLayout({1, 0}));
  ForceResultLayout(module.get(), LayoutUtil::MakeLayout({0, 1}));

  Literal argument = LiteralUtil::CreateR2<float>({{1.f, 2.f}, {3.f, 4.f}});

  Literal result = ExecuteAndTransfer(std::move(module), {&argument});
  LiteralTestUtil::ExpectR2Equal<float>({{2.f, 3.f}, {4.f, 5.f}}, result);
}

XLA_TEST_F(CustomCallTest, TupleOutput) {
  const char* kModuleStr = R"(
    HloModule m
    test {
      p0 = f32[] parameter(0)
      p1 = f32[] parameter(1)
      ROOT %custom-call = (f32[], f32[]) custom-call(f32[] %p0, f32[] %p1), custom_call_target="F32TupleSwap", operand_layout_constraints={f32[], f32[]}
    }
  )";
  TF_ASSERT_OK_AND_ASSIGN(auto module,
                          ParseAndReturnVerifiedModule(kModuleStr));

  Literal arg0 = LiteralUtil::CreateR0<float>(7.f);
  Literal arg1 = LiteralUtil::CreateR0<float>(42.f);

  Literal expected = LiteralUtil::MakeTuple({&arg1, &arg0});
  Literal result = ExecuteAndTransfer(std::move(module), {&arg0, &arg1});
  EXPECT_EQ(result, expected);
}

class CustomCallClientAPITest : public ClientLibraryTestBase {};

// When using the client API, CustomCall targets can't begin with '$' -- these
// are reserved for internal use.
XLA_TEST_F(CustomCallClientAPITest, IllegalCustomCallTarget) {
  XlaBuilder builder(TestName());
  CustomCall(&builder, "$illegal", /*operands=*/{},
             ShapeUtil::MakeShape(F32, {1}));

  StatusOr<std::unique_ptr<GlobalData>> result =
      Execute(&builder, /*arguments=*/{});
  EXPECT_FALSE(result.ok());
}

}  // namespace
}  // namespace xla
