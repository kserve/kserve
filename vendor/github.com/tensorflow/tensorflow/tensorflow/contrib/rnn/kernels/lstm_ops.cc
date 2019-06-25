/* Copyright 2016 The TensorFlow Authors. All Rights Reserved.

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

#define EIGEN_USE_THREADS

#if GOOGLE_CUDA
#define EIGEN_USE_GPU
#endif  // GOOGLE_CUDA

#include "tensorflow/contrib/rnn/kernels/lstm_ops.h"

#include <memory>
#include <vector>

#include "third_party/eigen3/unsupported/Eigen/CXX11/Tensor"
#include "tensorflow/core/framework/op_kernel.h"
#include "tensorflow/core/framework/register_types.h"
#include "tensorflow/core/framework/tensor.h"
#include "tensorflow/core/framework/tensor_shape.h"
#include "tensorflow/core/framework/tensor_types.h"
#include "tensorflow/core/framework/types.h"
#include "tensorflow/core/platform/logging.h"
#include "tensorflow/core/platform/macros.h"

namespace tensorflow {

typedef Eigen::ThreadPoolDevice CPUDevice;
typedef Eigen::GpuDevice GPUDevice;

namespace functor {

template <typename T>
void LSTMBlockCellFpropWithEigen(
    const LSTMBlockCell& cell, OpKernelContext* ctx, const CPUDevice& d,
    const float forget_bias, const float cell_clip, bool use_peephole,
    typename TTypes<T>::ConstMatrix x, typename TTypes<T>::ConstMatrix cs_prev,
    typename TTypes<T>::ConstMatrix h_prev, typename TTypes<T>::ConstMatrix w,
    typename TTypes<T>::ConstVec wci, typename TTypes<T>::ConstVec wcf,
    typename TTypes<T>::ConstVec wco, typename TTypes<T>::ConstVec b,
    typename TTypes<T>::Matrix xh, typename TTypes<T>::Matrix i,
    typename TTypes<T>::Matrix cs, typename TTypes<T>::Matrix f,
    typename TTypes<T>::Matrix o, typename TTypes<T>::Matrix ci,
    typename TTypes<T>::Matrix co, typename TTypes<T>::Matrix icfo,
    typename TTypes<T>::Matrix h) {
  // Concat xh = [x, h].
  xh.slice(cell.xh_x_offsets(), cell.xh_x_extents()).device(d) = x;
  xh.slice(cell.xh_h_offsets(), cell.xh_h_extents()).device(d) = h_prev;

  // states1 = xh * w + b
  typename TTypes<T>::ConstMatrix const_xh(xh.data(), xh.dimensions());
  TensorBlasGemm<CPUDevice, T, false /* USE_CUBLAS */>::compute(
      ctx, d, false, false, typename gemm_compute_type<T>::type(1.f), const_xh,
      w, typename gemm_compute_type<T>::type(0.f), icfo);
  Eigen::array<Eigen::DenseIndex, 2> b_shape({1, b.dimensions()[0]});
  Eigen::array<Eigen::DenseIndex, 2> broadcast_shape({cell.batch_size(), 1});
  icfo.device(d) += b.reshape(b_shape).broadcast(broadcast_shape);

  Eigen::array<Eigen::DenseIndex, 2> p_shape({1, cell.cell_size()});
  Eigen::array<Eigen::DenseIndex, 2> p_broadcast_shape({cell.batch_size(), 1});

  // Input gate.
  if (use_peephole) {
    auto i_peep = cs_prev * wci.reshape(p_shape).broadcast(p_broadcast_shape);
    i.device(d) =
        (icfo.slice(cell.icfo_i_offsets(), cell.cell_extents()) + i_peep)
            .sigmoid();
  } else {
    i.device(d) =
        icfo.slice(cell.icfo_i_offsets(), cell.cell_extents()).sigmoid();
  }

  // Cell input.
  ci.device(d) = icfo.slice(cell.icfo_c_offsets(), cell.cell_extents()).tanh();

  // Forget gate (w/ bias).
  if (use_peephole) {
    auto f_peep = cs_prev * wcf.reshape(p_shape).broadcast(p_broadcast_shape);
    f.device(d) = (icfo.slice(cell.icfo_f_offsets(), cell.cell_extents()) +
                   f.constant(T(forget_bias)) + f_peep)
                      .sigmoid();
  } else {
    f.device(d) = (icfo.slice(cell.icfo_f_offsets(), cell.cell_extents()) +
                   f.constant(T(forget_bias)))
                      .sigmoid();
  }

  // cs = ci .* i + f .* cs_prev
  cs.device(d) = i * ci + f * cs_prev;

  if (cell_clip > 0.0f) {
    cs.device(d) =
        cs.binaryExpr(cs.constant(T(cell_clip)), Eigen::scalar_clip_op<T>());
  }

  // co = tanh(cs)
  co.device(d) = cs.tanh();

  // Output gate.
  if (use_peephole) {
    auto o_peep = cs * wco.reshape(p_shape).broadcast(p_broadcast_shape);
    o.device(d) =
        (icfo.slice(cell.icfo_o_offsets(), cell.cell_extents()) + o_peep)
            .sigmoid();
  } else {
    o.device(d) =
        icfo.slice(cell.icfo_o_offsets(), cell.cell_extents()).sigmoid();
  }

  // h = o .* co
  h.device(d) = o * co;
}

template <typename Device, typename T, bool USE_CUBLAS>
void LSTMBlockCellBpropWithEigen(
    const LSTMBlockCell& cell, OpKernelContext* ctx, const Device& d,
    bool use_peephole, typename TTypes<T>::ConstMatrix x,
    typename TTypes<T>::ConstMatrix cs_prev,
    typename TTypes<T>::ConstMatrix h_prev, typename TTypes<T>::ConstMatrix w,
    typename TTypes<T>::ConstVec wci, typename TTypes<T>::ConstVec wcf,
    typename TTypes<T>::ConstVec wco, typename TTypes<T>::ConstVec b,
    typename TTypes<T>::ConstMatrix i, typename TTypes<T>::ConstMatrix cs,
    typename TTypes<T>::ConstMatrix f, typename TTypes<T>::ConstMatrix o,
    typename TTypes<T>::ConstMatrix ci, typename TTypes<T>::ConstMatrix co,
    typename TTypes<T>::ConstMatrix cs_grad,
    typename TTypes<T>::ConstMatrix h_grad, typename TTypes<T>::Matrix do_,
    typename TTypes<T>::Matrix dcs, typename TTypes<T>::Matrix dci,
    typename TTypes<T>::Matrix df, typename TTypes<T>::Matrix di,
    typename TTypes<T>::Matrix dicfo, typename TTypes<T>::Matrix cs_prev_grad,
    typename TTypes<T>::Vec wci_grad, typename TTypes<T>::Vec wcf_grad,
    typename TTypes<T>::Vec wco_grad) {
  // do[t] = sigm'(o[t]) .* dh[t] .* co[t]
  do_.device(d) = o * (o.constant(T(1)) - o) * h_grad * co;

  // dcs[t] += tanh'(cs[t]) .* dh[t] .* o[t] + dcs[t + 1] .* f[t + 1]
  dcs.device(d) = (co.constant(T(1)) - co * co) * h_grad * o + cs_grad;

  Eigen::array<Eigen::DenseIndex, 2> p_shape({1, cell.cell_size()});
  Eigen::array<Eigen::DenseIndex, 2> p_broadcast_shape({cell.batch_size(), 1});
  if (use_peephole) {
    dcs.device(d) =
        dcs + do_ * wco.reshape(p_shape).broadcast(p_broadcast_shape);
  }

  // dci[t] = tanh'(ci[t]) dcs[t] i[t]
  dci.device(d) = (ci.constant(T(1)) - ci * ci) * dcs * i;

  // df[t] = sigm'(f[t]) dcs[t] cs[t - 1]
  df.device(d) = f * (f.constant(T(1)) - f) * dcs * cs_prev;

  // di[t] = sigm'(i[t]) dcs[t] ci[t]
  di.device(d) = i * (i.constant(T(1)) - i) * dcs * ci;

  dicfo.slice(cell.icfo_i_offsets(), cell.cell_extents()).device(d) = di;
  dicfo.slice(cell.icfo_c_offsets(), cell.cell_extents()).device(d) = dci;
  dicfo.slice(cell.icfo_f_offsets(), cell.cell_extents()).device(d) = df;
  dicfo.slice(cell.icfo_o_offsets(), cell.cell_extents()).device(d) = do_;

  cs_prev_grad.device(d) = dcs * f;
  if (use_peephole) {
    cs_prev_grad.device(d) =
        cs_prev_grad + di * wci.reshape(p_shape).broadcast(p_broadcast_shape) +
        df * wcf.reshape(p_shape).broadcast(p_broadcast_shape);
    wci_grad.device(d) = (di * cs_prev).sum(Eigen::array<int, 1>({0}));
    wcf_grad.device(d) = (df * cs_prev).sum(Eigen::array<int, 1>({0}));
    wco_grad.device(d) = (do_ * cs).sum(Eigen::array<int, 1>({0}));
  }
}

