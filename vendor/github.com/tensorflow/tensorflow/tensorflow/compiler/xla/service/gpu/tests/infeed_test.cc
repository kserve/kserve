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

#include <unistd.h>
#include <memory>

#include "tensorflow/compiler/xla/client/global_data.h"
#include "tensorflow/compiler/xla/client/lib/arithmetic.h"
#include "tensorflow/compiler/xla/client/local_client.h"
#include "tensorflow/compiler/xla/client/xla_builder.h"
#include "tensorflow/compiler/xla/literal.h"
#include "tensorflow/compiler/xla/shape_util.h"
#include "tensorflow/compiler/xla/test_helpers.h"
#include "tensorflow/compiler/xla/tests/client_library_test_base.h"
#include "tensorflow/compiler/xla/tests/literal_test_util.h"
#include "tensorflow/compiler/xla/xla_data.pb.h"
#include "tensorflow/core/lib/math/math_util.h"
#include "tensorflow/core/platform/env.h"
#include "tensorflow/core/platform/types.h"

namespace xla {
namespace {

class InfeedTest : public ClientLibraryTestBase {
 protected:
  // Transfers the given literal to the infeed interface of the device, and
  // check if the returned data from Infeed HLO is same as the literal.
  void TestInfeedRoundTrip(const Literal& literal) {
    // TODO(b/30481585) Explicitly reset the Infeed state so that the
    // test is not affected by the state from the previous tests.
    ASSERT_IS_OK(client_->TransferToInfeed(literal));
    XlaBuilder builder(TestName());
    Infeed(&builder, literal.shape());
    if (ShapeUtil::IsTuple(literal.shape())) {
      // TODO(b/30609564): Use ComputeAndCompareLiteral instead.
      ComputeAndCompareTuple(&builder, literal, {});
    } else {
      ComputeAndCompareLiteral(&builder, literal, {});
    }
  }
};

TEST_F(InfeedTest, SingleInfeedR0Bool) {
  TestInfeedRoundTrip(LiteralUtil::CreateR0<bool>(true));
}

TEST_F(InfeedTest, SingleInfeedR1U32) {
  TestInfeedRoundTrip(LiteralUtil::CreateR1<uint32>({1, 2, 3}));
}

TEST_F(InfeedTest, SingleInfeedR2F32) {
  TestInfeedRoundTrip(LiteralUtil::CreateR2F32Linspace(0.0, 1.0, 128, 64));
}

TEST_F(InfeedTest, SingleInfeedR3F32) {
  TestInfeedRoundTrip(
      LiteralUtil::CreateR3({{{1.0f, 2.0f, 3.0f}, {4.0f, 5.0f, 6.0f}},
                             {{1.1f, 2.1f, 3.1f}, {6.1f, 3.5f, 2.8f}}}));
}

TEST_F(InfeedTest, SingleInfeedR3F32DifferentLayout) {
  const Layout r3_dim0minor = LayoutUtil::MakeLayout({0, 1, 2});
  const Layout r3_dim0major = LayoutUtil::MakeLayout({2, 1, 0});

  TestInfeedRoundTrip(LiteralUtil::CreateR3WithLayout(
      {{{1.0f, 2.0f, 3.0f}, {4.0f, 5.0f, 6.0f}},
       {{1.1f, 2.1f, 3.1f}, {6.1f, 3.5f, 2.8f}}},
      r3_dim0minor));

  TestInfeedRoundTrip(LiteralUtil::CreateR3WithLayout(
      {{{1.0f, 2.0f, 3.0f}, {4.0f, 5.0f, 6.0f}},
       {{1.1f, 2.1f, 3.1f}, {6.1f, 3.5f, 2.8f}}},
      r3_dim0major));
}

TEST_F(InfeedTest, SingleInfeedR4S32) {
  TestInfeedRoundTrip(LiteralUtil::CreateR4(
      {{{{1, -2}, {-4, 5}, {6, 7}}, {{8, 9}, {10, 11}, {12, 13}}},
       {{{10, 3}, {7, -2}, {3, 6}}, {{2, 5}, {-11, 5}, {-2, -5}}}}));
}

// Tests that a large infeed can be handled.
TEST_F(InfeedTest, LargeInfeed) {
  Array4D<float> array(80, 100, 8, 128);
  array.FillIota(1.0f);
  TestInfeedRoundTrip(LiteralUtil::CreateR4FromArray4D<float>(array));
}

TEST_F(InfeedTest, SingleInfeedTuple) {
  TestInfeedRoundTrip(LiteralUtil::MakeTupleFromSlices(
      {LiteralUtil::CreateR1<uint32>({1, 2, 3}),
       LiteralUtil::CreateR0<bool>(false)}));
}

TEST_F(InfeedTest, SingleInfeedEmptyTuple) {
  TestInfeedRoundTrip(LiteralUtil::MakeTuple({}));
}

// Tests that a large tuple infeed can be handled.
TEST_F(InfeedTest, SingleInfeedLargeTuple) {
  Array4D<float> array(40, 100, 8, 128);
  array.FillIota(1.0f);
  TestInfeedRoundTrip(LiteralUtil::MakeTupleFromSlices(
      {LiteralUtil::CreateR4FromArray4D<float>(array),
       LiteralUtil::CreateR0<int32>(5)}));
}

}  // namespace
}  // namespace xla
