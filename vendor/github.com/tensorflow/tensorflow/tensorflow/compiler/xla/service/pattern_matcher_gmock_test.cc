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

#include "tensorflow/compiler/xla/service/pattern_matcher_gmock.h"
#include "tensorflow/compiler/xla/service/pattern_matcher.h"
#include "tensorflow/compiler/xla/shape_util.h"
#include "tensorflow/compiler/xla/test.h"
#include "tensorflow/core/platform/test.h"

namespace xla {
namespace {

namespace m = ::xla::match;
using ::testing::Eq;
using ::testing::Not;

template <typename MatchedTy>
string Describe(const ::testing::Matcher<MatchedTy>& m) {
  std::stringstream ss;
  m.DescribeTo(&ss);
  return ss.str();
}

template <typename MatchedTy>
string Explain(
    const MatchedTy& val,
    const ::testing::Matcher<typename std::remove_cv<MatchedTy>::type>& m) {
  ::testing::StringMatchResultListener listener;
  EXPECT_THAT(val, ::testing::Not(m));  // For the error message.
  EXPECT_FALSE(m.MatchAndExplain(val, &listener));
  return listener.str();
}

// This file tests the GmockMatch function.  The actual explanation and
// description returned by matchers is tested in pattern_matchers_test.
TEST(PatternMatcherGmock, MatchShape) {
  Shape s = ShapeUtil::MakeShape(F32, {10, 100});
  // You can pass const Shape& or a const Shape*.
  EXPECT_THAT(s, GmockMatch(m::Shape()));
  EXPECT_THAT(&s, Not(GmockMatch(m::Shape().WithElementType(F16))));
  EXPECT_THAT(Describe<Shape>(GmockMatch(m::Shape().IsArray())),
              "a shape that represents an array");
}

TEST(PatternMatcherGmock, MatchLayout) {
  Layout l = LayoutUtil::MakeLayout({0, 1});
  EXPECT_THAT(l, GmockMatch(m::Layout()));
  EXPECT_THAT(&l, Not(GmockMatch(m::Layout().WithSparseFormat())));
  EXPECT_THAT(Describe<Layout>(GmockMatch(m::Layout().WithSparseFormat())),
              "a layout with format SPARSE");
}

TEST(PatternMatchGmock, MatchInstruction) {
  auto instr =
      HloInstruction::CreateParameter(0, ShapeUtil::MakeShape(F32, {42}), "p");
  EXPECT_THAT(instr.get(), GmockMatch(m::Parameter()));
  EXPECT_THAT(*instr, GmockMatch(m::Parameter(0)));
  EXPECT_THAT(*instr, Not(GmockMatch(m::Parameter(1))));
  EXPECT_THAT(Describe<HloInstruction*>(GmockMatch(m::Parameter())),
              "an HloInstruction with opcode parameter");
}

}  // anonymous namespace
}  // namespace xla
