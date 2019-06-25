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
#include <gtest/gtest.h>
#include "tensorflow/lite/interpreter.h"
#include "tensorflow/lite/kernels/register.h"
#include "tensorflow/lite/kernels/test_util.h"
#include "tensorflow/lite/model.h"

namespace tflite {
namespace {

using ::testing::ElementsAre;

class ComparisonOpModel : public SingleOpModel {
 public:
  ComparisonOpModel(std::initializer_list<int> input1_shape,
                    std::initializer_list<int> input2_shape,
                    TensorType input_type, BuiltinOperator op) {
    input1_ = AddInput(input_type);
    input2_ = AddInput(input_type);
    output_ = AddOutput(TensorType_BOOL);
    ConfigureBuiltinOp(op);
    BuildInterpreter({input1_shape, input2_shape});
  }

  ComparisonOpModel(const TensorData& input1, const TensorData& input2,
                    TensorType input_type, BuiltinOperator op) {
    input1_ = AddInput(input1);
    input2_ = AddInput(input2);
    output_ = AddOutput(TensorType_BOOL);
    ConfigureBuiltinOp(op);
    BuildInterpreter({GetShape(input1_), GetShape(input2_)});
  }

  int input1() { return input1_; }
  int input2() { return input2_; }

  std::vector<bool> GetOutput() { return ExtractVector<bool>(output_); }
  std::vector<int> GetOutputShape() { return GetTensorShape(output_); }

 private:
  int input1_;
  int input2_;
  int output_;

  void ConfigureBuiltinOp(BuiltinOperator op) {
    switch (op) {
      case BuiltinOperator_EQUAL: {
        SetBuiltinOp(op, BuiltinOptions_EqualOptions,
                     CreateEqualOptions(builder_).Union());
        break;
      }
      case BuiltinOperator_NOT_EQUAL: {
        SetBuiltinOp(op, BuiltinOptions_NotEqualOptions,
                     CreateNotEqualOptions(builder_).Union());
        break;
      }
      case BuiltinOperator_GREATER: {
        SetBuiltinOp(op, BuiltinOptions_GreaterOptions,
                     CreateGreaterOptions(builder_).Union());
        break;
      }
      case BuiltinOperator_GREATER_EQUAL: {
        SetBuiltinOp(op, BuiltinOptions_GreaterEqualOptions,
                     CreateGreaterEqualOptions(builder_).Union());
        break;
      }
      case BuiltinOperator_LESS: {
        SetBuiltinOp(op, BuiltinOptions_LessOptions,
                     CreateLessOptions(builder_).Union());
        break;
      }
      case BuiltinOperator_LESS_EQUAL: {
        SetBuiltinOp(op, BuiltinOptions_LessEqualOptions,
                     CreateLessEqualOptions(builder_).Union());
        break;
      }
      default: { FAIL() << "We shouldn't get here."; }
    }
  }
};

TEST(ComparisonsTest, EqualFloat) {
  ComparisonOpModel model({1, 1, 1, 4}, {1, 1, 1, 4}, TensorType_FLOAT32,
                          BuiltinOperator_EQUAL);
  model.PopulateTensor<float>(model.input1(), {0.1, 0.9, 0.7, 0.3});
  model.PopulateTensor<float>(model.input2(), {0.1, 0.2, 0.6, 0.5});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(true, false, false, false));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 1, 4));
}

TEST(ComparisonsTest, EqualInt) {
  ComparisonOpModel model({1, 1, 1, 4}, {1, 1, 1, 4}, TensorType_INT32,
                          BuiltinOperator_EQUAL);
  model.PopulateTensor<int>(model.input1(), {-1, 9, 7, 3});
  model.PopulateTensor<int>(model.input2(), {1, 2, 7, 5});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(false, false, true, false));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 1, 4));
}

TEST(ComparisonsTest, EqualBroadcast) {
  ComparisonOpModel model({1, 1, 1, 4}, {1, 1, 1, 1}, TensorType_INT32,
                          BuiltinOperator_EQUAL);
  model.PopulateTensor<int>(model.input1(), {-1, 9, 7, 3});
  model.PopulateTensor<int>(model.input2(), {7});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(false, false, true, false));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 1, 4));
}

