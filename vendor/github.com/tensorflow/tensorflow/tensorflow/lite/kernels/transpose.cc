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
#include <string.h>
#include <vector>
#include "tensorflow/lite/c/builtin_op_data.h"
#include "tensorflow/lite/c/c_api_internal.h"
#include "tensorflow/lite/kernels/internal/reference/reference_ops.h"
#include "tensorflow/lite/kernels/internal/tensor.h"
#include "tensorflow/lite/kernels/kernel_util.h"
#include "tensorflow/lite/kernels/op_macros.h"

namespace tflite {
namespace ops {
namespace builtin {
namespace transpose {

// This file has two implementations of Transpose.
enum KernelType {
  kReference,
};

struct TransposeContext {
  TransposeContext(TfLiteContext* context, TfLiteNode* node) {
    input = GetInput(context, node, 0);
    perm = GetInput(context, node, 1);
    output = GetOutput(context, node, 0);
  }
  const TfLiteTensor* input;
  const TfLiteTensor* perm;
  TfLiteTensor* output;
};

TfLiteStatus ResizeOutputTensor(TfLiteContext* context,
                                TransposeContext* op_context) {
  int dims = NumDimensions(op_context->input);
  const int* perm_data = GetTensorData<int32_t>(op_context->perm);

  // Ensure validity of the permutations tensor as a 1D tensor.
  TF_LITE_ENSURE_EQ(context, NumDimensions(op_context->perm), 1);
  TF_LITE_ENSURE_EQ(context, op_context->perm->dims->data[0], dims);
  for (int idx = 0; idx < dims; ++idx) {
    TF_LITE_ENSURE_MSG(context, (perm_data[idx] >= 0 && perm_data[idx] < dims),
                       "Transpose op permutations array is out of bounds.");
  }

  // Determine size of output tensor.
  TfLiteIntArray* input_size = op_context->input->dims;
  TfLiteIntArray* output_size = TfLiteIntArrayCopy(input_size);
  for (int idx = 0; idx < dims; ++idx) {
    output_size->data[idx] = input_size->data[perm_data[idx]];
  }

  return context->ResizeTensor(context, op_context->output, output_size);
}

TfLiteStatus Prepare(TfLiteContext* context, TfLiteNode* node) {
  TF_LITE_ENSURE_EQ(context, NumInputs(node), 2);
  TF_LITE_ENSURE_EQ(context, NumOutputs(node), 1);

  TransposeContext op_context(context, node);

  // Ensure validity of input tensor.
  TF_LITE_ENSURE_MSG(context, NumDimensions(op_context.input) <= 4,
                     "Transpose op only supports 1D-4D input arrays.");
  TF_LITE_ENSURE_EQ(context, op_context.input->type, op_context.output->type);

  if (!IsConstantTensor(op_context.perm)) {
    SetTensorToDynamic(op_context.output);
    return kTfLiteOk;
  }
  return ResizeOutputTensor(context, &op_context);
}

template <KernelType kernel_type>
TfLiteStatus Eval(TfLiteContext* context, TfLiteNode* node) {
  TransposeContext op_context(context, node);

  // Resize the output tensor if the output tensor is dynamic.
  if (IsDynamicTensor(op_context.output)) {
    TF_LITE_ENSURE_OK(context, ResizeOutputTensor(context, &op_context));
  }

  const int* perm_data = GetTensorData<int32_t>(op_context.perm);
  const int size = op_context.perm->dims->data[0];
  TransposeParams params;
  params.perm_count = size;
  for (int i = 0; i < size; ++i) {
    params.perm[i] = perm_data[i];
  }

#define TF_LITE_TRANSPOSE(type, scalar)                     \
  type::Transpose(params, GetTensorShape(op_context.input), \
                  GetTensorData<scalar>(op_context.input),  \
                  GetTensorShape(op_context.output),        \
                  GetTensorData<scalar>(op_context.output))

  switch (op_context.input->type) {
    case kTfLiteFloat32:
      if (kernel_type == kReference) {
        TF_LITE_TRANSPOSE(reference_ops, float);
      }
      break;
    case kTfLiteUInt8:
      if (kernel_type == kReference) {
        TF_LITE_TRANSPOSE(reference_ops, uint8_t);
      }
      break;
    case kTfLiteInt32:
      if (kernel_type == kReference) {
        TF_LITE_TRANSPOSE(reference_ops, int32_t);
      }
      break;
    case kTfLiteInt64:
      if (kernel_type == kReference) {
        TF_LITE_TRANSPOSE(reference_ops, int64_t);
      }
      break;
    default:
      context->ReportError(context,
                           "Type %d is currently not supported by Transpose.",
                           op_context.input->type);
      return kTfLiteError;
  }
#undef TF_LITE_TRANSPOSE

  return kTfLiteOk;
}

}  // namespace transpose

TfLiteRegistration* Register_TRANSPOSE_REF() {
  static TfLiteRegistration r = {nullptr, nullptr, transpose::Prepare,
                                 transpose::Eval<transpose::kReference>};
  return &r;
}

TfLiteRegistration* Register_TRANSPOSE() { return Register_TRANSPOSE_REF(); }

}  // namespace builtin
}  // namespace ops
}  // namespace tflite
