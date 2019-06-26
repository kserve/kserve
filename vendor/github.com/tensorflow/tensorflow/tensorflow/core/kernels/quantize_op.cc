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

// See docs in ../ops/math_ops.cc.

#define EIGEN_USE_THREADS

#include "tensorflow/core/framework/op.h"
#include "tensorflow/core/framework/op_kernel.h"
#include "tensorflow/core/framework/type_traits.h"
#include "tensorflow/core/framework/types.h"
#include "tensorflow/core/kernels/cwise_ops.h"
#include "tensorflow/core/kernels/meta_support.h"
#include "tensorflow/core/kernels/quantization_utils.h"
#include "tensorflow/core/lib/core/errors.h"

namespace {
enum {
  QUANTIZE_MODE_MIN_COMBINED,
  QUANTIZE_MODE_MIN_FIRST,
  QUANTIZE_MODE_SCALED,
};
enum {
  // Round half away from zero: if the fraction of y is exactly 0.5, then
  // round(y) = y + 0.5 if y > 0
  // round(y) = y - 0.5 if y < 0
  // E.g., -5.5 gets rounded to -6, -5.4 goes to -5,
  // 5.4 goes to 5, and 5.5 goes to 6.
  ROUND_HALF_AWAY_FROM_ZERO,
  // Round half to even: if the fraction of y is exactly 0.5, then round(y) is
  // the nearest even integer to y.
  // E.g., 23.5 gets rounded to 24, 24.5 gets rounded to 24, while -23.5 becomes
  // -24, and -24.5 gets rounded to 24.
  ROUND_HALF_TO_EVEN,
};
}  // namespace

namespace tensorflow {

typedef Eigen::ThreadPoolDevice CPUDevice;

// Quantize a tensor from float to T, with user-specified min_range and
// max_range.
// TODO(xbing): Add a new QuantizeOp just taking scale,
//              rather than min_range and max_range.
template <typename Device, typename T>
class QuantizeV2Op : public OpKernel {
 public:
  explicit QuantizeV2Op(OpKernelConstruction* ctx) : OpKernel(ctx) {
    half_range_ =
        !std::is_signed<T>::value
            ? 0.0f
            : (static_cast<double>(std::numeric_limits<T>::max()) -
               static_cast<double>(std::numeric_limits<T>::min()) + 1) /
                  2.0f;
    string mode_string;
    OP_REQUIRES_OK(ctx, ctx->GetAttr("mode", &mode_string));
    OP_REQUIRES(ctx,
                (mode_string == "MIN_COMBINED" || mode_string == "MIN_FIRST" ||
                 mode_string == "SCALED"),
                errors::InvalidArgument("Mode string must be 'MIN_COMBINED',"
                                        " 'MIN_FIRST', or 'SCALED', is '" +
                                        mode_string + "'"));
    if (mode_string == "MIN_COMBINED") {
      mode_ = QUANTIZE_MODE_MIN_COMBINED;
    } else if (mode_string == "MIN_FIRST") {
      mode_ = QUANTIZE_MODE_MIN_FIRST;
    } else if (mode_string == "SCALED") {
      mode_ = QUANTIZE_MODE_SCALED;
    }

    string round_mode_string;
    OP_REQUIRES_OK(ctx, ctx->GetAttr("round_mode", &round_mode_string));
    OP_REQUIRES(ctx,
                (round_mode_string == "HALF_AWAY_FROM_ZERO" ||
                 round_mode_string == "HALF_TO_EVEN"),
                errors::InvalidArgument("Round mode string must be "
                                        "'HALF_AWAY_FROM_ZERO' or "
                                        "'HALF_TO_EVEN', is '" +
                                        round_mode_string + "'"));
    if (round_mode_string == "HALF_AWAY_FROM_ZERO") {
      round_mode_ = ROUND_HALF_AWAY_FROM_ZERO;
    } else if (round_mode_string == "HALF_TO_EVEN") {
      OP_REQUIRES(ctx, mode_string == "SCALED",
                  errors::InvalidArgument("Round mode 'HALF_TO_EVEN' "
                                          "only supported for mode 'SCALED', "
                                          "but mode is '" +
                                          mode_string + "'."));
      round_mode_ = ROUND_HALF_TO_EVEN;
    }
  }