TEST(ComparisonsTest, EqualBroadcastTwoD) {
  ComparisonOpModel model({1, 1, 2, 4}, {1, 1, 1, 4}, TensorType_INT32,
                          BuiltinOperator_EQUAL);
  model.PopulateTensor<int>(model.input1(), {-1, 9, 7, 3, 2, 4, 2, 8});
  model.PopulateTensor<int>(model.input2(), {7, 1, 2, 4});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(false, false, false, false, false,
                                             false, true, false));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 2, 4));
}

TEST(ComparisonsTest, NotEqualFloat) {
  ComparisonOpModel model({1, 1, 1, 4}, {1, 1, 1, 4}, TensorType_FLOAT32,
                          BuiltinOperator_NOT_EQUAL);
  model.PopulateTensor<float>(model.input1(), {0.1, 0.9, 0.7, 0.3});
  model.PopulateTensor<float>(model.input2(), {0.1, 0.2, 0.6, 0.5});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(false, true, true, true));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 1, 4));
}

TEST(ComparisonsTest, NotEqualInt) {
  ComparisonOpModel model({1, 1, 1, 4}, {1, 1, 1, 4}, TensorType_INT32,
                          BuiltinOperator_NOT_EQUAL);
  model.PopulateTensor<int>(model.input1(), {-1, 9, 7, 3});
  model.PopulateTensor<int>(model.input2(), {1, 2, 7, 5});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(true, true, false, true));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 1, 4));
}

TEST(ComparisonsTest, NotEqualBroadcast) {
  ComparisonOpModel model({1, 1, 1, 4}, {1, 1, 1, 1}, TensorType_INT32,
                          BuiltinOperator_NOT_EQUAL);
  model.PopulateTensor<int>(model.input1(), {-1, 9, 7, 3});
  model.PopulateTensor<int>(model.input2(), {7});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(true, true, false, true));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 1, 4));
}

TEST(ComparisonsTest, NotEqualBroadcastTwoD) {
  ComparisonOpModel model({1, 1, 2, 4}, {1, 1, 1, 4}, TensorType_INT32,
                          BuiltinOperator_NOT_EQUAL);
  model.PopulateTensor<int>(model.input1(), {-1, 9, 7, 3, 2, 4, 2, 8});
  model.PopulateTensor<int>(model.input2(), {7, 1, 2, 4});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(),
              ElementsAre(true, true, true, true, true, true, false, true));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 2, 4));
}

TEST(ComparisonsTest, GreaterFloat) {
  ComparisonOpModel model({1, 1, 1, 4}, {1, 1, 1, 4}, TensorType_FLOAT32,
                          BuiltinOperator_GREATER);
  model.PopulateTensor<float>(model.input1(), {0.1, 0.9, 0.7, 0.3});
  model.PopulateTensor<float>(model.input2(), {0.1, 0.2, 0.6, 0.5});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(false, true, true, false));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 1, 4));
}

TEST(ComparisonsTest, GreaterInt) {
  ComparisonOpModel model({1, 1, 1, 4}, {1, 1, 1, 4}, TensorType_INT32,
                          BuiltinOperator_GREATER);
  model.PopulateTensor<int>(model.input1(), {-1, 9, 7, 3});
  model.PopulateTensor<int>(model.input2(), {1, 2, 7, 5});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(false, true, false, false));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 1, 4));
}

TEST(ComparisonsTest, GreaterBroadcast) {
  ComparisonOpModel model({1, 1, 1, 4}, {1, 1, 1, 1}, TensorType_INT32,
                          BuiltinOperator_GREATER);
  model.PopulateTensor<int>(model.input1(), {-1, 9, 7, 3});
  model.PopulateTensor<int>(model.input2(), {7});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(false, true, false, false));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 1, 4));
}

TEST(ComparisonsTest, GreaterBroadcastTwoD) {
  ComparisonOpModel model({1, 1, 2, 4}, {1, 1, 1, 4}, TensorType_INT32,
                          BuiltinOperator_GREATER);
  model.PopulateTensor<int>(model.input1(), {-1, 9, 7, 3, 2, 4, 2, 8});
  model.PopulateTensor<int>(model.input2(), {7, 1, 2, 4});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(),
              ElementsAre(false, true, true, false, false, true, false, true));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 2, 4));
}

