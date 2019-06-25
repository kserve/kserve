/* Copyright 2016 The TensorFlow Authors. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");

You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
==============================================================================*/

#include "tensorflow/core/framework/node_def_builder.h"
#include "tensorflow/core/framework/op.h"
#include "tensorflow/core/framework/shape_inference_testutil.h"
#include "tensorflow/core/lib/core/status_test_util.h"
#include "tensorflow/core/platform/test.h"

namespace tensorflow {

TEST(IoOpsTest, Save_ShapeFn) {
  ShapeInferenceTestOp op("Save");

  TF_ASSERT_OK(NodeDefBuilder("test", op.name)
                   .Input({"a", 0, DT_STRING})
                   .Input({"b", 0, DT_STRING})
                   .Input({{"c", 0, DT_FLOAT}, {"d", 0, DT_INT64}})
                   .Attr("T", {DT_FLOAT, DT_INT64})
                   .Finalize(&op.node_def));
  INFER_OK(op, "?;?;?;?", "");
  INFER_OK(op, "[];[2];?;?", "");

  // Filename must be scalar.
  INFER_ERROR("Shape must be rank 0 but is rank 1", op, "[?];?;?;?");

  // tensor_names must be vector matching number data elements (2 in this test).
  INFER_ERROR("Shape must be rank 1 but is rank 2", op, "[];[2,3];?;?");
  INFER_ERROR("Dimension must be 2 but is 3", op, "[];[3];?;?");
}

TEST(IoOpsTest, SaveSlices_ShapeFn) {
  ShapeInferenceTestOp op("SaveSlices");

  TF_ASSERT_OK(NodeDefBuilder("test", op.name)
                   .Input({"a", 0, DT_STRING})
                   .Input({"b", 0, DT_STRING})
                   .Input({"c", 0, DT_STRING})
                   .Input({{"d", 0, DT_FLOAT}, {"e", 0, DT_INT64}})
                   .Attr("T", {DT_FLOAT, DT_INT64})
                   .Finalize(&op.node_def));
  INFER_OK(op, "?;?;?;?;?", "");
  INFER_OK(op, "[];[2];[2];?;?", "");
  INFER_OK(op, "[];[2];[2];[100,200,300];[4,5]", "");

  // Filename must be scalar.
  INFER_ERROR("Shape must be rank 0 but is rank 1", op, "[?];?;?;?;?");

  // tensor_names must be vector matching number data elements (2 in this test).
  INFER_ERROR("Shape must be rank 1 but is rank 2", op, "[];[2,3];?;?;?");
  INFER_ERROR("Dimension must be 2 but is 3", op, "[];[3];?;?;?");

  // shapes_and_slices must be vector matching number data elements (2 in this
  // test).
  INFER_ERROR("Shape must be rank 1 but is rank 2", op, "[];[2];[2,3];?;?");
  INFER_ERROR("Dimension must be 2 but is 3", op, "[];[2];[3];?;?");
}

TEST(IoOpsTest, Restore_ShapeFn) {
  ShapeInferenceTestOp op("Restore");

  INFER_OK(op, "?;?", "?");
  INFER_OK(op, "[];[]", "?");

  // Both inputs must be scalars.
  INFER_ERROR("Shape must be rank 0 but is rank 1", op, "[?];[]");
  INFER_ERROR("Shape must be rank 0 but is rank 1", op, "[];[?]");
}

TEST(IoOpsTest, RestoreV2_ShapeFn) {
  ShapeInferenceTestOp op("RestoreV2");

  TF_ASSERT_OK(NodeDefBuilder("test", op.name)
                   .Input({"prefix", 0, DT_STRING})
                   .Input({"tensor_names", 0, DT_STRING})
                   .Input({"shapes_and_slices", 0, DT_STRING})
                   .Attr("dtypes", {DT_FLOAT, DT_INT64})
                   .Finalize(&op.node_def));

  INFER_OK(op, "?;?;?", "?;?");
  INFER_OK(op, "[];[10];[10]", "?;?");

  // Input shape validation.
  INFER_ERROR("Shape must be rank 0 but is rank 1", op, "[?];[?];[?]");
  INFER_ERROR("Shape must be rank 1 but is rank 2", op, "[];[?,?];[?]");
  INFER_ERROR("Shape must be rank 1 but is rank 2", op, "[];[?];[?,?]");
  INFER_ERROR("in both shapes must be equal", op, "[];[10];[20]");
}

TEST(IoOpsTest, RestoreSlice_ShapeFn) {
  ShapeInferenceTestOp op("RestoreSlice");

  INFER_OK(op, "?;?;?", "?");
  INFER_OK(op, "[];[];[]", "?");

  // All three inputs must be scalars.
  INFER_ERROR("Shape must be rank 0 but is rank 1", op, "[?];[];[]");
  INFER_ERROR("Shape must be rank 0 but is rank 1", op, "[];[?];[]");
  INFER_ERROR("Shape must be rank 0 but is rank 1", op, "[];[];[?]");
}

TEST(IoOpsTest, ShardedFilename_ShapeFn) {
  ShapeInferenceTestOp op("ShardedFilename");

  INFER_OK(op, "?;?;?", "[]");
  INFER_OK(op, "[];[];[]", "[]");

  // All three inputs must be scalars.
  INFER_ERROR("Shape must be rank 0 but is rank 1", op, "[?];[];[]");
  INFER_ERROR("Shape must be rank 0 but is rank 1", op, "[];[?];[]");
  INFER_ERROR("Shape must be rank 0 but is rank 1", op, "[];[];[?]");
}

TEST(IoOpsTest, ShardedFilespec_ShapeFn) {
  ShapeInferenceTestOp op("ShardedFilespec");

  INFER_OK(op, "?;?", "[]");
  INFER_OK(op, "[];[]", "[]");

  // Both inputs must be scalars.
  INFER_ERROR("Shape must be rank 0 but is rank 1", op, "[?];[]");
  INFER_ERROR("Shape must be rank 0 but is rank 1", op, "[];[?]");
}

TEST(IoOpsTest, SingleScalarInputAndOutput_ShapeFns) {
  for (const char* op_name : {"ReadFile"}) {
    ShapeInferenceTestOp op(op_name);

    INFER_OK(op, "?", "[]");
    INFER_OK(op, "[]", "[]");
    INFER_ERROR("Shape must be rank 0 but is rank 1", op, "[?]");
  }
}

TEST(IoOpsTest, TwoElementVectorInputsAndScalarOutput_ShapeFns) {
  for (const char* op_name :
       {"ReaderNumRecordsProduced", "ReaderNumWorkUnitsCompleted",
        "ReaderSerializeState"}) {
    ShapeInferenceTestOp op(op_name);

    INFER_OK(op, "?", "[]");
    INFER_OK(op, "[2]", "[]");
    INFER_ERROR("Shape must be rank 1 but is rank 0", op, "[]");
    INFER_ERROR("Dimension must be 2 but is 3", op, "[3]");
  }
}

TEST(IoOpsTest, ReaderRead_ShapeFn) {
  ShapeInferenceTestOp op("ReaderRead");

  INFER_OK(op, "?;?", "[];[]");
  INFER_OK(op, "[2];[?]", "[];[]");

  // Both inputs must be vectors of length 2.
  INFER_ERROR("Shape must be rank 1 but is rank 2", op, "[?,?];[2]");
  INFER_ERROR("Shape must be rank 1 but is rank 0", op, "[2];[]");
}

TEST(IoOpsTest, ReaderReadUpTo_ShapeFn) {
  ShapeInferenceTestOp op("ReaderReadUpTo");

  INFER_OK(op, "[2];[2];[]", "[?];[?]");

  // Third input must be scalar, first two must be vectors of 2
  INFER_ERROR("Shape must be rank 1 but is rank 0", op, "[];[2];[]");
  INFER_ERROR("Shape must be rank 1 but is rank 0", op, "[2];[];[]");
  INFER_ERROR("Shape must be rank 0 but is rank 1", op, "[2];[2];[?]");
}

TEST(IoOpsTest, ReaderReset_ShapeFn) {
  ShapeInferenceTestOp op("ReaderReset");

  INFER_OK(op, "[2]", "");
  INFER_OK(op, "[?]", "");
  INFER_OK(op, "?", "");
  INFER_ERROR("Shape must be rank 1 but is rank 0", op, "[]");
}

TEST(IoOpsTest, ReaderRestoreState_ShapeFn) {
  ShapeInferenceTestOp op("ReaderRestoreState");

  INFER_OK(op, "?;?", "");
  INFER_OK(op, "[2];[]", "");

  // First input must be a vector and the second a scalar
  INFER_ERROR("Shape must be rank 1 but is rank 0", op, "[];[]");
  INFER_ERROR("Shape must be rank 0 but is rank 1", op, "[?];[?]");
}

TEST(IoOpsTest, MatchingFiles_ShapeFn) {
  ShapeInferenceTestOp op("MatchingFiles");

  INFER_OK(op, "?", "[?]");
  INFER_OK(op, "[]", "[?]");
  INFER_OK(op, "[42]", "[?]");
  INFER_ERROR("Shape must be at most rank 1 but is rank 2", op, "[?,?]");
}

}  // end namespace tensorflow
