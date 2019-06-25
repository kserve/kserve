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

#include "tensorflow/compiler/xla/util.h"

#include <list>

#include "tensorflow/compiler/xla/test.h"
#include "tensorflow/compiler/xla/types.h"

namespace xla {
namespace {

// Verifies that, even with a different number of leading spaces, the
// Reindent routine turns them into a uniform number of leading spaces.
//
// Also throws in some trailing whitespace on the original to show it is
// removed.
TEST(UtilTest, ReindentsDifferentNumberOfLeadingSpacesUniformly) {
  string original = R"(   hello there  
      world)";
  string got = Reindent(original, "  ");
  string want = R"(  hello there
  world)";
  EXPECT_EQ(want, got);
}

TEST(UtilTest, HumanReadableNumFlopsExample) {
  ASSERT_EQ("1.00GFLOP/s", HumanReadableNumFlops(1e9, 1e9));
}

TEST(UtilTest, CommaSeparatedString) {
  EXPECT_EQ(CommaSeparatedString({}), "");
  EXPECT_EQ(CommaSeparatedString({"hello world"}), "hello world");
  EXPECT_EQ(CommaSeparatedString({1, 57, 2}, "foo", "bar"), "foo1, 57, 2bar");
}

TEST(UtilTest, VectorString) {
  std::list<int64> empty_list;
  EXPECT_EQ(VectorString(empty_list), "()");

  std::vector<float> float_vector = {5.5};
  EXPECT_EQ(VectorString(float_vector), "(5.5)");

  std::set<const char*> string_set = {"a", "b"};
  EXPECT_EQ(VectorString(string_set), "(a, b)");

  EXPECT_EQ(VectorString({}), "()");
  EXPECT_EQ(VectorString({1, 57, 2}), "(1, 57, 2)");
}

TEST(UtilTest, LogLines) {
  // Just make sure this code runs (not verifying the output).
  LogLines(tensorflow::INFO, "hello\n\nworld", __FILE__, __LINE__);
}

TEST(UtilTest, CommonFactors) {
  struct {
    std::vector<int64> a, b;
    std::vector<std::pair<int64, int64>> expected;
  } test_cases[] = {
      {/*.a =*/{0}, /*.b =*/{0}, /*.expected =*/{{0, 0}, {1, 1}}},
      {/*.a =*/{}, /*.b =*/{}, /*.expected =*/{{0, 0}}},
      {/*.a =*/{2, 5, 1, 3},
       /*.b =*/{1, 10, 3, 1},
       /*.expected =*/{{0, 0}, {0, 1}, {2, 2}, {3, 2}, {4, 3}, {4, 4}}},
  };
  for (const auto& test_case : test_cases) {
    EXPECT_TRUE(absl::c_equal(test_case.expected,
                              CommonFactors(test_case.a, test_case.b)));
  }
}

TEST(UtilTest, SanitizeFileName) {
  EXPECT_EQ(SanitizeFileName(""), "");
  EXPECT_EQ(SanitizeFileName("abc"), "abc");
  EXPECT_EQ(SanitizeFileName("/\\[]"), "____");
  EXPECT_EQ(SanitizeFileName("/A\\B[C]"), "_A_B_C_");
}

}  // namespace
}  // namespace xla
