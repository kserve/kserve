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

#include "tensorflow/lite/experimental/micro/micro_mutable_op_resolver.h"

#include "tensorflow/lite/experimental/micro/testing/micro_test.h"

namespace tflite {
namespace {
void* MockInit(TfLiteContext* context, const char* buffer, size_t length) {
  // Do nothing.
  return nullptr;
}

void MockFree(TfLiteContext* context, void* buffer) {
  // Do nothing.
}

TfLiteStatus MockPrepare(TfLiteContext* context, TfLiteNode* node) {
  return kTfLiteOk;
}

TfLiteStatus MockInvoke(TfLiteContext* context, TfLiteNode* node) {
  return kTfLiteOk;
}
}  // namespace
}  // namespace tflite

TF_LITE_MICRO_TESTS_BEGIN

TF_LITE_MICRO_TEST(TestOperations) {
  using tflite::BuiltinOperator_CONV_2D;
  using tflite::BuiltinOperator_RELU;
  using tflite::MicroMutableOpResolver;
  using tflite::OpResolver;

  static TfLiteRegistration r = {tflite::MockInit, tflite::MockFree,
                                 tflite::MockPrepare, tflite::MockInvoke};

  MicroMutableOpResolver micro_mutable_op_resolver;
  micro_mutable_op_resolver.AddBuiltin(BuiltinOperator_CONV_2D, &r, 0, 2);
  micro_mutable_op_resolver.AddCustom("mock_custom", &r, 0, 3);
  OpResolver* resolver = &micro_mutable_op_resolver;

  const TfLiteRegistration* registration =
      resolver->FindOp(BuiltinOperator_CONV_2D, 0);
  TF_LITE_MICRO_EXPECT_NE(nullptr, registration);
  TF_LITE_MICRO_EXPECT_EQ(nullptr, registration->init(nullptr, nullptr, 0));
  TF_LITE_MICRO_EXPECT_EQ(kTfLiteOk, registration->prepare(nullptr, nullptr));
  TF_LITE_MICRO_EXPECT_EQ(kTfLiteOk, registration->invoke(nullptr, nullptr));

  registration = resolver->FindOp(BuiltinOperator_CONV_2D, 10);
  TF_LITE_MICRO_EXPECT_EQ(nullptr, registration);

  registration = resolver->FindOp(BuiltinOperator_RELU, 0);
  TF_LITE_MICRO_EXPECT_EQ(nullptr, registration);

  registration = resolver->FindOp("mock_custom", 0);
  TF_LITE_MICRO_EXPECT_NE(nullptr, registration);
  TF_LITE_MICRO_EXPECT_EQ(nullptr, registration->init(nullptr, nullptr, 0));
  TF_LITE_MICRO_EXPECT_EQ(kTfLiteOk, registration->prepare(nullptr, nullptr));
  TF_LITE_MICRO_EXPECT_EQ(kTfLiteOk, registration->invoke(nullptr, nullptr));

  registration = resolver->FindOp("mock_custom", 10);
  TF_LITE_MICRO_EXPECT_EQ(nullptr, registration);

  registration = resolver->FindOp("nonexistent_custom", 0);
  TF_LITE_MICRO_EXPECT_EQ(nullptr, registration);
}

TF_LITE_MICRO_TESTS_END
