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
#include "tensorflow/lite/delegates/flex/delegate.h"

#include <gmock/gmock.h>
#include <gtest/gtest.h>
#include "tensorflow/lite/delegates/flex/test_util.h"

namespace tflite {
namespace flex {
namespace {

using ::testing::ElementsAre;

class DelegateTest : public testing::FlexModelTest {
 public:
  DelegateTest() {
    delegate_ = FlexDelegate::Create();
    interpreter_.reset(new Interpreter(&error_reporter_));
  }

  ~DelegateTest() override {
    // The delegate needs to be destructed after the interpreter because the
    // interpreter references data contained in the delegate.
    interpreter_.reset();
    delegate_.reset();
  }

  void ConfigureDelegate() {
    ASSERT_EQ(interpreter_->ModifyGraphWithDelegate(delegate_.get()),
              kTfLiteOk);
  }

 private:
  std::unique_ptr<FlexDelegate> delegate_;
};

TEST_F(DelegateTest, FullGraph) {
  // Define the graph.
  AddTensors(9, {0, 3}, {8}, kTfLiteFloat32, {3});

  AddTfOp(testing::kUnpack, {0}, {1, 2});
  AddTfOp(testing::kUnpack, {3}, {4, 5});
  AddTfOp(testing::kAdd, {1, 4}, {6});
  AddTfOp(testing::kAdd, {2, 5}, {7});
  AddTfOp(testing::kMul, {6, 7}, {8});

  // Apply the delegate.
  ConfigureDelegate();

  // Define inputs.
  SetShape(0, {2, 2, 1});
  SetValues(0, {1.1f, 2.2f, 3.3f, 4.4f});
  SetShape(3, {2, 2, 1});
  SetValues(3, {1.1f, 2.2f, 3.3f, 4.4f});

  ASSERT_TRUE(Invoke());

  ASSERT_THAT(GetShape(8), ElementsAre(2, 1));
  ASSERT_THAT(GetValues(8), ElementsAre(14.52f, 38.72f));
  ASSERT_EQ(GetType(8), kTfLiteFloat32);
}

TEST_F(DelegateTest, NonFloatTypeInference) {
  AddTensors(3, {0, 1}, {2}, kTfLiteInt32, {2});

  AddTfOp(testing::kAdd, {0, 1}, {2});

  ConfigureDelegate();

  SetShape(0, {2, 2});
  SetTypedValues<int>(0, {1, 2, 3, 4});
  SetShape(1, {2, 2});
  SetTypedValues<int>(1, {4, 3, 2, 1});

  ASSERT_TRUE(Invoke());

  ASSERT_THAT(GetShape(2), ElementsAre(2, 2));
  ASSERT_THAT(GetTypedValues<int>(2), ElementsAre(5, 5, 5, 5));
  ASSERT_EQ(GetType(2), kTfLiteInt32);
}

TEST_F(DelegateTest, StringInference) {
  AddTensors(3, {0, 1}, {2}, kTfLiteString, {2});

  AddTfOp(testing::kAdd, {0, 1}, {2});

  ConfigureDelegate();

  SetShape(0, {2, 2});
  SetStringValues(0, {"1", "2", "3", "4"});
  SetShape(1, {2, 2});
  SetStringValues(1, {"4", "3", "2", "1"});

  ASSERT_TRUE(Invoke());

  ASSERT_THAT(GetShape(2), ElementsAre(2, 2));
  ASSERT_THAT(GetStringValues(2), ElementsAre("14", "23", "32", "41"));
  ASSERT_EQ(GetType(2), kTfLiteString);
}

TEST_F(DelegateTest, MixedGraph) {
  AddTensors(9, {0, 3}, {8}, kTfLiteFloat32, {3});

  AddTfOp(testing::kUnpack, {0}, {1, 2});
  AddTfOp(testing::kUnpack, {3}, {4, 5});
  AddTfOp(testing::kAdd, {1, 4}, {6});
  AddTfOp(testing::kAdd, {2, 5}, {7});
  AddTfLiteMulOp({6, 7}, {8});

  ConfigureDelegate();

  SetShape(0, {2, 2, 1});
  SetValues(0, {1.1f, 2.2f, 3.3f, 4.4f});
  SetShape(3, {2, 2, 1});
  SetValues(3, {1.1f, 2.2f, 3.3f, 4.4f});

  ASSERT_TRUE(Invoke());

  ASSERT_THAT(GetShape(8), ElementsAre(2, 1));
  ASSERT_THAT(GetValues(8), ElementsAre(14.52f, 38.72f));
}

TEST_F(DelegateTest, SplitGraph) {
  AddTensors(10, {0}, {9}, kTfLiteFloat32, {3});

  AddTfOp(testing::kUnpack, {0}, {1, 2});
  AddTfOp(testing::kAdd, {1, 2}, {3});
  AddTfOp(testing::kUnpack, {3}, {4, 5});

  AddTfLiteMulOp({4, 5}, {6});

  AddTfOp(testing::kUnpack, {6}, {7, 8});
  AddTfOp(testing::kAdd, {7, 8}, {9});

  ConfigureDelegate();

  SetShape(0, {2, 2, 2, 1});
  SetValues(0, {3.0f, 1.0f, 0.5f, -1.0f, 0.0f, 1.0f, 1.5f, 3.0f});

  ASSERT_TRUE(Invoke());

  ASSERT_THAT(GetShape(9), ElementsAre(1));
  ASSERT_THAT(GetValues(9), ElementsAre(10.0f));
}

TEST_F(DelegateTest, OnlyTFLite) {
  // Only TFLite single op model.
  AddTensors(10, {0, 1}, {2}, kTfLiteFloat32, {3});
  AddTfLiteMulOp({0, 1}, {2});

  ConfigureDelegate();

  SetShape(0, {2, 2, 1});
  SetValues(0, {1.1f, 2.2f, 3.3f, 4.4f});
  SetShape(1, {2, 2, 1});
  SetValues(1, {1.0f, 2.0f, 3.0f, 4.0f});

  ASSERT_TRUE(Invoke());

  ASSERT_THAT(GetShape(2), ElementsAre(2, 2, 1));
  ASSERT_THAT(GetValues(2), ElementsAre(1.1f, 4.4f, 9.9f, 17.6f));
}

TEST_F(DelegateTest, MultipleInvokeCalls) {
  // Call Invoke() multiple times on the same model.
  AddTensors(10, {0, 1}, {2}, kTfLiteFloat32, {3});
  AddTfLiteMulOp({0, 1}, {2});

  ConfigureDelegate();

  SetShape(0, {2, 2, 1});
  SetValues(0, {1.1f, 2.2f, 3.3f, 4.4f});
  SetShape(1, {2, 2, 1});
  SetValues(1, {1.0f, 2.0f, 3.0f, 4.0f});

  ASSERT_TRUE(Invoke());

  ASSERT_THAT(GetShape(2), ElementsAre(2, 2, 1));
  ASSERT_THAT(GetValues(2), ElementsAre(1.1f, 4.4f, 9.9f, 17.6f));

  SetShape(0, {2, 2, 1});
  SetValues(1, {4.0f, 3.0f, 2.0f, 1.0f});
  SetShape(1, {2, 2, 1});
  SetValues(0, {4.4f, 3.3f, 2.2f, 1.1f});

  ASSERT_TRUE(Invoke());

  ASSERT_THAT(GetShape(2), ElementsAre(2, 2, 1));
  ASSERT_THAT(GetValues(2), ElementsAre(17.6f, 9.9f, 4.4f, 1.1f));
}

TEST_F(DelegateTest, MultipleInterpretersSameDelegate) {
  // Build a graph, configure the delegate and set inputs.
  {
    AddTensors(9, {0, 3}, {8}, kTfLiteFloat32, {3});
    AddTfOp(testing::kUnpack, {0}, {1, 2});
    AddTfOp(testing::kUnpack, {3}, {4, 5});
    AddTfOp(testing::kAdd, {1, 4}, {6});
    AddTfOp(testing::kAdd, {2, 5}, {7});
    AddTfOp(testing::kMul, {6, 7}, {8});
    ConfigureDelegate();
    SetShape(0, {2, 2, 1});
    SetValues(0, {1.1f, 2.2f, 3.3f, 4.4f});
    SetShape(3, {2, 2, 1});
    SetValues(3, {1.1f, 2.2f, 3.3f, 4.4f});
  }

  // Create a new interpreter, inject into the test framework and build
  // a different graph using the *same* delegate.
  std::unique_ptr<Interpreter> interpreter(new Interpreter(&error_reporter_));
  interpreter_.swap(interpreter);
  {
    AddTensors(10, {0}, {9}, kTfLiteFloat32, {3});
    AddTfOp(testing::kUnpack, {0}, {1, 2});
    AddTfOp(testing::kAdd, {1, 2}, {3});
    AddTfOp(testing::kUnpack, {3}, {4, 5});
    AddTfLiteMulOp({4, 5}, {6});
    AddTfOp(testing::kUnpack, {6}, {7, 8});
    AddTfOp(testing::kAdd, {7, 8}, {9});
    ConfigureDelegate();
    SetShape(0, {2, 2, 2, 1});
    SetValues(0, {3.0f, 1.0f, 0.5f, -1.0f, 0.0f, 1.0f, 1.5f, 3.0f});
  }

  // Swap back in the first interpreter and validate inference.
  interpreter_.swap(interpreter);
  {
    ASSERT_TRUE(Invoke());
    EXPECT_THAT(GetShape(8), ElementsAre(2, 1));
    EXPECT_THAT(GetValues(8), ElementsAre(14.52f, 38.72f));
  }

  // Swap in the second interpreter and validate inference.
  interpreter_.swap(interpreter);
  {
    ASSERT_TRUE(Invoke());
    EXPECT_THAT(GetShape(9), ElementsAre(1));
    EXPECT_THAT(GetValues(9), ElementsAre(10.0f));
  }
}

}  // namespace
}  // namespace flex
}  // namespace tflite

int main(int argc, char** argv) {
  ::tflite::LogToStderr();
  ::testing::InitGoogleTest(&argc, argv);
  return RUN_ALL_TESTS();
}