#define DEFINE_CPU_SPECS(T)                                                   \
  template <>                                                                 \
  void LSTMBlockCellFprop<CPUDevice, T, false /* USE_CUBLAS */>::operator()(  \
      OpKernelContext* ctx, const CPUDevice& d, const float forget_bias,      \
      const float cell_clip, bool use_peephole,                               \
      typename TTypes<T>::ConstMatrix x,                                      \
      typename TTypes<T>::ConstMatrix cs_prev,                                \
      typename TTypes<T>::ConstMatrix h_prev,                                 \
      typename TTypes<T>::ConstMatrix w, typename TTypes<T>::ConstVec wci,    \
      typename TTypes<T>::ConstVec wcf, typename TTypes<T>::ConstVec wco,     \
      typename TTypes<T>::ConstVec b, typename TTypes<T>::Matrix xh,          \
      typename TTypes<T>::Matrix i, typename TTypes<T>::Matrix cs,            \
      typename TTypes<T>::Matrix f, typename TTypes<T>::Matrix o,             \
      typename TTypes<T>::Matrix ci, typename TTypes<T>::Matrix co,           \
      typename TTypes<T>::Matrix icfo, typename TTypes<T>::Matrix h) {        \
    LSTMBlockCellFpropWithEigen<T>(                                           \
        *this, ctx, d, forget_bias, cell_clip, use_peephole, x, cs_prev,      \
        h_prev, w, wci, wcf, wco, b, xh, i, cs, f, o, ci, co, icfo, h);       \
  }                                                                           \
  template <>                                                                 \
  void LSTMBlockCellBprop<CPUDevice, T, false /* USE_CUBLAS */>::operator()(  \
      OpKernelContext* ctx, const CPUDevice& d, bool use_peephole,            \
      typename TTypes<T>::ConstMatrix x,                                      \
      typename TTypes<T>::ConstMatrix cs_prev,                                \
      typename TTypes<T>::ConstMatrix h_prev,                                 \
      typename TTypes<T>::ConstMatrix w, typename TTypes<T>::ConstVec wci,    \
      typename TTypes<T>::ConstVec wcf, typename TTypes<T>::ConstVec wco,     \
      typename TTypes<T>::ConstVec b, typename TTypes<T>::ConstMatrix i,      \
      typename TTypes<T>::ConstMatrix cs, typename TTypes<T>::ConstMatrix f,  \
      typename TTypes<T>::ConstMatrix o, typename TTypes<T>::ConstMatrix ci,  \
      typename TTypes<T>::ConstMatrix co,                                     \
      typename TTypes<T>::ConstMatrix cs_grad,                                \
      typename TTypes<T>::ConstMatrix h_grad, typename TTypes<T>::Matrix do_, \
      typename TTypes<T>::Matrix dcs, typename TTypes<T>::Matrix dci,         \
      typename TTypes<T>::Matrix df, typename TTypes<T>::Matrix di,           \
      typename TTypes<T>::Matrix dicfo,                                       \
      typename TTypes<T>::Matrix cs_prev_grad,                                \
      typename TTypes<T>::Vec wci_grad, typename TTypes<T>::Vec wcf_grad,     \
      typename TTypes<T>::Vec wco_grad) {                                     \
    LSTMBlockCellBpropWithEigen<CPUDevice, T, false /* USE_CUBLAS */>(        \
        *this, ctx, d, use_peephole, x, cs_prev, h_prev, w, wci, wcf, wco, b, \
        i, cs, f, o, ci, co, cs_grad, h_grad, do_, dcs, dci, df, di, dicfo,   \
        cs_prev_grad, wci_grad, wcf_grad, wco_grad);                          \
  }                                                                           \
  template struct LSTMBlockCellFprop<CPUDevice, T, false /* USE_CUBLAS */>;   \
  template struct LSTMBlockCellBprop<CPUDevice, T, false /* USE_CUBLAS */>;

DEFINE_CPU_SPECS(float);
DEFINE_CPU_SPECS(Eigen::half);
#undef DEFINE_CPU_SPECS

}  // namespace functor

template <typename Device, typename T, bool USE_CUBLAS>
class LSTMBlockCellOp : public OpKernel {
 public:
  explicit LSTMBlockCellOp(OpKernelConstruction* ctx) : OpKernel(ctx) {
    OP_REQUIRES_OK(ctx, ctx->GetAttr("forget_bias", &forget_bias_));
    OP_REQUIRES_OK(ctx, ctx->GetAttr("cell_clip", &cell_clip_));
    OP_REQUIRES_OK(ctx, ctx->GetAttr("use_peephole", &use_peephole_));
  }

  void Compute(OpKernelContext* ctx) override {
    const Tensor* x_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("x", &x_tensor));

    const Tensor* cs_prev_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("cs_prev", &cs_prev_tensor));

    const Tensor* h_prev_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("h_prev", &h_prev_tensor));

    const Tensor* w_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("w", &w_tensor));

    const Tensor* wci_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("wci", &wci_tensor));

    const Tensor* wcf_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("wcf", &wcf_tensor));

    const Tensor* wco_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("wco", &wco_tensor));

    const Tensor* b_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("b", &b_tensor));

    const int64 batch_size = x_tensor->dim_size(0);
    const int64 input_size = x_tensor->dim_size(1);
    const int64 cell_size = cs_prev_tensor->dim_size(1);

    // Sanity checks for our input shapes.
    OP_REQUIRES(ctx, cs_prev_tensor->dim_size(0) == batch_size,
                errors::InvalidArgument("cs_prev.dims(0) != batch_size: ",
                                        cs_prev_tensor->dim_size(0), " vs. ",
                                        batch_size));
    OP_REQUIRES(ctx, cs_prev_tensor->dim_size(1) == cell_size,
                errors::InvalidArgument("cs_prev.dims(1) != cell_size: ",
                                        cs_prev_tensor->dim_size(1), " vs. ",
                                        cell_size));

    OP_REQUIRES(ctx, h_prev_tensor->dim_size(0) == batch_size,
                errors::InvalidArgument("h_prev.dims(0) != batch_size: ",
                                        h_prev_tensor->dim_size(0), " vs. ",
                                        batch_size));
    OP_REQUIRES(ctx, h_prev_tensor->dim_size(1) == cell_size,
                errors::InvalidArgument(
                    "h_prev.dims(1) != cell_size: ", h_prev_tensor->dim_size(1),
                    " vs. ", cell_size));

    OP_REQUIRES(ctx, w_tensor->dim_size(0) == input_size + cell_size,
                errors::InvalidArgument(
                    "w.dim_size(0) != input_size + cell_size: ",
                    w_tensor->dim_size(0), " vs. ", input_size + cell_size));
    OP_REQUIRES(ctx, w_tensor->dim_size(1) == cell_size * 4,
                errors::InvalidArgument(
                    "w.dim_size(1) != cell_size * 4: ", w_tensor->dim_size(1),
                    " vs. ", cell_size * 4));

    OP_REQUIRES(ctx, b_tensor->dim_size(0) == cell_size * 4,
                errors::InvalidArgument(
                    "b.dim_size(0) != cell_size * 4: ", b_tensor->dim_size(0),
                    " vs. ", cell_size * 4));

    // Allocate our output tensors.
    Tensor* i_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->forward_input_or_allocate_output(
                            {"h_prev"}, "i",
                            TensorShape({batch_size, cell_size}), &i_tensor));

    Tensor* cs_tensor = nullptr;
    OP_REQUIRES_OK(
        ctx, ctx->allocate_output("cs", TensorShape({batch_size, cell_size}),
                                  &cs_tensor));

    Tensor* f_tensor = nullptr;
    OP_REQUIRES_OK(
        ctx, ctx->allocate_output("f", TensorShape({batch_size, cell_size}),
                                  &f_tensor));

    Tensor* o_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->forward_input_or_allocate_output(
                            {"cs_prev"}, "o",
                            TensorShape({batch_size, cell_size}), &o_tensor));

    Tensor* ci_tensor = nullptr;
    OP_REQUIRES_OK(
        ctx, ctx->allocate_output("ci", TensorShape({batch_size, cell_size}),
                                  &ci_tensor));

    Tensor* co_tensor = nullptr;
    OP_REQUIRES_OK(
        ctx, ctx->allocate_output("co", TensorShape({batch_size, cell_size}),
                                  &co_tensor));

    Tensor* h_tensor = nullptr;
    OP_REQUIRES_OK(
        ctx, ctx->allocate_output("h", TensorShape({batch_size, cell_size}),
                                  &h_tensor));

    // Allocate our temp tensors.
    Tensor xh_tensor;
    OP_REQUIRES_OK(ctx, ctx->allocate_temp(
                            DataTypeToEnum<T>::v(),
                            TensorShape({batch_size, input_size + cell_size}),
                            &xh_tensor));

    Tensor icfo_tensor;
    OP_REQUIRES_OK(ctx,
                   ctx->allocate_temp(DataTypeToEnum<T>::v(),
                                      TensorShape({batch_size, cell_size * 4}),
                                      &icfo_tensor));

    const Device& device = ctx->eigen_device<Device>();

    functor::LSTMBlockCellFprop<Device, T, USE_CUBLAS>(batch_size, input_size,
                                                       cell_size)(
        ctx, device, forget_bias_, cell_clip_, use_peephole_,
        x_tensor->matrix<T>(), cs_prev_tensor->matrix<T>(),
        h_prev_tensor->matrix<T>(), w_tensor->matrix<T>(), wci_tensor->vec<T>(),
        wcf_tensor->vec<T>(), wco_tensor->vec<T>(), b_tensor->vec<T>(),
        xh_tensor.matrix<T>(), i_tensor->matrix<T>(), cs_tensor->matrix<T>(),
        f_tensor->matrix<T>(), o_tensor->matrix<T>(), ci_tensor->matrix<T>(),
        co_tensor->matrix<T>(), icfo_tensor.matrix<T>(), h_tensor->matrix<T>());
  }

 private:
  float forget_bias_;
  float cell_clip_;
  bool use_peephole_;
};