TEST(ComparisonsTest, GreaterEqualFloat) {
  ComparisonOpModel model({1, 1, 1, 4}, {1, 1, 1, 4}, TensorType_FLOAT32,
                          BuiltinOperator_GREATER_EQUAL);
  model.PopulateTensor<float>(model.input1(), {0.1, 0.9, 0.7, 0.3});
  model.PopulateTensor<float>(model.input2(), {0.1, 0.2, 0.6, 0.5});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(true, true, true, false));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 1, 4));
}

TEST(ComparisonsTest, GreaterEqualInt) {
  ComparisonOpModel model({1, 1, 1, 4}, {1, 1, 1, 4}, TensorType_INT32,
                          BuiltinOperator_GREATER_EQUAL);
  model.PopulateTensor<int>(model.input1(), {-1, 9, 7, 3});
  model.PopulateTensor<int>(model.input2(), {1, 2, 7, 5});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(false, true, true, false));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 1, 4));
}

TEST(ComparisonsTest, GreaterEqualBroadcast) {
  ComparisonOpModel model({1, 1, 1, 4}, {1, 1, 1, 1}, TensorType_INT32,
                          BuiltinOperator_GREATER_EQUAL);
  model.PopulateTensor<int>(model.input1(), {-1, 9, 7, 3});
  model.PopulateTensor<int>(model.input2(), {7});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(false, true, true, false));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 1, 4));
}

TEST(ComparisonsTest, GreaterEqualBroadcastTwoD) {
  ComparisonOpModel model({1, 1, 2, 4}, {1, 1, 1, 4}, TensorType_INT32,
                          BuiltinOperator_GREATER_EQUAL);
  model.PopulateTensor<int>(model.input1(), {-1, 9, 7, 3, 2, 4, 2, 8});
  model.PopulateTensor<int>(model.input2(), {7, 1, 2, 4});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(),
              ElementsAre(false, true, true, false, false, true, true, true));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 2, 4));
}


TEST(ComparisonsTest, LessFloat) {
  ComparisonOpModel model({1, 1, 1, 4}, {1, 1, 1, 4}, TensorType_FLOAT32,
                          BuiltinOperator_LESS);
  model.PopulateTensor<float>(model.input1(), {0.1, 0.9, 0.7, 0.3});
  model.PopulateTensor<float>(model.input2(), {0.1, 0.2, 0.6, 0.5});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(false, false, false, true));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 1, 4));
}

TEST(ComparisonsTest, LessInt) {
  ComparisonOpModel model({1, 1, 1, 4}, {1, 1, 1, 4}, TensorType_INT32,
                          BuiltinOperator_LESS);
  model.PopulateTensor<int>(model.input1(), {-1, 9, 7, 3});
  model.PopulateTensor<int>(model.input2(), {1, 2, 6, 5});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(true, false, false, true));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 1, 4));
}

TEST(ComparisonsTest, LessBroadcast) {
  ComparisonOpModel model({1, 1, 1, 4}, {1, 1, 1, 1}, TensorType_INT32,
                          BuiltinOperator_LESS);
  model.PopulateTensor<int>(model.input1(), {-1, 9, 7, 3});
  model.PopulateTensor<int>(model.input2(), {7});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(true, false, false, true));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 1, 4));
}

TEST(ComparisonsTest, LessBroadcastTwoD) {
  ComparisonOpModel model({1, 1, 2, 4}, {1, 1, 1, 4}, TensorType_INT32,
                          BuiltinOperator_LESS);
  model.PopulateTensor<int>(model.input1(), {-1, 9, 7, 3, 2, 4, 6, 8});
  model.PopulateTensor<int>(model.input2(), {7, 1, 2, 4});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(),
              ElementsAre(true, false, false, true, true, false, false, false));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 2, 4));
}

