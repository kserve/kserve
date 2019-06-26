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
#include "tensorflow/lite/kernels/internal/quantization_util.h"
#include "tensorflow/lite/kernels/internal/reference/reference_ops.h"
#include "tensorflow/lite/kernels/internal/tensor.h"
#include "tensorflow/lite/kernels/kernel_util.h"
#include "tensorflow/lite/kernels/op_macros.h"

namespace tflite {
namespace ops {
namespace builtin {
namespace mul {

// This file has three implementation of Mul.
enum KernelType {
  kReference,
  kGenericOptimized,  // Neon-free
  kNeonOptimized,
};

constexpr int kInputTensor1 = 0;
constexpr int kInputTensor2 = 1;
constexpr int kOutputTensor = 0;

struct OpData {
  bool requires_broadcast;

  // Parameters used in the quantized paths where the output is 8bit
  int32 output_activation_min;
  int32 output_activation_max;

  // Parameters used in all quantized paths
  int32_t output_multiplier;
  int output_shift;
};

void* Init(TfLiteContext* context, const char* buffer, size_t length) {
  auto* data = new OpData;
  data->requires_broadcast = false;
  return data;
}

void Free(TfLiteContext* context, void* buffer) {
  delete reinterpret_cast<OpData*>(buffer);
}

TfLiteStatus Prepare(TfLiteContext* context, TfLiteNode* node) {
  auto* params = reinterpret_cast<TfLiteMulParams*>(node->builtin_data);
  OpData* data = reinterpret_cast<OpData*>(node->user_data);

  TF_LITE_ENSURE_EQ(context, NumInputs(node), 2);
  TF_LITE_ENSURE_EQ(context, NumOutputs(node), 1);

  const TfLiteTensor* input1 = GetInput(context, node, kInputTensor1);
  const TfLiteTensor* input2 = GetInput(context, node, kInputTensor2);
  TfLiteTensor* output = GetOutput(context, node, kOutputTensor);

  TF_LITE_ENSURE_EQ(context, input1->type, input2->type);

  data->requires_broadcast = !HaveSameShapes(input1, input2);

  TfLiteIntArray* output_size = nullptr;
  if (data->requires_broadcast) {
    TF_LITE_ENSURE_OK(context, CalculateShapeForBroadcast(
                                   context, input1, input2, &output_size));
  } else {
    output_size = TfLiteIntArrayCopy(input1->dims);
  }

  if (output->type == kTfLiteUInt8) {
    CalculateActivationRangeUint8(params->activation, output,
                                  &data->output_activation_min,
                                  &data->output_activation_max);
  }

  if (output->type == kTfLiteUInt8 || output->type == kTfLiteInt16) {
    double real_multiplier =
        input1->params.scale * input2->params.scale / output->params.scale;
    QuantizeMultiplierSmallerThanOneExp(
        real_multiplier, &data->output_multiplier, &data->output_shift);
  }

  return context->ResizeTensor(context, output, output_size);
}

template <KernelType kernel_type>
void EvalMul(TfLiteContext* context, TfLiteNode* node, TfLiteMulParams* params,
             const OpData* data, const TfLiteTensor* input1,
             const TfLiteTensor* input2, TfLiteTensor* output) {
#define TF_LITE_MUL(type, opname, data_type)                             \
  data_type output_activation_min, output_activation_max;                \
  CalculateActivationRange(params->activation, &output_activation_min,   \
                           &output_activation_max);                      \
  tflite::ArithmeticParams op_params;                                    \
  SetActivationParams(output_activation_min, output_activation_max,      \
                      &op_params);                                       \
  type::opname(op_params, GetTensorShape(input1),                        \
               GetTensorData<data_type>(input1), GetTensorShape(input2), \
               GetTensorData<data_type>(input2), GetTensorShape(output), \
               GetTensorData<data_type>(output))

  if (output->type == kTfLiteInt32) {
    if (kernel_type == kReference) {
      if (data->requires_broadcast) {
        TF_LITE_MUL(reference_ops, BroadcastMul4DSlow, int32_t);
      } else {
        TF_LITE_MUL(reference_ops, Mul, int32_t);
      }
    } else {
      if (data->requires_broadcast) {
        TF_LITE_MUL(optimized_ops, BroadcastMul4DSlow, int32_t);
      } else {
        TF_LITE_MUL(optimized_ops, Mul, int32_t);
      }
    }
  } else if (output->type == kTfLiteFloat32) {
    if (kernel_type == kReference) {
      if (data->requires_broadcast) {
        TF_LITE_MUL(reference_ops, BroadcastMul4DSlow, float);
      } else {
        TF_LITE_MUL(reference_ops, Mul, float);
      }
    } else {
      if (data->requires_broadcast) {
        TF_LITE_MUL(optimized_ops, BroadcastMul4DSlow, float);
      } else {
        TF_LITE_MUL(optimized_ops, Mul, float);
      }
    }
  }
#undef TF_LITE_MUL
}

template <KernelType kernel_type>
TfLiteStatus EvalQuantized(TfLiteContext* context, TfLiteNode* node,
                           TfLiteMulParams* params, const OpData* data,
                           const TfLiteTensor* input1,
                           const TfLiteTensor* input2, TfLiteTensor* output) {
  if (input1->type == kTfLiteUInt8 && input2->type == kTfLiteUInt8 &&
      output->type == kTfLiteUInt8) {
    tflite::ArithmeticParams op_params;
    SetActivationParams(data->output_activation_min,
                        data->output_activation_max, &op_params);
    op_params.input1_offset = -input1->params.zero_point;
    op_params.input2_offset = -input2->params.zero_point;
    op_params.output_offset = output->params.zero_point;
    op_params.output_multiplier = data->output_multiplier;
    op_params.output_shift = data->output_shift;
    bool need_broadcast = optimized_ops::ProcessBroadcastShapes(
        GetTensorShape(input1), GetTensorShape(input2), &op_params);
#define TF_LITE_MUL(type, opname)                                      \
  type::opname(op_params, GetTensorShape(input1),                      \
               GetTensorData<uint8_t>(input1), GetTensorShape(input2), \
               GetTensorData<uint8_t>(input2), GetTensorShape(output), \
               GetTensorData<uint8_t>(output))

    if (kernel_type == kReference) {
      if (need_broadcast) {
        TF_LITE_MUL(reference_ops, BroadcastMul4DSlow);
      } else {
        TF_LITE_MUL(reference_ops, Mul);
      }
    } else {
      if (need_broadcast) {
        TF_LITE_MUL(optimized_ops, BroadcastMulFivefold);
      } else {
        TF_LITE_MUL(optimized_ops, Mul);
      }
    }
#undef TF_LITE_MUL
  } else if (input1->type == kTfLiteInt16 && input2->type == kTfLiteInt16 &&
             output->type == kTfLiteInt16) {
#define TF_LITE_MUL(type, opname)                                      \
  tflite::ArithmeticParams op_params;                                  \
  type::opname(op_params, GetTensorShape(input1),                      \
               GetTensorData<int16_t>(input1), GetTensorShape(input2), \
               GetTensorData<int16_t>(input2), GetTensorShape(output), \
               GetTensorData<int16_t>(output))
    if (kernel_type == kReference) {
      TF_LITE_MUL(reference_ops, Mul);
    } else {
      TF_LITE_MUL(optimized_ops, Mul);
    }
#undef TF_LITE_MUL
  } else if (input1->type == kTfLiteInt16 && input2->type == kTfLiteInt16 &&
             output->type == kTfLiteUInt8) {
#define TF_LITE_MUL(type, opname)                                      \
  tflite::ArithmeticParams op_params;                                  \
  SetActivationParams(data->output_activation_min,                     \
                      data->output_activation_max, &op_params);        \
  op_params.output_offset = output->params.zero_point;                 \
  type::opname(op_params, GetTensorShape(input1),                      \
               GetTensorData<int16_t>(input1), GetTensorShape(input2), \
               GetTensorData<int16_t>(input2), GetTensorShape(output), \
               GetTensorData<uint8_t>(output))
    if (kernel_type == kReference) {
      TF_LITE_MUL(reference_ops, Mul);
    } else {
      TF_LITE_MUL(optimized_ops, Mul);
    }
#undef TF_LITE_MUL
  } else {
    context->ReportError(
        context, "Unsupported combination of input and output types in Mul.");
    return kTfLiteError;
  }
  return kTfLiteOk;
}

template <KernelType kernel_type>
TfLiteStatus Eval(TfLiteContext* context, TfLiteNode* node) {
  auto* params = reinterpret_cast<TfLiteMulParams*>(node->builtin_data);
  OpData* data = reinterpret_cast<OpData*>(node->user_data);

  const TfLiteTensor* input1 = GetInput(context, node, kInputTensor1);
  const TfLiteTensor* input2 = GetInput(context, node, kInputTensor2);
  TfLiteTensor* output = GetOutput(context, node, kOutputTensor);

  if (output->type == kTfLiteFloat32 || output->type == kTfLiteInt32) {
    EvalMul<kernel_type>(context, node, params, data, input1, input2, output);
  } else if (output->type == kTfLiteUInt8 || output->type == kTfLiteInt16) {
    TF_LITE_ENSURE_OK(
        context, EvalQuantized<kernel_type>(context, node, params, data, input1,
                                            input2, output));
  } else {
    context->ReportError(context,
                         "Mul only supports FLOAT32, INT32 and quantized UINT8 "
                         "and INT16 now, got %d.",
                         output->type);
    return kTfLiteError;
  }

  return kTfLiteOk;
}

}  // namespace mul

TfLiteRegistration* Register_MUL_REF() {
  static TfLiteRegistration r = {mul::Init, mul::Free, mul::Prepare,
                                 mul::Eval<mul::kReference>};
  return &r;
}

TfLiteRegistration* Register_MUL_GENERIC_OPT() {
  static TfLiteRegistration r = {mul::Init, mul::Free, mul::Prepare,
                                 mul::Eval<mul::kGenericOptimized>};
  return &r;
}

TfLiteRegistration* Register_MUL_NEON_OPT() {
  static TfLiteRegistration r = {mul::Init, mul::Free, mul::Prepare,
                                 mul::Eval<mul::kNeonOptimized>};
  return &r;
}

TfLiteRegistration* Register_MUL() {
#ifdef USE_NEON
  return Register_MUL_NEON_OPT();
#else
  return Register_MUL_GENERIC_OPT();
#endif
}

}  // namespace builtin
}  // namespace ops
}  // namespace tflite
