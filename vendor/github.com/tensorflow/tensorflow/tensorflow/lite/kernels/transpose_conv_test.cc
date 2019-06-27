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
#include <cstdarg>
#include <gtest/gtest.h>
#include "absl/memory/memory.h"
#include "tensorflow/lite/interpreter.h"
#include "tensorflow/lite/kernels/register.h"
#include "tensorflow/lite/kernels/test_util.h"
#include "tensorflow/lite/model.h"

namespace tflite {

namespace ops {
namespace builtin {

TfLiteRegistration* Register_TRANSPOSECONV_REF();
TfLiteRegistration* Register_TRANSPOSECONV_GENERIC_OPT();

}  // namespace builtin
}  // namespace ops

namespace {

using ::testing::ElementsAreArray;

class TransposeConvOpModel : public SingleOpModel {
 public:
  TransposeConvOpModel(TfLiteRegistration* registration,
                       const TensorData& filter, const TensorData& input,
                       const TensorData& output, Padding padding, int stride_w,
                       int stride_h) {
    // Just to be confusing, transpose_conv has an _input_ named "output_shape"
    // that sets the shape of the output tensor of the op :). It must always be
    // an int32 1D four element tensor.
    output_shape_ = AddInput({TensorType_INT32, {4}});
    filter_ = AddInput(filter);
    input_ = AddInput(input);

    output_ = AddOutput(output);

    SetBuiltinOp(
        BuiltinOperator_TRANSPOSE_CONV, BuiltinOptions_TransposeConvOptions,
        CreateTransposeConvOptions(builder_, padding, stride_w, stride_h)
            .Union());
    resolver_ = absl::make_unique<SingleOpResolver>(
        BuiltinOperator_TRANSPOSE_CONV, registration);
    BuildInterpreter(
        {GetShape(output_shape_), GetShape(input_), GetShape(filter_)});
  }

  void SetOutputShape(std::initializer_list<int> i) {
    PopulateTensor(output_shape_, i);
  }
  void SetFilter(std::initializer_list<float> f) { PopulateTensor(filter_, f); }
  void SetInput(std::initializer_list<float> data) {
    PopulateTensor(input_, data);
  }
  std::vector<float> GetOutput() { return ExtractVector<float>(output_); }
  std::vector<int> GetOutputShape() { return GetTensorShape(output_); }

