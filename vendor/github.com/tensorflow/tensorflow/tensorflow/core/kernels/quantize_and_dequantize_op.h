/* Copyright 2015 The TensorFlow Authors. All Rights Reserved.

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

#ifndef TENSORFLOW_CORE_KERNELS_QUANTIZE_AND_DEQUANTIZE_OP_H_
#define TENSORFLOW_CORE_KERNELS_QUANTIZE_AND_DEQUANTIZE_OP_H_

#include "third_party/eigen3/unsupported/Eigen/CXX11/Tensor"
#include "tensorflow/core/framework/op_kernel.h"
#include "tensorflow/core/framework/tensor_types.h"
#include "tensorflow/core/kernels/cwise_ops.h"

namespace tensorflow {

enum QuantizerRoundMode {
  // Round half up: if the fraction of y is exactly 0.5, then
  // round(y) = y + 0.5
  // E.g., -5.5 gets rounded to -5, -5.4 goes to -5,
  // 5.4 goes to 5, and 5.5 goes to 6.
  ROUND_HALF_UP,
  // Round half to even: if the fraction of y is exactly 0.5, then round(y) is
  // the nearest even integer to y.
  // E.g., 23.5 gets rounded to 24, 24.5 gets rounded to 24, while -23.5 becomes
  // -24, and -24.5 gets rounded to 24.
  ROUND_HALF_TO_EVEN,
};

namespace functor {

// TODO(pauldonnelly): 'signed_input' should really be called 'signed_output'.

template <typename Device, typename T>
struct QuantizeAndDequantizeOneScaleFunctor {
  void operator()(const Device& d, typename TTypes<T>::ConstVec input,
                  bool signed_input, int num_bits, bool range_given,
                  Tensor* input_min_tensor, Tensor* input_max_tensor,
                  QuantizerRoundMode round_mode, typename TTypes<T>::Vec out);
};

// The implementation below runs on both CPU and GPU.
template <typename Device, typename T, typename Func>
void ClampScaleAndRound(const Device& d, typename TTypes<T>::ConstVec input,
                        T min_range, T max_range, T scale, T inverse_scale,
                        Func round_func, typename TTypes<T>::Vec out) {
  out.device(d) = (input.cwiseMin(max_range).cwiseMax(min_range) * scale)
                      .unaryExpr(round_func) *
                  inverse_scale;
}

// The implementation below runs on both CPU and GPU.
template <typename Device, typename T>
void ClampScaleAndRound(const Device& d, typename TTypes<T>::ConstVec input,
                        T min_range, T max_range, T scale, T inverse_scale,
                        QuantizerRoundMode round_mode,
                        typename TTypes<T>::Vec out) {
  switch (round_mode) {
    case ROUND_HALF_TO_EVEN:
      ClampScaleAndRound(d, input, min_range, max_range, scale, inverse_scale,
                         Eigen::internal::scalar_round_op_google<T>(), out);
      break;
    case ROUND_HALF_UP:
      ClampScaleAndRound(d, input, min_range, max_range, scale, inverse_scale,
                         Eigen::internal::scalar_round_up_op<T>(), out);
      break;
  }
}

// The implementation below runs on both CPU and GPU.
template <typename Device, typename T, typename Func>
void ScaleAndRound(const Device& d, typename TTypes<T>::ConstVec input, T scale,
                   T inverse_scale, Func round_func,
                   typename TTypes<T>::Vec out) {
  out.device(d) = (input * scale).unaryExpr(round_func) * inverse_scale;
}

// The implementation below runs on both CPU and GPU.
template <typename Device, typename T>
void ScaleAndRound(const Device& d, typename TTypes<T>::ConstVec input, T scale,
                   T inverse_scale, QuantizerRoundMode round_mode,
                   typename TTypes<T>::Vec out) {
  switch (round_mode) {
    case ROUND_HALF_TO_EVEN:
      ScaleAndRound(d, input, scale, inverse_scale,
                    Eigen::internal::scalar_round_op_google<T>(), out);
      break;
    case ROUND_HALF_UP:
      ScaleAndRound(d, input, scale, inverse_scale,
                    Eigen::internal::scalar_round_up_op<T>(), out);
      break;
  }
}

// The implementation below runs on both CPU and GPU.
template <typename Device, typename T>
struct QuantizeAndDequantizeOneScaleImpl {
  static void Compute(const Device& d, typename TTypes<T>::ConstVec input,
                      bool signed_input, int num_bits, bool range_given,
                      Tensor* input_min_tensor, Tensor* input_max_tensor,
                      QuantizerRoundMode round_mode,
                      typename TTypes<T>::Vec out) {
    T min_range;
    T max_range;
    auto input_min = input_min_tensor->scalar<T>();
    auto input_max = input_max_tensor->scalar<T>();
    if (!range_given) {
      input_min.device(d) = input.minimum();
      input_max.device(d) = input.maximum();
      d.memcpyDeviceToHost(&min_range, input_min.data(), sizeof(T));
      d.memcpyDeviceToHost(&max_range, input_max.data(), sizeof(T));
    } else {
      // Copy the range values from their respective tensors on the host.
      min_range = input_min_tensor->scalar<T>()();
      max_range = input_max_tensor->scalar<T>()();
    }

    // Calculate the range for the simulated integer quantization:
    // e.g. [-128,127] for signed = true, num_bits = 8,
    // or [0, 255] for signed = false, num_bits = 8.
    const int64 min_quantized = signed_input ? -(1ULL << (num_bits - 1)) : 0;
    const int64 max_quantized = min_quantized + ((1ULL << num_bits) - 1);

    // Determine the maximum scaling factor that would scale
    // [min_range, max_range] to not exceed [min_quantized, max_quantized],
    // while keeping 0 unchanged.
    const T scale_from_min_side = (min_quantized * min_range > 0)
                                      ? min_quantized / min_range
                                      : std::numeric_limits<T>::max();
    const T scale_from_max_side = (max_quantized * max_range > 0)
                                      ? max_quantized / max_range
                                      : std::numeric_limits<T>::max();

    // Note: Avoids changing the side of the range that determines scale.
    T scale, inverse_scale;
    if (scale_from_min_side < scale_from_max_side) {
      scale = scale_from_min_side;
      inverse_scale = min_range / min_quantized;
      max_range = max_quantized * inverse_scale;
    } else {
      scale = scale_from_max_side;
      inverse_scale = max_range / max_quantized;
      min_range = min_quantized * inverse_scale;
    }

    if (range_given) {
      // Note: The clamping here is to avoid overflow in the quantized type.
      // The semantics of the op does not guarantee to clamp to the specified
      // min_range and max_range - because we may have changed either min_range
      // or max_range.
      ClampScaleAndRound(d, input, min_range, max_range, scale, inverse_scale,
                         round_mode, out);
    } else {
      ScaleAndRound(d, input, scale, inverse_scale, round_mode, out);
    }
  }
};

}  // end of namespace functor
}  // end of namespace tensorflow

#endif  // TENSORFLOW_CORE_KERNELS_QUANTIZE_AND_DEQUANTIZE_OP_H_
