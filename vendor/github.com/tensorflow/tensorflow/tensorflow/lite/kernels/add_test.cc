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
#include <gtest/gtest.h>
#include "tensorflow/lite/interpreter.h"
#include "tensorflow/lite/kernels/register.h"
#include "tensorflow/lite/kernels/test_util.h"
#include "tensorflow/lite/model.h"

namespace tflite {
namespace {

using ::testing::ElementsAreArray;

class BaseAddOpModel : public SingleOpModel {
 public:
  BaseAddOpModel(const TensorData& input1, const TensorData& input2,
                 const TensorData& output,
                 ActivationFunctionType activation_type) {
    input1_ = AddInput(input1);
    input2_ = AddInput(input2);
    output_ = AddOutput(output);
    SetBuiltinOp(BuiltinOperator_ADD, BuiltinOptions_AddOptions,
                 CreateAddOptions(builder_, activation_type).Union());
    BuildInterpreter({GetShape(input1_), GetShape(input2_)});
  }

  int input1() { return input1_; }
  int input2() { return input2_; }

 protected:
  int input1_;
  int input2_;
  int output_;
};

class FloatAddOpModel : public BaseAddOpModel {
 public:
  using BaseAddOpModel::BaseAddOpModel;

  std::vector<float> GetOutput() { return ExtractVector<float>(output_); }
};

class IntegerAddOpModel : public BaseAddOpModel {
 public:
  using BaseAddOpModel::BaseAddOpModel;

  std::vector<int32_t> GetOutput() { return ExtractVector<int32_t>(output_); }
};

class QuantizedAddOpModel : public BaseAddOpModel {
 public:
  using BaseAddOpModel::BaseAddOpModel;

  std::vector<float> GetDequantizedOutput() {
    return Dequantize<uint8_t>(ExtractVector<uint8_t>(output_),
                               GetScale(output_), GetZeroPoint(output_));
  }