 private:
  int output_shape_;
  int filter_;
  int input_;
  int output_;
};

const auto kKernelMap = new std::map<string, TfLiteRegistration*>({
    {"Reference", ops::builtin::Register_TRANSPOSECONV_REF()},
    {"GenericOptimized", ops::builtin::Register_TRANSPOSECONV_GENERIC_OPT()},
});

class TransposeConvOpTest : public SingleOpTest {
 protected:
  const std::map<string, TfLiteRegistration*>& GetKernelMap() override {
    return *kKernelMap;
  }
};

// Test case:
// output = tf.nn.conv2d_backprop_input(
//     tf.constant([ 1, 4, 4, 1 ]),
//     tf.constant(np.arange(1, 10), shape=[ 3, 3, 1, 1 ], dtype=tf.float32),
//     tf.constant(np.arange(1, 17), shape=[ 1, 4, 4, 1 ], dtype=tf.float32),
//     [1, 1, 1, 1 ],
//     "SAME")
TEST_P(TransposeConvOpTest, SimpleTest) {
  TransposeConvOpModel m(GetRegistration(), {TensorType_FLOAT32, {1, 4, 4, 1}},
                         {TensorType_FLOAT32, {1, 3, 3, 1}},
                         {TensorType_FLOAT32, {}}, Padding_SAME, 1, 1);
  m.SetOutputShape({1, 4, 4, 1});
  m.SetFilter({1, 2, 3, 4, 5, 6, 7, 8, 9});
  m.SetInput({1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16});
  m.Invoke();

  EXPECT_THAT(m.GetOutput(),
              ElementsAreArray({29, 62, 83, 75, 99, 192, 237, 198, 207, 372,
                                417, 330, 263, 446, 485, 365}));
  // GetOutputShape() should always be same as m.SetOutputShape(...);
  EXPECT_THAT(m.GetOutputShape(), ElementsAreArray({1, 4, 4, 1}));
}

// Test case:
// filter = tf.constant(np.arange(1, 19),
//                      shape=[ 3, 3, 1, 2 ],
//                      dtype=tf.float32)
// output = tf.nn.conv2d_backprop_input(
//     tf.constant([ 1, 4, 4, 1 ]),
//     filter,
//     tf.constant(np.arange(1, 33), shape=[ 1, 4, 4, 2 ], dtype=tf.float32),
//     [1, 1, 1, 1 ],
//     "SAME")
// And filter value is derived by:
// filter = tf.reshape(tf.transpose(filter, perm=[3, 0, 1, 2]), shape=[18, 1])
TEST_P(TransposeConvOpTest, TwoFiltersTest) {
  TransposeConvOpModel m(GetRegistration(), {TensorType_FLOAT32, {1, 4, 4, 2}},
                         {TensorType_FLOAT32, {1, 3, 3, 2}},
                         {TensorType_FLOAT32, {}}, Padding_SAME, 1, 1);
  m.SetOutputShape({1, 4, 4, 1});
  m.SetFilter({1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18});
  m.SetInput({1,  2,  3,  4,  5,  6,  7,  8,  9,  10, 11, 12, 13, 14, 15, 16,
              17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32});
  m.Invoke();

  EXPECT_THAT(m.GetOutput(),
              ElementsAreArray({184, 412, 568, 528, 678, 1347, 1689, 1434, 1494,
                                2715, 3057, 2442, 1968, 3352, 3652, 2760}));
  EXPECT_THAT(m.GetOutputShape(), ElementsAreArray({1, 4, 4, 1}));
}

// Test case:
// filter = tf.constant(np.arange(1, 19),
//                      shape=[ 3, 3, 1, 2 ],
//                      dtype=tf.float32)
// output = tf.nn.conv2d_backprop_input(
//     tf.constant([ 1, 6, 6, 1 ]),
//     filter,
//     tf.constant(np.arange(1, 33), shape=[ 1, 4, 4, 2 ], dtype=tf.float32),
//     [1, 1, 1, 1 ],
//     "VALID")
// And filter value is derived by:
// filter = tf.reshape(tf.transpose(filter, perm=[3, 0, 1, 2]), shape=[1, 18])
TEST_P(TransposeConvOpTest, PaddingValidTest) {
  TransposeConvOpModel m(GetRegistration(), {TensorType_FLOAT32, {1, 4, 4, 2}},
                         {TensorType_FLOAT32, {1, 3, 3, 2}},
                         {TensorType_FLOAT32, {}}, Padding_VALID, 1, 1);
  m.SetOutputShape({1, 6, 6, 1});
  m.SetFilter({1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18});
  m.SetInput({1,  2,  3,  4,  5,  6,  7,  8,  9,  10, 11, 12, 13, 14, 15, 16,
              17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32});
  m.Invoke();

  EXPECT_THAT(
      m.GetOutput(),
      ElementsAreArray({5,    22,   59,   101,  114,  83,   52,   184,  412,
                        568,  528,  344,  237,  678,  1347, 1689, 1434, 879,
                        597,  1494, 2715, 3057, 2442, 1431, 856,  1968, 3352,
                        3652, 2760, 1548, 689,  1534, 2543, 2729, 2010, 1103}));
  EXPECT_THAT(m.GetOutputShape(), ElementsAreArray({1, 6, 6, 1}));
}

// Test case:
// filter = tf.constant(np.arange(1, 10),
//                      shape=[ 3, 3, 1, 1 ],
//                      dtype=tf.float32)
// output = tf.nn.conv2d_backprop_input(
//     tf.constant([ 1, 5, 5, 1 ]),
//     filter,
//     tf.constant(np.arange(1, 5), shape=[ 1, 2, 2, 1 ], dtype=tf.float32),
//     [1, 2, 2, 1 ],
//     "VALID")
TEST_P(TransposeConvOpTest, StrideValidTest) {
  TransposeConvOpModel m(GetRegistration(), {TensorType_FLOAT32, {1, 2, 2, 1}},
                         {TensorType_FLOAT32, {1, 3, 3, 1}},
                         {TensorType_FLOAT32, {}}, Padding_VALID, 2, 2);
  m.SetOutputShape({1, 5, 5, 1});
  m.SetFilter({1, 2, 3, 4, 5, 6, 7, 8, 9});
  m.SetInput({1, 2, 3, 4});
  m.Invoke();

  EXPECT_THAT(
      m.GetOutput(),
      ElementsAreArray({1,  2,  5,  4,  6,  4,  5,  14, 10, 12, 10, 14, 36,
                        24, 30, 12, 15, 34, 20, 24, 21, 24, 55, 32, 36}));
  EXPECT_THAT(m.GetOutputShape(), ElementsAreArray({1, 5, 5, 1}));
}

// Test case:
// filter = tf.constant(np.arange(1, 19),
//                      shape=[ 3, 3, 2, 1 ],
//                      dtype=tf.float32)
// output = tf.nn.conv2d_backprop_input(
//     tf.constant([ 1, 5, 5, 2 ]),
//     filter,
//     tf.constant(np.arange(1, 5), shape=[ 1, 2, 2, 1 ], dtype=tf.float32),
//     [1, 2, 2, 1 ],
//     "VALID")
TEST_P(TransposeConvOpTest, MultiChannelTest) {
  TransposeConvOpModel m(GetRegistration(), {TensorType_FLOAT32, {1, 2, 2, 1}},
                         {TensorType_FLOAT32, {2, 3, 3, 1}},
                         {TensorType_FLOAT32, {}}, Padding_VALID, 2, 2);
  m.SetOutputShape({1, 5, 5, 2});
  m.SetFilter({1, 3, 5, 7, 9, 11, 13, 15, 17, 2, 4, 6, 8, 10, 12, 14, 16, 18});
  m.SetInput({1, 2, 3, 4});
  m.Invoke();

  EXPECT_THAT(
      m.GetOutput(),
      ElementsAreArray({1,  2,  3,  4,  7,  10,  6,   8,  10, 12, 7,  8,  9,
                        10, 25, 28, 18, 20, 22,  24,  16, 20, 24, 28, 62, 72,
                        42, 48, 54, 60, 21, 24,  27,  30, 61, 68, 36, 40, 44,
                        48, 39, 42, 45, 48, 103, 110, 60, 64, 68, 72}));
  EXPECT_THAT(m.GetOutputShape(), ElementsAreArray({1, 5, 5, 2}));
}

// Test case:
// filter = tf.constant(np.random.randint(1, 10, size=9),
//                      shape=[ 3, 3, 1, 1 ],
//                      dtype=tf.float32)
// output = tf.nn.conv2d_backprop_input(
//     tf.constant([ 1, 3, 4, 1 ]),
//     filter,
//     tf.constant([323, 521], shape=[ 1, 1, 2, 1], dtype=tf.float32),
//     [1, 3, 3, 1 ],
//     "SAME")
// And filter value is derived by:
// filter = tf.reshape(tf.transpose(filter, perm=[3, 0, 1, 2]), shape=[-1])
TEST_P(TransposeConvOpTest, AccuracyTest) {
  TransposeConvOpModel m(GetRegistration(), {TensorType_FLOAT32, {1, 1, 2, 1}},
                         {TensorType_FLOAT32, {1, 3, 3, 1}},
                         {TensorType_FLOAT32, {}}, Padding_SAME, 3, 3);
  m.SetOutputShape({1, 3, 4, 1});
  m.SetFilter({9, 5, 6, 9, 8, 5, 3, 1, 4});
  m.SetInput({323, 521});
  m.Invoke();

  EXPECT_THAT(m.GetOutput(), ElementsAreArray(ArrayFloatNear(
                                 {1615., 1938., 4689., 2605., 2584., 1615.,
                                  4689., 4168., 323., 1292., 1563., 521.})));
  EXPECT_THAT(m.GetOutputShape(), ElementsAreArray({1, 3, 4, 1}));
}

INSTANTIATE_TEST_CASE_P(
    TransposeConvOpTest, TransposeConvOpTest,
    ::testing::ValuesIn(SingleOpTest::GetKernelTags(*kKernelMap)));

}  // namespace
}  // namespace tflite

int main(int argc, char** argv) {
  ::tflite::LogToStderr();
  ::testing::InitGoogleTest(&argc, argv);
  return RUN_ALL_TESTS();
}
