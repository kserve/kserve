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

#include <cmath>
#include <vector>

#include "tensorflow/compiler/xla/client/local_client.h"
#include "tensorflow/compiler/xla/client/xla_builder.h"
#include "tensorflow/compiler/xla/tests/client_library_test_base.h"
#include "tensorflow/compiler/xla/tests/literal_test_util.h"
#include "tensorflow/compiler/xla/tests/test_macros.h"
#include "tensorflow/core/platform/test.h"

namespace xla {
namespace {

class LogTest : public ClientLibraryTestBase {};

XLA_TEST_F(LogTest, LogZeroValues) {
  XlaBuilder builder(TestName());
  auto x = ConstantR3FromArray3D<float>(&builder, Array3D<float>(3, 0, 0));
  Log(x);

  ComputeAndCompareR3<float>(&builder, Array3D<float>(3, 0, 0), {},
                             ErrorSpec(0.0001));
}

TEST_F(LogTest, LogTenValues) {
  std::vector<float> input = {-0.0, 1.0, 2.0,  -3.0, -4.0,
                              5.0,  6.0, -7.0, -8.0, 9.0};

  XlaBuilder builder(TestName());
  auto x = ConstantR1<float>(&builder, input);
  Log(x);

  std::vector<float> expected;
  expected.reserve(input.size());
  for (float f : input) {
    expected.push_back(std::log(f));
  }

  ComputeAndCompareR1<float>(&builder, expected, {}, ErrorSpec(0.0001));
}

}  // namespace
}  // namespace xla