TEST(ComparisonsTest, LessEqualFloat) {
  ComparisonOpModel model({1, 1, 1, 4}, {1, 1, 1, 4}, TensorType_FLOAT32,
                          BuiltinOperator_LESS_EQUAL);
  model.PopulateTensor<float>(model.input1(), {0.1, 0.9, 0.7, 0.3});
  model.PopulateTensor<float>(model.input2(), {0.1, 0.2, 0.6, 0.5});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(true, false, false, true));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 1, 4));
}

TEST(ComparisonsTest, LessEqualInt) {
  ComparisonOpModel model({1, 1, 1, 4}, {1, 1, 1, 4}, TensorType_INT32,
                          BuiltinOperator_LESS_EQUAL);
  model.PopulateTensor<int>(model.input1(), {-1, 9, 7, 3});
  model.PopulateTensor<int>(model.input2(), {1, 2, 7, 5});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(true, false, true, true));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 1, 4));
}

TEST(ComparisonsTest, LessEqualBroadcast) {
  ComparisonOpModel model({1, 1, 1, 4}, {1, 1, 1, 1}, TensorType_INT32,
                          BuiltinOperator_LESS_EQUAL);
  model.PopulateTensor<int>(model.input1(), {-1, 9, 7, 3});
  model.PopulateTensor<int>(model.input2(), {7});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(true, false, true, true));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 1, 4));
}

TEST(ComparisonsTest, LessEqualBroadcastTwoD) {
  ComparisonOpModel model({1, 1, 2, 4}, {1, 1, 1, 4}, TensorType_INT32,
                          BuiltinOperator_LESS_EQUAL);
  model.PopulateTensor<int>(model.input1(), {-1, 9, 7, 3, 2, 4, 2, 8});
  model.PopulateTensor<int>(model.input2(), {7, 1, 2, 4});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(),
              ElementsAre(true, false, false, true, true, false, true, false));
  EXPECT_THAT(model.GetOutputShape(), ElementsAre(1, 1, 2, 4));
}

TEST(QuantizedComparisonsTest, EqualQuantized) {
  const float kMin = -1.f;
  const float kMax = 128.f;
  ComparisonOpModel model({TensorType_UINT8, {1, 2, 2, 1}, kMin, kMax},
                          {TensorType_UINT8, {1, 2, 2, 1}, kMin, kMax},
                          TensorType_UINT8, BuiltinOperator_EQUAL);
  model.QuantizeAndPopulate<uint8_t>(model.input1(), {1, 9, 7, 3});
  model.QuantizeAndPopulate<uint8_t>(model.input2(), {1, 2, 7, 5});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(true, false, true, false));
}

TEST(QuantizedComparisonsTest, NotEqualQuantized) {
  const float kMin = -1.f;
  const float kMax = 128.f;
  ComparisonOpModel model({TensorType_UINT8, {1, 2, 2, 1}, kMin, kMax},
                          {TensorType_UINT8, {1, 2, 2, 1}, kMin, kMax},
                          TensorType_UINT8, BuiltinOperator_NOT_EQUAL);
  model.QuantizeAndPopulate<uint8_t>(model.input1(), {1, 9, 7, 3});
  model.QuantizeAndPopulate<uint8_t>(model.input2(), {1, 2, 7, 0});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(false, true, false, true));
}

TEST(ComparisonsTest, GreaterQuantized) {
  const float kMin = -1.f;
  const float kMax = 128.f;
  ComparisonOpModel model({TensorType_UINT8, {1, 2, 2, 1}, kMin, kMax},
                          {TensorType_UINT8, {1, 2, 2, 1}, kMin, kMax},
                          TensorType_UINT8, BuiltinOperator_GREATER);
  model.QuantizeAndPopulate<uint8_t>(model.input1(), {1, 9, 7, 3});
  model.QuantizeAndPopulate<uint8_t>(model.input2(), {1, 2, 6, 5});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(false, true, true, false));
}

TEST(ComparisonsTest, GreaterQuantizedSmallRange) {
  ComparisonOpModel model({TensorType_UINT8, {1, 2, 2, 1}, 0.0, 1.0},
                          {TensorType_UINT8, {1, 2, 2, 1}, 0.0, 2.0},
                          TensorType_UINT8, BuiltinOperator_GREATER);
  model.QuantizeAndPopulate<uint8_t>(model.input1(), {1.0, 0.5, 0.35, 0.1});
  model.QuantizeAndPopulate<uint8_t>(model.input2(), {1.01, 0.25, 0.3, 0.4});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(false, true, true, false));
}