  std::vector<float> GetDequantizedOutputInt16() {
    return Dequantize<int16_t>(ExtractVector<int16_t>(output_),
                               GetScale(output_), GetZeroPoint(output_));
  }
};

// for quantized Add, the error shouldn't exceed 2*step
float GetTolerance(float min, float max) {
  float kQuantizedStep = (max - min) / 255.0;
  float kQuantizedTolerance = 2.0 * kQuantizedStep;
  return kQuantizedTolerance;
}

float GetToleranceInt16(float min, float max) {
  float kQuantizedStep = (max - min) / 32767.f;
  float kQuantizedTolerance = 2.0 * kQuantizedStep;
  return kQuantizedTolerance;
}

TEST(FloatAddOpModel, NoActivation) {
  FloatAddOpModel m({TensorType_FLOAT32, {1, 2, 2, 1}},
                    {TensorType_FLOAT32, {1, 2, 2, 1}},
                    {TensorType_FLOAT32, {}}, ActivationFunctionType_NONE);
  m.PopulateTensor<float>(m.input1(), {-2.0, 0.2, 0.7, 0.8});
  m.PopulateTensor<float>(m.input2(), {0.1, 0.2, 0.3, 0.5});
  m.Invoke();
  EXPECT_THAT(m.GetOutput(), ElementsAreArray({-1.9, 0.4, 1.0, 1.3}));
}

TEST(FloatAddOpModel, ActivationRELU_N1_TO_1) {
  FloatAddOpModel m(
      {TensorType_FLOAT32, {1, 2, 2, 1}}, {TensorType_FLOAT32, {1, 2, 2, 1}},
      {TensorType_FLOAT32, {}}, ActivationFunctionType_RELU_N1_TO_1);
  m.PopulateTensor<float>(m.input1(), {-2.0, 0.2, 0.7, 0.8});
  m.PopulateTensor<float>(m.input2(), {0.1, 0.2, 0.3, 0.5});
  m.Invoke();
  EXPECT_THAT(m.GetOutput(), ElementsAreArray({-1.0, 0.4, 1.0, 1.0}));
}

TEST(FloatAddOpModel, VariousInputShapes) {
  std::vector<std::vector<int>> test_shapes = {
      {6}, {2, 3}, {2, 1, 3}, {1, 3, 1, 2}};
  for (int i = 0; i < test_shapes.size(); ++i) {
    FloatAddOpModel m({TensorType_FLOAT32, test_shapes[i]},
                      {TensorType_FLOAT32, test_shapes[i]},
                      {TensorType_FLOAT32, {}}, ActivationFunctionType_NONE);
    m.PopulateTensor<float>(m.input1(), {-2.0, 0.2, 0.7, 0.8, 1.1, 2.0});
    m.PopulateTensor<float>(m.input2(), {0.1, 0.2, 0.3, 0.5, 1.1, 0.1});
    m.Invoke();
    EXPECT_THAT(m.GetOutput(),
                ElementsAreArray({-1.9, 0.4, 1.0, 1.3, 2.2, 2.1}))
        << "With shape number " << i;
  }
}

TEST(FloatAddOpModel, WithBroadcast) {
  std::vector<std::vector<int>> test_shapes = {
      {6}, {2, 3}, {2, 1, 3}, {1, 3, 1, 2}};
  for (int i = 0; i < test_shapes.size(); ++i) {
    FloatAddOpModel m({TensorType_FLOAT32, test_shapes[i]},
                      {TensorType_FLOAT32, {}},  // always a scalar
                      {TensorType_FLOAT32, {}}, ActivationFunctionType_NONE);
    m.PopulateTensor<float>(m.input1(), {-2.0, 0.2, 0.7, 0.8, 1.1, 2.0});
    m.PopulateTensor<float>(m.input2(), {0.1});
    m.Invoke();
    EXPECT_THAT(
        m.GetOutput(),
        ElementsAreArray(ArrayFloatNear({-1.9, 0.3, 0.8, 0.9, 1.2, 2.1})))
        << "With shape number " << i;
  }
}

TEST(IntegerAddOpModel, NoActivation) {
  IntegerAddOpModel m({TensorType_INT32, {1, 2, 2, 1}},
                      {TensorType_INT32, {1, 2, 2, 1}}, {TensorType_INT32, {}},
                      ActivationFunctionType_NONE);
  m.PopulateTensor<int32_t>(m.input1(), {-20, 2, 7, 8});
  m.PopulateTensor<int32_t>(m.input2(), {1, 2, 3, 5});
  m.Invoke();
  EXPECT_THAT(m.GetOutput(), ElementsAreArray({-19, 4, 10, 13}));
}

TEST(IntegerAddOpModel, ActivationRELU_N1_TO_1) {
  IntegerAddOpModel m({TensorType_INT32, {1, 2, 2, 1}},
                      {TensorType_INT32, {1, 2, 2, 1}}, {TensorType_INT32, {}},
                      ActivationFunctionType_RELU_N1_TO_1);
  m.PopulateTensor<int32_t>(m.input1(), {-20, 2, 7, 8});
  m.PopulateTensor<int32_t>(m.input2(), {1, 2, 3, 5});
  m.Invoke();
  EXPECT_THAT(m.GetOutput(), ElementsAreArray({-1, 1, 1, 1}));
}

TEST(IntegerAddOpModel, VariousInputShapes) {
  std::vector<std::vector<int>> test_shapes = {
      {6}, {2, 3}, {2, 1, 3}, {1, 3, 1, 2}};
  for (int i = 0; i < test_shapes.size(); ++i) {
    IntegerAddOpModel m({TensorType_INT32, test_shapes[i]},
                        {TensorType_INT32, test_shapes[i]},
                        {TensorType_INT32, {}}, ActivationFunctionType_NONE);
    m.PopulateTensor<int32_t>(m.input1(), {-20, 2, 7, 8, 11, 20});
    m.PopulateTensor<int32_t>(m.input2(), {1, 2, 3, 5, 11, 1});
    m.Invoke();
    EXPECT_THAT(m.GetOutput(), ElementsAreArray({-19, 04, 10, 13, 22, 21}))
        << "With shape number " << i;
  }
}

TEST(IntegerAddOpModel, WithBroadcast) {
  std::vector<std::vector<int>> test_shapes = {
      {6}, {2, 3}, {2, 1, 3}, {1, 3, 1, 2}};
  for (int i = 0; i < test_shapes.size(); ++i) {
    IntegerAddOpModel m({TensorType_INT32, test_shapes[i]},
                        {TensorType_INT32, {}},  // always a scalar
                        {TensorType_INT32, {}}, ActivationFunctionType_NONE);
    m.PopulateTensor<int32_t>(m.input1(), {-20, 2, 7, 8, 11, 20});
    m.PopulateTensor<int32_t>(m.input2(), {1});
    m.Invoke();
    EXPECT_THAT(m.GetOutput(),
                ElementsAreArray(ArrayFloatNear({-19, 3, 8, 9, 12, 21})))
        << "With shape number " << i;
  }
}

TEST(QuantizedAddOpModel, QuantizedTestsNoActivation) {
  float kQuantizedTolerance = GetTolerance(-1.0, 1.0);
  std::vector<std::vector<float>> inputs1 = {
      {0.1, 0.2, 0.3, 0.4}, {-0.8, 0.2, 0.4, 0.7}, {-0.8, 0.2, 0.7, 0.3}};
  std::vector<std::vector<float>> inputs2 = {
      {0.6, 0.4, 0.3, 0.1}, {0.6, 0.4, 0.5, -0.8}, {0.6, 0.4, -0.8, 0.5}};
  std::vector<std::vector<float>> results = {
      {0.7, 0.6, 0.6, 0.5}, {-0.2, 0.6, 0.9, -0.1}, {-0.2, 0.6, -0.1, 0.8}};
  for (int i = 0; i < inputs1.size(); ++i) {
    QuantizedAddOpModel m({TensorType_UINT8, {1, 2, 2, 1}, -1.0, 1.0},
                          {TensorType_UINT8, {1, 2, 2, 1}, -1.0, 1.0},
                          {TensorType_UINT8, {}, -1.0, 1.0},
                          ActivationFunctionType_NONE);
    m.QuantizeAndPopulate<uint8_t>(m.input1(), inputs1[i]);
    m.QuantizeAndPopulate<uint8_t>(m.input2(), inputs2[i]);
    m.Invoke();
    EXPECT_THAT(m.GetDequantizedOutput(), ElementsAreArray(ArrayFloatNear(
                                              results[i], kQuantizedTolerance)))
        << "With test number " << i;
  }
}

TEST(QuantizedAddOpModel, QuantizedTestsNoActivationInt16) {
  const float kMin = -1.f;
  const float kMax = 32767.f / 32768.f;
  float kQuantizedTolerance = GetToleranceInt16(kMin, kMax);
  std::vector<std::vector<float>> inputs1 = {
      {0.1, 0.2, 0.3, 0.4}, {-0.8, 0.2, 0.4, 0.7}, {-0.8, 0.2, 0.7, 0.3}};
  std::vector<std::vector<float>> inputs2 = {
      {0.6, 0.4, 0.3, 0.1}, {0.6, 0.4, 0.5, -0.8}, {0.6, 0.4, -0.8, 0.5}};
  std::vector<std::vector<float>> results = {
      {0.7, 0.6, 0.6, 0.5}, {-0.2, 0.6, 0.9, -0.1}, {-0.2, 0.6, -0.1, 0.8}};
  for (int i = 0; i < inputs1.size(); ++i) {
    QuantizedAddOpModel m({TensorType_INT16, {1, 2, 2, 1}, kMin, kMax},
                          {TensorType_INT16, {1, 2, 2, 1}, kMin, kMax},
                          {TensorType_INT16, {}, kMin, kMax},
                          ActivationFunctionType_NONE);
    m.QuantizeAndPopulate<int16_t>(m.input1(), inputs1[i]);
    m.QuantizeAndPopulate<int16_t>(m.input2(), inputs2[i]);
    m.Invoke();
    EXPECT_THAT(
        m.GetDequantizedOutputInt16(),
        ElementsAreArray(ArrayFloatNear(results[i], kQuantizedTolerance)))
        << "With test number " << i;
  }
}

TEST(QuantizedAddOpModel, QuantizedTestsActivationRELU_N1_TO_1) {
  float kQuantizedTolerance = GetTolerance(-1.0, 1.0);
  std::vector<std::vector<float>> inputs1 = {{-0.8, 0.2, 0.9, 0.7},
                                             {-0.8, 0.2, 0.7, 0.3}};
  std::vector<std::vector<float>> inputs2 = {{0.6, 0.4, 0.9, -0.8},
                                             {0.6, 0.4, -0.8, 0.5}};
  std::vector<std::vector<float>> results = {{-0.2, 0.6, 1.0, -0.1},
                                             {-0.2, 0.6, -0.1, 0.8}};
  for (int i = 0; i < inputs1.size(); ++i) {
    QuantizedAddOpModel m({TensorType_UINT8, {1, 2, 2, 1}, -1.0, 1.0},
                          {TensorType_UINT8, {1, 2, 2, 1}, -1.0, 1.0},
                          {TensorType_UINT8, {}, -1.0, 1.0},
                          ActivationFunctionType_RELU_N1_TO_1);
    m.QuantizeAndPopulate<uint8_t>(m.input1(), inputs1[i]);
    m.QuantizeAndPopulate<uint8_t>(m.input2(), inputs2[i]);
    m.Invoke();
    EXPECT_THAT(m.GetDequantizedOutput(), ElementsAreArray(ArrayFloatNear(
                                              results[i], kQuantizedTolerance)))
        << "With test number " << i;
  }
}

TEST(QuantizedAddOpModel, QuantizedVariousInputShapes) {
  float kQuantizedTolerance = GetTolerance(-3.0, 3.0);
  std::vector<std::vector<int>> test_shapes = {
      {6}, {2, 3}, {2, 1, 3}, {1, 3, 1, 2}};
  for (int i = 0; i < test_shapes.size(); ++i) {
    QuantizedAddOpModel m({TensorType_UINT8, test_shapes[i], -3.0, 3.0},
                          {TensorType_UINT8, test_shapes[i], -3.0, 3.0},
                          {TensorType_UINT8, {}, -3.0, 3.0},
                          ActivationFunctionType_NONE);
    m.QuantizeAndPopulate<uint8_t>(m.input1(), {-2.0, 0.2, 0.7, 0.8, 1.1, 2.0});
    m.QuantizeAndPopulate<uint8_t>(m.input2(), {0.1, 0.3, 0.3, 0.5, 1.1, 0.1});
    m.Invoke();
    EXPECT_THAT(m.GetDequantizedOutput(),
                ElementsAreArray(ArrayFloatNear({-1.9, 0.5, 1.0, 1.3, 2.2, 2.1},
                                                kQuantizedTolerance)))
        << "With shape number " << i;
  }
}

TEST(QuantizedAddOpModel, QuantizedWithScalarBroadcast) {
  float kQuantizedTolerance = GetTolerance(-3.f, 3.f);
  std::vector<std::vector<int>> test_shapes = {
      {6}, {2, 3}, {2, 1, 3}, {1, 3, 1, 2}};
  for (int i = 0; i < test_shapes.size(); ++i) {
    QuantizedAddOpModel model_fixture(
        {TensorType_UINT8, test_shapes[i], -3.f, 3.f},
        {TensorType_UINT8, {}, -3.f, 3.f}, {TensorType_UINT8, {}, -3.f, 3.f},
        ActivationFunctionType_NONE);
    model_fixture.QuantizeAndPopulate<uint8_t>(
        model_fixture.input1(), {-2.0f, 0.2f, 0.7f, 0.8f, 1.1f, 2.0f});
    model_fixture.QuantizeAndPopulate<uint8_t>(model_fixture.input2(), {0.1f});
    model_fixture.Invoke();
    EXPECT_THAT(
        model_fixture.GetDequantizedOutput(),
        ElementsAreArray(ArrayFloatNear({-1.9f, 0.3f, 0.8f, 0.9f, 1.2f, 2.1f},
                                        kQuantizedTolerance)))
        << "With shape number " << i;
  }
  // Re-run with exchanged inputs.
  for (int i = 0; i < test_shapes.size(); ++i) {
    QuantizedAddOpModel model_fixture(
        {TensorType_UINT8, {}, -3.f, 3.f},
        {TensorType_UINT8, test_shapes[i], -3.f, 3.f},
        {TensorType_UINT8, {}, -3.f, 3.f}, ActivationFunctionType_NONE);
    model_fixture.QuantizeAndPopulate<uint8_t>(model_fixture.input1(), {0.1f});
    model_fixture.QuantizeAndPopulate<uint8_t>(
        model_fixture.input2(), {-2.0f, 0.2f, 0.7f, 0.8f, 1.1f, 2.0f});
    model_fixture.Invoke();
    EXPECT_THAT(
        model_fixture.GetDequantizedOutput(),
        ElementsAreArray(ArrayFloatNear({-1.9f, 0.3f, 0.8f, 0.9f, 1.2f, 2.1f},
                                        kQuantizedTolerance)))
        << "With shape number " << i;
  }
}

TEST(QuantizedAddOpModel, QuantizedWithMixedBroadcast) {
  float kQuantizedTolerance = GetTolerance(-3.f, 3.f);
  const std::vector<int> base_shape = {2, 3, 1, 2};
  std::vector<std::vector<int>> test_shapes = {
      {1, 1, 3, 2}, {1, 3, 1, 2}, {2, 1, 3, 1}, {2, 3, 1, 1}};
  std::vector<std::vector<float>> test_outputs = {
      {-0.1f, 2.6f,  -0.7f, 2.8f, 0.7f,  3.0f, 1.1f,  0.8f, 0.5f,
       1.0f,  1.9f,  1.4f,  1.0f, -0.8f, 0.4f, -0.6f, 1.8f, -0.2f,
       1.4f,  3.0f,  0.8f,  3.0f, 2.2f,  3.0f, -1.4f, 0.3f, -2.0f,
       0.5f,  -0.6f, 0.9f,  0.9f, -1.9f, 0.3f, -1.7f, 1.7f, -1.3f},
      {-0.1f, 2.6f, 0.5f, 1.0f, 1.8f, -0.2f, 1.4f, 3.0f, -2.0f, 0.5f, 1.7f,
       -1.3f},
      {-0.1f, 2.5f,  0.0f, 2.6f, -0.7f, 1.9f, 1.1f,  0.7f, 1.2f,
       0.8f,  0.5f,  0.1f, 1.0f, -0.9f, 1.1f, -0.8f, 0.4f, -1.5f,
       1.7f,  3.0f,  2.2f, 3.0f, 2.1f,  3.0f, -1.1f, 0.5f, -0.6f,
       1.0f,  -0.7f, 0.9f, 1.2f, -1.7f, 1.7f, -1.2f, 1.6f, -1.3f},
      {-0.1f, 2.5f, 1.2f, 0.8f, 0.4f, -1.5f, 1.7f, 3.0f, -0.6f, 1.0f, 1.6f,
       -1.3f}};
  for (int i = 0; i < test_shapes.size(); ++i) {
    QuantizedAddOpModel model_fixture(
        {TensorType_UINT8, base_shape, -3.f, 3.f},
        {TensorType_UINT8, test_shapes[i], -3.f, 3.f},
        {TensorType_UINT8, {}, -3.f, 3.f}, ActivationFunctionType_NONE);
    model_fixture.QuantizeAndPopulate<uint8_t>(
        model_fixture.input1(), {-0.3f, 2.3f, 0.9f, 0.5f, 0.8f, -1.1f, 1.2f,
                                 2.8f, -1.6f, 0.0f, 0.7f, -2.2f});
    model_fixture.QuantizeAndPopulate<uint8_t>(
        model_fixture.input2(), {0.2f, 0.3f, -0.4f, 0.5f, 1.0f, 0.9f});
    model_fixture.Invoke();
    EXPECT_THAT(
        model_fixture.GetDequantizedOutput(),
        ElementsAreArray(ArrayFloatNear(test_outputs[i], kQuantizedTolerance)))
        << "With shape number " << i;
  }
  // Re-run with exchanged inputs.
  for (int i = 0; i < test_shapes.size(); ++i) {
    QuantizedAddOpModel model_fixture(
        {TensorType_UINT8, test_shapes[i], -3.f, 3.f},
        {TensorType_UINT8, base_shape, -3.f, 3.f},
        {TensorType_UINT8, {}, -3.f, 3.f}, ActivationFunctionType_NONE);
    model_fixture.QuantizeAndPopulate<uint8_t>(
        model_fixture.input1(), {0.2f, 0.3f, -0.4f, 0.5f, 1.0f, 0.9f});
    model_fixture.QuantizeAndPopulate<uint8_t>(
        model_fixture.input2(), {-0.3f, 2.3f, 0.9f, 0.5f, 0.8f, -1.1f, 1.2f,
                                 2.8f, -1.6f, 0.0f, 0.7f, -2.2f});
    model_fixture.Invoke();
    EXPECT_THAT(
        model_fixture.GetDequantizedOutput(),
        ElementsAreArray(ArrayFloatNear(test_outputs[i], kQuantizedTolerance)))
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
