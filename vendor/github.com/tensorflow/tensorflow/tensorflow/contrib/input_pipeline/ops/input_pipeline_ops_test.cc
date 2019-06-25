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
#include "tensorflow/core/framework/tensor_testutil.h"
#include "tensorflow/core/platform/test.h"

namespace tensorflow {

TEST(InputPipelineOpsTest, ObtainNext_InvalidNumberOfInputs) {
  ShapeInferenceTestOp op("ObtainNext");
  op.input_tensors.resize(3);
  INFER_ERROR("Wrong number of inputs passed", op, "?;?;?");
}

TEST(InputPipelineOpsTest, ObtainNext) {
  ShapeInferenceTestOp op("ObtainNext");
  INFER_OK(op, "[100];[]", "[]");

  INFER_ERROR("Shape must be rank 1 but is rank 2", op, "[1,1];[]");
  INFER_ERROR("Shape must be rank 0 but is rank 1", op, "[1000];[1]");
}

}  // end namespace tensorflow
