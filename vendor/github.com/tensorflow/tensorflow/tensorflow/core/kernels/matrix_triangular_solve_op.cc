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

// See docs in ../ops/linalg_ops.cc.

#include "third_party/eigen3/Eigen/Core"
#include "tensorflow/core/framework/kernel_def_builder.h"
#include "tensorflow/core/framework/op_kernel.h"
#include "tensorflow/core/framework/tensor_shape.h"
#include "tensorflow/core/kernels/linalg_ops_common.h"
#include "tensorflow/core/lib/core/errors.h"
#include "tensorflow/core/platform/logging.h"
#include "tensorflow/core/platform/macros.h"
#include "tensorflow/core/platform/types.h"

#if GOOGLE_CUDA
#include "tensorflow/core/platform/stream_executor.h"
#endif  // GOOGLE_CUDA

namespace tensorflow {

#if GOOGLE_CUDA
namespace {
template <typename Scalar>
se::DeviceMemory<Scalar> AsDeviceMemory(const Scalar* cuda_memory) {
  se::DeviceMemoryBase wrapped(const_cast<Scalar*>(cuda_memory));
  se::DeviceMemory<Scalar> typed(wrapped);
  return typed;
}
}  // namespace
#endif  // GOOGLE_CUDA

template <class Scalar>
class MatrixTriangularSolveOp : public LinearAlgebraOp<Scalar> {
 public:
  INHERIT_LINALG_TYPEDEFS(Scalar);

  explicit MatrixTriangularSolveOp(OpKernelConstruction* context)
      : Base(context), lower_(true), adjoint_(false) {
    OP_REQUIRES_OK(context, context->GetAttr("lower", &lower_));
    OP_REQUIRES_OK(context, context->GetAttr("adjoint", &adjoint_));
  }

  void ValidateInputMatrixShapes(
      OpKernelContext* context,
      const TensorShapes& input_matrix_shapes) const final {
    Base::ValidateSquareSolver(context, input_matrix_shapes);
  }

  TensorShapes GetOutputMatrixShapes(
      const TensorShapes& input_matrix_shapes) const final {
    return TensorShapes({TensorShape({input_matrix_shapes[0].dim_size(1),
                                      input_matrix_shapes[1].dim_size(1)})});
  }

  int64 GetCostPerUnit(const TensorShapes& input_matrix_shapes) const final {
    double rows = static_cast<double>(input_matrix_shapes[0].dim_size(0));
    double num_rhss = static_cast<double>(input_matrix_shapes[1].dim_size(1));
    double cost = rows * rows * num_rhss *
                  (Eigen::TensorOpCost::AddCost<Scalar>() +
                   Eigen::TensorOpCost::MulCost<Scalar>());
    return cost >= static_cast<double>(kint64max) ? kint64max
                                                  : static_cast<int64>(cost);
  }

  bool EnableInputForwarding() const final { return false; }

  void ComputeMatrix(OpKernelContext* context, const ConstMatrixMaps& inputs,
                     MatrixMaps* outputs) final {
    const ConstMatrixMap& matrix = inputs[0];
    const ConstMatrixMap& rhs = inputs[1];
    MatrixMap& output = outputs->at(0);

    if (matrix.rows() == 0 || rhs.cols() == 0) {
      // To be consistent with the MatrixInverse op, we define the solution for
      // an empty set of equation as the empty matrix.
      return;
    }
    const RealScalar min_abs_pivot = matrix.diagonal().cwiseAbs().minCoeff();
    OP_REQUIRES(context, min_abs_pivot > RealScalar(0),
                errors::InvalidArgument("Input matrix is not invertible."));
    if (lower_) {
      auto triangle = matrix.template triangularView<Eigen::Lower>();
      if (adjoint_) {
        output.noalias() = triangle.adjoint().solve(rhs);
      } else {
        output.noalias() = triangle.solve(rhs);
      }
    } else {
      auto triangle = matrix.template triangularView<Eigen::Upper>();
      if (adjoint_) {
        output.noalias() = triangle.adjoint().solve(rhs);
      } else {
        output.noalias() = triangle.solve(rhs);
      }
    }
  }

 private:
  bool lower_;
  bool adjoint_;

  TF_DISALLOW_COPY_AND_ASSIGN(MatrixTriangularSolveOp);
};

REGISTER_LINALG_OP_CPU("MatrixTriangularSolve",
                       (MatrixTriangularSolveOp<float>), float);
REGISTER_LINALG_OP_CPU("MatrixTriangularSolve",
                       (MatrixTriangularSolveOp<double>), double);
REGISTER_LINALG_OP_CPU("MatrixTriangularSolve",
                       (MatrixTriangularSolveOp<complex64>), complex64);
REGISTER_LINALG_OP_CPU("MatrixTriangularSolve",
                       (MatrixTriangularSolveOp<complex128>), complex128);
REGISTER_LINALG_OP_CPU("BatchMatrixTriangularSolve",
                       (MatrixTriangularSolveOp<float>), float);
REGISTER_LINALG_OP_CPU("BatchMatrixTriangularSolve",
                       (MatrixTriangularSolveOp<double>), double);

#ifdef GOOGLE_CUDA

// TODO(rmlarsen): Re-factor to
// 1. Enable buffer forwarding from rhs->out.
// 2. Save Memcpy when buffer forwarding is used.
// 3. Copy entire rhs in a single Memcpy when forwarding is not used.
template <class Scalar>
class MatrixTriangularSolveOpGPU : public LinearAlgebraOp<Scalar> {
 public:
  INHERIT_LINALG_TYPEDEFS(Scalar);