#define REGISTER_KERNEL(T)                                             \
  REGISTER_KERNEL_BUILDER(                                             \
      Name("LSTMBlockCell").Device(DEVICE_CPU).TypeConstraint<T>("T"), \
      LSTMBlockCellOp<CPUDevice, T, false>);
REGISTER_KERNEL(float);
REGISTER_KERNEL(Eigen::half);
#undef REGISTER_KERNEL

#if GOOGLE_CUDA
namespace functor {
#define DECLARE_GPU_SPEC(T)                                                \
  template <>                                                              \
  void LSTMBlockCellFprop<GPUDevice, T, true>::operator()(                 \
      OpKernelContext* ctx, const GPUDevice& d, const float forget_bias,   \
      const float cell_clip, bool use_peephole,                            \
      typename TTypes<T>::ConstMatrix x,                                   \
      typename TTypes<T>::ConstMatrix cs_prev,                             \
      typename TTypes<T>::ConstMatrix h_prev,                              \
      typename TTypes<T>::ConstMatrix w, typename TTypes<T>::ConstVec wci, \
      typename TTypes<T>::ConstVec wcf, typename TTypes<T>::ConstVec wco,  \
      typename TTypes<T>::ConstVec b, typename TTypes<T>::Matrix xh,       \
      typename TTypes<T>::Matrix i, typename TTypes<T>::Matrix cs,         \
      typename TTypes<T>::Matrix f, typename TTypes<T>::Matrix o,          \
      typename TTypes<T>::Matrix ci, typename TTypes<T>::Matrix co,        \
      typename TTypes<T>::Matrix icfo, typename TTypes<T>::Matrix h);      \
                                                                           \
  extern template struct LSTMBlockCellFprop<GPUDevice, T, true>;

DECLARE_GPU_SPEC(float);
DECLARE_GPU_SPEC(Eigen::half);
#undef DECLARE_GPU_SPEC
}  // end namespace functor

#define REGISTER_GPU_KERNEL(T)                                         \
  REGISTER_KERNEL_BUILDER(                                             \
      Name("LSTMBlockCell").Device(DEVICE_GPU).TypeConstraint<T>("T"), \
      LSTMBlockCellOp<GPUDevice, T, true>);

REGISTER_GPU_KERNEL(float);
REGISTER_GPU_KERNEL(Eigen::half);
// REGISTER_GPU_KERNEL(double);
#undef REGISTER_GPU_KERNEL
#endif  // GOOGLE_CUDA

template <typename Device, typename T, bool USE_CUBLAS>
class LSTMBlockCellGradOp : public OpKernel {
 public:
  explicit LSTMBlockCellGradOp(OpKernelConstruction* ctx) : OpKernel(ctx) {
    OP_REQUIRES_OK(ctx, ctx->GetAttr("use_peephole", &use_peephole_));
  }

  void Compute(OpKernelContext* ctx) override {
    const Tensor* x_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("x", &x_tensor));