  void Compute(OpKernelContext* ctx) override {
    const Tensor& input = ctx->input(0);
    const float input_min_range = ctx->input(1).flat<float>()(0);
    const float input_max_range = ctx->input(2).flat<float>()(0);

    float min_range;
    float max_range;
    OP_REQUIRES(ctx, !(input_max_range < input_min_range),
                errors::InvalidArgument(
                    "input_max_range must be larger than input_min_range."));

    // When the minimum and maximum ranges are too close together, nudge them
    // apart by a small value so that they are slightly different. This helps
    // us avoid creating ill-formed buffers where all quantized values map to
    // the same float number. These kinds of buffers cause problems for
    // downstream ops when they need to do calculations on them.
    // We pick the value by making sure that zero is not more than 100x the
    // overall range from the maximum, so that the value can be easily
    // represented when we promote the quantized value to a higher
    // intermediate bit depth, since that's a common requirement.
    min_range = std::min(0.0f, input_min_range);
    const float epsilon = std::max(1.0f, std::max(fabsf(input_min_range),
                                                  fabsf(input_max_range))) /
                          100.0f;
    max_range = std::max(input_max_range, min_range + epsilon);
    max_range = std::max(0.0f, max_range);

    Tensor* output = nullptr;
    OP_REQUIRES_OK(ctx, ctx->allocate_output(0, input.shape(), &output));
    typename TTypes<T>::Vec o = output->template flat<T>();
    if (mode_ == QUANTIZE_MODE_MIN_COMBINED) {
      const float scale_factor =
          (static_cast<double>(std::numeric_limits<T>::max()) -
           static_cast<double>(std::numeric_limits<T>::min())) /
          (max_range - min_range);

      // Quantize:
      // Make input in range of [min_range, max_range], then
      // subtract min_range to be in range of [0, max_range - min_range]
      // Divide by (max_range - min_range) to get to [0, 1.0]
      // Multiply by range of T, after that shift left 1/2 range of T if
      // T is signed.
      // Note that the number is rounded before the cast. Rounding follows the
      // semantic of std::round, which implements "round-half-away-zero",
      // e.g., -5.5 gets rounded to -6, -5.4 goes to -5, 5.4 goes to 5,
      // and 5.5 goes to 6.
      bool is_signed = std::is_signed<T>::value;
      if (is_signed) {
        // The slow path.
        // TODO(xbing,yonghui): Speedup this path as well.
        o.device(ctx->template eigen_device<Device>()) =
            ((input.flat<float>().cwiseMin(max_range).cwiseMax(min_range) -
              min_range) *
                 scale_factor -
             half_range_)
                .round()
                .template cast<T>();
      } else {
        // The fast path that avoids unaryExpr
        // According to the micro-benchmark, adding device here doesn't help.
        o = ((input.flat<float>().cwiseMin(max_range).cwiseMax(min_range) -
              min_range) *
                 scale_factor +
             0.5f)
                .template cast<T>();
      }
    } else if (mode_ == QUANTIZE_MODE_MIN_FIRST) {
      if (meta::IsSupportedAndEnabled() && std::is_same<T, quint8>()) {
        TTypes<const float>::Vec input_array = input.flat<float>();

        meta::Quantize(ctx, input_array.data(), input_array.size(), min_range,
                       max_range, output->flat<quint8>().data());
      } else {
        FloatTensorToQuantizedInPlaceUsingEigen<T>(
            ctx->template eigen_device<Device>(), input, min_range, max_range,
            output);
      }
    } else if (mode_ == QUANTIZE_MODE_SCALED) {
      const int min_output_value = std::numeric_limits<T>::min();
      const int max_output_value = std::numeric_limits<T>::max();
      const float scale_factor_from_min_side =
          (min_output_value * min_range > 0)
              ? min_output_value / min_range
              : std::numeric_limits<float>::max();
      const float scale_factor_from_max_side =
          (max_output_value * max_range > 0)
              ? max_output_value / max_range
              : std::numeric_limits<float>::max();
      const float scale_factor =
          std::min(scale_factor_from_min_side, scale_factor_from_max_side);
      min_range = min_output_value / scale_factor;
      max_range = max_output_value / scale_factor;
      if (round_mode_ == ROUND_HALF_TO_EVEN) {
        // scalar_round_op_google implements "round-half-to-even".
        o.device(ctx->template eigen_device<Device>()) =
            (input.flat<float>().cwiseMin(max_range).cwiseMax(min_range) *
             scale_factor)
                .unaryExpr(Eigen::internal::scalar_round_op_google<float>())
                .template cast<T>();
      } else if (round_mode_ == ROUND_HALF_AWAY_FROM_ZERO) {
        // scalar_round_op implements "round-half-away-from-zero".
        o.device(ctx->template eigen_device<Device>()) =
            (input.flat<float>().cwiseMin(max_range).cwiseMax(min_range) *
             scale_factor)
                .unaryExpr(Eigen::internal::scalar_round_op<float>())
                .template cast<T>();
      }
    }

    Tensor* output_min_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->allocate_output(1, {}, &output_min_tensor));
    output_min_tensor->flat<float>()(0) = min_range;

    Tensor* output_max_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->allocate_output(2, {}, &output_max_tensor));
    output_max_tensor->flat<float>()(0) = max_range;
  }

 private:
  float half_range_;
  int mode_;
  int round_mode_;
};

REGISTER_KERNEL_BUILDER(
    Name("QuantizeV2").Device(DEVICE_CPU).TypeConstraint<quint8>("T"),
    QuantizeV2Op<CPUDevice, quint8>);
REGISTER_KERNEL_BUILDER(
    Name("QuantizeV2").Device(DEVICE_CPU).TypeConstraint<qint8>("T"),
    QuantizeV2Op<CPUDevice, qint8>);
REGISTER_KERNEL_BUILDER(
    Name("QuantizeV2").Device(DEVICE_CPU).TypeConstraint<quint16>("T"),
    QuantizeV2Op<CPUDevice, quint16>);
REGISTER_KERNEL_BUILDER(
    Name("QuantizeV2").Device(DEVICE_CPU).TypeConstraint<qint16>("T"),
    QuantizeV2Op<CPUDevice, qint16>);
REGISTER_KERNEL_BUILDER(
    Name("QuantizeV2").Device(DEVICE_CPU).TypeConstraint<qint32>("T"),
    QuantizeV2Op<CPUDevice, qint32>);
}  // namespace tensorflow
