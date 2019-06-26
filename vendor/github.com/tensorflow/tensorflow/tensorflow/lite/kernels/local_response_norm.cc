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
#include "tensorflow/lite/c/builtin_op_data.h"
#include "tensorflow/lite/c/c_api_internal.h"
#include "tensorflow/lite/kernels/internal/optimized/optimized_ops.h"
#include "tensorflow/lite/kernels/internal/reference/reference_ops.h"
#include "tensorflow/lite/kernels/internal/tensor.h"
#include "tensorflow/lite/kernels/kernel_util.h"
#include "tensorflow/lite/kernels/op_macros.h"

namespace tflite {
namespace ops {
namespace builtin {
namespace local_response_norm {

// This file has two implementation of LocalResponseNorm.
enum KernelType {
  kReference,
  kGenericOptimized,
};

constexpr int kInputTensor = 0;
constexpr int kOutputTensor = 0;

TfLiteStatus Prepare(TfLiteContext* context, TfLiteNode* node) {
  TF_LITE_ENSURE_EQ(context, NumInputs(node), 1);
  TF_LITE_ENSURE_EQ(context, NumOutputs(node), 1);

  const TfLiteTensor* input = GetInput(context, node, kInputTensor);
  TfLiteTensor* output = GetOutput(context, node, kOutputTensor);

  TF_LITE_ENSURE_EQ(context, NumDimensions(input), 4);

  TF_LITE_ENSURE_EQ(context, output->type, kTfLiteFloat32);
  TF_LITE_ENSURE_EQ(context, input->type, output->type);

  TfLiteIntArray* output_size = TfLiteIntArrayCreate(4);
  output_size->data[0] = input->dims->data[0];
  output_size->data[1] = input->dims->data[1];
  output_size->data[2] = input->dims->data[2];
  output_size->data[3] = input->dims->data[3];

  return context->ResizeTensor(context, output, output_size);
}

template <KernelType kernel_type>
TfLiteStatus Eval(TfLiteContext* context, TfLiteNode* node) {
  auto* params =
      reinterpret_cast<TfLiteLocalResponseNormParams*>(node->builtin_data);

  const TfLiteTensor* input = GetInput(context, node, kInputTensor);
  TfLiteTensor* output = GetOutput(context, node, kOutputTensor);

  if (output->type == kTfLiteFloat32) {
#define TF_LITE_LOCAL_RESPONSE_NORM(type)                            \
  tflite::LocalResponseNormalizationParams op_params;                \
  op_params.range = params->radius;                                  \
  op_params.bias = params->bias;                                     \
  op_params.alpha = params->alpha;                                   \
  op_params.beta = params->beta;                                     \
  type::LocalResponseNormalization(                                  \
      op_params, GetTensorShape(input), GetTensorData<float>(input), \
      GetTensorShape(output), GetTensorData<float>(output))
    if (kernel_type == kReference) {
      TF_LITE_LOCAL_RESPONSE_NORM(reference_ops);
    }
    if (kernel_type == kGenericOptimized) {
      TF_LITE_LOCAL_RESPONSE_NORM(optimized_ops);
    }
#undef TF_LITE_LOCAL_RESPONSE_NORM
  } else {
    context->ReportError(context, "Output type is %d, requires float.",
                         output->type);
    return kTfLiteError;
  }

  return kTfLiteOk;
}

}  // namespace local_response_norm

TfLiteRegistration* Register_LOCAL_RESPONSE_NORM_REF() {
  static TfLiteRegistration r = {
      nullptr, nullptr, local_response_norm::Prepare,
      local_response_norm::Eval<local_response_norm::kReference>};
  return &r;
}

TfLiteRegistration* Register_LOCAL_RESPONSE_NORM_GENERIC_OPT() {
  static TfLiteRegistration r = {
      nullptr, nullptr, local_response_norm::Prepare,
      local_response_norm::Eval<local_response_norm::kGenericOptimized>};
  return &r;
}

TfLiteRegistration* Register_LOCAL_RESPONSE_NORMALIZATION() {
  return Register_LOCAL_RESPONSE_NORM_GENERIC_OPT();
}

}  // namespace builtin
}  // namespace ops
}  // namespace tflite
