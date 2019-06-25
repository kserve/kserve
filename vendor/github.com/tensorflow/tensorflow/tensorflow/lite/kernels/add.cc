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
namespace add {

// This file has three implementation of Add.
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

  // These fields are used in both the general 8-bit -> 8bit quantized path,
  // and the special 16-bit -> 16bit quantized path
  int input1_shift;
  int input2_shift;
  int32 output_activation_min;
  int32 output_activation_max;

  // These fields are used only in the general 8-bit -> 8bit quantized path
  int32 input1_multiplier;
  int32 input2_multiplier;
  int32 output_multiplier;
  int output_shift;
  int left_shift;
  int32 input1_offset;
  int32 input2_offset;
  int32 output_offset;
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
  auto* params = reinterpret_cast<TfLiteAddParams*>(node->builtin_data);
  OpData* data = reinterpret_cast<OpData*>(node->user_data);

  TF_LITE_ENSURE_EQ(context, NumInputs(node), 2);
  TF_LITE_ENSURE_EQ(context, NumOutputs(node), 1);

  const TfLiteTensor* input1 = GetInput(context, node, kInputTensor1);
  const TfLiteTensor* input2 = GetInput(context, node, kInputTensor2);
  TfLiteTensor* output = GetOutput(context, node, kOutputTensor);

  TF_LITE_ENSURE_EQ(context, input1->type, input2->type);
  output->type = input2->type;

  data->requires_broadcast = !HaveSameShapes(input1, input2);

  TfLiteIntArray* output_size = nullptr;
  if (data->requires_broadcast) {
    TF_LITE_ENSURE_OK(context, CalculateShapeForBroadcast(
                                   context, input1, input2, &output_size));
  } else {
    output_size = TfLiteIntArrayCopy(input1->dims);
  }

  if (output->type == kTfLiteUInt8) {
    // 8bit -> 8bit general quantized path, with general rescalings
    data->input1_offset = -input1->params.zero_point;
    data->input2_offset = -input2->params.zero_point;
    data->output_offset = output->params.zero_point;
    data->left_shift = 20;
    const double twice_max_input_scale =
        2 * std::max(input1->params.scale, input2->params.scale);
    const double real_input1_multiplier =
        input1->params.scale / twice_max_input_scale;
    const double real_input2_multiplier =
        input2->params.scale / twice_max_input_scale;
    const double real_output_multiplier =
        twice_max_input_scale /
        ((1 << data->left_shift) * output->params.scale);

    QuantizeMultiplierSmallerThanOneExp(
        real_input1_multiplier, &data->input1_multiplier, &data->input1_shift);

    QuantizeMultiplierSmallerThanOneExp(
        real_input2_multiplier, &data->input2_multiplier, &data->input2_shift);

    QuantizeMultiplierSmallerThanOneExp(
        real_output_multiplier, &data->output_multiplier, &data->output_shift);

    CalculateActivationRangeUint8(params->activation, output,
                                  &data->output_activation_min,
                                  &data->output_activation_max);

  } else if (output->type == kTfLiteInt16) {
    // 16bit -> 16bit special quantized path, supporting only a rather
    // narrow case of quantization parameters: zero_points must all be 0
    // ("symmetric quantization") and scales must be power-of-two (which
    // we abbreviate as "POT" below). The intended use case for this path
    // is in LSTM cells, where, due to the constraints of implementing
    // some of the math in these LSTM cells in fixed-point arithmetic,
    // we need to have such symmetric, power-of-two quantization
    // (Fixed-point formats are inherently symmetric, power-of-two).
    TF_LITE_ENSURE_EQ(context, input1->params.zero_point, 0);
    TF_LITE_ENSURE_EQ(context, input2->params.zero_point, 0);
    TF_LITE_ENSURE_EQ(context, output->params.zero_point, 0);

    int input1_scale_log2_rounded;
    bool input1_scale_is_pot =
        CheckedLog2(input1->params.scale, &input1_scale_log2_rounded);
    TF_LITE_ENSURE(context, input1_scale_is_pot);

    int input2_scale_log2_rounded;
    bool input2_scale_is_pot =
        CheckedLog2(input2->params.scale, &input2_scale_log2_rounded);
    TF_LITE_ENSURE(context, input2_scale_is_pot);

    int output_scale_log2_rounded;
    bool output_scale_is_pot =
        CheckedLog2(output->params.scale, &output_scale_log2_rounded);
    TF_LITE_ENSURE(context, output_scale_is_pot);

    data->input1_shift = input1_scale_log2_rounded - output_scale_log2_rounded;
    data->input2_shift = input2_scale_log2_rounded - output_scale_log2_rounded;

    // Shifting of one input is supported. The graph quantization should ensure
    // that the other input matches the output.
    TF_LITE_ENSURE(context, data->input1_shift == 0 || data->input2_shift == 0);
    TF_LITE_ENSURE(context, data->input1_shift <= 0);
    TF_LITE_ENSURE(context, data->input2_shift <= 0);

    CalculateActivationRangeQuantized(context, params->activation, output,
                                      &data->output_activation_min,
                                      &data->output_activation_max);
  }

