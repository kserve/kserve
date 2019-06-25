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

#include "tensorflow/lite/c/builtin_op_data.h"
#include "tensorflow/lite/c/c_api_internal.h"
#include "tensorflow/lite/kernels/internal/reference/reference_ops.h"
#include "tensorflow/lite/kernels/internal/tensor.h"
#include "tensorflow/lite/kernels/internal/tensor_ctypes.h"
#include "tensorflow/lite/kernels/kernel_util.h"

namespace tflite {
namespace ops {
namespace builtin {
namespace fill {

namespace {

constexpr int kDimsTensor = 0;
constexpr int kValueTensor = 1;
constexpr int kOutputTensor = 0;

template <typename T>
TfLiteStatus ResizeOutputImpl(TfLiteContext* context, const TfLiteTensor* dims,
                              TfLiteTensor* output) {
  TfLiteIntArray* output_shape = TfLiteIntArrayCreate(dims->dims->data[0]);
  for (int i = 0; i < output_shape->size; ++i) {
    T data = GetTensorData<T>(dims)[i];
    if (data < 0) {
      context->ReportError(context, "Fill dimensions must be >= 0", dims->type);
      return kTfLiteError;
    }
    output_shape->data[i] = data;
  }
  return context->ResizeTensor(context, output, output_shape);
}

TfLiteStatus ResizeOutput(TfLiteContext* context, const TfLiteTensor* dims,
                          TfLiteTensor* output) {
  switch (dims->type) {
    case kTfLiteInt32:
      return ResizeOutputImpl<int32_t>(context, dims, output);
    case kTfLiteInt64:
      return ResizeOutputImpl<int64_t>(context, dims, output);
    default:
      context->ReportError(
          context,
          "Fill only currently supports int32, int64 for input 0, "
          "got %d.",
          dims->type);
      return kTfLiteError;
  }
}

}  // namespace

TfLiteStatus Prepare(TfLiteContext* context, TfLiteNode* node) {
  TF_LITE_ENSURE_EQ(context, NumInputs(node), 2);
  TF_LITE_ENSURE_EQ(context, NumOutputs(node), 1);

  const TfLiteTensor* dims = GetInput(context, node, kDimsTensor);
  const TfLiteTensor* value = GetInput(context, node, kValueTensor);

  // Make sure the 1st input tensor is 1-D.
  TF_LITE_ENSURE_EQ(context, NumDimensions(dims), 1);

  // Make sure the 1st input tensor is int32 or int64.
  const auto dtype = dims->type;
  TF_LITE_ENSURE(context, dtype == kTfLiteInt32 || dtype == kTfLiteInt64);

  // Make sure the 2nd input tensor is a scalar.
  TF_LITE_ENSURE_EQ(context, NumDimensions(value), 0);

  TfLiteTensor* output = GetOutput(context, node, kOutputTensor);
  output->type = value->type;

  if (IsConstantTensor(dims)) {
    TF_LITE_ENSURE_OK(context, ResizeOutput(context, dims, output));
  } else {
    SetTensorToDynamic(output);
  }
  return kTfLiteOk;
}

TfLiteStatus Eval(TfLiteContext* context, TfLiteNode* node) {
  const TfLiteTensor* value = GetInput(context, node, kValueTensor);

  TfLiteTensor* output = GetOutput(context, node, kOutputTensor);

  if (IsDynamicTensor(output)) {
    const TfLiteTensor* dims = GetInput(context, node, kDimsTensor);
    TF_LITE_ENSURE_OK(context, ResizeOutput(context, dims, output));
  }
#define TF_LITE_FILL(data_type)                                               \
  reference_ops::Fill(GetTensorShape(value), GetTensorData<data_type>(value), \
                      GetTensorShape(output),                                 \
                      GetTensorData<data_type>(output))
  switch (output->type) {
    case kTfLiteInt32:
      TF_LITE_FILL(int32_t);
      break;
    case kTfLiteInt64:
      TF_LITE_FILL(int64_t);
      break;
    case kTfLiteFloat32:
      TF_LITE_FILL(float);
      break;
    default:
      context->ReportError(
          context,
          "Fill only currently supports int32, int64, float32 for input 1,"
          "got %d.",
          value->type);
      return kTfLiteError;
  }
#undef TF_LITE_FILL
  return kTfLiteOk;
}

}  // namespace fill

TfLiteRegistration* Register_FILL() {
  static TfLiteRegistration r = {/*init=*/nullptr, /*free=*/nullptr,
                                 fill::Prepare, fill::Eval};
  return &r;
}

}  // namespace builtin
}  // namespace ops
}  // namespace tflite