    const Tensor* cs_prev_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("cs_prev", &cs_prev_tensor));

    const Tensor* h_prev_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("h_prev", &h_prev_tensor));

    const Tensor* w_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("w", &w_tensor));

    const Tensor* wci_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("wci", &wci_tensor));

    const Tensor* wcf_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("wcf", &wcf_tensor));

    const Tensor* wco_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("wco", &wco_tensor));

    const Tensor* b_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("b", &b_tensor));

    const Tensor* i_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("i", &i_tensor));

    const Tensor* cs_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("cs", &cs_tensor));

    const Tensor* f_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("f", &f_tensor));

    const Tensor* o_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("o", &o_tensor));

    const Tensor* ci_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("ci", &ci_tensor));

    const Tensor* co_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("co", &co_tensor));

    const Tensor* cs_grad_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("cs_grad", &cs_grad_tensor));

    const Tensor* h_grad_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("h_grad", &h_grad_tensor));

    const int64 batch_size = x_tensor->dim_size(0);
    const int64 input_size = x_tensor->dim_size(1);
    const int64 cell_size = cs_prev_tensor->dim_size(1);

    // Sanity checks for our input shapes.
    OP_REQUIRES(ctx, cs_prev_tensor->dim_size(0) == batch_size,
                errors::InvalidArgument("cs_prev.dims(0) != batch_size: ",
                                        cs_prev_tensor->dim_size(0), " vs. ",
                                        batch_size));
    OP_REQUIRES(ctx, cs_prev_tensor->dim_size(1) == cell_size,
                errors::InvalidArgument("cs_prev.dims(1) != cell_size: ",
                                        cs_prev_tensor->dim_size(1), " vs. ",
                                        cell_size));

    OP_REQUIRES(ctx, h_prev_tensor->dim_size(0) == batch_size,
                errors::InvalidArgument("h_prev.dims(0) != batch_size: ",
                                        h_prev_tensor->dim_size(0), " vs. ",
                                        batch_size));
    OP_REQUIRES(ctx, h_prev_tensor->dim_size(1) == cell_size,
                errors::InvalidArgument(
                    "h_prev.dims(1) != cell_size: ", h_prev_tensor->dim_size(1),
                    " vs. ", cell_size));

    OP_REQUIRES(ctx, w_tensor->dim_size(0) == input_size + cell_size,
                errors::InvalidArgument(
                    "w.dim_size(0) != input_size + cell_size: ",
                    w_tensor->dim_size(0), " vs. ", input_size + cell_size));
    OP_REQUIRES(ctx, w_tensor->dim_size(1) == cell_size * 4,
                errors::InvalidArgument(
                    "w.dim_size(1) != cell_size * 4: ", w_tensor->dim_size(1),
                    " vs. ", cell_size * 4));

    OP_REQUIRES(ctx, b_tensor->dim_size(0) == cell_size * 4,
                errors::InvalidArgument(
                    "b.dim_size(0) != cell_size * 4: ", b_tensor->dim_size(0),
                    " vs. ", cell_size * 4));

    OP_REQUIRES(ctx, i_tensor->dim_size(0) == batch_size,
                errors::InvalidArgument(
                    "i.dim_size(0) != batch_size: ", i_tensor->dim_size(0),
                    " vs. ", batch_size));
    OP_REQUIRES(ctx, i_tensor->dim_size(1) == cell_size,
                errors::InvalidArgument(
                    "i.dim_size(1) != cell_size: ", i_tensor->dim_size(1),
                    " vs. ", cell_size));

    OP_REQUIRES(ctx, cs_tensor->dim_size(0) == batch_size,
                errors::InvalidArgument(
                    "cs.dim_size(0) != batch_size: ", cs_tensor->dim_size(0),
                    " vs. ", batch_size));
    OP_REQUIRES(ctx, cs_tensor->dim_size(1) == cell_size,
                errors::InvalidArgument(
                    "cs.dim_size(1) != cell_size: ", cs_tensor->dim_size(1),
                    " vs. ", cell_size));

    OP_REQUIRES(ctx, f_tensor->dim_size(0) == batch_size,
                errors::InvalidArgument(
                    "f.dim_size(0) != batch_size: ", f_tensor->dim_size(0),
                    " vs. ", batch_size));
    OP_REQUIRES(ctx, f_tensor->dim_size(1) == cell_size,
                errors::InvalidArgument(
                    "i.dim_size(1) != cell_size: ", f_tensor->dim_size(1),
                    " vs. ", cell_size));

    OP_REQUIRES(ctx, o_tensor->dim_size(0) == batch_size,
                errors::InvalidArgument(
                    "o.dim_size(0) != batch_size: ", o_tensor->dim_size(0),
                    " vs. ", batch_size));
    OP_REQUIRES(ctx, o_tensor->dim_size(1) == cell_size,
                errors::InvalidArgument(
                    "o.dim_size(1) != cell_size: ", o_tensor->dim_size(1),
                    " vs. ", cell_size));

    OP_REQUIRES(ctx, ci_tensor->dim_size(0) == batch_size,
                errors::InvalidArgument(
                    "ci.dim_size(0) != batch_size: ", ci_tensor->dim_size(0),
                    " vs. ", batch_size));
    OP_REQUIRES(ctx, ci_tensor->dim_size(1) == cell_size,
                errors::InvalidArgument(
                    "ci.dim_size(1) != cell_size: ", ci_tensor->dim_size(1),
                    " vs. ", cell_size));

    OP_REQUIRES(ctx, co_tensor->dim_size(0) == batch_size,
                errors::InvalidArgument(
                    "co.dim_size(0) != batch_size: ", co_tensor->dim_size(0),
                    " vs. ", batch_size));
    OP_REQUIRES(ctx, co_tensor->dim_size(1) == cell_size,
                errors::InvalidArgument(
                    "co.dim_size(1) != cell_size: ", co_tensor->dim_size(1),
                    " vs. ", cell_size));

    OP_REQUIRES(ctx, cs_grad_tensor->dim_size(0) == batch_size,
                errors::InvalidArgument(
                    "cs_grad_tensor.dims(0) != batch_size: ",
                    cs_grad_tensor->dim_size(0), " vs. ", batch_size));
    OP_REQUIRES(ctx, cs_grad_tensor->dim_size(1) == cell_size,
                errors::InvalidArgument("cs_grad_tensor.dims(1) != cell_size: ",
                                        cs_grad_tensor->dim_size(1), " vs. ",
                                        cell_size));

    OP_REQUIRES(ctx, h_grad_tensor->dim_size(0) == batch_size,
                errors::InvalidArgument("h_grad_tensor.dims(0) != batch_size: ",
                                        h_grad_tensor->dim_size(0), " vs. ",
                                        batch_size));
    OP_REQUIRES(ctx, h_grad_tensor->dim_size(1) == cell_size,
                errors::InvalidArgument("h_grad_tensor.dims(1) != cell_size: ",
                                        h_grad_tensor->dim_size(1), " vs. ",
                                        cell_size));

    // Allocate our output tensors.
    Tensor* cs_prev_grad_tensor = nullptr;
    OP_REQUIRES_OK(
        ctx, ctx->forward_input_or_allocate_output(
                 {"cs_grad"}, "cs_prev_grad",
                 TensorShape({batch_size, cell_size}), &cs_prev_grad_tensor));

    Tensor* dicfo_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->allocate_output(
                            "dicfo", TensorShape({batch_size, cell_size * 4}),
                            &dicfo_tensor));

    Tensor* wci_grad_tensor = nullptr;
    OP_REQUIRES_OK(
        ctx, ctx->forward_input_or_allocate_output(
                 {"wci"}, "wci_grad", wci_tensor->shape(), &wci_grad_tensor));

    Tensor* wcf_grad_tensor = nullptr;
    OP_REQUIRES_OK(
        ctx, ctx->forward_input_or_allocate_output(
                 {"wcf"}, "wcf_grad", wcf_tensor->shape(), &wcf_grad_tensor));

    Tensor* wco_grad_tensor = nullptr;
    OP_REQUIRES_OK(
        ctx, ctx->forward_input_or_allocate_output(
                 {"wco"}, "wco_grad", wco_tensor->shape(), &wco_grad_tensor));

    // Allocate our temp tensors.
    Tensor do_tensor;
    OP_REQUIRES_OK(ctx, ctx->allocate_temp(DataTypeToEnum<T>::v(),
                                           TensorShape({batch_size, cell_size}),
                                           &do_tensor));

    Tensor dcs_tensor;
    OP_REQUIRES_OK(ctx, ctx->allocate_temp(DataTypeToEnum<T>::v(),
                                           TensorShape({batch_size, cell_size}),
                                           &dcs_tensor));

    Tensor dci_tensor;
    OP_REQUIRES_OK(ctx, ctx->allocate_temp(DataTypeToEnum<T>::v(),
                                           TensorShape({batch_size, cell_size}),
                                           &dci_tensor));

    Tensor df_tensor;
    OP_REQUIRES_OK(ctx, ctx->allocate_temp(DataTypeToEnum<T>::v(),
                                           TensorShape({batch_size, cell_size}),
                                           &df_tensor));

    Tensor di_tensor;
    OP_REQUIRES_OK(ctx, ctx->allocate_temp(DataTypeToEnum<T>::v(),
                                           TensorShape({batch_size, cell_size}),
                                           &di_tensor));

    const Device& device = ctx->eigen_device<Device>();

    functor::TensorZero<Device, T>()(device, wci_grad_tensor->flat<T>());
    functor::TensorZero<Device, T>()(device, wcf_grad_tensor->flat<T>());
    functor::TensorZero<Device, T>()(device, wco_grad_tensor->flat<T>());

    functor::LSTMBlockCellBprop<Device, T, USE_CUBLAS>(batch_size, input_size,
                                                       cell_size)(
        ctx, device, use_peephole_, x_tensor->matrix<T>(),
        cs_prev_tensor->matrix<T>(), h_prev_tensor->matrix<T>(),
        w_tensor->matrix<T>(), wci_tensor->vec<T>(), wcf_tensor->vec<T>(),
        wco_tensor->vec<T>(), b_tensor->vec<T>(), i_tensor->matrix<T>(),
        cs_tensor->matrix<T>(), f_tensor->matrix<T>(), o_tensor->matrix<T>(),
        ci_tensor->matrix<T>(), co_tensor->matrix<T>(),
        cs_grad_tensor->matrix<T>(), h_grad_tensor->matrix<T>(),
        do_tensor.matrix<T>(), dcs_tensor.matrix<T>(), dci_tensor.matrix<T>(),
        df_tensor.matrix<T>(), di_tensor.matrix<T>(), dicfo_tensor->matrix<T>(),
        cs_prev_grad_tensor->matrix<T>(), wci_grad_tensor->vec<T>(),
        wcf_grad_tensor->vec<T>(), wco_grad_tensor->vec<T>());
  }

 protected:
  bool use_peephole_;
};

#define REGISTER_KERNEL(T)                                                 \
  REGISTER_KERNEL_BUILDER(                                                 \
      Name("LSTMBlockCellGrad").Device(DEVICE_CPU).TypeConstraint<T>("T"), \
      LSTMBlockCellGradOp<CPUDevice, T, false>);
REGISTER_KERNEL(float);
REGISTER_KERNEL(Eigen::half);
#undef REGISTER_KERNEL

