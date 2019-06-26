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
#include <stdlib.h>
#include <string.h>
#include <algorithm>

#include "tensorflow/lite/c/builtin_op_data.h"
#include "tensorflow/lite/kernels/activation_functor.h"
#include "tensorflow/lite/kernels/internal/round.h"
#include "tensorflow/lite/kernels/op_macros.h"

#if defined(_MSC_VER)
#define __restrict__ __restrict
#endif

namespace tflite {
namespace tensor_utils {

float PortableClip(float f, float abs_limit) {
  float result = (abs_limit < f) ? abs_limit : f;
  result = (-abs_limit > result) ? -abs_limit : result;
  return result;
}

bool PortableIsZeroVector(const float* vector, int v_size) {
  for (int i = 0; i < v_size; ++i) {
    if (*vector++ != 0.0f) return false;
  }
  return true;
}

void PortableSymmetricQuantizeFloats(const float* values, const int size,
                                     int8_t* quantized_values, float* min_value,
                                     float* max_value, float* scaling_factor) {
  auto minmax = std::minmax_element(values, values + size);
  *min_value = *minmax.first;
  *max_value = *minmax.second;
  const int kScale = 127;
  const float range = std::max(std::abs(*min_value), std::abs(*max_value));
  if (range == 0) {
    memset(quantized_values, 0, size * sizeof(int8_t));
    *scaling_factor = 1;
    return;
  }
  *scaling_factor = range / kScale;
  const float scaling_factor_inv = kScale / range;
  for (int i = 0; i < size; ++i) {
    const int32_t quantized_value =
        static_cast<int32_t>(TfLiteRound(values[i] * scaling_factor_inv));
    // Clamp: just in case some odd numeric offset.
    quantized_values[i] = std::min(kScale, std::max(-kScale, quantized_value));
  }
}

void PortableMatrixBatchVectorMultiplyAccumulate(const float* matrix,
                                                 int m_rows, int m_cols,
                                                 const float* vector,
                                                 int n_batch, float* result,
                                                 int result_stride) {
  float* result_in_batch = result;
  for (int b = 0; b < n_batch; b++) {
    const float* matrix_ptr = matrix;
    for (int r = 0; r < m_rows; r++) {
      float dot_prod = 0.0f;
      const float* vector_in_batch = vector + b * m_cols;
      for (int c = 0; c < m_cols; c++) {
        dot_prod += *matrix_ptr++ * *vector_in_batch++;
      }
      *result_in_batch += dot_prod;
      result_in_batch += result_stride;
    }
  }
}

void PortableMatrixBatchVectorMultiplyAccumulate(
    const int8_t* __restrict__ matrix, const int m_rows, const int m_cols,
    const int8_t* __restrict__ vectors, const float* scaling_factors,
    int n_batch, float* __restrict__ result, int result_stride) {
  int batch, row, col;
  for (batch = 0; batch < n_batch; ++batch, vectors += m_cols) {
    const float batch_scaling_factor = scaling_factors[batch];
    // Get the address of the first row.
    const int8_t* row_ptr = matrix;
    for (row = 0; row < m_rows; ++row, result += result_stride) {
      // Initialize the dot product sum for the row to 0.
      int32_t dotprod = 0;
#if defined(__GNUC__)
      // Prefetch the row to cache.
      __builtin_prefetch(row_ptr, 0 /* prefetch for read */,
                         3 /* temporal locality */);
#endif
      // For every block of 16 8-bit elements (128-bit register) from each row.
      for (col = 0; col < m_cols; ++col, ++row_ptr) {
        dotprod += (*row_ptr) * (vectors[col]);
      }  // for col
      *result += (dotprod * batch_scaling_factor);
    }  // for row
  }    // for batch
}

void PortableVectorVectorCwiseProduct(const float* vector1,
                                      const float* vector2, int v_size,
                                      float* result) {
  for (int v = 0; v < v_size; v++) {
    *result++ = *vector1++ * *vector2++;
  }
}

float PortableVectorVectorDotProduct(const float* vector1, const float* vector2,
                                     int v_size) {
  float result = 0.0;
  for (int v = 0; v < v_size; v++) {
    result += *vector1++ * *vector2++;
  }
  return result;
}

void PortableBatchVectorBatchVectorDotProduct(const float* vector1,
                                              const float* vector2, int v_size,
                                              int n_batch, float* result,
                                              int result_stride) {
  float* result_ptr = result;
  const float* vector1_ptr = vector1;
  const float* vector2_ptr = vector2;
  for (int b = 0; b < n_batch; b++) {
    *result_ptr =
        PortableVectorVectorDotProduct(vector1_ptr, vector2_ptr, v_size);
    vector1_ptr += v_size;
    vector2_ptr += v_size;
    result_ptr += result_stride;
  }
}

void PortableVectorVectorCwiseProductAccumulate(const float* vector1,
                                                const float* vector2,
                                                int v_size, float* result) {
  for (int v = 0; v < v_size; v++) {
    *result++ += *vector1++ * *vector2++;
  }
}

void PortableVectorBatchVectorCwiseProduct(const float* vector, int v_size,
                                           const float* batch_vector,
                                           int n_batch, float* result) {
  for (int b = 0; b < n_batch; b++) {
    for (int v = 0; v < v_size; v++) {
      *result++ = vector[v] * *batch_vector++;
    }
  }
}

void PortableVectorBatchVectorCwiseProductAccumulate(const float* vector,
                                                     int v_size,
                                                     const float* batch_vector,
                                                     int n_batch,
                                                     float* result) {
  for (int b = 0; b < n_batch; b++) {
    for (int v = 0; v < v_size; v++) {
      *result++ += vector[v] * *batch_vector++;
    }
  }
}

void PortableVectorBatchVectorAdd(const float* vector, int v_size, int n_batch,
                                  float* batch_vector) {
  for (int b = 0; b < n_batch; b++) {
    for (int i = 0; i < v_size; ++i) {
      batch_vector[i] += vector[i];
    }
    batch_vector += v_size;
  }
}

void PortableVectorBatchVectorAssign(const float* vector, int v_size,
                                     int n_batch, float* batch_vector) {
  for (int b = 0; b < n_batch; b++) {
    memcpy(batch_vector + b * v_size, vector, v_size * sizeof(float));
  }
}

void PortableApplySigmoidToVector(const float* vector, int v_size,
                                  float* result) {
  auto sigmoid_func = ActivationFunctor(kTfLiteActSigmoid);
  for (int v = 0; v < v_size; v++) {
    *result++ = (sigmoid_func)(*vector++);
  }
}

void PortableApplyActivationToVector(const float* vector, int v_size,
                                     TfLiteFusedActivation activation,
                                     float* result) {
  auto activation_func = ActivationFunctor(activation);
  for (int v = 0; v < v_size; v++) {
    *result++ = (activation_func)(*vector++);
  }
}

void PortableCopyVector(const float* vector, int v_size, float* result) {
  memcpy(result, vector, v_size * sizeof(float));
}

void PortableSub1Vector(const float* vector, int v_size, float* result) {
  for (int v = 0; v < v_size; v++) {
    *result++ = 1.0f - *vector++;
  }
}

void PortableZeroVector(float* vector, int v_size) {
  memset(vector, 0, v_size * sizeof(float));
}

void PortableVectorScalarMultiply(const int8_t* vector, const int v_size,
                                  const float scale, float* result) {
  for (int v = 0; v < v_size; ++v) {
    *result++ = scale * *vector++;
  }
}

void PortableClipVector(const float* vector, int v_size, float abs_limit,
                        float* result) {
  for (int v = 0; v < v_size; v++) {
    *result++ = PortableClip(*vector++, abs_limit);
  }
}

void PortableVectorShiftLeft(float* vector, int v_size, float shift_value) {
  TF_LITE_ASSERT(v_size > 0);
  for (int i = 0; i < v_size - 1; i++) {
    vector[i] = vector[i + 1];
  }
  vector[v_size - 1] = shift_value;
}

void PortableReductionSumVector(const float* input_vector, float* output_vector,
                                int output_size, int reduction_size) {
  const float* input_vector_ptr = input_vector;
  for (int o = 0; o < output_size; o++) {
    for (int r = 0; r < reduction_size; r++) {
      output_vector[o] += *input_vector_ptr++;
    }
  }
}

void PortableMeanStddevNormalization(const float* input_vector,
                                     float* output_vector, int v_size,
                                     int n_batch, float normalization_epsilon) {
  for (int batch = 0; batch < n_batch; ++batch) {
    float sum = 0.0f;
    float sum_sq = 0.0f;
    for (int i = 0; i < v_size; ++i) {
      sum += input_vector[i];
      sum_sq += input_vector[i] * input_vector[i];
    }
    const float mean = sum / v_size;
    float stddev_inv = 0.0f;
    const float variance = sum_sq / v_size - mean * mean;
    if (variance == 0) {
      stddev_inv = 1.0f / sqrt(normalization_epsilon);
    } else {
      stddev_inv = 1.0f / sqrt(variance);
    }
    for (int i = 0; i < v_size; ++i) {
      output_vector[i] = (input_vector[i] - mean) * stddev_inv;
    }
    input_vector += v_size;
    output_vector += v_size;
  }
}

}  // namespace tensor_utils
}  // namespace tflite
