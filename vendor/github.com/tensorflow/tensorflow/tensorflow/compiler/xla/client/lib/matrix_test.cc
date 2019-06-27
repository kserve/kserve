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

#include "tensorflow/compiler/xla/client/lib/matrix.h"

#include "tensorflow/compiler/xla/client/lib/slicing.h"
#include "tensorflow/compiler/xla/client/xla_builder.h"
#include "tensorflow/compiler/xla/test.h"
#include "tensorflow/compiler/xla/tests/client_library_test_base.h"
#include "tensorflow/compiler/xla/tests/test_macros.h"
#include "tensorflow/compiler/xla/types.h"
#include "tensorflow/compiler/xla/xla_data.pb.h"

namespace xla {
namespace {

class MatrixTest : public ClientLibraryTestBase {
 protected:
  template <typename T>
  void TestMatrixDiagonal();
};

XLA_TEST_F(MatrixTest, Triangle) {
  XlaBuilder builder(TestName());
  Array3D<int32> input(2, 3, 4);
  input.FillIota(0);

  XlaOp a;
  auto a_data = CreateR3Parameter<int32>(input, 0, "a", &builder, &a);
  LowerTriangle(a);
  Array3D<int32> expected({{{0, 0, 0, 0}, {4, 5, 0, 0}, {8, 9, 10, 0}},
                           {{12, 0, 0, 0}, {16, 17, 0, 0}, {20, 21, 22, 0}}});

  ComputeAndCompareR3<int32>(&builder, expected, {a_data.get()});
}

template <typename T>
void MatrixTest::TestMatrixDiagonal() {
  XlaBuilder builder("GetMatrixDiagonal");
  Array3D<T> input(2, 3, 4);
  input.FillIota(0);

  XlaOp a;
  auto a_data = CreateR3Parameter<T>(input, 0, "a", &builder, &a);
  GetMatrixDiagonal(a);
  Array2D<T> expected({{0, 5, 10}, {12, 17, 22}});

  ComputeAndCompareR2<T>(&builder, expected, {a_data.get()});
}

XLA_TEST_F(MatrixTest, GetMatrixDiagonal_S32) { TestMatrixDiagonal<int32>(); }

XLA_TEST_F(MatrixTest, GetMatrixDiagonal_S64) { TestMatrixDiagonal<int64>(); }

XLA_TEST_F(MatrixTest, GetMatrixDiagonal_F32) { TestMatrixDiagonal<float>(); }

Array3D<float> BatchedAValsFull() {
  return {{
              {2, 0, 1, 2},
              {3, 6, 0, 1},
              {4, 7, 9, 0},
              {5, 8, 10, 11},
          },
          {
              {16, 24, 8, 12},
              {24, 61, 82, 48},
              {8, 82, 456, 106},
              {12, 48, 106, 62},
          }};
}

XLA_TEST_F(MatrixTest, RowBatchDot) {
  XlaBuilder builder(TestName());

  int n = 4;

  XlaOp a, row, index;
  auto a_data =
      CreateR3Parameter<float>(BatchedAValsFull(), 0, "a", &builder, &a);
  auto row_data = CreateR3Parameter<float>({{{9, 1, 0, 0}}, {{2, 4, 0, 0}}}, 1,
                                           "row", &builder, &row);
  // Select {{3, 6, 0, 1}, {24, 61,  82,  48}} out of BatchedAValsFull().
  auto index_data = CreateR0Parameter<int>(1, 2, "index", &builder, &index);

  auto l_index = DynamicSliceInMinorDims(
      a, {index, ConstantR0<int32>(&builder, 0)}, {1, n});
  BatchDot(l_index, TransposeInMinorDims(row));

  ComputeAndCompareR3<float>(&builder, {{{33}}, {{292}}},
                             {a_data.get(), row_data.get(), index_data.get()});
}
}  // namespace
}  // namespace xla