#if GOOGLE_CUDA
namespace functor {
#define DECLARE_GPU_SPEC(T)                                                   \
  template <>                                                                 \
  void LSTMBlockCellBprop<GPUDevice, T, true>::operator()(                    \
      OpKernelContext* ctx, const GPUDevice& d, bool use_peephole,            \
      typename TTypes<T>::ConstMatrix x,                                      \
      typename TTypes<T>::ConstMatrix cs_prev,                                \
      typename TTypes<T>::ConstMatrix h_prev,                                 \
      typename TTypes<T>::ConstMatrix w, typename TTypes<T>::ConstVec wci,    \
      typename TTypes<T>::ConstVec wcf, typename TTypes<T>::ConstVec wco,     \
      typename TTypes<T>::ConstVec b, typename TTypes<T>::ConstMatrix i,      \
      typename TTypes<T>::ConstMatrix cs, typename TTypes<T>::ConstMatrix f,  \
      typename TTypes<T>::ConstMatrix o, typename TTypes<T>::ConstMatrix ci,  \
      typename TTypes<T>::ConstMatrix co,                                     \
      typename TTypes<T>::ConstMatrix cs_grad,                                \
      typename TTypes<T>::ConstMatrix h_grad, typename TTypes<T>::Matrix do_, \
      typename TTypes<T>::Matrix dcs, typename TTypes<T>::Matrix dci,         \
      typename TTypes<T>::Matrix df, typename TTypes<T>::Matrix di,           \
      typename TTypes<T>::Matrix dicfo,                                       \
      typename TTypes<T>::Matrix cs_prev_grad,                                \
      typename TTypes<T>::Vec wci_grad, typename TTypes<T>::Vec wcf_grad,     \
      typename TTypes<T>::Vec wco_grad);                                      \
                                                                              \
  extern template struct LSTMBlockCellBprop<GPUDevice, T,                     \
                                            true /* USE_CUBLAS */>;

DECLARE_GPU_SPEC(float);
DECLARE_GPU_SPEC(Eigen::half);
// DECLARE_GPU_SPEC(double);
#undef DECLARE_GPU_SPEC
}  // namespace functor

#define REGISTER_GPU_KERNEL(T)                                             \
  REGISTER_KERNEL_BUILDER(                                                 \
      Name("LSTMBlockCellGrad").Device(DEVICE_GPU).TypeConstraint<T>("T"), \
      LSTMBlockCellGradOp<GPUDevice, T, true>);

REGISTER_GPU_KERNEL(float);
REGISTER_GPU_KERNEL(Eigen::half);
// REGISTER_GPU_KERNEL(double);
#undef REGISTER_GPU_KERNEL
#endif  // GOOGLE_CUDA

namespace {

// This helper class can be used to access timeslices of a 3D tensor. If a slice
// happens to be unaligned (usually because both batch size and number of cells
// are odd - this isn't common) this involves overhead, since data needs to be
// copied. However, if all slices are aligned, the bits aren't copied. In the
// cases where copying is needed, the outputs have to be recopied back.
// At the end of each time step you should call FinishTimeStep which does this,
// and also allows for reuse of temporary tensors.
template <typename Device, typename T>
class SliceHelper {
 public:
  explicit SliceHelper(OpKernelContext* ctx)
      : ctx_(ctx), device_(ctx_->eigen_device<Device>()) {}

  ~SliceHelper() {
    CHECK(copy_out_.empty());
    for (const auto& entry : pool_) {
      CHECK(!entry.second.second);  // nothing is in use
    }
  }

  // Slice through an input tensor. This may copy unaligned slices, but no
  // copying back will be done at the end.
  const Tensor InputSlice(const Tensor& t, int pos, const string& name) {
    Tensor res = UnalignedSlice(t, pos);
    if (res.IsAligned()) {
      return res;
    } else {
      return AlignTensor(res, name);
    }
  }

  // Slice through an output tensor. This may copy unaligned slices, and
  // schedule copying back on destruction.
  Tensor OutputSlice(Tensor* t, int pos, const string& name) {
    Tensor res = UnalignedSlice(*t, pos);
    if (res.IsAligned()) {
      return res;
    } else {
      Tensor aligned = AlignTensor(res, name);
      copy_out_.emplace_back(res, aligned);
      return aligned;
    }
  }

  void FinishTimeStep() {
    for (const auto& p : copy_out_) {
      const Tensor& aligned = p.second;
      Tensor original = p.first;
      // Copy from aligned back to original.
      functor::TensorCopyToUnaligned<Device, T>()(device_, aligned.flat<T>(),
                                                  original.unaligned_flat<T>());
    }
    copy_out_.clear();
    // Mark all entries as not in use.
    for (auto& entry : pool_) {
      entry.second.second = false;
    }
  }

 private:
  // Return a slice at position 'pos'. Result may be unaligned. The resulting
  // tensor always shares data with the source tensor.
  Tensor UnalignedSlice(const Tensor& t, int pos) const {
    Tensor res;
    // CHECK should never fail here, since the number of elements must match
    CHECK(res.CopyFrom(t.Slice(pos, pos + 1), {t.dim_size(1), t.dim_size(2)}));
    return res;
  }

  // Assumes input is not aligned, creates a temporary aligned tensor of the
  // same shape and copies the original tensor's content into it.
  Tensor AlignTensor(const Tensor& t, const string& name) {
    VLOG(1) << "AlignTensor called for " << name << ", shape "
            << t.shape().DebugString()
            << ". This is unnecessary copying. Consider using shapes with even "
            << "sizes";
    Tensor aligned;
    auto found = pool_.find(name);
    if (found != pool_.end()) {  // found in pool
      CHECK(!found->second.second) << "Tensor " << name << " is in use";
      found->second.second = true;  // mark in use
      aligned = found->second.first;
      CHECK(aligned.shape().IsSameSize(t.shape()));
      CHECK_EQ(aligned.dtype(), t.dtype());
    } else {  // allocate a new temporary tensor
      TF_CHECK_OK(ctx_->allocate_temp(t.dtype(), t.shape(), &aligned));
      pool_.emplace(name, std::make_pair(aligned, true));
    }
    functor::TensorCopyUnaligned<Device, T>()(device_, t.unaligned_flat<T>(),
                                              aligned.flat<T>());
    return aligned;
  }

  // Tensors to be copied.
  std::vector<std::pair<Tensor, const Tensor>> copy_out_;
  // A pool of pre-allocated temporary tensors, with an indicator for whether
  // it's in use.
  std::map<string, std::pair<Tensor, bool>> pool_;
  // Op context
  OpKernelContext* ctx_ = nullptr;
  // Device
  const Device& device_;
};

}  // namespace

template <typename Device, typename T, bool USE_CUBLAS>
class BlockLSTMOp : public OpKernel {
 public:
  explicit BlockLSTMOp(OpKernelConstruction* ctx) : OpKernel(ctx) {
    OP_REQUIRES_OK(ctx, ctx->GetAttr("forget_bias", &forget_bias_));
    OP_REQUIRES_OK(ctx, ctx->GetAttr("cell_clip", &cell_clip_));
    OP_REQUIRES_OK(ctx, ctx->GetAttr("use_peephole", &use_peephole_));
  }

  void Compute(OpKernelContext* ctx) override {
    const Tensor* seq_len_max_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("seq_len_max", &seq_len_max_tensor));

    const Tensor* x;
    OP_REQUIRES_OK(ctx, ctx->input("x", &x));
    OP_REQUIRES(ctx, x->dims() == 3, errors::InvalidArgument("x must be 3D"));
    const int64 timelen = x->dim_size(0);
    const int64 batch_size = x->dim_size(1);
    const int64 input_size = x->dim_size(2);

