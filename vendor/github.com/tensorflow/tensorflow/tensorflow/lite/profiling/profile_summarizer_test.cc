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

#include <string>
#include <vector>

#include <gmock/gmock.h>
#include <gtest/gtest.h>
#include "tensorflow/lite/context.h"
#include "tensorflow/lite/kernels/kernel_util.h"
#include "tensorflow/lite/kernels/test_util.h"
#include "tensorflow/lite/model.h"
#include "tensorflow/lite/profiling/profile_summarizer.h"
#include "tensorflow/lite/testing/util.h"
#include "tensorflow/lite/version.h"

namespace tflite {
namespace profiling {

namespace {

const char* kOpName = "SimpleOpEval";

#ifdef TFLITE_PROFILING_ENABLED
TfLiteStatus SimpleOpEval(TfLiteContext* context, TfLiteNode* node) {
  const TfLiteTensor* input1 = tflite::GetInput(context, node, /*index=*/0);
  const TfLiteTensor* input2 = tflite::GetInput(context, node, /*index=*/1);

  TfLiteTensor* output = GetOutput(context, node, /*index=*/0);

  int32_t* output_data = output->data.i32;
  *output_data = *(input1->data.i32) + *(input2->data.i32);
  return kTfLiteOk;
}

const char* SimpleOpProfilingString(const TfLiteContext* context,
                                    const TfLiteNode* node) {
  return "Profile";
}

TfLiteRegistration* RegisterSimpleOp() {
  static TfLiteRegistration registration = {
      nullptr,        nullptr, nullptr,
      SimpleOpEval,   nullptr, tflite::BuiltinOperator_CUSTOM,
      "SimpleOpEval", 1};
  return &registration;
}

TfLiteRegistration* RegisterSimpleOpWithProfilingDetails() {
  static TfLiteRegistration registration = {nullptr,
                                            nullptr,
                                            nullptr,
                                            SimpleOpEval,
                                            SimpleOpProfilingString,
                                            tflite::BuiltinOperator_CUSTOM,
                                            kOpName,
                                            1};
  return &registration;
}
#endif

class SimpleOpModel : public SingleOpModel {
 public:
  void Init(const std::function<TfLiteRegistration*()>& registration);
  tflite::Interpreter* GetInterpreter() { return interpreter_.get(); }
  void SetInputs(int32_t x, int32_t y) {
    PopulateTensor(inputs_[0], {x});
    PopulateTensor(inputs_[1], {y});
  }
  int32_t GetOutput() { return ExtractVector<int32_t>(output_)[0]; }

 private:
  int inputs_[2];
  int output_;
};

void SimpleOpModel::Init(
    const std::function<TfLiteRegistration*()>& registration) {
  inputs_[0] = AddInput({TensorType_INT32, {1}});
  inputs_[1] = AddInput({TensorType_INT32, {1}});
  output_ = AddOutput({TensorType_INT32, {}});
  SetCustomOp(kOpName, {}, registration);
  BuildInterpreter({GetShape(inputs_[0]), GetShape(inputs_[1])});
}

TEST(ProfileSummarizerTest, Empty) {
  ProfileSummarizer summarizer;
  std::string output = summarizer.GetOutputString();
  EXPECT_GT(output.size(), 0);
}

#ifdef TFLITE_PROFILING_ENABLED
TEST(ProfileSummarizerTest, Interpreter) {
  Profiler profiler;
  SimpleOpModel m;
  m.Init(RegisterSimpleOp);
  auto interpreter = m.GetInterpreter();
  interpreter->SetProfiler(&profiler);
  profiler.StartProfiling();
  m.SetInputs(1, 2);
  m.Invoke();
  // 3 = 1 + 2
  EXPECT_EQ(m.GetOutput(), 3);
  profiler.StopProfiling();
  ProfileSummarizer summarizer;
  auto events = profiler.GetProfileEvents();
  EXPECT_EQ(1, events.size());
  summarizer.ProcessProfiles(profiler.GetProfileEvents(), *interpreter);
  auto output = summarizer.GetOutputString();
  // TODO(shashishekhar): Add a better test here.
  ASSERT_TRUE(output.find("SimpleOpEval") != std::string::npos) << output;
}

TEST(ProfileSummarizerTest, InterpreterPlusProfilingDetails) {
  Profiler profiler;
  SimpleOpModel m;
  m.Init(RegisterSimpleOpWithProfilingDetails);
  auto interpreter = m.GetInterpreter();
  interpreter->SetProfiler(&profiler);
  profiler.StartProfiling();
  m.SetInputs(1, 2);
  m.Invoke();
  // 3 = 1 + 2
  EXPECT_EQ(m.GetOutput(), 3);
  profiler.StopProfiling();
  ProfileSummarizer summarizer;
  auto events = profiler.GetProfileEvents();
  EXPECT_EQ(1, events.size());
  summarizer.ProcessProfiles(profiler.GetProfileEvents(), *interpreter);
  auto output = summarizer.GetOutputString();
  // TODO(shashishekhar): Add a better test here.
  ASSERT_TRUE(output.find("SimpleOpEval:Profile") != std::string::npos)
      << output;
}

#endif

}  // namespace
}  // namespace profiling
}  // namespace tflite

int main(int argc, char** argv) {
  ::tflite::LogToStderr();
  ::testing::InitGoogleTest(&argc, argv);
  return RUN_ALL_TESTS();
}