  explicit MatrixTriangularSolveOpGPU(OpKernelConstruction* context)
      : Base(context), lower_(true), adjoint_(false) {
    OP_REQUIRES_OK(context, context->GetAttr("lower", &lower_));
    OP_REQUIRES_OK(context, context->GetAttr("adjoint", &adjoint_));
  }

  void ValidateInputMatrixShapes(
      OpKernelContext* context,
      const TensorShapes& input_matrix_shapes) const final {
    Base::ValidateSquareSolver(context, input_matrix_shapes);
  }

  TensorShapes GetOutputMatrixShapes(
      const TensorShapes& input_matrix_shapes) const final {
    return TensorShapes({TensorShape({input_matrix_shapes[0].dim_size(1),
                                      input_matrix_shapes[1].dim_size(1)})});
  }

  int64 GetCostPerUnit(const TensorShapes& input_matrix_shapes) const final {
    double rows = static_cast<double>(input_matrix_shapes[0].dim_size(0));
    double num_rhss = static_cast<double>(input_matrix_shapes[1].dim_size(1));
    double cost = rows * rows * num_rhss *
                  (Eigen::TensorOpCost::AddCost<Scalar>() +
                   Eigen::TensorOpCost::MulCost<Scalar>());
    return cost >= static_cast<double>(kint64max) ? kint64max
                                                  : static_cast<int64>(cost);
  }

  bool EnableInputForwarding() const final { return false; }

  void ComputeMatrix(OpKernelContext* context, const ConstMatrixMaps& inputs,
                     MatrixMaps* outputs) final {
    const ConstMatrixMap& matrix = inputs[0];
    const ConstMatrixMap& rhs = inputs[1];
    MatrixMap& output = outputs->at(0);

    if (matrix.rows() == 0 || rhs.cols() == 0) {
      // To be consistent with the MatrixInverse op, we define the solution for
      // an empty set of equation as the empty matrix.
      return;
    }

    auto matrix_ptr = AsDeviceMemory(matrix.data());
    auto rhs_ptr = AsDeviceMemory(rhs.data());
    auto out_ptr = AsDeviceMemory(output.data());

    auto* stream = context->op_device_context()->stream();
    uint64 rhs_elems = rhs.rows() * rhs.cols();
    bool copy_status =
        stream->ThenMemcpyD2D(&out_ptr, rhs_ptr, sizeof(Scalar) * rhs_elems)
            .ok();
    if (!copy_status) {
      context->SetStatus(
          errors::Internal("Failed to copy rhs into output before solve"));
    }

    // Cublas does
    // output = matrix \ rhs
    // where matrix, rhs and output are assumed to be in column major.
    // We want the output to be in row-major, so we can compute
    // output' = rhs' / matrix' (' stands for transpose)
    // Upper/lower needs to be swapped for this.

    se::blas::UpperLower upper_lower_matrix;
    se::blas::Transpose transpose_matrix;
    if (lower_) {
      upper_lower_matrix = se::blas::UpperLower::kUpper;
    } else {
      upper_lower_matrix = se::blas::UpperLower::kLower;
    }
    if (adjoint_) {
      transpose_matrix = se::blas::Transpose::kConjugateTranspose;
    } else {
      transpose_matrix = se::blas::Transpose::kNoTranspose;
    }
    uint64 leading_dim_matrix = matrix.cols();
    uint64 leading_dim_output = output.cols();
    uint64 colmajor_rows = output.cols();
    uint64 colmajor_cols = output.rows();
    bool blas_launch_status =
        stream
            ->ThenBlasTrsm(
                se::blas::Side::kRight /*side*/, upper_lower_matrix /*uplo*/,
                transpose_matrix /*trans*/,
                se::blas::Diagonal::kNonUnit /*diag*/, colmajor_rows /*m*/,
                colmajor_cols /*n*/, Scalar(1.0) /*alpha*/, matrix_ptr,
                leading_dim_matrix /*lda*/, &out_ptr,
                leading_dim_output /*ldb*/)
            .ok();
    if (!blas_launch_status) {
      context->SetStatus(errors::Internal("Blas TRSM launch failed"));
    }
  }

 private:
  bool lower_;
  bool adjoint_;

  TF_DISALLOW_COPY_AND_ASSIGN(MatrixTriangularSolveOpGPU);
};

REGISTER_LINALG_OP_GPU("MatrixTriangularSolve",
                       (MatrixTriangularSolveOpGPU<float>), float);
REGISTER_LINALG_OP_GPU("MatrixTriangularSolve",
                       (MatrixTriangularSolveOpGPU<double>), double);
REGISTER_LINALG_OP_GPU("MatrixTriangularSolve",
                       (MatrixTriangularSolveOpGPU<complex64>), complex64);
REGISTER_LINALG_OP_GPU("MatrixTriangularSolve",
                       (MatrixTriangularSolveOpGPU<complex128>), complex128);
REGISTER_LINALG_OP_GPU("BatchMatrixTriangularSolve",
                       (MatrixTriangularSolveOpGPU<float>), float);
REGISTER_LINALG_OP_GPU("BatchMatrixTriangularSolve",
                       (MatrixTriangularSolveOpGPU<double>), double);

#endif  // GOOGLE_CUDA

}  // namespace tensorflow