    const Tensor* cs_prev_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("cs_prev", &cs_prev_tensor));
    OP_REQUIRES(ctx, cs_prev_tensor->dims() == 2,
                errors::InvalidArgument("cs_prev must be 2D"));
    OP_REQUIRES(ctx, cs_prev_tensor->dim_size(0) == batch_size,
                errors::InvalidArgument("cs_prev.dims(0) != batch_size: ",
                                        cs_prev_tensor->dim_size(0), " vs. ",
                                        batch_size));
    const int64 cell_size = cs_prev_tensor->dim_size(1);

    if (batch_size * input_size % 2 == 1) {
      LOG(WARNING) << "BlockLSTMOp is inefficient when both batch_size and "
                   << "input_size are odd. You are using: batch_size="
                   << batch_size << ", input_size=" << input_size;
    }
    if (batch_size * cell_size % 2 == 1) {
      LOG(WARNING) << "BlockLSTMOp is inefficient when both batch_size and "
                   << "cell_size are odd. You are using: batch_size="
                   << batch_size << ", cell_size=" << cell_size;
    }

    const Tensor* h_prev_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("h_prev", &h_prev_tensor));
    OP_REQUIRES(ctx, h_prev_tensor->dims() == 2,
                errors::InvalidArgument("h_prev must be 2D"));
    OP_REQUIRES(ctx, h_prev_tensor->dim_size(0) == batch_size,
                errors::InvalidArgument("h_prev.dims(0) != batch_size: ",
                                        h_prev_tensor->dim_size(0), " vs. ",
                                        batch_size));
    OP_REQUIRES(ctx, h_prev_tensor->dim_size(1) == cell_size,
                errors::InvalidArgument(
                    "h_prev.dims(1) != cell_size: ", h_prev_tensor->dim_size(1),
                    " vs. ", cell_size));

    const Tensor* w_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("w", &w_tensor));
    OP_REQUIRES(ctx, w_tensor->dims() == 2,
                errors::InvalidArgument("w must be 2D"));
    OP_REQUIRES(ctx, w_tensor->dim_size(0) == input_size + cell_size,
                errors::InvalidArgument(
                    "w.dim_size(0) != input_size + cell_size: ",
                    w_tensor->dim_size(0), " vs. ", input_size + cell_size));
    OP_REQUIRES(ctx, w_tensor->dim_size(1) == cell_size * 4,
                errors::InvalidArgument(
                    "w.dim_size(1) != cell_size * 4: ", w_tensor->dim_size(1),
                    " vs. ", cell_size * 4));

    const Tensor* wci_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("wci", &wci_tensor));
    OP_REQUIRES(ctx, wci_tensor->dims() == 1,
                errors::InvalidArgument("wci must be 1D"));
    OP_REQUIRES(ctx, wci_tensor->dim_size(0) == cell_size,
                errors::InvalidArgument(
                    "wci.dim_size(0) != cell_size: ", wci_tensor->dim_size(0),
                    " vs. ", cell_size));

    const Tensor* wcf_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("wcf", &wcf_tensor));
    OP_REQUIRES(ctx, wcf_tensor->dims() == 1,
                errors::InvalidArgument("wcf must be 1D"));
    OP_REQUIRES(ctx, wcf_tensor->dim_size(0) == cell_size,
                errors::InvalidArgument(
                    "wcf.dim_size(0) != cell_size: ", wcf_tensor->dim_size(0),
                    " vs. ", cell_size));

    const Tensor* wco_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("wco", &wco_tensor));
    OP_REQUIRES(ctx, wco_tensor->dims() == 1,
                errors::InvalidArgument("wco must be 1D"));
    OP_REQUIRES(ctx, wco_tensor->dim_size(0) == cell_size,
                errors::InvalidArgument(
                    "wco.dim_size(0) != cell_size: ", wco_tensor->dim_size(0),
                    " vs. ", cell_size));

    const Tensor* b_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("b", &b_tensor));
    OP_REQUIRES(ctx, b_tensor->dims() == 1,
                errors::InvalidArgument("b must be 1D"));
    OP_REQUIRES(ctx, b_tensor->dim_size(0) == cell_size * 4,
                errors::InvalidArgument(
                    "b.dim_size(0) != cell_size * 4: ", b_tensor->dim_size(0),
                    " vs. ", cell_size * 4));

    TensorShape batch_cell_shape({timelen, batch_size, cell_size});
    Tensor* i_out;
    OP_REQUIRES_OK(ctx, ctx->allocate_output("i", batch_cell_shape, &i_out));

    Tensor* cs_out;
    OP_REQUIRES_OK(ctx, ctx->allocate_output("cs", batch_cell_shape, &cs_out));

    Tensor* f_out;
    OP_REQUIRES_OK(ctx, ctx->allocate_output("f", batch_cell_shape, &f_out));

    Tensor* o_out;
    OP_REQUIRES_OK(ctx, ctx->allocate_output("o", batch_cell_shape, &o_out));

    Tensor* ci_out;
    OP_REQUIRES_OK(ctx, ctx->allocate_output("ci", batch_cell_shape, &ci_out));

    Tensor* co_out;
    OP_REQUIRES_OK(ctx, ctx->allocate_output("co", batch_cell_shape, &co_out));

    Tensor* h_out;
    OP_REQUIRES_OK(ctx, ctx->allocate_output("h", batch_cell_shape, &h_out));

    Tensor xh_tensor;
    OP_REQUIRES_OK(ctx, ctx->allocate_temp(
                            DataTypeToEnum<T>::v(),
                            TensorShape({batch_size, input_size + cell_size}),
                            &xh_tensor));

    Tensor icfo_tensor;
    OP_REQUIRES_OK(ctx,
                   ctx->allocate_temp(DataTypeToEnum<T>::v(),
                                      TensorShape({batch_size, cell_size * 4}),
                                      &icfo_tensor));

    const Device& device = ctx->eigen_device<Device>();

    const int64 seq_len_max = seq_len_max_tensor->scalar<int64>()();
    SliceHelper<Device, T> slicer(ctx);
    for (int64 t = 0; t < seq_len_max; ++t) {
      const Tensor x_tensor = slicer.InputSlice(*x, t, "x");
      const Tensor& cs_prev_tensor2 =
          t == 0 ? *cs_prev_tensor
                 : slicer.OutputSlice(cs_out, t - 1, "cs_prev");
      const Tensor& h_prev_tensor2 =
          t == 0 ? *h_prev_tensor : slicer.OutputSlice(h_out, t - 1, "h_prev");

      Tensor i_tensor = slicer.OutputSlice(i_out, t, "i_out");
      Tensor cs_tensor = slicer.OutputSlice(cs_out, t, "cs_out");
      Tensor f_tensor = slicer.OutputSlice(f_out, t, "f_out");
      Tensor o_tensor = slicer.OutputSlice(o_out, t, "o_out");
      Tensor ci_tensor = slicer.OutputSlice(ci_out, t, "ci_out");
      Tensor co_tensor = slicer.OutputSlice(co_out, t, "co_out");
      Tensor h_tensor = slicer.OutputSlice(h_out, t, "h_out");

      functor::LSTMBlockCellFprop<Device, T, USE_CUBLAS>(batch_size, input_size,
                                                         cell_size)(
          ctx, device, forget_bias_, cell_clip_, use_peephole_,
          x_tensor.matrix<T>(), cs_prev_tensor2.matrix<T>(),
          h_prev_tensor2.matrix<T>(), w_tensor->matrix<T>(),
          wci_tensor->vec<T>(), wcf_tensor->vec<T>(), wco_tensor->vec<T>(),
          b_tensor->vec<T>(), xh_tensor.matrix<T>(), i_tensor.matrix<T>(),
          cs_tensor.matrix<T>(), f_tensor.matrix<T>(), o_tensor.matrix<T>(),
          ci_tensor.matrix<T>(), co_tensor.matrix<T>(), icfo_tensor.matrix<T>(),
          h_tensor.matrix<T>());
      slicer.FinishTimeStep();
    }

    if (seq_len_max < timelen) {
      Tensor cs_tensor = cs_out->Slice(seq_len_max, timelen);
      Tensor h_tensor = h_out->Slice(seq_len_max, timelen);

      functor::TensorUnalignedZero<Device, T>()(device,
                                                cs_tensor.unaligned_flat<T>());
      functor::TensorUnalignedZero<Device, T>()(device,
                                                h_tensor.unaligned_flat<T>());
    }
  }

 private:
  float forget_bias_;
  float cell_clip_;
  bool use_peephole_;
};

#define REGISTER_KERNEL(T)                                         \
  REGISTER_KERNEL_BUILDER(                                         \
      Name("BlockLSTM").Device(DEVICE_CPU).TypeConstraint<T>("T"), \
      BlockLSTMOp<CPUDevice, T, false>);
REGISTER_KERNEL(float);
REGISTER_KERNEL(Eigen::half);
#undef REGISTER_KERNEL