  return context->ResizeTensor(context, output, output_size);
}

template <KernelType kernel_type>
void EvalAdd(TfLiteContext* context, TfLiteNode* node, TfLiteAddParams* params,
             const OpData* data, const TfLiteTensor* input1,
             const TfLiteTensor* input2, TfLiteTensor* output) {
#define TF_LITE_ADD(type, opname, data_type)                             \
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
        TF_LITE_ADD(reference_ops, BroadcastAdd4DSlow, int32_t);
      } else {
        TF_LITE_ADD(reference_ops, Add, int32_t);
      }
    } else {
      if (data->requires_broadcast) {
        TF_LITE_ADD(optimized_ops, BroadcastAdd4DSlow, int32_t);
      } else {
        TF_LITE_ADD(optimized_ops, Add, int32_t);
      }
    }
  } else if (output->type == kTfLiteFloat32) {
    if (kernel_type == kReference) {
      if (data->requires_broadcast) {
        TF_LITE_ADD(reference_ops, BroadcastAdd4DSlow, float);
      } else {
        TF_LITE_ADD(reference_ops, Add, float);
      }
    } else {
      if (data->requires_broadcast) {
        TF_LITE_ADD(optimized_ops, BroadcastAdd4DSlow, float);
      } else {
        TF_LITE_ADD(optimized_ops, Add, float);
      }
    }
  }
#undef TF_LITE_ADD
}