TEST(ComparisonsTest, GreaterEqualQuantized) {
  const float kMin = -1.f;
  const float kMax = 128.f;
  ComparisonOpModel model({TensorType_UINT8, {1, 2, 2, 1}, kMin, kMax},
                          {TensorType_UINT8, {1, 2, 2, 1}, kMin, kMax},
                          TensorType_UINT8, BuiltinOperator_GREATER_EQUAL);
  model.QuantizeAndPopulate<uint8_t>(model.input1(), {1, 9, 7, 3});
  model.QuantizeAndPopulate<uint8_t>(model.input2(), {1, 2, 6, 5});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(true, true, true, false));
}

TEST(ComparisonsTest, LessQuantized) {
  const float kMin = -1.f;
  const float kMax = 128.f;
  ComparisonOpModel model({TensorType_UINT8, {1, 2, 2, 1}, kMin, kMax},
                          {TensorType_UINT8, {1, 2, 2, 1}, kMin, kMax},
                          TensorType_UINT8, BuiltinOperator_LESS);
  model.QuantizeAndPopulate<uint8_t>(model.input1(), {1, 9, 7, 3});
  model.QuantizeAndPopulate<uint8_t>(model.input2(), {1, 2, 6, 5});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(false, false, false, true));
}

TEST(ComparisonsTest, LessEqualQuantized) {
  const float kMin = -1.f;
  const float kMax = 128.f;
  ComparisonOpModel model({TensorType_UINT8, {1, 2, 2, 1}, kMin, kMax},
                          {TensorType_UINT8, {1, 2, 2, 1}, kMin, kMax},
                          TensorType_UINT8, BuiltinOperator_LESS_EQUAL);
  model.QuantizeAndPopulate<uint8_t>(model.input1(), {1, 9, 7, 3});
  model.QuantizeAndPopulate<uint8_t>(model.input2(), {1, 2, 6, 5});
  model.Invoke();

  EXPECT_THAT(model.GetOutput(), ElementsAre(true, false, false, true));
}

TEST(ComparisonsTest, QuantizedEqualWithBroadcast) {
  const float kMin = -1.f;
  const float kMax = 128.f;
  std::vector<std::vector<int>> test_shapes = {
      {6}, {2, 3}, {2, 1, 3}, {1, 3, 1, 2}};
  for (int i = 0; i < test_shapes.size(); ++i) {
    ComparisonOpModel model({TensorType_UINT8, test_shapes[i], kMin, kMax},
                            {TensorType_UINT8, {}, kMin, kMax},
                            TensorType_UINT8, BuiltinOperator_EQUAL);
    model.QuantizeAndPopulate<uint8_t>(model.input1(), {20, 2, 7, 8, 11, 20});
    model.QuantizeAndPopulate<uint8_t>(model.input2(), {2});
    model.Invoke();
    EXPECT_THAT(model.GetOutput(),
                ElementsAre(false, true, false, false, false, false))
        << "With shape number " << i;
  }
}

TEST(ComparisonsTest, QuantizedNotEqualWithBroadcast) {
  const float kMin = -1.f;
  const float kMax = 128.f;
  std::vector<std::vector<int>> test_shapes = {
      {6}, {2, 3}, {2, 1, 3}, {1, 3, 1, 2}};
  for (int i = 0; i < test_shapes.size(); ++i) {
    ComparisonOpModel model({TensorType_UINT8, test_shapes[i], kMin, kMax},
                            {TensorType_UINT8, {}, kMin, kMax},
                            TensorType_UINT8, BuiltinOperator_NOT_EQUAL);
    model.QuantizeAndPopulate<uint8_t>(model.input1(), {20, 2, 7, 8, 11, 20});
    model.QuantizeAndPopulate<uint8_t>(model.input2(), {2});
    model.Invoke();
    EXPECT_THAT(model.GetOutput(),
                ElementsAre(true, false, true, true, true, true))
        << "With shape number " << i;
  }
}