#if GOOGLE_CUDA
namespace functor {
#define DECLARE_GPU_SPEC(T)                                              \
  template <>                                                            \
  void TensorZero<GPUDevice, T>::operator()(const GPUDevice& d,          \
                                            typename TTypes<T>::Flat t); \
                                                                         \
  extern template struct TensorZero<GPUDevice, T>;                       \
                                                                         \
  template <>                                                            \
  void TensorUnalignedZero<GPUDevice, T>::operator()(                    \
      const GPUDevice& d, typename TTypes<T>::UnalignedFlat t);          \
                                                                         \
  extern template struct TensorUnalignedZero<GPUDevice, T>;

DECLARE_GPU_SPEC(float);
DECLARE_GPU_SPEC(Eigen::half);
// DECLARE_GPU_SPEC(double);
#undef DECLARE_GPU_SPEC
}  // end namespace functor

#define REGISTER_GPU_KERNEL(T)                           \
  REGISTER_KERNEL_BUILDER(Name("BlockLSTM")              \
                              .Device(DEVICE_GPU)        \
                              .HostMemory("seq_len_max") \
                              .TypeConstraint<T>("T"),   \
                          BlockLSTMOp<GPUDevice, T, true>);

REGISTER_GPU_KERNEL(float);
REGISTER_GPU_KERNEL(Eigen::half);
// REGISTER_GPU_KERNEL(double);
#undef REGISTER_GPU_KERNEL
#endif  // GOOGLE_CUDA

template <typename Device, typename T, bool USE_CUBLAS>
class BlockLSTMGradOp : public OpKernel {
 public:
  explicit BlockLSTMGradOp(OpKernelConstruction* ctx) : OpKernel(ctx) {
    OP_REQUIRES_OK(ctx, ctx->GetAttr("use_peephole", &use_peephole_));
  }

  void Compute(OpKernelContext* ctx) override {
    const Tensor* seq_len_max_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("seq_len_max", &seq_len_max_tensor));

    const Tensor* x;
    OP_REQUIRES_OK(ctx, ctx->input("x", &x));
    OP_REQUIRES(ctx, x->dims() == 3, errors::InvalidArgument("x must be 3D"));
    const int64 timelen = x->dim_size(0);
    const int64 batch_size = x->dim_size(1);
    const int64 input_size = x->dim_size(2);

    const Tensor* cs_prev_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("cs_prev", &cs_prev_tensor));

    const Tensor* h_prev_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("h_prev", &h_prev_tensor));

    const Tensor* w_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("w", &w_tensor));
    const int64 cell_size = w_tensor->dim_size(1) / 4;
    OP_REQUIRES(ctx, input_size + cell_size == w_tensor->dim_size(0),
                errors::InvalidArgument(
                    "w matrix rows don't match: ", input_size + cell_size,
                    " vs. ", w_tensor->dim_size(0)));

    const Tensor* wci_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("wci", &wci_tensor));

    const Tensor* wcf_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("wcf", &wcf_tensor));

    const Tensor* wco_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("wco", &wco_tensor));

    const Tensor* b_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("b", &b_tensor));
    OP_REQUIRES(
        ctx, cell_size == b_tensor->dim_size(0) / 4,
        errors::InvalidArgument("w and b cell_size don't match: ", cell_size,
                                " vs. ", b_tensor->dim_size(0)));

    const Tensor* i_out = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("i", &i_out));

    const Tensor* cs_out = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("cs", &cs_out));

    const Tensor* f_out = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("f", &f_out));

    const Tensor* o_out = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("o", &o_out));

    const Tensor* ci_out = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("ci", &ci_out));

    const Tensor* co_out = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("co", &co_out));

    const Tensor* h_out = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("h", &h_out));

    const Tensor* cs_grad = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("cs_grad", &cs_grad));

    const Tensor* h_grad = nullptr;
    OP_REQUIRES_OK(ctx, ctx->input("h_grad", &h_grad));

    TensorShape batch_input_shape({timelen, batch_size, input_size});
    Tensor* x_grad;
    OP_REQUIRES_OK(ctx,
                   ctx->allocate_output("x_grad", batch_input_shape, &x_grad));

    Tensor* cs_prev_grad_tensor = nullptr;
    OP_REQUIRES_OK(ctx,
                   ctx->allocate_output("cs_prev_grad", cs_prev_tensor->shape(),
                                        &cs_prev_grad_tensor));

    Tensor* h_prev_grad_tensor = nullptr;
    OP_REQUIRES_OK(ctx,
                   ctx->allocate_output("h_prev_grad", h_prev_tensor->shape(),
                                        &h_prev_grad_tensor));

    Tensor* w_grad_tensor = nullptr;
    OP_REQUIRES_OK(
        ctx, ctx->allocate_output("w_grad", w_tensor->shape(), &w_grad_tensor));

    Tensor* wci_grad_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->allocate_output("wci_grad", wci_tensor->shape(),
                                             &wci_grad_tensor));

    Tensor* wcf_grad_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->allocate_output("wcf_grad", wcf_tensor->shape(),
                                             &wcf_grad_tensor));

    Tensor* wco_grad_tensor = nullptr;
    OP_REQUIRES_OK(ctx, ctx->allocate_output("wco_grad", wco_tensor->shape(),
                                             &wco_grad_tensor));

    Tensor* b_grad_tensor = nullptr;
    OP_REQUIRES_OK(
        ctx, ctx->allocate_output("b_grad", b_tensor->shape(), &b_grad_tensor));

    TensorShape batch_cell_shape({batch_size, cell_size});

    Tensor xh_tensor;
    OP_REQUIRES_OK(ctx, ctx->allocate_temp(
                            DataTypeToEnum<T>::v(),
                            TensorShape({batch_size, input_size + cell_size}),
                            &xh_tensor));

    Tensor xh_grad_tensor;
    OP_REQUIRES_OK(ctx, ctx->allocate_temp(DataTypeToEnum<T>::v(),
                                           xh_tensor.shape(), &xh_grad_tensor));

    Tensor do_tensor;
    OP_REQUIRES_OK(ctx, ctx->allocate_temp(DataTypeToEnum<T>::v(),
                                           batch_cell_shape, &do_tensor));

    Tensor dcs_tensor;
    OP_REQUIRES_OK(ctx, ctx->allocate_temp(DataTypeToEnum<T>::v(),
                                           batch_cell_shape, &dcs_tensor));

    Tensor dci_tensor;
    OP_REQUIRES_OK(ctx, ctx->allocate_temp(DataTypeToEnum<T>::v(),
                                           batch_cell_shape, &dci_tensor));

    Tensor df_tensor;
    OP_REQUIRES_OK(ctx, ctx->allocate_temp(DataTypeToEnum<T>::v(),
                                           batch_cell_shape, &df_tensor));

    Tensor di_tensor;
    OP_REQUIRES_OK(ctx, ctx->allocate_temp(DataTypeToEnum<T>::v(),
                                           batch_cell_shape, &di_tensor));

    Tensor dicfo_tensor;
    OP_REQUIRES_OK(ctx,
                   ctx->allocate_temp(DataTypeToEnum<T>::v(),
                                      TensorShape({batch_size, cell_size * 4}),
                                      &dicfo_tensor));

    Tensor cs_grad_tensor;
    OP_REQUIRES_OK(ctx, ctx->allocate_temp(DataTypeToEnum<T>::v(),
                                           batch_cell_shape, &cs_grad_tensor));

    Tensor h_grad_tensor;
    OP_REQUIRES_OK(ctx, ctx->allocate_temp(DataTypeToEnum<T>::v(),
                                           batch_cell_shape, &h_grad_tensor));

    const Device& device = ctx->eigen_device<Device>();

    functor::TensorZero<Device, T>()(device, cs_grad_tensor.flat<T>());
    functor::TensorZero<Device, T>()(device, cs_prev_grad_tensor->flat<T>());
    functor::TensorZero<Device, T>()(device, h_grad_tensor.flat<T>());
    functor::TensorZero<Device, T>()(device, h_prev_grad_tensor->flat<T>());
    functor::TensorZero<Device, T>()(device, w_grad_tensor->flat<T>());
    functor::TensorZero<Device, T>()(device, wci_grad_tensor->flat<T>());
    functor::TensorZero<Device, T>()(device, wcf_grad_tensor->flat<T>());
    functor::TensorZero<Device, T>()(device, wco_grad_tensor->flat<T>());
    functor::TensorZero<Device, T>()(device, b_grad_tensor->flat<T>());

    const int64 seq_len_max = seq_len_max_tensor->scalar<int64>()();
    SliceHelper<Device, T> slicer(ctx);
    for (int64 t = seq_len_max - 1; t >= 0; --t) {
      const Tensor& x_tensor = slicer.InputSlice(*x, t, "x");
      const Tensor& cs_prev_tensor2 =
          t == 0 ? *cs_prev_tensor
                 : slicer.InputSlice(*cs_out, t - 1, "cs_prev");
      const Tensor& h_prev_tensor2 =
          t == 0 ? *h_prev_tensor : slicer.InputSlice(*h_out, t - 1, "h_prev");
      const Tensor& i_tensor = slicer.InputSlice(*i_out, t, "i_out");
      const Tensor& cs_tensor = slicer.InputSlice(*cs_out, t, "cs_out");
      const Tensor& f_tensor = slicer.InputSlice(*f_out, t, "f_out");
      const Tensor& o_tensor = slicer.InputSlice(*o_out, t, "o_out");
      const Tensor& ci_tensor = slicer.InputSlice(*ci_out, t, "ci_out");
      const Tensor& co_tensor = slicer.InputSlice(*co_out, t, "co_out");

      // Grab previous CS grad.
      const Tensor& const_cs_prev_grad_tensor = *cs_prev_grad_tensor;
      const Tensor const_cs_grad_slice =
          slicer.InputSlice(*cs_grad, t, "cs_grad");
      functor::TensorAdd<Device, T>()(
          device, const_cs_prev_grad_tensor.flat<T>(),
          const_cs_grad_slice.flat<T>(), cs_grad_tensor.flat<T>());

      // Combine previous h grad and h grad coming on top.
      const Tensor& const_h_prev_grad_tensor = *h_prev_grad_tensor;
      const Tensor const_h_grad_slice = slicer.InputSlice(*h_grad, t, "h_grad");
      functor::TensorAdd<Device, T>()(
          device, const_h_prev_grad_tensor.flat<T>(),
          const_h_grad_slice.flat<T>(), h_grad_tensor.flat<T>());

      const Tensor& const_cs_grad_tensor = cs_grad_tensor;
      const Tensor& const_h_grad_tensor = h_grad_tensor;

      Tensor x_grad_tensor = slicer.OutputSlice(x_grad, t, "x_grad");
      functor::BlockLSTMBprop<Device, T, USE_CUBLAS>(batch_size, input_size,
                                                     cell_size)(
          ctx, device, use_peephole_, x_tensor.matrix<T>(),
          cs_prev_tensor2.matrix<T>(), h_prev_tensor2.matrix<T>(),
          w_tensor->matrix<T>(), wci_tensor->vec<T>(), wcf_tensor->vec<T>(),
          wco_tensor->vec<T>(), b_tensor->vec<T>(), xh_tensor.matrix<T>(),
          i_tensor.matrix<T>(), cs_tensor.matrix<T>(), f_tensor.matrix<T>(),
          o_tensor.matrix<T>(), ci_tensor.matrix<T>(), co_tensor.matrix<T>(),
          const_cs_grad_tensor.matrix<T>(), const_h_grad_tensor.matrix<T>(),
          do_tensor.matrix<T>(), dcs_tensor.matrix<T>(), dci_tensor.matrix<T>(),
          df_tensor.matrix<T>(), di_tensor.matrix<T>(),
          dicfo_tensor.matrix<T>(), cs_prev_grad_tensor->matrix<T>(),
          h_prev_grad_tensor->matrix<T>(), xh_grad_tensor.matrix<T>(),
          x_grad_tensor.matrix<T>(), w_grad_tensor->matrix<T>(),
          wci_grad_tensor->vec<T>(), wcf_grad_tensor->vec<T>(),
          wco_grad_tensor->vec<T>(), b_grad_tensor->vec<T>());
      slicer.FinishTimeStep();
    }

    if (seq_len_max < timelen) {
      Tensor x_grad_tensor = x_grad->Slice(seq_len_max, timelen);
      functor::TensorUnalignedZero<Device, T>()(
          device, x_grad_tensor.unaligned_flat<T>());
    }
  }

 private:
  bool use_peephole_;
};