template <KernelType kernel_type>
TfLiteStatus EvalAddQuantized(TfLiteContext* context, TfLiteNode* node,
                              TfLiteAddParams* params, const OpData* data,
                              const TfLiteTensor* input1,
                              const TfLiteTensor* input2,
                              TfLiteTensor* output) {
  if (output->type == kTfLiteUInt8) {
    tflite::ArithmeticParams op_params;
    op_params.left_shift = data->left_shift;
    op_params.input1_offset = data->input1_offset;
    op_params.input1_multiplier = data->input1_multiplier;
    op_params.input1_shift = data->input1_shift;
    op_params.input2_offset = data->input2_offset;
    op_params.input2_multiplier = data->input2_multiplier;
    op_params.input2_shift = data->input2_shift;
    op_params.output_offset = data->output_offset;
    op_params.output_multiplier = data->output_multiplier;
    op_params.output_shift = data->output_shift;
    SetActivationParams(data->output_activation_min,
                        data->output_activation_max, &op_params);
    bool need_broadcast = optimized_ops::ProcessBroadcastShapes(
        GetTensorShape(input1), GetTensorShape(input2), &op_params);
#define TF_LITE_ADD(type, opname)                                      \
  type::opname(op_params, GetTensorShape(input1),                      \
               GetTensorData<uint8_t>(input1), GetTensorShape(input2), \
               GetTensorData<uint8_t>(input2), GetTensorShape(output), \
               GetTensorData<uint8_t>(output));
    if (kernel_type == kReference) {
      if (need_broadcast) {
        TF_LITE_ADD(reference_ops, BroadcastAdd4DSlow);
      } else {
        TF_LITE_ADD(reference_ops, Add);
      }
    } else {
      if (op_params.broadcast_category ==
          BroadcastableOpCategory::kGenericBroadcast) {
        TF_LITE_ADD(optimized_ops, BroadcastAdd4DSlow);
      } else if (need_broadcast) {
        TF_LITE_ADD(optimized_ops, BroadcastAddFivefold);
      } else {
        TF_LITE_ADD(optimized_ops, Add);
      }
    }
#undef TF_LITE_ADD
  } else if (output->type == kTfLiteInt16) {
#define TF_LITE_ADD(type, opname)                                      \
  tflite::ArithmeticParams op_params;                                  \
  op_params.input1_shift = data->input1_shift;                         \
  op_params.input2_shift = data->input2_shift;                         \
  SetActivationParams(data->output_activation_min,                     \
                      data->output_activation_max, &op_params);        \
  type::opname(op_params, GetTensorShape(input1),                      \
               GetTensorData<int16_t>(input1), GetTensorShape(input2), \
               GetTensorData<int16_t>(input2), GetTensorShape(output), \
               GetTensorData<int16_t>(output))
    // The quantized version of Add doesn't support activations, so we
    // always use BroadcastAdd.
    if (kernel_type == kReference) {
      TF_LITE_ADD(reference_ops, Add);
    } else {
      TF_LITE_ADD(optimized_ops, Add);
    }
#undef TF_LITE_ADD
  }

  return kTfLiteOk;
}

template <KernelType kernel_type>
TfLiteStatus Eval(TfLiteContext* context, TfLiteNode* node) {
  auto* params = reinterpret_cast<TfLiteAddParams*>(node->builtin_data);
  OpData* data = reinterpret_cast<OpData*>(node->user_data);

  const TfLiteTensor* input1 = GetInput(context, node, kInputTensor1);
  const TfLiteTensor* input2 = GetInput(context, node, kInputTensor2);
  TfLiteTensor* output = GetOutput(context, node, kOutputTensor);

  if (output->type == kTfLiteFloat32 || output->type == kTfLiteInt32) {
    EvalAdd<kernel_type>(context, node, params, data, input1, input2, output);
  } else if (output->type == kTfLiteUInt8 || output->type == kTfLiteInt16) {
    TF_LITE_ENSURE_OK(context,
                      EvalAddQuantized<kernel_type>(context, node, params, data,
                                                    input1, input2, output));
  } else {
    context->ReportError(context,
                         "Inputs and outputs not all float|uint8|int16 types.");
    return kTfLiteError;
  }

  return kTfLiteOk;
}

}  // namespace add

TfLiteRegistration* Register_ADD_REF() {
  static TfLiteRegistration r = {add::Init, add::Free, add::Prepare,
                                 add::Eval<add::kReference>};
  return &r;
}

TfLiteRegistration* Register_ADD_GENERIC_OPT() {
  static TfLiteRegistration r = {add::Init, add::Free, add::Prepare,
                                 add::Eval<add::kGenericOptimized>};
  return &r;
}

TfLiteRegistration* Register_ADD_NEON_OPT() {
  static TfLiteRegistration r = {add::Init, add::Free, add::Prepare,
                                 add::Eval<add::kNeonOptimized>};
  return &r;
}

TfLiteRegistration* Register_ADD() {
#ifdef USE_NEON
  return Register_ADD_NEON_OPT();
#else
  return Register_ADD_GENERIC_OPT();
#endif
}

}  // namespace builtin
}  // namespace ops
}  // namespace tflite