TEST(ComparisonsTest, QuantizedGreaterWithBroadcast) {
  const float kMin = -1.f;
  const float kMax = 128.f;
  std::vector<std::vector<int>> test_shapes = {
      {6}, {2, 3}, {2, 1, 3}, {1, 3, 1, 2}};
  for (int i = 0; i < test_shapes.size(); ++i) {
    ComparisonOpModel model({TensorType_UINT8, test_shapes[i], kMin, kMax},
                            {TensorType_UINT8, {}, kMin, kMax},
                            TensorType_UINT8, BuiltinOperator_GREATER);
    model.QuantizeAndPopulate<uint8_t>(model.input1(), {20, 2, 7, 8, 11, 20});
    model.QuantizeAndPopulate<uint8_t>(model.input2(), {8});
    model.Invoke();
    EXPECT_THAT(model.GetOutput(),
                ElementsAre(true, false, false, false, true, true))
        << "With shape number " << i;
  }
}

TEST(ComparisonsTest, QuantizedGreaterEqualWithBroadcast) {
  const float kMin = -1.f;
  const float kMax = 128.f;
  std::vector<std::vector<int>> test_shapes = {
      {6}, {2, 3}, {2, 1, 3}, {1, 3, 1, 2}};
  for (int i = 0; i < test_shapes.size(); ++i) {
    ComparisonOpModel model({TensorType_UINT8, test_shapes[i], kMin, kMax},
                            {TensorType_UINT8, {}, kMin, kMax},
                            TensorType_UINT8, BuiltinOperator_GREATER_EQUAL);
    model.QuantizeAndPopulate<uint8_t>(model.input1(), {20, 2, 7, 8, 11, 20});
    model.QuantizeAndPopulate<uint8_t>(model.input2(), {8});
    model.Invoke();
    EXPECT_THAT(model.GetOutput(),
                ElementsAre(true, false, false, true, true, true))
        << "With shape number " << i;
  }
}

TEST(ComparisonsTest, QuantizedLessWithBroadcast) {
  const float kMin = -1.f;
  const float kMax = 128.f;
  std::vector<std::vector<int>> test_shapes = {
      {6}, {2, 3}, {2, 1, 3}, {1, 3, 1, 2}};
  for (int i = 0; i < test_shapes.size(); ++i) {
    ComparisonOpModel model({TensorType_UINT8, test_shapes[i], kMin, kMax},
                            {TensorType_UINT8, {}, kMin, kMax},
                            TensorType_UINT8, BuiltinOperator_LESS);
    model.QuantizeAndPopulate<uint8_t>(model.input1(), {20, 2, 7, 8, 11, 20});
    model.QuantizeAndPopulate<uint8_t>(model.input2(), {8});
    model.Invoke();
    EXPECT_THAT(model.GetOutput(),
                ElementsAre(false, true, true, false, false, false))
        << "With shape number " << i;
  }
}

TEST(ComparisonsTest, QuantizedLessEqualWithBroadcast) {
  const float kMin = -1.f;
  const float kMax = 128.f;
  std::vector<std::vector<int>> test_shapes = {
      {6}, {2, 3}, {2, 1, 3}, {1, 3, 1, 2}};
  for (int i = 0; i < test_shapes.size(); ++i) {
    ComparisonOpModel model({TensorType_UINT8, test_shapes[i], kMin, kMax},
                            {TensorType_UINT8, {}, kMin, kMax},
                            TensorType_UINT8, BuiltinOperator_LESS_EQUAL);
    model.QuantizeAndPopulate<uint8_t>(model.input1(), {20, 2, 7, 8, 11, 20});
    model.QuantizeAndPopulate<uint8_t>(model.input2(), {8});
    model.Invoke();
    EXPECT_THAT(model.GetOutput(),
                ElementsAre(false, true, true, true, false, false))
        << "With shape number " << i;
  }
}

}  // namespace
}  // namespace tflite

int main(int argc, char** argv) {
  ::tflite::LogToStderr();
  ::testing::InitGoogleTest(&argc, argv);
  return RUN_ALL_TESTS();
}