#define REGISTER_KERNEL(T)                                             \
  REGISTER_KERNEL_BUILDER(                                             \
      Name("BlockLSTMGrad").Device(DEVICE_CPU).TypeConstraint<T>("T"), \
      BlockLSTMGradOp<CPUDevice, T, false>);
REGISTER_KERNEL(float);
REGISTER_KERNEL(Eigen::half);
#undef REGISTER_KERNEL

#if GOOGLE_CUDA
namespace functor {
#define DECLARE_GPU_SPEC(T)                                                    \
  template <>                                                                  \
  void TensorCopy<GPUDevice, T>::operator()(const GPUDevice& d,                \
                                            typename TTypes<T>::ConstFlat src, \
                                            typename TTypes<T>::Flat dst);     \
                                                                               \
  template <>                                                                  \
  void TensorCopyUnaligned<GPUDevice, T>::operator()(                          \
      const GPUDevice& d, typename TTypes<T>::UnalignedConstFlat src,          \
      typename TTypes<T>::Flat dst);                                           \
                                                                               \
  template <>                                                                  \
  void TensorCopyToUnaligned<GPUDevice, T>::operator()(                        \
      const GPUDevice& d, typename TTypes<T>::ConstFlat src,                   \
      typename TTypes<T>::UnalignedFlat dst);                                  \
                                                                               \
  template <>                                                                  \
  void TensorAdd<GPUDevice, T>::operator()(                                    \
      const GPUDevice& d, typename TTypes<T>::ConstFlat a,                     \
      typename TTypes<T>::ConstFlat b, typename TTypes<T>::Flat c);            \
                                                                               \
  template <>                                                                  \
  void BlockLSTMBprop<GPUDevice, T, true>::operator()(                         \
      OpKernelContext* ctx, const GPUDevice& d, bool use_peephole,             \
      typename TTypes<T>::ConstMatrix x,                                       \
      typename TTypes<T>::ConstMatrix cs_prev,                                 \
      typename TTypes<T>::ConstMatrix h_prev,                                  \
      typename TTypes<T>::ConstMatrix w, typename TTypes<T>::ConstVec wci,     \
      typename TTypes<T>::ConstVec wcf, typename TTypes<T>::ConstVec wco,      \
      typename TTypes<T>::ConstVec b, typename TTypes<T>::Matrix xh,           \
      typename TTypes<T>::ConstMatrix i, typename TTypes<T>::ConstMatrix cs,   \
      typename TTypes<T>::ConstMatrix f, typename TTypes<T>::ConstMatrix o,    \
      typename TTypes<T>::ConstMatrix ci, typename TTypes<T>::ConstMatrix co,  \
      typename TTypes<T>::ConstMatrix cs_grad,                                 \
      typename TTypes<T>::ConstMatrix h_grad, typename TTypes<T>::Matrix do_,  \
      typename TTypes<T>::Matrix dcs, typename TTypes<T>::Matrix dci,          \
      typename TTypes<T>::Matrix df, typename TTypes<T>::Matrix di,            \
      typename TTypes<T>::Matrix dicfo,                                        \
      typename TTypes<T>::Matrix cs_prev_grad,                                 \
      typename TTypes<T>::Matrix h_prev_grad,                                  \
      typename TTypes<T>::Matrix xh_grad, typename TTypes<T>::Matrix x_grad,   \
      typename TTypes<T>::Matrix w_grad, typename TTypes<T>::Vec wci_grad,     \
      typename TTypes<T>::Vec wcf_grad, typename TTypes<T>::Vec wco_grad,      \
      typename TTypes<T>::Vec b_grad);                                         \
                                                                               \
  extern template struct TensorCopy<GPUDevice, T>;                             \
  extern template struct TensorAdd<GPUDevice, T>;                              \
  extern template struct BlockLSTMBprop<GPUDevice, T, true>;

DECLARE_GPU_SPEC(float);
DECLARE_GPU_SPEC(Eigen::half);
// DECLARE_GPU_SPEC(double);
#undef DECLARE_GPU_SPEC
}  // end namespace functor

#define REGISTER_GPU_KERNEL(T)                           \
  REGISTER_KERNEL_BUILDER(Name("BlockLSTMGrad")          \
                              .Device(DEVICE_GPU)        \
                              .HostMemory("seq_len_max") \
                              .TypeConstraint<T>("T"),   \
                          BlockLSTMGradOp<GPUDevice, T, true>);

REGISTER_GPU_KERNEL(float);
REGISTER_GPU_KERNEL(Eigen::half);
// REGISTER_GPU_KERNEL(double);
#undef REGISTER_GPU_KERNEL
#endif  // GOOGLE_CUDA

}  // end namespace tensorflow
