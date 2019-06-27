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
#include <string.h>
#include <cmath>
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
namespace strided_slice {

enum KernelType {
  kReference,
  // TODO(soroosh): add kGenericOptimized
};

constexpr int kInputTensor = 0;
constexpr int kBeginTensor = 1;
constexpr int kEndTensor = 2;
constexpr int kStridesTensor = 3;
constexpr int kOutputTensor = 0;

struct StridedSliceContext {
  StridedSliceContext(TfLiteContext* context, TfLiteNode* node) {
    params = reinterpret_cast<TfLiteStridedSliceParams*>(node->builtin_data);
    input = GetInput(context, node, kInputTensor);
    begin = GetInput(context, node, kBeginTensor);
    end = GetInput(context, node, kEndTensor);
    strides = GetInput(context, node, kStridesTensor);
    output = GetOutput(context, node, kOutputTensor);
    dims = NumDimensions(input);
  }
  const TfLiteStridedSliceParams* params;
  const TfLiteTensor* input;
  const TfLiteTensor* begin;
  const TfLiteTensor* end;
  const TfLiteTensor* strides;
  TfLiteTensor* output;
  int dims;
};

// This Op only supports 1-4D cases and since we use the reference 4D
// implementation, the 1-3D tensors are mapped to 4D.
const int kMaxDim = 4;

inline int32_t PositiveRemainder(int32_t dividend, int32_t divisor) {
  return (divisor + (dividend % divisor)) % divisor;
}

inline int32_t ClampedIndex(int32_t index, int dim, bool pos_stride) {
  return pos_stride
             ? (index >= dim ? dim
                             : PositiveRemainder(
                                   std::min(std::max(index, -dim), dim), dim))
             : (index < -dim
                    ? -1
                    : PositiveRemainder(
                          std::min(std::max(index, -dim), dim - 1), dim));
}

// TODO(b/77971377) this logic should be removed, as it's a duplication of
// StartForAxis() & StopForAxis() in kernels/internal/reference/reference_ops.h
inline int32_t GetBeginValueAtIndex(StridedSliceContext* op_context, int idx) {
  const int dim = op_context->input->dims->data[idx];
  const bool pos_stride = GetTensorData<int32_t>(op_context->strides)[idx] > 0;
  return op_context->params->begin_mask & (1 << idx)
             ? pos_stride ? 0 : dim - 1
             : ClampedIndex(GetTensorData<int32_t>(op_context->begin)[idx], dim,
                            pos_stride);
}

inline int32_t GetEndValueAtIndex(StridedSliceContext* op_context, int idx) {
  const int dim = op_context->input->dims->data[idx];
  const bool pos_stride = GetTensorData<int32_t>(op_context->strides)[idx] > 0;
  return op_context->params->end_mask & (1 << idx)
             ? pos_stride ? dim : -1
             : ClampedIndex(GetTensorData<int32_t>(op_context->end)[idx], dim,
                            pos_stride);
}

// Processes the indexing tensors (begin, end and strides) to resize the
// output tensor. This function is callable from both Prepare() and Eval() as
// long as the caller ensures the indexing tensors are present.
TfLiteStatus ResizeOutputTensor(TfLiteContext* context,
                                StridedSliceContext* op_context) {
  std::vector<int> output_shape_vector;

  for (int idx = op_context->dims - 1; idx >= 0; --idx) {
    int32_t stride = GetTensorData<int32_t>(op_context->strides)[idx];
    TF_LITE_ENSURE_MSG(context, stride != 0, "stride value has to be non-zero");

    int32_t begin = GetBeginValueAtIndex(op_context, idx);
    int32_t end = GetEndValueAtIndex(op_context, idx);

    // When shrinking an axis, the end position does not matter (and can be
    // incorrect when negative indexing is used, see Issue #19260). Always use
    // begin + 1 to generate a length 1 slice, since begin has
    // already been adjusted for negative indices by GetBeginValueAtIndex.
    const bool shrink_axis = op_context->params->shrink_axis_mask & (1 << idx);
    if (shrink_axis) {
      end = begin + 1;
    }

    // This is valid for both positive and negative strides
    int32_t dim_shape = ceil((end - begin) / static_cast<float>(stride));
    dim_shape = dim_shape < 0 ? 0 : dim_shape;
    if (!shrink_axis) {
      output_shape_vector.push_back(dim_shape);
    }
  }

  TfLiteIntArray* output_shape =
      TfLiteIntArrayCreate(output_shape_vector.size());

  std::reverse_copy(output_shape_vector.begin(), output_shape_vector.end(),
                    output_shape->data);

  TF_LITE_ENSURE_STATUS(
      context->ResizeTensor(context, op_context->output, output_shape));

  return kTfLiteOk;
}

TfLiteStatus Prepare(TfLiteContext* context, TfLiteNode* node) {
  TF_LITE_ENSURE_EQ(context, NumInputs(node), 4);
  TF_LITE_ENSURE_EQ(context, NumOutputs(node), 1);

  StridedSliceContext op_context(context, node);

  // Ensure validity of input tensor and its dimension
  TF_LITE_ENSURE_EQ(context, NumDimensions(op_context.begin), 1);
  TF_LITE_ENSURE_EQ(context, NumDimensions(op_context.end), 1);
  TF_LITE_ENSURE_EQ(context, NumDimensions(op_context.strides), 1);
  TF_LITE_ENSURE_EQ(context, op_context.input->type, op_context.output->type);
  // Only INT32 begin/end/strides are supported
  // TODO(soroosh) add support for INT64
  TF_LITE_ENSURE_EQ(context, op_context.begin->type, kTfLiteInt32);
  TF_LITE_ENSURE_EQ(context, op_context.end->type, kTfLiteInt32);
  TF_LITE_ENSURE_EQ(context, op_context.strides->type, kTfLiteInt32);
  TF_LITE_ENSURE_MSG(context, op_context.dims <= 4,
                     "StridedSlice op only supports 1D-4D input arrays.");

  // TODO(soroosh): add the following missing functionalities
  TF_LITE_ENSURE_MSG(context, op_context.params->ellipsis_mask == 0,
                     "ellipsis_mask is not implemented yet.");
  TF_LITE_ENSURE_MSG(context, op_context.params->new_axis_mask == 0,
                     "new_axis_mask is not implemented yet.");

  // Postpone allocation of output if any of the indexing tensors is not
  // constant
  if (!(IsConstantTensor(op_context.begin) &&
        IsConstantTensor(op_context.end) &&
        IsConstantTensor(op_context.strides))) {
    SetTensorToDynamic(op_context.output);
    return kTfLiteOk;
  }
  return ResizeOutputTensor(context, &op_context);
}

template <KernelType kernel_type>
TfLiteStatus Eval(TfLiteContext* context, TfLiteNode* node) {
  StridedSliceContext op_context(context, node);

  if (IsDynamicTensor(op_context.output)) {
    TF_LITE_ENSURE_OK(context, ResizeOutputTensor(context, &op_context));
  }

  std::vector<int32_t> starts;
  std::vector<int32_t> stops;
  std::vector<int32_t> strides;

  for (int i = op_context.dims; i < kMaxDim; i++) {
    starts.emplace_back(0);
    stops.emplace_back(1);
    strides.emplace_back(1);
  }

  for (int idx = 0; idx < op_context.dims; ++idx) {
    starts.emplace_back(GetTensorData<int32_t>(op_context.begin)[idx]);
    stops.emplace_back(GetTensorData<int32_t>(op_context.end)[idx]);
    strides.emplace_back(GetTensorData<int32_t>(op_context.strides)[idx]);
  }

  int begin_mask = op_context.params->begin_mask << (4 - op_context.dims);
  int end_mask = op_context.params->end_mask << (4 - op_context.dims);
  int shrink_axis_mask = op_context.params->shrink_axis_mask
                         << (4 - op_context.dims);
  TF_LITE_ENSURE_EQ(context, starts.size(), 4);
  auto op_params = ::tflite::strided_slice::BuildStridedSliceParams(
      begin_mask, end_mask, shrink_axis_mask, starts, stops, strides);

#define TF_LITE_STRIDED_SLICE(kernel_type, data_type)                    \
  kernel_type::StridedSlice(op_params, GetTensorShape(op_context.input), \
                            GetTensorData<data_type>(op_context.input),  \
                            GetTensorShape(op_context.output),           \
                            GetTensorData<data_type>(op_context.output))

  switch (op_context.input->type) {
    case kTfLiteFloat32:
      if (kernel_type == kReference) {
        TF_LITE_STRIDED_SLICE(reference_ops, float);
      }
      break;
    case kTfLiteInt32:
      if (kernel_type == kReference) {
        TF_LITE_STRIDED_SLICE(reference_ops, int32_t);
      }
      break;
    case kTfLiteInt64:
      if (kernel_type == kReference) {
        TF_LITE_STRIDED_SLICE(reference_ops, int64_t);
      }
      break;
    case kTfLiteUInt8:
      if (kernel_type == kReference) {
        TF_LITE_STRIDED_SLICE(reference_ops, uint8_t);
      }
      break;
    default:
      context->ReportError(context,
                           "Type %d is currently not supported "
                           "by StridedSlice.",
                           op_context.input->type);
      return kTfLiteError;
  }
#undef TF_LITE_STRIDED_SLICE
  return kTfLiteOk;
}

}  // namespace strided_slice

TfLiteRegistration* Register_STRIDED_SLICE_REF() {
  static TfLiteRegistration r = {
      nullptr, nullptr, strided_slice::Prepare,
      strided_slice::Eval<strided_slice::kReference>};
  return &r;
}

// TODO(soroosh): add optimized
TfLiteRegistration* Register_STRIDED_SLICE() {
  return Register_STRIDED_SLICE_REF();
}

}  // namespace builtin
}  // namespace ops
}  // namespace tflite
